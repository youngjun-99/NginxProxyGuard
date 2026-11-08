package integration

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"nginx-proxy-guard/internal/database"
	"nginx-proxy-guard/internal/database/dialect"
	"nginx-proxy-guard/internal/database/dialect/sqlscan"
)

// These tests need a MySQL 8.4 server. NPG_MYSQL_TEST_URL names a database
// they drop and recreate, so point it at a scratch one:
//
//	docker run -d --name npg-mysql-test -e MYSQL_ROOT_PASSWORD=root -p 33071:3306 mysql:8.4
//	NPG_MYSQL_TEST_URL='mysql://root:root@127.0.0.1:33071/npg_it' \
//	    go test ./tests/integration/ -run MySQL -v
func mysqlTestURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("NPG_MYSQL_TEST_URL")
	if u == "" {
		t.Skip("NPG_MYSQL_TEST_URL is not set; skipping the MySQL integration tests")
	}
	return u
}

// rawMySQL opens a plain go-sql-driver connection with the session settings
// NPG uses, to the database named in rawURL (or none).
func rawMySQL(t *testing.T, rawURL, dbName string) *sql.DB {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	dsn, err := dialect.MySQLDSN(u.String(), false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// freshMySQL recreates the test database and opens it through NPG.
func freshMySQL(t *testing.T) *database.DB {
	t.Helper()
	raw := mysqlTestURL(t)
	u, _ := url.Parse(raw)
	name := strings.TrimPrefix(u.Path, "/")
	admin := rawMySQL(t, raw, "mysql")
	for _, q := range []string{
		"DROP DATABASE IF EXISTS `" + name + "`",
		"CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin",
	} {
		if _, err := admin.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db, err := database.New(raw)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if !db.IsMySQL() {
		t.Fatalf("engine detected as %s", db.Kind())
	}
	return db
}

func TestMySQLMigrationsApplyCleanly(t *testing.T) {
	db := freshMySQL(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if n, last := db.MigrationHealth(); n > 0 {
		t.Fatalf("%d migration statement(s) failed; last: %s", n, last)
	}
	// Every boot replays the upgrades; a second run must be just as clean.
	db2, err := database.New(mysqlTestURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := db2.RunMigrations(); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	if n, last := db2.MigrationHealth(); n > 0 {
		t.Fatalf("re-run: %d migration statement(s) failed; last: %s", n, last)
	}
}

// TestMySQLEveryQueryPrepares has the server PREPARE every statement the
// application contains, as translated. A prepare parses the statement and
// resolves every table, column and function it names without running it, so
// this catches a translation MySQL rejects and a query naming a column the
// MySQL schema lacks.
func TestMySQLEveryQueryPrepares(t *testing.T) {
	db := freshMySQL(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	u, _ := url.Parse(mysqlTestURL(t))
	raw := rawMySQL(t, mysqlTestURL(t), strings.TrimPrefix(u.Path, "/"))

	root, _ := filepath.Abs(filepath.Join("..", "..", "internal"))
	lits, err := sqlscan.Literals(root, sqlscan.SkipNonApplication)
	if err != nil {
		t.Fatal(err)
	}
	var failures []string
	prepared := 0
	for _, lit := range lits {
		stmts, err := dialect.PreparableSQL(lit.Text)
		if err != nil {
			if !errors.Is(err, dialect.ErrUntranslatable) {
				failures = append(failures, fmt.Sprintf("%s:%d: translate: %v", lit.File, lit.Line, err))
			}
			continue // untranslatable ones are TestEveryQueryTranslates' business
		}
		for _, q := range stmts {
			st, err := raw.Prepare(q)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s:%d: %v\n    %s", lit.File, lit.Line, err, oneLine(q)))
				continue
			}
			st.Close()
			prepared++
		}
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Fatalf("%d statement(s) MySQL refuses (%d prepared fine):\n\n%s", len(failures), prepared, strings.Join(failures, "\n"))
	}
	t.Logf("%d statements prepared on MySQL", prepared)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
