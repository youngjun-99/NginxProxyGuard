package dialect

import (
	"fmt"
	"strconv"
	"strings"
)

// rewriter carries per-statement state through the rewrite passes.
type rewriter struct{}

// ctx describes where a node list sits, for the few rules that depend on it.
type ctx struct {
	// parentFunc is the lower-cased word right before the group this list is
	// the content of ("any", "count", "in", ...), or "".
	parentFunc string
}

// keywordsNotFunctions are words that can precede a parenthesised group
// without being a function name, so `AND (x)::int` casts the group alone.
var keywordsNotFunctions = map[string]bool{
	"and": true, "or": true, "not": true, "in": true, "is": true, "like": true, "ilike": true,
	"then": true, "else": true, "when": true, "case": true, "where": true, "on": true,
	"select": true, "from": true, "by": true, "as": true, "at": true, "values": true,
	"returning": true, "set": true, "having": true, "join": true, "using": true, "into": true,
	"union": true, "all": true, "any": true, "some": true, "distinct": true, "limit": true,
	"offset": true, "between": true, "with": true, "over": true, "filter": true, "lateral": true,
	"update": true, "delete": true, "insert": true, "do": true, "nothing": true, "conflict": true,
	"except": true, "intersect": true, "end": true, "row": true, "recursive": true, "escape": true,
	"desc": true, "asc": true, "partition": true, "order": true, "group": true, "window": true,
	"materialized": true, "table": true, "only": true, "if": true, "member": true, "of": true,
	"interval": true, "ignore": true, "key": true, "exists": false,
}

// isFuncName reports whether n can be the name of a function called with the
// group that follows it.
func isFuncName(n node) bool {
	if n.param != nil || n.group != nil {
		return false
	}
	if n.tok.kind == tkQuoted {
		return true
	}
	return n.tok.kind == tkWord && !keywordsNotFunctions[n.lower()]
}

// primaryStart returns the index of the first node of the primary expression
// that ends at end: a literal, parameter, (qualified) name, function call,
// parenthesised group or ARRAY[...], with any trailing subscripts.
func primaryStart(items []node, end int) int {
	j := end
	for j > 0 && items[j].isBracket() && !items[j-1].is("ARRAY") {
		j--
	}
	n := items[j]
	switch {
	case n.isParen():
		if j > 0 && isFuncName(items[j-1]) {
			j--
			for j >= 2 && items[j-1].isPunct(".") && items[j-2].isIdentNode() {
				j -= 2
			}
		}
	case n.isBracket():
		if j > 0 && items[j-1].is("ARRAY") {
			j--
		}
	case n.isIdentNode():
		for j >= 2 && items[j-1].isPunct(".") && items[j-2].isIdentNode() {
			j -= 2
		}
	case n.isKind(tkString):
		if j > 0 && (items[j-1].is("INTERVAL") || items[j-1].is("DATE") || items[j-1].is("TIMESTAMP") || items[j-1].is("TIME")) {
			j--
		}
	}
	return j
}

// Operand boundaries for binary operators at comparison precedence.

// stopBefore are tokens that end an operand scanning rightwards.
var stopWords = map[string]bool{
	"and": true, "or": true, "then": true, "else": true, "end": true, "when": true,
	"where": true, "group": true, "order": true, "limit": true, "offset": true,
	"having": true, "union": true, "except": true, "intersect": true, "from": true,
	"as": true, "on": true, "using": true, "join": true, "inner": true, "left": true,
	"right": true, "full": true, "cross": true, "returning": true, "escape": true,
	"like": true, "ilike": true, "in": true, "is": true, "between": true, "not": true,
	"set": true, "values": true, "select": true, "for": true, "window": true,
	"asc": true, "desc": true, "nulls": true, "collate": true, "do": true, "isnull": true,
	"notnull": true, "similar": true, "member": true, "fetch": true,
}

var comparisonOps = map[string]bool{
	"=": true, "<>": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
	"~": true, "~*": true, "!~": true, "!~*": true, "<=>": true, "@>": true, "<@": true, "&&": true,
}

func isOperandStop(n node) bool {
	if n.param != nil || n.group != nil {
		return false
	}
	switch n.tok.kind {
	case tkPunct:
		return n.tok.text == "," || n.tok.text == ";"
	case tkOp:
		return comparisonOps[n.tok.text]
	case tkWord:
		return stopWords[n.lower()]
	}
	return false
}

// operandRight returns the last index of the operand that starts at start.
func operandRight(items []node, start int) int {
	end := start
	for k := start + 1; k < len(items); k++ {
		if isOperandStop(items[k]) {
			break
		}
		end = k
	}
	return end
}

// operandLeft returns the first index of the operand that ends at end.
func operandLeft(items []node, end int) int {
	start := end
	for k := end - 1; k >= 0; k-- {
		if isOperandStop(items[k]) || items[k].is("CASE") || items[k].is("BY") || items[k].is("DISTINCT") {
			break
		}
		start = k
	}
	return start
}

// splitTop splits a node list on top-level commas.
func splitTop(items []node) [][]node {
	var out [][]node
	start := 0
	for i, n := range items {
		if n.isPunct(",") {
			out = append(out, items[start:i])
			start = i + 1
		}
	}
	return append(out, items[start:])
}

// joinTop joins node lists with commas.
func joinTop(parts [][]node) []node {
	var out []node
	for i, p := range parts {
		if i > 0 {
			out = append(out, comma())
		}
		out = append(out, p...)
	}
	return out
}

func clone(items []node) []node { return append([]node(nil), items...) }

func replace(items []node, from, to int, with []node) []node {
	out := make([]node, 0, len(items)-(to-from+1)+len(with))
	out = append(out, items[:from]...)
	out = append(out, with...)
	return append(out, items[to+1:]...)
}

func indexWord(items []node, word string, from int) int {
	for i := from; i < len(items); i++ {
		if items[i].is(word) {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// The per-level pipeline
// ---------------------------------------------------------------------------

// rewriteList rewrites a node list and, first, every group inside it.
func (r *rewriter) rewriteList(items []node, c ctx) ([]node, error) {
	// Bottom-up: inner expressions and subqueries are finished before the
	// rules at this level look at them as single operands.
	for i := range items {
		if items[i].group == nil {
			continue
		}
		inner := ctx{}
		if items[i].isParen() && i > 0 && items[i-1].isWordNode() {
			inner.parentFunc = items[i-1].lower()
		}
		g := *items[i].group
		var err error
		if g.items, err = r.rewriteList(g.items, inner); err != nil {
			return nil, err
		}
		items[i] = node{group: &g}
	}

	steps := []func([]node, ctx) ([]node, error){
		r.params,
		r.rejectUnsupported,
		r.subscripts,
		r.arrays,
		r.filterClauses,
		r.functions,
		r.casts,
		r.jsonOps,
		r.overlaps,
		r.anyAll,
		r.likes,
		r.regexOps,
		r.distinctFrom,
		r.atTimeZone,
		r.intervals,
		r.clauses,
		r.quoteReserved,
	}
	var err error
	for _, step := range steps {
		if items, err = step(items, c); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// params turns $N tokens into argument references.
func (r *rewriter) params(items []node, _ ctx) ([]node, error) {
	for i, n := range items {
		if n.isKind(tkParam) {
			p, err := paramNode(n.tok)
			if err != nil {
				return nil, err
			}
			items[i] = p
		}
	}
	return items, nil
}

// rejectUnsupported fails fast on constructs with no MySQL form, so they are
// reported as untranslatable instead of reaching the server as a syntax error.
var unsupportedFunctions = map[string]string{
	"unnest":                    "set-returning array function",
	"generate_series":           "set-returning function",
	"array_position":            "array function",
	"array_remove":              "array function",
	"array_append":              "array function",
	"array_prepend":             "array function",
	"array_cat":                 "array function",
	"array_to_string":           "array function",
	"regexp_matches":            "regexp set-returning function",
	"regexp_split_to_table":     "regexp set-returning function",
	"regexp_split_to_array":     "regexp function",
	"jsonb_each":                "jsonb set-returning function",
	"jsonb_each_text":           "jsonb set-returning function",
	"json_each":                 "json set-returning function",
	"json_each_text":            "json set-returning function",
	"jsonb_array_elements":      "jsonb set-returning function",
	"jsonb_array_elements_text": "jsonb set-returning function",
	"jsonb_object_keys":         "jsonb set-returning function",
	"jsonb_set":                 "jsonb function",
	"jsonb_insert":              "jsonb function",
	"to_jsonb":                  "jsonb function",
	"to_json":                   "json function",
	"row_to_json":               "json function",
	"percentile_cont":           "ordered-set aggregate",
	"percentile_disc":           "ordered-set aggregate",
	"age":                       "interval function",
	"to_char":                   "formatting function",
	"format":                    "formatting function",
	"to_regclass":               "catalog function",
	"current_setting":           "configuration function",
	"set_config":                "configuration function",
	"pg_try_advisory_lock":      "advisory lock",
	"pg_advisory_lock":          "advisory lock",
	"pg_advisory_unlock":        "advisory lock",
	"pg_advisory_unlock_all":    "advisory lock",
	"pg_advisory_xact_lock":     "advisory lock",
	"time_bucket":               "TimescaleDB function",
	"hypertable_size":           "TimescaleDB function",
	"drop_chunks":               "TimescaleDB function",
	"show_chunks":               "TimescaleDB function",
	"compress_chunk":            "TimescaleDB function",
	"decompress_chunk":          "TimescaleDB function",
	"approximate_row_count":     "TimescaleDB function",
	"txid_current":              "transaction id function",
	"masklen":                   "inet function",
	"abbrev":                    "inet function",
	"set_masklen":               "inet function",
	"inet_merge":                "inet function",
	"make_timestamptz":          "date/time function",
}

func (r *rewriter) rejectUnsupported(items []node, _ ctx) ([]node, error) {
	for i, n := range items {
		if n.isKind(tkDollar) {
			return nil, untranslatable("dollar-quoted PL/pgSQL body")
		}
		if n.isWordNode() && i+1 < len(items) && items[i+1].isParen() {
			name := n.lower()
			if reason, bad := unsupportedFunctions[name]; bad {
				return nil, untranslatable("%s() (%s)", name, reason)
			}
			if strings.HasPrefix(name, "pg_") && name != "pg_sleep" {
				return nil, untranslatable("%s() (PostgreSQL catalog function)", name)
			}
		}
		if n.is("DISTINCT") && i+2 < len(items) && items[i+1].is("ON") && items[i+2].isParen() {
			return nil, untranslatable("SELECT DISTINCT ON")
		}
		if n.is("SIMILAR") && i+1 < len(items) && items[i+1].is("TO") {
			return nil, untranslatable("SIMILAR TO")
		}
		if n.isWordNode() && (strings.HasPrefix(n.lower(), "_timescaledb") || strings.HasPrefix(n.lower(), "timescaledb_information") || n.lower() == "pg_class" || n.lower() == "pg_namespace" || n.lower() == "pg_inherits" || n.lower() == "pg_stat_user_tables" || n.lower() == "pg_indexes" || n.lower() == "pg_constraint") {
			return nil, untranslatable("PostgreSQL/TimescaleDB catalog %s", n.tok.text)
		}
		if n.isOp("@>") || n.isOp("<@") || n.isOp("?|") || n.isOp("?&") {
			return nil, untranslatable("containment/overlap operator %s", n.tok.text)
		}
		if n.is("COPY") && i == 0 {
			return nil, untranslatable("COPY")
		}
	}
	return items, nil
}

// ---------------------------------------------------------------------------
// Arrays
// ---------------------------------------------------------------------------

// arrayJSON renders a text[] column (a lib/pq array literal, every element
// double-quoted) as a JSON array MySQL can search:
// '{"a","b"}' -> '["a","b"]'.
func arrayJSON(expr []node) []node {
	inner := []node{lit("[")}
	inner = append(inner, comma())
	inner = append(inner, call("SUBSTRING", expr, []node{num("2")}, append(call("CHAR_LENGTH", expr), op("-"), num("2")))...)
	inner = append(inner, comma(), lit("]"))
	cat := call("CONCAT", inner)
	return call("CAST", append(cat, kw("AS"), kw("JSON")))
}

// subscripts rewrites col[n] on a text[] value into a JSON element lookup.
func (r *rewriter) subscripts(items []node, _ ctx) ([]node, error) {
	for i := 1; i < len(items); i++ {
		if !items[i].isBracket() || items[i-1].is("ARRAY") {
			continue
		}
		prev := items[i-1]
		if !(prev.isIdentNode() || prev.isParen() || prev.isBracket()) {
			continue
		}
		start := primaryStart(items, i-1)
		base := clone(items[start:i])
		idx := items[i].group.items
		for _, n := range idx {
			if n.isOp(":") {
				return nil, untranslatable("array slice")
			}
		}
		var path []node
		switch {
		case len(idx) == 1 && idx[0].isKind(tkNumber):
			k, err := strconv.Atoi(idx[0].tok.text)
			if err != nil {
				return nil, untranslatable("array subscript %s", idx[0].tok.text)
			}
			path = []node{lit(fmt.Sprintf("$[%d]", k-1))}
		default:
			path = call("CONCAT", []node{lit("$[")}, append(clone(idx), op("-"), num("1")), []node{lit("]")})
		}
		repl := call("JSON_UNQUOTE", call("JSON_EXTRACT", arrayJSON(base), path))
		items = replace(items, start, i, repl)
		i = start + len(repl) - 1
	}
	return items, nil
}

// pgArrayLiteral renders string/number elements as the lib/pq array literal.
func pgArrayLiteral(elems [][]node) (string, bool) {
	parts := make([]string, 0, len(elems))
	for _, e := range elems {
		if len(e) != 1 {
			return "", false
		}
		switch {
		case e[0].isKind(tkString):
			v := unquoteString(e[0].tok.text)
			v = strings.ReplaceAll(v, `\`, `\\`)
			v = strings.ReplaceAll(v, `"`, `\"`)
			parts = append(parts, `"`+v+`"`)
		case e[0].isKind(tkNumber):
			parts = append(parts, e[0].tok.text)
		case e[0].is("NULL"):
			parts = append(parts, "NULL")
		default:
			return "", false
		}
	}
	return "{" + strings.Join(parts, ",") + "}", true
}

// arrays rewrites ARRAY[...] constructors into the text form the MySQL
// schema stores arrays in. Under ANY/ALL the constructor is left for anyAll,
// which turns it into an IN list instead.
func (r *rewriter) arrays(items []node, c ctx) ([]node, error) {
	for i := 0; i+1 < len(items); i++ {
		if !items[i].is("ARRAY") {
			continue
		}
		if items[i+1].isParen() {
			return nil, untranslatable("ARRAY(subquery)")
		}
		if !items[i+1].isBracket() {
			continue
		}
		if (c.parentFunc == "any" || c.parentFunc == "all") && len(items) == 2 {
			return items, nil
		}
		elems := splitTop(items[i+1].group.items)
		if len(items[i+1].group.items) == 0 {
			elems = nil
		}
		var repl []node
		if s, ok := pgArrayLiteral(elems); ok {
			repl = []node{{tok: token{tkString, quoteString(s)}}}
		} else {
			var quoted [][]node
			for _, e := range elems {
				quoted = append(quoted, call("JSON_QUOTE", call("CAST", append(clone(e), kw("AS"), kw("CHAR")))))
			}
			repl = call("CONCAT", []node{lit("{")}, call("CONCAT_WS", append([][]node{{lit(",")}}, quoted...)...), []node{lit("}")})
		}
		items = replace(items, i, i+1, repl)
		i += len(repl) - 1
	}
	return items, nil
}

// parsePGArrayLiteral splits '{a,"b c"}' into its elements.
func parsePGArrayLiteral(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, false
	}
	body := s[1 : len(s)-1]
	var out []string
	if strings.TrimSpace(body) == "" {
		return out, true
	}
	var b strings.Builder
	inQuote, quoted := false, false
	for i := 0; i < len(body); i++ {
		ch := body[i]
		switch {
		case inQuote && ch == '\\' && i+1 < len(body):
			i++
			b.WriteByte(body[i])
		case ch == '"':
			inQuote = !inQuote
			quoted = true
		case ch == ',' && !inQuote:
			v := b.String()
			if !quoted {
				v = strings.TrimSpace(v)
			}
			out = append(out, v)
			b.Reset()
			quoted = false
		default:
			b.WriteByte(ch)
		}
	}
	v := b.String()
	if !quoted {
		v = strings.TrimSpace(v)
	}
	return append(out, v), true
}

// anyAll rewrites `x = ANY(...)` and `x <> ALL(...)`.
func (r *rewriter) anyAll(items []node, _ ctx) ([]node, error) {
	for i := 1; i+1 < len(items); i++ {
		if !(items[i].is("ANY") || items[i].is("ALL") || items[i].is("SOME")) || !items[i+1].isParen() {
			continue
		}
		cmp := items[i-1]
		if !cmp.isKind(tkOp) {
			continue
		}
		isAny := !items[i].is("ALL")
		inner := items[i+1].group.items
		if len(inner) > 0 && (inner[0].is("SELECT") || inner[0].is("WITH")) {
			continue // a subquery: MySQL has = ANY (SELECT ...) natively
		}
		var positive bool
		switch {
		case isAny && cmp.isOp("="):
			positive = true
		case !isAny && (cmp.isOp("<>") || cmp.isOp("!=")):
			positive = false
		default:
			return nil, untranslatable("%s %s(array)", cmp.tok.text, strings.ToUpper(items[i].tok.text))
		}
		in := []node{kw("IN")}
		if !positive {
			in = []node{kw("NOT"), kw("IN")}
		}

		switch {
		case len(inner) == 1 && inner[0].param != nil:
			// A Go slice bound to $N: expanded into an IN list per execution.
			ref := *inner[0].param
			ref.kind, ref.negate = paramArray, !positive
			items = replace(items, i-1, i+1, append(in, paren(node{param: &ref})))
			i += len(in)

		case len(inner) == 1 && inner[0].isKind(tkString):
			elems, ok := parsePGArrayLiteral(unquoteString(inner[0].tok.text))
			if !ok {
				return nil, untranslatable("ANY(%s)", inner[0].tok.text)
			}
			items = replace(items, i-1, i+1, append(in, paren(inList(elems)...)))
			i += len(in)

		case len(inner) == 2 && inner[0].is("ARRAY") && inner[1].isBracket():
			var list []node
			if len(inner[1].group.items) == 0 {
				list = emptySet()
			} else {
				list = inner[1].group.items
			}
			items = replace(items, i-1, i+1, append(in, paren(list...)))
			i += len(in)

		default:
			// An array-valued column or expression: membership in its JSON
			// form.
			start := operandLeft(items, i-2)
			lhs := clone(items[start : i-1])
			member := append(lhs, kw("MEMBER"), kw("OF"), paren(arrayJSON(clone(inner))...))
			var repl []node
			if positive {
				repl = []node{paren(member...)}
			} else {
				repl = []node{kw("NOT"), paren(member...)}
			}
			items = replace(items, start, i+1, repl)
			i = start + len(repl) - 1
		}
	}
	return items, nil
}

// emptySet is an IN-list body that matches nothing, so `x IN (...)` is false
// and `x NOT IN (...)` true — what PostgreSQL answers for an empty array.
func emptySet() []node {
	return []node{kw("SELECT"), kw("NULL"), kw("FROM"), kw("DUAL"), kw("WHERE"), kw("FALSE")}
}

func inList(elems []string) []node {
	if len(elems) == 0 {
		return emptySet()
	}
	var out []node
	for i, e := range elems {
		if i > 0 {
			out = append(out, comma())
		}
		out = append(out, lit(e))
	}
	return out
}

// ---------------------------------------------------------------------------
// Casts
// ---------------------------------------------------------------------------

var multiWordTypes = map[string][]string{
	"character": {"varying"},
	"double":    {"precision"},
	"bit":       {"varying"},
}

func (r *rewriter) casts(items []node, _ ctx) ([]node, error) {
	for i := 0; i < len(items); i++ {
		if !items[i].isOp("::") {
			continue
		}
		if i == 0 || i+1 >= len(items) || !(items[i+1].isWordNode() || items[i+1].isKind(tkQuoted)) {
			return nil, fmt.Errorf("malformed cast")
		}
		end := i + 1
		typeName := strings.Trim(items[end].lower(), `"`)
		if items[end].isKind(tkQuoted) {
			typeName = strings.ToLower(strings.Trim(items[end].tok.text, `"`))
		}
		// schema-qualified type: public.block_reason
		if end+2 < len(items) && items[end+1].isPunct(".") && items[end+2].isIdentNode() {
			end += 2
			typeName = items[end].lower()
		}
		if next, ok := multiWordTypes[typeName]; ok && end+1 < len(items) && items[end+1].is(next[0]) {
			end++
			typeName += " " + next[0]
		}
		if (typeName == "timestamp" || typeName == "time") && end+3 < len(items) &&
			(items[end+1].is("WITH") || items[end+1].is("WITHOUT")) && items[end+2].is("TIME") && items[end+3].is("ZONE") {
			if items[end+1].is("WITH") {
				typeName += "tz"
			}
			end += 3
		}
		var typeArgs []node
		if end+1 < len(items) && items[end+1].isParen() {
			end++
			typeArgs = items[end].group.items
		}
		isArray := false
		for end+1 < len(items) && items[end+1].isBracket() && len(items[end+1].group.items) == 0 {
			end++
			isArray = true
		}

		start := primaryStart(items, i-1)
		operand := clone(items[start:i])
		repl, err := castTo(operand, typeName, typeArgs, isArray)
		if err != nil {
			return nil, err
		}
		items = replace(items, start, end, repl)
		i = start + len(repl) - 1
	}
	return items, nil
}

func castAs(operand []node, mysqlType ...node) []node {
	return call("CAST", append(append(clone(operand), kw("AS")), mysqlType...))
}

func isNullNode(operand []node) bool { return len(operand) == 1 && operand[0].is("NULL") }

func castTo(operand []node, typeName string, typeArgs []node, isArray bool) ([]node, error) {
	if isArray || isNullNode(operand) {
		return operand, nil
	}
	single := len(operand) == 1
	isStr := single && operand[0].isKind(tkString)

	switch typeName {
	case "int", "integer", "int4", "int2", "int8", "bigint", "smallint", "serial", "bigserial":
		return castAs(operand, kw("SIGNED")), nil

	case "numeric", "decimal":
		if len(typeArgs) > 0 {
			return castAs(operand, kw("DECIMAL"), paren(clone(typeArgs)...)), nil
		}
		return castAs(operand, kw("DECIMAL"), paren(num("65"), comma(), num("10"))), nil

	case "float", "float4", "float8", "real", "double precision":
		return castAs(operand, kw("DOUBLE")), nil

	case "text", "varchar", "character varying", "char", "character", "bpchar", "name", "citext":
		if isStr {
			return operand, nil
		}
		return castAs(operand, kw("CHAR")), nil

	case "json", "jsonb":
		return castAs(operand, kw("JSON")), nil

	case "boolean", "bool":
		if isStr {
			switch strings.ToLower(strings.TrimSpace(unquoteString(operand[0].tok.text))) {
			case "t", "true", "y", "yes", "on", "1":
				return []node{kw("TRUE")}, nil
			default:
				return []node{kw("FALSE")}, nil
			}
		}
		return operand, nil

	case "date":
		return call("DATE", operand), nil

	case "time", "timetz":
		return castAs(operand, kw("TIME"), paren(num("6"))), nil

	case "timestamp", "timestamptz":
		if isStr {
			return timestampLiteral(unquoteString(operand[0].tok.text))
		}
		return operand, nil

	case "interval":
		return intervalCast(operand)

	case "regclass", "regtype", "regproc", "oid", "xid", "xid8", "tid", "cid", "pg_lsn":
		return nil, untranslatable("cast to %s", typeName)
	}
	// uuid, inet, cidr, bytea, enum types, and anything else: the cast only
	// picked a PostgreSQL overload; MySQL takes the type from the context.
	return operand, nil
}

// timestampLiteral renders a timestamptz literal for MySQL.
func timestampLiteral(v string) ([]node, error) {
	s := strings.TrimSpace(v)
	switch strings.ToLower(s) {
	case "infinity", "+infinity":
		return castAs([]node{lit("9999-12-31 23:59:59.999999")}, kw("DATETIME"), paren(num("6"))), nil
	case "-infinity":
		return castAs([]node{lit("1000-01-01 00:00:00")}, kw("DATETIME"), paren(num("6"))), nil
	case "epoch":
		return castAs([]node{lit("1970-01-01 00:00:00")}, kw("DATETIME"), paren(num("6"))), nil
	case "now":
		return []node{kw("NOW"), paren(num("6"))}, nil
	}
	for _, suffix := range []string{"+00:00", "+00", "Z", " UTC", "+0000"} {
		if strings.HasSuffix(s, suffix) {
			s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
			break
		}
	}
	return castAs([]node{lit(s)}, kw("DATETIME"), paren(num("6"))), nil
}

// ---------------------------------------------------------------------------
// Intervals
// ---------------------------------------------------------------------------

var intervalUnits = map[string]string{
	"microsecond": "MICROSECOND", "microseconds": "MICROSECOND", "us": "MICROSECOND",
	"millisecond": "MILLISECOND", "milliseconds": "MILLISECOND", "ms": "MILLISECOND",
	"second": "SECOND", "seconds": "SECOND", "sec": "SECOND", "secs": "SECOND", "s": "SECOND",
	"minute": "MINUTE", "minutes": "MINUTE", "min": "MINUTE", "mins": "MINUTE", "m": "MINUTE",
	"hour": "HOUR", "hours": "HOUR", "hr": "HOUR", "hrs": "HOUR", "h": "HOUR",
	"day": "DAY", "days": "DAY", "d": "DAY",
	"week": "WEEK", "weeks": "WEEK", "w": "WEEK",
	"month": "MONTH", "months": "MONTH", "mon": "MONTH", "mons": "MONTH",
	"quarter": "QUARTER", "quarters": "QUARTER",
	"year": "YEAR", "years": "YEAR", "y": "YEAR", "yr": "YEAR", "yrs": "YEAR",
}

var unitSeconds = map[string]float64{
	"SECOND": 1, "MINUTE": 60, "HOUR": 3600, "DAY": 86400, "WEEK": 604800,
}

// intervalNodes renders a PostgreSQL interval string ('7 days', '1 hour 30
// minutes', '00:05:00') as MySQL's INTERVAL <n> <UNIT>.
func intervalNodes(text string) ([]node, error) {
	s := strings.ToLower(strings.TrimSpace(text))
	if strings.Count(s, ":") == 2 && !strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz") {
		var h, m int
		var sec float64
		if _, err := fmt.Sscanf(s, "%d:%d:%f", &h, &m, &sec); err == nil {
			return []node{kw("INTERVAL"), num(formatNumber(float64(h*3600+m*60) + sec)), kw("SECOND")}, nil
		}
	}
	// Split "1day" style into number and unit.
	var fields []string
	for _, f := range strings.Fields(s) {
		k := 0
		for k < len(f) && (f[k] >= '0' && f[k] <= '9' || f[k] == '.' || f[k] == '-' || f[k] == '+') {
			k++
		}
		if k > 0 && k < len(f) {
			fields = append(fields, f[:k], f[k:])
		} else {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 || len(fields)%2 != 0 {
		return nil, untranslatable("INTERVAL '%s'", text)
	}
	type part struct {
		n    float64
		unit string
	}
	var parts []part
	for k := 0; k < len(fields); k += 2 {
		n, err := strconv.ParseFloat(fields[k], 64)
		if err != nil {
			return nil, untranslatable("INTERVAL '%s'", text)
		}
		unit, ok := intervalUnits[strings.TrimSuffix(fields[k+1], ",")]
		if !ok {
			return nil, untranslatable("INTERVAL '%s'", text)
		}
		if unit == "MILLISECOND" {
			n, unit = n*1000, "MICROSECOND"
		}
		parts = append(parts, part{n, unit})
	}
	if len(parts) == 1 && parts[0].n == float64(int64(parts[0].n)) {
		return []node{kw("INTERVAL"), num(formatNumber(parts[0].n)), kw(parts[0].unit)}, nil
	}
	total := 0.0
	for _, p := range parts {
		secs, ok := unitSeconds[p.unit]
		switch {
		case ok:
			total += p.n * secs
		case p.unit == "MICROSECOND":
			total += p.n / 1e6
		default:
			return nil, untranslatable("INTERVAL '%s' (mixed calendar units)", text)
		}
	}
	return []node{kw("INTERVAL"), num(formatNumber(total)), kw("SECOND")}, nil
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// intervalCast handles expr::interval.
func intervalCast(operand []node) ([]node, error) {
	if len(operand) == 1 && operand[0].isKind(tkString) {
		return intervalNodes(unquoteString(operand[0].tok.text))
	}
	// $n::interval bound to a string such as '300 seconds': parsed at
	// execution time and inlined (number and unit are validated, never
	// copied through).
	if len(operand) == 1 && operand[0].param != nil && operand[0].param.kind == paramArg {
		ref := *operand[0].param
		ref.kind = paramInterval
		return []node{{param: &ref}}, nil
	}
	// ($1 || ' days')::interval — the quantity is bound, the unit literal.
	if len(operand) == 1 && operand[0].isParen() {
		g := operand[0].group.items
		if len(g) >= 3 && g[len(g)-2].isOp("||") && g[len(g)-1].isKind(tkString) {
			unitText := strings.ToLower(strings.TrimSpace(unquoteString(g[len(g)-1].tok.text)))
			if unit, ok := intervalUnits[unitText]; ok {
				qty := clone(g[:len(g)-2])
				if unit == "MILLISECOND" {
					qty, unit = append(qty, op("*"), num("1000")), "MICROSECOND"
				}
				return []node{kw("INTERVAL"), paren(qty...), kw(unit)}, nil
			}
		}
	}
	return nil, untranslatable("cast of a computed value to interval")
}

// intervals rewrites INTERVAL '...' literals and `INTERVAL '1 day' * n`.
func (r *rewriter) intervals(items []node, _ ctx) ([]node, error) {
	for i := 0; i+1 < len(items); i++ {
		if !items[i].is("INTERVAL") || !items[i+1].isKind(tkString) {
			continue
		}
		repl, err := intervalNodes(unquoteString(items[i+1].tok.text))
		if err != nil {
			return nil, err
		}
		end := i + 1
		// INTERVAL '1 day' * n  ->  INTERVAL (n) DAY  (a single-unit literal)
		if end+2 < len(items) && items[end+1].isOp("*") && repl[1].tok.text == "1" {
			k := operandRight(items, end+2)
			repl = []node{kw("INTERVAL"), paren(clone(items[end+2 : k+1])...), repl[2]}
			end = k
		}
		items = replace(items, i, end, repl)
		i += len(repl) - 1
	}
	// n * INTERVAL '1 day'
	for i := 1; i+2 < len(items); i++ {
		if items[i].isOp("*") && items[i+1].is("INTERVAL") && items[i+2].isKind(tkNumber) && items[i+2].tok.text == "1" && i+3 < len(items) && items[i+3].isWordNode() {
			start := primaryStart(items, i-1)
			repl := []node{kw("INTERVAL"), paren(clone(items[start:i])...), items[i+3]}
			items = replace(items, start, i+3, repl)
			i = start + len(repl) - 1
		}
	}
	return items, nil
}

// ---------------------------------------------------------------------------
// Operators
// ---------------------------------------------------------------------------

// jsonOps rewrites -> ->> #> #>> into JSON_EXTRACT.
func (r *rewriter) jsonOps(items []node, _ ctx) ([]node, error) {
	for i := 1; i+1 < len(items); i++ {
		o := items[i]
		if !(o.isOp("->") || o.isOp("->>") || o.isOp("#>") || o.isOp("#>>")) {
			continue
		}
		start := primaryStart(items, i-1)
		base := clone(items[start:i])
		key := items[i+1]
		var path []node
		switch {
		case (o.isOp("->") || o.isOp("->>")) && key.isKind(tkString):
			k := unquoteString(key.tok.text)
			path = []node{lit(`$."` + strings.ReplaceAll(k, `"`, `\"`) + `"`)}
		case (o.isOp("->") || o.isOp("->>")) && key.isKind(tkNumber):
			path = []node{lit("$[" + key.tok.text + "]")}
		case (o.isOp("->") || o.isOp("->>")) && key.param != nil:
			path = call("CONCAT", []node{lit(`$."`)}, []node{key}, []node{lit(`"`)})
		case key.isKind(tkString):
			elems, ok := parsePGArrayLiteral(unquoteString(key.tok.text))
			if !ok {
				return nil, untranslatable("json path %s", key.tok.text)
			}
			p := "$"
			for _, e := range elems {
				if _, err := strconv.Atoi(e); err == nil {
					p += "[" + e + "]"
				} else {
					p += `."` + strings.ReplaceAll(e, `"`, `\"`) + `"`
				}
			}
			path = []node{lit(p)}
		default:
			return nil, untranslatable("json operator %s with a computed key", o.tok.text)
		}
		extract := call("JSON_EXTRACT", base, path)
		var repl []node
		if o.isOp("->>") || o.isOp("#>>") {
			// JSON null comes back as SQL NULL, as with PostgreSQL's ->>.
			nullJSON := castAs([]node{lit("null")}, kw("JSON"))
			repl = call("JSON_UNQUOTE", call("NULLIF", extract, nullJSON))
		} else {
			repl = extract
		}
		items = replace(items, start, i+1, repl)
		i = start + len(repl) - 1
	}
	return items, nil
}

// filterClauses rewrites AGG(args) FILTER (WHERE cond) into a conditional
// aggregate.
func (r *rewriter) filterClauses(items []node, _ ctx) ([]node, error) {
	for i := 2; i+1 < len(items); i++ {
		if !items[i].is("FILTER") || !items[i+1].isParen() || !items[i-1].isParen() || !isFuncName(items[i-2]) {
			continue
		}
		cond := items[i+1].group.items
		if len(cond) == 0 || !cond[0].is("WHERE") {
			continue
		}
		cond = cond[1:]
		args := items[i-1].group.items
		var newArgs []node
		caseOf := func(v []node) []node {
			out := []node{kw("CASE"), kw("WHEN")}
			out = append(out, clone(cond)...)
			out = append(out, kw("THEN"))
			out = append(out, v...)
			return append(out, kw("END"))
		}
		switch {
		case len(args) == 1 && args[0].isOp("*"):
			newArgs = caseOf([]node{num("1")})
		default:
			distinct := len(args) > 0 && args[0].is("DISTINCT")
			rest := args
			if distinct {
				rest = args[1:]
			}
			// The first argument ends at a top-level comma or ORDER BY.
			end := len(rest)
			for k, n := range rest {
				if n.isPunct(",") || n.is("ORDER") {
					end = k
					break
				}
			}
			if distinct {
				newArgs = append(newArgs, kw("DISTINCT"))
			}
			newArgs = append(newArgs, caseOf(clone(rest[:end]))...)
			newArgs = append(newArgs, clone(rest[end:])...)
		}
		items = replace(items, i-1, i+1, []node{paren(newArgs...)})
	}
	return items, nil
}

// caseInsensitiveCollation is what ILIKE becomes: case-insensitive but, like
// PostgreSQL's ILIKE, accent-sensitive.
const caseInsensitiveCollation = "utf8mb4_0900_as_ci"

// likes rewrites ILIKE and gives every LIKE PostgreSQL's default escape.
func (r *rewriter) likes(items []node, _ ctx) ([]node, error) {
	for i := 0; i+1 < len(items); i++ {
		if !(items[i].is("LIKE") || items[i].is("ILIKE")) {
			continue
		}
		insensitive := items[i].is("ILIKE")
		if i+1 < len(items) && (items[i+1].is("ANY") || items[i+1].is("ALL")) {
			return nil, untranslatable("LIKE ANY/ALL")
		}
		end := operandRight(items, i+1)
		pattern := clone(items[i+1 : end+1])
		repl := []node{kw("LIKE")}
		if insensitive {
			repl = append(repl, paren(pattern...), kw("COLLATE"), kw(caseInsensitiveCollation))
		} else {
			repl = append(repl, pattern...)
		}
		if !(end+1 < len(items) && items[end+1].is("ESCAPE")) {
			repl = append(repl, kw("ESCAPE"), node{tok: token{tkString, `'\'`}})
		}
		items = replace(items, i, end, repl)
		i += len(repl) - 1
	}
	return items, nil
}

// regexOps rewrites ~ ~* !~ !~* into REGEXP_LIKE.
func (r *rewriter) regexOps(items []node, _ ctx) ([]node, error) {
	for i := 1; i+1 < len(items); i++ {
		o := items[i]
		if !(o.isOp("~") || o.isOp("~*") || o.isOp("!~") || o.isOp("!~*")) {
			continue
		}
		start := operandLeft(items, i-1)
		end := operandRight(items, i+1)
		mode := "c"
		if strings.HasSuffix(o.tok.text, "*") {
			mode = "i"
		}
		repl := call("REGEXP_LIKE", clone(items[start:i]), clone(items[i+1:end+1]), []node{lit(mode)})
		if strings.HasPrefix(o.tok.text, "!") {
			repl = append([]node{kw("NOT")}, repl...)
		}
		items = replace(items, start, end, repl)
		i = start + len(repl) - 1
	}
	return items, nil
}

// distinctFrom rewrites IS [NOT] DISTINCT FROM into the null-safe <=>.
func (r *rewriter) distinctFrom(items []node, _ ctx) ([]node, error) {
	for i := 1; i+2 < len(items); i++ {
		if !items[i].is("IS") {
			continue
		}
		k, not := i+1, false
		if items[k].is("NOT") {
			k, not = k+1, true
		}
		if k+1 >= len(items) || !items[k].is("DISTINCT") || !items[k+1].is("FROM") {
			continue
		}
		start := operandLeft(items, i-1)
		end := operandRight(items, k+2)
		eq := append(clone(items[start:i]), op("<=>"))
		eq = append(eq, clone(items[k+2:end+1])...)
		repl := []node{paren(eq...)}
		if !not {
			repl = []node{kw("NOT"), paren(eq...)}
		}
		items = replace(items, start, end, repl)
		i = start + len(repl) - 1
	}
	return items, nil
}

// atTimeZone drops AT TIME ZONE 'UTC': every MySQL session runs in UTC, so
// the conversion is the identity. Other zones would need MySQL's time zone
// tables and, depending on the operand's type, mean opposite things.
func (r *rewriter) atTimeZone(items []node, _ ctx) ([]node, error) {
	for i := 0; i+3 < len(items); i++ {
		if !(items[i].is("AT") && items[i+1].is("TIME") && items[i+2].is("ZONE")) {
			continue
		}
		z := items[i+3]
		if z.isKind(tkString) {
			switch strings.ToUpper(strings.TrimSpace(unquoteString(z.tok.text))) {
			case "UTC", "ETC/UTC", "GMT", "Z", "+00", "+00:00", "UCT", "ZULU":
				items = replace(items, i, i+3, nil)
				i--
				continue
			}
		}
		return nil, untranslatable("AT TIME ZONE other than UTC")
	}
	return items, nil
}

// overlaps rewrites the array overlap operator: a && b.
func (r *rewriter) overlaps(items []node, _ ctx) ([]node, error) {
	for i := 1; i+1 < len(items); i++ {
		if !items[i].isOp("&&") {
			continue
		}
		start := primaryStart(items, i-1)
		end := i + 1
		for end+1 < len(items) && items[end+1].isBracket() {
			end++
		}
		side := func(n []node) []node {
			if len(n) == 1 && n[0].param != nil && n[0].param.kind == paramArg {
				ref := *n[0].param
				ref.kind = paramJSONArray
				return castAs([]node{{param: &ref}}, kw("JSON"))
			}
			return arrayJSON(n)
		}
		repl := call("JSON_OVERLAPS", side(clone(items[start:i])), side(clone(items[i+1:end+1])))
		items = replace(items, start, end, repl)
		i = start + len(repl) - 1
	}
	return items, nil
}
