-- Migration 002: np_audit_heatmap materialized view.
-- Requires: np_audit_log (core schema), np_audit_anomalies (migration 001).
-- REFRESH MATERIALIZED VIEW CONCURRENTLY requires a unique index — added below.
-- Safe to re-run: DROP + CREATE IF NOT EXISTS pair is idempotent.

-- Drop existing view if it exists (allows clean re-apply during dev).
DROP MATERIALIZED VIEW IF EXISTS np_audit_heatmap;

CREATE MATERIALIZED VIEW np_audit_heatmap AS
SELECT
    tenant_id,
    user_id,
    EXTRACT(DOW  FROM created_at) AS day_of_week,   -- 0=Sunday, 6=Saturday
    EXTRACT(HOUR FROM created_at) AS hour_of_day,   -- 0–23 UTC
    COUNT(*)                      AS action_count
FROM np_audit_log
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY 1, 2, 3, 4;

COMMENT ON MATERIALIZED VIEW np_audit_heatmap
    IS 'Rolling 30-day action count heatmap per (tenant, user, day_of_week, hour_of_day). '
       'Refreshed on schedule by audit-analytics plugin (CONCURRENT — does not block reads).';

-- Unique index required for REFRESH CONCURRENTLY.
-- The composite natural key is (tenant_id, user_id, day_of_week, hour_of_day).
-- tenant_id can be NULL (non-cloud deploys), so we include it in the partial form.
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_heatmap_unique
    ON np_audit_heatmap (tenant_id, user_id, day_of_week, hour_of_day)
    NULLS NOT DISTINCT;

-- Fast lookups by user.
CREATE INDEX IF NOT EXISTS idx_audit_heatmap_user
    ON np_audit_heatmap (user_id);

-- Fast lookups by tenant.
CREATE INDEX IF NOT EXISTS idx_audit_heatmap_tenant
    ON np_audit_heatmap (tenant_id)
    WHERE tenant_id IS NOT NULL;
