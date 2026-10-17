-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Rollback migration 001: nself-cloud master tenant schema
-- Drops in reverse FK order to avoid constraint violations.
-- Plugin: nself-cloud

DROP TRIGGER IF EXISTS np_cloud_instances_updated_at ON public.np_cloud_instances;
DROP TRIGGER IF EXISTS np_cloud_tenants_updated_at   ON public.np_cloud_tenants;
DROP FUNCTION IF EXISTS public.np_cloud_set_updated_at();

DROP TABLE IF EXISTS public.np_cloud_team_memberships;
DROP TABLE IF EXISTS public.np_cloud_invitations;
DROP TABLE IF EXISTS public.np_cloud_domains;
DROP TABLE IF EXISTS public.np_cloud_billing_events;
DROP TABLE IF EXISTS public.np_cloud_instances;
DROP TABLE IF EXISTS public.np_cloud_tenants;
