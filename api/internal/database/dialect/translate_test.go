package dialect

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
)

func TestTranslate(t *testing.T) {
	cases := []struct{ in, want string }{
		// placeholders, casts, functions
		{`SELECT id FROM users WHERE id = $1`, `SELECT id FROM users WHERE id = ?`},
		{`SELECT $2, $1, $2`, `SELECT ?, ?, ?`},
		{`SELECT COUNT(*)::int FROM t`, `SELECT CAST(COUNT(*) AS SIGNED) FROM t`},
		{`SELECT id::text FROM t`, `SELECT CAST(id AS CHAR) FROM t`},
		{`SELECT ROUND(x::numeric, 2) FROM t`, `SELECT ROUND(CAST(x AS DECIMAL(65, 10)), 2) FROM t`},
		{`SELECT $1::uuid, 'a'::text, NULL::text`, `SELECT ?, 'a', NULL`},
		{`SELECT '{}'::jsonb`, `SELECT CAST('{}' AS JSON)`},
		{`SELECT x::double precision FROM t`, `SELECT CAST(x AS DOUBLE) FROM t`},
		{`SELECT created_at::date FROM t`, `SELECT DATE(created_at) FROM t`},
		{`SELECT 'blocked'::public.block_reason`, `SELECT 'blocked'`},
		{`SELECT now(), NOW(), CURRENT_TIMESTAMP`, `SELECT NOW(6), NOW(6), CURRENT_TIMESTAMP(6)`},
		{`INSERT INTO t (id) VALUES (gen_random_uuid())`, `INSERT INTO "t" (id) VALUES (UUID())`},
		{`SELECT COALESCE(SUM(a), 0) FROM public.t`, `SELECT COALESCE(SUM(a), 0) FROM t`},
		// intervals
		{`SELECT * FROM t WHERE created_at < NOW() - INTERVAL '7 days'`, `SELECT * FROM t WHERE created_at < NOW(6) - INTERVAL 7 DAY`},
		{`DELETE FROM t WHERE created_at < NOW() - ($1 || ' days')::interval`, `DELETE FROM "t" WHERE created_at < NOW(6) - INTERVAL (?) DAY`},
		{`SELECT NOW() - INTERVAL '1 hour 30 minutes'`, `SELECT NOW(6) - INTERVAL 5400 SECOND`},
		{`SELECT NOW() - '5 minutes'::interval`, `SELECT NOW(6) - INTERVAL 5 MINUTE`},
		{`SELECT NOW() - make_interval(secs => $1)`, `SELECT NOW(6) - INTERVAL (?) SECOND`},
		{`SELECT NOW() - INTERVAL '1 day' * $1`, `SELECT NOW(6) - INTERVAL (?) DAY`},
		{`UPDATE o SET next_attempt_at = now() + $2::interval WHERE id = $1`, `UPDATE "o" SET next_attempt_at = NOW(6) + ? WHERE id = ?`},
		{`SELECT id FROM certificates WHERE domain_names && $1`, `SELECT id FROM certificates WHERE JSON_OVERLAPS(CAST(CONCAT('[', SUBSTRING(domain_names, 2, CHAR_LENGTH(domain_names) - 2), ']') AS JSON), CAST(? AS JSON))`},
		// date_trunc / extract
		{`SELECT date_trunc('hour', created_at) FROM t`, `SELECT CAST(DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00') AS DATETIME(6)) FROM t`},
		{`SELECT EXTRACT(EPOCH FROM created_at)::bigint FROM t`, `SELECT CAST(UNIX_TIMESTAMP(created_at) AS SIGNED) FROM t`},
		{`SELECT EXTRACT(EPOCH FROM (NOW() - created_at)) FROM t`, `SELECT UNIX_TIMESTAMP(NOW(6)) - UNIX_TIMESTAMP(created_at) FROM t`},
		{`SELECT EXTRACT(HOUR FROM created_at) FROM t`, `SELECT EXTRACT(HOUR FROM created_at) FROM t`},
		{`SELECT to_timestamp($1)`, `SELECT FROM_UNIXTIME(?)`},
		// FILTER
		{`SELECT COUNT(*) FILTER (WHERE status >= 400) FROM t`, `SELECT COUNT(CASE WHEN status >= 400 THEN 1 END) FROM t`},
		{`SELECT SUM(bytes) FILTER (WHERE ok) FROM t`, `SELECT SUM(CASE WHEN ok THEN bytes END) FROM t`},
		{`SELECT COUNT(DISTINCT ip) FILTER (WHERE blocked) FROM t`, `SELECT COUNT(DISTINCT CASE WHEN blocked THEN ip END) FROM t`},
		// LIKE / ILIKE / regex
		{`SELECT * FROM t WHERE name ILIKE $1`, `SELECT * FROM t WHERE name LIKE (?) COLLATE utf8mb4_0900_as_ci ESCAPE '\'`},
		{`SELECT * FROM t WHERE name ILIKE '%' || $1 || '%' AND x = 1`, `SELECT * FROM t WHERE name LIKE ('%' || ? || '%') COLLATE utf8mb4_0900_as_ci ESCAPE '\' AND x = 1`},
		{`SELECT * FROM t WHERE name NOT LIKE $1`, `SELECT * FROM t WHERE name NOT LIKE ? ESCAPE '\'`},
		{`SELECT * FROM t WHERE host ~ '^a'`, `SELECT * FROM t WHERE REGEXP_LIKE(host, '^a', 'c')`},
		{`SELECT * FROM t WHERE host !~* $1`, `SELECT * FROM t WHERE NOT REGEXP_LIKE(host, ?, 'i')`},
		// distinct from, nulls, time zone
		{`SELECT * FROM t WHERE a IS DISTINCT FROM $1`, `SELECT * FROM t WHERE NOT (a <=> ?)`},
		{`SELECT * FROM t WHERE a IS NOT DISTINCT FROM b`, `SELECT * FROM t WHERE (a <=> b)`},
		{`SELECT * FROM t ORDER BY a DESC NULLS LAST, b ASC`, `SELECT * FROM t ORDER BY a DESC, b ASC`},
		{`SELECT * FROM t ORDER BY a NULLS LAST`, `SELECT * FROM t ORDER BY (a) IS NULL, a`},
		{`SELECT * FROM t ORDER BY a DESC NULLS FIRST LIMIT 5`, `SELECT * FROM t ORDER BY (a) IS NULL DESC, a DESC LIMIT 5`},
		{`SELECT created_at AT TIME ZONE 'UTC' FROM t`, `SELECT created_at FROM t`},
		{`SELECT * FROM t OFFSET 10`, `SELECT * FROM t LIMIT 18446744073709551615 OFFSET 10`},
		// arrays
		{`SELECT * FROM t WHERE id = ANY($1)`, `SELECT * FROM t WHERE id IN (?...)`},
		{`SELECT * FROM t WHERE id = ANY($1::uuid[])`, `SELECT * FROM t WHERE id IN (?...)`},
		{`SELECT * FROM t WHERE id <> ALL($1)`, `SELECT * FROM t WHERE id NOT IN (?...)`},
		{`SELECT * FROM t WHERE status = ANY(ARRAY['a', 'b'])`, `SELECT * FROM t WHERE status IN ('a', 'b')`},
		{`SELECT * FROM t WHERE x = ANY('{a,b}'::text[])`, `SELECT * FROM t WHERE x IN ('a', 'b')`},
		{`SELECT * FROM t WHERE x = ANY(SELECT y FROM u)`, `SELECT * FROM t WHERE x = ANY (SELECT y FROM u)`},
		{`SELECT * FROM proxy_hosts WHERE $1 = ANY(domain_names)`, `SELECT * FROM proxy_hosts WHERE (? MEMBER OF (CAST(CONCAT('[', SUBSTRING(domain_names, 2, CHAR_LENGTH(domain_names) - 2), ']') AS JSON)))`},
		{`SELECT domain_names[1] FROM t`, `SELECT JSON_UNQUOTE(JSON_EXTRACT(CAST(CONCAT('[', SUBSTRING(domain_names, 2, CHAR_LENGTH(domain_names) - 2), ']') AS JSON), '$[0]')) FROM t`},
		{`UPDATE t SET tags = ARRAY['a', 'b'], other = '{}'::text[]`, `UPDATE "t" SET tags = '{"a","b"}', other = '{}'`},
		{`SELECT array_length(cidrs, 1) FROM t`, `SELECT NULLIF(JSON_LENGTH(CAST(CONCAT('[', SUBSTRING(cidrs, 2, CHAR_LENGTH(cidrs) - 2), ']') AS JSON)), 0) FROM t`},
		{`SELECT string_agg(name, ', ' ORDER BY name) FROM t`, `SELECT GROUP_CONCAT(name ORDER BY name SEPARATOR ', ') FROM t`},
		{`SELECT array_agg(DISTINCT name) FROM t`, `SELECT CONCAT('{', GROUP_CONCAT(DISTINCT JSON_QUOTE(CAST(name AS CHAR)) SEPARATOR ','), '}') FROM t`},
		// json
		{`SELECT config->>'url' FROM t`, `SELECT JSON_UNQUOTE(NULLIF(JSON_EXTRACT(config, '$."url"'), CAST('null' AS JSON))) FROM t`},
		{`SELECT data->'a'->>'b' FROM t`, `SELECT JSON_UNQUOTE(NULLIF(JSON_EXTRACT(JSON_EXTRACT(data, '$."a"'), '$."b"'), CAST('null' AS JSON))) FROM t`},
		{`SELECT jsonb_build_object('a', 1)`, `SELECT JSON_OBJECT('a', 1)`},
		// misc functions
		{`SELECT GREATEST(a, b) FROM t`, `SELECT GREATEST(COALESCE(a, b), COALESCE(b, a)) FROM t`},
		{`SELECT host(client_ip) FROM t`, `SELECT (client_ip) FROM t`},
		{`SELECT split_part(x, ',', 1) FROM t`, `SELECT SUBSTRING_INDEX(x, ',', 1) FROM t`},
		{`SELECT length(x) FROM t`, `SELECT CHAR_LENGTH(x) FROM t`},
		{`SELECT bool_or(a) FROM t`, `SELECT MAX(a) FROM t`},
		{`SELECT key, value FROM settings WHERE key = $1`, `SELECT "key", value FROM settings WHERE "key" = ?`},
		{`SELECT regexp_replace(s, '(token=)[^&]+', '\1x', 'g')`, `SELECT REGEXP_REPLACE(s, '(token=)[^&]+', '$1x', 1, 0, 'c')`},
		// ON CONFLICT
		{`INSERT INTO t (a, b) VALUES ($1, $2) ON CONFLICT (a) DO NOTHING`, `INSERT IGNORE INTO "t" (a, b) VALUES (?, ?)`},
		{`INSERT INTO t (a, b) VALUES ($1, $2) ON CONFLICT DO NOTHING`, `INSERT IGNORE INTO "t" (a, b) VALUES (?, ?)`},
		{`INSERT INTO t (a, b) VALUES ($1, $2) ON CONFLICT (a) DO UPDATE SET b = EXCLUDED.b, n = t.n + 1`, `INSERT INTO "t" (a, b) VALUES (?, ?) ON DUPLICATE KEY UPDATE b = VALUES(b), n = t.n + 1`},
		{`INSERT INTO t AS c (a) VALUES ($1) ON CONFLICT (a) DO UPDATE SET n = c.n + 1`, `INSERT INTO "t" (a) VALUES (?) ON DUPLICATE KEY UPDATE n = "t".n + 1`},
		{`INSERT INTO global_fail2ban DEFAULT VALUES ON CONFLICT DO NOTHING`, `INSERT IGNORE INTO "global_fail2ban" () VALUES ()`},
		// UPDATE ... FROM / DELETE ... USING
		{`UPDATE upstreams u SET name = 'x' FROM proxy_hosts h WHERE h.id = u.proxy_host_id`, `UPDATE "upstreams" AS "u", proxy_hosts h SET "u".name = 'x' WHERE h.id = u.proxy_host_id`},
		{`DELETE FROM banned_ips a USING banned_ips b WHERE a.ip = b.ip`, `DELETE "a" FROM "banned_ips" AS "a", banned_ips b WHERE a.ip = b.ip`},
		// CTE
		{`WITH x AS MATERIALIZED (SELECT 1) SELECT * FROM x`, `WITH x AS (SELECT 1) SELECT * FROM x`},
		{`WITH s AS (SELECT a FROM u) INSERT INTO t (a) SELECT a FROM s`, `INSERT INTO "t" (a) WITH s AS (SELECT a FROM u) SELECT a FROM s`},
		{`SELECT * FROM t FOR UPDATE SKIP LOCKED`, `SELECT * FROM t FOR UPDATE SKIP LOCKED`},
		{`SELECT * FROM t FOR NO KEY UPDATE`, `SELECT * FROM t FOR UPDATE`},
		{`SELECT 'it''s', E'a\nb', 'C:\path'`, "SELECT 'it''s', 'a\nb', 'C:\\path'"},
		{`SELECT "timestamp" FROM logs_partitioned`, `SELECT "timestamp" FROM logs_partitioned`},
		{"SELECT count (*) FROM t -- comment\n", `SELECT count(*) FROM t`},
		{`TRUNCATE TABLE t RESTART IDENTITY CASCADE`, `TRUNCATE TABLE t`},
		{`SELECT h, count(*), GROUPING(h) = 1 FROM t GROUP BY ROLLUP(h)`, `SELECT h, count(*), GROUPING(h) = 1 FROM t GROUP BY h WITH ROLLUP`},
		{`SELECT b, h, count(*) FROM t GROUP BY b, ROLLUP(h) ORDER BY b`, `SELECT b, h, count(*) FROM t GROUP BY b, h WITH ROLLUP HAVING GROUPING(b) = 0 ORDER BY b`},
		{`SELECT date_trunc('hour', c) AS bucket, h FROM t GROUP BY date_trunc('hour', c), ROLLUP(h)`, `SELECT CAST(DATE_FORMAT(c, '%Y-%m-%d %H:00:00') AS DATETIME(6)) AS bucket, h FROM t GROUP BY "bucket", h WITH ROLLUP HAVING GROUPING("bucket") = 0`},
		{`INSERT INTO r (a, o) VALUES ($1, COALESCE((SELECT MAX(o) + 1 FROM r WHERE a = $1), 100))`, `INSERT INTO "r" (a, o) VALUES (?, COALESCE((SELECT * FROM (SELECT MAX(o) + 1 FROM r WHERE a = ?) AS npg_derived), 100))`},
		{`DELETE FROM logs WHERE id IN (SELECT id FROM logs WHERE ts < $1 LIMIT $2)`, `DELETE FROM "logs" WHERE id IN (SELECT * FROM (SELECT id FROM logs WHERE ts < ? LIMIT ?) AS npg_derived)`},
		{`UPDATE c SET consecutive_failures = 0, enabled = CASE WHEN consecutive_failures >= 10 THEN true ELSE enabled END WHERE id = $1`, `UPDATE "c" SET enabled = CASE WHEN consecutive_failures >= 10 THEN true ELSE enabled END, consecutive_failures = 0 WHERE id = ?`},
		{`UPDATE t SET a = 1, b = b + 1, c = t.a WHERE id = $1`, `UPDATE "t" SET b = b + 1, c = t.a, a = 1 WHERE id = ?`},
		{`UPDATE t SET n = n + 1, m = (SELECT max(n) FROM u)`, `UPDATE "t" SET n = n + 1, m = (SELECT max(n) FROM u)`},
		{`INSERT INTO t (k, a) VALUES ($1, $2) ON CONFLICT (k) DO UPDATE SET a = EXCLUDED.a, b = t.a`, `INSERT INTO "t" (k, a) VALUES (?, ?) ON DUPLICATE KEY UPDATE b = t.a, a = VALUES(a)`},
		{`INSERT INTO g (id) SELECT gen_random_uuid() WHERE NOT EXISTS (SELECT 1 FROM g LIMIT 1)`, `INSERT INTO "g" (id) SELECT UUID() WHERE NOT EXISTS(SELECT 1 FROM g LIMIT 1)`},
		{`UPDATE t SET n = 1 WHERE EXISTS (SELECT 1 FROM u WHERE u.id = t.id)`, `UPDATE "t" SET n = 1 WHERE EXISTS(SELECT 1 FROM u WHERE u.id = t.id)`},
	}
	for _, c := range cases {
		got, err := TranslateSQL(c.in)
		if err != nil {
			t.Errorf("%s\n  error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s\n   got: %s\n  want: %s", c.in, got, c.want)
		}
	}
}

func TestTranslateRejects(t *testing.T) {
	for _, q := range []string{
		`SELECT unnest(tags) FROM t`,
		`SELECT * FROM generate_series(1, 3)`,
		`SELECT DISTINCT ON (a) a, b FROM t`,
		`SELECT pg_total_relation_size('t')`,
		`DO $$ BEGIN END $$`,
		`INSERT INTO t (a) VALUES (1) ON CONFLICT (a) DO UPDATE SET b = 1 WHERE t.b < 1`,
		`SELECT x::regclass`,
		`SELECT * FROM timescaledb_information.chunks`,
		`SELECT created_at AT TIME ZONE 'Asia/Seoul' FROM t`,
		`SELECT 1; SELECT 2`,
		`UPDATE t SET n = 1 WHERE EXISTS (SELECT 1 FROM t x WHERE x.id = t.id)`,
		`UPDATE t SET a = b, b = a`,
		`SELECT a, b, count(*) FROM t GROUP BY CUBE(a, b)`,
	} {
		_, err := TranslateSQL(q)
		if err == nil {
			t.Errorf("%s: translated, want ErrUntranslatable", q)
			continue
		}
		if !errors.Is(err, ErrUntranslatable) {
			t.Errorf("%s: %v, want ErrUntranslatable", q, err)
		}
	}
}

func TestReturningIsPlanned(t *testing.T) {
	p, err := Translate(`INSERT INTO users (email, username) VALUES ($1, $2) RETURNING id, created_at`)
	if err != nil {
		t.Fatal(err)
	}
	if p.ret == nil || p.ret.kind != stmtInsert || len(p.ret.rows) != 1 || len(p.ret.columns) != 2 {
		t.Fatalf("unexpected plan %+v", p.ret)
	}
	if got := p.sql.text(); strings.Contains(got, "RETURNING") {
		t.Fatalf("Exec form still has RETURNING: %s", got)
	}
	p, err = Translate(`UPDATE certificates SET status = 'issued' WHERE status = 'renewing' RETURNING id`)
	if err != nil || p.ret == nil || p.ret.kind != stmtUpdate {
		t.Fatalf("update plan: %v %+v", err, p)
	}
	p, err = Translate(`DELETE FROM banned_ips WHERE id IN ($1, $2) RETURNING proxy_host_id`)
	if err != nil || p.ret == nil || p.ret.kind != stmtDelete {
		t.Fatalf("delete plan: %v %+v", err, p)
	}
}

func TestParsePGArrayLiteral(t *testing.T) {
	got, ok := parsePGArrayLiteral(`{"a,b",c,"d\"e"}`)
	if !ok || len(got) != 3 || got[0] != "a,b" || got[1] != "c" || got[2] != `d"e` {
		t.Fatalf("got %q %v", got, ok)
	}
	if got, ok := parsePGArrayLiteral(`{}`); !ok || len(got) != 0 {
		t.Fatalf("empty: %q %v", got, ok)
	}
}

func TestMySQLDSN(t *testing.T) {
	dsn, err := MySQLDSN("mysql://npg:se%40cret@db/nginx_proxy_guard?sslmode=disable&application_name=x", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"npg:se@cret@tcp(db:3306)/nginx_proxy_guard?", "parseTime=true", "loc=UTC", "clientFoundRows=true", "tls=false", "sql_mode=", "NO_BACKSLASH_ESCAPES", "transaction_isolation="} {
		if !strings.Contains(dsn, want) && !strings.Contains(strings.ReplaceAll(dsn, "%27", "'"), want) {
			t.Errorf("DSN %q lacks %q", dsn, want)
		}
	}
	if strings.Contains(dsn, "sslmode") || strings.Contains(dsn, "application_name") {
		t.Errorf("libpq parameters leaked into %q", dsn)
	}
	if Detect("mysql://u:p@h/db") != MySQL || Detect("postgres://u@h/db") != Postgres || Detect("u:p@tcp(h:3306)/db") != MySQL {
		t.Error("Detect")
	}
}

func TestRenderBindsArgumentsInOrder(t *testing.T) {
	p, err := Translate(`SELECT * FROM t WHERE a = $2 AND b = ANY($1) AND c < now() - $3::interval AND d && $4`)
	if err != nil {
		t.Fatal(err)
	}
	args := []driver.NamedValue{
		{Ordinal: 1, Value: pq.StringArray{"x", "y"}},
		{Ordinal: 2, Value: int64(7)},
		{Ordinal: 3, Value: "300 seconds"},
		{Ordinal: 4, Value: pq.StringArray{"p"}},
	}
	q, bound, err := render(p.sql, args, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT * FROM t WHERE a = ? AND b IN (?, ?) AND c < NOW(6) - INTERVAL 300 SECOND AND JSON_OVERLAPS(CAST(CONCAT('[', SUBSTRING(d, 2, CHAR_LENGTH(d) - 2), ']') AS JSON), CAST(? AS JSON))`
	if q != want {
		t.Fatalf("got  %s\nwant %s", q, want)
	}
	var vals []any
	for _, b := range bound {
		vals = append(vals, b.Value)
	}
	if fmt.Sprint(vals) != `[7 x y ["p"]]` {
		t.Fatalf("bound %v", vals)
	}
	// An empty array matches nothing (IN) / everything (NOT IN).
	q, _, err = render(p.sql, []driver.NamedValue{{Value: pq.StringArray{}}, {Value: int64(1)}, {Value: "1 day"}, {Value: pq.StringArray{}}}, nil)
	if err != nil || !strings.Contains(q, "b IN (SELECT NULL FROM DUAL WHERE FALSE)") {
		t.Fatalf("empty array: %s %v", q, err)
	}
	if _, _, err := render(p.sql, []driver.NamedValue{{Value: pq.StringArray{}}, {Value: int64(1)}, {Value: "1; DROP TABLE t"}, {Value: nil}}, nil); err == nil {
		t.Fatal("a malformed interval string must not be inlined")
	}
}
