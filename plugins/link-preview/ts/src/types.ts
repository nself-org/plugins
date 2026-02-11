/**
 * Link Preview Plugin Types
 * Complete type definitions for URL metadata extraction, caching, oEmbed, and analytics
 */

// =============================================================================
// Configuration
// =============================================================================

export interface LinkPreviewConfig {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Preview settings
  enabled: boolean;
  cacheTtlHours: number;
  timeoutSeconds: number;
  userAgent: string;
  maxPreviewsPerMessage: number;

  // Fetching
  maxResponseSizeMb: number;
  followRedirects: boolean;
  maxRedirects: number;
  respectRobotsTxt: boolean;

  // oEmbed
  oembedEnabled: boolean;
  oembedDiscovery: boolean;
  oembedMaxWidth: number;
  oembedMaxHeight: number;

  // Safety
  safetyCheckEnabled: boolean;
  phishingDetection: boolean;

  // Rate limiting
  rateLimitPerMinute: number;
  rateLimitPerDomain: number;

  // Security
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;

  logLevel: string;
}

// =============================================================================
// Enum/Union Types
// =============================================================================

export type PreviewStatus = 'success' | 'failed' | 'partial';

export type EmbedType = 'photo' | 'video' | 'rich' | 'link';

export type BlocklistPatternType = 'exact' | 'domain' | 'regex';

export type BlocklistReason = 'spam' | 'phishing' | 'malware' | 'offensive' | 'other';

export type SettingsScope = 'global' | 'channel' | 'user';

export type PreviewPosition = 'top' | 'bottom' | 'inline';

// =============================================================================
// Database Records
// =============================================================================

export interface LinkPreviewRecord {
  id: string;
  source_account_id: string;
  url: string;
  url_hash: string;
  title: string | null;
  description: string | null;
  image_url: string | null;
  video_url: string | null;
  audio_url: string | null;
  site_name: string | null;
  favicon_url: string | null;
  embed_html: string | null;
  embed_type: EmbedType | null;
  provider_name: string | null;
  provider_url: string | null;
  author_name: string | null;
  author_url: string | null;
  published_date: Date | null;
  word_count: number | null;
  reading_time_minutes: number | null;
  tags: string[];
  language: string | null;
  metadata: Record<string, unknown>;
  status: PreviewStatus;
  error_message: string | null;
  cache_expires_at: Date | null;
  fetch_duration_ms: number | null;
  http_status_code: number | null;
  content_type: string | null;
  content_length: number | null;
  is_safe: boolean;
  safety_check_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface LinkPreviewUsageRecord {
  id: string;
  source_account_id: string;
  preview_id: string;
  message_id: string | null;
  user_id: string | null;
  channel_id: string | null;
  clicked: boolean;
  clicked_at: Date | null;
  created_at: Date;
}

export interface LinkPreviewTemplateRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  url_pattern: string;
  priority: number;
  template_html: string;
  css_styles: string | null;
  metadata_extractors: unknown[];
  is_active: boolean;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface OEmbedProviderRecord {
  id: string;
  source_account_id: string;
  provider_name: string;
  provider_url: string;
  endpoint_url: string;
  url_schemes: string[];
  formats: string[];
  discovery: boolean;
  max_width: number | null;
  max_height: number | null;
  is_active: boolean;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface UrlBlocklistRecord {
  id: string;
  source_account_id: string;
  url_pattern: string;
  pattern_type: BlocklistPatternType;
  reason: BlocklistReason;
  description: string | null;
  added_by: string | null;
  expires_at: Date | null;
  created_at: Date;
}

export interface LinkPreviewSettingsRecord {
  id: string;
  source_account_id: string;
  scope: SettingsScope;
  scope_id: string | null;
  enabled: boolean;
  auto_expand: boolean;
  show_images: boolean;
  show_videos: boolean;
  max_previews_per_message: number;
  preview_position: PreviewPosition;
  custom_css: string | null;
  blocked_domains: string[];
  allowed_domains: string[];
  created_at: Date;
  updated_at: Date;
}

export interface LinkPreviewAnalyticsRecord {
  id: string;
  source_account_id: string;
  date: string;
  preview_id: string;
  views_count: number;
  clicks_count: number;
  unique_users_count: number;
  avg_click_rate: number;
  created_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface FetchPreviewRequest {
  url: string;
  force?: boolean;
}

export interface BatchFetchRequest {
  urls: string[];
}

export interface CreateTemplateRequest {
  name: string;
  description?: string;
  url_pattern: string;
  priority?: number;
  template_html: string;
  css_styles?: string;
  metadata_extractors?: unknown[];
}

export interface UpdateTemplateRequest {
  name?: string;
  description?: string;
  url_pattern?: string;
  priority?: number;
  template_html?: string;
  css_styles?: string;
  metadata_extractors?: unknown[];
  is_active?: boolean;
}

export interface AddOEmbedProviderRequest {
  provider_name: string;
  provider_url: string;
  endpoint_url: string;
  url_schemes: string[];
  formats?: string[];
  max_width?: number;
  max_height?: number;
}

export interface UpdateOEmbedProviderRequest {
  endpoint_url?: string;
  url_schemes?: string[];
  formats?: string[];
  max_width?: number;
  max_height?: number;
  is_active?: boolean;
}

export interface AddToBlocklistRequest {
  url_pattern: string;
  pattern_type: BlocklistPatternType;
  reason: BlocklistReason;
  description?: string;
  added_by?: string;
  expires_at?: string;
}

export interface UpdateSettingsRequest {
  scope: SettingsScope;
  scope_id?: string;
  enabled?: boolean;
  auto_expand?: boolean;
  show_images?: boolean;
  show_videos?: boolean;
  max_previews_per_message?: number;
  preview_position?: PreviewPosition;
  custom_css?: string;
  blocked_domains?: string[];
  allowed_domains?: string[];
}

export interface UpsertPreviewRequest {
  url: string;
  url_hash: string;
  title?: string;
  description?: string;
  image_url?: string;
  video_url?: string;
  audio_url?: string;
  site_name?: string;
  favicon_url?: string;
  embed_html?: string;
  embed_type?: EmbedType;
  provider_name?: string;
  provider_url?: string;
  author_name?: string;
  author_url?: string;
  published_date?: string;
  word_count?: number;
  reading_time_minutes?: number;
  tags?: string[];
  language?: string;
  metadata?: Record<string, unknown>;
  status?: PreviewStatus;
  error_message?: string;
  fetch_duration_ms?: number;
  http_status_code?: number;
  content_type?: string;
  content_length?: number;
  is_safe?: boolean;
}

export interface TrackUsageRequest {
  preview_id: string;
  message_id?: string;
  user_id?: string;
  channel_id?: string;
}

// =============================================================================
// Statistics
// =============================================================================

export interface PreviewCacheStats {
  total_previews: number;
  successful: number;
  failed: number;
  expired: number;
  avg_fetch_duration_ms: number;
  oembed_count: number;
  unique_sites: number;
}

export interface PopularPreview extends LinkPreviewRecord {
  usage_count: number;
  unique_users: number;
  click_count: number;
  click_through_rate: number;
}

export interface AnalyticsSummary {
  date: string;
  total_views: number;
  total_clicks: number;
  unique_previews: number;
}
