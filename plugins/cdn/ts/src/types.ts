/**
 * CDN Plugin Types
 * Complete type definitions for CDN zones, purging, analytics, and signed URLs
 */

// =============================================================================
// Enums and Literals
// =============================================================================

export type CdnProvider = 'cloudflare' | 'bunnycdn' | 'fastly' | 'akamai';

export type PurgeType = 'urls' | 'tags' | 'prefixes' | 'all';

export type PurgeStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

export type ZoneStatus = 'active' | 'inactive' | 'pending' | 'suspended';

// =============================================================================
// Database Record Types
// =============================================================================

export interface ZoneRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  provider: CdnProvider;
  zone_id: string;
  name: string;
  domain: string;
  origin_url: string | null;
  ssl_enabled: boolean;
  cache_ttl: number;
  status: ZoneStatus;
  config: Record<string, unknown>;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface PurgeRequestRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  zone_id: string;
  purge_type: PurgeType;
  urls: string[];
  tags: string[];
  prefixes: string[];
  status: PurgeStatus;
  provider_request_id: string | null;
  requested_by: string | null;
  completed_at: Date | null;
  error: string | null;
  created_at: Date;
}

export interface AnalyticsRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  zone_id: string;
  date: Date;
  requests_total: number;
  requests_cached: number;
  bandwidth_total: number;
  bandwidth_cached: number;
  unique_visitors: number;
  threats_blocked: number;
  status_2xx: number;
  status_3xx: number;
  status_4xx: number;
  status_5xx: number;
  top_paths: unknown[];
  top_countries: unknown[];
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface SignedUrlRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  zone_id: string;
  original_url: string;
  signed_url: string;
  expires_at: Date;
  ip_restriction: string | null;
  access_count: number;
  max_access: number | null;
  created_at: Date;
}

// =============================================================================
// Request Types
// =============================================================================

export interface CreateZoneRequest {
  provider: CdnProvider;
  zone_id: string;
  name: string;
  domain: string;
  origin_url?: string;
  ssl_enabled?: boolean;
  cache_ttl?: number;
  config?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface PurgeRequest {
  zone_id: string;
  purge_type?: PurgeType;
  urls?: string[];
  tags?: string[];
  prefixes?: string[];
  requested_by?: string;
}

export interface PurgeAllRequest {
  zone_id: string;
  confirm?: boolean;
  requested_by?: string;
}

export interface SignUrlRequest {
  zone_id: string;
  url: string;
  ttl?: number;
  ip_restriction?: string;
  max_access?: number;
}

export interface BatchSignRequest {
  zone_id: string;
  urls: string[];
  ttl?: number;
}

export interface AnalyticsQueryRequest {
  zone_id?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export interface SyncAnalyticsRequest {
  zone_id?: string;
  from?: string;
  to?: string;
}

export interface UpsertAnalyticsRequest {
  zone_id: string;
  date: string;
  requests_total?: number;
  requests_cached?: number;
  bandwidth_total?: number;
  bandwidth_cached?: number;
  unique_visitors?: number;
  threats_blocked?: number;
  status_2xx?: number;
  status_3xx?: number;
  status_4xx?: number;
  status_5xx?: number;
  top_paths?: unknown[];
  top_countries?: unknown[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Response Types
// =============================================================================

export interface AnalyticsSummary {
  zone_id: string;
  zone_name: string;
  domain: string;
  total_requests: number;
  cached_requests: number;
  cache_hit_rate: number;
  total_bandwidth: number;
  cached_bandwidth: number;
  total_visitors: number;
  total_4xx: number;
  total_5xx: number;
  days_covered: number;
}

export interface PluginStats {
  total_zones: number;
  active_zones: number;
  total_purge_requests: number;
  pending_purges: number;
  total_signed_urls: number;
  active_signed_urls: number;
  analytics_days_tracked: number;
  total_requests_tracked: number;
  total_bandwidth_tracked: number;
  by_provider: Record<string, number>;
}
