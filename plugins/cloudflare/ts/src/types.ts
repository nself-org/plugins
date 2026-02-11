/**
 * Cloudflare Plugin Types
 * All TypeScript interfaces for the cloudflare plugin
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface CfZoneRecord {
  id: string;
  source_account_id: string;
  name: string;
  status: string | null;
  type: string | null;
  name_servers: string[] | null;
  plan: Record<string, unknown> | null;
  settings: Record<string, unknown>;
  ssl_status: string | null;
  synced_at: Date;
}

export interface CfDnsRecord {
  id: string;
  source_account_id: string;
  zone_id: string;
  type: string;
  name: string;
  content: string;
  ttl: number;
  proxied: boolean;
  priority: number | null;
  locked: boolean;
  synced_at: Date;
}

export interface CfR2BucketRecord {
  id: string;
  source_account_id: string;
  name: string;
  location: string | null;
  storage_class: string;
  object_count: number;
  total_size_bytes: number;
  created_at: Date | null;
  synced_at: Date;
}

export interface CfCachePurgeRecord {
  id: string;
  source_account_id: string;
  zone_id: string;
  purge_type: string;
  urls: string[] | null;
  tags: string[] | null;
  hosts: string[] | null;
  prefixes: string[] | null;
  status: string;
  cf_response: Record<string, unknown> | null;
  created_at: Date;
}

export interface CfAnalyticsRecord {
  id: string;
  source_account_id: string;
  zone_id: string;
  date: Date;
  requests_total: number;
  requests_cached: number;
  requests_uncached: number;
  bandwidth_total: number;
  bandwidth_cached: number;
  threats_total: number;
  unique_visitors: number;
  status_codes: Record<string, unknown>;
  countries: Record<string, unknown>;
  synced_at: Date;
}

export interface CfWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface CreateDnsRecordRequest {
  zoneId: string;
  type: string;
  name: string;
  content: string;
  ttl?: number;
  priority?: number;
  proxied?: boolean;
}

export interface UpdateDnsRecordRequest {
  type?: string;
  name?: string;
  content?: string;
  ttl?: number;
  priority?: number;
  proxied?: boolean;
}

export interface PurgeCacheRequest {
  type: 'all' | 'urls' | 'tags' | 'hosts' | 'prefixes';
  urls?: string[];
  tags?: string[];
  hosts?: string[];
  prefixes?: string[];
}

export interface CreateR2BucketRequest {
  name: string;
  location?: string;
}

export interface SyncRequest {
  resources?: Array<'zones' | 'dns' | 'r2' | 'analytics'>;
}

export interface SyncResponse {
  synced: Record<string, number>;
  errors: string[];
  duration: number;
}

export interface GetAnalyticsQuery {
  from?: string;
  to?: string;
}

export interface AnalyticsResponse {
  daily: CfAnalyticsRecord[];
  totals: {
    requests: number;
    bandwidth: number;
    cached: number;
    threats: number;
    uniqueVisitors: number;
  };
}

export interface CacheStatsResponse {
  hitRate: number;
  totalRequests: number;
  cachedRequests: number;
  uncachedRequests: number;
}

// ============================================================================
// Cloudflare API Response Types
// ============================================================================

export interface CfApiZone {
  id: string;
  name: string;
  status: string;
  type: string;
  name_servers: string[];
  plan: {
    id: string;
    name: string;
    price: number;
    currency: string;
  };
  settings: Record<string, unknown>;
  ssl?: {
    status: string;
  };
}

export interface CfApiDnsRecord {
  id: string;
  zone_id: string;
  type: string;
  name: string;
  content: string;
  ttl: number;
  proxied: boolean;
  priority?: number;
  locked: boolean;
}

export interface CfApiR2Bucket {
  name: string;
  location?: string;
  creation_date?: string;
}

export interface CfApiResponse<T> {
  result: T;
  success: boolean;
  errors: Array<{ code: number; message: string }>;
  messages: string[];
  result_info?: {
    page: number;
    per_page: number;
    total_pages: number;
    count: number;
    total_count: number;
  };
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface CloudflareConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  appIds: string[];

  // Cloudflare API
  apiToken: string;
  apiKey: string;
  apiEmail: string;
  accountId: string;

  // Zone filtering
  zoneIds: string[];

  // R2
  r2AccessKey: string;
  r2SecretKey: string;

  // Sync
  syncInterval: number;
}

// ============================================================================
// Stats Types
// ============================================================================

export interface CloudflareStats {
  totalZones: number;
  totalDnsRecords: number;
  totalR2Buckets: number;
  totalCachePurges: number;
  totalAnalyticsRecords: number;
  lastSyncedAt: string | null;
}

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}
