package store

// Purpose: the durable status outbox of the run orchestrator (P7-CI-37) and
// the pipeline-scoped reads and writes it needs. The outbox is keyed by
// (revision, context) with a monotonic seq; a status posts only when its seq
// is not older than the last one posted (CI D13). Migration set
// report-outbox (HOST review R2): P7-HOST-02 extends it expand-only.
// Inputs: revision, status context, payloads (JSON, no secrets). Outputs: seq
// numbers, unposted entries, run views. SQL lives only in this package; every
// write is one transaction on the single writer connection.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
)

//go:embed migrations/report-outbox/*.sql
var outboxMigrations embed.FS

func init() {
	if err := Register("report-outbox", outboxMigrations); err != nil {
		panic(err)
	}
}

const ( // outbox entry states
	OutboxReserved   = "reserved"   // seq taken at run start, no payload yet
	OutboxPending    = "pending"    // payload ready, not posted
	OutboxPosting    = "posting"    // claimed by one process that is delivering it
	OutboxPosted     = "posted"     // delivered
	OutboxSuperseded = "superseded" // a newer seq was posted first
)

// OutboxEntry is one status owed to a reporter.
type OutboxEntry struct {
	Revision, Context string
	Seq               int64
	State             string
	Payload           []byte
	CreatedAt         int64
}

// OutboxReserve takes the next seq for (revision, context).
func (s *Store) OutboxReserve(ctx context.Context, revision, statusContext string) (int64, error) {
	var seq int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT coalesce(max(seq),0)+1 FROM outbox WHERE revision=? AND context=?", revision, statusContext).Scan(&seq); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO outbox(revision,context,seq,state,created_at) VALUES (?,?,?,?,?)", revision, statusContext, seq, OutboxReserved, s.now().UnixNano())
		return err
	})
	return seq, err
}

// OutboxReady stores the payload and makes the entry deliverable.
func (s *Store) OutboxReady(ctx context.Context, revision, statusContext string, seq int64, payload []byte) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE outbox SET state=?,payload_json=? WHERE revision=? AND context=? AND seq=? AND state IN (?,?)", OutboxPending, payload, revision, statusContext, seq, OutboxReserved, OutboxPending)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return coded("E607", "outbox entry is not open")
		}
		return nil
	})
}

// OutboxDecision is the payload of the newest entry of (revision, context) that holds one: the status the
// run was authorized to post, after the publisher, the release veto and the publish gates (D-1320). ok is
// false when no entry holds a payload (the run never reached its status).
func (s *Store) OutboxDecision(ctx context.Context, revision, statusContext string) (payload []byte, ok bool, err error) {
	err = s.readers.QueryRowContext(ctx, "SELECT payload_json FROM outbox WHERE revision=? AND context=? AND payload_json IS NOT NULL ORDER BY seq DESC LIMIT 1", revision, statusContext).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return payload, err == nil, err
}

// OutboxUnposted lists deliverable entries, oldest first.
func (s *Store) OutboxUnposted(ctx context.Context) ([]OutboxEntry, error) {
	return collect(ctx, s, func(r *sql.Rows) (e OutboxEntry, err error) {
		err = r.Scan(&e.Revision, &e.Context, &e.Seq, &e.State, &e.Payload, &e.CreatedAt)
		return
	}, "SELECT revision,context,seq,state,coalesce(payload_json,x''),created_at FROM outbox WHERE state=? OR (state=? AND claimed_at<=?) ORDER BY created_at,seq", OutboxPending, OutboxPosting, s.now().Add(-claimTTL).UnixNano())
}

// collect runs a read query and scans every row.
func collect[T any](ctx context.Context, s *Store, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error) {
	rows, err := s.readers.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RunJob is a persisted job of one run.
type RunJob struct {
	ID, Name, Kind, Status string
	Required               bool
	Idempotent             bool
}

// PipelineJobs lists the jobs of a run in name order.
func (s *Store) PipelineJobs(ctx context.Context, pipelineID string) ([]RunJob, error) {
	return collect(ctx, s, func(r *sql.Rows) (j RunJob, err error) {
		err = r.Scan(&j.ID, &j.Name, &j.Kind, &j.Status, &j.Required, &j.Idempotent)
		return
	}, "SELECT id,name,kind,status,required,idempotent FROM job WHERE pipeline_id=? ORDER BY name", pipelineID)
}

// PipelineAttempts lists every attempt of a run, job then n.
func (s *Store) PipelineAttempts(ctx context.Context, pipelineID string) ([]AttemptView, error) {
	return s.attemptViews(ctx, "SELECT a.id,a.job_id,a.n,a.state,a.epoch,coalesce(a.failure_class,'') FROM attempt a JOIN job j ON j.id=a.job_id WHERE j.pipeline_id=? ORDER BY a.job_id,a.n", pipelineID)
}

// LostAttempts lists attempts waiting for a retry or operator decision.
func (s *Store) LostAttempts(ctx context.Context) ([]AttemptView, error) {
	return s.attemptViews(ctx, "SELECT id,job_id,n,state,epoch,coalesce(failure_class,'') FROM attempt WHERE state=? ORDER BY id", "lost")
}

// PendingCancels lists attempts of a run with a cancel intent that have not
// reached a terminal state.
func (s *Store) PendingCancels(ctx context.Context, pipelineID string) ([]string, error) {
	return collect(ctx, s, func(r *sql.Rows) (id string, err error) {
		err = r.Scan(&id)
		return
	}, "SELECT a.id FROM cancel_intent c JOIN attempt a ON a.id=c.attempt_id JOIN job j ON j.id=a.job_id WHERE j.pipeline_id=? AND a.state NOT IN ('passed','failed','cancelled') ORDER BY a.id", pipelineID)
}

func (s *Store) attemptViews(ctx context.Context, query string, args ...any) ([]AttemptView, error) {
	return collect(ctx, s, func(r *sql.Rows) (a AttemptView, err error) {
		err = r.Scan(&a.ID, &a.JobID, &a.N, &a.State, &a.Epoch, &a.FailureClass)
		return
	}, query, args...)
}

// PipelineByID returns one run; ok is false when it does not exist.
func (s *Store) PipelineByID(ctx context.Context, id string) (PipelineRow, bool, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status FROM pipeline WHERE id=?`, id)
	if err != nil {
		return PipelineRow{}, false, err
	}
	defer func() { _ = rows.Close() }()
	list, err := scanPipelines(rows)
	if err != nil || len(list) == 0 {
		return PipelineRow{}, false, err
	}
	return list[0], true, nil
}

// FinishPipeline records the end state of a run.
func (s *Store) FinishPipeline(ctx context.Context, id, status string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE pipeline SET status=?,finished_at=? WHERE id=?", status, s.now().UnixNano(), id)
		return err
	})
}

// SetJobStatus records the end state of a job (queued, passed, failed, skipped, cancelled).
func (s *Store) SetJobStatus(ctx context.Context, jobID, status string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE job SET status=? WHERE id=?", status, jobID)
		return err
	})
}

// Coordinators lists every registered coordinator identity.
func (s *Store) Coordinators(ctx context.Context) ([]Coordinator, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT id,pid,process_start,boot_id,incarnation,heartbeat_at FROM coordinator ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Coordinator
	for rows.Next() {
		var c Coordinator
		if err = rows.Scan(&c.ID, &c.PID, &c.ProcessStart, &c.BootID, &c.Incarnation, &c.HeartbeatAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ExpireCoordinator marks a coordinator whose process is gone as stale now.
func (s *Store) ExpireCoordinator(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE coordinator SET heartbeat_at=0 WHERE id=?", id)
		return err
	})
}

// RecordRunEvidence stores the run-level evidence, which belongs to no single attempt.
func (s *Store) RecordRunEvidence(ctx context.Context, id, pipelineID, revision, inputDigest string, body []byte) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO evidence(id,pipeline_id,attempt_id,revision,input_digest,body,created_at) VALUES (?,?,NULL,?,?,?,?)", id, pipelineID, revision, inputDigest, body, s.now().UnixNano()); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, "coordinator", "evidence", id, "revision="+revision)
	})
}

// StrandedAttempt is a queued or finalizing attempt of a gone coordinator.
type StrandedAttempt struct {
	ID, PipelineID, State string
	Epoch                 int64
}

// StrandedAttempts lists queued/finalizing attempts of stale coordinators
// (SweepStale moves only leased and running ones).
func (s *Store) StrandedAttempts(ctx context.Context, currentBoot string) ([]StrandedAttempt, error) {
	stale, err := s.StaleCoordinators(ctx, currentBoot)
	if err != nil {
		return nil, err
	}
	var out []StrandedAttempt
	for _, c := range stale {
		rows, err := s.readers.QueryContext(ctx, "SELECT a.id,j.pipeline_id,a.state,a.epoch FROM attempt a JOIN job j ON j.id=a.job_id WHERE a.coordinator_id=? AND a.state IN ('queued','finalizing') ORDER BY a.id", c.ID)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var a StrandedAttempt
			if err = rows.Scan(&a.ID, &a.PipelineID, &a.State, &a.Epoch); err != nil {
				_ = rows.Close()
				return out, err
			}
			out = append(out, a)
		}
		if err = rows.Close(); err != nil {
			return out, err
		}
	}
	return out, nil
}
