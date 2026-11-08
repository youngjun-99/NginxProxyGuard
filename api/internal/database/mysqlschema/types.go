package mysqlschema

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// maxKeyBytes is InnoDB's index key limit with the DYNAMIC row format.
const maxKeyBytes = 3072

// bytesPerChar is utf8mb4's worst case; InnoDB budgets keys by it.
const bytesPerChar = 4

// maxSizedVarchar bounds a TEXT column that has to become VARCHAR to sit in a
// unique key: one such column alone fills the key budget.
const maxSizedVarchar = maxKeyBytes / bytesPerChar

// textIndexPrefix is the prefix length a non-unique index takes of a TEXT
// column: long enough to be selective, short enough for composites.
const textIndexPrefix = 191

// mysqlType is the MySQL rendering of a PostgreSQL column type.
type mysqlType struct {
	sql   string
	bytes int  // key bytes; 0 for types that cannot be indexed whole
	text  bool // TEXT: needs a prefix in an index, or sizing in a unique key
	json  bool // not indexable at all
	array bool // lib/pq array literal held in TEXT
	// exprDefault: MySQL accepts a default on this type only in expression
	// form, DEFAULT ('...').
	exprDefault bool
	boolean     bool
	datetime    bool
	date        bool
}

var (
	reVarchar = regexp.MustCompile(`^(?:character varying|varchar)\((\d+)\)$`)
	reChar    = regexp.MustCompile(`^(?:character|char|bpchar)\((\d+)\)$`)
	reNumeric = regexp.MustCompile(`^(?:numeric|decimal)\((\d+)(?:,(\d+))?\)$`)
	reTS      = regexp.MustCompile(`^timestamp(\(\d\))?( with(out)? time zone)?$`)
	reTime    = regexp.MustCompile(`^time(\(\d\))?( with(out)? time zone)?$`)
)

// baseType maps a normalised PostgreSQL type. It is the contract the runtime
// translator relies on; see the package comment of internal/database/dialect.
func baseType(pg string, enums map[string]bool) (mysqlType, error) {
	if strings.HasSuffix(pg, "[]") {
		return mysqlType{sql: "TEXT", text: true, array: true, exprDefault: true}, nil
	}
	if enums[pg] {
		return mysqlType{sql: "VARCHAR(64)", bytes: 64 * bytesPerChar}, nil
	}
	if m := reVarchar.FindStringSubmatch(pg); m != nil {
		n, _ := strconv.Atoi(m[1])
		return varchar(n), nil
	}
	if m := reChar.FindStringSubmatch(pg); m != nil {
		n, _ := strconv.Atoi(m[1])
		return mysqlType{sql: fmt.Sprintf("CHAR(%d)", n), bytes: n * bytesPerChar}, nil
	}
	if m := reNumeric.FindStringSubmatch(pg); m != nil {
		scale := m[2]
		if scale == "" {
			scale = "0"
		}
		return mysqlType{sql: fmt.Sprintf("DECIMAL(%s,%s)", m[1], scale), bytes: 32}, nil
	}
	if reTS.MatchString(pg) || pg == "timestamptz" {
		return mysqlType{sql: "DATETIME(6)", bytes: 8, datetime: true}, nil
	}
	if reTime.MatchString(pg) || pg == "timetz" {
		return mysqlType{sql: "TIME(6)", bytes: 6}, nil
	}
	switch pg {
	case "uuid":
		return mysqlType{sql: "CHAR(36)", bytes: 36 * bytesPerChar}, nil
	case "text", "character varying", "varchar", "citext", "name":
		return mysqlType{sql: "TEXT", text: true, exprDefault: true}, nil
	case "integer", "int", "int4":
		return mysqlType{sql: "INT", bytes: 4}, nil
	case "bigint", "int8":
		return mysqlType{sql: "BIGINT", bytes: 8}, nil
	case "smallint", "int2":
		return mysqlType{sql: "SMALLINT", bytes: 2}, nil
	case "boolean", "bool":
		return mysqlType{sql: "BOOLEAN", bytes: 1, boolean: true}, nil
	case "date":
		return mysqlType{sql: "DATE", bytes: 3, date: true}, nil
	case "numeric", "decimal":
		return mysqlType{sql: "DECIMAL(65,10)", bytes: 32}, nil
	case "double precision", "float8", "float":
		return mysqlType{sql: "DOUBLE", bytes: 8}, nil
	case "real", "float4":
		return mysqlType{sql: "FLOAT", bytes: 4}, nil
	case "jsonb", "json":
		return mysqlType{sql: "JSON", json: true, exprDefault: true}, nil
	case "inet":
		return varchar(45), nil
	case "cidr":
		return varchar(49), nil
	case "macaddr":
		return varchar(17), nil
	case "bytea":
		return mysqlType{sql: "LONGBLOB", exprDefault: true}, nil
	case "interval":
		return varchar(64), nil
	}
	return mysqlType{}, fmt.Errorf("no MySQL mapping for PostgreSQL type %q", pg)
}

func varchar(n int) mysqlType {
	return mysqlType{sql: fmt.Sprintf("VARCHAR(%d)", n), bytes: n * bytesPerChar}
}

// ---------------------------------------------------------------------------
// Literals
// ---------------------------------------------------------------------------

// mysqlString renders a string literal that means the same under every
// sql_mode: quotes are doubled, and a backslash — an escape character unless
// NO_BACKSLASH_ESCAPES is set — is spelled CHAR(92) so the session mode does
// not matter. isExpr reports that the result is an expression, not a literal.
func mysqlString(s string) (sql string, isExpr bool) {
	if !strings.Contains(s, `\`) {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'", false
	}
	var parts []string
	for i, piece := range strings.Split(s, `\`) {
		if i > 0 {
			parts = append(parts, "CHAR(92 USING utf8mb4)")
		}
		if piece != "" {
			parts = append(parts, "'"+strings.ReplaceAll(piece, "'", "''")+"'")
		}
	}
	return "CONCAT(" + strings.Join(parts, ", ") + ")", true
}

// pqArrayLiteral re-renders a PostgreSQL array literal ({a,b} or {"a","b"})
// in lib/pq's own form: every element double-quoted, " and \ escaped. That is
// the form pq.StringArray writes, so stored defaults and app-written values
// look alike to the translator.
func pqArrayLiteral(lit string) (string, error) {
	elems, err := parsePGArray(lit)
	if err != nil {
		return "", err
	}
	return renderPQArray(elems), nil
}

func renderPQArray(elems []string) string {
	quoted := make([]string, len(elems))
	for i, e := range elems {
		e = strings.ReplaceAll(e, `\`, `\\`)
		e = strings.ReplaceAll(e, `"`, `\"`)
		quoted[i] = `"` + e + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}"
}

func parsePGArray(lit string) ([]string, error) {
	lit = strings.TrimSpace(lit)
	if len(lit) < 2 || lit[0] != '{' || lit[len(lit)-1] != '}' {
		return nil, fmt.Errorf("not an array literal: %q", lit)
	}
	body := lit[1 : len(lit)-1]
	if strings.TrimSpace(body) == "" {
		return nil, nil
	}
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '\\' && i+1 < len(body):
			i++
			cur.WriteByte(body[i])
		case c == '"':
			inQuote = !inQuote
		case c == ',' && !inQuote:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		case c == '{' && !inQuote:
			return nil, fmt.Errorf("nested array literal %q is not supported", lit)
		default:
			cur.WriteByte(c)
		}
	}
	out = append(out, strings.TrimSpace(cur.String()))
	return out, nil
}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// stripCasts removes PostgreSQL ::type casts. MySQL resolves the type from
// context, and every cast in the schema exists to steer PostgreSQL's operator
// resolution (text vs varchar, enum literals), which MySQL does not need.
func stripCasts(toks []token) []token {
	var out []token
	for i := 0; i < len(toks); i++ {
		if !toks[i].isPunct("::") {
			out = append(out, toks[i])
			continue
		}
		i++ // first word of the type
		for i+1 < len(toks) && toks[i+1].isPunct(".") {
			i += 2 // schema-qualified
		}
		// multi-word types
		for i+1 < len(toks) && toks[i+1].kind == tkWord && isTypeWord(toks[i+1].text) {
			i++
		}
		if i+1 < len(toks) && toks[i+1].isPunct("(") {
			i = matchParen(toks, i+1)
		}
		for i+2 < len(toks) && toks[i+1].isPunct("[") && toks[i+2].isPunct("]") {
			i += 2
		}
	}
	return out
}

func isTypeWord(w string) bool {
	switch strings.ToLower(w) {
	case "varying", "precision", "with", "without", "time", "zone":
		return true
	}
	return false
}

// unwrap strips parentheses that enclose the whole expression.
func unwrap(toks []token) []token {
	for len(toks) >= 2 && toks[0].isPunct("(") && matchParen(toks, 0) == len(toks)-1 {
		toks = toks[1 : len(toks)-1]
	}
	return toks
}

// sqlKeywords pass through an expression unchanged.
var sqlKeywords = map[string]bool{
	"AND": true, "OR": true, "NOT": true, "IS": true, "NULL": true, "TRUE": true, "FALSE": true,
	"IN": true, "CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
	"LIKE": true, "BETWEEN": true, "DISTINCT": true, "FROM": true,
}

// deterministicFuncs are the functions an index expression or CHECK may use
// on both engines, with the MySQL spelling.
var deterministicFuncs = map[string]string{
	"lower": "LOWER", "upper": "UPPER", "coalesce": "COALESCE", "nullif": "NULLIF",
	"length": "CHAR_LENGTH", "char_length": "CHAR_LENGTH", "abs": "ABS", "trim": "TRIM",
	"btrim": "TRIM", "left": "LEFT", "right": "RIGHT", "substr": "SUBSTRING", "substring": "SUBSTRING",
	"greatest": "GREATEST", "least": "LEAST",
}

// translateExpr renders a PostgreSQL boolean or scalar expression from a
// CHECK, an index part or a partial-index predicate as MySQL. Columns of t
// are backtick-quoted. Anything outside the small vocabulary those clauses
// use is an error, so an unfamiliar construct is reported instead of being
// rendered into something that means another thing.
func translateExpr(toks []token, t *Table) (string, error) {
	toks = stripCasts(toks)
	toks, err := rewriteAnyArray(toks)
	if err != nil {
		return "", err
	}
	toks, err = rewriteRegex(toks)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	prevCall := false
	for i, tok := range toks {
		var piece string
		call := false
		switch tok.kind {
		case tkString:
			piece, _ = mysqlString(tok.val)
		case tkNumber:
			piece = tok.text
		case tkQuoted:
			piece = quoteIdent(tok.val)
		case tkWord:
			up := strings.ToUpper(tok.text)
			isCall := i+1 < len(toks) && toks[i+1].isPunct("(") && !sqlKeywords[up]
			switch {
			case tok.text == regexpLikeMarker:
				piece, call = "REGEXP_LIKE", true
			case isCall:
				fn, ok := deterministicFuncs[strings.ToLower(tok.text)]
				if !ok {
					return "", fmt.Errorf("function %s() has no checked MySQL equivalent", tok.text)
				}
				piece, call = fn, true
			case t != nil && t.column(tok.ident()) != nil:
				piece = quoteIdent(tok.ident())
			case sqlKeywords[up]:
				piece = up
			default:
				return "", fmt.Errorf("unknown identifier %q", tok.text)
			}
		case tkPunct:
			switch tok.text {
			case "(", ")", ",", "=", "<", ">", "<=", ">=", "<>", "!=", "+", "-", "*", "/", "%":
				piece = tok.text
			case "||":
				return "", fmt.Errorf("string concatenation is not supported in schema expressions")
			default:
				return "", fmt.Errorf("operator %q has no MySQL equivalent", tok.text)
			}
		default:
			return "", fmt.Errorf("unsupported token %q", tok.text)
		}
		if b.Len() > 0 && exprSpace(toks[i-1], tok, prevCall) {
			b.WriteByte(' ')
		}
		b.WriteString(piece)
		prevCall = call
	}
	return b.String(), nil
}

// exprSpace decides whether a space separates two rendered tokens.
func exprSpace(prev, next token, prevCall bool) bool {
	switch {
	case prev.isPunct("("):
		return false
	case next.isPunct(")"), next.isPunct(","):
		return false
	case next.isPunct("("):
		return !prevCall
	}
	return true
}

// rewriteAnyArray turns `x = ANY (ARRAY[a, b])` into `x IN (a, b)` and
// `x <> ALL (ARRAY[...])` into `x NOT IN (...)`.
func rewriteAnyArray(toks []token) ([]token, error) {
	var out []token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if (t.is("ANY") || t.is("ALL")) && len(out) > 0 && i+1 < len(toks) && toks[i+1].isPunct("(") {
			op := out[len(out)-1]
			end := matchParen(toks, i+1)
			inner := unwrap(toks[i+2 : end])
			if len(inner) < 3 || !inner[0].is("ARRAY") || !inner[1].isPunct("[") || !inner[len(inner)-1].isPunct("]") {
				return nil, fmt.Errorf("%s over a non-literal array has no MySQL equivalent", t.text)
			}
			var in []token
			switch {
			case t.is("ANY") && op.isPunct("="):
				in = []token{{kind: tkWord, text: "IN"}}
			case t.is("ALL") && (op.isPunct("<>") || op.isPunct("!=")):
				in = []token{{kind: tkWord, text: "NOT"}, {kind: tkWord, text: "IN"}}
			default:
				return nil, fmt.Errorf("%s %s (...) has no MySQL equivalent", op.text, t.text)
			}
			out = append(out[:len(out)-1], in...)
			out = append(out, token{kind: tkPunct, text: "("})
			out = append(out, inner[2:len(inner)-1]...)
			out = append(out, token{kind: tkPunct, text: ")"})
			i = end
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

const regexpLikeMarker = "\x00REGEXP_LIKE"

// rewriteRegex turns `x ~ 'p'` into REGEXP_LIKE(x, 'p', 'c'); ~* matches
// case-insensitively and the ! forms negate.
func rewriteRegex(toks []token) ([]token, error) {
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tkPunct || (t.text != "~" && t.text != "~*" && t.text != "!~" && t.text != "!~*") {
			continue
		}
		ls := operandStart(toks, i-1)
		re := operandEnd(toks, i+1)
		if ls < 0 || re < 0 {
			return nil, fmt.Errorf("cannot find the operands of %s", t.text)
		}
		flag := "c"
		if strings.HasSuffix(t.text, "*") {
			flag = "i"
		}
		var call []token
		if strings.HasPrefix(t.text, "!") {
			call = append(call, token{kind: tkWord, text: "NOT"})
		}
		call = append(call, token{kind: tkWord, text: regexpLikeMarker}, token{kind: tkPunct, text: "("})
		call = append(call, toks[ls:i]...)
		call = append(call, token{kind: tkPunct, text: ","})
		call = append(call, toks[i+1:re+1]...)
		call = append(call, token{kind: tkPunct, text: ","}, token{kind: tkString, text: "'" + flag + "'", val: flag}, token{kind: tkPunct, text: ")"})
		rest := append([]token(nil), toks[re+1:]...)
		toks = append(append(toks[:ls:ls], call...), rest...)
		i = ls + len(call) - 1
	}
	return toks, nil
}

// operandStart finds the first token of the operand ending at end.
func operandStart(toks []token, end int) int {
	if end < 0 {
		return -1
	}
	if toks[end].isPunct(")") {
		depth := 0
		for i := end; i >= 0; i-- {
			switch {
			case toks[i].isPunct(")"):
				depth++
			case toks[i].isPunct("("):
				depth--
				if depth == 0 {
					if i > 0 && toks[i-1].kind == tkWord {
						return i - 1
					}
					return i
				}
			}
		}
		return -1
	}
	i := end
	for i >= 2 && toks[i-1].isPunct(".") {
		i -= 2
	}
	return i
}

// operandEnd finds the last token of the operand starting at start.
func operandEnd(toks []token, start int) int {
	if start >= len(toks) {
		return -1
	}
	if toks[start].isPunct("(") {
		return matchParen(toks, start)
	}
	if toks[start].kind == tkWord && start+1 < len(toks) && toks[start+1].isPunct("(") {
		return matchParen(toks, start+1)
	}
	return start
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// exprColumns lists the columns of t an expression refers to, in order.
func exprColumns(toks []token, t *Table) []string {
	var cols []string
	seen := map[string]bool{}
	for i, tok := range toks {
		if !tok.isIdent() || (i+1 < len(toks) && toks[i+1].isPunct("(")) {
			continue
		}
		if i > 0 && toks[i-1].isPunct("::") {
			continue
		}
		if t.column(tok.ident()) != nil && !seen[tok.ident()] {
			seen[tok.ident()] = true
			cols = append(cols, tok.ident())
		}
	}
	return cols
}
