-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 003_rls.down.sql
-- Rollback: remove RLS policies and disable RLS for event-bus plugin tables.

-- np_event_bus_stats
DROP POLICY IF EXISTS "np_event_bus_stats_select_own" ON "public"."np_event_bus_stats";
DROP POLICY IF EXISTS "np_event_bus_stats_insert_own" ON "public"."np_event_bus_stats";
DROP POLICY IF EXISTS "np_event_bus_stats_update_own" ON "public"."np_event_bus_stats";
DROP POLICY IF EXISTS "np_event_bus_stats_delete_own" ON "public"."np_event_bus_stats";
ALTER TABLE "public"."np_event_bus_stats" DISABLE ROW LEVEL SECURITY;
