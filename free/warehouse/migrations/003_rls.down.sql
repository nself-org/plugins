-- 003_rls.down.sql
-- Rollback: remove RLS policies and disable RLS for warehouse plugin tables.

-- np_warehouse_watermarks
DROP POLICY IF EXISTS "np_warehouse_watermarks_select_own" ON "public"."np_warehouse_watermarks";
DROP POLICY IF EXISTS "np_warehouse_watermarks_insert_own" ON "public"."np_warehouse_watermarks";
DROP POLICY IF EXISTS "np_warehouse_watermarks_update_own" ON "public"."np_warehouse_watermarks";
DROP POLICY IF EXISTS "np_warehouse_watermarks_delete_own" ON "public"."np_warehouse_watermarks";
ALTER TABLE "public"."np_warehouse_watermarks" DISABLE ROW LEVEL SECURITY;

-- np_warehouse_errors
DROP POLICY IF EXISTS "np_warehouse_errors_select_own" ON "public"."np_warehouse_errors";
DROP POLICY IF EXISTS "np_warehouse_errors_insert_own" ON "public"."np_warehouse_errors";
DROP POLICY IF EXISTS "np_warehouse_errors_update_own" ON "public"."np_warehouse_errors";
DROP POLICY IF EXISTS "np_warehouse_errors_delete_own" ON "public"."np_warehouse_errors";
ALTER TABLE "public"."np_warehouse_errors" DISABLE ROW LEVEL SECURITY;
