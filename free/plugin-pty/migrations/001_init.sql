-- plugin-pty: PTY session and audit log tables
-- Multi-App Isolation Convention: source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Boot-applied by sdk/go/migrate with search_path pinned to np_plugin_pty, so every object is schema-qualified: the tables live in public.
-- Idempotent (IF NOT EXISTS): it applies over tables an earlier install already created.

CREATE TABLE IF NOT EXISTS public.np_pty_sessions (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    client_id         TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'active'
                                  CHECK (status IN ('active', 'closed', 'expired')),
    cols              INTEGER     NOT NULL DEFAULT 80,
    rows              INTEGER     NOT NULL DEFAULT 24,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_pty_sessions_sai
    ON public.np_pty_sessions (source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_pty_sessions_status
    ON public.np_pty_sessions (source_account_id, status);

CREATE TABLE IF NOT EXISTS public.np_pty_audit_log (
    id                BIGSERIAL   PRIMARY KEY,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    session_id        UUID        REFERENCES public.np_pty_sessions(id) ON DELETE SET NULL,
    event_type        TEXT        NOT NULL,   -- spawn, close, resize, error
    detail            TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_pty_audit_sai
    ON public.np_pty_audit_log (source_account_id, created_at);
CREATE INDEX IF NOT EXISTS idx_np_pty_audit_session
    ON public.np_pty_audit_log (session_id, created_at);
