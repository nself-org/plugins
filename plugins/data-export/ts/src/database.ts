/**
 * Data Export Database Operations
 * Complete CRUD operations for export, deletion, and import in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ExportRequestRecord,
  DeletionRequestRecord,
  PluginRegistryRecord,
  ImportJobRecord,
  ExportStats,
  CreateExportRequest,
  CreateDeletionRequest,
  RegisterPlugin,
  CreateImportJob,
  ValidationError,
} from './types.js';

const logger = createLogger('data-export:db');

export class ExportDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ExportDatabase {
    return new ExportDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing data export schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Export Requests
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS export_requests (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        request_type VARCHAR(16) NOT NULL DEFAULT 'user_data',
        requester_id VARCHAR(255) NOT NULL,
        target_user_id VARCHAR(255),
        target_plugins TEXT[] DEFAULT '{}',
        format VARCHAR(16) DEFAULT 'json',
        status VARCHAR(32) DEFAULT 'pending',
        file_path TEXT,
        file_size_bytes BIGINT,
        download_token VARCHAR(128),
        download_expires_at TIMESTAMP WITH TIME ZONE,
        error_message TEXT,
        tables_exported TEXT[],
        row_counts JSONB DEFAULT '{}',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_export_requests_source_account ON export_requests(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_export_requests_requester ON export_requests(requester_id);
      CREATE INDEX IF NOT EXISTS idx_export_requests_target_user ON export_requests(target_user_id);
      CREATE INDEX IF NOT EXISTS idx_export_requests_status ON export_requests(status);
      CREATE INDEX IF NOT EXISTS idx_export_requests_download_token ON export_requests(download_token);
      CREATE INDEX IF NOT EXISTS idx_export_requests_created ON export_requests(created_at DESC);

      -- =====================================================================
      -- Deletion Requests
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS export_deletion_requests (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        requester_id VARCHAR(255) NOT NULL,
        target_user_id VARCHAR(255) NOT NULL,
        reason TEXT,
        status VARCHAR(32) DEFAULT 'pending',
        verification_code VARCHAR(64),
        verified_at TIMESTAMP WITH TIME ZONE,
        cooldown_until TIMESTAMP WITH TIME ZONE,
        tables_processed TEXT[],
        rows_deleted JSONB DEFAULT '{}',
        error_message TEXT,
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_deletion_requests_source_account ON export_deletion_requests(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_deletion_requests_requester ON export_deletion_requests(requester_id);
      CREATE INDEX IF NOT EXISTS idx_deletion_requests_target_user ON export_deletion_requests(target_user_id);
      CREATE INDEX IF NOT EXISTS idx_deletion_requests_status ON export_deletion_requests(status);
      CREATE INDEX IF NOT EXISTS idx_deletion_requests_created ON export_deletion_requests(created_at DESC);

      -- =====================================================================
      -- Plugin Registry
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS export_plugin_registry (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        plugin_name VARCHAR(128) NOT NULL,
        tables TEXT[] NOT NULL,
        user_id_column VARCHAR(128) DEFAULT 'user_id',
        export_query TEXT,
        deletion_query TEXT,
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, plugin_name)
      );

      CREATE INDEX IF NOT EXISTS idx_plugin_registry_source_account ON export_plugin_registry(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_plugin_registry_plugin_name ON export_plugin_registry(plugin_name);
      CREATE INDEX IF NOT EXISTS idx_plugin_registry_enabled ON export_plugin_registry(enabled);

      -- =====================================================================
      -- Import Jobs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS export_import_jobs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        requester_id VARCHAR(255) NOT NULL,
        source_type VARCHAR(32) DEFAULT 'file',
        source_path TEXT,
        format VARCHAR(16) DEFAULT 'json',
        status VARCHAR(32) DEFAULT 'pending',
        tables_imported TEXT[],
        row_counts JSONB DEFAULT '{}',
        validation_errors JSONB DEFAULT '[]',
        error_message TEXT,
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_import_jobs_source_account ON export_import_jobs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_import_jobs_requester ON export_import_jobs(requester_id);
      CREATE INDEX IF NOT EXISTS idx_import_jobs_status ON export_import_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_import_jobs_created ON export_import_jobs(created_at DESC);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS export_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_webhook_events_source_account ON export_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_events_type ON export_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_webhook_events_processed ON export_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_webhook_events_created ON export_webhook_events(created_at DESC);
    `;

    await this.execute(schema);
    logger.success('Data export schema initialized');
  }

  // =========================================================================
  // Export Requests
  // =========================================================================

  async createExportRequest(request: CreateExportRequest): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO export_requests (
        source_account_id, request_type, requester_id, target_user_id,
        target_plugins, format, status, created_at, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
      RETURNING id`,
      [
        this.sourceAccountId,
        request.requestType,
        request.requesterId,
        request.targetUserId ?? null,
        request.targetPlugins ?? [],
        request.format ?? 'json',
        'pending',
      ]
    );

    return result.rows[0].id;
  }

  async getExportRequest(id: string): Promise<ExportRequestRecord | null> {
    const result = await this.query<ExportRequestRecord>(
      `SELECT * FROM export_requests
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listExportRequests(limit = 100, offset = 0): Promise<ExportRequestRecord[]> {
    const result = await this.query<ExportRequestRecord>(
      `SELECT * FROM export_requests
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async updateExportStatus(
    id: string,
    status: string,
    updates: {
      filePath?: string;
      fileSizeBytes?: number;
      downloadToken?: string;
      downloadExpiresAt?: Date;
      errorMessage?: string;
      tablesExported?: string[];
      rowCounts?: Record<string, number>;
      startedAt?: Date;
      completedAt?: Date;
    } = {}
  ): Promise<void> {
    const fields: string[] = ['status = $2', 'updated_at = NOW()'];
    const params: unknown[] = [id, status];
    let paramIndex = 3;

    if (updates.filePath !== undefined) {
      fields.push(`file_path = $${paramIndex++}`);
      params.push(updates.filePath);
    }
    if (updates.fileSizeBytes !== undefined) {
      fields.push(`file_size_bytes = $${paramIndex++}`);
      params.push(updates.fileSizeBytes);
    }
    if (updates.downloadToken !== undefined) {
      fields.push(`download_token = $${paramIndex++}`);
      params.push(updates.downloadToken);
    }
    if (updates.downloadExpiresAt !== undefined) {
      fields.push(`download_expires_at = $${paramIndex++}`);
      params.push(updates.downloadExpiresAt);
    }
    if (updates.errorMessage !== undefined) {
      fields.push(`error_message = $${paramIndex++}`);
      params.push(updates.errorMessage);
    }
    if (updates.tablesExported !== undefined) {
      fields.push(`tables_exported = $${paramIndex++}`);
      params.push(updates.tablesExported);
    }
    if (updates.rowCounts !== undefined) {
      fields.push(`row_counts = $${paramIndex++}`);
      params.push(JSON.stringify(updates.rowCounts));
    }
    if (updates.startedAt !== undefined) {
      fields.push(`started_at = $${paramIndex++}`);
      params.push(updates.startedAt);
    }
    if (updates.completedAt !== undefined) {
      fields.push(`completed_at = $${paramIndex++}`);
      params.push(updates.completedAt);
    }

    await this.execute(
      `UPDATE export_requests
       SET ${fields.join(', ')}
       WHERE id = $1 AND source_account_id = $${paramIndex}`,
      [...params, this.sourceAccountId]
    );
  }

  async deleteExportRequest(id: string): Promise<void> {
    await this.execute(
      `DELETE FROM export_requests WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Deletion Requests
  // =========================================================================

  async createDeletionRequest(request: CreateDeletionRequest, verificationCode: string): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO export_deletion_requests (
        source_account_id, requester_id, target_user_id, reason,
        status, verification_code, created_at, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
      RETURNING id`,
      [
        this.sourceAccountId,
        request.requesterId,
        request.targetUserId,
        request.reason ?? null,
        'pending',
        verificationCode,
      ]
    );

    return result.rows[0].id;
  }

  async getDeletionRequest(id: string): Promise<DeletionRequestRecord | null> {
    const result = await this.query<DeletionRequestRecord>(
      `SELECT * FROM export_deletion_requests
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listDeletionRequests(limit = 100, offset = 0): Promise<DeletionRequestRecord[]> {
    const result = await this.query<DeletionRequestRecord>(
      `SELECT * FROM export_deletion_requests
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async verifyDeletionRequest(id: string, cooldownUntil: Date): Promise<void> {
    await this.execute(
      `UPDATE export_deletion_requests
       SET status = $2, verified_at = NOW(), cooldown_until = $3, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $4`,
      [id, 'verifying', cooldownUntil, this.sourceAccountId]
    );
  }

  async updateDeletionStatus(
    id: string,
    status: string,
    updates: {
      errorMessage?: string;
      tablesProcessed?: string[];
      rowsDeleted?: Record<string, number>;
      startedAt?: Date;
      completedAt?: Date;
    } = {}
  ): Promise<void> {
    const fields: string[] = ['status = $2', 'updated_at = NOW()'];
    const params: unknown[] = [id, status];
    let paramIndex = 3;

    if (updates.errorMessage !== undefined) {
      fields.push(`error_message = $${paramIndex++}`);
      params.push(updates.errorMessage);
    }
    if (updates.tablesProcessed !== undefined) {
      fields.push(`tables_processed = $${paramIndex++}`);
      params.push(updates.tablesProcessed);
    }
    if (updates.rowsDeleted !== undefined) {
      fields.push(`rows_deleted = $${paramIndex++}`);
      params.push(JSON.stringify(updates.rowsDeleted));
    }
    if (updates.startedAt !== undefined) {
      fields.push(`started_at = $${paramIndex++}`);
      params.push(updates.startedAt);
    }
    if (updates.completedAt !== undefined) {
      fields.push(`completed_at = $${paramIndex++}`);
      params.push(updates.completedAt);
    }

    await this.execute(
      `UPDATE export_deletion_requests
       SET ${fields.join(', ')}
       WHERE id = $1 AND source_account_id = $${paramIndex}`,
      [...params, this.sourceAccountId]
    );
  }

  async cancelDeletionRequest(id: string): Promise<void> {
    await this.execute(
      `UPDATE export_deletion_requests
       SET status = $2, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $3`,
      [id, 'cancelled', this.sourceAccountId]
    );
  }

  // =========================================================================
  // Plugin Registry
  // =========================================================================

  async registerPlugin(plugin: RegisterPlugin): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO export_plugin_registry (
        source_account_id, plugin_name, tables, user_id_column,
        export_query, deletion_query, enabled, created_at, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
      ON CONFLICT (source_account_id, plugin_name) DO UPDATE SET
        tables = EXCLUDED.tables,
        user_id_column = EXCLUDED.user_id_column,
        export_query = EXCLUDED.export_query,
        deletion_query = EXCLUDED.deletion_query,
        enabled = EXCLUDED.enabled,
        updated_at = NOW()
      RETURNING id`,
      [
        this.sourceAccountId,
        plugin.pluginName,
        plugin.tables,
        plugin.userIdColumn ?? 'user_id',
        plugin.exportQuery ?? null,
        plugin.deletionQuery ?? null,
        plugin.enabled ?? true,
      ]
    );

    return result.rows[0].id;
  }

  async getPluginRegistry(id: string): Promise<PluginRegistryRecord | null> {
    const result = await this.query<PluginRegistryRecord>(
      `SELECT * FROM export_plugin_registry
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getPluginByName(pluginName: string): Promise<PluginRegistryRecord | null> {
    const result = await this.query<PluginRegistryRecord>(
      `SELECT * FROM export_plugin_registry
       WHERE plugin_name = $1 AND source_account_id = $2`,
      [pluginName, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listPluginRegistry(limit = 100, offset = 0): Promise<PluginRegistryRecord[]> {
    const result = await this.query<PluginRegistryRecord>(
      `SELECT * FROM export_plugin_registry
       WHERE source_account_id = $1
       ORDER BY plugin_name
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async listEnabledPlugins(): Promise<PluginRegistryRecord[]> {
    const result = await this.query<PluginRegistryRecord>(
      `SELECT * FROM export_plugin_registry
       WHERE source_account_id = $1 AND enabled = true
       ORDER BY plugin_name`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async updatePluginRegistry(
    id: string,
    updates: {
      tables?: string[];
      userIdColumn?: string;
      exportQuery?: string;
      deletionQuery?: string;
      enabled?: boolean;
    }
  ): Promise<void> {
    const fields: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [id];
    let paramIndex = 2;

    if (updates.tables !== undefined) {
      fields.push(`tables = $${paramIndex++}`);
      params.push(updates.tables);
    }
    if (updates.userIdColumn !== undefined) {
      fields.push(`user_id_column = $${paramIndex++}`);
      params.push(updates.userIdColumn);
    }
    if (updates.exportQuery !== undefined) {
      fields.push(`export_query = $${paramIndex++}`);
      params.push(updates.exportQuery);
    }
    if (updates.deletionQuery !== undefined) {
      fields.push(`deletion_query = $${paramIndex++}`);
      params.push(updates.deletionQuery);
    }
    if (updates.enabled !== undefined) {
      fields.push(`enabled = $${paramIndex++}`);
      params.push(updates.enabled);
    }

    await this.execute(
      `UPDATE export_plugin_registry
       SET ${fields.join(', ')}
       WHERE id = $1 AND source_account_id = $${paramIndex}`,
      [...params, this.sourceAccountId]
    );
  }

  async deletePluginRegistry(id: string): Promise<void> {
    await this.execute(
      `DELETE FROM export_plugin_registry WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Import Jobs
  // =========================================================================

  async createImportJob(job: CreateImportJob): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO export_import_jobs (
        source_account_id, requester_id, source_type, source_path, format, status, created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW())
      RETURNING id`,
      [
        this.sourceAccountId,
        job.requesterId,
        job.sourceType,
        job.sourcePath,
        job.format ?? 'json',
        'pending',
      ]
    );

    return result.rows[0].id;
  }

  async getImportJob(id: string): Promise<ImportJobRecord | null> {
    const result = await this.query<ImportJobRecord>(
      `SELECT * FROM export_import_jobs
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listImportJobs(limit = 100, offset = 0): Promise<ImportJobRecord[]> {
    const result = await this.query<ImportJobRecord>(
      `SELECT * FROM export_import_jobs
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async updateImportStatus(
    id: string,
    status: string,
    updates: {
      errorMessage?: string;
      tablesImported?: string[];
      rowCounts?: Record<string, number>;
      validationErrors?: ValidationError[];
      startedAt?: Date;
      completedAt?: Date;
    } = {}
  ): Promise<void> {
    const fields: string[] = ['status = $2'];
    const params: unknown[] = [id, status];
    let paramIndex = 3;

    if (updates.errorMessage !== undefined) {
      fields.push(`error_message = $${paramIndex++}`);
      params.push(updates.errorMessage);
    }
    if (updates.tablesImported !== undefined) {
      fields.push(`tables_imported = $${paramIndex++}`);
      params.push(updates.tablesImported);
    }
    if (updates.rowCounts !== undefined) {
      fields.push(`row_counts = $${paramIndex++}`);
      params.push(JSON.stringify(updates.rowCounts));
    }
    if (updates.validationErrors !== undefined) {
      fields.push(`validation_errors = $${paramIndex++}`);
      params.push(JSON.stringify(updates.validationErrors));
    }
    if (updates.startedAt !== undefined) {
      fields.push(`started_at = $${paramIndex++}`);
      params.push(updates.startedAt);
    }
    if (updates.completedAt !== undefined) {
      fields.push(`completed_at = $${paramIndex++}`);
      params.push(updates.completedAt);
    }

    await this.execute(
      `UPDATE export_import_jobs
       SET ${fields.join(', ')}
       WHERE id = $1 AND source_account_id = $${paramIndex}`,
      [...params, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: { id: string; type: string; payload: Record<string, unknown> }): Promise<void> {
    await this.execute(
      `INSERT INTO export_webhook_events (id, source_account_id, event_type, payload, created_at)
       VALUES ($1, $2, $3, $4, NOW())
       ON CONFLICT (id) DO NOTHING`,
      [event.id, this.sourceAccountId, event.type, JSON.stringify(event.payload)]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE export_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1 AND source_account_id = $3`,
      [eventId, error ?? null, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<ExportStats> {
    const result = await this.query<{
      total_exports: string;
      pending_exports: string;
      completed_exports: string;
      failed_exports: string;
      total_deletions: string;
      pending_deletions: string;
      completed_deletions: string;
      failed_deletions: string;
      total_imports: string;
      registered_plugins: string;
      last_export_at: Date | null;
      last_deletion_at: Date | null;
      last_import_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM export_requests WHERE source_account_id = $1) as total_exports,
        (SELECT COUNT(*) FROM export_requests WHERE source_account_id = $1 AND status = 'pending') as pending_exports,
        (SELECT COUNT(*) FROM export_requests WHERE source_account_id = $1 AND status = 'completed') as completed_exports,
        (SELECT COUNT(*) FROM export_requests WHERE source_account_id = $1 AND status = 'failed') as failed_exports,
        (SELECT COUNT(*) FROM export_deletion_requests WHERE source_account_id = $1) as total_deletions,
        (SELECT COUNT(*) FROM export_deletion_requests WHERE source_account_id = $1 AND status IN ('pending', 'verifying')) as pending_deletions,
        (SELECT COUNT(*) FROM export_deletion_requests WHERE source_account_id = $1 AND status = 'completed') as completed_deletions,
        (SELECT COUNT(*) FROM export_deletion_requests WHERE source_account_id = $1 AND status = 'failed') as failed_deletions,
        (SELECT COUNT(*) FROM export_import_jobs WHERE source_account_id = $1) as total_imports,
        (SELECT COUNT(*) FROM export_plugin_registry WHERE source_account_id = $1) as registered_plugins,
        (SELECT MAX(created_at) FROM export_requests WHERE source_account_id = $1) as last_export_at,
        (SELECT MAX(created_at) FROM export_deletion_requests WHERE source_account_id = $1) as last_deletion_at,
        (SELECT MAX(created_at) FROM export_import_jobs WHERE source_account_id = $1) as last_import_at`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      totalExports: parseInt(row.total_exports, 10),
      pendingExports: parseInt(row.pending_exports, 10),
      completedExports: parseInt(row.completed_exports, 10),
      failedExports: parseInt(row.failed_exports, 10),
      totalDeletions: parseInt(row.total_deletions, 10),
      pendingDeletions: parseInt(row.pending_deletions, 10),
      completedDeletions: parseInt(row.completed_deletions, 10),
      failedDeletions: parseInt(row.failed_deletions, 10),
      totalImports: parseInt(row.total_imports, 10),
      registeredPlugins: parseInt(row.registered_plugins, 10),
      lastExportAt: row.last_export_at,
      lastDeletionAt: row.last_deletion_at,
      lastImportAt: row.last_import_at,
    };
  }

  async countExportRequests(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM export_requests WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0].count, 10);
  }

  async countDeletionRequests(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM export_deletion_requests WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0].count, 10);
  }

  async countPluginRegistry(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM export_plugin_registry WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0].count, 10);
  }

  async countImportJobs(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM export_import_jobs WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0].count, 10);
  }
}
