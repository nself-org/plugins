package internal

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// EnsureTables creates the jobs tables if they do not exist.
//
// S18 upgrade: adds callback_url + signing + DLQ fields so the queue plugin
// ships a real HTTP-callback dispatcher (replacing the prior no-op worker).
// Existing rows get the new columns via ADD COLUMN IF NOT EXISTS so the
// migration is safe on warm installs.
func (d *DB) EnsureTables(ctx context.Context) error {
	ddl := `
CREATE TABLE IF NOT EXISTS np_jobs_queues (
    id   TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS np_jobs_jobs (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    queue         TEXT NOT NULL DEFAULT 'default',
    payload       JSONB NOT NULL DEFAULT '{}',
    priority      INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'pending',
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 3,
    scheduled_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- S18 additions: real HTTP-callback dispatch, DLQ parity with webhooks plugin.
ALTER TABLE np_jobs_jobs ADD COLUMN IF NOT EXISTS callback_url TEXT;
ALTER TABLE np_jobs_jobs ADD COLUMN IF NOT EXISTS sign_payload BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE np_jobs_jobs ADD COLUMN IF NOT EXISTS last_status_code INTEGER;
ALTER TABLE np_jobs_jobs ADD COLUMN IF NOT EXISTS last_duration_ms BIGINT;
ALTER TABLE np_jobs_jobs ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE np_jobs_jobs ADD COLUMN IF NOT EXISTS dlq BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS np_jobs_history (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    job_id        TEXT NOT NULL,
    attempt       INTEGER NOT NULL,
    success       BOOLEAN NOT NULL,
    status_code   INTEGER,
    error         TEXT,
    duration_ms   BIGINT,
    triggered_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_status ON np_jobs_jobs(status);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_queue ON np_jobs_jobs(queue);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_scheduled ON np_jobs_jobs(scheduled_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_priority ON np_jobs_jobs(priority DESC);
CREATE INDEX IF NOT EXISTS idx_np_jobs_history_job_id ON np_jobs_history(job_id);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_dlq ON np_jobs_jobs(dlq) WHERE dlq = TRUE;
`
	_, err := d.pool.Exec(ctx, ddl)
	return err
}

// RecordJobRun inserts a history row for one dispatch attempt.
