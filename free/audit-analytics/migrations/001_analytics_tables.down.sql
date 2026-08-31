-- 001_analytics_tables.down.sql
-- Reverse of 001_analytics_tables.sql
-- NOTE: Run 002_heatmap_view.down.sql BEFORE this file because
-- np_audit_heatmap materialized view depends on np_audit_anomalies.

DROP TABLE IF EXISTS np_audit_privileged_reviews;
DROP TABLE IF EXISTS np_audit_anomalies;
