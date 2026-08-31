-- plugin-clawde: session management and event log tables
-- Multi-App Isolation Convention: source_account_id TEXT NOT NULL DEFAULT 'primary'

-- np_clawde_sessions: tracks active/closed ClawDE sessions per account
CREATE TABLE IF NOT EXISTS np_clawde_sessions (
    id               TEXT NOT NULL,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    status           TEXT NOT NULL DEFAULT 'active'
                     CHECK (status IN ('active', 'closed', 'expired')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at        TIMESTAMPTZ,
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_clawde_sessions_account_status
    ON np_clawde_sessions (source_account_id, status);

-- np_clawde_events: append-only event log for ClawDE session events
CREATE TABLE IF NOT EXISTS np_clawde_events (
    id               BIGSERIAL PRIMARY KEY,
    session_id       TEXT NOT NULL,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    event_type       TEXT NOT NULL,
    payload          TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_clawde_events_session
    ON np_clawde_events (session_id, source_account_id, created_at);

CREATE INDEX IF NOT EXISTS idx_np_clawde_events_account
    ON np_clawde_events (source_account_id, created_at);
