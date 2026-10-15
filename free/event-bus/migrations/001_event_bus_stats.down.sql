-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_event_bus_stats.down.sql
-- Reverse of 001_event_bus_stats.sql

DROP TABLE IF EXISTS np_event_bus_stats;
