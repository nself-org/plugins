/**
 * Sports Data Database Operations
 * Complete CRUD operations for leagues, teams, games, standings, players, and favorites
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  LeagueRecord,
  TeamRecord,
  GameRecord,
  StandingRecord,
  PlayerRecord,
  FavoriteRecord,
  SyncStateRecord,
  GameWithTeams,
  StandingWithTeam,
  SportsDataStats,
} from './types.js';

const logger = createLogger('sports-data:db');

export class SportsDataDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): SportsDataDatabase {
    return new SportsDataDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing sports-data schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Leagues
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_leagues (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        external_id VARCHAR(255),
        provider VARCHAR(50) NOT NULL,
        name VARCHAR(255) NOT NULL,
        abbreviation VARCHAR(20),
        sport VARCHAR(50) NOT NULL,
        country VARCHAR(100),
        logo_url TEXT,
        season_year INTEGER,
        season_type VARCHAR(50),
        active BOOLEAN DEFAULT true,
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_leagues_source_app
        ON sports_leagues(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_leagues_sport
        ON sports_leagues(source_account_id, sport);

      -- =====================================================================
      -- Teams
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_teams (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        league_id UUID REFERENCES sports_leagues(id),
        external_id VARCHAR(255),
        provider VARCHAR(50) NOT NULL,
        name VARCHAR(255) NOT NULL,
        abbreviation VARCHAR(10),
        city VARCHAR(100),
        venue VARCHAR(255),
        logo_url TEXT,
        primary_color VARCHAR(7),
        secondary_color VARCHAR(7),
        conference VARCHAR(100),
        division VARCHAR(100),
        active BOOLEAN DEFAULT true,
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_teams_source_app
        ON sports_teams(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_teams_league
        ON sports_teams(league_id);

      -- =====================================================================
      -- Games
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_games (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        league_id UUID REFERENCES sports_leagues(id),
        external_id VARCHAR(255),
        provider VARCHAR(50) NOT NULL,
        home_team_id UUID REFERENCES sports_teams(id),
        away_team_id UUID REFERENCES sports_teams(id),
        status VARCHAR(30) NOT NULL DEFAULT 'scheduled',
        scheduled_at TIMESTAMPTZ NOT NULL,
        started_at TIMESTAMPTZ,
        ended_at TIMESTAMPTZ,
        home_score INTEGER,
        away_score INTEGER,
        period VARCHAR(20),
        clock VARCHAR(20),
        venue VARCHAR(255),
        broadcast TEXT[],
        odds JSONB,
        box_score JSONB,
        scoring_plays JSONB DEFAULT '[]',
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_games_source_app
        ON sports_games(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_games_league
        ON sports_games(league_id);
      CREATE INDEX IF NOT EXISTS idx_sports_games_scheduled
        ON sports_games(source_account_id, scheduled_at);
      CREATE INDEX IF NOT EXISTS idx_sports_games_status
        ON sports_games(source_account_id, status);
      CREATE INDEX IF NOT EXISTS idx_sports_games_teams
        ON sports_games(home_team_id, away_team_id);

      -- =====================================================================
      -- Standings
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_standings (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        league_id UUID NOT NULL REFERENCES sports_leagues(id),
        team_id UUID NOT NULL REFERENCES sports_teams(id),
        season_year INTEGER NOT NULL,
        season_type VARCHAR(50) DEFAULT 'regular',
        conference VARCHAR(100),
        division VARCHAR(100),
        wins INTEGER DEFAULT 0,
        losses INTEGER DEFAULT 0,
        ties INTEGER DEFAULT 0,
        overtime_losses INTEGER DEFAULT 0,
        win_percentage DOUBLE PRECISION DEFAULT 0,
        games_back DOUBLE PRECISION,
        streak VARCHAR(10),
        last_10 VARCHAR(10),
        rank_conference INTEGER,
        rank_division INTEGER,
        rank_overall INTEGER,
        points_for INTEGER DEFAULT 0,
        points_against INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, league_id, team_id, season_year, season_type)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_standings_source_app
        ON sports_standings(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_standings_league
        ON sports_standings(league_id, season_year);

      -- =====================================================================
      -- Players
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_players (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        team_id UUID REFERENCES sports_teams(id),
        external_id VARCHAR(255),
        provider VARCHAR(50) NOT NULL,
        name VARCHAR(255) NOT NULL,
        position VARCHAR(50),
        jersey_number INTEGER,
        height VARCHAR(20),
        weight INTEGER,
        birth_date DATE,
        photo_url TEXT,
        status VARCHAR(20) DEFAULT 'active',
        metadata JSONB DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, provider, external_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_players_source_app
        ON sports_players(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_players_team
        ON sports_players(team_id);

      -- =====================================================================
      -- Favorites
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_favorites (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        favorite_type VARCHAR(20) NOT NULL,
        favorite_id UUID NOT NULL,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, favorite_type, favorite_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_favorites_source_app
        ON sports_favorites(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sports_favorites_user
        ON sports_favorites(source_account_id, user_id);

      -- =====================================================================
      -- Sync State
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_sync_state (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider VARCHAR(50) NOT NULL,
        resource_type VARCHAR(50) NOT NULL,
        last_sync_at TIMESTAMPTZ,
        next_sync_at TIMESTAMPTZ,
        sync_cursor VARCHAR(255),
        status VARCHAR(20) DEFAULT 'idle',
        error TEXT,
        UNIQUE(source_account_id, provider, resource_type)
      );

      CREATE INDEX IF NOT EXISTS idx_sports_sync_state_source_app
        ON sports_sync_state(source_account_id);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sports_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_sports_webhook_events_source_app
        ON sports_webhook_events(source_account_id);
    `;

    await this.execute(schema);
    logger.info('Sports-data schema initialized successfully');
  }

  // =========================================================================
  // League Operations
  // =========================================================================

  async upsertLeague(league: Omit<LeagueRecord, 'id' | 'synced_at'>): Promise<LeagueRecord> {
    const result = await this.query<LeagueRecord>(
      `INSERT INTO sports_leagues (
        source_account_id, external_id, provider, name, abbreviation, sport,
        country, logo_url, season_year, season_type, active, metadata, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
      ON CONFLICT (source_account_id, provider, external_id) DO UPDATE SET
        name = EXCLUDED.name,
        abbreviation = EXCLUDED.abbreviation,
        logo_url = EXCLUDED.logo_url,
        season_year = EXCLUDED.season_year,
        season_type = EXCLUDED.season_type,
        active = EXCLUDED.active,
        metadata = EXCLUDED.metadata,
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, league.external_id, league.provider,
        league.name, league.abbreviation, league.sport,
        league.country, league.logo_url, league.season_year,
        league.season_type, league.active, JSON.stringify(league.metadata),
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

  async listLeagues(filters: { sport?: string; active?: boolean; limit?: number; offset?: number }): Promise<LeagueRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.sport) {
      conditions.push(`sport = $${paramIndex}`);
      values.push(filters.sport);
      paramIndex++;
    }

    if (filters.active !== undefined) {
      conditions.push(`active = $${paramIndex}`);
      values.push(filters.active);
      paramIndex++;
    }

    let sql = `SELECT * FROM sports_leagues WHERE ${conditions.join(' AND ')} ORDER BY name ASC`;
    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<LeagueRecord>(sql, values);
    return result.rows;
  }

  // =========================================================================
  // Team Operations
  // =========================================================================

  async upsertTeam(team: Omit<TeamRecord, 'id' | 'synced_at'>): Promise<TeamRecord> {
    const result = await this.query<TeamRecord>(
      `INSERT INTO sports_teams (
        source_account_id, league_id, external_id, provider, name, abbreviation,
        city, venue, logo_url, primary_color, secondary_color, conference,
        division, active, metadata, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
      ON CONFLICT (source_account_id, provider, external_id) DO UPDATE SET
        name = EXCLUDED.name,
        abbreviation = EXCLUDED.abbreviation,
        city = EXCLUDED.city,
        venue = EXCLUDED.venue,
        logo_url = EXCLUDED.logo_url,
        conference = EXCLUDED.conference,
        division = EXCLUDED.division,
        active = EXCLUDED.active,
        metadata = EXCLUDED.metadata,
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, team.league_id, team.external_id, team.provider,
        team.name, team.abbreviation, team.city, team.venue, team.logo_url,
        team.primary_color, team.secondary_color, team.conference,
        team.division, team.active, JSON.stringify(team.metadata),
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

  async listTeams(filters: {
    leagueId?: string; conference?: string; division?: string;
    limit?: number; offset?: number;
  }): Promise<TeamRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.leagueId) {
      conditions.push(`league_id = $${paramIndex}`);
      values.push(filters.leagueId);
      paramIndex++;
    }
    if (filters.conference) {
      conditions.push(`conference = $${paramIndex}`);
      values.push(filters.conference);
      paramIndex++;
    }
    if (filters.division) {
      conditions.push(`division = $${paramIndex}`);
      values.push(filters.division);
      paramIndex++;
    }

    let sql = `SELECT * FROM sports_teams WHERE ${conditions.join(' AND ')} ORDER BY name ASC`;
    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<TeamRecord>(sql, values);
    return result.rows;
  }

  // =========================================================================
  // Game Operations
  // =========================================================================

  async upsertGame(game: Omit<GameRecord, 'id' | 'synced_at'>): Promise<GameRecord> {
    const result = await this.query<GameRecord>(
      `INSERT INTO sports_games (
        source_account_id, league_id, external_id, provider, home_team_id,
        away_team_id, status, scheduled_at, started_at, ended_at, home_score,
        away_score, period, clock, venue, broadcast, odds, box_score,
        scoring_plays, metadata, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW())
      ON CONFLICT (source_account_id, provider, external_id) DO UPDATE SET
        status = EXCLUDED.status,
        started_at = EXCLUDED.started_at,
        ended_at = EXCLUDED.ended_at,
        home_score = EXCLUDED.home_score,
        away_score = EXCLUDED.away_score,
        period = EXCLUDED.period,
        clock = EXCLUDED.clock,
        broadcast = EXCLUDED.broadcast,
        odds = EXCLUDED.odds,
        box_score = EXCLUDED.box_score,
        scoring_plays = EXCLUDED.scoring_plays,
        metadata = EXCLUDED.metadata,
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, game.league_id, game.external_id, game.provider,
        game.home_team_id, game.away_team_id, game.status, game.scheduled_at,
        game.started_at, game.ended_at, game.home_score, game.away_score,
        game.period, game.clock, game.venue, game.broadcast,
        game.odds ? JSON.stringify(game.odds) : null,
        game.box_score ? JSON.stringify(game.box_score) : null,
        JSON.stringify(game.scoring_plays ?? []),
        JSON.stringify(game.metadata),
      ]
    );

    return result.rows[0];
  }

  async getGame(id: string): Promise<GameWithTeams | null> {
    const result = await this.query<GameWithTeams>(
      `SELECT g.*,
        ht.name as home_team_name, ht.abbreviation as home_team_abbreviation, ht.logo_url as home_team_logo_url,
        at.name as away_team_name, at.abbreviation as away_team_abbreviation, at.logo_url as away_team_logo_url,
        l.name as league_name
       FROM sports_games g
       LEFT JOIN sports_teams ht ON g.home_team_id = ht.id
       LEFT JOIN sports_teams at ON g.away_team_id = at.id
       LEFT JOIN sports_leagues l ON g.league_id = l.id
       WHERE g.id = $1 AND g.source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listGames(filters: {
    leagueId?: string; teamId?: string; status?: string;
    from?: Date; to?: Date; limit?: number; offset?: number;
  }): Promise<GameWithTeams[]> {
    const conditions: string[] = ['g.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.leagueId) {
      conditions.push(`g.league_id = $${paramIndex}`);
      values.push(filters.leagueId);
      paramIndex++;
    }
    if (filters.teamId) {
      conditions.push(`(g.home_team_id = $${paramIndex} OR g.away_team_id = $${paramIndex})`);
      values.push(filters.teamId);
      paramIndex++;
    }
    if (filters.status) {
      conditions.push(`g.status = $${paramIndex}`);
      values.push(filters.status);
      paramIndex++;
    }
    if (filters.from) {
      conditions.push(`g.scheduled_at >= $${paramIndex}`);
      values.push(filters.from);
      paramIndex++;
    }
    if (filters.to) {
      conditions.push(`g.scheduled_at <= $${paramIndex}`);
      values.push(filters.to);
      paramIndex++;
    }

    let sql = `
      SELECT g.*,
        ht.name as home_team_name, ht.abbreviation as home_team_abbreviation, ht.logo_url as home_team_logo_url,
        at.name as away_team_name, at.abbreviation as away_team_abbreviation, at.logo_url as away_team_logo_url,
        l.name as league_name
      FROM sports_games g
      LEFT JOIN sports_teams ht ON g.home_team_id = ht.id
      LEFT JOIN sports_teams at ON g.away_team_id = at.id
      LEFT JOIN sports_leagues l ON g.league_id = l.id
      WHERE ${conditions.join(' AND ')}
      ORDER BY g.scheduled_at ASC
    `;
    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<GameWithTeams>(sql, values);
    return result.rows;
  }

  async getLiveGames(): Promise<GameWithTeams[]> {
    const result = await this.query<GameWithTeams>(
      `SELECT g.*,
        ht.name as home_team_name, ht.abbreviation as home_team_abbreviation, ht.logo_url as home_team_logo_url,
        at.name as away_team_name, at.abbreviation as away_team_abbreviation, at.logo_url as away_team_logo_url,
        l.name as league_name
       FROM sports_games g
       LEFT JOIN sports_teams ht ON g.home_team_id = ht.id
       LEFT JOIN sports_teams at ON g.away_team_id = at.id
       LEFT JOIN sports_leagues l ON g.league_id = l.id
       WHERE g.source_account_id = $1 AND g.status = 'in_progress'
       ORDER BY g.scheduled_at ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async getTodayGames(): Promise<GameWithTeams[]> {
    const result = await this.query<GameWithTeams>(
      `SELECT g.*,
        ht.name as home_team_name, ht.abbreviation as home_team_abbreviation, ht.logo_url as home_team_logo_url,
        at.name as away_team_name, at.abbreviation as away_team_abbreviation, at.logo_url as away_team_logo_url,
        l.name as league_name
       FROM sports_games g
       LEFT JOIN sports_teams ht ON g.home_team_id = ht.id
       LEFT JOIN sports_teams at ON g.away_team_id = at.id
       LEFT JOIN sports_leagues l ON g.league_id = l.id
       WHERE g.source_account_id = $1
         AND g.scheduled_at >= CURRENT_DATE
         AND g.scheduled_at < CURRENT_DATE + INTERVAL '1 day'
       ORDER BY g.scheduled_at ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async getUpcomingGamesForUser(userId: string, days = 7): Promise<GameWithTeams[]> {
    const result = await this.query<GameWithTeams>(
      `SELECT g.*,
        ht.name as home_team_name, ht.abbreviation as home_team_abbreviation, ht.logo_url as home_team_logo_url,
        at.name as away_team_name, at.abbreviation as away_team_abbreviation, at.logo_url as away_team_logo_url,
        l.name as league_name
       FROM sports_games g
       LEFT JOIN sports_teams ht ON g.home_team_id = ht.id
       LEFT JOIN sports_teams at ON g.away_team_id = at.id
       LEFT JOIN sports_leagues l ON g.league_id = l.id
       WHERE g.source_account_id = $1
         AND g.scheduled_at >= NOW()
         AND g.scheduled_at <= NOW() + INTERVAL '${days} days'
         AND (
           g.home_team_id IN (SELECT favorite_id FROM sports_favorites WHERE user_id = $2 AND favorite_type = 'team' AND source_account_id = $1)
           OR g.away_team_id IN (SELECT favorite_id FROM sports_favorites WHERE user_id = $2 AND favorite_type = 'team' AND source_account_id = $1)
         )
       ORDER BY g.scheduled_at ASC`,
      [this.sourceAccountId, userId]
    );
    return result.rows;
  }

  async getScores(filters: { leagueId?: string; date?: Date }): Promise<GameWithTeams[]> {
    const conditions: string[] = ['g.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.leagueId) {
      conditions.push(`g.league_id = $${paramIndex}`);
      values.push(filters.leagueId);
      paramIndex++;
    }

    if (filters.date) {
      conditions.push(`g.scheduled_at >= $${paramIndex} AND g.scheduled_at < $${paramIndex} + INTERVAL '1 day'`);
      values.push(filters.date);
      paramIndex++;
    } else {
      conditions.push(`g.scheduled_at >= CURRENT_DATE AND g.scheduled_at < CURRENT_DATE + INTERVAL '1 day'`);
    }

    const result = await this.query<GameWithTeams>(
      `SELECT g.*,
        ht.name as home_team_name, ht.abbreviation as home_team_abbreviation, ht.logo_url as home_team_logo_url,
        at.name as away_team_name, at.abbreviation as away_team_abbreviation, at.logo_url as away_team_logo_url,
        l.name as league_name
       FROM sports_games g
       LEFT JOIN sports_teams ht ON g.home_team_id = ht.id
       LEFT JOIN sports_teams at ON g.away_team_id = at.id
       LEFT JOIN sports_leagues l ON g.league_id = l.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY g.scheduled_at ASC`,
      values
    );
    return result.rows;
  }

  // =========================================================================
  // Standing Operations
  // =========================================================================

  async upsertStanding(standing: Omit<StandingRecord, 'id' | 'synced_at'>): Promise<StandingRecord> {
    const result = await this.query<StandingRecord>(
      `INSERT INTO sports_standings (
        source_account_id, league_id, team_id, season_year, season_type,
        conference, division, wins, losses, ties, overtime_losses,
        win_percentage, games_back, streak, last_10, rank_conference,
        rank_division, rank_overall, points_for, points_against, metadata, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, NOW())
      ON CONFLICT (source_account_id, league_id, team_id, season_year, season_type) DO UPDATE SET
        conference = EXCLUDED.conference,
        division = EXCLUDED.division,
        wins = EXCLUDED.wins,
        losses = EXCLUDED.losses,
        ties = EXCLUDED.ties,
        overtime_losses = EXCLUDED.overtime_losses,
        win_percentage = EXCLUDED.win_percentage,
        games_back = EXCLUDED.games_back,
        streak = EXCLUDED.streak,
        last_10 = EXCLUDED.last_10,
        rank_conference = EXCLUDED.rank_conference,
        rank_division = EXCLUDED.rank_division,
        rank_overall = EXCLUDED.rank_overall,
        points_for = EXCLUDED.points_for,
        points_against = EXCLUDED.points_against,
        metadata = EXCLUDED.metadata,
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, standing.league_id, standing.team_id,
        standing.season_year, standing.season_type, standing.conference,
        standing.division, standing.wins, standing.losses, standing.ties,
        standing.overtime_losses, standing.win_percentage, standing.games_back,
        standing.streak, standing.last_10, standing.rank_conference,
        standing.rank_division, standing.rank_overall, standing.points_for,
        standing.points_against, JSON.stringify(standing.metadata),
      ]
    );

    return result.rows[0];
  }

  async listStandings(filters: {
    leagueId?: string; seasonYear?: number; seasonType?: string; conference?: string;
  }): Promise<StandingWithTeam[]> {
    const conditions: string[] = ['s.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.leagueId) {
      conditions.push(`s.league_id = $${paramIndex}`);
      values.push(filters.leagueId);
      paramIndex++;
    }
    if (filters.seasonYear) {
      conditions.push(`s.season_year = $${paramIndex}`);
      values.push(filters.seasonYear);
      paramIndex++;
    }
    if (filters.seasonType) {
      conditions.push(`s.season_type = $${paramIndex}`);
      values.push(filters.seasonType);
      paramIndex++;
    }
    if (filters.conference) {
      conditions.push(`s.conference = $${paramIndex}`);
      values.push(filters.conference);
      paramIndex++;
    }

    const result = await this.query<StandingWithTeam>(
      `SELECT s.*, t.name as team_name, t.abbreviation as team_abbreviation, t.logo_url as team_logo_url
       FROM sports_standings s
       JOIN sports_teams t ON s.team_id = t.id
       WHERE ${conditions.join(' AND ')}
       ORDER BY s.rank_overall ASC NULLS LAST, s.win_percentage DESC`,
      values
    );
    return result.rows;
  }

  // =========================================================================
  // Player Operations
  // =========================================================================

  async upsertPlayer(player: Omit<PlayerRecord, 'id' | 'synced_at'>): Promise<PlayerRecord> {
    const result = await this.query<PlayerRecord>(
      `INSERT INTO sports_players (
        source_account_id, team_id, external_id, provider, name, position,
        jersey_number, height, weight, birth_date, photo_url, status, metadata, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (source_account_id, provider, external_id) DO UPDATE SET
        team_id = EXCLUDED.team_id,
        name = EXCLUDED.name,
        position = EXCLUDED.position,
        jersey_number = EXCLUDED.jersey_number,
        height = EXCLUDED.height,
        weight = EXCLUDED.weight,
        photo_url = EXCLUDED.photo_url,
        status = EXCLUDED.status,
        metadata = EXCLUDED.metadata,
        synced_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, player.team_id, player.external_id, player.provider,
        player.name, player.position, player.jersey_number, player.height,
        player.weight, player.birth_date, player.photo_url, player.status,
        JSON.stringify(player.metadata),
      ]
    );

    return result.rows[0];
  }

  async getPlayer(id: string): Promise<PlayerRecord | null> {
    const result = await this.query<PlayerRecord>(
      `SELECT * FROM sports_players WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listPlayers(filters: {
    teamId?: string; position?: string; limit?: number; offset?: number;
  }): Promise<PlayerRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.teamId) {
      conditions.push(`team_id = $${paramIndex}`);
      values.push(filters.teamId);
      paramIndex++;
    }
    if (filters.position) {
      conditions.push(`position = $${paramIndex}`);
      values.push(filters.position);
      paramIndex++;
    }

    let sql = `SELECT * FROM sports_players WHERE ${conditions.join(' AND ')} ORDER BY name ASC`;
    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<PlayerRecord>(sql, values);
    return result.rows;
  }

  async searchPlayers(searchQuery: string, limit = 20): Promise<PlayerRecord[]> {
    const result = await this.query<PlayerRecord>(
      `SELECT * FROM sports_players
       WHERE source_account_id = $1
         AND name ILIKE $2
       ORDER BY name ASC
       LIMIT $3`,
      [this.sourceAccountId, `%${searchQuery}%`, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // Favorites Operations
  // =========================================================================

  async addFavorite(favorite: Omit<FavoriteRecord, 'id' | 'created_at'>): Promise<FavoriteRecord> {
    const result = await this.query<FavoriteRecord>(
      `INSERT INTO sports_favorites (
        source_account_id, user_id, favorite_type, favorite_id
      ) VALUES ($1, $2, $3, $4)
      ON CONFLICT (source_account_id, user_id, favorite_type, favorite_id) DO NOTHING
      RETURNING *`,
      [this.sourceAccountId, favorite.user_id, favorite.favorite_type, favorite.favorite_id]
    );

    return result.rows[0];
  }

  async listFavorites(userId: string): Promise<FavoriteRecord[]> {
    const result = await this.query<FavoriteRecord>(
      `SELECT * FROM sports_favorites
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY created_at DESC`,
      [this.sourceAccountId, userId]
    );
    return result.rows;
  }

  async deleteFavorite(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM sports_favorites WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  // =========================================================================
  // Sync State Operations
  // =========================================================================

  async getSyncStatus(): Promise<SyncStateRecord[]> {
    const result = await this.query<SyncStateRecord>(
      `SELECT * FROM sports_sync_state WHERE source_account_id = $1 ORDER BY resource_type ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async updateSyncState(provider: string, resourceType: string, status: string, error?: string): Promise<void> {
    await this.execute(
      `INSERT INTO sports_sync_state (source_account_id, provider, resource_type, status, last_sync_at, error)
       VALUES ($1, $2, $3, $4, NOW(), $5)
       ON CONFLICT (source_account_id, provider, resource_type) DO UPDATE SET
         status = EXCLUDED.status,
         last_sync_at = EXCLUDED.last_sync_at,
         error = EXCLUDED.error`,
      [this.sourceAccountId, provider, resourceType, status, error ?? null]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<SportsDataStats> {
    const result = await this.query<{
      total_leagues: string;
      total_teams: string;
      total_games: string;
      live_games: string;
      upcoming_games: string;
      completed_games: string;
      total_players: string;
      total_favorites: string;
      last_sync_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM sports_leagues WHERE source_account_id = $1) as total_leagues,
        (SELECT COUNT(*) FROM sports_teams WHERE source_account_id = $1) as total_teams,
        (SELECT COUNT(*) FROM sports_games WHERE source_account_id = $1) as total_games,
        (SELECT COUNT(*) FROM sports_games WHERE source_account_id = $1 AND status = 'in_progress') as live_games,
        (SELECT COUNT(*) FROM sports_games WHERE source_account_id = $1 AND status IN ('scheduled', 'queued') AND scheduled_at >= NOW()) as upcoming_games,
        (SELECT COUNT(*) FROM sports_games WHERE source_account_id = $1 AND status = 'completed') as completed_games,
        (SELECT COUNT(*) FROM sports_players WHERE source_account_id = $1) as total_players,
        (SELECT COUNT(*) FROM sports_favorites WHERE source_account_id = $1) as total_favorites,
        (SELECT MAX(last_sync_at) FROM sports_sync_state WHERE source_account_id = $1) as last_sync_at`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_leagues: parseInt(row.total_leagues, 10),
      total_teams: parseInt(row.total_teams, 10),
      total_games: parseInt(row.total_games, 10),
      live_games: parseInt(row.live_games, 10),
      upcoming_games: parseInt(row.upcoming_games, 10),
      completed_games: parseInt(row.completed_games, 10),
      total_players: parseInt(row.total_players, 10),
      total_favorites: parseInt(row.total_favorites, 10),
      last_sync_at: row.last_sync_at,
    };
  }
}
