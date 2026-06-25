-- Migration: 001_initial
-- Plugin: backup
-- Description: Creates np_backup_jobs and np_backup_schedules tables.
-- Multi-Tenant Convention Wall: source_account_id added to both tables.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_backup_jobs (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT   NOT NULL DEFAULT 'primary',
    status       VARCHAR(32) NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','running','completed','failed')),
    type         VARCHAR(32) NOT NULL DEFAULT 'full'
                 CHECK (type IN ('full','incremental','schema_only','data_only')),
    path         TEXT,
    size         BIGINT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_backup_jobs_status
    ON np_backup_jobs(status);
CREATE INDEX IF NOT EXISTS idx_np_backup_jobs_created
    ON np_backup_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_np_backup_jobs_account
    ON np_backup_jobs(source_account_id);

CREATE TABLE IF NOT EXISTS np_backup_schedules (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT    NOT NULL DEFAULT 'primary',
    cron_expr     VARCHAR(128) NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    last_run      TIMESTAMPTZ,
    next_run      TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_backup_schedules_enabled
    ON np_backup_schedules(enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_np_backup_schedules_next_run
    ON np_backup_schedules(next_run) WHERE enabled = true AND next_run IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_np_backup_schedules_account
    ON np_backup_schedules(source_account_id);

-- Idempotency: add source_account_id if upgrading from pre-multi-tenant schema
ALTER TABLE np_backup_jobs ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE np_backup_schedules ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
