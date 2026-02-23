/**
 * VPN Plugin Types
 * Comprehensive type definitions for multi-provider VPN management
 */

// ============================================================================
// Provider Types
// ============================================================================

export type VPNProvider =
  | 'nordvpn'
  | 'pia'
  | 'mullvad'
  | 'surfshark'
  | 'expressvpn'
  | 'protonvpn'
  | 'keepsolid'
  | 'cyberghost'
  | 'airvpn'
  | 'windscribe';

export type VPNProtocol = 'wireguard' | 'openvpn_udp' | 'openvpn_tcp' | 'ikev2' | 'nordlynx' | 'lightway';

export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'disconnecting' | 'reconnecting' | 'failed';

export type DownloadStatus = 'queued' | 'connecting_vpn' | 'downloading' | 'paused' | 'completed' | 'failed' | 'cancelled';

// ============================================================================
// Database Record Types
// ============================================================================

export interface VPNProviderRecord {
  id: string;
  name: string;
  display_name: string;
  cli_available: boolean;
  cli_command?: string;
  api_available: boolean;
  api_endpoint?: string;
  port_forwarding_supported: boolean;
  p2p_all_servers: boolean;
  p2p_server_count: number;
  total_servers: number;
  total_countries: number;
  wireguard_supported: boolean;
  openvpn_supported: boolean;
  kill_switch_available: boolean;
  split_tunneling_available: boolean;
  config: VPNProviderConfig;
  created_at: Date;
  updated_at: Date;
}

export interface VPNProviderConfig {
  api_url?: string;
  config_url?: string;
  supports_udp?: boolean;
  supports_tcp?: boolean;
  supports_wireguard?: boolean;
  supports_openvpn?: boolean;
  default_port?: number;
  [key: string]: unknown;
}

export interface VPNCredentialRecord {
  id: string;
  provider_id: string;
  username?: string;
  password_encrypted?: string;
  api_key_encrypted?: string;
  api_token_encrypted?: string;
  account_number?: string; // For Mullvad
  private_key_encrypted?: string; // For WireGuard
  additional_data?: Record<string, unknown>;
  expires_at?: Date;
  created_at: Date;
  updated_at: Date;
}

export interface VPNServerRecord {
  id: string;
  provider_id: string;
  hostname: string;
  ip_address: string;
  ipv6_address?: string;
  country_code: string;
  country_name: string;
  city?: string;
  region?: string;
  latitude?: number;
  longitude?: number;
  p2p_supported: boolean;
  port_forwarding_supported: boolean;
  protocols: VPNProtocol[];
  load?: number; // Current server load (0-100)
  capacity?: number;
  status: 'online' | 'offline' | 'maintenance';
  features: string[]; // e.g., ['p2p', 'double_vpn', 'obfuscated']
  public_key?: string; // WireGuard public key
  endpoint_port?: number;
  owned: boolean; // Provider owns the hardware
  metadata: VPNServerMetadata;
  last_seen: Date;
  created_at: Date;
  updated_at: Date;
}

export interface VPNServerMetadata {
  provider_server_id?: string;
  asn?: number;
  dns_servers?: string[];
  recommended?: boolean;
  premium_only?: boolean;
  [key: string]: unknown;
}

export interface VPNConnectionRecord {
  id: string;
  provider_id: string;
  server_id?: string;
  protocol: VPNProtocol;
  status: ConnectionStatus;
  local_ip?: string;
  vpn_ip?: string;
  interface_name?: string; // e.g., tun0, wg0
  dns_servers?: string[];
  connected_at?: Date;
  disconnected_at?: Date;
  duration_seconds?: number;
  bytes_sent?: bigint;
  bytes_received?: bigint;
  error_message?: string;
  kill_switch_enabled: boolean;
  port_forwarded?: number;
  requested_by?: string; // Plugin or user that requested connection
  metadata: VPNConnectionMetadata;
  created_at: Date;
}

export interface VPNConnectionMetadata {
  disconnect_reason?: string;
  reconnect_count?: number;
  client_version?: string;
  [key: string]: unknown;
}

export interface VPNDownloadRecord {
  id: string;
  connection_id?: string;
  magnet_link: string;
  info_hash: string;
  name?: string;
  destination_path: string;
  status: DownloadStatus;
  progress: number; // 0-100
  bytes_downloaded: bigint;
  bytes_total?: bigint;
  download_speed?: number; // bytes/second
  upload_speed?: number;
  peers: number;
  seeds: number;
  eta_seconds?: number;
  requested_by: string; // Plugin that requested download
  provider_id: string;
  server_id?: string;
  started_at?: Date;
  completed_at?: Date;
  error_message?: string;
  metadata: VPNDownloadMetadata;
  created_at: Date;
}

export interface VPNDownloadMetadata {
  content_type?: string;
  category?: string;
  release_group?: string;
  [key: string]: unknown;
}

export interface VPNConnectionLogRecord {
  id: string;
  connection_id: string;
  timestamp: Date;
  event_type: string; // 'connected', 'disconnected', 'error', 'reconnecting', etc.
  message: string;
  details?: Record<string, unknown>;
}

export interface VPNServerPerformanceRecord {
  id: string;
  server_id: string;
  timestamp: Date;
  ping_ms?: number;
  download_speed_mbps?: number;
  upload_speed_mbps?: number;
  load_percentage?: number;
  success_rate?: number; // Connection success rate (0-1)
  avg_connection_time_ms?: number;
}

export interface VPNLeakTestRecord {
  id: string;
  connection_id: string;
  test_type: 'dns' | 'ip' | 'webrtc' | 'ipv6';
  passed: boolean;
  expected_value?: string;
  actual_value?: string;
  details?: Record<string, unknown>;
  tested_at: Date;
}

// ============================================================================
// API Types
// ============================================================================

export interface ConnectVPNRequest {
  provider: VPNProvider;
  region?: string; // Country code or region
  city?: string;
  server?: string; // Specific server hostname
  protocol?: VPNProtocol;
  kill_switch?: boolean;
  port_forwarding?: boolean;
  requested_by?: string;
}

export interface ConnectVPNResponse {
  connection_id: string;
  provider: string;
  server: string;
  vpn_ip: string;
  interface: string;
  dns_servers: string[];
  port_forwarded?: number;
  connected_at: Date;
}

export interface DownloadRequest {
  magnet_link: string;
  destination?: string;
  provider?: VPNProvider;
  region?: string;
  requested_by: string;
}

export interface DownloadResponse {
  download_id: string;
  name?: string;
  status: DownloadStatus;
  provider: string;
  server?: string;
  created_at: Date;
}

export interface DownloadProgress {
  id: string;
  name?: string;
  status: DownloadStatus;
  progress: number;
  bytes_downloaded: string;
  bytes_total?: string;
  download_speed: number;
  upload_speed: number;
  peers: number;
  seeds: number;
  eta_seconds?: number;
  started_at?: Date;
  completed_at?: Date;
  error_message?: string;
}

export interface VPNStatus {
  connected: boolean;
  connection_id?: string;
  provider?: string;
  server?: string;
  vpn_ip?: string;
  interface?: string;
  protocol?: string;
  uptime_seconds?: number;
  bytes_sent?: string;
  bytes_received?: string;
  port_forwarded?: number;
  kill_switch_enabled?: boolean;
}

export interface ServerListQuery {
  provider?: VPNProvider;
  country?: string;
  city?: string;
  p2p_only?: boolean;
  port_forwarding?: boolean;
  protocol?: VPNProtocol;
  limit?: number;
  sort_by?: 'load' | 'latency' | 'performance';
}

export interface LeakTestResult {
  passed: boolean;
  tests: {
    dns: { passed: boolean; expected?: string; actual?: string };
    ip: { passed: boolean; expected?: string; actual?: string };
    webrtc: { passed: boolean; leaked_ips?: string[] };
    ipv6: { passed: boolean; leaked_ip?: string };
  };
  timestamp: Date;
}

// ============================================================================
// Provider Interface (abstraction for all VPN providers)
// ============================================================================

export interface IVPNProvider {
  readonly name: VPNProvider;
  readonly displayName: string;
  readonly cliAvailable: boolean;
  readonly apiAvailable: boolean;
  readonly portForwardingSupported: boolean;
  readonly p2pAllServers: boolean;

  /**
   * Initialize provider (check CLI, verify credentials)
   */
  initialize(): Promise<void>;

  /**
   * Authenticate with provider
   */
  authenticate(credentials: VPNCredentialRecord): Promise<boolean>;

  /**
   * Fetch latest server list from provider
   */
  fetchServers(): Promise<VPNServerRecord[]>;

  /**
   * Connect to VPN
   */
  connect(request: ConnectVPNRequest, credentials: VPNCredentialRecord): Promise<VPNConnectionRecord>;

  /**
   * Disconnect from VPN
   */
  disconnect(connectionId: string): Promise<void>;

  /**
   * Get current connection status
   */
  getStatus(): Promise<VPNStatus>;

  /**
   * Enable kill switch
   */
  enableKillSwitch(): Promise<void>;

  /**
   * Disable kill switch
   */
  disableKillSwitch(): Promise<void>;

  /**
   * Get forwarded port (if supported)
   */
  getForwardedPort?(): Promise<number | null>;

  /**
   * Test for leaks
   */
  testLeaks(): Promise<LeakTestResult>;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface VPNPluginConfig {
  database_url: string;
  default_provider?: VPNProvider;
  default_region?: string;
  download_path: string;
  enable_kill_switch: boolean;
  enable_auto_reconnect: boolean;
  server_carousel_enabled: boolean;
  carousel_interval_minutes: number;
  port: number;
  log_level: 'debug' | 'info' | 'warn' | 'error';
  torrent_manager_url: string;
  internal_api_key?: string;
}

// ============================================================================
// Provider Config Types (for provider-specific settings)
// ============================================================================

export interface NordVPNConfig {
  access_token?: string;
  cli_path?: string;
}

export interface PIAConfig {
  username?: string;
  password?: string;
  port_forwarding?: boolean;
}

export interface MullvadConfig {
  account_number?: string;
  device_name?: string;
}

// ============================================================================
// Torrent Types
// ============================================================================

export interface TorrentInfo {
  infoHash: string;
  name?: string;
  length?: number;
  files?: Array<{
    name: string;
    length: number;
    path: string;
  }>;
}

export interface TorrentProgress {
  progress: number;
  downloaded: number;
  total?: number;
  downloadSpeed: number;
  uploadSpeed: number;
  numPeers: number;
  ratio: number;
}

// ============================================================================
// Server Carousel Types
// ============================================================================

export interface CarouselState {
  enabled: boolean;
  interval_minutes: number;
  current_server_id?: string;
  next_rotation_at?: Date;
  rotation_count: number;
}

export interface ServerRotationEvent {
  from_server_id?: string;
  to_server_id: string;
  reason: 'scheduled' | 'manual' | 'performance' | 'failure';
  timestamp: Date;
}

// ============================================================================
// Statistics Types
// ============================================================================

export interface VPNStatistics {
  total_connections: number;
  active_connections: number;
  total_downloads: number;
  active_downloads: number;
  total_bytes_downloaded: string;
  providers: Array<{
    provider: string;
    connections: number;
    uptime_percentage: number;
    avg_speed_mbps: number;
  }>;
  top_servers: Array<{
    server: string;
    provider: string;
    country: string;
    connections: number;
    avg_speed_mbps: number;
  }>;
}

// ============================================================================
// Utility Types
// ============================================================================

export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

export type RequireAtLeastOne<T, Keys extends keyof T = keyof T> = Pick<T, Exclude<keyof T, Keys>> &
  {
    [K in Keys]-?: Required<Pick<T, K>> & Partial<Pick<T, Exclude<Keys, K>>>;
  }[Keys];
