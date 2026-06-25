-- Migration: 001_initial
-- Plugin: push
-- Description: Creates np_push_devices and np_push_outbox tables.
-- Multi-Tenant Convention Wall: source_account_id in both tables.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS + ALTER TABLE IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS np_push_devices (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    device_token      TEXT        NOT NULL,
    platform          TEXT        NOT NULL CHECK (platform IN ('ios', 'android')),
    app_id            TEXT        NOT NULL DEFAULT 'default',
    user_id           TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint: one token per (platform, app_id) combination.
CREATE UNIQUE INDEX IF NOT EXISTS idx_np_push_devices_token_platform_app
    ON np_push_devices(device_token, platform, app_id);
CREATE INDEX IF NOT EXISTS idx_np_push_devices_user_id
    ON np_push_devices(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_np_push_devices_account
    ON np_push_devices(source_account_id);

CREATE TABLE IF NOT EXISTS np_push_outbox (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    device_token      TEXT        NOT NULL,
    platform          TEXT        NOT NULL CHECK (platform IN ('ios', 'android')),
    payload           JSONB       NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','queued','delivered','retrying','failed')),
    attempts          INTEGER     NOT NULL DEFAULT 0,
    last_error        TEXT,
    dedupe_hash       TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Idempotency: unique on dedupe_hash prevents double-sends.
CREATE UNIQUE INDEX IF NOT EXISTS idx_np_push_outbox_dedupe
    ON np_push_outbox(dedupe_hash)
    WHERE status != 'failed';
CREATE INDEX IF NOT EXISTS idx_np_push_outbox_status
    ON np_push_outbox(status);
CREATE INDEX IF NOT EXISTS idx_np_push_outbox_created_at
    ON np_push_outbox(created_at);
CREATE INDEX IF NOT EXISTS idx_np_push_outbox_account
    ON np_push_outbox(source_account_id);

-- Idempotent backfill for schema upgrades (adds source_account_id if running against existing schema)
ALTER TABLE np_push_devices ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
ALTER TABLE np_push_outbox ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
