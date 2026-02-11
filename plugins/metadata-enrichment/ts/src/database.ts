import { Pool, PoolClient } from 'pg';
import { createLogger } from '@nself/plugin-utils';

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
      CREATE TABLE IF NOT EXISTS metadata_movies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        tmdb_id INT UNIQUE NOT NULL,
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
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE TABLE IF NOT EXISTS metadata_tv_shows (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        tmdb_id INT UNIQUE NOT NULL,
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
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_movies_tmdb ON metadata_movies(tmdb_id);
      CREATE INDEX IF NOT EXISTS idx_tv_shows_tmdb ON metadata_tv_shows(tmdb_id);
    `);
  }

  async searchMovies(query: string): Promise<any[]> {
    const result = await this.pool.query(
      `SELECT * FROM metadata_movies WHERE title ILIKE $1 LIMIT 20`,
      [`%${query}%`]
    );
    return result.rows;
  }

  async searchTVShows(query: string): Promise<any[]> {
    const result = await this.pool.query(
      `SELECT * FROM metadata_tv_shows WHERE name ILIKE $1 LIMIT 20`,
      [`%${query}%`]
    );
    return result.rows;
  }

  async close(): Promise<void> {
    await this.pool.end();
  }
}
