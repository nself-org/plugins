/**
 * TMDB Plugin Database
 * Schema initialization and CRUD operations for TMDB metadata
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  TmdbMovieRecord,
  TmdbTvShowRecord,
  TmdbTvSeasonRecord,
  TmdbTvEpisodeRecord,
  TmdbGenreRecord,
  TmdbMatchQueueRecord,
  TmdbStats,
  StatusResponse,
} from './types.js';

const logger = createLogger('tmdb:database');

export class TmdbDatabase {
  private db: Database;
  private sourceAccountId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  forSourceAccount(sourceAccountId: string): TmdbDatabase {
    const scoped = new TmdbDatabase(this.db);
    scoped.sourceAccountId = sourceAccountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  // ============================================================================
  // Schema Initialization
  // ============================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing TMDB database schema...');

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_movies (
        id INTEGER PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        imdb_id VARCHAR(20),
        title VARCHAR(500) NOT NULL,
        original_title VARCHAR(500),
        overview TEXT,
        tagline TEXT,
        release_date DATE,
        runtime INTEGER,
        status VARCHAR(50),
        poster_path VARCHAR(255),
        backdrop_path VARCHAR(255),
        budget BIGINT,
        revenue BIGINT,
        vote_average DOUBLE PRECISION,
        vote_count INTEGER,
        popularity DOUBLE PRECISION,
        original_language VARCHAR(10),
        genres JSONB DEFAULT '[]',
        production_companies JSONB DEFAULT '[]',
        production_countries JSONB DEFAULT '[]',
        spoken_languages JSONB DEFAULT '[]',
        credits JSONB DEFAULT '{}',
        keywords JSONB DEFAULT '[]',
        content_rating VARCHAR(20),
        synced_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_movies_source_app ON tmdb_movies(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_movies_imdb ON tmdb_movies(imdb_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_movies_title ON tmdb_movies(title)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_tv_shows (
        id INTEGER PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        imdb_id VARCHAR(20),
        name VARCHAR(500) NOT NULL,
        original_name VARCHAR(500),
        overview TEXT,
        first_air_date DATE,
        last_air_date DATE,
        status VARCHAR(50),
        type VARCHAR(50),
        number_of_seasons INTEGER,
        number_of_episodes INTEGER,
        episode_run_time INTEGER[],
        poster_path VARCHAR(255),
        backdrop_path VARCHAR(255),
        vote_average DOUBLE PRECISION,
        vote_count INTEGER,
        popularity DOUBLE PRECISION,
        original_language VARCHAR(10),
        genres JSONB DEFAULT '[]',
        networks JSONB DEFAULT '[]',
        created_by JSONB DEFAULT '[]',
        credits JSONB DEFAULT '{}',
        content_rating VARCHAR(20),
        synced_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_tv_source_app ON tmdb_tv_shows(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_tv_imdb ON tmdb_tv_shows(imdb_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_tv_seasons (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        show_id INTEGER NOT NULL REFERENCES tmdb_tv_shows(id),
        season_number INTEGER NOT NULL,
        name VARCHAR(500),
        overview TEXT,
        poster_path VARCHAR(255),
        air_date DATE,
        episode_count INTEGER,
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(show_id, season_number)
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_seasons_source_app ON tmdb_tv_seasons(source_account_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_tv_episodes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        show_id INTEGER NOT NULL REFERENCES tmdb_tv_shows(id),
        season_number INTEGER NOT NULL,
        episode_number INTEGER NOT NULL,
        name VARCHAR(500),
        overview TEXT,
        still_path VARCHAR(255),
        air_date DATE,
        runtime INTEGER,
        vote_average DOUBLE PRECISION,
        crew JSONB DEFAULT '[]',
        guest_stars JSONB DEFAULT '[]',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(show_id, season_number, episode_number)
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_episodes_source_app ON tmdb_tv_episodes(source_account_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_genres (
        id INTEGER PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(100) NOT NULL,
        media_type VARCHAR(10) NOT NULL
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_genres_source_app ON tmdb_genres(source_account_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_match_queue (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        media_id VARCHAR(255) NOT NULL,
        filename VARCHAR(500),
        parsed_title VARCHAR(500),
        parsed_year INTEGER,
        parsed_type VARCHAR(20),
        match_results JSONB DEFAULT '[]',
        best_match_id INTEGER,
        best_match_type VARCHAR(20),
        confidence DOUBLE PRECISION,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        reviewed_by VARCHAR(255),
        reviewed_at TIMESTAMPTZ,
        auto_accepted BOOLEAN DEFAULT false,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_match_source_app ON tmdb_match_queue(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_match_status ON tmdb_match_queue(source_account_id, status)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_match_media ON tmdb_match_queue(source_account_id, media_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS tmdb_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);

    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tmdb_webhook_events_source_app ON tmdb_webhook_events(source_account_id)`);

    logger.success('TMDB database schema initialized');
  }

  // ============================================================================
  // Movies CRUD
  // ============================================================================

  async upsertMovie(movie: Omit<TmdbMovieRecord, 'synced_at'>): Promise<void> {
    await this.db.execute(`
      INSERT INTO tmdb_movies (
        id, source_account_id, imdb_id, title, original_title, overview, tagline,
        release_date, runtime, status, poster_path, backdrop_path, budget, revenue,
        vote_average, vote_count, popularity, original_language, genres,
        production_companies, production_countries, spoken_languages, credits,
        keywords, content_rating, synced_at
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,NOW())
      ON CONFLICT (id) DO UPDATE SET
        source_account_id = EXCLUDED.source_account_id,
        imdb_id = EXCLUDED.imdb_id,
        title = EXCLUDED.title,
        original_title = EXCLUDED.original_title,
        overview = EXCLUDED.overview,
        tagline = EXCLUDED.tagline,
        release_date = EXCLUDED.release_date,
        runtime = EXCLUDED.runtime,
        status = EXCLUDED.status,
        poster_path = EXCLUDED.poster_path,
        backdrop_path = EXCLUDED.backdrop_path,
        budget = EXCLUDED.budget,
        revenue = EXCLUDED.revenue,
        vote_average = EXCLUDED.vote_average,
        vote_count = EXCLUDED.vote_count,
        popularity = EXCLUDED.popularity,
        original_language = EXCLUDED.original_language,
        genres = EXCLUDED.genres,
        production_companies = EXCLUDED.production_companies,
        production_countries = EXCLUDED.production_countries,
        spoken_languages = EXCLUDED.spoken_languages,
        credits = EXCLUDED.credits,
        keywords = EXCLUDED.keywords,
        content_rating = EXCLUDED.content_rating,
        synced_at = NOW()
    `, [
      movie.id, this.sourceAccountId, movie.imdb_id, movie.title, movie.original_title,
      movie.overview, movie.tagline, movie.release_date, movie.runtime, movie.status,
      movie.poster_path, movie.backdrop_path, movie.budget, movie.revenue,
      movie.vote_average, movie.vote_count, movie.popularity, movie.original_language,
      JSON.stringify(movie.genres), JSON.stringify(movie.production_companies),
      JSON.stringify(movie.production_countries), JSON.stringify(movie.spoken_languages),
      JSON.stringify(movie.credits), JSON.stringify(movie.keywords), movie.content_rating,
    ]);
  }

  async getMovie(id: number): Promise<TmdbMovieRecord | null> {
    return this.db.queryOne<TmdbMovieRecord>(
      `SELECT * FROM tmdb_movies WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async listMovies(limit: number = 50, offset: number = 0): Promise<{ movies: TmdbMovieRecord[]; total: number }> {
    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_movies WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<TmdbMovieRecord>(
      `SELECT * FROM tmdb_movies WHERE source_account_id = $1 ORDER BY popularity DESC NULLS LAST LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return { movies: result.rows, total };
  }

  // ============================================================================
  // TV Shows CRUD
  // ============================================================================

  async upsertTvShow(show: Omit<TmdbTvShowRecord, 'synced_at'>): Promise<void> {
    await this.db.execute(`
      INSERT INTO tmdb_tv_shows (
        id, source_account_id, imdb_id, name, original_name, overview,
        first_air_date, last_air_date, status, type, number_of_seasons,
        number_of_episodes, episode_run_time, poster_path, backdrop_path,
        vote_average, vote_count, popularity, original_language, genres,
        networks, created_by, credits, content_rating, synced_at
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,NOW())
      ON CONFLICT (id) DO UPDATE SET
        source_account_id = EXCLUDED.source_account_id,
        imdb_id = EXCLUDED.imdb_id,
        name = EXCLUDED.name,
        original_name = EXCLUDED.original_name,
        overview = EXCLUDED.overview,
        first_air_date = EXCLUDED.first_air_date,
        last_air_date = EXCLUDED.last_air_date,
        status = EXCLUDED.status,
        type = EXCLUDED.type,
        number_of_seasons = EXCLUDED.number_of_seasons,
        number_of_episodes = EXCLUDED.number_of_episodes,
        episode_run_time = EXCLUDED.episode_run_time,
        poster_path = EXCLUDED.poster_path,
        backdrop_path = EXCLUDED.backdrop_path,
        vote_average = EXCLUDED.vote_average,
        vote_count = EXCLUDED.vote_count,
        popularity = EXCLUDED.popularity,
        original_language = EXCLUDED.original_language,
        genres = EXCLUDED.genres,
        networks = EXCLUDED.networks,
        created_by = EXCLUDED.created_by,
        credits = EXCLUDED.credits,
        content_rating = EXCLUDED.content_rating,
        synced_at = NOW()
    `, [
      show.id, this.sourceAccountId, show.imdb_id, show.name, show.original_name,
      show.overview, show.first_air_date, show.last_air_date, show.status, show.type,
      show.number_of_seasons, show.number_of_episodes, show.episode_run_time,
      show.poster_path, show.backdrop_path, show.vote_average, show.vote_count,
      show.popularity, show.original_language, JSON.stringify(show.genres),
      JSON.stringify(show.networks), JSON.stringify(show.created_by),
      JSON.stringify(show.credits), show.content_rating,
    ]);
  }

  async getTvShow(id: number): Promise<TmdbTvShowRecord | null> {
    return this.db.queryOne<TmdbTvShowRecord>(
      `SELECT * FROM tmdb_tv_shows WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async listTvShows(limit: number = 50, offset: number = 0): Promise<{ shows: TmdbTvShowRecord[]; total: number }> {
    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_tv_shows WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<TmdbTvShowRecord>(
      `SELECT * FROM tmdb_tv_shows WHERE source_account_id = $1 ORDER BY popularity DESC NULLS LAST LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return { shows: result.rows, total };
  }

  // ============================================================================
  // Seasons & Episodes CRUD
  // ============================================================================

  async upsertSeason(season: Omit<TmdbTvSeasonRecord, 'id' | 'synced_at'>): Promise<void> {
    await this.db.execute(`
      INSERT INTO tmdb_tv_seasons (
        source_account_id, show_id, season_number, name, overview,
        poster_path, air_date, episode_count, synced_at
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
      ON CONFLICT (show_id, season_number) DO UPDATE SET
        source_account_id = EXCLUDED.source_account_id,
        name = EXCLUDED.name,
        overview = EXCLUDED.overview,
        poster_path = EXCLUDED.poster_path,
        air_date = EXCLUDED.air_date,
        episode_count = EXCLUDED.episode_count,
        synced_at = NOW()
    `, [
      this.sourceAccountId, season.show_id, season.season_number,
      season.name, season.overview, season.poster_path, season.air_date, season.episode_count,
    ]);
  }

  async getSeasons(showId: number): Promise<TmdbTvSeasonRecord[]> {
    const result = await this.db.query<TmdbTvSeasonRecord>(
      `SELECT * FROM tmdb_tv_seasons WHERE show_id = $1 AND source_account_id = $2 ORDER BY season_number`,
      [showId, this.sourceAccountId]
    );
    return result.rows;
  }

  async upsertEpisode(episode: Omit<TmdbTvEpisodeRecord, 'id' | 'synced_at'>): Promise<void> {
    await this.db.execute(`
      INSERT INTO tmdb_tv_episodes (
        source_account_id, show_id, season_number, episode_number, name,
        overview, still_path, air_date, runtime, vote_average, crew, guest_stars, synced_at
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW())
      ON CONFLICT (show_id, season_number, episode_number) DO UPDATE SET
        source_account_id = EXCLUDED.source_account_id,
        name = EXCLUDED.name,
        overview = EXCLUDED.overview,
        still_path = EXCLUDED.still_path,
        air_date = EXCLUDED.air_date,
        runtime = EXCLUDED.runtime,
        vote_average = EXCLUDED.vote_average,
        crew = EXCLUDED.crew,
        guest_stars = EXCLUDED.guest_stars,
        synced_at = NOW()
    `, [
      this.sourceAccountId, episode.show_id, episode.season_number, episode.episode_number,
      episode.name, episode.overview, episode.still_path, episode.air_date,
      episode.runtime, episode.vote_average, JSON.stringify(episode.crew),
      JSON.stringify(episode.guest_stars),
    ]);
  }

  async getEpisodes(showId: number, seasonNumber: number): Promise<TmdbTvEpisodeRecord[]> {
    const result = await this.db.query<TmdbTvEpisodeRecord>(
      `SELECT * FROM tmdb_tv_episodes WHERE show_id = $1 AND season_number = $2 AND source_account_id = $3 ORDER BY episode_number`,
      [showId, seasonNumber, this.sourceAccountId]
    );
    return result.rows;
  }

  async getEpisode(showId: number, seasonNumber: number, episodeNumber: number): Promise<TmdbTvEpisodeRecord | null> {
    return this.db.queryOne<TmdbTvEpisodeRecord>(
      `SELECT * FROM tmdb_tv_episodes WHERE show_id = $1 AND season_number = $2 AND episode_number = $3 AND source_account_id = $4`,
      [showId, seasonNumber, episodeNumber, this.sourceAccountId]
    );
  }

  // ============================================================================
  // Genres CRUD
  // ============================================================================

  async upsertGenre(genre: TmdbGenreRecord): Promise<void> {
    await this.db.execute(`
      INSERT INTO tmdb_genres (id, source_account_id, name, media_type)
      VALUES ($1, $2, $3, $4)
      ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        media_type = EXCLUDED.media_type
    `, [genre.id, this.sourceAccountId, genre.name, genre.media_type]);
  }

  async listGenres(mediaType?: string): Promise<TmdbGenreRecord[]> {
    if (mediaType) {
      const result = await this.db.query<TmdbGenreRecord>(
        `SELECT * FROM tmdb_genres WHERE source_account_id = $1 AND media_type = $2 ORDER BY name`,
        [this.sourceAccountId, mediaType]
      );
      return result.rows;
    }
    const result = await this.db.query<TmdbGenreRecord>(
      `SELECT * FROM tmdb_genres WHERE source_account_id = $1 ORDER BY media_type, name`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  // ============================================================================
  // Match Queue CRUD
  // ============================================================================

  async createMatchEntry(entry: {
    media_id: string;
    filename?: string;
    parsed_title?: string;
    parsed_year?: number;
    parsed_type?: string;
    match_results: unknown[];
    best_match_id?: number;
    best_match_type?: string;
    confidence?: number;
    status: string;
    auto_accepted: boolean;
  }): Promise<TmdbMatchQueueRecord> {
    const result = await this.db.query<TmdbMatchQueueRecord>(`
      INSERT INTO tmdb_match_queue (
        source_account_id, media_id, filename, parsed_title, parsed_year, parsed_type,
        match_results, best_match_id, best_match_type, confidence, status, auto_accepted
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
      RETURNING *
    `, [
      this.sourceAccountId, entry.media_id, entry.filename || null,
      entry.parsed_title || null, entry.parsed_year || null, entry.parsed_type || null,
      JSON.stringify(entry.match_results), entry.best_match_id || null,
      entry.best_match_type || null, entry.confidence || null, entry.status, entry.auto_accepted,
    ]);
    return result.rows[0];
  }

  async getMatchEntry(id: string): Promise<TmdbMatchQueueRecord | null> {
    return this.db.queryOne<TmdbMatchQueueRecord>(
      `SELECT * FROM tmdb_match_queue WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async listMatchQueue(status?: string, limit: number = 50, offset: number = 0): Promise<{ items: TmdbMatchQueueRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(status);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_match_queue WHERE ${whereClause}`,
      params
    );
    const total = parseInt(countResult?.count ?? '0', 10);

    const result = await this.db.query<TmdbMatchQueueRecord>(
      `SELECT * FROM tmdb_match_queue WHERE ${whereClause} ORDER BY created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      [...params, limit, offset]
    );

    return { items: result.rows, total };
  }

  async updateMatchStatus(id: string, status: string, reviewedBy?: string, tmdbId?: number, tmdbType?: string): Promise<TmdbMatchQueueRecord | null> {
    const result = await this.db.query<TmdbMatchQueueRecord>(`
      UPDATE tmdb_match_queue SET
        status = $3,
        reviewed_by = $4,
        reviewed_at = NOW(),
        best_match_id = COALESCE($5, best_match_id),
        best_match_type = COALESCE($6, best_match_type),
        updated_at = NOW()
      WHERE id = $1 AND source_account_id = $2
      RETURNING *
    `, [id, this.sourceAccountId, status, reviewedBy || null, tmdbId || null, tmdbType || null]);

    return result.rows[0] || null;
  }

  // ============================================================================
  // Webhook Events
  // ============================================================================

  async insertWebhookEvent(eventId: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.db.execute(`
      INSERT INTO tmdb_webhook_events (id, source_account_id, event_type, payload)
      VALUES ($1, $2, $3, $4)
      ON CONFLICT (id) DO NOTHING
    `, [eventId, this.sourceAccountId, eventType, JSON.stringify(payload)]);
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(): Promise<TmdbStats> {
    const movies = await this.db.countScoped('tmdb_movies', this.sourceAccountId);
    const tvShows = await this.db.countScoped('tmdb_tv_shows', this.sourceAccountId);
    const seasons = await this.db.countScoped('tmdb_tv_seasons', this.sourceAccountId);
    const episodes = await this.db.countScoped('tmdb_tv_episodes', this.sourceAccountId);
    const genres = await this.db.countScoped('tmdb_genres', this.sourceAccountId);

    const pendingResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_match_queue WHERE source_account_id = $1 AND status = 'pending'`,
      [this.sourceAccountId]
    );
    const acceptedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_match_queue WHERE source_account_id = $1 AND status = 'accepted'`,
      [this.sourceAccountId]
    );
    const rejectedResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_match_queue WHERE source_account_id = $1 AND status = 'rejected'`,
      [this.sourceAccountId]
    );

    return {
      totalMovies: movies,
      totalTvShows: tvShows,
      totalSeasons: seasons,
      totalEpisodes: episodes,
      totalGenres: genres,
      matchQueuePending: parseInt(pendingResult?.count ?? '0', 10),
      matchQueueAccepted: parseInt(acceptedResult?.count ?? '0', 10),
      matchQueueRejected: parseInt(rejectedResult?.count ?? '0', 10),
    };
  }

  async getStatus(): Promise<StatusResponse> {
    const stats = await this.getStats();

    const manualResult = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM tmdb_match_queue WHERE source_account_id = $1 AND status = 'manual'`,
      [this.sourceAccountId]
    );

    const lastSync = await this.db.queryOne<{ synced_at: Date }>(
      `SELECT synced_at FROM tmdb_movies WHERE source_account_id = $1 ORDER BY synced_at DESC LIMIT 1`,
      [this.sourceAccountId]
    );

    return {
      movies: stats.totalMovies,
      tvShows: stats.totalTvShows,
      seasons: stats.totalSeasons,
      episodes: stats.totalEpisodes,
      genres: stats.totalGenres,
      matchQueue: {
        pending: stats.matchQueuePending,
        accepted: stats.matchQueueAccepted,
        rejected: stats.matchQueueRejected,
        manual: parseInt(manualResult?.count ?? '0', 10),
      },
      lastSynced: lastSync?.synced_at?.toISOString() ?? null,
    };
  }

  async getMoviesOlderThan(days: number, limit: number = 100): Promise<TmdbMovieRecord[]> {
    const result = await this.db.query<TmdbMovieRecord>(
      `SELECT * FROM tmdb_movies WHERE source_account_id = $1 AND synced_at < NOW() - INTERVAL '${days} days' ORDER BY synced_at ASC LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async getTvShowsOlderThan(days: number, limit: number = 100): Promise<TmdbTvShowRecord[]> {
    const result = await this.db.query<TmdbTvShowRecord>(
      `SELECT * FROM tmdb_tv_shows WHERE source_account_id = $1 AND synced_at < NOW() - INTERVAL '${days} days' ORDER BY synced_at ASC LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }
}

export async function createTmdbDatabase(config: {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
}): Promise<TmdbDatabase> {
  const db = createDatabase(config);
  await db.connect();
  return new TmdbDatabase(db);
}
