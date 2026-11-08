package mysqlschema_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"nginx-proxy-guard/internal/database"
)

// These tests need a MySQL server. NPG_MYSQL_SCHEMA_DSN is a go-sql-driver
// DSN naming a database the test may drop and recreate:
//
//	docker run -d --name npg-mysql-test -e MYSQL_ROOT_PASSWORD=root -p 33071:3306 mysql:8.4
//	NPG_MYSQL_SCHEMA_DSN='root:root@tcp(127.0.0.1:33071)/npg_schema' \
//	    go test ./internal/database/mysqlschema -run Sync -v
//
// They use a plain go-sql-driver connection: the DDL Sync runs is MySQL to
// begin with, so it needs no translation.
func openEmptyDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("NPG_MYSQL_SCHEMA_DSN")
	if dsn == "" {
		t.Skip("NPG_MYSQL_SCHEMA_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("bad DSN: %v", err)
	}
	dbName := cfg.DBName
	if dbName == "" {
		t.Fatal("NPG_MYSQL_SCHEMA_DSN must name a database")
	}

	admin := cfg.Clone()
	admin.DBName = ""
	conn, err := sql.Open("mysql", admin.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, stmt := range []string{
		"DROP DATABASE IF EXISTS `" + dbName + "`",
		"CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin",
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSyncCreatesEverythingAndIsIdempotent(t *testing.T) {
	db := openEmptyDatabase(t)
	ctx := context.Background()
	schema, err := database.MySQLSchema()
	if err != nil {
		t.Fatalf("MySQLSchema: %v", err)
	}

	report, err := schema.Sync(ctx, db)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := report.Err(); err != nil {
		t.Fatalf("Sync on an empty database refused statements:\n%v", err)
	}
	if len(report.Drift) > 0 {
		t.Errorf("drift right after creating the schema: %v", report.Drift)
	}

	// Every table, with every column.
	live := map[string]map[string]bool{}
	rows, err := db.Query("SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE()")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var table, col string
		if err := rows.Scan(&table, &col); err != nil {
			t.Fatal(err)
		}
		if live[table] == nil {
			live[table] = map[string]bool{}
		}
		live[table][col] = true
	}
	rows.Close()
	for _, table := range schema.Tables() {
		if live[table] == nil {
			t.Errorf("table %s was not created", table)
			continue
		}
		for _, col := range schema.Columns(table) {
			if !live[table][col] {
				t.Errorf("column %s.%s was not created", table, col)
			}
		}
	}
	for _, view := range schema.Views() {
		if live[view] == nil {
			t.Errorf("view %s was not created", view)
		}
	}

	again, err := schema.Sync(ctx, db)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(again.Executed) > 0 || len(again.Failed) > 0 || len(again.Drift) > 0 {
		t.Fatalf("second Sync was not a no-op:\nexecuted: %v\nfailed: %v\ndrift: %v",
			again.Executed, again.Failed, again.Drift)
	}
}

// TestSyncRepairsAPartialDatabase covers the upgrade path: an install whose
// tables predate a column, an index and a foreign key gets them, and only
// them.
func TestSyncRepairsAPartialDatabase(t *testing.T) {
	db := openEmptyDatabase(t)
	ctx := context.Background()
	schema, err := database.MySQLSchema()
	if err != nil {
		t.Fatalf("MySQLSchema: %v", err)
	}
	if report, err := schema.Sync(ctx, db); err != nil || report.Err() != nil {
		t.Fatalf("Sync: %v %v", err, report.Err())
	}

	for _, stmt := range []string{
		"ALTER TABLE `proxy_hosts` DROP COLUMN `tags`",
		"ALTER TABLE `certificates` DROP INDEX `idx_certificates_status`",
		"ALTER TABLE `api_tokens` DROP FOREIGN KEY `api_tokens_user_id_fkey`",
		"DROP VIEW `logs_unified`",
		"DROP TABLE `dns_entry_states`",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	report, err := schema.Sync(ctx, db)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := report.Err(); err != nil {
		t.Fatalf("repair refused: %v", err)
	}
	joined := strings.Join(report.Executed, "\n")
	for _, want := range []string{"ADD COLUMN `tags`", "idx_certificates_status", "api_tokens_user_id_fkey", "VIEW `logs_unified`", "CREATE TABLE IF NOT EXISTS `dns_entry_states`"} {
		if !strings.Contains(joined, want) {
			t.Errorf("repair did not run %q; ran:\n%s", want, joined)
		}
	}
	if len(report.Executed) != 5 {
		t.Errorf("repair ran %d statements, want exactly 5:\n%s", len(report.Executed), joined)
	}
}

// TestSchemaBehaviour checks the translations that carry a guarantee rather
// than a shape: partial unique indexes, singletons, case-sensitive keys,
// updated_at stamping, and the lib/pq array defaults.
func TestSchemaBehaviour(t *testing.T) {
	db := openEmptyDatabase(t)
	ctx := context.Background()
	schema, err := database.MySQLSchema()
	if err != nil {
		t.Fatalf("MySQLSchema: %v", err)
	}
	if report, err := schema.Sync(ctx, db); err != nil || report.Err() != nil {
		t.Fatalf("Sync: %v %v", err, report.Err())
	}

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustFail := func(why, q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err == nil {
			t.Fatalf("%s: %s succeeded", why, q)
		}
	}

	// A proxy host to hang host-scoped rows off.
	mustExec("INSERT INTO proxy_hosts (id, domain_names, forward_scheme, forward_host, forward_port) VALUES ('11111111-1111-1111-1111-111111111111', '{\"a.example\"}', 'http', 'backend', 80)")

	// banned_ips: one global ban per address, one per (address, host), and
	// a global and a host-scoped ban for the same address may coexist.
	mustExec("INSERT INTO banned_ips (ip_address) VALUES ('10.0.0.1')")
	mustExec("INSERT INTO banned_ips (ip_address, proxy_host_id) VALUES ('10.0.0.1', '11111111-1111-1111-1111-111111111111')")
	mustFail("duplicate global ban", "INSERT INTO banned_ips (ip_address) VALUES ('10.0.0.1')")
	mustFail("duplicate host ban", "INSERT INTO banned_ips (ip_address, proxy_host_id) VALUES ('10.0.0.1', '11111111-1111-1111-1111-111111111111')")

	// Singleton tables take one row.
	mustExec("INSERT INTO global_waf () VALUES ()")
	mustFail("second global_waf row", "INSERT INTO global_waf () VALUES ()")

	// users: username is case-sensitive like PostgreSQL text, email is unique
	// case-insensitively through idx_users_email_lower.
	mustExec("INSERT INTO users (email, username, password_hash) VALUES ('a@x', 'Alice', 'h')")
	mustExec("INSERT INTO users (email, username, password_hash) VALUES ('b@x', 'alice', 'h')")
	mustFail("email differing only in case", "INSERT INTO users (email, username, password_hash) VALUES ('A@X', 'carol', 'h')")

	// Array defaults are lib/pq literals with quoted elements.
	var excluded string
	mustExec("INSERT INTO system_settings () VALUES ()")
	if err := db.QueryRow("SELECT system_logs_stdout_excluded FROM system_settings LIMIT 1").Scan(&excluded); err != nil {
		t.Fatal(err)
	}
	if excluded != `{"npg-proxy"}` {
		t.Errorf(`system_logs_stdout_excluded default = %q, want {"npg-proxy"}`, excluded)
	}

	// A trigger-stamped updated_at moves on UPDATE.
	mustExec("UPDATE proxy_hosts SET updated_at = '2000-01-01 00:00:00' WHERE id = '11111111-1111-1111-1111-111111111111'")
	mustExec("UPDATE proxy_hosts SET forward_port = 81 WHERE id = '11111111-1111-1111-1111-111111111111'")
	var stamped bool
	if err := db.QueryRow("SELECT updated_at > '2001-01-01' FROM proxy_hosts WHERE id = '11111111-1111-1111-1111-111111111111'").Scan(&stamped); err != nil {
		t.Fatal(err)
	}
	if !stamped {
		t.Error("proxy_hosts.updated_at was not stamped by the UPDATE")
	}

	// Deleting the host cascades to its bans.
	mustExec("DELETE FROM proxy_hosts WHERE id = '11111111-1111-1111-1111-111111111111'")
	var bans int
	if err := db.QueryRow("SELECT COUNT(*) FROM banned_ips").Scan(&bans); err != nil {
		t.Fatal(err)
	}
	if bans != 1 {
		t.Errorf("after deleting the host, %d bans remain, want the global one", bans)
	}
}
