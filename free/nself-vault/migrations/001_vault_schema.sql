-- +goose Up
-- nself-vault: per-device envelope encryption schema
-- Migration: 001_vault_schema
-- Convention: all tables use np_ prefix, tenant_id UUID nullable (Multi-Tenant Convention Wall)

-- Vault credential records
CREATE TABLE IF NOT EXISTS np_vault_devices (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        NOT NULL,
    tenant_id      UUID,
    device_pubkey  BYTEA       NOT NULL,
    device_label   TEXT,
    platform       TEXT,
    last_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_vault_devices_user_seen
    ON np_vault_devices (user_id, last_seen_at DESC)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_np_vault_devices_tenant
    ON np_vault_devices (tenant_id)
    WHERE tenant_id IS NOT NULL;

-- Vault credential records (the logical secret items)
CREATE TABLE IF NOT EXISTS np_vault_records (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID        NOT NULL,
    tenant_id        UUID,
    credential_kind  TEXT        NOT NULL,
    label            TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_vault_records_user_revoked
    ON np_vault_records (user_id, revoked_at)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_np_vault_records_tenant
    ON np_vault_records (tenant_id)
    WHERE tenant_id IS NOT NULL;

-- Per-device envelopes: one row per (record, device) pair
CREATE TABLE IF NOT EXISTS np_vault_envelopes (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id           UUID        NOT NULL REFERENCES np_vault_records(id) ON DELETE CASCADE,
    device_id           UUID        NOT NULL REFERENCES np_vault_devices(id) ON DELETE CASCADE,
    envelope_ciphertext BYTEA       NOT NULL,
    envelope_nonce      BYTEA       NOT NULL,
    sealed_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (record_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_np_vault_envelopes_record_device
    ON np_vault_envelopes (record_id, device_id);

CREATE INDEX IF NOT EXISTS idx_np_vault_envelopes_device
    ON np_vault_envelopes (device_id);

-- Immutable audit log — INSERT only, no UPDATE/DELETE granted to service role
CREATE TABLE IF NOT EXISTS np_vault_audit (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id   UUID,
    device_id   UUID,
    user_id     UUID        NOT NULL,
    tenant_id   UUID,
    action      TEXT        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata    JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_np_vault_audit_user_occurred
    ON np_vault_audit (user_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_np_vault_audit_tenant_occurred
    ON np_vault_audit (tenant_id, occurred_at DESC)
    WHERE tenant_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_np_vault_audit_record
    ON np_vault_audit (record_id)
    WHERE record_id IS NOT NULL;

-- Revoke UPDATE and DELETE on the audit table to enforce immutability
-- (service role may INSERT but not modify or delete entries)
REVOKE UPDATE, DELETE ON np_vault_audit FROM PUBLIC;

-- +goose Down
DROP TABLE IF EXISTS np_vault_audit;
DROP TABLE IF EXISTS np_vault_envelopes;
DROP TABLE IF EXISTS np_vault_records;
DROP TABLE IF EXISTS np_vault_devices;
