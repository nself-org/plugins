/**
 * Devices Plugin Types
 * Complete type definitions for device management, commands, telemetry, and ingest sessions
 */

// =============================================================================
// Device Types
// =============================================================================

export type DeviceStatus = 'unregistered' | 'bootstrap_ready' | 'challenged' | 'enrolled' | 'suspended' | 'revoked';

export type TrustLevel = 'untrusted' | 'pending' | 'trusted' | 'elevated';

export type DeviceType = 'antbox' | 'set_top_box' | 'kiosk' | 'camera' | 'sensor' | 'custom';

export type CommandType =
  | 'tune_channel'
  | 'start_recording'
  | 'stop_recording'
  | 'reboot'
  | 'update_firmware'
  | 'scan_channels'
  | 'get_diagnostics'
  | 'set_config'
  | 'custom'
  // nTV command types
  | 'SCAN_CHANNELS'
  | 'START_EVENT'
  | 'STOP_EVENT'
  | 'HEALTH'
  | 'UPDATE';

export type CommandStatus = 'dispatched' | 'acked' | 'running' | 'succeeded' | 'failed' | 'timeout' | 'cancelled';

export type CommandPriority = 'low' | 'normal' | 'high' | 'critical';

export type TelemetryType =
  | 'heartbeat'
  | 'cpu_usage'
  | 'memory_usage'
  | 'disk_usage'
  | 'temperature'
  | 'signal_strength'
  | 'tuner_status'
  | 'ingest_status'
  | 'error_report';

export type IngestStatus = 'idle' | 'connecting' | 'active' | 'degraded' | 'retrying' | 'stopped' | 'failed';

export type IngestProtocol = 'rtmp' | 'srt' | 'hls' | 'webrtc';

export interface DeviceRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  device_id: string;
  name: string | null;
  device_type: DeviceType;
  model: string | null;
  firmware_version: string | null;
  status: DeviceStatus;
  trust_level: TrustLevel;
  enrollment_token: string | null;
  enrollment_challenge: string | null;
  enrolled_at: Date | null;
  enrolled_by: string | null;
  public_key: string | null;
  last_seen_at: Date | null;
  last_ip: string | null;
  capabilities: string[];
  config: Record<string, unknown>;
  labels: Record<string, unknown>;
  metadata: Record<string, unknown>;
  revoked_at: Date | null;
  revoked_by: string | null;
  revoke_reason: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CommandRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  device_id: string;
  command_type: CommandType;
  command_id: string;
  payload: Record<string, unknown>;
  status: CommandStatus;
  priority: CommandPriority;
  dispatched_at: Date;
  acked_at: Date | null;
  started_at: Date | null;
  completed_at: Date | null;
  result: Record<string, unknown> | null;
  error: string | null;
  timeout_seconds: number;
  deadline: Date | null;
  retry_count: number;
  max_retries: number;
  idempotency_key: string;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface TelemetryRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  device_id: string;
  telemetry_type: TelemetryType;
  data: Record<string, unknown>;
  recorded_at: Date;
  received_at: Date;
}

export interface IngestSessionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  device_id: string;
  stream_id: string;
  status: IngestStatus;
  ingest_url: string | null;
  protocol: IngestProtocol;
  channel: string | null;
  quality: string | null;
  bitrate_kbps: number | null;
  fps: number | null;
  resolution: string | null;
  started_at: Date | null;
  last_heartbeat_at: Date | null;
  ended_at: Date | null;
  bytes_ingested: number;
  frames_dropped: number;
  error_count: number;
  last_error: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface AuditLogRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  device_id: string | null;
  action: string;
  actor_id: string | null;
  details: Record<string, unknown>;
  created_at: Date;
}

export interface BootstrapTokenRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  token: string;
  capabilities: string[];
  expires_at: Date;
  used: boolean;
  used_by_device_id: string | null;
  created_at: Date;
}

// =============================================================================
// nTV v1 Request Types
// =============================================================================

export interface CreateBootstrapTokenRequest {
  name: string;
  capabilities?: string[];
}

export interface EnrollDeviceRequest {
  token: string;
  name: string;
  public_key: string;
}

export interface DeviceHeartbeatRequest {
  cpu_usage?: number;
  memory_usage?: number;
  temperature?: number;
  disk_usage?: number;
  signal_quality?: number;
}

export interface SendCommandRequest {
  type: CommandType;
  payload?: Record<string, unknown>;
}

// =============================================================================
// Request Types
// =============================================================================

export interface RegisterDeviceRequest {
  device_id: string;
  name?: string;
  device_type: DeviceType;
  model?: string;
  firmware_version?: string;
  capabilities?: string[];
  config?: Record<string, unknown>;
  labels?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface UpdateDeviceRequest {
  name?: string;
  firmware_version?: string;
  capabilities?: string[];
  config?: Record<string, unknown>;
  labels?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface EnrollmentResponse {
  device_id: string;
  enrollment_token: string;
  enrollment_challenge: string;
  expires_at: string;
}

export interface ChallengeResponse {
  device_id: string;
  challenge_response: string;
  public_key: string;
}

export interface DispatchCommandRequest {
  device_id: string;
  command_type: CommandType;
  payload?: Record<string, unknown>;
  priority?: CommandPriority;
  timeout_seconds?: number;
  idempotency_key?: string;
  metadata?: Record<string, unknown>;
}

export interface SubmitTelemetryRequest {
  telemetry_type: TelemetryType;
  data: Record<string, unknown>;
  recorded_at?: string;
}

export interface StartIngestRequest {
  device_id: string;
  stream_id: string;
  protocol?: IngestProtocol;
  channel?: string;
  quality?: string;
  metadata?: Record<string, unknown>;
}

export interface IngestHeartbeatRequest {
  bytes_ingested?: number;
  frames_dropped?: number;
  bitrate_kbps?: number;
  fps?: number;
  resolution?: string;
  error_count?: number;
  last_error?: string;
}

export interface RevokeRequest {
  reason: string;
  actor_id?: string;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface FleetStats {
  total_devices: number;
  enrolled_devices: number;
  online_devices: number;
  suspended_devices: number;
  revoked_devices: number;
  total_commands: number;
  pending_commands: number;
  succeeded_commands: number;
  failed_commands: number;
  active_ingest_sessions: number;
  total_telemetry_records: number;
  last_activity: Date | null;
}

export interface DeviceHealth {
  device_id: string;
  name: string | null;
  status: DeviceStatus;
  trust_level: TrustLevel;
  last_seen_at: Date | null;
  recent_telemetry: TelemetryRecord[];
  pending_commands: number;
  active_ingest_sessions: number;
}
