package mysqlschema

import "strings"

// This file reads PostgreSQL source text: it splits a script into statements
// and a statement into tokens. It understands exactly as much PostgreSQL
// lexical structure as the schema files use — standard and E'' strings,
// "quoted" identifiers, $tag$ bodies, both comment styles — and nothing of
// PostgreSQL's grammar, which parse.go handles statement by statement.

type tokKind int

const (
	tkWord   tokKind = iota // keyword or bare identifier
	tkQuoted                // "quoted identifier"; val holds the name
	tkString                // '...' or E'...'; val holds the decoded value
	tkNumber
	tkParam  // $1
	tkDollar // $tag$ ... $tag$; val holds the body
	tkPunct
)

type token struct {
	kind tokKind
	text string // source spelling
	val  string // decoded value for strings, quoted identifiers and $ bodies
}

func (t token) is(word string) bool {
	return t.kind == tkWord && strings.EqualFold(t.text, word)
}

func (t token) isPunct(p string) bool { return t.kind == tkPunct && t.text == p }

// ident returns the identifier a token names: the bare word folded to lower
// case (PostgreSQL folds unquoted names) or a quoted name verbatim.
func (t token) ident() string {
	if t.kind == tkQuoted {
		return t.val
	}
	return strings.ToLower(t.text)
}

func (t token) isIdent() bool { return t.kind == tkWord || t.kind == tkQuoted }

// multiCharOps are matched longest first.
var multiCharOps = []string{
	"!~*", "->>", "#>>",
	"::", "<=", ">=", "<>", "!=", "||", "->", "#>", "~*", "!~", "@>", "<@", "=>", ":=", "&&",
}

// scan walks src calling emit for every token, and for comments and
// whitespace when keepTrivia is set (kind -1). It is the single definition of
// where strings and comments start and end, shared by the splitter and the
// tokenizer.
func scan(src string, emit func(kind tokKind, start, end int, val string)) {
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case c == '-' && i+1 < n && src[i+1] == '-':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = n - i
			}
			emit(-1, i, i+j, "")
			i += j
		case c == '/' && i+1 < n && src[i+1] == '*':
			depth, j := 1, i+2
			for j < n && depth > 0 {
				if strings.HasPrefix(src[j:], "/*") {
					depth++
					j += 2
				} else if strings.HasPrefix(src[j:], "*/") {
					depth--
					j += 2
				} else {
					j++
				}
			}
			emit(-1, i, j, "")
			i = j
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			j := i
			for j < n && strings.IndexByte(" \t\n\r\f", src[j]) >= 0 {
				j++
			}
			emit(-1, i, j, "")
			i = j
		case (c == 'E' || c == 'e') && i+1 < n && src[i+1] == '\'':
			end, val := scanEString(src, i+1)
			emit(tkString, i, end, val)
			i = end
		case c == '\'':
			end, val := scanString(src, i)
			emit(tkString, i, end, val)
			i = end
		case c == '"':
			j := i + 1
			var b strings.Builder
			for j < n {
				if src[j] == '"' {
					if j+1 < n && src[j+1] == '"' {
						b.WriteByte('"')
						j += 2
						continue
					}
					j++
					break
				}
				b.WriteByte(src[j])
				j++
			}
			emit(tkQuoted, i, j, b.String())
			i = j
		case c == '$':
			if j := i + 1; j < n && src[j] >= '0' && src[j] <= '9' {
				for j < n && src[j] >= '0' && src[j] <= '9' {
					j++
				}
				emit(tkParam, i, j, "")
				i = j
				continue
			}
			// $tag$ ... $tag$
			j := i + 1
			for j < n && (isIdentChar(src[j])) {
				j++
			}
			if j < n && src[j] == '$' {
				tag := src[i : j+1]
				body := j + 1
				end := strings.Index(src[body:], tag)
				if end < 0 {
					emit(tkDollar, i, n, src[body:])
					i = n
					continue
				}
				emit(tkDollar, i, body+end+len(tag), src[body:body+end])
				i = body + end + len(tag)
				continue
			}
			emit(tkPunct, i, i+1, "")
			i++
		case isIdentStart(c):
			j := i
			for j < n && (isIdentChar(src[j]) || src[j] == '$') {
				j++
			}
			emit(tkWord, i, j, "")
			i = j
		case c >= '0' && c <= '9' || (c == '.' && i+1 < n && src[i+1] >= '0' && src[i+1] <= '9'):
			j := i
			for j < n && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				j++
			}
			if j < n && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < n && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < n && src[k] >= '0' && src[k] <= '9' {
					j = k
					for j < n && src[j] >= '0' && src[j] <= '9' {
						j++
					}
				}
			}
			emit(tkNumber, i, j, "")
			i = j
		default:
			matched := false
			for _, op := range multiCharOps {
				if strings.HasPrefix(src[i:], op) {
					emit(tkPunct, i, i+len(op), "")
					i += len(op)
					matched = true
					break
				}
			}
			if !matched {
				// Multi-byte runes outside strings do not occur in the
				// schema; take one byte so the scan always advances.
				emit(tkPunct, i, i+1, "")
				i++
			}
		}
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}

// scanString reads a standard-conforming '...' literal starting at i.
func scanString(src string, i int) (int, string) {
	var b strings.Builder
	j := i + 1
	for j < len(src) {
		if src[j] == '\'' {
			if j+1 < len(src) && src[j+1] == '\'' {
				b.WriteByte('\'')
				j += 2
				continue
			}
			return j + 1, b.String()
		}
		b.WriteByte(src[j])
		j++
	}
	return len(src), b.String()
}

// scanEString reads an escape-string body; i points at its opening quote.
func scanEString(src string, i int) (int, string) {
	var b strings.Builder
	j := i + 1
	for j < len(src) {
		c := src[j]
		if c == '\\' && j+1 < len(src) {
			switch e := src[j+1]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			default:
				b.WriteByte(e)
			}
			j += 2
			continue
		}
		if c == '\'' {
			if j+1 < len(src) && src[j+1] == '\'' {
				b.WriteByte('\'')
				j += 2
				continue
			}
			return j + 1, b.String()
		}
		b.WriteByte(c)
		j++
	}
	return len(src), b.String()
}

// splitStatements cuts a script at top-level semicolons. Comments are removed
// from the result and blank statements dropped; the text between is kept
// verbatim, so a data statement reaches the translator exactly as written.
func splitStatements(src string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	scan(src, func(kind tokKind, start, end int, _ string) {
		text := src[start:end]
		switch {
		case kind == -1:
			if strings.HasPrefix(text, "--") || strings.HasPrefix(text, "/*") {
				cur.WriteByte(' ')
				return
			}
			cur.WriteString(text)
		case kind == tkPunct && text == ";":
			flush()
		default:
			cur.WriteString(text)
		}
	})
	flush()
	return out
}

// tokenize returns the code tokens of one statement.
func tokenize(stmt string) []token {
	var toks []token
	scan(stmt, func(kind tokKind, start, end int, val string) {
		if kind == -1 {
			return
		}
		toks = append(toks, token{kind: kind, text: stmt[start:end], val: val})
	})
	return toks
}

// normaliseData prepares a data statement for the translating driver:
// `public.` schema qualifiers are removed (on MySQL a qualifier names a
// database, so `public.users` would look for a database called public), and
// E'...' escape strings are respelled as standard strings with the same
// value, so the only string syntax left is the one MySQL reads the same way
// under NO_BACKSLASH_ESCAPES.
func normaliseData(stmt string) string {
	var b strings.Builder
	var pendingPublic string // a `public` word seen, waiting to see if a '.' follows
	scan(stmt, func(kind tokKind, start, end int, val string) {
		text := stmt[start:end]
		if pendingPublic != "" {
			if kind == tkPunct && text == "." {
				pendingPublic = ""
				return
			}
			b.WriteString(pendingPublic)
			pendingPublic = ""
		}
		switch {
		case kind == tkWord && strings.EqualFold(text, "public"):
			pendingPublic = text
		case kind == tkString && (text[0] == 'E' || text[0] == 'e'):
			b.WriteString("'" + strings.ReplaceAll(val, "'", "''") + "'")
		default:
			b.WriteString(text)
		}
	})
	b.WriteString(pendingPublic)
	return b.String()
}

// joinTokens renders tokens back to text with single spaces where needed. It
// is for diagnostics and for PostgreSQL expressions kept verbatim.
func joinTokens(toks []token) string {
	var b strings.Builder
	for i, t := range toks {
		if i > 0 && needsSpace(toks[i-1], t) {
			b.WriteByte(' ')
		}
		b.WriteString(t.text)
	}
	return b.String()
}

func needsSpace(prev, next token) bool {
	if prev.isPunct("(") || prev.isPunct(".") || prev.isPunct("::") || prev.isPunct("[") {
		return false
	}
	if next.isPunct(")") || next.isPunct(",") || next.isPunct(".") || next.isPunct("::") || next.isPunct("]") || next.isPunct("[") {
		return false
	}
	if next.isPunct("(") && prev.isIdent() {
		return false
	}
	return true
}

// firstWords returns the upper-cased leading words of a statement, enough to
// classify it.
func firstWords(toks []token, n int) string {
	var words []string
	for _, t := range toks {
		if len(words) == n {
			break
		}
		if t.kind != tkWord {
			break
		}
		words = append(words, strings.ToUpper(t.text))
	}
	return strings.Join(words, " ")
}
