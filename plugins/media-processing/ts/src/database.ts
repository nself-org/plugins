/**
 * Media Processing Database Operations
 * Complete CRUD operations for all media processing objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  EncodingProfileRecord,
  CreateEncodingProfileInput,
  UpdateEncodingProfileInput,
  JobRecord,
  CreateJobInput,
  JobOutputRecord,
  HlsManifestRecord,
  SubtitleRecord,
  TrickplayRecord,
  ProcessingStats,
  JobWithOutputs,
  VariantManifest,
  Resolution,
  JobStatus,
  OutputType,
  DropFolderEvent,
  UploadRecord,
  LeasedJob,
} from './types.js';

const logger = createLogger('media-processing:db');

export class MediaProcessingDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): MediaProcessingDatabase {
    return new MediaProcessingDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing media processing schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
      CREATE EXTENSION IF NOT EXISTS "pgcrypto";

      -- =====================================================================
      -- Encoding Profiles
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_encoding_profiles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(128) NOT NULL,
        description TEXT,
        container VARCHAR(16) DEFAULT 'mp4',
        video_codec VARCHAR(16) DEFAULT 'h264',
        audio_codec VARCHAR(16) DEFAULT 'aac',
        resolutions JSONB NOT NULL DEFAULT '[
          {"width":1920,"height":1080,"bitrate":5000000,"label":"1080p"},
          {"width":1280,"height":720,"bitrate":2500000,"label":"720p"},
          {"width":854,"height":480,"bitrate":1200000,"label":"480p"},
          {"width":640,"height":360,"bitrate":800000,"label":"360p"},
          {"width":426,"height":240,"bitrate":400000,"label":"240p"}
        ]'::jsonb,
        audio_bitrate INTEGER DEFAULT 128000,
        framerate INTEGER DEFAULT 30,
        preset VARCHAR(16) DEFAULT 'medium',
        hls_enabled BOOLEAN DEFAULT true,
        hls_segment_duration INTEGER DEFAULT 6,
        trickplay_enabled BOOLEAN DEFAULT false,
        trickplay_interval INTEGER DEFAULT 10,
        subtitle_extract BOOLEAN DEFAULT true,
        thumbnail_enabled BOOLEAN DEFAULT true,
        thumbnail_count INTEGER DEFAULT 5,
        is_default BOOLEAN DEFAULT false,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_mp_profiles_account ON mp_encoding_profiles(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_profiles_default ON mp_encoding_profiles(is_default) WHERE is_default = true;

      -- =====================================================================
      -- Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        input_url TEXT NOT NULL,
        input_type VARCHAR(32) DEFAULT 'file',
        profile_id UUID REFERENCES mp_encoding_profiles(id) ON DELETE SET NULL,
        status VARCHAR(32) DEFAULT 'pending',
        priority INTEGER DEFAULT 0,
        progress DOUBLE PRECISION DEFAULT 0,
        input_metadata JSONB DEFAULT '{}'::jsonb,
        output_base_path TEXT,
        error_message TEXT,
        duration_seconds DOUBLE PRECISION,
        file_size_bytes BIGINT,
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        heartbeat_at TIMESTAMP WITH TIME ZONE,
        leased_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_mp_jobs_account ON mp_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_jobs_status ON mp_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_mp_jobs_profile ON mp_jobs(profile_id);
      CREATE INDEX IF NOT EXISTS idx_mp_jobs_created ON mp_jobs(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_mp_jobs_priority ON mp_jobs(priority DESC, created_at ASC) WHERE status = 'pending';
      CREATE INDEX IF NOT EXISTS idx_mp_jobs_heartbeat ON mp_jobs(heartbeat_at) WHERE leased_by IS NOT NULL;

      -- =====================================================================
      -- Job Outputs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_job_outputs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES mp_jobs(id) ON DELETE CASCADE,
        output_type VARCHAR(32) NOT NULL,
        resolution_label VARCHAR(16),
        file_path TEXT NOT NULL,
        file_size_bytes BIGINT,
        content_type VARCHAR(128),
        width INTEGER,
        height INTEGER,
        bitrate INTEGER,
        duration_seconds DOUBLE PRECISION,
        language VARCHAR(16),
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_mp_outputs_account ON mp_job_outputs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_outputs_job ON mp_job_outputs(job_id);
      CREATE INDEX IF NOT EXISTS idx_mp_outputs_type ON mp_job_outputs(output_type);

      -- =====================================================================
      -- HLS Manifests
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_hls_manifests (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES mp_jobs(id) ON DELETE CASCADE,
        master_manifest_path TEXT NOT NULL,
        variant_manifests JSONB DEFAULT '[]'::jsonb,
        segment_count INTEGER DEFAULT 0,
        total_duration_seconds DOUBLE PRECISION,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_mp_hls_account ON mp_hls_manifests(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_hls_job ON mp_hls_manifests(job_id);

      -- =====================================================================
      -- Subtitles
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_subtitles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES mp_jobs(id) ON DELETE CASCADE,
        language VARCHAR(16) NOT NULL DEFAULT 'en',
        label VARCHAR(64),
        format VARCHAR(8) DEFAULT 'vtt',
        file_path TEXT NOT NULL,
        is_default BOOLEAN DEFAULT false,
        is_forced BOOLEAN DEFAULT false,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_mp_subtitles_account ON mp_subtitles(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_subtitles_job ON mp_subtitles(job_id);

      -- =====================================================================
      -- Trickplay
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_trickplay (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES mp_jobs(id) ON DELETE CASCADE,
        tile_width INTEGER DEFAULT 320,
        tile_height INTEGER DEFAULT 180,
        columns INTEGER DEFAULT 10,
        rows INTEGER DEFAULT 10,
        interval_seconds INTEGER DEFAULT 10,
        file_path TEXT NOT NULL,
        index_path TEXT,
        total_thumbnails INTEGER,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_mp_trickplay_account ON mp_trickplay(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_trickplay_job ON mp_trickplay(job_id);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS mp_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL DEFAULT '{}'::jsonb,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_mp_webhook_account ON mp_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_mp_webhook_processed ON mp_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_mp_webhook_created ON mp_webhook_events(created_at DESC);

      -- =====================================================================
      -- Watcher Events (UPGRADE 1c)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_mediap_watcher_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        file_path TEXT NOT NULL,
        file_size BIGINT DEFAULT 0,
        event_type VARCHAR(32) NOT NULL DEFAULT 'detected',
        job_id UUID REFERENCES mp_jobs(id) ON DELETE SET NULL,
        error_message TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_mediap_watcher_account ON np_mediap_watcher_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_mediap_watcher_type ON np_mediap_watcher_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_np_mediap_watcher_created ON np_mediap_watcher_events(created_at DESC);

      -- =====================================================================
      -- Uploads (UPGRADE 1e)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_mediap_uploads (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_id UUID NOT NULL REFERENCES mp_jobs(id) ON DELETE CASCADE,
        file_path TEXT NOT NULL,
        storage_path TEXT NOT NULL,
        storage_url TEXT,
        content_type VARCHAR(128) NOT NULL,
        file_size_bytes BIGINT DEFAULT 0,
        content_id VARCHAR(255),
        version INTEGER DEFAULT 1,
        uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_mediap_uploads_account ON np_mediap_uploads(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_mediap_uploads_job ON np_mediap_uploads(job_id);
      CREATE INDEX IF NOT EXISTS idx_np_mediap_uploads_content ON np_mediap_uploads(content_id);
    `;

    await this.db.execute(schema);

    // Migration: add heartbeat_at and leased_by columns to mp_jobs if missing
    await this.db.execute(`
      DO $$
      BEGIN
        IF NOT EXISTS (
          SELECT 1 FROM information_schema.columns
          WHERE table_name = 'mp_jobs' AND column_name = 'heartbeat_at'
        ) THEN
          ALTER TABLE mp_jobs ADD COLUMN heartbeat_at TIMESTAMP WITH TIME ZONE;
        END IF;
        IF NOT EXISTS (
          SELECT 1 FROM information_schema.columns
          WHERE table_name = 'mp_jobs' AND column_name = 'leased_by'
        ) THEN
          ALTER TABLE mp_jobs ADD COLUMN leased_by VARCHAR(255);
        END IF;
      END $$;
    `);

    logger.info('Schema initialized successfully');
  }

  // =========================================================================
  // Encoding Profiles
  // =========================================================================

  async createEncodingProfile(input: CreateEncodingProfileInput): Promise<EncodingProfileRecord> {
    const resolutions: Resolution[] = input.resolutions ?? [
      { width: 1920, height: 1080, bitrate: 5000000, label: '1080p' },
      { width: 1280, height: 720, bitrate: 2500000, label: '720p' },
      { width: 854, height: 480, bitrate: 1200000, label: '480p' },
      { width: 640, height: 360, bitrate: 800000, label: '360p' },
      { width: 426, height: 240, bitrate: 400000, label: '240p' },
    ];

    const result = await this.db.query<EncodingProfileRecord>(
      `INSERT INTO mp_encoding_profiles (
        source_account_id, name, description, container, video_codec, audio_codec,
        resolutions, audio_bitrate, framerate, preset, hls_enabled, hls_segment_duration,
        trickplay_enabled, trickplay_interval, subtitle_extract, thumbnail_enabled,
        thumbnail_count, is_default
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.name,
        input.description ?? null,
        input.container ?? 'mp4',
        input.video_codec ?? 'h264',
        input.audio_codec ?? 'aac',
        JSON.stringify(resolutions),
        input.audio_bitrate ?? 128000,
        input.framerate ?? 30,
        input.preset ?? 'medium',
        input.hls_enabled ?? true,
        input.hls_segment_duration ?? 6,
        input.trickplay_enabled ?? false,
        input.trickplay_interval ?? 10,
        input.subtitle_extract ?? true,
        input.thumbnail_enabled ?? true,
        input.thumbnail_count ?? 5,
        input.is_default ?? false,
      ]
    );

    if (result.rows.length === 0) {
      throw new Error('Failed to create encoding profile');
    }

    return this.mapEncodingProfile(result.rows[0]);
  }

  async getEncodingProfile(id: string): Promise<EncodingProfileRecord | null> {
    const result = await this.db.query<EncodingProfileRecord>(
      'SELECT * FROM mp_encoding_profiles WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows.length > 0 ? this.mapEncodingProfile(result.rows[0]) : null;
  }

  async listEncodingProfiles(): Promise<EncodingProfileRecord[]> {
    const result = await this.db.query<EncodingProfileRecord>(
      'SELECT * FROM mp_encoding_profiles WHERE source_account_id = $1 ORDER BY is_default DESC, created_at DESC',
      [this.sourceAccountId]
    );

    return result.rows.map(row => this.mapEncodingProfile(row));
  }

  async getDefaultEncodingProfile(): Promise<EncodingProfileRecord | null> {
    const result = await this.db.query<EncodingProfileRecord>(
      'SELECT * FROM mp_encoding_profiles WHERE source_account_id = $1 AND is_default = true LIMIT 1',
      [this.sourceAccountId]
    );

    return result.rows.length > 0 ? this.mapEncodingProfile(result.rows[0]) : null;
  }

  async updateEncodingProfile(input: UpdateEncodingProfileInput): Promise<EncodingProfileRecord> {
    const updates: string[] = [];
    const values: unknown[] = [input.id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      values.push(input.name);
    }
    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      values.push(input.description);
    }
    if (input.container !== undefined) {
      updates.push(`container = $${paramIndex++}`);
      values.push(input.container);
    }
    if (input.video_codec !== undefined) {
      updates.push(`video_codec = $${paramIndex++}`);
      values.push(input.video_codec);
    }
    if (input.audio_codec !== undefined) {
      updates.push(`audio_codec = $${paramIndex++}`);
      values.push(input.audio_codec);
    }
    if (input.resolutions !== undefined) {
      updates.push(`resolutions = $${paramIndex++}`);
      values.push(JSON.stringify(input.resolutions));
    }
    if (input.audio_bitrate !== undefined) {
      updates.push(`audio_bitrate = $${paramIndex++}`);
      values.push(input.audio_bitrate);
    }
    if (input.framerate !== undefined) {
      updates.push(`framerate = $${paramIndex++}`);
      values.push(input.framerate);
    }
    if (input.preset !== undefined) {
      updates.push(`preset = $${paramIndex++}`);
      values.push(input.preset);
    }
    if (input.hls_enabled !== undefined) {
      updates.push(`hls_enabled = $${paramIndex++}`);
      values.push(input.hls_enabled);
    }
    if (input.hls_segment_duration !== undefined) {
      updates.push(`hls_segment_duration = $${paramIndex++}`);
      values.push(input.hls_segment_duration);
    }
    if (input.trickplay_enabled !== undefined) {
      updates.push(`trickplay_enabled = $${paramIndex++}`);
      values.push(input.trickplay_enabled);
    }
    if (input.trickplay_interval !== undefined) {
      updates.push(`trickplay_interval = $${paramIndex++}`);
      values.push(input.trickplay_interval);
    }
    if (input.subtitle_extract !== undefined) {
      updates.push(`subtitle_extract = $${paramIndex++}`);
      values.push(input.subtitle_extract);
    }
    if (input.thumbnail_enabled !== undefined) {
      updates.push(`thumbnail_enabled = $${paramIndex++}`);
      values.push(input.thumbnail_enabled);
    }
    if (input.thumbnail_count !== undefined) {
      updates.push(`thumbnail_count = $${paramIndex++}`);
      values.push(input.thumbnail_count);
    }
    if (input.is_default !== undefined) {
      updates.push(`is_default = $${paramIndex++}`);
      values.push(input.is_default);
    }

    updates.push('updated_at = NOW()');

    const result = await this.db.query<EncodingProfileRecord>(
      `UPDATE mp_encoding_profiles SET ${updates.join(', ')} WHERE id = $1 AND source_account_id = $2 RETURNING *`,
      values
    );

    if (result.rows.length === 0) {
      throw new Error('Encoding profile not found');
    }

    return this.mapEncodingProfile(result.rows[0]);
  }

  async deleteEncodingProfile(id: string): Promise<void> {
    await this.db.execute(
      'DELETE FROM mp_encoding_profiles WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  private mapEncodingProfile(row: Record<string, unknown>): EncodingProfileRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      name: row.name as string,
      description: row.description as string | null,
      container: row.container as 'mp4' | 'mkv' | 'webm' | 'ts',
      video_codec: row.video_codec as 'h264' | 'h265' | 'vp9' | 'av1',
      audio_codec: row.audio_codec as 'aac' | 'opus' | 'mp3',
      resolutions: row.resolutions as Resolution[],
      audio_bitrate: row.audio_bitrate as number,
      framerate: row.framerate as number,
      preset: row.preset as 'ultrafast' | 'superfast' | 'veryfast' | 'faster' | 'fast' | 'medium' | 'slow' | 'slower' | 'veryslow',
      hls_enabled: row.hls_enabled as boolean,
      hls_segment_duration: row.hls_segment_duration as number,
      trickplay_enabled: row.trickplay_enabled as boolean,
      trickplay_interval: row.trickplay_interval as number,
      subtitle_extract: row.subtitle_extract as boolean,
      thumbnail_enabled: row.thumbnail_enabled as boolean,
      thumbnail_count: row.thumbnail_count as number,
      is_default: row.is_default as boolean,
      created_at: new Date(row.created_at as string),
      updated_at: new Date(row.updated_at as string),
    };
  }

  // =========================================================================
  // Jobs
  // =========================================================================

  async createJob(input: CreateJobInput): Promise<JobRecord> {
    const result = await this.db.query<JobRecord>(
      `INSERT INTO mp_jobs (
        source_account_id, input_url, input_type, profile_id, priority, output_base_path
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.input_url,
        input.input_type ?? 'file',
        input.profile_id ?? null,
        input.priority ?? 0,
        input.output_base_path ?? null,
      ]
    );

    if (result.rows.length === 0) {
      throw new Error('Failed to create job');
    }

    return this.mapJob(result.rows[0]);
  }

  async getJob(id: string): Promise<JobRecord | null> {
    const result = await this.db.query<JobRecord>(
      'SELECT * FROM mp_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows.length > 0 ? this.mapJob(result.rows[0]) : null;
  }

  async getJobWithOutputs(id: string): Promise<JobWithOutputs | null> {
    const job = await this.getJob(id);
    if (!job) return null;

    const [outputs, subtitles, hlsManifest, trickplay] = await Promise.all([
      this.getJobOutputs(id),
      this.getJobSubtitles(id),
      this.getHlsManifest(id),
      this.getTrickplay(id),
    ]);

    return {
      ...job,
      outputs,
      subtitles,
      hls_manifest: hlsManifest ?? undefined,
      trickplay: trickplay ?? undefined,
    };
  }

  async listJobs(status?: string, limit = 50, offset = 0): Promise<JobRecord[]> {
    let query = 'SELECT * FROM mp_jobs WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];

    if (status) {
      query += ' AND status = $2';
      params.push(status);
    }

    query += ' ORDER BY priority DESC, created_at DESC LIMIT $' + (params.length + 1) + ' OFFSET $' + (params.length + 2);
    params.push(limit, offset);

    const result = await this.db.query<JobRecord>(query, params);
    return result.rows.map(row => this.mapJob(row));
  }

  async updateJobStatus(id: string, status: JobStatus, progress?: number, error?: string): Promise<void> {
    const updates: string[] = ['status = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, status];
    let paramIndex = 4;

    if (progress !== undefined) {
      updates.push(`progress = $${paramIndex++}`);
      params.push(progress);
    }

    if (error !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(error);
    }

    if (status === 'encoding' || status === 'downloading') {
      updates.push('started_at = COALESCE(started_at, NOW())');
    }

    if (status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'qa_failed') {
      updates.push('completed_at = NOW()');
    }

    await this.db.execute(
      `UPDATE mp_jobs SET ${updates.join(', ')} WHERE id = $1 AND source_account_id = $2`,
      params
    );
  }

  async updateJobMetadata(id: string, metadata: Record<string, unknown>): Promise<void> {
    await this.db.execute(
      'UPDATE mp_jobs SET input_metadata = $3, updated_at = NOW() WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId, JSON.stringify(metadata)]
    );
  }

  async cancelJob(id: string): Promise<void> {
    await this.updateJobStatus(id, 'cancelled');
  }

  private mapJob(row: Record<string, unknown>): JobRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      input_url: row.input_url as string,
      input_type: row.input_type as 'file' | 'url' | 's3',
      profile_id: row.profile_id as string | null,
      status: row.status as JobStatus,
      priority: row.priority as number,
      progress: row.progress as number,
      input_metadata: row.input_metadata as Record<string, unknown>,
      output_base_path: row.output_base_path as string | null,
      error_message: row.error_message as string | null,
      duration_seconds: row.duration_seconds as number | null,
      file_size_bytes: row.file_size_bytes as number | null,
      started_at: row.started_at ? new Date(row.started_at as string) : null,
      completed_at: row.completed_at ? new Date(row.completed_at as string) : null,
      created_at: new Date(row.created_at as string),
      updated_at: new Date(row.updated_at as string),
    };
  }

  // =========================================================================
  // Job Outputs
  // =========================================================================

  async createJobOutput(output: Omit<JobOutputRecord, 'id' | 'source_account_id' | 'created_at'>): Promise<JobOutputRecord> {
    const result = await this.db.query<JobOutputRecord>(
      `INSERT INTO mp_job_outputs (
        source_account_id, job_id, output_type, resolution_label, file_path,
        file_size_bytes, content_type, width, height, bitrate, duration_seconds, language, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
      RETURNING *`,
      [
        this.sourceAccountId,
        output.job_id,
        output.output_type,
        output.resolution_label ?? null,
        output.file_path,
        output.file_size_bytes ?? null,
        output.content_type ?? null,
        output.width ?? null,
        output.height ?? null,
        output.bitrate ?? null,
        output.duration_seconds ?? null,
        output.language ?? null,
        JSON.stringify(output.metadata),
      ]
    );

    return this.mapJobOutput(result.rows[0]);
  }

  async getJobOutputs(jobId: string): Promise<JobOutputRecord[]> {
    const result = await this.db.query<JobOutputRecord>(
      'SELECT * FROM mp_job_outputs WHERE job_id = $1 AND source_account_id = $2 ORDER BY created_at',
      [jobId, this.sourceAccountId]
    );

    return result.rows.map(row => this.mapJobOutput(row));
  }

  private mapJobOutput(row: Record<string, unknown>): JobOutputRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      job_id: row.job_id as string,
      output_type: row.output_type as OutputType,
      resolution_label: row.resolution_label as string | null,
      file_path: row.file_path as string,
      file_size_bytes: row.file_size_bytes as number | null,
      content_type: row.content_type as string | null,
      width: row.width as number | null,
      height: row.height as number | null,
      bitrate: row.bitrate as number | null,
      duration_seconds: row.duration_seconds as number | null,
      language: row.language as string | null,
      metadata: row.metadata as Record<string, unknown>,
      created_at: new Date(row.created_at as string),
    };
  }

  // =========================================================================
  // HLS Manifests
  // =========================================================================

  async createHlsManifest(manifest: Omit<HlsManifestRecord, 'id' | 'source_account_id' | 'created_at'>): Promise<HlsManifestRecord> {
    const result = await this.db.query<HlsManifestRecord>(
      `INSERT INTO mp_hls_manifests (
        source_account_id, job_id, master_manifest_path, variant_manifests, segment_count, total_duration_seconds
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId,
        manifest.job_id,
        manifest.master_manifest_path,
        JSON.stringify(manifest.variant_manifests),
        manifest.segment_count,
        manifest.total_duration_seconds ?? null,
      ]
    );

    return this.mapHlsManifest(result.rows[0]);
  }

  async getHlsManifest(jobId: string): Promise<HlsManifestRecord | null> {
    const result = await this.db.query<HlsManifestRecord>(
      'SELECT * FROM mp_hls_manifests WHERE job_id = $1 AND source_account_id = $2',
      [jobId, this.sourceAccountId]
    );

    return result.rows.length > 0 ? this.mapHlsManifest(result.rows[0]) : null;
  }

  private mapHlsManifest(row: Record<string, unknown>): HlsManifestRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      job_id: row.job_id as string,
      master_manifest_path: row.master_manifest_path as string,
      variant_manifests: row.variant_manifests as VariantManifest[],
      segment_count: row.segment_count as number,
      total_duration_seconds: row.total_duration_seconds as number | null,
      created_at: new Date(row.created_at as string),
    };
  }

  // =========================================================================
  // Subtitles
  // =========================================================================

  async createSubtitle(subtitle: Omit<SubtitleRecord, 'id' | 'source_account_id' | 'created_at'>): Promise<SubtitleRecord> {
    const result = await this.db.query<SubtitleRecord>(
      `INSERT INTO mp_subtitles (
        source_account_id, job_id, language, label, format, file_path, is_default, is_forced
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        subtitle.job_id,
        subtitle.language,
        subtitle.label ?? null,
        subtitle.format,
        subtitle.file_path,
        subtitle.is_default,
        subtitle.is_forced,
      ]
    );

    return this.mapSubtitle(result.rows[0]);
  }

  async getJobSubtitles(jobId: string): Promise<SubtitleRecord[]> {
    const result = await this.db.query<SubtitleRecord>(
      'SELECT * FROM mp_subtitles WHERE job_id = $1 AND source_account_id = $2 ORDER BY is_default DESC, language',
      [jobId, this.sourceAccountId]
    );

    return result.rows.map(row => this.mapSubtitle(row));
  }

  private mapSubtitle(row: Record<string, unknown>): SubtitleRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      job_id: row.job_id as string,
      language: row.language as string,
      label: row.label as string | null,
      format: row.format as 'vtt' | 'srt' | 'ass',
      file_path: row.file_path as string,
      is_default: row.is_default as boolean,
      is_forced: row.is_forced as boolean,
      created_at: new Date(row.created_at as string),
    };
  }

  // =========================================================================
  // Trickplay
  // =========================================================================

  async createTrickplay(trickplay: Omit<TrickplayRecord, 'id' | 'source_account_id' | 'created_at'>): Promise<TrickplayRecord> {
    const result = await this.db.query<TrickplayRecord>(
      `INSERT INTO mp_trickplay (
        source_account_id, job_id, tile_width, tile_height, columns, rows,
        interval_seconds, file_path, index_path, total_thumbnails
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        trickplay.job_id,
        trickplay.tile_width,
        trickplay.tile_height,
        trickplay.columns,
        trickplay.rows,
        trickplay.interval_seconds,
        trickplay.file_path,
        trickplay.index_path ?? null,
        trickplay.total_thumbnails ?? null,
      ]
    );

    return this.mapTrickplay(result.rows[0]);
  }

  async getTrickplay(jobId: string): Promise<TrickplayRecord | null> {
    const result = await this.db.query<TrickplayRecord>(
      'SELECT * FROM mp_trickplay WHERE job_id = $1 AND source_account_id = $2',
      [jobId, this.sourceAccountId]
    );

    return result.rows.length > 0 ? this.mapTrickplay(result.rows[0]) : null;
  }

  private mapTrickplay(row: Record<string, unknown>): TrickplayRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      job_id: row.job_id as string,
      tile_width: row.tile_width as number,
      tile_height: row.tile_height as number,
      columns: row.columns as number,
      rows: row.rows as number,
      interval_seconds: row.interval_seconds as number,
      file_path: row.file_path as string,
      index_path: row.index_path as string | null,
      total_thumbnails: row.total_thumbnails as number | null,
      created_at: new Date(row.created_at as string),
    };
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(id: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.db.execute(
      `INSERT INTO mp_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (id) DO NOTHING`,
      [id, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.db.execute(
      `UPDATE mp_webhook_events
       SET processed = true, processed_at = NOW(), error = $3
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId, error ?? null]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<ProcessingStats> {
    const result = await this.db.query<{
      total_jobs: string;
      pending_jobs: string;
      running_jobs: string;
      completed_jobs: string;
      failed_jobs: string;
      total_duration_seconds: string | null;
      total_file_size_bytes: string | null;
      profiles: string;
      avg_processing_time: string | null;
      last_job_completed: string | null;
    }>(
      `SELECT
        COUNT(*)::text as total_jobs,
        COUNT(*) FILTER (WHERE status = 'pending')::text as pending_jobs,
        COUNT(*) FILTER (WHERE status IN ('downloading', 'analyzing', 'encoding', 'packaging', 'uploading'))::text as running_jobs,
        COUNT(*) FILTER (WHERE status = 'completed')::text as completed_jobs,
        COUNT(*) FILTER (WHERE status = 'failed')::text as failed_jobs,
        SUM(duration_seconds)::text as total_duration_seconds,
        SUM(file_size_bytes)::text as total_file_size_bytes,
        (SELECT COUNT(*)::text FROM mp_encoding_profiles WHERE source_account_id = $1) as profiles,
        AVG(EXTRACT(EPOCH FROM (completed_at - started_at)))::text as avg_processing_time,
        MAX(completed_at)::text as last_job_completed
       FROM mp_jobs
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      totalJobs: parseInt(row.total_jobs, 10),
      pendingJobs: parseInt(row.pending_jobs, 10),
      runningJobs: parseInt(row.running_jobs, 10),
      completedJobs: parseInt(row.completed_jobs, 10),
      failedJobs: parseInt(row.failed_jobs, 10),
      totalDurationSeconds: parseFloat(row.total_duration_seconds ?? '0'),
      totalFileSizeBytes: parseInt(row.total_file_size_bytes ?? '0', 10),
      profiles: parseInt(row.profiles, 10),
      averageProcessingTimeSeconds: row.avg_processing_time ? parseFloat(row.avg_processing_time) : null,
      lastJobCompletedAt: row.last_job_completed ? new Date(row.last_job_completed) : null,
    };
  }

  // =========================================================================
  // Job Leasing (UPGRADE 1g)
  // =========================================================================

  async leaseNextJob(workerId: string): Promise<LeasedJob | null> {
    const result = await this.db.query<LeasedJob>(
      `UPDATE mp_jobs SET
        leased_by = $2,
        heartbeat_at = NOW(),
        updated_at = NOW()
      WHERE id = (
        SELECT id FROM mp_jobs
        WHERE source_account_id = $1
          AND status = 'pending'
          AND leased_by IS NULL
        ORDER BY priority DESC, created_at ASC
        FOR UPDATE SKIP LOCKED
        LIMIT 1
      )
      RETURNING *`,
      [this.sourceAccountId, workerId]
    );

    return result.rows.length > 0 ? this.mapLeasedJob(result.rows[0]) : null;
  }

  async heartbeatJob(jobId: string, workerId: string): Promise<void> {
    await this.db.execute(
      `UPDATE mp_jobs SET heartbeat_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND leased_by = $3`,
      [jobId, this.sourceAccountId, workerId]
    );
  }

  async reclaimStaleJobs(timeoutMinutes: number): Promise<number> {
    const result = await this.db.query<{ id: string }>(
      `UPDATE mp_jobs SET
        status = 'failed',
        error_message = 'Stale heartbeat - worker did not respond within ' || $3 || ' minutes',
        leased_by = NULL,
        heartbeat_at = NULL,
        completed_at = NOW(),
        updated_at = NOW()
      WHERE source_account_id = $1
        AND leased_by IS NOT NULL
        AND status NOT IN ('completed', 'failed', 'cancelled', 'qa_failed')
        AND heartbeat_at < NOW() - ($3 || ' minutes')::interval
      RETURNING id`,
      [this.sourceAccountId, undefined, timeoutMinutes.toString()]
    );

    if (result.rows.length > 0) {
      logger.warn('Reclaimed stale jobs', { count: result.rows.length, jobIds: result.rows.map(r => r.id) });
    }

    return result.rows.length;
  }

  async releaseJobLease(jobId: string): Promise<void> {
    await this.db.execute(
      `UPDATE mp_jobs SET leased_by = NULL, heartbeat_at = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [jobId, this.sourceAccountId]
    );
  }

  private mapLeasedJob(row: Record<string, unknown>): LeasedJob {
    return {
      ...this.mapJob(row),
      leased_by: row.leased_by as string | null,
      heartbeat_at: row.heartbeat_at ? new Date(row.heartbeat_at as string) : null,
    };
  }

  // =========================================================================
  // Watcher Events (UPGRADE 1c)
  // =========================================================================

  async createWatcherEvent(event: Omit<DropFolderEvent, 'id' | 'source_account_id' | 'created_at'>): Promise<DropFolderEvent> {
    const result = await this.db.query<DropFolderEvent>(
      `INSERT INTO np_mediap_watcher_events (
        source_account_id, file_path, file_size, event_type, job_id, error_message
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId,
        event.file_path,
        event.file_size,
        event.event_type,
        event.job_id ?? null,
        event.error_message ?? null,
      ]
    );

    return this.mapWatcherEvent(result.rows[0]);
  }

  async listWatcherEvents(limit = 50): Promise<DropFolderEvent[]> {
    const result = await this.db.query<DropFolderEvent>(
      `SELECT * FROM np_mediap_watcher_events
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );

    return result.rows.map(row => this.mapWatcherEvent(row));
  }

  private mapWatcherEvent(row: Record<string, unknown>): DropFolderEvent {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      file_path: row.file_path as string,
      file_size: typeof row.file_size === 'string' ? parseInt(row.file_size, 10) : (row.file_size as number),
      event_type: row.event_type as DropFolderEvent['event_type'],
      job_id: row.job_id as string | null,
      error_message: row.error_message as string | null,
      created_at: new Date(row.created_at as string),
    };
  }

  // =========================================================================
  // Uploads (UPGRADE 1e)
  // =========================================================================

  async createUploadRecord(upload: Omit<UploadRecord, 'id' | 'source_account_id' | 'uploaded_at'>): Promise<UploadRecord> {
    const result = await this.db.query<UploadRecord>(
      `INSERT INTO np_mediap_uploads (
        source_account_id, job_id, file_path, storage_path, storage_url,
        content_type, file_size_bytes, content_id, version
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId,
        upload.job_id,
        upload.file_path,
        upload.storage_path,
        upload.storage_url ?? null,
        upload.content_type,
        upload.file_size_bytes,
        upload.content_id ?? null,
        upload.version,
      ]
    );

    return this.mapUploadRecord(result.rows[0]);
  }

  async getJobUploads(jobId: string): Promise<UploadRecord[]> {
    const result = await this.db.query<UploadRecord>(
      `SELECT * FROM np_mediap_uploads
       WHERE job_id = $1 AND source_account_id = $2
       ORDER BY uploaded_at`,
      [jobId, this.sourceAccountId]
    );

    return result.rows.map(row => this.mapUploadRecord(row));
  }

  async getNextUploadVersion(contentId: string): Promise<number> {
    const result = await this.db.query<{ max_version: string | null }>(
      `SELECT MAX(version)::text as max_version FROM np_mediap_uploads
       WHERE content_id = $1 AND source_account_id = $2`,
      [contentId, this.sourceAccountId]
    );

    const maxVersion = result.rows[0]?.max_version;
    return maxVersion ? parseInt(maxVersion, 10) + 1 : 1;
  }

  private mapUploadRecord(row: Record<string, unknown>): UploadRecord {
    return {
      id: row.id as string,
      source_account_id: row.source_account_id as string,
      job_id: row.job_id as string,
      file_path: row.file_path as string,
      storage_path: row.storage_path as string,
      storage_url: row.storage_url as string | null,
      content_type: row.content_type as string,
      file_size_bytes: typeof row.file_size_bytes === 'string' ? parseInt(row.file_size_bytes, 10) : (row.file_size_bytes as number),
      content_id: row.content_id as string | null,
      version: row.version as number,
      uploaded_at: new Date(row.uploaded_at as string),
    };
  }
}
