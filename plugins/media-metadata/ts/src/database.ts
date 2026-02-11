/**
 * TMDB Database Operations
 * Complete CRUD operations for all TMDB metadata in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  TmdbMovieRecord,
  TmdbTvShowRecord,
  TmdbTvSeasonRecord,
  TmdbTvEpisodeRecord,
  TmdbGenreRecord,
  TmdbMatchQueueRecord,
  TmdbWebhookEventRecord,
  StatsResponse,
} from './types.js';

const logger = createLogger('tmdb:db');

export class TmdbDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): TmdbDatabase {
    return new TmdbDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing TMDB schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
      CREATE EXTENSION IF NOT EXISTS "pg_trgm";

      -- =====================================================================
      -- Movies
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_movies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        tmdb_id INTEGER NOT NULL,
        imdb_id VARCHAR(20),
        title VARCHAR(500) NOT NULL,
        original_title VARCHAR(500),
        overview TEXT,
        release_date DATE,
        runtime_minutes INTEGER,
        vote_average DOUBLE PRECISION DEFAULT 0,
        vote_count INTEGER DEFAULT 0,
        popularity DOUBLE PRECISION DEFAULT 0,
        status VARCHAR(32),
        tagline TEXT,
        budget BIGINT,
        revenue BIGINT,
        genres TEXT[] DEFAULT '{}',
        spoken_languages TEXT[] DEFAULT '{}',
        production_countries TEXT[] DEFAULT '{}',
        poster_path TEXT,
        backdrop_path TEXT,
        cast JSONB DEFAULT '[]',
        crew JSONB DEFAULT '[]',
        content_rating VARCHAR(16),
        keywords TEXT[] DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, tmdb_id)
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_movies_account ON tmdb_movies(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_movies_tmdb_id ON tmdb_movies(tmdb_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_movies_imdb_id ON tmdb_movies(imdb_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_movies_title ON tmdb_movies USING gin(title gin_trgm_ops);
      CREATE INDEX IF NOT EXISTS idx_tmdb_movies_release_date ON tmdb_movies(release_date);
      CREATE INDEX IF NOT EXISTS idx_tmdb_movies_popularity ON tmdb_movies(popularity DESC);

      -- =====================================================================
      -- TV Shows
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_tv_shows (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        tmdb_id INTEGER NOT NULL,
        imdb_id VARCHAR(20),
        name VARCHAR(500) NOT NULL,
        original_name VARCHAR(500),
        overview TEXT,
        first_air_date DATE,
        last_air_date DATE,
        status VARCHAR(32),
        type VARCHAR(32),
        number_of_seasons INTEGER DEFAULT 0,
        number_of_episodes INTEGER DEFAULT 0,
        episode_run_time INTEGER[] DEFAULT '{}',
        vote_average DOUBLE PRECISION DEFAULT 0,
        vote_count INTEGER DEFAULT 0,
        popularity DOUBLE PRECISION DEFAULT 0,
        genres TEXT[] DEFAULT '{}',
        networks TEXT[] DEFAULT '{}',
        created_by TEXT[] DEFAULT '{}',
        poster_path TEXT,
        backdrop_path TEXT,
        content_rating VARCHAR(16),
        keywords TEXT[] DEFAULT '{}',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, tmdb_id)
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_shows_account ON tmdb_tv_shows(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_shows_tmdb_id ON tmdb_tv_shows(tmdb_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_shows_imdb_id ON tmdb_tv_shows(imdb_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_shows_name ON tmdb_tv_shows USING gin(name gin_trgm_ops);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_shows_first_air_date ON tmdb_tv_shows(first_air_date);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_shows_popularity ON tmdb_tv_shows(popularity DESC);

      -- =====================================================================
      -- TV Seasons
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_tv_seasons (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        show_tmdb_id INTEGER NOT NULL,
        season_number INTEGER NOT NULL,
        tmdb_id INTEGER,
        name VARCHAR(255),
        overview TEXT,
        air_date DATE,
        episode_count INTEGER DEFAULT 0,
        poster_path TEXT,
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, show_tmdb_id, season_number)
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_seasons_account ON tmdb_tv_seasons(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_seasons_show ON tmdb_tv_seasons(show_tmdb_id, season_number);

      -- =====================================================================
      -- TV Episodes
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_tv_episodes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        show_tmdb_id INTEGER NOT NULL,
        season_number INTEGER NOT NULL,
        episode_number INTEGER NOT NULL,
        tmdb_id INTEGER,
        name VARCHAR(500),
        overview TEXT,
        air_date DATE,
        runtime_minutes INTEGER,
        vote_average DOUBLE PRECISION DEFAULT 0,
        still_path TEXT,
        guest_stars JSONB DEFAULT '[]',
        crew JSONB DEFAULT '[]',
        synced_at TIMESTAMPTZ DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, show_tmdb_id, season_number, episode_number)
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_episodes_account ON tmdb_tv_episodes(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_tv_episodes_show ON tmdb_tv_episodes(show_tmdb_id, season_number, episode_number);

      -- =====================================================================
      -- Genres
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_genres (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        tmdb_id INTEGER NOT NULL,
        name VARCHAR(128) NOT NULL,
        media_type VARCHAR(8) NOT NULL DEFAULT 'movie',
        UNIQUE(source_account_id, tmdb_id, media_type)
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_genres_account ON tmdb_genres(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_genres_media_type ON tmdb_genres(media_type);

      -- =====================================================================
      -- Match Queue
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_match_queue (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        title VARCHAR(500) NOT NULL,
        year INTEGER,
        media_type VARCHAR(8) DEFAULT 'movie',
        source_id VARCHAR(255),
        source_plugin VARCHAR(64),
        candidates JSONB DEFAULT '[]',
        status VARCHAR(16) DEFAULT 'pending',
        matched_tmdb_id INTEGER,
        confidence DOUBLE PRECISION,
        reviewed_by VARCHAR(255),
        reviewed_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_match_queue_account ON tmdb_match_queue(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_match_queue_status ON tmdb_match_queue(status);
      CREATE INDEX IF NOT EXISTS idx_tmdb_match_queue_media_type ON tmdb_match_queue(media_type);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS tmdb_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_tmdb_webhook_events_account ON tmdb_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_tmdb_webhook_events_processed ON tmdb_webhook_events(processed);
    `;

    await this.db.execute(schema);
    logger.info('TMDB schema initialized successfully');
  }

  // =========================================================================
  // Movies
  // =========================================================================

  async upsertMovie(movie: Omit<TmdbMovieRecord, 'id' | 'created_at' | 'updated_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO tmdb_movies (
        source_account_id, tmdb_id, imdb_id, title, original_title, overview,
        release_date, runtime_minutes, vote_average, vote_count, popularity,
        status, tagline, budget, revenue, genres, spoken_languages,
        production_countries, poster_path, backdrop_path, cast, crew,
        content_rating, keywords, synced_at
      ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
        $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW()
      )
      ON CONFLICT (source_account_id, tmdb_id) DO UPDATE SET
        imdb_id = EXCLUDED.imdb_id,
        title = EXCLUDED.title,
        original_title = EXCLUDED.original_title,
        overview = EXCLUDED.overview,
        release_date = EXCLUDED.release_date,
        runtime_minutes = EXCLUDED.runtime_minutes,
        vote_average = EXCLUDED.vote_average,
        vote_count = EXCLUDED.vote_count,
        popularity = EXCLUDED.popularity,
        status = EXCLUDED.status,
        tagline = EXCLUDED.tagline,
        budget = EXCLUDED.budget,
        revenue = EXCLUDED.revenue,
        genres = EXCLUDED.genres,
        spoken_languages = EXCLUDED.spoken_languages,
        production_countries = EXCLUDED.production_countries,
        poster_path = EXCLUDED.poster_path,
        backdrop_path = EXCLUDED.backdrop_path,
        cast = EXCLUDED.cast,
        crew = EXCLUDED.crew,
        content_rating = EXCLUDED.content_rating,
        keywords = EXCLUDED.keywords,
        synced_at = NOW(),
        updated_at = NOW()`,
      [
        this.sourceAccountId,
        movie.tmdb_id,
        movie.imdb_id,
        movie.title,
        movie.original_title,
        movie.overview,
        movie.release_date,
        movie.runtime_minutes,
        movie.vote_average,
        movie.vote_count,
        movie.popularity,
        movie.status,
        movie.tagline,
        movie.budget,
        movie.revenue,
        movie.genres,
        movie.spoken_languages,
        movie.production_countries,
        movie.poster_path,
        movie.backdrop_path,
        JSON.stringify(movie.cast),
        JSON.stringify(movie.crew),
        movie.content_rating,
        movie.keywords,
      ]
    );
  }

  async getMovie(tmdbId: number): Promise<TmdbMovieRecord | null> {
    const result = await this.query<TmdbMovieRecord>(
      'SELECT * FROM tmdb_movies WHERE source_account_id = $1 AND tmdb_id = $2',
      [this.sourceAccountId, tmdbId]
    );
    return result.rows[0] ?? null;
  }

  async searchMoviesByTitle(title: string, limit = 10): Promise<TmdbMovieRecord[]> {
    const result = await this.query<TmdbMovieRecord>(
      `SELECT * FROM tmdb_movies
       WHERE source_account_id = $1 AND title ILIKE $2
       ORDER BY popularity DESC
       LIMIT $3`,
      [this.sourceAccountId, `%${title}%`, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // TV Shows
  // =========================================================================

  async upsertTvShow(show: Omit<TmdbTvShowRecord, 'id' | 'created_at' | 'updated_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO tmdb_tv_shows (
        source_account_id, tmdb_id, imdb_id, name, original_name, overview,
        first_air_date, last_air_date, status, type, number_of_seasons,
        number_of_episodes, episode_run_time, vote_average, vote_count,
        popularity, genres, networks, created_by, poster_path, backdrop_path,
        content_rating, keywords, synced_at
      ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
        $16, $17, $18, $19, $20, $21, $22, $23, NOW()
      )
      ON CONFLICT (source_account_id, tmdb_id) DO UPDATE SET
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
        vote_average = EXCLUDED.vote_average,
        vote_count = EXCLUDED.vote_count,
        popularity = EXCLUDED.popularity,
        genres = EXCLUDED.genres,
        networks = EXCLUDED.networks,
        created_by = EXCLUDED.created_by,
        poster_path = EXCLUDED.poster_path,
        backdrop_path = EXCLUDED.backdrop_path,
        content_rating = EXCLUDED.content_rating,
        keywords = EXCLUDED.keywords,
        synced_at = NOW(),
        updated_at = NOW()`,
      [
        this.sourceAccountId,
        show.tmdb_id,
        show.imdb_id,
        show.name,
        show.original_name,
        show.overview,
        show.first_air_date,
        show.last_air_date,
        show.status,
        show.type,
        show.number_of_seasons,
        show.number_of_episodes,
        show.episode_run_time,
        show.vote_average,
        show.vote_count,
        show.popularity,
        show.genres,
        show.networks,
        show.created_by,
        show.poster_path,
        show.backdrop_path,
        show.content_rating,
        show.keywords,
      ]
    );
  }

  async getTvShow(tmdbId: number): Promise<TmdbTvShowRecord | null> {
    const result = await this.query<TmdbTvShowRecord>(
      'SELECT * FROM tmdb_tv_shows WHERE source_account_id = $1 AND tmdb_id = $2',
      [this.sourceAccountId, tmdbId]
    );
    return result.rows[0] ?? null;
  }

  async searchTvShowsByName(name: string, limit = 10): Promise<TmdbTvShowRecord[]> {
    const result = await this.query<TmdbTvShowRecord>(
      `SELECT * FROM tmdb_tv_shows
       WHERE source_account_id = $1 AND name ILIKE $2
       ORDER BY popularity DESC
       LIMIT $3`,
      [this.sourceAccountId, `%${name}%`, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // TV Seasons
  // =========================================================================

  async upsertTvSeason(season: Omit<TmdbTvSeasonRecord, 'id' | 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO tmdb_tv_seasons (
        source_account_id, show_tmdb_id, season_number, tmdb_id, name,
        overview, air_date, episode_count, poster_path, synced_at
      ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW()
      )
      ON CONFLICT (source_account_id, show_tmdb_id, season_number) DO UPDATE SET
        tmdb_id = EXCLUDED.tmdb_id,
        name = EXCLUDED.name,
        overview = EXCLUDED.overview,
        air_date = EXCLUDED.air_date,
        episode_count = EXCLUDED.episode_count,
        poster_path = EXCLUDED.poster_path,
        synced_at = NOW()`,
      [
        this.sourceAccountId,
        season.show_tmdb_id,
        season.season_number,
        season.tmdb_id,
        season.name,
        season.overview,
        season.air_date,
        season.episode_count,
        season.poster_path,
      ]
    );
  }

  async getTvSeason(showTmdbId: number, seasonNumber: number): Promise<TmdbTvSeasonRecord | null> {
    const result = await this.query<TmdbTvSeasonRecord>(
      'SELECT * FROM tmdb_tv_seasons WHERE source_account_id = $1 AND show_tmdb_id = $2 AND season_number = $3',
      [this.sourceAccountId, showTmdbId, seasonNumber]
    );
    return result.rows[0] ?? null;
  }

  async getTvSeasonsByShow(showTmdbId: number): Promise<TmdbTvSeasonRecord[]> {
    const result = await this.query<TmdbTvSeasonRecord>(
      'SELECT * FROM tmdb_tv_seasons WHERE source_account_id = $1 AND show_tmdb_id = $2 ORDER BY season_number',
      [this.sourceAccountId, showTmdbId]
    );
    return result.rows;
  }

  // =========================================================================
  // TV Episodes
  // =========================================================================

  async upsertTvEpisode(episode: Omit<TmdbTvEpisodeRecord, 'id' | 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO tmdb_tv_episodes (
        source_account_id, show_tmdb_id, season_number, episode_number,
        tmdb_id, name, overview, air_date, runtime_minutes, vote_average,
        still_path, guest_stars, crew, synced_at
      ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW()
      )
      ON CONFLICT (source_account_id, show_tmdb_id, season_number, episode_number) DO UPDATE SET
        tmdb_id = EXCLUDED.tmdb_id,
        name = EXCLUDED.name,
        overview = EXCLUDED.overview,
        air_date = EXCLUDED.air_date,
        runtime_minutes = EXCLUDED.runtime_minutes,
        vote_average = EXCLUDED.vote_average,
        still_path = EXCLUDED.still_path,
        guest_stars = EXCLUDED.guest_stars,
        crew = EXCLUDED.crew,
        synced_at = NOW()`,
      [
        this.sourceAccountId,
        episode.show_tmdb_id,
        episode.season_number,
        episode.episode_number,
        episode.tmdb_id,
        episode.name,
        episode.overview,
        episode.air_date,
        episode.runtime_minutes,
        episode.vote_average,
        episode.still_path,
        JSON.stringify(episode.guest_stars),
        JSON.stringify(episode.crew),
      ]
    );
  }

  async getTvEpisode(
    showTmdbId: number,
    seasonNumber: number,
    episodeNumber: number
  ): Promise<TmdbTvEpisodeRecord | null> {
    const result = await this.query<TmdbTvEpisodeRecord>(
      `SELECT * FROM tmdb_tv_episodes
       WHERE source_account_id = $1 AND show_tmdb_id = $2
       AND season_number = $3 AND episode_number = $4`,
      [this.sourceAccountId, showTmdbId, seasonNumber, episodeNumber]
    );
    return result.rows[0] ?? null;
  }

  async getTvEpisodesBySeason(showTmdbId: number, seasonNumber: number): Promise<TmdbTvEpisodeRecord[]> {
    const result = await this.query<TmdbTvEpisodeRecord>(
      `SELECT * FROM tmdb_tv_episodes
       WHERE source_account_id = $1 AND show_tmdb_id = $2 AND season_number = $3
       ORDER BY episode_number`,
      [this.sourceAccountId, showTmdbId, seasonNumber]
    );
    return result.rows;
  }

  // =========================================================================
  // Genres
  // =========================================================================

  async upsertGenre(genre: Omit<TmdbGenreRecord, 'id'>): Promise<void> {
    await this.execute(
      `INSERT INTO tmdb_genres (source_account_id, tmdb_id, name, media_type)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (source_account_id, tmdb_id, media_type) DO UPDATE SET
         name = EXCLUDED.name`,
      [this.sourceAccountId, genre.tmdb_id, genre.name, genre.media_type]
    );
  }

  async getGenres(mediaType?: 'movie' | 'tv'): Promise<TmdbGenreRecord[]> {
    const sql = mediaType
      ? 'SELECT * FROM tmdb_genres WHERE source_account_id = $1 AND media_type = $2 ORDER BY name'
      : 'SELECT * FROM tmdb_genres WHERE source_account_id = $1 ORDER BY media_type, name';

    const params = mediaType ? [this.sourceAccountId, mediaType] : [this.sourceAccountId];
    const result = await this.query<TmdbGenreRecord>(sql, params);
    return result.rows;
  }

  // =========================================================================
  // Match Queue
  // =========================================================================

  async addToMatchQueue(
    item: Omit<TmdbMatchQueueRecord, 'id' | 'created_at' | 'reviewed_by' | 'reviewed_at'>
  ): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO tmdb_match_queue (
        source_account_id, title, year, media_type, source_id, source_plugin,
        candidates, status, matched_tmdb_id, confidence
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
       RETURNING id`,
      [
        this.sourceAccountId,
        item.title,
        item.year,
        item.media_type,
        item.source_id,
        item.source_plugin,
        JSON.stringify(item.candidates),
        item.status,
        item.matched_tmdb_id,
        item.confidence,
      ]
    );
    return result.rows[0].id;
  }

  async getMatchQueue(status?: string, limit = 100): Promise<TmdbMatchQueueRecord[]> {
    const sql = status
      ? `SELECT * FROM tmdb_match_queue
         WHERE source_account_id = $1 AND status = $2
         ORDER BY created_at DESC LIMIT $3`
      : `SELECT * FROM tmdb_match_queue
         WHERE source_account_id = $1
         ORDER BY created_at DESC LIMIT $2`;

    const params = status ? [this.sourceAccountId, status, limit] : [this.sourceAccountId, limit];
    const result = await this.query<TmdbMatchQueueRecord>(sql, params);
    return result.rows;
  }

  async updateMatchQueueItem(
    id: string,
    updates: Partial<Pick<TmdbMatchQueueRecord, 'status' | 'matched_tmdb_id' | 'reviewed_by'>>
  ): Promise<void> {
    const setClauses: string[] = [];
    const values: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (updates.status !== undefined) {
      setClauses.push(`status = $${paramIndex++}`);
      values.push(updates.status);
    }

    if (updates.matched_tmdb_id !== undefined) {
      setClauses.push(`matched_tmdb_id = $${paramIndex++}`);
      values.push(updates.matched_tmdb_id);
    }

    if (updates.reviewed_by !== undefined) {
      setClauses.push(`reviewed_by = $${paramIndex++}, reviewed_at = NOW()`);
      values.push(updates.reviewed_by);
    }

    if (setClauses.length === 0) return;

    await this.execute(
      `UPDATE tmdb_match_queue SET ${setClauses.join(', ')}
       WHERE source_account_id = $1 AND id = $2`,
      values
    );
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: Omit<TmdbWebhookEventRecord, 'created_at' | 'processed' | 'processed_at' | 'error'>): Promise<void> {
    await this.execute(
      `INSERT INTO tmdb_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (id) DO NOTHING`,
      [event.id, this.sourceAccountId, event.event_type, JSON.stringify(event.payload)]
    );
  }

  async markWebhookProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE tmdb_webhook_events
       SET processed = true, processed_at = NOW(), error = $3
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id, error ?? null]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<StatsResponse> {
    const [movies, tvShows, seasons, episodes, genres, matchQueue] = await Promise.all([
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM tmdb_movies WHERE source_account_id = $1',
        [this.sourceAccountId]
      ),
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM tmdb_tv_shows WHERE source_account_id = $1',
        [this.sourceAccountId]
      ),
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM tmdb_tv_seasons WHERE source_account_id = $1',
        [this.sourceAccountId]
      ),
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM tmdb_tv_episodes WHERE source_account_id = $1',
        [this.sourceAccountId]
      ),
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM tmdb_genres WHERE source_account_id = $1',
        [this.sourceAccountId]
      ),
      this.query<{ count: string }>(
        "SELECT COUNT(*) as count FROM tmdb_match_queue WHERE source_account_id = $1 AND status = 'pending'",
        [this.sourceAccountId]
      ),
    ]);

    const lastSyncResult = await this.query<{ max_synced: Date | null }>(
      `SELECT GREATEST(
        (SELECT MAX(synced_at) FROM tmdb_movies WHERE source_account_id = $1),
        (SELECT MAX(synced_at) FROM tmdb_tv_shows WHERE source_account_id = $1)
      ) as max_synced`,
      [this.sourceAccountId]
    );

    return {
      movies: parseInt(movies.rows[0]?.count ?? '0', 10),
      tvShows: parseInt(tvShows.rows[0]?.count ?? '0', 10),
      seasons: parseInt(seasons.rows[0]?.count ?? '0', 10),
      episodes: parseInt(episodes.rows[0]?.count ?? '0', 10),
      genres: parseInt(genres.rows[0]?.count ?? '0', 10),
      matchQueue: parseInt(matchQueue.rows[0]?.count ?? '0', 10),
      lastSyncedAt: lastSyncResult.rows[0]?.max_synced ?? null,
    };
  }
}
