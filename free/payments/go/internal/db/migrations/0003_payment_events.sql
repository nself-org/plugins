-- 0003: np_payment_events — raw webhook event audit log with idempotency
CREATE TABLE IF NOT EXISTS np_payment_events (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider    text NOT NULL,
  event_type  text NOT NULL,
  payload     jsonb NOT NULL,
  processed   bool DEFAULT false,
  error       text,
  created_at  timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_payment_events_provider ON np_payment_events(provider);
CREATE INDEX IF NOT EXISTS idx_np_payment_events_processed ON np_payment_events(processed) WHERE processed = false;
CREATE INDEX IF NOT EXISTS idx_np_payment_events_created_at ON np_payment_events(created_at);
