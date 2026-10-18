-- Restores the foreign keys the pre-v2 migrations declared, when the core tables exist (a fresh database may not have
-- them yet, which is why 001 no longer declares them). NOT VALID: enforced for new rows, existing rows are not rescanned.
DO $$
BEGIN
  IF to_regclass('public.np_tenants') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_kms_configs_tenant') THEN
    ALTER TABLE public.np_kms_configs ADD CONSTRAINT fk_np_kms_configs_tenant
      FOREIGN KEY (tenant_id) REFERENCES public.np_tenants(id) ON DELETE CASCADE NOT VALID;
  END IF;
  IF to_regclass('public.np_tenants') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_encrypted_values_tenant') THEN
    ALTER TABLE public.np_encrypted_values ADD CONSTRAINT fk_np_encrypted_values_tenant
      FOREIGN KEY (tenant_id) REFERENCES public.np_tenants(id) ON DELETE CASCADE NOT VALID;
  END IF;
  IF to_regclass('public.np_tenants') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_encryption_key_events_tenant') THEN
    ALTER TABLE public.np_encryption_key_events ADD CONSTRAINT fk_np_encryption_key_events_tenant
      FOREIGN KEY (tenant_id) REFERENCES public.np_tenants(id) ON DELETE CASCADE NOT VALID;
  END IF;
  IF to_regclass('public.np_users') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_encryption_key_events_user') THEN
    ALTER TABLE public.np_encryption_key_events ADD CONSTRAINT fk_np_encryption_key_events_user
      FOREIGN KEY (triggered_by) REFERENCES public.np_users(id) ON DELETE SET NULL NOT VALID;
  END IF;
END $$;
