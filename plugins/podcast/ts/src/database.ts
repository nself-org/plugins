/**
 * Podcast Database Operations
 * Complete CRUD operations for podcasts, episodes, subscriptions, playback positions, and categories
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  PodcastRecord,
  EpisodeRecord,
  SubscriptionRecord,
  PlaybackPositionRecord,
  CategoryRecord,
  PodcastWithStats,
  EpisodeWithPodcast,
  SubscriptionWithPodcast,
  PodcastStats,
  SyncResult,
} from './types.js';

const logger = createLogger('podcast:db');

export class PodcastDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): PodcastDatabase {
    return new PodcastDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing Podcast schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Categories
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_podcast_categories (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        slug VARCHAR(255) NOT NULL,
        parent_id UUID REFERENCES np_podcast_categories(id) ON DELETE SET NULL,
        podcast_count INTEGER DEFAULT 0,
        sort_order INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, slug)
      );

      CREATE INDEX IF NOT EXISTS idx_np_podcast_categories_source_app
        ON np_podcast_categories(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_categories_parent
        ON np_podcast_categories(parent_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_categories_slug
        ON np_podcast_categories(source_account_id, slug);

      -- =====================================================================
      -- Podcasts
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_podcast_podcasts (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        title VARCHAR(500) NOT NULL,
        description TEXT,
        author VARCHAR(255),
        feed_url TEXT NOT NULL,
        website_url TEXT,
        image_url TEXT,
        language VARCHAR(10) DEFAULT 'en',
        categories TEXT[] DEFAULT '{}',
        explicit BOOLEAN DEFAULT false,
        last_fetched_at TIMESTAMPTZ,
        last_published_at TIMESTAMPTZ,
        etag VARCHAR(255),
        last_modified VARCHAR(255),
        feed_status VARCHAR(20) DEFAULT 'active',
        episode_count INTEGER DEFAULT 0,
        search_vector tsvector,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, feed_url)
      );

      CREATE INDEX IF NOT EXISTS idx_np_podcast_podcasts_source_app
        ON np_podcast_podcasts(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_podcasts_feed_url
        ON np_podcast_podcasts(source_account_id, feed_url);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_podcasts_status
        ON np_podcast_podcasts(source_account_id, feed_status);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_podcasts_search
        ON np_podcast_podcasts USING GIN(search_vector);

      -- =====================================================================
      -- Episodes
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_podcast_episodes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        podcast_id UUID NOT NULL REFERENCES np_podcast_podcasts(id) ON DELETE CASCADE,
        guid VARCHAR(1000) NOT NULL,
        title VARCHAR(500) NOT NULL,
        description TEXT,
        content_html TEXT,
        published_at TIMESTAMPTZ,
        duration_seconds INTEGER,
        audio_url TEXT NOT NULL,
        audio_type VARCHAR(50),
        audio_size_bytes BIGINT,
        image_url TEXT,
        season_number INTEGER,
        episode_number INTEGER,
        episode_type VARCHAR(20) DEFAULT 'full',
        explicit BOOLEAN DEFAULT false,
        transcript_url TEXT,
        chapters_url TEXT,
        search_vector tsvector,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, podcast_id, guid)
      );

      CREATE INDEX IF NOT EXISTS idx_np_podcast_episodes_source_app
        ON np_podcast_episodes(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_episodes_podcast
        ON np_podcast_episodes(podcast_id, published_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_episodes_guid
        ON np_podcast_episodes(source_account_id, guid);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_episodes_published
        ON np_podcast_episodes(source_account_id, published_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_episodes_search
        ON np_podcast_episodes USING GIN(search_vector);

      -- =====================================================================
      -- Subscriptions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_podcast_subscriptions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id TEXT NOT NULL,
        podcast_id UUID NOT NULL REFERENCES np_podcast_podcasts(id) ON DELETE CASCADE,
        subscribed_at TIMESTAMPTZ DEFAULT NOW(),
        is_active BOOLEAN DEFAULT true,
        notification_enabled BOOLEAN DEFAULT true,
        auto_download BOOLEAN DEFAULT false,
        metadata JSONB DEFAULT '{}',
        UNIQUE(source_account_id, user_id, podcast_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_podcast_subscriptions_source_app
        ON np_podcast_subscriptions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_subscriptions_user
        ON np_podcast_subscriptions(source_account_id, user_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_subscriptions_podcast
        ON np_podcast_subscriptions(podcast_id);

      -- =====================================================================
      -- Playback Positions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_podcast_playback_positions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id TEXT NOT NULL,
        episode_id UUID NOT NULL REFERENCES np_podcast_episodes(id) ON DELETE CASCADE,
        position_seconds INTEGER NOT NULL DEFAULT 0,
        duration_seconds INTEGER,
        completed BOOLEAN DEFAULT false,
        completed_at TIMESTAMPTZ,
        device_id VARCHAR(255),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, episode_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_podcast_playback_source_app
        ON np_podcast_playback_positions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_playback_user
        ON np_podcast_playback_positions(source_account_id, user_id);
      CREATE INDEX IF NOT EXISTS idx_np_podcast_playback_episode
        ON np_podcast_playback_positions(episode_id);

      -- =====================================================================
      -- Search vector triggers
      -- =====================================================================

      CREATE OR REPLACE FUNCTION np_podcast_podcasts_search_update() RETURNS trigger AS $$
      BEGIN
        NEW.search_vector := to_tsvector('english', COALESCE(NEW.title, '') || ' ' || COALESCE(NEW.description, '') || ' ' || COALESCE(NEW.author, ''));
        RETURN NEW;
      END;
      $$ LANGUAGE plpgsql;

      DROP TRIGGER IF EXISTS trg_np_podcast_podcasts_search ON np_podcast_podcasts;
      CREATE TRIGGER trg_np_podcast_podcasts_search
        BEFORE INSERT OR UPDATE OF title, description, author ON np_podcast_podcasts
        FOR EACH ROW EXECUTE FUNCTION np_podcast_podcasts_search_update();

      CREATE OR REPLACE FUNCTION np_podcast_episodes_search_update() RETURNS trigger AS $$
      BEGIN
        NEW.search_vector := to_tsvector('english', COALESCE(NEW.title, '') || ' ' || COALESCE(NEW.description, ''));
        RETURN NEW;
      END;
      $$ LANGUAGE plpgsql;

      DROP TRIGGER IF EXISTS trg_np_podcast_episodes_search ON np_podcast_episodes;
      CREATE TRIGGER trg_np_podcast_episodes_search
        BEFORE INSERT OR UPDATE OF title, description ON np_podcast_episodes
        FOR EACH ROW EXECUTE FUNCTION np_podcast_episodes_search_update();
    `;

    await this.execute(schema);
    logger.info('Podcast schema initialized successfully');
  }

  // =========================================================================
  // Podcast Operations
  // =========================================================================

  async createPodcast(podcast: Omit<PodcastRecord, 'id' | 'created_at' | 'updated_at' | 'search_vector'>): Promise<PodcastRecord> {
    const result = await this.query<PodcastRecord>(
      `INSERT INTO np_podcast_podcasts (
        source_account_id, title, description, author, feed_url, website_url,
        image_url, language, categories, explicit, last_fetched_at,
        last_published_at, etag, last_modified, feed_status, episode_count, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
      RETURNING *`,
      [
        this.sourceAccountId, podcast.title, podcast.description, podcast.author,
        podcast.feed_url, podcast.website_url, podcast.image_url, podcast.language,
        podcast.categories, podcast.explicit, podcast.last_fetched_at,
        podcast.last_published_at, podcast.etag, podcast.last_modified,
        podcast.feed_status, podcast.episode_count, JSON.stringify(podcast.metadata),
      ]
    );

    return result.rows[0];
  }

  async getPodcast(id: string): Promise<PodcastRecord | null> {
    const result = await this.query<PodcastRecord>(
      `SELECT * FROM np_podcast_podcasts WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getPodcastByFeedUrl(feedUrl: string): Promise<PodcastRecord | null> {
    const result = await this.query<PodcastRecord>(
      `SELECT * FROM np_podcast_podcasts WHERE feed_url = $1 AND source_account_id = $2`,
      [feedUrl, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listPodcasts(filters: {
    category?: string; language?: string; feedStatus?: string;
    limit?: number; offset?: number;
  }): Promise<PodcastRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.category) {
      conditions.push(`$${paramIndex} = ANY(categories)`);
      values.push(filters.category);
      paramIndex++;
    }

    if (filters.language) {
      conditions.push(`language = $${paramIndex}`);
      values.push(filters.language);
      paramIndex++;
    }

    if (filters.feedStatus) {
      conditions.push(`feed_status = $${paramIndex}`);
      values.push(filters.feedStatus);
      paramIndex++;
    }

    let sql = `
      SELECT * FROM np_podcast_podcasts
      WHERE ${conditions.join(' AND ')}
      ORDER BY title ASC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<PodcastRecord>(sql, values);
    return result.rows;
  }

  async updatePodcast(id: string, updates: Partial<PodcastRecord>): Promise<PodcastRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'title', 'description', 'author', 'website_url', 'image_url',
      'language', 'categories', 'explicit', 'last_fetched_at',
      'last_published_at', 'etag', 'last_modified', 'feed_status', 'episode_count',
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
      return this.getPodcast(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<PodcastRecord>(
      `UPDATE np_podcast_podcasts
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deletePodcast(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_podcast_podcasts WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async searchPodcasts(filters: {
    query: string; category?: string; language?: string; limit?: number;
  }): Promise<PodcastRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.query) {
      conditions.push(`search_vector @@ plainto_tsquery('english', $${paramIndex})`);
      values.push(filters.query);
      paramIndex++;
    }

    if (filters.category) {
      conditions.push(`$${paramIndex} = ANY(categories)`);
      values.push(filters.category);
      paramIndex++;
    }

    if (filters.language) {
      conditions.push(`language = $${paramIndex}`);
      values.push(filters.language);
      paramIndex++;
    }

    const limit = filters.limit ?? 50;

    const result = await this.query<PodcastRecord>(
      `SELECT * FROM np_podcast_podcasts
       WHERE ${conditions.join(' AND ')}
       ORDER BY ts_rank(search_vector, plainto_tsquery('english', $2)) DESC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  // =========================================================================
  // Episode Operations
  // =========================================================================

  async createEpisode(episode: Omit<EpisodeRecord, 'id' | 'created_at' | 'updated_at' | 'search_vector'>): Promise<EpisodeRecord> {
    const result = await this.query<EpisodeRecord>(
      `INSERT INTO np_podcast_episodes (
        source_account_id, podcast_id, guid, title, description, content_html,
        published_at, duration_seconds, audio_url, audio_type, audio_size_bytes,
        image_url, season_number, episode_number, episode_type, explicit,
        transcript_url, chapters_url, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
      ON CONFLICT (source_account_id, podcast_id, guid) DO UPDATE SET
        title = EXCLUDED.title,
        description = EXCLUDED.description,
        content_html = EXCLUDED.content_html,
        published_at = EXCLUDED.published_at,
        duration_seconds = EXCLUDED.duration_seconds,
        audio_url = EXCLUDED.audio_url,
        audio_type = EXCLUDED.audio_type,
        audio_size_bytes = EXCLUDED.audio_size_bytes,
        image_url = EXCLUDED.image_url,
        season_number = EXCLUDED.season_number,
        episode_number = EXCLUDED.episode_number,
        episode_type = EXCLUDED.episode_type,
        explicit = EXCLUDED.explicit,
        transcript_url = EXCLUDED.transcript_url,
        chapters_url = EXCLUDED.chapters_url,
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, episode.podcast_id, episode.guid, episode.title,
        episode.description, episode.content_html, episode.published_at,
        episode.duration_seconds, episode.audio_url, episode.audio_type,
        episode.audio_size_bytes, episode.image_url, episode.season_number,
        episode.episode_number, episode.episode_type, episode.explicit,
        episode.transcript_url, episode.chapters_url, JSON.stringify(episode.metadata),
      ]
    );

    return result.rows[0];
  }

  async getEpisode(id: string): Promise<EpisodeRecord | null> {
    const result = await this.query<EpisodeRecord>(
      `SELECT * FROM np_podcast_episodes WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listEpisodes(filters: {
    podcastId?: string; seasonNumber?: number; episodeType?: string;
    limit?: number; offset?: number;
  }): Promise<EpisodeRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.podcastId) {
      conditions.push(`podcast_id = $${paramIndex}`);
      values.push(filters.podcastId);
      paramIndex++;
    }

    if (filters.seasonNumber !== undefined) {
      conditions.push(`season_number = $${paramIndex}`);
      values.push(filters.seasonNumber);
      paramIndex++;
    }

    if (filters.episodeType) {
      conditions.push(`episode_type = $${paramIndex}`);
      values.push(filters.episodeType);
      paramIndex++;
    }

    let sql = `
      SELECT * FROM np_podcast_episodes
      WHERE ${conditions.join(' AND ')}
      ORDER BY published_at DESC NULLS LAST
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<EpisodeRecord>(sql, values);
    return result.rows;
  }

  async searchEpisodes(filters: {
    query: string; podcastId?: string; limit?: number;
  }): Promise<EpisodeRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.query) {
      conditions.push(`search_vector @@ plainto_tsquery('english', $${paramIndex})`);
      values.push(filters.query);
      paramIndex++;
    }

    if (filters.podcastId) {
      conditions.push(`podcast_id = $${paramIndex}`);
      values.push(filters.podcastId);
      paramIndex++;
    }

    const limit = filters.limit ?? 50;

    const result = await this.query<EpisodeRecord>(
      `SELECT * FROM np_podcast_episodes
       WHERE ${conditions.join(' AND ')}
       ORDER BY ts_rank(search_vector, plainto_tsquery('english', $2)) DESC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  async deleteEpisode(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_podcast_episodes WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async getEpisodeCountForPodcast(podcastId: string): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_podcast_episodes WHERE podcast_id = $1 AND source_account_id = $2`,
      [podcastId, this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Subscription Operations
  // =========================================================================

  async createSubscription(sub: Omit<SubscriptionRecord, 'id' | 'subscribed_at'>): Promise<SubscriptionRecord> {
    const result = await this.query<SubscriptionRecord>(
      `INSERT INTO np_podcast_subscriptions (
        source_account_id, user_id, podcast_id, is_active,
        notification_enabled, auto_download, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      ON CONFLICT (source_account_id, user_id, podcast_id) DO UPDATE SET
        is_active = EXCLUDED.is_active,
        notification_enabled = EXCLUDED.notification_enabled,
        auto_download = EXCLUDED.auto_download
      RETURNING *`,
      [
        this.sourceAccountId, sub.user_id, sub.podcast_id, sub.is_active,
        sub.notification_enabled, sub.auto_download, JSON.stringify(sub.metadata),
      ]
    );

    return result.rows[0];
  }

  async getSubscription(id: string): Promise<SubscriptionRecord | null> {
    const result = await this.query<SubscriptionRecord>(
      `SELECT * FROM np_podcast_subscriptions WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listUserSubscriptions(userId: string): Promise<SubscriptionWithPodcast[]> {
    const result = await this.query<SubscriptionWithPodcast>(
      `SELECT
        s.id as subscription_id,
        s.user_id,
        s.podcast_id,
        p.title as podcast_title,
        p.image_url as podcast_image_url,
        p.author as podcast_author,
        s.subscribed_at,
        s.is_active,
        s.notification_enabled,
        s.auto_download,
        COALESCE(
          (SELECT COUNT(*) FROM np_podcast_episodes e
           LEFT JOIN np_podcast_playback_positions pp
             ON pp.episode_id = e.id AND pp.user_id = s.user_id AND pp.completed = true
           WHERE e.podcast_id = s.podcast_id AND e.source_account_id = $1
             AND pp.id IS NULL),
          0
        )::integer as unplayed_count
      FROM np_podcast_subscriptions s
      JOIN np_podcast_podcasts p ON s.podcast_id = p.id
      WHERE s.source_account_id = $1 AND s.user_id = $2 AND s.is_active = true
      ORDER BY p.title ASC`,
      [this.sourceAccountId, userId]
    );

    return result.rows;
  }

  async updateSubscription(id: string, updates: Partial<SubscriptionRecord>): Promise<SubscriptionRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = ['is_active', 'notification_enabled', 'auto_download'];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (fields.length === 0) {
      return this.getSubscription(id);
    }

    values.push(id, this.sourceAccountId);

    const result = await this.query<SubscriptionRecord>(
      `UPDATE np_podcast_subscriptions
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteSubscription(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_podcast_subscriptions WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Playback Position Operations
  // =========================================================================

  async upsertPlaybackPosition(position: Omit<PlaybackPositionRecord, 'id' | 'updated_at'>): Promise<PlaybackPositionRecord> {
    const result = await this.query<PlaybackPositionRecord>(
      `INSERT INTO np_podcast_playback_positions (
        source_account_id, user_id, episode_id, position_seconds,
        duration_seconds, completed, completed_at, device_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      ON CONFLICT (source_account_id, user_id, episode_id) DO UPDATE SET
        position_seconds = EXCLUDED.position_seconds,
        duration_seconds = COALESCE(EXCLUDED.duration_seconds, np_podcast_playback_positions.duration_seconds),
        completed = EXCLUDED.completed,
        completed_at = CASE WHEN EXCLUDED.completed = true AND np_podcast_playback_positions.completed = false
                            THEN NOW() ELSE np_podcast_playback_positions.completed_at END,
        device_id = EXCLUDED.device_id,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, position.user_id, position.episode_id,
        position.position_seconds, position.duration_seconds,
        position.completed, position.completed_at, position.device_id,
      ]
    );

    return result.rows[0];
  }

  async getPlaybackPosition(userId: string, episodeId: string): Promise<PlaybackPositionRecord | null> {
    const result = await this.query<PlaybackPositionRecord>(
      `SELECT * FROM np_podcast_playback_positions
       WHERE user_id = $1 AND episode_id = $2 AND source_account_id = $3`,
      [userId, episodeId, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getPlaybackPositions(userId: string, episodeIds: string[]): Promise<PlaybackPositionRecord[]> {
    if (episodeIds.length === 0) return [];

    const placeholders = episodeIds.map((_, i) => `$${i + 3}`).join(', ');
    const result = await this.query<PlaybackPositionRecord>(
      `SELECT * FROM np_podcast_playback_positions
       WHERE user_id = $1 AND source_account_id = $2
         AND episode_id IN (${placeholders})`,
      [userId, this.sourceAccountId, ...episodeIds]
    );

    return result.rows;
  }

  async getUserInProgressEpisodes(userId: string, limit = 50): Promise<EpisodeWithPodcast[]> {
    const result = await this.query<EpisodeWithPodcast>(
      `SELECT
        e.id as episode_id,
        e.title as episode_title,
        e.description,
        e.published_at,
        e.duration_seconds,
        e.audio_url,
        e.season_number,
        e.episode_number,
        p.id as podcast_id,
        p.title as podcast_title,
        p.image_url as podcast_image_url
      FROM np_podcast_playback_positions pp
      JOIN np_podcast_episodes e ON pp.episode_id = e.id
      JOIN np_podcast_podcasts p ON e.podcast_id = p.id
      WHERE pp.user_id = $1 AND pp.source_account_id = $2
        AND pp.completed = false AND pp.position_seconds > 0
      ORDER BY pp.updated_at DESC
      LIMIT $3`,
      [userId, this.sourceAccountId, limit]
    );

    return result.rows;
  }

  // =========================================================================
  // Category Operations
  // =========================================================================

  async createCategory(category: Omit<CategoryRecord, 'id' | 'created_at'>): Promise<CategoryRecord> {
    const result = await this.query<CategoryRecord>(
      `INSERT INTO np_podcast_categories (
        source_account_id, name, slug, parent_id, podcast_count, sort_order
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId, category.name, category.slug,
        category.parent_id, category.podcast_count, category.sort_order,
      ]
    );

    return result.rows[0];
  }

  async listCategories(): Promise<CategoryRecord[]> {
    const result = await this.query<CategoryRecord>(
      `SELECT * FROM np_podcast_categories
       WHERE source_account_id = $1
       ORDER BY sort_order ASC, name ASC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async deleteCategory(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_podcast_categories WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Sync Operations
  // =========================================================================

  async getPodcastsForSync(feedStatus?: string): Promise<PodcastRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (feedStatus) {
      conditions.push(`feed_status = $${paramIndex}`);
      values.push(feedStatus);
      paramIndex++;
    } else {
      conditions.push(`feed_status != 'error'`);
    }

    const result = await this.query<PodcastRecord>(
      `SELECT * FROM np_podcast_podcasts
       WHERE ${conditions.join(' AND ')}
       ORDER BY last_fetched_at ASC NULLS FIRST`,
      values
    );

    return result.rows;
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(): Promise<PodcastStats> {
    const result = await this.query<{
      total_podcasts: string;
      active_podcasts: string;
      total_episodes: string;
      total_subscriptions: string;
      total_categories: string;
      oldest_episode: Date | null;
      newest_episode: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_podcast_podcasts WHERE source_account_id = $1) as total_podcasts,
        (SELECT COUNT(*) FROM np_podcast_podcasts WHERE source_account_id = $1 AND feed_status = 'active') as active_podcasts,
        (SELECT COUNT(*) FROM np_podcast_episodes WHERE source_account_id = $1) as total_episodes,
        (SELECT COUNT(*) FROM np_podcast_subscriptions WHERE source_account_id = $1 AND is_active = true) as total_subscriptions,
        (SELECT COUNT(*) FROM np_podcast_categories WHERE source_account_id = $1) as total_categories,
        (SELECT MIN(published_at) FROM np_podcast_episodes WHERE source_account_id = $1) as oldest_episode,
        (SELECT MAX(published_at) FROM np_podcast_episodes WHERE source_account_id = $1) as newest_episode`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_podcasts: parseInt(row?.total_podcasts ?? '0', 10),
      active_podcasts: parseInt(row?.active_podcasts ?? '0', 10),
      total_episodes: parseInt(row?.total_episodes ?? '0', 10),
      total_subscriptions: parseInt(row?.total_subscriptions ?? '0', 10),
      total_categories: parseInt(row?.total_categories ?? '0', 10),
      oldest_episode: row?.oldest_episode ?? null,
      newest_episode: row?.newest_episode ?? null,
    };
  }
}
