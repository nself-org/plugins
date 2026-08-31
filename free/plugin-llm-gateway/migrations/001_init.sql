-- plugin-llm-gateway: request tracking and quota tables
-- Multi-App Isolation Convention: source_account_id TEXT NOT NULL DEFAULT 'primary'

-- np_llm_gateway_requests: per-request audit log with session context
CREATE TABLE IF NOT EXISTS np_llm_gateway_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    session_id       TEXT NOT NULL DEFAULT '',
    model            TEXT NOT NULL DEFAULT '',
    tokens_in        INTEGER NOT NULL DEFAULT 0,
    tokens_out       INTEGER NOT NULL DEFAULT 0,
    cached           BOOLEAN NOT NULL DEFAULT FALSE,
    context          TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_llm_gateway_requests_account
    ON np_llm_gateway_requests (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_llm_gateway_requests_session
    ON np_llm_gateway_requests (source_account_id, session_id);

-- np_llm_gateway_quota_usage: daily token quota per tenant (atomic via ON CONFLICT)
CREATE TABLE IF NOT EXISTS np_llm_gateway_quota_usage (
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    quota_date        DATE NOT NULL DEFAULT CURRENT_DATE,
    tokens_used       BIGINT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_account_id, quota_date)
);

CREATE INDEX IF NOT EXISTS idx_np_llm_gateway_quota_account
    ON np_llm_gateway_quota_usage (source_account_id);
