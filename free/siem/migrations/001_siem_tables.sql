-- Migration 001: SIEM destination config and ship log tables.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS.

CREATE TABLE IF NOT EXISTS np_siem_destinations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL CHECK (type IN ('loki','datadog','splunk','elastic','webhook')),
    endpoint_url     TEXT NOT NULL,
    api_key_ref      TEXT,           -- references np_secrets table; never stored raw
    batch_size       INT NOT NULL DEFAULT 500,
    flush_interval_s INT NOT NULL DEFAULT 30,
    schema           TEXT NOT NULL DEFAULT 'ocsf' CHECK (schema IN ('ocsf','ecs','raw')),
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    last_shipped_at  TIMESTAMPTZ,
    error_count      INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE np_siem_destinations IS 'SIEM shipping destinations — one row per configured target (Loki, Datadog, Splunk, Elastic, webhook).';
COMMENT ON COLUMN np_siem_destinations.api_key_ref IS 'Foreign reference to np_secrets.name; never store the raw key here.';

CREATE TABLE IF NOT EXISTS np_siem_ship_log (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    destination_id UUID REFERENCES np_siem_destinations(id) ON DELETE CASCADE,
    batch_size     INT,
    shipped_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status         TEXT NOT NULL CHECK (status IN ('ok','error','partial')),
    error_detail   TEXT
);

COMMENT ON TABLE np_siem_ship_log IS 'Per-batch ship log; last N attempts per destination for debugging.';

CREATE INDEX IF NOT EXISTS idx_siem_ship_log_dest ON np_siem_ship_log (destination_id, shipped_at DESC);
CREATE INDEX IF NOT EXISTS idx_siem_destinations_tenant ON np_siem_destinations (tenant_id) WHERE tenant_id IS NOT NULL;
