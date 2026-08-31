-- plugin-sms: initial schema
-- Multi-App Isolation: source_account_id TEXT NOT NULL DEFAULT 'primary'

CREATE TABLE IF NOT EXISTS np_sms_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    to_number TEXT NOT NULL,       -- E.164 format e.g. +14155552671
    from_number TEXT NOT NULL,     -- E.164 format
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',  -- queued, sent, failed, delivered, undelivered
    provider_sid TEXT,             -- Twilio message SID
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_sms_messages_sai ON np_sms_messages (source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_sms_messages_to ON np_sms_messages (to_number);
CREATE INDEX IF NOT EXISTS idx_np_sms_messages_created ON np_sms_messages (created_at);

CREATE TABLE IF NOT EXISTS np_sms_opt_outs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    phone_number TEXT NOT NULL,    -- E.164 format
    opted_out_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, phone_number)
);

CREATE INDEX IF NOT EXISTS idx_np_sms_opt_outs_sai ON np_sms_opt_outs (source_account_id);
