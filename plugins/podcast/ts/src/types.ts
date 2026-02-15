/**
 * Podcast Plugin Types
 * Complete type definitions for podcasts, episodes, subscriptions, playback positions, and categories
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface PodcastRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  title: string;
  description: string | null;
  author: string | null;
  feed_url: string;
  website_url: string | null;
  image_url: string | null;
  language: string;
  categories: string[];
  explicit: boolean;
  last_fetched_at: Date | null;
  last_published_at: Date | null;
  etag: string | null;
  last_modified: string | null;
  feed_status: string;
  episode_count: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface EpisodeRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  podcast_id: string;
  guid: string;
  title: string;
  description: string | null;
  content_html: string | null;
  published_at: Date | null;
  duration_seconds: number | null;
  audio_url: string;
  audio_type: string | null;
  audio_size_bytes: number | null;
  image_url: string | null;
  season_number: number | null;
  episode_number: number | null;
  episode_type: string;
  explicit: boolean;
  transcript_url: string | null;
  chapters_url: string | null;
  search_vector: unknown;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface SubscriptionRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  user_id: string;
  podcast_id: string;
  subscribed_at: Date;
  is_active: boolean;
  notification_enabled: boolean;
  auto_download: boolean;
  metadata: Record<string, unknown>;
}

export interface PlaybackPositionRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  user_id: string;
  episode_id: string;
  position_seconds: number;
  duration_seconds: number | null;
  completed: boolean;
  completed_at: Date | null;
  device_id: string | null;
  updated_at: Date;
}

export interface CategoryRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  slug: string;
  parent_id: string | null;
  podcast_count: number;
  sort_order: number;
  created_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface CreatePodcastRequest {
  feed_url: string;
  title?: string;
  description?: string;
  author?: string;
  website_url?: string;
  image_url?: string;
  language?: string;
  categories?: string[];
  explicit?: boolean;
}

export interface UpdatePodcastRequest {
  title?: string;
  description?: string;
  author?: string;
  website_url?: string;
  image_url?: string;
  language?: string;
  categories?: string[];
  explicit?: boolean;
  feed_status?: string;
}

export interface ListPodcastsQuery {
  category?: string;
  language?: string;
  feed_status?: string;
  limit?: number;
  offset?: number;
}

export interface ListEpisodesQuery {
  podcast_id?: string;
  season_number?: number;
  episode_type?: string;
  limit?: number;
  offset?: number;
}

export interface SearchPodcastsRequest {
  query: string;
  category?: string;
  language?: string;
  limit?: number;
}

export interface SearchEpisodesRequest {
  query: string;
  podcast_id?: string;
  limit?: number;
}

export interface SubscribeRequest {
  user_id: string;
  podcast_id: string;
  notification_enabled?: boolean;
  auto_download?: boolean;
}

export interface UpdateSubscriptionRequest {
  is_active?: boolean;
  notification_enabled?: boolean;
  auto_download?: boolean;
}

export interface UpdatePlaybackPositionRequest {
  user_id: string;
  episode_id: string;
  position_seconds: number;
  duration_seconds?: number;
  completed?: boolean;
  device_id?: string;
}

export interface GetPlaybackPositionsQuery {
  user_id: string;
  episode_ids?: string;
}

export interface SyncFeedRequest {
  podcast_id?: string;
  force?: boolean;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface PodcastWithStats {
  [key: string]: unknown;
  id: string;
  title: string;
  description: string | null;
  author: string | null;
  feed_url: string;
  website_url: string | null;
  image_url: string | null;
  language: string;
  categories: string[];
  episode_count: number;
  last_published_at: Date | null;
  feed_status: string;
}

export interface EpisodeWithPodcast {
  [key: string]: unknown;
  episode_id: string;
  episode_title: string;
  description: string | null;
  published_at: Date | null;
  duration_seconds: number | null;
  audio_url: string;
  season_number: number | null;
  episode_number: number | null;
  podcast_id: string;
  podcast_title: string;
  podcast_image_url: string | null;
}

export interface SubscriptionWithPodcast {
  [key: string]: unknown;
  subscription_id: string;
  user_id: string;
  podcast_id: string;
  podcast_title: string;
  podcast_image_url: string | null;
  podcast_author: string | null;
  subscribed_at: Date;
  is_active: boolean;
  notification_enabled: boolean;
  auto_download: boolean;
  unplayed_count: number;
}

export interface SyncResult {
  podcast_id: string;
  podcast_title: string;
  new_episodes: number;
  updated_episodes: number;
  errors: string[];
}

// =============================================================================
// Stats Types
// =============================================================================

export interface PodcastStats {
  total_podcasts: number;
  active_podcasts: number;
  total_episodes: number;
  total_subscriptions: number;
  total_categories: number;
  oldest_episode: Date | null;
  newest_episode: Date | null;
}
