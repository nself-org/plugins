/**
 * Geolocation Plugin Database
 * Schema initialization and CRUD operations
 */

import { createDatabase, Database, createLogger } from '@nself/plugin-utils';
import {
  GeoLocationRecord,
  GeoLatestRecord,
  GeoFenceRecord,
  GeoFenceEventRecord,
  GeoWebhookEventRecord,
  GeolocationStats,
  UpdateLocationRequest,
  CreateGeofenceRequest,
  UpdateGeofenceRequest,
} from './types.js';

const logger = createLogger('geolocation:database');

export class GeolocationDatabase {
  private db: Database;
  private currentAppId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  /**
   * Create scoped database instance for a specific source account
   */
  forSourceAccount(appId: string): GeolocationDatabase {
    const scoped = new GeolocationDatabase(this.db);
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
    logger.info('Initializing geolocation database schema...');

    // Location history table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS geo_locations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        device_id VARCHAR(255),
        latitude DOUBLE PRECISION NOT NULL,
        longitude DOUBLE PRECISION NOT NULL,
        altitude DOUBLE PRECISION,
        accuracy DOUBLE PRECISION,
        speed DOUBLE PRECISION,
        heading DOUBLE PRECISION,
        battery_level INTEGER,
        is_charging BOOLEAN,
        activity_type VARCHAR(20),
        address TEXT,
        metadata JSONB DEFAULT '{}',
        recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_locations_source_app
      ON geo_locations(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_locations_user
      ON geo_locations(source_account_id, user_id, recorded_at DESC);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_locations_recorded
      ON geo_locations(recorded_at);
    `);

    // Latest location table (one row per user)
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS geo_latest (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        device_id VARCHAR(255),
        latitude DOUBLE PRECISION NOT NULL,
        longitude DOUBLE PRECISION NOT NULL,
        altitude DOUBLE PRECISION,
        accuracy DOUBLE PRECISION,
        speed DOUBLE PRECISION,
        heading DOUBLE PRECISION,
        battery_level INTEGER,
        is_charging BOOLEAN,
        activity_type VARCHAR(20),
        address TEXT,
        recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, user_id)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_latest_source_app
      ON geo_latest(source_account_id);
    `);

    // Geofences table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS geo_fences (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        owner_id VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        description TEXT,
        fence_type VARCHAR(20) DEFAULT 'circle',
        latitude DOUBLE PRECISION NOT NULL,
        longitude DOUBLE PRECISION NOT NULL,
        radius_meters DOUBLE PRECISION,
        polygon JSONB,
        address TEXT,
        trigger_on VARCHAR(20) DEFAULT 'both',
        active BOOLEAN DEFAULT true,
        schedule JSONB,
        notify_user_ids TEXT[] DEFAULT '{}',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_fences_source_app
      ON geo_fences(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_fences_owner
      ON geo_fences(source_account_id, owner_id);
    `);

    // Geofence events table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS geo_fence_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        fence_id UUID NOT NULL REFERENCES geo_fences(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        event_type VARCHAR(10) NOT NULL,
        latitude DOUBLE PRECISION NOT NULL,
        longitude DOUBLE PRECISION NOT NULL,
        triggered_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_fence_events_source_app
      ON geo_fence_events(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_fence_events_fence
      ON geo_fence_events(fence_id, triggered_at DESC);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_geo_fence_events_user
      ON geo_fence_events(source_account_id, user_id, triggered_at DESC);
    `);

    // Webhook events table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS geo_webhook_events (
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
      CREATE INDEX IF NOT EXISTS idx_geo_webhook_events_source_app
      ON geo_webhook_events(source_account_id);
    `);

    logger.success('Geolocation database schema initialized');
  }

  // ============================================================================
  // Location Updates
  // ============================================================================

  async insertLocation(req: UpdateLocationRequest): Promise<GeoLocationRecord> {
    const recordedAt = req.recordedAt ? new Date(req.recordedAt) : new Date();

    const result = await this.db.query<GeoLocationRecord>(`
      INSERT INTO geo_locations (
        source_account_id, user_id, device_id, latitude, longitude,
        altitude, accuracy, speed, heading, battery_level,
        is_charging, activity_type, address, metadata, recorded_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
      RETURNING *
    `, [
      this.currentAppId,
      req.userId,
      req.deviceId || null,
      req.latitude,
      req.longitude,
      req.altitude ?? null,
      req.accuracy ?? null,
      req.speed ?? null,
      req.heading ?? null,
      req.batteryLevel ?? null,
      req.isCharging ?? null,
      req.activityType ?? null,
      req.address ?? null,
      JSON.stringify(req.metadata || {}),
      recordedAt,
    ]);

    return result.rows[0];
  }

  async upsertLatest(req: UpdateLocationRequest): Promise<GeoLatestRecord> {
    const recordedAt = req.recordedAt ? new Date(req.recordedAt) : new Date();

    const result = await this.db.query<GeoLatestRecord>(`
      INSERT INTO geo_latest (
        source_account_id, user_id, device_id, latitude, longitude,
        altitude, accuracy, speed, heading, battery_level,
        is_charging, activity_type, address, recorded_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      ON CONFLICT (source_account_id, user_id) DO UPDATE SET
        device_id = EXCLUDED.device_id,
        latitude = EXCLUDED.latitude,
        longitude = EXCLUDED.longitude,
        altitude = EXCLUDED.altitude,
        accuracy = EXCLUDED.accuracy,
        speed = EXCLUDED.speed,
        heading = EXCLUDED.heading,
        battery_level = EXCLUDED.battery_level,
        is_charging = EXCLUDED.is_charging,
        activity_type = EXCLUDED.activity_type,
        address = EXCLUDED.address,
        recorded_at = EXCLUDED.recorded_at
      RETURNING *
    `, [
      this.currentAppId,
      req.userId,
      req.deviceId || null,
      req.latitude,
      req.longitude,
      req.altitude ?? null,
      req.accuracy ?? null,
      req.speed ?? null,
      req.heading ?? null,
      req.batteryLevel ?? null,
      req.isCharging ?? null,
      req.activityType ?? null,
      req.address ?? null,
      recordedAt,
    ]);

    return result.rows[0];
  }

  // ============================================================================
  // Latest Locations
  // ============================================================================

  async getLatestByUserIds(userIds: string[]): Promise<GeoLatestRecord[]> {
    if (userIds.length === 0) {
      const result = await this.db.query<GeoLatestRecord>(`
        SELECT * FROM geo_latest
        WHERE source_account_id = $1
        ORDER BY recorded_at DESC
      `, [this.currentAppId]);
      return result.rows;
    }

    const placeholders = userIds.map((_, i) => `$${i + 2}`).join(', ');
    const result = await this.db.query<GeoLatestRecord>(`
      SELECT * FROM geo_latest
      WHERE source_account_id = $1 AND user_id IN (${placeholders})
      ORDER BY recorded_at DESC
    `, [this.currentAppId, ...userIds]);

    return result.rows;
  }

  async getLatestByUserId(userId: string): Promise<GeoLatestRecord | null> {
    const result = await this.db.query<GeoLatestRecord>(`
      SELECT * FROM geo_latest
      WHERE source_account_id = $1 AND user_id = $2
    `, [this.currentAppId, userId]);

    return result.rows[0] || null;
  }

  // ============================================================================
  // Location History
  // ============================================================================

  async getHistory(userId: string, from?: string, to?: string, limit: number = 1000, offset: number = 0): Promise<{ points: GeoLocationRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1', 'user_id = $2'];
    const params: unknown[] = [this.currentAppId, userId];
    let paramIndex = 3;

    if (from) {
      conditions.push(`recorded_at >= $${paramIndex++}`);
      params.push(from);
    }

    if (to) {
      conditions.push(`recorded_at <= $${paramIndex++}`);
      params.push(to);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM geo_locations WHERE ${whereClause}
    `, params);

    const total = parseInt(String(countResult.rows[0]?.count || 0));

    const result = await this.db.query<GeoLocationRecord>(`
      SELECT * FROM geo_locations
      WHERE ${whereClause}
      ORDER BY recorded_at DESC
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `, [...params, limit, offset]);

    return { points: result.rows, total };
  }

  async deleteHistory(userId: string, olderThan?: string): Promise<number> {
    const conditions: string[] = ['source_account_id = $1', 'user_id = $2'];
    const params: unknown[] = [this.currentAppId, userId];

    if (olderThan) {
      conditions.push('recorded_at < $3');
      params.push(olderThan);
    }

    const whereClause = conditions.join(' AND ');

    const result = await this.db.query<{ count: number }>(`
      WITH deleted AS (
        DELETE FROM geo_locations WHERE ${whereClause} RETURNING *
      )
      SELECT COUNT(*) as count FROM deleted
    `, params);

    return parseInt(String(result.rows[0]?.count || 0));
  }

  // ============================================================================
  // Geofences
  // ============================================================================

  async createFence(req: CreateGeofenceRequest): Promise<GeoFenceRecord> {
    const result = await this.db.query<GeoFenceRecord>(`
      INSERT INTO geo_fences (
        source_account_id, owner_id, name, description, fence_type,
        latitude, longitude, radius_meters, polygon, address,
        trigger_on, active, schedule, notify_user_ids, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
      RETURNING *
    `, [
      this.currentAppId,
      req.ownerId,
      req.name,
      req.description || null,
      req.fenceType || 'circle',
      req.latitude,
      req.longitude,
      req.radiusMeters || null,
      req.polygon ? JSON.stringify(req.polygon) : null,
      req.address || null,
      req.triggerOn || 'both',
      req.active ?? true,
      req.schedule ? JSON.stringify(req.schedule) : null,
      req.notifyUserIds || [],
      JSON.stringify(req.metadata || {}),
    ]);

    return result.rows[0];
  }

  async getFences(ownerId?: string): Promise<GeoFenceRecord[]> {
    if (ownerId) {
      const result = await this.db.query<GeoFenceRecord>(`
        SELECT * FROM geo_fences
        WHERE source_account_id = $1 AND owner_id = $2
        ORDER BY created_at DESC
      `, [this.currentAppId, ownerId]);
      return result.rows;
    }

    const result = await this.db.query<GeoFenceRecord>(`
      SELECT * FROM geo_fences
      WHERE source_account_id = $1
      ORDER BY created_at DESC
    `, [this.currentAppId]);

    return result.rows;
  }

  async getFenceById(id: string): Promise<GeoFenceRecord | null> {
    const result = await this.db.query<GeoFenceRecord>(`
      SELECT * FROM geo_fences
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);

    return result.rows[0] || null;
  }

  async updateFence(id: string, updates: UpdateGeofenceRequest): Promise<GeoFenceRecord> {
    const setClauses: string[] = [];
    const params: unknown[] = [this.currentAppId, id];
    let paramIndex = 3;

    if (updates.name !== undefined) {
      setClauses.push(`name = $${paramIndex++}`);
      params.push(updates.name);
    }

    if (updates.description !== undefined) {
      setClauses.push(`description = $${paramIndex++}`);
      params.push(updates.description);
    }

    if (updates.latitude !== undefined) {
      setClauses.push(`latitude = $${paramIndex++}`);
      params.push(updates.latitude);
    }

    if (updates.longitude !== undefined) {
      setClauses.push(`longitude = $${paramIndex++}`);
      params.push(updates.longitude);
    }

    if (updates.radiusMeters !== undefined) {
      setClauses.push(`radius_meters = $${paramIndex++}`);
      params.push(updates.radiusMeters);
    }

    if (updates.polygon !== undefined) {
      setClauses.push(`polygon = $${paramIndex++}`);
      params.push(JSON.stringify(updates.polygon));
    }

    if (updates.address !== undefined) {
      setClauses.push(`address = $${paramIndex++}`);
      params.push(updates.address);
    }

    if (updates.triggerOn !== undefined) {
      setClauses.push(`trigger_on = $${paramIndex++}`);
      params.push(updates.triggerOn);
    }

    if (updates.active !== undefined) {
      setClauses.push(`active = $${paramIndex++}`);
      params.push(updates.active);
    }

    if (updates.schedule !== undefined) {
      setClauses.push(`schedule = $${paramIndex++}`);
      params.push(JSON.stringify(updates.schedule));
    }

    if (updates.notifyUserIds !== undefined) {
      setClauses.push(`notify_user_ids = $${paramIndex++}`);
      params.push(updates.notifyUserIds);
    }

    if (updates.metadata !== undefined) {
      setClauses.push(`metadata = $${paramIndex++}`);
      params.push(JSON.stringify(updates.metadata));
    }

    setClauses.push('updated_at = NOW()');

    const result = await this.db.query<GeoFenceRecord>(`
      UPDATE geo_fences
      SET ${setClauses.join(', ')}
      WHERE source_account_id = $1 AND id = $2
      RETURNING *
    `, params);

    return result.rows[0];
  }

  async deleteFence(id: string): Promise<void> {
    await this.db.execute(`
      DELETE FROM geo_fences
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);
  }

  async toggleFence(id: string): Promise<GeoFenceRecord> {
    const result = await this.db.query<GeoFenceRecord>(`
      UPDATE geo_fences
      SET active = NOT active, updated_at = NOW()
      WHERE source_account_id = $1 AND id = $2
      RETURNING *
    `, [this.currentAppId, id]);

    return result.rows[0];
  }

  // ============================================================================
  // Geofence Events
  // ============================================================================

  async insertFenceEvent(fenceId: string, userId: string, eventType: string, latitude: number, longitude: number): Promise<GeoFenceEventRecord> {
    const result = await this.db.query<GeoFenceEventRecord>(`
      INSERT INTO geo_fence_events (source_account_id, fence_id, user_id, event_type, latitude, longitude)
      VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *
    `, [this.currentAppId, fenceId, userId, eventType, latitude, longitude]);

    return result.rows[0];
  }

  async getFenceEvents(fenceId: string, from?: string, to?: string, limit: number = 100): Promise<GeoFenceEventRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'fence_id = $2'];
    const params: unknown[] = [this.currentAppId, fenceId];
    let paramIndex = 3;

    if (from) {
      conditions.push(`triggered_at >= $${paramIndex++}`);
      params.push(from);
    }

    if (to) {
      conditions.push(`triggered_at <= $${paramIndex++}`);
      params.push(to);
    }

    const whereClause = conditions.join(' AND ');

    const result = await this.db.query<GeoFenceEventRecord>(`
      SELECT * FROM geo_fence_events
      WHERE ${whereClause}
      ORDER BY triggered_at DESC
      LIMIT $${paramIndex}
    `, [...params, limit]);

    return result.rows;
  }

  async getUserFenceEvents(userId: string, from?: string, to?: string, limit: number = 100): Promise<GeoFenceEventRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'user_id = $2'];
    const params: unknown[] = [this.currentAppId, userId];
    let paramIndex = 3;

    if (from) {
      conditions.push(`triggered_at >= $${paramIndex++}`);
      params.push(from);
    }

    if (to) {
      conditions.push(`triggered_at <= $${paramIndex++}`);
      params.push(to);
    }

    const whereClause = conditions.join(' AND ');

    const result = await this.db.query<GeoFenceEventRecord>(`
      SELECT * FROM geo_fence_events
      WHERE ${whereClause}
      ORDER BY triggered_at DESC
      LIMIT $${paramIndex}
    `, [...params, limit]);

    return result.rows;
  }

  // ============================================================================
  // Geofence Checking
  // ============================================================================

  async checkGeofences(userId: string, latitude: number, longitude: number): Promise<Array<{ fenceId: string; fenceName: string; eventType: 'enter' | 'exit' }>> {
    const events: Array<{ fenceId: string; fenceName: string; eventType: 'enter' | 'exit' }> = [];

    // Get all active fences
    const fences = await this.db.query<GeoFenceRecord>(`
      SELECT * FROM geo_fences
      WHERE source_account_id = $1 AND active = true
    `, [this.currentAppId]);

    for (const fence of fences.rows) {
      if (!fence.radius_meters) continue;

      // Calculate distance using Haversine formula
      const distance = this.haversineDistance(
        latitude, longitude,
        fence.latitude, fence.longitude,
      );

      const isInside = distance <= fence.radius_meters;

      // Check last known state for this user+fence
      const lastEvent = await this.db.query<GeoFenceEventRecord>(`
        SELECT * FROM geo_fence_events
        WHERE source_account_id = $1 AND fence_id = $2 AND user_id = $3
        ORDER BY triggered_at DESC
        LIMIT 1
      `, [this.currentAppId, fence.id, userId]);

      const wasInside = lastEvent.rows[0]?.event_type === 'enter';

      if (isInside && !wasInside && (fence.trigger_on === 'enter' || fence.trigger_on === 'both')) {
        await this.insertFenceEvent(fence.id, userId, 'enter', latitude, longitude);
        events.push({ fenceId: fence.id, fenceName: fence.name, eventType: 'enter' });
      } else if (!isInside && wasInside && (fence.trigger_on === 'exit' || fence.trigger_on === 'both')) {
        await this.insertFenceEvent(fence.id, userId, 'exit', latitude, longitude);
        events.push({ fenceId: fence.id, fenceName: fence.name, eventType: 'exit' });
      }
    }

    return events;
  }

  /**
   * Haversine distance formula (meters)
   */
  private haversineDistance(lat1: number, lon1: number, lat2: number, lon2: number): number {
    const R = 6371000; // Earth's radius in meters
    const dLat = this.toRad(lat2 - lat1);
    const dLon = this.toRad(lon2 - lon1);
    const a =
      Math.sin(dLat / 2) * Math.sin(dLat / 2) +
      Math.cos(this.toRad(lat1)) * Math.cos(this.toRad(lat2)) *
      Math.sin(dLon / 2) * Math.sin(dLon / 2);
    const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
    return R * c;
  }

  private toRad(deg: number): number {
    return deg * (Math.PI / 180);
  }

  // ============================================================================
  // Proximity
  // ============================================================================

  async findNearby(latitude: number, longitude: number, radiusMeters: number, userIds?: string[]): Promise<Array<{ userId: string; latitude: number; longitude: number; distanceMeters: number; lastSeenAt: string }>> {
    let query: string;
    let params: unknown[];

    if (userIds && userIds.length > 0) {
      const placeholders = userIds.map((_, i) => `$${i + 2}`).join(', ');
      query = `
        SELECT user_id, latitude, longitude, recorded_at
        FROM geo_latest
        WHERE source_account_id = $1 AND user_id IN (${placeholders})
      `;
      params = [this.currentAppId, ...userIds];
    } else {
      query = `
        SELECT user_id, latitude, longitude, recorded_at
        FROM geo_latest
        WHERE source_account_id = $1
      `;
      params = [this.currentAppId];
    }

    const result = await this.db.query<{ user_id: string; latitude: number; longitude: number; recorded_at: Date }>(query, params);

    const nearby: Array<{ userId: string; latitude: number; longitude: number; distanceMeters: number; lastSeenAt: string }> = [];

    for (const row of result.rows) {
      const distance = this.haversineDistance(latitude, longitude, row.latitude, row.longitude);
      if (distance <= radiusMeters) {
        nearby.push({
          userId: row.user_id,
          latitude: row.latitude,
          longitude: row.longitude,
          distanceMeters: Math.round(distance),
          lastSeenAt: row.recorded_at.toISOString(),
        });
      }
    }

    return nearby.sort((a, b) => a.distanceMeters - b.distanceMeters);
  }

  async getDistance(userId1: string, userId2: string): Promise<{ distanceMeters: number; user1: GeoLatestRecord; user2: GeoLatestRecord } | null> {
    const user1 = await this.getLatestByUserId(userId1);
    const user2 = await this.getLatestByUserId(userId2);

    if (!user1 || !user2) return null;

    const distance = this.haversineDistance(
      user1.latitude, user1.longitude,
      user2.latitude, user2.longitude,
    );

    return {
      distanceMeters: Math.round(distance),
      user1,
      user2,
    };
  }

  // ============================================================================
  // Webhook Events
  // ============================================================================

  async insertWebhookEvent(eventId: string, eventType: string, payload: Record<string, unknown>): Promise<GeoWebhookEventRecord> {
    const result = await this.db.query<GeoWebhookEventRecord>(`
      INSERT INTO geo_webhook_events (id, source_account_id, event_type, payload)
      VALUES ($1, $2, $3, $4)
      RETURNING *
    `, [eventId, this.currentAppId, eventType, JSON.stringify(payload)]);

    return result.rows[0];
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(): Promise<GeolocationStats> {
    const locationsResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM geo_locations WHERE source_account_id = $1
    `, [this.currentAppId]);

    const usersResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(DISTINCT user_id) as count FROM geo_latest WHERE source_account_id = $1
    `, [this.currentAppId]);

    const fencesResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM geo_fences WHERE source_account_id = $1
    `, [this.currentAppId]);

    const fenceEventsResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM geo_fence_events WHERE source_account_id = $1
    `, [this.currentAppId]);

    const lastLocationResult = await this.db.query<{ recorded_at: Date }>(`
      SELECT recorded_at FROM geo_locations
      WHERE source_account_id = $1
      ORDER BY recorded_at DESC
      LIMIT 1
    `, [this.currentAppId]);

    return {
      totalLocations: parseInt(String(locationsResult.rows[0]?.count || 0)),
      totalUsers: parseInt(String(usersResult.rows[0]?.count || 0)),
      totalFences: parseInt(String(fencesResult.rows[0]?.count || 0)),
      totalFenceEvents: parseInt(String(fenceEventsResult.rows[0]?.count || 0)),
      lastLocationAt: lastLocationResult.rows[0]?.recorded_at?.toISOString() || null,
    };
  }
}

/**
 * Create and initialize geolocation database
 */
export async function createGeolocationDatabase(dbConfig: {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
}): Promise<GeolocationDatabase> {
  const db = createDatabase(dbConfig);
  await db.connect();
  return new GeolocationDatabase(db);
}
