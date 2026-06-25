-- Migration: 001_initial
-- Plugin: cron
-- Description: Creates np_cron_jobs and np_cron_runs tables.
-- Multi-Tenant Convention Wall: source_account_id in both tables.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS + IF NOT EXISTS indexes.

CREATE TABLE IF NOT EXISTS np_cron_jobs (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT      NOT NULL DEFAULT 'primary',
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
);

CREATE INDEX IF NOT EXISTS idx_np_cron_jobs_next_run
    ON np_cron_jobs(next_run_at) WHERE enabled = TRUE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_np_cron_jobs_name
    ON np_cron_jobs(name);
CREATE INDEX IF NOT EXISTS idx_np_cron_jobs_account
    ON np_cron_jobs(source_account_id);

CREATE TABLE IF NOT EXISTS np_cron_runs (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT      NOT NULL DEFAULT 'primary',
    job_id          UUID        NOT NULL REFERENCES np_cron_jobs(id) ON DELETE CASCADE,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    status          TEXT        NOT NULL DEFAULT 'pending',
    http_status     INTEGER,
    error           TEXT,
    duration_ms     BIGINT,
    attempt         INTEGER     NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_np_cron_runs_job_id
    ON np_cron_runs(job_id);
CREATE INDEX IF NOT EXISTS idx_np_cron_runs_started_at
    ON np_cron_runs(started_at);
CREATE INDEX IF NOT EXISTS idx_np_cron_runs_account
    ON np_cron_runs(source_account_id);

-- Idempotent backfill for schema upgrades
ALTER TABLE np_cron_jobs ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE np_cron_runs ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
