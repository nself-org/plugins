-- Migration 002: add source_account_id to the link-preview cache (Multi-App Isolation, Multi-Tenant Convention Wall).
-- Additive only, safe on a running install. Applies to the table an earlier Go release created; on a fresh database
-- the table does not exist yet (003 creates it, with the column), so IF EXISTS makes this a no-op there.
-- The account index is created by 003, after the table is guaranteed to exist.
ALTER TABLE IF EXISTS public.np_link_preview_cache
    ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';
