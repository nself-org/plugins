-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_soc2_tables.down.sql
-- Reverse of 001_soc2_tables.sql

-- Drop in declaration order (no FK references between these three tables).
DROP TABLE IF EXISTS np_change_log;
DROP TABLE IF EXISTS np_access_reviews;
DROP TABLE IF EXISTS np_vendor_registry;
