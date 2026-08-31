-- Rollback 002: Drop RLS on np_byok_* tables

BEGIN;

DROP POLICY IF EXISTS rls_np_kms_configs_tenant           ON np_kms_configs;
DROP POLICY IF EXISTS rls_np_encrypted_values_tenant      ON np_encrypted_values;
DROP POLICY IF EXISTS rls_np_encryption_key_events_tenant ON np_encryption_key_events;

ALTER TABLE np_kms_configs           DISABLE ROW LEVEL SECURITY;
ALTER TABLE np_encrypted_values      DISABLE ROW LEVEL SECURITY;
ALTER TABLE np_encryption_key_events DISABLE ROW LEVEL SECURITY;

COMMIT;
