-- SOC 2 Readiness Pack: the tables and their RLS in public, where the runtime queries them (D-0832)
-- Migration: 004_public_soc2_tables.sql
-- 001_soc2_tables.sql and 002_rls_policies.sql ran with search_path pinned to np_compliance, so their tables and
-- policies landed in that schema and nothing reads them. Both are released and frozen (migration-immutable): this
-- file repeats them with every table qualified. On an install where the old Go startup DDL (InitSOC2Schema, now
-- removed) already made the public tables, the CREATEs are no-ops apart from idx_change_log_changed_by, and the
-- tables gain the RLS 002 meant them to have (tenant_id filter; an unset app.current_tenant_id still sees every row).
-- The np_compliance copies are left in place: nothing is dropped or moved.
-- Idempotent: CREATE ... IF NOT EXISTS, and DROP POLICY IF EXISTS before each CREATE POLICY.

-- Change management log (CC7)
CREATE TABLE IF NOT EXISTS public.np_change_log (
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
CREATE INDEX IF NOT EXISTS idx_change_log_tenant     ON public.np_change_log(tenant_id, applied_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_log_type       ON public.np_change_log(change_type, applied_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_log_changed_by ON public.np_change_log(changed_by);

-- Access reviews (CC6)
CREATE TABLE IF NOT EXISTS public.np_access_reviews (
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
CREATE INDEX IF NOT EXISTS idx_access_reviews_tenant ON public.np_access_reviews(tenant_id, period_end DESC);
CREATE INDEX IF NOT EXISTS idx_access_reviews_status ON public.np_access_reviews(status, period_end);

-- Vendor registry (CC9)
CREATE TABLE IF NOT EXISTS public.np_vendor_registry (
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
CREATE INDEX IF NOT EXISTS idx_vendor_registry_tenant    ON public.np_vendor_registry(tenant_id);
CREATE INDEX IF NOT EXISTS idx_vendor_registry_risk_tier ON public.np_vendor_registry(risk_tier);

-- ============================================================================
-- np_change_log
-- ============================================================================

ALTER TABLE public.np_change_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_change_log FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_change_log_tenant ON public.np_change_log;
CREATE POLICY rls_np_change_log_tenant ON public.np_change_log
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)::uuid
        OR current_setting('app.current_tenant_id', true) IS NULL
    );

-- ============================================================================
-- np_access_reviews
-- ============================================================================

ALTER TABLE public.np_access_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_access_reviews FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_access_reviews_tenant ON public.np_access_reviews;
CREATE POLICY rls_np_access_reviews_tenant ON public.np_access_reviews
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)::uuid
        OR current_setting('app.current_tenant_id', true) IS NULL
    );

-- ============================================================================
-- np_vendor_registry
-- ============================================================================

ALTER TABLE public.np_vendor_registry ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_vendor_registry FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rls_np_vendor_registry_tenant ON public.np_vendor_registry;
CREATE POLICY rls_np_vendor_registry_tenant ON public.np_vendor_registry
    USING (
        tenant_id = current_setting('app.current_tenant_id', true)::uuid
        OR current_setting('app.current_tenant_id', true) IS NULL
    );

