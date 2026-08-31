-- plugin-clawde: PTY relay session tracking for nself-ai-cc :3760 (E7)
-- Multi-App Isolation Convention: source_account_id TEXT NOT NULL DEFAULT 'primary'

CREATE TABLE IF NOT EXISTS np_pty_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    session_id       TEXT UNIQUE NOT NULL,
    binary_path      TEXT,
    args             JSONB,
    status           TEXT NOT NULL DEFAULT 'active',
    started_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_pty_sessions_account
    ON np_pty_sessions (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_pty_sessions_session_id
    ON np_pty_sessions (session_id);

CREATE INDEX IF NOT EXISTS idx_np_pty_sessions_status
    ON np_pty_sessions (status);
