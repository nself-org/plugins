/**
 * Discovery Plugin Redis Cache-Aside Implementation
 *
 * Pattern: Check cache first, fall back to database, populate cache on miss.
 * Graceful degradation: if Redis is unavailable, queries go directly to the database.
 */

import Redis from 'ioredis';
import { createLogger } from '@nself/plugin-utils';
import { DiscoveryDatabase } from './database.js';
import { config } from './config.js';
import type {
  TrendingItem,
  PopularItem,
  RecentItem,
  ContinueWatchingItem,
  CacheEntry,
} from './types.js';

const logger = createLogger('discovery:cache');

// ============================================================================
// Cache Key Patterns
// ============================================================================

const CACHE_KEYS = {
  trending: (limit: number, window: number, accountId?: string) =>
    `disc:trending:${accountId || 'all'}:${limit}:${window}`,
  popular: (limit: number, accountId?: string) =>
    `disc:popular:${accountId || 'all'}:${limit}`,
  recent: (limit: number, accountId?: string) =>
    `disc:recent:${accountId || 'all'}:${limit}`,
  continue: (userId: string, limit: number, accountId?: string) =>
    `disc:continue:${accountId || 'all'}:${userId}:${limit}`,
} as const;

const CACHE_PREFIX = 'disc:';

// ============================================================================
// Discovery Cache
// ============================================================================

export class DiscoveryCache {
  private redis: Redis | null = null;
  private redisConnected = false;
  private db: DiscoveryDatabase;
  private connectAttempts = 0;
  private maxConnectAttempts = 3;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(db: DiscoveryDatabase) {
    this.db = db;
  }

  /**
   * Initialize Redis connection.
   * Non-blocking: if Redis is unavailable, the plugin operates without cache.
   */
  async connect(): Promise<void> {
    try {
      this.redis = new Redis(config.redis_url, {
        maxRetriesPerRequest: 3,
        retryStrategy: (times: number) => {
          if (times > this.maxConnectAttempts) {
            logger.warn('Redis max retries exceeded, operating without cache');
            return null; // Stop retrying
          }
          return Math.min(times * 200, 2000);
        },
        lazyConnect: true,
        enableOfflineQueue: false,
      });

      this.redis.on('connect', () => {
        this.redisConnected = true;
        this.connectAttempts = 0;
        logger.info('Redis connected');
      });

      this.redis.on('error', (err) => {
        if (this.redisConnected) {
          logger.error('Redis error', { error: err.message });
        }
        this.redisConnected = false;
      });

      this.redis.on('close', () => {
        this.redisConnected = false;
        logger.warn('Redis connection closed');
        this.scheduleReconnect();
      });

      await this.redis.connect();
    } catch (error) {
      this.redisConnected = false;
      logger.warn('Redis connection failed, operating without cache', {
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  /**
   * Schedule a reconnection attempt.
   */
  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    if (this.connectAttempts >= this.maxConnectAttempts) return;

    this.reconnectTimer = setTimeout(async () => {
      this.reconnectTimer = null;
      this.connectAttempts++;
      logger.info('Attempting Redis reconnection', { attempt: this.connectAttempts });
      await this.connect();
    }, 5000);
  }

  /**
   * Check if Redis is available.
   */
  isConnected(): boolean {
    return this.redisConnected && this.redis !== null;
  }

  // ============================================================================
  // Generic Cache Operations
  // ============================================================================

  /**
   * Get a value from cache.
   */
  private async cacheGet<T>(key: string): Promise<CacheEntry<T> | null> {
    if (!this.isConnected() || !this.redis) return null;

    try {
      const raw = await this.redis.get(key);
      if (!raw) return null;

      const entry = JSON.parse(raw) as CacheEntry<T>;
      return entry;
    } catch (error) {
      logger.warn('Cache get failed', {
        key,
        error: error instanceof Error ? error.message : String(error),
      });
      return null;
    }
  }

  /**
   * Set a value in cache with TTL.
   */
  private async cacheSet<T>(key: string, data: T[], ttl: number): Promise<void> {
    if (!this.isConnected() || !this.redis) return;

    try {
      const entry: CacheEntry<T> = {
        data,
        cached_at: new Date().toISOString(),
        ttl,
        source: 'cache',
      };

      await this.redis.setex(key, ttl, JSON.stringify(entry));
    } catch (error) {
      logger.warn('Cache set failed', {
        key,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  /**
   * Delete a specific cache key.
   */
  private async cacheDel(key: string): Promise<void> {
    if (!this.isConnected() || !this.redis) return;

    try {
      await this.redis.del(key);
    } catch (error) {
      logger.warn('Cache delete failed', {
        key,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  // ============================================================================
  // Trending Feed (cache-aside)
  // ============================================================================

  /**
   * Get trending content. Checks Redis first, falls back to database.
   */
  async getTrending(
    limit: number,
    windowHours: number,
    sourceAccountId?: string
  ): Promise<{ items: TrendingItem[]; cached: boolean; cached_at: string | null }> {
    const cacheKey = CACHE_KEYS.trending(limit, windowHours, sourceAccountId);

    // Try cache first
    const cached = await this.cacheGet<TrendingItem>(cacheKey);
    if (cached) {
      logger.debug('Trending cache hit', { limit, windowHours });
      return { items: cached.data, cached: true, cached_at: cached.cached_at };
    }

    // Cache miss: query database
    logger.debug('Trending cache miss, querying database', { limit, windowHours });
    const items = await this.db.getTrending(limit, windowHours, sourceAccountId);

    // Populate cache
    await this.cacheSet(cacheKey, items, config.cache_ttl_trending);

    return { items, cached: false, cached_at: null };
  }

  // ============================================================================
  // Popular Feed (cache-aside)
  // ============================================================================

  /**
   * Get popular content. Checks Redis first, falls back to database.
   */
  async getPopular(
    limit: number,
    sourceAccountId?: string
  ): Promise<{ items: PopularItem[]; cached: boolean; cached_at: string | null }> {
    const cacheKey = CACHE_KEYS.popular(limit, sourceAccountId);

    const cached = await this.cacheGet<PopularItem>(cacheKey);
    if (cached) {
      logger.debug('Popular cache hit', { limit });
      return { items: cached.data, cached: true, cached_at: cached.cached_at };
    }

    logger.debug('Popular cache miss, querying database', { limit });
    const items = await this.db.getPopular(limit, sourceAccountId);

    await this.cacheSet(cacheKey, items, config.cache_ttl_popular);

    return { items, cached: false, cached_at: null };
  }

  // ============================================================================
  // Recent Feed (cache-aside)
  // ============================================================================

  /**
   * Get recently added content. Checks Redis first, falls back to database.
   */
  async getRecent(
    limit: number,
    sourceAccountId?: string
  ): Promise<{ items: RecentItem[]; cached: boolean; cached_at: string | null }> {
    const cacheKey = CACHE_KEYS.recent(limit, sourceAccountId);

    const cached = await this.cacheGet<RecentItem>(cacheKey);
    if (cached) {
      logger.debug('Recent cache hit', { limit });
      return { items: cached.data, cached: true, cached_at: cached.cached_at };
    }

    logger.debug('Recent cache miss, querying database', { limit });
    const items = await this.db.getRecent(limit, sourceAccountId);

    await this.cacheSet(cacheKey, items, config.cache_ttl_recent);

    return { items, cached: false, cached_at: null };
  }

  // ============================================================================
  // Continue Watching Feed (cache-aside, per user)
  // ============================================================================

  /**
   * Get continue watching for a specific user.
   */
  async getContinueWatching(
    userId: string,
    limit: number,
    sourceAccountId?: string
  ): Promise<{ items: ContinueWatchingItem[]; cached: boolean; cached_at: string | null }> {
    const cacheKey = CACHE_KEYS.continue(userId, limit, sourceAccountId);

    const cached = await this.cacheGet<ContinueWatchingItem>(cacheKey);
    if (cached) {
      logger.debug('Continue watching cache hit', { userId, limit });
      return { items: cached.data, cached: true, cached_at: cached.cached_at };
    }

    logger.debug('Continue watching cache miss, querying database', { userId, limit });
    const items = await this.db.getContinueWatching(userId, limit, sourceAccountId);

    await this.cacheSet(cacheKey, items, config.cache_ttl_continue);

    return { items, cached: false, cached_at: null };
  }

  // ============================================================================
  // Cache Invalidation
  // ============================================================================

  /**
   * Invalidate all trending caches.
   */
  async invalidateTrending(): Promise<number> {
    return this.invalidateByPattern('disc:trending:*');
  }

  /**
   * Invalidate all popular caches.
   */
  async invalidatePopular(): Promise<number> {
    return this.invalidateByPattern('disc:popular:*');
  }

  /**
   * Invalidate all recent caches.
   */
  async invalidateRecent(): Promise<number> {
    return this.invalidateByPattern('disc:recent:*');
  }

  /**
   * Invalidate continue watching cache for a specific user.
   */
  async invalidateContinueWatching(userId: string): Promise<number> {
    return this.invalidateByPattern(`disc:continue:*:${userId}:*`);
  }

  /**
   * Invalidate all discovery caches.
   */
  async invalidateAll(): Promise<number> {
    return this.invalidateByPattern(`${CACHE_PREFIX}*`);
  }

  /**
   * Invalidate cache keys matching a pattern using SCAN (non-blocking).
   */
  private async invalidateByPattern(pattern: string): Promise<number> {
    if (!this.isConnected() || !this.redis) return 0;

    try {
      let cursor = '0';
      let deleted = 0;

      do {
        const [nextCursor, keys] = await this.redis.scan(cursor, 'MATCH', pattern, 'COUNT', 100);
        cursor = nextCursor;

        if (keys.length > 0) {
          await this.redis.del(...keys);
          deleted += keys.length;
        }
      } while (cursor !== '0');

      if (deleted > 0) {
        logger.info('Cache invalidated', { pattern, deleted });
      }

      return deleted;
    } catch (error) {
      logger.warn('Cache invalidation failed', {
        pattern,
        error: error instanceof Error ? error.message : String(error),
      });
      return 0;
    }
  }

  /**
   * Get total number of discovery cache keys.
   */
  async getCacheKeyCount(): Promise<number> {
    if (!this.isConnected() || !this.redis) return 0;

    try {
      let cursor = '0';
      let count = 0;

      do {
        const [nextCursor, keys] = await this.redis.scan(cursor, 'MATCH', `${CACHE_PREFIX}*`, 'COUNT', 100);
        cursor = nextCursor;
        count += keys.length;
      } while (cursor !== '0');

      return count;
    } catch {
      return 0;
    }
  }

  /**
   * Close Redis connection.
   */
  async close(): Promise<void> {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    if (this.redis) {
      try {
        await this.redis.quit();
      } catch {
        // Ignore quit errors during shutdown
      }
      this.redis = null;
      this.redisConnected = false;
      logger.info('Redis connection closed');
    }
  }
}
