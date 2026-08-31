-- Migration 0001 (up): entitlement_grants
-- Purpose: durable storage for ad-hoc feature grants (overrides) that the
--          entitlements plugin's grants CRUD endpoints manage. A grant gives a
--          user/workspace access to a feature independent of their plan — used
--          for comps, trials, and manual overrides.
--
-- Multi-Tenant Convention Wall: this table uses Convention A (Multi-App
-- Isolation) — `source_account_id TEXT NOT NULL DEFAULT 'primary'` — matching
-- every other entitlement_* table in this plugin (plugin.json multiApp.supported
-- = true, isolationColumn = source_account_id). It is NOT a Cloud-tenancy table,
-- so it deliberately has no tenant_id column.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS entitlement_grants (
    id                UUID NOT NULL DEFAULT uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    user_id           TEXT,
    workspace_id      TEXT,
    feature_key       TEXT NOT NULL,
    reason            TEXT,
    granted_by        TEXT,
    expires_at        TIMESTAMPTZ,
    revoked_at        TIMESTAMPTZ,
    metadata          JSONB NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id),
    CHECK (user_id IS NOT NULL OR workspace_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_entitlement_grants_source ON entitlement_grants(source_account_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_grants_user ON entitlement_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_grants_workspace ON entitlement_grants(workspace_id);
CREATE INDEX IF NOT EXISTS idx_entitlement_grants_feature ON entitlement_grants(feature_key);
CREATE INDEX IF NOT EXISTS idx_entitlement_grants_active
    ON entitlement_grants(source_account_id, feature_key)
    WHERE revoked_at IS NULL;

-- Row-Level Security (Convention A): every query is scoped to the caller's
-- source_account_id, set by the SDK middleware as app.source_account_id. With a
-- DEFAULT of 'primary', single-app deploys transparently see only 'primary' rows.
ALTER TABLE entitlement_grants ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entitlement_grants_isolation ON entitlement_grants;
CREATE POLICY entitlement_grants_isolation ON entitlement_grants
    USING (source_account_id = current_setting('app.source_account_id', true))
    WITH CHECK (source_account_id = current_setting('app.source_account_id', true));
