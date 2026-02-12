/**
 * ROM Discovery Plugin Types
 * Complete type definitions for ROM metadata, download queue, scrapers, and popularity tracking
 */

// =============================================================================
// Configuration
// =============================================================================

export interface RomDiscoveryConfig {
  /** PostgreSQL connection string */
  database_url: string;
  /** Server port */
  port: number;
  /** Server host */
  host: string;
  /** Enable automated scrapers */
  enable_scrapers: boolean;
  /** Default cron schedule for scrapers */
  scraper_schedule: string;
  /** Minimum quality score filter default */
  default_quality_min: number;
  /** Minimum popularity score filter default */
  default_popularity_min: number;
  /** Max concurrent downloads */
  max_concurrent_downloads: number;
  /** Max download size in MB */
  max_download_size_mb: number;
  /** Retro gaming plugin URL */
  retro_gaming_url: string;
  /** CDN URL for assets */
  cdn_url: string;
  /** Log level */
  log_level: string;
}

// =============================================================================
// Database Record Types
// =============================================================================

export interface RomMetadataRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  rom_title: string;
  rom_title_normalized: string;
  platform: string;
  region: string | null;
  file_name: string;
  file_size_bytes: number | null;
  file_hash_md5: string | null;
  file_hash_sha256: string | null;
  file_hash_crc32: string | null;
  download_url: string | null;
  download_source: string | null;
  download_url_verified_at: Date | null;
  download_url_dead: boolean;
  release_year: number | null;
  release_month: number | null;
  release_day: number | null;
  version: string | null;
  quality_score: number;
  popularity_score: number;
  release_group: string | null;
  is_verified_dump: boolean;
  is_hack: boolean;
  is_translation: boolean;
  is_homebrew: boolean;
  is_public_domain: boolean;
  game_title: string | null;
  genre: string | null;
  publisher: string | null;
  developer: string | null;
  description: string | null;
  igdb_id: number | null;
  mobygames_id: number | null;
  box_art_url: string | null;
  screenshot_urls: string[];
  is_community_rom: boolean;
  community_source_url: string | null;
  community_update_year: number | null;
  scraped_from: string | null;
  scraped_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface DownloadQueueRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  user_id: string | null;
  rom_metadata_id: string;
  job_id: string | null;
  status: 'pending' | 'downloading' | 'verifying' | 'completed' | 'failed' | 'cancelled';
  download_started_at: Date | null;
  download_completed_at: Date | null;
  download_progress_percent: number;
  downloaded_bytes: number;
  total_bytes: number;
  object_storage_path: string | null;
  checksum_verified: boolean;
  error_message: string | null;
  retry_count: number;
  max_retries: number;
  created_at: Date;
  updated_at: Date;
}

export interface ScraperJobRecord {
  [key: string]: unknown;
  id: string;
  scraper_name: string;
  scraper_type: string;
  scraper_source_url: string;
  enabled: boolean;
  last_run_at: Date | null;
  last_run_status: string | null;
  last_run_duration_seconds: number | null;
  roms_found: number;
  roms_added: number;
  roms_updated: number;
  roms_removed: number;
  errors: string[];
  cron_schedule: string;
  next_run_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface PopularityTrackingRecord {
  [key: string]: unknown;
  id: string;
  rom_metadata_id: string;
  download_count: number;
  search_count: number;
  play_count: number;
  archive_org_downloads: number;
  computed_popularity_score: number;
  last_score_update_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface SearchRomsQuery {
  q?: string;
  platform?: string;
  region?: string;
  quality_min?: string;
  popularity_min?: string;
  verified_only?: string;
  homebrew_only?: string;
  community_only?: string;
  show_hacks?: string;
  show_translations?: string;
  genre?: string;
  sort?: 'popularity' | 'quality' | 'title' | 'year';
  order?: 'asc' | 'desc';
  limit?: string;
  offset?: string;
}

export interface DownloadRequest {
  rom_metadata_id: string;
  user_id: string;
}

export interface UpdateScraperRequest {
  enabled?: boolean;
  cron_schedule?: string;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface PlatformStats {
  platform: string;
  rom_count: number;
  verified_count: number;
  homebrew_count: number;
  avg_quality: number;
}

export interface FeaturedRoms {
  most_popular: RomMetadataRecord[];
  verified_dumps: RomMetadataRecord[];
  community_updated: RomMetadataRecord[];
  legal_homebrew: RomMetadataRecord[];
}

export interface RomDiscoveryStats {
  total_roms: number;
  total_platforms: number;
  total_verified: number;
  total_homebrew: number;
  total_community: number;
  total_downloads_queued: number;
  total_downloads_completed: number;
  active_scrapers: number;
  avg_quality_score: number;
  avg_popularity_score: number;
}

// =============================================================================
// Scraper Types
// =============================================================================

export interface ScraperResult {
  roms_found: number;
  roms_added: number;
  roms_updated: number;
  roms_removed: number;
  errors: string[];
  duration_seconds: number;
}

export interface ArchiveOrgItem {
  identifier: string;
  title: string;
  description: string;
  mediatype: string;
  collection: string[];
  downloads: number;
  files: ArchiveOrgFile[];
}

export interface ArchiveOrgFile {
  name: string;
  size: string;
  md5: string;
  sha1: string;
  format: string;
  source: string;
}

export interface ArchiveOrgSearchResponse {
  responseHeader: {
    status: number;
    params: Record<string, string>;
  };
  response: {
    numFound: number;
    start: number;
    docs: ArchiveOrgSearchDoc[];
  };
}

export interface ArchiveOrgSearchDoc {
  identifier: string;
  title: string;
  description?: string;
  mediatype: string;
  collection: string[];
  downloads?: number;
  date?: string;
  creator?: string;
  subject?: string[];
}

export interface ArchiveOrgMetadataResponse {
  metadata: {
    identifier: string;
    title: string;
    description?: string;
    collection?: string | string[];
    date?: string;
    creator?: string;
    subject?: string | string[];
  };
  files: Array<{
    name: string;
    size?: string;
    md5?: string;
    sha1?: string;
    format?: string;
    source?: string;
  }>;
}

// =============================================================================
// Platform Mapping Types
// =============================================================================

export interface PlatformMapping {
  /** Display name for the platform */
  displayName: string;
  /** Archive.org collection identifiers */
  archiveCollections: string[];
  /** ROM file extensions */
  fileExtensions: string[];
  /** No-Intro DAT set name */
  noIntroName: string | null;
  /** Redump set name */
  redumpName: string | null;
}

// =============================================================================
// Legal Compliance & Audit Types
// =============================================================================

export interface AuditLogRecord {
  [key: string]: unknown;
  id: number;
  source_account_id: string;
  user_id: string;
  action: string;
  rom_metadata_id: string | null;
  rom_name: string | null;
  rom_platform: string | null;
  rom_source: string | null;
  ip_address: string | null;
  user_agent: string | null;
  details: Record<string, unknown>;
  created_at: Date;
}

export interface LegalAcceptanceRecord {
  id: number;
  source_account_id: string;
  user_id: string;
  disclaimer_version: string;
  accepted_at: Date;
  ip_address: string | null;
  user_agent: string | null;
}

export interface AcceptDisclaimerRequest {
  user_id: string;
}

export interface AuditLogQuery {
  user_id?: string;
  action?: string;
  from_date?: string;
  to_date?: string;
  limit?: string;
  offset?: string;
}
