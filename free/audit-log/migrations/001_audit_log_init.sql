-- Migration: 001_audit_log_init
-- Plugin: audit-log
-- Description: Creates the np_auditlog_events append-only audit table,
--              monthly range partitions for performance, RLS policies for
--              multi-tenant isolation, and indexes for common query patterns.
--
-- Boot-applied by sdk/go/migrate with search_path pinned to np_audit_log, so every object is schema-qualified:
-- the table lives in public (the cli tracking rule). Idempotent (IF NOT EXISTS, guarded policy blocks): it
-- applies over a table the old Go code created, and the policies are never created twice.
--
-- IMPORTANT: This table is APPEND-ONLY.
--   - np_auditlog_no_delete policy: FOR DELETE USING (false)
--   - np_auditlog_no_update policy: FOR UPDATE USING (false)
-- No application code or database role should be granted UPDATE/DELETE on
-- np_auditlog_events. The service role used by this plugin should only have
-- INSERT + SELECT privileges.

-- ============================================================
-- Parent table (partitioned by created_at, monthly ranges)
-- ============================================================
CREATE TABLE IF NOT EXISTS public.np_auditlog_events (
    id                TEXT         NOT NULL,
    source_account_id TEXT         NOT NULL DEFAULT 'primary',
    actor_user_id     TEXT         NOT NULL DEFAULT '',
    actor_type        TEXT         NOT NULL,
    event_type        TEXT         NOT NULL,
    resource_type     TEXT         NOT NULL DEFAULT '',
    resource_id       TEXT         NOT NULL DEFAULT '',
    ip_address        TEXT         NOT NULL DEFAULT '',
    user_agent        TEXT         NOT NULL DEFAULT '',
    metadata          JSONB        NOT NULL DEFAULT '{}',
    severity          TEXT         NOT NULL DEFAULT 'info',
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- ============================================================
-- Default catch-all partition
-- Holds rows until a month-specific partition is created.
-- Use pg_partman or a cron job to create monthly partitions.
-- ============================================================
CREATE TABLE IF NOT EXISTS public.np_auditlog_events_default
    PARTITION OF public.np_auditlog_events DEFAULT;

-- ============================================================
-- Indexes
-- ============================================================

-- Primary query pattern: fetch all events for an account in a time range.
CREATE INDEX IF NOT EXISTS idx_np_auditlog_account_time
    ON public.np_auditlog_events (source_account_id, created_at DESC);

-- Filter by event type (e.g. "auth.login", "plugin.installed").
CREATE INDEX IF NOT EXISTS idx_np_auditlog_event_type
    ON public.np_auditlog_events (event_type);

-- Filter by the actor that triggered the event.
CREATE INDEX IF NOT EXISTS idx_np_auditlog_actor_user
    ON public.np_auditlog_events (actor_user_id);

-- ============================================================
-- Row-Level Security
-- ============================================================

ALTER TABLE public.np_auditlog_events ENABLE ROW LEVEL SECURITY;

-- SELECT: session may only read its own account's events.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE schemaname = 'public' AND tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_select'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_select ON public.np_auditlog_events
                FOR SELECT
                USING (source_account_id = current_setting('app.source_account_id', true))
        $policy$;
    END IF;
END $$;

-- INSERT: session may only write events for its own account.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE schemaname = 'public' AND tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_insert'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_insert ON public.np_auditlog_events
                FOR INSERT
                WITH CHECK (source_account_id = current_setting('app.source_account_id', true))
        $policy$;
    END IF;
END $$;

-- DELETE: always false — this table is append-only.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE schemaname = 'public' AND tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_no_delete'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_no_delete ON public.np_auditlog_events
                FOR DELETE
                USING (false)
        $policy$;
    END IF;
END $$;

-- UPDATE: always false — this table is append-only.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE schemaname = 'public' AND tablename = 'np_auditlog_events' AND policyname = 'np_auditlog_no_update'
    ) THEN
        EXECUTE $policy$
            CREATE POLICY np_auditlog_no_update ON public.np_auditlog_events
                FOR UPDATE
                USING (false)
        $policy$;
    END IF;
END $$;

-- ============================================================
-- Comments
-- ============================================================

COMMENT ON TABLE public.np_auditlog_events IS
    'Append-only audit log for nself security-relevant events. '
    'Partitioned by created_at (monthly). RLS enforces account isolation. '
    'DELETE and UPDATE are permanently disabled via policy.';

COMMENT ON COLUMN public.np_auditlog_events.actor_type IS
    'Who triggered the event: user | system | plugin';

COMMENT ON COLUMN public.np_auditlog_events.event_type IS
    'Semantic event category: auth.login | auth.logout | auth.login_failed | '
    'auth.mfa_enabled | privilege.change | secret.accessed | '
    'plugin.installed | plugin.uninstalled';

COMMENT ON COLUMN public.np_auditlog_events.severity IS
    'Event severity: info | warning | critical';

COMMENT ON COLUMN public.np_auditlog_events.metadata IS
    'Arbitrary JSON payload for event-specific context (e.g. old/new role, '
    'plugin name, secret key name). Never store raw secret values here.';
