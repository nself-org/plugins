/**
 * Backup Database Operations
 * Complete CRUD operations for backup management
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  BackupScheduleRecord,
  BackupArtifactRecord,
  BackupRestoreJobRecord,
  BackupStats,
  CreateScheduleRequest,
  UpdateScheduleRequest,
} from './types.js';

const logger = createLogger('backup:db');

export class BackupDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): BackupDatabase {
    return new BackupDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing backup plugin schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Backup Schedules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS backup_schedules (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        schedule_cron VARCHAR(128) NOT NULL,
        backup_type VARCHAR(32) DEFAULT 'full' CHECK (backup_type IN ('full', 'incremental', 'schema_only', 'data_only')),
        target_provider VARCHAR(32) DEFAULT 'local' CHECK (target_provider IN ('local', 's3', 'r2', 'gcs')),
        target_config JSONB DEFAULT '{}',
        include_tables TEXT[] DEFAULT '{}',
        exclude_tables TEXT[] DEFAULT '{}',
        compression VARCHAR(16) DEFAULT 'gzip' CHECK (compression IN ('none', 'gzip', 'zstd')),
        encryption_enabled BOOLEAN DEFAULT false,
        encryption_key_id VARCHAR(255),
        retention_days INTEGER DEFAULT 30,
        max_backups INTEGER DEFAULT 10,
        enabled BOOLEAN DEFAULT true,
        last_run_at TIMESTAMP WITH TIME ZONE,
        next_run_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_backup_schedules_source_account
        ON backup_schedules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_backup_schedules_enabled
        ON backup_schedules(enabled) WHERE enabled = true;
      CREATE INDEX IF NOT EXISTS idx_backup_schedules_next_run
        ON backup_schedules(next_run_at) WHERE enabled = true AND next_run_at IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_backup_schedules_created
        ON backup_schedules(created_at DESC);

      -- =====================================================================
      -- Backup Artifacts
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS backup_artifacts (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        schedule_id UUID REFERENCES backup_schedules(id) ON DELETE SET NULL,
        backup_type VARCHAR(32) NOT NULL CHECK (backup_type IN ('full', 'incremental', 'schema_only', 'data_only')),
        status VARCHAR(32) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'expired')),
        file_path TEXT,
        file_size_bytes BIGINT,
        checksum_sha256 VARCHAR(64),
        tables_included TEXT[] DEFAULT '{}',
        row_counts JSONB DEFAULT '{}',
        duration_ms INTEGER,
        error_message TEXT,
        target_provider VARCHAR(32) NOT NULL CHECK (target_provider IN ('local', 's3', 'r2', 'gcs')),
        target_location TEXT,
        expires_at TIMESTAMP WITH TIME ZONE,
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_backup_artifacts_source_account
        ON backup_artifacts(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_backup_artifacts_schedule
        ON backup_artifacts(schedule_id);
      CREATE INDEX IF NOT EXISTS idx_backup_artifacts_status
        ON backup_artifacts(status);
      CREATE INDEX IF NOT EXISTS idx_backup_artifacts_created
        ON backup_artifacts(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_backup_artifacts_expires
        ON backup_artifacts(expires_at) WHERE expires_at IS NOT NULL;

      -- =====================================================================
      -- Restore Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS backup_restore_jobs (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        artifact_id UUID REFERENCES backup_artifacts(id) ON DELETE CASCADE,
        status VARCHAR(32) DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
        target_database VARCHAR(255) NOT NULL,
        tables_to_restore TEXT[] DEFAULT '{}',
        restore_mode VARCHAR(32) DEFAULT 'merge' CHECK (restore_mode IN ('merge', 'replace', 'dry_run')),
        conflict_strategy VARCHAR(32) DEFAULT 'skip' CHECK (conflict_strategy IN ('skip', 'overwrite', 'error')),
        rows_restored INTEGER DEFAULT 0,
        errors JSONB DEFAULT '[]',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_backup_restore_jobs_source_account
        ON backup_restore_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_backup_restore_jobs_artifact
        ON backup_restore_jobs(artifact_id);
      CREATE INDEX IF NOT EXISTS idx_backup_restore_jobs_status
        ON backup_restore_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_backup_restore_jobs_created
        ON backup_restore_jobs(created_at DESC);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS backup_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_backup_webhook_events_source_account
        ON backup_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_backup_webhook_events_type
        ON backup_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_backup_webhook_events_processed
        ON backup_webhook_events(processed, created_at);
      CREATE INDEX IF NOT EXISTS idx_backup_webhook_events_created
        ON backup_webhook_events(created_at DESC);
    `;

    await this.execute(schema);
    logger.info('Schema initialized successfully');
  }

  // =========================================================================
  // Schedule Operations
  // =========================================================================

  async createSchedule(data: CreateScheduleRequest): Promise<BackupScheduleRecord> {
    const result = await this.query<BackupScheduleRecord>(
      `INSERT INTO backup_schedules (
        source_account_id, name, schedule_cron, backup_type, target_provider,
        target_config, include_tables, exclude_tables, compression,
        encryption_enabled, encryption_key_id, retention_days, max_backups, enabled
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.name,
        data.schedule_cron,
        data.backup_type ?? 'full',
        data.target_provider ?? 'local',
        JSON.stringify(data.target_config ?? {}),
        data.include_tables ?? [],
        data.exclude_tables ?? [],
        data.compression ?? 'gzip',
        data.encryption_enabled ?? false,
        data.encryption_key_id ?? null,
        data.retention_days ?? 30,
        data.max_backups ?? 10,
        data.enabled ?? true,
      ]
    );

    return result.rows[0];
  }

  async getSchedule(id: string): Promise<BackupScheduleRecord | null> {
    const result = await this.query<BackupScheduleRecord>(
      'SELECT * FROM backup_schedules WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listSchedules(limit = 100, offset = 0): Promise<BackupScheduleRecord[]> {
    const result = await this.query<BackupScheduleRecord>(
      `SELECT * FROM backup_schedules
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async updateSchedule(id: string, data: UpdateScheduleRequest): Promise<BackupScheduleRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      params.push(data.name);
    }
    if (data.schedule_cron !== undefined) {
      updates.push(`schedule_cron = $${paramIndex++}`);
      params.push(data.schedule_cron);
    }
    if (data.backup_type !== undefined) {
      updates.push(`backup_type = $${paramIndex++}`);
      params.push(data.backup_type);
    }
    if (data.target_provider !== undefined) {
      updates.push(`target_provider = $${paramIndex++}`);
      params.push(data.target_provider);
    }
    if (data.target_config !== undefined) {
      updates.push(`target_config = $${paramIndex++}`);
      params.push(JSON.stringify(data.target_config));
    }
    if (data.include_tables !== undefined) {
      updates.push(`include_tables = $${paramIndex++}`);
      params.push(data.include_tables);
    }
    if (data.exclude_tables !== undefined) {
      updates.push(`exclude_tables = $${paramIndex++}`);
      params.push(data.exclude_tables);
    }
    if (data.compression !== undefined) {
      updates.push(`compression = $${paramIndex++}`);
      params.push(data.compression);
    }
    if (data.encryption_enabled !== undefined) {
      updates.push(`encryption_enabled = $${paramIndex++}`);
      params.push(data.encryption_enabled);
    }
    if (data.encryption_key_id !== undefined) {
      updates.push(`encryption_key_id = $${paramIndex++}`);
      params.push(data.encryption_key_id);
    }
    if (data.retention_days !== undefined) {
      updates.push(`retention_days = $${paramIndex++}`);
      params.push(data.retention_days);
    }
    if (data.max_backups !== undefined) {
      updates.push(`max_backups = $${paramIndex++}`);
      params.push(data.max_backups);
    }
    if (data.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex++}`);
      params.push(data.enabled);
    }

    if (updates.length === 0) {
      return this.getSchedule(id);
    }

    updates.push(`updated_at = NOW()`);

    const result = await this.query<BackupScheduleRecord>(
      `UPDATE backup_schedules
       SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteSchedule(id: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM backup_schedules WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  async updateScheduleLastRun(id: string, lastRunAt: Date, nextRunAt: Date | null): Promise<void> {
    await this.execute(
      'UPDATE backup_schedules SET last_run_at = $1, next_run_at = $2, updated_at = NOW() WHERE id = $3',
      [lastRunAt, nextRunAt, id]
    );
  }

  async getDueSchedules(): Promise<BackupScheduleRecord[]> {
    const result = await this.query<BackupScheduleRecord>(
      `SELECT * FROM backup_schedules
       WHERE source_account_id = $1
       AND enabled = true
       AND (next_run_at IS NULL OR next_run_at <= NOW())
       ORDER BY next_run_at ASC NULLS FIRST`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  // =========================================================================
  // Artifact Operations
  // =========================================================================

  async createArtifact(data: {
    scheduleId?: string;
    backupType: string;
    targetProvider: string;
    targetLocation?: string;
    expiresAt?: Date;
  }): Promise<BackupArtifactRecord> {
    const result = await this.query<BackupArtifactRecord>(
      `INSERT INTO backup_artifacts (
        source_account_id, schedule_id, backup_type, target_provider,
        target_location, expires_at, status, started_at
      ) VALUES ($1, $2, $3, $4, $5, $6, 'running', NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        data.scheduleId ?? null,
        data.backupType,
        data.targetProvider,
        data.targetLocation ?? null,
        data.expiresAt ?? null,
      ]
    );

    return result.rows[0];
  }

  async updateArtifact(id: string, data: {
    status?: string;
    filePath?: string;
    fileSize?: number;
    checksum?: string;
    tablesIncluded?: string[];
    rowCounts?: Record<string, number>;
    durationMs?: number;
    errorMessage?: string;
    completedAt?: Date;
  }): Promise<void> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.status !== undefined) {
      updates.push(`status = $${paramIndex++}`);
      params.push(data.status);
    }
    if (data.filePath !== undefined) {
      updates.push(`file_path = $${paramIndex++}`);
      params.push(data.filePath);
    }
    if (data.fileSize !== undefined) {
      updates.push(`file_size_bytes = $${paramIndex++}`);
      params.push(data.fileSize);
    }
    if (data.checksum !== undefined) {
      updates.push(`checksum_sha256 = $${paramIndex++}`);
      params.push(data.checksum);
    }
    if (data.tablesIncluded !== undefined) {
      updates.push(`tables_included = $${paramIndex++}`);
      params.push(data.tablesIncluded);
    }
    if (data.rowCounts !== undefined) {
      updates.push(`row_counts = $${paramIndex++}`);
      params.push(JSON.stringify(data.rowCounts));
    }
    if (data.durationMs !== undefined) {
      updates.push(`duration_ms = $${paramIndex++}`);
      params.push(data.durationMs);
    }
    if (data.errorMessage !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(data.errorMessage);
    }
    if (data.completedAt !== undefined) {
      updates.push(`completed_at = $${paramIndex++}`);
      params.push(data.completedAt);
    }

    if (updates.length === 0) {
      return;
    }

    await this.execute(
      `UPDATE backup_artifacts
       SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2`,
      params
    );
  }

  async getArtifact(id: string): Promise<BackupArtifactRecord | null> {
    const result = await this.query<BackupArtifactRecord>(
      'SELECT * FROM backup_artifacts WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listArtifacts(limit = 100, offset = 0, status?: string): Promise<BackupArtifactRecord[]> {
    let sql = `SELECT * FROM backup_artifacts WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];

    if (status) {
      sql += ` AND status = $2`;
      params.push(status);
    }

    sql += ` ORDER BY created_at DESC LIMIT $${params.length + 1} OFFSET $${params.length + 2}`;
    params.push(limit, offset);

    const result = await this.query<BackupArtifactRecord>(sql, params);
    return result.rows;
  }

  async deleteArtifact(id: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM backup_artifacts WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  async expireOldArtifacts(): Promise<number> {
    return this.execute(
      `UPDATE backup_artifacts
       SET status = 'expired'
       WHERE source_account_id = $1
       AND status = 'completed'
       AND expires_at IS NOT NULL
       AND expires_at < NOW()`,
      [this.sourceAccountId]
    );
  }

  async deleteExpiredArtifacts(): Promise<string[]> {
    const result = await this.query<{ id: string; file_path: string }>(
      `DELETE FROM backup_artifacts
       WHERE source_account_id = $1
       AND status = 'expired'
       RETURNING id, file_path`,
      [this.sourceAccountId]
    );

    return result.rows.map(row => row.file_path).filter(Boolean);
  }

  // =========================================================================
  // Restore Job Operations
  // =========================================================================

  async createRestoreJob(data: {
    artifactId: string;
    targetDatabase: string;
    tablesToRestore?: string[];
    restoreMode: string;
    conflictStrategy: string;
  }): Promise<BackupRestoreJobRecord> {
    const result = await this.query<BackupRestoreJobRecord>(
      `INSERT INTO backup_restore_jobs (
        source_account_id, artifact_id, target_database, tables_to_restore,
        restore_mode, conflict_strategy, status, started_at
      ) VALUES ($1, $2, $3, $4, $5, $6, 'running', NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        data.artifactId,
        data.targetDatabase,
        data.tablesToRestore ?? [],
        data.restoreMode,
        data.conflictStrategy,
      ]
    );

    return result.rows[0];
  }

  async updateRestoreJob(id: string, data: {
    status?: string;
    rowsRestored?: number;
    errors?: Array<{ table: string; error: string }>;
    completedAt?: Date;
  }): Promise<void> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.status !== undefined) {
      updates.push(`status = $${paramIndex++}`);
      params.push(data.status);
    }
    if (data.rowsRestored !== undefined) {
      updates.push(`rows_restored = $${paramIndex++}`);
      params.push(data.rowsRestored);
    }
    if (data.errors !== undefined) {
      updates.push(`errors = $${paramIndex++}`);
      params.push(JSON.stringify(data.errors));
    }
    if (data.completedAt !== undefined) {
      updates.push(`completed_at = $${paramIndex++}`);
      params.push(data.completedAt);
    }

    if (updates.length === 0) {
      return;
    }

    await this.execute(
      `UPDATE backup_restore_jobs
       SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2`,
      params
    );
  }

  async getRestoreJob(id: string): Promise<BackupRestoreJobRecord | null> {
    const result = await this.query<BackupRestoreJobRecord>(
      'SELECT * FROM backup_restore_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listRestoreJobs(limit = 100, offset = 0): Promise<BackupRestoreJobRecord[]> {
    const result = await this.query<BackupRestoreJobRecord>(
      `SELECT * FROM backup_restore_jobs
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  // =========================================================================
  // Webhook Event Operations
  // =========================================================================

  async insertWebhookEvent(event: {
    id: string;
    eventType: string;
    payload: Record<string, unknown>;
  }): Promise<void> {
    await this.execute(
      `INSERT INTO backup_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (id) DO NOTHING`,
      [event.id, this.sourceAccountId, event.eventType, JSON.stringify(event.payload)]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE backup_webhook_events
       SET processed = true, processed_at = NOW(), error = $1
       WHERE id = $2 AND source_account_id = $3`,
      [error ?? null, id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<BackupStats> {
    const scheduleStats = await this.query<{
      total_schedules: number;
      active_schedules: number;
    }>(
      `SELECT
        COUNT(*)::int as total_schedules,
        COUNT(*) FILTER (WHERE enabled = true)::int as active_schedules
       FROM backup_schedules
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const artifactStats = await this.query<{
      total_artifacts: number;
      completed_artifacts: number;
      failed_artifacts: number;
      total_size_bytes: number;
      oldest_backup: Date | null;
      newest_backup: Date | null;
    }>(
      `SELECT
        COUNT(*)::int as total_artifacts,
        COUNT(*) FILTER (WHERE status = 'completed')::int as completed_artifacts,
        COUNT(*) FILTER (WHERE status = 'failed')::int as failed_artifacts,
        COALESCE(SUM(file_size_bytes), 0)::bigint as total_size_bytes,
        MIN(completed_at) as oldest_backup,
        MAX(completed_at) as newest_backup
       FROM backup_artifacts
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const restoreStats = await this.query<{
      active_restore_jobs: number;
    }>(
      `SELECT
        COUNT(*)::int as active_restore_jobs
       FROM backup_restore_jobs
       WHERE source_account_id = $1
       AND status IN ('pending', 'running')`,
      [this.sourceAccountId]
    );

    return {
      total_schedules: scheduleStats.rows[0]?.total_schedules ?? 0,
      active_schedules: scheduleStats.rows[0]?.active_schedules ?? 0,
      total_artifacts: artifactStats.rows[0]?.total_artifacts ?? 0,
      completed_artifacts: artifactStats.rows[0]?.completed_artifacts ?? 0,
      failed_artifacts: artifactStats.rows[0]?.failed_artifacts ?? 0,
      total_size_bytes: artifactStats.rows[0]?.total_size_bytes ?? 0,
      oldest_backup: artifactStats.rows[0]?.oldest_backup ?? null,
      newest_backup: artifactStats.rows[0]?.newest_backup ?? null,
      active_restore_jobs: restoreStats.rows[0]?.active_restore_jobs ?? 0,
    };
  }
}
