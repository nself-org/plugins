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
  'np_fileproc_thumbnails',
  'np_fileproc_scans',
  'np_fileproc_metadata',
  'np_fileproc_jobs',
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
  // Schema Initialization
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Checking schema...');

    // Check if schema exists by looking for the jobs table
    const schemaExists = await this.pool.query(
      `SELECT EXISTS (
        SELECT FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_name = 'np_fileproc_jobs'
      )`,
    );

    if (!schemaExists.rows[0].exists) {
      logger.info('Creating initial schema...');
      await this.createInitialSchema();
    } else {
      logger.info('Schema exists, running migrations...');
      await this.migrateMultiApp();
    }
  }

  private async createInitialSchema(): Promise<void> {
    const schema = `
      -- Enable UUID extension
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- File Processing Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_fileproc_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        file_id VARCHAR(255) NOT NULL,
        file_path TEXT NOT NULL,
        file_name VARCHAR(500) NOT NULL,
        file_size BIGINT NOT NULL,
        mime_type VARCHAR(100) NOT NULL,
        storage_provider VARCHAR(50) NOT NULL,
        storage_bucket VARCHAR(255) NOT NULL,

        -- Processing status
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        priority INTEGER DEFAULT 5,
        attempts INTEGER DEFAULT 0,
        max_attempts INTEGER DEFAULT 3,

        -- Processing operations
        operations JSONB NOT NULL DEFAULT '[]',

        -- Results
        thumbnails JSONB DEFAULT '[]',
        metadata JSONB DEFAULT '{}',
        scan_result JSONB,
        optimization_result JSONB,

        -- Error tracking
        error_message TEXT,
        error_stack TEXT,
        last_error_at TIMESTAMPTZ,

        -- Timing
        started_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        duration_ms INTEGER,

        -- Queue metadata
        queue_name VARCHAR(100) DEFAULT 'default',
        scheduled_for TIMESTAMPTZ,
        webhook_url TEXT,
        webhook_secret VARCHAR(255),
        callback_data JSONB,

        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_account ON np_fileproc_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_file_id ON np_fileproc_jobs(file_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_status ON np_fileproc_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_priority ON np_fileproc_jobs(priority DESC);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_created ON np_fileproc_jobs(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_queue ON np_fileproc_jobs(queue_name, status);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_jobs_scheduled ON np_fileproc_jobs(scheduled_for) WHERE status = 'pending';

      -- =====================================================================
      -- File Thumbnails
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_fileproc_thumbnails (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_id UUID REFERENCES np_fileproc_jobs(id) ON DELETE CASCADE,
        file_id VARCHAR(255) NOT NULL,

        -- Thumbnail details
        thumbnail_path TEXT NOT NULL,
        thumbnail_url TEXT,
        width INTEGER NOT NULL,
        height INTEGER NOT NULL,
        size_bytes BIGINT,
        format VARCHAR(20) NOT NULL,

        -- Processing details
        source_width INTEGER,
        source_height INTEGER,
        quality INTEGER,
        optimization_applied BOOLEAN DEFAULT FALSE,

        -- Metadata
        generation_time_ms INTEGER,
        storage_provider VARCHAR(50) NOT NULL,
        storage_bucket VARCHAR(255) NOT NULL,

        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_fileproc_thumbnails_account ON np_fileproc_thumbnails(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_thumbnails_job_id ON np_fileproc_thumbnails(job_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_thumbnails_file_id ON np_fileproc_thumbnails(file_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_thumbnails_dimensions ON np_fileproc_thumbnails(width, height);

      -- =====================================================================
      -- File Virus Scans
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_fileproc_scans (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_id UUID REFERENCES np_fileproc_jobs(id) ON DELETE CASCADE,
        file_id VARCHAR(255) NOT NULL,

        -- Scan details
        scanner VARCHAR(50) NOT NULL DEFAULT 'clamav',
        scan_status VARCHAR(20) NOT NULL,

        -- Results
        is_clean BOOLEAN,
        threats_found INTEGER DEFAULT 0,
        threat_names TEXT[],
        signature_version VARCHAR(100),

        -- Scan metadata
        scan_duration_ms INTEGER,
        file_size_scanned BIGINT,

        -- Error handling
        error_message TEXT,

        -- Timestamps
        scanned_at TIMESTAMPTZ DEFAULT NOW(),
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_fileproc_scans_account ON np_fileproc_scans(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_scans_job_id ON np_fileproc_scans(job_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_scans_file_id ON np_fileproc_scans(file_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_scans_status ON np_fileproc_scans(scan_status);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_scans_infected ON np_fileproc_scans(is_clean) WHERE is_clean = FALSE;

      -- =====================================================================
      -- File Metadata
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_fileproc_metadata (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_id UUID REFERENCES np_fileproc_jobs(id) ON DELETE CASCADE,
        file_id VARCHAR(255) NOT NULL UNIQUE,

        -- Basic file info
        mime_type VARCHAR(100) NOT NULL,
        file_extension VARCHAR(50),
        file_size BIGINT NOT NULL,

        -- Image metadata
        width INTEGER,
        height INTEGER,
        aspect_ratio DECIMAL(10,4),
        color_space VARCHAR(50),
        bit_depth INTEGER,
        has_alpha BOOLEAN,

        -- EXIF data (before stripping)
        exif_data JSONB,
        camera_make VARCHAR(100),
        camera_model VARCHAR(100),
        lens_model VARCHAR(100),
        focal_length VARCHAR(50),
        aperture VARCHAR(50),
        shutter_speed VARCHAR(50),
        iso INTEGER,
        flash VARCHAR(50),
        orientation INTEGER,

        -- Location data (if present)
        gps_latitude DECIMAL(10,6),
        gps_longitude DECIMAL(10,6),
        gps_altitude DECIMAL(10,2),
        location_name TEXT,

        -- Date/time
        date_taken TIMESTAMPTZ,
        date_modified TIMESTAMPTZ,

        -- Video metadata
        duration_seconds DECIMAL(10,2),
        video_codec VARCHAR(50),
        audio_codec VARCHAR(50),
        frame_rate DECIMAL(10,2),
        bitrate BIGINT,

        -- Audio metadata
        audio_channels INTEGER,
        sample_rate INTEGER,

        -- Document metadata
        page_count INTEGER,
        word_count INTEGER,
        author VARCHAR(255),
        title VARCHAR(500),
        subject VARCHAR(500),

        -- Hashes for duplicate detection
        md5_hash VARCHAR(32),
        sha256_hash VARCHAR(64),
        perceptual_hash VARCHAR(64),

        -- Processing info
        exif_stripped BOOLEAN DEFAULT FALSE,
        metadata_extracted_at TIMESTAMPTZ,
        extraction_duration_ms INTEGER,

        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_account ON np_fileproc_metadata(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_file_id ON np_fileproc_metadata(file_id);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_mime_type ON np_fileproc_metadata(mime_type);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_dimensions ON np_fileproc_metadata(width, height);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_md5 ON np_fileproc_metadata(md5_hash);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_sha256 ON np_fileproc_metadata(sha256_hash);
      CREATE INDEX IF NOT EXISTS idx_np_fileproc_metadata_date_taken ON np_fileproc_metadata(date_taken);
    `;

    await this.pool.query(schema);
    logger.info('Initial schema created successfully');
  }

  // =========================================================================
  // Migration: add source_account_id to existing tables (for old installs)
  // =========================================================================

  private async migrateMultiApp(): Promise<void> {
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
      INSERT INTO np_fileproc_jobs (
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
    const query = 'SELECT * FROM np_fileproc_jobs WHERE id = $1 AND source_account_id = $2';
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows[0] || null;
  }

  async updateJobStatus(
    jobId: string,
    status: ProcessingStatus,
    error?: { message: string; stack?: string }
  ): Promise<void> {
    const query = `
      UPDATE np_fileproc_jobs
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
      UPDATE np_fileproc_jobs
      SET status = 'processing', attempts = attempts + 1, updated_at = NOW()
      WHERE id = (
        SELECT id FROM np_fileproc_jobs
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
      query = 'SELECT * FROM np_fileproc_jobs WHERE status = $1 AND source_account_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4';
      params.push(status, this.sourceAccountId, limit, offset);
    } else {
      query = 'SELECT * FROM np_fileproc_jobs WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3';
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
      INSERT INTO np_fileproc_thumbnails (
        job_id, file_id, thumbnail_path, thumbnail_url,
        width, height, size_bytes, format,
        generation_time_ms, storage_provider, storage_bucket,
        source_account_id
      )
      SELECT
        $1, j.file_id, $2, $3, $4, $5, $6, $7, $8, j.storage_provider, j.storage_bucket,
        j.source_account_id
      FROM np_fileproc_jobs j
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
    const query = 'SELECT * FROM np_fileproc_thumbnails WHERE job_id = $1 AND source_account_id = $2 ORDER BY width';
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows;
  }

  // =========================================================================
  // Virus Scans
  // =========================================================================

  async saveScan(jobId: string, scan: ScanResult): Promise<string> {
    const query = `
      INSERT INTO np_fileproc_scans (
        job_id, file_id, scan_status, is_clean,
        threats_found, threat_names, signature_version,
        scan_duration_ms, file_size_scanned,
        source_account_id
      )
      SELECT
        $1, j.file_id, $2, $3, $4, $5, $6, $7, j.file_size,
        j.source_account_id
      FROM np_fileproc_jobs j
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
    const query = 'SELECT * FROM np_fileproc_scans WHERE job_id = $1 AND source_account_id = $2 ORDER BY scanned_at DESC LIMIT 1';
    const result = await this.pool.query(query, [jobId, this.sourceAccountId]);
    return result.rows[0] || null;
  }

  // =========================================================================
  // Metadata
  // =========================================================================

  async saveMetadata(jobId: string, metadata: MetadataResult): Promise<string> {
    const query = `
      INSERT INTO np_fileproc_metadata (
        job_id, file_id, mime_type, file_size,
        exif_data, exif_stripped,
        metadata_extracted_at, extraction_duration_ms,
        source_account_id
      )
      SELECT
        $1, j.file_id, j.mime_type, j.file_size,
        $2, $3, NOW(), $4,
        j.source_account_id
      FROM np_fileproc_jobs j
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
      FROM np_fileproc_metadata m
      JOIN np_fileproc_jobs j ON m.job_id = j.id
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
        (SELECT COUNT(*) FROM np_fileproc_thumbnails WHERE source_account_id = $1) AS thumbnails_generated,
        (SELECT COALESCE(SUM(size_bytes), 0) FROM np_fileproc_thumbnails WHERE source_account_id = $1) AS storage_used
      FROM np_fileproc_jobs
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
      DELETE FROM np_fileproc_jobs
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
