// Package sqlscan finds the SQL statements written as string literals in the
// Go sources, for tests that check every query the application can send.
package sqlscan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Literal is one SQL statement found in the sources.
type Literal struct {
	File string // relative to the scanned root, slash-separated
	Line int
	Text string
}

// Markers that exclude a literal when they appear on its line or the two
// lines above it:
//
//	//npg:pg-only  PostgreSQL-only SQL behind an engine branch
//	//npg:mysql    MySQL SQL sent through dialect.Raw
const (
	MarkerPGOnly = "npg:pg-only"
	MarkerMySQL  = "npg:mysql"
)

const rawPrefix = "/*npg:raw*/"

// PGOnlyFiles hold nothing but PostgreSQL-only SQL, relative to internal/.
var PGOnlyFiles = map[string]bool{
	// The PostgreSQL migration runner: TimescaleDB, PL/pgSQL, catalog repair.
	// MySQL runs migration_mysql.go instead.
	"database/migration.go": true,

	// Partition, chunk and ANALYZE upkeep. PartitionScheduler.run hands MySQL
	// to scheduler/partition_mysql.go before any of it is reached.
	"scheduler/partition.go": true,
}

// SkipNonApplication reports files whose SQL is not application SQL to be
// checked: the PostgreSQL-only files, and the translator and schema packages
// themselves (their literals are examples and MySQL DDL).
func SkipNonApplication(rel string) bool {
	return PGOnlyFiles[rel] || strings.HasPrefix(rel, "database/dialect/") || strings.HasPrefix(rel, "database/mysqlschema/")
}

var statementStart = regexp.MustCompile(`(?is)^\s*(SELECT|INSERT\s+INTO|UPDATE\s+\S+\s+(AS\s+\S+\s+|\S+\s+)?SET|DELETE\s+FROM|WITH\s+(RECURSIVE\s+)?\S+\s+AS)\b`)

// Literals returns the whole-statement SQL literals in non-test Go files under
// root. skip reports files (relative paths) to leave out. Fragments of
// statements assembled at run time and Printf templates are not returned:
// they are not statements on their own.
func Literals(root string, skip func(rel string) bool) ([]Literal, error) {
	var out []Literal
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if (skip != nil && skip(rel)) || strings.HasSuffix(rel, "_mysql.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(src), "\n")
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return nil
		}
		fragments := fragmentLiterals(file)
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || fragments[lit] {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil || !statementStart.MatchString(text) || strings.HasPrefix(text, rawPrefix) || len(strings.Fields(text)) < 4 {
				return true
			}
			if strings.Contains(text, "%s") || strings.Contains(text, "%d") || strings.Contains(text, "%v") || strings.Contains(text, "%q") {
				return true
			}
			line := fset.Position(lit.Pos()).Line
			if marked(lines, line) {
				return true
			}
			out = append(out, Literal{File: rel, Line: line, Text: text})
			return true
		})
		return nil
	})
	return out, err
}

func marked(lines []string, line int) bool {
	for l := line - 3; l < line; l++ {
		if l >= 0 && l < len(lines) && (strings.Contains(lines[l], MarkerPGOnly) || strings.Contains(lines[l], MarkerMySQL)) {
			return true
		}
	}
	return false
}

// fragmentLiterals finds literals that are only part of a statement: operands
// of a + concatenation, the start of a string later extended with +=, and the
// arguments of dialect.Raw (MySQL, not PostgreSQL).
func fragmentLiterals(file *ast.File) map[*ast.BasicLit]bool {
	out := map[*ast.BasicLit]bool{}
	markAll := func(n ast.Node) {
		ast.Inspect(n, func(m ast.Node) bool {
			if lit, ok := m.(*ast.BasicLit); ok {
				out[lit] = true
			}
			return true
		})
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if x.Op == token.ADD {
				markAll(x)
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Raw" {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "dialect" {
					markAll(x)
				}
			}
		case *ast.FuncDecl, *ast.FuncLit:
			var body *ast.BlockStmt
			if fd, ok := x.(*ast.FuncDecl); ok {
				body = fd.Body
			} else {
				body = x.(*ast.FuncLit).Body
			}
			if body == nil {
				return true
			}
			extended := map[string]bool{}
			ast.Inspect(body, func(m ast.Node) bool {
				if as, ok := m.(*ast.AssignStmt); ok && as.Tok == token.ADD_ASSIGN {
					for _, l := range as.Lhs {
						if id, ok := l.(*ast.Ident); ok {
							extended[id.Name] = true
						}
					}
				}
				return true
			})
			ast.Inspect(body, func(m ast.Node) bool {
				as, ok := m.(*ast.AssignStmt)
				if !ok || (as.Tok != token.DEFINE && as.Tok != token.ASSIGN) {
					if vs, ok := m.(*ast.ValueSpec); ok {
						for i, name := range vs.Names {
							if extended[name.Name] && i < len(vs.Values) {
								markAll(vs.Values[i])
							}
						}
					}
					return true
				}
				for i, l := range as.Lhs {
					if id, ok := l.(*ast.Ident); ok && extended[id.Name] && i < len(as.Rhs) {
						markAll(as.Rhs[i])
					}
				}
				return true
			})
		}
		return true
	})
	return out
}
