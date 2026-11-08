package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"nginx-proxy-guard/internal/database/dialect"
)

// The #298 guarantees on MySQL: go-sql-driver's error text never reaches a
// response, and the two conditions that are the caller's fault map onto the
// same 400s as on PostgreSQL.

func useMySQL(t *testing.T) {
	t.Helper()
	dialect.SetActive(dialect.MySQL)
	t.Cleanup(func() { dialect.SetActive(dialect.Postgres) })
}

var mysqlDriverTextMarkers = []string{"Error 1", "23000", "22001", "Duplicate entry", "Data too long", "for key"}

func assertNoMySQLDriverText(t *testing.T, label, got string) {
	t.Helper()
	for _, marker := range mysqlDriverTextMarkers {
		if strings.Contains(got, marker) {
			t.Errorf("%s leaked driver text %q: %q", label, marker, got)
		}
	}
}

func TestMySQLOverLongValueBecomesClientError(t *testing.T) {
	useMySQL(t)
	err := fmt.Errorf("failed to update provider: %w",
		&mysql.MySQLError{Number: 1406, SQLState: [5]byte{'2', '2', '0', '0', '1'}, Message: "Data too long for column 'name' at row 1"})

	status, msg, ok := ClientStatusForDBError(err)
	if !ok || status != http.StatusBadRequest || msg != ErrMsgValueTooLong {
		t.Fatalf("ClientStatusForDBError = %d %q %v, want 400 %q", status, msg, ok, ErrMsgValueTooLong)
	}
	if d := SafeErrorDetail(err); d != "" {
		t.Errorf("SafeErrorDetail = %q, want \"\"", d)
	}
	assertNoMySQLDriverText(t, "SafeErrorMessage", SafeErrorMessage(err))
}

func TestMySQLDuplicateKeyStaysAServerErrorWithoutDriverText(t *testing.T) {
	useMySQL(t)
	err := fmt.Errorf("failed to create: %w",
		&mysql.MySQLError{Number: 1062, SQLState: [5]byte{'2', '3', '0', '0', '0'}, Message: "Duplicate entry 'a' for key 'users.users_username_key'"})
	if _, _, ok := ClientStatusForDBError(err); ok {
		t.Error("a duplicate key is not the caller's malformed input; it must stay unclassified here")
	}
	if got := SafeErrorMessage(err); got != ErrMsgDatabaseError {
		t.Errorf("SafeErrorMessage = %q, want %q", got, ErrMsgDatabaseError)
	}
	assertNoMySQLDriverText(t, "SafeErrorDetail", SafeErrorDetail(err))
}

// A chain flattened with %v can no longer be unwrapped; the rendered text is
// still recognised and cut.
func TestMySQLFlattenedChainIsStillScrubbed(t *testing.T) {
	useMySQL(t)
	inner := &mysql.MySQLError{Number: 1062, SQLState: [5]byte{'2', '3', '0', '0', '0'}, Message: "Duplicate entry 'a' for key 'x'"}
	err := errors.New(fmt.Sprintf("sync: %v", inner))
	if got := SafeErrorDetail(err); got != "" {
		t.Errorf("SafeErrorDetail = %q, want \"\"", got)
	}
	got := SafeClientText("failed to validate certificate_id: " + inner.Error())
	if got != "failed to validate certificate_id: A database error occurred" {
		t.Errorf("SafeClientText = %q", got)
	}
}

// A statement the translator refused quotes the SQL it could not rewrite.
func TestMySQLUntranslatableStatementIsADriverError(t *testing.T) {
	useMySQL(t)
	err := fmt.Errorf("failed to list: %w: generate_series() in SELECT ...", dialect.ErrUntranslatable)
	if got := SafeErrorDetail(err); got != "" {
		t.Errorf("SafeErrorDetail = %q, want \"\"", got)
	}
	if got := SafeErrorMessage(err); got != ErrMsgDatabaseError {
		t.Errorf("SafeErrorMessage = %q, want %q", got, ErrMsgDatabaseError)
	}
}
