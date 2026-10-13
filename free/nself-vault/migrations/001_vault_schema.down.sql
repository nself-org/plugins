-- Rollback of 001_vault_schema (not applied at boot).
DROP TABLE IF EXISTS public.np_vault_audit;
DROP TABLE IF EXISTS public.np_vault_envelopes;
DROP TABLE IF EXISTS public.np_vault_records;
DROP TABLE IF EXISTS public.np_vault_devices;
