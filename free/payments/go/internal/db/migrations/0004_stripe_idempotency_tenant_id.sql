-- P99 SIEGE F3 fix: add tenant_id to np_stripe_idempotency (Convention Wall compliance)
-- Table was created by 0004_stripe_idempotency.sql.
-- This additive migration brings it into compliance with the Multi-Tenant Convention Wall
-- (PPI Hard Rule 2026-04-20): every np_* table holding paying-customer data must carry
-- tenant_id UUID + index for Cloud multi-tenancy isolation.
-- Self-host operators: tenant_id remains NULL — no behavioural change.
-- Note: idempotency keys are scoped per-tenant in Cloud so that tenant A's retry
-- cannot observe tenant B's checkout session even if they submit the same X-Request-Id.

ALTER TABLE np_stripe_idempotency
    ADD COLUMN IF NOT EXISTS tenant_id UUID;   -- NULL = self-host; non-NULL = Cloud tenant

CREATE INDEX IF NOT EXISTS idx_np_stripe_idempotency_tenant_id
    ON np_stripe_idempotency (tenant_id)
    WHERE tenant_id IS NOT NULL;
