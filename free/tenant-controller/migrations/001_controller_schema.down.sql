-- Rollback 001: Controller schema teardown.
-- WARNING: This destroys the controller registry. Run only on full uninstall.
DROP TABLE IF EXISTS nself_controller.port_map CASCADE;
DROP TABLE IF EXISTS nself_controller.project_audit_log CASCADE;
DROP TABLE IF EXISTS nself_controller.projects CASCADE;
DROP FUNCTION IF EXISTS nself_controller.set_updated_at() CASCADE;
DROP SCHEMA IF EXISTS nself_controller CASCADE;
