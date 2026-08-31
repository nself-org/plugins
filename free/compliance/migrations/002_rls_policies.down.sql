-- Rollback 002: Drop RLS on np_compliance_* tables

BEGIN;

DROP POLICY IF EXISTS rls_np_change_log_tenant      ON np_change_log;
DROP POLICY IF EXISTS rls_np_access_reviews_tenant  ON np_access_reviews;
DROP POLICY IF EXISTS rls_np_vendor_registry_tenant ON np_vendor_registry;

ALTER TABLE np_change_log      DISABLE ROW LEVEL SECURITY;
ALTER TABLE np_access_reviews  DISABLE ROW LEVEL SECURITY;
ALTER TABLE np_vendor_registry DISABLE ROW LEVEL SECURITY;

COMMIT;
