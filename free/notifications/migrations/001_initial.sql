-- notifications plugin: initial schema
-- 3 tables: np_notifications_notifications, np_notifications_templates,
--           np_notifications_preferences
-- Boot-applied by sdk/go/migrate with search_path pinned to np_notifications, so every object is
-- schema-qualified: the tables live in public (the cli tracking rule). Idempotent.
-- source_account_id (Multi-Tenant Convention Wall) was never in the old Go schema, so each table also gets an
-- ADD COLUMN IF NOT EXISTS: an install whose tables the old Go code created gains the column in place.

CREATE TABLE IF NOT EXISTS public.np_notifications_notifications (
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

ALTER TABLE public.np_notifications_notifications ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_status ON public.np_notifications_notifications(status);
CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_channel ON public.np_notifications_notifications(channel);
CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_recipient ON public.np_notifications_notifications(recipient);
CREATE INDEX IF NOT EXISTS idx_np_notifications_notifications_source ON public.np_notifications_notifications(source_account_id);

CREATE TABLE IF NOT EXISTS public.np_notifications_templates (
    id               TEXT PRIMARY KEY,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    name             TEXT NOT NULL UNIQUE,
    channel          TEXT NOT NULL,
    subject_template TEXT NOT NULL DEFAULT '',
    body_template    TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE public.np_notifications_templates ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_name ON public.np_notifications_templates(name);
CREATE INDEX IF NOT EXISTS idx_np_notifications_templates_source ON public.np_notifications_templates(source_account_id);

-- The key stays (user_id): internal/db.go upserts ON CONFLICT (user_id), and installs created by the old Go code have that key.
CREATE TABLE IF NOT EXISTS public.np_notifications_preferences (
    user_id       TEXT NOT NULL,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    email_enabled BOOLEAN NOT NULL DEFAULT true,
    push_enabled  BOOLEAN NOT NULL DEFAULT true,
    sms_enabled   BOOLEAN NOT NULL DEFAULT true,
    quiet_start   TEXT,
    quiet_end     TEXT,
    channels      JSONB NOT NULL DEFAULT '{}',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id)
);

ALTER TABLE public.np_notifications_preferences ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_notifications_prefs_source ON public.np_notifications_preferences(source_account_id);
