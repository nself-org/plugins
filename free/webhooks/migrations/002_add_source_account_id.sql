-- Migration: Add source_account_id to webhooks tables (Multi-App Isolation)
-- Required by the Multi-Tenant Convention Wall hard rule.
-- All np_* tables must have source_account_id TEXT NOT NULL DEFAULT 'primary'.

BEGIN;

ALTER TABLE IF EXISTS np_webhooks_endpoints
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

ALTER TABLE IF EXISTS np_webhooks_deliveries
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_webhooks_endpoints_account
  ON np_webhooks_endpoints (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_webhooks_deliveries_account
  ON np_webhooks_deliveries (source_account_id);

COMMIT;
