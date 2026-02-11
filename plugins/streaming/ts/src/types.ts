/**
 * Streaming Plugin Type Definitions
 */

// =============================================================================
// Enums / Union Types
// =============================================================================

export type StreamStatus = 'idle' | 'live' | 'ended' | 'offline';
export type QualityPreset = 'low' | 'medium' | 'hd' | 'full_hd' | 'ultra_hd';
export type StreamVisibility = 'public' | 'unlisted' | 'private' | 'subscriber_only';
export type RecordingStatus = 'processing' | 'ready' | 'failed';
export type ReportReason = 'spam' | 'harassment' | 'violence' | 'sexual_content' | 'hate_speech' | 'copyright' | 'other';
export type ReportStatus = 'pending' | 'reviewing' | 'resolved' | 'dismissed';
export type ScheduleStatus = 'scheduled' | 'live' | 'completed' | 'cancelled';

// =============================================================================
// Stream Types
// =============================================================================

export interface Stream {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  title: string;
  description: string | null;
  category: string | null;
  tags: string[];

  broadcaster_id: string;

  status: StreamStatus;
  started_at: Date | null;
  ended_at: Date | null;

  quality_preset: QualityPreset;
  enable_chat: boolean;
  enable_recording: boolean;
  enable_dvr: boolean;
  dvr_duration_seconds: number;

  visibility: StreamVisibility;
  requires_password: boolean;
  password_hash: string | null;
  allowed_users: string[];
  blocked_users: string[];

  allowed_countries: string[];
  blocked_countries: string[];

  rtmp_url: string | null;
  hls_url: string | null;
  webrtc_url: string | null;
  thumbnail_url: string | null;

  peak_viewers: number;
  total_views: number;
  duration_seconds: number;

  is_flagged: boolean;
  flag_reason: string | null;
  flagged_at: Date | null;
  is_taken_down: boolean;
  takedown_reason: string | null;
  taken_down_at: Date | null;

  metadata: Record<string, unknown>;
}

export interface CreateStreamInput {
  title: string;
  description?: string;
  category?: string;
  tags?: string[];
  broadcaster_id: string;
  quality_preset?: QualityPreset;
  enable_chat?: boolean;
  enable_recording?: boolean;
  enable_dvr?: boolean;
  dvr_duration_seconds?: number;
  visibility?: StreamVisibility;
  requires_password?: boolean;
  password?: string;
  allowed_countries?: string[];
  blocked_countries?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateStreamInput {
  title?: string;
  description?: string;
  category?: string;
  tags?: string[];
  quality_preset?: QualityPreset;
  enable_chat?: boolean;
  enable_recording?: boolean;
  enable_dvr?: boolean;
  dvr_duration_seconds?: number;
  visibility?: StreamVisibility;
  requires_password?: boolean;
  password?: string;
  allowed_countries?: string[];
  blocked_countries?: string[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Stream Key Types
// =============================================================================

export interface StreamKey {
  id: string;
  source_account_id: string;
  created_at: Date;

  stream_id: string;
  key: string;
  name: string;

  is_active: boolean;
  last_used_at: Date | null;

  ip_whitelist: string[];

  metadata: Record<string, unknown>;
}

export interface CreateStreamKeyInput {
  name: string;
  ip_whitelist?: string[];
}

// =============================================================================
// Viewer Types
// =============================================================================

export interface Viewer {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  stream_id: string;

  user_id: string | null;
  anonymous_id: string | null;

  ip_address: string | null;
  user_agent: string | null;
  country: string | null;
  city: string | null;

  joined_at: Date;
  last_heartbeat: Date;
  left_at: Date | null;
  watch_duration_seconds: number;

  current_quality: string | null;

  metadata: Record<string, unknown>;
}

// =============================================================================
// Recording Types
// =============================================================================

export interface Recording {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  stream_id: string;

  title: string;
  description: string | null;

  video_url: string;
  thumbnail_url: string | null;
  duration_seconds: number;
  file_size_bytes: number | null;

  recorded_at: Date;

  status: RecordingStatus;
  transcoding_progress: number;

  visibility: StreamVisibility;

  views: number;

  metadata: Record<string, unknown>;
}

// =============================================================================
// Clip Types
// =============================================================================

export interface Clip {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  stream_id: string | null;
  recording_id: string | null;

  title: string;
  description: string | null;

  created_by: string;

  start_offset_seconds: number;
  duration_seconds: number;

  video_url: string;
  thumbnail_url: string | null;

  status: RecordingStatus;

  views: number;
  shares: number;

  metadata: Record<string, unknown>;
}

export interface CreateClipInput {
  stream_id?: string;
  recording_id?: string;
  title: string;
  description?: string;
  created_by: string;
  start_offset_seconds: number;
  duration_seconds: number;
  video_url: string;
  thumbnail_url?: string;
}

// =============================================================================
// Analytics Types
// =============================================================================

export interface StreamAnalytics {
  id: string;
  source_account_id: string;
  created_at: Date;

  stream_id: string;

  bucket_start: Date;
  bucket_end: Date;

  concurrent_viewers: number;
  unique_viewers: number;
  new_viewers: number;
  returning_viewers: number;

  chat_messages: number;
  reactions: number;
  shares: number;

  avg_bitrate: number | null;
  avg_framerate: number | null;
  buffering_events: number;

  viewer_countries: Record<string, number>;

  metadata: Record<string, unknown>;
}

// =============================================================================
// Moderator Types
// =============================================================================

export interface Moderator {
  id: string;
  source_account_id: string;
  created_at: Date;

  stream_id: string;
  user_id: string;

  can_delete_messages: boolean;
  can_timeout_users: boolean;
  can_ban_users: boolean;
  can_manage_moderators: boolean;
}

export interface ModeratorPermissionsInput {
  can_delete_messages?: boolean;
  can_timeout_users?: boolean;
  can_ban_users?: boolean;
  can_manage_moderators?: boolean;
}

// =============================================================================
// Chat Types
// =============================================================================

export interface ChatMessage {
  id: string;
  source_account_id: string;
  created_at: Date;

  stream_id: string;
  user_id: string;

  content: string;

  is_deleted: boolean;
  deleted_by: string | null;
  deleted_at: Date | null;

  metadata: Record<string, unknown>;
}

export interface SendChatInput {
  user_id: string;
  content: string;
}

// =============================================================================
// Report Types
// =============================================================================

export interface Report {
  id: string;
  source_account_id: string;
  created_at: Date;

  stream_id: string;
  reported_by: string;

  reason: ReportReason;
  description: string | null;

  status: ReportStatus;
  reviewed_by: string | null;
  reviewed_at: Date | null;
  resolution: string | null;
}

export interface CreateReportInput {
  reported_by: string;
  reason: ReportReason;
  description?: string;
}

// =============================================================================
// Schedule Types
// =============================================================================

export interface ScheduledStream {
  id: string;
  source_account_id: string;
  created_at: Date;
  updated_at: Date;

  stream_id: string | null;
  broadcaster_id: string;

  title: string;
  description: string | null;
  scheduled_start: Date;
  estimated_duration_minutes: number | null;

  notify_followers: boolean;
  notified: boolean;

  status: ScheduleStatus;

  metadata: Record<string, unknown>;
}

export interface CreateScheduleInput {
  broadcaster_id: string;
  title: string;
  description?: string;
  scheduled_start: string;
  estimated_duration_minutes?: number;
  notify_followers?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Configuration Types
// =============================================================================

export interface StreamingConfig {
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };
  server: {
    port: number;
    host: string;
  };
  rtmp: {
    port: number;
  };
  recording: {
    enabled: boolean;
    s3_bucket: string;
    s3_region: string;
  };
  dvr: {
    enabled: boolean;
    window_seconds: number;
  };
  chat: {
    rate_limit_messages: number;
    rate_limit_window: number;
  };
  analytics: {
    bucket_interval_minutes: number;
    retention_days: number;
  };
  cdn: {
    url: string;
  };
}

// =============================================================================
// API Query/Response Types
// =============================================================================

export interface ListStreamsQuery {
  status?: StreamStatus;
  category?: string;
  broadcaster_id?: string;
  visibility?: StreamVisibility;
  limit?: string;
  offset?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface StreamStatsResponse {
  current_viewers: number;
  peak_viewers: number;
  total_views: number;
  duration_seconds: number;
  chat_messages: number;
}
