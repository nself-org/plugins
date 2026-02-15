/**
 * DDNS Plugin Types
 * Complete type definitions for dynamic DNS configuration and update logging
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface DdnsConfigRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  provider: string;
  domain: string;
  hostname: string;
  token: string;
  api_key: string | null;
  zone_id: string | null;
  record_type: string;
  current_ip: string | null;
  last_check_at: Date | null;
  last_update_at: Date | null;
  check_interval: number;
  is_enabled: boolean;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface DdnsUpdateLogRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  config_id: string;
  provider: string;
  domain: string;
  old_ip: string | null;
  new_ip: string;
  status: 'success' | 'failed' | 'skipped';
  response_code: number | null;
  response_message: string | null;
  error: string | null;
  duration_ms: number;
  created_at: Date;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface CreateDdnsConfigRequest {
  provider: string;
  domain: string;
  hostname?: string;
  token: string;
  api_key?: string;
  zone_id?: string;
  record_type?: string;
  check_interval?: number;
}

export interface UpdateDdnsConfigRequest {
  provider?: string;
  domain?: string;
  hostname?: string;
  token?: string;
  api_key?: string;
  zone_id?: string;
  record_type?: string;
  check_interval?: number;
  is_enabled?: boolean;
}

export interface ForceUpdateRequest {
  config_id?: string;
}

export interface ListConfigsQuery {
  provider?: string;
  is_enabled?: string;
  limit?: number;
  offset?: number;
}

export interface ListHistoryQuery {
  config_id?: string;
  status?: string;
  limit?: number;
  offset?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface StatusResponse {
  configs: Array<{
    id: string;
    provider: string;
    domain: string;
    current_ip: string | null;
    last_check_at: Date | null;
    last_update_at: Date | null;
    is_enabled: boolean;
  }>;
  external_ip: string | null;
}

export interface UpdateResponse {
  config_id: string;
  provider: string;
  domain: string;
  old_ip: string | null;
  new_ip: string;
  status: 'success' | 'failed' | 'skipped';
  message: string;
}

export interface ProviderInfo {
  name: string;
  display_name: string;
  website: string;
  requires_api_key: boolean;
  requires_zone_id: boolean;
  supports_ipv6: boolean;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface DdnsStats {
  total_configs: number;
  enabled_configs: number;
  total_updates: number;
  successful_updates: number;
  failed_updates: number;
  skipped_updates: number;
  last_update_at: Date | null;
  last_check_at: Date | null;
}
