package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"nginx-proxy-guard/internal/database/dialect"
)

// Engine-specific query shapes for the few statements the MySQL translator
// cannot rewrite mechanically. Each helper takes the PostgreSQL form as is, so
// PostgreSQL keeps running exactly the statement it always ran, and builds the
// MySQL form — sent untranslated via dialect.Raw — beside it.

// tablesExistQuery returns pgQuery on PostgreSQL. On MySQL, which has no
// to_regclass(), it returns a statement producing the same single boolean:
// whether every one of tables exists in the current database. The feature
// checks that call it (SSO, RBAC, notifications) exist because upgrade
// migrations warn and continue, so a feature whose tables are missing must
// switch off rather than error.
func tablesExistQuery(pgQuery string, tables ...string) string {
	if !dialect.IsMySQL() {
		return pgQuery
	}
	quoted := make([]string, len(tables))
	for i, t := range tables {
		quoted[i] = "'" + strings.ReplaceAll(t, "'", "''") + "'"
	}
	return dialect.Raw(fmt.Sprintf(
		"SELECT COUNT(*) = %d FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN (%s)",
		len(tables), strings.Join(quoted, ", ")))
}

// mysqlArrayJSON is the JSON-array view of a text[] column stored on MySQL as
// the lib/pq array literal ({"a","b"}): lib/pq quotes every element, so
// swapping the braces for brackets yields a valid JSON array that MEMBER OF
// and JSON_TABLE can read. An empty array ('{}') becomes '[]'.
func mysqlArrayJSON(column string) string {
	return fmt.Sprintf("CAST(CONCAT('[', SUBSTRING(%s, 2, CHAR_LENGTH(%s) - 2), ']') AS JSON)", column, column)
}

// queryFreshTableStats runs a single-row MySQL query over
// information_schema.TABLES with the statistics cache off. MySQL 8 caches
// TABLE_ROWS and DATA_LENGTH for information_schema_stats_expiry seconds — a
// day by default — so without this a size or row estimate could be a day old.
// The variable cannot be set with a SET_VAR hint, so the query runs on a pinned
// connection whose session setting is restored afterwards.
func queryFreshTableStats(ctx context.Context, db *sql.DB, query string, args []interface{}, dest ...interface{}) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, dialect.Raw("SET SESSION information_schema_stats_expiry = 0")); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), dialect.Raw("SET SESSION information_schema_stats_expiry = DEFAULT"))
	}()
	return conn.QueryRowContext(ctx, dialect.Raw(query), args...).Scan(dest...)
}

// mysqlInCIDR is PostgreSQL's `column <<= 'cidr'::inet` for MySQL, which
// keeps addresses as text and has no inet type. The range is resolved in Go
// into its first and last address and compared on INET6_ATON's binary form,
// with the address family pinned by length (4 or 16 bytes) as PostgreSQL pins
// it: an IPv4 range never matches an IPv6 address, IPv4-mapped ones included.
// The bounds are inlined as hex literals of parsed bytes, so the predicate
// carries no placeholders and fits a raw or a translated statement alike. ok
// is false when cidr does not parse.
func mysqlInCIDR(column, cidr string) (predicate string, ok bool) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return "", false
	}
	first := network.IP
	if v4 := first.To4(); v4 != nil {
		first = v4
	}
	mask := network.Mask
	if len(mask) != len(first) {
		mask = mask[len(mask)-len(first):]
	}
	last := make(net.IP, len(first))
	for i := range first {
		last[i] = first[i] | ^mask[i]
	}
	return fmt.Sprintf("(LENGTH(INET6_ATON(%s)) = %d AND INET6_ATON(%s) BETWEEN UNHEX('%s') AND UNHEX('%s'))",
		column, len(first), column, hex.EncodeToString(first), hex.EncodeToString(last)), true
}
