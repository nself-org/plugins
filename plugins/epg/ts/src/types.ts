/**
 * EPG Plugin Types
 * Complete type definitions for channels, programs, schedules, and channel groups
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface ChannelRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  channel_number: string | null;
  call_sign: string | null;
  name: string;
  display_name: string | null;
  logo_url: string | null;
  category: string | null;
  language: string;
  country: string;
  stream_url: string | null;
  stream_type: string | null;
  is_hd: boolean;
  is_4k: boolean;
  is_active: boolean;
  sort_order: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface ProgramRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  external_id: string | null;
  title: string;
  episode_title: string | null;
  description: string | null;
  long_description: string | null;
  categories: string[];
  genre: string | null;
  season_number: number | null;
  episode_number: number | null;
  original_air_date: Date | null;
  year: number | null;
  duration_minutes: number | null;
  content_rating: string | null;
  star_rating: number | null;
  poster_url: string | null;
  thumbnail_url: string | null;
  directors: string[];
  actors: string[];
  is_new: boolean;
  is_live: boolean;
  is_premiere: boolean;
  is_finale: boolean;
  is_movie: boolean;
  language: string;
  subtitles: string[];
  audio_format: string | null;
  video_format: string | null;
  production_code: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface ScheduleRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  channel_id: string;
  program_id: string;
  start_time: Date;
  end_time: Date;
  is_rerun: boolean;
  is_live: boolean;
  metadata: Record<string, unknown>;
}

export interface ChannelGroupRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  sort_order: number;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface ChannelGroupMemberRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  group_id: string;
  channel_id: string;
  sort_order: number;
}

export interface WebhookEventRecord {
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

// =============================================================================
// API Request Types
// =============================================================================

export interface CreateChannelRequest {
  channel_number?: string;
  call_sign?: string;
  name: string;
  display_name?: string;
  logo_url?: string;
  category?: string;
  language?: string;
  country?: string;
  stream_url?: string;
  stream_type?: string;
  is_hd?: boolean;
  is_4k?: boolean;
}

export interface UpdateChannelRequest {
  channel_number?: string;
  call_sign?: string;
  name?: string;
  display_name?: string;
  logo_url?: string;
  category?: string;
  stream_url?: string;
  stream_type?: string;
  is_hd?: boolean;
  is_4k?: boolean;
  is_active?: boolean;
  sort_order?: number;
}

export interface ListChannelsQuery {
  category?: string;
  is_active?: string;
  group_id?: string;
  limit?: number;
  offset?: number;
}

export interface CreateProgramRequest {
  title: string;
  episode_title?: string;
  description?: string;
  long_description?: string;
  categories?: string[];
  genre?: string;
  season_number?: number;
  episode_number?: number;
  original_air_date?: string;
  year?: number;
  duration_minutes?: number;
  content_rating?: string;
  star_rating?: number;
  poster_url?: string;
  thumbnail_url?: string;
  directors?: string[];
  actors?: string[];
  is_new?: boolean;
  is_live?: boolean;
  is_premiere?: boolean;
  is_finale?: boolean;
  is_movie?: boolean;
}

export interface SearchProgramsRequest {
  query: string;
  genre?: string;
  content_rating?: string;
  is_movie?: boolean;
  language?: string;
  limit?: number;
}

export interface GetScheduleQuery {
  channel_ids?: string;
  date?: string;
  hours?: number;
  timezone?: string;
}

export interface GetScheduleChannelQuery {
  date?: string;
  days?: number;
}

export interface GetScheduleProgramQuery {
  days?: number;
}

export interface GetTonightQuery {
  date?: string;
  primetime?: string;
}

export interface CreateChannelGroupRequest {
  name: string;
  description?: string;
  channel_ids?: string[];
}

export interface UpdateChannelGroupRequest {
  name?: string;
  description?: string;
  sort_order?: number;
}

export interface ImportXmltvRequest {
  url?: string;
  xml_data?: string;
}

export interface ImportManualRequest {
  schedules: Array<{
    channel_id: string;
    program_title: string;
    start_time: string;
    end_time: string;
    description?: string;
    categories?: string[];
    is_live?: boolean;
  }>;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface ScheduleEntry {
  [key: string]: unknown;
  schedule_id: string;
  program_id: string;
  title: string;
  episode_title: string | null;
  description: string | null;
  categories: string[];
  content_rating: string | null;
  start_time: Date;
  end_time: Date;
  duration_minutes: number | null;
  is_live: boolean;
  is_new: boolean;
  is_rerun: boolean;
}

export interface ChannelSchedule {
  channel_id: string;
  channel_name: string;
  channel_number: string | null;
  logo_url: string | null;
  programs: ScheduleEntry[];
}

export interface WhatsOnNowEntry {
  channel_id: string;
  channel_name: string;
  channel_number: string | null;
  logo_url: string | null;
  current_program: ScheduleEntry | null;
  next_program: ScheduleEntry | null;
}

export interface ImportXmltvResponse {
  channels_imported: number;
  programs_imported: number;
  schedules_imported: number;
  errors: string[];
}

// =============================================================================
// Recording Types
// =============================================================================

export interface RecordingRuleRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  user_id: string;
  rule_type: 'single' | 'series' | 'keyword';
  program_id: string | null;
  channel_id: string | null;
  series_title: string | null;
  keyword: string | null;
  priority: number;
  keep_count: number | null;
  start_padding_minutes: number;
  end_padding_minutes: number;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface ScheduledRecordingRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  recording_rule_id: string | null;
  program_id: string | null;
  channel_id: string | null;
  scheduled_start: Date;
  scheduled_end: Date;
  status: 'scheduled' | 'recording' | 'completed' | 'failed' | 'conflict' | 'cancelled';
  antserver_job_id: string | null;
  error_message: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateRecordingRuleRequest {
  user_id: string;
  rule_type: 'single' | 'series' | 'keyword';
  program_id?: string;
  channel_id?: string;
  series_title?: string;
  keyword?: string;
  priority?: number;
  keep_count?: number;
  start_padding_minutes?: number;
  end_padding_minutes?: number;
}

export interface CreateRecordingRuleData {
  user_id: string;
  rule_type: 'single' | 'series' | 'keyword';
  program_id?: string | null;
  channel_id?: string | null;
  series_title?: string | null;
  keyword?: string | null;
  priority?: number;
  keep_count?: number | null;
  start_padding_minutes?: number;
  end_padding_minutes?: number;
}

export interface CreateScheduledRecordingData {
  recording_rule_id?: string | null;
  program_id?: string | null;
  channel_id?: string | null;
  scheduled_start: Date;
  scheduled_end: Date;
  status?: 'scheduled' | 'recording' | 'completed' | 'failed' | 'conflict' | 'cancelled';
  antserver_job_id?: string | null;
  error_message?: string | null;
}

export interface RecordingTriggerEvent {
  recording_id: string;
  status: 'started' | 'completed' | 'failed';
  antserver_job_id?: string;
  error_message?: string;
}

export interface ListScheduledRecordingsQuery {
  status?: string;
  from?: string;
  to?: string;
}

export interface ConflictCheckQuery {
  start: string;
  end: string;
  channel_id?: string;
}

export interface ResolveConflictRequest {
  recording_ids: string[];
  strategy: 'priority' | 'keep';
  keep_id?: string;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface EpgStats {
  total_channels: number;
  active_channels: number;
  total_programs: number;
  total_schedules: number;
  total_channel_groups: number;
  oldest_schedule: Date | null;
  newest_schedule: Date | null;
}
