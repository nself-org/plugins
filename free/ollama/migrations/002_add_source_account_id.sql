-- Migration: Add source_account_id to ollama tables (Multi-App Isolation)
-- Required by the Multi-Tenant Convention Wall hard rule.

BEGIN;

ALTER TABLE IF EXISTS np_ollama_model_registry
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_ollama_model_registry_account
  ON np_ollama_model_registry (source_account_id);

COMMIT;
