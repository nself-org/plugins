-- 0004: np_stripe_idempotency — deduplication map for CreateCheckout calls
--
-- Each row records a (request_uuid, stripe_session_id) pair so that a client
-- retry with the same X-Request-Id can receive the already-created session
-- without hitting Stripe again. Rows expire after 24 hours to match Stripe's
-- own idempotency window; a daily cron or the Migrate function handles cleanup.
CREATE TABLE IF NOT EXISTS np_stripe_idempotency (
  request_uuid     text        PRIMARY KEY,
  session_id       text        NOT NULL,
  session_url      text        NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  expires_at       timestamptz NOT NULL DEFAULT (now() + INTERVAL '24 hours')
);

-- Index for TTL sweeps: quickly find and delete expired rows.
CREATE INDEX IF NOT EXISTS idx_np_stripe_idempotency_expires_at
  ON np_stripe_idempotency(expires_at);
