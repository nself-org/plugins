-- Migration: 001_initial
-- Plugin: notify
-- Description: Creates public.np_notify_notifications and public.np_notify_templates tables.
-- Multi-Tenant Convention Wall: source_account_id in both tables.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS public.np_notify_notifications (
    id                TEXT         PRIMARY KEY,
    source_account_id TEXT         NOT NULL DEFAULT 'primary',
    channel           TEXT         NOT NULL,
    recipient         TEXT         NOT NULL,
    subject           TEXT         NOT NULL DEFAULT '',
    body              TEXT         NOT NULL DEFAULT '',
    status            TEXT         NOT NULL DEFAULT 'pending',
    sent_at           TIMESTAMPTZ,
    error             TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
-- Installs created by the former Go DDL lack source_account_id: add it before any index uses it.
ALTER TABLE public.np_notify_notifications ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_notify_notifications_status
    ON public.np_notify_notifications(status);
CREATE INDEX IF NOT EXISTS idx_np_notify_notifications_channel
    ON public.np_notify_notifications(channel);
CREATE INDEX IF NOT EXISTS idx_np_notify_notifications_account
    ON public.np_notify_notifications(source_account_id);

CREATE TABLE IF NOT EXISTS public.np_notify_templates (
    id               TEXT         PRIMARY KEY,
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    name             TEXT         NOT NULL UNIQUE,
    channel          TEXT         NOT NULL,
    subject_template TEXT         NOT NULL DEFAULT '',
    body_template    TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
-- Installs created by the former Go DDL lack source_account_id: add it before any index uses it.
ALTER TABLE public.np_notify_templates ADD COLUMN IF NOT EXISTS source_account_id TEXT NOT NULL DEFAULT 'primary';

CREATE INDEX IF NOT EXISTS idx_np_notify_templates_name
    ON public.np_notify_templates(name);
CREATE INDEX IF NOT EXISTS idx_np_notify_templates_account
    ON public.np_notify_templates(source_account_id);
