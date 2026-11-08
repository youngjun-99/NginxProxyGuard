package dialect

import (
	"errors"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

// ErrUntranslatable is wrapped by every error the translator returns for a
// PostgreSQL construct it has no faithful MySQL form for. Such a statement
// never reaches the server. errors.Is(err, ErrUntranslatable) tells "this
// query needs an engine branch" apart from a real database failure.
var ErrUntranslatable = errors.New("statement cannot be translated for MySQL")

// SQLSTATE codes repositories branch on. MySQL reports the same standard
// classes for these conditions, so one set of constants serves both drivers.
const (
	SQLStateUniqueViolation           = "23505"
	SQLStateForeignKeyViolation       = "23503"
	SQLStateInvalidTextRepresentation = "22P02"
	SQLStateStringDataRightTruncation = "22001"
)

// MySQL error numbers. MySQL's SQLSTATE for every integrity error is the
// generic 23000, so the number is what distinguishes them.
const (
	mysqlDupEntry          = 1062 // ER_DUP_ENTRY
	mysqlDupEntryWithKey   = 1586 // ER_DUP_ENTRY_WITH_KEY_NAME
	mysqlNoReferencedRow   = 1216 // ER_NO_REFERENCED_ROW
	mysqlRowIsReferenced   = 1217 // ER_ROW_IS_REFERENCED
	mysqlRowIsReferenced2  = 1451 // ER_ROW_IS_REFERENCED_2
	mysqlNoReferencedRow2  = 1452 // ER_NO_REFERENCED_ROW_2
	mysqlDataTooLong       = 1406 // ER_DATA_TOO_LONG
	mysqlTruncatedWrongVal = 1292 // ER_TRUNCATED_WRONG_VALUE (bad datetime etc.)
	mysqlWrongValueForType = 1411 // ER_WRONG_VALUE_FOR_TYPE
	mysqlInvalidJSONText   = 3140 // ER_INVALID_JSON_TEXT
)

// SQLState returns the PostgreSQL-style SQLSTATE of a driver error, or "" when
// err carries no driver error. MySQL errors are mapped onto the PostgreSQL
// codes callers already test for, so code written against lib/pq's codes
// keeps working: a duplicate key is 23505, a dangling reference 23503, an
// over-long value 22001 and a malformed value 22P02.
func SQLState(err error) string {
	if err == nil {
		return ""
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code)
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case mysqlDupEntry, mysqlDupEntryWithKey:
			return SQLStateUniqueViolation
		case mysqlNoReferencedRow, mysqlNoReferencedRow2, mysqlRowIsReferenced, mysqlRowIsReferenced2:
			return SQLStateForeignKeyViolation
		case mysqlDataTooLong:
			return SQLStateStringDataRightTruncation
		case mysqlTruncatedWrongVal, mysqlWrongValueForType, mysqlInvalidJSONText:
			return SQLStateInvalidTextRepresentation
		}
		return string(myErr.SQLState[:])
	}
	return ""
}

// IsUniqueViolation reports whether the database refused a row because it
// collides with an existing one, on either engine.
func IsUniqueViolation(err error) bool { return SQLState(err) == SQLStateUniqueViolation }

// IsForeignKeyViolation reports whether a foreign key refused the write: the
// referenced row is missing, or a child row still points at the one being
// deleted.
func IsForeignKeyViolation(err error) bool { return SQLState(err) == SQLStateForeignKeyViolation }

// ViolatedConstraint names the constraint (unique index) a violation hit, or
// "" when unknown. Tables with more than one unique key need it to say which
// value collided. lib/pq carries the name; go-sql-driver only has it in the
// message — "for key 'users.users_email_key'" on MySQL 8 — so the table
// prefix is cut to give the same name PostgreSQL would.
func ViolatedConstraint(err error) string {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Constraint
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		const marker = "for key '"
		i := strings.LastIndex(myErr.Message, marker)
		if i < 0 {
			return ""
		}
		key, _, _ := strings.Cut(myErr.Message[i+len(marker):], "'")
		if dot := strings.LastIndex(key, "."); dot >= 0 {
			key = key[dot+1:]
		}
		return key
	}
	return ""
}

// IsDriverError reports whether a database driver error of either engine sits
// anywhere in err's chain.
func IsDriverError(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return true
	}
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr)
}

// mysqlDriverText matches how go-sql-driver renders a server error:
// "Error 1062 (23000): Duplicate entry ...". The SQLSTATE part is absent for
// errors raised before the server sent one.
var mysqlDriverText = regexp.MustCompile(`Error \d{4,5}( \([0-9A-Z]{5}\))?: `)

// DriverTextIndex returns where a rendered driver error starts inside
// message, or -1. lib/pq prefixes "pq: "; go-sql-driver prefixes
// "Error <number> (<SQLSTATE>): ". Everything from that index on was written by
// the driver and the server, not by us.
func DriverTextIndex(message string) int {
	idx := strings.Index(message, "pq: ")
	if loc := mysqlDriverText.FindStringIndex(message); loc != nil && (idx < 0 || loc[0] < idx) {
		idx = loc[0]
	}
	return idx
}

// DriverTextSQLState extracts the PostgreSQL-style SQLSTATE from rendered
// driver text (from DriverTextIndex on), or "". It lets a message that was
// flattened to a string still be classified.
func DriverTextSQLState(driverText string) string {
	if strings.HasPrefix(driverText, "pq: ") {
		// lib/pq renders "pq: <message> (<SQLSTATE>)" once the code is known.
		if m := pqCodeSuffix.FindStringSubmatch(driverText); m != nil {
			return m[1]
		}
		return ""
	}
	m := mysqlNumber.FindStringSubmatch(driverText)
	if m == nil {
		return ""
	}
	switch m[1] {
	case "1062", "1586":
		return SQLStateUniqueViolation
	case "1216", "1217", "1451", "1452":
		return SQLStateForeignKeyViolation
	case "1406":
		return SQLStateStringDataRightTruncation
	case "1292", "1411", "3140":
		return SQLStateInvalidTextRepresentation
	}
	return ""
}

var (
	pqCodeSuffix = regexp.MustCompile(`\(([0-9A-Z]{5})\)`)
	mysqlNumber  = regexp.MustCompile(`^Error (\d{4,5})`)
)
