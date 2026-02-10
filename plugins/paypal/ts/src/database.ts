/**
 * PayPal Database Operations
 * Schema initialization, CRUD operations, statistics
 */

import { createLogger, createDatabase, type Database } from '@nself/plugin-utils';
import type {
  TransactionRecord,
  OrderRecord,
  CaptureRecord,
  AuthorizationRecord,
  RefundRecord,
  SubscriptionRecord,
  SubscriptionPlanRecord,
  ProductRecord,
  DisputeRecord,
  PayoutRecord,
  InvoiceRecord,
  PayerRecord,
  BalanceRecord,
  SyncStats,
} from './types.js';

const logger = createLogger('paypal:database');

export class PayPalDatabase {
  private db: Database;
  private sourceAccountId: string;

  constructor(db: Database, sourceAccountId = 'primary') {
    this.db = db;
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(accountId: string): PayPalDatabase {
    return new PayPalDatabase(this.db, accountId);
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  // ─── Schema Initialization ─────────────────────────────────────────────

  async initializeSchema(): Promise<void> {
    logger.info('Initializing PayPal database schema...');

    // 14 tables
    await this.db.query(`
      CREATE TABLE IF NOT EXISTS paypal_transactions (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_code VARCHAR(50),
        initiation_date TIMESTAMP WITH TIME ZONE,
        updated_date TIMESTAMP WITH TIME ZONE,
        amount NUMERIC(20, 2) DEFAULT 0,
        fee_amount NUMERIC(20, 2),
        currency VARCHAR(10) DEFAULT 'USD',
        status VARCHAR(50),
        subject TEXT,
        note TEXT,
        payer_email VARCHAR(255),
        payer_id VARCHAR(255),
        payer_name VARCHAR(255),
        invoice_id VARCHAR(255),
        custom_field TEXT,
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_transactions_payer ON paypal_transactions(payer_email);
      CREATE INDEX IF NOT EXISTS idx_paypal_transactions_date ON paypal_transactions(initiation_date DESC);
      CREATE INDEX IF NOT EXISTS idx_paypal_transactions_status ON paypal_transactions(status);
      CREATE INDEX IF NOT EXISTS idx_paypal_transactions_account ON paypal_transactions(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_orders (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        status VARCHAR(50),
        intent VARCHAR(50),
        payer_email VARCHAR(255),
        payer_id VARCHAR(255),
        payer_name VARCHAR(255),
        total_amount NUMERIC(20, 2) DEFAULT 0,
        currency VARCHAR(10) DEFAULT 'USD',
        description TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_orders_status ON paypal_orders(status);
      CREATE INDEX IF NOT EXISTS idx_paypal_orders_date ON paypal_orders(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_paypal_orders_account ON paypal_orders(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_captures (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        order_id VARCHAR(255),
        status VARCHAR(50),
        amount NUMERIC(20, 2) DEFAULT 0,
        currency VARCHAR(10) DEFAULT 'USD',
        fee_amount NUMERIC(20, 2),
        net_amount NUMERIC(20, 2),
        final_capture BOOLEAN DEFAULT false,
        invoice_id VARCHAR(255),
        custom_id VARCHAR(255),
        seller_protection VARCHAR(50),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_captures_order ON paypal_captures(order_id);
      CREATE INDEX IF NOT EXISTS idx_paypal_captures_date ON paypal_captures(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_paypal_captures_account ON paypal_captures(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_authorizations (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        order_id VARCHAR(255),
        status VARCHAR(50),
        amount NUMERIC(20, 2) DEFAULT 0,
        currency VARCHAR(10) DEFAULT 'USD',
        invoice_id VARCHAR(255),
        custom_id VARCHAR(255),
        seller_protection VARCHAR(50),
        expiration_time TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_authorizations_order ON paypal_authorizations(order_id);
      CREATE INDEX IF NOT EXISTS idx_paypal_authorizations_account ON paypal_authorizations(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_refunds (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        capture_id VARCHAR(255),
        status VARCHAR(50),
        amount NUMERIC(20, 2) DEFAULT 0,
        currency VARCHAR(10) DEFAULT 'USD',
        fee_amount NUMERIC(20, 2),
        net_amount NUMERIC(20, 2),
        invoice_id VARCHAR(255),
        note_to_payer TEXT,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_refunds_capture ON paypal_refunds(capture_id);
      CREATE INDEX IF NOT EXISTS idx_paypal_refunds_date ON paypal_refunds(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_paypal_refunds_account ON paypal_refunds(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_subscriptions (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        plan_id VARCHAR(255),
        status VARCHAR(50),
        subscriber_email VARCHAR(255),
        subscriber_payer_id VARCHAR(255),
        subscriber_name VARCHAR(255),
        start_time TIMESTAMP WITH TIME ZONE,
        quantity VARCHAR(50),
        outstanding_balance NUMERIC(20, 2),
        last_payment_amount NUMERIC(20, 2),
        last_payment_time TIMESTAMP WITH TIME ZONE,
        next_billing_time TIMESTAMP WITH TIME ZONE,
        failed_payments_count INTEGER DEFAULT 0,
        currency VARCHAR(10),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_subscriptions_status ON paypal_subscriptions(status);
      CREATE INDEX IF NOT EXISTS idx_paypal_subscriptions_plan ON paypal_subscriptions(plan_id);
      CREATE INDEX IF NOT EXISTS idx_paypal_subscriptions_account ON paypal_subscriptions(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_subscription_plans (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        product_id VARCHAR(255),
        name VARCHAR(255),
        description TEXT,
        status VARCHAR(50),
        billing_cycles JSONB DEFAULT '[]',
        payment_preferences JSONB,
        taxes JSONB,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_sub_plans_product ON paypal_subscription_plans(product_id);
      CREATE INDEX IF NOT EXISTS idx_paypal_sub_plans_account ON paypal_subscription_plans(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_products (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255),
        description TEXT,
        type VARCHAR(50),
        category VARCHAR(100),
        image_url TEXT,
        home_url TEXT,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_products_account ON paypal_products(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_disputes (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        reason VARCHAR(100),
        status VARCHAR(50),
        amount NUMERIC(20, 2) DEFAULT 0,
        currency VARCHAR(10) DEFAULT 'USD',
        outcome_code VARCHAR(50),
        refunded_amount NUMERIC(20, 2),
        life_cycle_stage VARCHAR(50),
        channel VARCHAR(50),
        seller_transaction_id VARCHAR(255),
        buyer_transaction_id VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_disputes_status ON paypal_disputes(status);
      CREATE INDEX IF NOT EXISTS idx_paypal_disputes_date ON paypal_disputes(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_paypal_disputes_account ON paypal_disputes(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_payouts (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        batch_status VARCHAR(50),
        sender_batch_id VARCHAR(255),
        email_subject TEXT,
        amount NUMERIC(20, 2),
        currency VARCHAR(10),
        fees NUMERIC(20, 2),
        time_created TIMESTAMP WITH TIME ZONE,
        time_completed TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_payouts_status ON paypal_payouts(batch_status);
      CREATE INDEX IF NOT EXISTS idx_paypal_payouts_account ON paypal_payouts(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_invoices (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        status VARCHAR(50),
        invoice_number VARCHAR(255),
        invoice_date VARCHAR(50),
        currency VARCHAR(10) DEFAULT 'USD',
        recipient_email VARCHAR(255),
        recipient_name VARCHAR(255),
        total_amount NUMERIC(20, 2),
        due_amount NUMERIC(20, 2),
        paid_amount NUMERIC(20, 2),
        note TEXT,
        due_date VARCHAR(50),
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_invoices_status ON paypal_invoices(status);
      CREATE INDEX IF NOT EXISTS idx_paypal_invoices_account ON paypal_invoices(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_payers (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        email VARCHAR(255),
        name VARCHAR(255),
        given_name VARCHAR(255),
        surname VARCHAR(255),
        phone VARCHAR(50),
        country_code VARCHAR(10),
        first_seen TIMESTAMP WITH TIME ZONE,
        last_seen TIMESTAMP WITH TIME ZONE,
        total_amount NUMERIC(20, 2) DEFAULT 0,
        transaction_count INTEGER DEFAULT 0,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_payers_email ON paypal_payers(email);
      CREATE INDEX IF NOT EXISTS idx_paypal_payers_account ON paypal_payers(source_account_id);

      CREATE TABLE IF NOT EXISTS paypal_balances (
        currency_code VARCHAR(10) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        total_balance NUMERIC(20, 2),
        available_balance NUMERIC(20, 2),
        withheld_balance NUMERIC(20, 2),
        captured_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (currency_code, source_account_id)
      );

      CREATE TABLE IF NOT EXISTS paypal_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        event_type VARCHAR(255),
        resource_type VARCHAR(255),
        summary TEXT,
        resource JSONB DEFAULT '{}',
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_paypal_webhook_events_type ON paypal_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_paypal_webhook_events_date ON paypal_webhook_events(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_paypal_webhook_events_account ON paypal_webhook_events(source_account_id);
    `);

    logger.info('Created 14 tables');

    // 6 views
    await this.db.query(`
      CREATE OR REPLACE VIEW paypal_donation_summary AS
      SELECT
        t.payer_id,
        t.payer_email,
        t.payer_name,
        t.source_account_id,
        COUNT(*) AS donation_count,
        SUM(t.amount) AS total_donated,
        MIN(t.initiation_date) AS first_donation,
        MAX(t.initiation_date) AS last_donation,
        t.currency
      FROM paypal_transactions t
      WHERE t.status = 'S' AND t.amount > 0
      GROUP BY t.payer_id, t.payer_email, t.payer_name, t.source_account_id, t.currency;

      CREATE OR REPLACE VIEW paypal_active_subscriptions AS
      SELECT
        s.id,
        s.plan_id,
        p.name AS plan_name,
        s.subscriber_email,
        s.subscriber_name,
        s.start_time,
        s.next_billing_time,
        s.last_payment_amount,
        s.last_payment_time,
        s.currency,
        s.source_account_id
      FROM paypal_subscriptions s
      LEFT JOIN paypal_subscription_plans p ON s.plan_id = p.id AND s.source_account_id = p.source_account_id
      WHERE s.status = 'ACTIVE';

      CREATE OR REPLACE VIEW paypal_recurring_revenue AS
      SELECT
        s.source_account_id,
        s.currency,
        COUNT(*) AS active_subscriptions,
        SUM(s.last_payment_amount) AS estimated_mrr
      FROM paypal_subscriptions s
      WHERE s.status = 'ACTIVE' AND s.last_payment_amount IS NOT NULL
      GROUP BY s.source_account_id, s.currency;

      CREATE OR REPLACE VIEW paypal_dispute_summary AS
      SELECT
        d.source_account_id,
        d.status,
        COUNT(*) AS dispute_count,
        SUM(d.amount) AS total_disputed,
        d.currency
      FROM paypal_disputes d
      GROUP BY d.source_account_id, d.status, d.currency;

      CREATE OR REPLACE VIEW paypal_top_donors AS
      SELECT
        p.id AS payer_id,
        p.email,
        p.name,
        p.total_amount,
        p.transaction_count,
        p.first_seen,
        p.last_seen,
        p.source_account_id
      FROM paypal_payers p
      WHERE p.total_amount > 0
      ORDER BY p.total_amount DESC;

      CREATE OR REPLACE VIEW paypal_unified_payments AS
      SELECT
        t.id AS payment_id,
        'transaction' AS payment_type,
        t.source_account_id,
        t.payer_id,
        t.payer_email,
        t.payer_name,
        t.amount,
        t.fee_amount,
        (t.amount - COALESCE(t.fee_amount, 0)) AS net_amount,
        t.currency,
        t.status,
        t.subject AS description,
        t.invoice_id,
        t.metadata,
        t.initiation_date AS created_at
      FROM paypal_transactions t
      WHERE t.amount > 0
      ORDER BY t.initiation_date DESC;
    `);

    logger.info('Created 6 views');
    logger.success('PayPal database schema initialized');
  }

  // ─── Upsert Methods ───────────────────────────────────────────────────

  async upsertTransactions(records: TransactionRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_transactions (id, source_account_id, event_code, initiation_date, updated_date, amount, fee_amount, currency, status, subject, note, payer_email, payer_id, payer_name, invoice_id, custom_field, metadata, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           event_code = EXCLUDED.event_code,
           updated_date = EXCLUDED.updated_date,
           amount = EXCLUDED.amount,
           fee_amount = EXCLUDED.fee_amount,
           currency = EXCLUDED.currency,
           status = EXCLUDED.status,
           subject = EXCLUDED.subject,
           note = EXCLUDED.note,
           payer_email = EXCLUDED.payer_email,
           payer_id = EXCLUDED.payer_id,
           payer_name = EXCLUDED.payer_name,
           invoice_id = EXCLUDED.invoice_id,
           custom_field = EXCLUDED.custom_field,
           metadata = EXCLUDED.metadata,
           synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.event_code, record.initiation_date, record.updated_date, record.amount, record.fee_amount, record.currency, record.status, record.subject, record.note, record.payer_email, record.payer_id, record.payer_name, record.invoice_id, record.custom_field, JSON.stringify(record.metadata)]
      );
      count++;
    }
    return count;
  }

  async upsertOrders(records: OrderRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_orders (id, source_account_id, status, intent, payer_email, payer_id, payer_name, total_amount, currency, description, metadata, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, intent = EXCLUDED.intent, payer_email = EXCLUDED.payer_email,
           payer_id = EXCLUDED.payer_id, payer_name = EXCLUDED.payer_name, total_amount = EXCLUDED.total_amount,
           currency = EXCLUDED.currency, description = EXCLUDED.description, metadata = EXCLUDED.metadata,
           updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.status, record.intent, record.payer_email, record.payer_id, record.payer_name, record.total_amount, record.currency, record.description, JSON.stringify(record.metadata), record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertCaptures(records: CaptureRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_captures (id, source_account_id, order_id, status, amount, currency, fee_amount, net_amount, final_capture, invoice_id, custom_id, seller_protection, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, amount = EXCLUDED.amount, fee_amount = EXCLUDED.fee_amount,
           net_amount = EXCLUDED.net_amount, final_capture = EXCLUDED.final_capture,
           seller_protection = EXCLUDED.seller_protection, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.order_id, record.status, record.amount, record.currency, record.fee_amount, record.net_amount, record.final_capture, record.invoice_id, record.custom_id, record.seller_protection, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertAuthorizations(records: AuthorizationRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_authorizations (id, source_account_id, order_id, status, amount, currency, invoice_id, custom_id, seller_protection, expiration_time, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, amount = EXCLUDED.amount, seller_protection = EXCLUDED.seller_protection,
           expiration_time = EXCLUDED.expiration_time, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.order_id, record.status, record.amount, record.currency, record.invoice_id, record.custom_id, record.seller_protection, record.expiration_time, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertRefunds(records: RefundRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_refunds (id, source_account_id, capture_id, status, amount, currency, fee_amount, net_amount, invoice_id, note_to_payer, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, amount = EXCLUDED.amount, fee_amount = EXCLUDED.fee_amount,
           net_amount = EXCLUDED.net_amount, note_to_payer = EXCLUDED.note_to_payer,
           updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.capture_id, record.status, record.amount, record.currency, record.fee_amount, record.net_amount, record.invoice_id, record.note_to_payer, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertSubscriptions(records: SubscriptionRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_subscriptions (id, source_account_id, plan_id, status, subscriber_email, subscriber_payer_id, subscriber_name, start_time, quantity, outstanding_balance, last_payment_amount, last_payment_time, next_billing_time, failed_payments_count, currency, metadata, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, subscriber_email = EXCLUDED.subscriber_email,
           subscriber_name = EXCLUDED.subscriber_name, outstanding_balance = EXCLUDED.outstanding_balance,
           last_payment_amount = EXCLUDED.last_payment_amount, last_payment_time = EXCLUDED.last_payment_time,
           next_billing_time = EXCLUDED.next_billing_time, failed_payments_count = EXCLUDED.failed_payments_count,
           metadata = EXCLUDED.metadata, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.plan_id, record.status, record.subscriber_email, record.subscriber_payer_id, record.subscriber_name, record.start_time, record.quantity, record.outstanding_balance, record.last_payment_amount, record.last_payment_time, record.next_billing_time, record.failed_payments_count, record.currency, JSON.stringify(record.metadata), record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertSubscriptionPlans(records: SubscriptionPlanRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_subscription_plans (id, source_account_id, product_id, name, description, status, billing_cycles, payment_preferences, taxes, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           name = EXCLUDED.name, description = EXCLUDED.description, status = EXCLUDED.status,
           billing_cycles = EXCLUDED.billing_cycles, payment_preferences = EXCLUDED.payment_preferences,
           taxes = EXCLUDED.taxes, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.product_id, record.name, record.description, record.status, JSON.stringify(record.billing_cycles), JSON.stringify(record.payment_preferences), JSON.stringify(record.taxes), record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertProducts(records: ProductRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_products (id, source_account_id, name, description, type, category, image_url, home_url, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           name = EXCLUDED.name, description = EXCLUDED.description, type = EXCLUDED.type,
           category = EXCLUDED.category, image_url = EXCLUDED.image_url, home_url = EXCLUDED.home_url,
           updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.name, record.description, record.type, record.category, record.image_url, record.home_url, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertDisputes(records: DisputeRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_disputes (id, source_account_id, reason, status, amount, currency, outcome_code, refunded_amount, life_cycle_stage, channel, seller_transaction_id, buyer_transaction_id, metadata, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, outcome_code = EXCLUDED.outcome_code, refunded_amount = EXCLUDED.refunded_amount,
           life_cycle_stage = EXCLUDED.life_cycle_stage, metadata = EXCLUDED.metadata,
           updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.reason, record.status, record.amount, record.currency, record.outcome_code, record.refunded_amount, record.life_cycle_stage, record.channel, record.seller_transaction_id, record.buyer_transaction_id, JSON.stringify(record.metadata), record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertPayouts(records: PayoutRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_payouts (id, source_account_id, batch_status, sender_batch_id, email_subject, amount, currency, fees, time_created, time_completed, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           batch_status = EXCLUDED.batch_status, amount = EXCLUDED.amount, fees = EXCLUDED.fees,
           time_completed = EXCLUDED.time_completed, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.batch_status, record.sender_batch_id, record.email_subject, record.amount, record.currency, record.fees, record.time_created, record.time_completed]
      );
      count++;
    }
    return count;
  }

  async upsertInvoices(records: InvoiceRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_invoices (id, source_account_id, status, invoice_number, invoice_date, currency, recipient_email, recipient_name, total_amount, due_amount, paid_amount, note, due_date, metadata, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, total_amount = EXCLUDED.total_amount, due_amount = EXCLUDED.due_amount,
           paid_amount = EXCLUDED.paid_amount, note = EXCLUDED.note, metadata = EXCLUDED.metadata, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.status, record.invoice_number, record.invoice_date, record.currency, record.recipient_email, record.recipient_name, record.total_amount, record.due_amount, record.paid_amount, record.note, record.due_date, JSON.stringify(record.metadata)]
      );
      count++;
    }
    return count;
  }

  async upsertPayers(records: PayerRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_payers (id, source_account_id, email, name, given_name, surname, phone, country_code, first_seen, last_seen, total_amount, transaction_count, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           email = COALESCE(EXCLUDED.email, paypal_payers.email),
           name = COALESCE(EXCLUDED.name, paypal_payers.name),
           last_seen = GREATEST(EXCLUDED.last_seen, paypal_payers.last_seen),
           total_amount = EXCLUDED.total_amount,
           transaction_count = EXCLUDED.transaction_count,
           synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.email, record.name, record.given_name, record.surname, record.phone, record.country_code, record.first_seen, record.last_seen, record.total_amount, record.transaction_count]
      );
      count++;
    }
    return count;
  }

  async upsertBalances(records: BalanceRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO paypal_balances (currency_code, source_account_id, total_balance, available_balance, withheld_balance, captured_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
         ON CONFLICT (currency_code, source_account_id) DO UPDATE SET
           total_balance = EXCLUDED.total_balance, available_balance = EXCLUDED.available_balance,
           withheld_balance = EXCLUDED.withheld_balance, captured_at = NOW(), synced_at = NOW()`,
        [record.currency_code, this.sourceAccountId, record.total_balance, record.available_balance, record.withheld_balance]
      );
      count++;
    }
    return count;
  }

  // ─── Webhook Event Storage ─────────────────────────────────────────────

  async insertWebhookEvent(event: {
    id: string;
    event_type: string;
    resource_type: string;
    summary: string;
    resource: Record<string, unknown>;
    created_at: Date;
  }): Promise<void> {
    await this.db.execute(
      `INSERT INTO paypal_webhook_events (id, event_type, resource_type, summary, resource, source_account_id, created_at, synced_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
       ON CONFLICT (id) DO NOTHING`,
      [event.id, event.event_type, event.resource_type, event.summary, JSON.stringify(event.resource), this.sourceAccountId, event.created_at]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.db.execute(
      `UPDATE paypal_webhook_events SET processed = true, processed_at = NOW(), error = $2 WHERE id = $1 AND source_account_id = $3`,
      [eventId, error ?? null, this.sourceAccountId]
    );
  }

  // ─── Statistics ────────────────────────────────────────────────────────

  async getStats(): Promise<SyncStats> {
    const tables = [
      { key: 'transactions', table: 'paypal_transactions' },
      { key: 'orders', table: 'paypal_orders' },
      { key: 'captures', table: 'paypal_captures' },
      { key: 'authorizations', table: 'paypal_authorizations' },
      { key: 'refunds', table: 'paypal_refunds' },
      { key: 'subscriptions', table: 'paypal_subscriptions' },
      { key: 'subscriptionPlans', table: 'paypal_subscription_plans' },
      { key: 'products', table: 'paypal_products' },
      { key: 'disputes', table: 'paypal_disputes' },
      { key: 'payouts', table: 'paypal_payouts' },
      { key: 'invoices', table: 'paypal_invoices' },
      { key: 'payers', table: 'paypal_payers' },
      { key: 'balances', table: 'paypal_balances' },
    ] as const;

    const stats: SyncStats = {
      transactions: 0, orders: 0, captures: 0, authorizations: 0,
      refunds: 0, subscriptions: 0, subscriptionPlans: 0, products: 0,
      disputes: 0, payouts: 0, invoices: 0, payers: 0, balances: 0,
      lastSyncedAt: null,
    };

    for (const { key, table } of tables) {
      try {
        const count = await this.db.countScoped(table, this.sourceAccountId);
        (stats as unknown as Record<string, unknown>)[key] = count;
      } catch {
        // Table might not exist yet
      }
    }

    try {
      const result = await this.db.queryOne<{ max: Date | null }>(
        `SELECT MAX(synced_at) as max FROM paypal_transactions WHERE source_account_id = $1`,
        [this.sourceAccountId]
      );
      stats.lastSyncedAt = result?.max ?? null;
    } catch {
      // Ignore
    }

    return stats;
  }

  // ─── Query Methods ─────────────────────────────────────────────────────

  async queryTransactions(options?: { limit?: number; offset?: number; status?: string }): Promise<TransactionRecord[]> {
    let sql = 'SELECT * FROM paypal_transactions';
    const params: unknown[] = [];
    const conditions: string[] = [];

    conditions.push(`source_account_id = $${params.length + 1}`);
    params.push(this.sourceAccountId);

    if (options?.status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(options.status);
    }

    sql += ` WHERE ${conditions.join(' AND ')}`;
    sql += ' ORDER BY initiation_date DESC';
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }

    const result = await this.db.query<TransactionRecord>(sql, params);
    return result.rows;
  }

  async queryOrders(options?: { limit?: number; offset?: number }): Promise<OrderRecord[]> {
    let sql = 'SELECT * FROM paypal_orders WHERE source_account_id = $1 ORDER BY created_at DESC';
    const params: unknown[] = [this.sourceAccountId];
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<OrderRecord>(sql, params);
    return result.rows;
  }

  async querySubscriptions(options?: { limit?: number; offset?: number; status?: string }): Promise<SubscriptionRecord[]> {
    let sql = 'SELECT * FROM paypal_subscriptions';
    const params: unknown[] = [];
    const conditions: string[] = [];

    conditions.push(`source_account_id = $${params.length + 1}`);
    params.push(this.sourceAccountId);

    if (options?.status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(options.status);
    }

    sql += ` WHERE ${conditions.join(' AND ')}`;
    sql += ' ORDER BY created_at DESC';
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<SubscriptionRecord>(sql, params);
    return result.rows;
  }

  async queryDisputes(options?: { limit?: number; offset?: number }): Promise<DisputeRecord[]> {
    let sql = 'SELECT * FROM paypal_disputes WHERE source_account_id = $1 ORDER BY created_at DESC';
    const params: unknown[] = [this.sourceAccountId];
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<DisputeRecord>(sql, params);
    return result.rows;
  }

  async queryRefunds(options?: { limit?: number; offset?: number }): Promise<RefundRecord[]> {
    let sql = 'SELECT * FROM paypal_refunds WHERE source_account_id = $1 ORDER BY created_at DESC';
    const params: unknown[] = [this.sourceAccountId];
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<RefundRecord>(sql, params);
    return result.rows;
  }

  async queryWebhookEvents(options?: { limit?: number }): Promise<Array<Record<string, unknown>>> {
    const limit = options?.limit ?? 50;
    const result = await this.db.query(`SELECT * FROM paypal_webhook_events WHERE source_account_id = $2 ORDER BY created_at DESC LIMIT $1`, [limit, this.sourceAccountId]);
    return result.rows;
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  // ─── Multi-App Cleanup ──────────────────────────────────────────────────

  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    // Child tables first, parent tables last to respect FK constraints
    return this.db.cleanupForAccount([
      'paypal_webhook_events',
      'paypal_balances',
      'paypal_payouts',
      'paypal_refunds',
      'paypal_captures',
      'paypal_authorizations',
      'paypal_disputes',
      'paypal_invoices',
      'paypal_payers',
      'paypal_subscriptions',
      'paypal_subscription_plans',
      'paypal_products',
      'paypal_orders',
      'paypal_transactions',
    ], sourceAccountId);
  }
}

export function createPayPalDatabase(config?: {
  host?: string; port?: number; database?: string; user?: string; password?: string; ssl?: boolean;
}): PayPalDatabase {
  const db = createDatabase({
    host: config?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    port: config?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: config?.database ?? process.env.POSTGRES_DB ?? 'nself',
    user: config?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    password: config?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    ssl: config?.ssl ?? process.env.POSTGRES_SSL === 'true',
  });
  return new PayPalDatabase(db);
}
