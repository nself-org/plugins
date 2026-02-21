/**
 * mDNS Database Operations
 * Complete CRUD operations for mDNS services and discovery logs
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ServiceRecord,
  DiscoveryLogRecord,
  MdnsStats,
} from './types.js';

const logger = createLogger('mdns:db');

export class MdnsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): MdnsDatabase {
    return new MdnsDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing mDNS schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- mDNS Services (advertised by this instance)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_mdns_services (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        service_name VARCHAR(255) NOT NULL,
        service_type VARCHAR(128) NOT NULL DEFAULT '_ntv._tcp',
        port INTEGER NOT NULL,
        host VARCHAR(255) NOT NULL DEFAULT 'localhost',
        domain VARCHAR(128) NOT NULL DEFAULT 'local',
        txt_records JSONB DEFAULT '{}',
        is_advertised BOOLEAN DEFAULT false,
        is_active BOOLEAN DEFAULT true,
        last_seen_at TIMESTAMPTZ DEFAULT NOW(),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, service_name, service_type)
      );

      CREATE INDEX IF NOT EXISTS idx_np_mdns_services_source_app
        ON np_mdns_services(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_mdns_services_type
        ON np_mdns_services(source_account_id, service_type);
      CREATE INDEX IF NOT EXISTS idx_np_mdns_services_advertised
        ON np_mdns_services(source_account_id, is_advertised) WHERE is_advertised = true;

      -- =====================================================================
      -- mDNS Discovery Log (services discovered on the network)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_mdns_discovery_log (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        service_type VARCHAR(128) NOT NULL,
        service_name VARCHAR(255) NOT NULL,
        host VARCHAR(255) NOT NULL,
        port INTEGER NOT NULL,
        addresses TEXT[] DEFAULT '{}',
        txt_records JSONB DEFAULT '{}',
        discovered_at TIMESTAMPTZ DEFAULT NOW(),
        last_seen_at TIMESTAMPTZ DEFAULT NOW(),
        is_available BOOLEAN DEFAULT true,
        metadata JSONB DEFAULT '{}',
        UNIQUE(source_account_id, service_name, service_type, host)
      );

      CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_log_source_app
        ON np_mdns_discovery_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_log_type
        ON np_mdns_discovery_log(source_account_id, service_type);
      CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_log_available
        ON np_mdns_discovery_log(source_account_id, is_available) WHERE is_available = true;
    `;

    await this.execute(schema);
    logger.info('mDNS schema initialized successfully');
  }

  // =========================================================================
  // Service Operations
  // =========================================================================

  async createService(service: Omit<ServiceRecord, 'id' | 'created_at' | 'updated_at'>): Promise<ServiceRecord> {
    const result = await this.query<ServiceRecord>(
      `INSERT INTO np_mdns_services (
        source_account_id, service_name, service_type, port, host,
        domain, txt_records, is_advertised, is_active, last_seen_at, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId, service.service_name, service.service_type,
        service.port, service.host, service.domain,
        JSON.stringify(service.txt_records), service.is_advertised,
        service.is_active, service.last_seen_at, JSON.stringify(service.metadata),
      ]
    );

    return result.rows[0];
  }

  async getService(id: string): Promise<ServiceRecord | null> {
    const result = await this.query<ServiceRecord>(
      `SELECT * FROM np_mdns_services WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listServices(filters: {
    serviceType?: string; isAdvertised?: boolean; isActive?: boolean;
    limit?: number; offset?: number;
  }): Promise<ServiceRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.serviceType) {
      conditions.push(`service_type = $${paramIndex}`);
      values.push(filters.serviceType);
      paramIndex++;
    }

    if (filters.isAdvertised !== undefined) {
      conditions.push(`is_advertised = $${paramIndex}`);
      values.push(filters.isAdvertised);
      paramIndex++;
    }

    if (filters.isActive !== undefined) {
      conditions.push(`is_active = $${paramIndex}`);
      values.push(filters.isActive);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_mdns_services
      WHERE ${conditions.join(' AND ')}
      ORDER BY service_name ASC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<ServiceRecord>(sql, values);
    return result.rows;
  }

  async updateService(id: string, updates: Partial<ServiceRecord>): Promise<ServiceRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'service_name', 'service_type', 'port', 'host', 'domain',
      'is_advertised', 'is_active',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (updates.txt_records !== undefined) {
      fields.push(`txt_records = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.txt_records));
      paramIndex++;
    }

    if (updates.metadata !== undefined) {
      fields.push(`metadata = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.metadata));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getService(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<ServiceRecord>(
      `UPDATE np_mdns_services
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteService(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_mdns_services WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async setAdvertised(id: string, advertised: boolean): Promise<ServiceRecord | null> {
    const result = await this.query<ServiceRecord>(
      `UPDATE np_mdns_services
       SET is_advertised = $1, updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3
       RETURNING *`,
      [advertised, id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Discovery Log Operations
  // =========================================================================

  async upsertDiscovery(discovery: Omit<DiscoveryLogRecord, 'id'>): Promise<DiscoveryLogRecord> {
    const result = await this.query<DiscoveryLogRecord>(
      `INSERT INTO np_mdns_discovery_log (
        source_account_id, service_type, service_name, host, port,
        addresses, txt_records, discovered_at, last_seen_at, is_available, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      ON CONFLICT (source_account_id, service_name, service_type, host) DO UPDATE SET
        port = EXCLUDED.port,
        addresses = EXCLUDED.addresses,
        txt_records = EXCLUDED.txt_records,
        last_seen_at = EXCLUDED.last_seen_at,
        is_available = EXCLUDED.is_available
      RETURNING *`,
      [
        this.sourceAccountId, discovery.service_type, discovery.service_name,
        discovery.host, discovery.port, discovery.addresses,
        JSON.stringify(discovery.txt_records), discovery.discovered_at,
        discovery.last_seen_at, discovery.is_available,
        JSON.stringify(discovery.metadata),
      ]
    );

    return result.rows[0];
  }

  async listDiscoveries(filters: {
    serviceType?: string; isAvailable?: boolean;
    limit?: number; offset?: number;
  }): Promise<DiscoveryLogRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.serviceType) {
      conditions.push(`service_type = $${paramIndex}`);
      values.push(filters.serviceType);
      paramIndex++;
    }

    if (filters.isAvailable !== undefined) {
      conditions.push(`is_available = $${paramIndex}`);
      values.push(filters.isAvailable);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_mdns_discovery_log
      WHERE ${conditions.join(' AND ')}
      ORDER BY last_seen_at DESC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<DiscoveryLogRecord>(sql, values);
    return result.rows;
  }

  async markUnavailable(id: string): Promise<boolean> {
    const count = await this.execute(
      `UPDATE np_mdns_discovery_log
       SET is_available = false
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<MdnsStats> {
    const result = await this.query<{
      total_services: string;
      advertised_services: string;
      active_services: string;
      total_discovered: string;
      available_discovered: string;
      last_discovery_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_mdns_services WHERE source_account_id = $1) as total_services,
        (SELECT COUNT(*) FROM np_mdns_services WHERE source_account_id = $1 AND is_advertised = true) as advertised_services,
        (SELECT COUNT(*) FROM np_mdns_services WHERE source_account_id = $1 AND is_active = true) as active_services,
        (SELECT COUNT(*) FROM np_mdns_discovery_log WHERE source_account_id = $1) as total_discovered,
        (SELECT COUNT(*) FROM np_mdns_discovery_log WHERE source_account_id = $1 AND is_available = true) as available_discovered,
        (SELECT MAX(discovered_at) FROM np_mdns_discovery_log WHERE source_account_id = $1) as last_discovery_at`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_services: parseInt(row.total_services, 10),
      advertised_services: parseInt(row.advertised_services, 10),
      active_services: parseInt(row.active_services, 10),
      total_discovered: parseInt(row.total_discovered, 10),
      available_discovered: parseInt(row.available_discovered, 10),
      last_discovery_at: row.last_discovery_at,
    };
  }
}
