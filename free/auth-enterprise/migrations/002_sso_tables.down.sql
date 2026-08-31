-- 002_sso_tables.down.sql
-- Reverse of 002_sso_tables.sql
-- Drops SSO provider registry, session log, and CSRF state cache.
-- RLS policies are dropped automatically when their table is dropped.

-- Drop in FK-safe order:
-- np_sso_sessions.provider_id → np_sso_providers (ON DELETE SET NULL, so safe to drop either order)
-- np_sso_state_cache.provider_id → np_sso_providers (ON DELETE CASCADE)
-- Drop dependents before parent to ensure clean cascade.

DROP TABLE IF EXISTS np_sso_state_cache;
DROP TABLE IF EXISTS np_sso_sessions;
DROP TABLE IF EXISTS np_sso_providers;
