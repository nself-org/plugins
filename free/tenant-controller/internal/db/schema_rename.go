package db

// controllerSchema is the Postgres schema that holds the controller registry
// (projects, project_audit_log, port_map, schema_migrations). Manifest v2
// requires plugin schemas to be named np_<plugin with - as _>.
const controllerSchema = "np_tenant_controller"

// legacyControllerSchema is the pre-manifest-v2 schema name. The Postgres ROLE
// of the same name (nself_controller) is NOT renamed; only the schema moved.
const legacyControllerSchema = "nself_controller"

// renameLegacySchemaSQL moves an existing install from nself_controller to
// np_tenant_controller. ALTER SCHEMA ... RENAME is metadata-only: tables, rows,
// the set_updated_at function, trigger, grants and default privileges follow the
// schema, so the rename is lossless. If both schemas exist the state is
// ambiguous, so it raises instead of merging or dropping anything.
//
// migrations/003_rename_controller_schema.sql only asserts the result: the rename has to run before the
// boot migration ledger creates np_tenant_controller, so it cannot live in a migration file.
const renameLegacySchemaSQL = `
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
`
