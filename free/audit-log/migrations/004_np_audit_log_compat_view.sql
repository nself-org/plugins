-- Migration: 004_np_audit_log_compat_view
-- Plugin: audit-log (free)
-- Description: Publishes public.np_audit_log, the read model that audit-analytics and siem query.
--              No plugin created that name before; audit-log writes public.np_auditlog_events (hq debt D-0959).
--
-- Canonical store: np_auditlog_events. np_audit_log is a view over it, so there is one copy of the data.
-- Column map (reader name <- np_auditlog_events):
--   id           <- id            uuid; NULL when the stored id is not a uuid (the plugin always writes uuids)
--   tenant_id    <- tenant_id     uuid, NULL on single-tenant deploys
--   user_id      <- actor_user_id uuid; NULL when empty or not a uuid
--   action       <- event_type
--   table_name   <- resource_type
--   record_id    <- resource_id
--   session_data <- metadata plus user_name and ip_address when present
--   context      <- {"ip": ip_address}, ip omitted when empty
--   created_at   <- created_at
-- The view runs with its owner's rights, so analytics reads every account's events (the RLS policies on the table
-- stay in force for sessions that query the table directly). It is read-only: no INSERT, UPDATE or DELETE rule.

CREATE OR REPLACE VIEW public.np_audit_log AS
SELECT
    CASE WHEN e.id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN e.id::uuid END AS id,
    e.tenant_id,
    CASE WHEN e.actor_user_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN e.actor_user_id::uuid END AS user_id,
    e.event_type    AS action,
    e.resource_type AS table_name,
    e.resource_id   AS record_id,
    e.metadata
        || jsonb_strip_nulls(jsonb_build_object(
               'user_name',  NULLIF(e.metadata->>'user_name', ''),
               'ip_address', NULLIF(e.ip_address, ''))) AS session_data,
    jsonb_strip_nulls(jsonb_build_object('ip', NULLIF(e.ip_address, ''))) AS context,
    e.created_at
FROM public.np_auditlog_events e;

COMMENT ON VIEW public.np_audit_log IS
    'Read model over np_auditlog_events for audit-analytics and siem. Owned by the audit-log plugin; read-only.';
