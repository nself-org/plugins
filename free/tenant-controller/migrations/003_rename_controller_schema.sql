-- Migration 003: the controller schema is np_tenant_controller (nself_controller before manifest v2).
--
-- The rename itself runs in the daemon before any migration (internal/db: renameLegacySchemaSQL), because the
-- boot migration ledger lives in np_tenant_controller and would otherwise create that schema first and make
-- the rename ambiguous. By the time this file runs the legacy schema must be gone; if it is not (both schemas
-- existed), fail loudly instead of merging or dropping anything. The Postgres ROLE nself_controller is not
-- renamed. Idempotent: no-op on fresh installs and on installs that were already renamed.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'nself_controller') THEN
    RAISE EXCEPTION 'legacy schema nself_controller still exists; resolve manually (no automatic merge)';
  END IF;
END
$$;
