/**
 * Database operations for file processing
 */

import { createLogger } from '@nself/plugin-utils';
import { Pool } from 'pg';
import type {
  ProcessingJob,
  FileThumbnail,
  FileScan,
  FileMetadata,
  CreateJobRequest,
  ProcessingStatus,
  ThumbnailResult,
  ScanResult,
  MetadataResult,
} from './types.js';

const logger = createLogger('file-processing:database');

// Tables that belong to this plugin
const ALL_TABLES = [
  'file_thumbnails',
  'file_scans',
  'file_metadata',
  'file_processing_jobs',
] as const;

export class Database {
  private pool: Pool;
  private sourceAccountId: string;

  constructor(
    config?: { host?: string; port?: number; database?: string; user?: string; password?: string; ssl?: boolean },
    sourceAccountId = 'primary',
  ) {
    const dbConfig = config ?? {
      host: process.env.POSTGRES_HOST ?? 'localhost',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    };

    this.pool = new Pool({
      host: dbConfig.host,
      port: dbConfig.port,
      database: dbConfig.database,
      user: dbConfig.user,
      password: dbConfig.password,
      ssl: dbConfig.ssl ? { rejectUnauthorized: false } : false,
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });

    this.sourceAccountId = sourceAccountId;
  }

  /** Return a new Database handle scoped to a different source account, sharing the same pool. */
  forSourceAccount(accountId: string): Database {
    const scoped = Object.create(Database.prototype) as Database;
    scoped.pool = this.pool;
    scoped.sourceAccountId = accountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // =========================================================================
  // Migration: add source_account_id to existing tables
  // =========================================================================

  async migrateMultiApp(): Promise<void> {
    for (const table of ALL_TABLES) {
      const colCheck = await this.pool.query(
        `SELECT 1 FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = $1
           AND column_name = 'source_account_id'`,
        [table],
      );

      if (colCheck.rowCount === 0) {
        logger.info(`Adding source_account_id to ${table}`);
        await this.pool.query(
          `ALTER TABLE ${table} ADD COLUMN source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`,
        );
        await this.pool.query(
          `CREATE INDEX IF NOT EXISTS idx_${table}_account ON ${table}(source_account_id)`,
        );
      }
    }
  }

  // =========================================================================
  // Processing Jobs
  // =========================================================================

  async createJob(data: CreateJobRequest): Promise<string> {
    const query = `
      INSERT INTO file_processing_jobs (
        file_id, file_path, file_name, file_size, mime_type,
        storage_provider, storage_bucket, operations, priority,
        webhook_url, webhook_secret, callback_data,
        source_account_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
      RETURNING id
    `;

    const result = await this.pool.query(query, [
      data.fileId,
      data.filePath,
      data.fileName,
      data.fileSize,
      data.mimeType,
      process.env.FILE_STORAGE_PROVIDER,
      process.env.FILE_STORAGE_BUCKET,
      JSON.stringify(data.operations || ['thumbnail', 'optimize', 'metadata']),
      data.priority || 5,
      data.webhookUrl,
      data.webhookSecret,
      data.callbackData ? JSON.stringify(data.callbackData) : null,
      this.sourceAccountId,
    ]);

    return result.rows[0].id;
  }

  async getJob(jobId: string): Promise<ProcessingJob | null> {
    const query = 'SELECT * FROM file_processing_jobs WHERE id = $1 AND source_account_id = $2';
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows[0] || null;
  }

  async updateJobStatus(
    jobId: string,
    status: ProcessingStatus,
    error?: { message: string; stack?: string }
  ): Promise<void> {
    const query = `
      UPDATE file_processing_jobs
      SET status = $2,
          error_message = $3,
          error_stack = $4,
          last_error_at = CASE WHEN $2 = 'failed' THEN NOW() ELSE last_error_at END,
          updated_at = NOW()
      WHERE id = $1 AND source_account_id = $5
    `;

    await this.pool.query(query, [
      jobId,
      status,
      error?.message || null,
      error?.stack || null,
      this.sourceAccountId,
    ]);
  }

  async getNextJob(queueName: string = 'default'): Promise<string | null> {
    // The get_next_job function is not multi-app aware, so we use direct SQL
    const query = `
      UPDATE file_processing_jobs
      SET status = 'processing', attempts = attempts + 1, updated_at = NOW()
      WHERE id = (
        SELECT id FROM file_processing_jobs
        WHERE status = 'pending'
          AND queue_name = $1
          AND source_account_id = $2
          AND (scheduled_for IS NULL OR scheduled_for <= NOW())
        ORDER BY priority DESC, created_at ASC
        LIMIT 1
        FOR UPDATE SKIP LOCKED
      )
      RETURNING id
    `;
    const result = await this.pool.query(query, [queueName, this.sourceAccountId]);
    return result.rows[0]?.id || null;
  }

  async listJobs(
    status?: ProcessingStatus,
    limit: number = 50,
    offset: number = 0
  ): Promise<ProcessingJob[]> {
    let query: string;
    const params: unknown[] = [];

    if (status) {
      query = 'SELECT * FROM file_processing_jobs WHERE status = $1 AND source_account_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4';
      params.push(status, this.sourceAccountId, limit, offset);
    } else {
      query = 'SELECT * FROM file_processing_jobs WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3';
      params.push(this.sourceAccountId, limit, offset);
    }

    const result = await this.pool.query(query, params);
    return result.rows;
  }

  // =========================================================================
  // Thumbnails
  // =========================================================================

  async saveThumbnail(jobId: string, thumbnail: ThumbnailResult): Promise<string> {
    const query = `
      INSERT INTO file_thumbnails (
        job_id, file_id, thumbnail_path, thumbnail_url,
        width, height, size_bytes, format,
        generation_time_ms, storage_provider, storage_bucket,
        source_account_id
      )
      SELECT
        $1, j.file_id, $2, $3, $4, $5, $6, $7, $8, j.storage_provider, j.storage_bucket,
        j.source_account_id
      FROM file_processing_jobs j
      WHERE j.id = $1 AND j.source_account_id = $9
      RETURNING id
    `;

    const result = await this.pool.query(query, [
      jobId,
      thumbnail.path,
      thumbnail.url,
      thumbnail.width,
      thumbnail.height,
      thumbnail.size,
      thumbnail.format,
      thumbnail.generationTime,
      this.sourceAccountId,
    ]);

    return result.rows[0].id;
  }

  async getThumbnails(jobId: string): Promise<FileThumbnail[]> {
    const query = 'SELECT * FROM file_thumbnails WHERE job_id = $1 AND source_account_id = $2 ORDER BY width';
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows;
  }

  // =========================================================================
  // Virus Scans
  // =========================================================================

  async saveScan(jobId: string, scan: ScanResult): Promise<string> {
    const query = `
      INSERT INTO file_scans (
        job_id, file_id, scan_status, is_clean,
        threats_found, threat_names, signature_version,
        scan_duration_ms, file_size_scanned,
        source_account_id
      )
      SELECT
        $1, j.file_id, $2, $3, $4, $5, $6, $7, j.file_size,
        j.source_account_id
      FROM file_processing_jobs j
      WHERE j.id = $1 AND j.source_account_id = $8
      RETURNING id
    `;

    const result = await this.pool.query(query, [
      jobId,
      scan.status,
      scan.isClean,
      scan.threatsFound,
      scan.threatNames,
      scan.signatureVersion,
      scan.scanDuration,
      this.sourceAccountId,
    ]);

    return result.rows[0].id;
  }

  async getScan(jobId: string): Promise<FileScan | null> {
    const query = 'SELECT * FROM file_scans WHERE job_id = $1 AND source_account_id = $2 ORDER BY scanned_at DESC LIMIT 1';
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows[0] || null;
  }

  // =========================================================================
  // Metadata
  // =========================================================================

  async saveMetadata(jobId: string, metadata: MetadataResult): Promise<string> {
    const query = `
      INSERT INTO file_metadata (
        job_id, file_id, mime_type, file_size,
        exif_data, exif_stripped,
        metadata_extracted_at, extraction_duration_ms,
        source_account_id
      )
      SELECT
        $1, j.file_id, j.mime_type, j.file_size,
        $2, $3, NOW(), $4,
        j.source_account_id
      FROM file_processing_jobs j
      WHERE j.id = $1 AND j.source_account_id = $5
      ON CONFLICT (file_id) DO UPDATE SET
        exif_data = EXCLUDED.exif_data,
        exif_stripped = EXCLUDED.exif_stripped,
        metadata_extracted_at = EXCLUDED.metadata_extracted_at,
        extraction_duration_ms = EXCLUDED.extraction_duration_ms,
        source_account_id = EXCLUDED.source_account_id,
        updated_at = NOW()
      RETURNING id
    `;

    const result = await this.pool.query(query, [
      jobId,
      JSON.stringify(metadata.extracted),
      metadata.exifStripped,
      metadata.extractionTime,
      this.sourceAccountId,
    ]);

    return result.rows[0].id;
  }

  async getMetadata(jobId: string): Promise<FileMetadata | null> {
    const query = `
      SELECT m.*
      FROM file_metadata m
      JOIN file_processing_jobs j ON m.job_id = j.id
      WHERE j.id = $1
        AND m.source_account_id = $2
    `;
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows[0] || null;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<{
    pending: number;
    processing: number;
    completed: number;
    failed: number;
    avgDurationMs: number;
    totalProcessed: number;
    thumbnailsGenerated: number;
    storageUsed: number;
  }> {
    const query = `
      SELECT
        COUNT(*) FILTER (WHERE status = 'pending') AS pending,
        COUNT(*) FILTER (WHERE status = 'processing') AS processing,
        COUNT(*) FILTER (WHERE status = 'completed') AS completed,
        COUNT(*) FILTER (WHERE status = 'failed') AS failed,
        COALESCE(AVG(duration_ms) FILTER (WHERE status = 'completed'), 0) AS avg_duration_ms,
        COUNT(*) FILTER (WHERE status IN ('completed', 'failed')) AS total_processed,
        (SELECT COUNT(*) FROM file_thumbnails WHERE source_account_id = $1) AS thumbnails_generated,
        (SELECT COALESCE(SUM(size_bytes), 0) FROM file_thumbnails WHERE source_account_id = $1) AS storage_used
      FROM file_processing_jobs
      WHERE source_account_id = $1
    `;

    const result = await this.pool.query(query, [this.sourceAccountId]);
    const row = result.rows[0];

    return {
      pending: parseInt(row.pending, 10),
      processing: parseInt(row.processing, 10),
      completed: parseInt(row.completed, 10),
      failed: parseInt(row.failed, 10),
      avgDurationMs: Math.round(parseFloat(row.avg_duration_ms)),
      totalProcessed: parseInt(row.total_processed, 10),
      thumbnailsGenerated: parseInt(row.thumbnails_generated, 10),
      storageUsed: parseInt(row.storage_used, 10),
    };
  }

  async cleanup(retentionDays: number = 30): Promise<number> {
    const query = `
      DELETE FROM file_processing_jobs
      WHERE status IN ('completed', 'cancelled')
        AND completed_at < NOW() - ($1 || ' days')::INTERVAL
        AND source_account_id = $2
    `;
    const result = await this.pool.query(query, [retentionDays, this.sourceAccountId]);
    return result.rowCount ?? 0;
  }

  // =========================================================================
  // Multi-App Cleanup
  // =========================================================================

  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    let total = 0;
    // Child tables first, parent table last to respect FK constraints
    for (const table of ALL_TABLES) {
      const result = await this.pool.query(
        `DELETE FROM ${table} WHERE source_account_id = $1`,
        [sourceAccountId],
      );
      total += result.rowCount ?? 0;
    }
    return total;
  }
}
