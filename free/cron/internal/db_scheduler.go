package internal

import (
	"context"
	"fmt"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetDueJobIDs(pool *pgxpool.Pool) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT id FROM np_cron_jobs
		 WHERE enabled = true AND next_run_at < NOW()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PruneOldRuns deletes run history older than the given number of days.
func PruneOldRuns(pool *pgxpool.Pool, retentionDays int) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx,
		`DELETE FROM np_cron_runs
		 WHERE started_at < NOW() - ($1 || ' days')::INTERVAL`,
		fmt.Sprintf("%d", retentionDays),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
