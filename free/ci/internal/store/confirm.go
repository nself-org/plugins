package store

// Purpose: the store side of `--confirm-flaky` (P7-CI-40). A failed attempt
// with failure class code is never retried automatically; the operator may
// ask for one more attempt, which is recorded as an audit row and then
// allowed by CreateAttempt exactly once (each request names the attempt it
// follows, so it cannot be reused for the next).
// Inputs:  the failed attempt id, who asked and why.
// Outputs: an audit row (action confirm-flaky, target the attempt).
// Constraints: only a failed attempt of class code, of an idempotent job that
// is not a release; the failed attempt keeps its result; nothing here changes
// attempt state.

import (
	"context"
	"database/sql"
	"errors"
)

// AuthorizeConfirmation records a request to rerun a failed (code) attempt once more.
func (s *Store) AuthorizeConfirmation(ctx context.Context, actor, attemptID, reason string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var state, class, kind string
		var idempotent int
		err := tx.QueryRowContext(ctx, "SELECT a.state,coalesce(a.failure_class,''),j.kind,j.idempotent FROM attempt a JOIN job j ON j.id=a.job_id WHERE a.id=?", attemptID).Scan(&state, &class, &kind, &idempotent)
		if errors.Is(err, sql.ErrNoRows) {
			return coded("E604", "unknown attempt")
		}
		if err != nil {
			return err
		}
		if state != "failed" || class != "code" || kind == "release" || idempotent == 0 {
			return coded("E609", "only a failed code attempt of a repeatable job can be confirmed")
		}
		return s.appendAudit(ctx, tx, actor, "confirm-flaky", attemptID, reason)
	})
}

// confirmationCount is the number of confirm-flaky requests that name the
// failed attempt previousID (0 for an attempt in any other state).
func confirmationCount(ctx context.Context, tx *sql.Tx, previousID, state string) (int, error) {
	if state != "failed" {
		return 0, nil
	}
	var n int
	// the request must name a code-class failure: an audit label alone never authorises a retry of an infra attempt
	err := tx.QueryRowContext(ctx, "SELECT count(*) FROM audit u JOIN attempt a ON a.id=u.target WHERE u.action='confirm-flaky' AND u.target=? AND a.failure_class='code'", previousID).Scan(&n)
	return n, err
}
