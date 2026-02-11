/**
 * Cloudflare Plugin Database
 * Schema initialization and CRUD operations
 */

import { createDatabase, Database, createLogger } from '@nself/plugin-utils';
import {
  CfZoneRecord,
  CfDnsRecord,
  CfR2BucketRecord,
  CfCachePurgeRecord,
  CfAnalyticsRecord,
  CfWebhookEventRecord,
  CloudflareStats,
} from './types.js';

const logger = createLogger('cloudflare:database');

export class CloudflareDatabase {
  private db: Database;
  private currentAppId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  /**
   * Create scoped database instance for a specific source account
   */
  forSourceAccount(appId: string): CloudflareDatabase {
    const scoped = new CloudflareDatabase(this.db);
    scoped.currentAppId = appId;
    return scoped;
  }

  /**
   * Get current source account ID
   */
  getCurrentAppId(): string {
    return this.currentAppId;
  }

  /**
   * Initialize database schema
   */
  async initSchema(): Promise<void> {
    logger.info('Initializing cloudflare database schema...');

    // Zones table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS cf_zones (
        id VARCHAR(64) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        status VARCHAR(50),
        type VARCHAR(20),
        name_servers TEXT[],
        plan JSONB,
        settings JSONB DEFAULT '{}',
        ssl_status VARCHAR(50),
        synced_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_zones_source_app
      ON cf_zones(source_account_id);
    `);

    // DNS records table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS cf_dns_records (
        id VARCHAR(64) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        zone_id VARCHAR(64) NOT NULL REFERENCES cf_zones(id),
        type VARCHAR(10) NOT NULL,
        name VARCHAR(255) NOT NULL,
        content TEXT NOT NULL,
        ttl INTEGER DEFAULT 1,
        proxied BOOLEAN DEFAULT true,
        priority INTEGER,
        locked BOOLEAN DEFAULT false,
        synced_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_dns_source_app
      ON cf_dns_records(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_dns_zone
      ON cf_dns_records(zone_id);
    `);

    // R2 buckets table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS cf_r2_buckets (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(63) NOT NULL,
        location VARCHAR(50),
        storage_class VARCHAR(50) DEFAULT 'Standard',
        object_count BIGINT DEFAULT 0,
        total_size_bytes BIGINT DEFAULT 0,
        created_at TIMESTAMPTZ,
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_r2_buckets_source_app
      ON cf_r2_buckets(source_account_id);
    `);

    // Cache purge log table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS cf_cache_purge_log (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        zone_id VARCHAR(64) NOT NULL,
        purge_type VARCHAR(20) NOT NULL,
        urls TEXT[],
        tags TEXT[],
        hosts TEXT[],
        prefixes TEXT[],
        status VARCHAR(20) NOT NULL,
        cf_response JSONB,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_cache_purge_source_app
      ON cf_cache_purge_log(source_account_id);
    `);

    // Analytics table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS cf_analytics (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        zone_id VARCHAR(64) NOT NULL,
        date DATE NOT NULL,
        requests_total BIGINT DEFAULT 0,
        requests_cached BIGINT DEFAULT 0,
        requests_uncached BIGINT DEFAULT 0,
        bandwidth_total BIGINT DEFAULT 0,
        bandwidth_cached BIGINT DEFAULT 0,
        threats_total BIGINT DEFAULT 0,
        unique_visitors BIGINT DEFAULT 0,
        status_codes JSONB DEFAULT '{}',
        countries JSONB DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, zone_id, date)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_analytics_source_app
      ON cf_analytics(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_analytics_date
      ON cf_analytics(source_account_id, zone_id, date);
    `);

    // Webhook events table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS cf_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_cf_webhook_events_source_app
      ON cf_webhook_events(source_account_id);
    `);

    logger.success('Cloudflare database schema initialized');
  }

  // ============================================================================
  // Zones
  // ============================================================================

  async upsertZone(zone: Omit<CfZoneRecord, 'synced_at'>): Promise<CfZoneRecord> {
    const result = await this.db.query<CfZoneRecord>(`
      INSERT INTO cf_zones (id, source_account_id, name, status, type, name_servers, plan, settings, ssl_status, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
      ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        status = EXCLUDED.status,
        type = EXCLUDED.type,
        name_servers = EXCLUDED.name_servers,
        plan = EXCLUDED.plan,
        settings = EXCLUDED.settings,
        ssl_status = EXCLUDED.ssl_status,
        synced_at = NOW()
      RETURNING *
    `, [
      zone.id,
      this.currentAppId,
      zone.name,
      zone.status,
      zone.type,
      zone.name_servers,
      JSON.stringify(zone.plan),
      JSON.stringify(zone.settings),
      zone.ssl_status,
    ]);

    return result.rows[0];
  }

  async getZones(): Promise<CfZoneRecord[]> {
    const result = await this.db.query<CfZoneRecord>(`
      SELECT * FROM cf_zones
      WHERE source_account_id = $1
      ORDER BY name
    `, [this.currentAppId]);

    return result.rows;
  }

  async getZoneById(id: string): Promise<CfZoneRecord | null> {
    const result = await this.db.query<CfZoneRecord>(`
      SELECT * FROM cf_zones
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);

    return result.rows[0] || null;
  }

  async updateZoneSettings(id: string, settings: Record<string, unknown>): Promise<CfZoneRecord> {
    const result = await this.db.query<CfZoneRecord>(`
      UPDATE cf_zones
      SET settings = $3, synced_at = NOW()
      WHERE source_account_id = $1 AND id = $2
      RETURNING *
    `, [this.currentAppId, id, JSON.stringify(settings)]);

    return result.rows[0];
  }

  // ============================================================================
  // DNS Records
  // ============================================================================

  async upsertDnsRecord(record: Omit<CfDnsRecord, 'synced_at'>): Promise<CfDnsRecord> {
    const result = await this.db.query<CfDnsRecord>(`
      INSERT INTO cf_dns_records (id, source_account_id, zone_id, type, name, content, ttl, proxied, priority, locked, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
      ON CONFLICT (id) DO UPDATE SET
        type = EXCLUDED.type,
        name = EXCLUDED.name,
        content = EXCLUDED.content,
        ttl = EXCLUDED.ttl,
        proxied = EXCLUDED.proxied,
        priority = EXCLUDED.priority,
        locked = EXCLUDED.locked,
        synced_at = NOW()
      RETURNING *
    `, [
      record.id,
      this.currentAppId,
      record.zone_id,
      record.type,
      record.name,
      record.content,
      record.ttl,
      record.proxied,
      record.priority,
      record.locked,
    ]);

    return result.rows[0];
  }

  async getDnsRecordsByZone(zoneId: string): Promise<CfDnsRecord[]> {
    const result = await this.db.query<CfDnsRecord>(`
      SELECT * FROM cf_dns_records
      WHERE source_account_id = $1 AND zone_id = $2
      ORDER BY type, name
    `, [this.currentAppId, zoneId]);

    return result.rows;
  }

  async getDnsRecordById(id: string): Promise<CfDnsRecord | null> {
    const result = await this.db.query<CfDnsRecord>(`
      SELECT * FROM cf_dns_records
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);

    return result.rows[0] || null;
  }

  async deleteDnsRecord(id: string): Promise<void> {
    await this.db.execute(`
      DELETE FROM cf_dns_records
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);
  }

  // ============================================================================
  // R2 Buckets
  // ============================================================================

  async upsertR2Bucket(bucket: Omit<CfR2BucketRecord, 'id' | 'synced_at'>): Promise<CfR2BucketRecord> {
    const result = await this.db.query<CfR2BucketRecord>(`
      INSERT INTO cf_r2_buckets (source_account_id, name, location, storage_class, object_count, total_size_bytes, created_at, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
      ON CONFLICT (source_account_id, name) DO UPDATE SET
        location = EXCLUDED.location,
        storage_class = EXCLUDED.storage_class,
        object_count = EXCLUDED.object_count,
        total_size_bytes = EXCLUDED.total_size_bytes,
        synced_at = NOW()
      RETURNING *
    `, [
      this.currentAppId,
      bucket.name,
      bucket.location,
      bucket.storage_class,
      bucket.object_count,
      bucket.total_size_bytes,
      bucket.created_at,
    ]);

    return result.rows[0];
  }

  async getR2Buckets(): Promise<CfR2BucketRecord[]> {
    const result = await this.db.query<CfR2BucketRecord>(`
      SELECT * FROM cf_r2_buckets
      WHERE source_account_id = $1
      ORDER BY name
    `, [this.currentAppId]);

    return result.rows;
  }

  async getR2BucketByName(name: string): Promise<CfR2BucketRecord | null> {
    const result = await this.db.query<CfR2BucketRecord>(`
      SELECT * FROM cf_r2_buckets
      WHERE source_account_id = $1 AND name = $2
    `, [this.currentAppId, name]);

    return result.rows[0] || null;
  }

  async deleteR2Bucket(name: string): Promise<void> {
    await this.db.execute(`
      DELETE FROM cf_r2_buckets
      WHERE source_account_id = $1 AND name = $2
    `, [this.currentAppId, name]);
  }

  // ============================================================================
  // Cache Purge Log
  // ============================================================================

  async insertCachePurge(purge: Omit<CfCachePurgeRecord, 'id' | 'created_at'>): Promise<CfCachePurgeRecord> {
    const result = await this.db.query<CfCachePurgeRecord>(`
      INSERT INTO cf_cache_purge_log (source_account_id, zone_id, purge_type, urls, tags, hosts, prefixes, status, cf_response)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *
    `, [
      this.currentAppId,
      purge.zone_id,
      purge.purge_type,
      purge.urls,
      purge.tags,
      purge.hosts,
      purge.prefixes,
      purge.status,
      JSON.stringify(purge.cf_response),
    ]);

    return result.rows[0];
  }

  async getCachePurgesByZone(zoneId: string, limit: number = 50): Promise<CfCachePurgeRecord[]> {
    const result = await this.db.query<CfCachePurgeRecord>(`
      SELECT * FROM cf_cache_purge_log
      WHERE source_account_id = $1 AND zone_id = $2
      ORDER BY created_at DESC
      LIMIT $3
    `, [this.currentAppId, zoneId, limit]);

    return result.rows;
  }

  // ============================================================================
  // Analytics
  // ============================================================================

  async upsertAnalytics(analytics: Omit<CfAnalyticsRecord, 'id' | 'synced_at'>): Promise<CfAnalyticsRecord> {
    const result = await this.db.query<CfAnalyticsRecord>(`
      INSERT INTO cf_analytics (
        source_account_id, zone_id, date, requests_total, requests_cached,
        requests_uncached, bandwidth_total, bandwidth_cached, threats_total,
        unique_visitors, status_codes, countries, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
      ON CONFLICT (source_account_id, zone_id, date) DO UPDATE SET
        requests_total = EXCLUDED.requests_total,
        requests_cached = EXCLUDED.requests_cached,
        requests_uncached = EXCLUDED.requests_uncached,
        bandwidth_total = EXCLUDED.bandwidth_total,
        bandwidth_cached = EXCLUDED.bandwidth_cached,
        threats_total = EXCLUDED.threats_total,
        unique_visitors = EXCLUDED.unique_visitors,
        status_codes = EXCLUDED.status_codes,
        countries = EXCLUDED.countries,
        synced_at = NOW()
      RETURNING *
    `, [
      this.currentAppId,
      analytics.zone_id,
      analytics.date,
      analytics.requests_total,
      analytics.requests_cached,
      analytics.requests_uncached,
      analytics.bandwidth_total,
      analytics.bandwidth_cached,
      analytics.threats_total,
      analytics.unique_visitors,
      JSON.stringify(analytics.status_codes),
      JSON.stringify(analytics.countries),
    ]);

    return result.rows[0];
  }

  async getAnalytics(zoneId: string, from: string, to: string): Promise<CfAnalyticsRecord[]> {
    const result = await this.db.query<CfAnalyticsRecord>(`
      SELECT * FROM cf_analytics
      WHERE source_account_id = $1 AND zone_id = $2
        AND date >= $3::date AND date <= $4::date
      ORDER BY date ASC
    `, [this.currentAppId, zoneId, from, to]);

    return result.rows;
  }

  async getCacheStats(zoneId: string): Promise<{ hitRate: number; totalRequests: number; cachedRequests: number; uncachedRequests: number }> {
    const result = await this.db.query<{
      total_requests: number;
      cached_requests: number;
      uncached_requests: number;
    }>(`
      SELECT
        COALESCE(SUM(requests_total), 0)::bigint AS total_requests,
        COALESCE(SUM(requests_cached), 0)::bigint AS cached_requests,
        COALESCE(SUM(requests_uncached), 0)::bigint AS uncached_requests
      FROM cf_analytics
      WHERE source_account_id = $1 AND zone_id = $2
        AND date >= NOW() - INTERVAL '30 days'
    `, [this.currentAppId, zoneId]);

    const row = result.rows[0];
    const totalRequests = Number(row?.total_requests || 0);
    const cachedRequests = Number(row?.cached_requests || 0);
    const uncachedRequests = Number(row?.uncached_requests || 0);

    return {
      hitRate: totalRequests > 0 ? cachedRequests / totalRequests : 0,
      totalRequests,
      cachedRequests,
      uncachedRequests,
    };
  }

  // ============================================================================
  // Webhook Events
  // ============================================================================

  async insertWebhookEvent(eventId: string, eventType: string, payload: Record<string, unknown>): Promise<CfWebhookEventRecord> {
    const result = await this.db.query<CfWebhookEventRecord>(`
      INSERT INTO cf_webhook_events (id, source_account_id, event_type, payload)
      VALUES ($1, $2, $3, $4)
      RETURNING *
    `, [eventId, this.currentAppId, eventType, JSON.stringify(payload)]);

    return result.rows[0];
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(): Promise<CloudflareStats> {
    const zonesResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM cf_zones WHERE source_account_id = $1
    `, [this.currentAppId]);

    const dnsResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM cf_dns_records WHERE source_account_id = $1
    `, [this.currentAppId]);

    const r2Result = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM cf_r2_buckets WHERE source_account_id = $1
    `, [this.currentAppId]);

    const purgeResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM cf_cache_purge_log WHERE source_account_id = $1
    `, [this.currentAppId]);

    const analyticsResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM cf_analytics WHERE source_account_id = $1
    `, [this.currentAppId]);

    const lastSyncResult = await this.db.query<{ synced_at: Date }>(`
      SELECT synced_at FROM cf_zones
      WHERE source_account_id = $1
      ORDER BY synced_at DESC
      LIMIT 1
    `, [this.currentAppId]);

    return {
      totalZones: parseInt(String(zonesResult.rows[0]?.count || 0)),
      totalDnsRecords: parseInt(String(dnsResult.rows[0]?.count || 0)),
      totalR2Buckets: parseInt(String(r2Result.rows[0]?.count || 0)),
      totalCachePurges: parseInt(String(purgeResult.rows[0]?.count || 0)),
      totalAnalyticsRecords: parseInt(String(analyticsResult.rows[0]?.count || 0)),
      lastSyncedAt: lastSyncResult.rows[0]?.synced_at?.toISOString() || null,
    };
  }
}

/**
 * Create and initialize cloudflare database
 */
export async function createCloudflareDatabase(dbConfig: {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
}): Promise<CloudflareDatabase> {
  const db = createDatabase(dbConfig);
  await db.connect();
  return new CloudflareDatabase(db);
}
