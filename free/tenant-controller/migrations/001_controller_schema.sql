-- B46: Multi-Tenant Master Controller
-- Migration 001: Controller schema, project registry, audit log, port map.
--
-- This schema is owned by the controller role only.
-- No user-facing GraphQL mutation may trigger schema creation or deletion.
-- Project schemas (project_{slug}) are created dynamically by the controller at runtime.

CREATE SCHEMA IF NOT EXISTS nself_controller;

-- projects: master registry of all managed projects.
CREATE TABLE IF NOT EXISTS nself_controller.projects (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug         text NOT NULL UNIQUE,
  domain       text NOT NULL UNIQUE,
  schema_name  text NOT NULL UNIQUE,  -- project_{slug}
  role_name    text NOT NULL UNIQUE,  -- nself_project_{slug}
  hasura_source text NOT NULL UNIQUE, -- project_{slug}
  hasura_port  integer NOT NULL UNIQUE,
  status       text NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'suspended', 'deleting')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz
);

CREATE INDEX IF NOT EXISTS idx_controller_projects_status
  ON nself_controller.projects (status)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_controller_projects_domain
  ON nself_controller.projects (domain)
  WHERE deleted_at IS NULL;

-- project_audit_log: immutable lifecycle event log, routed through Q02 pipeline.
CREATE TABLE IF NOT EXISTS nself_controller.project_audit_log (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type   text NOT NULL,
  -- project_create | project_delete | project_migrate
  -- project_rotate_credentials | source_register | source_deregister | nginx_reload
  -- project_suspend | project_resume
  slug         text NOT NULL,
  initiated_by text NOT NULL,   -- 'cli:nself_controller' | 'admin:user_id:{id}'
  detail       jsonb,           -- event-specific structured payload
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_slug_ts
  ON nself_controller.project_audit_log (slug, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_event_ts
  ON nself_controller.project_audit_log (event_type, created_at DESC);

-- port_map: tracks which port is assigned to which project's Hasura instance.
-- Prevents port collisions on project create.
CREATE TABLE IF NOT EXISTS nself_controller.port_map (
  port         integer PRIMARY KEY,
  slug         text NOT NULL REFERENCES nself_controller.projects (slug) ON DELETE CASCADE,
  service      text NOT NULL DEFAULT 'hasura',  -- 'hasura' | future extensions
  allocated_at timestamptz NOT NULL DEFAULT now()
);

-- Trigger: keep projects.updated_at current.
CREATE OR REPLACE FUNCTION nself_controller.set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_projects_updated_at ON nself_controller.projects;
CREATE TRIGGER trg_projects_updated_at
  BEFORE UPDATE ON nself_controller.projects
  FOR EACH ROW EXECUTE FUNCTION nself_controller.set_updated_at();

-- Controller admin role (used by the controller daemon).
-- Created only if it doesn't exist; credentials are set by the controller at startup.
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nself_controller') THEN
    CREATE ROLE nself_controller LOGIN PASSWORD 'PLACEHOLDER_ROTATED_AT_RUNTIME';
  END IF;
END $$;

-- Grant controller role full access to its own schema.
GRANT ALL ON SCHEMA nself_controller TO nself_controller;
GRANT ALL ON ALL TABLES IN SCHEMA nself_controller TO nself_controller;
GRANT ALL ON ALL SEQUENCES IN SCHEMA nself_controller TO nself_controller;
ALTER DEFAULT PRIVILEGES IN SCHEMA nself_controller
  GRANT ALL ON TABLES TO nself_controller;
ALTER DEFAULT PRIVILEGES IN SCHEMA nself_controller
  GRANT ALL ON SEQUENCES TO nself_controller;
