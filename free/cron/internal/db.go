package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CronJob represents a stored cron job.
type CronJob struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	CronExpr        string     `json:"cron_expr"`
	CallbackURL     string     `json:"callback_url"`
	Payload         *string    `json:"payload,omitempty"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	RunCount        int64      `json:"run_count"`
	MaxAttempts     int        `json:"max_attempts"`
	CreatedAt       time.Time  `json:"created_at"`
}

// CronRun represents a single execution record.
type CronRun struct {
	ID          string     `json:"id"`
	JobID       string     `json:"job_id"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Status      string     `json:"status"`
	HTTPStatus  *int       `json:"http_status,omitempty"`
	Error       *string    `json:"error,omitempty"`
	DurationMs  *int64     `json:"duration_ms,omitempty"`
	Attempt     int        `json:"attempt"`
}

// Migrate runs idempotent schema creation for np_cron_jobs and np_cron_runs.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS np_cron_jobs (
			id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			name            TEXT        NOT NULL,
			cron_expr       TEXT        NOT NULL,
			callback_url    TEXT        NOT NULL,
			payload         JSONB,
			enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
			last_run_at     TIMESTAMPTZ,
			next_run_at     TIMESTAMPTZ,
			run_count       BIGINT      NOT NULL DEFAULT 0,
			max_attempts    INTEGER     NOT NULL DEFAULT 3,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_cron_jobs_next_run
			ON np_cron_jobs(next_run_at) WHERE enabled = TRUE`,
		`CREATE TABLE IF NOT EXISTS np_cron_runs (
			id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			job_id          UUID        NOT NULL REFERENCES np_cron_jobs(id) ON DELETE CASCADE,
			started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			completed_at    TIMESTAMPTZ,
			status          TEXT        NOT NULL DEFAULT 'pending',
			http_status     INTEGER,
			error           TEXT,
			duration_ms     BIGINT,
			attempt         INTEGER     NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_np_cron_runs_job_id
			ON np_cron_runs(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_np_cron_runs_started_at
			ON np_cron_runs(started_at)`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// CreateJob inserts a new cron job and returns it.
func CreateJob(pool *pgxpool.Pool, name, cronExpr, callbackURL string, payload *string, enabled bool) (*CronJob, error) {
	nextRun := NextRunTime(cronExpr)
	id := uuid.New().String()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx,
		`INSERT INTO np_cron_jobs (id, name, cron_expr, callback_url, payload, enabled, next_run_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, name, cronExpr, callbackURL, payload, enabled, nextRun,
	)
	if err != nil {
		return nil, err
	}

	return GetJob(pool, id)
}

// GetJob retrieves a single job by ID.
func GetJob(pool *pgxpool.Pool, id string) (*CronJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := pool.QueryRow(ctx,
		`SELECT id, name, cron_expr, callback_url, payload, enabled,
		        last_run_at, next_run_at, run_count, max_attempts, created_at
		 FROM np_cron_jobs WHERE id = $1`, id)

	var j CronJob
	err := row.Scan(&j.ID, &j.Name, &j.CronExpr, &j.CallbackURL, &j.Payload,
		&j.Enabled, &j.LastRunAt, &j.NextRunAt, &j.RunCount, &j.MaxAttempts, &j.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// ListJobs returns all jobs ordered by creation time.
func ListJobs(pool *pgxpool.Pool) ([]CronJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT id, name, cron_expr, callback_url, payload, enabled,
		        last_run_at, next_run_at, run_count, max_attempts, created_at
		 FROM np_cron_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []CronJob
	for rows.Next() {
		var j CronJob
		if err := rows.Scan(&j.ID, &j.Name, &j.CronExpr, &j.CallbackURL, &j.Payload,
			&j.Enabled, &j.LastRunAt, &j.NextRunAt, &j.RunCount, &j.MaxAttempts, &j.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJob updates mutable fields of a job. Returns the updated job.
func UpdateJob(pool *pgxpool.Pool, id string, name, cronExpr, callbackURL *string, payload *string, enabled *bool, maxAttempts *int) (*CronJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Recalculate next_run_at if cron_expr changed.
	var nextRun *time.Time
	if cronExpr != nil {
		t := NextRunTime(*cronExpr)
		nextRun = t
	}

	tag, err := pool.Exec(ctx,
		`UPDATE np_cron_jobs SET
			name         = COALESCE($2, name),
			cron_expr    = COALESCE($3, cron_expr),
			callback_url = COALESCE($4, callback_url),
			payload      = COALESCE($5, payload),
			enabled      = COALESCE($6, enabled),
			max_attempts = COALESCE($7, max_attempts),
			next_run_at  = COALESCE($8, next_run_at)
		 WHERE id = $1`,
		id, name, cronExpr, callbackURL, payload, enabled, maxAttempts, nextRun,
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil
	}

	return GetJob(pool, id)
}

// DeleteJob removes a job by ID. Returns true if a row was deleted.
func DeleteJob(pool *pgxpool.Pool, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx, `DELETE FROM np_cron_jobs WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RecordRun records a job execution in np_cron_runs and updates the job metadata.
func RecordRun(pool *pgxpool.Pool, jobID string, success bool, httpStatus *int, errMsg *string, durationMs int64, attempt int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := "success"
	if !success {
		status = "failed"
	}
	now := time.Now().UTC()

	_, err := pool.Exec(ctx,
		`INSERT INTO np_cron_runs (id, job_id, started_at, completed_at, status, http_status, error, duration_ms, attempt)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		uuid.New().String(), jobID, now, now, status, httpStatus, errMsg, durationMs, attempt,
	)
	if err != nil {
		return err
	}

	// Update job last_run_at, run_count, next_run_at.
	job, err := GetJob(pool, jobID)
	if err != nil {
		return err
	}
	nextRun := NextRunTime(job.CronExpr)

	_, err = pool.Exec(ctx,
		`UPDATE np_cron_jobs SET last_run_at = $1, run_count = run_count + 1, next_run_at = $2
		 WHERE id = $3`,
		now, nextRun, jobID,
	)
	return err
}

// GetJobRuns retrieves execution history for a specific job.
func GetJobRuns(pool *pgxpool.Pool, jobID string, limit, offset int) ([]CronRun, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT id, job_id, started_at, completed_at, status, http_status, error, duration_ms, attempt
		 FROM np_cron_runs
		 WHERE job_id = $1
		 ORDER BY started_at DESC
		 LIMIT $2 OFFSET $3`,
		jobID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []CronRun
	for rows.Next() {
		var r CronRun
		if err := rows.Scan(&r.ID, &r.JobID, &r.StartedAt, &r.CompletedAt,
			&r.Status, &r.HTTPStatus, &r.Error, &r.DurationMs, &r.Attempt); err != nil {
			return nil, 0, err
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_cron_runs WHERE job_id = $1`, jobID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	return runs, total, nil
}

// GetDueJobIDs returns IDs of enabled jobs whose next_run_at is in the past.
func GetDueJobIDs(pool *pgxpool.Pool) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT id FROM np_cron_jobs
		 WHERE enabled = true AND next_run_at < NOW()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PruneOldRuns deletes run history older than the given number of days.
func PruneOldRuns(pool *pgxpool.Pool, retentionDays int) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tag, err := pool.Exec(ctx,
		`DELETE FROM np_cron_runs
		 WHERE started_at < NOW() - ($1 || ' days')::INTERVAL`,
		fmt.Sprintf("%d", retentionDays),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
