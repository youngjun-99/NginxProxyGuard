package dialect

import (
	"strings"
	"sync"
)

type stmtKind int

const (
	stmtOther stmtKind = iota
	stmtSelect
	stmtInsert
	stmtUpdate
	stmtDelete
)

// Plan is a translated statement.
//
// sql is what runs when the caller does not want rows back (Exec), and for
// every statement without RETURNING. A statement with RETURNING additionally
// carries ret, which the driver executes when rows are wanted: MySQL has no
// RETURNING, so the write and a read of the same rows are run together in one
// transaction (returning.go).
type Plan struct {
	sql *template
	ret *returning
}

// returning is a write statement taken apart for RETURNING emulation. Node
// lists are already translated; they are emitted at execution time, once the
// table's keys are known.
type returning struct {
	kind  stmtKind
	table string // MySQL table name, unquoted
	// ref is "t" or "t AS a": the FROM item the emulation reads the rows from.
	ref   []node
	alias string // qualifier the statement uses for the table ("" = the name)
	exprs []node // the RETURNING list

	// INSERT
	head      []node     // INSERT [IGNORE] INTO t
	columns   []string   // explicit column list, unquoted
	rows      [][][]node // VALUES rows, one node list per column
	tail      []node     // ON DUPLICATE KEY UPDATE ... (nil when absent)
	ignore    bool       // ON CONFLICT DO NOTHING
	upsert    bool       // ON CONFLICT DO UPDATE
	conflict  []string   // ON CONFLICT (cols) target
	constrain string     // ON CONFLICT ON CONSTRAINT name
	// conflictWhere is the arbiter predicate of ON CONFLICT (cols) WHERE ...
	conflictWhere []node
	// conflictExprs is the target when it is not plain columns.
	conflictExprs [][]node

	// UPDATE / DELETE
	set   []node // assignments after SET (UPDATE)
	where []node // condition after WHERE, nil when absent
}

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

// translations caches plans by statement text. The repositories' statements
// are a finite set, but a few are built with IN lists of varying length, so
// the cache is bounded: when it fills it is simply dropped and refilled.
var translations = struct {
	sync.RWMutex
	m map[string]*Plan
}{m: map[string]*Plan{}}

const maxCachedTranslations = 8192

// Translate rewrites one PostgreSQL statement for MySQL. Errors that wrap
// ErrUntranslatable mean the construct has no faithful MySQL form; the
// statement never reaches the server.
func Translate(query string) (*Plan, error) {
	translations.RLock()
	p, ok := translations.m[query]
	translations.RUnlock()
	if ok {
		return p, nil
	}
	p, err := translate(query)
	if err != nil {
		return nil, err
	}
	translations.Lock()
	if len(translations.m) >= maxCachedTranslations {
		translations.m = map[string]*Plan{}
	}
	translations.m[query] = p
	translations.Unlock()
	return p, nil
}

// TranslateSQL returns the translated text of query with `?` placeholders —
// what the driver sends when no rows are requested. For tests and tooling.
func TranslateSQL(query string) (string, error) {
	p, err := Translate(query)
	if err != nil {
		return "", err
	}
	return p.sql.text(), nil
}

// text renders the template with bare placeholders.
func (t *template) text() string {
	var b strings.Builder
	for _, s := range t.segs {
		if s.param != nil {
			switch s.param.kind {
			case paramArray:
				b.WriteString("?...")
			default:
				b.WriteString("?")
			}
			continue
		}
		b.WriteString(s.text)
	}
	return b.String()
}

func translate(query string) (*Plan, error) {
	toks, err := lex(query)
	if err != nil {
		return nil, err
	}
	items, err := parse(toks)
	if err != nil {
		return nil, err
	}
	for len(items) > 0 && items[len(items)-1].isPunct(";") {
		items = items[:len(items)-1]
	}
	for _, n := range items {
		if n.isPunct(";") {
			return nil, untranslatable("multiple statements in one call")
		}
	}
	items = stripPublic(items)

	r := &rewriter{}
	if items, err = r.rewriteList(items, ctx{}); err != nil {
		return nil, err
	}
	return r.statement(items)
}

// stripPublic removes the `public.` schema qualifier: on MySQL the database
// is the schema, and `public` would name another database.
func stripPublic(items []node) []node {
	out := items[:0:0]
	for i := 0; i < len(items); i++ {
		n := items[i]
		if n.is("public") && i+2 < len(items) && items[i+1].isPunct(".") && items[i+2].isIdentNode() {
			i++
			continue
		}
		if n.group != nil {
			g := *n.group
			g.items = stripPublic(g.items)
			n = node{group: &g}
		}
		out = append(out, n)
	}
	return out
}

// ---------------------------------------------------------------------------
// Statement level
// ---------------------------------------------------------------------------

func (r *rewriter) statement(items []node) (*Plan, error) {
	var err error
	// A leading WITH: find where the main statement starts.
	main := 0
	if len(items) > 0 && items[0].is("WITH") {
		k := 1
		if k < len(items) && items[k].is("RECURSIVE") {
			k++
		}
		for k < len(items) {
			// name [(cols)] AS (query)
			k++
			if k < len(items) && items[k].isParen() {
				k++
			}
			if k < len(items) && items[k].is("AS") {
				k++
			}
			if k >= len(items) || !items[k].isParen() {
				return nil, untranslatable("malformed WITH clause")
			}
			body := items[k].group.items
			if len(body) > 0 && (body[0].is("INSERT") || body[0].is("UPDATE") || body[0].is("DELETE")) {
				return nil, untranslatable("data-modifying CTE")
			}
			k++
			if k < len(items) && items[k].isPunct(",") {
				k++
				continue
			}
			break
		}
		main = k
	}
	if main >= len(items) {
		return finish(items, nil)
	}
	if items, err = shieldSubqueries(items, dmlTarget(items[main:])); err != nil {
		return nil, err
	}

	switch {
	case items[main].is("INSERT"):
		return r.insert(items, main)
	case items[main].is("UPDATE"):
		return r.update(items, main)
	case items[main].is("DELETE"):
		return r.delete(items, main)
	case items[main].is("TRUNCATE"):
		return truncate(items)
	case items[main].is("SET") || items[main].is("RESET") || items[main].is("VACUUM") || items[main].is("LISTEN") || items[main].is("NOTIFY") || items[main].is("DISCARD"):
		return nil, untranslatable("%s statement", strings.ToUpper(items[main].tok.text))
	case items[main].is("ANALYZE"):
		out := []node{kw("ANALYZE"), kw("TABLE")}
		return finish(append(out, items[main+1:]...), nil)
	}
	return finish(items, nil)
}

func finish(items []node, ret *returning) (*Plan, error) {
	t, err := emit(items)
	if err != nil {
		return nil, err
	}
	return &Plan{sql: t, ret: ret}, nil
}

func truncate(items []node) (*Plan, error) {
	out := []node{kw("TRUNCATE"), kw("TABLE")}
	k := 1
	if k < len(items) && items[k].is("TABLE") {
		k++
	}
	if k < len(items) && items[k].is("ONLY") {
		k++
	}
	name := []node{}
	for ; k < len(items); k++ {
		n := items[k]
		if n.isPunct(",") {
			return nil, untranslatable("TRUNCATE of several tables")
		}
		if n.is("RESTART") || n.is("CONTINUE") || n.is("CASCADE") || n.is("RESTRICT") {
			break
		}
		name = append(name, n)
	}
	return finish(append(out, name...), nil)
}

// topIndex finds word at the top level of items, from start.
func topIndex(items []node, from int, words ...string) int {
	for i := from; i < len(items); i++ {
		for _, w := range words {
			if items[i].is(w) {
				return i
			}
		}
	}
	return -1
}

// identName returns the unquoted name of an identifier node.
func identName(n node) string {
	if n.isKind(tkQuoted) {
		return strings.ReplaceAll(strings.Trim(n.tok.text, `"`), `""`, `"`)
	}
	return n.tok.text
}

// tableAndAlias reads `name [[AS] alias]` starting at k and returns the name,
// the alias ("" if none) and the index after them.
func tableAndAlias(items []node, k int, stops ...string) (string, string, int, bool) {
	if k < len(items) && items[k].is("ONLY") {
		k++
	}
	if k >= len(items) || !items[k].isIdentNode() {
		return "", "", k, false
	}
	name := identName(items[k])
	k++
	if k+1 < len(items) && items[k].isPunct(".") && items[k+1].isIdentNode() {
		name = identName(items[k+1]) // schema.table: MySQL has one schema
		k += 2
	}
	alias := ""
	if k < len(items) && items[k].is("AS") {
		k++
		if k < len(items) && items[k].isIdentNode() {
			alias = identName(items[k])
			k++
		}
		return name, alias, k, true
	}
	if k < len(items) && items[k].isIdentNode() {
		isStop := false
		for _, s := range stops {
			if items[k].is(s) {
				isStop = true
			}
		}
		if !isStop {
			alias = identName(items[k])
			k++
		}
	}
	return name, alias, k, true
}

func quotedIdent(name string) node {
	return node{tok: token{tkQuoted, `"` + strings.ReplaceAll(name, `"`, `""`) + `"`}}
}

// tableRef renders `t` or `t AS a`.
func tableRef(table, alias string) []node {
	out := []node{quotedIdent(table)}
	if alias != "" {
		out = append(out, kw("AS"), quotedIdent(alias))
	}
	return out
}

// requalify replaces `from.` qualifiers with `to.` throughout items.
func requalify(items []node, from, to string) []node {
	out := clone(items)
	for i := 0; i+1 < len(out); i++ {
		if out[i].isIdentNode() && strings.EqualFold(identName(out[i]), from) && out[i+1].isPunct(".") &&
			(i == 0 || !out[i-1].isPunct(".")) {
			out[i] = quotedIdent(to)
		}
		if out[i].group != nil {
			g := *out[i].group
			g.items = requalify(g.items, from, to)
			out[i] = node{group: &g}
		}
	}
	if n := len(out); n > 0 && out[n-1].group != nil {
		g := *out[n-1].group
		g.items = requalify(g.items, from, to)
		out[n-1] = node{group: &g}
	}
	return out
}

// ---------------------------------------------------------------------------
// INSERT
// ---------------------------------------------------------------------------

func (r *rewriter) insert(items []node, main int) (*Plan, error) {
	with := clone(items[:main])
	body := items[main:]
	if len(body) < 3 || !body[1].is("INTO") {
		return nil, untranslatable("malformed INSERT")
	}
	table, alias, k, ok := tableAndAlias(body, 2, "VALUES", "SELECT", "DEFAULT", "WITH", "OVERRIDING")
	if !ok {
		return nil, untranslatable("malformed INSERT target")
	}

	var colList *node
	var columns []string
	if k < len(body) && body[k].isParen() && !(len(body[k].group.items) > 0 && (body[k].group.items[0].is("SELECT") || body[k].group.items[0].is("WITH"))) {
		c := body[k]
		colList = &c
		for _, part := range splitTop(c.group.items) {
			if len(part) != 1 || !part[0].isIdentNode() {
				return nil, untranslatable("INSERT column list")
			}
			columns = append(columns, identName(part[0]))
		}
		k++
	}

	conflictAt := -1
	for i := k; i+1 < len(body); i++ {
		if body[i].is("ON") && body[i+1].is("CONFLICT") {
			conflictAt = i
			break
		}
	}
	retAt := topIndex(body, k, "RETURNING")
	srcEnd := len(body)
	if conflictAt >= 0 {
		srcEnd = conflictAt
	} else if retAt >= 0 {
		srcEnd = retAt
	}
	source := clone(body[k:srcEnd])

	// DEFAULT VALUES
	if len(source) == 2 && source[0].is("DEFAULT") && source[1].is("VALUES") {
		source = []node{paren(), kw("VALUES"), paren()}
		if colList != nil {
			return nil, untranslatable("DEFAULT VALUES with a column list")
		}
	}

	// A WITH in front of INSERT moves in front of its SELECT, the only place
	// MySQL accepts it.
	if len(with) > 0 {
		if len(source) == 0 || !source[0].is("SELECT") {
			return nil, untranslatable("WITH ... INSERT ... VALUES")
		}
		source = append(with, source...)
	}

	ret := &returning{kind: stmtInsert, table: table, columns: columns}
	if alias != "" {
		ret.alias = alias
	}

	var tail []node
	if conflictAt >= 0 {
		end := len(body)
		if retAt >= 0 {
			end = retAt
		}
		clause := body[conflictAt+2 : end]
		c := 0
		if c < len(clause) && clause[c].isParen() {
			for _, part := range splitTop(clause[c].group.items) {
				// An expression target — (lower(name)) for a case-insensitive
				// unique index — is kept whole: RETURNING finds the row again
				// by evaluating it over the inserted values.
				ret.conflictExprs = append(ret.conflictExprs, clone(part))
				if len(part) == 1 && part[0].isIdentNode() {
					ret.conflict = append(ret.conflict, identName(part[0]))
				}
			}
			if len(ret.conflict) != len(ret.conflictExprs) {
				ret.conflict = nil
			} else {
				ret.conflictExprs = nil
			}
			c++
			if c < len(clause) && clause[c].is("WHERE") {
				// ON CONFLICT (col) WHERE pred: the arbiter is a partial unique
				// index, which the MySQL schema renders as a functional unique
				// key that only covers rows matching pred. ON DUPLICATE KEY
				// fires on it the same way; the predicate is kept to find the
				// row again for RETURNING.
				w := c + 1
				for c < len(clause) && !clause[c].is("DO") {
					c++
				}
				ret.conflictWhere = clone(clause[w:c])
			}
		} else if c+2 < len(clause) && clause[c].is("ON") && clause[c+1].is("CONSTRAINT") {
			ret.constrain = identName(clause[c+2])
			c += 3
		}
		if c >= len(clause) || !clause[c].is("DO") || c+1 >= len(clause) {
			return nil, untranslatable("malformed ON CONFLICT")
		}
		switch {
		case clause[c+1].is("NOTHING"):
			ret.ignore = true
		case clause[c+1].is("UPDATE"):
			if c+2 >= len(clause) || !clause[c+2].is("SET") {
				return nil, untranslatable("malformed ON CONFLICT DO UPDATE")
			}
			sets := clause[c+3:]
			if w := topIndex(sets, 0, "WHERE"); w >= 0 {
				return nil, untranslatable("conditional upsert (ON CONFLICT ... DO UPDATE ... WHERE)")
			}
			sets = excludedToValues(sets)
			if alias != "" {
				sets = requalify(sets, alias, table)
			}
			var err error
			if sets, err = orderAssignments(sets, []string{table, alias}); err != nil {
				return nil, err
			}
			tail = append([]node{kw("ON"), kw("DUPLICATE"), kw("KEY"), kw("UPDATE")}, sets...)
			ret.upsert = true
		default:
			return nil, untranslatable("ON CONFLICT action")
		}
	}

	head := []node{kw("INSERT")}
	if ret.ignore {
		head = append(head, kw("IGNORE"))
	}
	head = append(head, kw("INTO"), quotedIdent(table))
	ret.head = head
	ret.tail = tail

	stmt := clone(head)
	if colList != nil {
		stmt = append(stmt, *colList)
	}
	stmt = append(stmt, source...)
	stmt = append(stmt, tail...)

	if retAt < 0 {
		return finish(stmt, nil)
	}

	ret.exprs = clone(body[retAt+1:])
	if alias != "" {
		ret.ref = tableRef(table, alias)
	} else {
		ret.ref = tableRef(table, "")
	}
	// Only INSERT ... VALUES can be read back: the rows are known.
	if len(source) > 0 && source[0].is("VALUES") {
		for _, row := range splitTop(source[1:]) {
			if len(row) != 1 || !row[0].isParen() {
				return nil, untranslatable("INSERT ... VALUES row shape")
			}
			vals := splitTop(row[0].group.items)
			if len(columns) > 0 && len(vals) != len(columns) {
				return nil, untranslatable("INSERT VALUES arity")
			}
			ret.rows = append(ret.rows, vals)
		}
	} else if len(source) == 3 && source[1].is("VALUES") { // DEFAULT VALUES
		ret.rows = [][][]node{{}}
	} else {
		ret.rows = nil // INSERT ... SELECT: only Exec works
	}
	// An upsert is read back by its conflict target, so every target column
	// must be among the inserted ones. Checked here so the sweep test catches
	// a statement that would otherwise fail only when it runs.
	if ret.upsert && ret.rows != nil && len(columns) > 0 {
		for _, col := range ret.conflict {
			if indexOf(columns, col) < 0 {
				return nil, untranslatable("upsert RETURNING: conflict column %s is not inserted", col)
			}
		}
		for _, e := range ret.conflictExprs {
			if _, used := substituteColumns(e, columns, make([][]node, len(columns))); !used {
				return nil, untranslatable("upsert RETURNING: conflict expression reads no inserted column")
			}
		}
	}
	return finish(stmt, ret)
}

// excludedToValues turns EXCLUDED.col into VALUES(col).
func excludedToValues(items []node) []node {
	out := make([]node, 0, len(items))
	for i := 0; i < len(items); i++ {
		n := items[i]
		if n.is("EXCLUDED") && i+2 < len(items) && items[i+1].isPunct(".") && items[i+2].isIdentNode() {
			out = append(out, call("VALUES", []node{items[i+2]})...)
			i += 2
			continue
		}
		if n.group != nil {
			g := *n.group
			g.items = excludedToValues(g.items)
			n = node{group: &g}
		}
		out = append(out, n)
	}
	return out
}

// ---------------------------------------------------------------------------
// UPDATE
// ---------------------------------------------------------------------------

func (r *rewriter) update(items []node, main int) (*Plan, error) {
	with := clone(items[:main])
	body := items[main:]
	table, alias, k, ok := tableAndAlias(body, 1, "SET")
	if !ok || k >= len(body) || !body[k].is("SET") {
		return nil, untranslatable("malformed UPDATE")
	}
	fromAt := topIndex(body, k+1, "FROM")
	whereAt := topIndex(body, k+1, "WHERE")
	retAt := topIndex(body, k+1, "RETURNING")
	setEnd := len(body)
	for _, x := range []int{fromAt, whereAt, retAt} {
		if x >= 0 && x < setEnd {
			setEnd = x
		}
	}
	sets := clone(body[k+1 : setEnd])
	for _, part := range splitTop(sets) {
		if len(part) > 0 && part[0].isParen() {
			return nil, untranslatable("UPDATE ... SET (a, b) = (...)")
		}
	}
	sets, err := orderAssignments(sets, []string{table, alias})
	if err != nil {
		return nil, err
	}
	var where []node
	if whereAt >= 0 {
		end := len(body)
		if retAt >= 0 {
			end = retAt
		}
		where = clone(body[whereAt+1 : end])
	}

	ref := tableRef(table, alias)
	qual := table
	if alias != "" {
		qual = alias
	}

	stmt := append(clone(with), kw("UPDATE"))
	stmt = append(stmt, ref...)
	if fromAt >= 0 {
		// UPDATE t SET ... FROM f WHERE ...  ->  UPDATE t, f SET t.col = ... WHERE ...
		if retAt >= 0 {
			return nil, untranslatable("UPDATE ... FROM ... RETURNING")
		}
		end := len(body)
		if whereAt >= 0 {
			end = whereAt
		}
		stmt = append(stmt, comma())
		stmt = append(stmt, clone(body[fromAt+1:end])...)
		sets = qualifySetTargets(sets, qual)
	}
	stmt = append(stmt, kw("SET"))
	stmt = append(stmt, sets...)
	if where != nil {
		stmt = append(stmt, kw("WHERE"))
		stmt = append(stmt, where...)
	}
	if retAt < 0 {
		return finish(stmt, nil)
	}
	if len(with) > 0 {
		return nil, untranslatable("WITH ... UPDATE ... RETURNING")
	}
	ret := &returning{kind: stmtUpdate, table: table, alias: alias, ref: ref, set: sets, where: where, exprs: clone(body[retAt+1:])}
	return finish(stmt, ret)
}

// qualifySetTargets prefixes each SET target with the table's qualifier, so a
// multi-table UPDATE does not find the column ambiguous.
func qualifySetTargets(sets []node, qual string) []node {
	parts := splitTop(sets)
	for i, p := range parts {
		if len(p) >= 2 && p[0].isIdentNode() && p[1].isOp("=") {
			parts[i] = append([]node{quotedIdent(qual), dot()}, p...)
		}
	}
	return joinTop(parts)
}

// ---------------------------------------------------------------------------
// DELETE
// ---------------------------------------------------------------------------

func (r *rewriter) delete(items []node, main int) (*Plan, error) {
	with := clone(items[:main])
	body := items[main:]
	if len(body) < 3 || !body[1].is("FROM") {
		return nil, untranslatable("malformed DELETE")
	}
	table, alias, k, ok := tableAndAlias(body, 2, "USING", "WHERE", "RETURNING")
	if !ok {
		return nil, untranslatable("malformed DELETE target")
	}
	usingAt := topIndex(body, k, "USING")
	whereAt := topIndex(body, k, "WHERE")
	retAt := topIndex(body, k, "RETURNING")
	var where []node
	if whereAt >= 0 {
		end := len(body)
		if retAt >= 0 {
			end = retAt
		}
		where = clone(body[whereAt+1 : end])
	}
	ref := tableRef(table, alias)

	var stmt []node
	if usingAt >= 0 {
		// DELETE FROM t a USING u b WHERE ...  ->  DELETE a FROM t a, u b WHERE ...
		if retAt >= 0 {
			return nil, untranslatable("DELETE ... USING ... RETURNING")
		}
		end := len(body)
		if whereAt >= 0 {
			end = whereAt
		}
		target := table
		if alias != "" {
			target = alias
		}
		stmt = append(clone(with), kw("DELETE"), quotedIdent(target), kw("FROM"))
		stmt = append(stmt, ref...)
		stmt = append(stmt, comma())
		stmt = append(stmt, clone(body[usingAt+1:end])...)
	} else {
		stmt = append(clone(with), kw("DELETE"), kw("FROM"))
		stmt = append(stmt, ref...)
	}
	if where != nil {
		stmt = append(stmt, kw("WHERE"))
		stmt = append(stmt, where...)
	}
	if retAt < 0 {
		return finish(stmt, nil)
	}
	if len(with) > 0 {
		return nil, untranslatable("WITH ... DELETE ... RETURNING")
	}
	ret := &returning{kind: stmtDelete, table: table, alias: alias, ref: ref, where: where, exprs: clone(body[retAt+1:])}
	return finish(stmt, ret)
}

// PreparableSQL returns the MySQL statements a plan sends, with every
// placeholder a plain `?` (an array argument as a single element, an interval
// argument as INTERVAL 1 SECOND). It exists for validation tooling: asking the
// server to PREPARE the result checks the translation's syntax and every
// table and column it names, without running anything. A RETURNING plan adds
// the read-back SELECT it issues.
func PreparableSQL(query string) ([]string, error) {
	p, err := Translate(query)
	if err != nil {
		return nil, err
	}
	out := []string{p.sql.preparable()}
	if p.ret != nil {
		q := append([]node{kw("SELECT")}, p.ret.exprs...)
		q = append(q, kw("FROM"))
		q = append(q, p.ret.ref...)
		q = append(q, kw("WHERE"), kw("FALSE"))
		t, err := emit(q)
		if err != nil {
			return nil, err
		}
		out = append(out, t.preparable())
	}
	return out, nil
}

func (t *template) preparable() string {
	var b strings.Builder
	for _, s := range t.segs {
		switch {
		case s.param == nil:
			b.WriteString(s.text)
		case s.param.kind == paramInterval:
			b.WriteString("INTERVAL 1 SECOND")
		default:
			b.WriteString("?")
		}
	}
	return b.String()
}

// dmlTarget returns the table an INSERT, UPDATE or DELETE writes, or "".
func dmlTarget(body []node) string {
	k := -1
	switch {
	case len(body) > 2 && body[0].is("INSERT") && body[1].is("INTO"):
		// INSERT ... SELECT may read its own target: MySQL buffers the rows
		// through a temporary table. Only INSERT ... VALUES is refused.
		if topIndex(body, 2, "VALUES") < 0 {
			return ""
		}
		k = 2
	case len(body) > 1 && body[0].is("UPDATE"):
		k = 1
	case len(body) > 2 && body[0].is("DELETE") && body[1].is("FROM"):
		k = 2
	}
	if k < 0 {
		return ""
	}
	if body[k].is("ONLY") {
		k++
	}
	if k < len(body) && body[k].isIdentNode() {
		if k+2 < len(body) && body[k+1].isPunct(".") && body[k+2].isIdentNode() {
			return identName(body[k+2])
		}
		return identName(body[k])
	}
	return ""
}

// shieldSubqueries turns two kinds of subquery MySQL refuses into derived
// tables, which it accepts and materialises:
//
//   - a subquery reading the table the statement writes (error 1093, "can't
//     specify target table for update in FROM clause"), e.g. the next
//     sort_order computed inside INSERT ... VALUES;
//   - IN (SELECT ... LIMIT n) (error 1235), the batched-delete idiom.
//
// A correlated subquery cannot become a derived table (it would lose sight of
// the outer row), so EXISTS (...) reading the written table is refused as
// untranslatable instead of failing on the server.
func shieldSubqueries(items []node, target string) ([]node, error) {
	out := clone(items)
	for i, n := range out {
		if !n.isParen() {
			continue
		}
		g := n.group.items
		isQuery := len(g) > 0 && (g[0].is("SELECT") || g[0].is("WITH"))
		underExists := i > 0 && out[i-1].is("EXISTS")
		underIn := i > 0 && (out[i-1].is("IN") || out[i-1].is("ANY") || out[i-1].is("ALL") || out[i-1].is("SOME"))
		selfRead := isQuery && target != "" && readsTable(g, target)
		if selfRead && underExists {
			return nil, untranslatable("EXISTS subquery reading %s inside a statement that writes it (MySQL error 1093)", target)
		}
		if isQuery && (selfRead || (underIn && topIndex(g, 0, "LIMIT") >= 0)) {
			wrapped := []node{kw("SELECT"), op("*"), kw("FROM"), paren(g...), kw("AS"), kw("npg_derived")}
			out[i] = paren(wrapped...)
			continue
		}
		inner, err := shieldSubqueries(g, target)
		if err != nil {
			return nil, err
		}
		cp := *n.group
		cp.items = inner
		out[i] = node{group: &cp}
	}
	return out, nil
}

// readsTable reports whether a query names table in a FROM or JOIN at its own
// level.
func readsTable(items []node, table string) bool {
	for i := 1; i < len(items); i++ {
		if !items[i].isIdentNode() || !strings.EqualFold(identName(items[i]), table) {
			continue
		}
		prev := items[i-1]
		if prev.is("FROM") || prev.is("JOIN") || prev.isPunct(",") {
			return true
		}
	}
	return false
}

// orderAssignments reorders a SET list so that MySQL computes what
// PostgreSQL would.
//
// PostgreSQL evaluates every expression of an UPDATE's SET list (and of
// ON CONFLICT DO UPDATE) against the row as it was. MySQL assigns left to
// right and lets later expressions see the values just assigned, so
// `SET failures = 0, enabled = CASE WHEN failures >= 10 ...` tests the new 0.
// Moving each assignment ahead of the ones that overwrite the columns it reads
// restores PostgreSQL's meaning. A cycle — a = b, b = a — has no such order and
// is refused.
func orderAssignments(sets []node, qualifiers []string) ([]node, error) {
	parts := splitTop(sets)
	if len(parts) < 2 {
		return sets, nil
	}
	targets := make([]string, len(parts))
	exprs := make([][]node, len(parts))
	for i, p := range parts {
		eq := -1
		for k, n := range p {
			if n.isOp("=") {
				eq = k
				break
			}
		}
		if eq < 1 || !p[eq-1].isIdentNode() {
			return sets, nil // not a plain assignment list; leave it
		}
		targets[i] = strings.ToLower(identName(p[eq-1]))
		exprs[i] = p[eq+1:]
	}
	assignedBy := map[string]int{}
	for i, t := range targets {
		assignedBy[t] = i
	}
	quals := map[string]bool{}
	for _, q := range qualifiers {
		if q != "" {
			quals[strings.ToLower(q)] = true
		}
	}
	// before[j] lists the assignments j must precede: those overwriting a
	// column j reads.
	before := make([][]int, len(parts))
	violation := false
	for j := range parts {
		for col := range columnReads(exprs[j], quals) {
			if i, ok := assignedBy[col]; ok && i != j {
				before[j] = append(before[j], i)
				if i < j {
					violation = true
				}
			}
		}
	}
	if !violation {
		return sets, nil
	}
	// Stable topological order: repeatedly emit the earliest assignment that
	// no remaining assignment must precede.
	placed := make([]bool, len(parts))
	var order []int
	for len(order) < len(parts) {
		progressed := false
		for c := range parts {
			if placed[c] {
				continue
			}
			blocked := false
			for j := range parts {
				if placed[j] || j == c {
					continue
				}
				for _, i := range before[j] {
					if i == c {
						blocked = true // j reads what c overwrites: j goes first
					}
				}
			}
			if !blocked {
				placed[c] = true
				order = append(order, c)
				progressed = true
				break
			}
		}
		if !progressed {
			return nil, untranslatable("SET list whose assignments read each other's columns in a cycle")
		}
	}
	reordered := make([][]node, len(order))
	for k, i := range order {
		reordered[k] = parts[i]
	}
	return joinTop(reordered), nil
}

// columnReads collects the column names an expression reads, unqualified or
// qualified by the table being written. Subqueries and VALUES(col) (the
// inserted row in ON DUPLICATE KEY UPDATE) are not reads of the old row.
func columnReads(expr []node, quals map[string]bool) map[string]bool {
	out := map[string]bool{}
	var walk func(items []node)
	walk = func(items []node) {
		for i := 0; i < len(items); i++ {
			n := items[i]
			if n.group != nil {
				g := n.group.items
				if len(g) > 0 && (g[0].is("SELECT") || g[0].is("WITH")) {
					continue
				}
				if i > 0 && items[i-1].is("VALUES") {
					continue
				}
				walk(g)
				continue
			}
			if !n.isIdentNode() || n.param != nil {
				continue
			}
			if i+1 < len(items) && items[i+1].isParen() {
				continue // a function name
			}
			if i+1 < len(items) && items[i+1].isPunct(".") {
				continue // a qualifier; the column follows
			}
			if i >= 2 && items[i-1].isPunct(".") {
				if !quals[strings.ToLower(identName(items[i-2]))] {
					continue // another table's column
				}
			}
			out[strings.ToLower(identName(n))] = true
		}
	}
	walk(expr)
	return out
}
