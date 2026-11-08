package dialect

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

// These tests run the translating driver against a real MySQL server. Point
// NPG_MYSQL_TEST_URL at an empty scratch database:
//
//	docker run -d --name npg-mysql-test -e MYSQL_ROOT_PASSWORD=root \
//	    -e MYSQL_DATABASE=npg -p 33071:3306 mysql:8.4
//	NPG_MYSQL_TEST_URL='mysql://root:root@127.0.0.1:33071/npg' \
//	    go test ./internal/database/dialect/ -run MySQL -v
func openMySQL(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("NPG_MYSQL_TEST_URL")
	if url == "" {
		t.Skip("NPG_MYSQL_TEST_URL is not set")
	}
	dsn, err := MySQLDSN(url, false)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		"DROP TABLE IF EXISTS drv_items",
		"DROP TABLE IF EXISTS drv_counters",
		"DROP TABLE IF EXISTS drv_serial",
		`CREATE TABLE drv_items (
			id CHAR(36) NOT NULL DEFAULT (UUID()) PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			tags TEXT,
			config JSON,
			enabled BOOLEAN NOT NULL DEFAULT true,
			hits INT NOT NULL DEFAULT 0,
			created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_drv_items_name (name)
		)`,
		`CREATE TABLE drv_counters (
			bucket DATETIME(6) NOT NULL, host VARCHAR(50) NOT NULL, n INT NOT NULL,
			PRIMARY KEY (bucket, host)
		)`,
		`CREATE TABLE drv_serial (id BIGINT AUTO_INCREMENT PRIMARY KEY, v TEXT)`,
	} {
		if _, err := db.Exec(Raw(ddl)); err != nil {
			t.Fatalf("%s: %v", ddl, err)
		}
	}
	forgetTableMeta()
	return db
}

func TestMySQLReturning(t *testing.T) {
	db := openMySQL(t)
	ctx := context.Background()

	// INSERT ... RETURNING with a UUID default: the driver supplies the key.
	var id string
	var createdAt time.Time
	var enabled bool
	err := db.QueryRowContext(ctx,
		`INSERT INTO drv_items (name, tags, config) VALUES ($1, $2, $3::jsonb) RETURNING id, created_at, enabled`,
		"alpha", pq.StringArray{"a.example", "b.example"}, `{"url": "https://x", "n": 1}`).Scan(&id, &createdAt, &enabled)
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || createdAt.IsZero() || !enabled || createdAt.Location() != time.UTC {
		t.Fatalf("got id=%q created_at=%v enabled=%v", id, createdAt, enabled)
	}

	// Multi-row INSERT ... RETURNING keeps row order.
	rows, err := db.QueryContext(ctx, `INSERT INTO drv_items (name) VALUES ($1), ($2) RETURNING name`, "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	if len(names) != 2 || names[0] != "beta" || names[1] != "gamma" {
		t.Fatalf("multi-row RETURNING: %v", names)
	}

	// ON CONFLICT DO NOTHING RETURNING: no row when it already existed.
	err = db.QueryRowContext(ctx, `INSERT INTO drv_items (name) VALUES ($1) ON CONFLICT (name) DO NOTHING RETURNING id`, "alpha").Scan(&id)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DO NOTHING on a duplicate: %v", err)
	}

	// Upsert RETURNING finds the existing row by its conflict target.
	var hits int
	var gotID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO drv_items (name, hits) VALUES ($1, 1)
		ON CONFLICT (name) DO UPDATE SET hits = drv_items.hits + EXCLUDED.hits
		RETURNING id, hits`, "alpha").Scan(&gotID, &hits)
	if err != nil || hits != 1 {
		t.Fatalf("upsert: hits=%d err=%v", hits, err)
	}

	// An expression conflict target: the row is found by lower(name).
	if _, err := db.ExecContext(ctx, Raw("CREATE UNIQUE INDEX uq_drv_items_lower ON drv_items ((LOWER(name)))")); err != nil {
		t.Fatal(err)
	}
	var lowerID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO drv_items (name, hits) VALUES ($1, 7)
		ON CONFLICT (lower((name)::text)) DO UPDATE SET hits = EXCLUDED.hits
		RETURNING id, hits`, "ALPHA").Scan(&lowerID, &hits)
	if err != nil || lowerID != gotID || hits != 7 {
		t.Fatalf("expression-target upsert: id=%s (want %s) hits=%d err=%v", lowerID, gotID, hits, err)
	}

	// UPDATE ... RETURNING whose WHERE tests the column it sets.
	rows, err = db.QueryContext(ctx, `UPDATE drv_items SET enabled = false WHERE enabled = true AND name <> $1 RETURNING name, enabled`, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		var name string
		var en bool
		rows.Scan(&name, &en)
		if en {
			t.Fatalf("%s still enabled in RETURNING", name)
		}
		n++
	}
	rows.Close()
	if n != 2 {
		t.Fatalf("UPDATE RETURNING returned %d rows, want 2", n)
	}

	// DELETE ... RETURNING returns what was deleted.
	rows, err = db.QueryContext(ctx, `DELETE FROM drv_items WHERE name = ANY($1) RETURNING name`, pq.StringArray{"beta", "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	n = 0
	for rows.Next() {
		n++
	}
	rows.Close()
	var left int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drv_items`).Scan(&left)
	if n != 2 || left != 1 {
		t.Fatalf("DELETE RETURNING: returned %d, left %d", n, left)
	}

	// AUTO_INCREMENT keys come back through LAST_INSERT_ID.
	var serial int64
	if err := db.QueryRowContext(ctx, `INSERT INTO drv_serial (v) VALUES ($1) RETURNING id`, "x").Scan(&serial); err != nil || serial < 1 {
		t.Fatalf("serial: %d %v", serial, err)
	}

	// Inside a caller's transaction the emulation joins it.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO drv_items (name) VALUES ($1) RETURNING id`, "in-tx").Scan(&id); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drv_items WHERE name = 'in-tx'`).Scan(&left)
	if left != 0 {
		t.Fatal("rolled-back RETURNING insert is visible")
	}
}

func TestMySQLExpressions(t *testing.T) {
	db := openMySQL(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO drv_items (name, tags, config, hits) VALUES
		('Alpha', '{"a.example","b.example"}', '{"url":"https://a","n":3}', 5),
		('beta', '{}', '{"url":null}', 0)`); err != nil {
		t.Fatal(err)
	}

	// Array round-trip through lib/pq's codec, and membership in a column.
	var tags pq.StringArray
	if err := db.QueryRowContext(ctx, `SELECT tags FROM drv_items WHERE $1 = ANY(tags)`, "b.example").Scan(&tags); err != nil || len(tags) != 2 {
		t.Fatalf("ANY(column): %v %v", tags, err)
	}
	var first string
	if err := db.QueryRowContext(ctx, `SELECT tags[1] FROM drv_items WHERE name = 'Alpha'`).Scan(&first); err != nil || first != "a.example" {
		t.Fatalf("subscript: %q %v", first, err)
	}
	var overlap int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drv_items WHERE tags && $1`, pq.StringArray{"x", "a.example"}).Scan(&overlap)
	if overlap != 1 {
		t.Fatalf("&&: %d", overlap)
	}

	// JSON operators, with JSON null as SQL NULL.
	var url sql.NullString
	db.QueryRowContext(ctx, `SELECT config->>'url' FROM drv_items WHERE name = 'beta'`).Scan(&url)
	if url.Valid {
		t.Fatalf("JSON null came back as %q", url.String)
	}
	var nval int
	if err := db.QueryRowContext(ctx, `SELECT (config->>'n')::int FROM drv_items WHERE name = 'Alpha'`).Scan(&nval); err != nil || nval != 3 {
		t.Fatalf("->> cast: %d %v", nval, err)
	}

	// ILIKE is case-insensitive; = stays case-sensitive.
	var c1, c2 int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drv_items WHERE name ILIKE $1`, "alp%").Scan(&c1)
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drv_items WHERE name = $1`, "alpha").Scan(&c2)
	if c1 != 1 || c2 != 0 {
		t.Fatalf("ILIKE=%d, case-sensitive equality=%d", c1, c2)
	}

	// LIKE honours backslash escapes as PostgreSQL does.
	var esc int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drv_items WHERE name LIKE $1`, `Alph\%`).Scan(&esc)
	if esc != 0 {
		t.Fatal(`\% must match a literal percent sign`)
	}

	// FILTER, intervals, date_trunc, EXTRACT, NULLS LAST, GREATEST.
	var total, enabledHits int
	var bucket time.Time
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE hits > 0), COALESCE(SUM(hits) FILTER (WHERE enabled), 0)::int,
		       date_trunc('hour', MAX(created_at))
		FROM drv_items WHERE created_at > now() - INTERVAL '1 day' AND created_at < now() + ($1 || ' minutes')::interval`, 5).
		Scan(&total, &enabledHits, &bucket)
	if err != nil || total != 1 || enabledHits != 5 || bucket.Minute() != 0 {
		t.Fatalf("aggregates: %d %d %v %v", total, enabledHits, bucket, err)
	}
	var age float64
	if err := db.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM (now() - created_at)) FROM drv_items LIMIT 1`).Scan(&age); err != nil || age < 0 || age > 60 {
		t.Fatalf("epoch difference: %v %v", age, err)
	}
	var g int
	db.QueryRowContext(ctx, `SELECT GREATEST(NULL, hits) FROM drv_items WHERE name = 'Alpha'`).Scan(&g)
	if g != 5 {
		t.Fatalf("GREATEST ignoring NULL: %d", g)
	}
	var top string
	db.QueryRowContext(ctx, `SELECT name FROM drv_items ORDER BY (config->>'n')::int DESC NULLS LAST LIMIT 1`).Scan(&top)
	if top != "Alpha" {
		t.Fatalf("NULLS LAST: %s", top)
	}

	// Literal '?' in the text forces a server-side prepare; it must still work.
	var q string
	if err := db.QueryRowContext(ctx, `SELECT 'what?' || $1`, "!").Scan(&q); err != nil || q != "what?!" {
		t.Fatalf("literal ?: %q %v", q, err)
	}

	// time.Time arguments are stored and compared as UTC.
	loc, _ := time.LoadLocation("Asia/Seoul")
	at := time.Date(2026, 1, 2, 9, 0, 0, 0, loc) // 00:00 UTC
	if _, err := db.ExecContext(ctx, `INSERT INTO drv_counters (bucket, host, n) VALUES ($1, $2, 1)
		ON CONFLICT (bucket, host) DO UPDATE SET n = drv_counters.n + EXCLUDED.n`, at, "h"); err != nil {
		t.Fatal(err)
	}
	db.ExecContext(ctx, `INSERT INTO drv_counters (bucket, host, n) VALUES ($1, $2, 1)
		ON CONFLICT (bucket, host) DO UPDATE SET n = drv_counters.n + EXCLUDED.n`, at, "h")
	var got time.Time
	var cnt int
	db.QueryRowContext(ctx, `SELECT bucket, n FROM drv_counters`).Scan(&got, &cnt)
	if !got.Equal(at) || cnt != 2 || got.Hour() != 0 {
		t.Fatalf("UTC round-trip: %v (n=%d), want %v", got, cnt, at.UTC())
	}

	// RowsAffected counts matched rows, so an idempotent update is not "not found".
	res, _ := db.ExecContext(ctx, `UPDATE drv_items SET hits = hits WHERE name = 'Alpha'`)
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("RowsAffected for an unchanged row: %d", n)
	}
	// GROUP BY bucket, ROLLUP(host): per-bucket subtotals, no grand total.
	rows, err := db.QueryContext(ctx, `
		SELECT date_trunc('day', created_at) AS day, name, count(*), GROUPING(name) = 1 AS is_total
		FROM drv_items GROUP BY date_trunc('day', created_at), ROLLUP(name)`)
	if err != nil {
		t.Fatal(err)
	}
	var detail, subtotal int
	for rows.Next() {
		var day sql.NullTime
		var name sql.NullString
		var n int
		var isTotal bool
		rows.Scan(&day, &name, &n, &isTotal)
		if !day.Valid {
			t.Fatal("grand-total row leaked from WITH ROLLUP")
		}
		if isTotal {
			subtotal++
		} else {
			detail++
		}
	}
	rows.Close()
	if detail != 2 || subtotal != 1 {
		t.Fatalf("rollup rows: %d detail, %d subtotal", detail, subtotal)
	}

	// SET expressions see the old row, as in PostgreSQL: hits is read before
	// it is reset.
	db.ExecContext(ctx, `UPDATE drv_items SET hits = 0, enabled = (hits > 3) WHERE name = 'Alpha'`)
	var en bool
	var h int
	db.QueryRowContext(ctx, `SELECT enabled, hits FROM drv_items WHERE name = 'Alpha'`).Scan(&en, &h)
	if !en || h != 0 {
		t.Fatalf("SET order: enabled=%v hits=%d, want true 0", en, h)
	}

	// Unique violations classify like PostgreSQL's.
	_, err = db.ExecContext(ctx, `INSERT INTO drv_items (name) VALUES ('Alpha')`)
	if !IsUniqueViolation(err) || ViolatedConstraint(err) != "uq_drv_items_name" {
		t.Fatalf("unique violation: %v (%q)", err, ViolatedConstraint(err))
	}
}
