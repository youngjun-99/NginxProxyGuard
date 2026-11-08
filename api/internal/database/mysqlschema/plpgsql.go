package mysqlschema

import (
	"fmt"
	"strings"
)

// doBlock replays the static DDL inside a PL/pgSQL DO block.
//
// The schema files wrap DDL in DO blocks for one reason: PostgreSQL has no
// IF NOT EXISTS for ADD CONSTRAINT or CREATE TYPE, so the statement is guarded
// by an exception handler or a catalog check to stay re-runnable. The guards
// say "only if not done yet", which the model gets for free, so every static
// DDL statement in the block is applied in order, whatever branch it sits in.
// The branches that do differ — TimescaleDB present or not — differ only in
// how a column is widened, and both arrive at the same type.
//
// Left out, and recorded: dynamic SQL (EXECUTE, which only ever targets
// TimescaleDB chunks, partitions and the legacy log view) and DML, which in a
// DO block is always a one-shot repair of rows a PostgreSQL install
// accumulated before a fix; a MySQL install never had them.
func (s *Schema) doBlock(c *cursor) error {
	var body string
	for _, t := range c.toks {
		if t.kind == tkDollar {
			body = t.val
			break
		}
	}
	if body == "" {
		return fmt.Errorf("DO block without a $-quoted body")
	}

	var applied, dml, dynamic int
	var firstDDL, firstDML string
	inDeclare := false

	for _, piece := range splitPL(tokenize(body)) {
		p := piece
		if inDeclare {
			i := indexWord(p, "BEGIN")
			if i < 0 {
				continue
			}
			p, inDeclare = p[i+1:], false
		}
		p, inDeclare = stripControl(p, inDeclare)
		if len(p) == 0 {
			continue
		}
		switch head := strings.ToUpper(p[0].text); {
		case len(p) > 1 && (p[1].isPunct(":=") || p[1].isPunct("=")) && p[0].isIdent():
			// variable assignment
		case head == "NULL", head == "RAISE", head == "RETURN", head == "PERFORM", head == "GET",
			head == "SELECT", head == "WITH", head == "CONTINUE", head == "EXIT", head == "COMMIT":
		case head == "EXECUTE":
			dynamic++
		case head == "INSERT", head == "UPDATE", head == "DELETE", head == "TRUNCATE":
			dml++
			if firstDML == "" {
				firstDML = snippet(p)
			}
		case head == "ALTER", head == "CREATE", head == "DROP", head == "COMMENT":
			if err := s.applyDDL(p); err != nil {
				return fmt.Errorf("DO block: %w", err)
			}
			applied++
			if firstDDL == "" {
				firstDDL = snippet(p)
			}
		case head == "SET":
			// SET LOCAL statement_timeout and friends
		default:
			return fmt.Errorf("DO block: unrecognised PL/pgSQL statement %q", snippet(p))
		}
	}

	var left []string
	if dynamic > 0 {
		left = append(left, fmt.Sprintf("%d dynamic EXECUTE (TimescaleDB chunks/partitions/legacy view)", dynamic))
	}
	if dml > 0 {
		left = append(left, fmt.Sprintf("%d data statement(s), a repair of PostgreSQL install history (%s)", dml, firstDML))
	}
	if len(left) > 0 {
		obj := "DO block"
		if firstDDL != "" {
			obj += " (applied: " + firstDDL + ")"
		}
		s.skip(obj, "left out "+strings.Join(left, "; "))
	} else if applied == 0 {
		s.skip("DO block", "no static DDL inside")
	}
	return nil
}

// splitPL cuts a PL/pgSQL body at semicolons outside parentheses.
func splitPL(toks []token) [][]token {
	var out [][]token
	depth, start := 0, 0
	for i, t := range toks {
		switch {
		case t.isPunct("("):
			depth++
		case t.isPunct(")"):
			depth--
		case t.isPunct(";") && depth == 0:
			out = append(out, toks[start:i])
			start = i + 1
		}
	}
	if start < len(toks) {
		out = append(out, toks[start:])
	}
	return out
}

// stripControl removes the control-flow words in front of a statement:
// BEGIN, IF ... THEN, ELSIF ... THEN, ELSE, EXCEPTION WHEN ... THEN, WHEN ...
// THEN, FOR/WHILE ... LOOP, LOOP, labels, and DECLARE (returning true while
// still inside the declaration section).
func stripControl(p []token, inDeclare bool) ([]token, bool) {
	for len(p) > 0 {
		switch {
		case p[0].is("DECLARE"):
			i := indexWord(p, "BEGIN")
			if i < 0 {
				return nil, true
			}
			p = p[i+1:]
		case p[0].is("BEGIN"), p[0].is("ELSE"), p[0].is("LOOP"):
			p = p[1:]
		case p[0].is("END"):
			return nil, inDeclare
		case p[0].isPunct("<") && len(p) > 1 && p[1].isPunct("<"):
			// <<label>>
			i := 2
			for i < len(p) && !p[i].isPunct(">") {
				i++
			}
			p = p[min(i+2, len(p)):]
		case p[0].is("IF"), p[0].is("ELSIF"), p[0].is("EXCEPTION"), p[0].is("WHEN"):
			i := indexWord(p, "THEN")
			if i < 0 {
				return nil, inDeclare
			}
			p = p[i+1:]
		case p[0].is("FOR"), p[0].is("WHILE"), p[0].is("FOREACH"):
			i := indexWord(p, "LOOP")
			if i < 0 {
				return nil, inDeclare
			}
			p = p[i+1:]
		default:
			return p, inDeclare
		}
	}
	return p, inDeclare
}

// indexWord finds a keyword at parenthesis depth 0.
func indexWord(toks []token, word string) int {
	depth := 0
	for i, t := range toks {
		switch {
		case t.isPunct("("):
			depth++
		case t.isPunct(")"):
			depth--
		case depth == 0 && t.is(word):
			return i
		}
	}
	return -1
}
