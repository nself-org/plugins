/**
 * mDNS Plugin Types
 * Complete type definitions for mDNS service discovery and advertising
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface ServiceRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  service_name: string;
  service_type: string;
  port: number;
  host: string;
  domain: string;
  txt_records: Record<string, string>;
  is_advertised: boolean;
  is_active: boolean;
  last_seen_at: Date;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface DiscoveryLogRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  service_type: string;
  service_name: string;
  host: string;
  port: number;
  addresses: string[];
  txt_records: Record<string, string>;
  discovered_at: Date;
  last_seen_at: Date;
  is_available: boolean;
  metadata: Record<string, unknown>;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface AdvertiseServiceRequest {
  service_name: string;
  service_type?: string;
  port: number;
  host?: string;
  domain?: string;
  txt_records?: Record<string, string>;
}

export interface UpdateServiceRequest {
  service_name?: string;
  service_type?: string;
  port?: number;
  host?: string;
  domain?: string;
  txt_records?: Record<string, string>;
  is_advertised?: boolean;
  is_active?: boolean;
}

export interface ListServicesQuery {
  service_type?: string;
  is_advertised?: string;
  is_active?: string;
  limit?: number;
  offset?: number;
}

export interface DiscoverRequest {
  service_type?: string;
  timeout?: number;
  domain?: string;
}

export interface ListDiscoveryQuery {
  service_type?: string;
  is_available?: string;
  limit?: number;
  offset?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface DiscoverResponse {
  services: DiscoveryLogRecord[];
  count: number;
  scan_duration_ms: number;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface MdnsStats {
  total_services: number;
  advertised_services: number;
  active_services: number;
  total_discovered: number;
  available_discovered: number;
  last_discovery_at: Date | null;
}
