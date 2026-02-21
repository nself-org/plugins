import { Pool, PoolClient } from 'pg';
import { createLogger } from '@nself/plugin-utils';
import type {
  SubtitleRecord,
  SubtitleDownloadRecord,
  UpsertSubtitleInput,
  InsertDownloadInput,
  SubtitleStats,
  QCResultRecord,
  InsertQCResultInput,
  QualityCheckDetails,
} from './types.js';

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
      CREATE TABLE IF NOT EXISTS np_subtmgr_subtitles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        media_id VARCHAR(255) NOT NULL,
        media_type VARCHAR(50) NOT NULL,
        language VARCHAR(10) NOT NULL,
        file_path TEXT NOT NULL,
        source VARCHAR(50) NOT NULL,
        sync_score DECIMAL(5,2),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_subtmgr_subtitles_media
        ON np_subtmgr_subtitles(media_id, language);

      CREATE INDEX IF NOT EXISTS idx_np_subtmgr_subtitles_account
        ON np_subtmgr_subtitles(source_account_id);

      CREATE TABLE IF NOT EXISTS np_subtmgr_downloads (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        subtitle_id UUID REFERENCES np_subtmgr_subtitles(id) ON DELETE CASCADE,
        media_id VARCHAR(255) NOT NULL,
        media_type VARCHAR(50) NOT NULL,
        media_title VARCHAR(255),
        language VARCHAR(10) NOT NULL,
        file_path TEXT NOT NULL,
        file_size_bytes BIGINT,
        opensubtitles_file_id INT,
        file_hash VARCHAR(64),
        sync_score DECIMAL(5,2),
        source VARCHAR(50) NOT NULL DEFAULT 'opensubtitles',
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_subtmgr_downloads_media
        ON np_subtmgr_downloads(media_id, language);

      CREATE INDEX IF NOT EXISTS idx_np_subtmgr_downloads_account
        ON np_subtmgr_downloads(source_account_id);

      -- Add QC columns to downloads table (safe to re-run)
      ALTER TABLE np_subtmgr_downloads
        ADD COLUMN IF NOT EXISTS qc_status VARCHAR(20),
        ADD COLUMN IF NOT EXISTS qc_details JSONB;

      -- QC results table
      CREATE TABLE IF NOT EXISTS np_subtmgr_qc_results (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        download_id UUID REFERENCES np_subtmgr_downloads(id) ON DELETE CASCADE,
        status VARCHAR(20) NOT NULL,
        checks JSONB NOT NULL DEFAULT '[]',
        issues JSONB NOT NULL DEFAULT '[]',
        cue_count INT NOT NULL DEFAULT 0,
        total_duration_ms BIGINT NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_subtmgr_qc_results_download
        ON np_subtmgr_qc_results(download_id);

      CREATE INDEX IF NOT EXISTS idx_np_subtmgr_qc_results_account
        ON np_subtmgr_qc_results(source_account_id);
    `);
  }

  // ---------------------------------------------------------------------------
  // Subtitles CRUD
  // ---------------------------------------------------------------------------

  async searchSubtitles(
    mediaId: string,
    language: string,
    sourceAccountId: string = 'primary',
  ): Promise<SubtitleRecord[]> {
    const result = await this.pool.query(
      `SELECT * FROM np_subtmgr_subtitles
       WHERE media_id = $1 AND language = $2 AND source_account_id = $3
       ORDER BY sync_score DESC NULLS LAST, updated_at DESC`,
      [mediaId, language, sourceAccountId],
    );
    return result.rows;
  }

  async upsertSubtitle(data: UpsertSubtitleInput): Promise<SubtitleRecord> {
    const result = await this.pool.query(
      `INSERT INTO np_subtmgr_subtitles
        (source_account_id, media_id, media_type, language, file_path, source, sync_score)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       ON CONFLICT (id) DO UPDATE SET
         media_type = EXCLUDED.media_type,
         language = EXCLUDED.language,
         file_path = EXCLUDED.file_path,
         source = EXCLUDED.source,
         sync_score = EXCLUDED.sync_score,
         updated_at = NOW()
       RETURNING *`,
      [
        data.source_account_id || 'primary',
        data.media_id,
        data.media_type,
        data.language,
        data.file_path,
        data.source,
        data.sync_score ?? null,
      ],
    );
    return result.rows[0];
  }

  // ---------------------------------------------------------------------------
  // Downloads CRUD
  // ---------------------------------------------------------------------------

  async insertDownload(data: InsertDownloadInput): Promise<SubtitleDownloadRecord> {
    const result = await this.pool.query(
      `INSERT INTO np_subtmgr_downloads
        (source_account_id, subtitle_id, media_id, media_type, media_title,
         language, file_path, file_size_bytes, opensubtitles_file_id,
         file_hash, sync_score, source)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
       RETURNING *`,
      [
        data.source_account_id || 'primary',
        data.subtitle_id ?? null,
        data.media_id,
        data.media_type,
        data.media_title ?? null,
        data.language,
        data.file_path,
        data.file_size_bytes ?? null,
        data.opensubtitles_file_id ?? null,
        data.file_hash ?? null,
        data.sync_score ?? null,
        data.source,
      ],
    );
    return result.rows[0];
  }

  async getDownloadByMediaId(
    mediaId: string,
    language: string,
    sourceAccountId: string = 'primary',
  ): Promise<SubtitleDownloadRecord | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_subtmgr_downloads
       WHERE media_id = $1 AND language = $2 AND source_account_id = $3
       ORDER BY created_at DESC
       LIMIT 1`,
      [mediaId, language, sourceAccountId],
    );
    return result.rows[0] ?? null;
  }

  async listDownloads(
    sourceAccountId: string = 'primary',
    limit: number = 50,
    offset: number = 0,
  ): Promise<{ downloads: SubtitleDownloadRecord[]; total: number }> {
    const countResult = await this.pool.query(
      `SELECT COUNT(*) AS total FROM np_subtmgr_downloads WHERE source_account_id = $1`,
      [sourceAccountId],
    );
    const total = parseInt(countResult.rows[0].total, 10);

    const result = await this.pool.query(
      `SELECT * FROM np_subtmgr_downloads
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [sourceAccountId, limit, offset],
    );

    return { downloads: result.rows, total };
  }

  async deleteDownload(id: string): Promise<boolean> {
    const result = await this.pool.query(
      `DELETE FROM np_subtmgr_downloads WHERE id = $1 RETURNING id`,
      [id],
    );
    return (result.rowCount ?? 0) > 0;
  }

  // ---------------------------------------------------------------------------
  // Stats
  // ---------------------------------------------------------------------------

  async getStats(sourceAccountId: string = 'primary'): Promise<SubtitleStats> {
    const subtitleCountResult = await this.pool.query(
      `SELECT COUNT(*) AS total FROM np_subtmgr_subtitles WHERE source_account_id = $1`,
      [sourceAccountId],
    );

    const downloadCountResult = await this.pool.query(
      `SELECT COUNT(*) AS total FROM np_subtmgr_downloads WHERE source_account_id = $1`,
      [sourceAccountId],
    );

    const languagesResult = await this.pool.query(
      `SELECT language, COUNT(*)::int AS count
       FROM np_subtmgr_downloads
       WHERE source_account_id = $1
       GROUP BY language
       ORDER BY count DESC`,
      [sourceAccountId],
    );

    const sourcesResult = await this.pool.query(
      `SELECT source, COUNT(*)::int AS count
       FROM np_subtmgr_downloads
       WHERE source_account_id = $1
       GROUP BY source
       ORDER BY count DESC`,
      [sourceAccountId],
    );

    return {
      total_subtitles: parseInt(subtitleCountResult.rows[0].total, 10),
      total_downloads: parseInt(downloadCountResult.rows[0].total, 10),
      languages: languagesResult.rows,
      sources: sourcesResult.rows,
    };
  }

  // ---------------------------------------------------------------------------
  // QC Results CRUD
  // ---------------------------------------------------------------------------

  async insertQCResult(data: InsertQCResultInput): Promise<QCResultRecord> {
    const result = await this.pool.query(
      `INSERT INTO np_subtmgr_qc_results
        (source_account_id, download_id, status, checks, issues, cue_count, total_duration_ms)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING *`,
      [
        data.source_account_id || 'primary',
        data.download_id,
        data.status,
        JSON.stringify(data.checks),
        JSON.stringify(data.issues),
        data.cue_count,
        data.total_duration_ms,
      ],
    );
    return result.rows[0];
  }

  async getQCResult(downloadId: string): Promise<QCResultRecord | null> {
    const result = await this.pool.query(
      `SELECT * FROM np_subtmgr_qc_results
       WHERE download_id = $1
       ORDER BY created_at DESC
       LIMIT 1`,
      [downloadId],
    );
    return result.rows[0] ?? null;
  }

  async updateDownloadQC(
    downloadId: string,
    qcStatus: string,
    qcDetails: QualityCheckDetails,
  ): Promise<void> {
    await this.pool.query(
      `UPDATE np_subtmgr_downloads
       SET qc_status = $1, qc_details = $2, updated_at = NOW()
       WHERE id = $3`,
      [qcStatus, JSON.stringify(qcDetails), downloadId],
    );
  }

  async close(): Promise<void> {
    await this.pool.end();
  }
}
