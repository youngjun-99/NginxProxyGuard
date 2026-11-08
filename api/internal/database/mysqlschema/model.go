// Package mysqlschema derives NPG's MySQL schema from its PostgreSQL one.
//
// The PostgreSQL files stay the single place a schema change is written:
// migrations/001_init.sql (pg_dump-shaped, re-runnable) followed by the
// boot-time upgrade list in database/migration.go. Build replays both, in
// order, against an in-memory model — a CREATE TABLE IF NOT EXISTS of a table
// that exists is a no-op, a later ADD COLUMN adds, DROP CONSTRAINT drops —
// so the model ends in the state a PostgreSQL install reaches after its
// first boot. Render turns that state into MySQL DDL and Sync brings a live
// MySQL database up to it without ever dropping or rewriting what is there.
//
// Statements with no schema meaning on MySQL (TimescaleDB calls, PL/pgSQL
// functions, comments, partition plumbing) are recorded as skipped with the
// reason, and the reasons are part of the rendered golden file, so what was
// left out is reviewable rather than silent. INSERT/UPDATE/DELETE statements
// are kept as PostgreSQL text for the translating driver to run.
package mysqlschema

// Schema is the simulated end state of the PostgreSQL schema.
type Schema struct {
	tables   map[string]*Table
	views    map[string]*View
	enums    map[string]bool
	indexOf  map[string]string // index name -> table; PostgreSQL index names are schema-wide
	children map[string]bool   // partition children, removed when the replay ends

	updatedAtFuncs map[string]bool // trigger functions that stamp NEW.updated_at

	retired map[string]bool // index names deliberately not built

	skips []Skip

	initData    []DataStatement
	upgradeData []DataStatement

	source string // what is being replayed, for skip records

	// rendering results, filled by finish()
	rendered bool
	plans    map[string]*tablePlan
	fks      []fkPlan
}

// Table is one table of the model.
type Table struct {
	Name        string
	Columns     []*Column
	PrimaryKey  *Key
	Uniques     []*Key // UNIQUE constraints
	Indexes     []*Index
	ForeignKeys []*ForeignKey
	Checks      []*Check
	// AutoUpdatedAt is set when a BEFORE UPDATE trigger stamps updated_at;
	// MySQL expresses that as ON UPDATE CURRENT_TIMESTAMP(6).
	AutoUpdatedAt bool
	Partitioned   bool
}

// Column is a column as PostgreSQL declares it. Type mapping happens at render
// time so a later ALTER COLUMN TYPE is honoured.
type Column struct {
	Name    string
	PGType  string // normalised: lower case, no schema, e.g. "character varying(255)"
	NotNull bool
	Default []token // PostgreSQL expression; nil when there is none
	Serial  bool    // serial/bigserial/identity/nextval: AUTO_INCREMENT
}

// Key is a primary key or unique constraint.
type Key struct {
	Name    string
	Columns []string
}

// Index is a CREATE INDEX.
type Index struct {
	Name   string
	Unique bool
	Method string // btree unless stated
	Parts  []IndexPart
	Where  []token // partial-index predicate
}

// IndexPart is a plain column or an expression.
type IndexPart struct {
	Column string
	Expr   []token
	Desc   bool
}

// ForeignKey is a FOREIGN KEY constraint.
type ForeignKey struct {
	Name       string
	Columns    []string
	RefTable   string
	RefColumns []string
	OnDelete   string
	OnUpdate   string
}

// Check is a CHECK constraint.
type Check struct {
	Name string
	Expr []token
}

// View is a CREATE VIEW, kept as PostgreSQL tokens.
type View struct {
	Name string
	Body []token
}

// Skip records something the replay deliberately did not carry over.
type Skip struct {
	Source string // 001_init.sql or the upgrade's description
	Object string
	Reason string
}

// DataStatement is an INSERT/UPDATE/DELETE in PostgreSQL syntax, for the
// translating driver to execute. `public.` qualifiers are removed and E”
// strings respelled as standard strings (see normaliseData).
type DataStatement struct {
	Source string
	SQL    string
}

// Upgrade is one entry of database/migration.go's upgrade list.
type Upgrade struct {
	Desc string
	SQL  string
}

// Options tunes the replay.
type Options struct {
	// RetiredIndexes are index names NPG no longer builds even though older
	// statements still declare them (database.retiredLogsPartitionedIndexes).
	RetiredIndexes []string
}

func (s *Schema) table(name string) *Table { return s.tables[name] }

func (t *Table) column(name string) *Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (s *Schema) skip(object, reason string) {
	for _, k := range s.skips {
		if k.Object == object && k.Reason == reason {
			return
		}
	}
	s.skips = append(s.skips, Skip{Source: s.source, Object: object, Reason: reason})
}
