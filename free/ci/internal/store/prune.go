package store

// Purpose: the store half of history retention (P7-CI-39): list pipelines with
// their times, and delete finished pipelines with everything under them.
// Inputs:  pipeline ids chosen by internal/history.
// Outputs: PipelineAge rows; the number of pipelines actually removed.
// Constraints: a pipeline whose status is "running" is never deleted (the
// check and the delete share one transaction, batches of pruneBatch); the removal covers jobs,
// attempts, leases and reservations, cancel intents, idempotency keys,
// declared artifacts, evidence and event-index rows of the pipeline, and one
// audit row records the prune; the audit log itself is never pruned.

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
)

// PipelineAge is a pipeline with its times (unix nanoseconds; FinishedAt is 0
// while it runs).
type PipelineAge struct {
	ID, Status, Revision string
	CreatedAt            int64
	FinishedAt           int64
}

// PipelineAges lists every pipeline, newest first.
func (s *Store) PipelineAges(ctx context.Context) ([]PipelineAge, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT id,status,revision,created_at,coalesce(finished_at,0) FROM pipeline ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineAge
	for rows.Next() {
		var p PipelineAge
		if err = rows.Scan(&p.ID, &p.Status, &p.Revision, &p.CreatedAt, &p.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// pruneBatch bounds one transaction: the single writer connection is held for
// one batch at a time, so a first prune of a long history never starves a run.
const pruneBatch = 100

// pruneSteps delete dependents before the rows they reference. Each takes one
// argument, a JSON array of pipeline ids, and walks every table once per batch.
var pruneSteps = []string{
	`DELETE FROM host_reservation WHERE lease_id IN (SELECT l.id FROM lease l JOIN attempt a ON l.attempt_id=a.id JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM lease WHERE attempt_id IN (SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM cancel_intent WHERE attempt_id IN (SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM sched_admitted WHERE attempt_id IN (SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM sched_refusal WHERE attempt_id IN (SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM idempotency_key WHERE attempt_id IN (SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM declared_artifact WHERE attempt_id IN (SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM evidence WHERE pipeline_id IN (SELECT value FROM json_each(?))`,
	`DELETE FROM event_index WHERE run_id IN (SELECT value FROM json_each(?))`,
	`DELETE FROM attempt WHERE job_id IN (SELECT id FROM job WHERE pipeline_id IN (SELECT value FROM json_each(?)))`,
	`DELETE FROM job WHERE pipeline_id IN (SELECT value FROM json_each(?))`,
	`DELETE FROM pipeline WHERE id IN (SELECT value FROM json_each(?))`,
}

// DeletePipelines removes the finished pipelines among ids, pruneBatch per
// transaction, and returns how many went. Unknown ids and running pipelines
// are skipped, not errors; each batch re-checks the status inside its own
// transaction, and one audit row per batch records the prune.
func (s *Store) DeletePipelines(ctx context.Context, ids []string) (int, error) {
	removed := 0
	for len(ids) > 0 {
		n := min(pruneBatch, len(ids))
		got, err := s.deleteBatch(ctx, ids[:n])
		removed += got
		if err != nil {
			return removed, err
		}
		ids = ids[n:]
	}
	return removed, nil
}

func (s *Store) deleteBatch(ctx context.Context, ids []string) (int, error) {
	in, _ := json.Marshal(ids)
	removed := 0
	err := s.write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM pipeline WHERE status<>'running' AND id IN (SELECT value FROM json_each(?))`, string(in))
		if err != nil {
			return err
		}
		var finished []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			finished = append(finished, id)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if removed = len(finished); removed == 0 {
			return nil
		}
		arg, _ := json.Marshal(finished)
		for _, q := range pruneSteps {
			if _, err = tx.ExecContext(ctx, q, string(arg)); err != nil {
				return err
			}
		}
		return s.appendAudit(ctx, tx, "ci", "runs.prune", strconv.Itoa(removed)+" run(s)", "retention prune")
	})
	if err != nil {
		removed = 0
	}
	return removed, err
}
