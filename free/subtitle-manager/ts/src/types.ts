export interface SubtitleManagerConfig {
  database_url: string;
  port: number;
  opensubtitles_api_key?: string;
  subtitle_storage_path: string;
  log_level: string;
  alass_path: string;
  ffsubsync_path: string;
}

export interface SubtitleRecord {
  id: string;
  source_account_id: string;
  media_id: string;
  media_type: 'movie' | 'tv_episode';
  language: string;
  file_path: string;
  source: string;
  sync_score?: number;
  created_at: Date;
  updated_at: Date;
}

export interface SubtitleDownloadRecord {
  id: string;
  source_account_id: string;
  subtitle_id?: string;
  media_id: string;
  media_type: string;
  media_title?: string;
  language: string;
  file_path: string;
  file_size_bytes?: number;
  opensubtitles_file_id?: number;
  file_hash?: string;
  sync_score?: number;
  source: string;
  qc_status?: string;
  qc_details?: QualityCheckDetails;
  created_at: Date;
  updated_at: Date;
}

export interface QualityCheckDetails {
  errors?: string[];
  warnings?: string[];
  info?: string[];
  fix_count?: number;
  original_encoding?: string;
  output_encoding?: string;
  timing_issues?: boolean;
  overlaps_fixed?: number;
  gaps_fixed?: number;
  [key: string]: unknown;
}

export interface UpsertSubtitleInput {
  source_account_id?: string;
  media_id: string;
  media_type: string;
  language: string;
  file_path: string;
  source: string;
  sync_score?: number;
}

export interface InsertDownloadInput {
  source_account_id?: string;
  subtitle_id?: string;
  media_id: string;
  media_type: string;
  media_title?: string;
  language: string;
  file_path: string;
  file_size_bytes?: number;
  opensubtitles_file_id?: number;
  file_hash?: string;
  sync_score?: number;
  source: string;
}

export interface SubtitleStats {
  total_subtitles: number;
  total_downloads: number;
  languages: { language: string; count: number }[];
  sources: { source: string; count: number }[];
}

// ---------------------------------------------------------------------------
// Sync types
// ---------------------------------------------------------------------------

export interface SyncResult {
  originalPath: string;
  syncedPath: string;
  confidence: number;
  offsetMs: number;
  method: 'alass' | 'ffsubsync' | 'both';
  alassResult?: { confidence: number; offsetMs: number; framerateAdjusted: boolean };
  ffsubsyncResult?: { confidence: number; offsetMs: number };
}

// ---------------------------------------------------------------------------
// QC types
// ---------------------------------------------------------------------------

export interface QCResult {
  status: 'pass' | 'warn' | 'fail';
  checks: QCCheck[];
  issues: QCIssue[];
  cueCount: number;
  totalDurationMs: number;
}

export interface QCCheck {
  name: string;
  passed: boolean;
  message: string;
}

export interface QCIssue {
  severity: 'error' | 'warning';
  check: string;
  cueIndex?: number;
  message: string;
}

export interface QCResultRecord {
  id: string;
  source_account_id: string;
  download_id: string;
  status: string;
  checks: QCCheck[];
  issues: QCIssue[];
  cue_count: number;
  total_duration_ms: number;
  created_at: Date;
}

export interface InsertQCResultInput {
  source_account_id?: string;
  download_id: string;
  status: string;
  checks: QCCheck[];
  issues: QCIssue[];
  cue_count: number;
  total_duration_ms: number;
}

// ---------------------------------------------------------------------------
// Subtitle cue (used for parsing and QC)
// ---------------------------------------------------------------------------

export interface SubtitleCue {
  index: number;
  startMs: number;
  endMs: number;
  text: string;
}
