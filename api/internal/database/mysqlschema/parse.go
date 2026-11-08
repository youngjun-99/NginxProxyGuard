package mysqlschema

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// excludedTables exist in the PostgreSQL schema only as history. Nothing at
// runtime reads or writes them, so a MySQL install, which has no such
// history, does not get them.
var excludedTables = map[string]string{
	"logs": "pre-partitioning log table: the app writes logs_partitioned only, and " +
		"PostgreSQL keeps it solely for the logs_unified UNION, which on MySQL projects logs_partitioned alone",
	"dashboard_stats_hourly_partitioned": "transitional partitioned copy of dashboard_stats_hourly; " +
		"the app reads and writes dashboard_stats_hourly",
}

// Build replays the PostgreSQL schema (initSQL, then each upgrade in order)
// into a model ready to render for MySQL.
func Build(initSQL string, upgrades []Upgrade, opts Options) (*Schema, error) {
	s := &Schema{
		tables:         map[string]*Table{},
		views:          map[string]*View{},
		enums:          map[string]bool{},
		indexOf:        map[string]string{},
		children:       map[string]bool{},
		updatedAtFuncs: map[string]bool{},
		retired:        map[string]bool{},
	}
	for _, name := range opts.RetiredIndexes {
		s.retired[strings.ToLower(name)] = true
	}

	s.source = "001_init.sql"
	for _, stmt := range splitStatements(initSQL) {
		if err := s.apply(stmt, &s.initData); err != nil {
			return nil, fmt.Errorf("001_init.sql: %w\nstatement: %s", err, truncate(stmt))
		}
	}
	for _, u := range upgrades {
		s.source = u.Desc
		for _, stmt := range splitStatements(u.SQL) {
			if err := s.apply(stmt, &s.upgradeData); err != nil {
				return nil, fmt.Errorf("upgrade %q: %w\nstatement: %s", u.Desc, err, truncate(stmt))
			}
		}
	}
	s.source = "replay"

	for name := range s.children {
		s.dropTable(name)
	}
	for name, reason := range excludedTables {
		if s.tables[name] != nil {
			s.dropTable(name)
			s.skip("table "+name, reason)
		}
	}

	if err := s.plan(); err != nil {
		return nil, err
	}
	return s, nil
}

func truncate(stmt string) string {
	stmt = strings.Join(strings.Fields(stmt), " ")
	if len(stmt) > 200 {
		return stmt[:200] + "..."
	}
	return stmt
}

func (s *Schema) dropTable(name string) {
	delete(s.tables, name)
	for idx, owner := range s.indexOf {
		if owner == name {
			delete(s.indexOf, idx)
		}
	}
}

// apply replays one PostgreSQL statement against the model. data receives the
// statement when it is DML.
func (s *Schema) apply(stmt string, data *[]DataStatement) error {
	toks := tokenize(stmt)
	if len(toks) == 0 {
		return nil
	}
	head := firstWords(toks, 4)
	switch {
	case strings.HasPrefix(head, "INSERT"), strings.HasPrefix(head, "UPDATE"),
		strings.HasPrefix(head, "DELETE"), strings.HasPrefix(head, "WITH"):
		*data = append(*data, DataStatement{Source: s.source, SQL: normaliseData(stmt)})
		return nil
	case strings.HasPrefix(head, "TRUNCATE"):
		*data = append(*data, DataStatement{Source: s.source, SQL: normaliseData(stmt)})
		return nil
	}
	return s.applyDDL(toks)
}

// applyDDL handles everything that is not DML. It is also the entry point for
// DDL found inside DO blocks.
func (s *Schema) applyDDL(toks []token) error {
	c := &cursor{toks: toks}
	switch {
	case c.peekWords("CREATE", "EXTENSION"):
		c.pos = 2
		c.acceptWords("IF", "NOT", "EXISTS")
		s.skip("extension "+c.next().ident(), "PostgreSQL extension: UUID() and the MySQL schema need none")
	case c.peekWords("CREATE", "TYPE"):
		return s.createType(c)
	case c.peekWords("ALTER", "TYPE"):
		// Enum values are not enforced on MySQL (enums are VARCHAR(64)), so
		// adding one changes nothing.
	case c.peekWords("CREATE", "FUNCTION"), c.peekWords("CREATE", "OR", "REPLACE", "FUNCTION"),
		c.peekWords("CREATE", "PROCEDURE"), c.peekWords("CREATE", "OR", "REPLACE", "PROCEDURE"):
		s.createFunction(c)
	case c.peekWords("CREATE", "TRIGGER"), c.peekWords("CREATE", "OR", "REPLACE", "TRIGGER"):
		s.createTrigger(c)
	case c.peekWords("CREATE", "TABLE"), c.peekWords("CREATE", "UNLOGGED", "TABLE"):
		return s.createTable(c)
	case c.peekWords("CREATE", "INDEX"), c.peekWords("CREATE", "UNIQUE", "INDEX"):
		return s.createIndex(c)
	case c.peekWords("ALTER", "TABLE"):
		return s.alterTable(c)
	case c.peekWords("ALTER", "INDEX"):
		s.alterIndex(c)
	case c.peekWords("DROP", "INDEX"):
		s.dropIndex(c)
	case c.peekWords("DROP", "TABLE"):
		c.pos = 2
		c.acceptWords("IF", "EXISTS")
		for _, name := range c.nameList() {
			s.dropTable(name)
		}
	case c.peekWords("CREATE", "VIEW"), c.peekWords("CREATE", "OR", "REPLACE", "VIEW"):
		s.createView(c)
	case c.peekWords("DROP", "VIEW"):
		c.pos = 2
		c.acceptWords("IF", "EXISTS")
		for _, name := range c.nameList() {
			delete(s.views, name)
		}
	case c.peekWords("DROP", "TRIGGER"), c.peekWords("DROP", "FUNCTION"):
		// Triggers become column attributes, functions do not exist.
	case c.peekWords("DO"):
		return s.doBlock(c)
	case c.peekWords("COMMENT", "ON"):
		// Documentation only.
	case c.peekWords("SELECT"):
		s.skip(snippet(toks), "PostgreSQL/TimescaleDB function call with no schema effect on MySQL")
	case c.peekWords("CREATE", "SEQUENCE"), c.peekWords("ALTER", "SEQUENCE"):
		// nextval() defaults become AUTO_INCREMENT.
	case c.peekWords("SET"), c.peekWords("RESET"), c.peekWords("ANALYZE"), c.peekWords("VACUUM"),
		c.peekWords("GRANT"), c.peekWords("REVOKE"):
		// Session and maintenance statements.
	default:
		return fmt.Errorf("unrecognised statement %q", snippet(toks))
	}
	return nil
}

func snippet(toks []token) string {
	s := joinTokens(toks)
	if len(s) > 90 {
		s = s[:90] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// CREATE TYPE / FUNCTION / TRIGGER / VIEW
// ---------------------------------------------------------------------------

func (s *Schema) createType(c *cursor) error {
	c.pos = 2
	name := c.qualifiedName()
	if c.acceptWords("AS", "ENUM") {
		s.enums[name] = true
		return nil
	}
	s.skip("type "+name, "only enum types are used; this one has no MySQL mapping")
	return nil
}

var updatedAtBody = regexp.MustCompile(`(?is)NEW\.updated_at\s*:?=\s*(now\(\)|current_timestamp|clock_timestamp\(\))`)

func (s *Schema) createFunction(c *cursor) {
	for !c.peekWord("FUNCTION") && !c.peekWord("PROCEDURE") {
		c.pos++
	}
	c.pos++
	name := c.qualifiedName()
	for _, t := range c.toks[c.pos:] {
		if t.kind == tkDollar && updatedAtBody.MatchString(t.val) {
			s.updatedAtFuncs[name] = true
			return
		}
	}
	s.skip("function "+name, "PL/pgSQL function; MySQL gets no stored routines, so its callers need a MySQL branch")
}

func (s *Schema) createTrigger(c *cursor) {
	for !c.peekWord("TRIGGER") {
		c.pos++
	}
	c.pos++
	trigger := c.next().ident()
	var table, fn string
	before, update := false, false
	for !c.done() {
		t := c.next()
		switch {
		case t.is("BEFORE"):
			before = true
		case t.is("UPDATE"):
			update = true
		case t.is("ON") && table == "":
			table = c.qualifiedName()
		case t.is("FUNCTION") || t.is("PROCEDURE"):
			fn = c.qualifiedName()
		}
	}
	if tbl := s.tables[table]; tbl != nil && before && update && s.updatedAtFuncs[fn] && tbl.column("updated_at") != nil {
		tbl.AutoUpdatedAt = true
		return
	}
	s.skip("trigger "+trigger, "not an updated_at stamp; no MySQL equivalent")
}

func (s *Schema) createView(c *cursor) {
	for !c.peekWord("VIEW") {
		c.pos++
	}
	c.pos++
	name := c.qualifiedName()
	if !c.acceptWords("AS") {
		return
	}
	s.views[name] = &View{Name: name, Body: c.toks[c.pos:]}
}

// ---------------------------------------------------------------------------
// CREATE TABLE
// ---------------------------------------------------------------------------

func (s *Schema) createTable(c *cursor) error {
	c.pos = 1
	c.acceptWords("UNLOGGED")
	c.expectWord("TABLE")
	c.acceptWords("IF", "NOT", "EXISTS")
	name := c.qualifiedName()

	if c.acceptWords("PARTITION", "OF") {
		s.children[name] = true
		return nil
	}
	if s.tables[name] != nil || s.children[name] {
		return nil // IF NOT EXISTS of an existing table
	}
	if !c.peekPunct("(") {
		return fmt.Errorf("CREATE TABLE %s: expected a column list", name)
	}
	body, err := c.group()
	if err != nil {
		return err
	}
	t := &Table{Name: name}
	for _, elem := range splitTopLevel(body) {
		if len(elem) == 0 {
			continue
		}
		if elem[0].is("LIKE") {
			return fmt.Errorf("CREATE TABLE %s (LIKE ...) is not supported", name)
		}
		if isTableConstraint(elem) {
			if err := s.addTableConstraint(t, elem); err != nil {
				return fmt.Errorf("table %s: %w", name, err)
			}
			continue
		}
		if err := s.addColumn(t, elem); err != nil {
			return fmt.Errorf("table %s: %w", name, err)
		}
	}
	if c.acceptWords("PARTITION", "BY") {
		t.Partitioned = true
	}
	s.tables[name] = t
	return nil
}

func isTableConstraint(elem []token) bool {
	switch {
	case elem[0].is("CONSTRAINT"), elem[0].is("CHECK"), elem[0].is("EXCLUDE"),
		elem[0].is("PRIMARY") && len(elem) > 1 && elem[1].is("KEY"),
		elem[0].is("FOREIGN") && len(elem) > 1 && elem[1].is("KEY"),
		elem[0].is("UNIQUE") && len(elem) > 1 && (elem[1].isPunct("(") || elem[1].is("NULLS")):
		return true
	}
	return false
}

// columnModifiers end a type or a DEFAULT expression.
var columnModifiers = map[string]bool{
	"NOT": true, "NULL": true, "DEFAULT": true, "PRIMARY": true, "UNIQUE": true,
	"REFERENCES": true, "CHECK": true, "CONSTRAINT": true, "GENERATED": true, "COLLATE": true,
}

func (s *Schema) addColumn(t *Table, elem []token) error {
	c := &cursor{toks: elem}
	col := &Column{Name: c.next().ident()}

	var typ []token
	for !c.done() && !(c.peek().kind == tkWord && columnModifiers[strings.ToUpper(c.peek().text)]) {
		typ = append(typ, c.next())
	}
	col.PGType = normaliseType(typ)
	switch col.PGType {
	case "serial", "serial4":
		col.PGType, col.Serial = "integer", true
	case "bigserial", "serial8":
		col.PGType, col.Serial = "bigint", true
	case "smallserial", "serial2":
		col.PGType, col.Serial = "smallint", true
	}

	if existing := t.column(col.Name); existing != nil {
		return nil // ADD COLUMN IF NOT EXISTS
	}
	t.Columns = append(t.Columns, col)

	var constraintName string
	for !c.done() {
		switch tok := c.next(); {
		case tok.is("NOT"):
			c.expectWord("NULL")
			col.NotNull = true
		case tok.is("NULL"):
			col.NotNull = false
		case tok.is("DEFAULT"):
			col.Default = c.until(func(t token) bool {
				return t.kind == tkWord && columnModifiers[strings.ToUpper(t.text)]
			}, true)
			if isNextval(col.Default) {
				col.Default, col.Serial = nil, true
			}
		case tok.is("CONSTRAINT"):
			constraintName = c.next().ident()
			continue
		case tok.is("PRIMARY"):
			c.expectWord("KEY")
			t.PrimaryKey = &Key{Name: orDefault(constraintName, t.Name+"_pkey"), Columns: []string{col.Name}}
			col.NotNull = true
		case tok.is("UNIQUE"):
			t.addUnique(&Key{Name: orDefault(constraintName, pgName(t.Name, []string{col.Name}, "key")), Columns: []string{col.Name}})
		case tok.is("REFERENCES"):
			fk := &ForeignKey{Name: orDefault(constraintName, pgName(t.Name, []string{col.Name}, "fkey")), Columns: []string{col.Name}}
			parseReferences(c, fk)
			t.addForeignKey(fk)
		case tok.is("CHECK"):
			expr, err := c.group()
			if err != nil {
				return err
			}
			t.addCheck(&Check{Name: orDefault(constraintName, pgName(t.Name, []string{col.Name}, "check")), Expr: expr})
		case tok.is("GENERATED"):
			rest := joinTokens(c.toks[c.pos:])
			if strings.Contains(strings.ToUpper(rest), "IDENTITY") {
				col.Serial = true
				c.pos = len(c.toks)
			} else {
				return fmt.Errorf("generated column %s is not supported", col.Name)
			}
		case tok.is("COLLATE"):
			c.next()
		default:
			return fmt.Errorf("column %s: unexpected %q", col.Name, tok.text)
		}
		constraintName = ""
	}
	return nil
}

func isNextval(expr []token) bool {
	return len(expr) > 0 && expr[0].is("nextval")
}

func orDefault(name, def string) string {
	if name != "" {
		return name
	}
	return def
}

// pgName reproduces PostgreSQL's default constraint names: table, columns and
// a suffix joined by underscores, cut to 63 bytes. Later DROP CONSTRAINT
// statements name constraints this way.
func pgName(table string, cols []string, suffix string) string {
	name := table + "_" + strings.Join(cols, "_") + "_" + suffix
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func parseReferences(c *cursor, fk *ForeignKey) {
	fk.RefTable = c.qualifiedName()
	if c.peekPunct("(") {
		g, _ := c.group()
		fk.RefColumns = identList(g)
	}
	for !c.done() {
		switch {
		case c.acceptWords("ON", "DELETE"):
			fk.OnDelete = referentialAction(c)
		case c.acceptWords("ON", "UPDATE"):
			fk.OnUpdate = referentialAction(c)
		case c.acceptWords("MATCH"):
			c.next()
		case c.acceptWords("NOT", "DEFERRABLE"), c.acceptWords("DEFERRABLE"),
			c.acceptWords("INITIALLY", "DEFERRED"), c.acceptWords("INITIALLY", "IMMEDIATE"),
			c.acceptWords("NOT", "VALID"):
		default:
			return
		}
	}
}

func referentialAction(c *cursor) string {
	switch {
	case c.acceptWords("CASCADE"):
		return "CASCADE"
	case c.acceptWords("SET", "NULL"):
		return "SET NULL"
	case c.acceptWords("SET", "DEFAULT"):
		return "SET DEFAULT"
	case c.acceptWords("RESTRICT"):
		return "RESTRICT"
	case c.acceptWords("NO", "ACTION"):
		return ""
	}
	return ""
}

func (s *Schema) addTableConstraint(t *Table, elem []token) error {
	c := &cursor{toks: elem}
	var name string
	if c.acceptWords("CONSTRAINT") {
		name = c.next().ident()
	}
	switch {
	case c.acceptWords("PRIMARY", "KEY"):
		g, err := c.group()
		if err != nil {
			return err
		}
		cols := identList(g)
		t.PrimaryKey = &Key{Name: orDefault(name, t.Name+"_pkey"), Columns: cols}
		for _, col := range cols {
			if cc := t.column(col); cc != nil {
				cc.NotNull = true
			}
		}
	case c.acceptWords("UNIQUE"):
		c.acceptWords("NULLS", "NOT", "DISTINCT")
		c.acceptWords("NULLS", "DISTINCT")
		g, err := c.group()
		if err != nil {
			return err
		}
		cols := identList(g)
		t.addUnique(&Key{Name: orDefault(name, pgName(t.Name, cols, "key")), Columns: cols})
	case c.acceptWords("CHECK"):
		expr, err := c.group()
		if err != nil {
			return err
		}
		t.addCheck(&Check{Name: orDefault(name, pgName(t.Name, checkColumns(t, expr), "check")), Expr: expr})
	case c.acceptWords("FOREIGN", "KEY"):
		g, err := c.group()
		if err != nil {
			return err
		}
		cols := identList(g)
		c.expectWord("REFERENCES")
		fk := &ForeignKey{Name: orDefault(name, pgName(t.Name, cols, "fkey")), Columns: cols}
		parseReferences(c, fk)
		t.addForeignKey(fk)
	case c.acceptWords("EXCLUDE"):
		s.skip("constraint "+name+" on "+t.Name, "EXCLUDE constraints have no MySQL equivalent")
	default:
		return fmt.Errorf("unrecognised table constraint %q", snippet(elem))
	}
	return nil
}

// checkColumns lists the table's columns a CHECK mentions, for its default
// name.
func checkColumns(t *Table, expr []token) []string {
	var cols []string
	seen := map[string]bool{}
	for _, tok := range expr {
		if tok.isIdent() && t.column(tok.ident()) != nil && !seen[tok.ident()] {
			seen[tok.ident()] = true
			cols = append(cols, tok.ident())
		}
	}
	if len(cols) > 1 {
		return cols[:0]
	}
	return cols
}

func (t *Table) addUnique(k *Key) {
	for i, u := range t.Uniques {
		if u.Name == k.Name {
			t.Uniques[i] = k
			return
		}
	}
	t.Uniques = append(t.Uniques, k)
}

func (t *Table) addForeignKey(fk *ForeignKey) {
	for i, f := range t.ForeignKeys {
		if f.Name == fk.Name {
			t.ForeignKeys[i] = fk
			return
		}
	}
	t.ForeignKeys = append(t.ForeignKeys, fk)
}

func (t *Table) addCheck(ch *Check) {
	for i, c := range t.Checks {
		if c.Name == ch.Name {
			t.Checks[i] = ch
			return
		}
	}
	t.Checks = append(t.Checks, ch)
}

// dropConstraint removes a constraint of any kind by name.
func (t *Table) dropConstraint(name string) bool {
	if t.PrimaryKey != nil && t.PrimaryKey.Name == name {
		t.PrimaryKey = nil
		return true
	}
	for i, u := range t.Uniques {
		if u.Name == name {
			t.Uniques = append(t.Uniques[:i], t.Uniques[i+1:]...)
			return true
		}
	}
	for i, f := range t.ForeignKeys {
		if f.Name == name {
			t.ForeignKeys = append(t.ForeignKeys[:i], t.ForeignKeys[i+1:]...)
			return true
		}
	}
	for i, ch := range t.Checks {
		if ch.Name == name {
			t.Checks = append(t.Checks[:i], t.Checks[i+1:]...)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// ALTER TABLE
// ---------------------------------------------------------------------------

func (s *Schema) alterTable(c *cursor) error {
	c.pos = 2
	ifExists := c.acceptWords("IF", "EXISTS")
	c.acceptWords("ONLY")
	name := c.qualifiedName()

	t := s.tables[name]
	if t == nil {
		if s.children[name] || ifExists {
			return nil
		}
		// A table the model does not have was created by Go code on
		// PostgreSQL only (hypertable plumbing); nothing to carry over.
		s.skip("ALTER TABLE "+name, "table is not part of the schema model")
		return nil
	}

	for _, action := range splitTopLevel(c.toks[c.pos:]) {
		if err := s.alterAction(t, action); err != nil {
			return fmt.Errorf("ALTER TABLE %s: %w", name, err)
		}
	}
	return nil
}

func (s *Schema) alterAction(t *Table, action []token) error {
	c := &cursor{toks: action}
	switch {
	case c.acceptWords("ADD"):
		if c.peekWord("CONSTRAINT") || c.peekWords("PRIMARY", "KEY") || c.peekWords("FOREIGN", "KEY") ||
			c.peekWord("CHECK") || c.peekWord("EXCLUDE") || (c.peekWord("UNIQUE") && len(c.toks) > c.pos+1 && c.toks[c.pos+1].isPunct("(")) {
			return s.addTableConstraint(t, c.toks[c.pos:])
		}
		c.acceptWords("COLUMN")
		c.acceptWords("IF", "NOT", "EXISTS")
		return s.addColumn(t, c.toks[c.pos:])

	case c.acceptWords("DROP", "CONSTRAINT"):
		c.acceptWords("IF", "EXISTS")
		t.dropConstraint(c.next().ident())

	case c.acceptWords("DROP"):
		c.acceptWords("COLUMN")
		c.acceptWords("IF", "EXISTS")
		col := c.next().ident()
		for i, cc := range t.Columns {
			if cc.Name == col {
				t.Columns = append(t.Columns[:i], t.Columns[i+1:]...)
				break
			}
		}

	case c.acceptWords("ALTER"):
		c.acceptWords("COLUMN")
		col := t.column(c.next().ident())
		if col == nil {
			return nil
		}
		switch {
		case c.acceptWords("SET", "DEFAULT"):
			col.Default = c.toks[c.pos:]
		case c.acceptWords("DROP", "DEFAULT"):
			col.Default = nil
		case c.acceptWords("SET", "NOT", "NULL"):
			col.NotNull = true
		case c.acceptWords("DROP", "NOT", "NULL"):
			col.NotNull = false
		case c.acceptWords("SET", "DATA", "TYPE"), c.acceptWords("TYPE"):
			typ := c.until(func(t token) bool { return t.is("USING") || t.is("COLLATE") }, false)
			col.PGType = normaliseType(typ)
		case c.acceptWords("SET", "STATISTICS"), c.acceptWords("SET", "STORAGE"), c.acceptWords("SET", "COMPRESSION"):
		default:
			return fmt.Errorf("unsupported ALTER COLUMN %q", snippet(action))
		}

	case c.acceptWords("RENAME", "COLUMN"), c.peekWord("RENAME") && !c.peekWords("RENAME", "TO") && !c.peekWords("RENAME", "CONSTRAINT"):
		c.acceptWords("RENAME")
		from := c.next().ident()
		c.expectWord("TO")
		to := c.next().ident()
		if col := t.column(from); col != nil {
			col.Name = to
		}

	case c.acceptWords("RENAME", "TO"):
		to := c.next().ident()
		delete(s.tables, t.Name)
		for idx, owner := range s.indexOf {
			if owner == t.Name {
				s.indexOf[idx] = to
			}
		}
		t.Name = to
		s.tables[to] = t

	case c.acceptWords("ATTACH", "PARTITION"):
		s.children[c.qualifiedName()] = true
		t.Partitioned = true

	case c.acceptWords("DETACH", "PARTITION"), c.acceptWords("OWNER", "TO"), c.acceptWords("SET"),
		c.acceptWords("RESET"), c.acceptWords("ENABLE"), c.acceptWords("DISABLE"),
		c.acceptWords("NO", "INHERIT"), c.acceptWords("INHERIT"), c.acceptWords("CLUSTER"),
		c.acceptWords("VALIDATE", "CONSTRAINT"), c.acceptWords("REPLICA", "IDENTITY"):
		// No schema meaning on MySQL.

	default:
		return fmt.Errorf("unsupported action %q", snippet(action))
	}
	return nil
}

// ---------------------------------------------------------------------------
// CREATE / ALTER / DROP INDEX
// ---------------------------------------------------------------------------

func (s *Schema) createIndex(c *cursor) error {
	c.pos = 1
	unique := c.acceptWords("UNIQUE")
	c.expectWord("INDEX")
	c.acceptWords("CONCURRENTLY")
	c.acceptWords("IF", "NOT", "EXISTS")
	var name string
	if !c.peekWord("ON") {
		name = c.qualifiedName()
	}
	c.expectWord("ON")
	c.acceptWords("ONLY")
	table := c.qualifiedName()
	method := "btree"
	if c.acceptWords("USING") {
		method = strings.ToLower(c.next().text)
	}
	body, err := c.group()
	if err != nil {
		return err
	}

	t := s.tables[table]
	if t == nil {
		if !s.children[table] {
			s.skip("index "+name, "on "+table+", which is not part of the schema model")
		}
		return nil
	}
	if name == "" {
		return fmt.Errorf("unnamed index on %s", table)
	}
	if _, exists := s.indexOf[name]; exists {
		return nil // IF NOT EXISTS
	}
	if s.retired[name] {
		s.skip("index "+name, "retired: NPG no longer builds it (retiredLogsPartitionedIndexes)")
		return nil
	}
	switch method {
	case "btree", "hash":
	default:
		s.skip("index "+name, method+" index; InnoDB has no equivalent")
		return nil
	}

	idx := &Index{Name: name, Unique: unique, Method: method}
	for _, part := range splitTopLevel(body) {
		idx.Parts = append(idx.Parts, parseIndexPart(part))
	}
	for !c.done() {
		switch {
		case c.acceptWords("INCLUDE"), c.acceptWords("WITH"):
			_, _ = c.group()
		case c.acceptWords("NULLS", "NOT", "DISTINCT"), c.acceptWords("NULLS", "DISTINCT"):
		case c.acceptWords("TABLESPACE"):
			c.next()
		case c.acceptWords("WHERE"):
			idx.Where = c.toks[c.pos:]
			c.pos = len(c.toks)
		default:
			return fmt.Errorf("index %s: unexpected %q", name, c.peek().text)
		}
	}
	t.Indexes = append(t.Indexes, idx)
	s.indexOf[name] = table
	return nil
}

var opclassSuffix = regexp.MustCompile(`^[a-z_0-9]+_ops$`)

func parseIndexPart(part []token) IndexPart {
	var p IndexPart
	// Strip trailing ordering, NULLS FIRST/LAST and an operator class.
	for len(part) > 1 {
		last := part[len(part)-1]
		switch {
		case last.is("DESC"):
			p.Desc = true
			part = part[:len(part)-1]
		case last.is("ASC"):
			part = part[:len(part)-1]
		case (last.is("FIRST") || last.is("LAST")) && len(part) > 2 && part[len(part)-2].is("NULLS"):
			part = part[:len(part)-2]
		case last.kind == tkWord && opclassSuffix.MatchString(strings.ToLower(last.text)):
			part = part[:len(part)-1]
			if len(part) > 2 && part[len(part)-1].isPunct(".") {
				part = part[:len(part)-2] // schema-qualified opclass
			}
		default:
			goto done
		}
	}
done:
	if len(part) == 1 && part[0].isIdent() {
		p.Column = part[0].ident()
		return p
	}
	// "(expr)" or function(...)
	if len(part) >= 2 && part[0].isPunct("(") && matchParen(part, 0) == len(part)-1 {
		part = part[1 : len(part)-1]
	}
	if len(part) == 1 && part[0].isIdent() && !part[0].is("TRUE") && !part[0].is("FALSE") {
		p.Column = part[0].ident()
		return p
	}
	p.Expr = part
	return p
}

func (s *Schema) alterIndex(c *cursor) {
	c.pos = 2
	c.acceptWords("IF", "EXISTS")
	name := c.qualifiedName()
	if c.acceptWords("RENAME", "TO") {
		to := c.next().ident()
		table, ok := s.indexOf[name]
		if !ok {
			return
		}
		if _, clash := s.indexOf[to]; clash {
			return
		}
		for _, idx := range s.tables[table].Indexes {
			if idx.Name == name {
				idx.Name = to
			}
		}
		delete(s.indexOf, name)
		s.indexOf[to] = table
	}
	// ATTACH PARTITION and SET (...) have no MySQL meaning.
}

func (s *Schema) dropIndex(c *cursor) {
	c.pos = 2
	c.acceptWords("CONCURRENTLY")
	c.acceptWords("IF", "EXISTS")
	for _, name := range c.nameList() {
		table, ok := s.indexOf[name]
		if !ok {
			continue
		}
		t := s.tables[table]
		for i, idx := range t.Indexes {
			if idx.Name == name {
				t.Indexes = append(t.Indexes[:i], t.Indexes[i+1:]...)
				break
			}
		}
		delete(s.indexOf, name)
	}
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// normaliseType renders a PostgreSQL type as lower-case words with the schema
// qualifier removed: "character varying(255)", "timestamp with time zone",
// "text[]", "block_reason".
func normaliseType(toks []token) string {
	var b strings.Builder
	var prev token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.isIdent() && i+1 < len(toks) && toks[i+1].isPunct(".") {
			i++ // schema qualifier
			continue
		}
		if b.Len() > 0 && prev.kind != tkPunct && t.kind != tkPunct {
			b.WriteByte(' ')
		}
		b.WriteString(strings.ToLower(t.text))
		prev = t
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// cursor
// ---------------------------------------------------------------------------

type cursor struct {
	toks []token
	pos  int
}

func (c *cursor) done() bool { return c.pos >= len(c.toks) }

func (c *cursor) peek() token {
	if c.done() {
		return token{}
	}
	return c.toks[c.pos]
}

func (c *cursor) next() token {
	t := c.peek()
	if !c.done() {
		c.pos++
	}
	return t
}

func (c *cursor) peekWord(w string) bool { return c.peek().is(w) }

func (c *cursor) peekPunct(p string) bool { return c.peek().isPunct(p) }

func (c *cursor) peekWords(words ...string) bool {
	for i, w := range words {
		if c.pos+i >= len(c.toks) || !c.toks[c.pos+i].is(w) {
			return false
		}
	}
	return true
}

func (c *cursor) acceptWords(words ...string) bool {
	if c.peekWords(words...) {
		c.pos += len(words)
		return true
	}
	return false
}

func (c *cursor) expectWord(w string) {
	if c.peekWord(w) {
		c.pos++
	}
}

// qualifiedName reads [schema.]name and returns the name.
func (c *cursor) qualifiedName() string {
	name := c.next().ident()
	for c.peekPunct(".") {
		c.pos++
		name = c.next().ident()
	}
	return name
}

// nameList reads comma-separated qualified names up to CASCADE/RESTRICT.
func (c *cursor) nameList() []string {
	var names []string
	for !c.done() && !c.peekWord("CASCADE") && !c.peekWord("RESTRICT") {
		names = append(names, c.qualifiedName())
		if c.peekPunct(",") {
			c.pos++
		}
	}
	return names
}

// group reads a parenthesised group and returns its inner tokens.
func (c *cursor) group() ([]token, error) {
	if !c.peekPunct("(") {
		return nil, fmt.Errorf("expected '(' near %q", snippet(c.toks[c.pos:]))
	}
	end := matchParen(c.toks, c.pos)
	if end < 0 {
		return nil, fmt.Errorf("unbalanced parentheses")
	}
	inner := c.toks[c.pos+1 : end]
	c.pos = end + 1
	return inner, nil
}

// until collects tokens at depth 0 until stop matches. When atLeastOne is
// set the first token is always taken (DEFAULT NULL).
func (c *cursor) until(stop func(token) bool, atLeastOne bool) []token {
	var out []token
	depth := 0
	for !c.done() {
		t := c.peek()
		if depth == 0 && stop(t) && !(atLeastOne && len(out) == 0) {
			break
		}
		if t.isPunct("(") || t.isPunct("[") {
			depth++
		} else if t.isPunct(")") || t.isPunct("]") {
			depth--
		}
		out = append(out, t)
		c.pos++
	}
	return out
}

func matchParen(toks []token, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch {
		case toks[i].isPunct("("):
			depth++
		case toks[i].isPunct(")"):
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits tokens at commas outside parentheses and brackets.
func splitTopLevel(toks []token) [][]token {
	var out [][]token
	depth, start := 0, 0
	for i, t := range toks {
		switch {
		case t.isPunct("(") || t.isPunct("["):
			depth++
		case t.isPunct(")") || t.isPunct("]"):
			depth--
		case t.isPunct(",") && depth == 0:
			out = append(out, toks[start:i])
			start = i + 1
		}
	}
	if start < len(toks) {
		out = append(out, toks[start:])
	}
	return out
}

func identList(toks []token) []string {
	var out []string
	for _, part := range splitTopLevel(toks) {
		if len(part) > 0 {
			out = append(out, part[0].ident())
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
