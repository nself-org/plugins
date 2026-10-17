-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_analytics_tables.down.sql
-- Reverse of 001_analytics_tables.sql
-- NOTE: Run 002_heatmap_view.down.sql BEFORE this file because
-- np_audit_heatmap materialized view depends on np_audit_anomalies.

DROP TABLE IF EXISTS np_audit_privileged_reviews;
DROP TABLE IF EXISTS np_audit_anomalies;
