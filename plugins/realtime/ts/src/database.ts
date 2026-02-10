/**
 * Database operations for realtime plugin
 */

import pg from 'pg';
import type {
  Connection,
  Room,
  RoomMember,
  Presence,
  TypingIndicator,
  RealtimeEvent,
  DeviceInfo,
} from './types.js';

const { Pool } = pg;

export class Database {
  private pool: pg.Pool;
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
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });

    this.sourceAccountId = this.normalizeId(sourceAccountId);
  }

  /**
   * Create a new Database instance scoped to a specific source account,
   * sharing the same underlying connection pool.
   */
  forSourceAccount(sourceAccountId: string): Database {
    const scoped = Object.create(Database.prototype) as Database;
    Object.defineProperty(scoped, 'pool', { value: this.pool, writable: false });
    Object.defineProperty(scoped, 'sourceAccountId', { value: this.normalizeId(sourceAccountId), writable: false });
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^[-_]+|[-_]+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  async query<T = unknown>(text: string, params?: unknown[]): Promise<T[]> {
    const result = await this.pool.query(text, params);
    return result.rows;
  }

  async queryOne<T = unknown>(text: string, params?: unknown[]): Promise<T | null> {
    const rows = await this.query<T>(text, params);
    return rows[0] || null;
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // -------------------------------------------------------------------------
  // Schema Initialization & Migration
  // -------------------------------------------------------------------------

  async initializeSchema(): Promise<void> {
    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- Connections
      CREATE TABLE IF NOT EXISTS realtime_connections (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        socket_id VARCHAR(255) UNIQUE NOT NULL,
        user_id VARCHAR(255),
        session_id VARCHAR(255),
        status VARCHAR(20) DEFAULT 'connected',
        transport VARCHAR(20),
        ip_address INET,
        user_agent TEXT,
        device_info JSONB,
        connected_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        disconnected_at TIMESTAMP WITH TIME ZONE,
        last_ping TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        last_pong TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        latency_ms INTEGER,
        metadata JSONB DEFAULT '{}'
      );

      CREATE INDEX IF NOT EXISTS idx_realtime_connections_socket_id ON realtime_connections(socket_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_connections_user_id ON realtime_connections(user_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_connections_status ON realtime_connections(status);
      CREATE INDEX IF NOT EXISTS idx_realtime_connections_connected_at ON realtime_connections(connected_at);
      CREATE INDEX IF NOT EXISTS idx_realtime_connections_source_account ON realtime_connections(source_account_id);

      -- Rooms (UNIQUE on name + source_account_id so each app gets its own namespace)
      CREATE TABLE IF NOT EXISTS realtime_rooms (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        type VARCHAR(50) DEFAULT 'channel',
        visibility VARCHAR(20) DEFAULT 'public',
        max_members INTEGER,
        is_active BOOLEAN DEFAULT TRUE,
        settings JSONB DEFAULT '{}',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(name, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_realtime_rooms_name ON realtime_rooms(name);
      CREATE INDEX IF NOT EXISTS idx_realtime_rooms_type ON realtime_rooms(type);
      CREATE INDEX IF NOT EXISTS idx_realtime_rooms_is_active ON realtime_rooms(is_active);
      CREATE INDEX IF NOT EXISTS idx_realtime_rooms_source_account ON realtime_rooms(source_account_id);

      -- Room Members
      CREATE TABLE IF NOT EXISTS realtime_room_members (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        room_id UUID REFERENCES realtime_rooms(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        role VARCHAR(50) DEFAULT 'member',
        is_muted BOOLEAN DEFAULT FALSE,
        is_banned BOOLEAN DEFAULT FALSE,
        joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        metadata JSONB DEFAULT '{}',
        UNIQUE(room_id, user_id)
      );

      CREATE INDEX IF NOT EXISTS idx_realtime_room_members_room_id ON realtime_room_members(room_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_room_members_user_id ON realtime_room_members(user_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_room_members_joined_at ON realtime_room_members(joined_at);
      CREATE INDEX IF NOT EXISTS idx_realtime_room_members_source_account ON realtime_room_members(source_account_id);

      -- Presence (UNIQUE on user_id + source_account_id)
      CREATE TABLE IF NOT EXISTS realtime_presence (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        status VARCHAR(20) DEFAULT 'offline',
        custom_status TEXT,
        custom_emoji VARCHAR(100),
        last_active TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        last_heartbeat TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        expires_at TIMESTAMP WITH TIME ZONE,
        connections_count INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(user_id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_realtime_presence_user_id ON realtime_presence(user_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_presence_status ON realtime_presence(status);
      CREATE INDEX IF NOT EXISTS idx_realtime_presence_last_active ON realtime_presence(last_active);
      CREATE INDEX IF NOT EXISTS idx_realtime_presence_source_account ON realtime_presence(source_account_id);

      -- Typing Indicators
      CREATE TABLE IF NOT EXISTS realtime_typing (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        room_id UUID REFERENCES realtime_rooms(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        thread_id VARCHAR(255),
        started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
        UNIQUE(room_id, user_id, thread_id)
      );

      CREATE INDEX IF NOT EXISTS idx_realtime_typing_room_id ON realtime_typing(room_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_typing_user_id ON realtime_typing(user_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_typing_expires_at ON realtime_typing(expires_at);
      CREATE INDEX IF NOT EXISTS idx_realtime_typing_source_account ON realtime_typing(source_account_id);

      -- Events
      CREATE TABLE IF NOT EXISTS realtime_events (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(100) NOT NULL,
        socket_id VARCHAR(255),
        user_id VARCHAR(255),
        room_id UUID REFERENCES realtime_rooms(id) ON DELETE SET NULL,
        payload JSONB DEFAULT '{}',
        ip_address INET,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_realtime_events_event_type ON realtime_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_realtime_events_socket_id ON realtime_events(socket_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_events_user_id ON realtime_events(user_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_events_room_id ON realtime_events(room_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_events_created_at ON realtime_events(created_at);
      CREATE INDEX IF NOT EXISTS idx_realtime_events_source_account ON realtime_events(source_account_id);

      -- -----------------------------------------------------------------------
      -- Migration: add source_account_id to tables that already exist
      -- -----------------------------------------------------------------------
      ALTER TABLE realtime_connections ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE realtime_rooms ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE realtime_room_members ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE realtime_presence ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE realtime_typing ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';
      ALTER TABLE realtime_events ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary';

      CREATE INDEX IF NOT EXISTS idx_realtime_connections_source_account ON realtime_connections(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_rooms_source_account ON realtime_rooms(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_room_members_source_account ON realtime_room_members(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_presence_source_account ON realtime_presence(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_typing_source_account ON realtime_typing(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_realtime_events_source_account ON realtime_events(source_account_id);

      -- Migrate UNIQUE constraints for existing tables.
      -- Drop the old single-column unique constraints if they exist and recreate
      -- as compound constraints. The IF NOT EXISTS on the new constraint is handled
      -- by checking pg_constraint first.

      -- Rooms: (name) -> (name, source_account_id)
      DO $$ BEGIN
        IF EXISTS (
          SELECT 1 FROM pg_constraint
          WHERE conname = 'realtime_rooms_name_key'
        ) THEN
          ALTER TABLE realtime_rooms DROP CONSTRAINT realtime_rooms_name_key;
        END IF;
      END $$;

      DO $$ BEGIN
        IF NOT EXISTS (
          SELECT 1 FROM pg_constraint
          WHERE conname = 'realtime_rooms_name_source_account_id_key'
        ) THEN
          ALTER TABLE realtime_rooms ADD CONSTRAINT realtime_rooms_name_source_account_id_key UNIQUE (name, source_account_id);
        END IF;
      END $$;

      -- Presence: (user_id) -> (user_id, source_account_id)
      DO $$ BEGIN
        IF EXISTS (
          SELECT 1 FROM pg_constraint
          WHERE conname = 'realtime_presence_user_id_key'
        ) THEN
          ALTER TABLE realtime_presence DROP CONSTRAINT realtime_presence_user_id_key;
        END IF;
      END $$;

      DO $$ BEGIN
        IF NOT EXISTS (
          SELECT 1 FROM pg_constraint
          WHERE conname = 'realtime_presence_user_id_source_account_id_key'
        ) THEN
          ALTER TABLE realtime_presence ADD CONSTRAINT realtime_presence_user_id_source_account_id_key UNIQUE (user_id, source_account_id);
        END IF;
      END $$;
    `;

    await this.query(schema);
  }

  // -------------------------------------------------------------------------
  // Connections
  // -------------------------------------------------------------------------

  async createConnection(data: {
    socketId: string;
    userId?: string;
    sessionId?: string;
    transport: 'websocket' | 'polling';
    ipAddress?: string;
    userAgent?: string;
    deviceInfo?: DeviceInfo;
  }): Promise<Connection> {
    const result = await this.query<Connection>(
      `INSERT INTO realtime_connections (
        source_account_id, socket_id, user_id, session_id, status, transport,
        ip_address, user_agent, device_info
      ) VALUES ($1, $2, $3, $4, 'connected', $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.socketId,
        data.userId || null,
        data.sessionId || null,
        data.transport,
        data.ipAddress || null,
        data.userAgent || null,
        data.deviceInfo ? JSON.stringify(data.deviceInfo) : null,
      ]
    );
    return result[0];
  }

  async updateConnection(socketId: string, data: Partial<Connection>): Promise<void> {
    const updates: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    if (data.status) {
      updates.push(`status = $${paramIndex++}`);
      values.push(data.status);
    }
    if (data.latency_ms !== undefined) {
      updates.push(`latency_ms = $${paramIndex++}`);
      values.push(data.latency_ms);
    }
    if (data.user_id !== undefined) {
      updates.push(`user_id = $${paramIndex++}`);
      values.push(data.user_id);
    }

    if (updates.length === 0) return;

    values.push(socketId);
    values.push(this.sourceAccountId);
    await this.query(
      `UPDATE realtime_connections SET ${updates.join(', ')} WHERE socket_id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      values
    );
  }

  async disconnectConnection(socketId: string): Promise<void> {
    await this.query(
      `UPDATE realtime_connections
       SET status = 'disconnected', disconnected_at = NOW()
       WHERE socket_id = $1 AND source_account_id = $2`,
      [socketId, this.sourceAccountId]
    );
  }

  async updatePing(socketId: string): Promise<void> {
    await this.query(
      `UPDATE realtime_connections SET last_ping = NOW() WHERE socket_id = $1 AND source_account_id = $2`,
      [socketId, this.sourceAccountId]
    );
  }

  async updatePong(socketId: string, latencyMs: number): Promise<void> {
    await this.query(
      `UPDATE realtime_connections
       SET last_pong = NOW(), latency_ms = $2
       WHERE socket_id = $1 AND source_account_id = $3`,
      [socketId, latencyMs, this.sourceAccountId]
    );
  }

  async getActiveConnections(): Promise<Connection[]> {
    return this.query<Connection>(
      `SELECT * FROM realtime_connections WHERE status = 'connected' AND source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  async getConnectionCount(): Promise<number> {
    const result = await this.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM realtime_connections WHERE status = 'connected' AND source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result?.count || '0', 10);
  }

  // -------------------------------------------------------------------------
  // Rooms
  // -------------------------------------------------------------------------

  async createRoom(data: {
    name: string;
    type?: string;
    visibility?: string;
    maxMembers?: number;
  }): Promise<Room> {
    const result = await this.query<Room>(
      `INSERT INTO realtime_rooms (source_account_id, name, type, visibility, max_members)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (name, source_account_id) DO UPDATE SET updated_at = NOW()
       RETURNING *`,
      [this.sourceAccountId, data.name, data.type || 'channel', data.visibility || 'public', data.maxMembers || null]
    );
    return result[0];
  }

  async getRoomByName(name: string): Promise<Room | null> {
    return this.queryOne<Room>(
      `SELECT * FROM realtime_rooms WHERE name = $1 AND is_active = TRUE AND source_account_id = $2`,
      [name, this.sourceAccountId]
    );
  }

  async getRoomById(id: string): Promise<Room | null> {
    return this.queryOne<Room>(
      `SELECT * FROM realtime_rooms WHERE id = $1 AND is_active = TRUE AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getAllRooms(): Promise<Room[]> {
    return this.query<Room>(
      `SELECT * FROM realtime_rooms WHERE is_active = TRUE AND source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  async deleteRoom(name: string): Promise<void> {
    await this.query(
      `UPDATE realtime_rooms SET is_active = FALSE WHERE name = $1 AND source_account_id = $2`,
      [name, this.sourceAccountId]
    );
  }

  // -------------------------------------------------------------------------
  // Room Members
  // -------------------------------------------------------------------------

  async addRoomMember(roomId: string, userId: string, role: string = 'member'): Promise<void> {
    await this.query(
      `INSERT INTO realtime_room_members (source_account_id, room_id, user_id, role)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (room_id, user_id) DO UPDATE SET last_seen = NOW()`,
      [this.sourceAccountId, roomId, userId, role]
    );
  }

  async removeRoomMember(roomId: string, userId: string): Promise<void> {
    await this.query(
      `DELETE FROM realtime_room_members WHERE room_id = $1 AND user_id = $2 AND source_account_id = $3`,
      [roomId, userId, this.sourceAccountId]
    );
  }

  async getRoomMembers(roomId: string): Promise<RoomMember[]> {
    return this.query<RoomMember>(
      `SELECT * FROM realtime_room_members WHERE room_id = $1 AND source_account_id = $2`,
      [roomId, this.sourceAccountId]
    );
  }

  async getUserRooms(userId: string): Promise<Room[]> {
    return this.query<Room>(
      `SELECT r.* FROM realtime_rooms r
       JOIN realtime_room_members rm ON r.id = rm.room_id
       WHERE rm.user_id = $1 AND r.is_active = TRUE
         AND r.source_account_id = $2 AND rm.source_account_id = $2`,
      [userId, this.sourceAccountId]
    );
  }

  async updateMemberLastSeen(roomId: string, userId: string): Promise<void> {
    await this.query(
      `UPDATE realtime_room_members SET last_seen = NOW()
       WHERE room_id = $1 AND user_id = $2 AND source_account_id = $3`,
      [roomId, userId, this.sourceAccountId]
    );
  }

  // -------------------------------------------------------------------------
  // Presence
  // -------------------------------------------------------------------------

  async upsertPresence(
    userId: string,
    status: 'online' | 'away' | 'busy' | 'offline',
    customStatus?: { text: string; emoji?: string; expiresAt?: Date }
  ): Promise<Presence> {
    const result = await this.query<Presence>(
      `INSERT INTO realtime_presence (
        source_account_id, user_id, status, custom_status, custom_emoji, expires_at, last_heartbeat
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW())
      ON CONFLICT (user_id, source_account_id) DO UPDATE SET
        status = $3,
        custom_status = $4,
        custom_emoji = $5,
        expires_at = $6,
        last_heartbeat = NOW(),
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        userId,
        status,
        customStatus?.text || null,
        customStatus?.emoji || null,
        customStatus?.expiresAt || null,
      ]
    );
    return result[0];
  }

  async updatePresenceHeartbeat(userId: string): Promise<void> {
    await this.query(
      `UPDATE realtime_presence
       SET last_heartbeat = NOW(), last_active = NOW()
       WHERE user_id = $1 AND source_account_id = $2`,
      [userId, this.sourceAccountId]
    );
  }

  async incrementConnectionCount(userId: string): Promise<void> {
    await this.query(
      `UPDATE realtime_presence
       SET connections_count = connections_count + 1, status = 'online'
       WHERE user_id = $1 AND source_account_id = $2`,
      [userId, this.sourceAccountId]
    );
  }

  async decrementConnectionCount(userId: string): Promise<void> {
    await this.query(
      `UPDATE realtime_presence
       SET connections_count = GREATEST(connections_count - 1, 0)
       WHERE user_id = $1 AND source_account_id = $2`,
      [userId, this.sourceAccountId]
    );
  }

  async getPresence(userId: string): Promise<Presence | null> {
    return this.queryOne<Presence>(
      `SELECT * FROM realtime_presence WHERE user_id = $1 AND source_account_id = $2`,
      [userId, this.sourceAccountId]
    );
  }

  async getAllPresence(): Promise<Presence[]> {
    return this.query<Presence>(
      `SELECT * FROM realtime_presence WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  // -------------------------------------------------------------------------
  // Typing Indicators
  // -------------------------------------------------------------------------

  async setTyping(roomId: string, userId: string, threadId?: string): Promise<void> {
    const expiresAt = new Date(Date.now() + 3000); // 3 seconds
    await this.query(
      `INSERT INTO realtime_typing (source_account_id, room_id, user_id, thread_id, expires_at)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (room_id, user_id, thread_id) DO UPDATE
       SET started_at = NOW(), expires_at = $5`,
      [this.sourceAccountId, roomId, userId, threadId || null, expiresAt]
    );
  }

  async clearTyping(roomId: string, userId: string, threadId?: string): Promise<void> {
    await this.query(
      `DELETE FROM realtime_typing
       WHERE room_id = $1 AND user_id = $2 AND thread_id IS NOT DISTINCT FROM $3
         AND source_account_id = $4`,
      [roomId, userId, threadId || null, this.sourceAccountId]
    );
  }

  async getTypingUsers(roomId: string, threadId?: string): Promise<TypingIndicator[]> {
    return this.query<TypingIndicator>(
      `SELECT * FROM realtime_typing
       WHERE room_id = $1 AND thread_id IS NOT DISTINCT FROM $2 AND expires_at > NOW()
         AND source_account_id = $3`,
      [roomId, threadId || null, this.sourceAccountId]
    );
  }

  async cleanExpiredTyping(): Promise<void> {
    await this.query(`DELETE FROM realtime_typing WHERE expires_at < NOW()`);
  }

  // -------------------------------------------------------------------------
  // Events
  // -------------------------------------------------------------------------

  async logEvent(data: {
    eventType: string;
    socketId?: string;
    userId?: string;
    roomId?: string;
    payload?: unknown;
    ipAddress?: string;
  }): Promise<void> {
    await this.query(
      `INSERT INTO realtime_events (
        source_account_id, event_type, socket_id, user_id, room_id, payload, ip_address
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
      [
        this.sourceAccountId,
        data.eventType,
        data.socketId || null,
        data.userId || null,
        data.roomId || null,
        data.payload ? JSON.stringify(data.payload) : null,
        data.ipAddress || null,
      ]
    );
  }

  async getRecentEvents(limit: number = 100): Promise<RealtimeEvent[]> {
    return this.query<RealtimeEvent>(
      `SELECT * FROM realtime_events WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2`,
      [this.sourceAccountId, limit]
    );
  }

  // -------------------------------------------------------------------------
  // Statistics
  // -------------------------------------------------------------------------

  async getStats(): Promise<{
    connections: number;
    authenticatedConnections: number;
    rooms: number;
    presence: { online: number; away: number; busy: number; offline: number };
    eventsLastHour: number;
  }> {
    const [connections, authenticated, rooms, presence, events] = await Promise.all([
      this.queryOne<{ count: string }>(
        `SELECT COUNT(*) as count FROM realtime_connections WHERE status = 'connected' AND source_account_id = $1`,
        [this.sourceAccountId]
      ),
      this.queryOne<{ count: string }>(
        `SELECT COUNT(*) as count FROM realtime_connections
         WHERE status = 'connected' AND user_id IS NOT NULL AND source_account_id = $1`,
        [this.sourceAccountId]
      ),
      this.queryOne<{ count: string }>(
        `SELECT COUNT(*) as count FROM realtime_rooms WHERE is_active = TRUE AND source_account_id = $1`,
        [this.sourceAccountId]
      ),
      this.query<{ status: string; count: string }>(
        `SELECT status, COUNT(*) as count FROM realtime_presence WHERE source_account_id = $1 GROUP BY status`,
        [this.sourceAccountId]
      ),
      this.queryOne<{ count: string }>(
        `SELECT COUNT(*) as count FROM realtime_events
         WHERE created_at > NOW() - INTERVAL '1 hour' AND source_account_id = $1`,
        [this.sourceAccountId]
      ),
    ]);

    const presenceMap = presence.reduce((acc, p) => {
      acc[p.status as keyof typeof acc] = parseInt(p.count, 10);
      return acc;
    }, { online: 0, away: 0, busy: 0, offline: 0 });

    return {
      connections: parseInt(connections?.count || '0', 10),
      authenticatedConnections: parseInt(authenticated?.count || '0', 10),
      rooms: parseInt(rooms?.count || '0', 10),
      presence: presenceMap,
      eventsLastHour: parseInt(events?.count || '0', 10),
    };
  }

  // -------------------------------------------------------------------------
  // Multi-App Cleanup
  // -------------------------------------------------------------------------

  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    const tables = [
      'realtime_events',
      'realtime_typing',
      'realtime_room_members',
      'realtime_presence',
      'realtime_rooms',
      'realtime_connections',
    ];

    let total = 0;
    for (const table of tables) {
      const result = await this.queryOne<{ count: string }>(
        `WITH deleted AS (DELETE FROM ${table} WHERE source_account_id = $1 RETURNING 1)
         SELECT COUNT(*) as count FROM deleted`,
        [sourceAccountId]
      );
      total += parseInt(result?.count || '0', 10);
    }
    return total;
  }
}
