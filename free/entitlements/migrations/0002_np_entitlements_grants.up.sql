-- Migration 0002 (up): entitlement_grants becomes np_entitlements_grants
-- Purpose: plugin tables carry the np_<plugin>_ prefix the manifest requires (E106). An install
--          that ran 0001 keeps its rows, policy and indexes; replaying this is a no-op.
ALTER TABLE IF EXISTS public.entitlement_grants RENAME TO np_entitlements_grants;
