package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var transitions = map[model.JobState]map[model.JobState]bool{
	"queued":         {"leased": true, "cancelled": true},
	"leased":         {"running": true, "lost": true, "cancelled": true},
	"running":        {"finalizing": true, "failed": true, "lost": true, "cancelled": true},
	"finalizing":     {"passed": true, "failed": true, "cancelled": true},
	"lost":           {"retryable": true, "needs-operator": true},
	"retryable":      {"queued": true},
	"needs-operator": {"queued": true, "failed": true, "cancelled": true},
	"failed":         {"queued": true},
}

// Allowed is the pure state transition table.
func Allowed(from, to model.JobState) bool { return transitions[from][to] }

// TransitionDetail carries CAS epoch, failure class, and operator authority.
type TransitionDetail struct {
	Epoch         int64
	FailureClass  model.FailureClass
	Actor, Reason string
	Operator      bool
}

// Transition changes one attempt and dependent rows in a single write transaction.
func (s *Store) Transition(ctx context.Context, id string, from, to model.JobState, detail TransitionDetail) error {
	if !Allowed(from, to) {
		return coded("E604", fmt.Sprintf("%s -> %s", from, to))
	}
	return s.write(ctx, func(tx *sql.Tx) error { return s.apply(ctx, tx, id, from, to, detail) })
}

func (s *Store) apply(ctx context.Context, tx *sql.Tx, id string, from, to model.JobState, d TransitionDetail) error {
	var state, kind string
	var epoch int64
	var idempotent, infraMax, n int
	var jobID string
	err := tx.QueryRowContext(ctx, `SELECT a.state,a.epoch,j.kind,j.idempotent,j.infra_max,a.n,j.id FROM attempt a JOIN job j ON j.id=a.job_id WHERE a.id=?`, id).Scan(&state, &epoch, &kind, &idempotent, &infraMax, &n, &jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return coded("E604", "unknown attempt")
	}
	if err != nil {
		return err
	}
	if state != string(from) || epoch != d.Epoch {
		return coded("E605", "attempt state or epoch changed")
	}
	if to == "retryable" && (kind == "release" || idempotent == 0 || n > infraMax) {
		return coded("E604", "retry forbidden or exhausted")
	}
	if from == "lost" && (kind == "release" || idempotent == 0) && to != "needs-operator" {
		return coded("E609", "at-most-once attempt needs operator")
	}
	if (from == "failed" || from == "needs-operator") && to == "queued" && !d.Operator {
		return coded("E609", "operator action required")
	}
	if from == "needs-operator" && !d.Operator {
		return coded("E609", "operator action required")
	}
	if from == "finalizing" && !d.Operator {
		return coded("E609", "finalization requires operator action")
	}
	if from == "finalizing" && to == "passed" {
		var missing int
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM declared_artifact WHERE attempt_id=? AND (committed=0 OR committed_sha256!=expected_sha256)`, id).Scan(&missing)
		if err != nil {
			return err
		}
		if missing > 0 {
			to = "failed"
			d.FailureClass = "infra"
		}
	}
	var cancelEpoch int64
	err = tx.QueryRowContext(ctx, "SELECT epoch FROM cancel_intent WHERE attempt_id=?", id).Scan(&cancelEpoch)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && to != "cancelled" {
		return coded("E605", "cancel intent won")
	}
	if to == "cancelled" && err == nil && cancelEpoch != epoch {
		return coded("E605", "cancel intent epoch changed")
	}
	result, err := tx.ExecContext(ctx, "UPDATE attempt SET state=?,epoch=epoch+1,failure_class=?,ended_at=CASE WHEN ? IN ('passed','failed','cancelled') THEN ? ELSE ended_at END WHERE id=? AND state=? AND epoch=?", to, d.FailureClass, to, s.now().UnixNano(), id, from, epoch)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return coded("E605", "compare-and-set lost")
	}
	if to == "cancelled" {
		if _, err = tx.ExecContext(ctx, "DELETE FROM cancel_intent WHERE attempt_id=?", id); err != nil {
			return err
		}
	}
	if to == "lost" || to == "cancelled" || to == "passed" || to == "failed" {
		if _, err = tx.ExecContext(ctx, "DELETE FROM host_reservation WHERE lease_id IN (SELECT id FROM lease WHERE attempt_id=?)", id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM lease WHERE attempt_id=?", id); err != nil {
			return err
		}
	}
	if d.Operator || to == "needs-operator" || from == "retryable" && to == "queued" {
		actor := d.Actor
		if actor == "" {
			actor = "coordinator"
		}
		action := "transition"
		if from == "retryable" && to == "queued" {
			action = "retry-queued"
		}
		if err = s.appendAudit(ctx, tx, actor, action, id, fmt.Sprintf("%s:%s:%s", from, to, d.Reason)); err != nil {
			return err
		}
	}
	_ = jobID
	return nil
}

// RequestCancel stores a CAS intent. A competing result commit yields E605.
func (s *Store) RequestCancel(ctx context.Context, id, reason string, epoch int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var current int64
		var state string
		if err := tx.QueryRowContext(ctx, "SELECT epoch,state FROM attempt WHERE id=?", id).Scan(&current, &state); err != nil {
			return err
		}
		if current != epoch || state == "passed" || state == "failed" || state == "cancelled" {
			return coded("E605", "result committed or epoch changed")
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO cancel_intent(attempt_id,epoch,requested_at,reason) VALUES (?,?,?,?)", id, epoch, s.now().UnixNano(), reason)
		if err != nil {
			return coded("E605", "cancel already requested")
		}
		return s.appendAudit(ctx, tx, "operator", "cancel", id, reason)
	})
}

// AttemptStateEpoch obtains state and epoch under the same writer transaction.
func (s *Store) AttemptStateEpoch(ctx context.Context, id string) (model.JobState, int64, error) {
	var state model.JobState
	var epoch int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT state,epoch FROM attempt WHERE id=?", id).Scan(&state, &epoch)
	})
	return state, epoch, err
}
