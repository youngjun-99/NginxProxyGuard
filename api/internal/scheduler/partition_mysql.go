package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"nginx-proxy-guard/internal/database/dialect"
)

// mysqlRetentionBatch bounds one retention DELETE so no single statement
// holds row locks on a busy log table for long.
const mysqlRetentionBatch = 10000

// runMySQL is the scheduler's daily pass on MySQL. The PostgreSQL pass creates
// monthly partitions, drops whole partitions and TimescaleDB chunks, and
// ANALYZEs partitions; none of those objects exist on MySQL, where every log
// table is a plain InnoDB table. What remains is the retention itself, applied
// with the same settings and the same "below one day means unset" guard, as a
// batched row DELETE on each table's time column.
func (s *PartitionScheduler) runMySQL() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	settings, err := s.systemSettingsRepo.Get(ctx)
	if err != nil {
		log.Printf("[PartitionScheduler] Failed to get system settings for retention: %v", err)
		return
	}

	// Access, error and WAF logs share logs_partitioned; challenge_logs follows
	// the access-log retention as on PostgreSQL (a challenge record describes a
	// request and must not outlive its log row).
	s.deleteOlderThanMySQL(ctx, "logs_partitioned", settings.AccessLogRetentionDays)
	s.deleteOlderThanMySQL(ctx, "challenge_logs", settings.AccessLogRetentionDays)
	s.deleteOlderThanMySQL(ctx, "system_logs", settings.SystemLogRetentionDays)
	s.deleteOlderThanMySQL(ctx, "audit_logs", settings.AuditLogRetentionDays)

	s.cleanupDashboardStats()
}

// deleteOlderThanMySQL deletes rows of table whose created_at is older than
// retentionDays, mysqlRetentionBatch rows at a time. A retention below one day
// means "unset" and deletes nothing — never "delete everything".
//
// MySQL's single-table DELETE takes ORDER BY/LIMIT directly; the PostgreSQL
// form, `WHERE id IN (SELECT id ... LIMIT n)`, is refused there (a LIMIT inside
// an IN subquery, and a subquery on the table being deleted from).
func (s *PartitionScheduler) deleteOlderThanMySQL(ctx context.Context, table string, retentionDays int) {
	if retentionDays < 1 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	query := dialect.Raw(fmt.Sprintf("DELETE FROM `%s` WHERE `created_at` < ? LIMIT %d", table, mysqlRetentionBatch))

	var total int64
	for {
		result, err := s.db.ExecContext(ctx, query, cutoff)
		if err != nil {
			log.Printf("[PartitionScheduler] Failed to apply retention to %s after %d rows: %v", table, total, err)
			return
		}
		n, _ := result.RowsAffected()
		total += n
		if n < mysqlRetentionBatch {
			break
		}
		select {
		case <-ctx.Done():
			log.Printf("[PartitionScheduler] Retention on %s stopped after %d rows: %v", table, total, ctx.Err())
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	if total > 0 {
		log.Printf("[PartitionScheduler] Deleted %d rows older than %d days from %s", total, retentionDays, table)
	}
}
