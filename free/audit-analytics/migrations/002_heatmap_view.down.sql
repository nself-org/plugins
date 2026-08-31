-- 002_heatmap_view.down.sql
-- Reverse of 002_heatmap_view.sql

-- Drop indexes first (dropped automatically with the view, but explicit for clarity).
DROP MATERIALIZED VIEW IF EXISTS np_audit_heatmap;
