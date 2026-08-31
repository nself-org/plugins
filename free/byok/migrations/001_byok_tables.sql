-- BYOK Per-Tenant Encryption: initial schema
-- Migration: 001_byok_tables.sql

-- Per-tenant KMS configuration
CREATE TABLE IF NOT EXISTS np_kms_configs (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        UUID REFERENCES np_tenants(id) ON DELETE CASCADE,
  provider         TEXT NOT NULL CHECK (provider IN ('aws', 'gcp', 'vault')),
  key_ref          TEXT NOT NULL,  -- KMS key ID / ARN / Vault key path
  region           TEXT,           -- AWS/GCP region (nullable for Vault)
  endpoint_url     TEXT,           -- Vault endpoint URL
  credentials_ref  TEXT,           -- np_secrets key name holding access credentials
  enabled          BOOLEAN NOT NULL DEFAULT TRUE,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_verified_at TIMESTAMPTZ,
  UNIQUE (tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_kms_configs_tenant  ON np_kms_configs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_kms_configs_enabled ON np_kms_configs(enabled, provider);

-- Generic encrypted values store (any plugin can use)
CREATE TABLE IF NOT EXISTS np_encrypted_values (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id      UUID REFERENCES np_tenants(id) ON DELETE CASCADE,
  record_group   TEXT NOT NULL,  -- logical grouping, e.g. 'claw_memories_batch_001'
  field_name     TEXT NOT NULL,
  encrypted_data BYTEA NOT NULL, -- AES-256-GCM ciphertext
  encrypted_dek  BYTEA NOT NULL, -- DEK wrapped by CMK
  kms_key_ref    TEXT NOT NULL,  -- which CMK wrapped this DEK
  iv             BYTEA NOT NULL CHECK (octet_length(iv) = 12),  -- 12-byte GCM nonce
  algorithm      TEXT NOT NULL DEFAULT 'AES-256-GCM',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_enc_values_tenant       ON np_encrypted_values(tenant_id);
CREATE INDEX IF NOT EXISTS idx_enc_values_record_group ON np_encrypted_values(tenant_id, record_group);
CREATE INDEX IF NOT EXISTS idx_enc_values_field        ON np_encrypted_values(tenant_id, field_name);

-- Key rotation and event audit trail
CREATE TABLE IF NOT EXISTS np_encryption_key_events (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID REFERENCES np_tenants(id) ON DELETE CASCADE,
  event_type   TEXT NOT NULL CHECK (event_type IN (
                 'key_configured', 'key_rotated', 'key_revoked',
                 'verify_ok', 'verify_fail',
                 'rotation_started', 'rotation_completed', 'rotation_failed'
               )),
  key_ref      TEXT NOT NULL,
  triggered_by UUID REFERENCES np_users(id) ON DELETE SET NULL,
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  details      JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_key_events_tenant    ON np_encryption_key_events(tenant_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_key_events_type      ON np_encryption_key_events(event_type, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_key_events_triggered ON np_encryption_key_events(triggered_by);
