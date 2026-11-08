package dialect

import (
	"fmt"
	"strings"
)

// tokKind classifies a lexical token of a PostgreSQL statement.
type tokKind int

const (
	tkSpace   tokKind = iota // whitespace
	tkComment                // -- line or /* block */ comment
	tkWord                   // identifier or keyword
	tkQuoted                 // "quoted identifier"
	tkString                 // '...' literal; text is the literal as MySQL must see it
	tkDollar                 // $$...$$ or $tag$...$tag$ body (PL/pgSQL; never translatable)
	tkNumber                 // numeric literal
	tkParam                  // $N placeholder
	tkOp                     // operator: ::, ||, ->>, <>, =, +, ...
	tkPunct                  // ( ) [ ] , ; .
)

type token struct {
	kind tokKind
	text string
}

func (t token) is(word string) bool { return t.kind == tkWord && strings.EqualFold(t.text, word) }
func (t token) punct(p string) bool { return t.kind == tkPunct && t.text == p }
func (t token) op(o string) bool    { return t.kind == tkOp && t.text == o }
func (t token) noise() bool         { return t.kind == tkSpace || t.kind == tkComment }

// lower returns the word in lower case, or "" for a non-word token.
func (t token) lower() string {
	if t.kind != tkWord {
		return ""
	}
	return strings.ToLower(t.text)
}

// operators PostgreSQL code here can contain, longest first so the lexer is
// greedy.
var operators = []string{
	"!~*", "->>", "#>>", "<<=", ">>=", "<=>",
	"::", "=>", "||", "->", "#>", "@>", "<@", "<>", "!=", "<=", ">=", "~*", "!~", "&&", "?|", "?&", "<<", ">>",
	"=", "<", ">", "+", "-", "*", "/", "%", "~", "?", "^", "|", "&", "#", "!", "@", ":",
}

// lex splits a PostgreSQL statement into tokens. String literals are
// normalised to the form MySQL reads under NO_BACKSLASH_ESCAPES: a standard
// '...' literal passes through unchanged, and an E'...' escape string is
// decoded and re-quoted so its backslash escapes mean what PostgreSQL meant.
func lex(sql string) ([]token, error) {
	var out []token
	i, n := 0, len(sql)
	for i < n {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			j := i + 1
			for j < n && strings.IndexByte(" \t\n\r\f", sql[j]) >= 0 {
				j++
			}
			out = append(out, token{tkSpace, sql[i:j]})
			i = j

		case c == '-' && i+1 < n && sql[i+1] == '-':
			j := strings.IndexByte(sql[i:], '\n')
			if j < 0 {
				j = n
			} else {
				j += i
			}
			out = append(out, token{tkComment, sql[i:j]})
			i = j

		case c == '/' && i+1 < n && sql[i+1] == '*':
			// PostgreSQL block comments nest.
			depth, j := 1, i+2
			for j < n && depth > 0 {
				switch {
				case strings.HasPrefix(sql[j:], "/*"):
					depth++
					j += 2
				case strings.HasPrefix(sql[j:], "*/"):
					depth--
					j += 2
				default:
					j++
				}
			}
			if depth > 0 {
				return nil, fmt.Errorf("unterminated block comment")
			}
			out = append(out, token{tkComment, sql[i:j]})
			i = j

		case c == '\'':
			lit, j, err := lexStandardString(sql, i)
			if err != nil {
				return nil, err
			}
			out = append(out, token{tkString, lit})
			i = j

		case (c == 'E' || c == 'e') && i+1 < n && sql[i+1] == '\'' && !wordBefore(out):
			value, j, err := lexEscapeString(sql, i+1)
			if err != nil {
				return nil, err
			}
			out = append(out, token{tkString, quoteString(value)})
			i = j

		case c == '"':
			j := i + 1
			for {
				k := strings.IndexByte(sql[j:], '"')
				if k < 0 {
					return nil, fmt.Errorf("unterminated quoted identifier")
				}
				j += k + 1
				if j < n && sql[j] == '"' { // "" escapes a quote
					j++
					continue
				}
				break
			}
			out = append(out, token{tkQuoted, sql[i:j]})
			i = j

		case c == '$':
			if j := i + 1; j < n && sql[j] >= '0' && sql[j] <= '9' {
				for j < n && sql[j] >= '0' && sql[j] <= '9' {
					j++
				}
				out = append(out, token{tkParam, sql[i:j]})
				i = j
				continue
			}
			// $tag$ ... $tag$
			j := i + 1
			for j < n && (isIdentChar(sql[j])) {
				j++
			}
			if j < n && sql[j] == '$' {
				tag := sql[i : j+1]
				end := strings.Index(sql[j+1:], tag)
				if end < 0 {
					return nil, fmt.Errorf("unterminated dollar-quoted string")
				}
				stop := j + 1 + end + len(tag)
				out = append(out, token{tkDollar, sql[i:stop]})
				i = stop
				continue
			}
			out = append(out, token{tkOp, "$"})
			i++

		case c >= '0' && c <= '9' || (c == '.' && i+1 < n && sql[i+1] >= '0' && sql[i+1] <= '9'):
			j := i
			for j < n && (sql[j] >= '0' && sql[j] <= '9' || sql[j] == '.') {
				// "1..2" is not a number we write; stop at a second dot.
				if sql[j] == '.' && j+1 < n && sql[j+1] == '.' {
					break
				}
				j++
			}
			if j < n && (sql[j] == 'e' || sql[j] == 'E') {
				k := j + 1
				if k < n && (sql[k] == '+' || sql[k] == '-') {
					k++
				}
				if k < n && sql[k] >= '0' && sql[k] <= '9' {
					for k < n && sql[k] >= '0' && sql[k] <= '9' {
						k++
					}
					j = k
				}
			}
			out = append(out, token{tkNumber, sql[i:j]})
			i = j

		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentChar(sql[j]) {
				j++
			}
			out = append(out, token{tkWord, sql[i:j]})
			i = j

		case strings.IndexByte("()[],;.", c) >= 0:
			out = append(out, token{tkPunct, string(c)})
			i++

		default:
			matched := false
			for _, op := range operators {
				if strings.HasPrefix(sql[i:], op) {
					out = append(out, token{tkOp, op})
					i += len(op)
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("unexpected character %q", c)
			}
		}
	}
	return out, nil
}

// wordBefore reports whether the previous token is a word glued to the
// current position, so the E in `somename'...'` is not an escape prefix.
func wordBefore(toks []token) bool {
	return len(toks) > 0 && toks[len(toks)-1].kind == tkWord
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}

// lexStandardString reads a '...' literal starting at i, where ” escapes a
// quote and a backslash is an ordinary character (standard_conforming_strings).
// It returns the literal text verbatim: with NO_BACKSLASH_ESCAPES MySQL reads
// it identically.
func lexStandardString(sql string, i int) (string, int, error) {
	j := i + 1
	for {
		k := strings.IndexByte(sql[j:], '\'')
		if k < 0 {
			return "", 0, fmt.Errorf("unterminated string literal")
		}
		j += k + 1
		if j < len(sql) && sql[j] == '\'' {
			j++
			continue
		}
		return sql[i:j], j, nil
	}
}

// lexEscapeString decodes an E'...' literal whose opening quote is at i.
func lexEscapeString(sql string, i int) (string, int, error) {
	var b strings.Builder
	j := i + 1
	for j < len(sql) {
		c := sql[j]
		switch {
		case c == '\'' && j+1 < len(sql) && sql[j+1] == '\'':
			b.WriteByte('\'')
			j += 2
		case c == '\'':
			return b.String(), j + 1, nil
		case c == '\\' && j+1 < len(sql):
			e := sql[j+1]
			j += 2
			switch e {
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
		default:
			b.WriteByte(c)
			j++
		}
	}
	return "", 0, fmt.Errorf("unterminated escape string literal")
}

// quoteString renders value as a standard SQL literal.
func quoteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// unquoteString returns the value of a standard '...' literal token.
func unquoteString(lit string) string {
	if len(lit) < 2 {
		return lit
	}
	return strings.ReplaceAll(lit[1:len(lit)-1], "''", "'")
}
