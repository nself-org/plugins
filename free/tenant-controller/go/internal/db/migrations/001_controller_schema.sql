-- B46 Migration 001: embedded copy for the Go binary.
-- See plugins-pro/paid/tenant-controller/migrations/001_controller_schema.sql
-- for the canonical CLI-applied version. This embedded copy runs at daemon startup.

CREATE SCHEMA IF NOT EXISTS nself_controller;

CREATE TABLE IF NOT EXISTS nself_controller.projects (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug         text NOT NULL UNIQUE,
  domain       text NOT NULL UNIQUE,
  schema_name  text NOT NULL UNIQUE,
  role_name    text NOT NULL UNIQUE,
  hasura_source text NOT NULL UNIQUE,
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

CREATE TABLE IF NOT EXISTS nself_controller.project_audit_log (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type   text NOT NULL,
  slug         text NOT NULL,
  initiated_by text NOT NULL,
  detail       jsonb,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_slug_ts
  ON nself_controller.project_audit_log (slug, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_log_event_ts
  ON nself_controller.project_audit_log (event_type, created_at DESC);

CREATE TABLE IF NOT EXISTS nself_controller.port_map (
  port         integer PRIMARY KEY,
  slug         text NOT NULL REFERENCES nself_controller.projects (slug) ON DELETE CASCADE,
  service      text NOT NULL DEFAULT 'hasura',
  allocated_at timestamptz NOT NULL DEFAULT now()
);

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
