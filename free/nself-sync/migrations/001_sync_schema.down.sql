-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Rollback of 001_sync_schema (not applied at boot).
DROP MATERIALIZED VIEW IF EXISTS public.np_sync_state;
DROP TABLE IF EXISTS public.np_sync_cursors;
DROP TABLE IF EXISTS public.np_devices;
DROP TABLE IF EXISTS public.np_sync_events;
