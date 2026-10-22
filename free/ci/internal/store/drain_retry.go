package store

import (
	"context"
	"database/sql"
)

// DrainHandBack is a planned hand-back, not a failed infrastructure retry.
const DrainHandBack = "coordinator.drain"

// infraAttemptNumber discounts planned hand-backs while keeping attempt sequence numbers unique.
func infraAttemptNumber(ctx context.Context, tx *sql.Tx, jobID string, n int) (int, error) {
	var drained int
	err := tx.QueryRowContext(ctx, `SELECT count(DISTINCT a.id) FROM attempt a JOIN audit u ON u.target=a.id WHERE a.job_id=? AND a.n<=? AND u.action='retry-queued' AND u.detail=?`, jobID, n, DrainHandBack).Scan(&drained)
	return n - drained, err
}
