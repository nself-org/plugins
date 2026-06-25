package internal

import (
	"context"
	"time"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
