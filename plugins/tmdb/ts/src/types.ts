/**
 * TMDB Plugin Types
 * All TypeScript interfaces for the TMDB metadata plugin
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface TmdbMovieRecord {
  id: number;
  source_account_id: string;
  imdb_id: string | null;
  title: string;
  original_title: string | null;
  overview: string | null;
  tagline: string | null;
  release_date: string | null;
  runtime: number | null;
  status: string | null;
  poster_path: string | null;
  backdrop_path: string | null;
  budget: number | null;
  revenue: number | null;
  vote_average: number | null;
  vote_count: number | null;
  popularity: number | null;
  original_language: string | null;
  genres: unknown[];
  production_companies: unknown[];
  production_countries: unknown[];
  spoken_languages: unknown[];
  credits: Record<string, unknown>;
  keywords: unknown[];
  content_rating: string | null;
  synced_at: Date;
}

export interface TmdbTvShowRecord {
  id: number;
  source_account_id: string;
  imdb_id: string | null;
  name: string;
  original_name: string | null;
  overview: string | null;
  first_air_date: string | null;
  last_air_date: string | null;
  status: string | null;
  type: string | null;
  number_of_seasons: number | null;
  number_of_episodes: number | null;
  episode_run_time: number[] | null;
  poster_path: string | null;
  backdrop_path: string | null;
  vote_average: number | null;
  vote_count: number | null;
  popularity: number | null;
  original_language: string | null;
  genres: unknown[];
  networks: unknown[];
  created_by: unknown[];
  credits: Record<string, unknown>;
  content_rating: string | null;
  synced_at: Date;
}

export interface TmdbTvSeasonRecord {
  id: string;
  source_account_id: string;
  show_id: number;
  season_number: number;
  name: string | null;
  overview: string | null;
  poster_path: string | null;
  air_date: string | null;
  episode_count: number | null;
  synced_at: Date;
}

export interface TmdbTvEpisodeRecord {
  id: string;
  source_account_id: string;
  show_id: number;
  season_number: number;
  episode_number: number;
  name: string | null;
  overview: string | null;
  still_path: string | null;
  air_date: string | null;
  runtime: number | null;
  vote_average: number | null;
  crew: unknown[];
  guest_stars: unknown[];
  synced_at: Date;
}

export interface TmdbGenreRecord {
  id: number;
  source_account_id: string;
  name: string;
  media_type: string;
}

export interface TmdbMatchQueueRecord {
  id: string;
  source_account_id: string;
  media_id: string;
  filename: string | null;
  parsed_title: string | null;
  parsed_year: number | null;
  parsed_type: string | null;
  match_results: unknown[];
  best_match_id: number | null;
  best_match_type: string | null;
  confidence: number | null;
  status: 'pending' | 'accepted' | 'rejected' | 'manual';
  reviewed_by: string | null;
  reviewed_at: Date | null;
  auto_accepted: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface TmdbWebhookEventRecord {
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

// ============================================================================
// API Types - TMDB External
// ============================================================================

export interface TmdbApiMovieResult {
  id: number;
  title: string;
  original_title: string;
  overview: string;
  release_date: string;
  poster_path: string | null;
  backdrop_path: string | null;
  vote_average: number;
  vote_count: number;
  popularity: number;
  genre_ids: number[];
  original_language: string;
  adult: boolean;
}

export interface TmdbApiTvResult {
  id: number;
  name: string;
  original_name: string;
  overview: string;
  first_air_date: string;
  poster_path: string | null;
  backdrop_path: string | null;
  vote_average: number;
  vote_count: number;
  popularity: number;
  genre_ids: number[];
  original_language: string;
}

export interface TmdbApiSearchResponse<T> {
  page: number;
  results: T[];
  total_pages: number;
  total_results: number;
}

export interface TmdbApiMovieDetails {
  id: number;
  imdb_id: string | null;
  title: string;
  original_title: string;
  overview: string;
  tagline: string;
  release_date: string;
  runtime: number | null;
  status: string;
  poster_path: string | null;
  backdrop_path: string | null;
  budget: number;
  revenue: number;
  vote_average: number;
  vote_count: number;
  popularity: number;
  original_language: string;
  genres: Array<{ id: number; name: string }>;
  production_companies: Array<{ id: number; name: string; logo_path: string | null; origin_country: string }>;
  production_countries: Array<{ iso_3166_1: string; name: string }>;
  spoken_languages: Array<{ iso_639_1: string; name: string; english_name: string }>;
  credits?: {
    cast: Array<{ id: number; name: string; character: string; order: number; profile_path: string | null }>;
    crew: Array<{ id: number; name: string; job: string; department: string; profile_path: string | null }>;
  };
  keywords?: { keywords: Array<{ id: number; name: string }> };
  release_dates?: {
    results: Array<{
      iso_3166_1: string;
      release_dates: Array<{ certification: string; type: number }>;
    }>;
  };
}

export interface TmdbApiTvDetails {
  id: number;
  name: string;
  original_name: string;
  overview: string;
  first_air_date: string;
  last_air_date: string;
  status: string;
  type: string;
  number_of_seasons: number;
  number_of_episodes: number;
  episode_run_time: number[];
  poster_path: string | null;
  backdrop_path: string | null;
  vote_average: number;
  vote_count: number;
  popularity: number;
  original_language: string;
  genres: Array<{ id: number; name: string }>;
  networks: Array<{ id: number; name: string; logo_path: string | null; origin_country: string }>;
  created_by: Array<{ id: number; name: string; profile_path: string | null }>;
  credits?: {
    cast: Array<{ id: number; name: string; character: string; order: number }>;
    crew: Array<{ id: number; name: string; job: string; department: string }>;
  };
  content_ratings?: {
    results: Array<{ iso_3166_1: string; rating: string }>;
  };
}

export interface TmdbApiSeasonDetails {
  id: number;
  season_number: number;
  name: string;
  overview: string;
  poster_path: string | null;
  air_date: string;
  episodes: Array<{
    id: number;
    episode_number: number;
    name: string;
    overview: string;
    still_path: string | null;
    air_date: string;
    runtime: number | null;
    vote_average: number;
    crew: unknown[];
    guest_stars: unknown[];
  }>;
}

export interface TmdbApiImagesResponse {
  id: number;
  posters: TmdbApiImage[];
  backdrops: TmdbApiImage[];
  logos: TmdbApiImage[];
}

export interface TmdbApiImage {
  file_path: string;
  width: number;
  height: number;
  iso_639_1: string | null;
  aspect_ratio: number;
  vote_average: number;
  vote_count: number;
}

export interface TmdbApiConfiguration {
  images: {
    base_url: string;
    secure_base_url: string;
    poster_sizes: string[];
    backdrop_sizes: string[];
    logo_sizes: string[];
    still_sizes: string[];
    profile_sizes: string[];
  };
}

export interface TmdbApiGenre {
  id: number;
  name: string;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface TmdbSearchRequest {
  query: string;
  year?: number;
  language?: string;
}

export interface TmdbSearchResult {
  id: number;
  title?: string;
  name?: string;
  overview?: string;
  releaseDate?: string;
  firstAirDate?: string;
  posterPath?: string;
  voteAverage?: number;
  matchScore?: number;
}

export interface TmdbSearchResponse {
  results: TmdbSearchResult[];
  total: number;
}

export interface MatchMediaRequest {
  mediaId: string;
  filename?: string;
  title?: string;
  year?: number;
  type?: 'movie' | 'tv';
}

export interface MatchMediaResponse {
  matchQueueId: string;
  bestMatch?: {
    id: number;
    title: string;
    confidence: number;
    autoAccepted: boolean;
  };
  alternatives: Array<{
    id: number;
    title: string;
    confidence: number;
  }>;
}

export interface BatchMatchRequest {
  items: MatchMediaRequest[];
}

export interface BatchMatchResponse {
  processed: number;
  autoAccepted: number;
  needsReview: number;
}

export interface ConfirmMatchRequest {
  tmdbId: number;
  tmdbType: 'movie' | 'tv';
}

export interface RefreshMetadataResponse {
  refreshed: boolean;
  changed: string[];
}

export interface TmdbImagesResponse {
  posters: TmdbImage[];
  backdrops: TmdbImage[];
  logos: TmdbImage[];
}

export interface TmdbImage {
  path: string;
  width: number;
  height: number;
  language?: string;
}

export interface TmdbConfiguration {
  imageBaseUrl: string;
  posterSizes: string[];
  backdropSizes: string[];
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface TmdbConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  appIds: string[];

  // TMDB API
  tmdbApiKey: string;
  tmdbApiReadAccessToken: string;
  omdbApiKey: string;

  // Matching
  autoAcceptThreshold: number;
  filenameParsing: boolean;
  defaultLanguage: string;

  // Caching
  cacheTtlDays: number;
  refreshCron: string;

  // Images
  imageBaseUrl: string;
  posterSize: string;
  backdropSize: string;

  // Rate limiting
  rateLimitRequests: number;
  rateLimitWindowMs: number;

  // Security
  security: SecurityConfig;
}

export interface SecurityConfig {
  apiKey?: string;
  rateLimitMax?: number;
  rateLimitWindowMs?: number;
}

// ============================================================================
// Health/Status Types
// ============================================================================

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}

export interface ReadyCheckResponse {
  ready: boolean;
  database: 'ok' | 'error';
  tmdbApi: 'ok' | 'error' | 'unconfigured';
  timestamp: string;
}

export interface LiveCheckResponse {
  alive: boolean;
  uptime: number;
  memory: {
    used: number;
    total: number;
  };
  stats: TmdbStats;
}

export interface TmdbStats {
  totalMovies: number;
  totalTvShows: number;
  totalSeasons: number;
  totalEpisodes: number;
  totalGenres: number;
  matchQueuePending: number;
  matchQueueAccepted: number;
  matchQueueRejected: number;
}

export interface StatusResponse {
  movies: number;
  tvShows: number;
  seasons: number;
  episodes: number;
  genres: number;
  matchQueue: {
    pending: number;
    accepted: number;
    rejected: number;
    manual: number;
  };
  lastSynced: string | null;
}
