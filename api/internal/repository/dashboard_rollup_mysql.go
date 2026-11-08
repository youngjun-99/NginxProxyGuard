package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nginx-proxy-guard/internal/database/dialect"
)

// The hourly rollup on MySQL. It computes exactly what the PostgreSQL
// statements in dashboard_rollup.go compute; the shapes differ only where
// MySQL has no counterpart:
//
//   - no generate_series. The PostgreSQL statement walks every hour of the
//     window and then keeps only the hours that have requests, WAF blocks or
//     an existing row. Here that set of hours is built directly as the union
//     of those three sources, which is the same set without the walk.
//   - no FILTER: count(*) FILTER (WHERE c) is COUNT(CASE WHEN c THEN 1 END).
//   - no WITH before INSERT: MySQL puts the CTEs inside the INSERT's SELECT.
//   - no ON CONFLICT with an arbiter: ON DUPLICATE KEY UPDATE fires on the
//     unique key the row collides with — (proxy_host_id, hour_bucket) for a
//     host row, the global-row key for a NULL host — which are the two
//     arbiters the PostgreSQL statements name.
//
// Every bound value is computed in Go, so the hour boundaries are the same
// UTC hours PostgreSQL's date_trunc('hour', x, 'UTC') gives.

// mysqlHourOf truncates a DATETIME(6) column, stored in UTC, to its hour.
func mysqlHourOf(col string) string {
	return "CAST(DATE_FORMAT(" + col + ", '%Y-%m-%d %H:00:00') AS DATETIME(6))"
}

// mysqlAveragedRequestFilter is averagedRequestFilter: IS DISTINCT FROM is
// NOT (a <=> b) in MySQL.
const mysqlAveragedRequestFilter = `NOT (status_code <=> 101) AND request_time <= 60`

// mysqlRollupHostFilter is the host and URI filter both rollups share.
func mysqlRollupHostFilter(alias string) string {
	p := alias
	if p != "" {
		p += "."
	}
	return strings.NewReplacer("{p}", p).Replace(`{p}host IS NOT NULL
      AND {p}host NOT IN ('localhost', 'nginx', '127.0.0.1', '', '_', '0.0.0.0')
      AND {p}host NOT LIKE 'localhost:%'
      AND ({p}request_uri NOT IN ('/health', '/nginx_status') OR {p}request_uri IS NULL)
      AND ({p}request_uri NOT LIKE '/.well-known/%' OR {p}request_uri IS NULL)
      AND ({p}request_uri NOT LIKE '/__npg_canary%' OR {p}request_uri IS NULL)`)
}

// mysqlHourlyRollupSQL is hourlyRollupRecomputeSQL for MySQL. Arguments: the
// window start truncated to its hour, and the window end, each repeated in
// the order the placeholders appear (see recomputeHourlyRollupMySQL).
var mysqlHourlyRollupSQL = `
INSERT INTO dashboard_stats_hourly (
    proxy_host_id, hour_bucket, total_requests,
    status_2xx, status_3xx, status_4xx, status_5xx,
    avg_response_time, timed_requests, bytes_sent,
    waf_blocked, rate_limited, bot_blocked
)
WITH agg AS (
    SELECT ` + mysqlHourOf("created_at") + ` AS hour_bucket,
           COUNT(*) AS total,
           COUNT(CASE WHEN status_code BETWEEN 200 AND 299 THEN 1 END) AS s2,
           COUNT(CASE WHEN status_code BETWEEN 300 AND 399 THEN 1 END) AS s3,
           COUNT(CASE WHEN status_code BETWEEN 400 AND 499 THEN 1 END) AS s4,
           COUNT(CASE WHEN status_code >= 500 THEN 1 END) AS s5,
           AVG(CASE WHEN ` + mysqlAveragedRequestFilter + ` THEN request_time END) * 1000 AS avg_ms,
           COUNT(CASE WHEN ` + mysqlAveragedRequestFilter + ` THEN request_time END) AS timed,
           COALESCE(SUM(body_bytes_sent), 0) AS bytes,
           COUNT(CASE WHEN block_reason = 'rate_limit' THEN 1 END) AS rl,
           COUNT(CASE WHEN block_reason = 'bot_filter' THEN 1 END) AS bot
    FROM logs_partitioned
    WHERE log_type = 'access'
      AND created_at >= ? AND created_at < ?
      AND ` + mysqlRollupHostFilter("") + `
    GROUP BY 1
), waf AS (
    SELECT ` + mysqlHourOf("created_at") + ` AS hour_bucket, COUNT(*) AS waf
    FROM logs_partitioned
    WHERE log_type = 'modsec' AND block_reason = 'waf'
      AND created_at >= ? AND created_at < ?
      AND ` + mysqlRollupHostFilter("") + `
    GROUP BY 1
), hours AS (
    SELECT hour_bucket FROM agg
    UNION
    SELECT hour_bucket FROM waf
    UNION
    SELECT hour_bucket FROM dashboard_stats_hourly
     WHERE proxy_host_id IS NULL AND hour_bucket >= ? AND hour_bucket < ?
)
SELECT NULL, h.hour_bucket, COALESCE(a.total, 0),
       COALESCE(a.s2, 0), COALESCE(a.s3, 0), COALESCE(a.s4, 0), COALESCE(a.s5, 0),
       COALESCE(a.avg_ms, 0), COALESCE(a.timed, 0), COALESCE(a.bytes, 0),
       COALESCE(w.waf, 0), COALESCE(a.rl, 0), COALESCE(a.bot, 0)
FROM hours h
LEFT JOIN agg a ON a.hour_bucket = h.hour_bucket
LEFT JOIN waf w ON w.hour_bucket = h.hour_bucket
ON DUPLICATE KEY UPDATE
    total_requests    = VALUES(total_requests),
    status_2xx        = VALUES(status_2xx),
    status_3xx        = VALUES(status_3xx),
    status_4xx        = VALUES(status_4xx),
    status_5xx        = VALUES(status_5xx),
    avg_response_time = VALUES(avg_response_time),
    timed_requests    = VALUES(timed_requests),
    bytes_sent        = VALUES(bytes_sent),
    waf_blocked       = VALUES(waf_blocked),
    rate_limited      = VALUES(rate_limited),
    bot_blocked       = VALUES(bot_blocked)`

// mysqlHourlyHostRollupSQL is hourlyHostRollupSQL for MySQL. Arguments: the
// truncated window start and the window end.
var mysqlHourlyHostRollupSQL = `
INSERT INTO dashboard_stats_hourly (
    proxy_host_id, hour_bucket, total_requests, status_2xx, status_3xx,
    status_4xx, status_5xx, avg_response_time, timed_requests, bytes_sent,
    waf_blocked, rate_limited, bot_blocked
)
SELECT l.proxy_host_id, ` + mysqlHourOf("l.created_at") + ` AS hour_bucket,
       COUNT(*),
       COUNT(CASE WHEN l.status_code BETWEEN 200 AND 299 THEN 1 END),
       COUNT(CASE WHEN l.status_code BETWEEN 300 AND 399 THEN 1 END),
       COUNT(CASE WHEN l.status_code BETWEEN 400 AND 499 THEN 1 END),
       COUNT(CASE WHEN l.status_code >= 500 THEN 1 END),
       COALESCE(AVG(CASE WHEN ` + mysqlAveragedRequestFilter + ` THEN l.request_time END) * 1000, 0),
       COUNT(CASE WHEN ` + mysqlAveragedRequestFilter + ` THEN l.request_time END),
       COALESCE(SUM(l.body_bytes_sent), 0),
       0,
       COUNT(CASE WHEN l.block_reason = 'rate_limit' THEN 1 END),
       COUNT(CASE WHEN l.block_reason = 'bot_filter' THEN 1 END)
FROM logs_partitioned l
JOIN proxy_hosts p ON p.id = l.proxy_host_id
WHERE l.log_type = 'access'
  AND l.created_at >= ? AND l.created_at < ?
  AND ` + mysqlRollupHostFilter("l") + `
GROUP BY l.proxy_host_id, 2
ON DUPLICATE KEY UPDATE
    total_requests = VALUES(total_requests),
    status_2xx = VALUES(status_2xx),
    status_3xx = VALUES(status_3xx),
    status_4xx = VALUES(status_4xx),
    status_5xx = VALUES(status_5xx),
    avg_response_time = VALUES(avg_response_time),
    timed_requests = VALUES(timed_requests),
    bytes_sent = VALUES(bytes_sent),
    waf_blocked = VALUES(waf_blocked),
    rate_limited = VALUES(rate_limited),
    bot_blocked = VALUES(bot_blocked)`

// mysqlHourlyHostRollupCleanupSQL is hourlyHostRollupCleanupSQL for MySQL.
// Arguments: start, end (the active pairs), start, end (the rows to check).
var mysqlHourlyHostRollupCleanupSQL = `
WITH active AS (
    SELECT DISTINCT proxy_host_id, ` + mysqlHourOf("created_at") + ` AS hour_bucket
    FROM logs_partitioned
    WHERE created_at >= ? AND created_at < ?
      AND proxy_host_id IS NOT NULL
      AND log_type = 'access'
      AND ` + mysqlRollupHostFilter("") + `
)
DELETE FROM dashboard_stats_hourly
WHERE proxy_host_id IS NOT NULL
  AND hour_bucket >= ? AND hour_bucket < ?
  AND NOT EXISTS (
    SELECT 1 FROM active a
    WHERE a.proxy_host_id = dashboard_stats_hourly.proxy_host_id
      AND a.hour_bucket = dashboard_stats_hourly.hour_bucket
  )`

// recomputeHourlyRollupMySQL is RecomputeHourlyRollup on MySQL.
func (r *DashboardRepository) recomputeHourlyRollupMySQL(ctx context.Context, from, to time.Time) (int64, error) {
	start := from.UTC().Truncate(time.Hour)
	end := to.UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, dialect.Raw(mysqlHourlyRollupSQL), start, end, start, end, start, end)
	if err != nil {
		return 0, fmt.Errorf("recompute hourly dashboard stats: %w", err)
	}
	if _, err := tx.ExecContext(ctx, dialect.Raw(mysqlHourlyHostRollupSQL), start, end); err != nil {
		return 0, fmt.Errorf("recompute hourly host stats: %w", err)
	}
	if _, err := tx.ExecContext(ctx, dialect.Raw(mysqlHourlyHostRollupCleanupSQL), start, end, start, end); err != nil {
		return 0, fmt.Errorf("clean stale hourly host stats: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
