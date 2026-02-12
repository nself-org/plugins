export interface MetadataEnrichmentConfig {
  database_url: string;
  port: number;
  tmdb_api_key: string;
  tvdb_api_key?: string;
  musicbrainz_user_agent: string;
  object_storage_url?: string;
  log_level: string;
  api_key?: string;
  rate_limit_max?: number;
  rate_limit_window_ms?: number;
}

export interface MovieMetadata {
  id: string;
  source_account_id: string;
  tmdb_id: number;
  imdb_id?: string;
  title: string;
  original_title?: string;
  overview?: string;
  release_date?: Date;
  runtime?: number;
  genres?: string[];
  vote_average?: number;
  vote_count?: number;
  poster_path?: string;
  backdrop_path?: string;
  raw_response?: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface TVShowMetadata {
  id: string;
  source_account_id: string;
  tmdb_id: number;
  tvdb_id?: number;
  imdb_id?: string;
  name: string;
  original_name?: string;
  overview?: string;
  first_air_date?: Date;
  last_air_date?: Date;
  number_of_seasons: number;
  number_of_episodes: number;
  genres?: string[];
  vote_average?: number;
  vote_count?: number;
  poster_path?: string;
  backdrop_path?: string;
  raw_response?: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}
