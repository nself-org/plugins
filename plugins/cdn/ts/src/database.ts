/**
 * CDN Plugin Database Operations
 * Complete CRUD operations for CDN zones, purge requests, analytics, and signed URLs
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ZoneRecord,
  PurgeRequestRecord,
  AnalyticsRecord,
  SignedUrlRecord,
  CreateZoneRequest,
  UpsertAnalyticsRequest,
  AnalyticsSummary,
  PluginStats,
  PurgeType,
  PurgeStatus,
} from './types.js';

const logger = createLogger('cdn:db');

export class CdnDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): CdnDatabase {
    return new CdnDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing CDN schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- CDN Zones
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cdn_zones (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(64) NOT NULL,
        zone_id VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        domain VARCHAR(255) NOT NULL,
        origin_url TEXT,
        ssl_enabled BOOLEAN DEFAULT TRUE,
        cache_ttl INTEGER DEFAULT 86400,
        status VARCHAR(32) DEFAULT 'active',
        config JSONB DEFAULT '{}',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, zone_id)
      );

      CREATE INDEX IF NOT EXISTS idx_cdn_zones_source_account
        ON cdn_zones(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cdn_zones_provider
        ON cdn_zones(provider);
      CREATE INDEX IF NOT EXISTS idx_cdn_zones_domain
        ON cdn_zones(domain);

      -- =====================================================================
      -- Purge Requests
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cdn_purge_requests (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        zone_id UUID REFERENCES cdn_zones(id),
        purge_type VARCHAR(16) NOT NULL,
        urls JSONB DEFAULT '[]',
        tags JSONB DEFAULT '[]',
        prefixes JSONB DEFAULT '[]',
        status VARCHAR(32) NOT NULL DEFAULT 'pending',
        provider_request_id VARCHAR(255),
        requested_by VARCHAR(255),
        completed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_cdn_purge_source_account
        ON cdn_purge_requests(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cdn_purge_status
        ON cdn_purge_requests(status);
      CREATE INDEX IF NOT EXISTS idx_cdn_purge_zone
        ON cdn_purge_requests(zone_id);

      -- =====================================================================
      -- Analytics
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cdn_analytics (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        zone_id UUID REFERENCES cdn_zones(id),
        date DATE NOT NULL,
        requests_total BIGINT DEFAULT 0,
        requests_cached BIGINT DEFAULT 0,
        bandwidth_total BIGINT DEFAULT 0,
        bandwidth_cached BIGINT DEFAULT 0,
        unique_visitors BIGINT DEFAULT 0,
        threats_blocked BIGINT DEFAULT 0,
        status_2xx BIGINT DEFAULT 0,
        status_3xx BIGINT DEFAULT 0,
        status_4xx BIGINT DEFAULT 0,
        status_5xx BIGINT DEFAULT 0,
        top_paths JSONB DEFAULT '[]',
        top_countries JSONB DEFAULT '[]',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, zone_id, date)
      );

      CREATE INDEX IF NOT EXISTS idx_cdn_analytics_source_account
        ON cdn_analytics(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cdn_analytics_date
        ON cdn_analytics(date);
      CREATE INDEX IF NOT EXISTS idx_cdn_analytics_zone
        ON cdn_analytics(zone_id);

      -- =====================================================================
      -- Signed URLs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cdn_signed_urls (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        zone_id UUID REFERENCES cdn_zones(id),
        original_url TEXT NOT NULL,
        signed_url TEXT NOT NULL,
        expires_at TIMESTAMPTZ NOT NULL,
        ip_restriction VARCHAR(45),
        access_count INTEGER DEFAULT 0,
        max_access INTEGER,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_cdn_signed_source_account
        ON cdn_signed_urls(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cdn_signed_expires
        ON cdn_signed_urls(expires_at);
      CREATE INDEX IF NOT EXISTS idx_cdn_signed_zone
        ON cdn_signed_urls(zone_id);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

      CREATE OR REPLACE VIEW cdn_bandwidth_by_zone AS
      SELECT z.source_account_id,
             z.name AS zone_name,
             z.domain,
             z.provider,
             a.date,
             a.bandwidth_total,
             a.bandwidth_cached,
             ROUND(100.0 * a.bandwidth_cached / NULLIF(a.bandwidth_total, 0), 1) AS cache_bandwidth_pct,
             a.requests_total,
             a.requests_cached
      FROM cdn_analytics a
      JOIN cdn_zones z ON a.zone_id = z.id
      WHERE a.date > CURRENT_DATE - INTERVAL '30 days'
      ORDER BY a.date DESC, z.name;

      CREATE OR REPLACE VIEW cdn_cache_hit_rate AS
      SELECT z.source_account_id,
             z.name AS zone_name,
             z.domain,
             SUM(a.requests_total) AS total_requests,
             SUM(a.requests_cached) AS cached_requests,
             ROUND(100.0 * SUM(a.requests_cached) / NULLIF(SUM(a.requests_total), 0), 1) AS hit_rate_pct,
             SUM(a.bandwidth_total) AS total_bandwidth,
             SUM(a.bandwidth_cached) AS cached_bandwidth,
             SUM(a.status_4xx) AS total_4xx,
             SUM(a.status_5xx) AS total_5xx
      FROM cdn_analytics a
      JOIN cdn_zones z ON a.zone_id = z.id
      WHERE a.date > CURRENT_DATE - INTERVAL '30 days'
      GROUP BY z.source_account_id, z.name, z.domain;

      CREATE OR REPLACE VIEW cdn_top_paths AS
      SELECT z.source_account_id,
             z.name AS zone_name,
             a.date,
             path_entry->>'path' AS path,
             (path_entry->>'requests')::BIGINT AS requests,
             (path_entry->>'bandwidth')::BIGINT AS bandwidth
      FROM cdn_analytics a
      JOIN cdn_zones z ON a.zone_id = z.id,
           jsonb_array_elements(a.top_paths) AS path_entry
      WHERE a.date > CURRENT_DATE - INTERVAL '7 days'
      ORDER BY (path_entry->>'requests')::BIGINT DESC;
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Zones
  // =========================================================================

  async createZone(request: CreateZoneRequest): Promise<ZoneRecord> {
    const result = await this.query<ZoneRecord>(
      `INSERT INTO cdn_zones (
        source_account_id, provider, zone_id, name, domain, origin_url,
        ssl_enabled, cache_ttl, config, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      ON CONFLICT (source_account_id, provider, zone_id)
      DO UPDATE SET
        name = EXCLUDED.name,
        domain = EXCLUDED.domain,
        origin_url = COALESCE(EXCLUDED.origin_url, cdn_zones.origin_url),
        ssl_enabled = EXCLUDED.ssl_enabled,
        cache_ttl = EXCLUDED.cache_ttl,
        config = EXCLUDED.config,
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        request.provider,
        request.zone_id,
        request.name,
        request.domain,
        request.origin_url ?? null,
        request.ssl_enabled ?? true,
        request.cache_ttl ?? 86400,
        JSON.stringify(request.config ?? {}),
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getZone(id: string): Promise<ZoneRecord | null> {
    const result = await this.query<ZoneRecord>(
      `SELECT * FROM cdn_zones WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listZones(provider?: string): Promise<ZoneRecord[]> {
    if (provider) {
      const result = await this.query<ZoneRecord>(
        `SELECT * FROM cdn_zones WHERE source_account_id = $1 AND provider = $2 ORDER BY name`,
        [this.sourceAccountId, provider]
      );
      return result.rows;
    }

    const result = await this.query<ZoneRecord>(
      `SELECT * FROM cdn_zones WHERE source_account_id = $1 ORDER BY name`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async deleteZone(id: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM cdn_zones WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Purge Requests
  // =========================================================================

  async createPurgeRequest(
    zoneId: string,
    purgeType: PurgeType,
    options: { urls?: string[]; tags?: string[]; prefixes?: string[]; requested_by?: string } = {}
  ): Promise<PurgeRequestRecord> {
    const result = await this.query<PurgeRequestRecord>(
      `INSERT INTO cdn_purge_requests (
        source_account_id, zone_id, purge_type, urls, tags, prefixes, requested_by
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        zoneId,
        purgeType,
        JSON.stringify(options.urls ?? []),
        JSON.stringify(options.tags ?? []),
        JSON.stringify(options.prefixes ?? []),
        options.requested_by ?? null,
      ]
    );

    return result.rows[0];
  }

  async getPurgeRequest(id: string): Promise<PurgeRequestRecord | null> {
    const result = await this.query<PurgeRequestRecord>(
      `SELECT * FROM cdn_purge_requests WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async updatePurgeStatus(id: string, status: PurgeStatus, providerRequestId?: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE cdn_purge_requests
       SET status = $2,
           provider_request_id = COALESCE($3, provider_request_id),
           error = $4,
           completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE NULL END
       WHERE id = $1`,
      [id, status, providerRequestId ?? null, error ?? null]
    );
  }

  async listPurgeRequests(options: { zone_id?: string; status?: PurgeStatus; limit?: number; offset?: number } = {}): Promise<PurgeRequestRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (options.zone_id) {
      params.push(options.zone_id);
      conditions.push(`zone_id = $${params.length}`);
    }

    if (options.status) {
      params.push(options.status);
      conditions.push(`status = $${params.length}`);
    }

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;
    params.push(limit);
    params.push(offset);

    const result = await this.query<PurgeRequestRecord>(
      `SELECT * FROM cdn_purge_requests
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async getPendingPurges(): Promise<PurgeRequestRecord[]> {
    const result = await this.query<PurgeRequestRecord>(
      `SELECT * FROM cdn_purge_requests
       WHERE source_account_id = $1 AND status = 'pending'
       ORDER BY created_at ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Analytics
  // =========================================================================

  async upsertAnalytics(request: UpsertAnalyticsRequest): Promise<AnalyticsRecord> {
    const result = await this.query<AnalyticsRecord>(
      `INSERT INTO cdn_analytics (
        source_account_id, zone_id, date, requests_total, requests_cached,
        bandwidth_total, bandwidth_cached, unique_visitors, threats_blocked,
        status_2xx, status_3xx, status_4xx, status_5xx,
        top_paths, top_countries, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
      ON CONFLICT (source_account_id, zone_id, date)
      DO UPDATE SET
        requests_total = EXCLUDED.requests_total,
        requests_cached = EXCLUDED.requests_cached,
        bandwidth_total = EXCLUDED.bandwidth_total,
        bandwidth_cached = EXCLUDED.bandwidth_cached,
        unique_visitors = EXCLUDED.unique_visitors,
        threats_blocked = EXCLUDED.threats_blocked,
        status_2xx = EXCLUDED.status_2xx,
        status_3xx = EXCLUDED.status_3xx,
        status_4xx = EXCLUDED.status_4xx,
        status_5xx = EXCLUDED.status_5xx,
        top_paths = EXCLUDED.top_paths,
        top_countries = EXCLUDED.top_countries,
        metadata = EXCLUDED.metadata
      RETURNING *`,
      [
        this.sourceAccountId,
        request.zone_id,
        request.date,
        request.requests_total ?? 0,
        request.requests_cached ?? 0,
        request.bandwidth_total ?? 0,
        request.bandwidth_cached ?? 0,
        request.unique_visitors ?? 0,
        request.threats_blocked ?? 0,
        request.status_2xx ?? 0,
        request.status_3xx ?? 0,
        request.status_4xx ?? 0,
        request.status_5xx ?? 0,
        JSON.stringify(request.top_paths ?? []),
        JSON.stringify(request.top_countries ?? []),
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getAnalytics(options: {
    zone_id?: string;
    from?: string;
    to?: string;
    limit?: number;
    offset?: number;
  } = {}): Promise<{ data: AnalyticsRecord[]; total: number }> {
    const conditions: string[] = ['a.source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (options.zone_id) {
      params.push(options.zone_id);
      conditions.push(`a.zone_id = $${params.length}`);
    }

    if (options.from) {
      params.push(options.from);
      conditions.push(`a.date >= $${params.length}`);
    }

    if (options.to) {
      params.push(options.to);
      conditions.push(`a.date <= $${params.length}`);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.query<{ count: number }>(
      `SELECT COUNT(*) as count FROM cdn_analytics a WHERE ${whereClause}`,
      params
    );
    const total = countResult.rows[0]?.count ?? 0;

    const limit = options.limit ?? 30;
    const offset = options.offset ?? 0;
    params.push(limit);
    params.push(offset);

    const result = await this.query<AnalyticsRecord>(
      `SELECT a.* FROM cdn_analytics a
       WHERE ${whereClause}
       ORDER BY a.date DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return { data: result.rows, total };
  }

  async getAnalyticsSummary(zoneId?: string): Promise<AnalyticsSummary[]> {
    const conditions: string[] = [
      'z.source_account_id = $1',
      'a.date > CURRENT_DATE - INTERVAL \'30 days\'',
    ];
    const params: unknown[] = [this.sourceAccountId];

    if (zoneId) {
      params.push(zoneId);
      conditions.push(`z.id = $${params.length}`);
    }

    const result = await this.query<Record<string, unknown>>(
      `SELECT
        z.id as zone_id,
        z.name as zone_name,
        z.domain,
        COALESCE(SUM(a.requests_total), 0) as total_requests,
        COALESCE(SUM(a.requests_cached), 0) as cached_requests,
        ROUND(100.0 * COALESCE(SUM(a.requests_cached), 0) / NULLIF(COALESCE(SUM(a.requests_total), 0), 0), 1) as cache_hit_rate,
        COALESCE(SUM(a.bandwidth_total), 0) as total_bandwidth,
        COALESCE(SUM(a.bandwidth_cached), 0) as cached_bandwidth,
        COALESCE(SUM(a.unique_visitors), 0) as total_visitors,
        COALESCE(SUM(a.status_4xx), 0) as total_4xx,
        COALESCE(SUM(a.status_5xx), 0) as total_5xx,
        COUNT(DISTINCT a.date) as days_covered
       FROM cdn_zones z
       LEFT JOIN cdn_analytics a ON z.id = a.zone_id AND a.date > CURRENT_DATE - INTERVAL '30 days'
       WHERE ${conditions.join(' AND ')}
       GROUP BY z.id, z.name, z.domain`,
      params
    );

    return result.rows as unknown as AnalyticsSummary[];
  }

  // =========================================================================
  // Signed URLs
  // =========================================================================

  async createSignedUrl(
    zoneId: string,
    originalUrl: string,
    signedUrl: string,
    expiresAt: Date,
    options: { ip_restriction?: string; max_access?: number } = {}
  ): Promise<SignedUrlRecord> {
    const result = await this.query<SignedUrlRecord>(
      `INSERT INTO cdn_signed_urls (
        source_account_id, zone_id, original_url, signed_url, expires_at,
        ip_restriction, max_access
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        zoneId,
        originalUrl,
        signedUrl,
        expiresAt.toISOString(),
        options.ip_restriction ?? null,
        options.max_access ?? null,
      ]
    );

    return result.rows[0];
  }

  async getSignedUrl(id: string): Promise<SignedUrlRecord | null> {
    const result = await this.query<SignedUrlRecord>(
      `SELECT * FROM cdn_signed_urls WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async incrementAccessCount(id: string): Promise<void> {
    await this.execute(
      `UPDATE cdn_signed_urls SET access_count = access_count + 1 WHERE id = $1`,
      [id]
    );
  }

  async cleanExpiredSignedUrls(): Promise<number> {
    return this.execute(
      `DELETE FROM cdn_signed_urls
       WHERE source_account_id = $1 AND expires_at < NOW()`,
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getPluginStats(): Promise<PluginStats> {
    const result = await this.query<{
      total_zones: number;
      active_zones: number;
      total_purges: number;
      pending_purges: number;
      total_signed: number;
      active_signed: number;
      analytics_days: number;
      total_requests: number;
      total_bandwidth: number;
    }>(
      `WITH zones AS (
        SELECT
          COUNT(*) as total,
          COUNT(*) FILTER (WHERE status = 'active') as active
        FROM cdn_zones WHERE source_account_id = $1
      ),
      purges AS (
        SELECT
          COUNT(*) as total,
          COUNT(*) FILTER (WHERE status = 'pending') as pending
        FROM cdn_purge_requests WHERE source_account_id = $1
      ),
      signed AS (
        SELECT
          COUNT(*) as total,
          COUNT(*) FILTER (WHERE expires_at > NOW()) as active
        FROM cdn_signed_urls WHERE source_account_id = $1
      ),
      analytics AS (
        SELECT
          COUNT(DISTINCT date) as days,
          COALESCE(SUM(requests_total), 0) as requests,
          COALESCE(SUM(bandwidth_total), 0) as bandwidth
        FROM cdn_analytics WHERE source_account_id = $1
      )
      SELECT
        z.total as total_zones,
        z.active as active_zones,
        p.total as total_purges,
        p.pending as pending_purges,
        s.total as total_signed,
        s.active as active_signed,
        a.days as analytics_days,
        a.requests as total_requests,
        a.bandwidth as total_bandwidth
      FROM zones z
      CROSS JOIN purges p
      CROSS JOIN signed s
      CROSS JOIN analytics a`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // By provider
    const providerResult = await this.query<{ provider: string; count: number }>(
      `SELECT provider, COUNT(*) as count FROM cdn_zones
       WHERE source_account_id = $1 GROUP BY provider`,
      [this.sourceAccountId]
    );

    const byProvider: Record<string, number> = {};
    for (const r of providerResult.rows) {
      byProvider[r.provider] = r.count;
    }

    return {
      total_zones: row?.total_zones ?? 0,
      active_zones: row?.active_zones ?? 0,
      total_purge_requests: row?.total_purges ?? 0,
      pending_purges: row?.pending_purges ?? 0,
      total_signed_urls: row?.total_signed ?? 0,
      active_signed_urls: row?.active_signed ?? 0,
      analytics_days_tracked: row?.analytics_days ?? 0,
      total_requests_tracked: row?.total_requests ?? 0,
      total_bandwidth_tracked: row?.total_bandwidth ?? 0,
      by_provider: byProvider,
    };
  }
}
