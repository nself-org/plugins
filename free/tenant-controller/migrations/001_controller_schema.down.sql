-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- Rollback 001: Controller schema teardown.
-- WARNING: This destroys the controller registry. Run only on full uninstall.
DROP TABLE IF EXISTS np_tenant_controller.port_map CASCADE;
DROP TABLE IF EXISTS np_tenant_controller.project_audit_log CASCADE;
DROP TABLE IF EXISTS np_tenant_controller.projects CASCADE;
DROP FUNCTION IF EXISTS np_tenant_controller.set_updated_at() CASCADE;
DROP SCHEMA IF EXISTS np_tenant_controller CASCADE;
-- After the 003 down the schema carries its legacy name again; tear that down too.
DROP SCHEMA IF EXISTS nself_controller CASCADE;
