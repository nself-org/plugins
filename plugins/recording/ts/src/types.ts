/**
 * Recording Plugin Types
 * Complete type definitions for recordings, schedules, and encode jobs
 */

// =============================================================================
// Recording Types
// =============================================================================

export type RecordingStatus =
  | 'scheduled'
  | 'starting'
  | 'recording'
  | 'processing'
  | 'finalizing'
  | 'encoding'
  | 'uploading'
  | 'published'
  | 'failed'
  | 'cancelled';

export type SourceType = 'live_tv' | 'device_ingest' | 'upload' | 'stream_capture';

export type PublishStatus = 'unpublished' | 'publishing' | 'published' | 'unpublishing';

export type EnrichmentStatus = 'pending' | 'enriching' | 'enriched' | 'failed';

export type RecordingPriority = 'low' | 'normal' | 'high' | 'critical';

export type ScheduleType = 'manual' | 'recurring' | 'sports_event' | 'keyword';

export type EncodeStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

export interface RecordingRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  title: string;
  description: string | null;
  source_type: SourceType;
  source_id: string | null;
  source_channel: string | null;
  source_device_id: string | null;
  status: RecordingStatus;
  priority: RecordingPriority;
  scheduled_start: Date;
  scheduled_end: Date;
  actual_start: Date | null;
  actual_end: Date | null;
  duration_seconds: number | null;
  file_path: string | null;
  file_size: number | null;
  file_format: string | null;
  thumbnail_url: string | null;
  encode_status: string | null;
  encode_progress: number;
  encode_started_at: Date | null;
  encode_completed_at: Date | null;
  publish_status: PublishStatus;
  published_at: Date | null;
  storage_object_id: string | null;
  sports_event_id: string | null;
  media_metadata_id: string | null;
  enrichment_status: EnrichmentStatus;
  tags: string[];
  category: string | null;
  content_rating: string | null;
  commercial_markers: Record<string, unknown>[] | null;
  custom_fields: Record<string, unknown>;
  metadata: Record<string, unknown>;
  created_by: string | null;
  created_at: Date;
  updated_at: Date;
  deleted_at: Date | null;
}

export interface ScheduleRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  app_id: string;
  name: string;
  schedule_type: ScheduleType;
  source_channel: string | null;
  source_device_id: string | null;
  recurrence_rule: string | null;
  duration_minutes: number;
  lead_time_minutes: number;
  trail_time_minutes: number;
  sports_league: string | null;
  sports_team_id: string | null;
  auto_enrich: boolean;
  auto_publish: boolean;
  priority: RecordingPriority;
  active: boolean;
  last_triggered_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface EncodeJobRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  recording_id: string;
  profile: string;
  status: EncodeStatus;
  input_path: string;
  output_path: string | null;
  output_size: number | null;
  progress: number;
  settings: Record<string, unknown>;
  started_at: Date | null;
  completed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Request Types
// =============================================================================

export interface CreateRecordingRequest {
  title: string;
  description?: string;
  source_type: SourceType;
  source_id?: string;
  source_channel?: string;
  source_device_id?: string;
  scheduled_start: string;
  scheduled_end: string;
  priority?: RecordingPriority;
  sports_event_id?: string;
  tags?: string[];
  category?: string;
  content_rating?: string;
  custom_fields?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_by?: string;
}

export interface UpdateRecordingRequest {
  title?: string;
  description?: string;
  scheduled_start?: string;
  scheduled_end?: string;
  priority?: RecordingPriority;
  tags?: string[];
  category?: string;
  content_rating?: string;
  custom_fields?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface CreateScheduleRequest {
  name: string;
  schedule_type: ScheduleType;
  source_channel?: string;
  source_device_id?: string;
  recurrence_rule?: string;
  duration_minutes: number;
  lead_time_minutes?: number;
  trail_time_minutes?: number;
  sports_league?: string;
  sports_team_id?: string;
  auto_enrich?: boolean;
  auto_publish?: boolean;
  priority?: RecordingPriority;
  metadata?: Record<string, unknown>;
}

export interface UpdateScheduleRequest {
  name?: string;
  source_channel?: string;
  source_device_id?: string;
  recurrence_rule?: string;
  duration_minutes?: number;
  lead_time_minutes?: number;
  trail_time_minutes?: number;
  auto_enrich?: boolean;
  auto_publish?: boolean;
  priority?: RecordingPriority;
  active?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// nTV v1 Request Types
// =============================================================================

export interface NtvCreateRecordingRequest {
  channel_id: string;
  program_id?: string;
  title: string;
  start_time: string;
  end_time: string;
}

export interface NtvScheduleRecordingRequest {
  title: string;
  channel_id: string;
  program_id?: string;
  start_time: string;
  end_time: string;
  recurring: boolean;
  series_id?: string;
  priority?: RecordingPriority;
}

export interface TriggerEncodeRequest {
  profile?: string;
  settings?: Record<string, unknown>;
}

export interface SportsWebhookPayload {
  event_id: string;
  event_type: string;
  sport: string;
  league: string;
  team_ids: string[];
  start_time: string;
  end_time: string;
  channel?: string;
  title?: string;
  metadata?: Record<string, unknown>;
}

export interface DeviceWebhookPayload {
  device_id: string;
  action: string;
  recording_id?: string;
  file_path?: string;
  file_size?: number;
  duration_seconds?: number;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface RecordingStats {
  total_recordings: number;
  scheduled: number;
  recording_now: number;
  encoding: number;
  published: number;
  failed: number;
  cancelled: number;
  total_storage_gb: number;
  total_duration_hours: number;
  total_schedules: number;
  active_schedules: number;
  pending_encode_jobs: number;
  running_encode_jobs: number;
  last_activity: Date | null;
}
