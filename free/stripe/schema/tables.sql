-- =============================================================================
-- Stripe Plugin Schema
-- Tables for storing synced Stripe data
-- 23 tables, 6 views
--
-- All tables include source_account_id for multi-account sync support.
-- =============================================================================

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Core Objects
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_customers (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    email VARCHAR(255),
    name VARCHAR(255),
    phone VARCHAR(50),
    description TEXT,
    currency VARCHAR(3),
    default_source VARCHAR(255),
    invoice_prefix VARCHAR(50),
    balance BIGINT DEFAULT 0,
    delinquent BOOLEAN DEFAULT FALSE,
    tax_exempt VARCHAR(20) DEFAULT 'none',
    metadata JSONB DEFAULT '{}',
    address JSONB,
    shipping JSONB,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_customers_email ON stripe_customers(email);
CREATE INDEX IF NOT EXISTS idx_stripe_customers_created ON stripe_customers(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_customers_source_account ON stripe_customers(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_products (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    description TEXT,
    active BOOLEAN DEFAULT TRUE,
    type VARCHAR(20) DEFAULT 'service',
    images JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    attributes JSONB DEFAULT '[]',
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

CREATE TABLE IF NOT EXISTS stripe_prices (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    product_id VARCHAR(255),
    active BOOLEAN DEFAULT TRUE,
    currency VARCHAR(3) NOT NULL,
    unit_amount BIGINT,
    unit_amount_decimal VARCHAR(50),
    type VARCHAR(20) NOT NULL,
    billing_scheme VARCHAR(20) DEFAULT 'per_unit',
    recurring JSONB,
    tiers JSONB,
    tiers_mode VARCHAR(20),
    transform_quantity JSONB,
    lookup_key VARCHAR(255),
    nickname VARCHAR(255),
    tax_behavior VARCHAR(20) DEFAULT 'unspecified',
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
-- Discounts & Promotions
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_coupons (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    name VARCHAR(255),
    amount_off BIGINT,
    percent_off DECIMAL(5,2),
    currency VARCHAR(3),
    duration VARCHAR(20) NOT NULL,
    duration_in_months INTEGER,
    max_redemptions INTEGER,
    times_redeemed INTEGER DEFAULT 0,
    redeem_by TIMESTAMP WITH TIME ZONE,
    valid BOOLEAN DEFAULT TRUE,
    applies_to JSONB,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_coupons_source_account ON stripe_coupons(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_promotion_codes (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    coupon_id VARCHAR(255),
    code VARCHAR(255) NOT NULL,
    customer_id VARCHAR(255),
    active BOOLEAN DEFAULT TRUE,
    max_redemptions INTEGER,
    times_redeemed INTEGER DEFAULT 0,
    expires_at TIMESTAMP WITH TIME ZONE,
    restrictions JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_promo_codes_code ON stripe_promotion_codes(code);
CREATE INDEX IF NOT EXISTS idx_stripe_promo_codes_coupon ON stripe_promotion_codes(coupon_id);
CREATE INDEX IF NOT EXISTS idx_stripe_promo_codes_source_account ON stripe_promotion_codes(source_account_id);

-- =============================================================================
-- Billing Objects
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_subscriptions (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    status VARCHAR(20) NOT NULL,
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
    items JSONB NOT NULL DEFAULT '[]',
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

CREATE TABLE IF NOT EXISTS stripe_subscription_items (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    subscription_id VARCHAR(255),
    price_id VARCHAR(255),
    quantity INTEGER DEFAULT 1,
    billing_thresholds JSONB,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_sub_items_subscription ON stripe_subscription_items(subscription_id);
CREATE INDEX IF NOT EXISTS idx_stripe_sub_items_source_account ON stripe_subscription_items(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_subscription_schedules (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    subscription_id VARCHAR(255),
    status VARCHAR(20) NOT NULL,
    current_phase JSONB,
    default_settings JSONB DEFAULT '{}',
    end_behavior VARCHAR(20),
    phases JSONB DEFAULT '[]',
    released_at TIMESTAMP WITH TIME ZONE,
    released_subscription VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_sub_schedules_customer ON stripe_subscription_schedules(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_sub_schedules_source_account ON stripe_subscription_schedules(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_invoices (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    subscription_id VARCHAR(255),
    status VARCHAR(20),
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
    billing_reason VARCHAR(50),
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
    lines JSONB DEFAULT '[]',
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

CREATE TABLE IF NOT EXISTS stripe_invoice_items (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    invoice_id VARCHAR(255),
    subscription_id VARCHAR(255),
    subscription_item_id VARCHAR(255),
    price_id VARCHAR(255),
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    description TEXT,
    discountable BOOLEAN DEFAULT TRUE,
    quantity INTEGER DEFAULT 1,
    unit_amount BIGINT,
    unit_amount_decimal VARCHAR(50),
    period_start TIMESTAMP WITH TIME ZONE,
    period_end TIMESTAMP WITH TIME ZONE,
    proration BOOLEAN DEFAULT FALSE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_invoice_items_invoice ON stripe_invoice_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_stripe_invoice_items_customer ON stripe_invoice_items(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_invoice_items_source_account ON stripe_invoice_items(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_credit_notes (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    invoice_id VARCHAR(255),
    customer_id VARCHAR(255),
    type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    amount BIGINT NOT NULL,
    subtotal BIGINT NOT NULL,
    subtotal_excluding_tax BIGINT,
    total BIGINT NOT NULL,
    total_excluding_tax BIGINT,
    discount_amount BIGINT DEFAULT 0,
    out_of_band_amount BIGINT,
    reason VARCHAR(50),
    memo TEXT,
    number VARCHAR(255),
    pdf TEXT,
    voided_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_credit_notes_invoice ON stripe_credit_notes(invoice_id);
CREATE INDEX IF NOT EXISTS idx_stripe_credit_notes_customer ON stripe_credit_notes(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_credit_notes_source_account ON stripe_credit_notes(source_account_id);

-- =============================================================================
-- Payment Objects
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_charges (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    payment_intent_id VARCHAR(255),
    invoice_id VARCHAR(255),
    amount BIGINT NOT NULL,
    amount_captured BIGINT DEFAULT 0,
    amount_refunded BIGINT DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    paid BOOLEAN DEFAULT FALSE,
    captured BOOLEAN DEFAULT FALSE,
    refunded BOOLEAN DEFAULT FALSE,
    disputed BOOLEAN DEFAULT FALSE,
    failure_code VARCHAR(100),
    failure_message TEXT,
    outcome JSONB,
    description TEXT,
    receipt_email VARCHAR(255),
    receipt_number VARCHAR(255),
    receipt_url TEXT,
    statement_descriptor VARCHAR(22),
    statement_descriptor_suffix VARCHAR(22),
    payment_method_id VARCHAR(255),
    payment_method_details JSONB,
    billing_details JSONB,
    shipping JSONB,
    fraud_details JSONB,
    balance_transaction_id VARCHAR(255),
    application_fee_id VARCHAR(255),
    application_fee_amount BIGINT,
    transfer_id VARCHAR(255),
    transfer_group VARCHAR(255),
    on_behalf_of VARCHAR(255),
    source_transfer VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_charges_customer ON stripe_charges(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_charges_payment_intent ON stripe_charges(payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_stripe_charges_invoice ON stripe_charges(invoice_id);
CREATE INDEX IF NOT EXISTS idx_stripe_charges_status ON stripe_charges(status);
CREATE INDEX IF NOT EXISTS idx_stripe_charges_created ON stripe_charges(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_charges_source_account ON stripe_charges(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_refunds (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    charge_id VARCHAR(255),
    payment_intent_id VARCHAR(255),
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    reason VARCHAR(50),
    receipt_number VARCHAR(255),
    description TEXT,
    failure_balance_transaction VARCHAR(255),
    failure_reason VARCHAR(100),
    balance_transaction_id VARCHAR(255),
    source_transfer_reversal VARCHAR(255),
    transfer_reversal VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_refunds_charge ON stripe_refunds(charge_id);
CREATE INDEX IF NOT EXISTS idx_stripe_refunds_payment_intent ON stripe_refunds(payment_intent_id);
CREATE INDEX IF NOT EXISTS idx_stripe_refunds_source_account ON stripe_refunds(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_disputes (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    charge_id VARCHAR(255),
    payment_intent_id VARCHAR(255),
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(30) NOT NULL,
    reason VARCHAR(50) NOT NULL,
    is_charge_refundable BOOLEAN DEFAULT FALSE,
    balance_transactions JSONB DEFAULT '[]',
    evidence JSONB DEFAULT '{}',
    evidence_details JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_disputes_charge ON stripe_disputes(charge_id);
CREATE INDEX IF NOT EXISTS idx_stripe_disputes_status ON stripe_disputes(status);
CREATE INDEX IF NOT EXISTS idx_stripe_disputes_source_account ON stripe_disputes(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_payment_intents (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    invoice_id VARCHAR(255),
    amount BIGINT NOT NULL,
    amount_capturable BIGINT DEFAULT 0,
    amount_received BIGINT DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(30) NOT NULL,
    capture_method VARCHAR(20) DEFAULT 'automatic',
    confirmation_method VARCHAR(20) DEFAULT 'automatic',
    payment_method_id VARCHAR(255),
    payment_method_types JSONB DEFAULT '["card"]',
    setup_future_usage VARCHAR(20),
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

CREATE TABLE IF NOT EXISTS stripe_setup_intents (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    payment_method_id VARCHAR(255),
    status VARCHAR(30) NOT NULL,
    usage VARCHAR(20) DEFAULT 'off_session',
    payment_method_types JSONB DEFAULT '["card"]',
    client_secret VARCHAR(255),
    description TEXT,
    cancellation_reason VARCHAR(50),
    last_setup_error JSONB,
    next_action JSONB,
    single_use_mandate VARCHAR(255),
    mandate VARCHAR(255),
    on_behalf_of VARCHAR(255),
    application VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_setup_intents_customer ON stripe_setup_intents(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_setup_intents_status ON stripe_setup_intents(status);
CREATE INDEX IF NOT EXISTS idx_stripe_setup_intents_source_account ON stripe_setup_intents(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_payment_methods (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    type VARCHAR(30) NOT NULL,
    billing_details JSONB,
    card JSONB,
    bank_account JSONB,
    sepa_debit JSONB,
    us_bank_account JSONB,
    link JSONB,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_customer ON stripe_payment_methods(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_type ON stripe_payment_methods(type);
CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_source_account ON stripe_payment_methods(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_balance_transactions (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    net BIGINT NOT NULL,
    fee BIGINT DEFAULT 0,
    fee_details JSONB DEFAULT '[]',
    type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    description TEXT,
    source VARCHAR(255),
    reporting_category VARCHAR(50),
    available_on TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_balance_txns_type ON stripe_balance_transactions(type);
CREATE INDEX IF NOT EXISTS idx_stripe_balance_txns_created ON stripe_balance_transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_stripe_balance_txns_source ON stripe_balance_transactions(source);
CREATE INDEX IF NOT EXISTS idx_stripe_balance_tx_source_account ON stripe_balance_transactions(source_account_id);

-- =============================================================================
-- Checkout Objects
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_checkout_sessions (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    customer_email VARCHAR(255),
    payment_intent_id VARCHAR(255),
    subscription_id VARCHAR(255),
    invoice_id VARCHAR(255),
    mode VARCHAR(20) NOT NULL,
    status VARCHAR(20),
    payment_status VARCHAR(20),
    currency VARCHAR(3),
    amount_total BIGINT,
    amount_subtotal BIGINT,
    total_details JSONB,
    success_url TEXT,
    cancel_url TEXT,
    url TEXT,
    client_reference_id VARCHAR(255),
    customer_creation VARCHAR(20),
    billing_address_collection VARCHAR(20),
    shipping_address_collection JSONB,
    shipping_cost JSONB,
    shipping_details JSONB,
    custom_text JSONB,
    consent JSONB,
    consent_collection JSONB,
    expires_at TIMESTAMP WITH TIME ZONE,
    livemode BOOLEAN DEFAULT TRUE,
    locale VARCHAR(10),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_checkout_customer ON stripe_checkout_sessions(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_checkout_status ON stripe_checkout_sessions(status);
CREATE INDEX IF NOT EXISTS idx_stripe_checkout_payment_status ON stripe_checkout_sessions(payment_status);
CREATE INDEX IF NOT EXISTS idx_stripe_checkout_sessions_source_account ON stripe_checkout_sessions(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_checkout_session_line_items (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    session_id VARCHAR(255),
    price_id VARCHAR(255),
    product_id VARCHAR(255),
    description TEXT,
    quantity INTEGER,
    amount_total BIGINT NOT NULL,
    amount_subtotal BIGINT NOT NULL,
    amount_discount BIGINT DEFAULT 0,
    amount_tax BIGINT DEFAULT 0,
    currency VARCHAR(3) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_stripe_checkout_items_session ON stripe_checkout_session_line_items(session_id);
CREATE INDEX IF NOT EXISTS idx_stripe_checkout_items_source_account ON stripe_checkout_session_line_items(source_account_id);

-- =============================================================================
-- Tax Objects
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_tax_ids (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    customer_id VARCHAR(255),
    type VARCHAR(50) NOT NULL,
    value VARCHAR(255) NOT NULL,
    country VARCHAR(2),
    verification JSONB,
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_tax_ids_customer ON stripe_tax_ids(customer_id);
CREATE INDEX IF NOT EXISTS idx_stripe_tax_ids_source_account ON stripe_tax_ids(source_account_id);

CREATE TABLE IF NOT EXISTS stripe_tax_rates (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    display_name VARCHAR(255) NOT NULL,
    description TEXT,
    percentage DECIMAL(5,4) NOT NULL,
    inclusive BOOLEAN DEFAULT FALSE,
    active BOOLEAN DEFAULT TRUE,
    country VARCHAR(2),
    state VARCHAR(50),
    jurisdiction VARCHAR(255),
    tax_type VARCHAR(50),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_tax_rates_active ON stripe_tax_rates(active);
CREATE INDEX IF NOT EXISTS idx_stripe_tax_rates_source_account ON stripe_tax_rates(source_account_id);

-- =============================================================================
-- Webhook Events (for audit and replay)
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    type VARCHAR(100) NOT NULL,
    api_version VARCHAR(50),
    data JSONB NOT NULL,
    object_type VARCHAR(100),
    object_id VARCHAR(255),
    request_id VARCHAR(255),
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
ALTER TABLE IF EXISTS stripe_coupons ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_promotion_codes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_subscriptions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_subscription_items ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_subscription_schedules ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_invoices ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_invoice_items ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_credit_notes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_charges ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_refunds ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_disputes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_payment_intents ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_setup_intents ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_payment_methods ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_balance_transactions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_checkout_sessions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_checkout_session_line_items ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_tax_ids ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_tax_rates ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
ALTER TABLE IF EXISTS stripe_webhook_events ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';

-- =============================================================================
-- Analytics Views
-- =============================================================================

-- Active subscriptions with customer info
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
LEFT JOIN stripe_customers c ON s.customer_id = c.id AND s.source_account_id = c.source_account_id
WHERE s.status IN ('active', 'trialing', 'past_due')
  AND (c.deleted_at IS NULL OR c.id IS NULL);

-- Monthly recurring revenue calculation
CREATE OR REPLACE VIEW stripe_mrr AS
SELECT
    DATE_TRUNC('month', s.created_at) AS month,
    COUNT(*) AS subscription_count,
    SUM(
      CASE
        WHEN (s.items->0->'price'->'recurring'->>'interval') = 'month'
        THEN COALESCE((s.items->0->'price'->>'unit_amount')::BIGINT, 0)
        WHEN (s.items->0->'price'->'recurring'->>'interval') = 'year'
        THEN COALESCE((s.items->0->'price'->>'unit_amount')::BIGINT, 0) / 12
        ELSE 0
      END
    ) AS mrr_cents
FROM stripe_subscriptions s
WHERE s.status IN ('active', 'trialing')
GROUP BY DATE_TRUNC('month', s.created_at)
ORDER BY month DESC;

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
LEFT JOIN stripe_customers c ON pi.customer_id = c.id AND pi.source_account_id = c.source_account_id
WHERE pi.status IN ('requires_payment_method', 'canceled')
  AND pi.last_payment_error IS NOT NULL
ORDER BY pi.created_at DESC;

-- Revenue by product
CREATE OR REPLACE VIEW stripe_revenue_by_product AS
SELECT
    p.id AS product_id,
    p.name AS product_name,
    c.source_account_id,
    COUNT(DISTINCT c.id) AS charge_count,
    SUM(c.amount) AS total_amount,
    c.currency
FROM stripe_charges c
JOIN stripe_invoices i ON c.invoice_id = i.id AND c.source_account_id = i.source_account_id
JOIN stripe_prices pr ON i.lines->0->>'price' = pr.id AND i.source_account_id = pr.source_account_id
JOIN stripe_products p ON pr.product_id = p.id AND pr.source_account_id = p.source_account_id
WHERE c.status = 'succeeded'
GROUP BY p.id, p.name, c.currency, c.source_account_id
ORDER BY total_amount DESC;

-- Dispute summary
CREATE OR REPLACE VIEW stripe_dispute_summary AS
SELECT
    d.status,
    d.reason,
    COUNT(*) AS dispute_count,
    SUM(d.amount) AS total_amount,
    d.currency
FROM stripe_disputes d
GROUP BY d.status, d.reason, d.currency
ORDER BY dispute_count DESC;

-- Daily revenue
CREATE OR REPLACE VIEW stripe_daily_revenue AS
SELECT
    DATE(c.created_at) AS date,
    COUNT(*) AS charge_count,
    SUM(c.amount) AS gross_amount,
    SUM(c.amount_refunded) AS refunded_amount,
    SUM(c.amount - c.amount_refunded) AS net_amount,
    c.currency
FROM stripe_charges c
WHERE c.status = 'succeeded'
GROUP BY DATE(c.created_at), c.currency
ORDER BY date DESC;

-- Unified payments: merges charges across all accounts into one queryable table
CREATE OR REPLACE VIEW stripe_unified_payments AS
SELECT
    c.id AS payment_id,
    'charge' AS payment_type,
    c.source_account_id,
    c.customer_id,
    cust.email AS customer_email,
    cust.name AS customer_name,
    c.amount,
    c.amount_refunded,
    (c.amount - c.amount_refunded) AS net_amount,
    c.currency,
    c.status,
    c.description,
    c.invoice_id,
    c.payment_intent_id,
    c.payment_method_id,
    c.receipt_email,
    c.metadata,
    c.created_at
FROM stripe_charges c
LEFT JOIN stripe_customers cust ON c.customer_id = cust.id AND c.source_account_id = cust.source_account_id
WHERE c.status = 'succeeded'
ORDER BY c.created_at DESC;

-- =============================================================================
-- Webhook Idempotency (S76-T05)
-- Tracks processed Stripe event IDs to prevent double-handling.
-- Pattern mirrors ping_api's stripe_events table so both canonical paths
-- share the same dedup guarantee (see decisions.md: Stripe webhook canonical path).
-- =============================================================================

CREATE TABLE IF NOT EXISTS stripe_events (
    stripe_event_id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_type VARCHAR(128) NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Fast dedup lookup — must be indexed for O(1) idempotency check.
CREATE INDEX IF NOT EXISTS idx_stripe_events_source_account ON stripe_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_stripe_events_processed_at ON stripe_events(processed_at);
