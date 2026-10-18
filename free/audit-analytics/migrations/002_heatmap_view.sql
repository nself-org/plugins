-- Migration 002: np_audit_heatmap materialized view.
-- Requires: public.np_audit_log (audit-log plugin) and public.np_audit_anomalies (migration 001).
-- The view reads np_audit_log, which belongs to the audit-log plugin. On a database where that table does not
-- exist yet the view is skipped (a fresh database must still boot); install audit-log first to get the heatmap.
-- Safe to re-run: the view and its indexes are created only when missing.

DO $mig$
BEGIN
    IF to_regclass('public.np_audit_log') IS NOT NULL AND to_regclass('public.np_audit_heatmap') IS NULL THEN
        EXECUTE $v$
            CREATE MATERIALIZED VIEW public.np_audit_heatmap AS
            SELECT
                tenant_id,
                user_id,
                EXTRACT(DOW  FROM created_at) AS day_of_week,   -- 0=Sunday, 6=Saturday
                EXTRACT(HOUR FROM created_at) AS hour_of_day,   -- 0-23 UTC
                COUNT(*)                      AS action_count
            FROM public.np_audit_log
            WHERE created_at > NOW() - INTERVAL '30 days'
            GROUP BY 1, 2, 3, 4
        $v$;
        EXECUTE $v$
            COMMENT ON MATERIALIZED VIEW public.np_audit_heatmap
                IS 'Rolling 30-day action count heatmap per (tenant, user, day_of_week, hour_of_day). Refreshed on schedule by the audit-analytics plugin.'
        $v$;
        -- Unique index required for REFRESH MATERIALIZED VIEW CONCURRENTLY; tenant_id can be NULL (non-cloud deploys).
        EXECUTE 'CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_heatmap_unique ON public.np_audit_heatmap (tenant_id, user_id, day_of_week, hour_of_day) NULLS NOT DISTINCT';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_audit_heatmap_user ON public.np_audit_heatmap (user_id)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_audit_heatmap_tenant ON public.np_audit_heatmap (tenant_id) WHERE tenant_id IS NOT NULL';
    END IF;
END
$mig$;
