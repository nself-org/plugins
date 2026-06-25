-- Migration: Add source_account_id to link-preview tables (Multi-App Isolation)
-- Required by the Multi-Tenant Convention Wall hard rule.
-- All np_* tables must have source_account_id TEXT NOT NULL DEFAULT 'primary'.

BEGIN;

ALTER TABLE IF EXISTS np_link_preview_cache
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_link_preview_cache_account
  ON np_link_preview_cache (source_account_id);

COMMIT;
