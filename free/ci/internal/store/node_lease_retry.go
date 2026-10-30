package store

// Purpose: the lookup a run needs to adopt the retry row the lease engine queued when it failed an expired lease
//          (LeaseFail inserts <attempt>-retry-<n+1> in state queued, with the previous runner recorded).
// Inputs:  a job row id and an attempt number.
// Outputs: the id and state of that attempt; ok false when the job has no attempt n.
// Constraints: read only.

import (
	"context"
	"database/sql"
	"errors"
)

// AttemptAt returns the attempt n of a job: its id and state.
func (s *Store) AttemptAt(ctx context.Context, jobID string, n int) (id, state string, ok bool, err error) {
	err = s.readers.QueryRowContext(ctx, "SELECT id,state FROM attempt WHERE job_id=? AND n=?", jobID, n).Scan(&id, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return id, state, true, nil
}
