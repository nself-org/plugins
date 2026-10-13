-- Migration: Add source_account_id to ollama tables (Multi-App Isolation)
-- Required by the Multi-Tenant Convention Wall hard rule.
-- Boot-applied by sdk/go/migrate (its own transaction, search_path pinned to np_ollama): every object is qualified with public.

ALTER TABLE IF EXISTS public.np_ollama_model_registry
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_ollama_model_registry_account
  ON public.np_ollama_model_registry (source_account_id);
