package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nginx-proxy-guard/internal/database/dialect"
	"nginx-proxy-guard/internal/database/mysqlschema"
)

// MySQLSchema is the MySQL schema derived from the PostgreSQL one:
// migrations/001_init.sql replayed together with the upgrade list, which stay
// the single place a schema change is written. See package mysqlschema.
func MySQLSchema() (*mysqlschema.Schema, error) {
	mysqlSchemaOnce.Do(func() {
		var initSQL []byte
		initSQL, mysqlSchemaErr = migrationFS.ReadFile("migrations/001_init.sql")
		if mysqlSchemaErr != nil {
			return
		}
		steps := postgresUpgrades()
		upgrades := make([]mysqlschema.Upgrade, len(steps))
		for i, s := range steps {
			upgrades[i] = mysqlschema.Upgrade{Desc: s.desc, SQL: s.sql}
		}
		retired := make([]string, len(retiredLogsPartitionedIndexes))
		for i, r := range retiredLogsPartitionedIndexes {
			retired[i] = r.Name
		}
		mysqlSchemaCache, mysqlSchemaErr = mysqlschema.Build(string(initSQL), upgrades,
			mysqlschema.Options{RetiredIndexes: retired})
	})
	return mysqlSchemaCache, mysqlSchemaErr
}

var (
	mysqlSchemaOnce  sync.Once
	mysqlSchemaCache *mysqlschema.Schema
	mysqlSchemaErr   error
)

// runMySQLMigrations is RunMigrations for MySQL.
//
// The contract is PostgreSQL's: a fresh database gets the whole schema and
// its seed rows or the boot fails; an existing one is brought forward on every
// boot, warn-and-continue, with failures surfaced through MigrationHealth()
// instead of refusing to start — a partially migrated schema degrades a
// feature, a refused boot takes the whole proxy down.
//
// What differs is how. There is no MySQL copy of the schema to keep in step:
// MySQLSchema() derives it from the PostgreSQL files and Sync adds whatever
// the live database lacks, so a column added to 001_init.sql and the upgrade
// list reaches MySQL installs on their next boot. Data statements — seeds and
// the upgrade list's INSERT/UPDATE/DELETE — run through the translating
// driver as the PostgreSQL text they are.
//
// What is deliberately absent: every TimescaleDB step (hypertables,
// compression policies, chunk repair), the native-partition and pg_trgm
// migrations, and the legacy logs -> logs_partitioned backfill. They repair or
// tune the history of PostgreSQL installs; a MySQL install starts on plain
// InnoDB tables and has none.
func (db *DB) runMySQLMigrations() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if _, err := db.ExecContext(ctx, dialect.Raw("CREATE TABLE IF NOT EXISTS `schema_migrations` ("+
		"`version` VARCHAR(255) NOT NULL, "+
		"`applied_at` DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), "+
		"PRIMARY KEY (`version`)"+
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	var applied int
	if err := db.QueryRowContext(ctx, dialect.Raw(
		"SELECT COUNT(*) FROM `schema_migrations` WHERE `version` = '001_init'")).Scan(&applied); err != nil {
		return fmt.Errorf("failed to check migration status: %w", err)
	}
	fresh := applied == 0

	schema, err := MySQLSchema()
	if err != nil {
		return fmt.Errorf("failed to derive the MySQL schema: %w", err)
	}

	if fresh {
		log.Println("[Migration] Fresh MySQL database: creating the schema...")
	}
	report, err := schema.Sync(ctx, db.DB)
	if err != nil {
		return fmt.Errorf("failed to read the MySQL schema: %w", err)
	}
	if len(report.Executed) > 0 {
		log.Printf("[Migration] MySQL schema: %d statement(s) applied (%d table(s) created)", len(report.Executed), len(report.Created))
	}
	if fresh && len(report.Failed) > 0 {
		// Half a schema is not something later boots can repair.
		return fmt.Errorf("failed to create the MySQL schema: %w", report.Err())
	}
	for _, f := range report.Failed {
		desc := "mysql schema: " + truncateSQL(f.Statement)
		log.Printf("Warning: upgrade %q failed: %v", desc, f.Err)
		db.RecordMigrationFailure(desc, f.Err)
	}

	if fresh {
		log.Println("Running initial data (001_init.sql)...")
		for _, d := range schema.InitData() {
			if _, err := db.ExecContext(ctx, d.SQL); err != nil {
				if errors.Is(err, dialect.ErrUntranslatable) {
					log.Printf("[Migration] skipped untranslatable initial data statement: %v", err)
					continue
				}
				return fmt.Errorf("failed to apply initial data: %w\nstatement: %s", err, truncateSQL(d.SQL))
			}
		}
		if _, err := db.ExecContext(ctx, dialect.Raw(
			"INSERT INTO `schema_migrations` (`version`) VALUES ('001_init')")); err != nil {
			return fmt.Errorf("failed to update migration version: %w", err)
		}
		log.Println("Initial schema migration completed successfully")
	} else {
		log.Println("Schema already initialized, running upgrades only...")
	}

	// The upgrade list's data statements, every boot, as on PostgreSQL — but
	// only those written after MySQL support existed. The earlier ones seed
	// rows 001_init.sql already seeds or repair rows a PostgreSQL install
	// accumulated before a fix (DELETE ... USING de-duplications, UPDATE ...
	// FROM renames, defaults still naming npm-guard containers); no MySQL
	// install is older than the fix. Later entries can concern both engines
	// and run through the translator. One it has no MySQL form for is skipped,
	// not counted as a failure.
	baseline := mysqlBaselineUpgrades()
	var skipped []string
	for _, d := range schema.UpgradeData() {
		if baseline[d.Source] {
			continue
		}
		if _, err := db.ExecContext(ctx, d.SQL); err != nil {
			if errors.Is(err, dialect.ErrUntranslatable) {
				skipped = append(skipped, d.Source)
				continue
			}
			log.Printf("Warning: upgrade %q failed: %v", d.Source, err)
			db.RecordMigrationFailure(d.Source, err)
		}
	}
	if len(skipped) > 0 {
		log.Printf("[Migration] %d upgrade data statement(s) have no MySQL form and were skipped (repairs of PostgreSQL install history): %s",
			len(skipped), strings.Join(skipped, "; "))
	}

	if err := applyExploitRulesAutoDisableMySQL(ctx, db); err != nil {
		log.Printf("Warning: exploit rules auto-disable migration failed: %v", err)
	}

	if count, lastErr := db.MigrationHealth(); count > 0 {
		log.Printf("[Migration] ERROR: %d upgrade statement(s) failed — schema may be partially migrated and affected features can return errors until a restart retries them. Last failure: %s", count, lastErr)
	}
	log.Println("Schema migration completed")
	return nil
}

// mysqlBaselineUpgrade is the last entry of postgresUpgrades() that predates
// MySQL support. Data statements of it and every entry before it are
// PostgreSQL install history (see runMySQLMigrations); entries appended after
// it run on MySQL too. TestMySQLBaselineUpgradeExists keeps the name honest.
const mysqlBaselineUpgrade = "system_settings.instance_name (#369)"

// mysqlBaselineUpgrades returns the descriptions of the baseline entries. When
// the marker is missing — renamed — every entry runs, which is the safe side.
func mysqlBaselineUpgrades() map[string]bool {
	seen := map[string]bool{}
	for _, u := range postgresUpgrades() {
		seen[u.desc] = true
		if u.desc == mysqlBaselineUpgrade {
			return seen
		}
	}
	log.Printf("[Migration] Warning: baseline upgrade %q not found; running every upgrade data statement on MySQL", mysqlBaselineUpgrade)
	return nil
}

// overlyBroadExploitRules are the three system rules ExploitRulesAutoDisableSQL
// turns off (GitHub issue #123).
var overlyBroadExploitRules = []string{
	"a5cb921c-2c10-475b-8753-a56e2af1e5ba", // SQL Commands
	"41d1f7bf-9179-44cb-b41a-1685ce88d965", // SQL Keywords
	"aa90285b-2986-46f9-80e6-99946327cd24", // XSS Special Characters
}

// applyExploitRulesAutoDisableMySQL is ExploitRulesAutoDisableSQL for MySQL.
// The PostgreSQL form is a DO block because it branches on ROW_COUNT; MySQL
// has no anonymous blocks, so the branch is here. Same guard, same effect: a
// rule is touched only while auto_disabled_at is NULL, so an admin who turned
// one back on keeps that choice and repeated boots change nothing. A fresh
// install seeds the three rules already disabled, so this normally finds
// nothing to do.
func applyExploitRulesAutoDisableMySQL(ctx context.Context, db *DB) error {
	res, err := db.ExecContext(ctx, dialect.Raw(
		"UPDATE `exploit_block_rules` SET `enabled` = FALSE, `auto_disabled_at` = CURRENT_TIMESTAMP(6) "+
			"WHERE `id` IN (?, ?, ?) AND `is_system` = TRUE AND `enabled` = TRUE AND `auto_disabled_at` IS NULL"),
		overlyBroadExploitRules[0], overlyBroadExploitRules[1], overlyBroadExploitRules[2])
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return nil
	}
	details := fmt.Sprintf(`{"rule_ids": ["%s"], "reason": %q, "rollback_hint": %q}`,
		strings.Join(overlyBroadExploitRules, `", "`),
		"github issue #123: simple keyword matching produces false positives on legitimate search queries",
		"set enabled=true in the UI; the migration will not touch these rows again once auto_disabled_at is set")
	_, err = db.ExecContext(ctx, dialect.Raw(
		"INSERT INTO `system_logs` (`source`, `level`, `message`, `details`, `component`) VALUES ('internal', 'info', ?, ?, 'exploit_rules_migration')"),
		fmt.Sprintf("Auto-disabled %d overly-broad exploit rule(s) to prevent false positives on search/CMS endpoints. Review and optionally re-enable at /waf/exploit-rules.", n),
		details)
	return err
}

// truncateSQL keeps migration log lines readable; seed INSERTs run to
// kilobytes and their tail says nothing about a failure.
func truncateSQL(stmt string) string {
	stmt = strings.Join(strings.Fields(stmt), " ")
	if len(stmt) > 160 {
		return stmt[:160] + "..."
	}
	return stmt
}
