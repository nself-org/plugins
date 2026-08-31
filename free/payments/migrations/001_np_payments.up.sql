-- 001_np_payments.up.sql — unified payments plugin schema
-- Runs via nself migrate; also embedded in Go binary for autonomous startup.

-- np_subscriptions: unified subscription state across all providers
CREATE TABLE IF NOT EXISTS np_subscriptions (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id              uuid,
  provider             text NOT NULL,
  provider_sub_id      text UNIQUE NOT NULL,
  provider_customer_id text,
  plan_id              text,
  status               text NOT NULL DEFAULT 'active',
  current_period_start timestamptz,
  current_period_end   timestamptz,
  cancel_at_period_end bool DEFAULT false,
  trial_end            timestamptz,
  grace_period_end     timestamptz,
  metadata             jsonb DEFAULT '{}',
  source_account_id    text NOT NULL DEFAULT 'primary',
  created_at           timestamptz DEFAULT now(),
  updated_at           timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_subscriptions_user_id   ON np_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_np_subscriptions_status    ON np_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_np_subscriptions_provider  ON np_subscriptions(provider);

-- Hasura RLS: user sees only their own subscription rows.
-- Apply in Hasura console: select filter {"user_id": {"_eq": "X-Hasura-User-Id"}}

-- np_usage_records: metered billing usage units
CREATE TABLE IF NOT EXISTS np_usage_records (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id uuid REFERENCES np_subscriptions(id) ON DELETE CASCADE,
  metric_id       text NOT NULL,
  quantity        int NOT NULL CHECK (quantity > 0),
  recorded_at     timestamptz DEFAULT now(),
  source_account_id text NOT NULL DEFAULT 'primary'
);

CREATE INDEX IF NOT EXISTS idx_np_usage_records_subscription_id ON np_usage_records(subscription_id);
CREATE INDEX IF NOT EXISTS idx_np_usage_records_metric_id       ON np_usage_records(metric_id);
CREATE INDEX IF NOT EXISTS idx_np_usage_records_recorded_at     ON np_usage_records(recorded_at);

-- np_payment_events: raw webhook event audit log (idempotency via processed flag)
CREATE TABLE IF NOT EXISTS np_payment_events (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider    text NOT NULL,
  event_type  text NOT NULL,
  payload     jsonb NOT NULL,
  processed   bool DEFAULT false,
  error       text,
  created_at  timestamptz DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_payment_events_provider   ON np_payment_events(provider);
CREATE INDEX IF NOT EXISTS idx_np_payment_events_processed  ON np_payment_events(processed) WHERE processed = false;
CREATE INDEX IF NOT EXISTS idx_np_payment_events_created_at ON np_payment_events(created_at);
