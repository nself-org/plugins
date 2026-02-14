/**
 * Discovery Plugin Database Operations
 *
 * READ-ONLY access to: media_items, watch_progress, user_ratings
 * READ-WRITE access to: np_disc_trending_cache, np_disc_popular_cache
 */

import { Pool, QueryResult } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type {
  TrendingItem,
  PopularItem,
  RecentItem,
  ContinueWatchingItem,
  DiscoveryStatistics,
} from './types.js';

const logger = createLogger('discovery:database');

export class DiscoveryDatabase {
  private pool: Pool;

  constructor(connectionString: string) {
    this.pool = new Pool({
      connectionString,
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });

    this.pool.on('error', (err) => {
      logger.error('Unexpected database error', { error: err instanceof Error ? err.message : String(err) });
    });
  }

  /**
   * Execute a raw query
   */
  async query<T extends Record<string, unknown> = Record<string, unknown>>(
    text: string,
    values?: unknown[]
  ): Promise<QueryResult<T>> {
    return this.pool.query<T>(text, values);
  }

  /**
   * Initialize cache tables owned by this plugin.
   * Source tables (media_items, watch_progress, user_ratings) are NOT created here --
   * they are expected to exist from the main application or other plugins.
   */
  async initializeSchema(): Promise<void> {
    logger.info('Initializing discovery cache schema');

    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      // Trending cache: precomputed trending scores
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_disc_trending_cache (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          media_item_id VARCHAR(255) NOT NULL,
          trending_score DECIMAL(12, 4) NOT NULL DEFAULT 0,
          view_count INTEGER NOT NULL DEFAULT 0,
          avg_rating DECIMAL(3, 2) NOT NULL DEFAULT 0,
          completion_rate DECIMAL(5, 4) NOT NULL DEFAULT 0,
          window_hours INTEGER NOT NULL DEFAULT 24,
          computed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          UNIQUE(media_item_id, window_hours, source_account_id)
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_disc_trending_score
          ON np_disc_trending_cache(trending_score DESC);
        CREATE INDEX IF NOT EXISTS idx_np_disc_trending_computed
          ON np_disc_trending_cache(computed_at DESC);
        CREATE INDEX IF NOT EXISTS idx_np_disc_trending_account
          ON np_disc_trending_cache(source_account_id);
      `);

      // Popular cache: precomputed popularity scores
      await client.query(`
        CREATE TABLE IF NOT EXISTS np_disc_popular_cache (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          media_item_id VARCHAR(255) NOT NULL,
          view_count INTEGER NOT NULL DEFAULT 0,
          avg_rating DECIMAL(3, 2) NOT NULL DEFAULT 0,
          popularity_score DECIMAL(12, 4) NOT NULL DEFAULT 0,
          computed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
          source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
          UNIQUE(media_item_id, source_account_id)
        )
      `);

      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_np_disc_popular_score
          ON np_disc_popular_cache(popularity_score DESC);
        CREATE INDEX IF NOT EXISTS idx_np_disc_popular_computed
          ON np_disc_popular_cache(computed_at DESC);
        CREATE INDEX IF NOT EXISTS idx_np_disc_popular_account
          ON np_disc_popular_cache(source_account_id);
      `);

      // Analytics views (live queries against source tables)
      await client.query(`
        CREATE OR REPLACE VIEW np_disc_trending_live AS
        SELECT
          mi.id,
          mi.title,
          mi.description,
          mi.type,
          mi.genre,
          mi.thumbnail_url,
          mi.poster_url,
          mi.duration_seconds,
          mi.metadata,
          mi.source_account_id,
          COUNT(DISTINCT wp.id) AS view_count,
          COALESCE(AVG(ur.rating), 0) AS avg_rating,
          COALESCE(AVG(
            CASE WHEN wp.duration_seconds > 0
              THEN LEAST(wp.progress_seconds::DECIMAL / wp.duration_seconds, 1.0)
              ELSE 0
            END
          ), 0) AS completion_rate,
          (
            COUNT(DISTINCT wp.id) * 0.50 +
            COALESCE(AVG(ur.rating), 0) * 0.30 +
            COALESCE(AVG(
              CASE WHEN wp.duration_seconds > 0
                THEN LEAST(wp.progress_seconds::DECIMAL / wp.duration_seconds, 1.0)
                ELSE 0
              END
            ), 0) * 0.20
          ) AS trending_score
        FROM media_items mi
        LEFT JOIN watch_progress wp ON mi.id = wp.media_item_id
          AND wp.last_watched_at >= NOW() - INTERVAL '24 hours'
        LEFT JOIN user_ratings ur ON mi.id = ur.media_item_id
        GROUP BY mi.id
        HAVING COUNT(DISTINCT wp.id) > 0
        ORDER BY trending_score DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW np_disc_popular_live AS
        SELECT
          mi.id,
          mi.title,
          mi.description,
          mi.type,
          mi.genre,
          mi.thumbnail_url,
          mi.poster_url,
          mi.duration_seconds,
          mi.metadata,
          mi.source_account_id,
          COUNT(DISTINCT wp.id) AS view_count,
          COALESCE(AVG(ur.rating), 0) AS avg_rating
        FROM media_items mi
        LEFT JOIN watch_progress wp ON mi.id = wp.media_item_id
        LEFT JOIN user_ratings ur ON mi.id = ur.media_item_id
        GROUP BY mi.id
        HAVING COUNT(DISTINCT wp.id) > 0
        ORDER BY view_count DESC, avg_rating DESC
      `);

      await client.query(`
        CREATE OR REPLACE VIEW np_disc_recent_live AS
        SELECT
          mi.id,
          mi.title,
          mi.description,
          mi.type,
          mi.genre,
          mi.thumbnail_url,
          mi.poster_url,
          mi.duration_seconds,
          mi.metadata,
          mi.source_account_id,
          mi.created_at
        FROM media_items mi
        ORDER BY mi.created_at DESC
      `);

      await client.query('COMMIT');
      logger.info('Discovery cache schema initialized successfully');
    } catch (error) {
      await client.query('ROLLBACK');
      logger.error('Failed to initialize discovery schema', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    } finally {
      client.release();
    }
  }

  // ============================================================================
  // Trending Feed
  // ============================================================================

  /**
   * Query trending content directly from source tables.
   * trending_score = (view_count * 0.50) + (avg_rating * 0.30) + (completion_rate * 0.20)
   */
  async getTrending(limit: number, windowHours: number, sourceAccountId?: string): Promise<TrendingItem[]> {
    const conditions: string[] = [];
    const values: unknown[] = [];
    let paramIdx = 1;

    conditions.push(`wp.last_watched_at >= NOW() - ($${paramIdx}::INTEGER || ' hours')::INTERVAL`);
    values.push(windowHours);
    paramIdx++;

    if (sourceAccountId) {
      conditions.push(`mi.source_account_id = $${paramIdx}`);
      values.push(sourceAccountId);
      paramIdx++;
    }

    values.push(limit);

    const result = await this.pool.query<TrendingItem>(
      `SELECT
        mi.id,
        mi.title,
        mi.description,
        mi.type,
        mi.genre,
        mi.thumbnail_url,
        mi.poster_url,
        mi.duration_seconds,
        mi.metadata,
        COUNT(DISTINCT wp.id)::INTEGER AS view_count,
        COALESCE(AVG(ur.rating), 0)::DECIMAL(3,2) AS avg_rating,
        COALESCE(AVG(
          CASE WHEN wp.duration_seconds > 0
            THEN LEAST(wp.progress_seconds::DECIMAL / wp.duration_seconds, 1.0)
            ELSE 0
          END
        ), 0)::DECIMAL(5,4) AS completion_rate,
        (
          COUNT(DISTINCT wp.id) * 0.50 +
          COALESCE(AVG(ur.rating), 0) * 0.30 +
          COALESCE(AVG(
            CASE WHEN wp.duration_seconds > 0
              THEN LEAST(wp.progress_seconds::DECIMAL / wp.duration_seconds, 1.0)
              ELSE 0
            END
          ), 0) * 0.20
        )::DECIMAL(12,4) AS trending_score
      FROM media_items mi
      INNER JOIN watch_progress wp ON mi.id = wp.media_item_id
      LEFT JOIN user_ratings ur ON mi.id = ur.media_item_id
      WHERE ${conditions.join(' AND ')}
      GROUP BY mi.id, mi.title, mi.description, mi.type, mi.genre,
               mi.thumbnail_url, mi.poster_url, mi.duration_seconds, mi.metadata
      HAVING COUNT(DISTINCT wp.id) > 0
      ORDER BY trending_score DESC
      LIMIT $${paramIdx}`,
      values
    );

    return result.rows;
  }

  /**
   * Refresh the trending cache table with precomputed scores.
   */
  async refreshTrendingCache(windowHours: number, sourceAccountId: string = 'primary'): Promise<number> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      // Clear stale entries for this window and account
      await client.query(
        `DELETE FROM np_disc_trending_cache
         WHERE window_hours = $1 AND source_account_id = $2`,
        [windowHours, sourceAccountId]
      );

      // Insert fresh computed data
      const result = await client.query(
        `INSERT INTO np_disc_trending_cache
          (media_item_id, trending_score, view_count, avg_rating, completion_rate, window_hours, source_account_id)
        SELECT
          mi.id,
          (
            COUNT(DISTINCT wp.id) * 0.50 +
            COALESCE(AVG(ur.rating), 0) * 0.30 +
            COALESCE(AVG(
              CASE WHEN wp.duration_seconds > 0
                THEN LEAST(wp.progress_seconds::DECIMAL / wp.duration_seconds, 1.0)
                ELSE 0
              END
            ), 0) * 0.20
          ),
          COUNT(DISTINCT wp.id),
          COALESCE(AVG(ur.rating), 0),
          COALESCE(AVG(
            CASE WHEN wp.duration_seconds > 0
              THEN LEAST(wp.progress_seconds::DECIMAL / wp.duration_seconds, 1.0)
              ELSE 0
            END
          ), 0),
          $1,
          $2
        FROM media_items mi
        INNER JOIN watch_progress wp ON mi.id = wp.media_item_id
          AND wp.last_watched_at >= NOW() - ($1::INTEGER || ' hours')::INTERVAL
        LEFT JOIN user_ratings ur ON mi.id = ur.media_item_id
        WHERE mi.source_account_id = $2
        GROUP BY mi.id
        HAVING COUNT(DISTINCT wp.id) > 0`,
        [windowHours, sourceAccountId]
      );

      await client.query('COMMIT');
      const count = result.rowCount ?? 0;
      logger.info('Trending cache refreshed', { count, windowHours, sourceAccountId });
      return count;
    } catch (error) {
      await client.query('ROLLBACK');
      logger.error('Failed to refresh trending cache', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    } finally {
      client.release();
    }
  }

  // ============================================================================
  // Popular Feed
  // ============================================================================

  /**
   * Query popular content directly from source tables.
   * Ordered by total view count (all time) weighted by average rating.
   */
  async getPopular(limit: number, sourceAccountId?: string): Promise<PopularItem[]> {
    const conditions: string[] = [];
    const values: unknown[] = [];
    let paramIdx = 1;

    if (sourceAccountId) {
      conditions.push(`mi.source_account_id = $${paramIdx}`);
      values.push(sourceAccountId);
      paramIdx++;
    }

    values.push(limit);

    const whereClause = conditions.length > 0 ? `WHERE ${conditions.join(' AND ')}` : '';

    const result = await this.pool.query<PopularItem>(
      `SELECT
        mi.id,
        mi.title,
        mi.description,
        mi.type,
        mi.genre,
        mi.thumbnail_url,
        mi.poster_url,
        mi.duration_seconds,
        mi.metadata,
        COUNT(DISTINCT wp.id)::INTEGER AS view_count,
        COALESCE(AVG(ur.rating), 0)::DECIMAL(3,2) AS avg_rating
      FROM media_items mi
      LEFT JOIN watch_progress wp ON mi.id = wp.media_item_id
      LEFT JOIN user_ratings ur ON mi.id = ur.media_item_id
      ${whereClause}
      GROUP BY mi.id, mi.title, mi.description, mi.type, mi.genre,
               mi.thumbnail_url, mi.poster_url, mi.duration_seconds, mi.metadata
      HAVING COUNT(DISTINCT wp.id) > 0
      ORDER BY view_count DESC, avg_rating DESC
      LIMIT $${paramIdx}`,
      values
    );

    return result.rows;
  }

  /**
   * Refresh the popular cache table.
   */
  async refreshPopularCache(sourceAccountId: string = 'primary'): Promise<number> {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');

      await client.query(
        `DELETE FROM np_disc_popular_cache WHERE source_account_id = $1`,
        [sourceAccountId]
      );

      const result = await client.query(
        `INSERT INTO np_disc_popular_cache
          (media_item_id, view_count, avg_rating, popularity_score, source_account_id)
        SELECT
          mi.id,
          COUNT(DISTINCT wp.id),
          COALESCE(AVG(ur.rating), 0),
          (COUNT(DISTINCT wp.id) * 0.70 + COALESCE(AVG(ur.rating), 0) * 0.30),
          $1
        FROM media_items mi
        LEFT JOIN watch_progress wp ON mi.id = wp.media_item_id
        LEFT JOIN user_ratings ur ON mi.id = ur.media_item_id
        WHERE mi.source_account_id = $1
        GROUP BY mi.id
        HAVING COUNT(DISTINCT wp.id) > 0`,
        [sourceAccountId]
      );

      await client.query('COMMIT');
      const count = result.rowCount ?? 0;
      logger.info('Popular cache refreshed', { count, sourceAccountId });
      return count;
    } catch (error) {
      await client.query('ROLLBACK');
      logger.error('Failed to refresh popular cache', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    } finally {
      client.release();
    }
  }

  // ============================================================================
  // Recent Feed
  // ============================================================================

  /**
   * Query recently added content.
   */
  async getRecent(limit: number, sourceAccountId?: string): Promise<RecentItem[]> {
    const conditions: string[] = [];
    const values: unknown[] = [];
    let paramIdx = 1;

    if (sourceAccountId) {
      conditions.push(`mi.source_account_id = $${paramIdx}`);
      values.push(sourceAccountId);
      paramIdx++;
    }

    values.push(limit);

    const whereClause = conditions.length > 0 ? `WHERE ${conditions.join(' AND ')}` : '';

    const result = await this.pool.query<RecentItem>(
      `SELECT
        mi.id,
        mi.title,
        mi.description,
        mi.type,
        mi.genre,
        mi.thumbnail_url,
        mi.poster_url,
        mi.duration_seconds,
        mi.metadata,
        mi.created_at
      FROM media_items mi
      ${whereClause}
      ORDER BY mi.created_at DESC
      LIMIT $${paramIdx}`,
      values
    );

    return result.rows;
  }

  // ============================================================================
  // Continue Watching Feed
  // ============================================================================

  /**
   * Query continue watching items for a specific user.
   * Returns items where 5% < progress < 95% (started but not finished).
   */
  async getContinueWatching(
    userId: string,
    limit: number,
    sourceAccountId?: string
  ): Promise<ContinueWatchingItem[]> {
    const conditions: string[] = [
      'wp.user_id = $1',
      'wp.progress_percent > 5',
      'wp.progress_percent < 95',
      'wp.completed = false',
    ];
    const values: unknown[] = [userId];
    let paramIdx = 2;

    if (sourceAccountId) {
      conditions.push(`mi.source_account_id = $${paramIdx}`);
      values.push(sourceAccountId);
      paramIdx++;
    }

    values.push(limit);

    const result = await this.pool.query<ContinueWatchingItem>(
      `SELECT
        mi.id,
        mi.title,
        mi.description,
        mi.type,
        mi.genre,
        mi.thumbnail_url,
        mi.poster_url,
        mi.duration_seconds,
        mi.metadata,
        wp.progress_seconds,
        wp.progress_percent,
        wp.last_watched_at
      FROM watch_progress wp
      INNER JOIN media_items mi ON wp.media_item_id = mi.id
      WHERE ${conditions.join(' AND ')}
      ORDER BY wp.last_watched_at DESC
      LIMIT $${paramIdx}`,
      values
    );

    return result.rows;
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  /**
   * Get discovery plugin statistics.
   */
  async getStatistics(): Promise<DiscoveryStatistics> {
    const [mediaResult, progressResult, ratingsResult, trendingCacheResult, popularCacheResult] =
      await Promise.all([
        this.pool.query<{ count: string }>('SELECT COUNT(*) AS count FROM media_items'),
        this.pool.query<{ count: string }>('SELECT COUNT(*) AS count FROM watch_progress'),
        this.pool.query<{ count: string }>('SELECT COUNT(*) AS count FROM user_ratings'),
        this.pool.query<{ count: string }>('SELECT COUNT(*) AS count FROM np_disc_trending_cache'),
        this.pool.query<{ count: string }>('SELECT COUNT(*) AS count FROM np_disc_popular_cache'),
      ]);

    return {
      total_media_items: parseInt(mediaResult.rows[0]?.count ?? '0', 10),
      total_watch_progress: parseInt(progressResult.rows[0]?.count ?? '0', 10),
      total_user_ratings: parseInt(ratingsResult.rows[0]?.count ?? '0', 10),
      trending_cache_entries: parseInt(trendingCacheResult.rows[0]?.count ?? '0', 10),
      popular_cache_entries: parseInt(popularCacheResult.rows[0]?.count ?? '0', 10),
      cache_hit_rate: 0, // Tracked at cache layer, not DB
    };
  }

  /**
   * Check database connectivity.
   */
  async isConnected(): Promise<boolean> {
    try {
      await this.pool.query('SELECT 1');
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get last cache computation time for trending.
   */
  async getTrendingLastComputed(sourceAccountId: string = 'primary'): Promise<Date | null> {
    const result = await this.pool.query<{ max: Date | null }>(
      `SELECT MAX(computed_at) AS max FROM np_disc_trending_cache WHERE source_account_id = $1`,
      [sourceAccountId]
    );
    return result.rows[0]?.max ?? null;
  }

  /**
   * Get last cache computation time for popular.
   */
  async getPopularLastComputed(sourceAccountId: string = 'primary'): Promise<Date | null> {
    const result = await this.pool.query<{ max: Date | null }>(
      `SELECT MAX(computed_at) AS max FROM np_disc_popular_cache WHERE source_account_id = $1`,
      [sourceAccountId]
    );
    return result.rows[0]?.max ?? null;
  }

  /**
   * Clear all cache tables.
   */
  async clearCache(): Promise<void> {
    await this.pool.query('TRUNCATE np_disc_trending_cache');
    await this.pool.query('TRUNCATE np_disc_popular_cache');
    logger.info('All database cache tables cleared');
  }

  /**
   * Close database connections.
   */
  async close(): Promise<void> {
    await this.pool.end();
    logger.info('Database connection pool closed');
  }
}
