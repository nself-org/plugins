-- Migration: 003_add_tenant_id_forward_compat
-- Plugin: audit-log (free)
-- Sprint: S74-T03
-- Description: Adds nullable tenant_id column to np_auditlog_events for forward
--              compatibility with nSelf Cloud multi-tenancy (v1.1.0+).
--
-- Design intent:
--   source_account_id (existing) = multi-APP isolation within one nSelf deploy.
--   tenant_id (new, nullable)    = paying Cloud customer isolation.
--
--   In v1.0.9, tenant_id is NULL for all rows (single-user + multi-app deploys).
--   In v1.1.0+, a backfill migration will populate tenant_id from the user→tenant
--   mapping table once that table exists.
--
-- The Hasura permission for the 'user' role uses an _or clause so that:
--   - When X-Hasura-Tenant-Id header is absent: rows with tenant_id IS NULL are visible
--     (single-user deployment behaviour, unchanged).
--   - When X-Hasura-Tenant-Id header is present: only rows with matching tenant_id
--     are visible (Cloud multi-tenant isolation).
--
-- IMPORTANT: this table is APPEND-ONLY per Migration 001. This migration does not
-- change the append-only constraint.
--
-- Safe to apply on:
--   - Fresh databases (column does not exist yet)
--   - Existing databases (IF NOT EXISTS + NULL backfill is a no-op for existing rows)

-- ============================================================
-- Column
-- ============================================================

ALTER TABLE np_auditlog_events
    ADD COLUMN IF NOT EXISTS tenant_id UUID NULL;

-- ============================================================
-- Index (partial — only rows that have a tenant assigned)
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_np_auditlog_tenant_id
    ON np_auditlog_events (tenant_id)
    WHERE tenant_id IS NOT NULL;

-- ============================================================
-- Comment
-- ============================================================

COMMENT ON COLUMN np_auditlog_events.tenant_id IS
    'Cloud multi-tenancy: NULL for single-user and multi-app deployments. '
    'Populated in v1.1.0+ once the user→tenant mapping table exists. '
    'Semantics differ from source_account_id: see multi-tenant-conventions.md.';
