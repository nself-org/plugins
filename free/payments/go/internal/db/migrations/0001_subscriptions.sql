-- 0001: np_subscriptions — unified subscription state table
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

CREATE INDEX IF NOT EXISTS idx_np_subscriptions_user_id ON np_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_np_subscriptions_status ON np_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_np_subscriptions_provider ON np_subscriptions(provider);
