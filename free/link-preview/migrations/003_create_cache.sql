-- Migration 003: the np_link_preview_cache table and its indexes.
-- This DDL used to run from Go code at start (Migrate in internal/db.go); it lives here now. Idempotent: it applies over a
-- table an earlier release created (002 has already added source_account_id there). Boot-applied with search_path pinned
-- to np_link_preview, so every object is schema-qualified: the table lives in public (the cli tracking rule).
CREATE TABLE IF NOT EXISTS public.np_link_preview_cache (
    id                TEXT PRIMARY KEY,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    url               TEXT NOT NULL,
    title             TEXT,
    description       TEXT,
    image             TEXT,
    site_name         TEXT,
    type              TEXT,
    fetched_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_link_preview_cache_url
    ON public.np_link_preview_cache (url);
CREATE INDEX IF NOT EXISTS idx_np_link_preview_cache_expires_at
    ON public.np_link_preview_cache (expires_at);
CREATE INDEX IF NOT EXISTS idx_np_link_preview_cache_account
    ON public.np_link_preview_cache (source_account_id);
