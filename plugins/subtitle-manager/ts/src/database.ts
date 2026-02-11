import { Pool, PoolClient } from 'pg';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('subtitle-manager:database');

export class SubtitleManagerDatabase {
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
      CREATE TABLE IF NOT EXISTS subtitles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        media_id VARCHAR(255) NOT NULL,
        media_type VARCHAR(50) NOT NULL,
        language VARCHAR(10) NOT NULL,
        file_path TEXT NOT NULL,
        source VARCHAR(50) NOT NULL,
        sync_score DECIMAL(5,2),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_subtitles_media
        ON subtitles(media_id, language);
    `);
  }

  async searchSubtitles(mediaId: string, language: string): Promise<any[]> {
    const result = await this.pool.query(
      `SELECT * FROM subtitles WHERE media_id = $1 AND language = $2`,
      [mediaId, language]
    );
    return result.rows;
  }

  async close(): Promise<void> {
    await this.pool.end();
  }
}
