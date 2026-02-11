/**
 * Sports Plugin Types
 * Complete type definitions for sports schedule, teams, events, and sync tracking
 */

// =============================================================================
// Enums and Literals
// =============================================================================

export type EventStatus =
  | 'scheduled'
  | 'pregame'
  | 'in_progress'
  | 'halftime'
  | 'delayed'
  | 'suspended'
  | 'resumed'
  | 'final'
  | 'postponed'
  | 'rescheduled'
  | 'cancelled';

export type EventType = 'regular' | 'playoff' | 'preseason' | 'allstar' | 'exhibition';

export type SyncStatus = 'pending' | 'running' | 'completed' | 'failed';

export type Sport = 'football' | 'basketball' | 'baseball' | 'hockey' | 'soccer' | 'other';

// =============================================================================
// Database Record Types
// =============================================================================

export interface LeagueRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  external_id: string;
  provider: string;
  name: string;
  abbreviation: string | null;
  sport: Sport;
  country: string | null;
  season_type: string | null;
  current_season: string | null;
  logo_url: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface TeamRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  league_id: string | null;
  external_id: string;
  provider: string;
  name: string;
  abbreviation: string | null;
  city: string | null;
  conference: string | null;
  division: string | null;
  logo_url: string | null;
  primary_color: string | null;
  secondary_color: string | null;
  venue_name: string | null;
  venue_city: string | null;
  venue_timezone: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
}

export interface EventRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  external_id: string;
  provider: string;
  canonical_id: string | null;
  league_id: string | null;
  home_team_id: string | null;
  away_team_id: string | null;
  event_type: EventType;
  status: EventStatus;
  scheduled_at: Date;
  started_at: Date | null;
  ended_at: Date | null;
  venue_name: string | null;
  venue_city: string | null;
  venue_timezone: string | null;
  broadcast_network: string | null;
  broadcast_channel: string | null;
  season: string | null;
  season_type: string | null;
  week: number | null;
  home_score: number | null;
  away_score: number | null;
  period: string | null;
  clock: string | null;
  is_final: boolean;
  is_locked: boolean;
  lock_reason: string | null;
  locked_at: Date | null;
  operator_override: boolean;
  operator_notes: string | null;
  recording_trigger_sent: boolean;
  recording_trigger_sent_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  synced_at: Date;
  deleted_at: Date | null;
}

export interface ProviderSyncRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  provider: string;
  resource_type: string;
  status: SyncStatus;
  started_at: Date | null;
  completed_at: Date | null;
  records_synced: number;
  errors: unknown[];
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface ScheduleCacheRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  provider: string;
  cache_key: string;
  data: Record<string, unknown>;
  fetched_at: Date;
  expires_at: Date;
}

export interface WebhookEventRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  provider: string;
  event_type: string;
  event_id: string | null;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Request Types
// =============================================================================

export interface UpsertLeagueRequest {
  external_id: string;
  provider: string;
  name: string;
  abbreviation?: string;
  sport: Sport;
  country?: string;
  season_type?: string;
  current_season?: string;
  logo_url?: string;
  metadata?: Record<string, unknown>;
}

export interface UpsertTeamRequest {
  league_id?: string;
  external_id: string;
  provider: string;
  name: string;
  abbreviation?: string;
  city?: string;
  conference?: string;
  division?: string;
  logo_url?: string;
  primary_color?: string;
  secondary_color?: string;
  venue_name?: string;
  venue_city?: string;
  venue_timezone?: string;
  metadata?: Record<string, unknown>;
}

export interface UpsertEventRequest {
  external_id: string;
  provider: string;
  canonical_id?: string;
  league_id?: string;
  home_team_id?: string;
  away_team_id?: string;
  event_type?: EventType;
  status?: EventStatus;
  scheduled_at: string | Date;
  started_at?: string | Date;
  ended_at?: string | Date;
  venue_name?: string;
  venue_city?: string;
  venue_timezone?: string;
  broadcast_network?: string;
  broadcast_channel?: string;
  season?: string;
  season_type?: string;
  week?: number;
  home_score?: number;
  away_score?: number;
  period?: string;
  clock?: string;
  is_final?: boolean;
  metadata?: Record<string, unknown>;
}

export interface LockEventRequest {
  reason: string;
}

export interface OverrideEventRequest {
  scheduled_at?: string;
  broadcast_channel?: string;
  notes: string;
}

export interface TriggerRecordingRequest {
  recording_plugin_url?: string;
}

export interface SyncRequest {
  providers?: string[];
  resources?: string[];
  leagues?: string[];
}

export interface ReconcileRequest {
  lookback_days?: number;
}

// =============================================================================
// Response Types
// =============================================================================

export interface EventWithDetails extends EventRecord {
  league_name?: string;
  sport?: string;
  home_team_name?: string;
  home_abbr?: string;
  away_team_name?: string;
  away_abbr?: string;
}

export interface SyncStats {
  leagues: number;
  teams: number;
  events: number;
  by_provider: Record<string, { leagues: number; teams: number; events: number }>;
  last_sync: Date | null;
}

export interface CacheStats {
  entries: number;
  expired: number;
  active: number;
}

export interface PluginStats {
  leagues: number;
  teams: number;
  events: number;
  upcoming_events: number;
  live_events: number;
  by_provider: Record<string, number>;
  last_sync: Date | null;
}
