/**
 * Discovery Plugin Types
 * Type definitions for content discovery feeds: trending, popular, recent, continue watching
 */

// ============================================================================
// Configuration
// ============================================================================

export interface DiscoveryConfig {
  /** PostgreSQL connection string */
  database_url: string;
  /** Redis connection string */
  redis_url: string;
  /** HTTP server port */
  port: number;
  /** Trending window in hours (default: 24) */
  trending_window_hours: number;
  /** Default result limit (default: 20) */
  default_limit: number;
  /** Cache TTL for trending feed in seconds (default: 900 = 15min) */
  cache_ttl_trending: number;
  /** Cache TTL for popular feed in seconds (default: 3600 = 1hr) */
  cache_ttl_popular: number;
  /** Cache TTL for recent feed in seconds (default: 1800 = 30min) */
  cache_ttl_recent: number;
  /** Cache TTL for continue watching per user in seconds (default: 300 = 5min) */
  cache_ttl_continue: number;
  /** Log level */
  log_level: 'debug' | 'info' | 'warn' | 'error';
}

// ============================================================================
// Source Table Types (READ-ONLY — these exist in the main database)
// ============================================================================

/**
 * Represents a row from the media_items table.
 * This plugin reads from this table but never writes to it.
 */
export interface MediaItem {
  id: string;
  title: string;
  description: string | null;
  type: string;
  genre: string | null;
  thumbnail_url: string | null;
  poster_url: string | null;
  duration_seconds: number | null;
  release_date: Date | null;
  created_at: Date;
  updated_at: Date;
  source_account_id: string;
  metadata: Record<string, unknown>;
}

/**
 * Represents a row from the watch_progress table.
 * Tracks how far a user has watched a piece of content.
 */
export interface WatchProgress {
  id: string;
  user_id: string;
  media_item_id: string;
  progress_seconds: number;
  duration_seconds: number;
  progress_percent: number;
  completed: boolean;
  last_watched_at: Date;
  created_at: Date;
  updated_at: Date;
  source_account_id: string;
}

/**
 * Represents a row from the user_ratings table.
 */
export interface UserRating {
  id: string;
  user_id: string;
  media_item_id: string;
  rating: number;
  created_at: Date;
  updated_at: Date;
  source_account_id: string;
}

// ============================================================================
// Feed Item Types (API response shapes)
// ============================================================================

export interface TrendingItem {
  id: string;
  title: string;
  description: string | null;
  type: string;
  genre: string | null;
  thumbnail_url: string | null;
  poster_url: string | null;
  duration_seconds: number | null;
  trending_score: number;
  view_count: number;
  avg_rating: number;
  completion_rate: number;
  metadata: Record<string, unknown>;
}

export interface PopularItem {
  id: string;
  title: string;
  description: string | null;
  type: string;
  genre: string | null;
  thumbnail_url: string | null;
  poster_url: string | null;
  duration_seconds: number | null;
  view_count: number;
  avg_rating: number;
  metadata: Record<string, unknown>;
}

export interface RecentItem {
  id: string;
  title: string;
  description: string | null;
  type: string;
  genre: string | null;
  thumbnail_url: string | null;
  poster_url: string | null;
  duration_seconds: number | null;
  created_at: Date;
  metadata: Record<string, unknown>;
}

export interface ContinueWatchingItem {
  id: string;
  title: string;
  description: string | null;
  type: string;
  genre: string | null;
  thumbnail_url: string | null;
  poster_url: string | null;
  duration_seconds: number | null;
  progress_seconds: number;
  progress_percent: number;
  last_watched_at: Date;
  metadata: Record<string, unknown>;
}

// ============================================================================
// Cache Types
// ============================================================================

export interface CacheEntry<T> {
  data: T[];
  cached_at: string;
  ttl: number;
  source: 'cache' | 'database';
}

// ============================================================================
// Cache Table Types (owned by this plugin)
// ============================================================================

export interface TrendingCacheRecord {
  id: string;
  media_item_id: string;
  trending_score: number;
  view_count: number;
  avg_rating: number;
  completion_rate: number;
  window_hours: number;
  computed_at: Date;
  source_account_id: string;
}

export interface PopularCacheRecord {
  id: string;
  media_item_id: string;
  view_count: number;
  avg_rating: number;
  popularity_score: number;
  computed_at: Date;
  source_account_id: string;
}

// ============================================================================
// API Response Types
// ============================================================================

export interface FeedResponse<T> {
  items: T[];
  count: number;
  cached: boolean;
  cached_at: string | null;
  generated_at: string;
}

export interface HealthResponse {
  status: 'ok' | 'degraded' | 'error';
  timestamp: string;
  database: boolean;
  redis: boolean;
  version: string;
}

export interface StatusResponse {
  feeds: {
    trending: { cached: boolean; item_count: number; last_computed: string | null };
    popular: { cached: boolean; item_count: number; last_computed: string | null };
    recent: { cached: boolean; item_count: number };
    continue_watching: { cached_users: number };
  };
  cache: {
    connected: boolean;
    keys: number;
  };
  database: {
    connected: boolean;
    media_items: number;
    watch_progress: number;
    user_ratings: number;
  };
}

// ============================================================================
// Query Parameter Types
// ============================================================================

export interface TrendingQuery {
  limit?: number;
  window_hours?: number;
  source_account_id?: string;
}

export interface PopularQuery {
  limit?: number;
  source_account_id?: string;
}

export interface RecentQuery {
  limit?: number;
  source_account_id?: string;
}

export interface ContinueWatchingQuery {
  limit?: number;
  source_account_id?: string;
}

// ============================================================================
// Statistics Types
// ============================================================================

export interface DiscoveryStatistics {
  total_media_items: number;
  total_watch_progress: number;
  total_user_ratings: number;
  trending_cache_entries: number;
  popular_cache_entries: number;
  cache_hit_rate: number;
}
