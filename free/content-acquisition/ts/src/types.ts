/**
 * Content Acquisition Types
 */

export interface ContentAcquisitionConfig {
  database_url: string;
  port: number;
  metadata_enrichment_url: string;
  torrent_manager_url: string;
  vpn_manager_url: string;
  subtitle_manager_url: string;
  media_processing_url: string;
  ntv_backend_url: string;
  redis_host: string;
  redis_port: number;
  log_level: string;
  /** Interval in minutes between scheduled RSS feed checks (default: 30) */
  rss_check_interval: number;
}

export interface QualityProfile {
  id: string;
  source_account_id: string;
  name: string;
  description?: string;
  preferred_qualities: string[];
  max_size_gb?: number;
  min_size_gb?: number;
  preferred_sources: string[];
  excluded_sources: string[];
  preferred_groups?: string[];
  excluded_groups?: string[];
  preferred_languages: string[];
  require_subtitles: boolean;
  min_seeders: number;
  wait_for_better_quality: boolean;
  wait_hours: number;
  created_at: Date;
  updated_at: Date;
}

export interface ContentMetadata {
  tmdb_id?: number;
  imdb_id?: string;
  tvdb_id?: number;
  year?: number;
  genres?: string[];
  overview?: string;
  poster_path?: string;
  backdrop_path?: string;
  [key: string]: unknown;
}

export interface Subscription {
  id: string;
  source_account_id: string;
  subscription_type: 'tv_show' | 'movie_collection' | 'artist' | 'podcast';
  content_id?: string;
  content_name: string;
  content_metadata?: ContentMetadata;
  quality_profile_id?: string;
  enabled: boolean;
  auto_upgrade: boolean;
  monitor_future_seasons: boolean;
  monitor_existing_seasons: boolean;
  season_folder: boolean;
  last_check_at?: Date;
  last_download_at?: Date;
  next_check_at?: Date;
  created_at: Date;
  updated_at: Date;
}

export interface RSSFeed {
  id: string;
  source_account_id: string;
  name: string;
  url: string;
  feed_type: 'tv_shows' | 'movies' | 'anime' | 'music';
  enabled: boolean;
  check_interval_minutes: number;
  quality_profile_id?: string;
  last_check_at?: Date;
  last_success_at?: Date;
  last_error?: string;
  consecutive_failures: number;
  next_check_at?: Date;
  created_at: Date;
  updated_at: Date;
}

export interface RSSFeedItem {
  id: string;
  feed_id: string;
  source_account_id: string;
  title: string;
  link?: string;
  magnet_uri?: string;
  info_hash?: string;
  pub_date?: Date;
  parsed_title?: string;
  parsed_year?: number;
  parsed_season?: number;
  parsed_episode?: number;
  parsed_quality?: string;
  parsed_source?: string;
  parsed_group?: string;
  size_bytes?: number;
  seeders?: number;
  leechers?: number;
  status: 'pending' | 'matched' | 'downloaded' | 'rejected' | 'failed';
  matched_subscription_id?: string;
  rejection_reason?: string;
  download_id?: string;
  created_at: Date;
  processed_at?: Date;
}

export interface ReleaseCalendarItem {
  id: string;
  source_account_id: string;
  content_type: 'movie' | 'tv_episode' | 'album';
  content_id: string;
  content_name: string;
  season?: number;
  episode?: number;
  release_date: Date;
  digital_release_date?: Date;
  physical_release_date?: Date;
  subscription_id?: string;
  quality_profile_id?: string;
  monitoring_enabled: boolean;
  status: 'awaiting' | 'searching' | 'found' | 'downloaded' | 'failed';
  first_search_at?: Date;
  found_at?: Date;
  download_id?: string;
  created_at: Date;
  updated_at: Date;
}

export interface AcquisitionQueueItem {
  id: string;
  source_account_id: string;
  content_type: 'movie' | 'tv_episode' | 'music' | 'other';
  content_name: string;
  year?: number;
  season?: number;
  episode?: number;
  quality_profile_id?: string;
  requested_by: string;
  request_source_id?: string;
  status: 'pending' | 'searching' | 'matched' | 'downloading' | 'completed' | 'failed';
  priority: number;
  attempts: number;
  max_attempts: number;
  matched_torrent?: MatchedTorrentInfo;
  download_id?: string;
  error_message?: string;
  created_at: Date;
  started_at?: Date;
  completed_at?: Date;
}

export interface MatchedTorrentInfo {
  name: string;
  info_hash: string;
  magnet_uri?: string;
  size_bytes: number;
  seeders: number;
  quality?: string;
  source: string;
  [key: string]: unknown;
}

export interface AcquisitionHistoryItem {
  id: string;
  source_account_id: string;
  content_type: string;
  content_name: string;
  year?: number;
  season?: number;
  episode?: number;
  torrent_title?: string;
  torrent_source?: string;
  quality?: string;
  size_bytes?: number;
  download_id?: string;
  status: 'success' | 'failed' | 'upgraded';
  acquired_from: string;
  upgrade_of?: string;
  created_at: Date;
}

export interface PipelineRunRecord {
  id: number;
  source_account_id: string;
  trigger_type: string;
  trigger_source: string | null;
  content_title: string;
  content_type: string | null;
  status: string;
  vpn_check_status: string;
  torrent_status: string;
  torrent_download_id: string | null;
  metadata_status: string;
  subtitle_status: string;
  encoding_status: string;
  encoding_job_id: string | null;
  publishing_status: string;
  detected_at: Date;
  vpn_checked_at: Date | null;
  torrent_submitted_at: Date | null;
  download_completed_at: Date | null;
  metadata_enriched_at: Date | null;
  subtitles_fetched_at: Date | null;
  encoding_completed_at: Date | null;
  published_at: Date | null;
  pipeline_completed_at: Date | null;
  error_message: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface PipelineTriggerRequest {
  content_title: string;
  content_type?: string;
  magnet_url?: string;
  torrent_url?: string;
}

// =============================================================================
// Download State Machine
// =============================================================================

/**
 * All possible download states.
 *
 * Happy path: created -> vpn_connecting -> searching -> downloading -> encoding
 *             -> subtitles -> uploading -> finalizing -> completed
 *
 * Any state can transition to `failed` or `cancelled`.
 */
export type DownloadState =
  | 'created'
  | 'vpn_connecting'
  | 'searching'
  | 'downloading'
  | 'encoding'
  | 'subtitles'
  | 'uploading'
  | 'finalizing'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'paused';

export interface DownloadStateTransition {
  id: string;
  download_id: string;
  from_state: string | null;
  to_state: string;
  metadata?: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Movie Monitoring
// =============================================================================

export interface MovieMonitoring {
  id: string;
  source_account_id: string;
  user_id: string;
  movie_title: string;
  tmdb_id?: number;
  release_date?: Date;
  digital_release_date?: Date;
  quality_profile: string;
  auto_download: boolean;
  auto_upgrade: boolean;
  status: 'scheduled' | 'searching' | 'downloading' | 'downloaded' | 'failed';
  downloaded_quality?: string;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Downloads (state-machine driven)
// =============================================================================

export interface Download {
  id: string;
  source_account_id: string;
  user_id: string;
  content_type: string;
  title: string;
  state: DownloadState;
  progress: number;
  magnet_uri?: string;
  torrent_id?: string;
  encoding_job_id?: string;
  quality_profile: string;
  retry_count: number;
  error_message?: string;
  show_id?: string;
  season_number?: number;
  episode_number?: number;
  tmdb_id?: number;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Download Rules
// =============================================================================

export interface DownloadRule {
  id: string;
  source_account_id: string;
  user_id: string;
  name: string;
  conditions: Record<string, unknown>;
  action: 'auto-download' | 'notify' | 'skip';
  priority: number;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Download Queue
// =============================================================================

export interface DownloadQueueItem {
  download_id: string;
  priority: number;
  created_at: Date;
}

// =============================================================================
// Quality Profile Presets
// =============================================================================

export interface QualityProfilePreset {
  name: string;
  description: string;
  max_resolution: string;
  min_resolution: string;
  preferred_sources: string[];
  max_size_movie_gb: number;
  max_size_episode_gb: number;
}

// =============================================================================
// Dashboard Summary
// =============================================================================

export interface DashboardSummary {
  active_downloads: number;
  completed_today: number;
  failed_today: number;
  active_subscriptions: number;
  monitored_movies: number;
  enabled_feeds: number;
  enabled_rules: number;
  queue_depth: number;
}

// =============================================================================
// Feed Validation Result
// =============================================================================

export interface FeedValidationResult {
  valid: boolean;
  title?: string;
  item_count?: number;
  latest_item_date?: string;
  error?: string;
}
