-- Rollback 001: Controller schema teardown.
-- WARNING: This destroys the controller registry. Run only on full uninstall.
DROP TABLE IF EXISTS np_tenant_controller.port_map CASCADE;
DROP TABLE IF EXISTS np_tenant_controller.project_audit_log CASCADE;
DROP TABLE IF EXISTS np_tenant_controller.projects CASCADE;
DROP FUNCTION IF EXISTS np_tenant_controller.set_updated_at() CASCADE;
DROP SCHEMA IF EXISTS np_tenant_controller CASCADE;
