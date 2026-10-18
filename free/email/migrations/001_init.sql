-- plugin-email: initial schema
-- Multi-App Isolation: source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Tables live in public (the service queries them unqualified, Hasura tracks schema public). The boot runner
-- pins search_path to np_email first, so every object here is schema-qualified.

CREATE TABLE IF NOT EXISTS public.np_email_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    name TEXT NOT NULL,
    subject TEXT NOT NULL,
    body_html TEXT NOT NULL,
    body_text TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, name)
);

CREATE TABLE IF NOT EXISTS public.np_email_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    template_id UUID REFERENCES public.np_email_templates(id) ON DELETE SET NULL,
    to_address TEXT NOT NULL,
    from_address TEXT NOT NULL,
    subject TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued', -- queued, sent, failed, bounced
    provider_message_id TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_np_email_messages_sai ON public.np_email_messages (source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_email_messages_status ON public.np_email_messages (status);

CREATE TABLE IF NOT EXISTS public.np_email_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    message_id UUID REFERENCES public.np_email_messages(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL, -- delivered, opened, clicked, bounced, unsubscribed
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_np_email_events_sai ON public.np_email_events (source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_email_events_message ON public.np_email_events (message_id);
