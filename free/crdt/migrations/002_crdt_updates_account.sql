-- CRDT plugin: tenant column on the update log
-- Migration: 002_crdt_updates_account
-- The manifest declares source_account_id isolation on every table. Installs created by the old in-code
-- Migrate lack it on np_crdt_updates; fresh installs and existing ones both get it here.

ALTER TABLE public.np_crdt_updates
  ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_crdt_updates_account
  ON public.np_crdt_updates (source_account_id);
