-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Migration 0002 (down): np_entitlements_grants
ALTER TABLE IF EXISTS public.np_entitlements_grants RENAME TO entitlement_grants;
