-- 003_rls.sql
-- Row-Level Security for event-bus plugin tables.
-- Convention Wall (Multi-App Isolation): source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Pattern: PatternPublic with source_account_id isolation (internal stats table).
-- Prerequisite: 002_source_account_id.sql (T04) must have run first.

-- ──────────────────────────────────────────────────────────────────────────────
-- np_event_bus_stats
-- ──────────────────────────────────────────────────────────────────────────────
ALTER TABLE "public"."np_event_bus_stats" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "public"."np_event_bus_stats" FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "np_event_bus_stats_select_own" ON "public"."np_event_bus_stats";
DROP POLICY IF EXISTS "np_event_bus_stats_insert_own" ON "public"."np_event_bus_stats";
DROP POLICY IF EXISTS "np_event_bus_stats_update_own" ON "public"."np_event_bus_stats";
DROP POLICY IF EXISTS "np_event_bus_stats_delete_own" ON "public"."np_event_bus_stats";

CREATE POLICY "np_event_bus_stats_select_own" ON "public"."np_event_bus_stats"
  FOR SELECT USING (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_event_bus_stats_insert_own" ON "public"."np_event_bus_stats"
  FOR INSERT WITH CHECK (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_event_bus_stats_update_own" ON "public"."np_event_bus_stats"
  FOR UPDATE USING (
    source_account_id = current_setting('app.source_account_id', true)
  ) WITH CHECK (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_event_bus_stats_delete_own" ON "public"."np_event_bus_stats"
  FOR DELETE USING (
    source_account_id = current_setting('app.source_account_id', true)
  );
