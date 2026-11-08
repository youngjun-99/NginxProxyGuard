package database

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// TestMySQLBaselineUpgradeExists pins mysqlBaselineUpgrade to a real entry:
// renamed, it would silently make every historical repair run on MySQL.
func TestMySQLBaselineUpgradeExists(t *testing.T) {
	for _, u := range postgresUpgrades() {
		if u.desc == mysqlBaselineUpgrade {
			return
		}
	}
	t.Fatalf("mysqlBaselineUpgrade %q is not an entry of postgresUpgrades()", mysqlBaselineUpgrade)
}

// TestMySQLMigrations runs RunMigrations against a real MySQL server, twice:
// a fresh database must come up whole with its seed rows, and the second boot
// must change nothing and record no failure. NPG_MYSQL_MIGRATION_URL is a
// mysql:// DATABASE_URL naming a database the test may drop and recreate:
//
//	NPG_MYSQL_MIGRATION_URL='mysql://root:root@127.0.0.1:33071/npg_schema' \
//	    go test ./internal/database -run TestMySQLMigrations -v
func TestMySQLMigrations(t *testing.T) {
	raw := os.Getenv("NPG_MYSQL_MIGRATION_URL")
	if raw == "" {
		t.Skip("NPG_MYSQL_MIGRATION_URL is not set")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	pass, _ := u.User.Password()
	host := u.Host
	if u.Port() == "" {
		host += ":3306"
	}
	admin, err := sql.Open("mysql", u.User.Username()+":"+pass+"@tcp("+host+")/")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"DROP DATABASE IF EXISTS `" + dbName + "`",
		"CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin",
	} {
		if _, err := admin.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	admin.Close()

	for boot := 1; boot <= 2; boot++ {
		db, err := New(raw)
		if err != nil {
			t.Fatalf("boot %d: connect: %v", boot, err)
		}
		if !db.IsMySQL() {
			t.Fatalf("DATABASE_URL %q did not select MySQL", raw)
		}
		if err := db.RunMigrations(); err != nil {
			db.Close()
			t.Fatalf("boot %d: RunMigrations: %v", boot, err)
		}
		if n, last := db.MigrationHealth(); n > 0 {
			db.Close()
			t.Fatalf("boot %d: %d migration statement(s) failed; last: %s", boot, n, last)
		}

		var admins, roles, rules, disabled int
		for q, dst := range map[string]*int{
			"SELECT COUNT(*) FROM users WHERE username = 'admin' AND role_id = '00000000-0000-0000-0000-0000000000a1'": &admins,
			"SELECT COUNT(*) FROM roles WHERE is_builtin":                                                              &roles,
			"SELECT COUNT(*) FROM exploit_block_rules WHERE is_system":                                                 &rules,
			"SELECT COUNT(*) FROM exploit_block_rules WHERE is_system AND NOT enabled":                                 &disabled,
		} {
			if err := db.QueryRow(q).Scan(dst); err != nil {
				db.Close()
				t.Fatalf("boot %d: %s: %v", boot, q, err)
			}
		}
		db.Close()
		if admins != 1 || roles != 3 || rules < 20 || disabled != 3 {
			t.Fatalf("boot %d: seed rows wrong: admin=%d builtin roles=%d system rules=%d disabled=%d",
				boot, admins, roles, rules, disabled)
		}
	}
}
