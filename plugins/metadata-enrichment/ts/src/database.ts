import { Pool, PoolClient } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type { MovieMetadata, TVShowMetadata } from './types.js';

const logger = createLogger('metadata-enrichment:database');

export class MetadataEnrichmentDatabase {
  private pool: Pool;

  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString });
  }

  async initialize(): Promise<void> {
    const client = await this.pool.connect();
    try {
      await this.createSchema(client);
      logger.info('Database schema initialized');
    } finally {
      client.release();
    }
  }

  private async createSchema(client: PoolClient): Promise<void> {
    await client.query(`
      CREATE TABLE IF NOT EXISTS np_metaenrich_movies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
        tmdb_id INT NOT NULL,
        imdb_id VARCHAR(20),
        title VARCHAR(500) NOT NULL,
        original_title VARCHAR(500),
        overview TEXT,
        release_date DATE,
        runtime INT,
        genres VARCHAR(50)[],
        vote_average DECIMAL(3,1),
        vote_count INT,
        poster_path VARCHAR(500),
        backdrop_path VARCHAR(500),
        raw_response JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, tmdb_id)
      );

      CREATE TABLE IF NOT EXISTS np_metaenrich_tv_shows (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
        tmdb_id INT NOT NULL,
        tvdb_id INT,
        imdb_id VARCHAR(20),
        name VARCHAR(500) NOT NULL,
        original_name VARCHAR(500),
        overview TEXT,
        first_air_date DATE,
        last_air_date DATE,
        number_of_seasons INT,
        number_of_episodes INT,
        genres VARCHAR(50)[],
        vote_average DECIMAL(3,1),
        vote_count INT,
        poster_path VARCHAR(500),
        backdrop_path VARCHAR(500),
        raw_response JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, tmdb_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_metaenrich_movies_tmdb
        ON np_metaenrich_movies(source_account_id, tmdb_id);
      CREATE INDEX IF NOT EXISTS idx_np_metaenrich_movies_title
        ON np_metaenrich_movies(source_account_id, title);
      CREATE INDEX IF NOT EXISTS idx_np_metaenrich_tv_shows_tmdb
        ON np_metaenrich_tv_shows(source_account_id, tmdb_id);
      CREATE INDEX IF NOT EXISTS idx_np_metaenrich_tv_shows_name
        ON np_metaenrich_tv_shows(source_account_id, name);
    `);
  }

  // ---------------------------------------------------------------------------
  // Movie upsert and retrieval
  // ---------------------------------------------------------------------------

  async upsertMovie(movie: Partial<MovieMetadata> & { tmdb_id: number; title: string }, sourceAccountId = 'primary'): Promise<MovieMetadata> {
    const result = await this.pool.query(
      `INSERT INTO np_metaenrich_movies (
        source_account_id, tmdb_id, imdb_id, title, original_title,
        overview, release_date, runtime, genres, vote_average,
        vote_count, poster_path, backdrop_path, raw_response, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
      ON CONFLICT (source_account_id, tmdb_id) DO UPDATE SET
        imdb_id = COALESCE(EXCLUDED.imdb_id, np_metaenrich_movies.imdb_id),
        title = EXCLUDED.title,
        original_title = COALESCE(EXCLUDED.original_title, np_metaenrich_movies.original_title),
        overview = COALESCE(EXCLUDED.overview, np_metaenrich_movies.overview),
        release_date = COALESCE(EXCLUDED.release_date, np_metaenrich_movies.release_date),
        runtime = COALESCE(EXCLUDED.runtime, np_metaenrich_movies.runtime),
        genres = COALESCE(EXCLUDED.genres, np_metaenrich_movies.genres),
        vote_average = COALESCE(EXCLUDED.vote_average, np_metaenrich_movies.vote_average),
        vote_count = COALESCE(EXCLUDED.vote_count, np_metaenrich_movies.vote_count),
        poster_path = COALESCE(EXCLUDED.poster_path, np_metaenrich_movies.poster_path),
        backdrop_path = COALESCE(EXCLUDED.backdrop_path, np_metaenrich_movies.backdrop_path),
        raw_response = COALESCE(EXCLUDED.raw_response, np_metaenrich_movies.raw_response),
        updated_at = NOW()
      RETURNING *`,
      [
        sourceAccountId,
        movie.tmdb_id,
        movie.imdb_id ?? null,
        movie.title,
        movie.original_title ?? null,
        movie.overview ?? null,
        movie.release_date ?? null,
        movie.runtime ?? null,
        movie.genres ?? null,
        movie.vote_average ?? null,
        movie.vote_count ?? null,
        movie.poster_path ?? null,
        movie.backdrop_path ?? null,
        movie.raw_response ?? '{}',
      ]
    );
    return result.rows[0] as MovieMetadata;
  }

  async getMovieByTmdbId(tmdbId: number, sourceAccountId = 'primary'): Promise<MovieMetadata | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_metaenrich_movies
       WHERE source_account_id = $1 AND tmdb_id = $2`,
      [sourceAccountId, tmdbId]
    );
    return (result.rows[0] as MovieMetadata) ?? null;
  }

  // ---------------------------------------------------------------------------
  // TV Show upsert and retrieval
  // ---------------------------------------------------------------------------

  async upsertTVShow(show: Partial<TVShowMetadata> & { tmdb_id: number; name: string }, sourceAccountId = 'primary'): Promise<TVShowMetadata> {
    const result = await this.pool.query(
      `INSERT INTO np_metaenrich_tv_shows (
        source_account_id, tmdb_id, tvdb_id, imdb_id, name, original_name,
        overview, first_air_date, last_air_date, number_of_seasons,
        number_of_episodes, genres, vote_average, vote_count,
        poster_path, backdrop_path, raw_response, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
      ON CONFLICT (source_account_id, tmdb_id) DO UPDATE SET
        tvdb_id = COALESCE(EXCLUDED.tvdb_id, np_metaenrich_tv_shows.tvdb_id),
        imdb_id = COALESCE(EXCLUDED.imdb_id, np_metaenrich_tv_shows.imdb_id),
        name = EXCLUDED.name,
        original_name = COALESCE(EXCLUDED.original_name, np_metaenrich_tv_shows.original_name),
        overview = COALESCE(EXCLUDED.overview, np_metaenrich_tv_shows.overview),
        first_air_date = COALESCE(EXCLUDED.first_air_date, np_metaenrich_tv_shows.first_air_date),
        last_air_date = COALESCE(EXCLUDED.last_air_date, np_metaenrich_tv_shows.last_air_date),
        number_of_seasons = COALESCE(EXCLUDED.number_of_seasons, np_metaenrich_tv_shows.number_of_seasons),
        number_of_episodes = COALESCE(EXCLUDED.number_of_episodes, np_metaenrich_tv_shows.number_of_episodes),
        genres = COALESCE(EXCLUDED.genres, np_metaenrich_tv_shows.genres),
        vote_average = COALESCE(EXCLUDED.vote_average, np_metaenrich_tv_shows.vote_average),
        vote_count = COALESCE(EXCLUDED.vote_count, np_metaenrich_tv_shows.vote_count),
        poster_path = COALESCE(EXCLUDED.poster_path, np_metaenrich_tv_shows.poster_path),
        backdrop_path = COALESCE(EXCLUDED.backdrop_path, np_metaenrich_tv_shows.backdrop_path),
        raw_response = COALESCE(EXCLUDED.raw_response, np_metaenrich_tv_shows.raw_response),
        updated_at = NOW()
      RETURNING *`,
      [
        sourceAccountId,
        show.tmdb_id,
        show.tvdb_id ?? null,
        show.imdb_id ?? null,
        show.name,
        show.original_name ?? null,
        show.overview ?? null,
        show.first_air_date ?? null,
        show.last_air_date ?? null,
        show.number_of_seasons ?? null,
        show.number_of_episodes ?? null,
        show.genres ?? null,
        show.vote_average ?? null,
        show.vote_count ?? null,
        show.poster_path ?? null,
        show.backdrop_path ?? null,
        show.raw_response ?? '{}',
      ]
    );
    return result.rows[0] as TVShowMetadata;
  }

  async getTVShowByTmdbId(tmdbId: number, sourceAccountId = 'primary'): Promise<TVShowMetadata | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_metaenrich_tv_shows
       WHERE source_account_id = $1 AND tmdb_id = $2`,
      [sourceAccountId, tmdbId]
    );
    return (result.rows[0] as TVShowMetadata) ?? null;
  }

  // ---------------------------------------------------------------------------
  // Search (local cache)
  // ---------------------------------------------------------------------------

  async searchMovies(query: string, sourceAccountId = 'primary'): Promise<MovieMetadata[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_metaenrich_movies
       WHERE source_account_id = $1 AND title ILIKE $2
       ORDER BY vote_count DESC NULLS LAST
       LIMIT 20`,
      [sourceAccountId, `%${query}%`]
    );
    return result.rows as MovieMetadata[];
  }

  async searchTVShows(query: string, sourceAccountId = 'primary'): Promise<TVShowMetadata[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_metaenrich_tv_shows
       WHERE source_account_id = $1 AND name ILIKE $2
       ORDER BY vote_count DESC NULLS LAST
       LIMIT 20`,
      [sourceAccountId, `%${query}%`]
    );
    return result.rows as TVShowMetadata[];
  }

  async close(): Promise<void> {
    await this.pool.end();
  }
}
