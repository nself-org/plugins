-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Drops the foreign keys added by 004_tenant_fk.sql.
ALTER TABLE public.np_kms_configs DROP CONSTRAINT IF EXISTS fk_np_kms_configs_tenant;
ALTER TABLE public.np_encrypted_values DROP CONSTRAINT IF EXISTS fk_np_encrypted_values_tenant;
ALTER TABLE public.np_encryption_key_events DROP CONSTRAINT IF EXISTS fk_np_encryption_key_events_tenant;
ALTER TABLE public.np_encryption_key_events DROP CONSTRAINT IF EXISTS fk_np_encryption_key_events_user;
