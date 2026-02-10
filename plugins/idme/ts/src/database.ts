/**
 * ID.me Database Operations
 */

import { Pool } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type {
  IDmeVerificationRecord,
  IDmeGroupRecord,
  IDmeBadgeRecord,
  IDmeAttributeRecord,
  IDmeWebhookEvent,
  IDmeTokens,
  IDmeUserProfile,
  IDmeVerification,
} from './types.js';
import { BADGE_CONFIG } from './types.js';

const logger = createLogger('idme:database');

export class IDmeDatabase {
  private pool: Pool;
  private readonly sourceAccountId: string;

  constructor(config?: { host?: string; port?: number; database?: string; user?: string; password?: string; ssl?: boolean }, sourceAccountId = 'primary') {
    const dbConfig = config ?? {
      host: process.env.POSTGRES_HOST ?? 'localhost',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    };

    this.pool = new Pool({
      host: dbConfig.host,
      port: dbConfig.port,
      database: dbConfig.database,
      user: dbConfig.user,
      password: dbConfig.password,
      ssl: dbConfig.ssl ? { rejectUnauthorized: false } : false,
    });
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
    logger.info('Database connection pool created');
  }

  /**
   * Create a new IDmeDatabase instance scoped to a different source account,
   * sharing the same underlying connection pool.
   */
  forSourceAccount(sourceAccountId: string): IDmeDatabase {
    return IDmeDatabase.fromPool(this.pool, sourceAccountId);
  }

  /** Internal factory that wraps an existing pool without creating a new one. */
  private static fromPool(pool: Pool, sourceAccountId: string): IDmeDatabase {
    const instance = Object.create(IDmeDatabase.prototype) as IDmeDatabase;
    /* eslint-disable @typescript-eslint/no-explicit-any */
    (instance as any).pool = pool;
    (instance as any).sourceAccountId = instance.normalizeSourceAccountId(sourceAccountId);
    /* eslint-enable @typescript-eslint/no-explicit-any */
    return instance;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^[-_]+|[-_]+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  /**
   * Initialize database schema (CREATE TABLE + migration for existing tables)
   */
  async initializeSchema(): Promise<void> {
    logger.info('Initializing ID.me schema...');

    const schema = `
      CREATE TABLE IF NOT EXISTS idme_verifications (
        id SERIAL PRIMARY KEY,
        user_id VARCHAR(255) NOT NULL,
        idme_user_id VARCHAR(255),
        email VARCHAR(255),
        verified BOOLEAN DEFAULT FALSE,
        verification_level VARCHAR(50),
        first_name VARCHAR(255),
        last_name VARCHAR(255),
        birth_date DATE,
        zip VARCHAR(20),
        phone VARCHAR(50),
        access_token TEXT,
        refresh_token TEXT,
        token_expires_at TIMESTAMP WITH TIME ZONE,
        verified_at TIMESTAMP WITH TIME ZONE,
        last_synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        metadata JSONB DEFAULT '{}',
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(idme_user_id, source_account_id)
      );
      CREATE INDEX IF NOT EXISTS idx_idme_verifications_user ON idme_verifications(user_id);
      CREATE INDEX IF NOT EXISTS idx_idme_verifications_email ON idme_verifications(email);
      CREATE INDEX IF NOT EXISTS idx_idme_verifications_source_account ON idme_verifications(source_account_id);

      CREATE TABLE IF NOT EXISTS idme_groups (
        id SERIAL PRIMARY KEY,
        verification_id INTEGER,
        user_id VARCHAR(255) NOT NULL,
        group_type VARCHAR(50) NOT NULL,
        group_name VARCHAR(255),
        verified BOOLEAN DEFAULT FALSE,
        verified_at TIMESTAMP WITH TIME ZONE,
        expires_at TIMESTAMP WITH TIME ZONE,
        affiliation VARCHAR(255),
        rank VARCHAR(255),
        status VARCHAR(50),
        metadata JSONB DEFAULT '{}',
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(verification_id, group_type, source_account_id)
      );
      CREATE INDEX IF NOT EXISTS idx_idme_groups_user ON idme_groups(user_id);
      CREATE INDEX IF NOT EXISTS idx_idme_groups_type ON idme_groups(group_type);
      CREATE INDEX IF NOT EXISTS idx_idme_groups_source_account ON idme_groups(source_account_id);

      CREATE TABLE IF NOT EXISTS idme_badges (
        id SERIAL PRIMARY KEY,
        verification_id INTEGER,
        user_id VARCHAR(255) NOT NULL,
        badge_type VARCHAR(50) NOT NULL,
        badge_name VARCHAR(255),
        badge_icon VARCHAR(50),
        badge_color VARCHAR(20),
        verified_at TIMESTAMP WITH TIME ZONE,
        expires_at TIMESTAMP WITH TIME ZONE,
        active BOOLEAN DEFAULT TRUE,
        display_order INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(verification_id, badge_type, source_account_id)
      );
      CREATE INDEX IF NOT EXISTS idx_idme_badges_user ON idme_badges(user_id);
      CREATE INDEX IF NOT EXISTS idx_idme_badges_source_account ON idme_badges(source_account_id);

      CREATE TABLE IF NOT EXISTS idme_attributes (
        id SERIAL PRIMARY KEY,
        verification_id INTEGER,
        user_id VARCHAR(255) NOT NULL,
        attribute_key VARCHAR(255) NOT NULL,
        attribute_value TEXT,
        attribute_type VARCHAR(50) DEFAULT 'string',
        verified BOOLEAN DEFAULT FALSE,
        verified_at TIMESTAMP WITH TIME ZONE,
        source VARCHAR(50) DEFAULT 'idme',
        metadata JSONB DEFAULT '{}',
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(verification_id, attribute_key, source_account_id)
      );
      CREATE INDEX IF NOT EXISTS idx_idme_attributes_user ON idme_attributes(user_id);
      CREATE INDEX IF NOT EXISTS idx_idme_attributes_key ON idme_attributes(attribute_key);
      CREATE INDEX IF NOT EXISTS idx_idme_attributes_source_account ON idme_attributes(source_account_id);

      CREATE TABLE IF NOT EXISTS idme_webhook_events (
        id SERIAL PRIMARY KEY,
        event_id VARCHAR(255) UNIQUE,
        event_type VARCHAR(100) NOT NULL,
        user_id VARCHAR(255),
        verification_id INTEGER,
        payload JSONB DEFAULT '{}',
        processed BOOLEAN DEFAULT FALSE,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_idme_webhook_events_type ON idme_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_idme_webhook_events_source_account ON idme_webhook_events(source_account_id);
    `;

    await this.pool.query(schema);

    // Migration: add source_account_id to existing tables that lack it
    const migration = `
      ALTER TABLE idme_verifications ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE idme_groups ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE idme_badges ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE idme_attributes ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE idme_webhook_events ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';

      CREATE INDEX IF NOT EXISTS idx_idme_verifications_source_account ON idme_verifications(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_idme_groups_source_account ON idme_groups(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_idme_badges_source_account ON idme_badges(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_idme_attributes_source_account ON idme_attributes(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_idme_webhook_events_source_account ON idme_webhook_events(source_account_id);
    `;

    await this.pool.query(migration);
    logger.info('ID.me schema initialized');
  }

  /**
   * Store or update verification record
   */
  async upsertVerification(
    userId: string,
    idmeUserId: string,
    profile: IDmeUserProfile,
    tokens: IDmeTokens,
    verification: IDmeVerification
  ): Promise<IDmeVerificationRecord> {
    const query = `
      INSERT INTO idme_verifications (
        user_id, idme_user_id, email, verified, first_name, last_name,
        birth_date, zip, phone, access_token, refresh_token, token_expires_at,
        verified_at, last_synced_at, metadata, source_account_id
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), $14, $15)
      ON CONFLICT (idme_user_id, source_account_id)
      DO UPDATE SET
        verified = $4,
        first_name = $5,
        last_name = $6,
        birth_date = $7,
        zip = $8,
        phone = $9,
        access_token = $10,
        refresh_token = $11,
        token_expires_at = $12,
        verified_at = $13,
        last_synced_at = NOW(),
        metadata = $14,
        updated_at = NOW()
      RETURNING *
    `;

    const values = [
      userId,
      idmeUserId,
      profile.email,
      verification.verified,
      profile.firstName,
      profile.lastName,
      profile.birthDate ? new Date(profile.birthDate) : null,
      profile.zip,
      profile.phone,
      tokens.accessToken,
      tokens.refreshToken,
      tokens.expiresAt,
      verification.verified ? new Date() : null,
      JSON.stringify(verification.attributes),
      this.sourceAccountId,
    ];

    const result = await this.pool.query(query, values);
    logger.info('Verification upserted', { userId, idmeUserId });
    return result.rows[0];
  }

  /**
   * Store verification groups
   */
  async syncGroups(verificationId: string, userId: string, verification: IDmeVerification): Promise<void> {
    for (const group of verification.groups) {
      const query = `
        INSERT INTO idme_groups (
          verification_id, user_id, group_type, group_name, verified,
          verified_at, affiliation, rank, status, metadata, source_account_id
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        ON CONFLICT (verification_id, group_type, source_account_id)
        DO UPDATE SET
          verified = $5,
          verified_at = $6,
          affiliation = $7,
          rank = $8,
          status = $9,
          metadata = $10,
          updated_at = NOW()
      `;

      const values = [
        verificationId,
        userId,
        group.type,
        group.name,
        group.verified,
        group.verifiedAt ? new Date(group.verifiedAt) : null,
        verification.attributes.affiliation,
        verification.attributes.rank,
        verification.attributes.status,
        JSON.stringify({}),
        this.sourceAccountId,
      ];

      await this.pool.query(query, values);
    }

    logger.info('Groups synced', { verificationId, count: verification.groups.length });
  }

  /**
   * Create badges for verified groups
   */
  async syncBadges(verificationId: string, userId: string, verification: IDmeVerification): Promise<void> {
    for (const group of verification.groups) {
      const badgeConfig = BADGE_CONFIG[group.type];
      if (!badgeConfig) continue;

      const query = `
        INSERT INTO idme_badges (
          verification_id, user_id, badge_type, badge_name, badge_icon,
          badge_color, verified_at, active, display_order, source_account_id
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        ON CONFLICT (verification_id, badge_type, source_account_id)
        DO UPDATE SET
          badge_name = $4,
          badge_icon = $5,
          badge_color = $6,
          verified_at = $7,
          active = $8,
          updated_at = NOW()
      `;

      const values = [
        verificationId,
        userId,
        group.type,
        badgeConfig.name,
        badgeConfig.icon,
        badgeConfig.color,
        group.verifiedAt ? new Date(group.verifiedAt) : null,
        true,
        0,
        this.sourceAccountId,
      ];

      await this.pool.query(query, values);
    }

    logger.info('Badges synced', { verificationId, count: verification.groups.length });
  }

  /**
   * Store verification attributes
   */
  async syncAttributes(verificationId: string, userId: string, verification: IDmeVerification): Promise<void> {
    const attributes = verification.attributes;
    const entries = Object.entries(attributes);

    for (const [key, value] of entries) {
      if (!value) continue;

      const query = `
        INSERT INTO idme_attributes (
          verification_id, user_id, attribute_key, attribute_value,
          attribute_type, verified, verified_at, source, source_account_id
        )
        VALUES ($1, $2, $3, $4, $5, $6, NOW(), 'idme', $7)
        ON CONFLICT (verification_id, attribute_key, source_account_id)
        DO UPDATE SET
          attribute_value = $4,
          verified = $6,
          verified_at = NOW(),
          updated_at = NOW()
      `;

      const values = [verificationId, userId, key, String(value), 'string', true, this.sourceAccountId];

      await this.pool.query(query, values);
    }

    logger.info('Attributes synced', { verificationId, count: entries.length });
  }

  /**
   * Get verification by user ID (scoped to source account)
   */
  async getVerificationByUserId(userId: string): Promise<IDmeVerificationRecord | null> {
    const result = await this.pool.query(
      'SELECT * FROM idme_verifications WHERE user_id = $1 AND source_account_id = $2',
      [userId, this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  /**
   * Get verification by email (scoped to source account)
   */
  async getVerificationByEmail(email: string): Promise<IDmeVerificationRecord | null> {
    const result = await this.pool.query(
      'SELECT * FROM idme_verifications WHERE email = $1 AND source_account_id = $2',
      [email, this.sourceAccountId]
    );
    return result.rows[0] || null;
  }

  /**
   * Store webhook event (scoped to source account)
   */
  async storeWebhookEvent(
    eventType: string,
    payload: Record<string, unknown>,
    eventId?: string,
    userId?: string
  ): Promise<IDmeWebhookEvent> {
    const query = `
      INSERT INTO idme_webhook_events (
        event_id, event_type, user_id, payload, received_at, source_account_id
      )
      VALUES ($1, $2, $3, $4, NOW(), $5)
      ON CONFLICT (event_id) DO UPDATE SET retry_count = idme_webhook_events.retry_count + 1
      RETURNING *
    `;

    const values = [eventId || `evt_${Date.now()}`, eventType, userId, JSON.stringify(payload), this.sourceAccountId];

    const result = await this.pool.query(query, values);
    logger.info('Webhook event stored', { eventType, eventId });
    return result.rows[0];
  }

  /**
   * Mark webhook event as processed (scoped to source account)
   */
  async markWebhookProcessed(eventId: string): Promise<void> {
    await this.pool.query(
      'UPDATE idme_webhook_events SET processed = TRUE, processed_at = NOW() WHERE event_id = $1 AND source_account_id = $2',
      [eventId, this.sourceAccountId]
    );
    logger.debug('Webhook marked processed', { eventId });
  }

  /**
   * Delete all data for a specific source account across all IDme tables.
   * Tables are ordered child-first to avoid FK constraint violations.
   */
  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    const tables = [
      'idme_webhook_events',
      'idme_attributes',
      'idme_badges',
      'idme_groups',
      'idme_verifications',
    ];

    let total = 0;
    for (const table of tables) {
      const result = await this.pool.query(
        `DELETE FROM ${table} WHERE source_account_id = $1`,
        [sourceAccountId]
      );
      total += result.rowCount ?? 0;
    }

    logger.info('Cleaned up account data', { sourceAccountId, deletedRows: total });
    return total;
  }

  /**
   * Close database connection
   */
  async close(): Promise<void> {
    await this.pool.end();
    logger.info('Database connection closed');
  }
}

/**
 * Helper to create database instance
 */
export function createDatabase(connectionString?: string): IDmeDatabase {
  return new IDmeDatabase(connectionString);
}
