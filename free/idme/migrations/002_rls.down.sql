-- 002_rls.down.sql
-- Rollback: remove RLS policies and disable RLS for idme plugin tables.

-- idme_verifications
DROP POLICY IF EXISTS "idme_verifications_select_own" ON "public"."idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_insert_own" ON "public"."idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_update_own" ON "public"."idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_delete_own" ON "public"."idme_verifications";
ALTER TABLE "public"."idme_verifications" DISABLE ROW LEVEL SECURITY;
