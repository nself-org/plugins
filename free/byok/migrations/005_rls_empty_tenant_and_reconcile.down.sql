-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Restores the 002 policies. The CHECK constraints and the dropped duplicate foreign keys stay.
DROP POLICY IF EXISTS rls_np_kms_configs_tenant ON public.np_kms_configs;
CREATE POLICY rls_np_kms_configs_tenant ON public.np_kms_configs
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid);
DROP POLICY IF EXISTS rls_np_encrypted_values_tenant ON public.np_encrypted_values;
CREATE POLICY rls_np_encrypted_values_tenant ON public.np_encrypted_values
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid);
DROP POLICY IF EXISTS rls_np_encryption_key_events_tenant ON public.np_encryption_key_events;
CREATE POLICY rls_np_encryption_key_events_tenant ON public.np_encryption_key_events
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid);
