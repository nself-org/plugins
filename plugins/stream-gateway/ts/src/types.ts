/**
 * Stream Gateway Plugin Types
 * Complete type definitions for stream admission, sessions, rules, and analytics
 */

// =============================================================================
// Session Types
// =============================================================================

export type SessionStatus = 'active' | 'ended' | 'evicted' | 'denied';

export type StreamType = 'live' | 'vod' | 'dvr' | 'timeshift';

export type StreamStatus = 'inactive' | 'active' | 'ended';

export type RuleType = 'concurrent_limit' | 'device_limit' | 'geo_block' | 'time_window' | 'user_role';

export type RuleAction = 'allow' | 'deny';

export type DenialReason = 'concurrent_limit' | 'device_limit' | 'geo_blocked' | 'time_window' | 'user_role' | 'stream_full' | 'not_found';

export interface StreamSessionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  stream_id: string;
  stream_type: StreamType;
  user_id: string;
  device_id: string | null;
  device_type: string | null;
  status: SessionStatus;
  quality: string;
  started_at: Date;
  last_heartbeat_at: Date;
  ended_at: Date | null;
  duration_seconds: number | null;
  bytes_transferred: number;
  denial_reason: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface StreamRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  stream_id: string;
  title: string | null;
  stream_type: StreamType;
  status: StreamStatus;
  source_device_id: string | null;
  ingest_url: string | null;
  playback_url: string | null;
  thumbnail_url: string | null;
  max_viewers: number | null;
  current_viewers: number;
  total_viewers: number;
  peak_viewers: number;
  started_at: Date | null;
  ended_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface AdmissionRuleRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  name: string;
  rule_type: RuleType;
  conditions: Record<string, unknown>;
  action: RuleAction;
  priority: number;
  active: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface ViewerAnalyticsRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  stream_id: string;
  period_start: Date;
  period_end: Date;
  avg_viewers: number | null;
  peak_viewers: number | null;
  unique_viewers: number | null;
  total_view_minutes: number | null;
  quality_distribution: Record<string, unknown>;
  device_distribution: Record<string, unknown>;
  created_at: Date;
}

// =============================================================================
// Request Types
// =============================================================================

export interface AdmitRequest {
  stream_id: string;
  user_id: string;
  device_id?: string;
  device_type?: string;
  quality?: string;
  metadata?: Record<string, unknown>;
}

export interface AdmitResponse {
  admitted: boolean;
  session_id?: string;
  playback_url?: string;
  heartbeat_interval_seconds?: number;
  max_quality?: string;
  expires_at?: string;
  reason?: DenialReason;
  message?: string;
  current_sessions?: number;
  max_sessions?: number;
}

export interface HeartbeatRequest {
  session_id: string;
  bytes_transferred?: number;
  quality?: string;
}

export interface EndSessionRequest {
  session_id: string;
  bytes_transferred?: number;
}

export interface CreateStreamRequest {
  stream_id: string;
  title?: string;
  stream_type?: StreamType;
  source_device_id?: string;
  ingest_url?: string;
  playback_url?: string;
  thumbnail_url?: string;
  max_viewers?: number;
  metadata?: Record<string, unknown>;
}

export interface UpdateStreamRequest {
  title?: string;
  status?: StreamStatus;
  playback_url?: string;
  thumbnail_url?: string;
  max_viewers?: number;
  metadata?: Record<string, unknown>;
}

export interface CreateRuleRequest {
  name: string;
  rule_type: RuleType;
  conditions: Record<string, unknown>;
  action?: RuleAction;
  priority?: number;
  active?: boolean;
}

export interface UpdateRuleRequest {
  name?: string;
  conditions?: Record<string, unknown>;
  action?: RuleAction;
  priority?: number;
  active?: boolean;
}

// =============================================================================
// nTV v1 API Types
// =============================================================================

export interface V1AdmitRequest {
  user_id: string;
  content_id: string;
  device_id?: string;
  content_rating?: string;
}

export interface V1AdmitResponse {
  admitted: true;
  session_id: string;
  signed_url: string;
  token: string;
  expires_at: string;
}

export interface V1AdmitDeniedResponse {
  admitted: false;
  reason: string;
}

export interface V1HeartbeatRequest {
  session_id: string;
}

export interface V1HeartbeatResponse {
  active: true;
  duration_seconds: number;
}

export interface V1ActiveSession {
  session_id: string;
  user_id: string;
  content_id: string;
  device_id: string | null;
  started_at: Date;
  last_heartbeat: Date;
}

export interface FamilyMemberRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  family_id: string;
  user_id: string;
  role: string;
  created_at: Date;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface GatewayStats {
  total_streams: number;
  active_streams: number;
  total_sessions: number;
  active_sessions: number;
  denied_sessions: number;
  total_rules: number;
  active_rules: number;
  peak_concurrent_viewers: number;
  last_activity: Date | null;
}

export interface AnalyticsSummary {
  total_streams: number;
  total_view_minutes: number;
  avg_viewers_per_stream: number;
  peak_viewers: number;
  unique_viewers: number;
  top_streams: Array<{
    stream_id: string;
    total_view_minutes: number;
    peak_viewers: number;
  }>;
}
