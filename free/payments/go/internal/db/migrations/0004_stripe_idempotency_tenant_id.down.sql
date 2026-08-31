-- P99 SIEGE F3 fix rollback: remove tenant_id from np_stripe_idempotency

DROP INDEX IF EXISTS idx_np_stripe_idempotency_tenant_id;
ALTER TABLE np_stripe_idempotency DROP COLUMN IF EXISTS tenant_id;
