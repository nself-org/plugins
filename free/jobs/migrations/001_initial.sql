-- jobs plugin: initial schema
-- CODE WINS: table names from internal/db_schema.go
-- 3 tables: np_jobs_queues, np_jobs_jobs (with source_account_id), np_jobs_history

CREATE TABLE IF NOT EXISTS np_jobs_queues (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS np_jobs_jobs (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    queue            TEXT NOT NULL DEFAULT 'default',
    payload          JSONB NOT NULL DEFAULT '{}',
    priority         INTEGER NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'pending',
    attempts         INTEGER NOT NULL DEFAULT 0,
    max_attempts     INTEGER NOT NULL DEFAULT 3,
    scheduled_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    error            TEXT,
    callback_url     TEXT,
    sign_payload     BOOLEAN NOT NULL DEFAULT TRUE,
    last_status_code INTEGER,
    last_duration_ms BIGINT,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    dlq              BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS np_jobs_history (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    job_id       TEXT NOT NULL,
    attempt      INTEGER NOT NULL,
    success      BOOLEAN NOT NULL,
    status_code  INTEGER,
    error        TEXT,
    duration_ms  BIGINT,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_status ON np_jobs_jobs(status);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_queue ON np_jobs_jobs(queue);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_scheduled ON np_jobs_jobs(scheduled_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_priority ON np_jobs_jobs(priority DESC);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_source_account ON np_jobs_jobs(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_jobs_history_job_id ON np_jobs_history(job_id);
CREATE INDEX IF NOT EXISTS idx_np_jobs_jobs_dlq ON np_jobs_jobs(dlq) WHERE dlq = TRUE;
