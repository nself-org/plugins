-- SOC 2 Readiness Pack: new tables
-- Migration: 001_soc2_tables.sql
-- Created: 2026-04-23

-- Change management log (CC7)
CREATE TABLE IF NOT EXISTS np_change_log (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   UUID,
  changed_by  UUID,
  change_type TEXT NOT NULL CHECK (change_type IN ('config', 'plugin', 'user', 'schema')),
  description TEXT NOT NULL,
  ticket_ref  TEXT,
  approved_by UUID,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  rollback_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_change_log_tenant     ON np_change_log(tenant_id, applied_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_log_type       ON np_change_log(change_type, applied_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_log_changed_by ON np_change_log(changed_by);

-- Access reviews (CC6)
CREATE TABLE IF NOT EXISTS np_access_reviews (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID,
  reviewer_id  UUID,
  period_start DATE NOT NULL,
  period_end   DATE NOT NULL,
  status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'complete', 'overdue')),
  findings     JSONB NOT NULL DEFAULT '{}',
  completed_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_access_reviews_tenant ON np_access_reviews(tenant_id, period_end DESC);
CREATE INDEX IF NOT EXISTS idx_access_reviews_status ON np_access_reviews(status, period_end);

-- Vendor registry (CC9)
CREATE TABLE IF NOT EXISTS np_vendor_registry (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID,
  vendor_name   TEXT NOT NULL,
  data_shared   TEXT[] NOT NULL DEFAULT '{}',
  risk_tier     TEXT NOT NULL DEFAULT 'low' CHECK (risk_tier IN ('low', 'medium', 'high', 'critical')),
  last_reviewed DATE,
  baa_signed    BOOLEAN NOT NULL DEFAULT FALSE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_vendor_registry_tenant    ON np_vendor_registry(tenant_id);
CREATE INDEX IF NOT EXISTS idx_vendor_registry_risk_tier ON np_vendor_registry(risk_tier);
