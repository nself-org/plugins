package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LeaseFail atomically fences, records the old runner and schedules at most one retry.
func (s *Store) LeaseFail(ctx context.Context, l RemoteLease, now time.Time, reason string, expired bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var current, deadline int64
		var runner, kind, jobID, coordinatorID string
		var idempotent, maximum, n int
		err := tx.QueryRowContext(ctx, `SELECT l.epoch,l.expires_at,l.runner_id,j.kind,j.id,j.idempotent,j.infra_max,a.n,coalesce(a.coordinator_id,'') FROM lease l JOIN attempt a ON a.id=l.attempt_id JOIN job j ON j.id=a.job_id WHERE l.id=?`, l.ID).Scan(&current, &deadline, &runner, &kind, &jobID, &idempotent, &maximum, &n, &coordinatorID)
		if errors.Is(err, sql.ErrNoRows) {
			return coded("E605", "lease changed concurrently")
		}
		if err != nil {
			return err
		}
		if current != l.Epoch || expired && deadline > now.UnixNano() {
			return coded("E605", "lease renewed concurrently")
		}
		if _, err = tx.ExecContext(ctx, "UPDATE attempt SET previous_runner_id=? WHERE id=?", runner, l.AttemptID); err != nil {
			return err
		}
		if err := s.apply(ctx, tx, l.AttemptID, l.State, "lost", TransitionDetail{Epoch: l.AttemptEpoch, FailureClass: "lost", Reason: reason}); err != nil {
			return err
		}
		used, err := infraAttemptNumber(ctx, tx, jobID, n)
		if err != nil {
			return err
		}
		if kind == "release" || idempotent == 0 || reason != DrainHandBack && used > maximum {
			return s.apply(ctx, tx, l.AttemptID, "lost", "needs-operator", TransitionDetail{Epoch: l.AttemptEpoch + 1, FailureClass: "lost", Reason: reason})
		}
		if err := s.apply(ctx, tx, l.AttemptID, "lost", "retryable", TransitionDetail{Epoch: l.AttemptEpoch + 1, FailureClass: "lost", Reason: reason}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, "coordinator", "retry-queued", l.AttemptID, reason); err != nil {
			return err
		}
		var coordinator any
		if coordinatorID != "" {
			coordinator = coordinatorID
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO attempt(id,job_id,n,coordinator_id,state,previous_runner_id) VALUES (?,?,?,?,?,?)", fmt.Sprintf("%s-retry-%d", l.AttemptID, n+1), jobID, n+1, coordinator, "queued", runner)
		return err
	})
}
