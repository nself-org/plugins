-- Migration: 002_public_tables
-- Plugin: cron
-- Description: Creates np_cron_jobs and np_cron_runs in public, where the runtime queries them (D-0832).
-- 001_initial.sql ran with search_path pinned to np_cron and so created its tables in np_cron, where nothing reads
-- them. 001 is released and frozen (migration-immutable), so this file repeats it with every object qualified.
-- On an install where the old Go startup DDL already made the public tables, this only adds what that DDL lacked
-- (source_account_id and its indexes). The np_cron copies are left in place: nothing is dropped or moved.
-- Idempotent: CREATE ... IF NOT EXISTS and ADD COLUMN IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS public.np_cron_jobs (
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

CREATE TABLE IF NOT EXISTS public.np_cron_runs (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT      NOT NULL DEFAULT 'primary',
    job_id          UUID        NOT NULL REFERENCES public.np_cron_jobs(id) ON DELETE CASCADE,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    status          TEXT        NOT NULL DEFAULT 'pending',
    http_status     INTEGER,
    error           TEXT,
    duration_ms     BIGINT,
    attempt         INTEGER     NOT NULL DEFAULT 1
);

-- The Go DDL made both tables without the tenant column.
ALTER TABLE public.np_cron_jobs ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE public.np_cron_runs ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_cron_jobs_next_run
    ON public.np_cron_jobs(next_run_at) WHERE enabled = TRUE;
CREATE UNIQUE INDEX IF NOT EXISTS idx_np_cron_jobs_name
    ON public.np_cron_jobs(name);
CREATE INDEX IF NOT EXISTS idx_np_cron_jobs_account
    ON public.np_cron_jobs(source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_cron_runs_job_id
    ON public.np_cron_runs(job_id);
CREATE INDEX IF NOT EXISTS idx_np_cron_runs_started_at
    ON public.np_cron_runs(started_at);
CREATE INDEX IF NOT EXISTS idx_np_cron_runs_account
    ON public.np_cron_runs(source_account_id);
