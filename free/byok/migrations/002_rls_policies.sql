-- Migration 002: Enable RLS on np_byok_* tables (tenant_id isolation)
-- PERM-HASURA-01 remediation — Postgres RLS was missing from byok tables.
-- Convention B (cloud multi-tenancy): tenant_id UUID per the Convention Wall.
-- These tables hold sensitive key material; RLS is a critical defense layer.

BEGIN;

-- ============================================================================
-- np_kms_configs
-- ============================================================================

ALTER TABLE np_kms_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_kms_configs FORCE ROW LEVEL SECURITY;

-- nself_admin (service role) bypasses RLS via BYPASSRLS privilege.
-- Tenant users may only see their own KMS config.
DROP POLICY IF EXISTS rls_np_kms_configs_tenant ON np_kms_configs;
CREATE POLICY rls_np_kms_configs_tenant ON np_kms_configs
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid);

-- ============================================================================
-- np_encrypted_values
-- ============================================================================

ALTER TABLE np_encrypted_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_encrypted_values FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_encrypted_values_tenant ON np_encrypted_values;
CREATE POLICY rls_np_encrypted_values_tenant ON np_encrypted_values
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid);

-- ============================================================================
-- np_encryption_key_events
-- ============================================================================

ALTER TABLE np_encryption_key_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_encryption_key_events FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_encryption_key_events_tenant ON np_encryption_key_events;
CREATE POLICY rls_np_encryption_key_events_tenant ON np_encryption_key_events
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid);

COMMIT;
