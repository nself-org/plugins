/**
 * DDNS Database Operations
 * Complete CRUD operations for DDNS configurations and update logs
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  DdnsConfigRecord,
  DdnsUpdateLogRecord,
  DdnsStats,
} from './types.js';

const logger = createLogger('ddns:db');

export class DdnsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): DdnsDatabase {
    return new DdnsDatabase(this.db, sourceAccountId);
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
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing DDNS schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- DDNS Configuration
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_ddns_config (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(64) NOT NULL,
        domain VARCHAR(255) NOT NULL,
        hostname VARCHAR(255) NOT NULL DEFAULT '@',
        token TEXT NOT NULL,
        api_key TEXT,
        zone_id VARCHAR(128),
        record_type VARCHAR(10) NOT NULL DEFAULT 'A',
        current_ip VARCHAR(45),
        last_check_at TIMESTAMPTZ,
        last_update_at TIMESTAMPTZ,
        check_interval INTEGER NOT NULL DEFAULT 300,
        is_enabled BOOLEAN DEFAULT true,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, domain, hostname)
      );

      CREATE INDEX IF NOT EXISTS idx_np_ddns_config_source_app
        ON np_ddns_config(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_ddns_config_provider
        ON np_ddns_config(source_account_id, provider);
      CREATE INDEX IF NOT EXISTS idx_np_ddns_config_enabled
        ON np_ddns_config(source_account_id, is_enabled) WHERE is_enabled = true;

      -- =====================================================================
      -- DDNS Update Log
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_ddns_update_log (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        config_id UUID NOT NULL REFERENCES np_ddns_config(id) ON DELETE CASCADE,
        provider VARCHAR(64) NOT NULL,
        domain VARCHAR(255) NOT NULL,
        old_ip VARCHAR(45),
        new_ip VARCHAR(45) NOT NULL,
        status VARCHAR(20) NOT NULL DEFAULT 'success',
        response_code INTEGER,
        response_message TEXT,
        error TEXT,
        duration_ms INTEGER NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_source_app
        ON np_ddns_update_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_config
        ON np_ddns_update_log(config_id);
      CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_status
        ON np_ddns_update_log(source_account_id, status);
      CREATE INDEX IF NOT EXISTS idx_np_ddns_update_log_created
        ON np_ddns_update_log(source_account_id, created_at DESC);
    `;

    await this.execute(schema);
    logger.info('DDNS schema initialized successfully');
  }

  // =========================================================================
  // Config Operations
  // =========================================================================

  async createConfig(config: Omit<DdnsConfigRecord, 'id' | 'created_at' | 'updated_at'>): Promise<DdnsConfigRecord> {
    const result = await this.query<DdnsConfigRecord>(
      `INSERT INTO np_ddns_config (
        source_account_id, provider, domain, hostname, token,
        api_key, zone_id, record_type, current_ip, last_check_at,
        last_update_at, check_interval, is_enabled, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId, config.provider, config.domain,
        config.hostname, config.token, config.api_key,
        config.zone_id, config.record_type, config.current_ip,
        config.last_check_at, config.last_update_at, config.check_interval,
        config.is_enabled, JSON.stringify(config.metadata),
      ]
    );

    return result.rows[0];
  }

  async getConfig(id: string): Promise<DdnsConfigRecord | null> {
    const result = await this.query<DdnsConfigRecord>(
      `SELECT * FROM np_ddns_config WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listConfigs(filters: {
    provider?: string; isEnabled?: boolean;
    limit?: number; offset?: number;
  }): Promise<DdnsConfigRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.provider) {
      conditions.push(`provider = $${paramIndex}`);
      values.push(filters.provider);
      paramIndex++;
    }

    if (filters.isEnabled !== undefined) {
      conditions.push(`is_enabled = $${paramIndex}`);
      values.push(filters.isEnabled);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_ddns_config
      WHERE ${conditions.join(' AND ')}
      ORDER BY domain ASC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<DdnsConfigRecord>(sql, values);
    return result.rows;
  }

  async updateConfig(id: string, updates: Partial<DdnsConfigRecord>): Promise<DdnsConfigRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'provider', 'domain', 'hostname', 'token', 'api_key',
      'zone_id', 'record_type', 'check_interval', 'is_enabled',
      'current_ip', 'last_check_at', 'last_update_at',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (updates.metadata !== undefined) {
      fields.push(`metadata = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.metadata));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getConfig(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<DdnsConfigRecord>(
      `UPDATE np_ddns_config
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteConfig(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_ddns_config WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async updateCurrentIp(id: string, ip: string): Promise<DdnsConfigRecord | null> {
    const result = await this.query<DdnsConfigRecord>(
      `UPDATE np_ddns_config
       SET current_ip = $1, last_check_at = NOW(), last_update_at = NOW(), updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3
       RETURNING *`,
      [ip, id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async updateLastCheck(id: string): Promise<DdnsConfigRecord | null> {
    const result = await this.query<DdnsConfigRecord>(
      `UPDATE np_ddns_config
       SET last_check_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Update Log Operations
  // =========================================================================

  async createUpdateLog(log: Omit<DdnsUpdateLogRecord, 'id' | 'created_at'>): Promise<DdnsUpdateLogRecord> {
    const result = await this.query<DdnsUpdateLogRecord>(
      `INSERT INTO np_ddns_update_log (
        source_account_id, config_id, provider, domain,
        old_ip, new_ip, status, response_code,
        response_message, error, duration_ms
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId, log.config_id, log.provider,
        log.domain, log.old_ip, log.new_ip, log.status,
        log.response_code, log.response_message, log.error,
        log.duration_ms,
      ]
    );

    return result.rows[0];
  }

  async listUpdateLogs(filters: {
    configId?: string; status?: string;
    limit?: number; offset?: number;
  }): Promise<DdnsUpdateLogRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.configId) {
      conditions.push(`config_id = $${paramIndex}`);
      values.push(filters.configId);
      paramIndex++;
    }

    if (filters.status) {
      conditions.push(`status = $${paramIndex}`);
      values.push(filters.status);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_ddns_update_log
      WHERE ${conditions.join(' AND ')}
      ORDER BY created_at DESC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<DdnsUpdateLogRecord>(sql, values);
    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<DdnsStats> {
    const result = await this.query<{
      total_configs: string;
      enabled_configs: string;
      total_updates: string;
      successful_updates: string;
      failed_updates: string;
      skipped_updates: string;
      last_update_at: Date | null;
      last_check_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_ddns_config WHERE source_account_id = $1) as total_configs,
        (SELECT COUNT(*) FROM np_ddns_config WHERE source_account_id = $1 AND is_enabled = true) as enabled_configs,
        (SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1) as total_updates,
        (SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1 AND status = 'success') as successful_updates,
        (SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1 AND status = 'failed') as failed_updates,
        (SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1 AND status = 'skipped') as skipped_updates,
        (SELECT MAX(last_update_at) FROM np_ddns_config WHERE source_account_id = $1) as last_update_at,
        (SELECT MAX(last_check_at) FROM np_ddns_config WHERE source_account_id = $1) as last_check_at`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_configs: parseInt(row.total_configs, 10),
      enabled_configs: parseInt(row.enabled_configs, 10),
      total_updates: parseInt(row.total_updates, 10),
      successful_updates: parseInt(row.successful_updates, 10),
      failed_updates: parseInt(row.failed_updates, 10),
      skipped_updates: parseInt(row.skipped_updates, 10),
      last_update_at: row.last_update_at,
      last_check_at: row.last_check_at,
    };
  }
}
