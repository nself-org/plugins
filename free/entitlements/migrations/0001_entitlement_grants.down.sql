-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Migration 0001 (down): entitlement_grants
DROP POLICY IF EXISTS entitlement_grants_isolation ON public.entitlement_grants;
DROP TABLE IF EXISTS public.entitlement_grants;
