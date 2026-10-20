-- Creates the np_audit_heatmap materialized view when it is missing and its source (public.np_audit_log, the
-- audit-log reader view) exists. Idempotent. Mirrors migrations/002_heatmap_view.sql, which runs once and skips
-- when the source is absent at that time; HeatmapRefresher.Refresh runs this before every refresh so a view
-- skipped at boot appears as soon as audit-log has created its source (hq quality debt D-0959).
DO $ensure$
BEGIN
    IF to_regclass('public.np_audit_log') IS NOT NULL AND to_regclass('public.np_audit_heatmap') IS NULL THEN
        EXECUTE $v$
            CREATE MATERIALIZED VIEW public.np_audit_heatmap AS
            SELECT
                tenant_id,
                user_id,
                EXTRACT(DOW  FROM created_at) AS day_of_week,
                EXTRACT(HOUR FROM created_at) AS hour_of_day,
                COUNT(*)                      AS action_count
            FROM public.np_audit_log
            WHERE created_at > NOW() - INTERVAL '30 days'
            GROUP BY 1, 2, 3, 4
        $v$;
        EXECUTE 'CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_heatmap_unique ON public.np_audit_heatmap (tenant_id, user_id, day_of_week, hour_of_day) NULLS NOT DISTINCT';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_audit_heatmap_user ON public.np_audit_heatmap (user_id)';
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_audit_heatmap_tenant ON public.np_audit_heatmap (tenant_id) WHERE tenant_id IS NOT NULL';
    END IF;
END
$ensure$;
