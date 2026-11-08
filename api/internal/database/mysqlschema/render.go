package mysqlschema

import (
	"fmt"
	"sort"
	"strings"
)

// tablePlan is a table's MySQL rendering.
type tablePlan struct {
	table   *Table
	types   map[string]mysqlType
	columns []namedSQL // column definitions in order
	keys    []namedSQL // PRIMARY KEY, UNIQUE KEY and KEY clauses; name "" for the PK
	checks  []namedSQL
	notes   []string // what was changed or dropped while rendering, for the golden file
}

// namedSQL is a clause and the object name Sync looks it up by.
type namedSQL struct {
	name string
	sql  string
}

type fkPlan struct {
	table string
	name  string
	sql   string // ALTER TABLE ... ADD CONSTRAINT ...
}

// plan renders every table. It runs once, at the end of Build, so every
// decision — VARCHAR sizes, prefixes, the name MySQL needs to be unique — is
// made against the final model.
func (s *Schema) plan() error {
	s.plans = map[string]*tablePlan{}
	for _, name := range sortedKeys(s.tables) {
		t := s.tables[name]
		p := &tablePlan{table: t, types: map[string]mysqlType{}}
		for _, c := range t.Columns {
			mt, err := baseType(c.PGType, s.enums)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", t.Name, c.Name, err)
			}
			p.types[c.Name] = mt
		}
		s.plans[name] = p
	}

	// TEXT columns in a primary or unique key become VARCHAR sized to the key
	// budget: InnoDB cannot index TEXT whole, and a prefix would change what
	// "unique" means.
	for _, name := range sortedKeys(s.plans) {
		p := s.plans[name]
		for _, key := range p.uniqueKeyColumnSets() {
			p.sizeTextColumns(key)
		}
	}

	// A foreign key's columns must match the referenced columns' types
	// exactly, so the child takes the parent's final type.
	for _, name := range sortedKeys(s.plans) {
		p := s.plans[name]
		for _, fk := range p.table.ForeignKeys {
			parent := s.plans[fk.RefTable]
			if parent == nil {
				continue
			}
			refCols := fk.RefColumns
			if len(refCols) == 0 && parent.table.PrimaryKey != nil {
				refCols = parent.table.PrimaryKey.Columns
			}
			for i, col := range fk.Columns {
				if i < len(refCols) {
					if pt, ok := parent.types[refCols[i]]; ok {
						if ct, ok := p.types[col]; ok && ct.sql != pt.sql {
							p.types[col] = pt
						}
					}
				}
			}
		}
	}

	checkNames := map[string]bool{}
	fkNames := map[string]bool{}
	for _, name := range sortedKeys(s.plans) {
		p := s.plans[name]
		if err := s.renderTable(p, checkNames); err != nil {
			return fmt.Errorf("table %s: %w", name, err)
		}
	}
	for _, name := range sortedKeys(s.plans) {
		p := s.plans[name]
		for _, fk := range p.table.ForeignKeys {
			parent := s.tables[fk.RefTable]
			if parent == nil {
				s.skipNote("foreign key "+fk.Name+" on "+name, "references "+fk.RefTable+", which is not part of the schema")
				continue
			}
			refCols := fk.RefColumns
			if len(refCols) == 0 && parent.PrimaryKey != nil {
				refCols = parent.PrimaryKey.Columns
			}
			fkName := uniqueName(fk.Name, fkNames)
			sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				quoteIdent(name), quoteIdent(fkName), quoteList(fk.Columns), quoteIdent(fk.RefTable), quoteList(refCols))
			if fk.OnDelete != "" {
				sql += " ON DELETE " + fk.OnDelete
			}
			if fk.OnUpdate != "" {
				sql += " ON UPDATE " + fk.OnUpdate
			}
			s.fks = append(s.fks, fkPlan{table: name, name: fkName, sql: sql})
		}
	}
	s.rendered = true
	return nil
}

// skipNote records a rendering-time omission.
func (s *Schema) skipNote(object, reason string) {
	prev := s.source
	s.source = "render"
	s.skip(object, reason)
	s.source = prev
}

// maxFunctionalChars is the longest string a functional key part may
// produce: MySQL stores a longer one as TEXT, which it cannot index.
const maxFunctionalChars = 512

// keyColumns is one primary key or unique key/index, as the columns whose
// bytes count against the key.
type keyColumns struct {
	cols []string
	// functional marks the columns read inside a functional key part — an
	// expression, or a partial index's CASE — whose result must stay within
	// maxFunctionalChars.
	functional map[string]bool
}

// uniqueKeyColumnSets lists every primary key and unique key or index: plain
// columns, and the columns an expression part reads (its result is as wide as
// they are).
func (p *tablePlan) uniqueKeyColumnSets() []keyColumns {
	var sets []keyColumns
	t := p.table
	if t.PrimaryKey != nil {
		sets = append(sets, keyColumns{cols: t.PrimaryKey.Columns})
	}
	for _, u := range t.Uniques {
		sets = append(sets, keyColumns{cols: u.Columns})
	}
	for _, idx := range t.Indexes {
		if !idx.Unique {
			continue
		}
		k := keyColumns{functional: map[string]bool{}}
		for _, part := range idx.Parts {
			if part.Column != "" {
				k.cols = append(k.cols, part.Column)
				if idx.Where != nil {
					k.functional[part.Column] = true
				}
				continue
			}
			for _, c := range exprColumns(part.Expr, t) {
				k.cols = append(k.cols, c)
				k.functional[c] = true
			}
		}
		sets = append(sets, k)
	}
	return sets
}

// sizeTextColumns gives the TEXT columns of one key the budget left by the
// others.
func (p *tablePlan) sizeTextColumns(key keyColumns) {
	fixed, var_ := 0, []string{}
	for _, c := range key.cols {
		mt, ok := p.types[c]
		if !ok {
			continue
		}
		if mt.text && !mt.array {
			var_ = append(var_, c)
		} else {
			fixed += mt.bytes
		}
	}
	if len(var_) == 0 {
		return
	}
	n := (maxKeyBytes - fixed) / (bytesPerChar * len(var_))
	if n > maxSizedVarchar {
		n = maxSizedVarchar
	}
	for _, c := range var_ {
		size := n
		if key.functional[c] && size > maxFunctionalChars {
			size = maxFunctionalChars
		}
		if cur := p.types[c]; cur.text {
			p.types[c] = varchar(size)
			p.notes = append(p.notes, fmt.Sprintf("%s: text -> VARCHAR(%d) (unique/primary key column)", c, size))
		}
	}
}

func (s *Schema) renderTable(p *tablePlan, checkNames map[string]bool) error {
	t := p.table

	pk := map[string]bool{}
	if t.PrimaryKey != nil {
		for _, c := range t.PrimaryKey.Columns {
			pk[c] = true
		}
	}

	for _, c := range t.Columns {
		def, err := s.columnDef(p, c, pk[c.Name])
		if err != nil {
			return fmt.Errorf("column %s: %w", c.Name, err)
		}
		p.columns = append(p.columns, namedSQL{name: c.Name, sql: def})
	}

	if t.PrimaryKey != nil {
		p.keys = append(p.keys, namedSQL{sql: "PRIMARY KEY (" + quoteList(t.PrimaryKey.Columns) + ")"})
	}
	for _, u := range t.Uniques {
		p.keys = append(p.keys, namedSQL{name: u.Name, sql: fmt.Sprintf("UNIQUE KEY %s (%s)", quoteIdent(u.Name), quoteList(u.Columns))})
	}
	for _, idx := range t.Indexes {
		clause, note, err := s.indexClause(p, idx)
		if err != nil {
			return fmt.Errorf("index %s: %w", idx.Name, err)
		}
		if note != "" {
			p.notes = append(p.notes, idx.Name+": "+note)
		}
		if clause == "" {
			continue
		}
		p.keys = append(p.keys, namedSQL{name: idx.Name, sql: clause})
	}

	// MySQL refuses a CHECK on a column a foreign key's referential action
	// writes (error 3823).
	actionCols := map[string]bool{}
	for _, fk := range t.ForeignKeys {
		if fk.OnDelete != "" && fk.OnDelete != "RESTRICT" || fk.OnUpdate != "" && fk.OnUpdate != "RESTRICT" {
			for _, c := range fk.Columns {
				actionCols[c] = true
			}
		}
	}
	for _, ch := range t.Checks {
		expr := unwrap(stripCasts(ch.Expr))
		if len(expr) == 1 && expr[0].isIdent() {
			if mt, ok := p.types[expr[0].ident()]; ok && mt.boolean {
				// CHECK (flag): MySQL wants a boolean expression and BOOLEAN
				// is TINYINT there.
				expr = append(expr, token{kind: tkPunct, text: "="}, token{kind: tkWord, text: "TRUE"})
			}
		}
		sql, err := translateExpr(expr, t)
		if err != nil {
			s.skipNote("check "+ch.Name+" on "+t.Name, err.Error())
			continue
		}
		blocked := ""
		for _, c := range exprColumns(expr, t) {
			if actionCols[c] {
				blocked = c
			}
		}
		if blocked != "" {
			s.skipNote("check "+ch.Name+" on "+t.Name, "column "+blocked+" is written by a foreign key action, which MySQL forbids a CHECK to read")
			continue
		}
		name := uniqueName(ch.Name, checkNames)
		p.checks = append(p.checks, namedSQL{name: name, sql: fmt.Sprintf("CONSTRAINT %s CHECK (%s)", quoteIdent(name), sql)})
	}
	return nil
}

func (s *Schema) columnDef(p *tablePlan, c *Column, inPK bool) (string, error) {
	mt := p.types[c.Name]
	var b strings.Builder
	b.WriteString(quoteIdent(c.Name))
	b.WriteByte(' ')
	b.WriteString(mt.sql)
	if c.NotNull || inPK {
		b.WriteString(" NOT NULL")
	}
	if c.Serial {
		b.WriteString(" AUTO_INCREMENT")
		return b.String(), nil
	}
	if c.Default != nil {
		def, err := renderDefault(c, mt)
		if err != nil {
			s.skipNote("default of "+p.table.Name+"."+c.Name, err.Error())
		} else if def != "" {
			b.WriteString(" DEFAULT ")
			b.WriteString(def)
		}
	}
	if p.table.AutoUpdatedAt && c.Name == "updated_at" && mt.datetime {
		b.WriteString(" ON UPDATE CURRENT_TIMESTAMP(6)")
	}
	return b.String(), nil
}

// renderDefault maps a PostgreSQL DEFAULT. "" means no default.
func renderDefault(c *Column, mt mysqlType) (string, error) {
	expr := unwrap(stripCasts(c.Default))
	if len(expr) == 0 {
		return "", nil
	}
	if len(expr) > 2 && expr[0].isIdent() && expr[1].isPunct(".") {
		expr = expr[2:] // public.uuid_generate_v4()
	}
	wrap := func(lit string, isExpr bool) string {
		if mt.exprDefault || isExpr {
			return "(" + lit + ")"
		}
		return lit
	}
	first := expr[0]
	switch {
	case len(expr) == 1 && first.kind == tkString:
		v := first.val
		if mt.array {
			lit, err := pqArrayLiteral(v)
			if err != nil {
				return "", err
			}
			v = lit
		}
		if mt.boolean {
			switch strings.ToLower(v) {
			case "t", "true", "y", "yes", "on", "1":
				return "TRUE", nil
			default:
				return "FALSE", nil
			}
		}
		return wrap(mysqlString(v)), nil
	case len(expr) == 1 && first.kind == tkNumber,
		len(expr) == 2 && first.isPunct("-") && expr[1].kind == tkNumber:
		return joinTokens(expr), nil
	case len(expr) == 1 && (first.is("TRUE") || first.is("FALSE")):
		return strings.ToUpper(first.text), nil
	case len(expr) == 1 && first.is("NULL"):
		return "NULL", nil
	case isNow(expr):
		switch {
		case mt.datetime:
			return "CURRENT_TIMESTAMP(6)", nil
		case mt.date:
			return "(CURRENT_DATE)", nil
		}
		return "", fmt.Errorf("now() default on a %s column", mt.sql)
	case len(expr) == 1 && first.is("CURRENT_DATE"):
		return "(CURRENT_DATE)", nil
	case len(expr) >= 3 && (first.is("gen_random_uuid") || first.is("uuid_generate_v4")) && expr[1].isPunct("("):
		return "(UUID())", nil
	case first.is("ARRAY") && len(expr) >= 3 && expr[1].isPunct("["):
		var elems []string
		for _, part := range splitTopLevel(expr[2 : len(expr)-1]) {
			if len(part) != 1 || part[0].kind != tkString {
				return "", fmt.Errorf("non-literal array default %q", joinTokens(expr))
			}
			elems = append(elems, part[0].val)
		}
		return wrap(mysqlString(renderPQArray(elems))), nil
	}
	return "", fmt.Errorf("default %q has no MySQL equivalent; left without a default", joinTokens(expr))
}

func isNow(expr []token) bool {
	switch {
	case len(expr) == 1 && (expr[0].is("CURRENT_TIMESTAMP") || expr[0].is("LOCALTIMESTAMP")):
		return true
	case len(expr) == 3 && expr[1].isPunct("(") && expr[2].isPunct(")"):
		switch strings.ToLower(expr[0].text) {
		case "now", "transaction_timestamp", "statement_timestamp", "clock_timestamp", "current_timestamp":
			return true
		}
	}
	return false
}

// indexClause renders an index as a CREATE TABLE key clause. It returns ""
// when the index cannot exist on MySQL, with the reason in note.
func (s *Schema) indexClause(p *tablePlan, idx *Index) (clause, note string, err error) {
	t := p.table
	// Only a unique index keeps its predicate (inside each key part); a
	// non-unique one simply covers every row.
	var pred string
	if idx.Where != nil && idx.Unique {
		if pred, err = translateExpr(idx.Where, t); err != nil {
			return "", "", fmt.Errorf("predicate: %w", err)
		}
	}

	type part struct {
		sql      string
		bytes    int
		text     bool // TEXT column needing a prefix
		col      string
		desc     bool
		function bool
	}
	var parts []part
	for _, ip := range idx.Parts {
		var pp part
		pp.desc = ip.Desc
		if ip.Column != "" {
			mt, ok := p.types[ip.Column]
			if !ok {
				return "", "", fmt.Errorf("unknown column %s", ip.Column)
			}
			if mt.json {
				return "", "", fmt.Errorf("JSON column %s cannot be indexed", ip.Column)
			}
			pp.col, pp.bytes, pp.text = ip.Column, mt.bytes, mt.text
			pp.sql = quoteIdent(ip.Column)
			if idx.Unique && pred != "" {
				if mt.bytes > maxFunctionalChars*bytesPerChar {
					return "", "", fmt.Errorf("partial unique over %s column %s, wider than a functional key part allows", mt.sql, ip.Column)
				}
				pp.sql = fmt.Sprintf("(CASE WHEN %s THEN %s END)", pred, quoteIdent(ip.Column))
				pp.function, pp.text = true, false
			}
		} else {
			expr, err := translateExpr(ip.Expr, t)
			if err != nil {
				return "", "", err
			}
			for _, c := range exprColumns(ip.Expr, t) {
				pp.bytes += p.types[c].bytes
				if p.types[c].text || p.types[c].bytes > maxFunctionalChars*bytesPerChar {
					return "", "", fmt.Errorf("expression over %s column %s, wider than a functional key part allows", p.types[c].sql, c)
				}
			}
			if len(exprColumns(ip.Expr, t)) == 0 {
				// A constant: the singleton pattern, ((true)) on PostgreSQL.
				expr = "1"
				pp.bytes = 8
			}
			if pred != "" {
				expr = fmt.Sprintf("CASE WHEN %s THEN %s END", pred, expr)
			}
			pp.sql = "(" + expr + ")"
			pp.function = true
		}
		parts = append(parts, pp)
	}

	if !idx.Unique {
		// Non-unique: a TEXT column, or a VARCHAR too wide for what is left of
		// the budget, is indexed by prefix. A predicate is dropped: it only
		// limited which rows the index covers.
		fixed, prefixed := 0, 0
		for _, pp := range parts {
			if pp.text {
				prefixed++
			} else {
				fixed += pp.bytes
			}
		}
		prefix := textIndexPrefix
		if prefixed > 0 {
			if room := (maxKeyBytes - fixed) / (bytesPerChar * prefixed); room < prefix {
				prefix = room
			}
		}
		for i := range parts {
			if parts[i].text {
				parts[i].sql = fmt.Sprintf("%s(%d)", quoteIdent(parts[i].col), prefix)
			}
		}
		if idx.Where != nil {
			note = "partial-index predicate dropped (the index covers every row)"
		}
	} else {
		total := 0
		for _, pp := range parts {
			total += pp.bytes
		}
		if total > maxKeyBytes {
			return "", "", fmt.Errorf("unique key is %d bytes, over InnoDB's %d", total, maxKeyBytes)
		}
		if pred != "" {
			note = "partial unique index: each key part is CASE WHEN <predicate> THEN <part> END, so rows outside the predicate index NULL and never collide"
		}
	}

	var cols []string
	for _, pp := range parts {
		sql := pp.sql
		if pp.function {
			sql = "(" + sql + ")"
		}
		if pp.desc {
			sql += " DESC"
		}
		cols = append(cols, sql)
	}
	kind := "KEY"
	if idx.Unique {
		kind = "UNIQUE KEY"
	}
	return fmt.Sprintf("%s %s (%s)", kind, quoteIdent(idx.Name), strings.Join(cols, ", ")), note, nil
}

func quoteList(cols []string) string {
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = quoteIdent(c)
	}
	return strings.Join(q, ", ")
}

// uniqueName returns name, or name with a numeric suffix when taken; MySQL
// scopes CHECK and FOREIGN KEY names to the whole database.
func uniqueName(name string, taken map[string]bool) string {
	candidate := name
	for i := 2; taken[candidate]; i++ {
		suffix := fmt.Sprintf("_%d", i)
		base := name
		if len(base)+len(suffix) > 64 {
			base = base[:64-len(suffix)]
		}
		candidate = base + suffix
	}
	if len(candidate) > 64 {
		candidate = candidate[:64]
	}
	taken[candidate] = true
	return candidate
}

const tableOptions = "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"

// createTableSQL is the full CREATE TABLE for a missing table, foreign keys
// excluded (they are added once every table exists).
func (p *tablePlan) createTableSQL() string {
	var lines []string
	for _, c := range p.columns {
		lines = append(lines, "    "+c.sql)
	}
	for _, k := range p.keys {
		lines = append(lines, "    "+k.sql)
	}
	for _, ch := range p.checks {
		lines = append(lines, "    "+ch.sql)
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n%s\n) %s", quoteIdent(p.table.Name), strings.Join(lines, ",\n"), tableOptions)
}

// viewSQL renders a view for MySQL, or "" when it cannot be.
//
// The only view, logs_unified, UNIONs the legacy logs table with
// logs_partitioned on PostgreSQL. Branches over tables the model does not
// have are dropped, which on MySQL leaves the logs_partitioned projection.
func (s *Schema) viewSQL(v *View) (string, error) {
	var branches []string
	for _, branch := range splitUnionAll(v.Body) {
		c := &cursor{toks: branch}
		if !c.acceptWords("SELECT") {
			return "", fmt.Errorf("view %s: unsupported body", v.Name)
		}
		from := indexWord(branch, "FROM")
		if from < 0 {
			return "", fmt.Errorf("view %s: no FROM", v.Name)
		}
		fc := &cursor{toks: branch[from+1:]}
		table := fc.qualifiedName()
		if !fc.done() {
			return "", fmt.Errorf("view %s: only plain projections are supported", v.Name)
		}
		if s.tables[table] == nil {
			continue
		}
		var cols []string
		for _, item := range splitTopLevel(branch[1:from]) {
			last := item[len(item)-1]
			if !last.isIdent() || (len(item) != 1 && !(len(item) == 3 && item[1].isPunct("."))) {
				return "", fmt.Errorf("view %s: only plain columns are supported", v.Name)
			}
			cols = append(cols, quoteIdent(last.ident()))
		}
		branches = append(branches, fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), quoteIdent(table)))
	}
	if len(branches) == 0 {
		return "", nil
	}
	return fmt.Sprintf("CREATE OR REPLACE VIEW %s AS %s", quoteIdent(v.Name), strings.Join(branches, " UNION ALL ")), nil
}

func splitUnionAll(toks []token) [][]token {
	var out [][]token
	start, depth := 0, 0
	for i := 0; i < len(toks); i++ {
		switch {
		case toks[i].isPunct("("):
			depth++
		case toks[i].isPunct(")"):
			depth--
		case depth == 0 && toks[i].is("UNION") && i+1 < len(toks) && toks[i+1].is("ALL"):
			out = append(out, toks[start:i])
			start = i + 2
			i++
		}
	}
	return append(out, toks[start:])
}

// viewColumns lists a view's column names as rendered.
func (s *Schema) viewColumns(v *View) []string {
	branches := splitUnionAll(v.Body)
	if len(branches) == 0 {
		return nil
	}
	from := indexWord(branches[0], "FROM")
	if from < 1 {
		return nil
	}
	var cols []string
	for _, item := range splitTopLevel(branches[0][1:from]) {
		cols = append(cols, item[len(item)-1].ident())
	}
	return cols
}

// Statements is the full MySQL DDL for an empty database, in execution
// order: tables, then foreign keys, then views.
func (s *Schema) Statements() ([]string, error) {
	var out []string
	for _, name := range sortedKeys(s.plans) {
		out = append(out, s.plans[name].createTableSQL())
	}
	for _, fk := range s.fks {
		out = append(out, fk.sql)
	}
	for _, name := range sortedKeys(s.views) {
		sql, err := s.viewSQL(s.views[name])
		if err != nil {
			return nil, err
		}
		if sql != "" {
			out = append(out, sql)
		}
	}
	return out, nil
}

// Render is the reviewable form of the MySQL schema: every statement Sync
// would run on an empty database, and every object left out with the
// reason. It is checked in as testdata/schema.mysql.sql.
func (s *Schema) Render() (string, error) {
	stmts, err := s.Statements()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("-- NginxProxyGuard — MySQL schema, derived from the PostgreSQL schema.\n")
	b.WriteString("--\n")
	b.WriteString("-- GENERATED, do not edit. Source: migrations/001_init.sql followed by the\n")
	b.WriteString("-- upgrade list in database/migration.go, replayed by internal/database/mysqlschema.\n")
	b.WriteString("-- Regenerate: go test ./internal/database/mysqlschema -run Golden -update\n")
	b.WriteString("--\n")
	b.WriteString("-- The running server does not read this file: database.runMySQLMigrations\n")
	b.WriteString("-- builds the same statements at boot and Sync applies whatever is missing.\n\n")

	b.WriteString("-- ---------------------------------------------------------------------------\n")
	b.WriteString("-- Left out, and why\n")
	b.WriteString("-- ---------------------------------------------------------------------------\n")
	skips := append([]Skip(nil), s.skips...)
	sort.SliceStable(skips, func(i, j int) bool { return skips[i].Object < skips[j].Object })
	for _, k := range skips {
		fmt.Fprintf(&b, "-- %s [%s]\n--     %s\n", oneLine(k.Object), oneLine(k.Source), oneLine(k.Reason))
	}
	b.WriteString("\n")

	notes := false
	for _, name := range sortedKeys(s.plans) {
		if len(s.plans[name].notes) > 0 {
			if !notes {
				b.WriteString("-- ---------------------------------------------------------------------------\n")
				b.WriteString("-- Rendered differently from PostgreSQL\n")
				b.WriteString("-- ---------------------------------------------------------------------------\n")
				notes = true
			}
			for _, n := range s.plans[name].notes {
				fmt.Fprintf(&b, "-- %s.%s\n", name, n)
			}
		}
	}
	if notes {
		b.WriteString("\n")
	}

	for _, stmt := range stmts {
		b.WriteString(stmt)
		b.WriteString(";\n\n")
	}
	fmt.Fprintf(&b, "-- %d data statement(s) from 001_init.sql and %d from the upgrade list run\n", len(s.initData), len(s.upgradeData))
	b.WriteString("-- through the translating driver after the DDL (see database/migration_mysql.go).\n")
	return b.String(), nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// ---------------------------------------------------------------------------
// Accessors
// ---------------------------------------------------------------------------

// Tables lists the tables of the MySQL schema.
func (s *Schema) Tables() []string { return sortedKeys(s.tables) }

// Columns lists a table's columns in declaration order.
func (s *Schema) Columns(table string) []string {
	t := s.tables[table]
	if t == nil {
		return nil
	}
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = c.Name
	}
	return cols
}

// Views lists the views of the MySQL schema.
func (s *Schema) Views() []string { return sortedKeys(s.views) }

// Skipped lists what the replay left out.
func (s *Schema) Skipped() []Skip { return append([]Skip(nil), s.skips...) }

// InitData returns the data statements of 001_init.sql, in order, as
// PostgreSQL text without schema qualifiers.
func (s *Schema) InitData() []DataStatement { return append([]DataStatement(nil), s.initData...) }

// UpgradeData returns the data statements of the upgrade list, in order.
func (s *Schema) UpgradeData() []DataStatement { return append([]DataStatement(nil), s.upgradeData...) }

// PartitionChildren reports the partition tables the replay folded into
// their parents.
func (s *Schema) PartitionChildren() []string { return sortedKeys(s.children) }
