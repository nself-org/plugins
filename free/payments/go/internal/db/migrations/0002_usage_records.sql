-- 0002: np_usage_records — metered billing usage tracking
CREATE TABLE IF NOT EXISTS np_usage_records (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id uuid REFERENCES np_subscriptions(id) ON DELETE CASCADE,
  metric_id       text NOT NULL,
  quantity        int NOT NULL CHECK (quantity > 0),
  recorded_at     timestamptz DEFAULT now(),
  source_account_id text NOT NULL DEFAULT 'primary'
);

CREATE INDEX IF NOT EXISTS idx_np_usage_records_subscription_id ON np_usage_records(subscription_id);
CREATE INDEX IF NOT EXISTS idx_np_usage_records_metric_id ON np_usage_records(metric_id);
CREATE INDEX IF NOT EXISTS idx_np_usage_records_recorded_at ON np_usage_records(recorded_at);
