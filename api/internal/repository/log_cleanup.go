package repository

import (
	"context"
	"fmt"
	"time"

	"nginx-proxy-guard/internal/database/dialect"
)

func (r *LogRepository) DeleteOld(ctx context.Context, retentionDays int) (int64, error) {
	if dialect.IsMySQL() {
		return r.deleteOldMySQL(ctx, retentionDays)
	}

	// Use partition DROP instead of row-level DELETE for efficiency.
	// drop_old_partitions() drops entire monthly partitions older than the cutoff,
	// which is orders of magnitude faster than DELETE on large tables.
	retentionMonths := retentionDays / 30
	if retentionMonths < 1 {
		retentionMonths = 1
	}
	var dropped int
	//npg:pg-only
	err := r.db.QueryRowContext(ctx, `SELECT drop_old_partitions('logs_p', $1)`, retentionMonths).Scan(&dropped)
	if err != nil {
		return 0, fmt.Errorf("failed to drop old log partitions: %w", err)
	}
	return int64(dropped), nil
}

// deleteOldMySQL is DeleteOld on MySQL, where logs_partitioned is a plain
// table with no partitions to drop: rows older than retentionDays are deleted
// in batches, and the count returned is rows rather than partitions. A
// retention below one day deletes nothing.
func (r *LogRepository) deleteOldMySQL(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, nil
	}
	const batch = 10000
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	query := dialect.Raw(fmt.Sprintf("DELETE FROM logs_partitioned WHERE created_at < ? LIMIT %d", batch))

	var total int64
	for {
		result, err := r.db.ExecContext(ctx, query, cutoff)
		if err != nil {
			return total, fmt.Errorf("failed to delete old logs: %w", err)
		}
		n, _ := result.RowsAffected()
		total += n
		if n < batch {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}
