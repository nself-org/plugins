package store

// Purpose: the job class of an attempt (P7-NODE-20 drain): its kind and whether it may run twice.
// Inputs:  an attempt id. Outputs: the job kind and the idempotent flag.
// Constraints: read-only; an unknown attempt is an error, never a default, so the drain fails closed.

import "context"

// AttemptJobClass returns the kind ("release", "gate", ...) and idempotence of the job an attempt belongs to.
func (s *Store) AttemptJobClass(ctx context.Context, attemptID string) (kind string, idempotent bool, err error) {
	var idem int
	err = s.readers.QueryRowContext(ctx, `SELECT j.kind,j.idempotent FROM attempt a JOIN job j ON j.id=a.job_id WHERE a.id=?`, attemptID).Scan(&kind, &idem)
	return kind, idem != 0, err
}
