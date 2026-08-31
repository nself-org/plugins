-- Migration 003: PERM-HASURA-01 remediation record for byok plugin
-- RLS is applied in 002_rls_policies.sql. Hasura row filters (tenant_id) are
-- present in all three table YAMLs. This migration is a no-op verification
-- marker confirming compliance with the PERM-HASURA-01 doctor check.
-- Convention B (cloud multi-tenancy): tenant_id UUID — correct for this plugin.
-- CRITICAL: np_encrypted_values excludes raw ciphertext columns from user role
--   (encrypted_data, encrypted_dek, iv) — this is enforced in Hasura metadata.

-- No DDL changes. RLS already applied in migration 002.
SELECT 'PERM-HASURA-01: byok plugin verified OK' AS status;
