/**
 * Retro Gaming Plugin Types
 * Complete type definitions for ROMs, save states, play sessions, emulator cores,
 * controller configs, and core installations
 */

// =============================================================================
// Configuration
// =============================================================================

export interface RetroGamingConfig {
  databaseUrl: string;
  port: number;
  host: string;
  igdbClientId: string;
  igdbClientSecret: string;
  igdbApiUrl: string;
  igdbOAuthUrl: string;
  mobyGamesApiKey: string;
  storageBucket: string;
  romPathPrefix: string;
  saveStatePathPrefix: string;
  corePathPrefix: string;
  cdnUrl: string;
  logLevel: string;

  // Database connection params
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Security
  security: import('@nself/plugin-utils').SecurityConfig;
}

// =============================================================================
// Database Record Types
// =============================================================================

export interface RomRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  rom_file_path: string;
  rom_file_size_bytes: number | null;
  rom_file_hash: string | null;
  game_title: string;
  game_title_normalized: string;
  platform: string;
  region: string | null;
  release_year: number | null;
  genre: string | null;
  publisher: string | null;
  developer: string | null;
  igdb_id: number | null;
  moby_games_id: number | null;
  box_art_url: string | null;
  box_art_local_path: string | null;
  screenshot_urls: string[];
  screenshot_local_paths: string[];
  description: string | null;
  description_source: string | null;
  recommended_core: string | null;
  core_overrides: Record<string, unknown>;
  user_rating: number | null;
  play_count: number;
  last_played_at: Date | null;
  favorite: boolean;
  scan_source: string | null;
  added_by_user_id: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface SaveStateRecord {
  [key: string]: unknown;
  id: string;
  user_id: string;
  rom_id: string;
  source_account_id: string;
  slot: number;
  save_state_file_path: string;
  save_state_file_size_bytes: number | null;
  screenshot_url: string | null;
  screenshot_local_path: string | null;
  emulator_core: string;
  emulator_version: string | null;
  description: string | null;
  play_time_seconds: number;
  created_at: Date;
  updated_at: Date;
}

export interface PlaySessionRecord {
  [key: string]: unknown;
  id: string;
  user_id: string;
  rom_id: string;
  source_account_id: string;
  platform: string;
  device_id: string | null;
  emulator_core: string;
  started_at: Date;
  ended_at: Date | null;
  duration_seconds: number | null;
  save_state_id: string | null;
  auto_save_created: boolean;
  controller_type: string | null;
  created_at: Date;
}

export interface EmulatorCoreRecord {
  [key: string]: unknown;
  id: string;
  core_name: string;
  display_name: string;
  platform: string;
  core_wasm_path: string | null;
  core_wasm_size_bytes: number | null;
  version: string;
  license: string | null;
  author: string | null;
  homepage_url: string | null;
  supports_save_states: boolean;
  supports_rewind: boolean;
  supports_fast_forward: boolean;
  supports_cheats: boolean;
  default_config: Record<string, unknown>;
  is_recommended: boolean;
  priority: number;
  created_at: Date;
  updated_at: Date;
}

export interface ControllerConfigRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  user_id: string;
  config_name: string;
  platform: string | null;
  controller_type: string;
  button_mapping: Record<string, unknown>;
  touch_layout: Record<string, unknown>;
  analog_sensitivity: number;
  vibration_enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface CoreInstallationRecord {
  [key: string]: unknown;
  id: string;
  user_id: string;
  source_account_id: string;
  device_id: string;
  device_platform: string;
  core_name: string;
  core_version: string;
  installed_at: Date;
  last_used_at: Date | null;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface CreateRomRequest {
  rom_file_path: string;
  rom_file_size_bytes?: number;
  rom_file_hash?: string;
  game_title: string;
  platform: string;
  region?: string;
  release_year?: number;
  genre?: string;
  publisher?: string;
  developer?: string;
  igdb_id?: number;
  moby_games_id?: number;
  box_art_url?: string;
  box_art_local_path?: string;
  screenshot_urls?: string[];
  screenshot_local_paths?: string[];
  description?: string;
  description_source?: string;
  recommended_core?: string;
  core_overrides?: Record<string, unknown>;
  scan_source?: string;
  added_by_user_id?: string;
}

export interface UpdateRomRequest {
  game_title?: string;
  platform?: string;
  region?: string;
  release_year?: number;
  genre?: string;
  publisher?: string;
  developer?: string;
  igdb_id?: number;
  moby_games_id?: number;
  box_art_url?: string;
  box_art_local_path?: string;
  screenshot_urls?: string[];
  screenshot_local_paths?: string[];
  description?: string;
  description_source?: string;
  recommended_core?: string;
  core_overrides?: Record<string, unknown>;
  user_rating?: number;
  favorite?: boolean;
}

export interface ScanRomsRequest {
  files: Array<{
    rom_file_path: string;
    rom_file_size_bytes?: number;
    rom_file_hash?: string;
    game_title?: string;
    platform?: string;
  }>;
  scan_source?: string;
  auto_enrich?: boolean;
}

export interface CreateSaveStateRequest {
  user_id: string;
  slot: number;
  save_state_file_path: string;
  save_state_file_size_bytes?: number;
  screenshot_url?: string;
  screenshot_local_path?: string;
  emulator_core: string;
  emulator_version?: string;
  description?: string;
  play_time_seconds?: number;
}

export interface StartSessionRequest {
  user_id: string;
  rom_id: string;
  platform: string;
  device_id?: string;
  emulator_core: string;
  controller_type?: string;
}

export interface EndSessionRequest {
  save_state_id?: string;
  auto_save_created?: boolean;
}

export interface CreateControllerConfigRequest {
  user_id: string;
  config_name: string;
  platform?: string;
  controller_type: string;
  button_mapping: Record<string, unknown>;
  touch_layout?: Record<string, unknown>;
  analog_sensitivity?: number;
  vibration_enabled?: boolean;
}

export interface RecordCoreInstallationRequest {
  user_id: string;
  device_id: string;
  device_platform: string;
  core_name: string;
  core_version: string;
}

export interface ListRomsQuery {
  platform?: string;
  genre?: string;
  favorite?: string;
  search?: string;
  sort?: string;
  limit?: number;
  offset?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface RomStats {
  total_roms: number;
  total_platforms: number;
  total_play_sessions: number;
  total_play_time_seconds: number;
  total_save_states: number;
  total_favorites: number;
  roms_by_platform: Array<{ platform: string; count: number }>;
  most_played: Array<{ rom_id: string; game_title: string; play_count: number }>;
}

export interface ScanRomsResponse {
  roms_created: number;
  roms_skipped: number;
  errors: string[];
}

// =============================================================================
// IGDB Types
// =============================================================================

export interface IgdbAuthResponse {
  access_token: string;
  expires_in: number;
  token_type: string;
}

export interface IgdbGame {
  id: number;
  name: string;
  summary?: string;
  storyline?: string;
  first_release_date?: number;
  cover?: { url: string };
  screenshots?: Array<{ url: string }>;
  genres?: Array<{ name: string }>;
  involved_companies?: Array<{
    company: { name: string };
    publisher: boolean;
    developer: boolean;
  }>;
  platforms?: Array<{ name: string; abbreviation?: string }>;
}
