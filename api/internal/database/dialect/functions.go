package dialect

import (
	"strconv"
	"strings"
)

// dateTruncFormats maps date_trunc units onto DATE_FORMAT patterns.
var dateTruncFormats = map[string]string{
	"second": "%Y-%m-%d %H:%i:%s",
	"minute": "%Y-%m-%d %H:%i:00",
	"hour":   "%Y-%m-%d %H:00:00",
	"day":    "%Y-%m-%d 00:00:00",
	"month":  "%Y-%m-01 00:00:00",
	"year":   "%Y-01-01 00:00:00",
}

func datetime6(expr []node) []node {
	return castAs(expr, kw("DATETIME"), paren(num("6")))
}

// dateTrunc renders date_trunc(unit, x) for MySQL. The result is a DATETIME
// so it scans into time.Time and groups like a timestamp.
func dateTrunc(unit string, x []node) ([]node, error) {
	if f, ok := dateTruncFormats[unit]; ok {
		return datetime6(call("DATE_FORMAT", x, []node{lit(f)})), nil
	}
	switch unit {
	case "week": // ISO weeks start on Monday, as PostgreSQL's do
		return datetime6(call("DATE_SUB", call("DATE", x), []node{kw("INTERVAL"), call("WEEKDAY", x)[0], call("WEEKDAY", x)[1], kw("DAY")})), nil
	case "quarter":
		mk := call("MAKEDATE", call("YEAR", x), []node{num("1")})
		q := append(call("QUARTER", x), op("-"), num("1"))
		return datetime6(append(mk, op("+"), kw("INTERVAL"), paren(q...), kw("QUARTER"))), nil
	}
	return nil, untranslatable("date_trunc('%s', ...)", unit)
}

// extractField renders EXTRACT(field FROM x) / date_part('field', x).
func extractField(field string, x []node) ([]node, error) {
	switch field {
	case "epoch":
		// EXTRACT(EPOCH FROM a - b) is an interval's length in PostgreSQL.
		// MySQL subtracts DATETIMEs as numbers (20240102 - 20240101), so the
		// difference must be taken between the two epochs instead.
		inner := x
		if len(inner) == 1 && inner[0].isParen() {
			inner = inner[0].group.items
		}
		for k := len(inner) - 1; k > 0; k-- {
			if inner[k].isOp("-") && !(k+1 < len(inner) && inner[k+1].is("INTERVAL")) {
				a, b := clone(inner[:k]), clone(inner[k+1:])
				return append(append(call("UNIX_TIMESTAMP", a), op("-")), call("UNIX_TIMESTAMP", b)...), nil
			}
			if inner[k].isOp("+") || inner[k].is("INTERVAL") {
				break
			}
		}
		return call("UNIX_TIMESTAMP", x), nil
	case "dow":
		return []node{paren(append(call("DAYOFWEEK", x), op("-"), num("1"))...)}, nil
	case "isodow":
		return []node{paren(append(call("WEEKDAY", x), op("+"), num("1"))...)}, nil
	case "doy":
		return call("DAYOFYEAR", x), nil
	case "week":
		return call("WEEK", x, []node{num("3")}), nil
	case "year", "month", "day", "hour", "minute", "quarter", "microsecond":
		return call("EXTRACT", append([]node{kw(strings.ToUpper(field)), kw("FROM")}, x...)), nil
	case "second":
		// PostgreSQL includes the fraction.
		s := call("EXTRACT", append([]node{kw("SECOND"), kw("FROM")}, x...))
		us := call("EXTRACT", append([]node{kw("MICROSECOND"), kw("FROM")}, clone(x)...))
		return []node{paren(append(append(s, op("+")), append(us, op("/"), num("1000000"))...)...)}, nil
	}
	return nil, untranslatable("EXTRACT(%s ...)", field)
}

// greatestIgnoringNulls emulates PostgreSQL's GREATEST/LEAST, which skip NULL
// arguments; MySQL's return NULL as soon as one argument is NULL.
func greatestIgnoringNulls(name string, args [][]node) []node {
	if len(args) < 2 {
		return call(name, args...)
	}
	var wrapped [][]node
	for i := range args {
		parts := [][]node{clone(args[i])}
		for j := range args {
			if j != i {
				parts = append(parts, clone(args[j]))
			}
		}
		wrapped = append(wrapped, call("COALESCE", parts...))
	}
	return call(name, wrapped...)
}

// regexReplacement converts PostgreSQL's \1 back-references to ICU's $1.
func regexReplacement(lit string) string {
	v := unquoteString(lit)
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		switch {
		case v[i] == '\\' && i+1 < len(v) && v[i+1] >= '0' && v[i+1] <= '9':
			b.WriteByte('$')
			b.WriteByte(v[i+1])
			i++
		case v[i] == '\\' && i+1 < len(v) && v[i+1] == '&':
			b.WriteString("$0")
			i++
		case v[i] == '$':
			b.WriteString(`\$`)
		default:
			b.WriteByte(v[i])
		}
	}
	return b.String()
}

// functions rewrites function calls whose MySQL spelling or semantics differ.
func (r *rewriter) functions(items []node, _ ctx) ([]node, error) {
	for i := 0; i < len(items); i++ {
		n := items[i]
		if !n.isWordNode() {
			continue
		}
		name := n.lower()

		// Niladic keywords.
		if i+1 >= len(items) || !items[i+1].isParen() {
			switch name {
			case "current_timestamp", "localtimestamp":
				items = replace(items, i, i, []node{kw("CURRENT_TIMESTAMP"), paren(num("6"))})
				i++
			}
			continue
		}
		if i > 0 && items[i-1].isPunct(".") {
			continue // a qualified name, not a builtin
		}
		args := items[i+1].group.items
		argv := splitTop(args)
		if len(args) == 0 {
			argv = nil
		}
		var repl []node
		var err error

		switch name {
		case "now", "statement_timestamp", "transaction_timestamp":
			if len(argv) == 0 {
				repl = []node{kw("NOW"), paren(num("6"))}
			}
		case "clock_timestamp":
			repl = []node{kw("SYSDATE"), paren(num("6"))}
		case "current_timestamp":
			// CURRENT_TIMESTAMP(n) is fine as written.
		case "gen_random_uuid", "uuid_generate_v4":
			repl = []node{kw("UUID"), paren()}
		case "random":
			repl = []node{kw("RAND"), paren()}
		case "current_database":
			repl = []node{kw("DATABASE"), paren()}
		case "pg_sleep":
			repl = call("SLEEP", argv...)

		case "date_trunc":
			if len(argv) < 2 || len(argv[0]) != 1 || !argv[0][0].isKind(tkString) {
				return nil, untranslatable("date_trunc with a computed unit")
			}
			if len(argv) == 3 {
				z := argv[2]
				if !(len(z) == 1 && z[0].isKind(tkString) && strings.EqualFold(unquoteString(z[0].tok.text), "UTC")) {
					return nil, untranslatable("date_trunc in a time zone other than UTC")
				}
			}
			repl, err = dateTrunc(strings.ToLower(unquoteString(argv[0][0].tok.text)), argv[1])

		case "date_part":
			if len(argv) != 2 || len(argv[0]) != 1 || !argv[0][0].isKind(tkString) {
				return nil, untranslatable("date_part with a computed field")
			}
			repl, err = extractField(strings.ToLower(unquoteString(argv[0][0].tok.text)), argv[1])

		case "extract":
			if len(args) >= 3 && args[0].isWordNode() && args[1].is("FROM") {
				repl, err = extractField(args[0].lower(), clone(args[2:]))
			}

		case "to_timestamp":
			if len(argv) == 1 {
				repl = call("FROM_UNIXTIME", argv[0])
			} else {
				err = untranslatable("to_timestamp(text, format)")
			}

		case "make_interval":
			// make_interval(secs => x) appears in date arithmetic only.
			if len(argv) != 1 || len(argv[0]) < 3 || !argv[0][0].isWordNode() || !argv[0][1].isOp("=>") {
				return nil, untranslatable("make_interval with positional arguments")
			}
			unitWord := map[string]string{"secs": "SECOND", "mins": "MINUTE", "hours": "HOUR", "days": "DAY", "weeks": "WEEK", "months": "MONTH", "years": "YEAR"}[argv[0][0].lower()]
			if unitWord == "" {
				return nil, untranslatable("make_interval(%s => ...)", argv[0][0].tok.text)
			}
			repl = []node{kw("INTERVAL"), paren(clone(argv[0][2:])...), kw(unitWord)}

		case "string_agg":
			if len(argv) < 2 {
				return nil, untranslatable("string_agg arity")
			}
			sepAndOrder := argv[len(argv)-1]
			if len(sepAndOrder) == 0 || !sepAndOrder[0].isKind(tkString) {
				return nil, untranslatable("string_agg with a computed separator")
			}
			value := joinTop(argv[:len(argv)-1])
			inner := clone(value)
			inner = append(inner, clone(sepAndOrder[1:])...) // ORDER BY ...
			inner = append(inner, kw("SEPARATOR"), sepAndOrder[0])
			repl = []node{kw("GROUP_CONCAT"), paren(inner...)}

		case "array_agg":
			if len(argv) != 1 {
				return nil, untranslatable("array_agg arity")
			}
			a := argv[0]
			var distinct []node
			if len(a) > 0 && a[0].is("DISTINCT") {
				distinct, a = []node{kw("DISTINCT")}, a[1:]
			}
			orderAt := len(a)
			for k, x := range a {
				if x.is("ORDER") {
					orderAt = k
					break
				}
			}
			elem := call("JSON_QUOTE", castAs(a[:orderAt], kw("CHAR")))
			inner := append(append(distinct, elem...), clone(a[orderAt:])...)
			inner = append(inner, kw("SEPARATOR"), lit(","))
			repl = call("CONCAT", []node{lit("{")}, []node{kw("GROUP_CONCAT"), paren(inner...)}, []node{lit("}")})

		case "json_agg", "jsonb_agg":
			repl = call("JSON_ARRAYAGG", argv...)
		case "json_build_object", "jsonb_build_object":
			repl = call("JSON_OBJECT", argv...)
		case "json_build_array", "jsonb_build_array":
			repl = call("JSON_ARRAY", argv...)
		case "jsonb_array_length", "json_array_length":
			repl = call("JSON_LENGTH", argv...)
		case "jsonb_typeof", "json_typeof":
			repl = call("LOWER", call("JSON_TYPE", argv...))

		case "array_length":
			if len(argv) != 2 {
				return nil, untranslatable("array_length arity")
			}
			// An empty array has no length in PostgreSQL: NULL, not 0.
			repl = call("NULLIF", call("JSON_LENGTH", arrayJSON(argv[0])), []node{num("0")})
		case "cardinality":
			repl = call("JSON_LENGTH", arrayJSON(argv[0]))

		case "length", "char_length", "character_length":
			repl = call("CHAR_LENGTH", argv...)
		case "octet_length":
			repl = call("LENGTH", argv...)
		case "strpos":
			if len(argv) == 2 {
				repl = call("LOCATE", argv[1], argv[0])
			}
		case "btrim":
			if len(argv) == 1 {
				repl = call("TRIM", argv[0])
			}
		case "lpad", "rpad":
			if len(argv) == 2 {
				repl = call(strings.ToUpper(name), argv[0], argv[1], []node{lit(" ")})
			}
		case "log":
			if len(argv) == 1 {
				repl = call("LOG10", argv[0])
			}
		case "trunc":
			if len(argv) == 1 {
				repl = call("TRUNCATE", argv[0], []node{num("0")})
			} else {
				repl = call("TRUNCATE", argv...)
			}
		case "split_part":
			repl, err = splitPart(argv)
		case "host":
			// inet values are stored as plain address text; there is no mask
			// to drop.
			if len(argv) == 1 {
				repl = []node{paren(argv[0]...)}
			}
		case "family":
			if len(argv) == 1 {
				repl = call("IF", append(call("LOCATE", []node{lit(":")}, argv[0]), op(">"), num("0")), []node{num("6")}, []node{num("4")})
			}
		case "regexp_replace":
			repl, err = regexpReplace(argv)
		case "greatest", "least":
			repl = greatestIgnoringNulls(strings.ToUpper(name), argv)
		case "bool_or":
			repl = call("MAX", argv...)
		case "bool_and", "every":
			repl = call("MIN", argv...)
		case "substring", "substr":
			if len(args) >= 3 && args[1].is("FROM") && args[2].isKind(tkString) && (len(args) == 3 || !args[3].is("FOR")) {
				err = untranslatable("substring(text FROM pattern)")
			}
		}
		if err != nil {
			return nil, err
		}
		if repl != nil {
			items = replace(items, i, i+1, repl)
			i += len(repl) - 1
		}
	}
	return items, nil
}

func splitPart(argv [][]node) ([]node, error) {
	if len(argv) != 3 || len(argv[2]) != 1 || !argv[2][0].isKind(tkNumber) {
		return nil, untranslatable("split_part with a computed field number")
	}
	k, err := strconv.Atoi(argv[2][0].tok.text)
	if err != nil || k < 1 {
		return nil, untranslatable("split_part field %s", argv[2][0].tok.text)
	}
	s, d := argv[0], argv[1]
	if k == 1 {
		return call("SUBSTRING_INDEX", s, d, []node{num("1")}), nil
	}
	// PostgreSQL answers '' past the last field; SUBSTRING_INDEX would repeat
	// the last one.
	fields := append(call("CHAR_LENGTH", s), op("-"))
	fields = append(fields, call("CHAR_LENGTH", call("REPLACE", s, d, []node{lit("")}))...)
	count := append([]node{paren(fields...), op("/")}, call("CHAR_LENGTH", d)...)
	cond := append(count, op(">="), num(strconv.Itoa(k-1)))
	val := call("SUBSTRING_INDEX", call("SUBSTRING_INDEX", s, d, []node{num(strconv.Itoa(k))}), d, []node{num("-1")})
	out := []node{kw("CASE"), kw("WHEN")}
	out = append(out, cond...)
	out = append(out, kw("THEN"))
	out = append(out, val...)
	out = append(out, kw("ELSE"), lit(""), kw("END"))
	return []node{paren(out...)}, nil
}

func regexpReplace(argv [][]node) ([]node, error) {
	if len(argv) < 3 || len(argv) > 4 {
		return nil, untranslatable("regexp_replace arity")
	}
	occurrence, mode := "1", "c"
	if len(argv) == 4 {
		if len(argv[3]) != 1 || !argv[3][0].isKind(tkString) {
			return nil, untranslatable("regexp_replace with computed flags")
		}
		flags := unquoteString(argv[3][0].tok.text)
		if strings.Contains(flags, "g") {
			occurrence = "0"
		}
		if strings.Contains(flags, "i") {
			mode = "i"
		}
	}
	repl := argv[2]
	if len(repl) == 1 && repl[0].isKind(tkString) {
		repl = []node{lit(regexReplacement(repl[0].tok.text))}
	}
	return call("REGEXP_REPLACE", argv[0], argv[1], repl, []node{num("1")}, []node{num(occurrence)}, []node{lit(mode)}), nil
}

// ---------------------------------------------------------------------------
// Clauses
// ---------------------------------------------------------------------------

// clauses handles the clause-level differences inside one list: NULLS
// FIRST/LAST, MATERIALIZED CTEs, lock strengths, OFFSET without LIMIT,
// LIMIT ALL, FETCH FIRST.
func (r *rewriter) clauses(items []node, _ ctx) ([]node, error) {
	// WITH x AS [NOT] MATERIALIZED (...)
	for i := 0; i+1 < len(items); i++ {
		if items[i].is("AS") && items[i+1].is("MATERIALIZED") {
			items = replace(items, i+1, i+1, nil)
		} else if items[i].is("AS") && i+2 < len(items) && items[i+1].is("NOT") && items[i+2].is("MATERIALIZED") {
			items = replace(items, i+1, i+2, nil)
		}
	}

	// FOR NO KEY UPDATE / FOR KEY SHARE
	for i := 0; i+2 < len(items); i++ {
		if items[i].is("FOR") && items[i+1].is("NO") && items[i+2].is("KEY") && i+3 < len(items) && items[i+3].is("UPDATE") {
			items = replace(items, i+1, i+2, nil)
		} else if items[i].is("FOR") && items[i+1].is("KEY") && items[i+2].is("SHARE") {
			items = replace(items, i+1, i+1, nil)
		}
	}

	items = nullsOrdering(items)

	var err error
	if items, err = rollups(items); err != nil {
		return nil, err
	}

	// FETCH FIRST n ROWS ONLY -> LIMIT n
	for i := 0; i+4 < len(items); i++ {
		if items[i].is("FETCH") && (items[i+1].is("FIRST") || items[i+1].is("NEXT")) &&
			(items[i+3].is("ROWS") || items[i+3].is("ROW")) && items[i+4].is("ONLY") {
			items = replace(items, i, i+4, []node{kw("LIMIT"), items[i+2]})
		}
	}

	// LIMIT ALL
	for i := 0; i+1 < len(items); i++ {
		if items[i].is("LIMIT") && items[i+1].is("ALL") {
			items = replace(items, i, i+1, nil)
			i--
		}
	}

	// OFFSET without LIMIT: MySQL needs a LIMIT in front of an OFFSET.
	hasLimit := indexWord(items, "LIMIT", 0) >= 0
	if off := indexWord(items, "OFFSET", 0); off >= 0 && !hasLimit {
		items = replace(items, off, off-1, []node{kw("LIMIT"), num("18446744073709551615")})
	}
	return items, nil
}

// nullsOrdering rewrites ORDER BY items carrying NULLS FIRST/LAST. Where the
// requested order is MySQL's default (NULLs sort first ascending, last
// descending) the clause is just dropped, keeping any index-backed ordering.
func nullsOrdering(items []node) []node {
	for i := 0; i+1 < len(items); i++ {
		if !(items[i].is("ORDER") && items[i+1].is("BY")) {
			continue
		}
		start := i + 2
		end := len(items)
		for k := start; k < len(items); k++ {
			if items[k].is("LIMIT") || items[k].is("OFFSET") || items[k].is("FETCH") || items[k].is("FOR") ||
				items[k].is("UNION") || items[k].is("EXCEPT") || items[k].is("INTERSECT") || items[k].is("SEPARATOR") ||
				items[k].is("ROWS") || items[k].is("RANGE") || items[k].isPunct(";") {
				end = k
				break
			}
		}
		parts := splitTop(items[start:end])
		changed := false
		for p, part := range parts {
			if len(part) < 3 || !part[len(part)-2].is("NULLS") {
				continue
			}
			first := part[len(part)-1].is("FIRST")
			body := part[:len(part)-2]
			desc := len(body) > 0 && body[len(body)-1].is("DESC")
			expr := body
			if len(body) > 0 && (body[len(body)-1].is("DESC") || body[len(body)-1].is("ASC")) {
				expr = body[:len(body)-1]
			}
			changed = true
			if first != desc { // ASC NULLS FIRST or DESC NULLS LAST: MySQL's default
				parts[p] = clone(body)
				continue
			}
			key := []node{paren(clone(expr)...), kw("IS"), kw("NULL")}
			if first {
				key = append(key, kw("DESC"))
			}
			parts[p] = append(append(key, comma()), clone(body)...)
		}
		if changed {
			repl := joinTop(parts)
			items = replace(items, start, end-1, repl)
			i = start + len(repl) - 1
		}
	}
	return items
}

// reservedInMySQL are words MySQL reserves that PostgreSQL code uses as
// identifiers. `key` is the one the schema has (settings.key, and the key
// column jsonb_each returns); the others are cheap insurance.
var reservedInMySQL = map[string]bool{
	"key": true, "keys": true, "rank": true, "groups": true, "usage": true, "condition": true,
	"system": true, "change": true, "release": true, "virtual": true, "stored": true,
	"generated": true, "function": true, "read": true, "write": true, "signal": true,
	"lead": true, "lag": true, "cume_dist": true, "dense_rank": true, "percent_rank": true,
	"row_number": true, "ntile": true, "first_value": true, "last_value": true, "nth_value": true,
	"maxvalue": true, "match": true, "describe": true,
}

func (r *rewriter) quoteReserved(items []node, _ ctx) ([]node, error) {
	for i, n := range items {
		if !n.isWordNode() || !reservedInMySQL[n.lower()] {
			continue
		}
		if i+1 < len(items) && items[i+1].isParen() {
			continue // a function call: rank(), mod(...)
		}
		if n.lower() == "key" && i > 0 && (items[i-1].is("DUPLICATE") || items[i-1].is("PRIMARY") || items[i-1].is("FOREIGN")) {
			continue
		}
		items[i] = node{tok: token{tkQuoted, `"` + n.tok.text + `"`}}
	}
	return items, nil
}

// rollups rewrites GROUP BY ... ROLLUP(...) into MySQL's WITH ROLLUP.
//
// PostgreSQL's `GROUP BY a, ROLLUP(b)` produces the grouping sets (a, b) and
// (a): a subtotal per a. MySQL's `GROUP BY a, b WITH ROLLUP` produces those
// plus the grand total (), so a HAVING GROUPING(a) = 0 drops exactly that
// extra row. CUBE and GROUPING SETS have no MySQL form.
func rollups(items []node) ([]node, error) {
	for i := 0; i+1 < len(items); i++ {
		if !(items[i].is("GROUP") && items[i+1].is("BY")) {
			continue
		}
		start := i + 2
		end := len(items)
		for k := start; k < len(items); k++ {
			n := items[k]
			if n.is("HAVING") || n.is("ORDER") || n.is("LIMIT") || n.is("OFFSET") || n.is("WINDOW") ||
				n.is("UNION") || n.is("EXCEPT") || n.is("INTERSECT") || n.is("FOR") || n.isPunct(";") {
				end = k
				break
			}
		}
		parts := splitTop(items[start:end])
		var plain [][]node
		var rolled []node
		for p, part := range parts {
			if len(part) == 2 && (part[0].is("CUBE")) && part[1].isParen() {
				return nil, untranslatable("GROUP BY CUBE")
			}
			if len(part) >= 2 && part[0].is("GROUPING") && part[1].is("SETS") {
				return nil, untranslatable("GROUP BY GROUPING SETS")
			}
			if len(part) == 2 && part[0].is("ROLLUP") && part[1].isParen() {
				if p != len(parts)-1 {
					return nil, untranslatable("ROLLUP(...) followed by more GROUP BY items")
				}
				rolled = part[1].group.items
				continue
			}
			plain = append(plain, part)
		}
		if rolled == nil {
			continue
		}
		// MySQL resolves HAVING GROUPING(x) only for a plain column or a
		// select-list alias, so an expression item is grouped by its alias.
		for k, p := range plain {
			if len(p) == 1 && p[0].isIdentNode() || len(p) == 3 && p[0].isIdentNode() && p[1].isPunct(".") && p[2].isIdentNode() {
				continue
			}
			alias := selectAliasFor(items[:i], p)
			if alias == "" {
				return nil, untranslatable("GROUP BY <expression>, ROLLUP(...) without a select-list alias for the expression")
			}
			plain[k] = []node{quotedIdent(alias)}
		}
		all := append(append([][]node(nil), plain...), splitTop(rolled)...)
		repl := append(joinTop(all), kw("WITH"), kw("ROLLUP"))
		if len(plain) > 0 {
			var conds []node
			for k, p := range plain {
				if k > 0 {
					conds = append(conds, kw("AND"))
				}
				conds = append(conds, call("GROUPING", clone(p))...)
				conds = append(conds, op("="), num("0"))
			}
			if end < len(items) && items[end].is("HAVING") {
				// Fold into the existing HAVING: (theirs) AND ours.
				hEnd := len(items)
				for k := end + 1; k < len(items); k++ {
					if items[k].is("ORDER") || items[k].is("LIMIT") || items[k].is("OFFSET") || items[k].is("WINDOW") ||
						items[k].is("UNION") || items[k].is("EXCEPT") || items[k].is("INTERSECT") || items[k].is("FOR") {
						hEnd = k
						break
					}
				}
				having := append([]node{kw("HAVING"), paren(clone(items[end+1 : hEnd])...), kw("AND")}, conds...)
				repl = append(repl, having...)
				items = replace(items, start, hEnd-1, repl)
			} else {
				repl = append(repl, kw("HAVING"))
				repl = append(repl, conds...)
				items = replace(items, start, end-1, repl)
			}
		} else {
			items = replace(items, start, end-1, repl)
		}
		i = start + len(repl) - 1
	}
	return items, nil
}

// selectAliasFor finds `expr AS alias` in the select list that precedes a
// GROUP BY and returns the alias of the item equal to expr.
func selectAliasFor(before []node, expr []node) string {
	sel := -1
	for k, n := range before {
		if n.is("SELECT") {
			sel = k
		}
	}
	if sel < 0 {
		return ""
	}
	from := topIndex(before, sel+1, "FROM")
	if from < 0 {
		from = len(before)
	}
	want, err := emit(expr)
	if err != nil {
		return ""
	}
	for _, part := range splitTop(before[sel+1 : from]) {
		if len(part) < 3 || !part[len(part)-2].is("AS") || !part[len(part)-1].isIdentNode() {
			continue
		}
		got, err := emit(part[:len(part)-2])
		if err == nil && got.text() == want.text() {
			return identName(part[len(part)-1])
		}
	}
	return ""
}
