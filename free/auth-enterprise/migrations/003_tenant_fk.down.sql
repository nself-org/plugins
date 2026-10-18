-- Drops the foreign keys added by 003_tenant_fk.sql.
ALTER TABLE public.np_mfa_enrollments DROP CONSTRAINT IF EXISTS fk_np_mfa_enrollments_tenant;
ALTER TABLE public.np_mfa_policies DROP CONSTRAINT IF EXISTS fk_np_mfa_policies_tenant;
ALTER TABLE public.np_sso_providers DROP CONSTRAINT IF EXISTS fk_np_sso_providers_tenant;
