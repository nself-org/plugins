-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 002_heatmap_view.down.sql
-- Reverse of 002_heatmap_view.sql

-- Drop indexes first (dropped automatically with the view, but explicit for clarity).
DROP MATERIALIZED VIEW IF EXISTS np_audit_heatmap;
