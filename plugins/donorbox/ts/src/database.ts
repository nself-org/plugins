/**
 * Donorbox Database Operations
 * Schema initialization, CRUD operations, statistics
 */

import { createLogger, createDatabase, type Database } from '@nself/plugin-utils';
import type {
  CampaignRecord,
  DonorRecord,
  DonationRecord,
  PlanRecord,
  EventRecord,
  TicketRecord,
  SyncStats,
} from './types.js';

const logger = createLogger('donorbox:database');

export class DonorboxDatabase {
  private db: Database;
  private sourceAccountId: string;

  constructor(db: Database, sourceAccountId = 'primary') {
    this.db = db;
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(accountId: string): DonorboxDatabase {
    return new DonorboxDatabase(this.db, accountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  // ─── Schema Initialization ─────────────────────────────────────────────

  async initializeSchema(): Promise<void> {
    logger.info('Initializing Donorbox database schema...');

    // 7 tables
    await this.db.query(`
      CREATE TABLE IF NOT EXISTS donorbox_campaigns (
        id INTEGER NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255),
        slug VARCHAR(255),
        currency VARCHAR(10) DEFAULT 'USD',
        goal_amount NUMERIC(20, 2),
        total_raised NUMERIC(20, 2) DEFAULT 0,
        donations_count INTEGER DEFAULT 0,
        is_active BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_campaigns_active ON donorbox_campaigns(is_active);
      CREATE INDEX IF NOT EXISTS idx_donorbox_campaigns_account ON donorbox_campaigns(source_account_id);

      CREATE TABLE IF NOT EXISTS donorbox_donors (
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
        last_donation_at TIMESTAMP WITH TIME ZONE,
        total NUMERIC(20, 2) DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_donors_email ON donorbox_donors(email);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donors_account ON donorbox_donors(source_account_id);

      CREATE TABLE IF NOT EXISTS donorbox_donations (
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
        donation_date TIMESTAMP WITH TIME ZONE,
        processing_fee NUMERIC(20, 2),
        status VARCHAR(50),
        recurring BOOLEAN DEFAULT false,
        comment TEXT,
        designation VARCHAR(255),
        stripe_charge_id VARCHAR(255),
        paypal_transaction_id VARCHAR(255),
        questions JSONB DEFAULT '[]',
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_campaign ON donorbox_donations(campaign_id);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_donor ON donorbox_donations(donor_id);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_date ON donorbox_donations(donation_date DESC);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_status ON donorbox_donations(status);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_stripe ON donorbox_donations(stripe_charge_id);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_paypal ON donorbox_donations(paypal_transaction_id);
      CREATE INDEX IF NOT EXISTS idx_donorbox_donations_account ON donorbox_donations(source_account_id);

      CREATE TABLE IF NOT EXISTS donorbox_plans (
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
        started_at TIMESTAMP WITH TIME ZONE,
        last_donation_date TIMESTAMP WITH TIME ZONE,
        next_donation_date TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_plans_status ON donorbox_plans(status);
      CREATE INDEX IF NOT EXISTS idx_donorbox_plans_donor ON donorbox_plans(donor_id);
      CREATE INDEX IF NOT EXISTS idx_donorbox_plans_account ON donorbox_plans(source_account_id);

      CREATE TABLE IF NOT EXISTS donorbox_events (
        id INTEGER NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255),
        slug VARCHAR(255),
        description TEXT,
        start_date TIMESTAMP WITH TIME ZONE,
        end_date TIMESTAMP WITH TIME ZONE,
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
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_events_active ON donorbox_events(is_active);
      CREATE INDEX IF NOT EXISTS idx_donorbox_events_account ON donorbox_events(source_account_id);

      CREATE TABLE IF NOT EXISTS donorbox_tickets (
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
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_tickets_event ON donorbox_tickets(event_id);
      CREATE INDEX IF NOT EXISTS idx_donorbox_tickets_account ON donorbox_tickets(source_account_id);

      CREATE TABLE IF NOT EXISTS donorbox_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        event_type VARCHAR(255),
        payload JSONB DEFAULT '{}',
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_donorbox_webhook_events_type ON donorbox_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_donorbox_webhook_events_account ON donorbox_webhook_events(source_account_id);
    `);

    logger.info('Created 7 tables');

    // 5 views
    await this.db.query(`
      CREATE OR REPLACE VIEW donorbox_unified_donations AS
      SELECT
        d.id AS donation_id,
        d.source_account_id,
        d.campaign_id,
        d.campaign_name,
        d.donor_id,
        d.donor_email,
        d.donor_name,
        d.amount,
        d.amount_refunded,
        (d.amount - d.amount_refunded) AS net_amount,
        d.currency,
        d.donation_type,
        d.donation_date,
        d.status,
        d.recurring,
        d.comment,
        d.designation,
        d.stripe_charge_id,
        d.paypal_transaction_id,
        d.processing_fee
      FROM donorbox_donations d
      WHERE d.status != 'refunded'
      ORDER BY d.donation_date DESC;

      CREATE OR REPLACE VIEW donorbox_campaign_summary AS
      SELECT
        c.id AS campaign_id,
        c.name,
        c.source_account_id,
        c.goal_amount,
        c.total_raised,
        c.donations_count,
        c.is_active,
        c.currency,
        COUNT(DISTINCT d.donor_id) AS unique_donors
      FROM donorbox_campaigns c
      LEFT JOIN donorbox_donations d ON d.campaign_id = c.id AND d.source_account_id = c.source_account_id
      GROUP BY c.id, c.name, c.source_account_id, c.goal_amount, c.total_raised, c.donations_count, c.is_active, c.currency;

      CREATE OR REPLACE VIEW donorbox_daily_donations AS
      SELECT
        d.source_account_id,
        DATE(d.donation_date) AS donation_day,
        d.currency,
        COUNT(*) AS donation_count,
        SUM(d.amount) AS total_amount,
        SUM(d.amount - d.amount_refunded) AS net_amount
      FROM donorbox_donations d
      WHERE d.donation_date IS NOT NULL
      GROUP BY d.source_account_id, DATE(d.donation_date), d.currency
      ORDER BY donation_day DESC;

      CREATE OR REPLACE VIEW donorbox_recurring_summary AS
      SELECT
        p.source_account_id,
        p.status,
        p.currency,
        COUNT(*) AS plan_count,
        SUM(p.amount) AS total_recurring_amount
      FROM donorbox_plans p
      GROUP BY p.source_account_id, p.status, p.currency;

      CREATE OR REPLACE VIEW donorbox_top_donors AS
      SELECT
        d.id AS donor_id,
        d.source_account_id,
        d.email,
        CONCAT(d.first_name, ' ', d.last_name) AS name,
        d.total,
        d.donations_count,
        d.last_donation_at
      FROM donorbox_donors d
      WHERE d.total > 0
      ORDER BY d.total DESC;
    `);

    logger.info('Created 5 views');
    logger.success('Donorbox database schema initialized');
  }

  // ─── Upsert Methods ───────────────────────────────────────────────────

  async upsertCampaigns(records: CampaignRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO donorbox_campaigns (id, source_account_id, name, slug, currency, goal_amount, total_raised, donations_count, is_active, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           name = EXCLUDED.name, slug = EXCLUDED.slug, goal_amount = EXCLUDED.goal_amount,
           total_raised = EXCLUDED.total_raised, donations_count = EXCLUDED.donations_count,
           is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.name, record.slug, record.currency, record.goal_amount, record.total_raised, record.donations_count, record.is_active, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertDonors(records: DonorRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO donorbox_donors (id, source_account_id, first_name, last_name, email, phone, address, city, state, zip_code, country, employer, donations_count, last_donation_at, total, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, email = EXCLUDED.email,
           phone = EXCLUDED.phone, address = EXCLUDED.address, city = EXCLUDED.city,
           state = EXCLUDED.state, zip_code = EXCLUDED.zip_code, country = EXCLUDED.country,
           employer = EXCLUDED.employer, donations_count = EXCLUDED.donations_count,
           last_donation_at = EXCLUDED.last_donation_at, total = EXCLUDED.total,
           updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.first_name, record.last_name, record.email, record.phone, record.address, record.city, record.state, record.zip_code, record.country, record.employer, record.donations_count, record.last_donation_at, record.total, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertDonations(records: DonationRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO donorbox_donations (id, source_account_id, campaign_id, campaign_name, donor_id, donor_email, donor_name, amount, converted_amount, converted_net_amount, amount_refunded, currency, donation_type, donation_date, processing_fee, status, recurring, comment, designation, stripe_charge_id, paypal_transaction_id, questions, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, amount_refunded = EXCLUDED.amount_refunded,
           converted_amount = EXCLUDED.converted_amount, converted_net_amount = EXCLUDED.converted_net_amount,
           comment = EXCLUDED.comment, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.campaign_id, record.campaign_name, record.donor_id, record.donor_email, record.donor_name, record.amount, record.converted_amount, record.converted_net_amount, record.amount_refunded, record.currency, record.donation_type, record.donation_date, record.processing_fee, record.status, record.recurring, record.comment, record.designation, record.stripe_charge_id, record.paypal_transaction_id, JSON.stringify(record.questions), record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertPlans(records: PlanRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO donorbox_plans (id, source_account_id, campaign_id, campaign_name, donor_id, donor_email, type, amount, currency, status, started_at, last_donation_date, next_donation_date, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, amount = EXCLUDED.amount, last_donation_date = EXCLUDED.last_donation_date,
           next_donation_date = EXCLUDED.next_donation_date, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.campaign_id, record.campaign_name, record.donor_id, record.donor_email, record.type, record.amount, record.currency, record.status, record.started_at, record.last_donation_date, record.next_donation_date, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertEvents(records: EventRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO donorbox_events (id, source_account_id, name, slug, description, start_date, end_date, timezone, venue_name, address, city, state, country, zip_code, currency, tickets_count, is_active, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           name = EXCLUDED.name, description = EXCLUDED.description, tickets_count = EXCLUDED.tickets_count,
           is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.name, record.slug, record.description, record.start_date, record.end_date, record.timezone, record.venue_name, record.address, record.city, record.state, record.country, record.zip_code, record.currency, record.tickets_count, record.is_active, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  async upsertTickets(records: TicketRecord[]): Promise<number> {
    let count = 0;
    for (const record of records) {
      await this.db.execute(
        `INSERT INTO donorbox_tickets (id, source_account_id, event_id, event_name, donor_id, donor_email, ticket_type, quantity, amount, currency, status, created_at, updated_at, synced_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
         ON CONFLICT (id, source_account_id) DO UPDATE SET
           status = EXCLUDED.status, quantity = EXCLUDED.quantity, amount = EXCLUDED.amount,
           updated_at = EXCLUDED.updated_at, synced_at = NOW()`,
        [record.id, this.sourceAccountId, record.event_id, record.event_name, record.donor_id, record.donor_email, record.ticket_type, record.quantity, record.amount, record.currency, record.status, record.created_at, record.updated_at]
      );
      count++;
    }
    return count;
  }

  // ─── Webhook Event Storage ─────────────────────────────────────────────

  async insertWebhookEvent(event: {
    id: string;
    event_type: string;
    payload: Record<string, unknown>;
  }): Promise<void> {
    await this.db.execute(
      `INSERT INTO donorbox_webhook_events (id, event_type, payload, source_account_id, created_at, synced_at)
       VALUES ($1, $2, $3, $4, NOW(), NOW())
       ON CONFLICT (id) DO NOTHING`,
      [event.id, event.event_type, JSON.stringify(event.payload), this.sourceAccountId]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.db.execute(
      `UPDATE donorbox_webhook_events SET processed = true, processed_at = NOW(), error = $2 WHERE id = $1 AND source_account_id = $3`,
      [eventId, error ?? null, this.sourceAccountId]
    );
  }

  // ─── Statistics ────────────────────────────────────────────────────────

  async getStats(): Promise<SyncStats> {
    const tables = [
      { key: 'campaigns', table: 'donorbox_campaigns' },
      { key: 'donors', table: 'donorbox_donors' },
      { key: 'donations', table: 'donorbox_donations' },
      { key: 'plans', table: 'donorbox_plans' },
      { key: 'events', table: 'donorbox_events' },
      { key: 'tickets', table: 'donorbox_tickets' },
    ] as const;

    const stats: SyncStats = {
      campaigns: 0, donors: 0, donations: 0, plans: 0,
      events: 0, tickets: 0, lastSyncedAt: null,
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
        `SELECT MAX(synced_at) as max FROM donorbox_donations WHERE source_account_id = $1`,
        [this.sourceAccountId]
      );
      stats.lastSyncedAt = result?.max ?? null;
    } catch {
      // Ignore
    }

    return stats;
  }

  // ─── Query Methods ─────────────────────────────────────────────────────

  async queryCampaigns(options?: { limit?: number; offset?: number }): Promise<CampaignRecord[]> {
    const params: unknown[] = [this.sourceAccountId];
    let sql = 'SELECT * FROM donorbox_campaigns WHERE source_account_id = $1 ORDER BY created_at DESC';
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<CampaignRecord>(sql, params);
    return result.rows;
  }

  async queryDonors(options?: { limit?: number; offset?: number }): Promise<DonorRecord[]> {
    const params: unknown[] = [this.sourceAccountId];
    let sql = 'SELECT * FROM donorbox_donors WHERE source_account_id = $1 ORDER BY total DESC';
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<DonorRecord>(sql, params);
    return result.rows;
  }

  async queryDonations(options?: { limit?: number; offset?: number; status?: string }): Promise<DonationRecord[]> {
    const params: unknown[] = [this.sourceAccountId];
    let sql = 'SELECT * FROM donorbox_donations WHERE source_account_id = $1';
    if (options?.status) { sql += ` AND status = $${params.length + 1}`; params.push(options.status); }
    sql += ' ORDER BY donation_date DESC';
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<DonationRecord>(sql, params);
    return result.rows;
  }

  async queryPlans(options?: { limit?: number; offset?: number; status?: string }): Promise<PlanRecord[]> {
    const params: unknown[] = [this.sourceAccountId];
    let sql = 'SELECT * FROM donorbox_plans WHERE source_account_id = $1';
    if (options?.status) { sql += ` AND status = $${params.length + 1}`; params.push(options.status); }
    sql += ' ORDER BY created_at DESC';
    if (options?.limit) { sql += ` LIMIT $${params.length + 1}`; params.push(options.limit); }
    if (options?.offset) { sql += ` OFFSET $${params.length + 1}`; params.push(options.offset); }
    const result = await this.db.query<PlanRecord>(sql, params);
    return result.rows;
  }

  async queryWebhookEvents(options?: { limit?: number }): Promise<Array<Record<string, unknown>>> {
    const limit = options?.limit ?? 50;
    const result = await this.db.query(`SELECT * FROM donorbox_webhook_events WHERE source_account_id = $2 ORDER BY created_at DESC LIMIT $1`, [limit, this.sourceAccountId]);
    return result.rows;
  }

  // ─── Multi-App Cleanup ────────────────────────────────────────────────

  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    return this.db.cleanupForAccount([
      'donorbox_webhook_events',
      'donorbox_tickets',
      'donorbox_events',
      'donorbox_plans',
      'donorbox_donations',
      'donorbox_donors',
      'donorbox_campaigns',
    ], sourceAccountId);
  }
}

export function createDonorboxDatabase(config?: {
  host?: string; port?: number; database?: string; user?: string; password?: string; ssl?: boolean;
}): DonorboxDatabase {
  const db = createDatabase({
    host: config?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    port: config?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: config?.database ?? process.env.POSTGRES_DB ?? 'nself',
    user: config?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    password: config?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    ssl: config?.ssl ?? process.env.POSTGRES_SSL === 'true',
  });
  return new DonorboxDatabase(db);
}
