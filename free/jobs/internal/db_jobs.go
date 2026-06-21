package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"encoding/json"
	"strconv"
	"time"
)

func (d *DB) CreateJob(ctx context.Context, queue string, payload json.RawMessage, priority int, delay time.Duration, maxAttempts int, opts ...CreateJobOpts) (*Job, error) {
	var o CreateJobOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	signPayload := true
	if o.SignPayload != nil {
		signPayload = *o.SignPayload
	}
	accountID := o.AccountID
	if accountID == "" {
		accountID = "primary"
	}

	// Ensure queue record exists.
	_, err := d.pool.Exec(ctx,
		`INSERT INTO np_jobs_queues (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		queue,
	)
	if err != nil {
		return nil, err
	}

	scheduledAt := time.Now().Add(delay)

	var callbackURL interface{}
	if o.CallbackURL != "" {
		callbackURL = o.CallbackURL
	}

	var j Job
	err = d.pool.QueryRow(ctx,
		`INSERT INTO np_jobs_jobs (queue, payload, priority, max_attempts, scheduled_at, callback_url, sign_payload, source_account_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, queue, payload, priority, status, attempts, max_attempts,
		           scheduled_at, started_at, completed_at, error, created_at, updated_at`,
		queue, payload, priority, maxAttempts, scheduledAt, callbackURL, signPayload, accountID,
	).Scan(
		&j.ID, &j.Queue, &j.Payload, &j.Priority, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.ScheduledAt, &j.StartedAt,
		&j.CompletedAt, &j.Error, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// GetCallbackURL returns the callback URL + sign_payload setting for a job.
// A nil URL means the job has no callback and should be ack-processed.
func (d *DB) GetCallbackURL(ctx context.Context, id string) (*string, bool, error) {
	var url *string
	var sign bool
	err := d.pool.QueryRow(ctx,
		`SELECT callback_url, sign_payload FROM np_jobs_jobs WHERE id = $1`,
		id,
	).Scan(&url, &sign)
	if err == pgx.ErrNoRows {
		return nil, true, nil
	}
	return url, sign, err
}

// GetJob retrieves a single job by ID.
func (d *DB) GetJob(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := d.pool.QueryRow(ctx,
		`SELECT id, queue, payload, priority, status, attempts, max_attempts,
		        scheduled_at, started_at, completed_at, error, created_at, updated_at
		 FROM np_jobs_jobs WHERE id = $1`, id,
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

// ListJobs returns jobs with optional queue and status filters.
func (d *DB) ListJobs(ctx context.Context, queue, status string, limit, offset int) ([]Job, error) {
	query := `SELECT id, queue, payload, priority, status, attempts, max_attempts,
	                 scheduled_at, started_at, completed_at, error, created_at, updated_at
	          FROM np_jobs_jobs WHERE 1=1`
	args := []interface{}{}
	argN := 0

	if queue != "" {
		argN++
		query += " AND queue = $" + itoa(argN)
		args = append(args, queue)
	}
	if status != "" {
		argN++
		query += " AND status = $" + itoa(argN)
		args = append(args, status)
	}

	query += " ORDER BY priority DESC, scheduled_at ASC"

	argN++
	query += " LIMIT $" + itoa(argN)
	args = append(args, limit)

	argN++
	query += " OFFSET $" + itoa(argN)
	args = append(args, offset)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.Queue, &j.Payload, &j.Priority, &j.Status,
			&j.Attempts, &j.MaxAttempts, &j.ScheduledAt, &j.StartedAt,
			&j.CompletedAt, &j.Error, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// DeleteJob removes a job by ID. Returns true if a row was deleted.
func (d *DB) DeleteJob(ctx context.Context, id string) (bool, error) {
	tag, err := d.pool.Exec(ctx, `DELETE FROM np_jobs_jobs WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RetryJob resets a failed job to pending with zero attempts.
func (d *DB) RetryJob(ctx context.Context, id string) (*Job, error) {
	var j Job
	err := d.pool.QueryRow(ctx,
		`UPDATE np_jobs_jobs
		 SET status = $1, attempts = 0, error = NULL, started_at = NULL,
		     completed_at = NULL, scheduled_at = now(), updated_at = now()
		 WHERE id = $2 AND status = $3
		 RETURNING id, queue, payload, priority, status, attempts, max_attempts,
		           scheduled_at, started_at, completed_at, error, created_at, updated_at`,
		StatusPending, id, StatusFailed,
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

// ListQueuesWithStats returns all queues with job count breakdowns.
func (d *DB) ListQueuesWithStats(ctx context.Context) ([]QueueStats, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT
			q.name,
			COALESCE(SUM(CASE WHEN j.status = 'pending'   THEN 1 ELSE 0 END), 0) AS pending,
			COALESCE(SUM(CASE WHEN j.status = 'active'    THEN 1 ELSE 0 END), 0) AS active,
			COALESCE(SUM(CASE WHEN j.status = 'completed' THEN 1 ELSE 0 END), 0) AS completed,
			COALESCE(SUM(CASE WHEN j.status = 'failed'    THEN 1 ELSE 0 END), 0) AS failed
		FROM np_jobs_queues q
		LEFT JOIN np_jobs_jobs j ON j.queue = q.name
		GROUP BY q.name
		ORDER BY q.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []QueueStats
	for rows.Next() {
		var s QueueStats
		if err := rows.Scan(&s.Name, &s.Pending, &s.Active, &s.Completed, &s.Failed); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// ClaimNextJob atomically claims the next pending job that is due.
func (d *DB) ClaimNextJob(ctx context.Context) (*Job, error) {
	var j Job
	err := d.pool.QueryRow(ctx,
		`UPDATE np_jobs_jobs
		 SET status = $1, started_at = now(), attempts = attempts + 1, updated_at = now()
		 WHERE id = (
		     SELECT id FROM np_jobs_jobs
		     WHERE status = $2 AND scheduled_at <= now()
		     ORDER BY priority DESC, scheduled_at ASC
		     FOR UPDATE SKIP LOCKED
		     LIMIT 1
		 )
		 RETURNING id, queue, payload, priority, status, attempts, max_attempts,
		           scheduled_at, started_at, completed_at, error, created_at, updated_at`,
		StatusActive, StatusPending,
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

// CompleteJob marks a job as completed.
func (d *DB) CompleteJob(ctx context.Context, id string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE np_jobs_jobs SET status = $1, completed_at = now(), updated_at = now() WHERE id = $2`,
		StatusCompleted, id,
	)
	return err
}

// FailJob marks a job as failed with an error message, or resets to pending
// with capped exponential backoff if retries remain.
//
// S18: backoff schedule matches the webhooks plugin (2s, 8s, 30s, 2m, 5m) —
// shared standard across jobs/webhooks/cron so operators see consistent retry
// behavior across all three plugins. Attempts is 1-indexed (ClaimNextJob
// already bumped it before dispatch).
func (d *DB) FailJob(ctx context.Context, id string, errMsg string, maxAttempts int) error {
	// Compute backoff in SQL so the math lives next to the update. attempts=1
	// → 2s; 2→8s; 3→30s; 4→120s; anything higher caps at 300s.
	_, err := d.pool.Exec(ctx,
		`UPDATE np_jobs_jobs
		 SET error = $1,
		     status = CASE WHEN attempts >= $2 THEN $3 ELSE $4 END,
		     scheduled_at = CASE
		         WHEN attempts >= $2 THEN scheduled_at
		         WHEN attempts = 1   THEN now() + interval '2 seconds'
		         WHEN attempts = 2   THEN now() + interval '8 seconds'
		         WHEN attempts = 3   THEN now() + interval '30 seconds'
		         WHEN attempts = 4   THEN now() + interval '2 minutes'
		         ELSE                     now() + interval '5 minutes'
		     END,
		     updated_at = now()
		 WHERE id = $5`,
		errMsg, maxAttempts, StatusFailed, StatusPending, id,
	)
	return err
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
