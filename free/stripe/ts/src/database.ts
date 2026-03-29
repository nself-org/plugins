/**
 * Stripe Database Operations
 * Complete CRUD operations for all Stripe objects in PostgreSQL
 * Modeled after supabase/stripe-sync-engine for 100% data coverage
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  StripeCustomerRecord,
  StripeProductRecord,
  StripePriceRecord,
  StripeSubscriptionRecord,
  StripeSubscriptionItemRecord,
  StripeSubscriptionScheduleRecord,
  StripeInvoiceRecord,
  StripeInvoiceItemRecord,
  StripePaymentIntentRecord,
  StripePaymentMethodRecord,
  StripeWebhookEventRecord,
  StripeChargeRecord,
  StripeRefundRecord,
  StripeDisputeRecord,
  StripeCouponRecord,
  StripePromotionCodeRecord,
  StripeSetupIntentRecord,
  StripeCheckoutSessionRecord,
  StripeBalanceTransactionRecord,
  StripeCreditNoteRecord,
  StripeTaxRateRecord,
  StripeTaxIdRecord,
  SyncStats,
} from './types.js';

const logger = createLogger('stripe:db');

export class StripeDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): StripeDatabase {
    return new StripeDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');

    return normalized.length > 0 ? normalized : 'primary';
  }

  private withSourceAccountContext(sql: string, params: unknown[] = []): { sql: string; params: unknown[] } {
    if (!/^\s*INSERT INTO\s+stripe_/i.test(sql) || !/\bON\s+CONFLICT\b/i.test(sql) || /\bsource_account_id\b/i.test(sql)) {
      return { sql, params };
    }

    const accountPlaceholder = `$${params.length + 1}`;

    const withColumn = sql.replace(
      /INSERT INTO\s+(stripe_[a-z_]+)\s*\(([\s\S]*?)\)\s*VALUES/i,
      (_match, tableName: string, columns: string) => {
        return `INSERT INTO ${tableName} (${columns.trimEnd()}, source_account_id) VALUES`;
      }
    );

    const withValue = withColumn.replace(
      /\)\s*VALUES\s*\(([\s\S]*?)\)\s*ON\s+CONFLICT/i,
      (_match, values: string) => {
        return `) VALUES (${values.trimEnd()}, ${accountPlaceholder}) ON CONFLICT`;
      }
    );

    const withUpdate = withValue.replace(
      /DO\s+UPDATE\s+SET\s*/i,
      match => `${match}source_account_id = EXCLUDED.source_account_id, `
    );

    if (withUpdate === sql) {
      return { sql, params };
    }

    return { sql: withUpdate, params: [...params, this.sourceAccountId] };
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    const context = this.withSourceAccountContext(sql, params ?? []);
    return this.db.execute(context.sql, context.params);
  }

  // =========================================================================
  // Schema Management - Complete Stripe Schema
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing complete Stripe schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Core Objects
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_customers (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_products (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_prices (
        id VARCHAR(255) PRIMARY KEY,
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

      -- =====================================================================
      -- Discounts & Promotions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_coupons (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_promotion_codes (
        id VARCHAR(255) PRIMARY KEY,
        coupon_id VARCHAR(255) REFERENCES stripe_coupons(id) ON DELETE SET NULL,
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

      -- =====================================================================
      -- Billing Objects
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_subscriptions (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_subscription_items (
        id VARCHAR(255) PRIMARY KEY,
        subscription_id VARCHAR(255) REFERENCES stripe_subscriptions(id) ON DELETE CASCADE,
        price_id VARCHAR(255),
        quantity INTEGER DEFAULT 1,
        billing_thresholds JSONB,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_stripe_sub_items_subscription ON stripe_subscription_items(subscription_id);

      CREATE TABLE IF NOT EXISTS stripe_subscription_schedules (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_invoices (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_invoice_items (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_credit_notes (
        id VARCHAR(255) PRIMARY KEY,
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

      -- =====================================================================
      -- Payment Objects
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_charges (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_refunds (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_disputes (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_payment_intents (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_setup_intents (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_payment_methods (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_balance_transactions (
        id VARCHAR(255) PRIMARY KEY,
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

      -- =====================================================================
      -- Checkout Objects
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_checkout_sessions (
        id VARCHAR(255) PRIMARY KEY,
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

      CREATE TABLE IF NOT EXISTS stripe_checkout_session_line_items (
        id VARCHAR(255) PRIMARY KEY,
        session_id VARCHAR(255) REFERENCES stripe_checkout_sessions(id) ON DELETE CASCADE,
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

      -- =====================================================================
      -- Tax Objects
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_tax_ids (
        id VARCHAR(255) PRIMARY KEY,
        customer_id VARCHAR(255),
        type VARCHAR(50) NOT NULL,
        value VARCHAR(255) NOT NULL,
        country VARCHAR(2),
        verification JSONB,
        created_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_stripe_tax_ids_customer ON stripe_tax_ids(customer_id);

      CREATE TABLE IF NOT EXISTS stripe_tax_rates (
        id VARCHAR(255) PRIMARY KEY,
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

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS stripe_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
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

      -- =====================================================================
      -- Multi-Account Source Tracking
      -- =====================================================================

      ALTER TABLE stripe_customers ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_products ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_prices ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_coupons ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_promotion_codes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_subscriptions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_subscription_items ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_subscription_schedules ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_invoices ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_invoice_items ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_credit_notes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_charges ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_refunds ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_disputes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_payment_intents ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_setup_intents ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_payment_methods ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_balance_transactions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_checkout_sessions ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_checkout_session_line_items ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_tax_ids ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_tax_rates ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE stripe_webhook_events ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';

      CREATE INDEX IF NOT EXISTS idx_stripe_customers_source_account ON stripe_customers(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_products_source_account ON stripe_products(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_prices_source_account ON stripe_prices(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_coupons_source_account ON stripe_coupons(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_promo_codes_source_account ON stripe_promotion_codes(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_subscriptions_source_account ON stripe_subscriptions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_sub_items_source_account ON stripe_subscription_items(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_sub_schedules_source_account ON stripe_subscription_schedules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_invoices_source_account ON stripe_invoices(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_invoice_items_source_account ON stripe_invoice_items(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_credit_notes_source_account ON stripe_credit_notes(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_charges_source_account ON stripe_charges(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_refunds_source_account ON stripe_refunds(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_disputes_source_account ON stripe_disputes(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_payment_intents_source_account ON stripe_payment_intents(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_setup_intents_source_account ON stripe_setup_intents(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_payment_methods_source_account ON stripe_payment_methods(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_balance_tx_source_account ON stripe_balance_transactions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_checkout_sessions_source_account ON stripe_checkout_sessions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_checkout_items_source_account ON stripe_checkout_session_line_items(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_tax_ids_source_account ON stripe_tax_ids(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_tax_rates_source_account ON stripe_tax_rates(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_stripe_webhook_events_source_account ON stripe_webhook_events(source_account_id);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

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

      CREATE OR REPLACE VIEW stripe_mrr AS
      SELECT
        s.source_account_id,
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
      GROUP BY s.source_account_id, DATE_TRUNC('month', s.created_at)
      ORDER BY month DESC;

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

      CREATE OR REPLACE VIEW stripe_dispute_summary AS
      SELECT
        d.source_account_id,
        d.status,
        d.reason,
        COUNT(*) AS dispute_count,
        SUM(d.amount) AS total_amount,
        d.currency
      FROM stripe_disputes d
      GROUP BY d.source_account_id, d.status, d.reason, d.currency
      ORDER BY dispute_count DESC;

      CREATE OR REPLACE VIEW stripe_daily_revenue AS
      SELECT
        c.source_account_id,
        DATE(c.created_at) AS date,
        COUNT(*) AS charge_count,
        SUM(c.amount) AS gross_amount,
        SUM(c.amount_refunded) AS refunded_amount,
        SUM(c.amount - c.amount_refunded) AS net_amount,
        c.currency
      FROM stripe_charges c
      WHERE c.status = 'succeeded'
      GROUP BY c.source_account_id, DATE(c.created_at), c.currency
      ORDER BY date DESC;

      -- Unified payments view: merges charges across all accounts into one queryable table
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
    `;

    await this.db.executeSqlFile(schema);
    logger.success('Complete Stripe schema initialized (23 tables, 7 views)');
  }

  // =========================================================================
  // Customers
  // =========================================================================

  async upsertCustomer(customer: StripeCustomerRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_customers (
        id, email, name, phone, description, currency, default_source,
        invoice_prefix, balance, delinquent, tax_exempt, metadata,
        address, shipping, created_at, deleted_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
      ON CONFLICT (id) DO UPDATE SET
        email = EXCLUDED.email, name = EXCLUDED.name, phone = EXCLUDED.phone,
        description = EXCLUDED.description, currency = EXCLUDED.currency,
        default_source = EXCLUDED.default_source, invoice_prefix = EXCLUDED.invoice_prefix,
        balance = EXCLUDED.balance, delinquent = EXCLUDED.delinquent,
        tax_exempt = EXCLUDED.tax_exempt, metadata = EXCLUDED.metadata,
        address = EXCLUDED.address, shipping = EXCLUDED.shipping,
        deleted_at = EXCLUDED.deleted_at, updated_at = NOW(), synced_at = NOW()`,
      [
        customer.id, customer.email, customer.name, customer.phone,
        customer.description, customer.currency, customer.default_source,
        customer.invoice_prefix, customer.balance, customer.delinquent,
        customer.tax_exempt, JSON.stringify(customer.metadata),
        customer.address ? JSON.stringify(customer.address) : null,
        customer.shipping ? JSON.stringify(customer.shipping) : null,
        customer.created_at, customer.deleted_at,
      ]
    );
  }

  async upsertCustomers(customers: StripeCustomerRecord[]): Promise<number> {
    for (const customer of customers) await this.upsertCustomer(customer);
    return customers.length;
  }

  async getCustomer(id: string): Promise<StripeCustomerRecord | null> {
    return this.db.queryOne<StripeCustomerRecord>('SELECT * FROM stripe_customers WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listCustomers(limit = 100, offset = 0): Promise<StripeCustomerRecord[]> {
    const result = await this.db.query<StripeCustomerRecord>(
      'SELECT * FROM stripe_customers WHERE deleted_at IS NULL AND source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  async countCustomers(): Promise<number> {
    return this.db.countScoped('stripe_customers', this.sourceAccountId, 'deleted_at IS NULL');
  }

  async markCustomerDeleted(id: string): Promise<void> {
    await this.execute('UPDATE stripe_customers SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  // =========================================================================
  // Products
  // =========================================================================

  async upsertProduct(product: StripeProductRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_products (
        id, name, description, active, type, images, metadata, attributes,
        shippable, statement_descriptor, tax_code, unit_label, url,
        default_price_id, created_at, deleted_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
      ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name, description = EXCLUDED.description, active = EXCLUDED.active,
        type = EXCLUDED.type, images = EXCLUDED.images, metadata = EXCLUDED.metadata,
        attributes = EXCLUDED.attributes, shippable = EXCLUDED.shippable,
        statement_descriptor = EXCLUDED.statement_descriptor, tax_code = EXCLUDED.tax_code,
        unit_label = EXCLUDED.unit_label, url = EXCLUDED.url,
        default_price_id = EXCLUDED.default_price_id, deleted_at = EXCLUDED.deleted_at,
        updated_at = NOW(), synced_at = NOW()`,
      [
        product.id, product.name, product.description, product.active, product.type,
        JSON.stringify(product.images), JSON.stringify(product.metadata),
        JSON.stringify(product.attributes), product.shippable, product.statement_descriptor,
        product.tax_code, product.unit_label, product.url, product.default_price_id,
        product.created_at, product.deleted_at,
      ]
    );
  }

  async upsertProducts(products: StripeProductRecord[]): Promise<number> {
    for (const product of products) await this.upsertProduct(product);
    return products.length;
  }

  async getProduct(id: string): Promise<StripeProductRecord | null> {
    return this.db.queryOne<StripeProductRecord>('SELECT * FROM stripe_products WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listProducts(limit = 100, offset = 0): Promise<StripeProductRecord[]> {
    const result = await this.db.query<StripeProductRecord>(
      'SELECT * FROM stripe_products WHERE deleted_at IS NULL AND source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  async countProducts(): Promise<number> {
    return this.db.countScoped('stripe_products', this.sourceAccountId, 'deleted_at IS NULL');
  }

  // =========================================================================
  // Prices
  // =========================================================================

  async upsertPrice(price: StripePriceRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_prices (
        id, product_id, active, currency, unit_amount, unit_amount_decimal,
        type, billing_scheme, recurring, tiers, tiers_mode, transform_quantity,
        lookup_key, nickname, tax_behavior, metadata, created_at, deleted_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW())
      ON CONFLICT (id) DO UPDATE SET
        product_id = EXCLUDED.product_id, active = EXCLUDED.active, currency = EXCLUDED.currency,
        unit_amount = EXCLUDED.unit_amount, unit_amount_decimal = EXCLUDED.unit_amount_decimal,
        type = EXCLUDED.type, billing_scheme = EXCLUDED.billing_scheme, recurring = EXCLUDED.recurring,
        tiers = EXCLUDED.tiers, tiers_mode = EXCLUDED.tiers_mode,
        transform_quantity = EXCLUDED.transform_quantity, lookup_key = EXCLUDED.lookup_key,
        nickname = EXCLUDED.nickname, tax_behavior = EXCLUDED.tax_behavior,
        metadata = EXCLUDED.metadata, deleted_at = EXCLUDED.deleted_at,
        updated_at = NOW(), synced_at = NOW()`,
      [
        price.id, price.product_id, price.active, price.currency, price.unit_amount,
        price.unit_amount_decimal, price.type, price.billing_scheme,
        price.recurring ? JSON.stringify(price.recurring) : null,
        price.tiers ? JSON.stringify(price.tiers) : null, price.tiers_mode,
        price.transform_quantity ? JSON.stringify(price.transform_quantity) : null,
        price.lookup_key, price.nickname, price.tax_behavior,
        JSON.stringify(price.metadata), price.created_at, price.deleted_at,
      ]
    );
  }

  async upsertPrices(prices: StripePriceRecord[]): Promise<number> {
    for (const price of prices) await this.upsertPrice(price);
    return prices.length;
  }

  async getPrice(id: string): Promise<StripePriceRecord | null> {
    return this.db.queryOne<StripePriceRecord>('SELECT * FROM stripe_prices WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listPrices(limit = 100, offset = 0): Promise<StripePriceRecord[]> {
    const result = await this.db.query<StripePriceRecord>(
      'SELECT * FROM stripe_prices WHERE deleted_at IS NULL AND source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  async countPrices(): Promise<number> {
    return this.db.countScoped('stripe_prices', this.sourceAccountId, 'deleted_at IS NULL');
  }

  // =========================================================================
  // Coupons
  // =========================================================================

  async upsertCoupon(coupon: StripeCouponRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_coupons (
        id, name, amount_off, percent_off, currency, duration, duration_in_months,
        max_redemptions, times_redeemed, redeem_by, valid, applies_to, metadata,
        created_at, deleted_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
      ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name, amount_off = EXCLUDED.amount_off, percent_off = EXCLUDED.percent_off,
        currency = EXCLUDED.currency, duration = EXCLUDED.duration,
        duration_in_months = EXCLUDED.duration_in_months, max_redemptions = EXCLUDED.max_redemptions,
        times_redeemed = EXCLUDED.times_redeemed, redeem_by = EXCLUDED.redeem_by,
        valid = EXCLUDED.valid, applies_to = EXCLUDED.applies_to, metadata = EXCLUDED.metadata,
        deleted_at = EXCLUDED.deleted_at, updated_at = NOW(), synced_at = NOW()`,
      [
        coupon.id, coupon.name, coupon.amount_off, coupon.percent_off, coupon.currency,
        coupon.duration, coupon.duration_in_months, coupon.max_redemptions,
        coupon.times_redeemed, coupon.redeem_by, coupon.valid,
        coupon.applies_to ? JSON.stringify(coupon.applies_to) : null,
        JSON.stringify(coupon.metadata), coupon.created_at, coupon.deleted_at,
      ]
    );
  }

  async upsertCoupons(coupons: StripeCouponRecord[]): Promise<number> {
    for (const coupon of coupons) await this.upsertCoupon(coupon);
    return coupons.length;
  }

  async countCoupons(): Promise<number> {
    return this.db.countScoped('stripe_coupons', this.sourceAccountId, 'deleted_at IS NULL');
  }

  async getCoupon(id: string): Promise<StripeCouponRecord | null> {
    return this.db.queryOne<StripeCouponRecord>('SELECT * FROM stripe_coupons WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listCoupons(limit = 100, offset = 0): Promise<StripeCouponRecord[]> {
    const result = await this.db.query<StripeCouponRecord>(
      'SELECT * FROM stripe_coupons WHERE deleted_at IS NULL AND source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Promotion Codes
  // =========================================================================

  async upsertPromotionCode(code: StripePromotionCodeRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_promotion_codes (
        id, coupon_id, code, customer_id, active, max_redemptions, times_redeemed,
        expires_at, restrictions, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
      ON CONFLICT (id) DO UPDATE SET
        coupon_id = EXCLUDED.coupon_id, code = EXCLUDED.code, customer_id = EXCLUDED.customer_id,
        active = EXCLUDED.active, max_redemptions = EXCLUDED.max_redemptions,
        times_redeemed = EXCLUDED.times_redeemed, expires_at = EXCLUDED.expires_at,
        restrictions = EXCLUDED.restrictions, metadata = EXCLUDED.metadata,
        updated_at = NOW(), synced_at = NOW()`,
      [
        code.id, code.coupon_id, code.code, code.customer_id, code.active,
        code.max_redemptions, code.times_redeemed, code.expires_at,
        JSON.stringify(code.restrictions), JSON.stringify(code.metadata), code.created_at,
      ]
    );
  }

  async upsertPromotionCodes(codes: StripePromotionCodeRecord[]): Promise<number> {
    for (const code of codes) await this.upsertPromotionCode(code);
    return codes.length;
  }

  async countPromotionCodes(): Promise<number> {
    return this.db.countScoped('stripe_promotion_codes', this.sourceAccountId);
  }

  async getPromotionCode(id: string): Promise<StripePromotionCodeRecord | null> {
    return this.db.queryOne<StripePromotionCodeRecord>('SELECT * FROM stripe_promotion_codes WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listPromotionCodes(limit = 100, offset = 0): Promise<StripePromotionCodeRecord[]> {
    const result = await this.db.query<StripePromotionCodeRecord>(
      'SELECT * FROM stripe_promotion_codes WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Subscriptions
  // =========================================================================

  async upsertSubscription(subscription: StripeSubscriptionRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_subscriptions (
        id, customer_id, status, current_period_start, current_period_end,
        cancel_at, canceled_at, cancel_at_period_end, ended_at, trial_start,
        trial_end, collection_method, billing_cycle_anchor, billing_thresholds,
        days_until_due, default_payment_method_id, default_source, discount,
        items, latest_invoice_id, pending_setup_intent, pending_update,
        schedule_id, start_date, transfer_data, application_fee_percent,
        automatic_tax, payment_settings, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, status = EXCLUDED.status,
        current_period_start = EXCLUDED.current_period_start,
        current_period_end = EXCLUDED.current_period_end, cancel_at = EXCLUDED.cancel_at,
        canceled_at = EXCLUDED.canceled_at, cancel_at_period_end = EXCLUDED.cancel_at_period_end,
        ended_at = EXCLUDED.ended_at, trial_start = EXCLUDED.trial_start, trial_end = EXCLUDED.trial_end,
        collection_method = EXCLUDED.collection_method, billing_cycle_anchor = EXCLUDED.billing_cycle_anchor,
        billing_thresholds = EXCLUDED.billing_thresholds, days_until_due = EXCLUDED.days_until_due,
        default_payment_method_id = EXCLUDED.default_payment_method_id, default_source = EXCLUDED.default_source,
        discount = EXCLUDED.discount, items = EXCLUDED.items, latest_invoice_id = EXCLUDED.latest_invoice_id,
        pending_setup_intent = EXCLUDED.pending_setup_intent, pending_update = EXCLUDED.pending_update,
        schedule_id = EXCLUDED.schedule_id, start_date = EXCLUDED.start_date,
        transfer_data = EXCLUDED.transfer_data, application_fee_percent = EXCLUDED.application_fee_percent,
        automatic_tax = EXCLUDED.automatic_tax, payment_settings = EXCLUDED.payment_settings,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        subscription.id, subscription.customer_id, subscription.status,
        subscription.current_period_start, subscription.current_period_end,
        subscription.cancel_at, subscription.canceled_at, subscription.cancel_at_period_end,
        subscription.ended_at, subscription.trial_start, subscription.trial_end,
        subscription.collection_method, subscription.billing_cycle_anchor,
        subscription.billing_thresholds ? JSON.stringify(subscription.billing_thresholds) : null,
        subscription.days_until_due, subscription.default_payment_method_id, subscription.default_source,
        subscription.discount ? JSON.stringify(subscription.discount) : null,
        JSON.stringify(subscription.items), subscription.latest_invoice_id,
        subscription.pending_setup_intent,
        subscription.pending_update ? JSON.stringify(subscription.pending_update) : null,
        subscription.schedule_id, subscription.start_date,
        subscription.transfer_data ? JSON.stringify(subscription.transfer_data) : null,
        subscription.application_fee_percent, JSON.stringify(subscription.automatic_tax),
        JSON.stringify(subscription.payment_settings), JSON.stringify(subscription.metadata),
        subscription.created_at,
      ]
    );
  }

  async upsertSubscriptions(subscriptions: StripeSubscriptionRecord[]): Promise<number> {
    for (const subscription of subscriptions) await this.upsertSubscription(subscription);
    return subscriptions.length;
  }

  async getSubscription(id: string): Promise<StripeSubscriptionRecord | null> {
    return this.db.queryOne<StripeSubscriptionRecord>('SELECT * FROM stripe_subscriptions WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listSubscriptions(limit = 100, offset = 0): Promise<StripeSubscriptionRecord[]> {
    const result = await this.db.query<StripeSubscriptionRecord>(
      'SELECT * FROM stripe_subscriptions WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  async countSubscriptions(status?: string): Promise<number> {
    if (status) return this.db.countScoped('stripe_subscriptions', this.sourceAccountId, 'status = $1', [status]);
    return this.db.countScoped('stripe_subscriptions', this.sourceAccountId);
  }

  // =========================================================================
  // Subscription Items
  // =========================================================================

  async upsertSubscriptionItem(item: StripeSubscriptionItemRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_subscription_items (
        id, subscription_id, price_id, quantity, billing_thresholds, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
      ON CONFLICT (id) DO UPDATE SET
        subscription_id = EXCLUDED.subscription_id, price_id = EXCLUDED.price_id,
        quantity = EXCLUDED.quantity, billing_thresholds = EXCLUDED.billing_thresholds,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        item.id, item.subscription_id, item.price_id, item.quantity,
        item.billing_thresholds ? JSON.stringify(item.billing_thresholds) : null,
        JSON.stringify(item.metadata), item.created_at,
      ]
    );
  }

  async upsertSubscriptionItems(items: StripeSubscriptionItemRecord[]): Promise<number> {
    for (const item of items) await this.upsertSubscriptionItem(item);
    return items.length;
  }

  // =========================================================================
  // Subscription Schedules
  // =========================================================================

  async upsertSubscriptionSchedule(schedule: StripeSubscriptionScheduleRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_subscription_schedules (
        id, customer_id, subscription_id, status, current_phase, default_settings,
        end_behavior, phases, released_at, released_subscription, metadata,
        created_at, canceled_at, completed_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, subscription_id = EXCLUDED.subscription_id,
        status = EXCLUDED.status, current_phase = EXCLUDED.current_phase,
        default_settings = EXCLUDED.default_settings, end_behavior = EXCLUDED.end_behavior,
        phases = EXCLUDED.phases, released_at = EXCLUDED.released_at,
        released_subscription = EXCLUDED.released_subscription, metadata = EXCLUDED.metadata,
        canceled_at = EXCLUDED.canceled_at, completed_at = EXCLUDED.completed_at,
        updated_at = NOW(), synced_at = NOW()`,
      [
        schedule.id, schedule.customer_id, schedule.subscription_id, schedule.status,
        schedule.current_phase ? JSON.stringify(schedule.current_phase) : null,
        JSON.stringify(schedule.default_settings), schedule.end_behavior,
        JSON.stringify(schedule.phases), schedule.released_at, schedule.released_subscription,
        JSON.stringify(schedule.metadata), schedule.created_at, schedule.canceled_at,
        schedule.completed_at,
      ]
    );
  }

  async upsertSubscriptionSchedules(schedules: StripeSubscriptionScheduleRecord[]): Promise<number> {
    for (const schedule of schedules) await this.upsertSubscriptionSchedule(schedule);
    return schedules.length;
  }

  // =========================================================================
  // Invoices
  // =========================================================================

  async upsertInvoice(invoice: StripeInvoiceRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_invoices (
        id, customer_id, subscription_id, status, collection_method, currency,
        amount_due, amount_paid, amount_remaining, subtotal, subtotal_excluding_tax,
        total, total_excluding_tax, tax, total_tax_amounts, discount, discounts,
        account_country, account_name, billing_reason, number, receipt_number,
        statement_descriptor, description, footer, customer_email, customer_name,
        customer_address, customer_phone, customer_shipping, customer_tax_exempt,
        customer_tax_ids, default_payment_method_id, default_source, lines,
        hosted_invoice_url, invoice_pdf, payment_intent_id, charge_id,
        attempt_count, attempted, auto_advance, next_payment_attempt,
        webhooks_delivered_at, paid, paid_out_of_band, period_start, period_end,
        due_date, effective_at, finalized_at, marked_uncollectible_at, voided_at,
        metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42, $43, $44, $45, $46, $47, $48, $49, $50, $51, $52, $53, $54, $55, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, subscription_id = EXCLUDED.subscription_id,
        status = EXCLUDED.status, collection_method = EXCLUDED.collection_method,
        amount_due = EXCLUDED.amount_due, amount_paid = EXCLUDED.amount_paid,
        amount_remaining = EXCLUDED.amount_remaining, subtotal = EXCLUDED.subtotal,
        subtotal_excluding_tax = EXCLUDED.subtotal_excluding_tax, total = EXCLUDED.total,
        total_excluding_tax = EXCLUDED.total_excluding_tax, tax = EXCLUDED.tax,
        total_tax_amounts = EXCLUDED.total_tax_amounts, discount = EXCLUDED.discount,
        discounts = EXCLUDED.discounts, lines = EXCLUDED.lines,
        hosted_invoice_url = EXCLUDED.hosted_invoice_url, invoice_pdf = EXCLUDED.invoice_pdf,
        payment_intent_id = EXCLUDED.payment_intent_id, charge_id = EXCLUDED.charge_id,
        attempt_count = EXCLUDED.attempt_count, attempted = EXCLUDED.attempted,
        auto_advance = EXCLUDED.auto_advance, next_payment_attempt = EXCLUDED.next_payment_attempt,
        webhooks_delivered_at = EXCLUDED.webhooks_delivered_at, paid = EXCLUDED.paid,
        paid_out_of_band = EXCLUDED.paid_out_of_band, finalized_at = EXCLUDED.finalized_at,
        marked_uncollectible_at = EXCLUDED.marked_uncollectible_at, voided_at = EXCLUDED.voided_at,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        invoice.id, invoice.customer_id, invoice.subscription_id, invoice.status,
        invoice.collection_method, invoice.currency, invoice.amount_due, invoice.amount_paid,
        invoice.amount_remaining, invoice.subtotal, invoice.subtotal_excluding_tax,
        invoice.total, invoice.total_excluding_tax, invoice.tax,
        JSON.stringify(invoice.total_tax_amounts),
        invoice.discount ? JSON.stringify(invoice.discount) : null,
        JSON.stringify(invoice.discounts), invoice.account_country, invoice.account_name,
        invoice.billing_reason, invoice.number, invoice.receipt_number,
        invoice.statement_descriptor, invoice.description, invoice.footer,
        invoice.customer_email, invoice.customer_name,
        invoice.customer_address ? JSON.stringify(invoice.customer_address) : null,
        invoice.customer_phone,
        invoice.customer_shipping ? JSON.stringify(invoice.customer_shipping) : null,
        invoice.customer_tax_exempt, JSON.stringify(invoice.customer_tax_ids),
        invoice.default_payment_method_id, invoice.default_source, JSON.stringify(invoice.lines),
        invoice.hosted_invoice_url, invoice.invoice_pdf, invoice.payment_intent_id,
        invoice.charge_id, invoice.attempt_count, invoice.attempted, invoice.auto_advance,
        invoice.next_payment_attempt, invoice.webhooks_delivered_at, invoice.paid,
        invoice.paid_out_of_band, invoice.period_start, invoice.period_end, invoice.due_date,
        invoice.effective_at, invoice.finalized_at, invoice.marked_uncollectible_at,
        invoice.voided_at, JSON.stringify(invoice.metadata), invoice.created_at,
      ]
    );
  }

  async upsertInvoices(invoices: StripeInvoiceRecord[]): Promise<number> {
    for (const invoice of invoices) await this.upsertInvoice(invoice);
    return invoices.length;
  }

  async getInvoice(id: string): Promise<StripeInvoiceRecord | null> {
    return this.db.queryOne<StripeInvoiceRecord>('SELECT * FROM stripe_invoices WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listInvoices(limit = 100, offset = 0): Promise<StripeInvoiceRecord[]> {
    const result = await this.db.query<StripeInvoiceRecord>(
      'SELECT * FROM stripe_invoices WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  async countInvoices(status?: string): Promise<number> {
    if (status) return this.db.countScoped('stripe_invoices', this.sourceAccountId, 'status = $1', [status]);
    return this.db.countScoped('stripe_invoices', this.sourceAccountId);
  }

  // =========================================================================
  // Invoice Items
  // =========================================================================

  async upsertInvoiceItem(item: StripeInvoiceItemRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_invoice_items (
        id, customer_id, invoice_id, subscription_id, subscription_item_id, price_id,
        amount, currency, description, discountable, quantity, unit_amount,
        unit_amount_decimal, period_start, period_end, proration, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, invoice_id = EXCLUDED.invoice_id,
        subscription_id = EXCLUDED.subscription_id, subscription_item_id = EXCLUDED.subscription_item_id,
        price_id = EXCLUDED.price_id, amount = EXCLUDED.amount, currency = EXCLUDED.currency,
        description = EXCLUDED.description, discountable = EXCLUDED.discountable,
        quantity = EXCLUDED.quantity, unit_amount = EXCLUDED.unit_amount,
        unit_amount_decimal = EXCLUDED.unit_amount_decimal, period_start = EXCLUDED.period_start,
        period_end = EXCLUDED.period_end, proration = EXCLUDED.proration,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        item.id, item.customer_id, item.invoice_id, item.subscription_id,
        item.subscription_item_id, item.price_id, item.amount, item.currency,
        item.description, item.discountable, item.quantity, item.unit_amount,
        item.unit_amount_decimal, item.period_start, item.period_end, item.proration,
        JSON.stringify(item.metadata), item.created_at,
      ]
    );
  }

  async upsertInvoiceItems(items: StripeInvoiceItemRecord[]): Promise<number> {
    for (const item of items) await this.upsertInvoiceItem(item);
    return items.length;
  }

  // =========================================================================
  // Charges
  // =========================================================================

  async upsertCharge(charge: StripeChargeRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_charges (
        id, customer_id, payment_intent_id, invoice_id, amount, amount_captured,
        amount_refunded, currency, status, paid, captured, refunded, disputed,
        failure_code, failure_message, outcome, description, receipt_email,
        receipt_number, receipt_url, statement_descriptor, statement_descriptor_suffix,
        payment_method_id, payment_method_details, billing_details, shipping,
        fraud_details, balance_transaction_id, application_fee_id, application_fee_amount,
        transfer_id, transfer_group, on_behalf_of, source_transfer, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, payment_intent_id = EXCLUDED.payment_intent_id,
        invoice_id = EXCLUDED.invoice_id, amount = EXCLUDED.amount,
        amount_captured = EXCLUDED.amount_captured, amount_refunded = EXCLUDED.amount_refunded,
        status = EXCLUDED.status, paid = EXCLUDED.paid, captured = EXCLUDED.captured,
        refunded = EXCLUDED.refunded, disputed = EXCLUDED.disputed,
        failure_code = EXCLUDED.failure_code, failure_message = EXCLUDED.failure_message,
        outcome = EXCLUDED.outcome, receipt_url = EXCLUDED.receipt_url,
        payment_method_details = EXCLUDED.payment_method_details, billing_details = EXCLUDED.billing_details,
        fraud_details = EXCLUDED.fraud_details, balance_transaction_id = EXCLUDED.balance_transaction_id,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        charge.id, charge.customer_id, charge.payment_intent_id, charge.invoice_id,
        charge.amount, charge.amount_captured, charge.amount_refunded, charge.currency,
        charge.status, charge.paid, charge.captured, charge.refunded, charge.disputed,
        charge.failure_code, charge.failure_message,
        charge.outcome ? JSON.stringify(charge.outcome) : null,
        charge.description, charge.receipt_email, charge.receipt_number, charge.receipt_url,
        charge.statement_descriptor, charge.statement_descriptor_suffix, charge.payment_method_id,
        charge.payment_method_details ? JSON.stringify(charge.payment_method_details) : null,
        charge.billing_details ? JSON.stringify(charge.billing_details) : null,
        charge.shipping ? JSON.stringify(charge.shipping) : null,
        charge.fraud_details ? JSON.stringify(charge.fraud_details) : null,
        charge.balance_transaction_id, charge.application_fee_id, charge.application_fee_amount,
        charge.transfer_id, charge.transfer_group, charge.on_behalf_of, charge.source_transfer,
        JSON.stringify(charge.metadata), charge.created_at,
      ]
    );
  }

  async upsertCharges(charges: StripeChargeRecord[]): Promise<number> {
    for (const charge of charges) await this.upsertCharge(charge);
    return charges.length;
  }

  async countCharges(): Promise<number> {
    return this.db.countScoped('stripe_charges', this.sourceAccountId);
  }

  async getCharge(id: string): Promise<StripeChargeRecord | null> {
    return this.db.queryOne<StripeChargeRecord>('SELECT * FROM stripe_charges WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listCharges(limit = 100, offset = 0): Promise<StripeChargeRecord[]> {
    const result = await this.db.query<StripeChargeRecord>(
      'SELECT * FROM stripe_charges WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Refunds
  // =========================================================================

  async upsertRefund(refund: StripeRefundRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_refunds (
        id, charge_id, payment_intent_id, amount, currency, status, reason,
        receipt_number, description, failure_balance_transaction, failure_reason,
        balance_transaction_id, source_transfer_reversal, transfer_reversal,
        metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
      ON CONFLICT (id) DO UPDATE SET
        charge_id = EXCLUDED.charge_id, payment_intent_id = EXCLUDED.payment_intent_id,
        amount = EXCLUDED.amount, status = EXCLUDED.status, reason = EXCLUDED.reason,
        failure_balance_transaction = EXCLUDED.failure_balance_transaction,
        failure_reason = EXCLUDED.failure_reason, balance_transaction_id = EXCLUDED.balance_transaction_id,
        metadata = EXCLUDED.metadata, synced_at = NOW()`,
      [
        refund.id, refund.charge_id, refund.payment_intent_id, refund.amount, refund.currency,
        refund.status, refund.reason, refund.receipt_number, refund.description,
        refund.failure_balance_transaction, refund.failure_reason, refund.balance_transaction_id,
        refund.source_transfer_reversal, refund.transfer_reversal,
        JSON.stringify(refund.metadata), refund.created_at,
      ]
    );
  }

  async upsertRefunds(refunds: StripeRefundRecord[]): Promise<number> {
    for (const refund of refunds) await this.upsertRefund(refund);
    return refunds.length;
  }

  async countRefunds(): Promise<number> {
    return this.db.countScoped('stripe_refunds', this.sourceAccountId);
  }

  async getRefund(id: string): Promise<StripeRefundRecord | null> {
    return this.db.queryOne<StripeRefundRecord>('SELECT * FROM stripe_refunds WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listRefunds(limit = 100, offset = 0): Promise<StripeRefundRecord[]> {
    const result = await this.db.query<StripeRefundRecord>(
      'SELECT * FROM stripe_refunds WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Disputes
  // =========================================================================

  async upsertDispute(dispute: StripeDisputeRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_disputes (
        id, charge_id, payment_intent_id, amount, currency, status, reason,
        is_charge_refundable, balance_transactions, evidence, evidence_details,
        metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (id) DO UPDATE SET
        charge_id = EXCLUDED.charge_id, payment_intent_id = EXCLUDED.payment_intent_id,
        amount = EXCLUDED.amount, status = EXCLUDED.status, reason = EXCLUDED.reason,
        is_charge_refundable = EXCLUDED.is_charge_refundable,
        balance_transactions = EXCLUDED.balance_transactions, evidence = EXCLUDED.evidence,
        evidence_details = EXCLUDED.evidence_details, metadata = EXCLUDED.metadata,
        updated_at = NOW(), synced_at = NOW()`,
      [
        dispute.id, dispute.charge_id, dispute.payment_intent_id, dispute.amount,
        dispute.currency, dispute.status, dispute.reason, dispute.is_charge_refundable,
        JSON.stringify(dispute.balance_transactions), JSON.stringify(dispute.evidence),
        JSON.stringify(dispute.evidence_details), JSON.stringify(dispute.metadata),
        dispute.created_at,
      ]
    );
  }

  async upsertDisputes(disputes: StripeDisputeRecord[]): Promise<number> {
    for (const dispute of disputes) await this.upsertDispute(dispute);
    return disputes.length;
  }

  async countDisputes(): Promise<number> {
    return this.db.countScoped('stripe_disputes', this.sourceAccountId);
  }

  async getDispute(id: string): Promise<StripeDisputeRecord | null> {
    return this.db.queryOne<StripeDisputeRecord>('SELECT * FROM stripe_disputes WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listDisputes(limit = 100, offset = 0): Promise<StripeDisputeRecord[]> {
    const result = await this.db.query<StripeDisputeRecord>(
      'SELECT * FROM stripe_disputes WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Payment Intents
  // =========================================================================

  async upsertPaymentIntent(pi: StripePaymentIntentRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_payment_intents (
        id, customer_id, invoice_id, amount, amount_capturable, amount_received,
        currency, status, capture_method, confirmation_method, payment_method_id,
        payment_method_types, setup_future_usage, client_secret, description,
        receipt_email, statement_descriptor, statement_descriptor_suffix, shipping,
        application_fee_amount, transfer_data, transfer_group, on_behalf_of,
        cancellation_reason, canceled_at, charges, last_payment_error, next_action,
        processing, review, automatic_payment_methods, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, invoice_id = EXCLUDED.invoice_id,
        amount = EXCLUDED.amount, amount_capturable = EXCLUDED.amount_capturable,
        amount_received = EXCLUDED.amount_received, status = EXCLUDED.status,
        capture_method = EXCLUDED.capture_method, confirmation_method = EXCLUDED.confirmation_method,
        payment_method_id = EXCLUDED.payment_method_id, payment_method_types = EXCLUDED.payment_method_types,
        setup_future_usage = EXCLUDED.setup_future_usage, client_secret = EXCLUDED.client_secret,
        description = EXCLUDED.description, receipt_email = EXCLUDED.receipt_email,
        statement_descriptor = EXCLUDED.statement_descriptor,
        statement_descriptor_suffix = EXCLUDED.statement_descriptor_suffix,
        shipping = EXCLUDED.shipping, application_fee_amount = EXCLUDED.application_fee_amount,
        transfer_data = EXCLUDED.transfer_data, transfer_group = EXCLUDED.transfer_group,
        on_behalf_of = EXCLUDED.on_behalf_of, cancellation_reason = EXCLUDED.cancellation_reason,
        canceled_at = EXCLUDED.canceled_at, charges = EXCLUDED.charges,
        last_payment_error = EXCLUDED.last_payment_error, next_action = EXCLUDED.next_action,
        processing = EXCLUDED.processing, review = EXCLUDED.review,
        automatic_payment_methods = EXCLUDED.automatic_payment_methods,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        pi.id, pi.customer_id, pi.invoice_id, pi.amount, pi.amount_capturable, pi.amount_received,
        pi.currency, pi.status, pi.capture_method, pi.confirmation_method, pi.payment_method_id,
        JSON.stringify(pi.payment_method_types), pi.setup_future_usage, pi.client_secret,
        pi.description, pi.receipt_email, pi.statement_descriptor, pi.statement_descriptor_suffix,
        pi.shipping ? JSON.stringify(pi.shipping) : null, pi.application_fee_amount,
        pi.transfer_data ? JSON.stringify(pi.transfer_data) : null, pi.transfer_group,
        pi.on_behalf_of, pi.cancellation_reason, pi.canceled_at, JSON.stringify(pi.charges),
        pi.last_payment_error ? JSON.stringify(pi.last_payment_error) : null,
        pi.next_action ? JSON.stringify(pi.next_action) : null,
        pi.processing ? JSON.stringify(pi.processing) : null, pi.review,
        pi.automatic_payment_methods ? JSON.stringify(pi.automatic_payment_methods) : null,
        JSON.stringify(pi.metadata), pi.created_at,
      ]
    );
  }

  async upsertPaymentIntents(paymentIntents: StripePaymentIntentRecord[]): Promise<number> {
    for (const pi of paymentIntents) await this.upsertPaymentIntent(pi);
    return paymentIntents.length;
  }

  async countPaymentIntents(): Promise<number> {
    return this.db.countScoped('stripe_payment_intents', this.sourceAccountId);
  }

  async getPaymentIntent(id: string): Promise<StripePaymentIntentRecord | null> {
    return this.db.queryOne<StripePaymentIntentRecord>('SELECT * FROM stripe_payment_intents WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listPaymentIntents(limit = 100, offset = 0): Promise<StripePaymentIntentRecord[]> {
    const result = await this.db.query<StripePaymentIntentRecord>(
      'SELECT * FROM stripe_payment_intents WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Setup Intents
  // =========================================================================

  async upsertSetupIntent(si: StripeSetupIntentRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_setup_intents (
        id, customer_id, payment_method_id, status, usage, payment_method_types,
        client_secret, description, cancellation_reason, last_setup_error,
        next_action, single_use_mandate, mandate, on_behalf_of, application,
        metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, payment_method_id = EXCLUDED.payment_method_id,
        status = EXCLUDED.status, usage = EXCLUDED.usage,
        payment_method_types = EXCLUDED.payment_method_types, client_secret = EXCLUDED.client_secret,
        description = EXCLUDED.description, cancellation_reason = EXCLUDED.cancellation_reason,
        last_setup_error = EXCLUDED.last_setup_error, next_action = EXCLUDED.next_action,
        single_use_mandate = EXCLUDED.single_use_mandate, mandate = EXCLUDED.mandate,
        on_behalf_of = EXCLUDED.on_behalf_of, application = EXCLUDED.application,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        si.id, si.customer_id, si.payment_method_id, si.status, si.usage,
        JSON.stringify(si.payment_method_types), si.client_secret, si.description,
        si.cancellation_reason, si.last_setup_error ? JSON.stringify(si.last_setup_error) : null,
        si.next_action ? JSON.stringify(si.next_action) : null, si.single_use_mandate,
        si.mandate, si.on_behalf_of, si.application, JSON.stringify(si.metadata), si.created_at,
      ]
    );
  }

  async upsertSetupIntents(setupIntents: StripeSetupIntentRecord[]): Promise<number> {
    for (const si of setupIntents) await this.upsertSetupIntent(si);
    return setupIntents.length;
  }

  async countSetupIntents(): Promise<number> {
    return this.db.countScoped('stripe_setup_intents', this.sourceAccountId);
  }

  // =========================================================================
  // Payment Methods
  // =========================================================================

  async upsertPaymentMethod(pm: StripePaymentMethodRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_payment_methods (
        id, customer_id, type, billing_details, card, bank_account,
        sepa_debit, us_bank_account, link, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, type = EXCLUDED.type,
        billing_details = EXCLUDED.billing_details, card = EXCLUDED.card,
        bank_account = EXCLUDED.bank_account, sepa_debit = EXCLUDED.sepa_debit,
        us_bank_account = EXCLUDED.us_bank_account, link = EXCLUDED.link,
        metadata = EXCLUDED.metadata, updated_at = NOW(), synced_at = NOW()`,
      [
        pm.id, pm.customer_id, pm.type, JSON.stringify(pm.billing_details),
        pm.card ? JSON.stringify(pm.card) : null,
        pm.bank_account ? JSON.stringify(pm.bank_account) : null,
        pm.sepa_debit ? JSON.stringify(pm.sepa_debit) : null,
        pm.us_bank_account ? JSON.stringify(pm.us_bank_account) : null,
        pm.link ? JSON.stringify(pm.link) : null, JSON.stringify(pm.metadata), pm.created_at,
      ]
    );
  }

  async upsertPaymentMethods(paymentMethods: StripePaymentMethodRecord[]): Promise<number> {
    for (const pm of paymentMethods) await this.upsertPaymentMethod(pm);
    return paymentMethods.length;
  }

  async countPaymentMethods(): Promise<number> {
    return this.db.countScoped('stripe_payment_methods', this.sourceAccountId);
  }

  async getPaymentMethod(id: string): Promise<StripePaymentMethodRecord | null> {
    return this.db.queryOne<StripePaymentMethodRecord>('SELECT * FROM stripe_payment_methods WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listPaymentMethods(limit = 100, offset = 0): Promise<StripePaymentMethodRecord[]> {
    const result = await this.db.query<StripePaymentMethodRecord>(
      'SELECT * FROM stripe_payment_methods WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Balance Transactions
  // =========================================================================

  async upsertBalanceTransaction(bt: StripeBalanceTransactionRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_balance_transactions (
        id, amount, currency, net, fee, fee_details, type, status,
        description, source, reporting_category, available_on, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (id) DO UPDATE SET
        amount = EXCLUDED.amount, net = EXCLUDED.net, fee = EXCLUDED.fee,
        fee_details = EXCLUDED.fee_details, status = EXCLUDED.status,
        description = EXCLUDED.description, reporting_category = EXCLUDED.reporting_category,
        available_on = EXCLUDED.available_on, synced_at = NOW()`,
      [
        bt.id, bt.amount, bt.currency, bt.net, bt.fee, JSON.stringify(bt.fee_details),
        bt.type, bt.status, bt.description, bt.source, bt.reporting_category,
        bt.available_on, bt.created_at,
      ]
    );
  }

  async upsertBalanceTransactions(transactions: StripeBalanceTransactionRecord[]): Promise<number> {
    for (const bt of transactions) await this.upsertBalanceTransaction(bt);
    return transactions.length;
  }

  async countBalanceTransactions(): Promise<number> {
    return this.db.countScoped('stripe_balance_transactions', this.sourceAccountId);
  }

  async getBalanceTransaction(id: string): Promise<StripeBalanceTransactionRecord | null> {
    return this.db.queryOne<StripeBalanceTransactionRecord>('SELECT * FROM stripe_balance_transactions WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listBalanceTransactions(limit = 100, offset = 0): Promise<StripeBalanceTransactionRecord[]> {
    const result = await this.db.query<StripeBalanceTransactionRecord>(
      'SELECT * FROM stripe_balance_transactions WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Checkout Sessions
  // =========================================================================

  async upsertCheckoutSession(session: StripeCheckoutSessionRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_checkout_sessions (
        id, customer_id, customer_email, payment_intent_id, subscription_id,
        invoice_id, mode, status, payment_status, currency, amount_total,
        amount_subtotal, total_details, success_url, cancel_url, url,
        client_reference_id, customer_creation, billing_address_collection,
        shipping_address_collection, shipping_cost, shipping_details,
        custom_text, consent, consent_collection, expires_at, livemode,
        locale, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, customer_email = EXCLUDED.customer_email,
        payment_intent_id = EXCLUDED.payment_intent_id, subscription_id = EXCLUDED.subscription_id,
        invoice_id = EXCLUDED.invoice_id, status = EXCLUDED.status,
        payment_status = EXCLUDED.payment_status, amount_total = EXCLUDED.amount_total,
        amount_subtotal = EXCLUDED.amount_subtotal, total_details = EXCLUDED.total_details,
        url = EXCLUDED.url, expires_at = EXCLUDED.expires_at, metadata = EXCLUDED.metadata,
        synced_at = NOW()`,
      [
        session.id, session.customer_id, session.customer_email, session.payment_intent_id,
        session.subscription_id, session.invoice_id, session.mode, session.status,
        session.payment_status, session.currency, session.amount_total, session.amount_subtotal,
        session.total_details ? JSON.stringify(session.total_details) : null,
        session.success_url, session.cancel_url, session.url, session.client_reference_id,
        session.customer_creation, session.billing_address_collection,
        session.shipping_address_collection ? JSON.stringify(session.shipping_address_collection) : null,
        session.shipping_cost ? JSON.stringify(session.shipping_cost) : null,
        session.shipping_details ? JSON.stringify(session.shipping_details) : null,
        session.custom_text ? JSON.stringify(session.custom_text) : null,
        session.consent ? JSON.stringify(session.consent) : null,
        session.consent_collection ? JSON.stringify(session.consent_collection) : null,
        session.expires_at, session.livemode, session.locale,
        JSON.stringify(session.metadata), session.created_at,
      ]
    );
  }

  async upsertCheckoutSessions(sessions: StripeCheckoutSessionRecord[]): Promise<number> {
    for (const session of sessions) await this.upsertCheckoutSession(session);
    return sessions.length;
  }

  async countCheckoutSessions(): Promise<number> {
    return this.db.countScoped('stripe_checkout_sessions', this.sourceAccountId);
  }

  // =========================================================================
  // Credit Notes
  // =========================================================================

  async upsertCreditNote(cn: StripeCreditNoteRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_credit_notes (
        id, invoice_id, customer_id, type, status, currency, amount, subtotal,
        subtotal_excluding_tax, total, total_excluding_tax, discount_amount,
        out_of_band_amount, reason, memo, number, pdf, voided_at, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW())
      ON CONFLICT (id) DO UPDATE SET
        status = EXCLUDED.status, voided_at = EXCLUDED.voided_at,
        metadata = EXCLUDED.metadata, synced_at = NOW()`,
      [
        cn.id, cn.invoice_id, cn.customer_id, cn.type, cn.status, cn.currency,
        cn.amount, cn.subtotal, cn.subtotal_excluding_tax, cn.total,
        cn.total_excluding_tax, cn.discount_amount, cn.out_of_band_amount,
        cn.reason, cn.memo, cn.number, cn.pdf, cn.voided_at,
        JSON.stringify(cn.metadata), cn.created_at,
      ]
    );
  }

  async upsertCreditNotes(creditNotes: StripeCreditNoteRecord[]): Promise<number> {
    for (const cn of creditNotes) await this.upsertCreditNote(cn);
    return creditNotes.length;
  }

  async countCreditNotes(): Promise<number> {
    return this.db.countScoped('stripe_credit_notes', this.sourceAccountId);
  }

  // =========================================================================
  // Tax Rates
  // =========================================================================

  async upsertTaxRate(tr: StripeTaxRateRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_tax_rates (
        id, display_name, description, percentage, inclusive, active,
        country, state, jurisdiction, tax_type, metadata, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
      ON CONFLICT (id) DO UPDATE SET
        display_name = EXCLUDED.display_name, description = EXCLUDED.description,
        percentage = EXCLUDED.percentage, inclusive = EXCLUDED.inclusive,
        active = EXCLUDED.active, country = EXCLUDED.country, state = EXCLUDED.state,
        jurisdiction = EXCLUDED.jurisdiction, tax_type = EXCLUDED.tax_type,
        metadata = EXCLUDED.metadata, synced_at = NOW()`,
      [
        tr.id, tr.display_name, tr.description, tr.percentage, tr.inclusive,
        tr.active, tr.country, tr.state, tr.jurisdiction, tr.tax_type,
        JSON.stringify(tr.metadata), tr.created_at,
      ]
    );
  }

  async upsertTaxRates(taxRates: StripeTaxRateRecord[]): Promise<number> {
    for (const tr of taxRates) await this.upsertTaxRate(tr);
    return taxRates.length;
  }

  async countTaxRates(): Promise<number> {
    return this.db.countScoped('stripe_tax_rates', this.sourceAccountId);
  }

  async getTaxRate(id: string): Promise<StripeTaxRateRecord | null> {
    return this.db.queryOne<StripeTaxRateRecord>('SELECT * FROM stripe_tax_rates WHERE id = $1 AND source_account_id = $2', [id, this.sourceAccountId]);
  }

  async listTaxRates(limit = 100, offset = 0): Promise<StripeTaxRateRecord[]> {
    const result = await this.db.query<StripeTaxRateRecord>(
      'SELECT * FROM stripe_tax_rates WHERE active = TRUE AND source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Tax IDs
  // =========================================================================

  async upsertTaxId(taxId: StripeTaxIdRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_tax_ids (
        id, customer_id, type, value, country, verification, created_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
      ON CONFLICT (id) DO UPDATE SET
        customer_id = EXCLUDED.customer_id, type = EXCLUDED.type,
        value = EXCLUDED.value, country = EXCLUDED.country,
        verification = EXCLUDED.verification, synced_at = NOW()`,
      [
        taxId.id, taxId.customer_id, taxId.type, taxId.value, taxId.country,
        taxId.verification ? JSON.stringify(taxId.verification) : null, taxId.created_at,
      ]
    );
  }

  async upsertTaxIds(taxIds: StripeTaxIdRecord[]): Promise<number> {
    for (const taxId of taxIds) await this.upsertTaxId(taxId);
    return taxIds.length;
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: StripeWebhookEventRecord): Promise<void> {
    await this.execute(
      `INSERT INTO stripe_webhook_events (
        id, type, api_version, data, object_type, object_id, request_id,
        request_idempotency_key, livemode, pending_webhooks, processed,
        processed_at, error, retry_count, created_at, received_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
      ON CONFLICT (id) DO UPDATE SET
        processed = EXCLUDED.processed, processed_at = EXCLUDED.processed_at,
        error = EXCLUDED.error, retry_count = EXCLUDED.retry_count`,
      [
        event.id, event.type, event.api_version, JSON.stringify(event.data),
        event.object_type, event.object_id, event.request_id, event.request_idempotency_key,
        event.livemode, event.pending_webhooks, event.processed, event.processed_at,
        event.error, event.retry_count, event.created_at,
      ]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      'UPDATE stripe_webhook_events SET processed = TRUE, processed_at = NOW(), error = $2 WHERE id = $1 AND source_account_id = $3',
      [id, error ?? null, this.sourceAccountId]
    );
  }

  async getUnprocessedEvents(limit = 100): Promise<StripeWebhookEventRecord[]> {
    const result = await this.db.query<StripeWebhookEventRecord>(
      `SELECT * FROM stripe_webhook_events
       WHERE processed = FALSE AND retry_count < 3 AND source_account_id = $2
       ORDER BY received_at ASC LIMIT $1`,
      [limit, this.sourceAccountId]
    );
    return result.rows;
  }

  async incrementRetryCount(id: string): Promise<void> {
    await this.execute(
      'UPDATE stripe_webhook_events SET retry_count = retry_count + 1 WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  async listWebhookEvents(type?: string, limit = 100, offset = 0): Promise<StripeWebhookEventRecord[]> {
    if (type) {
      const result = await this.db.query<StripeWebhookEventRecord>(
        'SELECT * FROM stripe_webhook_events WHERE type = $1 AND source_account_id = $4 ORDER BY created_at DESC LIMIT $2 OFFSET $3',
        [type, limit, offset, this.sourceAccountId]
      );
      return result.rows;
    }
    const result = await this.db.query<StripeWebhookEventRecord>(
      'SELECT * FROM stripe_webhook_events WHERE source_account_id = $3 ORDER BY created_at DESC LIMIT $1 OFFSET $2',
      [limit, offset, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(): Promise<SyncStats> {
    const [
      customers, products, prices, coupons, promotionCodes, subscriptions,
      subscriptionItems, subscriptionSchedules, invoices, invoiceItems,
      creditNotes, charges, refunds, disputes, paymentIntents, setupIntents,
      paymentMethods, balanceTransactions, checkoutSessions, taxIds, taxRates,
      lastSyncResult
    ] = await Promise.all([
      this.countCustomers(),
      this.countProducts(),
      this.countPrices(),
      this.countCoupons(),
      this.countPromotionCodes(),
      this.countSubscriptions(),
      this.db.countScoped('stripe_subscription_items', this.sourceAccountId),
      this.db.countScoped('stripe_subscription_schedules', this.sourceAccountId),
      this.countInvoices(),
      this.db.countScoped('stripe_invoice_items', this.sourceAccountId),
      this.countCreditNotes(),
      this.countCharges(),
      this.countRefunds(),
      this.countDisputes(),
      this.countPaymentIntents(),
      this.countSetupIntents(),
      this.countPaymentMethods(),
      this.countBalanceTransactions(),
      this.countCheckoutSessions(),
      this.db.countScoped('stripe_tax_ids', this.sourceAccountId),
      this.countTaxRates(),
      this.db.queryOne<{ max_synced: Date | null }>(
        'SELECT MAX(synced_at) as max_synced FROM stripe_customers WHERE source_account_id = $1',
        [this.sourceAccountId]
      ),
    ]);

    return {
      customers, products, prices, coupons, promotionCodes, subscriptions,
      subscriptionItems, subscriptionSchedules, invoices, invoiceItems,
      creditNotes, charges, refunds, disputes, paymentIntents, setupIntents,
      paymentMethods, balanceTransactions, checkoutSessions, taxIds, taxRates,
      lastSyncedAt: lastSyncResult?.max_synced ?? null,
    };
  }

  // =========================================================================
  // Multi-App Cleanup
  // =========================================================================

  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    // Child tables first, parent tables last to respect FK constraints
    return this.db.cleanupForAccount([
      'stripe_webhook_events',
      'stripe_balance_transactions',
      'stripe_refunds',
      'stripe_disputes',
      'stripe_charges',
      'stripe_credit_notes',
      'stripe_invoice_items',
      'stripe_invoices',
      'stripe_checkout_sessions',
      'stripe_subscription_items',
      'stripe_subscription_schedules',
      'stripe_subscriptions',
      'stripe_promotion_codes',
      'stripe_coupons',
      'stripe_tax_ids',
      'stripe_tax_rates',
      'stripe_prices',
      'stripe_products',
      'stripe_payment_methods',
      'stripe_setup_intents',
      'stripe_payment_intents',
      'stripe_customers',
    ], sourceAccountId);
  }
}
