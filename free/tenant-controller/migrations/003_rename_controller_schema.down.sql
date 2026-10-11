-- Rollback 003: rename np_tenant_controller back to nself_controller.
-- Metadata-only and lossless. Raises if nself_controller already exists
-- (never merges or drops). No-op if np_tenant_controller does not exist.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'np_tenant_controller') THEN
    IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'nself_controller') THEN
      RAISE EXCEPTION 'both schemas nself_controller and np_tenant_controller exist; resolve manually (no automatic merge)';
    END IF;
    ALTER SCHEMA np_tenant_controller RENAME TO nself_controller;
  END IF;
END
$$;
