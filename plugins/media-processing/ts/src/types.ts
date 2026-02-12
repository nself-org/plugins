/**
 * Media Processing Plugin Types
 * Complete type definitions for media encoding and processing
 */

// =============================================================================
// Configuration Types
// =============================================================================

export interface MediaProcessingPluginConfig {
  port: number;
  host: string;
  ffmpegPath: string;
  ffprobePath: string;
  outputBasePath: string;
  maxConcurrentJobs: number;
  maxInputSizeGb: number;
  hardwareAccel: 'none' | 'nvenc' | 'vaapi' | 'qsv';
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Encoding Profile Types
// =============================================================================

export interface Resolution {
  width: number;
  height: number;
  bitrate: number;
  label: string;
}

export interface EncodingProfileRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  container: 'mp4' | 'mkv' | 'webm' | 'ts';
  video_codec: 'h264' | 'h265' | 'vp9' | 'av1';
  audio_codec: 'aac' | 'opus' | 'mp3';
  resolutions: Resolution[];
  audio_bitrate: number;
  framerate: number;
  preset: 'ultrafast' | 'superfast' | 'veryfast' | 'faster' | 'fast' | 'medium' | 'slow' | 'slower' | 'veryslow';
  hls_enabled: boolean;
  hls_segment_duration: number;
  trickplay_enabled: boolean;
  trickplay_interval: number;
  subtitle_extract: boolean;
  thumbnail_enabled: boolean;
  thumbnail_count: number;
  is_default: boolean;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateEncodingProfileInput {
  name: string;
  description?: string;
  container?: 'mp4' | 'mkv' | 'webm' | 'ts';
  video_codec?: 'h264' | 'h265' | 'vp9' | 'av1';
  audio_codec?: 'aac' | 'opus' | 'mp3';
  resolutions?: Resolution[];
  audio_bitrate?: number;
  framerate?: number;
  preset?: 'ultrafast' | 'superfast' | 'veryfast' | 'faster' | 'fast' | 'medium' | 'slow' | 'slower' | 'veryslow';
  hls_enabled?: boolean;
  hls_segment_duration?: number;
  trickplay_enabled?: boolean;
  trickplay_interval?: number;
  subtitle_extract?: boolean;
  thumbnail_enabled?: boolean;
  thumbnail_count?: number;
  is_default?: boolean;
}

export interface UpdateEncodingProfileInput extends Partial<CreateEncodingProfileInput> {
  id: string;
}

// =============================================================================
// Job Types
// =============================================================================

export type JobStatus =
  | 'pending'
  | 'downloading'
  | 'analyzing'
  | 'encoding'
  | 'packaging'
  | 'uploading'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'qa_failed';

export interface MediaMetadata {
  format?: string;
  duration?: number;
  bitrate?: number;
  size?: number;
  streams?: MediaStream[];
  chapters?: MediaChapter[];
  [key: string]: unknown;
}

export interface MediaStream {
  index: number;
  codec_type: 'video' | 'audio' | 'subtitle' | 'data';
  codec_name: string;
  codec_long_name?: string;
  profile?: string;
  width?: number;
  height?: number;
  sample_rate?: string;
  channels?: number;
  channel_layout?: string;
  bit_rate?: string;
  duration?: string;
  language?: string;
  tags?: Record<string, string>;
}

export interface MediaChapter {
  id: number;
  time_base: string;
  start: number;
  start_time: string;
  end: number;
  end_time: string;
  tags?: Record<string, string>;
}

export interface JobRecord {
  id: string;
  source_account_id: string;
  input_url: string;
  input_type: 'file' | 'url' | 's3';
  profile_id: string | null;
  status: JobStatus;
  priority: number;
  progress: number;
  input_metadata: MediaMetadata;
  output_base_path: string | null;
  error_message: string | null;
  duration_seconds: number | null;
  file_size_bytes: number | null;
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateJobInput {
  input_url: string;
  input_type?: 'file' | 'url' | 's3';
  profile_id?: string;
  priority?: number;
  output_base_path?: string;
}

export interface JobWithOutputs extends JobRecord {
  outputs: JobOutputRecord[];
  hls_manifest?: HlsManifestRecord;
  subtitles: SubtitleRecord[];
  trickplay?: TrickplayRecord;
}

// =============================================================================
// Job Output Types
// =============================================================================

export type OutputType =
  | 'video'
  | 'audio'
  | 'thumbnail'
  | 'trickplay'
  | 'subtitle'
  | 'hls_manifest'
  | 'hls_segment';

export interface JobOutputRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  output_type: OutputType;
  resolution_label: string | null;
  file_path: string;
  file_size_bytes: number | null;
  content_type: string | null;
  width: number | null;
  height: number | null;
  bitrate: number | null;
  duration_seconds: number | null;
  language: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// HLS Manifest Types
// =============================================================================

export interface VariantManifest {
  resolution_label: string;
  bandwidth: number;
  width: number;
  height: number;
  codecs: string;
  manifest_path: string;
}

export interface HlsManifestRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  master_manifest_path: string;
  variant_manifests: VariantManifest[];
  segment_count: number;
  total_duration_seconds: number | null;
  created_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Subtitle Types
// =============================================================================

export interface SubtitleRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  language: string;
  label: string | null;
  format: 'vtt' | 'srt' | 'ass';
  file_path: string;
  is_default: boolean;
  is_forced: boolean;
  created_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Trickplay Types
// =============================================================================

export interface TrickplayRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  tile_width: number;
  tile_height: number;
  columns: number;
  rows: number;
  interval_seconds: number;
  file_path: string;
  index_path: string | null;
  total_thumbnails: number | null;
  created_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface WebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// FFmpeg Types
// =============================================================================

export interface FFprobeResult {
  format: {
    filename: string;
    nb_streams: number;
    format_name: string;
    format_long_name: string;
    duration?: string;
    size?: string;
    bit_rate?: string;
    probe_score?: number;
    tags?: Record<string, string>;
  };
  streams: MediaStream[];
  chapters?: MediaChapter[];
}

export interface FFmpegProgress {
  frame: number;
  fps: number;
  bitrate: string;
  total_size: number;
  out_time_ms: number;
  out_time: string;
  dup_frames: number;
  drop_frames: number;
  speed: string;
  progress: string;
}

export interface EncodingTask {
  jobId: string;
  profile: EncodingProfileRecord;
  inputPath: string;
  outputBasePath: string;
  metadata: MediaMetadata;
}

// =============================================================================
// Statistics Types
// =============================================================================

export interface ProcessingStats {
  totalJobs: number;
  pendingJobs: number;
  runningJobs: number;
  completedJobs: number;
  failedJobs: number;
  totalDurationSeconds: number;
  totalFileSizeBytes: number;
  profiles: number;
  averageProcessingTimeSeconds: number | null;
  lastJobCompletedAt: Date | null;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
  timestamp: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  pageSize: number;
  hasMore: boolean;
}

// =============================================================================
// Packager Types (UPGRADE 1a)
// =============================================================================

export type PackagerType = 'shaka' | 'bento4' | 'ffmpeg-only';

export type OutputFormat = 'hls' | 'dash' | 'cmaf';

export interface PackagerStreamDescriptor {
  input: string;
  stream: 'audio' | 'video';
  language?: string;
  bandwidth?: number;
  output?: string;
}

export interface PackagerOptions {
  hlsMasterPlaylistOutput?: string;
  mpdOutput?: string;
  segmentDuration?: number;
}

// =============================================================================
// Content Identification Types (UPGRADE 1d)
// =============================================================================

export interface ParsedMediaInfo {
  title: string;
  year?: number;
  season?: number;
  episode?: number;
  resolution?: string;
  source?: string;
  codec?: string;
  releaseGroup?: string;
  raw: string;
}

export interface TmdbSearchResult {
  id: number;
  title: string;
  release_date?: string;
  media_type?: string;
  overview?: string;
  vote_average?: number;
}

export interface ContentIdentification extends ParsedMediaInfo {
  tmdb_id?: number;
  tmdb_title?: string;
  tmdb_year?: number;
  confidence: number;
}

// =============================================================================
// QA Validation Types (UPGRADE 1f)
// =============================================================================

export type QAStatus = 'pass' | 'warn' | 'fail';

export interface QACheck {
  name: string;
  status: QAStatus;
  message: string;
  details?: Record<string, unknown>;
}

export interface QAResult {
  status: QAStatus;
  checks: QACheck[];
  issues: string[];
  timestamp: string;
}

// =============================================================================
// Drop-Folder Watcher Types (UPGRADE 1c)
// =============================================================================

export interface DropFolderEvent {
  id: string;
  source_account_id: string;
  file_path: string;
  file_size: number;
  event_type: 'detected' | 'settled' | 'validated' | 'submitted' | 'error';
  job_id: string | null;
  error_message: string | null;
  created_at: Date;
  [key: string]: unknown;
}

export interface WatcherStatus {
  running: boolean;
  watchPath: string | null;
  filesDetected: number;
  jobsSubmitted: number;
  errors: number;
  startedAt: Date | null;
}

// =============================================================================
// Upload Record Types (UPGRADE 1e)
// =============================================================================

export interface UploadRecord {
  id: string;
  source_account_id: string;
  job_id: string;
  file_path: string;
  storage_path: string;
  storage_url: string | null;
  content_type: string;
  file_size_bytes: number;
  content_id: string | null;
  version: number;
  uploaded_at: Date;
  [key: string]: unknown;
}

// =============================================================================
// Job Leasing Types (UPGRADE 1g)
// =============================================================================

export interface LeasedJob extends JobRecord {
  leased_by: string | null;
  heartbeat_at: Date | null;
}
