/**
 * Sports Data Plugin Types
 * Complete type definitions for leagues, teams, games, standings, players, and favorites
 */

// =============================================================================
// Database Record Types
// =============================================================================

export interface LeagueRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  external_id: string | null;
  provider: string;
  name: string;
  abbreviation: string | null;
  sport: string;
  country: string | null;
  logo_url: string | null;
  season_year: number | null;
  season_type: string | null;
  active: boolean;
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface TeamRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  league_id: string | null;
  external_id: string | null;
  provider: string;
  name: string;
  abbreviation: string | null;
  city: string | null;
  venue: string | null;
  logo_url: string | null;
  primary_color: string | null;
  secondary_color: string | null;
  conference: string | null;
  division: string | null;
  active: boolean;
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface GameRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  league_id: string | null;
  external_id: string | null;
  provider: string;
  home_team_id: string | null;
  away_team_id: string | null;
  status: 'queued' | 'scheduled' | 'in_progress' | 'completed' | 'archived' | 'postponed' | 'cancelled';
  scheduled_at: Date;
  started_at: Date | null;
  ended_at: Date | null;
  home_score: number | null;
  away_score: number | null;
  period: string | null;
  clock: string | null;
  venue: string | null;
  broadcast: string[] | null;
  odds: Record<string, unknown> | null;
  box_score: Record<string, unknown> | null;
  scoring_plays: Record<string, unknown>[];
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface StandingRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  league_id: string;
  team_id: string;
  season_year: number;
  season_type: string;
  conference: string | null;
  division: string | null;
  wins: number;
  losses: number;
  ties: number;
  overtime_losses: number;
  win_percentage: number;
  games_back: number | null;
  streak: string | null;
  last_10: string | null;
  rank_conference: number | null;
  rank_division: number | null;
  rank_overall: number | null;
  points_for: number;
  points_against: number;
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface PlayerRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  team_id: string | null;
  external_id: string | null;
  provider: string;
  name: string;
  position: string | null;
  jersey_number: number | null;
  height: string | null;
  weight: number | null;
  birth_date: Date | null;
  photo_url: string | null;
  status: string;
  metadata: Record<string, unknown>;
  synced_at: Date;
}

export interface FavoriteRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  user_id: string;
  favorite_type: 'team' | 'league' | 'player';
  favorite_id: string;
  created_at: Date;
}

export interface SyncStateRecord {
  [key: string]: unknown;
  id: string;
  source_account_id: string;
  provider: string;
  resource_type: string;
  last_sync_at: Date | null;
  next_sync_at: Date | null;
  sync_cursor: string | null;
  status: 'idle' | 'syncing' | 'error';
  error: string | null;
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

export interface ListGamesQuery {
  league_id?: string;
  team_id?: string;
  status?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export interface ListTeamsQuery {
  league_id?: string;
  conference?: string;
  division?: string;
  limit?: number;
  offset?: number;
}

export interface ListLeaguesQuery {
  sport?: string;
  active?: string;
  limit?: number;
  offset?: number;
}

export interface ListStandingsQuery {
  league_id?: string;
  season_year?: number;
  season_type?: string;
  conference?: string;
}

export interface ListPlayersQuery {
  team_id?: string;
  position?: string;
  limit?: number;
  offset?: number;
}

export interface SearchPlayersQuery {
  query: string;
  limit?: number;
}

export interface CreateFavoriteRequest {
  user_id: string;
  favorite_type: 'team' | 'league' | 'player';
  favorite_id: string;
}

export interface ListFavoritesQuery {
  user_id: string;
}

export interface TriggerSyncRequest {
  provider?: string;
  resources?: string[];
}

export interface ScoresQuery {
  league_id?: string;
  date?: string;
}

export interface UpcomingGamesQuery {
  user_id?: string;
  days?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface GameWithTeams extends GameRecord {
  home_team_name: string | null;
  home_team_abbreviation: string | null;
  home_team_logo_url: string | null;
  away_team_name: string | null;
  away_team_abbreviation: string | null;
  away_team_logo_url: string | null;
  league_name: string | null;
}

export interface StandingWithTeam extends StandingRecord {
  team_name: string;
  team_abbreviation: string | null;
  team_logo_url: string | null;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface SportsDataStats {
  total_leagues: number;
  total_teams: number;
  total_games: number;
  live_games: number;
  upcoming_games: number;
  completed_games: number;
  total_players: number;
  total_favorites: number;
  last_sync_at: Date | null;
}
