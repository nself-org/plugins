-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- E10: nCloud Usage Metering — rollback
DROP TABLE IF EXISTS np_cloud_usage_alerts;
DROP TABLE IF EXISTS np_cloud_usage_reported;
DROP TABLE IF EXISTS np_cloud_usage_raw;
DROP TABLE IF EXISTS np_cloud_tenants;
