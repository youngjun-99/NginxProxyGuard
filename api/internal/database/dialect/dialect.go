// Package dialect is the one place that knows which SQL engine NPG is talking
// to, and how to talk to MySQL while the rest of the code base speaks
// PostgreSQL.
//
// Two engines are supported: PostgreSQL/TimescaleDB (the default, and the
// engine the schema is designed in) and MySQL 8.0.17+ (8.4 LTS recommended).
//
// The repositories hold well over a hundred files of PostgreSQL: $N
// placeholders, ::casts, ON CONFLICT, RETURNING, FILTER (WHERE ...), ILIKE,
// INTERVAL '7 days', text[] columns. Rather than fork every query, MySQL is
// served one layer down: a database/sql driver (driver.go) rewrites each
// statement on its way to the server (translate.go) and emulates the parts
// MySQL has no syntax for, RETURNING above all (returning.go). Repositories
// keep a single PostgreSQL query and stay unaware of the engine; only the
// handful of constructs with no mechanical equivalent (TimescaleDB catalog
// queries, COPY, advisory locks) branch on IsMySQL().
package dialect

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
)

// Kind identifies a supported database engine.
type Kind string

const (
	Postgres Kind = "postgres"
	MySQL    Kind = "mysql"
)

func (k Kind) String() string { return string(k) }

// Detect picks the engine from DATABASE_URL. Anything it does not recognise is
// PostgreSQL, which keeps every existing install exactly as it was.
func Detect(databaseURL string) Kind {
	scheme, _, hasScheme := strings.Cut(databaseURL, "://")
	if !hasScheme {
		// go-sql-driver's native DSN has no scheme: user:pass@tcp(host:3306)/db.
		if strings.Contains(databaseURL, "@tcp(") || strings.Contains(databaseURL, "@unix(") {
			return MySQL
		}
		return Postgres
	}
	if strings.EqualFold(scheme, "mysql") {
		return MySQL
	}
	return Postgres
}

// ---------------------------------------------------------------------------
// Process-wide engine
// ---------------------------------------------------------------------------

// active is the engine of this process' database. NPG opens exactly one
// database, so the value really is process-wide; database.New publishes it
// before any repository is built. Repositories receive a bare *sql.DB, so the
// few queries that need an engine-specific form read it from here.
var active atomic.Value

// SetActive publishes the engine of the process' database connection.
func SetActive(k Kind) { active.Store(k) }

// Active returns the published engine; PostgreSQL until one is published.
func Active() Kind {
	if k, ok := active.Load().(Kind); ok {
		return k
	}
	return Postgres
}

// IsMySQL reports whether the process' database is MySQL.
func IsMySQL() bool { return Active() == MySQL }

// ---------------------------------------------------------------------------
// Untranslated statements
// ---------------------------------------------------------------------------

// RawPrefix marks a statement that is already written for MySQL. The
// translating driver strips the marker and sends the rest verbatim. The schema
// sync uses it for its DDL, which is MySQL to begin with; it is a plain SQL
// comment, so a statement carrying it is also valid on an untranslated
// connection.
const RawPrefix = "/*npg:raw*/"

// Raw marks query as MySQL that the driver must not translate.
func Raw(query string) string { return RawPrefix + query }

// ---------------------------------------------------------------------------
// DSN
// ---------------------------------------------------------------------------

// sessionSQLMode makes a MySQL session read PostgreSQL-shaped SQL the way
// PostgreSQL would:
//
//   - ANSI_QUOTES: "double quotes" delimit identifiers, not strings, so a
//     quoted reserved column such as "timestamp" parses unchanged.
//   - PIPES_AS_CONCAT: || concatenates instead of meaning OR.
//   - NO_BACKSLASH_ESCAPES: a backslash inside '...' is an ordinary character,
//     as with PostgreSQL's standard_conforming_strings. go-sql-driver reads the
//     server status flag this sets and escapes interpolated values to match.
//   - STRICT_ALL_TABLES: an over-long or out-of-range value is an error, not a
//     silent truncation.
//   - NO_ENGINE_SUBSTITUTION: a missing storage engine fails loudly.
//
// ONLY_FULL_GROUP_BY is deliberately absent: a few dashboard aggregates rely
// on PostgreSQL's functional-dependency rules, which MySQL's check is stricter
// about in some shapes.
const sessionSQLMode = "ANSI_QUOTES,PIPES_AS_CONCAT,NO_BACKSLASH_ESCAPES,STRICT_ALL_TABLES,NO_ENGINE_SUBSTITUTION"

// forcedParams are not preferences: each closes a behavioural gap with
// PostgreSQL, so they override whatever DATABASE_URL says.
var forcedParams = map[string]string{
	// DATETIME scans into time.Time, read and written as UTC. The PostgreSQL
	// columns are timestamptz and the application normalises to UTC; MySQL's
	// DATETIME has no zone, so the driver must pin one.
	"parseTime": "true",
	"loc":       "UTC",

	// UPDATE reports matched rows, as PostgreSQL does, not changed rows.
	// Repositories read RowsAffected()==0 as "not found"; without this an
	// idempotent update that writes the same values would be a 404.
	"clientFoundRows": "true",

	// Arguments are interpolated client-side: one round trip per statement
	// instead of prepare/execute/close.
	"interpolateParams": "true",

	// utf8mb4 with a binary collation: PostgreSQL compares text
	// case-sensitively and so must the unique keys on usernames and domain
	// names. ILIKE is translated into an explicit case-insensitive match.
	"charset":   "utf8mb4",
	"collation": "utf8mb4_bin",
}

// sessionVariables are replayed by go-sql-driver as SET statements during the
// handshake.
var sessionVariables = map[string]string{
	"sql_mode":  "'" + sessionSQLMode + "'",
	"time_zone": "'+00:00'",
	// PostgreSQL's default isolation. MySQL's REPEATABLE READ keeps one
	// snapshot for the whole transaction and takes gap locks; code written for
	// PostgreSQL expects each statement to see what has been committed.
	"transaction_isolation": "'READ-COMMITTED'",
	// string_agg/array_agg become GROUP_CONCAT, which truncates at 1 KiB by
	// default.
	"group_concat_max_len": "16777216",
}

// MySQLDSN turns DATABASE_URL (mysql://user:pass@host:3306/db?...) or a native
// go-sql-driver DSN into the DSN the driver expects, with the parameters
// above forced. multiStatements is granted only to connections that need it.
func MySQLDSN(databaseURL string, multiStatements bool) (string, error) {
	base, params, err := splitDSN(databaseURL)
	if err != nil {
		return "", err
	}
	translateLibpqParams(params)
	for k, v := range forcedParams {
		params.Set(k, v)
	}
	for k, v := range sessionVariables {
		params.Set(k, v)
	}
	if multiStatements {
		params.Set("multiStatements", "true")
	} else {
		params.Del("multiStatements")
	}
	return base + "?" + params.Encode(), nil
}

// sslmodeToTLS maps libpq's sslmode onto go-sql-driver's tls parameter, so a
// PostgreSQL URL whose scheme was changed still connects. prefer/allow have no
// exact counterpart; opportunistic encryption without verification is the
// nearest.
var sslmodeToTLS = map[string]string{
	"disable":     "false",
	"allow":       "skip-verify",
	"prefer":      "skip-verify",
	"require":     "skip-verify",
	"verify-ca":   "true",
	"verify-full": "true",
}

// libpqOnlyParams mean nothing to MySQL. They must be removed rather than
// passed through: go-sql-driver replays unknown parameters as
// `SET <name>=<value>`, and the handshake then fails with "Unknown system
// variable".
var libpqOnlyParams = []string{
	"sslmode", "search_path", "application_name", "fallback_application_name",
	"target_session_attrs", "options", "connect_timeout", "client_encoding",
	"sslcert", "sslkey", "sslrootcert", "sslcrl", "sslpassword", "channel_binding",
	"gssencmode", "krbsrvname", "service", "passfile", "requiressl", "binary_parameters",
}

func translateLibpqParams(params url.Values) {
	if mode := params.Get("sslmode"); mode != "" && params.Get("tls") == "" {
		if tls, ok := sslmodeToTLS[strings.ToLower(mode)]; ok {
			params.Set("tls", tls)
		}
	}
	if t := params.Get("connect_timeout"); t != "" && params.Get("timeout") == "" {
		if _, err := strconv.Atoi(t); err == nil {
			params.Set("timeout", t+"s")
		}
	}
	for _, name := range libpqOnlyParams {
		params.Del(name)
	}
}

// splitDSN normalises either notation into go-sql-driver's
// "user:pass@proto(addr)/dbname" prefix plus query values.
func splitDSN(databaseURL string) (string, url.Values, error) {
	if !strings.Contains(databaseURL, "://") {
		base, rawQuery, _ := strings.Cut(databaseURL, "?")
		params, err := url.ParseQuery(rawQuery)
		if err != nil {
			return "", nil, fmt.Errorf("invalid DSN parameters: %w", err)
		}
		if base == "" {
			return "", nil, fmt.Errorf("empty database DSN")
		}
		return base, params, nil
	}

	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid DATABASE_URL: %w", err)
	}
	if u.Host == "" {
		return "", nil, fmt.Errorf("DATABASE_URL is missing a host")
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", nil, fmt.Errorf("DATABASE_URL is missing a database name")
	}

	var creds string
	if u.User != nil {
		creds = u.User.Username()
		if pass, ok := u.User.Password(); ok {
			creds += ":" + pass
		}
		creds += "@"
	}
	host := u.Host
	if u.Port() == "" {
		host += ":3306"
	}
	return creds + "tcp(" + host + ")/" + dbName, u.Query(), nil
}

// Redact removes the password from a database URL so it can be logged.
func Redact(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil || u.User == nil {
		return "(redacted)"
	}
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(u.User.Username(), "****")
	}
	return u.String()
}
