package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
)

func (d *DB) RecordJobRun(ctx context.Context, jobID string, attempt int, success bool, statusCode *int, errMsg *string, durationMs int64) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_jobs_history (job_id, attempt, success, status_code, error, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		jobID, attempt, success, statusCode, errMsg, durationMs)
	return err
}

// MarkDLQ permanently moves a job to the dead-letter queue (status=failed, dlq=true).
func (d *DB) MarkDLQ(ctx context.Context, id string, errMsg string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE np_jobs_jobs
		 SET status = $1, dlq = TRUE, error = $2, completed_at = now(), updated_at = now()
		 WHERE id = $3`,
		StatusFailed, errMsg, id)
	return err
}

// ListDLQ returns jobs currently in the dead-letter queue.
func (d *DB) ListDLQ(ctx context.Context, limit, offset int) ([]Job, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, queue, payload, priority, status, attempts, max_attempts,
		        scheduled_at, started_at, completed_at, error, created_at, updated_at
		 FROM np_jobs_jobs WHERE dlq = TRUE
		 ORDER BY updated_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Queue, &j.Payload, &j.Priority, &j.Status,
			&j.Attempts, &j.MaxAttempts, &j.ScheduledAt, &j.StartedAt,
			&j.CompletedAt, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ReviveDLQ pulls a job out of DLQ, resetting attempts so the worker picks it up again.
func (d *DB) ReviveDLQ(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := d.pool.QueryRow(ctx,
		`UPDATE np_jobs_jobs
		 SET dlq = FALSE, status = $1, attempts = 0, error = NULL,
		     started_at = NULL, completed_at = NULL,
		     scheduled_at = now(), updated_at = now()
		 WHERE id = $2 AND dlq = TRUE
		 RETURNING id, queue, payload, priority, status, attempts, max_attempts,
		           scheduled_at, started_at, completed_at, error, created_at, updated_at`,
		StatusPending, id,
	).Scan(
		&j.ID, &j.Queue, &j.Payload, &j.Priority, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.ScheduledAt, &j.StartedAt,
		&j.CompletedAt, &j.Error, &j.CreatedAt, &j.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// CreateJobOpts controls optional fields on CreateJob.
type CreateJobOpts struct {
	CallbackURL string
	SignPayload *bool
	AccountID   string
}

// CreateJob inserts a new job and ensures the queue exists. Returns the created job.
//
// S18: optional callback_url enables the real dispatcher to POST the payload
// to a receiver. When unset the worker treats the job as ack-only (legacy
// behavior preserved for existing callers).
