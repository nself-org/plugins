-- donorbox plugin: initial schema
-- CODE WINS: 7 tables from internal/db.go (np_donorbox_* prefix)
-- Tables: np_donorbox_campaigns, np_donorbox_donors, np_donorbox_donations,
--         np_donorbox_plans, np_donorbox_events, np_donorbox_tickets,
--         np_donorbox_webhook_events
-- Note: spec said "subscriptions/refunds/plans" but code uses "events/tickets/plans"

CREATE TABLE IF NOT EXISTS np_donorbox_campaigns (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255),
    slug VARCHAR(255),
    currency VARCHAR(10) DEFAULT 'USD',
    goal_amount NUMERIC(20, 2),
    total_raised NUMERIC(20, 2) DEFAULT 0,
    donations_count INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_campaigns_active ON np_donorbox_campaigns(is_active);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_campaigns_account ON np_donorbox_campaigns(source_account_id);

CREATE TABLE IF NOT EXISTS np_donorbox_donors (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    email VARCHAR(255),
    phone VARCHAR(50),
    address TEXT,
    city VARCHAR(255),
    state VARCHAR(100),
    zip_code VARCHAR(20),
    country VARCHAR(100),
    employer VARCHAR(255),
    donations_count INTEGER DEFAULT 0,
    last_donation_at TIMESTAMPTZ,
    total NUMERIC(20, 2) DEFAULT 0,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_donors_email ON np_donorbox_donors(email);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_donors_account ON np_donorbox_donors(source_account_id);

CREATE TABLE IF NOT EXISTS np_donorbox_donations (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    campaign_id INTEGER,
    campaign_name VARCHAR(255),
    donor_id INTEGER,
    donor_email VARCHAR(255),
    donor_name VARCHAR(255),
    amount NUMERIC(20, 2) DEFAULT 0,
    converted_amount NUMERIC(20, 2),
    converted_net_amount NUMERIC(20, 2),
    amount_refunded NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    donation_type VARCHAR(50),
    donation_date TIMESTAMPTZ,
    processing_fee NUMERIC(20, 2),
    status VARCHAR(50),
    recurring BOOLEAN DEFAULT false,
    comment TEXT,
    designation VARCHAR(255),
    stripe_charge_id VARCHAR(255),
    paypal_transaction_id VARCHAR(255),
    questions JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_campaign ON np_donorbox_donations(campaign_id);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_donor ON np_donorbox_donations(donor_id);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_date ON np_donorbox_donations(donation_date DESC);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_status ON np_donorbox_donations(status);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_donations_account ON np_donorbox_donations(source_account_id);

CREATE TABLE IF NOT EXISTS np_donorbox_plans (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    campaign_id INTEGER,
    campaign_name VARCHAR(255),
    donor_id INTEGER,
    donor_email VARCHAR(255),
    type VARCHAR(50),
    amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    status VARCHAR(50),
    started_at TIMESTAMPTZ,
    last_donation_date TIMESTAMPTZ,
    next_donation_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_plans_status ON np_donorbox_plans(status);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_plans_donor ON np_donorbox_plans(donor_id);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_plans_account ON np_donorbox_plans(source_account_id);

CREATE TABLE IF NOT EXISTS np_donorbox_events (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255),
    slug VARCHAR(255),
    description TEXT,
    start_date TIMESTAMPTZ,
    end_date TIMESTAMPTZ,
    timezone VARCHAR(50),
    venue_name VARCHAR(255),
    address TEXT,
    city VARCHAR(255),
    state VARCHAR(100),
    country VARCHAR(100),
    zip_code VARCHAR(20),
    currency VARCHAR(10) DEFAULT 'USD',
    tickets_count INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_events_active ON np_donorbox_events(is_active);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_events_account ON np_donorbox_events(source_account_id);

CREATE TABLE IF NOT EXISTS np_donorbox_tickets (
    id INTEGER NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_id INTEGER,
    event_name VARCHAR(255),
    donor_id INTEGER,
    donor_email VARCHAR(255),
    ticket_type VARCHAR(100),
    quantity INTEGER DEFAULT 0,
    amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    status VARCHAR(50),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_tickets_event ON np_donorbox_tickets(event_id);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_tickets_account ON np_donorbox_tickets(source_account_id);

CREATE TABLE IF NOT EXISTS np_donorbox_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255),
    payload JSONB DEFAULT '{}',
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMPTZ,
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_donorbox_webhook_events_type ON np_donorbox_webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_np_donorbox_webhook_events_account ON np_donorbox_webhook_events(source_account_id);
