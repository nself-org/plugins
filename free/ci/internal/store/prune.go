package store

// Purpose: the store half of history retention (P7-CI-39): list pipelines with
// their times, and delete finished pipelines with everything under them.
// Inputs:  pipeline ids chosen by internal/history.
// Outputs: PipelineAge rows; the number of pipelines actually removed.
// Constraints: a pipeline whose status is "running" is never deleted (the
// check and the delete share one transaction); the removal covers jobs,
// attempts, leases and reservations, cancel intents, idempotency keys,
// declared artifacts, evidence and event-index rows of the pipeline, and one
// audit row records the prune; the audit log itself is never pruned.

import (
	"context"
	"database/sql"
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

// attemptsOf selects the attempts of a pipeline; the argument is its id.
const attemptsOf = `(SELECT a.id FROM attempt a JOIN job j ON a.job_id=j.id WHERE j.pipeline_id=?)`

// pruneSteps deletes dependents before the rows they reference.
var pruneSteps = []string{
	`DELETE FROM host_reservation WHERE lease_id IN (SELECT id FROM lease WHERE attempt_id IN ` + attemptsOf + `)`,
	`DELETE FROM lease WHERE attempt_id IN ` + attemptsOf,
	`DELETE FROM cancel_intent WHERE attempt_id IN ` + attemptsOf,
	`DELETE FROM idempotency_key WHERE attempt_id IN ` + attemptsOf,
	`DELETE FROM declared_artifact WHERE attempt_id IN ` + attemptsOf,
	`DELETE FROM evidence WHERE pipeline_id=?`,
	`DELETE FROM event_index WHERE run_id=?`,
	`DELETE FROM attempt WHERE job_id IN (SELECT id FROM job WHERE pipeline_id=?)`,
	`DELETE FROM job WHERE pipeline_id=?`,
	`DELETE FROM pipeline WHERE id=?`,
}

// DeletePipelines removes the finished pipelines among ids in one
// transaction and returns how many went. Unknown ids and running pipelines
// are skipped, not errors.
func (s *Store) DeletePipelines(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	removed := 0
	err := s.write(ctx, func(tx *sql.Tx) error {
		removed = 0
		for _, id := range ids {
			var status string
			err := tx.QueryRowContext(ctx, `SELECT status FROM pipeline WHERE id=?`, id).Scan(&status)
			if err == sql.ErrNoRows || status == "running" {
				continue
			}
			if err != nil {
				return err
			}
			for _, q := range pruneSteps {
				if _, err = tx.ExecContext(ctx, q, id); err != nil {
					return err
				}
			}
			removed++
		}
		if removed == 0 {
			return nil
		}
		return s.appendAudit(ctx, tx, "ci", "runs.prune", strconv.Itoa(removed)+" run(s)", "retention prune")
	})
	return removed, err
}
