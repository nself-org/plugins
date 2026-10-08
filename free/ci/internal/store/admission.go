package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PipelineRow is durable pipeline metadata, never a secret-bearing config.
type PipelineRow struct{ ID, Project, Revision, Trigger, SourceTrust, PrivacyZone, PolicyDigest, InputDigest, SelectionMode, SelectionReason, SelectionDigest, Status string }
type JobRow struct {
	ID, PipelineID, Name, Kind, InputDigest string
	Idempotent, Required                    bool
	InfraMax                                int
}
type AttemptRow struct {
	ID, JobID, CoordinatorID string
	N                        int
}

// CreatePipeline records a new aggregate root.
func (s *Store) CreatePipeline(ctx context.Context, p PipelineRow) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, p.Project, p.Revision, p.Trigger, p.SourceTrust, p.PrivacyZone, p.PolicyDigest, p.InputDigest, p.SelectionMode, p.SelectionReason, p.SelectionDigest, p.Status, s.now().UnixNano())
		return err
	})
}

func (s *Store) CreateJob(ctx context.Context, j JobRow) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES (?,?,?,?,?,?,?,?,?)", j.ID, j.PipelineID, j.Name, j.Kind, j.Idempotent, j.InfraMax, j.InputDigest, j.Required, "queued")
		return err
	})
}

// CreateAttempt enforces attempt sequencing and at-most-once recovery.
func (s *Store) CreateAttempt(ctx context.Context, a AttemptRow, keyDigest string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var kind, state, previousID string
		var idempotent, infraMax int
		var last int
		err := tx.QueryRowContext(ctx, "SELECT kind,idempotent,infra_max FROM job WHERE id=?", a.JobID).Scan(&kind, &idempotent, &infraMax)
		if err != nil {
			return err
		}
		_ = tx.QueryRowContext(ctx, "SELECT coalesce(max(n),0) FROM attempt WHERE job_id=?", a.JobID).Scan(&last)
		if a.N != last+1 {
			return coded("E604", "attempt number must increase by one")
		}
		if keyDigest != "" {
			var n int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM idempotency_key WHERE key_digest=?", keyDigest).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				return coded("E608", "idempotency key reused")
			}
		}
		if last > 0 {
			err = tx.QueryRowContext(ctx, "SELECT id,state FROM attempt WHERE job_id=? AND n=?", a.JobID, last).Scan(&previousID, &state)
			if err != nil {
				return err
			}
			if kind == "release" || idempotent == 0 {
				if state != "needs-operator" {
					return coded("E609", "at-most-once job needs operator decision")
				}
				var approvals int
				err = tx.QueryRowContext(ctx, "SELECT count(*) FROM audit WHERE action='operator-retry' AND target=?", a.JobID).Scan(&approvals)
				if err != nil {
					return err
				}
				if approvals == 0 {
					return coded("E609", "operator audit required")
				}
			}
			if kind != "release" && idempotent != 0 {
				var ready int
				err = tx.QueryRowContext(ctx, "SELECT count(*) FROM audit WHERE action='retry-queued' AND target=?", previousID).Scan(&ready)
				if err != nil {
					return err
				}
				if state != "queued" || ready == 0 || last > infraMax {
					return coded("E604", "retry not authorized")
				}
			}
		}
		var coordinator any
		if a.CoordinatorID != "" {
			coordinator = a.CoordinatorID
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO attempt(id,job_id,n,coordinator_id,state) VALUES (?,?,?,?,?)", a.ID, a.JobID, a.N, coordinator, "queued")
		if err != nil {
			return err
		}
		if keyDigest != "" {
			_, err = tx.ExecContext(ctx, "INSERT INTO idempotency_key(key_digest,attempt_id) VALUES (?,?)", keyDigest, a.ID)
			if err != nil {
				return coded("E608", "idempotency key reused")
			}
		}
		return nil
	})
}

func (s *Store) OperatorRetry(ctx context.Context, actor, jobID, reason string) error {
	return s.AppendAudit(ctx, actor, "operator-retry", jobID, reason)
}

type Demand struct {
	AttemptID, RunnerID, LeaseID, TokenDigest string
	CPU, MemMB, CapacityCPU, CapacityMemMB    int
	TTL                                       time.Duration
	Epoch                                     int64
}
type AdmissionStep func(context.Context, AdmissionView) error
type AdmissionView struct {
	Host               string
	Demand             Demand
	UsedCPU, UsedMemMB int
}

// AdmitAndLease serializes host reservations and the state transition.
func (s *Store) AdmitAndLease(ctx context.Context, host string, d Demand, steps ...AdmissionStep) error {
	if host == "" || d.CPU <= 0 || d.MemMB <= 0 || d.CapacityCPU <= 0 || d.CapacityMemMB <= 0 {
		return coded("E604", "invalid demand")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		var usedCPU, usedMem int
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(sum(cpu),0),coalesce(sum(mem_mb),0) FROM host_reservation WHERE host=?", host).Scan(&usedCPU, &usedMem); err != nil {
			return err
		}
		view := AdmissionView{host, d, usedCPU, usedMem}
		if usedCPU+d.CPU > d.CapacityCPU || usedMem+d.MemMB > d.CapacityMemMB {
			return coded("E604", "host capacity exhausted")
		}
		for _, step := range steps {
			if err := step(ctx, view); err != nil {
				return err
			}
		}
		if err := s.apply(ctx, tx, d.AttemptID, "queued", "leased", TransitionDetail{Epoch: d.Epoch}); err != nil {
			return err
		}
		ttl := d.TTL
		if ttl <= 0 {
			ttl = s.ttl
		}
		now := s.now().UnixNano()
		_, err := tx.ExecContext(ctx, "INSERT INTO lease(id,attempt_id,host,runner_id,expires_at,heartbeat_at,token_digest) VALUES (?,?,?,?,?,?,?)", d.LeaseID, d.AttemptID, host, d.RunnerID, now+int64(ttl), now, d.TokenDigest)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO host_reservation(host,lease_id,cpu,mem_mb) VALUES (?,?,?,?)", host, d.LeaseID, d.CPU, d.MemMB)
		return err
	})
}

// ReserveBudget is a pluggable admission step; the default has no external budget.
func (s *Store) ReserveBudget(ctx context.Context, view AdmissionView) error {
	_ = ctx
	_ = view
	return nil
}

func (s *Store) RecordEvidence(ctx context.Context, id, pipelineID, attemptID, revision, inputDigest string, body []byte) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO evidence(id,pipeline_id,attempt_id,revision,input_digest,body,created_at) VALUES (?,?,?,?,?,?,?)", id, pipelineID, attemptID, revision, inputDigest, body, s.now().UnixNano())
		if err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, "coordinator", "evidence", id, fmt.Sprintf("revision=%s", revision))
	})
}
