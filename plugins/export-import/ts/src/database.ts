/**
 * Export/Import Database Operations
 * Complete CRUD operations for export, import, migration, backup, restore, transforms, and audit
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ExportJobRecord,
  ImportJobRecord,
  MigrationJobRecord,
  BackupSnapshotRecord,
  RestoreJobRecord,
  TransformTemplateRecord,
  DataTransferAuditRecord,
  CreateExportJobRequest,
  CreateImportJobRequest,
  CreateMigrationJobRequest,
  CreateBackupSnapshotRequest,
  CreateRestoreJobRequest,
  CreateTransformTemplateRequest,
  UpdateTransformTemplateRequest,
  CreateAuditEntryRequest,
  ExportImportStats,
  ExportJobStatus,
  ImportJobStatus,
  MigrationJobStatus,
  RestoreJobStatus,
  JobType,
  BackupScheduleConfig,
} from './types.js';

const logger = createLogger('export-import:db');

export class ExportImportDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ExportImportDatabase {
    return new ExportImportDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing export/import schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Export Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_export_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        description TEXT,
        export_type VARCHAR(50) NOT NULL,
        format VARCHAR(50) NOT NULL,
        scope JSONB NOT NULL DEFAULT '{}',
        filters JSONB DEFAULT '{}',
        compression VARCHAR(20),
        encryption_enabled BOOLEAN DEFAULT false,
        encryption_key_id UUID,
        status VARCHAR(50) DEFAULT 'pending',
        progress_percentage INTEGER DEFAULT 0,
        total_records INTEGER DEFAULT 0,
        exported_records INTEGER DEFAULT 0,
        output_path TEXT,
        output_size_bytes BIGINT,
        checksum VARCHAR(64),
        error_message TEXT,
        metadata JSONB DEFAULT '{}',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        expires_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_export_jobs_source_account ON ei_export_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_export_jobs_user ON ei_export_jobs(user_id);
      CREATE INDEX IF NOT EXISTS idx_ei_export_jobs_status ON ei_export_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_ei_export_jobs_created ON ei_export_jobs(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_ei_export_jobs_expires ON ei_export_jobs(expires_at) WHERE expires_at IS NOT NULL;

      -- =====================================================================
      -- Import Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_import_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        description TEXT,
        import_type VARCHAR(50) NOT NULL,
        source_format VARCHAR(50) NOT NULL,
        source_path TEXT NOT NULL,
        source_size_bytes BIGINT,
        mapping_rules JSONB DEFAULT '{}',
        conflict_resolution VARCHAR(50) DEFAULT 'skip',
        validation_mode VARCHAR(50) DEFAULT 'strict',
        dry_run BOOLEAN DEFAULT false,
        status VARCHAR(50) DEFAULT 'pending',
        progress_percentage INTEGER DEFAULT 0,
        total_records INTEGER DEFAULT 0,
        imported_records INTEGER DEFAULT 0,
        skipped_records INTEGER DEFAULT 0,
        failed_records INTEGER DEFAULT 0,
        validation_errors JSONB DEFAULT '[]',
        error_message TEXT,
        metadata JSONB DEFAULT '{}',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_import_jobs_source_account ON ei_import_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_import_jobs_user ON ei_import_jobs(user_id);
      CREATE INDEX IF NOT EXISTS idx_ei_import_jobs_status ON ei_import_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_ei_import_jobs_created ON ei_import_jobs(created_at DESC);

      -- =====================================================================
      -- Migration Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_migration_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        source_platform VARCHAR(50) NOT NULL,
        source_credentials JSONB NOT NULL DEFAULT '{}',
        destination_scope JSONB NOT NULL DEFAULT '{}',
        migration_plan JSONB NOT NULL DEFAULT '{}',
        status VARCHAR(50) DEFAULT 'pending',
        phase VARCHAR(50),
        progress_percentage INTEGER DEFAULT 0,
        estimated_duration_minutes INTEGER,
        total_items INTEGER DEFAULT 0,
        migrated_items INTEGER DEFAULT 0,
        failed_items INTEGER DEFAULT 0,
        warnings JSONB DEFAULT '[]',
        error_message TEXT,
        metadata JSONB DEFAULT '{}',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_migration_jobs_source_account ON ei_migration_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_migration_jobs_user ON ei_migration_jobs(user_id);
      CREATE INDEX IF NOT EXISTS idx_ei_migration_jobs_platform ON ei_migration_jobs(source_platform);
      CREATE INDEX IF NOT EXISTS idx_ei_migration_jobs_status ON ei_migration_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_ei_migration_jobs_created ON ei_migration_jobs(created_at DESC);

      -- =====================================================================
      -- Backup Snapshots
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_backup_snapshots (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        backup_type VARCHAR(50) NOT NULL,
        base_snapshot_id UUID REFERENCES ei_backup_snapshots(id) ON DELETE SET NULL,
        scope JSONB NOT NULL DEFAULT '{}',
        compression VARCHAR(20) NOT NULL DEFAULT 'gzip',
        encryption_enabled BOOLEAN DEFAULT false,
        encryption_key_id UUID,
        storage_backend VARCHAR(50) NOT NULL DEFAULT 'local',
        storage_path TEXT NOT NULL,
        total_size_bytes BIGINT,
        compressed_size_bytes BIGINT,
        checksum VARCHAR(64),
        verification_status VARCHAR(50),
        verified_at TIMESTAMP WITH TIME ZONE,
        retention_days INTEGER DEFAULT 30,
        expires_at TIMESTAMP WITH TIME ZONE,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_snapshots_source_account ON ei_backup_snapshots(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_snapshots_type ON ei_backup_snapshots(backup_type);
      CREATE INDEX IF NOT EXISTS idx_ei_snapshots_created ON ei_backup_snapshots(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_ei_snapshots_expires ON ei_backup_snapshots(expires_at) WHERE expires_at IS NOT NULL;

      -- =====================================================================
      -- Restore Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_restore_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        snapshot_id UUID NOT NULL REFERENCES ei_backup_snapshots(id) ON DELETE CASCADE,
        restore_type VARCHAR(50) NOT NULL,
        target_scope JSONB NOT NULL DEFAULT '{}',
        restore_point TIMESTAMP WITH TIME ZONE,
        conflict_resolution VARCHAR(50) DEFAULT 'skip',
        status VARCHAR(50) DEFAULT 'pending',
        progress_percentage INTEGER DEFAULT 0,
        total_items INTEGER DEFAULT 0,
        restored_items INTEGER DEFAULT 0,
        failed_items INTEGER DEFAULT 0,
        error_message TEXT,
        metadata JSONB DEFAULT '{}',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_restore_jobs_source_account ON ei_restore_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_restore_jobs_user ON ei_restore_jobs(user_id);
      CREATE INDEX IF NOT EXISTS idx_ei_restore_jobs_snapshot ON ei_restore_jobs(snapshot_id);
      CREATE INDEX IF NOT EXISTS idx_ei_restore_jobs_status ON ei_restore_jobs(status);

      -- =====================================================================
      -- Transform Templates
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_transform_templates (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255),
        name VARCHAR(255) NOT NULL,
        description TEXT,
        source_format VARCHAR(50) NOT NULL,
        target_format VARCHAR(50) NOT NULL,
        transformations JSONB NOT NULL DEFAULT '{}',
        is_public BOOLEAN DEFAULT false,
        usage_count INTEGER DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_templates_source_account ON ei_transform_templates(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_templates_formats ON ei_transform_templates(source_format, target_format);
      CREATE INDEX IF NOT EXISTS idx_ei_templates_public ON ei_transform_templates(is_public) WHERE is_public = true;

      -- =====================================================================
      -- Data Transfer Audit Log
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_data_transfer_audit (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        job_type VARCHAR(50) NOT NULL,
        job_id UUID NOT NULL,
        user_id VARCHAR(255) NOT NULL,
        action VARCHAR(100) NOT NULL,
        records_affected INTEGER,
        data_size_bytes BIGINT,
        ip_address INET,
        user_agent TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_ei_audit_source_account ON ei_data_transfer_audit(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ei_audit_job ON ei_data_transfer_audit(job_type, job_id);
      CREATE INDEX IF NOT EXISTS idx_ei_audit_user ON ei_data_transfer_audit(user_id);
      CREATE INDEX IF NOT EXISTS idx_ei_audit_created ON ei_data_transfer_audit(created_at DESC);

      -- =====================================================================
      -- Backup Schedule Config (stored in DB for persistence)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ei_backup_schedule (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        enabled BOOLEAN DEFAULT false,
        frequency VARCHAR(20) NOT NULL DEFAULT 'daily',
        time VARCHAR(10) NOT NULL DEFAULT '02:00',
        backup_type VARCHAR(50) NOT NULL DEFAULT 'incremental',
        retention_days INTEGER DEFAULT 90,
        storage_backend VARCHAR(50) NOT NULL DEFAULT 'local',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_ei_backup_schedule_source_account ON ei_backup_schedule(source_account_id);
    `;

    await this.db.execute(schema);
    logger.success('Export/import schema initialized');
  }

  // =========================================================================
  // Export Jobs
  // =========================================================================

  async createExportJob(data: CreateExportJobRequest): Promise<ExportJobRecord> {
    const result = await this.db.query<ExportJobRecord>(
      `INSERT INTO ei_export_jobs (
        source_account_id, user_id, name, description, export_type, format,
        scope, filters, compression, encryption_enabled
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.user_id,
        data.name,
        data.description ?? null,
        data.export_type,
        data.format,
        JSON.stringify(data.scope),
        JSON.stringify(data.filters ?? {}),
        data.compression ?? 'gzip',
        data.encryption_enabled ?? false,
      ]
    );

    // Create audit entry
    await this.createAuditEntry({
      job_type: 'export',
      job_id: result.rows[0].id,
      user_id: data.user_id,
      action: 'created',
    });

    return result.rows[0];
  }

  async getExportJob(id: string): Promise<ExportJobRecord | null> {
    const result = await this.db.query<ExportJobRecord>(
      'SELECT * FROM ei_export_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listExportJobs(limit = 100, offset = 0, status?: ExportJobStatus): Promise<ExportJobRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.db.query<ExportJobRecord>(
      `SELECT * FROM ei_export_jobs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async updateExportJobStatus(id: string, status: ExportJobStatus, extra?: {
    progress_percentage?: number;
    exported_records?: number;
    total_records?: number;
    output_path?: string;
    output_size_bytes?: number;
    checksum?: string;
    error_message?: string;
    metadata?: Record<string, unknown>;
  }): Promise<ExportJobRecord | null> {
    const updates: string[] = ['status = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, status];
    let paramIndex = 4;

    if (status === 'running' && !extra?.progress_percentage) {
      updates.push('started_at = COALESCE(started_at, NOW())');
    }
    if (status === 'completed' || status === 'failed' || status === 'cancelled') {
      updates.push('completed_at = NOW()');
    }

    if (extra?.progress_percentage !== undefined) {
      updates.push(`progress_percentage = $${paramIndex++}`);
      params.push(extra.progress_percentage);
    }
    if (extra?.exported_records !== undefined) {
      updates.push(`exported_records = $${paramIndex++}`);
      params.push(extra.exported_records);
    }
    if (extra?.total_records !== undefined) {
      updates.push(`total_records = $${paramIndex++}`);
      params.push(extra.total_records);
    }
    if (extra?.output_path !== undefined) {
      updates.push(`output_path = $${paramIndex++}`);
      params.push(extra.output_path);
    }
    if (extra?.output_size_bytes !== undefined) {
      updates.push(`output_size_bytes = $${paramIndex++}`);
      params.push(extra.output_size_bytes);
    }
    if (extra?.checksum !== undefined) {
      updates.push(`checksum = $${paramIndex++}`);
      params.push(extra.checksum);
    }
    if (extra?.error_message !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(extra.error_message);
    }
    if (extra?.metadata !== undefined) {
      updates.push(`metadata = metadata || $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.metadata));
    }

    const result = await this.db.query<ExportJobRecord>(
      `UPDATE ei_export_jobs SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteExportJob(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM ei_export_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Import Jobs
  // =========================================================================

  async createImportJob(data: CreateImportJobRequest): Promise<ImportJobRecord> {
    const result = await this.db.query<ImportJobRecord>(
      `INSERT INTO ei_import_jobs (
        source_account_id, user_id, name, description, import_type, source_format,
        source_path, source_size_bytes, mapping_rules, conflict_resolution,
        validation_mode, dry_run
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.user_id,
        data.name,
        data.description ?? null,
        data.import_type,
        data.source_format,
        data.source_path,
        data.source_size_bytes ?? null,
        JSON.stringify(data.mapping_rules ?? {}),
        data.conflict_resolution ?? 'skip',
        data.validation_mode ?? 'strict',
        data.dry_run ?? false,
      ]
    );

    await this.createAuditEntry({
      job_type: 'import',
      job_id: result.rows[0].id,
      user_id: data.user_id,
      action: 'created',
    });

    return result.rows[0];
  }

  async getImportJob(id: string): Promise<ImportJobRecord | null> {
    const result = await this.db.query<ImportJobRecord>(
      'SELECT * FROM ei_import_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listImportJobs(limit = 100, offset = 0, status?: ImportJobStatus): Promise<ImportJobRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.db.query<ImportJobRecord>(
      `SELECT * FROM ei_import_jobs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async updateImportJobStatus(id: string, status: ImportJobStatus, extra?: {
    progress_percentage?: number;
    imported_records?: number;
    skipped_records?: number;
    failed_records?: number;
    total_records?: number;
    validation_errors?: unknown[];
    error_message?: string;
    metadata?: Record<string, unknown>;
  }): Promise<ImportJobRecord | null> {
    const updates: string[] = ['status = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, status];
    let paramIndex = 4;

    if (status === 'running') {
      updates.push('started_at = COALESCE(started_at, NOW())');
    }
    if (status === 'completed' || status === 'failed' || status === 'cancelled') {
      updates.push('completed_at = NOW()');
    }

    if (extra?.progress_percentage !== undefined) {
      updates.push(`progress_percentage = $${paramIndex++}`);
      params.push(extra.progress_percentage);
    }
    if (extra?.imported_records !== undefined) {
      updates.push(`imported_records = $${paramIndex++}`);
      params.push(extra.imported_records);
    }
    if (extra?.skipped_records !== undefined) {
      updates.push(`skipped_records = $${paramIndex++}`);
      params.push(extra.skipped_records);
    }
    if (extra?.failed_records !== undefined) {
      updates.push(`failed_records = $${paramIndex++}`);
      params.push(extra.failed_records);
    }
    if (extra?.total_records !== undefined) {
      updates.push(`total_records = $${paramIndex++}`);
      params.push(extra.total_records);
    }
    if (extra?.validation_errors !== undefined) {
      updates.push(`validation_errors = $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.validation_errors));
    }
    if (extra?.error_message !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(extra.error_message);
    }
    if (extra?.metadata !== undefined) {
      updates.push(`metadata = metadata || $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.metadata));
    }

    const result = await this.db.query<ImportJobRecord>(
      `UPDATE ei_import_jobs SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteImportJob(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM ei_import_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Migration Jobs
  // =========================================================================

  async createMigrationJob(data: CreateMigrationJobRequest): Promise<MigrationJobRecord> {
    const result = await this.db.query<MigrationJobRecord>(
      `INSERT INTO ei_migration_jobs (
        source_account_id, user_id, name, source_platform,
        source_credentials, destination_scope, migration_plan
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.user_id,
        data.name,
        data.source_platform,
        JSON.stringify(data.source_credentials),
        JSON.stringify(data.destination_scope),
        JSON.stringify(data.migration_plan),
      ]
    );

    await this.createAuditEntry({
      job_type: 'migration',
      job_id: result.rows[0].id,
      user_id: data.user_id,
      action: 'created',
    });

    return result.rows[0];
  }

  async getMigrationJob(id: string): Promise<MigrationJobRecord | null> {
    const result = await this.db.query<MigrationJobRecord>(
      'SELECT * FROM ei_migration_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listMigrationJobs(limit = 100, offset = 0, status?: MigrationJobStatus): Promise<MigrationJobRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.db.query<MigrationJobRecord>(
      `SELECT * FROM ei_migration_jobs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async updateMigrationJobStatus(id: string, status: MigrationJobStatus, extra?: {
    phase?: string;
    progress_percentage?: number;
    migrated_items?: number;
    failed_items?: number;
    total_items?: number;
    warnings?: unknown[];
    error_message?: string;
    metadata?: Record<string, unknown>;
  }): Promise<MigrationJobRecord | null> {
    const updates: string[] = ['status = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, status];
    let paramIndex = 4;

    if (status === 'running') {
      updates.push('started_at = COALESCE(started_at, NOW())');
    }
    if (status === 'completed' || status === 'failed' || status === 'cancelled') {
      updates.push('completed_at = NOW()');
    }

    if (extra?.phase !== undefined) {
      updates.push(`phase = $${paramIndex++}`);
      params.push(extra.phase);
    }
    if (extra?.progress_percentage !== undefined) {
      updates.push(`progress_percentage = $${paramIndex++}`);
      params.push(extra.progress_percentage);
    }
    if (extra?.migrated_items !== undefined) {
      updates.push(`migrated_items = $${paramIndex++}`);
      params.push(extra.migrated_items);
    }
    if (extra?.failed_items !== undefined) {
      updates.push(`failed_items = $${paramIndex++}`);
      params.push(extra.failed_items);
    }
    if (extra?.total_items !== undefined) {
      updates.push(`total_items = $${paramIndex++}`);
      params.push(extra.total_items);
    }
    if (extra?.warnings !== undefined) {
      updates.push(`warnings = $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.warnings));
    }
    if (extra?.error_message !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(extra.error_message);
    }
    if (extra?.metadata !== undefined) {
      updates.push(`metadata = metadata || $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.metadata));
    }

    const result = await this.db.query<MigrationJobRecord>(
      `UPDATE ei_migration_jobs SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteMigrationJob(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM ei_migration_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Backup Snapshots
  // =========================================================================

  async createBackupSnapshot(data: CreateBackupSnapshotRequest): Promise<BackupSnapshotRecord> {
    const storagePath = data.storage_path ?? `/backups/${Date.now()}-${data.backup_type}`;
    const expiresAt = data.retention_days
      ? new Date(Date.now() + data.retention_days * 24 * 60 * 60 * 1000)
      : null;

    const result = await this.db.query<BackupSnapshotRecord>(
      `INSERT INTO ei_backup_snapshots (
        source_account_id, name, description, backup_type, scope,
        compression, encryption_enabled, storage_backend, storage_path,
        retention_days, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.name,
        data.description ?? null,
        data.backup_type,
        JSON.stringify(data.scope),
        data.compression,
        data.encryption_enabled ?? false,
        data.storage_backend,
        storagePath,
        data.retention_days ?? 30,
        expiresAt,
      ]
    );

    return result.rows[0];
  }

  async getBackupSnapshot(id: string): Promise<BackupSnapshotRecord | null> {
    const result = await this.db.query<BackupSnapshotRecord>(
      'SELECT * FROM ei_backup_snapshots WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listBackupSnapshots(limit = 100, offset = 0): Promise<BackupSnapshotRecord[]> {
    const result = await this.db.query<BackupSnapshotRecord>(
      `SELECT * FROM ei_backup_snapshots
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async updateBackupSnapshot(id: string, extra: {
    total_size_bytes?: number;
    compressed_size_bytes?: number;
    checksum?: string;
    verification_status?: string;
    verified_at?: Date;
    metadata?: Record<string, unknown>;
  }): Promise<BackupSnapshotRecord | null> {
    const updates: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (extra.total_size_bytes !== undefined) {
      updates.push(`total_size_bytes = $${paramIndex++}`);
      params.push(extra.total_size_bytes);
    }
    if (extra.compressed_size_bytes !== undefined) {
      updates.push(`compressed_size_bytes = $${paramIndex++}`);
      params.push(extra.compressed_size_bytes);
    }
    if (extra.checksum !== undefined) {
      updates.push(`checksum = $${paramIndex++}`);
      params.push(extra.checksum);
    }
    if (extra.verification_status !== undefined) {
      updates.push(`verification_status = $${paramIndex++}`);
      params.push(extra.verification_status);
    }
    if (extra.verified_at !== undefined) {
      updates.push(`verified_at = $${paramIndex++}`);
      params.push(extra.verified_at);
    }
    if (extra.metadata !== undefined) {
      updates.push(`metadata = metadata || $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.metadata));
    }

    const result = await this.db.query<BackupSnapshotRecord>(
      `UPDATE ei_backup_snapshots SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async verifyBackupSnapshot(id: string): Promise<BackupSnapshotRecord | null> {
    return this.updateBackupSnapshot(id, {
      verification_status: 'verified',
      verified_at: new Date(),
    });
  }

  async deleteBackupSnapshot(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM ei_backup_snapshots WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async cleanupExpiredSnapshots(): Promise<number> {
    const result = await this.db.query<{ count: string }>(
      `WITH deleted AS (
        DELETE FROM ei_backup_snapshots
        WHERE source_account_id = $1
          AND expires_at IS NOT NULL
          AND expires_at < NOW()
        RETURNING id
      )
      SELECT COUNT(*) as count FROM deleted`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Restore Jobs
  // =========================================================================

  async createRestoreJob(data: CreateRestoreJobRequest): Promise<RestoreJobRecord> {
    const result = await this.db.query<RestoreJobRecord>(
      `INSERT INTO ei_restore_jobs (
        source_account_id, user_id, snapshot_id, restore_type,
        target_scope, restore_point, conflict_resolution
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.user_id,
        data.snapshot_id,
        data.restore_type,
        JSON.stringify(data.target_scope),
        data.restore_point ?? null,
        data.conflict_resolution ?? 'skip',
      ]
    );

    await this.createAuditEntry({
      job_type: 'restore',
      job_id: result.rows[0].id,
      user_id: data.user_id,
      action: 'created',
    });

    return result.rows[0];
  }

  async getRestoreJob(id: string): Promise<RestoreJobRecord | null> {
    const result = await this.db.query<RestoreJobRecord>(
      'SELECT * FROM ei_restore_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listRestoreJobs(limit = 100, offset = 0, snapshotId?: string): Promise<RestoreJobRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (snapshotId) {
      conditions.push(`snapshot_id = $${paramIndex++}`);
      params.push(snapshotId);
    }

    params.push(limit, offset);

    const result = await this.db.query<RestoreJobRecord>(
      `SELECT * FROM ei_restore_jobs
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async updateRestoreJobStatus(id: string, status: RestoreJobStatus, extra?: {
    progress_percentage?: number;
    restored_items?: number;
    failed_items?: number;
    total_items?: number;
    error_message?: string;
    metadata?: Record<string, unknown>;
  }): Promise<RestoreJobRecord | null> {
    const updates: string[] = ['status = $3', 'updated_at = NOW()'];
    const params: unknown[] = [id, this.sourceAccountId, status];
    let paramIndex = 4;

    if (status === 'running') {
      updates.push('started_at = COALESCE(started_at, NOW())');
    }
    if (status === 'completed' || status === 'failed' || status === 'cancelled') {
      updates.push('completed_at = NOW()');
    }

    if (extra?.progress_percentage !== undefined) {
      updates.push(`progress_percentage = $${paramIndex++}`);
      params.push(extra.progress_percentage);
    }
    if (extra?.restored_items !== undefined) {
      updates.push(`restored_items = $${paramIndex++}`);
      params.push(extra.restored_items);
    }
    if (extra?.failed_items !== undefined) {
      updates.push(`failed_items = $${paramIndex++}`);
      params.push(extra.failed_items);
    }
    if (extra?.total_items !== undefined) {
      updates.push(`total_items = $${paramIndex++}`);
      params.push(extra.total_items);
    }
    if (extra?.error_message !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(extra.error_message);
    }
    if (extra?.metadata !== undefined) {
      updates.push(`metadata = metadata || $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(extra.metadata));
    }

    const result = await this.db.query<RestoreJobRecord>(
      `UPDATE ei_restore_jobs SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteRestoreJob(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM ei_restore_jobs WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Transform Templates
  // =========================================================================

  async createTransformTemplate(data: CreateTransformTemplateRequest): Promise<TransformTemplateRecord> {
    const result = await this.db.query<TransformTemplateRecord>(
      `INSERT INTO ei_transform_templates (
        source_account_id, user_id, name, description,
        source_format, target_format, transformations, is_public
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.user_id ?? null,
        data.name,
        data.description ?? null,
        data.source_format,
        data.target_format,
        JSON.stringify(data.transformations),
        data.is_public ?? false,
      ]
    );
    return result.rows[0];
  }

  async getTransformTemplate(id: string): Promise<TransformTemplateRecord | null> {
    const result = await this.db.query<TransformTemplateRecord>(
      'SELECT * FROM ei_transform_templates WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listTransformTemplates(limit = 100, offset = 0, filters?: {
    source_format?: string;
    target_format?: string;
  }): Promise<TransformTemplateRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.source_format) {
      conditions.push(`source_format = $${paramIndex++}`);
      params.push(filters.source_format);
    }
    if (filters?.target_format) {
      conditions.push(`target_format = $${paramIndex++}`);
      params.push(filters.target_format);
    }

    params.push(limit, offset);

    const result = await this.db.query<TransformTemplateRecord>(
      `SELECT * FROM ei_transform_templates
       WHERE ${conditions.join(' AND ')}
       ORDER BY usage_count DESC, created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async updateTransformTemplate(id: string, data: UpdateTransformTemplateRequest): Promise<TransformTemplateRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      params.push(data.name);
    }
    if (data.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      params.push(data.description);
    }
    if (data.transformations !== undefined) {
      updates.push(`transformations = $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(data.transformations));
    }
    if (data.is_public !== undefined) {
      updates.push(`is_public = $${paramIndex++}`);
      params.push(data.is_public);
    }

    if (updates.length === 0) {
      return this.getTransformTemplate(id);
    }

    updates.push('updated_at = NOW()');

    const result = await this.db.query<TransformTemplateRecord>(
      `UPDATE ei_transform_templates SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async incrementTemplateUsage(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE ei_transform_templates SET usage_count = usage_count + 1, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async deleteTransformTemplate(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM ei_transform_templates WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Audit Log
  // =========================================================================

  async createAuditEntry(data: CreateAuditEntryRequest): Promise<DataTransferAuditRecord> {
    const result = await this.db.query<DataTransferAuditRecord>(
      `INSERT INTO ei_data_transfer_audit (
        source_account_id, job_type, job_id, user_id, action,
        records_affected, data_size_bytes, ip_address, user_agent, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.job_type,
        data.job_id,
        data.user_id,
        data.action,
        data.records_affected ?? null,
        data.data_size_bytes ?? null,
        data.ip_address ?? null,
        data.user_agent ?? null,
        JSON.stringify(data.metadata ?? {}),
      ]
    );
    return result.rows[0];
  }

  async listAuditEntries(limit = 100, offset = 0, filters?: {
    job_type?: JobType;
    user_id?: string;
  }): Promise<DataTransferAuditRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.job_type) {
      conditions.push(`job_type = $${paramIndex++}`);
      params.push(filters.job_type);
    }
    if (filters?.user_id) {
      conditions.push(`user_id = $${paramIndex++}`);
      params.push(filters.user_id);
    }

    params.push(limit, offset);

    const result = await this.db.query<DataTransferAuditRecord>(
      `SELECT * FROM ei_data_transfer_audit
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async getAuditEntry(id: string): Promise<DataTransferAuditRecord | null> {
    const result = await this.db.query<DataTransferAuditRecord>(
      'SELECT * FROM ei_data_transfer_audit WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Backup Schedule
  // =========================================================================

  async getBackupSchedule(): Promise<BackupScheduleConfig | null> {
    const result = await this.db.query<Record<string, unknown>>(
      'SELECT * FROM ei_backup_schedule WHERE source_account_id = $1',
      [this.sourceAccountId]
    );
    if (!result.rows[0]) return null;
    const row = result.rows[0];
    return {
      enabled: row.enabled as boolean,
      frequency: row.frequency as BackupScheduleConfig['frequency'],
      time: row.time as string,
      backup_type: row.backup_type as BackupScheduleConfig['backup_type'],
      retention_days: row.retention_days as number,
      storage_backend: row.storage_backend as BackupScheduleConfig['storage_backend'],
    };
  }

  async upsertBackupSchedule(data: BackupScheduleConfig): Promise<BackupScheduleConfig> {
    await this.db.execute(
      `INSERT INTO ei_backup_schedule (
        source_account_id, enabled, frequency, time,
        backup_type, retention_days, storage_backend
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      ON CONFLICT (source_account_id) DO UPDATE SET
        enabled = EXCLUDED.enabled,
        frequency = EXCLUDED.frequency,
        time = EXCLUDED.time,
        backup_type = EXCLUDED.backup_type,
        retention_days = EXCLUDED.retention_days,
        storage_backend = EXCLUDED.storage_backend,
        updated_at = NOW()`,
      [
        this.sourceAccountId,
        data.enabled,
        data.frequency,
        data.time,
        data.backup_type,
        data.retention_days,
        data.storage_backend,
      ]
    );
    return data;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<ExportImportStats> {
    const exportResult = await this.db.query<{ status: string; count: string }>(
      `SELECT status, COUNT(*) as count FROM ei_export_jobs
       WHERE source_account_id = $1 GROUP BY status`,
      [this.sourceAccountId]
    );

    const importResult = await this.db.query<{ status: string; count: string }>(
      `SELECT status, COUNT(*) as count FROM ei_import_jobs
       WHERE source_account_id = $1 GROUP BY status`,
      [this.sourceAccountId]
    );

    const migrationResult = await this.db.query<{ status: string; count: string }>(
      `SELECT status, COUNT(*) as count FROM ei_migration_jobs
       WHERE source_account_id = $1 GROUP BY status`,
      [this.sourceAccountId]
    );

    const snapshotResult = await this.db.query<{ total: string; verified: string; expired: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE verification_status = 'verified') as verified,
        COUNT(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at < NOW()) as expired
       FROM ei_backup_snapshots
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const restoreResult = await this.db.query<{ status: string; count: string }>(
      `SELECT status, COUNT(*) as count FROM ei_restore_jobs
       WHERE source_account_id = $1 GROUP BY status`,
      [this.sourceAccountId]
    );

    const templateResult = await this.db.query<{ total: string; public_count: string }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE is_public = true) as public_count
       FROM ei_transform_templates
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const auditResult = await this.db.query<{ count: string }>(
      'SELECT COUNT(*) as count FROM ei_data_transfer_audit WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    const toStatusMap = (rows: { status: string; count: string }[]) => {
      const map = new Map(rows.map(r => [r.status, parseInt(r.count, 10)]));
      return {
        total: rows.reduce((sum, r) => sum + parseInt(r.count, 10), 0),
        pending: map.get('pending') ?? 0,
        running: map.get('running') ?? 0,
        completed: map.get('completed') ?? 0,
        failed: map.get('failed') ?? 0,
      };
    };

    const snapRow = snapshotResult.rows[0];

    return {
      export_jobs: toStatusMap(exportResult.rows),
      import_jobs: toStatusMap(importResult.rows),
      migration_jobs: toStatusMap(migrationResult.rows),
      backup_snapshots: {
        total: parseInt(snapRow?.total ?? '0', 10),
        verified: parseInt(snapRow?.verified ?? '0', 10),
        expired: parseInt(snapRow?.expired ?? '0', 10),
      },
      restore_jobs: toStatusMap(restoreResult.rows),
      transform_templates: {
        total: parseInt(templateResult.rows[0]?.total ?? '0', 10),
        public_count: parseInt(templateResult.rows[0]?.public_count ?? '0', 10),
      },
      audit_entries: parseInt(auditResult.rows[0]?.count ?? '0', 10),
    };
  }
}
