package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

//go:embed migrations/node-lease/*.sql
var leaseMigrations embed.FS

func init() {
	if err := Register("node-lease", leaseMigrations); err != nil {
		panic(err)
	}
}

// RemoteLease is the durable fencing and expiry view of one remote attempt.
type RemoteLease struct {
	ID, AttemptID, RunnerID, CoordinatorID string
	State                                  model.JobState
	Epoch, AttemptEpoch, ExpiresAt         int64
}

// LeaseEpoch composes the coordinator incarnation and a monotone attempt counter.
func LeaseEpoch(incarnation, previous int64) int64 {
	if incarnation < previous>>32 {
		incarnation = previous >> 32
	}
	if incarnation < 1 || incarnation > 0x7fffffff {
		return 0
	}
	counter := int64(1)
	if incarnation == previous>>32 {
		counter = (previous & 0xffffffff) + 1
	}
	if counter > 0xffffffff {
		return 0
	}
	return incarnation<<32 | counter
}

// LeaseGrant atomically reserves a host, transitions the attempt, and issues a fence.
func (s *Store) LeaseGrant(ctx context.Context, host string, d Demand, incarnation int64) (int64, error) {
	if host == "" || d.AttemptID == "" || d.LeaseID == "" || d.RunnerID == "" || d.CPU < 1 || d.MemMB < 1 || d.CapacityCPU < 1 || d.CapacityMemMB < 1 {
		return 0, coded("E604", "invalid lease demand")
	}
	var fence int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		var usedCPU, usedMem int
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(sum(cpu),0),coalesce(sum(mem_mb),0) FROM host_reservation WHERE host=?", host).Scan(&usedCPU, &usedMem); err != nil {
			return err
		}
		if usedCPU+d.CPU > d.CapacityCPU || usedMem+d.MemMB > d.CapacityMemMB {
			return coded("E604", "host capacity exhausted")
		}
		var previous int64
		if err := tx.QueryRowContext(ctx, "SELECT lease_epoch FROM attempt WHERE id=?", d.AttemptID).Scan(&previous); err != nil {
			return err
		}
		fence = LeaseEpoch(incarnation, previous)
		if fence == 0 {
			return coded("E607", "fencing epoch exhausted")
		}
		if err := s.apply(ctx, tx, d.AttemptID, "queued", "leased", TransitionDetail{Epoch: d.Epoch}); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE attempt SET lease_epoch=? WHERE id=?", fence, d.AttemptID); err != nil {
			return err
		}
		ttl := d.TTL
		if ttl <= 0 {
			ttl = s.ttl
		}
		now := s.now().UnixNano()
		if _, err := tx.ExecContext(ctx, "INSERT INTO lease(id,attempt_id,host,runner_id,expires_at,heartbeat_at,token_digest,epoch) VALUES (?,?,?,?,?,?,?,?)", d.LeaseID, d.AttemptID, host, d.RunnerID, now+int64(ttl), now, d.TokenDigest, fence); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO host_reservation(host,lease_id,cpu,mem_mb) VALUES (?,?,?,?)", host, d.LeaseID, d.CPU, d.MemMB)
		return err
	})
	return fence, err
}

// LeaseGet reads state and the lease fence together for carrier validation.
func (s *Store) LeaseGet(ctx context.Context, id string) (RemoteLease, error) {
	var l RemoteLease
	err := s.readers.QueryRowContext(ctx, `SELECT l.id,l.attempt_id,l.runner_id,coalesce(a.coordinator_id,''),a.state,l.epoch,a.epoch,l.expires_at FROM lease l JOIN attempt a ON a.id=l.attempt_id WHERE l.id=?`, id).Scan(&l.ID, &l.AttemptID, &l.RunnerID, &l.CoordinatorID, &l.State, &l.Epoch, &l.AttemptEpoch, &l.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l, coded("E662", "stale lease epoch; stop the job and clean its workspace")
	}
	return l, err
}

// LeaseFence validates a carrier log or result against the durable lease epoch.
func (s *Store) LeaseFence(ctx context.Context, id string, epoch int64) error {
	var current, expires int64
	var state string
	err := s.readers.QueryRowContext(ctx, `SELECT l.epoch,a.state,l.expires_at FROM lease l JOIN attempt a ON a.id=l.attempt_id WHERE l.id=?`, id).Scan(&current, &state, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return coded("E662", "stale lease epoch; stop the job and clean its workspace")
	}
	if err != nil {
		return err
	}
	if current != epoch || state != "leased" && state != "running" || expires <= s.now().UnixNano() {
		return coded("E662", "stale lease epoch; stop the job and clean its workspace")
	}
	return nil
}

// LeaseRenew validates the fence even when a heartbeat is coalesced by the caller.
func (s *Store) LeaseRenew(ctx context.Context, id string, epoch int64, now time.Time, ttl time.Duration, durable bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var current, expires int64
		var state string
		err := tx.QueryRowContext(ctx, `SELECT l.epoch,a.state,l.expires_at FROM lease l JOIN attempt a ON a.id=l.attempt_id WHERE l.id=?`, id).Scan(&current, &state, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return coded("E662", "stale lease epoch; stop the job and clean its workspace")
		}
		if err != nil {
			return err
		}
		if current != epoch || state != "leased" && state != "running" || expires <= now.UnixNano() {
			return coded("E662", "stale lease epoch; stop the job and clean its workspace")
		}
		if !durable {
			return nil
		}
		_, err = tx.ExecContext(ctx, "UPDATE lease SET heartbeat_at=?,expires_at=? WHERE id=? AND epoch=?", now.UnixNano(), now.Add(ttl).UnixNano(), id, epoch)
		return err
	})
}

// LeaseExpired returns only leases whose durable deadline has elapsed.
func (s *Store) LeaseExpired(ctx context.Context, now time.Time) ([]RemoteLease, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT l.id,l.attempt_id,l.runner_id,coalesce(a.coordinator_id,''),a.state,l.epoch,a.epoch,l.expires_at FROM lease l JOIN attempt a ON a.id=l.attempt_id WHERE l.expires_at<=? AND a.state IN ('leased','running') ORDER BY l.id`, now.UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RemoteLease
	for rows.Next() {
		var l RemoteLease
		if err := rows.Scan(&l.ID, &l.AttemptID, &l.RunnerID, &l.CoordinatorID, &l.State, &l.Epoch, &l.AttemptEpoch, &l.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LeaseGrace extends leases owned by another coordinator or older lease incarnation.
func (s *Store) LeaseGrace(ctx context.Context, currentID string, incarnation int64, now time.Time, ttl time.Duration) (int64, error) {
	var changed int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE lease AS l SET expires_at=max(expires_at,?) WHERE EXISTS (SELECT 1 FROM attempt a WHERE a.id=l.attempt_id AND a.state IN ('leased','running') AND ((a.coordinator_id IS NULL OR a.coordinator_id!=?) OR (l.epoch>0 AND l.epoch<?)))`, now.Add(ttl).UnixNano(), currentID, incarnation<<32)
		if err != nil {
			return err
		}
		changed, err = res.RowsAffected()
		return err
	})
	return changed, err
}

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
		if kind == "release" || idempotent == 0 || n > maximum {
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

// LeasePreviousRunner gives placement the runner excluded by retry anti-affinity.
func (s *Store) LeasePreviousRunner(ctx context.Context, attemptID string) (string, error) {
	var runner sql.NullString
	err := s.readers.QueryRowContext(ctx, "SELECT previous_runner_id FROM attempt WHERE id=?", attemptID).Scan(&runner)
	return runner.String, err
}

// LeaseResult rejects stale carrier results and lets the transition CAS decide cancel races.
func (s *Store) LeaseResult(ctx context.Context, id string, epoch int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var attemptID, runnerID string
		var current, attemptEpoch, expires int64
		err := tx.QueryRowContext(ctx, `SELECT l.attempt_id,l.runner_id,l.epoch,a.epoch,l.expires_at FROM lease l JOIN attempt a ON a.id=l.attempt_id WHERE l.id=?`, id).Scan(&attemptID, &runnerID, &current, &attemptEpoch, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return coded("E662", "stale lease epoch; stop the job and clean its workspace")
		}
		if err != nil {
			return err
		}
		if current != epoch || expires <= s.now().UnixNano() {
			return coded("E662", "stale lease epoch; stop the job and clean its workspace")
		}
		if err := s.apply(ctx, tx, attemptID, "running", "finalizing", TransitionDetail{Epoch: attemptEpoch}); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE node_failure_streak SET failures=0 WHERE node_id=? AND tripped=0", runnerID)
		return err
	})
}

// LeaseFailure increments a durable streak; tripped is true only once per streak.
func (s *Store) LeaseFailure(ctx context.Context, nodeID string, infra bool, threshold int) (int, bool, error) {
	if nodeID == "" || threshold < 1 {
		return 0, false, coded("E604", "invalid breaker configuration")
	}
	var count, tripped int
	err := s.write(ctx, func(tx *sql.Tx) error {
		if !infra {
			return tx.QueryRowContext(ctx, "SELECT failures,tripped FROM node_failure_streak WHERE node_id=?", nodeID).Scan(&count, &tripped)
		}
		var state string
		nodeErr := tx.QueryRowContext(ctx, "SELECT state FROM node WHERE id=?", nodeID).Scan(&state)
		if nodeErr != nil && !errors.Is(nodeErr, sql.ErrNoRows) {
			return nodeErr
		}
		if nodeErr == nil && state == "online" {
			if _, err := tx.ExecContext(ctx, "UPDATE node_failure_streak SET failures=0,tripped=0 WHERE node_id=? AND tripped=1", nodeID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO node_failure_streak(node_id,failures) VALUES (?,1) ON CONFLICT(node_id) DO UPDATE SET failures=failures+1", nodeID)
		if err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, "SELECT failures,tripped FROM node_failure_streak WHERE node_id=?", nodeID).Scan(&count, &tripped); err != nil {
			return err
		}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return count, err == nil && infra && count >= threshold && tripped == 0, err
}

// LeaseBreakerTripped records a successful maintenance transition. Failed transitions remain retryable.
func (s *Store) LeaseBreakerTripped(ctx context.Context, nodeID string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx, "SELECT state FROM node WHERE id=?", nodeID).Scan(&state); err != nil {
			return err
		}
		if state != "maintenance" {
			return coded("E605", "breaker maintenance transition lost")
		}
		_, err := tx.ExecContext(ctx, "UPDATE node_failure_streak SET tripped=1 WHERE node_id=?", nodeID)
		return err
	})
}
