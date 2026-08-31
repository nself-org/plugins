-- E10: nCloud Usage Metering
-- Migration 002: Per-tenant metering tables for CPU/RAM/storage/egress billing.
--
-- Three tables:
--   np_cloud_tenants       — links cloud users to Hetzner resources + Stripe subscription items
--   np_cloud_usage_raw     — hourly readings collected from Hetzner API
--   np_cloud_usage_reported — daily Stripe usage records for idempotent billing

-- np_cloud_tenants: one row per nCloud customer.
CREATE TABLE IF NOT EXISTS np_cloud_tenants (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id               UUID NOT NULL,
  hetzner_server_id     INT,
  hetzner_volume_id     INT,
  stripe_subscription_id TEXT,
  stripe_si_vcpu        TEXT,     -- Stripe subscription item ID for vCPU-hours
  stripe_si_storage     TEXT,     -- Stripe subscription item ID for storage GB-months
  stripe_si_egress      TEXT,     -- Stripe subscription item ID for egress GB
  region                TEXT NOT NULL DEFAULT 'fsn1',
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cloud_tenants_user_id
  ON np_cloud_tenants (user_id);

CREATE INDEX IF NOT EXISTS idx_cloud_tenants_hetzner_server
  ON np_cloud_tenants (hetzner_server_id)
  WHERE hetzner_server_id IS NOT NULL;

-- np_cloud_usage_raw: one row per tenant per dimension per hour bucket.
-- On cron double-fire within the same hour use ON CONFLICT ... DO UPDATE to
-- replace the stale reading rather than inserting a duplicate.
CREATE TABLE IF NOT EXISTS np_cloud_usage_raw (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    UUID NOT NULL REFERENCES np_cloud_tenants(id) ON DELETE CASCADE,
  dimension    TEXT NOT NULL CHECK (dimension IN ('vcpu', 'storage', 'egress')),
  hour_bucket  TIMESTAMPTZ NOT NULL,  -- date_trunc('hour', collected_at)
  collected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  vcpu_hours   NUMERIC(10,4),
  storage_gb   NUMERIC(10,4),
  egress_gb    NUMERIC(10,4),
  UNIQUE (tenant_id, dimension, hour_bucket)
);

CREATE INDEX IF NOT EXISTS idx_usage_raw_tenant_dim_hour
  ON np_cloud_usage_raw (tenant_id, dimension, hour_bucket DESC);

CREATE INDEX IF NOT EXISTS idx_usage_raw_hour_bucket
  ON np_cloud_usage_raw (hour_bucket DESC);

-- np_cloud_usage_reported: idempotent record of every Stripe usage report.
-- UNIQUE constraint on (tenant_id, report_date, dimension) ensures the daily
-- cron is safe to re-run — a second call on the same date is a no-op.
CREATE TABLE IF NOT EXISTS np_cloud_usage_reported (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id             UUID NOT NULL REFERENCES np_cloud_tenants(id) ON DELETE CASCADE,
  report_date           DATE NOT NULL,
  dimension             TEXT NOT NULL CHECK (dimension IN ('vcpu', 'storage', 'egress')),
  quantity              INT NOT NULL,   -- scaled integer sent to Stripe (vCPU × 100, etc.)
  stripe_usage_record_id TEXT,
  reported_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, report_date, dimension)
);

CREATE INDEX IF NOT EXISTS idx_usage_reported_tenant_date
  ON np_cloud_usage_reported (tenant_id, report_date DESC);

-- np_cloud_usage_alerts: per-tenant per-dimension alert thresholds.
CREATE TABLE IF NOT EXISTS np_cloud_usage_alerts (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id      UUID NOT NULL REFERENCES np_cloud_tenants(id) ON DELETE CASCADE,
  dimension      TEXT NOT NULL CHECK (dimension IN ('vcpu', 'storage', 'egress')),
  threshold_pct  INT NOT NULL DEFAULT 80 CHECK (threshold_pct BETWEEN 1 AND 100),
  alerted_at     TIMESTAMPTZ,  -- NULL = not yet alerted this period
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, dimension)
);
