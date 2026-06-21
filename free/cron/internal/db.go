package internal

import (
	"context"
	"fmt"
	"time"

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
		// Unique constraint on name enables ON CONFLICT (name) upsert for env-seeded jobs.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_np_cron_jobs_name
			ON np_cron_jobs(name)`,
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
