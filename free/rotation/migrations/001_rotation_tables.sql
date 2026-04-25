-- Migration 001: Secret rotation schedules and event log
-- Plugin: rotation (free)
-- Tables: np_secret_rotation_schedules, np_secret_rotation_events

CREATE TABLE IF NOT EXISTS np_secret_rotation_schedules (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    secret_name       TEXT        NOT NULL,
    interval_days     INT         NOT NULL,
    window_days       INT         NOT NULL DEFAULT 7,
    notify_email      TEXT,
    notify_webhook    TEXT,
    last_rotated_at   TIMESTAMPTZ,
    next_rotation_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    enabled           BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_account_id, secret_name)
);

CREATE INDEX idx_np_rotation_schedules_account
    ON np_secret_rotation_schedules (source_account_id);

CREATE INDEX idx_np_rotation_schedules_next
    ON np_secret_rotation_schedules (next_rotation_at)
    WHERE enabled = TRUE;

CREATE TABLE IF NOT EXISTS np_secret_rotation_events (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    secret_name       TEXT        NOT NULL,
    rotated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    status            TEXT        NOT NULL CHECK (status IN ('ok', 'failed', 'rolled_back')),
    verify_result     TEXT,
    error_detail      TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_np_rotation_events_account
    ON np_secret_rotation_events (source_account_id);

CREATE INDEX idx_np_rotation_events_secret
    ON np_secret_rotation_events (source_account_id, secret_name, rotated_at DESC);
