/**
 * Sports Plugin Database Operations
 * Complete CRUD operations for sports data in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  LeagueRecord,
  TeamRecord,
  EventRecord,
  EventWithDetails,
  ProviderSyncRecord,
  ScheduleCacheRecord,
  UpsertLeagueRequest,
  UpsertTeamRequest,
  UpsertEventRequest,
  SyncStats,
  CacheStats,
  PluginStats,
  SyncStatus,
} from './types.js';

const logger = createLogger('sports:db');

export class SportsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): SportsDatabase {
    return new SportsDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');

    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing sports schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Leagues
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_leagues (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        external_id VARCHAR(255) NOT NULL,
        provider VARCHAR(64) NOT NULL,
        name VARCHAR(255) NOT NULL,
        abbreviation VARCHAR(32),
        sport VARCHAR(64) NOT NULL,
        country VARCHAR(3),
        season_type VARCHAR(32),
        current_season VARCHAR(32),
        logo_url TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_leagues_source_account
        ON sports_leagues(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_leagues_sport
        ON sports_leagues(sport);
      CREATE INDEX IF NOT EXISTS idx_sports_leagues_provider
        ON sports_leagues(provider);

      -- =====================================================================
      -- Teams
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_teams (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        league_id UUID REFERENCES sports_leagues(id),
        external_id VARCHAR(255) NOT NULL,
        provider VARCHAR(64) NOT NULL,
        name VARCHAR(255) NOT NULL,
        abbreviation VARCHAR(16),
        city VARCHAR(128),
        conference VARCHAR(128),
        division VARCHAR(128),
        logo_url TEXT,
        primary_color VARCHAR(7),
        secondary_color VARCHAR(7),
        venue_name VARCHAR(255),
        venue_city VARCHAR(128),
        venue_timezone VARCHAR(64),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_teams_source_account
        ON sports_teams(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_teams_league
        ON sports_teams(league_id);
      CREATE INDEX IF NOT EXISTS idx_sports_teams_abbr
        ON sports_teams(abbreviation);

      -- =====================================================================
      -- Events (games/matches)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        external_id VARCHAR(255) NOT NULL,
        provider VARCHAR(64) NOT NULL,
        canonical_id VARCHAR(255),
        league_id UUID REFERENCES sports_leagues(id),
        home_team_id UUID REFERENCES sports_teams(id),
        away_team_id UUID REFERENCES sports_teams(id),
        event_type VARCHAR(32) NOT NULL DEFAULT 'regular',
        status VARCHAR(32) NOT NULL DEFAULT 'scheduled',
        scheduled_at TIMESTAMPTZ NOT NULL,
        started_at TIMESTAMPTZ,
        ended_at TIMESTAMPTZ,
        venue_name VARCHAR(255),
        venue_city VARCHAR(128),
        venue_timezone VARCHAR(64),
        broadcast_network VARCHAR(128),
        broadcast_channel VARCHAR(128),
        season VARCHAR(32),
        season_type VARCHAR(32),
        week INTEGER,
        home_score INTEGER,
        away_score INTEGER,
        period VARCHAR(32),
        clock VARCHAR(16),
        is_final BOOLEAN DEFAULT FALSE,
        is_locked BOOLEAN DEFAULT FALSE,
        lock_reason VARCHAR(255),
        locked_at TIMESTAMPTZ,
        operator_override BOOLEAN DEFAULT FALSE,
        operator_notes TEXT,
        recording_trigger_sent BOOLEAN DEFAULT FALSE,
        recording_trigger_sent_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        deleted_at TIMESTAMPTZ,
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_events_source_account
        ON sports_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_events_league
        ON sports_events(league_id);
      CREATE INDEX IF NOT EXISTS idx_sports_events_scheduled
        ON sports_events(scheduled_at);
      CREATE INDEX IF NOT EXISTS idx_sports_events_status
        ON sports_events(status);
      CREATE INDEX IF NOT EXISTS idx_sports_events_home
        ON sports_events(home_team_id);
      CREATE INDEX IF NOT EXISTS idx_sports_events_away
        ON sports_events(away_team_id);
      CREATE INDEX IF NOT EXISTS idx_sports_events_canonical
        ON sports_events(canonical_id);
      CREATE INDEX IF NOT EXISTS idx_sports_events_season
        ON sports_events(season, week);

      -- =====================================================================
      -- Provider Sync Tracking
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_provider_syncs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(64) NOT NULL,
        resource_type VARCHAR(64) NOT NULL,
        status VARCHAR(32) NOT NULL DEFAULT 'pending',
        started_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        records_synced INTEGER DEFAULT 0,
        errors JSONB DEFAULT '[]',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_sports_syncs_source_account
        ON sports_provider_syncs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_syncs_provider
        ON sports_provider_syncs(provider);

      -- =====================================================================
      -- Schedule Cache
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_schedule_cache (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(64) NOT NULL,
        cache_key VARCHAR(255) NOT NULL,
        data JSONB NOT NULL,
        fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        expires_at TIMESTAMPTZ NOT NULL,
        UNIQUE(source_account_id, provider, cache_key)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_cache_source_account
        ON sports_schedule_cache(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_cache_expires
        ON sports_schedule_cache(expires_at);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_webhook_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(64) NOT NULL,
        event_type VARCHAR(128) NOT NULL,
        event_id VARCHAR(255),
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT FALSE,
        processed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_sports_webhooks_source_account
        ON sports_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_webhooks_type
        ON sports_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_sports_webhooks_processed
        ON sports_webhook_events(processed);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

      CREATE OR REPLACE VIEW sports_upcoming_events AS
      SELECT e.*, l.name AS league_name, l.sport,
             ht.name AS home_team_name, ht.abbreviation AS home_abbr,
             at2.name AS away_team_name, at2.abbreviation AS away_abbr
      FROM sports_events e
      JOIN sports_leagues l ON e.league_id = l.id
      LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
      LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
      WHERE e.status = 'scheduled'
        AND e.scheduled_at BETWEEN NOW() AND NOW() + INTERVAL '7 days'
        AND e.deleted_at IS NULL
      ORDER BY e.scheduled_at ASC;

      CREATE OR REPLACE VIEW sports_live_events AS
      SELECT e.*, l.name AS league_name, l.sport,
             ht.name AS home_team_name, ht.abbreviation AS home_abbr,
             at2.name AS away_team_name, at2.abbreviation AS away_abbr
      FROM sports_events e
      JOIN sports_leagues l ON e.league_id = l.id
      LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
      LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
      WHERE e.status IN ('in_progress', 'halftime', 'delayed')
        AND e.deleted_at IS NULL
      ORDER BY e.started_at ASC;

      CREATE OR REPLACE VIEW sports_untriggered_recordings AS
      SELECT e.*
      FROM sports_events e
      WHERE e.recording_trigger_sent = FALSE
        AND e.status = 'scheduled'
        AND e.scheduled_at BETWEEN NOW() AND NOW() + INTERVAL '24 hours'
        AND e.deleted_at IS NULL;
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Leagues
  // =========================================================================

  async upsertLeague(request: UpsertLeagueRequest): Promise<LeagueRecord> {
    const result = await this.query<LeagueRecord>(
      `INSERT INTO sports_leagues (
        source_account_id, external_id, provider, name, abbreviation,
        sport, country, season_type, current_season, logo_url, metadata,
        updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
      ON CONFLICT (source_account_id, provider, external_id)
      DO UPDATE SET
        name = EXCLUDED.name,
        abbreviation = COALESCE(EXCLUDED.abbreviation, sports_leagues.abbreviation),
        sport = EXCLUDED.sport,
        country = COALESCE(EXCLUDED.country, sports_leagues.country),
        season_type = COALESCE(EXCLUDED.season_type, sports_leagues.season_type),
        current_season = COALESCE(EXCLUDED.current_season, sports_leagues.current_season),
        logo_url = COALESCE(EXCLUDED.logo_url, sports_leagues.logo_url),
        metadata = EXCLUDED.metadata,
        updated_at = NOW(),
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        request.external_id,
        request.provider,
        request.name,
        request.abbreviation ?? null,
        request.sport,
        request.country ?? null,
        request.season_type ?? null,
        request.current_season ?? null,
        request.logo_url ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getLeague(id: string): Promise<LeagueRecord | null> {
    const result = await this.query<LeagueRecord>(
      `SELECT * FROM sports_leagues WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listLeagues(sport?: string): Promise<LeagueRecord[]> {
    if (sport) {
      const result = await this.query<LeagueRecord>(
        `SELECT * FROM sports_leagues WHERE source_account_id = $1 AND sport = $2 ORDER BY name`,
        [this.sourceAccountId, sport]
      );
      return result.rows;
    }

    const result = await this.query<LeagueRecord>(
      `SELECT * FROM sports_leagues WHERE source_account_id = $1 ORDER BY name`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async deleteLeague(id: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM sports_leagues WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Teams
  // =========================================================================

  async upsertTeam(request: UpsertTeamRequest): Promise<TeamRecord> {
    const result = await this.query<TeamRecord>(
      `INSERT INTO sports_teams (
        source_account_id, league_id, external_id, provider, name, abbreviation,
        city, conference, division, logo_url, primary_color, secondary_color,
        venue_name, venue_city, venue_timezone, metadata, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW())
      ON CONFLICT (source_account_id, provider, external_id)
      DO UPDATE SET
        league_id = COALESCE(EXCLUDED.league_id, sports_teams.league_id),
        name = EXCLUDED.name,
        abbreviation = COALESCE(EXCLUDED.abbreviation, sports_teams.abbreviation),
        city = COALESCE(EXCLUDED.city, sports_teams.city),
        conference = COALESCE(EXCLUDED.conference, sports_teams.conference),
        division = COALESCE(EXCLUDED.division, sports_teams.division),
        logo_url = COALESCE(EXCLUDED.logo_url, sports_teams.logo_url),
        primary_color = COALESCE(EXCLUDED.primary_color, sports_teams.primary_color),
        secondary_color = COALESCE(EXCLUDED.secondary_color, sports_teams.secondary_color),
        venue_name = COALESCE(EXCLUDED.venue_name, sports_teams.venue_name),
        venue_city = COALESCE(EXCLUDED.venue_city, sports_teams.venue_city),
        venue_timezone = COALESCE(EXCLUDED.venue_timezone, sports_teams.venue_timezone),
        metadata = EXCLUDED.metadata,
        updated_at = NOW(),
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        request.league_id ?? null,
        request.external_id,
        request.provider,
        request.name,
        request.abbreviation ?? null,
        request.city ?? null,
        request.conference ?? null,
        request.division ?? null,
        request.logo_url ?? null,
        request.primary_color ?? null,
        request.secondary_color ?? null,
        request.venue_name ?? null,
        request.venue_city ?? null,
        request.venue_timezone ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getTeam(id: string): Promise<TeamRecord | null> {
    const result = await this.query<TeamRecord>(
      `SELECT * FROM sports_teams WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listTeams(options: { league_id?: string; sport?: string; search?: string } = {}): Promise<TeamRecord[]> {
    const conditions: string[] = ['t.source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (options.league_id) {
      params.push(options.league_id);
      conditions.push(`t.league_id = $${params.length}`);
    }

    if (options.sport) {
      params.push(options.sport);
      conditions.push(`l.sport = $${params.length}`);
    }

    if (options.search) {
      params.push(`%${options.search}%`);
      conditions.push(`(t.name ILIKE $${params.length} OR t.city ILIKE $${params.length} OR t.abbreviation ILIKE $${params.length})`);
    }

    const result = await this.query<TeamRecord>(
      `SELECT t.* FROM sports_teams t
       LEFT JOIN sports_leagues l ON t.league_id = l.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY t.name`,
      params
    );
    return result.rows;
  }

  async deleteTeam(id: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM sports_teams WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Events
  // =========================================================================

  async upsertEvent(request: UpsertEventRequest): Promise<EventRecord> {
    const scheduledAt = request.scheduled_at instanceof Date
      ? request.scheduled_at.toISOString()
      : request.scheduled_at;
    const startedAt = request.started_at
      ? (request.started_at instanceof Date ? request.started_at.toISOString() : request.started_at)
      : null;
    const endedAt = request.ended_at
      ? (request.ended_at instanceof Date ? request.ended_at.toISOString() : request.ended_at)
      : null;

    const result = await this.query<EventRecord>(
      `INSERT INTO sports_events (
        source_account_id, external_id, provider, canonical_id,
        league_id, home_team_id, away_team_id, event_type, status,
        scheduled_at, started_at, ended_at, venue_name, venue_city,
        venue_timezone, broadcast_network, broadcast_channel,
        season, season_type, week, home_score, away_score,
        period, clock, is_final, metadata, updated_at, synced_at
      ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
        $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, NOW(), NOW()
      )
      ON CONFLICT (source_account_id, provider, external_id)
      DO UPDATE SET
        canonical_id = COALESCE(EXCLUDED.canonical_id, sports_events.canonical_id),
        league_id = COALESCE(EXCLUDED.league_id, sports_events.league_id),
        home_team_id = COALESCE(EXCLUDED.home_team_id, sports_events.home_team_id),
        away_team_id = COALESCE(EXCLUDED.away_team_id, sports_events.away_team_id),
        event_type = COALESCE(EXCLUDED.event_type, sports_events.event_type),
        status = CASE
          WHEN sports_events.is_locked THEN sports_events.status
          ELSE COALESCE(EXCLUDED.status, sports_events.status)
        END,
        scheduled_at = CASE
          WHEN sports_events.is_locked THEN sports_events.scheduled_at
          ELSE COALESCE(EXCLUDED.scheduled_at, sports_events.scheduled_at)
        END,
        started_at = COALESCE(EXCLUDED.started_at, sports_events.started_at),
        ended_at = COALESCE(EXCLUDED.ended_at, sports_events.ended_at),
        venue_name = COALESCE(EXCLUDED.venue_name, sports_events.venue_name),
        venue_city = COALESCE(EXCLUDED.venue_city, sports_events.venue_city),
        venue_timezone = COALESCE(EXCLUDED.venue_timezone, sports_events.venue_timezone),
        broadcast_network = COALESCE(EXCLUDED.broadcast_network, sports_events.broadcast_network),
        broadcast_channel = COALESCE(EXCLUDED.broadcast_channel, sports_events.broadcast_channel),
        season = COALESCE(EXCLUDED.season, sports_events.season),
        season_type = COALESCE(EXCLUDED.season_type, sports_events.season_type),
        week = COALESCE(EXCLUDED.week, sports_events.week),
        home_score = COALESCE(EXCLUDED.home_score, sports_events.home_score),
        away_score = COALESCE(EXCLUDED.away_score, sports_events.away_score),
        period = COALESCE(EXCLUDED.period, sports_events.period),
        clock = COALESCE(EXCLUDED.clock, sports_events.clock),
        is_final = COALESCE(EXCLUDED.is_final, sports_events.is_final),
        metadata = EXCLUDED.metadata,
        updated_at = NOW(),
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        request.external_id,
        request.provider,
        request.canonical_id ?? null,
        request.league_id ?? null,
        request.home_team_id ?? null,
        request.away_team_id ?? null,
        request.event_type ?? 'regular',
        request.status ?? 'scheduled',
        scheduledAt,
        startedAt,
        endedAt,
        request.venue_name ?? null,
        request.venue_city ?? null,
        request.venue_timezone ?? null,
        request.broadcast_network ?? null,
        request.broadcast_channel ?? null,
        request.season ?? null,
        request.season_type ?? null,
        request.week ?? null,
        request.home_score ?? null,
        request.away_score ?? null,
        request.period ?? null,
        request.clock ?? null,
        request.is_final ?? false,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getEvent(id: string): Promise<EventWithDetails | null> {
    const result = await this.query<EventWithDetails>(
      `SELECT e.*, l.name AS league_name, l.sport,
              ht.name AS home_team_name, ht.abbreviation AS home_abbr,
              at2.name AS away_team_name, at2.abbreviation AS away_abbr
       FROM sports_events e
       LEFT JOIN sports_leagues l ON e.league_id = l.id
       LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
       LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
       WHERE e.id = $1 AND e.source_account_id = $2 AND e.deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listEvents(options: {
    league_id?: string;
    team_id?: string;
    status?: string;
    from?: string;
    to?: string;
    season?: string;
    week?: number;
    limit?: number;
    offset?: number;
  } = {}): Promise<{ data: EventWithDetails[]; total: number }> {
    const conditions: string[] = ['e.source_account_id = $1', 'e.deleted_at IS NULL'];
    const params: unknown[] = [this.sourceAccountId];

    if (options.league_id) {
      params.push(options.league_id);
      conditions.push(`e.league_id = $${params.length}`);
    }

    if (options.team_id) {
      params.push(options.team_id);
      conditions.push(`(e.home_team_id = $${params.length} OR e.away_team_id = $${params.length})`);
    }

    if (options.status) {
      params.push(options.status);
      conditions.push(`e.status = $${params.length}`);
    }

    if (options.from) {
      params.push(options.from);
      conditions.push(`e.scheduled_at >= $${params.length}`);
    }

    if (options.to) {
      params.push(options.to);
      conditions.push(`e.scheduled_at <= $${params.length}`);
    }

    if (options.season) {
      params.push(options.season);
      conditions.push(`e.season = $${params.length}`);
    }

    if (options.week !== undefined) {
      params.push(options.week);
      conditions.push(`e.week = $${params.length}`);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.query<{ count: number }>(
      `SELECT COUNT(*) as count FROM sports_events e WHERE ${whereClause}`,
      params
    );
    const total = countResult.rows[0]?.count ?? 0;

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;
    params.push(limit);
    params.push(offset);

    const result = await this.query<EventWithDetails>(
      `SELECT e.*, l.name AS league_name, l.sport,
              ht.name AS home_team_name, ht.abbreviation AS home_abbr,
              at2.name AS away_team_name, at2.abbreviation AS away_abbr
       FROM sports_events e
       LEFT JOIN sports_leagues l ON e.league_id = l.id
       LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
       LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
       WHERE ${whereClause}
       ORDER BY e.scheduled_at ASC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return { data: result.rows, total };
  }

  async getUpcomingEvents(options: { league_id?: string; team_id?: string; limit?: number } = {}): Promise<EventWithDetails[]> {
    const conditions: string[] = [
      'e.source_account_id = $1',
      'e.deleted_at IS NULL',
      'e.status = \'scheduled\'',
      'e.scheduled_at BETWEEN NOW() AND NOW() + INTERVAL \'7 days\'',
    ];
    const params: unknown[] = [this.sourceAccountId];

    if (options.league_id) {
      params.push(options.league_id);
      conditions.push(`e.league_id = $${params.length}`);
    }

    if (options.team_id) {
      params.push(options.team_id);
      conditions.push(`(e.home_team_id = $${params.length} OR e.away_team_id = $${params.length})`);
    }

    const limit = options.limit ?? 50;
    params.push(limit);

    const result = await this.query<EventWithDetails>(
      `SELECT e.*, l.name AS league_name, l.sport,
              ht.name AS home_team_name, ht.abbreviation AS home_abbr,
              at2.name AS away_team_name, at2.abbreviation AS away_abbr
       FROM sports_events e
       LEFT JOIN sports_leagues l ON e.league_id = l.id
       LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
       LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY e.scheduled_at ASC
       LIMIT $${params.length}`,
      params
    );

    return result.rows;
  }

  async getLiveEvents(leagueId?: string): Promise<EventWithDetails[]> {
    const conditions: string[] = [
      'e.source_account_id = $1',
      'e.deleted_at IS NULL',
      "e.status IN ('in_progress', 'halftime', 'delayed')",
    ];
    const params: unknown[] = [this.sourceAccountId];

    if (leagueId) {
      params.push(leagueId);
      conditions.push(`e.league_id = $${params.length}`);
    }

    const result = await this.query<EventWithDetails>(
      `SELECT e.*, l.name AS league_name, l.sport,
              ht.name AS home_team_name, ht.abbreviation AS home_abbr,
              at2.name AS away_team_name, at2.abbreviation AS away_abbr
       FROM sports_events e
       LEFT JOIN sports_leagues l ON e.league_id = l.id
       LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
       LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY e.started_at ASC`,
      params
    );

    return result.rows;
  }

  async getTodayEvents(options: { league_id?: string; timezone?: string } = {}): Promise<EventWithDetails[]> {
    const tz = options.timezone ?? 'UTC';
    const conditions: string[] = [
      'e.source_account_id = $1',
      'e.deleted_at IS NULL',
      `DATE(e.scheduled_at AT TIME ZONE $2) = CURRENT_DATE`,
    ];
    const params: unknown[] = [this.sourceAccountId, tz];

    if (options.league_id) {
      params.push(options.league_id);
      conditions.push(`e.league_id = $${params.length}`);
    }

    const result = await this.query<EventWithDetails>(
      `SELECT e.*, l.name AS league_name, l.sport,
              ht.name AS home_team_name, ht.abbreviation AS home_abbr,
              at2.name AS away_team_name, at2.abbreviation AS away_abbr
       FROM sports_events e
       LEFT JOIN sports_leagues l ON e.league_id = l.id
       LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
       LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY e.scheduled_at ASC`,
      params
    );

    return result.rows;
  }

  async lockEvent(id: string, reason: string): Promise<EventRecord | null> {
    const result = await this.query<EventRecord>(
      `UPDATE sports_events
       SET is_locked = TRUE, lock_reason = $3, locked_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [id, this.sourceAccountId, reason]
    );
    return result.rows[0] ?? null;
  }

  async unlockEvent(id: string): Promise<EventRecord | null> {
    const result = await this.query<EventRecord>(
      `UPDATE sports_events
       SET is_locked = FALSE, lock_reason = NULL, locked_at = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async overrideEvent(id: string, scheduledAt?: string, broadcastChannel?: string, notes?: string): Promise<EventRecord | null> {
    const setParts: string[] = [
      'operator_override = TRUE',
      'updated_at = NOW()',
    ];
    const params: unknown[] = [id, this.sourceAccountId];

    if (scheduledAt) {
      params.push(scheduledAt);
      setParts.push(`scheduled_at = $${params.length}`);
    }

    if (broadcastChannel) {
      params.push(broadcastChannel);
      setParts.push(`broadcast_channel = $${params.length}`);
    }

    if (notes) {
      params.push(notes);
      setParts.push(`operator_notes = $${params.length}`);
    }

    const result = await this.query<EventRecord>(
      `UPDATE sports_events
       SET ${setParts.join(', ')}
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async markRecordingTriggered(id: string): Promise<EventRecord | null> {
    const result = await this.query<EventRecord>(
      `UPDATE sports_events
       SET recording_trigger_sent = TRUE, recording_trigger_sent_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async softDeleteEvent(id: string): Promise<boolean> {
    const rowCount = await this.execute(
      `UPDATE sports_events SET deleted_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Provider Sync Tracking
  // =========================================================================

  async createSyncRecord(provider: string, resourceType: string): Promise<ProviderSyncRecord> {
    const result = await this.query<ProviderSyncRecord>(
      `INSERT INTO sports_provider_syncs (source_account_id, provider, resource_type, status, started_at)
       VALUES ($1, $2, $3, 'running', NOW())
       RETURNING *`,
      [this.sourceAccountId, provider, resourceType]
    );
    return result.rows[0];
  }

  async updateSyncRecord(id: string, status: SyncStatus, recordsSynced: number, errors?: unknown[]): Promise<void> {
    await this.execute(
      `UPDATE sports_provider_syncs
       SET status = $2, records_synced = $3, errors = $4,
           completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN NOW() ELSE NULL END
       WHERE id = $1`,
      [id, status, recordsSynced, JSON.stringify(errors ?? [])]
    );
  }

  async getLastSync(provider?: string): Promise<ProviderSyncRecord | null> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (provider) {
      params.push(provider);
      conditions.push(`provider = $${params.length}`);
    }

    const result = await this.query<ProviderSyncRecord>(
      `SELECT * FROM sports_provider_syncs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC LIMIT 1`,
      params
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Schedule Cache
  // =========================================================================

  async setCache(provider: string, cacheKey: string, data: Record<string, unknown>, ttlSeconds: number): Promise<void> {
    await this.execute(
      `INSERT INTO sports_schedule_cache (source_account_id, provider, cache_key, data, fetched_at, expires_at)
       VALUES ($1, $2, $3, $4, NOW(), NOW() + $5 * INTERVAL '1 second')
       ON CONFLICT (source_account_id, provider, cache_key)
       DO UPDATE SET data = EXCLUDED.data, fetched_at = NOW(), expires_at = NOW() + $5 * INTERVAL '1 second'`,
      [this.sourceAccountId, provider, cacheKey, JSON.stringify(data), ttlSeconds]
    );
  }

  async getCache(provider: string, cacheKey: string): Promise<ScheduleCacheRecord | null> {
    const result = await this.query<ScheduleCacheRecord>(
      `SELECT * FROM sports_schedule_cache
       WHERE source_account_id = $1 AND provider = $2 AND cache_key = $3 AND expires_at > NOW()`,
      [this.sourceAccountId, provider, cacheKey]
    );
    return result.rows[0] ?? null;
  }

  async getCacheStats(): Promise<CacheStats> {
    const result = await this.query<{ total: number; expired: number; active: number }>(
      `SELECT
         COUNT(*) as total,
         COUNT(*) FILTER (WHERE expires_at <= NOW()) as expired,
         COUNT(*) FILTER (WHERE expires_at > NOW()) as active
       FROM sports_schedule_cache
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      entries: row?.total ?? 0,
      expired: row?.expired ?? 0,
      active: row?.active ?? 0,
    };
  }

  async clearCache(): Promise<number> {
    return this.execute(
      `DELETE FROM sports_schedule_cache WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  async clearExpiredCache(): Promise<number> {
    return this.execute(
      `DELETE FROM sports_schedule_cache WHERE source_account_id = $1 AND expires_at <= NOW()`,
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(provider: string, eventType: string, payload: Record<string, unknown>, eventId?: string): Promise<void> {
    await this.execute(
      `INSERT INTO sports_webhook_events (source_account_id, provider, event_type, event_id, payload)
       VALUES ($1, $2, $3, $4, $5)`,
      [this.sourceAccountId, provider, eventType, eventId ?? null, JSON.stringify(payload)]
    );
  }

  async markWebhookProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE sports_webhook_events
       SET processed = TRUE, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [id, error ?? null]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getPluginStats(): Promise<PluginStats> {
    const result = await this.query<{
      leagues: number;
      teams: number;
      events: number;
      upcoming_events: number;
      live_events: number;
    }>(
      `WITH league_count AS (
        SELECT COUNT(*) as count FROM sports_leagues WHERE source_account_id = $1
      ),
      team_count AS (
        SELECT COUNT(*) as count FROM sports_teams WHERE source_account_id = $1
      ),
      event_count AS (
        SELECT COUNT(*) as count FROM sports_events WHERE source_account_id = $1 AND deleted_at IS NULL
      ),
      upcoming AS (
        SELECT COUNT(*) as count FROM sports_events
        WHERE source_account_id = $1 AND status = 'scheduled'
          AND scheduled_at BETWEEN NOW() AND NOW() + INTERVAL '7 days'
          AND deleted_at IS NULL
      ),
      live AS (
        SELECT COUNT(*) as count FROM sports_events
        WHERE source_account_id = $1 AND status IN ('in_progress', 'halftime', 'delayed')
          AND deleted_at IS NULL
      )
      SELECT
        lc.count as leagues,
        tc.count as teams,
        ec.count as events,
        u.count as upcoming_events,
        li.count as live_events
      FROM league_count lc
      CROSS JOIN team_count tc
      CROSS JOIN event_count ec
      CROSS JOIN upcoming u
      CROSS JOIN live li`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // By provider counts
    const providerResult = await this.query<{ provider: string; count: number }>(
      `SELECT provider, COUNT(*) as count FROM sports_events
       WHERE source_account_id = $1 AND deleted_at IS NULL
       GROUP BY provider`,
      [this.sourceAccountId]
    );

    const byProvider: Record<string, number> = {};
    for (const r of providerResult.rows) {
      byProvider[r.provider] = r.count;
    }

    const lastSync = await this.getLastSync();

    return {
      leagues: row?.leagues ?? 0,
      teams: row?.teams ?? 0,
      events: row?.events ?? 0,
      upcoming_events: row?.upcoming_events ?? 0,
      live_events: row?.live_events ?? 0,
      by_provider: byProvider,
      last_sync: lastSync?.completed_at ?? null,
    };
  }

  async getSyncStats(): Promise<SyncStats> {
    const stats = await this.getPluginStats();

    const providerDetailResult = await this.query<{ provider: string; leagues: number; teams: number; events: number }>(
      `WITH pl AS (
        SELECT provider, COUNT(*) as count FROM sports_leagues WHERE source_account_id = $1 GROUP BY provider
      ),
      pt AS (
        SELECT provider, COUNT(*) as count FROM sports_teams WHERE source_account_id = $1 GROUP BY provider
      ),
      pe AS (
        SELECT provider, COUNT(*) as count FROM sports_events WHERE source_account_id = $1 AND deleted_at IS NULL GROUP BY provider
      )
      SELECT
        COALESCE(pl.provider, pt.provider, pe.provider) as provider,
        COALESCE(pl.count, 0) as leagues,
        COALESCE(pt.count, 0) as teams,
        COALESCE(pe.count, 0) as events
      FROM pl
      FULL OUTER JOIN pt ON pl.provider = pt.provider
      FULL OUTER JOIN pe ON COALESCE(pl.provider, pt.provider) = pe.provider`,
      [this.sourceAccountId]
    );

    const byProvider: Record<string, { leagues: number; teams: number; events: number }> = {};
    for (const r of providerDetailResult.rows) {
      byProvider[r.provider] = {
        leagues: r.leagues,
        teams: r.teams,
        events: r.events,
      };
    }

    return {
      leagues: stats.leagues,
      teams: stats.teams,
      events: stats.events,
      by_provider: byProvider,
      last_sync: stats.last_sync,
    };
  }
}
