/**
 * Content Progress Database Operations
 * Complete CRUD operations for progress tracking in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ProgressPositionRecord,
  ProgressHistoryRecord,
  WatchlistRecord,
  FavoriteRecord,
  WebhookEventRecord,
  UpdateProgressRequest,
  CreateHistoryRequest,
  AddToWatchlistRequest,
  UpdateWatchlistRequest,
  AddToFavoritesRequest,
  UserStats,
  PluginStats,
  ContinueWatchingItem,
  RecentlyWatchedItem,
  ContentType,
} from './types.js';

const logger = createLogger('progress:db');

export class ProgressDatabase {
  private db: Database;
  private readonly sourceAccountId: string;
  private readonly completeThreshold: number;
  private readonly historySampleSeconds: number;
  private lastHistoryInsert: Map<string, number> = new Map();

  constructor(db?: Database, sourceAccountId = 'primary', completeThreshold = 95, historySampleSeconds = 30) {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
    this.completeThreshold = completeThreshold;
    this.historySampleSeconds = historySampleSeconds;
  }

  forSourceAccount(sourceAccountId: string): ProgressDatabase {
    return new ProgressDatabase(this.db, sourceAccountId, this.completeThreshold, this.historySampleSeconds);
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
    logger.info('Initializing content progress schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Progress Positions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_progress_positions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(64) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        position_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
        duration_seconds DOUBLE PRECISION,
        progress_percent DOUBLE PRECISION DEFAULT 0,
        completed BOOLEAN DEFAULT FALSE,
        completed_at TIMESTAMP WITH TIME ZONE,
        device_id VARCHAR(255),
        audio_track VARCHAR(16),
        subtitle_track VARCHAR(16),
        quality VARCHAR(16),
        metadata JSONB DEFAULT '{}',
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, content_type, content_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_progress_positions_source_account
        ON np_progress_positions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_positions_user
        ON np_progress_positions(user_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_positions_content
        ON np_progress_positions(content_type, content_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_positions_completed
        ON np_progress_positions(completed);
      CREATE INDEX IF NOT EXISTS idx_np_progress_positions_updated
        ON np_progress_positions(updated_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_progress_positions_user_updated
        ON np_progress_positions(user_id, updated_at DESC);

      -- =====================================================================
      -- Progress History
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_progress_history (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(64) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        action VARCHAR(16) NOT NULL DEFAULT 'play',
        position_seconds DOUBLE PRECISION,
        device_id VARCHAR(255),
        session_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_progress_history_source_account
        ON np_progress_history(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_history_user
        ON np_progress_history(user_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_history_content
        ON np_progress_history(content_type, content_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_history_created
        ON np_progress_history(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_progress_history_user_created
        ON np_progress_history(user_id, created_at DESC);

      -- =====================================================================
      -- Watchlists
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_progress_watchlists (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(64) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        priority INTEGER DEFAULT 0,
        added_from VARCHAR(64),
        notes TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, content_type, content_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_source_account
        ON np_progress_watchlists(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_user
        ON np_progress_watchlists(user_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_priority
        ON np_progress_watchlists(priority DESC);
      CREATE INDEX IF NOT EXISTS idx_np_progress_watchlists_user_priority
        ON np_progress_watchlists(user_id, priority DESC);

      -- =====================================================================
      -- Favorites
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_progress_favorites (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(64) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, content_type, content_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_source_account
        ON np_progress_favorites(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_user
        ON np_progress_favorites(user_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_favorites_created
        ON np_progress_favorites(created_at DESC);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_progress_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT FALSE,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_source_account
        ON np_progress_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_type
        ON np_progress_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_processed
        ON np_progress_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_np_progress_webhook_events_created
        ON np_progress_webhook_events(created_at DESC);
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Progress Positions
  // =========================================================================

  async updateProgress(request: UpdateProgressRequest): Promise<ProgressPositionRecord> {
    const progressPercent = request.duration_seconds
      ? (request.position_seconds / request.duration_seconds) * 100
      : 0;

    const completed = progressPercent >= this.completeThreshold;
    const completedAt = completed ? new Date() : null;

    const result = await this.query<ProgressPositionRecord>(
      `INSERT INTO np_progress_positions (
        source_account_id, user_id, content_type, content_id,
        position_seconds, duration_seconds, progress_percent,
        completed, completed_at, device_id, audio_track, subtitle_track,
        quality, metadata, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
      ON CONFLICT (source_account_id, user_id, content_type, content_id)
      DO UPDATE SET
        position_seconds = EXCLUDED.position_seconds,
        duration_seconds = COALESCE(EXCLUDED.duration_seconds, np_progress_positions.duration_seconds),
        progress_percent = EXCLUDED.progress_percent,
        completed = EXCLUDED.completed,
        completed_at = CASE
          WHEN EXCLUDED.completed AND np_progress_positions.completed_at IS NULL
          THEN EXCLUDED.completed_at
          ELSE np_progress_positions.completed_at
        END,
        device_id = COALESCE(EXCLUDED.device_id, np_progress_positions.device_id),
        audio_track = COALESCE(EXCLUDED.audio_track, np_progress_positions.audio_track),
        subtitle_track = COALESCE(EXCLUDED.subtitle_track, np_progress_positions.subtitle_track),
        quality = COALESCE(EXCLUDED.quality, np_progress_positions.quality),
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        request.user_id,
        request.content_type,
        request.content_id,
        request.position_seconds,
        request.duration_seconds ?? null,
        progressPercent,
        completed,
        completedAt,
        request.device_id ?? null,
        request.audio_track ?? null,
        request.subtitle_track ?? null,
        request.quality ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    // Record history event (with sampling)
    await this.maybeSampleHistoryEvent({
      user_id: request.user_id,
      content_type: request.content_type,
      content_id: request.content_id,
      action: 'play',
      position_seconds: request.position_seconds,
      device_id: request.device_id,
    });

    return result.rows[0];
  }

  private async maybeSampleHistoryEvent(request: CreateHistoryRequest): Promise<void> {
    const key = `${request.user_id}:${request.content_type}:${request.content_id}`;
    const now = Date.now();
    const lastInsert = this.lastHistoryInsert.get(key) ?? 0;
    const elapsed = (now - lastInsert) / 1000;

    if (elapsed >= this.historySampleSeconds) {
      await this.insertHistoryEvent(request);
      this.lastHistoryInsert.set(key, now);
    }
  }

  async getProgress(userId: string, contentType: ContentType, contentId: string): Promise<ProgressPositionRecord | null> {
    const result = await this.query<ProgressPositionRecord>(
      `SELECT * FROM np_progress_positions
       WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
      [this.sourceAccountId, userId, contentType, contentId]
    );

    return result.rows[0] ?? null;
  }

  async getUserProgress(userId: string, limit = 100, offset = 0): Promise<ProgressPositionRecord[]> {
    const result = await this.query<ProgressPositionRecord>(
      `SELECT * FROM np_progress_positions
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY updated_at DESC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, userId, limit, offset]
    );

    return result.rows;
  }

  async deleteProgress(userId: string, contentType: ContentType, contentId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM np_progress_positions
       WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
      [this.sourceAccountId, userId, contentType, contentId]
    );

    return rowCount > 0;
  }

  async markCompleted(userId: string, contentType: ContentType, contentId: string): Promise<ProgressPositionRecord | null> {
    const result = await this.query<ProgressPositionRecord>(
      `UPDATE np_progress_positions
       SET completed = TRUE,
           completed_at = COALESCE(completed_at, NOW()),
           progress_percent = 100,
           updated_at = NOW()
       WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4
       RETURNING *`,
      [this.sourceAccountId, userId, contentType, contentId]
    );

    if (result.rows[0]) {
      await this.insertHistoryEvent({
        user_id: userId,
        content_type: contentType,
        content_id: contentId,
        action: 'complete',
      });
    }

    return result.rows[0] ?? null;
  }

  async getContinueWatching(userId: string, limit = 20): Promise<ContinueWatchingItem[]> {
    const result = await this.query<ContinueWatchingItem>(
      `SELECT id, source_account_id, user_id, content_type, content_id,
              position_seconds, duration_seconds, progress_percent, updated_at, metadata
       FROM np_progress_positions
       WHERE source_account_id = $1
         AND user_id = $2
         AND completed = FALSE
         AND progress_percent > 1
         AND progress_percent < $3
       ORDER BY updated_at DESC
       LIMIT $4`,
      [this.sourceAccountId, userId, this.completeThreshold, limit]
    );

    return result.rows;
  }

  async getRecentlyWatched(userId: string, limit = 50): Promise<RecentlyWatchedItem[]> {
    const result = await this.query<RecentlyWatchedItem>(
      `SELECT id, source_account_id, user_id, content_type, content_id,
              position_seconds, duration_seconds, progress_percent, completed,
              completed_at, updated_at, metadata
       FROM np_progress_positions
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY updated_at DESC
       LIMIT $3`,
      [this.sourceAccountId, userId, limit]
    );

    return result.rows;
  }

  // =========================================================================
  // Progress History
  // =========================================================================

  async insertHistoryEvent(request: CreateHistoryRequest): Promise<ProgressHistoryRecord> {
    const result = await this.query<ProgressHistoryRecord>(
      `INSERT INTO np_progress_history (
        source_account_id, user_id, content_type, content_id,
        action, position_seconds, device_id, session_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.user_id,
        request.content_type,
        request.content_id,
        request.action,
        request.position_seconds ?? null,
        request.device_id ?? null,
        request.session_id ?? null,
      ]
    );

    return result.rows[0];
  }

  async getUserHistory(userId: string, limit = 100, offset = 0): Promise<ProgressHistoryRecord[]> {
    const result = await this.query<ProgressHistoryRecord>(
      `SELECT * FROM np_progress_history
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY created_at DESC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, userId, limit, offset]
    );

    return result.rows;
  }

  // =========================================================================
  // Watchlist
  // =========================================================================

  async addToWatchlist(request: AddToWatchlistRequest): Promise<WatchlistRecord> {
    const result = await this.query<WatchlistRecord>(
      `INSERT INTO np_progress_watchlists (
        source_account_id, user_id, content_type, content_id,
        priority, added_from, notes
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      ON CONFLICT (source_account_id, user_id, content_type, content_id)
      DO UPDATE SET
        priority = EXCLUDED.priority,
        notes = EXCLUDED.notes
      RETURNING *`,
      [
        this.sourceAccountId,
        request.user_id,
        request.content_type,
        request.content_id,
        request.priority ?? 0,
        request.added_from ?? null,
        request.notes ?? null,
      ]
    );

    return result.rows[0];
  }

  async getWatchlist(userId: string, limit = 100, offset = 0): Promise<WatchlistRecord[]> {
    const result = await this.query<WatchlistRecord>(
      `SELECT * FROM np_progress_watchlists
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY priority DESC, created_at DESC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, userId, limit, offset]
    );

    return result.rows;
  }

  async updateWatchlistItem(
    userId: string,
    contentType: ContentType,
    contentId: string,
    updates: UpdateWatchlistRequest
  ): Promise<WatchlistRecord | null> {
    const setParts: string[] = [];
    const params: unknown[] = [this.sourceAccountId, userId, contentType, contentId];

    if (updates.priority !== undefined) {
      setParts.push(`priority = $${params.length + 1}`);
      params.push(updates.priority);
    }

    if (updates.notes !== undefined) {
      setParts.push(`notes = $${params.length + 1}`);
      params.push(updates.notes);
    }

    if (setParts.length === 0) {
      return null;
    }

    const result = await this.query<WatchlistRecord>(
      `UPDATE np_progress_watchlists
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async removeFromWatchlist(userId: string, contentType: ContentType, contentId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM np_progress_watchlists
       WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
      [this.sourceAccountId, userId, contentType, contentId]
    );

    return rowCount > 0;
  }

  // =========================================================================
  // Favorites
  // =========================================================================

  async addToFavorites(request: AddToFavoritesRequest): Promise<FavoriteRecord> {
    const result = await this.query<FavoriteRecord>(
      `INSERT INTO np_progress_favorites (
        source_account_id, user_id, content_type, content_id
      ) VALUES ($1, $2, $3, $4)
      ON CONFLICT (source_account_id, user_id, content_type, content_id)
      DO NOTHING
      RETURNING *`,
      [this.sourceAccountId, request.user_id, request.content_type, request.content_id]
    );

    return result.rows[0];
  }

  async getFavorites(userId: string, limit = 100, offset = 0): Promise<FavoriteRecord[]> {
    const result = await this.query<FavoriteRecord>(
      `SELECT * FROM np_progress_favorites
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY created_at DESC
       LIMIT $3 OFFSET $4`,
      [this.sourceAccountId, userId, limit, offset]
    );

    return result.rows;
  }

  async removeFromFavorites(userId: string, contentType: ContentType, contentId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM np_progress_favorites
       WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4`,
      [this.sourceAccountId, userId, contentType, contentId]
    );

    return rowCount > 0;
  }

  async isFavorite(userId: string, contentType: ContentType, contentId: string): Promise<boolean> {
    const result = await this.query<{ exists: boolean }>(
      `SELECT EXISTS(
        SELECT 1 FROM np_progress_favorites
        WHERE source_account_id = $1 AND user_id = $2 AND content_type = $3 AND content_id = $4
      ) as exists`,
      [this.sourceAccountId, userId, contentType, contentId]
    );

    return result.rows[0]?.exists ?? false;
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(id: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO np_progress_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (id) DO NOTHING`,
      [id, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE np_progress_webhook_events
       SET processed = TRUE, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [id, error ?? null]
    );
  }

  async listWebhookEvents(eventType?: string, limit = 100, offset = 0): Promise<WebhookEventRecord[]> {
    if (eventType) {
      const result = await this.query<WebhookEventRecord>(
        `SELECT * FROM np_progress_webhook_events
         WHERE source_account_id = $1 AND event_type = $2
         ORDER BY created_at DESC
         LIMIT $3 OFFSET $4`,
        [this.sourceAccountId, eventType, limit, offset]
      );
      return result.rows;
    }

    const result = await this.query<WebhookEventRecord>(
      `SELECT * FROM np_progress_webhook_events
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getUserStats(userId: string): Promise<UserStats> {
    const result = await this.query<{
      total_watch_time_seconds: number;
      content_completed: number;
      content_in_progress: number;
      watchlist_count: number;
      favorites_count: number;
      most_watched_type: ContentType | null;
      recent_activity: Date | null;
    }>(
      `WITH watch_time AS (
        SELECT COALESCE(SUM(position_seconds), 0) as total_seconds
        FROM np_progress_positions
        WHERE source_account_id = $1 AND user_id = $2
      ),
      counts AS (
        SELECT
          COUNT(*) FILTER (WHERE completed = TRUE) as completed,
          COUNT(*) FILTER (WHERE completed = FALSE AND progress_percent > 1) as in_progress
        FROM np_progress_positions
        WHERE source_account_id = $1 AND user_id = $2
      ),
      watchlist AS (
        SELECT COUNT(*) as count
        FROM np_progress_watchlists
        WHERE source_account_id = $1 AND user_id = $2
      ),
      favorites AS (
        SELECT COUNT(*) as count
        FROM np_progress_favorites
        WHERE source_account_id = $1 AND user_id = $2
      ),
      most_watched AS (
        SELECT content_type
        FROM np_progress_positions
        WHERE source_account_id = $1 AND user_id = $2
        GROUP BY content_type
        ORDER BY COUNT(*) DESC
        LIMIT 1
      ),
      recent AS (
        SELECT MAX(updated_at) as last_activity
        FROM np_progress_positions
        WHERE source_account_id = $1 AND user_id = $2
      )
      SELECT
        w.total_seconds as total_watch_time_seconds,
        c.completed as content_completed,
        c.in_progress as content_in_progress,
        wl.count as watchlist_count,
        f.count as favorites_count,
        mw.content_type as most_watched_type,
        r.last_activity as recent_activity
      FROM watch_time w
      CROSS JOIN counts c
      CROSS JOIN watchlist wl
      CROSS JOIN favorites f
      LEFT JOIN most_watched mw ON TRUE
      LEFT JOIN recent r ON TRUE`,
      [this.sourceAccountId, userId]
    );

    const row = result.rows[0];
    return {
      total_watch_time_seconds: row?.total_watch_time_seconds ?? 0,
      total_watch_time_hours: (row?.total_watch_time_seconds ?? 0) / 3600,
      content_completed: row?.content_completed ?? 0,
      content_in_progress: row?.content_in_progress ?? 0,
      watchlist_count: row?.watchlist_count ?? 0,
      favorites_count: row?.favorites_count ?? 0,
      most_watched_type: row?.most_watched_type ?? null,
      recent_activity: row?.recent_activity ?? null,
    };
  }

  async getPluginStats(): Promise<PluginStats> {
    const result = await this.query<{
      total_users: number;
      total_positions: number;
      total_completed: number;
      total_in_progress: number;
      total_watchlist: number;
      total_favorites: number;
      total_history_events: number;
      last_activity: Date | null;
    }>(
      `WITH users AS (
        SELECT COUNT(DISTINCT user_id) as count
        FROM np_progress_positions
        WHERE source_account_id = $1
      ),
      positions AS (
        SELECT
          COUNT(*) as total,
          COUNT(*) FILTER (WHERE completed = TRUE) as completed,
          COUNT(*) FILTER (WHERE completed = FALSE AND progress_percent > 1) as in_progress,
          MAX(updated_at) as last_activity
        FROM np_progress_positions
        WHERE source_account_id = $1
      ),
      watchlist AS (
        SELECT COUNT(*) as count
        FROM np_progress_watchlists
        WHERE source_account_id = $1
      ),
      favorites AS (
        SELECT COUNT(*) as count
        FROM np_progress_favorites
        WHERE source_account_id = $1
      ),
      history AS (
        SELECT COUNT(*) as count
        FROM np_progress_history
        WHERE source_account_id = $1
      )
      SELECT
        u.count as total_users,
        p.total as total_positions,
        p.completed as total_completed,
        p.in_progress as total_in_progress,
        w.count as total_watchlist,
        f.count as total_favorites,
        h.count as total_history_events,
        p.last_activity
      FROM users u
      CROSS JOIN positions p
      CROSS JOIN watchlist w
      CROSS JOIN favorites f
      CROSS JOIN history h`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_users: row?.total_users ?? 0,
      total_positions: row?.total_positions ?? 0,
      total_completed: row?.total_completed ?? 0,
      total_in_progress: row?.total_in_progress ?? 0,
      total_watchlist: row?.total_watchlist ?? 0,
      total_favorites: row?.total_favorites ?? 0,
      total_history_events: row?.total_history_events ?? 0,
      last_activity: row?.last_activity ?? null,
    };
  }
}
