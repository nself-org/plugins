/**
 * Geocoding Plugin Database Operations
 * Complete CRUD operations for geocoding cache, geofences, and places
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  GeoCacheRecord,
  GeofenceRecord,
  GeofenceEventRecord,
  PlaceRecord,
  CreateGeofenceRequest,
  UpdateGeofenceRequest,
  GeoResult,
  CacheStatsResponse,
  PluginStats,
  QueryType,
  GeofenceEventType,
} from './types.js';

const logger = createLogger('geocoding:db');

export class GeocodingDatabase {
  private db: Database;
  private readonly sourceAccountId: string;
  private readonly cacheTtlDays: number;

  constructor(db?: Database, sourceAccountId = 'primary', cacheTtlDays = 365) {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
    this.cacheTtlDays = cacheTtlDays;
  }

  forSourceAccount(sourceAccountId: string): GeocodingDatabase {
    return new GeocodingDatabase(this.db, sourceAccountId, this.cacheTtlDays);
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
    logger.info('Initializing geocoding schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Geocode Cache
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS geo_cache (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        query_type VARCHAR(16) NOT NULL,
        query_hash VARCHAR(64) NOT NULL,
        query_text TEXT NOT NULL,
        provider VARCHAR(64) NOT NULL,
        lat DOUBLE PRECISION,
        lng DOUBLE PRECISION,
        formatted_address TEXT,
        street_number VARCHAR(32),
        street_name VARCHAR(255),
        city VARCHAR(255),
        state VARCHAR(128),
        state_code VARCHAR(8),
        country VARCHAR(128),
        country_code VARCHAR(4),
        postal_code VARCHAR(32),
        place_id VARCHAR(255),
        place_type VARCHAR(64),
        accuracy VARCHAR(32),
        bounds JSONB,
        raw_response JSONB,
        hit_count INTEGER DEFAULT 1,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        expires_at TIMESTAMPTZ,
        UNIQUE(source_account_id, query_hash, provider)
      );

      CREATE INDEX IF NOT EXISTS idx_geo_cache_source_account
        ON geo_cache(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_geo_cache_hash
        ON geo_cache(query_hash);
      CREATE INDEX IF NOT EXISTS idx_geo_cache_coords
        ON geo_cache(lat, lng);
      CREATE INDEX IF NOT EXISTS idx_geo_cache_city
        ON geo_cache(city, state_code);
      CREATE INDEX IF NOT EXISTS idx_geo_cache_expires
        ON geo_cache(expires_at);

      -- =====================================================================
      -- Geofences
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS geo_geofences (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        fence_type VARCHAR(16) NOT NULL DEFAULT 'circle',
        center_lat DOUBLE PRECISION NOT NULL,
        center_lng DOUBLE PRECISION NOT NULL,
        radius_meters DOUBLE PRECISION,
        polygon JSONB,
        active BOOLEAN DEFAULT TRUE,
        notify_on_enter BOOLEAN DEFAULT TRUE,
        notify_on_exit BOOLEAN DEFAULT TRUE,
        notify_url TEXT,
        metadata JSONB DEFAULT '{}',
        created_by VARCHAR(255),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        deleted_at TIMESTAMPTZ
      );

      CREATE INDEX IF NOT EXISTS idx_geo_geofences_source_account
        ON geo_geofences(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_geo_geofences_center
        ON geo_geofences(center_lat, center_lng);
      CREATE INDEX IF NOT EXISTS idx_geo_geofences_active
        ON geo_geofences(active);

      -- =====================================================================
      -- Geofence Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS geo_geofence_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        geofence_id UUID REFERENCES geo_geofences(id),
        event_type VARCHAR(16) NOT NULL,
        entity_id VARCHAR(255) NOT NULL,
        entity_type VARCHAR(64) NOT NULL DEFAULT 'user',
        lat DOUBLE PRECISION NOT NULL,
        lng DOUBLE PRECISION NOT NULL,
        notified BOOLEAN DEFAULT FALSE,
        notified_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_geo_events_source_account
        ON geo_geofence_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_geo_events_fence
        ON geo_geofence_events(geofence_id);
      CREATE INDEX IF NOT EXISTS idx_geo_events_entity
        ON geo_geofence_events(entity_id);

      -- =====================================================================
      -- Places
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS geo_places (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(64) NOT NULL,
        provider_place_id VARCHAR(255) NOT NULL,
        name VARCHAR(512) NOT NULL,
        category VARCHAR(128),
        lat DOUBLE PRECISION NOT NULL,
        lng DOUBLE PRECISION NOT NULL,
        formatted_address TEXT,
        phone VARCHAR(32),
        website TEXT,
        rating FLOAT,
        review_count INTEGER,
        hours JSONB,
        photos JSONB DEFAULT '[]',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, provider_place_id)
      );

      CREATE INDEX IF NOT EXISTS idx_geo_places_source_account
        ON geo_places(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_geo_places_coords
        ON geo_places(lat, lng);
      CREATE INDEX IF NOT EXISTS idx_geo_places_name
        ON geo_places(name);
      CREATE INDEX IF NOT EXISTS idx_geo_places_category
        ON geo_places(category);

      -- =====================================================================
      -- API Quotas and Rate Limiting
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS geo_api_quotas (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        quota_type VARCHAR(16) NOT NULL DEFAULT 'daily',
        period_start TIMESTAMPTZ NOT NULL,
        period_end TIMESTAMPTZ NOT NULL,
        api_calls INTEGER DEFAULT 0,
        geocode_calls INTEGER DEFAULT 0,
        cache_hits INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, quota_type, period_start)
      );

      CREATE INDEX IF NOT EXISTS idx_geo_quotas_source_account
        ON geo_api_quotas(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_geo_quotas_period
        ON geo_api_quotas(period_start, period_end);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

      CREATE OR REPLACE VIEW geo_cache_hit_rate AS
      SELECT source_account_id,
             query_type,
             COUNT(*) AS total_entries,
             SUM(hit_count) AS total_hits,
             ROUND(AVG(hit_count), 2) AS avg_hits_per_entry,
             COUNT(*) FILTER (WHERE hit_count > 1) AS reused_entries,
             ROUND(100.0 * COUNT(*) FILTER (WHERE hit_count > 1) / NULLIF(COUNT(*), 0), 1) AS reuse_pct
      FROM geo_cache
      WHERE expires_at IS NULL OR expires_at > NOW()
      GROUP BY source_account_id, query_type;

      CREATE OR REPLACE VIEW geo_volume_daily AS
      SELECT source_account_id,
             provider,
             DATE(created_at) AS day,
             COUNT(*) AS geocode_count,
             COUNT(DISTINCT query_hash) AS unique_queries
      FROM geo_cache
      WHERE created_at > NOW() - INTERVAL '30 days'
      GROUP BY source_account_id, provider, DATE(created_at)
      ORDER BY day DESC;

      CREATE OR REPLACE VIEW geo_geofence_activity AS
      SELECT g.source_account_id,
             g.id AS geofence_id,
             g.name AS geofence_name,
             g.fence_type,
             COUNT(e.id) AS total_events,
             COUNT(e.id) FILTER (WHERE e.event_type = 'enter') AS enter_count,
             COUNT(e.id) FILTER (WHERE e.event_type = 'exit') AS exit_count,
             COUNT(DISTINCT e.entity_id) AS unique_entities,
             MAX(e.created_at) AS last_event_at
      FROM geo_geofences g
      LEFT JOIN geo_geofence_events e ON g.id = e.geofence_id
      WHERE g.active = TRUE AND g.deleted_at IS NULL
      GROUP BY g.source_account_id, g.id, g.name, g.fence_type;
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Geocode Cache
  // =========================================================================

  private computeQueryHash(queryType: string, queryText: string): string {
    // Simple hash for cache lookup - normalize the query text
    const normalized = `${queryType}:${queryText.toLowerCase().trim().replace(/\s+/g, ' ')}`;
    let hash = 0;
    for (let i = 0; i < normalized.length; i++) {
      const char = normalized.charCodeAt(i);
      hash = ((hash << 5) - hash) + char;
      hash = hash & hash; // Convert to 32bit integer
    }
    return Math.abs(hash).toString(36).padStart(12, '0');
  }

  async getCachedGeocode(queryType: QueryType, queryText: string, provider: string): Promise<GeoCacheRecord | null> {
    const queryHash = this.computeQueryHash(queryType, queryText);

    const result = await this.query<GeoCacheRecord>(
      `UPDATE geo_cache
       SET hit_count = hit_count + 1, updated_at = NOW()
       WHERE source_account_id = $1 AND query_hash = $2 AND provider = $3
         AND (expires_at IS NULL OR expires_at > NOW())
       RETURNING *`,
      [this.sourceAccountId, queryHash, provider]
    );

    return result.rows[0] ?? null;
  }

  async getCachedGeocodeAnyProvider(queryType: QueryType, queryText: string): Promise<GeoCacheRecord | null> {
    const queryHash = this.computeQueryHash(queryType, queryText);

    const result = await this.query<GeoCacheRecord>(
      `UPDATE geo_cache
       SET hit_count = hit_count + 1, updated_at = NOW()
       WHERE source_account_id = $1 AND query_hash = $2
         AND (expires_at IS NULL OR expires_at > NOW())
       RETURNING *`,
      [this.sourceAccountId, queryHash]
    );

    return result.rows[0] ?? null;
  }

  async getStaleCache(queryType: QueryType, queryText: string): Promise<GeoCacheRecord | null> {
    const queryHash = this.computeQueryHash(queryType, queryText);

    const result = await this.query<GeoCacheRecord>(
      `SELECT * FROM geo_cache
       WHERE source_account_id = $1 AND query_hash = $2
       ORDER BY updated_at DESC LIMIT 1`,
      [this.sourceAccountId, queryHash]
    );

    return result.rows[0] ?? null;
  }

  async upsertCacheEntry(
    queryType: QueryType,
    queryText: string,
    provider: string,
    result: GeoResult,
    rawResponse?: Record<string, unknown>
  ): Promise<GeoCacheRecord> {
    const queryHash = this.computeQueryHash(queryType, queryText);
    const expiresAt = this.cacheTtlDays > 0
      ? new Date(Date.now() + this.cacheTtlDays * 24 * 60 * 60 * 1000).toISOString()
      : null;

    const dbResult = await this.query<GeoCacheRecord>(
      `INSERT INTO geo_cache (
        source_account_id, query_type, query_hash, query_text, provider,
        lat, lng, formatted_address, street_number, street_name,
        city, state, state_code, country, country_code, postal_code,
        place_id, place_type, accuracy, raw_response, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
      ON CONFLICT (source_account_id, query_hash, provider)
      DO UPDATE SET
        lat = EXCLUDED.lat,
        lng = EXCLUDED.lng,
        formatted_address = EXCLUDED.formatted_address,
        street_number = EXCLUDED.street_number,
        street_name = EXCLUDED.street_name,
        city = EXCLUDED.city,
        state = EXCLUDED.state,
        state_code = EXCLUDED.state_code,
        country = EXCLUDED.country,
        country_code = EXCLUDED.country_code,
        postal_code = EXCLUDED.postal_code,
        place_id = EXCLUDED.place_id,
        place_type = EXCLUDED.place_type,
        accuracy = EXCLUDED.accuracy,
        raw_response = EXCLUDED.raw_response,
        hit_count = geo_cache.hit_count + 1,
        updated_at = NOW(),
        expires_at = EXCLUDED.expires_at
      RETURNING *`,
      [
        this.sourceAccountId,
        queryType,
        queryHash,
        queryText,
        provider,
        result.lat,
        result.lng,
        result.formatted_address ?? null,
        result.street_number ?? null,
        result.street_name ?? null,
        result.city ?? null,
        result.state ?? null,
        result.state_code ?? null,
        result.country ?? null,
        result.country_code ?? null,
        result.postal_code ?? null,
        result.place_id ?? null,
        result.place_type ?? null,
        result.accuracy ?? null,
        rawResponse ? JSON.stringify(rawResponse) : null,
        expiresAt,
      ]
    );

    return dbResult.rows[0];
  }

  async getCacheStats(): Promise<CacheStatsResponse> {
    const result = await this.query<{
      total_entries: number;
      active_entries: number;
      expired_entries: number;
      total_hits: number;
      avg_hits: number;
      reuse_pct: number;
    }>(
      `SELECT
        COUNT(*) as total_entries,
        COUNT(*) FILTER (WHERE expires_at IS NULL OR expires_at > NOW()) as active_entries,
        COUNT(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at <= NOW()) as expired_entries,
        COALESCE(SUM(hit_count), 0) as total_hits,
        ROUND(COALESCE(AVG(hit_count), 0), 2) as avg_hits,
        ROUND(100.0 * COUNT(*) FILTER (WHERE hit_count > 1) / NULLIF(COUNT(*), 0), 1) as reuse_pct
       FROM geo_cache
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // By query type
    const typeResult = await this.query<{ query_type: string; count: number }>(
      `SELECT query_type, COUNT(*) as count FROM geo_cache
       WHERE source_account_id = $1 GROUP BY query_type`,
      [this.sourceAccountId]
    );

    const byQueryType: Record<string, number> = {};
    for (const r of typeResult.rows) {
      byQueryType[r.query_type] = r.count;
    }

    // By provider
    const providerResult = await this.query<{ provider: string; count: number }>(
      `SELECT provider, COUNT(*) as count FROM geo_cache
       WHERE source_account_id = $1 GROUP BY provider`,
      [this.sourceAccountId]
    );

    const byProvider: Record<string, number> = {};
    for (const r of providerResult.rows) {
      byProvider[r.provider] = r.count;
    }

    return {
      total_entries: row?.total_entries ?? 0,
      active_entries: row?.active_entries ?? 0,
      expired_entries: row?.expired_entries ?? 0,
      total_hits: row?.total_hits ?? 0,
      avg_hits_per_entry: row?.avg_hits ?? 0,
      reuse_percentage: row?.reuse_pct ?? 0,
      by_query_type: byQueryType,
      by_provider: byProvider,
    };
  }

  async clearCache(olderThanDays?: number): Promise<number> {
    if (olderThanDays) {
      return this.execute(
        `DELETE FROM geo_cache
         WHERE source_account_id = $1 AND created_at < NOW() - $2 * INTERVAL '1 day'`,
        [this.sourceAccountId, olderThanDays]
      );
    }

    return this.execute(
      `DELETE FROM geo_cache WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Geofences
  // =========================================================================

  async createGeofence(request: CreateGeofenceRequest): Promise<GeofenceRecord> {
    const result = await this.query<GeofenceRecord>(
      `INSERT INTO geo_geofences (
        source_account_id, name, description, fence_type, center_lat, center_lng,
        radius_meters, polygon, notify_on_enter, notify_on_exit, notify_url,
        metadata, created_by
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.name,
        request.description ?? null,
        request.fence_type ?? 'circle',
        request.center_lat,
        request.center_lng,
        request.radius_meters ?? null,
        request.polygon ? JSON.stringify(request.polygon) : null,
        request.notify_on_enter ?? true,
        request.notify_on_exit ?? true,
        request.notify_url ?? null,
        JSON.stringify(request.metadata ?? {}),
        request.created_by ?? null,
      ]
    );

    return result.rows[0];
  }

  async getGeofence(id: string): Promise<GeofenceRecord | null> {
    const result = await this.query<GeofenceRecord>(
      `SELECT * FROM geo_geofences
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listGeofences(options: {
    active?: boolean;
    near_lat?: number;
    near_lng?: number;
    radius?: number;
  } = {}): Promise<GeofenceRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'deleted_at IS NULL'];
    const params: unknown[] = [this.sourceAccountId];

    if (options.active !== undefined) {
      params.push(options.active);
      conditions.push(`active = $${params.length}`);
    }

    if (options.near_lat !== undefined && options.near_lng !== undefined) {
      const radius = options.radius ?? 10000; // Default 10km
      // Approximate distance filter using lat/lng degrees
      const latDegrees = radius / 111320;
      const lngDegrees = radius / (111320 * Math.cos((options.near_lat * Math.PI) / 180));
      params.push(options.near_lat - latDegrees);
      params.push(options.near_lat + latDegrees);
      params.push(options.near_lng - lngDegrees);
      params.push(options.near_lng + lngDegrees);
      conditions.push(`center_lat BETWEEN $${params.length - 3} AND $${params.length - 2}`);
      conditions.push(`center_lng BETWEEN $${params.length - 1} AND $${params.length}`);
    }

    const result = await this.query<GeofenceRecord>(
      `SELECT * FROM geo_geofences
       WHERE ${conditions.join(' AND ')}
       ORDER BY name`,
      params
    );

    return result.rows;
  }

  async updateGeofence(id: string, updates: UpdateGeofenceRequest): Promise<GeofenceRecord | null> {
    const setParts: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId];

    if (updates.name !== undefined) {
      params.push(updates.name);
      setParts.push(`name = $${params.length}`);
    }
    if (updates.description !== undefined) {
      params.push(updates.description);
      setParts.push(`description = $${params.length}`);
    }
    if (updates.center_lat !== undefined) {
      params.push(updates.center_lat);
      setParts.push(`center_lat = $${params.length}`);
    }
    if (updates.center_lng !== undefined) {
      params.push(updates.center_lng);
      setParts.push(`center_lng = $${params.length}`);
    }
    if (updates.radius_meters !== undefined) {
      params.push(updates.radius_meters);
      setParts.push(`radius_meters = $${params.length}`);
    }
    if (updates.polygon !== undefined) {
      params.push(JSON.stringify(updates.polygon));
      setParts.push(`polygon = $${params.length}`);
    }
    if (updates.active !== undefined) {
      params.push(updates.active);
      setParts.push(`active = $${params.length}`);
    }
    if (updates.notify_on_enter !== undefined) {
      params.push(updates.notify_on_enter);
      setParts.push(`notify_on_enter = $${params.length}`);
    }
    if (updates.notify_on_exit !== undefined) {
      params.push(updates.notify_on_exit);
      setParts.push(`notify_on_exit = $${params.length}`);
    }
    if (updates.notify_url !== undefined) {
      params.push(updates.notify_url);
      setParts.push(`notify_url = $${params.length}`);
    }
    if (updates.metadata !== undefined) {
      params.push(JSON.stringify(updates.metadata));
      setParts.push(`metadata = $${params.length}`);
    }

    const result = await this.query<GeofenceRecord>(
      `UPDATE geo_geofences SET ${setParts.join(', ')}
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteGeofence(id: string): Promise<boolean> {
    const rowCount = await this.execute(
      `UPDATE geo_geofences SET deleted_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async evaluateGeofences(lat: number, lng: number): Promise<{ geofence: GeofenceRecord; inside: boolean; distance_meters: number }[]> {
    // Get all active circle geofences
    const geofences = await this.listGeofences({ active: true });
    const results: { geofence: GeofenceRecord; inside: boolean; distance_meters: number }[] = [];

    for (const fence of geofences) {
      if (fence.fence_type === 'circle' && fence.radius_meters) {
        // Haversine distance calculation
        const R = 6371000; // Earth's radius in meters
        const dLat = ((lat - fence.center_lat) * Math.PI) / 180;
        const dLng = ((lng - fence.center_lng) * Math.PI) / 180;
        const a =
          Math.sin(dLat / 2) * Math.sin(dLat / 2) +
          Math.cos((fence.center_lat * Math.PI) / 180) *
            Math.cos((lat * Math.PI) / 180) *
            Math.sin(dLng / 2) *
            Math.sin(dLng / 2);
        const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
        const distance = R * c;

        results.push({
          geofence: fence,
          inside: distance <= fence.radius_meters,
          distance_meters: Math.round(distance),
        });
      }
    }

    return results;
  }

  async insertGeofenceEvent(
    geofenceId: string,
    eventType: GeofenceEventType,
    entityId: string,
    entityType: string,
    lat: number,
    lng: number
  ): Promise<GeofenceEventRecord> {
    const result = await this.query<GeofenceEventRecord>(
      `INSERT INTO geo_geofence_events (
        source_account_id, geofence_id, event_type, entity_id, entity_type, lat, lng
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [this.sourceAccountId, geofenceId, eventType, entityId, entityType, lat, lng]
    );
    return result.rows[0];
  }

  async getGeofenceEvents(
    geofenceId: string,
    options: { from?: string; to?: string; entity_id?: string } = {}
  ): Promise<GeofenceEventRecord[]> {
    const conditions: string[] = [
      'source_account_id = $1',
      'geofence_id = $2',
    ];
    const params: unknown[] = [this.sourceAccountId, geofenceId];

    if (options.from) {
      params.push(options.from);
      conditions.push(`created_at >= $${params.length}`);
    }
    if (options.to) {
      params.push(options.to);
      conditions.push(`created_at <= $${params.length}`);
    }
    if (options.entity_id) {
      params.push(options.entity_id);
      conditions.push(`entity_id = $${params.length}`);
    }

    const result = await this.query<GeofenceEventRecord>(
      `SELECT * FROM geo_geofence_events
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC`,
      params
    );

    return result.rows;
  }

  // =========================================================================
  // Places
  // =========================================================================

  async upsertPlace(
    provider: string,
    providerPlaceId: string,
    name: string,
    lat: number,
    lng: number,
    details: {
      category?: string;
      formatted_address?: string;
      phone?: string;
      website?: string;
      rating?: number;
      review_count?: number;
      hours?: Record<string, unknown>;
      photos?: unknown[];
      metadata?: Record<string, unknown>;
    } = {}
  ): Promise<PlaceRecord> {
    const result = await this.query<PlaceRecord>(
      `INSERT INTO geo_places (
        source_account_id, provider, provider_place_id, name, category,
        lat, lng, formatted_address, phone, website, rating, review_count,
        hours, photos, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
      ON CONFLICT (source_account_id, provider, provider_place_id)
      DO UPDATE SET
        name = EXCLUDED.name,
        category = COALESCE(EXCLUDED.category, geo_places.category),
        lat = EXCLUDED.lat,
        lng = EXCLUDED.lng,
        formatted_address = COALESCE(EXCLUDED.formatted_address, geo_places.formatted_address),
        phone = COALESCE(EXCLUDED.phone, geo_places.phone),
        website = COALESCE(EXCLUDED.website, geo_places.website),
        rating = COALESCE(EXCLUDED.rating, geo_places.rating),
        review_count = COALESCE(EXCLUDED.review_count, geo_places.review_count),
        hours = COALESCE(EXCLUDED.hours, geo_places.hours),
        photos = COALESCE(EXCLUDED.photos, geo_places.photos),
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        provider,
        providerPlaceId,
        name,
        details.category ?? null,
        lat,
        lng,
        details.formatted_address ?? null,
        details.phone ?? null,
        details.website ?? null,
        details.rating ?? null,
        details.review_count ?? null,
        details.hours ? JSON.stringify(details.hours) : null,
        JSON.stringify(details.photos ?? []),
        JSON.stringify(details.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async searchPlaces(options: {
    query?: string;
    lat?: number;
    lng?: number;
    radius?: number;
    category?: string;
    limit?: number;
  } = {}): Promise<PlaceRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (options.query) {
      params.push(`%${options.query}%`);
      conditions.push(`name ILIKE $${params.length}`);
    }

    if (options.category) {
      params.push(options.category);
      conditions.push(`category = $${params.length}`);
    }

    if (options.lat !== undefined && options.lng !== undefined && options.radius) {
      const latDegrees = options.radius / 111320;
      const lngDegrees = options.radius / (111320 * Math.cos((options.lat * Math.PI) / 180));
      params.push(options.lat - latDegrees);
      params.push(options.lat + latDegrees);
      params.push(options.lng - lngDegrees);
      params.push(options.lng + lngDegrees);
      conditions.push(`lat BETWEEN $${params.length - 3} AND $${params.length - 2}`);
      conditions.push(`lng BETWEEN $${params.length - 1} AND $${params.length}`);
    }

    const limit = options.limit ?? 20;
    params.push(limit);

    const result = await this.query<PlaceRecord>(
      `SELECT * FROM geo_places
       WHERE ${conditions.join(' AND ')}
       ORDER BY rating DESC NULLS LAST
       LIMIT $${params.length}`,
      params
    );

    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getPluginStats(): Promise<PluginStats> {
    const result = await this.query<{
      total_cache_entries: number;
      total_geofences: number;
      active_geofences: number;
      total_geofence_events: number;
      total_places: number;
      total_hits: number;
      total_entries_with_hits: number;
    }>(
      `WITH cache AS (
        SELECT COUNT(*) as count, COALESCE(SUM(hit_count), 0) as total_hits
        FROM geo_cache WHERE source_account_id = $1
      ),
      fences AS (
        SELECT
          COUNT(*) as total,
          COUNT(*) FILTER (WHERE active = TRUE AND deleted_at IS NULL) as active
        FROM geo_geofences WHERE source_account_id = $1
      ),
      events AS (
        SELECT COUNT(*) as count FROM geo_geofence_events WHERE source_account_id = $1
      ),
      places AS (
        SELECT COUNT(*) as count FROM geo_places WHERE source_account_id = $1
      )
      SELECT
        c.count as total_cache_entries,
        f.total as total_geofences,
        f.active as active_geofences,
        e.count as total_geofence_events,
        p.count as total_places,
        c.total_hits,
        c.count as total_entries_with_hits
      FROM cache c
      CROSS JOIN fences f
      CROSS JOIN events e
      CROSS JOIN places p`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // Cache hit rate: total_hits / (total_hits + total_entries) where new entries count as misses
    const totalHits = row?.total_hits ?? 0;
    const totalEntries = row?.total_cache_entries ?? 0;
    const hitRate = totalEntries > 0 ? ((totalHits - totalEntries) / totalHits) * 100 : 0;

    // By provider
    const providerResult = await this.query<{ provider: string; count: number }>(
      `SELECT provider, COUNT(*) as count FROM geo_cache
       WHERE source_account_id = $1 GROUP BY provider`,
      [this.sourceAccountId]
    );

    const byProvider: Record<string, number> = {};
    for (const r of providerResult.rows) {
      byProvider[r.provider] = r.count;
    }

    return {
      total_cache_entries: row?.total_cache_entries ?? 0,
      total_geofences: row?.total_geofences ?? 0,
      active_geofences: row?.active_geofences ?? 0,
      total_geofence_events: row?.total_geofence_events ?? 0,
      total_places: row?.total_places ?? 0,
      cache_hit_rate: Math.max(0, Math.round(hitRate * 10) / 10),
      by_provider: byProvider,
    };
  }

  // =========================================================================
  // API Quotas and Rate Limiting
  // =========================================================================

  async incrementApiQuota(quotaType: 'daily' | 'monthly' = 'daily', isGeocodeCall: boolean = false, isCacheHit: boolean = false): Promise<void> {
    const now = new Date();
    let periodStart: Date;
    let periodEnd: Date;

    if (quotaType === 'daily') {
      periodStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
      periodEnd = new Date(periodStart.getTime() + 24 * 60 * 60 * 1000);
    } else {
      periodStart = new Date(now.getFullYear(), now.getMonth(), 1);
      periodEnd = new Date(now.getFullYear(), now.getMonth() + 1, 1);
    }

    await this.execute(
      `INSERT INTO geo_api_quotas (
        source_account_id, quota_type, period_start, period_end,
        api_calls, geocode_calls, cache_hits
      ) VALUES ($1, $2, $3, $4, 1, $5, $6)
      ON CONFLICT (source_account_id, quota_type, period_start)
      DO UPDATE SET
        api_calls = geo_api_quotas.api_calls + 1,
        geocode_calls = geo_api_quotas.geocode_calls + $5,
        cache_hits = geo_api_quotas.cache_hits + $6,
        updated_at = NOW()`,
      [
        this.sourceAccountId,
        quotaType,
        periodStart.toISOString(),
        periodEnd.toISOString(),
        isGeocodeCall ? 1 : 0,
        isCacheHit ? 1 : 0,
      ]
    );
  }

  async getQuotaUsage(quotaType: 'daily' | 'monthly' = 'daily'): Promise<{
    api_calls: number;
    geocode_calls: number;
    cache_hits: number;
    period_start: Date;
    period_end: Date;
  } | null> {
    const now = new Date();
    let periodStart: Date;

    if (quotaType === 'daily') {
      periodStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    } else {
      periodStart = new Date(now.getFullYear(), now.getMonth(), 1);
    }

    const result = await this.query<{
      api_calls: number;
      geocode_calls: number;
      cache_hits: number;
      period_start: Date;
      period_end: Date;
    }>(
      `SELECT api_calls, geocode_calls, cache_hits, period_start, period_end
       FROM geo_api_quotas
       WHERE source_account_id = $1 AND quota_type = $2 AND period_start = $3`,
      [this.sourceAccountId, quotaType, periodStart.toISOString()]
    );

    return result.rows[0] ?? null;
  }

  async checkQuotaLimit(quotaType: 'daily' | 'monthly' = 'daily', maxCalls: number): Promise<{ allowed: boolean; current: number; limit: number }> {
    const usage = await this.getQuotaUsage(quotaType);
    const current = usage?.api_calls ?? 0;

    return {
      allowed: current < maxCalls,
      current,
      limit: maxCalls,
    };
  }
}
