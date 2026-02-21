/**
 * Content Progress Plugin Types
 * Complete type definitions for progress tracking, watchlists, and favorites
 */

export interface ContentProgressConfig {
  port: number;
  host: string;
  completeThreshold: number;
  historySampleSeconds: number;
  historyRetentionDays: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Progress Types
// =============================================================================

export type ContentType = 'movie' | 'episode' | 'video' | 'audio' | 'article' | 'course';

export type ProgressAction = 'play' | 'pause' | 'seek' | 'complete' | 'resume';

export interface ProgressPositionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: ContentType;
  content_id: string;
  position_seconds: number;
  duration_seconds: number | null;
  progress_percent: number;
  completed: boolean;
  completed_at: Date | null;
  device_id: string | null;
  audio_track: string | null;
  subtitle_track: string | null;
  quality: string | null;
  metadata: Record<string, unknown>;
  updated_at: Date;
  created_at: Date;
}

export interface UpdateProgressRequest {
  user_id: string;
  content_type: ContentType;
  content_id: string;
  position_seconds: number;
  duration_seconds?: number;
  device_id?: string;
  audio_track?: string;
  subtitle_track?: string;
  quality?: string;
  metadata?: Record<string, unknown>;
}

export interface ProgressHistoryRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: ContentType;
  content_id: string;
  action: ProgressAction;
  position_seconds: number | null;
  device_id: string | null;
  session_id: string | null;
  created_at: Date;
}

export interface CreateHistoryRequest {
  user_id: string;
  content_type: ContentType;
  content_id: string;
  action: ProgressAction;
  position_seconds?: number;
  device_id?: string;
  session_id?: string;
}

// =============================================================================
// Watchlist Types
// =============================================================================

export interface WatchlistRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: ContentType;
  content_id: string;
  priority: number;
  added_from: string | null;
  notes: string | null;
  created_at: Date;
}

export interface AddToWatchlistRequest {
  user_id: string;
  content_type: ContentType;
  content_id: string;
  priority?: number;
  added_from?: string;
  notes?: string;
}

export interface UpdateWatchlistRequest {
  priority?: number;
  notes?: string;
}

// =============================================================================
// Favorites Types
// =============================================================================

export interface FavoriteRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: ContentType;
  content_id: string;
  created_at: Date;
}

export interface AddToFavoritesRequest {
  user_id: string;
  content_type: ContentType;
  content_id: string;
}

// =============================================================================
// Webhook Events
// =============================================================================

export interface WebhookEventRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

export interface WebhookEvent {
  id: string;
  type: string;
  data: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Stats and Analytics Types
// =============================================================================

export interface UserStats {
  total_watch_time_seconds: number;
  total_watch_time_hours: number;
  content_completed: number;
  content_in_progress: number;
  watchlist_count: number;
  favorites_count: number;
  most_watched_type: ContentType | null;
  recent_activity: Date | null;
}

export interface PluginStats {
  total_users: number;
  total_positions: number;
  total_completed: number;
  total_in_progress: number;
  total_watchlist: number;
  total_favorites: number;
  total_history_events: number;
  last_activity: Date | null;
}

// =============================================================================
// Continue Watching Types
// =============================================================================

export interface ContinueWatchingItem extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: ContentType;
  content_id: string;
  position_seconds: number;
  duration_seconds: number | null;
  progress_percent: number;
  updated_at: Date;
  metadata: Record<string, unknown>;
}

export interface RecentlyWatchedItem extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: ContentType;
  content_id: string;
  position_seconds: number;
  duration_seconds: number | null;
  progress_percent: number;
  completed: boolean;
  completed_at: Date | null;
  updated_at: Date;
  metadata: Record<string, unknown>;
}
