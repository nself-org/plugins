-- Restores the foreign keys the pre-v2 migrations declared, when the core tables exist (a fresh database may not have
-- them yet, which is why 001 no longer declares them). NOT VALID: enforced for new rows, existing rows are not rescanned.
DO $$
BEGIN
  IF to_regclass('public.np_tenants') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_mfa_enrollments_tenant') THEN
    ALTER TABLE public.np_mfa_enrollments ADD CONSTRAINT fk_np_mfa_enrollments_tenant
      FOREIGN KEY (tenant_id) REFERENCES public.np_tenants(id) ON DELETE CASCADE NOT VALID;
  END IF;
  IF to_regclass('public.np_tenants') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_mfa_policies_tenant') THEN
    ALTER TABLE public.np_mfa_policies ADD CONSTRAINT fk_np_mfa_policies_tenant
      FOREIGN KEY (tenant_id) REFERENCES public.np_tenants(id) ON DELETE CASCADE NOT VALID;
  END IF;
  IF to_regclass('public.np_tenants') IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_np_sso_providers_tenant') THEN
    ALTER TABLE public.np_sso_providers ADD CONSTRAINT fk_np_sso_providers_tenant
      FOREIGN KEY (tenant_id) REFERENCES public.np_tenants(id) ON DELETE CASCADE NOT VALID;
  END IF;
END $$;
