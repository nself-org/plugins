-- Migration 003: rename the controller schema nself_controller -> np_tenant_controller.
--
-- Manifest v2 requires a plugin schema to be named np_<plugin with - as _>.
-- ALTER SCHEMA ... RENAME is metadata-only, so tables, rows, the set_updated_at
-- function, the trigger, grants and default privileges all follow the schema.
-- The Postgres ROLE nself_controller is NOT renamed.
--
-- Idempotent: no-op on fresh installs (001 already creates np_tenant_controller)
-- and on installs that were already renamed. If BOTH schemas exist the state is
-- ambiguous, so this raises instead of merging or dropping anything.
-- The Go daemon runs the same guarded rename at startup (db.Migrate).

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'nself_controller') THEN
    IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'np_tenant_controller') THEN
      RAISE EXCEPTION 'both schemas nself_controller and np_tenant_controller exist; resolve manually (no automatic merge)';
    END IF;
    ALTER SCHEMA nself_controller RENAME TO np_tenant_controller;
  END IF;
END
$$;
