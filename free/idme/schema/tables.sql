-- =============================================================================
-- ID.me Plugin Schema
-- Tables for OAuth authentication and identity verification
-- Reference copy. The plugin applies migrations/000_np_prefix_and_tables.sql itself at boot; nothing runs this file.
-- =============================================================================

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Verifications
-- Main table for tracking user verification status
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_idme_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',  -- Multi-app isolation (Convention A)
    user_id UUID NOT NULL,                          -- Link to your users table
    idme_user_id VARCHAR(255) UNIQUE,               -- ID.me's user identifier
    email VARCHAR(255) NOT NULL,
    verified BOOLEAN DEFAULT FALSE,                 -- Overall verification status
    verification_level VARCHAR(50),                 -- identity, phone, email, etc.
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    birth_date DATE,
    zip VARCHAR(10),
    phone VARCHAR(50),
    access_token TEXT,                              -- OAuth access token (encrypted in production)
    refresh_token TEXT,                             -- OAuth refresh token
    token_expires_at TIMESTAMP WITH TIME ZONE,
    verified_at TIMESTAMP WITH TIME ZONE,
    last_synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    metadata JSONB DEFAULT '{}',                    -- Additional ID.me metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_idme_verifications_user ON np_idme_verifications(user_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_verifications_email ON np_idme_verifications(email);
CREATE INDEX IF NOT EXISTS idx_np_idme_verifications_idme_user ON np_idme_verifications(idme_user_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_verifications_verified ON np_idme_verifications(verified);

-- =============================================================================
-- Groups
-- Verification groups (military, veteran, first responder, etc.)
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_idme_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',  -- Multi-app isolation (Convention A)
    verification_id UUID NOT NULL REFERENCES np_idme_verifications(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    group_type VARCHAR(50) NOT NULL,                -- military, veteran, first_responder, government, teacher, student, nurse
    group_name VARCHAR(255) NOT NULL,
    verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,            -- Some verifications may expire
    affiliation VARCHAR(255),                       -- Branch, organization, etc.
    rank VARCHAR(100),                              -- Military rank (if applicable)
    status VARCHAR(100),                            -- Active, reserve, retired, etc.
    metadata JSONB DEFAULT '{}',                    -- Group-specific metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(verification_id, group_type)
);

CREATE INDEX IF NOT EXISTS idx_np_idme_groups_verification ON np_idme_groups(verification_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_groups_user ON np_idme_groups(user_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_groups_type ON np_idme_groups(group_type);
CREATE INDEX IF NOT EXISTS idx_np_idme_groups_verified ON np_idme_groups(verified);

-- =============================================================================
-- Badges
-- Visual badges for verified groups
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_idme_badges (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',  -- Multi-app isolation (Convention A)
    verification_id UUID NOT NULL REFERENCES np_idme_verifications(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    badge_type VARCHAR(50) NOT NULL,                -- Same as group_type
    badge_name VARCHAR(255) NOT NULL,
    badge_icon VARCHAR(255),                        -- Icon/emoji for the badge
    badge_color VARCHAR(50),                        -- Color for UI display
    verified_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    active BOOLEAN DEFAULT TRUE,
    display_order INTEGER DEFAULT 0,                -- Order for displaying multiple badges
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(verification_id, badge_type)
);

CREATE INDEX IF NOT EXISTS idx_np_idme_badges_verification ON np_idme_badges(verification_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_badges_user ON np_idme_badges(user_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_badges_type ON np_idme_badges(badge_type);
CREATE INDEX IF NOT EXISTS idx_np_idme_badges_active ON np_idme_badges(active);

-- =============================================================================
-- Attributes
-- Additional verified attributes from ID.me
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_idme_attributes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',  -- Multi-app isolation (Convention A)
    verification_id UUID NOT NULL REFERENCES np_idme_verifications(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    attribute_key VARCHAR(100) NOT NULL,            -- branch, service_era, specialty, etc.
    attribute_value TEXT,
    attribute_type VARCHAR(50),                     -- string, date, boolean, etc.
    verified BOOLEAN DEFAULT FALSE,
    verified_at TIMESTAMP WITH TIME ZONE,
    source VARCHAR(100) DEFAULT 'idme',             -- Source of the attribute
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(verification_id, attribute_key)
);

CREATE INDEX IF NOT EXISTS idx_np_idme_attributes_verification ON np_idme_attributes(verification_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_attributes_user ON np_idme_attributes(user_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_attributes_key ON np_idme_attributes(attribute_key);

-- =============================================================================
-- Webhook Events (for audit and replay)
-- =============================================================================

CREATE TABLE IF NOT EXISTS np_idme_webhook_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',  -- Multi-app isolation (Convention A)
    event_id VARCHAR(255) UNIQUE,                   -- ID.me event ID
    event_type VARCHAR(100) NOT NULL,               -- verification.created, group.verified, etc.
    user_id UUID,
    verification_id UUID REFERENCES np_idme_verifications(id),
    payload JSONB NOT NULL,                         -- Full webhook payload
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_idme_webhook_events_type ON np_idme_webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_np_idme_webhook_events_user ON np_idme_webhook_events(user_id);
CREATE INDEX IF NOT EXISTS idx_np_idme_webhook_events_processed ON np_idme_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_np_idme_webhook_events_created ON np_idme_webhook_events(created_at);

-- =============================================================================
-- Views for common queries
-- =============================================================================

-- All verified users with their groups
CREATE OR REPLACE VIEW idme_verified_users AS
SELECT
    v.id AS verification_id,
    v.user_id,
    v.email,
    v.first_name,
    v.last_name,
    v.verified,
    v.verified_at,
    COALESCE(
        json_agg(
            json_build_object(
                'type', g.group_type,
                'name', g.group_name,
                'verified_at', g.verified_at,
                'affiliation', g.affiliation
            )
        ) FILTER (WHERE g.id IS NOT NULL),
        '[]'
    ) AS groups,
    COALESCE(
        json_agg(
            json_build_object(
                'type', b.badge_type,
                'name', b.badge_name,
                'icon', b.badge_icon
            )
        ) FILTER (WHERE b.id IS NOT NULL),
        '[]'
    ) AS badges
FROM np_idme_verifications v
LEFT JOIN np_idme_groups g ON v.id = g.verification_id AND g.verified = TRUE
LEFT JOIN np_idme_badges b ON v.id = b.verification_id AND b.active = TRUE
WHERE v.verified = TRUE
GROUP BY v.id, v.user_id, v.email, v.first_name, v.last_name, v.verified, v.verified_at;

-- Group verification summary
CREATE OR REPLACE VIEW idme_group_summary AS
SELECT
    group_type,
    group_name,
    COUNT(*) AS total_verified,
    COUNT(DISTINCT user_id) AS unique_users,
    MIN(verified_at) AS first_verified,
    MAX(verified_at) AS last_verified
FROM np_idme_groups
WHERE verified = TRUE
GROUP BY group_type, group_name
ORDER BY total_verified DESC;

-- Recent verifications
CREATE OR REPLACE VIEW idme_recent_verifications AS
SELECT
    v.id,
    v.user_id,
    v.email,
    v.first_name || ' ' || v.last_name AS full_name,
    v.verified_at,
    COUNT(g.id) AS groups_count,
    json_agg(g.group_type) FILTER (WHERE g.id IS NOT NULL) AS groups
FROM np_idme_verifications v
LEFT JOIN np_idme_groups g ON v.id = g.verification_id AND g.verified = TRUE
WHERE v.verified = TRUE
  AND v.verified_at >= NOW() - INTERVAL '30 days'
GROUP BY v.id, v.user_id, v.email, v.first_name, v.last_name, v.verified_at
ORDER BY v.verified_at DESC;

-- =============================================================================
-- Functions
-- =============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply triggers to all tables
DROP TRIGGER IF EXISTS update_idme_verifications_updated_at ON np_idme_verifications;
CREATE TRIGGER update_idme_verifications_updated_at
    BEFORE UPDATE ON np_idme_verifications
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_idme_groups_updated_at ON np_idme_groups;
CREATE TRIGGER update_idme_groups_updated_at
    BEFORE UPDATE ON np_idme_groups
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_idme_badges_updated_at ON np_idme_badges;
CREATE TRIGGER update_idme_badges_updated_at
    BEFORE UPDATE ON np_idme_badges
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_idme_attributes_updated_at ON np_idme_attributes;
CREATE TRIGGER update_idme_attributes_updated_at
    BEFORE UPDATE ON np_idme_attributes
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
