package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides PostgreSQL operations for the backup plugin tables.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// --------------------------------------------------------------------------
// Migration
// --------------------------------------------------------------------------

// Migrate creates the np_backup_jobs and np_backup_schedules tables if they
// do not already exist.
func (s *Store) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ddl := `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_backup_jobs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    status      VARCHAR(32)  NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','running','completed','failed')),
    type        VARCHAR(32)  NOT NULL DEFAULT 'full'
                CHECK (type IN ('full','incremental','schema_only','data_only')),
    path        TEXT,
    size        BIGINT,
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error       TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_backup_jobs_status
    ON np_backup_jobs(status);
CREATE INDEX IF NOT EXISTS idx_np_backup_jobs_created
    ON np_backup_jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS np_backup_schedules (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    cron_expr   VARCHAR(128) NOT NULL,
    enabled     BOOLEAN      NOT NULL DEFAULT true,
    last_run    TIMESTAMPTZ,
    next_run    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_backup_schedules_enabled
    ON np_backup_schedules(enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_np_backup_schedules_next_run
    ON np_backup_schedules(next_run) WHERE enabled = true AND next_run IS NOT NULL;
`

	_, err := s.pool.Exec(ctx, ddl)
	return err
}

// --------------------------------------------------------------------------
// BackupJob
// --------------------------------------------------------------------------

// BackupJob represents a row in np_backup_jobs.
type BackupJob struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Type        string     `json:"type"`
	Path        *string    `json:"path"`
	Size        *int64     `json:"size"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Error       *string    `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
}

// InsertJob creates a new backup job in "running" status and returns it.
func (s *Store) InsertJob(ctx context.Context, backupType string) (*BackupJob, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO np_backup_jobs (status, type, started_at)
		 VALUES ('running', $1, now())
		 RETURNING id, status, type, path, size, started_at, completed_at, error, created_at`,
		backupType,
	)

	var j BackupJob
	err := row.Scan(&j.ID, &j.Status, &j.Type, &j.Path, &j.Size,
		&j.StartedAt, &j.CompletedAt, &j.Error, &j.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}
	return &j, nil
}

// CompleteJob marks a job as completed with the given path and size.
func (s *Store) CompleteJob(ctx context.Context, id string, path string, size int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE np_backup_jobs
		 SET status = 'completed', path = $2, size = $3, completed_at = now()
		 WHERE id = $1`,
		id, path, size,
	)
	return err
}

// FailJob marks a job as failed with an error message.
func (s *Store) FailJob(ctx context.Context, id string, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE np_backup_jobs
		 SET status = 'failed', error = $2, completed_at = now()
		 WHERE id = $1`,
		id, errMsg,
	)
	return err
}

// GetJob retrieves a single backup job by ID.
func (s *Store) GetJob(ctx context.Context, id string) (*BackupJob, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, status, type, path, size, started_at, completed_at, error, created_at
		 FROM np_backup_jobs WHERE id = $1`,
		id,
	)

	var j BackupJob
	err := row.Scan(&j.ID, &j.Status, &j.Type, &j.Path, &j.Size,
		&j.StartedAt, &j.CompletedAt, &j.Error, &j.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &j, nil
}

// ListJobs returns backup jobs ordered by created_at descending.
func (s *Store) ListJobs(ctx context.Context, limit, offset int) ([]BackupJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, status, type, path, size, started_at, completed_at, error, created_at
		 FROM np_backup_jobs
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var jobs []BackupJob
	for rows.Next() {
		var j BackupJob
		if err := rows.Scan(&j.ID, &j.Status, &j.Type, &j.Path, &j.Size,
			&j.StartedAt, &j.CompletedAt, &j.Error, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// DeleteJob deletes a backup job by ID and returns whether a row was removed.
func (s *Store) DeleteJob(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM np_backup_jobs WHERE id = $1`, id,
	)
	if err != nil {
		return false, fmt.Errorf("delete job: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// --------------------------------------------------------------------------
// BackupSchedule
// --------------------------------------------------------------------------

// BackupSchedule represents a row in np_backup_schedules.
type BackupSchedule struct {
	ID        string     `json:"id"`
	CronExpr  string     `json:"cron_expr"`
	Enabled   bool       `json:"enabled"`
	LastRun   *time.Time `json:"last_run"`
	NextRun   *time.Time `json:"next_run"`
	CreatedAt time.Time  `json:"created_at"`
}

// InsertSchedule creates a new schedule and returns it.
func (s *Store) InsertSchedule(ctx context.Context, cronExpr string, enabled bool) (*BackupSchedule, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO np_backup_schedules (cron_expr, enabled)
		 VALUES ($1, $2)
		 RETURNING id, cron_expr, enabled, last_run, next_run, created_at`,
		cronExpr, enabled,
	)

	var sc BackupSchedule
	err := row.Scan(&sc.ID, &sc.CronExpr, &sc.Enabled, &sc.LastRun, &sc.NextRun, &sc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert schedule: %w", err)
	}
	return &sc, nil
}

// ListSchedules returns all schedules ordered by created_at descending.
func (s *Store) ListSchedules(ctx context.Context, limit, offset int) ([]BackupSchedule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, cron_expr, enabled, last_run, next_run, created_at
		 FROM np_backup_schedules
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()

	var schedules []BackupSchedule
	for rows.Next() {
		var sc BackupSchedule
		if err := rows.Scan(&sc.ID, &sc.CronExpr, &sc.Enabled, &sc.LastRun, &sc.NextRun, &sc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, sc)
	}
	return schedules, rows.Err()
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// MarshalJSON is a convenience that marshals v and returns the bytes.
// Panics on error (only used for known-safe structures).
func MarshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal json: %v", err))
	}
	return b
}
