-- Rollback of 001_sync_schema (not applied at boot).
DROP MATERIALIZED VIEW IF EXISTS public.np_sync_state;
DROP TABLE IF EXISTS public.np_sync_cursors;
DROP TABLE IF EXISTS public.np_devices;
DROP TABLE IF EXISTS public.np_sync_events;
