-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh): the idme_* tables an install from before the np_ prefix had
-- (installer-created, no source_account_id column), one row each, plus two of their old-named indexes. Migration 000
-- renames them to np_idme_*. Test data only: fake ids and addresses.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS idme_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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

CREATE TABLE IF NOT EXISTS idme_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    verification_id UUID NOT NULL REFERENCES idme_verifications(id) ON DELETE CASCADE,
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

CREATE TABLE IF NOT EXISTS idme_badges (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    verification_id UUID NOT NULL REFERENCES idme_verifications(id) ON DELETE CASCADE,
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

CREATE TABLE IF NOT EXISTS idme_attributes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    verification_id UUID NOT NULL REFERENCES idme_verifications(id) ON DELETE CASCADE,
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

CREATE TABLE IF NOT EXISTS idme_webhook_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id VARCHAR(255) UNIQUE,                   -- ID.me event ID
    event_type VARCHAR(100) NOT NULL,               -- verification.created, group.verified, etc.
    user_id UUID,
    verification_id UUID REFERENCES idme_verifications(id),
    payload JSONB NOT NULL,                         -- Full webhook payload
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO idme_verifications (id, user_id, idme_user_id, email, verified, verification_level)
VALUES ('00000000-0000-4000-8000-000000000101', '00000000-0000-4000-8000-000000000190', 'idme-legacy-user', 'legacy@example.invalid', TRUE, 'identity');

INSERT INTO idme_groups (id, verification_id, user_id, group_type, group_name, verified)
VALUES ('00000000-0000-4000-8000-000000000102', '00000000-0000-4000-8000-000000000101', '00000000-0000-4000-8000-000000000190', 'veteran', 'Veteran', TRUE);

INSERT INTO idme_badges (id, verification_id, user_id, badge_type, badge_name)
VALUES ('00000000-0000-4000-8000-000000000103', '00000000-0000-4000-8000-000000000101', '00000000-0000-4000-8000-000000000190', 'veteran', 'Veteran');

INSERT INTO idme_attributes (id, verification_id, user_id, attribute_key, attribute_value)
VALUES ('00000000-0000-4000-8000-000000000104', '00000000-0000-4000-8000-000000000101', '00000000-0000-4000-8000-000000000190', 'branch', 'legacy-branch');

INSERT INTO idme_webhook_events (id, event_id, event_type, verification_id, payload)
VALUES ('00000000-0000-4000-8000-000000000105', 'legacy-event', 'verification.created', '00000000-0000-4000-8000-000000000101', '{}');

CREATE INDEX idx_idme_verifications_user ON idme_verifications(user_id);
CREATE INDEX idx_idme_groups_type ON idme_groups(group_type);
