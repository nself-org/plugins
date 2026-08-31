-- Migration 002: Enable RLS on np_compliance_* tables (tenant_id isolation)
-- PERM-HASURA-01 remediation — Postgres RLS was missing from compliance tables.
-- Convention B (cloud multi-tenancy): tenant_id UUID per the Convention Wall.

BEGIN;

-- ============================================================================
-- np_change_log
-- ============================================================================

ALTER TABLE np_change_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_change_log FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_change_log_tenant ON np_change_log;
CREATE POLICY rls_np_change_log_tenant ON np_change_log
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)::uuid
        OR current_setting('app.current_tenant_id', true) IS NULL
    );

-- ============================================================================
-- np_access_reviews
-- ============================================================================

ALTER TABLE np_access_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_access_reviews FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_access_reviews_tenant ON np_access_reviews;
CREATE POLICY rls_np_access_reviews_tenant ON np_access_reviews
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)::uuid
        OR current_setting('app.current_tenant_id', true) IS NULL
    );

-- ============================================================================
-- np_vendor_registry
-- ============================================================================

ALTER TABLE np_vendor_registry ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_vendor_registry FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_vendor_registry_tenant ON np_vendor_registry;
CREATE POLICY rls_np_vendor_registry_tenant ON np_vendor_registry
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)::uuid
        OR current_setting('app.current_tenant_id', true) IS NULL
    );

COMMIT;
