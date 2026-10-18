-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 003_heatmap_view_retry.down.sql
-- Reverse of 003_heatmap_view_retry.sql: the view is shared with 002, so the down step is the same drop.

DROP MATERIALIZED VIEW IF EXISTS np_audit_heatmap;
