package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"nginx-proxy-guard/internal/database"
	"nginx-proxy-guard/internal/database/dialect"
	"nginx-proxy-guard/internal/model"
)

// withMySQL runs the test with MySQL published as the process' engine.
func withMySQL(t *testing.T) {
	t.Helper()
	dialect.SetActive(dialect.MySQL)
	t.Cleanup(func() { dialect.SetActive(dialect.Postgres) })
}

// queryMatcherFunc adapts a plain function to sqlmock.QueryMatcher.
type queryMatcherFunc func(expected, actual string) error

func (f queryMatcherFunc) Match(expected, actual string) error { return f(expected, actual) }

// containsAll matches when the actual SQL contains every "|"-separated part
// of the expectation and none of the parts prefixed with "!".
var containsAll = queryMatcherFunc(func(expected, actual string) error {
	for _, part := range strings.Split(expected, "|") {
		if neg, ok := strings.CutPrefix(part, "!"); ok {
			if strings.Contains(actual, neg) {
				return fmt.Errorf("query contains %q:\n%s", neg, actual)
			}
			continue
		}
		if !strings.Contains(actual, part) {
			return fmt.Errorf("query lacks %q:\n%s", part, actual)
		}
	}
	return nil
})

// The MySQL form of GetRulesWithExclusionStatus swaps the DISTINCT ON CTEs by
// exact text; a reworded PostgreSQL statement would make the swap a silent
// no-op and send DISTINCT ON to MySQL.
func TestExclusionStatusUsesRowNumberOnMySQL(t *testing.T) {
	withMySQL(t)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(containsAll))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("ROW_NUMBER() OVER (PARTITION BY rule_id|FROM exploit_block_rules r|!DISTINCT ON").
		WithArgs("host-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, _ = (&ExploitBlockRuleRepository{db: db}).GetRulesWithExclusionStatus(context.Background(), "host-1")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTablesExistQuery(t *testing.T) {
	pg := `SELECT to_regclass('public.roles') IS NOT NULL`
	if got := tablesExistQuery(pg, "roles"); got != pg {
		t.Fatalf("PostgreSQL query changed: %q", got)
	}
	withMySQL(t)
	got := tablesExistQuery(pg, "roles", "role_permissions")
	want := dialect.Raw("SELECT COUNT(*) = 2 FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('roles', 'role_permissions')")
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestMySQLInCIDR(t *testing.T) {
	for _, tc := range []struct{ cidr, want string }{
		{"10.0.0.0/8", "(LENGTH(INET6_ATON(c)) = 4 AND INET6_ATON(c) BETWEEN UNHEX('0a000000') AND UNHEX('0affffff'))"},
		{"192.168.1.77/24", "(LENGTH(INET6_ATON(c)) = 4 AND INET6_ATON(c) BETWEEN UNHEX('c0a80100') AND UNHEX('c0a801ff'))"},
		{"::1/128", "(LENGTH(INET6_ATON(c)) = 16 AND INET6_ATON(c) BETWEEN UNHEX('00000000000000000000000000000001') AND UNHEX('00000000000000000000000000000001'))"},
		{"0.0.0.0/0", "(LENGTH(INET6_ATON(c)) = 4 AND INET6_ATON(c) BETWEEN UNHEX('00000000') AND UNHEX('ffffffff'))"},
	} {
		got, ok := mysqlInCIDR("c", tc.cidr)
		if !ok || got != tc.want {
			t.Errorf("mysqlInCIDR(%q) = %q, %v\nwant %q", tc.cidr, got, ok, tc.want)
		}
	}
	if _, ok := mysqlInCIDR("c", "not-a-cidr"); ok {
		t.Error("an unparsable range must be refused")
	}
}

// MySQL has no COPY: a batch is multi-row INSERTs of at most
// mysqlLogInsertRows rows, all inside one transaction.
func TestLogBatchUsesMultiRowInsertsOnMySQL(t *testing.T) {
	withMySQL(t)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(containsAll))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logs := make([]model.CreateLogRequest, mysqlLogInsertRows+1)
	for i := range logs {
		logs[i] = model.CreateLogRequest{LogType: "access", Host: "a.example"}
	}
	mock.ExpectBegin()
	mock.ExpectExec(dialect.RawPrefix + "INSERT INTO logs_partitioned (`log_type`").
		WillReturnResult(sqlmock.NewResult(0, int64(mysqlLogInsertRows)))
	mock.ExpectExec(dialect.RawPrefix + "INSERT INTO logs_partitioned (`log_type`").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	r := &LogRepository{db: &database.DB{DB: db}}
	if err := r.createBatchTx(context.Background(), logs); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
