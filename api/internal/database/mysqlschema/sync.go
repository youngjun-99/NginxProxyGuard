package mysqlschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"nginx-proxy-guard/internal/database/dialect"
)

// SyncReport says what Sync did.
type SyncReport struct {
	Executed []string    // DDL that ran
	Failed   []SyncError // DDL the server refused
	Drift    []string    // differences Sync does not repair (it never alters existing objects)
	Created  []string    // tables created by this run
}

// SyncError is one refused statement.
type SyncError struct {
	Statement string
	Err       error
}

func (e SyncError) Error() string {
	return fmt.Sprintf("%v (statement: %s)", e.Err, oneLine(truncate(e.Statement)))
}

// Err folds the failures into one error, or nil.
func (r *SyncReport) Err() error {
	if len(r.Failed) == 0 {
		return nil
	}
	errs := make([]error, len(r.Failed))
	for i, f := range r.Failed {
		errs[i] = f
	}
	return errors.Join(errs...)
}

// liveSchema is what information_schema says the connected database has.
type liveSchema struct {
	tables      map[string]bool
	views       map[string]bool
	columns     map[string]map[string]string // table -> column -> COLUMN_TYPE
	viewColumns map[string][]string
	indexes     map[string]map[string]bool // table -> index name
	constraints map[string]bool            // FK and CHECK names, schema-wide
}

// Sync brings the connected MySQL database up to the model.
//
// It only ever adds: missing tables (whole, with their keys and checks),
// missing columns, indexes, checks and foreign keys, and views whose column
// list changed. It never drops, retypes or rewrites an existing object —
// a schema change that needs that is a migration of its own — and reports
// such differences as drift. Every statement runs on its own, so one the
// server refuses does not stop the rest; the refusals are in the report.
// Running it again changes nothing.
func (s *Schema) Sync(ctx context.Context, db *sql.DB) (*SyncReport, error) {
	live, err := readLive(ctx, db)
	if err != nil {
		return nil, err
	}
	report := &SyncReport{}
	exec := func(stmt string) bool {
		if _, err := db.ExecContext(ctx, dialect.Raw(stmt)); err != nil {
			report.Failed = append(report.Failed, SyncError{Statement: stmt, Err: err})
			return false
		}
		report.Executed = append(report.Executed, stmt)
		return true
	}

	for _, name := range sortedKeys(s.plans) {
		p := s.plans[name]
		if !live.tables[name] {
			if exec(p.createTableSQL()) {
				report.Created = append(report.Created, name)
			}
			continue
		}

		cols := live.columns[name]
		for i, c := range p.columns {
			if have, ok := cols[c.name]; ok {
				want := strings.ToLower(columnTypeOf(c.sql))
				if !sameType(have, want) {
					report.Drift = append(report.Drift, fmt.Sprintf("%s.%s is %s, the schema says %s", name, c.name, have, want))
				}
				continue
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteIdent(name), c.sql)
			if i > 0 {
				stmt += " AFTER " + quoteIdent(p.columns[i-1].name)
			}
			exec(stmt)
		}
		for _, k := range p.keys {
			if k.name == "" { // primary key
				if !live.indexes[name]["PRIMARY"] {
					report.Drift = append(report.Drift, name+" has no primary key")
				}
				continue
			}
			if !live.indexes[name][k.name] {
				exec(fmt.Sprintf("ALTER TABLE %s ADD %s", quoteIdent(name), k.sql))
			}
		}
		for _, ch := range p.checks {
			if !live.constraints[ch.name] {
				exec(fmt.Sprintf("ALTER TABLE %s ADD %s", quoteIdent(name), ch.sql))
			}
		}
	}

	for _, fk := range s.fks {
		if !live.constraints[fk.name] {
			exec(fk.sql)
		}
	}

	for _, name := range sortedKeys(s.views) {
		v := s.views[name]
		stmt, err := s.viewSQL(v)
		if err != nil || stmt == "" {
			continue
		}
		if live.views[name] && equalStrings(live.viewColumns[name], s.viewColumns(v)) {
			continue
		}
		exec(stmt)
	}

	for _, d := range report.Drift {
		log.Printf("[MySQL schema] drift (not repaired): %s", d)
	}
	return report, nil
}

func readLive(ctx context.Context, db *sql.DB) (*liveSchema, error) {
	live := &liveSchema{
		tables:      map[string]bool{},
		views:       map[string]bool{},
		columns:     map[string]map[string]string{},
		viewColumns: map[string][]string{},
		indexes:     map[string]map[string]bool{},
		constraints: map[string]bool{},
	}

	rows, err := db.QueryContext(ctx, dialect.Raw(
		"SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()"))
	if err != nil {
		return nil, fmt.Errorf("reading tables: %w", err)
	}
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			rows.Close()
			return nil, err
		}
		if typ == "VIEW" {
			live.views[strings.ToLower(name)] = true
		} else {
			live.tables[strings.ToLower(name)] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, dialect.Raw(
		"SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME, ORDINAL_POSITION"))
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}
	for rows.Next() {
		var table, col, typ string
		if err := rows.Scan(&table, &col, &typ); err != nil {
			rows.Close()
			return nil, err
		}
		table, col = strings.ToLower(table), strings.ToLower(col)
		if live.views[table] {
			live.viewColumns[table] = append(live.viewColumns[table], col)
			continue
		}
		if live.columns[table] == nil {
			live.columns[table] = map[string]string{}
		}
		live.columns[table][col] = strings.ToLower(typ)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, dialect.Raw(
		"SELECT DISTINCT TABLE_NAME, INDEX_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE()"))
	if err != nil {
		return nil, fmt.Errorf("reading indexes: %w", err)
	}
	for rows.Next() {
		var table, idx string
		if err := rows.Scan(&table, &idx); err != nil {
			rows.Close()
			return nil, err
		}
		table = strings.ToLower(table)
		if live.indexes[table] == nil {
			live.indexes[table] = map[string]bool{}
		}
		if idx != "PRIMARY" {
			idx = strings.ToLower(idx)
		}
		live.indexes[table][idx] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, dialect.Raw(
		"SELECT CONSTRAINT_NAME FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND CONSTRAINT_TYPE IN ('FOREIGN KEY', 'CHECK')"))
	if err != nil {
		return nil, fmt.Errorf("reading constraints: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		live.constraints[strings.ToLower(name)] = true
	}
	rows.Close()
	return live, rows.Err()
}

// columnTypeOf extracts the type from a rendered column definition.
func columnTypeOf(def string) string {
	// `name` TYPE ...
	rest := def[strings.Index(def, "` ")+2:]
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// sameType compares information_schema's COLUMN_TYPE with a rendered type.
func sameType(have, want string) bool {
	if have == want {
		return true
	}
	switch want {
	case "boolean":
		return have == "tinyint(1)"
	case "int":
		return have == "int" || have == "int(11)"
	case "bigint":
		return have == "bigint" || have == "bigint(20)"
	case "smallint":
		return have == "smallint" || have == "smallint(6)"
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
