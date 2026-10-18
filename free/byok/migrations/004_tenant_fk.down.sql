-- Drops the foreign keys added by 004_tenant_fk.sql.
ALTER TABLE public.np_kms_configs DROP CONSTRAINT IF EXISTS fk_np_kms_configs_tenant;
ALTER TABLE public.np_encrypted_values DROP CONSTRAINT IF EXISTS fk_np_encrypted_values_tenant;
ALTER TABLE public.np_encryption_key_events DROP CONSTRAINT IF EXISTS fk_np_encryption_key_events_tenant;
ALTER TABLE public.np_encryption_key_events DROP CONSTRAINT IF EXISTS fk_np_encryption_key_events_user;
