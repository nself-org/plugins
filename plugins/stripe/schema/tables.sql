-- =============================================================================
-- Stripe Plugin Schema
-- Tables for storing synced Stripe data
-- =============================================================================

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Customers
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_customers (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe customer ID (cus_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    email VARCHAR(255),
    name VARCHAR(255),
    phone VARCHAR(50),
    description TEXT,
    currency VARCHAR(3),
    default_source VARCHAR(255),                    -- Default payment source ID
    invoice_prefix VARCHAR(50),
    balance BIGINT DEFAULT 0,                       -- Customer balance in cents
    delinquent BOOLEAN DEFAULT FALSE,
    tax_exempt VARCHAR(20) DEFAULT 'none',          -- none, exempt, reverse
    metadata JSONB DEFAULT '{}',
    address JSONB,                                  -- Billing address
    shipping JSONB,                                 -- Shipping address
    created_at TIMESTAMP WITH TIME ZONE,            -- Stripe created timestamp
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,            -- Soft delete
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_customers_email ON stripe_customers(email);
CREATE INDEX IF NOT EXISTS idx_stripe_customers_created ON stripe_customers(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_customers_source_account ON stripe_customers(source_account_id);

-- =============================================================================
-- Products
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_products (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe product ID (prod_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    description TEXT,
    active BOOLEAN DEFAULT TRUE,
    type VARCHAR(20) DEFAULT 'service',             -- service, good
    images JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    attributes JSONB DEFAULT '[]',                  -- Product attributes
    shippable BOOLEAN,
    statement_descriptor VARCHAR(22),
    tax_code VARCHAR(255),
    unit_label VARCHAR(255),
    url VARCHAR(2048),
    default_price_id VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_products_active ON stripe_products(active);
CREATE INDEX IF NOT EXISTS idx_stripe_products_source_account ON stripe_products(source_account_id);

-- =============================================================================
-- Prices
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_prices (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe price ID (price_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    product_id VARCHAR(255) REFERENCES stripe_products(id),
    active BOOLEAN DEFAULT TRUE,
    currency VARCHAR(3) NOT NULL,
    unit_amount BIGINT,                             -- Amount in cents
    unit_amount_decimal VARCHAR(50),                -- For sub-cent precision
    type VARCHAR(20) NOT NULL,                      -- one_time, recurring
    billing_scheme VARCHAR(20) DEFAULT 'per_unit',  -- per_unit, tiered
    recurring JSONB,                                -- Recurring billing config
    tiers JSONB,                                    -- Tier pricing
    tiers_mode VARCHAR(20),                         -- graduated, volume
    transform_quantity JSONB,
    lookup_key VARCHAR(255),
    nickname VARCHAR(255),
    tax_behavior VARCHAR(20) DEFAULT 'unspecified', -- inclusive, exclusive, unspecified
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_prices_product ON stripe_prices(product_id);
CREATE INDEX IF NOT EXISTS idx_stripe_prices_active ON stripe_prices(active);
CREATE INDEX IF NOT EXISTS idx_stripe_prices_lookup_key ON stripe_prices(lookup_key);
CREATE INDEX IF NOT EXISTS idx_stripe_prices_source_account ON stripe_prices(source_account_id);

-- =============================================================================
-- Subscriptions
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_subscriptions (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe subscription ID (sub_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255) REFERENCES stripe_customers(id),
    status VARCHAR(20) NOT NULL,                    -- active, past_due, canceled, etc.
    current_period_start TIMESTAMP WITH TIME ZONE,
    current_period_end TIMESTAMP WITH TIME ZONE,
    cancel_at TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    ended_at TIMESTAMP WITH TIME ZONE,
    trial_start TIMESTAMP WITH TIME ZONE,
    trial_end TIMESTAMP WITH TIME ZONE,
    collection_method VARCHAR(20) DEFAULT 'charge_automatically',
    billing_cycle_anchor TIMESTAMP WITH TIME ZONE,
    billing_thresholds JSONB,
    days_until_due INTEGER,
    default_payment_method_id VARCHAR(255),
    default_source VARCHAR(255),
    discount JSONB,
    items JSONB NOT NULL DEFAULT '[]',              -- Subscription items
    latest_invoice_id VARCHAR(255),
    pending_setup_intent VARCHAR(255),
    pending_update JSONB,
    schedule_id VARCHAR(255),
    start_date TIMESTAMP WITH TIME ZONE,
    transfer_data JSONB,
    application_fee_percent DECIMAL(5,2),
    automatic_tax JSONB,
    payment_settings JSONB,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_customer ON stripe_subscriptions(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_status ON stripe_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_period ON stripe_subscriptions(current_period_end);
CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_source_account ON stripe_subscriptions(source_account_id);

-- =============================================================================
-- Invoices
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_invoices (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe invoice ID (in_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255) REFERENCES stripe_customers(id),
    subscription_id VARCHAR(255) REFERENCES stripe_subscriptions(id),
    status VARCHAR(20),                             -- draft, open, paid, uncollectible, void
    collection_method VARCHAR(20),
    currency VARCHAR(3) NOT NULL,
    amount_due BIGINT NOT NULL,
    amount_paid BIGINT DEFAULT 0,
    amount_remaining BIGINT DEFAULT 0,
    subtotal BIGINT NOT NULL,
    subtotal_excluding_tax BIGINT,
    total BIGINT NOT NULL,
    total_excluding_tax BIGINT,
    tax BIGINT,
    total_tax_amounts JSONB DEFAULT '[]',
    discount JSONB,
    discounts JSONB DEFAULT '[]',
    account_country VARCHAR(2),
    account_name VARCHAR(255),
    billing_reason VARCHAR(50),                     -- subscription_create, subscription_cycle, etc.
    number VARCHAR(255),
    receipt_number VARCHAR(255),
    statement_descriptor VARCHAR(255),
    description TEXT,
    footer TEXT,
    customer_email VARCHAR(255),
    customer_name VARCHAR(255),
    customer_address JSONB,
    customer_phone VARCHAR(50),
    customer_shipping JSONB,
    customer_tax_exempt VARCHAR(20),
    customer_tax_ids JSONB DEFAULT '[]',
    default_payment_method_id VARCHAR(255),
    default_source VARCHAR(255),
    lines JSONB DEFAULT '[]',                       -- Invoice line items
    hosted_invoice_url TEXT,
    invoice_pdf TEXT,
    payment_intent_id VARCHAR(255),
    charge_id VARCHAR(255),
    attempt_count INTEGER DEFAULT 0,
    attempted BOOLEAN DEFAULT FALSE,
    auto_advance BOOLEAN DEFAULT TRUE,
    next_payment_attempt TIMESTAMP WITH TIME ZONE,
    webhooks_delivered_at TIMESTAMP WITH TIME ZONE,
    paid BOOLEAN DEFAULT FALSE,
    paid_out_of_band BOOLEAN DEFAULT FALSE,
    period_start TIMESTAMP WITH TIME ZONE,
    period_end TIMESTAMP WITH TIME ZONE,
    due_date TIMESTAMP WITH TIME ZONE,
    effective_at TIMESTAMP WITH TIME ZONE,
    finalized_at TIMESTAMP WITH TIME ZONE,
    marked_uncollectible_at TIMESTAMP WITH TIME ZONE,
    voided_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_invoices_customer ON stripe_invoices(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_invoices_subscription ON stripe_invoices(subscription_id);
CREATE INDEX IF NOT EXISTS idx_stripe_invoices_status ON stripe_invoices(status);
CREATE INDEX IF NOT EXISTS idx_stripe_invoices_created ON stripe_invoices(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_invoices_source_account ON stripe_invoices(source_account_id);

-- =============================================================================
-- Payment Intents
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_payment_intents (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe payment intent ID (pi_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255) REFERENCES stripe_customers(id),
    invoice_id VARCHAR(255) REFERENCES stripe_invoices(id),
    amount BIGINT NOT NULL,
    amount_capturable BIGINT DEFAULT 0,
    amount_received BIGINT DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(30) NOT NULL,                    -- requires_payment_method, succeeded, etc.
    capture_method VARCHAR(20) DEFAULT 'automatic', -- automatic, manual
    confirmation_method VARCHAR(20) DEFAULT 'automatic',
    payment_method_id VARCHAR(255),
    payment_method_types JSONB DEFAULT '["card"]',
    setup_future_usage VARCHAR(20),                 -- on_session, off_session
    client_secret VARCHAR(255),
    description TEXT,
    receipt_email VARCHAR(255),
    statement_descriptor VARCHAR(22),
    statement_descriptor_suffix VARCHAR(22),
    shipping JSONB,
    application_fee_amount BIGINT,
    transfer_data JSONB,
    transfer_group VARCHAR(255),
    on_behalf_of VARCHAR(255),
    cancellation_reason VARCHAR(50),
    canceled_at TIMESTAMP WITH TIME ZONE,
    charges JSONB DEFAULT '[]',
    last_payment_error JSONB,
    next_action JSONB,
    processing JSONB,
    review VARCHAR(255),
    automatic_payment_methods JSONB,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_customer ON stripe_payment_intents(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_invoice ON stripe_payment_intents(invoice_id);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_status ON stripe_payment_intents(status);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_created ON stripe_payment_intents(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_source_account ON stripe_payment_intents(source_account_id);

-- =============================================================================
-- Payment Methods
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_payment_methods (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe payment method ID (pm_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255) REFERENCES stripe_customers(id),
    type VARCHAR(30) NOT NULL,                      -- card, bank_account, etc.
    billing_details JSONB,
    card JSONB,                                     -- Card details (brand, last4, exp_month, etc.)
    bank_account JSONB,                             -- Bank account details
    sepa_debit JSONB,                               -- SEPA debit details
    us_bank_account JSONB,                          -- US bank account details
    link JSONB,                                     -- Link details
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_customer ON stripe_payment_methods(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_type ON stripe_payment_methods(type);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_source_account ON stripe_payment_methods(source_account_id);

-- =============================================================================
-- Webhook Events (for audit and replay)
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_webhook_events (
    id VARCHAR(255) PRIMARY KEY,                    -- Stripe event ID (evt_xxx)
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    type VARCHAR(100) NOT NULL,                     -- Event type (customer.created, etc.)
    api_version VARCHAR(50),
    data JSONB NOT NULL,                            -- Full event data
    object_type VARCHAR(100),                       -- Type of object (customer, invoice, etc.)
    object_id VARCHAR(255),                         -- ID of the object
    request_id VARCHAR(255),                        -- Stripe request ID
    request_idempotency_key VARCHAR(255),
    livemode BOOLEAN DEFAULT TRUE,
    pending_webhooks INTEGER DEFAULT 0,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_type ON stripe_webhook_events(type);
CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_object ON stripe_webhook_events(object_type, object_id);
CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_processed ON stripe_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_created ON stripe_webhook_events(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_source_account ON stripe_webhook_events(source_account_id);

-- =============================================================================
-- Multi-account source tracking backfill (existing installs)
-- =============================================================================

ALTER TABLE IF EXISTS stripe_customers ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_products ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_prices ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_subscriptions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_invoices ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_payment_intents ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_payment_methods ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_webhook_events ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';

-- =============================================================================
-- Views for common queries
-- =============================================================================

-- Active subscriptions with customer and product info
CREATE OR REPLACE VIEW stripe_active_subscriptions AS
SELECT
    s.id AS subscription_id,
    s.source_account_id,
    s.status,
    c.id AS customer_id,
    c.email AS customer_email,
    c.name AS customer_name,
    s.current_period_start,
    s.current_period_end,
    s.cancel_at_period_end,
    s.items,
    s.metadata
FROM stripe_subscriptions s
JOIN stripe_customers c ON s.customer_id = c.id AND s.source_account_id = c.source_account_id
WHERE s.status IN ('active', 'trialing', 'past_due')
  AND c.deleted_at IS NULL;

-- Monthly recurring revenue calculation
CREATE OR REPLACE VIEW stripe_mrr AS
SELECT
    DATE_TRUNC('month', s.created_at) AS month,
    SUM(
        CASE
            WHEN (s.items->0->'price'->>'recurring'->>'interval') = 'month'
            THEN (s.items->0->'price'->>'unit_amount')::BIGINT
            WHEN (s.items->0->'price'->>'recurring'->>'interval') = 'year'
            THEN (s.items->0->'price'->>'unit_amount')::BIGINT / 12
            ELSE 0
        END
    ) AS mrr_cents
FROM stripe_subscriptions s
WHERE s.status IN ('active', 'trialing')
GROUP BY DATE_TRUNC('month', s.created_at)
ORDER BY month;

-- Recent failed payments
CREATE OR REPLACE VIEW stripe_failed_payments AS
SELECT
    pi.id AS payment_intent_id,
    pi.source_account_id,
    pi.amount,
    pi.currency,
    pi.status,
    pi.last_payment_error,
    c.id AS customer_id,
    c.email AS customer_email,
    c.name AS customer_name,
    pi.created_at
FROM stripe_payment_intents pi
JOIN stripe_customers c ON pi.customer_id = c.id AND pi.source_account_id = c.source_account_id
WHERE pi.status IN ('requires_payment_method', 'canceled')
  AND pi.last_payment_error IS NOT NULL
ORDER BY pi.created_at DESC;
