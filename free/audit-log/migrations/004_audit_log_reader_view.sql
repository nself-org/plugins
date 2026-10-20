-- Migration: 004_audit_log_reader_view
-- Plugin: audit-log (free)
-- Description: public.np_audit_log, a read-only view over the canonical table public.np_auditlog_events, for the
--              readers that predate it (audit-analytics anomaly and top-actor queries, the heatmap view, siem).
--
-- np_auditlog_events is the canonical audit table (hq quality debt D-0959). The view keeps the column names those
-- readers use: id, tenant_id, user_id, action, table_name, record_id, session_data, context, created_at.
--   id, user_id   uuid, NULL when the stored text is not a uuid (events carry free-form actor ids; the readers'
--                 tables store uuids)
--   action        event_type
--   table_name    resource_type
--   record_id     resource_id
--   session_data  {user_name, ip_address}
--   context       metadata plus ip (the ip_address column, else metadata.ip)
-- The view is read-only by construction (a projection with expressions); the table stays append-only.
-- security_invoker (PostgreSQL 15+, the stack baseline is 16) runs the view with the caller's privileges, so the
-- table's row-level security still applies to whoever reads the view.

CREATE OR REPLACE VIEW public.np_audit_log WITH (security_invoker = true) AS
SELECT
    CASE WHEN e.id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN e.id::uuid END AS id,
    e.tenant_id,
    CASE WHEN e.actor_user_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN e.actor_user_id::uuid END AS user_id,
    e.event_type AS action,
    e.resource_type AS table_name,
    e.resource_id AS record_id,
    jsonb_build_object('user_name', e.metadata->>'user_name', 'ip_address', NULLIF(e.ip_address, '')) AS session_data,
    e.metadata || jsonb_build_object('ip', COALESCE(NULLIF(e.ip_address, ''), e.metadata->>'ip')) AS context,
    e.created_at
FROM public.np_auditlog_events e;

COMMENT ON VIEW public.np_audit_log IS 'Read-only projection of np_auditlog_events for the audit-analytics and siem readers (D-0959).';
