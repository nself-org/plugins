-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 002_rls.down.sql
-- Rollback: remove RLS policies and disable RLS for idme plugin tables.

-- np_idme_verifications
DROP POLICY IF EXISTS "idme_verifications_select_own" ON "public"."np_idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_insert_own" ON "public"."np_idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_update_own" ON "public"."np_idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_delete_own" ON "public"."np_idme_verifications";
ALTER TABLE "public"."np_idme_verifications" DISABLE ROW LEVEL SECURITY;
