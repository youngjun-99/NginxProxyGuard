package dialect

import (
	"fmt"
	"strconv"
	"strings"
)

// node is one element of a statement: a token, or a bracketed group.
//
// Parenthesised and bracketed spans are parsed into groups so a rewrite can
// treat `f(a, (b + c))` or `(SELECT ...)` as a single operand. Every rewrite
// rule then works on one flat list of siblings, and recursion into groups
// covers subqueries and nested expressions.
type node struct {
	tok   token
	group *group

	// paramRef replaces a token during rewriting: emitted as `?` bound to a
	// caller argument, an expanded array argument, or a synthesised value.
	param *paramRef

	// fn marks a word the rewriter emitted as a function name, which must
	// touch its parenthesis even when it is also a keyword (VALUES(col)).
	fn bool
}

type group struct {
	open  string // "(" or "["
	items []node
}

func (n node) isGroup() bool         { return n.group != nil }
func (n node) isParen() bool         { return n.group != nil && n.group.open == "(" }
func (n node) isBracket() bool       { return n.group != nil && n.group.open == "[" }
func (n node) is(word string) bool   { return n.group == nil && n.param == nil && n.tok.is(word) }
func (n node) isOp(o string) bool    { return n.group == nil && n.param == nil && n.tok.op(o) }
func (n node) isPunct(p string) bool { return n.group == nil && n.param == nil && n.tok.punct(p) }
func (n node) isKind(k tokKind) bool { return n.group == nil && n.param == nil && n.tok.kind == k }
func (n node) isParamNode() bool     { return n.param != nil || n.isKind(tkParam) }
func (n node) lower() string         { return n.tok.lower() }
func (n node) isWordNode() bool      { return n.isKind(tkWord) }
func (n node) isIdentNode() bool     { return n.isKind(tkWord) || n.isKind(tkQuoted) }
func (n node) isLiteral() bool       { return n.isKind(tkString) || n.isKind(tkNumber) }
func (n node) firstIs(word string) bool {
	return n.group != nil && len(n.group.items) > 0 && n.group.items[0].is(word)
}

// Constructors used by rewrites.
func kw(text string) node  { return node{tok: token{tkWord, text}} }
func lit(text string) node { return node{tok: token{tkString, quoteString(text)}} }
func num(text string) node { return node{tok: token{tkNumber, text}} }
func op(text string) node  { return node{tok: token{tkOp, text}} }
func comma() node          { return node{tok: token{tkPunct, ","}} }
func dot() node            { return node{tok: token{tkPunct, "."}} }
func paren(items ...node) node {
	return node{group: &group{open: "(", items: items}}
}

// call builds name(arg1, arg2, ...).
func call(name string, args ...[]node) []node {
	var inner []node
	for i, a := range args {
		if i > 0 {
			inner = append(inner, comma())
		}
		inner = append(inner, a...)
	}
	return []node{{tok: token{tkWord, name}, fn: true}, paren(inner...)}
}

// parse builds the node list, dropping whitespace and comments: emit
// re-inserts the spacing MySQL needs.
func parse(toks []token) ([]node, error) {
	var stack []*group
	root := &group{}
	cur := root
	for _, t := range toks {
		if t.noise() {
			continue
		}
		switch {
		case t.punct("(") || t.punct("["):
			g := &group{open: t.text}
			cur.items = append(cur.items, node{group: g})
			stack = append(stack, cur)
			cur = g
		case t.punct(")") || t.punct("]"):
			want := "("
			if t.text == "]" {
				want = "["
			}
			if len(stack) == 0 || cur.open != want {
				return nil, fmt.Errorf("unbalanced %q", t.text)
			}
			cur = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		default:
			cur.items = append(cur.items, node{tok: t})
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unbalanced brackets")
	}
	return root.items, nil
}

// ---------------------------------------------------------------------------
// Parameters and emission
// ---------------------------------------------------------------------------

type paramKind int

const (
	paramArg       paramKind = iota // a caller argument
	paramArray                      // a caller array argument expanded into an IN list
	paramSynth                      // a value the driver synthesises (RETURNING emulation)
	paramInterval                   // a caller interval string ('300 seconds'), inlined as INTERVAL n UNIT
	paramJSONArray                  // a caller array argument bound as a JSON array
)

type paramRef struct {
	kind  paramKind
	index int // caller argument (0-based) or synthesised slot
	// negate turns an expanded array into NOT IN (x <> ALL($1)).
	negate bool
}

// segment is a piece of an emitted statement: SQL text, or a placeholder.
type segment struct {
	text  string
	param *paramRef
}

// template is a translated statement ready to be bound to arguments.
type template struct {
	segs []segment
}

// paramNode converts a $N token into a caller-argument reference.
func paramNode(t token) (node, error) {
	n, err := strconv.Atoi(t.text[1:])
	if err != nil || n < 1 {
		return node{}, fmt.Errorf("invalid placeholder %s", t.text)
	}
	return node{param: &paramRef{kind: paramArg, index: n - 1}}, nil
}

// emit renders nodes into a template.
func emit(items []node) (*template, error) {
	e := &emitter{}
	if err := e.list(items); err != nil {
		return nil, err
	}
	e.flush()
	return &template{segs: e.segs}, nil
}

type emitter struct {
	segs []segment
	b    strings.Builder
	// prev describes the last thing written, to decide on spacing.
	prev prevKind
}

type prevKind int

const (
	prevNone  prevKind = iota
	prevOpen           // ( or [
	prevDot            // .
	prevWord           // identifier, keyword, literal, param, closing bracket
	prevOther          // operator, comma
)

func (e *emitter) flush() {
	if e.b.Len() > 0 {
		e.segs = append(e.segs, segment{text: e.b.String()})
		e.b.Reset()
	}
}

func (e *emitter) space() {
	if e.prev != prevNone && e.prev != prevOpen && e.prev != prevDot {
		e.b.WriteByte(' ')
	}
}

func (e *emitter) list(items []node) error {
	for i, n := range items {
		switch {
		case n.param != nil:
			e.space()
			e.flush()
			e.segs = append(e.segs, segment{param: n.param})
			e.prev = prevWord

		case n.group != nil:
			// A word directly before ( is a function call or a keyword; MySQL
			// requires a built-in function name to touch its parenthesis.
			if !(n.group.open == "(" && i > 0 && items[i-1].isWordNode() && (items[i-1].fn || isFuncName(items[i-1]))) {
				e.space()
			}
			e.b.WriteString(n.group.open)
			e.prev = prevOpen
			if err := e.list(n.group.items); err != nil {
				return err
			}
			if n.group.open == "(" {
				e.b.WriteByte(')')
			} else {
				e.b.WriteByte(']')
			}
			e.prev = prevWord

		default:
			t := n.tok
			switch t.kind {
			case tkParam:
				return fmt.Errorf("internal: untranslated placeholder %s", t.text)
			case tkDollar:
				return untranslatable("dollar-quoted PL/pgSQL body")
			case tkPunct:
				switch t.text {
				case ",", ";":
					e.b.WriteString(t.text)
					e.prev = prevOther
				case ".":
					e.b.WriteString(".")
					e.prev = prevDot
				}
			default:
				e.space()
				e.b.WriteString(t.text)
				if t.kind == tkOp {
					e.prev = prevOther
				} else {
					e.prev = prevWord
				}
			}
		}
	}
	return nil
}

// untranslatable builds an error wrapping ErrUntranslatable.
func untranslatable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUntranslatable, fmt.Sprintf(format, args...))
}
