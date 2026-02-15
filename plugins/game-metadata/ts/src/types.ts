/**
 * Game Metadata Plugin Types
 * Complete type definitions for game catalog, metadata, artwork, platforms, and genres
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface GameCatalogRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  title: string;
  slug: string;
  platform_id: string | null;
  genre_id: string | null;
  release_date: Date | null;
  developer: string | null;
  publisher: string | null;
  description: string | null;
  igdb_id: number | null;
  rom_hash_md5: string | null;
  rom_hash_sha1: string | null;
  rom_hash_sha256: string | null;
  rom_hash_crc32: string | null;
  rom_filename: string | null;
  rom_size_bytes: number | null;
  tier: string | null;
  rating: number | null;
  players_min: number;
  players_max: number;
  is_verified: boolean;
  metadata: Record<string, unknown>;
  search_vector: unknown;
  created_at: Date;
  updated_at: Date;
}

export interface GameMetadataRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  game_id: string;
  source: string;
  igdb_id: number | null;
  igdb_url: string | null;
  summary: string | null;
  storyline: string | null;
  total_rating: number | null;
  total_rating_count: number | null;
  aggregated_rating: number | null;
  aggregated_rating_count: number | null;
  first_release_date: Date | null;
  genres: string[];
  themes: string[];
  keywords: string[];
  game_modes: string[];
  franchises: string[];
  alternative_names: string[];
  websites: Record<string, string>;
  age_ratings: Record<string, string>;
  involved_companies: Record<string, unknown>[];
  raw_data: Record<string, unknown>;
  fetched_at: Date;
  created_at: Date;
  updated_at: Date;
}

export interface GameArtworkRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  game_id: string;
  artwork_type: string;
  url: string | null;
  local_path: string | null;
  width: number | null;
  height: number | null;
  mime_type: string | null;
  file_size_bytes: number | null;
  source: string;
  igdb_image_id: string | null;
  is_primary: boolean;
  sort_order: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface GamePlatformRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  abbreviation: string | null;
  slug: string;
  igdb_id: number | null;
  generation: number | null;
  manufacturer: string | null;
  platform_family: string | null;
  category: string | null;
  release_date: Date | null;
  summary: string | null;
  is_active: boolean;
  sort_order: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface GameGenreRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  name: string;
  slug: string;
  igdb_id: number | null;
  description: string | null;
  parent_id: string | null;
  is_active: boolean;
  sort_order: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Tier Requirements
// =============================================================================

export interface TierRequirement {
  tier: string;
  label: string;
  description: string;
  min_rating: number | null;
  max_games: number | null;
  features: string[];
}

// =============================================================================
// API Request Types
// =============================================================================

export interface LookupGameRequest {
  title?: string;
  hash?: string;
  hash_type?: 'md5' | 'sha1' | 'sha256' | 'crc32';
  platform?: string;
}

export interface CreateGameRequest {
  title: string;
  platform_id?: string;
  genre_id?: string;
  release_date?: string;
  developer?: string;
  publisher?: string;
  description?: string;
  igdb_id?: number;
  rom_hash_md5?: string;
  rom_hash_sha1?: string;
  rom_hash_sha256?: string;
  rom_hash_crc32?: string;
  rom_filename?: string;
  rom_size_bytes?: number;
  tier?: string;
  rating?: number;
  players_min?: number;
  players_max?: number;
}

export interface UpdateGameRequest {
  title?: string;
  platform_id?: string;
  genre_id?: string;
  release_date?: string;
  developer?: string;
  publisher?: string;
  description?: string;
  igdb_id?: number;
  rom_hash_md5?: string;
  rom_hash_sha1?: string;
  rom_hash_sha256?: string;
  rom_hash_crc32?: string;
  rom_filename?: string;
  rom_size_bytes?: number;
  tier?: string;
  rating?: number;
  players_min?: number;
  players_max?: number;
  is_verified?: boolean;
}

export interface SearchGamesRequest {
  query: string;
  platform_id?: string;
  genre_id?: string;
  tier?: string;
  is_verified?: boolean;
  limit?: number;
}

export interface EnrichGameRequest {
  game_id: string;
  force?: boolean;
}

export interface ListGamesQuery {
  platform_id?: string;
  genre_id?: string;
  tier?: string;
  is_verified?: string;
  limit?: number;
  offset?: number;
}

export interface CreatePlatformRequest {
  name: string;
  abbreviation?: string;
  igdb_id?: number;
  generation?: number;
  manufacturer?: string;
  platform_family?: string;
  category?: string;
  release_date?: string;
  summary?: string;
}

export interface CreateGenreRequest {
  name: string;
  igdb_id?: number;
  description?: string;
  parent_id?: string;
}

export interface CreateArtworkRequest {
  game_id: string;
  artwork_type: string;
  url?: string;
  local_path?: string;
  width?: number;
  height?: number;
  mime_type?: string;
  file_size_bytes?: number;
  source?: string;
  igdb_image_id?: string;
  is_primary?: boolean;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface GameWithDetails {
  game: GameCatalogRecord;
  metadata: GameMetadataRecord | null;
  artwork: GameArtworkRecord[];
  platform: GamePlatformRecord | null;
  genre: GameGenreRecord | null;
}

export interface EnrichResult {
  game_id: string;
  igdb_id: number | null;
  metadata_updated: boolean;
  artwork_count: number;
  errors: string[];
}

export interface GameMetadataStats {
  total_games: number;
  verified_games: number;
  total_platforms: number;
  total_genres: number;
  total_artwork: number;
  total_metadata: number;
  games_with_igdb: number;
  games_with_hashes: number;
  tier_breakdown: Record<string, number>;
}
