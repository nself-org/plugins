BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS np_cs_evidence (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    type VARCHAR(100) NOT NULL,
    content JSONB NOT NULL DEFAULT '{}',
    reason TEXT NOT NULL,
    source VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(128) NOT NULL,
    collector_id VARCHAR(128),
    collector_role VARCHAR(50),
    priority VARCHAR(20) NOT NULL DEFAULT 'normal',
    is_encrypted BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    legal_hold_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_evidence FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_evidence_owner ON np_cs_evidence;
CREATE POLICY rls_np_cs_evidence_owner ON np_cs_evidence
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_legal_holds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    description TEXT,
    scope JSONB NOT NULL DEFAULT '{}',
    criteria JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_by VARCHAR(128),
    released_by VARCHAR(128),
    released_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_legal_holds ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_legal_holds FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_legal_holds_owner ON np_cs_legal_holds;
CREATE POLICY rls_np_cs_legal_holds_owner ON np_cs_legal_holds
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_evidence_exports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    legal_hold_id UUID,
    evidence_ids JSONB NOT NULL DEFAULT '[]',
    format VARCHAR(20) NOT NULL DEFAULT 'json',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    requested_by VARCHAR(128),
    download_url TEXT,
    expires_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_evidence_exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_evidence_exports FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_evidence_exports_owner ON np_cs_evidence_exports;
CREATE POLICY rls_np_cs_evidence_exports_owner ON np_cs_evidence_exports
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_spam_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    description TEXT,
    rule_type VARCHAR(50) NOT NULL,
    pattern TEXT,
    conditions JSONB NOT NULL DEFAULT '{}',
    action VARCHAR(50) NOT NULL DEFAULT 'flag',
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 0,
    created_by VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_spam_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_spam_rules FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_spam_rules_owner ON np_cs_spam_rules;
CREATE POLICY rls_np_cs_spam_rules_owner ON np_cs_spam_rules
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_spam_configs (
    workspace_id VARCHAR(128) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    sensitivity VARCHAR(20) NOT NULL DEFAULT 'medium',
    auto_delete BOOLEAN NOT NULL DEFAULT false,
    notify_moderators BOOLEAN NOT NULL DEFAULT true,
    quarantine_threshold NUMERIC(4,3) NOT NULL DEFAULT 0.8,
    config JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, source_account_id)
);
ALTER TABLE np_cs_spam_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_spam_configs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_spam_configs_owner ON np_cs_spam_configs;
CREATE POLICY rls_np_cs_spam_configs_owner ON np_cs_spam_configs
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_rate_limit_violations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    user_id VARCHAR(128) NOT NULL,
    channel_id VARCHAR(128),
    workspace_id VARCHAR(128),
    limit_type VARCHAR(50) NOT NULL,
    action_taken VARCHAR(50) NOT NULL DEFAULT 'blocked',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_rate_limit_violations ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_rate_limit_violations FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_rate_limit_violations_owner ON np_cs_rate_limit_violations;
CREATE POLICY rls_np_cs_rate_limit_violations_owner ON np_cs_rate_limit_violations
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_raid_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    workspace_id VARCHAR(128) NOT NULL,
    channel_id VARCHAR(128),
    event_type VARCHAR(50) NOT NULL DEFAULT 'join_spike',
    severity VARCHAR(20) NOT NULL DEFAULT 'medium',
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_raid_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_raid_events FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_raid_events_owner ON np_cs_raid_events;
CREATE POLICY rls_np_cs_raid_events_owner ON np_cs_raid_events
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_lockdowns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    workspace_id VARCHAR(128) NOT NULL,
    channel_id VARCHAR(128),
    level VARCHAR(20) NOT NULL DEFAULT 'partial',
    reason TEXT,
    activated_by VARCHAR(128),
    deactivated_by VARCHAR(128),
    deactivated_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE np_cs_lockdowns ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_lockdowns FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_lockdowns_owner ON np_cs_lockdowns;
CREATE POLICY rls_np_cs_lockdowns_owner ON np_cs_lockdowns
    USING (source_account_id = current_setting('app.current_source_account_id', true));

CREATE TABLE IF NOT EXISTS np_cs_abuse_scores (
    user_id VARCHAR(128) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    trust_score NUMERIC(5,4) NOT NULL DEFAULT 0.5,
    risk_level VARCHAR(20) NOT NULL DEFAULT 'low',
    total_events INTEGER NOT NULL DEFAULT 0,
    positive_events INTEGER NOT NULL DEFAULT 0,
    negative_events INTEGER NOT NULL DEFAULT 0,
    last_event_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, source_account_id)
);
ALTER TABLE np_cs_abuse_scores ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cs_abuse_scores FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rls_np_cs_abuse_scores_owner ON np_cs_abuse_scores;
CREATE POLICY rls_np_cs_abuse_scores_owner ON np_cs_abuse_scores
    USING (source_account_id = current_setting('app.current_source_account_id', true));

COMMIT;
