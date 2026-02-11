/**
 * TMDB Plugin Types
 * Complete type definitions for TMDB API responses and database records
 */

// =============================================================================
// Configuration
// =============================================================================

export interface TmdbPluginConfig {
  apiKey: string;
  apiReadAccessToken?: string;
  port: number;
  host: string;
  imageBaseUrl: string;
  defaultLanguage: string;
  autoEnrich: boolean;
  confidenceThreshold: number;
  cacheTtlDays: number;
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
// TMDB API Response Types
// =============================================================================

export interface TmdbMovie {
  id: number;
  imdb_id?: string | null;
  title: string;
  original_title: string;
  overview: string;
  release_date: string;
  runtime?: number | null;
  vote_average: number;
  vote_count: number;
  popularity: number;
  status: string;
  tagline?: string | null;
  budget?: number;
  revenue?: number;
  genres: TmdbGenre[];
  spoken_languages: TmdbLanguage[];
  production_countries: TmdbCountry[];
  poster_path: string | null;
  backdrop_path: string | null;
  credits?: TmdbCredits;
  release_dates?: {
    results: Array<{
      iso_3166_1: string;
      release_dates: Array<{
        certification: string;
        type: number;
      }>;
    }>;
  };
  keywords?: {
    keywords: Array<{ id: number; name: string }>;
  };
}

export interface TmdbTvShow {
  id: number;
  name: string;
  original_name: string;
  overview: string;
  first_air_date: string;
  last_air_date?: string | null;
  status: string;
  type: string;
  number_of_seasons: number;
  number_of_episodes: number;
  episode_run_time: number[];
  vote_average: number;
  vote_count: number;
  popularity: number;
  genres: TmdbGenre[];
  networks: Array<{ id: number; name: string }>;
  created_by: Array<{ id: number; name: string }>;
  poster_path: string | null;
  backdrop_path: string | null;
  content_ratings?: {
    results: Array<{
      iso_3166_1: string;
      rating: string;
    }>;
  };
  keywords?: {
    results: Array<{ id: number; name: string }>;
  };
  external_ids?: {
    imdb_id?: string | null;
  };
}

export interface TmdbTvSeason {
  id: number;
  season_number: number;
  name: string;
  overview: string;
  air_date: string | null;
  episode_count: number;
  poster_path: string | null;
  episodes?: TmdbTvEpisode[];
}

export interface TmdbTvEpisode {
  id: number;
  episode_number: number;
  season_number: number;
  name: string;
  overview: string;
  air_date: string | null;
  runtime?: number | null;
  vote_average: number;
  still_path: string | null;
  guest_stars: TmdbCastMember[];
  crew: TmdbCrewMember[];
}

export interface TmdbGenre {
  id: number;
  name: string;
}

export interface TmdbLanguage {
  english_name: string;
  iso_639_1: string;
  name: string;
}

export interface TmdbCountry {
  iso_3166_1: string;
  name: string;
}

export interface TmdbCredits {
  cast: TmdbCastMember[];
  crew: TmdbCrewMember[];
}

export interface TmdbCastMember {
  id: number;
  name: string;
  character: string;
  order: number;
  profile_path: string | null;
}

export interface TmdbCrewMember {
  id: number;
  name: string;
  job: string;
  department: string;
  profile_path: string | null;
}

export interface TmdbSearchResult {
  page: number;
  results: TmdbSearchItem[];
  total_pages: number;
  total_results: number;
}

export interface TmdbSearchItem {
  id: number;
  media_type?: 'movie' | 'tv';
  title?: string;
  name?: string;
  original_title?: string;
  original_name?: string;
  release_date?: string;
  first_air_date?: string;
  overview: string;
  poster_path: string | null;
  backdrop_path: string | null;
  vote_average: number;
  popularity: number;
}

// =============================================================================
// Database Record Types
// =============================================================================

export interface TmdbMovieRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  tmdb_id: number;
  imdb_id: string | null;
  title: string;
  original_title: string;
  overview: string | null;
  release_date: Date | null;
  runtime_minutes: number | null;
  vote_average: number;
  vote_count: number;
  popularity: number;
  status: string;
  tagline: string | null;
  budget: number | null;
  revenue: number | null;
  genres: string[];
  spoken_languages: string[];
  production_countries: string[];
  poster_path: string | null;
  backdrop_path: string | null;
  cast: unknown[];
  crew: unknown[];
  content_rating: string | null;
  keywords: string[];
  synced_at: Date;
  created_at: Date;
  updated_at: Date;
}

export interface TmdbTvShowRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  tmdb_id: number;
  imdb_id: string | null;
  name: string;
  original_name: string;
  overview: string | null;
  first_air_date: Date | null;
  last_air_date: Date | null;
  status: string;
  type: string;
  number_of_seasons: number;
  number_of_episodes: number;
  episode_run_time: number[];
  vote_average: number;
  vote_count: number;
  popularity: number;
  genres: string[];
  networks: string[];
  created_by: string[];
  poster_path: string | null;
  backdrop_path: string | null;
  content_rating: string | null;
  keywords: string[];
  synced_at: Date;
  created_at: Date;
  updated_at: Date;
}

export interface TmdbTvSeasonRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  show_tmdb_id: number;
  season_number: number;
  tmdb_id: number | null;
  name: string;
  overview: string | null;
  air_date: Date | null;
  episode_count: number;
  poster_path: string | null;
  synced_at: Date;
  created_at: Date;
}

export interface TmdbTvEpisodeRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  show_tmdb_id: number;
  season_number: number;
  episode_number: number;
  tmdb_id: number | null;
  name: string;
  overview: string | null;
  air_date: Date | null;
  runtime_minutes: number | null;
  vote_average: number;
  still_path: string | null;
  guest_stars: unknown[];
  crew: unknown[];
  synced_at: Date;
  created_at: Date;
}

export interface TmdbGenreRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  tmdb_id: number;
  name: string;
  media_type: 'movie' | 'tv';
}

export interface TmdbMatchQueueRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  title: string;
  year: number | null;
  media_type: 'movie' | 'tv';
  source_id: string | null;
  source_plugin: string | null;
  candidates: unknown[];
  status: 'pending' | 'matched' | 'manual_review' | 'no_match';
  matched_tmdb_id: number | null;
  confidence: number | null;
  reviewed_by: string | null;
  reviewed_at: Date | null;
  created_at: Date;
}

export interface TmdbWebhookEventRecord {
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
// Business Logic Types
// =============================================================================

export interface LookupRequest extends Record<string, unknown> {
  title: string;
  year?: number;
  media_type?: 'movie' | 'tv';
}

export interface LookupResult {
  matched: boolean;
  confidence: number;
  tmdb_id: number | null;
  media_type: 'movie' | 'tv' | null;
  title: string | null;
  year: number | null;
  candidates: MatchCandidate[];
}

export interface MatchCandidate {
  tmdb_id: number;
  title: string;
  year: number | null;
  media_type: 'movie' | 'tv';
  confidence: number;
  overview: string;
  poster_path: string | null;
}

export interface EnrichRequest extends Record<string, unknown> {
  title: string;
  year?: number;
  media_type: 'movie' | 'tv';
  force?: boolean;
}

export interface EnrichResult {
  success: boolean;
  tmdb_id: number | null;
  media_type: 'movie' | 'tv' | null;
  cached: boolean;
  metadata: TmdbMovieRecord | TmdbTvShowRecord | null;
}

export interface BatchLookupRequest {
  items: LookupRequest[];
}

export interface BatchLookupResult {
  results: LookupResult[];
  duration: number;
}

export interface StatsResponse {
  movies: number;
  tvShows: number;
  seasons: number;
  episodes: number;
  genres: number;
  matchQueue: number;
  lastSyncedAt: Date | null;
}

export interface SearchParams {
  query: string;
  media_type?: 'movie' | 'tv';
  year?: number;
  page?: number;
}
