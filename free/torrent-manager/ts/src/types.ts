/**
 * Torrent Manager Types
 * Complete TypeScript interfaces for torrent management
 */

// ============================================================================
// Configuration
// ============================================================================

export interface TorrentManagerConfig {
  database_url: string;
  port: number;
  vpn_manager_url: string;
  vpn_required: boolean;
  default_client: TorrentClientType;
  transmission_host: string;
  transmission_port: number;
  transmission_username?: string;
  transmission_password?: string;
  qbittorrent_host: string;
  qbittorrent_port: number;
  qbittorrent_username?: string;
  qbittorrent_password?: string;
  download_path: string;
  enabled_sources?: string;  // Comma-separated list of enabled search sources
  search_enabled_sources: string[];
  search_timeout_ms: number;
  search_cache_ttl_seconds: number;
  seeding_ratio_limit: number;
  seeding_time_limit_hours: number;
  max_active_downloads: number;
  log_level: string;
}

// ============================================================================
// Torrent Clients
// ============================================================================

export type TorrentClientType = 'transmission' | 'qbittorrent';

export type TorrentClientStatus = 'connected' | 'disconnected' | 'error';

export interface TorrentClient {
  id: string;
  source_account_id: string;
  client_type: TorrentClientType;
  host: string;
  port: number;
  username?: string;
  password_encrypted?: string;
  is_default: boolean;
  status: TorrentClientStatus;
  last_connected_at?: Date;
  last_error?: string;
  created_at: Date;
  updated_at: Date;
}

export interface TorrentClientAdapter {
  readonly type: TorrentClientType;
  connect(): Promise<boolean>;
  disconnect(): Promise<void>;
  isConnected(): Promise<boolean>;
  addTorrent(magnetUri: string, options: AddTorrentOptions): Promise<TorrentDownload>;
  getTorrent(id: string): Promise<TorrentDownload | null>;
  listTorrents(filter?: TorrentFilter): Promise<TorrentDownload[]>;
  pauseTorrent(id: string): Promise<void>;
  resumeTorrent(id: string): Promise<void>;
  removeTorrent(id: string, deleteFiles: boolean): Promise<void>;
  getStats(): Promise<TorrentClientStats>;
}

export interface AddTorrentOptions {
  download_path?: string;
  category?: TorrentCategory;
  paused?: boolean;
  priority?: number;
}

export interface TorrentFilter {
  status?: TorrentDownloadStatus;
  category?: TorrentCategory;
}

export interface TorrentClientStats {
  total_torrents: number;
  active_torrents: number;
  paused_torrents: number;
  seeding_torrents: number;
  download_speed_bytes: number;
  upload_speed_bytes: number;
  downloaded_bytes: number;
  uploaded_bytes: number;
}

// ============================================================================
// Torrent Downloads
// ============================================================================

export type TorrentDownloadStatus =
  | 'queued'
  | 'downloading'
  | 'paused'
  | 'completed'
  | 'seeding'
  | 'failed'
  | 'removed';

export type TorrentCategory = 'movie' | 'tv' | 'music' | 'podcast' | 'other';

export interface TorrentMetadata {
  title?: string;
  year?: number;
  season?: number;
  episode?: number;
  quality?: string;
  codec?: string;
  source?: string;
  release_group?: string;
  [key: string]: unknown;
}

export interface TorrentDownload {
  id: string;
  source_account_id: string;
  client_id: string;
  client_torrent_id: string;

  // Torrent Info
  name: string;
  info_hash: string;
  magnet_uri: string;

  // Status
  status: TorrentDownloadStatus;
  category: TorrentCategory;

  // Progress
  size_bytes: number;
  downloaded_bytes: number;
  uploaded_bytes: number;
  progress_percent: number;
  ratio: number;

  // Speed
  download_speed_bytes: number;
  upload_speed_bytes: number;

  // Peers
  seeders: number;
  leechers: number;
  peers_connected: number;

  // Files
  download_path: string;
  files_count: number;

  // Seeding Policy
  stop_at_ratio?: number;
  stop_at_time_hours?: number;

  // VPN
  vpn_ip?: string;
  vpn_interface?: string;

  // Error tracking
  error_message?: string;

  // Metadata
  content_id?: string;
  requested_by: string;
  metadata?: TorrentMetadata;

  // Timestamps
  added_at: Date;
  started_at?: Date;
  completed_at?: Date;
  stopped_at?: Date;

  created_at: Date;
  updated_at: Date;
}

// ============================================================================
// Torrent Search
// ============================================================================

export type TorrentSourceType = '1337x' | 'thepiratebay' | 'rarbg' | 'yts' | 'eztv' | 'kickass';

export interface TorrentSource {
  id: string;
  source_account_id: string;
  source_name: TorrentSourceType;
  base_url: string;
  is_active: boolean;
  priority: number;
  requires_proxy: boolean;
  last_success_at?: Date;
  last_failure_at?: Date;
  failure_count: number;
  created_at: Date;
  updated_at: Date;
}

export interface TorrentSearchQuery {
  query: string;
  category?: TorrentCategory;
  min_seeders?: number;
  min_size_mb?: number;
  max_size_mb?: number;
  quality?: string;
  sources?: TorrentSourceType[];
}

export interface TorrentSearchResult {
  source: TorrentSourceType;
  name: string;
  magnet_uri?: string;
  torrent_url?: string;
  info_hash: string;
  size_bytes: number;
  seeders: number;
  leechers: number;
  upload_date?: Date;
  uploader?: string;
  trusted_uploader: boolean;
  category: TorrentCategory;
  quality?: string;
  score: number;
}

export interface TorrentSearchCache {
  id: string;
  source_account_id: string;
  query_hash: string;
  query: string;
  results: TorrentSearchResult[];
  results_count: number;
  sources_searched: TorrentSourceType[];
  search_duration_ms: number;
  cached_at: Date;
  expires_at: Date;
  created_at: Date;
}

// ============================================================================
// Torrent Files
// ============================================================================

export interface TorrentFile {
  id: string;
  download_id: string;
  source_account_id: string;
  file_index: number;
  file_name: string;
  file_path: string;
  size_bytes: number;
  downloaded_bytes: number;
  progress_percent: number;
  priority: number;
  is_selected: boolean;
  created_at: Date;
  updated_at: Date;
}

// ============================================================================
// Trackers
// ============================================================================

export interface TorrentTracker {
  id: string;
  download_id: string;
  source_account_id: string;
  tracker_url: string;
  tier: number;
  status: string;
  seeders?: number;
  leechers?: number;
  last_announce_at?: Date;
  last_scrape_at?: Date;
  created_at: Date;
  updated_at: Date;
}

// ============================================================================
// Seeding Policy
// ============================================================================

export interface SeedingPolicy {
  id: string;
  source_account_id: string;
  policy_name: string;
  description?: string;

  // Ratio Limits
  ratio_limit?: number;
  ratio_action: 'stop' | 'pause' | 'remove';

  // Time Limits
  time_limit_hours?: number;
  time_action: 'stop' | 'pause' | 'remove';

  // Size Limits
  max_seeding_size_gb?: number;

  // Category Rules
  applies_to_categories: TorrentCategory[];

  // Priority
  priority: number;
  is_active: boolean;

  created_at: Date;
  updated_at: Date;
}

// ============================================================================
// Per-Download Seeding Policy
// ============================================================================

export interface DownloadSeedingPolicy {
  id: string;
  source_account_id: string;
  download_id: string;
  ratio_limit: number;
  time_limit_hours: number;
  auto_remove: boolean;
  keep_files: boolean;
  favorite: boolean;
  created_at: Date;
  updated_at: Date;
}

// ============================================================================
// Source Registry
// ============================================================================

export interface SourceRegistryEntry {
  name: string;
  active_from: string;
  retired_at: string | null;
  category: string;
  trust_score: number;
  strengths: string[];
}

// ============================================================================
// Statistics
// ============================================================================

export interface TorrentStats {
  total_downloads: number;
  active_downloads: number;
  completed_downloads: number;
  failed_downloads: number;
  seeding_torrents: number;

  total_downloaded_bytes: number;
  total_uploaded_bytes: number;
  overall_ratio: number;

  download_speed_bytes: number;
  upload_speed_bytes: number;

  disk_space_used_bytes: number;
  disk_space_available_bytes: number;
}

// ============================================================================
// VPN Integration
// ============================================================================

export interface VPNStatus {
  connected: boolean;
  provider?: string;
  server?: string;
  ip?: string;
  interface?: string;
}

// ============================================================================
// Webhook Events
// ============================================================================

export interface WebhookEvent {
  event: string;
  data: Record<string, unknown>;
  timestamp: Date;
}

// ============================================================================
// API Responses
// ============================================================================

export interface APIResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
}

export interface PaginatedResponse<T = unknown> {
  items: T[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}
