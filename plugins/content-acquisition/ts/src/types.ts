/**
 * Content Acquisition Types
 */

export interface ContentAcquisitionConfig {
  database_url: string;
  port: number;
  metadata_enrichment_url: string;
  torrent_manager_url: string;
  vpn_manager_url: string;
  redis_host: string;
  redis_port: number;
  log_level: string;
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

export interface Subscription {
  id: string;
  source_account_id: string;
  subscription_type: 'tv_show' | 'movie_collection' | 'artist' | 'podcast';
  content_id?: string;
  content_name: string;
  content_metadata?: Record<string, any>;
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
  matched_torrent?: Record<string, any>;
  download_id?: string;
  error_message?: string;
  created_at: Date;
  started_at?: Date;
  completed_at?: Date;
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
