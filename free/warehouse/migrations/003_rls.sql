-- 003_rls.sql
-- Row-Level Security for warehouse plugin tables.
-- Convention Wall (Multi-App Isolation): source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Pattern: PatternPublic with source_account_id isolation (internal tracking tables).
-- Prerequisite: 002_source_account_id.sql (T04) must have run first.

-- ──────────────────────────────────────────────────────────────────────────────
-- np_warehouse_watermarks
-- ──────────────────────────────────────────────────────────────────────────────
ALTER TABLE "public"."np_warehouse_watermarks" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "public"."np_warehouse_watermarks" FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "np_warehouse_watermarks_select_own" ON "public"."np_warehouse_watermarks";
DROP POLICY IF EXISTS "np_warehouse_watermarks_insert_own" ON "public"."np_warehouse_watermarks";
DROP POLICY IF EXISTS "np_warehouse_watermarks_update_own" ON "public"."np_warehouse_watermarks";
DROP POLICY IF EXISTS "np_warehouse_watermarks_delete_own" ON "public"."np_warehouse_watermarks";

CREATE POLICY "np_warehouse_watermarks_select_own" ON "public"."np_warehouse_watermarks"
  FOR SELECT USING (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_warehouse_watermarks_insert_own" ON "public"."np_warehouse_watermarks"
  FOR INSERT WITH CHECK (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_warehouse_watermarks_update_own" ON "public"."np_warehouse_watermarks"
  FOR UPDATE USING (
    source_account_id = current_setting('app.source_account_id', true)
  ) WITH CHECK (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_warehouse_watermarks_delete_own" ON "public"."np_warehouse_watermarks"
  FOR DELETE USING (
    source_account_id = current_setting('app.source_account_id', true)
  );

-- ──────────────────────────────────────────────────────────────────────────────
-- np_warehouse_errors
-- ──────────────────────────────────────────────────────────────────────────────
ALTER TABLE "public"."np_warehouse_errors" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "public"."np_warehouse_errors" FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "np_warehouse_errors_select_own" ON "public"."np_warehouse_errors";
DROP POLICY IF EXISTS "np_warehouse_errors_insert_own" ON "public"."np_warehouse_errors";
DROP POLICY IF EXISTS "np_warehouse_errors_update_own" ON "public"."np_warehouse_errors";
DROP POLICY IF EXISTS "np_warehouse_errors_delete_own" ON "public"."np_warehouse_errors";

CREATE POLICY "np_warehouse_errors_select_own" ON "public"."np_warehouse_errors"
  FOR SELECT USING (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_warehouse_errors_insert_own" ON "public"."np_warehouse_errors"
  FOR INSERT WITH CHECK (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_warehouse_errors_update_own" ON "public"."np_warehouse_errors"
  FOR UPDATE USING (
    source_account_id = current_setting('app.source_account_id', true)
  ) WITH CHECK (
    source_account_id = current_setting('app.source_account_id', true)
  );

CREATE POLICY "np_warehouse_errors_delete_own" ON "public"."np_warehouse_errors"
  FOR DELETE USING (
    source_account_id = current_setting('app.source_account_id', true)
  );
