package dialect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nginx-proxy-guard/internal/database/dialect/sqlscan"
)

// TestEveryQueryTranslates sweeps the SQL literals under internal/ through
// the translator.
//
// It is what keeps MySQL support from rotting: repositories write
// PostgreSQL and rely on the driver to rewrite it, so a new query using a
// construct the translator cannot express would otherwise only fail on a
// MySQL install, at run time. Writing such a query fails here instead.
//
// SQL that must stay PostgreSQL-only — behind a dialect.IsMySQL() branch that
// has a MySQL counterpart, or in code only reached on PostgreSQL — is marked
// with a `//npg:pg-only` comment on the line above the literal (or listed in
// sqlscan.PGOnlyFiles when the whole file is). MySQL written for the raw path is
// marked `//npg:mysql`.
func TestEveryQueryTranslates(t *testing.T) {
	root := internalRoot(t)
	var failures []string
	checked := 0
	lits, err := sqlscan.Literals(root, sqlscan.SkipNonApplication)
	if err != nil {
		t.Fatal(err)
	}
	for _, lit := range lits {
		checked++
		_, err := Translate(lit.Text)
		if err != nil && errors.Is(err, ErrUntranslatable) {
			failures = append(failures, fmt.Sprintf("%s:%d: %v\n    %s", lit.File, lit.Line, err, snippet(lit.Text)))
		}
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Fatalf("%d of %d SQL literals cannot be translated for MySQL. Add a MySQL form behind dialect.IsMySQL() and mark the PostgreSQL literal //npg:pg-only:\n\n%s",
			len(failures), checked, strings.Join(failures, "\n"))
	}
	t.Logf("%d SQL literals translate", checked)
}

func internalRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "repository")); err != nil {
		t.Skipf("cannot locate internal/: %v", err)
	}
	return root
}

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 140 {
		return s[:140] + "..."
	}
	return s
}
