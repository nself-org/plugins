/**
 * Recording Database Operations
 * Complete CRUD operations for recordings, schedules, and encode jobs
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  RecordingRecord,
  ScheduleRecord,
  EncodeJobRecord,
  RecordingStatus,
  PublishStatus,
  EncodeStatus,
  CreateRecordingRequest,
  UpdateRecordingRequest,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  RecordingStats,
} from './types.js';

const logger = createLogger('recording:db');

export class RecordingDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(sourceAccountId: string): RecordingDatabase {
    return new RecordingDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing recording schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Recordings
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_rec_recordings (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        title VARCHAR(512) NOT NULL,
        description TEXT,
        source_type VARCHAR(32) NOT NULL,
        source_id VARCHAR(255),
        source_channel VARCHAR(128),
        source_device_id VARCHAR(255),
        status VARCHAR(32) NOT NULL DEFAULT 'scheduled',
        priority VARCHAR(16) DEFAULT 'normal',
        scheduled_start TIMESTAMPTZ NOT NULL,
        scheduled_end TIMESTAMPTZ NOT NULL,
        actual_start TIMESTAMPTZ,
        actual_end TIMESTAMPTZ,
        duration_seconds INTEGER,
        file_path TEXT,
        file_size BIGINT,
        file_format VARCHAR(16),
        thumbnail_url TEXT,
        encode_status VARCHAR(32),
        encode_progress FLOAT DEFAULT 0,
        encode_started_at TIMESTAMPTZ,
        encode_completed_at TIMESTAMPTZ,
        publish_status VARCHAR(32) DEFAULT 'unpublished',
        published_at TIMESTAMPTZ,
        storage_object_id VARCHAR(255),
        sports_event_id VARCHAR(255),
        media_metadata_id VARCHAR(255),
        enrichment_status VARCHAR(32) DEFAULT 'pending',
        tags JSONB DEFAULT '[]',
        category VARCHAR(128),
        content_rating VARCHAR(16),
        commercial_markers JSONB,
        custom_fields JSONB DEFAULT '{}',
        metadata JSONB DEFAULT '{}',
        created_by VARCHAR(255),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        deleted_at TIMESTAMPTZ
      );

      CREATE INDEX IF NOT EXISTS idx_rec_source_account
        ON np_rec_recordings(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_rec_app
        ON np_rec_recordings(app_id);
      CREATE INDEX IF NOT EXISTS idx_rec_status
        ON np_rec_recordings(status);
      CREATE INDEX IF NOT EXISTS idx_rec_scheduled
        ON np_rec_recordings(scheduled_start);
      CREATE INDEX IF NOT EXISTS idx_rec_source
        ON np_rec_recordings(source_type, source_id);
      CREATE INDEX IF NOT EXISTS idx_rec_device
        ON np_rec_recordings(source_device_id);
      CREATE INDEX IF NOT EXISTS idx_rec_sports
        ON np_rec_recordings(sports_event_id);
      CREATE INDEX IF NOT EXISTS idx_rec_published
        ON np_rec_recordings(publish_status, published_at);
      CREATE INDEX IF NOT EXISTS idx_rec_tags
        ON np_rec_recordings USING GIN(tags);
      CREATE INDEX IF NOT EXISTS idx_rec_category
        ON np_rec_recordings(category);

      -- =====================================================================
      -- Schedules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_rec_schedules (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        name VARCHAR(255) NOT NULL,
        schedule_type VARCHAR(32) NOT NULL,
        source_channel VARCHAR(128),
        source_device_id VARCHAR(255),
        recurrence_rule VARCHAR(255),
        duration_minutes INTEGER NOT NULL,
        lead_time_minutes INTEGER DEFAULT 5,
        trail_time_minutes INTEGER DEFAULT 15,
        sports_league VARCHAR(64),
        sports_team_id VARCHAR(255),
        auto_enrich BOOLEAN DEFAULT TRUE,
        auto_publish BOOLEAN DEFAULT FALSE,
        priority VARCHAR(16) DEFAULT 'normal',
        active BOOLEAN DEFAULT TRUE,
        last_triggered_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_rec_schedules_source_account
        ON np_rec_schedules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_rec_schedules_app
        ON np_rec_schedules(app_id);
      CREATE INDEX IF NOT EXISTS idx_np_rec_schedules_type
        ON np_rec_schedules(schedule_type);
      CREATE INDEX IF NOT EXISTS idx_np_rec_schedules_active
        ON np_rec_schedules(active) WHERE active = TRUE;

      -- =====================================================================
      -- Encode Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_rec_encode_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        recording_id UUID NOT NULL REFERENCES np_rec_recordings(id) ON DELETE CASCADE,
        profile VARCHAR(64) NOT NULL,
        status VARCHAR(32) NOT NULL DEFAULT 'pending',
        input_path TEXT NOT NULL,
        output_path TEXT,
        output_size BIGINT,
        progress FLOAT DEFAULT 0,
        settings JSONB DEFAULT '{}',
        started_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_rec_encode_source_account
        ON np_rec_encode_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_rec_encode_recording
        ON np_rec_encode_jobs(recording_id);
      CREATE INDEX IF NOT EXISTS idx_rec_encode_status
        ON np_rec_encode_jobs(status);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

      CREATE OR REPLACE VIEW np_rec_recordings_by_status AS
      SELECT source_account_id, app_id, status, publish_status,
             COUNT(*) AS recording_count,
             COALESCE(SUM(duration_seconds), 0) / 3600.0 AS total_hours,
             COALESCE(SUM(file_size), 0) / (1024.0 * 1024 * 1024) AS total_gb
      FROM np_rec_recordings
      WHERE deleted_at IS NULL
      GROUP BY source_account_id, app_id, status, publish_status
      ORDER BY source_account_id, app_id, status;

      CREATE OR REPLACE VIEW rec_storage_by_type AS
      SELECT source_account_id, app_id, source_type,
             COUNT(*) AS recording_count,
             COALESCE(SUM(file_size), 0) / (1024.0 * 1024 * 1024) AS storage_gb,
             COALESCE(AVG(duration_seconds), 0) / 60.0 AS avg_duration_minutes
      FROM np_rec_recordings
      WHERE deleted_at IS NULL AND file_size IS NOT NULL
      GROUP BY source_account_id, app_id, source_type
      ORDER BY storage_gb DESC;

      CREATE OR REPLACE VIEW rec_success_rate AS
      SELECT source_account_id, app_id,
             COUNT(*) AS total_scheduled,
             COUNT(*) FILTER (WHERE status = 'published') AS completed,
             COUNT(*) FILTER (WHERE status = 'failed') AS failed,
             COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled,
             ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'published') / NULLIF(COUNT(*), 0), 2) AS success_rate
      FROM np_rec_recordings
      WHERE deleted_at IS NULL
      GROUP BY source_account_id, app_id;

      CREATE OR REPLACE VIEW rec_scheduled_vs_completed AS
      SELECT source_account_id, app_id,
             DATE_TRUNC('day', scheduled_start) AS day,
             COUNT(*) AS scheduled,
             COUNT(*) FILTER (WHERE status = 'published') AS completed,
             COUNT(*) FILTER (WHERE status = 'failed') AS failed
      FROM np_rec_recordings
      WHERE deleted_at IS NULL
        AND scheduled_start >= NOW() - INTERVAL '30 days'
      GROUP BY source_account_id, app_id, DATE_TRUNC('day', scheduled_start)
      ORDER BY day DESC;
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Recordings
  // =========================================================================

  async createRecording(appId: string, request: CreateRecordingRequest): Promise<RecordingRecord> {
    const result = await this.query<RecordingRecord>(
      `INSERT INTO np_rec_recordings (
        source_account_id, app_id, title, description, source_type,
        source_id, source_channel, source_device_id, priority,
        scheduled_start, scheduled_end, sports_event_id,
        tags, category, content_rating, custom_fields, metadata, created_by
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        request.title,
        request.description ?? null,
        request.source_type,
        request.source_id ?? null,
        request.source_channel ?? null,
        request.source_device_id ?? null,
        request.priority ?? 'normal',
        request.scheduled_start,
        request.scheduled_end,
        request.sports_event_id ?? null,
        JSON.stringify(request.tags ?? []),
        request.category ?? null,
        request.content_rating ?? null,
        JSON.stringify(request.custom_fields ?? {}),
        JSON.stringify(request.metadata ?? {}),
        request.created_by ?? null,
      ]
    );

    return result.rows[0];
  }

  async getRecording(recordingId: string): Promise<RecordingRecord | null> {
    const result = await this.query<RecordingRecord>(
      `SELECT * FROM np_rec_recordings
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL`,
      [this.sourceAccountId, recordingId]
    );
    return result.rows[0] ?? null;
  }

  async listRecordings(
    appId?: string,
    status?: RecordingStatus,
    publishStatus?: PublishStatus,
    category?: string,
    limit = 100,
    offset = 0
  ): Promise<RecordingRecord[]> {
    const conditions = ['source_account_id = $1', 'deleted_at IS NULL'];
    const params: unknown[] = [this.sourceAccountId];

    if (appId) {
      conditions.push(`app_id = $${params.length + 1}`);
      params.push(appId);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    if (publishStatus) {
      conditions.push(`publish_status = $${params.length + 1}`);
      params.push(publishStatus);
    }

    if (category) {
      conditions.push(`category = $${params.length + 1}`);
      params.push(category);
    }

    params.push(limit, offset);

    const result = await this.query<RecordingRecord>(
      `SELECT * FROM np_rec_recordings
       WHERE ${conditions.join(' AND ')}
       ORDER BY scheduled_start DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async updateRecording(recordingId: string, updates: UpdateRecordingRequest): Promise<RecordingRecord | null> {
    const setParts: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, recordingId];

    if (updates.title !== undefined) {
      setParts.push(`title = $${params.length + 1}`);
      params.push(updates.title);
    }

    if (updates.description !== undefined) {
      setParts.push(`description = $${params.length + 1}`);
      params.push(updates.description);
    }

    if (updates.scheduled_start !== undefined) {
      setParts.push(`scheduled_start = $${params.length + 1}`);
      params.push(updates.scheduled_start);
    }

    if (updates.scheduled_end !== undefined) {
      setParts.push(`scheduled_end = $${params.length + 1}`);
      params.push(updates.scheduled_end);
    }

    if (updates.priority !== undefined) {
      setParts.push(`priority = $${params.length + 1}`);
      params.push(updates.priority);
    }

    if (updates.tags !== undefined) {
      setParts.push(`tags = $${params.length + 1}`);
      params.push(JSON.stringify(updates.tags));
    }

    if (updates.category !== undefined) {
      setParts.push(`category = $${params.length + 1}`);
      params.push(updates.category);
    }

    if (updates.content_rating !== undefined) {
      setParts.push(`content_rating = $${params.length + 1}`);
      params.push(updates.content_rating);
    }

    if (updates.custom_fields !== undefined) {
      setParts.push(`custom_fields = $${params.length + 1}`);
      params.push(JSON.stringify(updates.custom_fields));
    }

    if (updates.metadata !== undefined) {
      setParts.push(`metadata = $${params.length + 1}`);
      params.push(JSON.stringify(updates.metadata));
    }

    const result = await this.query<RecordingRecord>(
      `UPDATE np_rec_recordings
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteRecording(recordingId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `UPDATE np_rec_recordings
       SET deleted_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL`,
      [this.sourceAccountId, recordingId]
    );
    return rowCount > 0;
  }

  async updateRecordingStatus(recordingId: string, status: RecordingStatus, extra?: Record<string, unknown>): Promise<RecordingRecord | null> {
    const setParts: string[] = [`status = $3`, 'updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, recordingId, status];

    if (status === 'recording') {
      setParts.push('actual_start = COALESCE(actual_start, NOW())');
    } else if (status === 'finalizing' || status === 'encoding') {
      setParts.push('actual_end = COALESCE(actual_end, NOW())');
      setParts.push('duration_seconds = EXTRACT(EPOCH FROM (COALESCE(actual_end, NOW()) - actual_start))::INTEGER');
    }

    if (extra) {
      for (const [key, value] of Object.entries(extra)) {
        setParts.push(`${key} = $${params.length + 1}`);
        params.push(typeof value === 'object' ? JSON.stringify(value) : value);
      }
    }

    const result = await this.query<RecordingRecord>(
      `UPDATE np_rec_recordings
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async cancelRecording(recordingId: string): Promise<RecordingRecord | null> {
    const result = await this.query<RecordingRecord>(
      `UPDATE np_rec_recordings
       SET status = 'cancelled', updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
         AND status IN ('scheduled', 'starting')
         AND deleted_at IS NULL
       RETURNING *`,
      [this.sourceAccountId, recordingId]
    );
    return result.rows[0] ?? null;
  }

  async publishRecording(recordingId: string): Promise<RecordingRecord | null> {
    const result = await this.query<RecordingRecord>(
      `UPDATE np_rec_recordings
       SET publish_status = 'published', published_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [this.sourceAccountId, recordingId]
    );
    return result.rows[0] ?? null;
  }

  async unpublishRecording(recordingId: string): Promise<RecordingRecord | null> {
    const result = await this.query<RecordingRecord>(
      `UPDATE np_rec_recordings
       SET publish_status = 'unpublished', published_at = NULL, updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [this.sourceAccountId, recordingId]
    );
    return result.rows[0] ?? null;
  }

  async finalizeRecording(recordingId: string): Promise<RecordingRecord | null> {
    // Idempotent: only transitions from 'recording' or 'finalizing'; already 'processing'/'ready' is a no-op success
    const existing = await this.getRecording(recordingId);
    if (!existing) return null;

    if (existing.status === 'processing' || existing.status === 'published') {
      // Already finalized or beyond -- return current state for idempotency
      return existing;
    }

    const result = await this.query<RecordingRecord>(
      `UPDATE np_rec_recordings
       SET status = 'processing',
           actual_end = COALESCE(actual_end, NOW()),
           duration_seconds = EXTRACT(EPOCH FROM (COALESCE(actual_end, NOW()) - actual_start))::INTEGER,
           updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
         AND status IN ('recording', 'finalizing')
         AND deleted_at IS NULL
       RETURNING *`,
      [this.sourceAccountId, recordingId]
    );

    return result.rows[0] ?? existing;
  }

  async getPublishedRecordings(appId?: string, category?: string, limit = 100, offset = 0): Promise<RecordingRecord[]> {
    return this.listRecordings(appId, undefined, 'published', category, limit, offset);
  }

  // =========================================================================
  // Schedules
  // =========================================================================

  async createSchedule(appId: string, request: CreateScheduleRequest): Promise<ScheduleRecord> {
    const result = await this.query<ScheduleRecord>(
      `INSERT INTO np_rec_schedules (
        source_account_id, app_id, name, schedule_type,
        source_channel, source_device_id, recurrence_rule,
        duration_minutes, lead_time_minutes, trail_time_minutes,
        sports_league, sports_team_id, auto_enrich, auto_publish,
        priority, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        request.name,
        request.schedule_type,
        request.source_channel ?? null,
        request.source_device_id ?? null,
        request.recurrence_rule ?? null,
        request.duration_minutes,
        request.lead_time_minutes ?? 5,
        request.trail_time_minutes ?? 15,
        request.sports_league ?? null,
        request.sports_team_id ?? null,
        request.auto_enrich ?? true,
        request.auto_publish ?? false,
        request.priority ?? 'normal',
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getSchedule(scheduleId: string): Promise<ScheduleRecord | null> {
    const result = await this.query<ScheduleRecord>(
      `SELECT * FROM np_rec_schedules
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, scheduleId]
    );
    return result.rows[0] ?? null;
  }

  async listSchedules(appId?: string, activeOnly = false, limit = 100, offset = 0): Promise<ScheduleRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (appId) {
      conditions.push(`app_id = $${params.length + 1}`);
      params.push(appId);
    }

    if (activeOnly) {
      conditions.push('active = TRUE');
    }

    params.push(limit, offset);

    const result = await this.query<ScheduleRecord>(
      `SELECT * FROM np_rec_schedules
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async updateSchedule(scheduleId: string, updates: UpdateScheduleRequest): Promise<ScheduleRecord | null> {
    const setParts: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, scheduleId];

    if (updates.name !== undefined) {
      setParts.push(`name = $${params.length + 1}`);
      params.push(updates.name);
    }

    if (updates.source_channel !== undefined) {
      setParts.push(`source_channel = $${params.length + 1}`);
      params.push(updates.source_channel);
    }

    if (updates.source_device_id !== undefined) {
      setParts.push(`source_device_id = $${params.length + 1}`);
      params.push(updates.source_device_id);
    }

    if (updates.recurrence_rule !== undefined) {
      setParts.push(`recurrence_rule = $${params.length + 1}`);
      params.push(updates.recurrence_rule);
    }

    if (updates.duration_minutes !== undefined) {
      setParts.push(`duration_minutes = $${params.length + 1}`);
      params.push(updates.duration_minutes);
    }

    if (updates.lead_time_minutes !== undefined) {
      setParts.push(`lead_time_minutes = $${params.length + 1}`);
      params.push(updates.lead_time_minutes);
    }

    if (updates.trail_time_minutes !== undefined) {
      setParts.push(`trail_time_minutes = $${params.length + 1}`);
      params.push(updates.trail_time_minutes);
    }

    if (updates.auto_enrich !== undefined) {
      setParts.push(`auto_enrich = $${params.length + 1}`);
      params.push(updates.auto_enrich);
    }

    if (updates.auto_publish !== undefined) {
      setParts.push(`auto_publish = $${params.length + 1}`);
      params.push(updates.auto_publish);
    }

    if (updates.priority !== undefined) {
      setParts.push(`priority = $${params.length + 1}`);
      params.push(updates.priority);
    }

    if (updates.active !== undefined) {
      setParts.push(`active = $${params.length + 1}`);
      params.push(updates.active);
    }

    if (updates.metadata !== undefined) {
      setParts.push(`metadata = $${params.length + 1}`);
      params.push(JSON.stringify(updates.metadata));
    }

    const result = await this.query<ScheduleRecord>(
      `UPDATE np_rec_schedules
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteSchedule(scheduleId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM np_rec_schedules
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, scheduleId]
    );
    return rowCount > 0;
  }

  async markScheduleTriggered(scheduleId: string): Promise<void> {
    await this.execute(
      `UPDATE np_rec_schedules
       SET last_triggered_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, scheduleId]
    );
  }

  // =========================================================================
  // Encode Jobs
  // =========================================================================

  async createEncodeJob(recordingId: string, profile: string, inputPath: string, settings?: Record<string, unknown>): Promise<EncodeJobRecord> {
    const result = await this.query<EncodeJobRecord>(
      `INSERT INTO np_rec_encode_jobs (
        source_account_id, recording_id, profile, input_path, settings
      ) VALUES ($1, $2, $3, $4, $5)
      RETURNING *`,
      [
        this.sourceAccountId,
        recordingId,
        profile,
        inputPath,
        JSON.stringify(settings ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getEncodeJob(jobId: string): Promise<EncodeJobRecord | null> {
    const result = await this.query<EncodeJobRecord>(
      `SELECT * FROM np_rec_encode_jobs
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, jobId]
    );
    return result.rows[0] ?? null;
  }

  async listEncodeJobs(recordingId?: string, status?: EncodeStatus, limit = 100, offset = 0): Promise<EncodeJobRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (recordingId) {
      conditions.push(`recording_id = $${params.length + 1}`);
      params.push(recordingId);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.query<EncodeJobRecord>(
      `SELECT * FROM np_rec_encode_jobs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async updateEncodeJobStatus(
    jobId: string,
    status: EncodeStatus,
    extra?: { progress?: number; output_path?: string; output_size?: number; error?: string }
  ): Promise<EncodeJobRecord | null> {
    const setParts: string[] = [`status = $3`];
    const params: unknown[] = [this.sourceAccountId, jobId, status];

    if (status === 'running') {
      setParts.push('started_at = COALESCE(started_at, NOW())');
    } else if (status === 'completed' || status === 'failed') {
      setParts.push('completed_at = NOW()');
    }

    if (extra?.progress !== undefined) {
      setParts.push(`progress = $${params.length + 1}`);
      params.push(extra.progress);
    }

    if (extra?.output_path !== undefined) {
      setParts.push(`output_path = $${params.length + 1}`);
      params.push(extra.output_path);
    }

    if (extra?.output_size !== undefined) {
      setParts.push(`output_size = $${params.length + 1}`);
      params.push(extra.output_size);
    }

    if (extra?.error !== undefined) {
      setParts.push(`error = $${params.length + 1}`);
      params.push(extra.error);
    }

    const result = await this.query<EncodeJobRecord>(
      `UPDATE np_rec_encode_jobs
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async getEncodeJobsForRecording(recordingId: string): Promise<EncodeJobRecord[]> {
    const result = await this.query<EncodeJobRecord>(
      `SELECT * FROM np_rec_encode_jobs
       WHERE source_account_id = $1 AND recording_id = $2
       ORDER BY created_at ASC`,
      [this.sourceAccountId, recordingId]
    );
    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getRecordingStats(): Promise<RecordingStats> {
    const result = await this.query<{
      total_recordings: number;
      scheduled: number;
      recording_now: number;
      encoding: number;
      published: number;
      failed: number;
      cancelled: number;
      total_storage_gb: number;
      total_duration_hours: number;
      last_activity: Date | null;
    }>(
      `SELECT
        COUNT(*) AS total_recordings,
        COUNT(*) FILTER (WHERE status = 'scheduled') AS scheduled,
        COUNT(*) FILTER (WHERE status = 'recording') AS recording_now,
        COUNT(*) FILTER (WHERE status = 'encoding') AS encoding,
        COUNT(*) FILTER (WHERE status = 'published') AS published,
        COUNT(*) FILTER (WHERE status = 'failed') AS failed,
        COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled,
        COALESCE(SUM(file_size), 0) / (1024.0 * 1024 * 1024) AS total_storage_gb,
        COALESCE(SUM(duration_seconds), 0) / 3600.0 AS total_duration_hours,
        MAX(updated_at) AS last_activity
       FROM np_rec_recordings
       WHERE source_account_id = $1 AND deleted_at IS NULL`,
      [this.sourceAccountId]
    );

    const scheduleResult = await this.query<{
      total_schedules: number;
      active_schedules: number;
    }>(
      `SELECT
        COUNT(*) AS total_schedules,
        COUNT(*) FILTER (WHERE active = TRUE) AS active_schedules
       FROM np_rec_schedules
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const encodeResult = await this.query<{
      pending_encode_jobs: number;
      running_encode_jobs: number;
    }>(
      `SELECT
        COUNT(*) FILTER (WHERE status = 'pending') AS pending_encode_jobs,
        COUNT(*) FILTER (WHERE status = 'running') AS running_encode_jobs
       FROM np_rec_encode_jobs
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    const srow = scheduleResult.rows[0];
    const erow = encodeResult.rows[0];

    return {
      total_recordings: row?.total_recordings ?? 0,
      scheduled: row?.scheduled ?? 0,
      recording_now: row?.recording_now ?? 0,
      encoding: row?.encoding ?? 0,
      published: row?.published ?? 0,
      failed: row?.failed ?? 0,
      cancelled: row?.cancelled ?? 0,
      total_storage_gb: row?.total_storage_gb ?? 0,
      total_duration_hours: row?.total_duration_hours ?? 0,
      total_schedules: srow?.total_schedules ?? 0,
      active_schedules: srow?.active_schedules ?? 0,
      pending_encode_jobs: erow?.pending_encode_jobs ?? 0,
      running_encode_jobs: erow?.running_encode_jobs ?? 0,
      last_activity: row?.last_activity ?? null,
    };
  }
}
