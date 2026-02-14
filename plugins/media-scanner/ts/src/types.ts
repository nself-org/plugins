/**
 * Media Scanner Plugin Types
 * All TypeScript interfaces for scanning, parsing, probing, matching, and indexing
 */

import type { SecurityConfig } from '@nself/plugin-utils';

// ─── Configuration ──────────────────────────────────────────────────────────

export interface MediaScannerConfig {
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // MeiliSearch
  meilisearchUrl: string;
  meilisearchKey: string;

  // TMDB
  tmdbApiKey: string;

  // Library
  libraryPaths: string[];
  scanIntervalHours: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

// ─── Scanner ────────────────────────────────────────────────────────────────

export type ScanState = 'pending' | 'scanning' | 'completed' | 'failed';

export interface ScanRecord {
  id: string;
  source_account_id: string;
  paths: string[];
  recursive: boolean;
  state: ScanState;
  files_found: number;
  files_processed: number;
  errors: ScanError[];
  started_at: Date | null;
  completed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface ScanError {
  path: string;
  error: string;
  timestamp: string;
}

export interface ScanRequest {
  paths: string[];
  recursive?: boolean;
}

export interface ScanResponse {
  scan_id: string;
  files_found: number;
}

export interface ScanStatusResponse {
  scan_id: string;
  state: ScanState;
  files_found: number;
  files_processed: number;
  errors: ScanError[];
}

export interface DiscoveredFile {
  path: string;
  filename: string;
  size: number;
  modified_at: Date;
}

// ─── Parser ─────────────────────────────────────────────────────────────────

export interface ParsedFilename {
  title: string;
  year: number | null;
  season: number | null;
  episode: number | null;
  quality: string | null;
  resolution: string | null;
  codec: string | null;
  group: string | null;
}

export interface ParseRequest {
  filename: string;
}

// ─── FFprobe ────────────────────────────────────────────────────────────────

export interface MediaInfo {
  duration_seconds: number;
  video_codec: string | null;
  video_resolution: string | null;
  video_bitrate: number | null;
  audio_tracks: number;
  audio_languages: string[];
  subtitle_tracks: number;
  subtitle_languages: string[];
}

export interface ProbeRequest {
  path: string;
}

export interface FFprobeStream {
  index: number;
  codec_name?: string;
  codec_type: string;
  width?: number;
  height?: number;
  bit_rate?: string;
  duration?: string;
  channels?: number;
  tags?: {
    language?: string;
    title?: string;
    [key: string]: string | undefined;
  };
}

export interface FFprobeFormat {
  filename: string;
  nb_streams: number;
  format_name: string;
  duration: string;
  size: string;
  bit_rate: string;
  tags?: Record<string, string>;
}

export interface FFprobeOutput {
  streams: FFprobeStream[];
  format: FFprobeFormat;
}

// ─── Matcher ────────────────────────────────────────────────────────────────

export interface MatchRequest {
  title: string;
  year?: number;
  type: 'movie' | 'tv';
}

export interface MatchResult {
  provider: string;
  id: string;
  title: string;
  year: number | null;
  confidence: number;
}

export interface MatchResponse {
  matches: MatchResult[];
}

export interface TmdbSearchResult {
  id: number;
  title?: string;
  name?: string;
  original_title?: string;
  original_name?: string;
  release_date?: string;
  first_air_date?: string;
  overview?: string;
  vote_average?: number;
  popularity?: number;
  genre_ids?: number[];
  poster_path?: string;
  backdrop_path?: string;
}

export interface TmdbSearchResponse {
  page: number;
  results: TmdbSearchResult[];
  total_results: number;
  total_pages: number;
}

// ─── Search / Index ─────────────────────────────────────────────────────────

export interface IndexRequest {
  id: string;
  title: string;
  type: 'movie' | 'tv';
  genre?: string[];
  year?: number;
  rating?: number;
  description?: string;
  cast?: string[];
  poster_path?: string;
  backdrop_path?: string;
  file_path?: string;
  duration_seconds?: number;
  resolution?: string;
  codec?: string;
}

export interface IndexResponse {
  indexed: boolean;
}

export interface SearchQuery {
  q: string;
  type?: 'movie' | 'tv';
  genre?: string;
  year?: number;
  limit?: number;
  offset?: number;
}

export interface SearchResult {
  id: string;
  title: string;
  type: string;
  year: number | null;
  rating: number | null;
  genre?: string[];
  description?: string;
  poster_path?: string;
}

// ─── Media File Record ──────────────────────────────────────────────────────

export interface MediaFileRecord {
  id: string;
  source_account_id: string;
  scan_id: string | null;
  file_path: string;
  filename: string;
  file_size: number;
  modified_at: Date | null;
  parsed_title: string | null;
  parsed_year: number | null;
  parsed_season: number | null;
  parsed_episode: number | null;
  parsed_quality: string | null;
  parsed_resolution: string | null;
  parsed_codec: string | null;
  parsed_group: string | null;
  duration_seconds: number | null;
  video_codec: string | null;
  video_resolution: string | null;
  video_bitrate: number | null;
  audio_tracks: number;
  audio_languages: string[];
  subtitle_tracks: number;
  subtitle_languages: string[];
  match_provider: string | null;
  match_id: string | null;
  match_title: string | null;
  match_confidence: number | null;
  indexed: boolean;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

// ─── Statistics ─────────────────────────────────────────────────────────────

export interface LibraryStats {
  total_items: number;
  movies: number;
  tv_shows: number;
  total_size_gb: number;
  last_scan: Date | null;
  indexed_count: number;
  matched_count: number;
  unmatched_count: number;
}

// ─── Media Extensions ───────────────────────────────────────────────────────

export const MEDIA_EXTENSIONS = new Set([
  '.mkv',
  '.mp4',
  '.avi',
  '.ts',
  '.m4v',
  '.webm',
  '.mov',
  '.wmv',
  '.flv',
  '.mpg',
  '.mpeg',
  '.m2ts',
  '.vob',
  '.ogv',
  '.3gp',
]);
