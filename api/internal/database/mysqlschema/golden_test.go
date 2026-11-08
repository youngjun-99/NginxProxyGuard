package mysqlschema_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nginx-proxy-guard/internal/database"
)

var update = flag.Bool("update", false, "rewrite testdata/schema.mysql.sql")

const goldenPath = "testdata/schema.mysql.sql"

// TestGolden keeps testdata/schema.mysql.sql equal to what the replay
// derives from the PostgreSQL schema. The file is the reviewable face of the
// MySQL schema: a change to 001_init.sql or the upgrade list shows up in a
// diff of it. Regenerate with
//
//	go test ./internal/database/mysqlschema -run Golden -update
func TestGolden(t *testing.T) {
	schema, err := database.MySQLSchema()
	if err != nil {
		t.Fatalf("MySQLSchema: %v", err)
	}
	got, err := schema.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading %s (run with -update to create it): %v", goldenPath, err)
	}
	if string(want) != got {
		t.Fatalf("%s is stale: the PostgreSQL schema changed. Regenerate with\n"+
			"  go test ./internal/database/mysqlschema -run Golden -update\nfirst difference: %s",
			goldenPath, firstDiff(string(want), got))
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return "line " + itoa(i+1) + ":\n  file: " + x + "\n  now:  " + y
		}
	}
	return "(none)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestEveryTableAndColumnCarriesOver guards the replay itself: every table the
// PostgreSQL schema ends with is in the MySQL schema, minus partition children
// and the two documented history tables, with every column.
func TestEveryTableAndColumnCarriesOver(t *testing.T) {
	schema, err := database.MySQLSchema()
	if err != nil {
		t.Fatalf("MySQLSchema: %v", err)
	}
	tables := map[string]bool{}
	for _, name := range schema.Tables() {
		tables[name] = true
	}
	for _, must := range []string{
		"users", "proxy_hosts", "logs_partitioned", "system_settings", "global_settings",
		"certificates", "banned_ips", "notification_outbox", "dns_entry_states", "raw_log_reclaim_job",
		"traffic_hourly_backfill", "role_permissions", "sso_providers", "user_identities",
	} {
		if !tables[must] {
			t.Errorf("table %s is missing from the MySQL schema", must)
		}
	}
	for _, child := range schema.PartitionChildren() {
		if tables[child] {
			t.Errorf("partition child %s leaked into the MySQL schema", child)
		}
	}
	for _, history := range []string{"logs", "dashboard_stats_hourly_partitioned"} {
		if tables[history] {
			t.Errorf("history table %s should not be carried over", history)
		}
	}

	// Columns added only by upgrades or late in 001_init.sql must be there.
	cols := map[string]bool{}
	for _, c := range schema.Columns("proxy_hosts") {
		cols[c] = true
	}
	for _, must := range []string{"tags", "waf_use_global", "ddns_proxied", "auth_bypass_paths", "stream_listen_port"} {
		if !cols[must] {
			t.Errorf("proxy_hosts.%s is missing", must)
		}
	}
}

// TestDataStatementsArePostgreSQLWithoutSchema checks the hand-off to the
// translating driver: seeds are there, in PostgreSQL syntax, and none still
// names the public schema, which on MySQL would name a database.
func TestDataStatementsArePostgreSQLWithoutSchema(t *testing.T) {
	schema, err := database.MySQLSchema()
	if err != nil {
		t.Fatalf("MySQLSchema: %v", err)
	}
	var seededAdmin, seededRules bool
	for _, d := range append(schema.InitData(), schema.UpgradeData()...) {
		for _, prefix := range []string{"INSERT INTO public.", "UPDATE public.", "DELETE FROM public.", " JOIN public.", " FROM public."} {
			if strings.Contains(d.SQL, prefix) {
				t.Errorf("%s: still schema-qualified (%q): %.120s", d.Source, prefix, d.SQL)
			}
		}
		if strings.Contains(d.SQL, "E'") {
			t.Errorf("%s: E'' string reached the translator: %.120s", d.Source, d.SQL)
		}
		if strings.HasPrefix(d.SQL, "INSERT INTO users") {
			seededAdmin = true
		}
		if strings.HasPrefix(d.SQL, "INSERT INTO exploit_block_rules") {
			seededRules = true
		}
	}
	if !seededAdmin || !seededRules {
		t.Errorf("seed statements missing: admin=%v exploit rules=%v", seededAdmin, seededRules)
	}
}
