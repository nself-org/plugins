-- notifications plugin: initial schema
-- CODE WINS: table names from internal/db.go (np_notifications_* prefix)
-- 3 tables: np_notifications_notifications, np_notifications_templates,
--           np_notifications_preferences
-- Adding source_account_id (not in Go schema but required by Multi-Tenant Convention Wall)

CREATE TABLE IF NOT EXISTS np_notifications_notifications (
    id         TEXT PRIMARY KEY,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    channel    TEXT NOT NULL,
    recipient  TEXT NOT NULL,
    template   TEXT NOT NULL DEFAULT '',
    data       JSONB NOT NULL DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'pending',
    sent_at    TIMESTAMPTZ,
    error      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_status ON np_notifications_notifications(status);
CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_channel ON np_notifications_notifications(channel);
CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_recipient ON np_notifications_notifications(recipient);
CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_source ON np_notifications_notifications(source_account_id);

CREATE TABLE IF NOT EXISTS np_notifications_templates (
    id               TEXT PRIMARY KEY,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    name             TEXT NOT NULL UNIQUE,
    channel          TEXT NOT NULL,
    subject_template TEXT NOT NULL DEFAULT '',
    body_template    TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_name ON np_notifications_templates(name);
CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_source ON np_notifications_templates(source_account_id);

CREATE TABLE IF NOT EXISTS np_notifications_preferences (
    user_id       TEXT NOT NULL,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    email_enabled BOOLEAN NOT NULL DEFAULT true,
    push_enabled  BOOLEAN NOT NULL DEFAULT true,
    sms_enabled   BOOLEAN NOT NULL DEFAULT true,
    quiet_start   TEXT,
    quiet_end     TEXT,
    channels      JSONB NOT NULL DEFAULT '{}',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_notifications_prefs_source ON np_notifications_preferences(source_account_id);
