-- D-0956. Three repairs for databases that came from the pre-v2 schemas.
--
-- 1. The tenant policies cast current_setting('app.current_tenant_id', true) to uuid. After a
--    transaction-local set_config on a pooled connection, Postgres leaves the setting as an empty
--    string, and ''::uuid raises instead of matching no rows. NULLIF turns the empty string into NULL.
--    The plugin sets the setting per transaction (tenantdb.With); a session without it sees no rows.
-- 2. The old inline Go schema created the tables without the iv-length and event_type CHECKs that
--    001 declares, and CREATE TABLE IF NOT EXISTS never added them. Add them under the names Postgres
--    gives inline CHECKs, NOT VALID so existing rows are not rescanned.
-- 3. Databases built by the pre-v2 migration runner carry unnamed tenant foreign keys that 004 repeats
--    under named constraints. Drop the unnamed duplicate once the named one exists.

DROP POLICY IF EXISTS rls_np_kms_configs_tenant ON public.np_kms_configs;
CREATE POLICY rls_np_kms_configs_tenant ON public.np_kms_configs
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS rls_np_encrypted_values_tenant ON public.np_encrypted_values;
CREATE POLICY rls_np_encrypted_values_tenant ON public.np_encrypted_values
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS rls_np_encryption_key_events_tenant ON public.np_encryption_key_events;
CREATE POLICY rls_np_encryption_key_events_tenant ON public.np_encryption_key_events
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_kms_configs'::regclass
                  AND conname = 'np_kms_configs_provider_check') THEN
    ALTER TABLE public.np_kms_configs ADD CONSTRAINT np_kms_configs_provider_check
      CHECK (provider IN ('aws', 'gcp', 'vault')) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encrypted_values'::regclass
                  AND conname = 'np_encrypted_values_iv_check') THEN
    ALTER TABLE public.np_encrypted_values ADD CONSTRAINT np_encrypted_values_iv_check
      CHECK (octet_length(iv) = 12) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encryption_key_events'::regclass
                  AND conname = 'np_encryption_key_events_event_type_check') THEN
    ALTER TABLE public.np_encryption_key_events ADD CONSTRAINT np_encryption_key_events_event_type_check
      CHECK (event_type IN (
        'key_configured', 'key_rotated', 'key_revoked',
        'verify_ok', 'verify_fail',
        'rotation_started', 'rotation_completed', 'rotation_failed'
      )) NOT VALID;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_kms_configs'::regclass
              AND conname = 'fk_np_kms_configs_tenant')
     AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_kms_configs'::regclass
                  AND conname = 'np_kms_configs_tenant_id_fkey') THEN
    ALTER TABLE public.np_kms_configs DROP CONSTRAINT np_kms_configs_tenant_id_fkey;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encrypted_values'::regclass
              AND conname = 'fk_np_encrypted_values_tenant')
     AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encrypted_values'::regclass
                  AND conname = 'np_encrypted_values_tenant_id_fkey') THEN
    ALTER TABLE public.np_encrypted_values DROP CONSTRAINT np_encrypted_values_tenant_id_fkey;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encryption_key_events'::regclass
              AND conname = 'fk_np_encryption_key_events_tenant')
     AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encryption_key_events'::regclass
                  AND conname = 'np_encryption_key_events_tenant_id_fkey') THEN
    ALTER TABLE public.np_encryption_key_events DROP CONSTRAINT np_encryption_key_events_tenant_id_fkey;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encryption_key_events'::regclass
              AND conname = 'fk_np_encryption_key_events_user')
     AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.np_encryption_key_events'::regclass
                  AND conname = 'np_encryption_key_events_triggered_by_fkey') THEN
    ALTER TABLE public.np_encryption_key_events DROP CONSTRAINT np_encryption_key_events_triggered_by_fkey;
  END IF;
END $$;
