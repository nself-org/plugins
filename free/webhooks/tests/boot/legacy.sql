-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh LEGACY pass): the TS-era webhooks schema (unprefixed webhook_* tables)
-- an old install holds, one row each with fixed primary keys. Test data only.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =====================================================================
-- Webhook Endpoints
-- =====================================================================

CREATE TABLE IF NOT EXISTS webhook_endpoints (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  url TEXT NOT NULL,
  description TEXT,
  secret VARCHAR(255) NOT NULL,
  events TEXT[] NOT NULL,
  headers JSONB DEFAULT '{}',
  enabled BOOLEAN DEFAULT TRUE,
  failure_count INTEGER DEFAULT 0,
  last_success_at TIMESTAMP WITH TIME ZONE,
  last_failure_at TIMESTAMP WITH TIME ZONE,
  disabled_at TIMESTAMP WITH TIME ZONE,
  disabled_reason TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_source_account
  ON webhook_endpoints(source_account_id);
CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_enabled
  ON webhook_endpoints(enabled);
CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_events
  ON webhook_endpoints USING GIN(events);
CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_created
  ON webhook_endpoints(created_at DESC);

-- =====================================================================
-- Webhook Deliveries
-- =====================================================================

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  endpoint_id UUID NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  status VARCHAR(32) DEFAULT 'pending',
  response_status INTEGER,
  response_body TEXT,
  response_time_ms INTEGER,
  attempt_count INTEGER DEFAULT 0,
  max_attempts INTEGER DEFAULT 5,
  next_retry_at TIMESTAMP WITH TIME ZONE,
  error_message TEXT,
  signature VARCHAR(255),
  delivered_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_source_account
  ON webhook_deliveries(source_account_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint
  ON webhook_deliveries(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status
  ON webhook_deliveries(status);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event_type
  ON webhook_deliveries(event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_next_retry
  ON webhook_deliveries(next_retry_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created
  ON webhook_deliveries(created_at DESC);

-- =====================================================================
-- Webhook Event Types
-- =====================================================================

CREATE TABLE IF NOT EXISTS webhook_event_types (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  source_plugin VARCHAR(128),
  schema JSONB,
  sample_payload JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX IF NOT EXISTS idx_webhook_event_types_source_account
  ON webhook_event_types(source_account_id);
CREATE INDEX IF NOT EXISTS idx_webhook_event_types_name
  ON webhook_event_types(name);
CREATE INDEX IF NOT EXISTS idx_webhook_event_types_source_plugin
  ON webhook_event_types(source_plugin);

-- =====================================================================
-- Webhook Dead Letters
-- =====================================================================

CREATE TABLE IF NOT EXISTS webhook_dead_letters (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  delivery_id UUID REFERENCES webhook_deliveries(id),
  endpoint_id UUID REFERENCES webhook_endpoints(id),
  event_type VARCHAR(128),
  payload JSONB,
  last_error TEXT,
  attempt_count INTEGER,
  resolved BOOLEAN DEFAULT FALSE,
  resolved_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_source_account
  ON webhook_dead_letters(source_account_id);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_delivery
  ON webhook_dead_letters(delivery_id);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_endpoint
  ON webhook_dead_letters(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_resolved
  ON webhook_dead_letters(resolved);
CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_created
  ON webhook_dead_letters(created_at DESC);

INSERT INTO webhook_endpoints (id, url, secret, events) VALUES ('00000000-0000-4000-8000-0000000000c1', 'https://example.invalid/hook', 'whsec_fixture', '{user.created}');
INSERT INTO webhook_deliveries (id, endpoint_id, event_type, payload) VALUES ('00000000-0000-4000-8000-0000000000c2', '00000000-0000-4000-8000-0000000000c1', 'user.created', '{}');
INSERT INTO webhook_event_types (id, name) VALUES ('00000000-0000-4000-8000-0000000000c3', 'user.created');
INSERT INTO webhook_dead_letters (id, delivery_id, endpoint_id) VALUES ('00000000-0000-4000-8000-0000000000c4', '00000000-0000-4000-8000-0000000000c2', '00000000-0000-4000-8000-0000000000c1');
