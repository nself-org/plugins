-- 002_rls.sql
-- Row-Level Security for idme plugin tables.
-- Convention Wall (Multi-App Isolation): source_account_id TEXT NOT NULL DEFAULT 'primary'
-- Pattern: PatternUserOwned for idme_verifications (user_id column present).
-- Prerequisite: 001_source_account_id.sql (T04) must have run first.

-- ──────────────────────────────────────────────────────────────────────────────
-- idme_verifications (PatternUserOwned — user_id column present)
-- ──────────────────────────────────────────────────────────────────────────────
ALTER TABLE "public"."idme_verifications" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "public"."idme_verifications" FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "idme_verifications_select_own" ON "public"."idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_insert_own" ON "public"."idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_update_own" ON "public"."idme_verifications";
DROP POLICY IF EXISTS "idme_verifications_delete_own" ON "public"."idme_verifications";

CREATE POLICY "idme_verifications_select_own" ON "public"."idme_verifications"
  FOR SELECT USING (
    (current_setting('hasura.user', true))::uuid = user_id
    OR current_setting('hasura.role', true) = 'admin'
  );

CREATE POLICY "idme_verifications_insert_own" ON "public"."idme_verifications"
  FOR INSERT WITH CHECK (
    (current_setting('hasura.user', true))::uuid = user_id
    OR current_setting('hasura.role', true) = 'admin'
  );

CREATE POLICY "idme_verifications_update_own" ON "public"."idme_verifications"
  FOR UPDATE USING (
    (current_setting('hasura.user', true))::uuid = user_id
    OR current_setting('hasura.role', true) = 'admin'
  ) WITH CHECK (
    (current_setting('hasura.user', true))::uuid = user_id
    OR current_setting('hasura.role', true) = 'admin'
  );

CREATE POLICY "idme_verifications_delete_own" ON "public"."idme_verifications"
  FOR DELETE USING (
    (current_setting('hasura.user', true))::uuid = user_id
    OR current_setting('hasura.role', true) = 'admin'
  );
