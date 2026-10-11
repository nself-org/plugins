-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh LEGACY pass): the TS-era feature-flags schema (unprefixed ff_* tables)
-- an old install holds, one row each with fixed primary keys. Test data only.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =====================================================================
-- Flags
-- =====================================================================

CREATE TABLE IF NOT EXISTS ff_flags (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  key VARCHAR(255) NOT NULL,
  name VARCHAR(255),
  description TEXT,
  flag_type VARCHAR(16) DEFAULT 'release',
  enabled BOOLEAN DEFAULT false,
  default_value JSONB DEFAULT 'false',
  tags TEXT[] DEFAULT '{}',
  owner VARCHAR(255),
  stale_after_days INTEGER,
  last_evaluated_at TIMESTAMP WITH TIME ZONE,
  evaluation_count BIGINT DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, key)
);
CREATE INDEX IF NOT EXISTS idx_ff_flags_source_account ON ff_flags(source_account_id);
CREATE INDEX IF NOT EXISTS idx_ff_flags_key ON ff_flags(key);
CREATE INDEX IF NOT EXISTS idx_ff_flags_type ON ff_flags(flag_type);
CREATE INDEX IF NOT EXISTS idx_ff_flags_enabled ON ff_flags(enabled);
CREATE INDEX IF NOT EXISTS idx_ff_flags_tags ON ff_flags USING GIN(tags);

-- =====================================================================
-- Rules
-- =====================================================================

CREATE TABLE IF NOT EXISTS ff_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  flag_id UUID NOT NULL REFERENCES ff_flags(id) ON DELETE CASCADE,
  name VARCHAR(255),
  rule_type VARCHAR(32) NOT NULL,
  conditions JSONB NOT NULL,
  value JSONB NOT NULL DEFAULT 'true',
  priority INTEGER DEFAULT 0,
  enabled BOOLEAN DEFAULT true,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ff_rules_source_account ON ff_rules(source_account_id);
CREATE INDEX IF NOT EXISTS idx_ff_rules_flag_id ON ff_rules(flag_id);
CREATE INDEX IF NOT EXISTS idx_ff_rules_priority ON ff_rules(priority DESC);

-- =====================================================================
-- Segments
-- =====================================================================

CREATE TABLE IF NOT EXISTS ff_segments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  match_type VARCHAR(8) DEFAULT 'all',
  rules JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);
CREATE INDEX IF NOT EXISTS idx_ff_segments_source_account ON ff_segments(source_account_id);
CREATE INDEX IF NOT EXISTS idx_ff_segments_name ON ff_segments(name);

-- =====================================================================
-- Evaluations
-- =====================================================================

CREATE TABLE IF NOT EXISTS ff_evaluations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  flag_key VARCHAR(255) NOT NULL,
  user_id VARCHAR(255),
  context JSONB,
  result JSONB,
  rule_id UUID,
  reason VARCHAR(64),
  evaluated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ff_evaluations_source_account ON ff_evaluations(source_account_id);
CREATE INDEX IF NOT EXISTS idx_ff_evaluations_flag_key ON ff_evaluations(flag_key);
CREATE INDEX IF NOT EXISTS idx_ff_evaluations_user_id ON ff_evaluations(user_id);
CREATE INDEX IF NOT EXISTS idx_ff_evaluations_evaluated_at ON ff_evaluations(evaluated_at DESC);

-- =====================================================================
-- Webhook Events
-- =====================================================================

CREATE TABLE IF NOT EXISTS ff_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ff_webhook_events_source_account ON ff_webhook_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_ff_webhook_events_type ON ff_webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_ff_webhook_events_processed ON ff_webhook_events(processed);

INSERT INTO ff_flags (id, key, name) VALUES ('00000000-0000-4000-8000-0000000000b1', 'legacy-flag', 'Legacy flag');
INSERT INTO ff_rules (id, flag_id, rule_type, conditions) VALUES ('00000000-0000-4000-8000-0000000000b2', '00000000-0000-4000-8000-0000000000b1', 'segment', '[]');
INSERT INTO ff_segments (id, name) VALUES ('00000000-0000-4000-8000-0000000000b3', 'legacy-segment');
INSERT INTO ff_evaluations (id, flag_key, user_id) VALUES ('00000000-0000-4000-8000-0000000000b4', 'legacy-flag', 'user-1');
INSERT INTO ff_webhook_events (id, event_type) VALUES ('legacy-ff-event', 'flag.updated');
