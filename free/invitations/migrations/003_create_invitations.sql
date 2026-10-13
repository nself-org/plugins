-- Migration 003: the np_invitations_invitations table and its indexes.
-- This DDL used to run from Go code at start (Migrate in internal/db.go); it lives here now. Idempotent: it applies over a
-- table an earlier release created (002 has already added source_account_id there). Boot-applied with search_path pinned
-- to np_invitations, so every object is schema-qualified: the table lives in public (the cli tracking rule).
CREATE TABLE IF NOT EXISTS public.np_invitations_invitations (
    id                TEXT PRIMARY KEY,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    email             TEXT NOT NULL,
    role              TEXT NOT NULL DEFAULT 'member',
    token             TEXT NOT NULL UNIQUE,
    status            TEXT NOT NULL DEFAULT 'pending',
    invited_by        TEXT NOT NULL,
    expires_at        TIMESTAMPTZ,
    accepted_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_invitations_status
    ON public.np_invitations_invitations (status);
CREATE INDEX IF NOT EXISTS idx_np_invitations_email
    ON public.np_invitations_invitations (email);
CREATE INDEX IF NOT EXISTS idx_np_invitations_token
    ON public.np_invitations_invitations (token);
CREATE INDEX IF NOT EXISTS idx_np_invitations_source_account
    ON public.np_invitations_invitations (source_account_id);
