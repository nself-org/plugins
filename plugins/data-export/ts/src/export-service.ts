/**
 * Data Export Service
 * Handles export, deletion, and import operations
 */

import { promises as fs } from 'fs';
import { join } from 'path';
import { createLogger } from '@nself/plugin-utils';
import { ExportDatabase } from './database.js';
import type {
  ExportRequestRecord,
  PluginRegistryRecord,
  ExportData,
  TableExport,
  ExportFormat,
} from './types.js';

const logger = createLogger('data-export:service');

export class ExportService {
  constructor(
    private db: ExportDatabase,
    private storagePath: string,
    private downloadExpiryHours: number,
    private deletionCooldownHours: number,
    private verificationCodeLength: number
  ) {}

  // =========================================================================
  // Export Operations
  // =========================================================================

  async processExportRequest(requestId: string): Promise<void> {
    const request = await this.db.getExportRequest(requestId);
    if (!request) {
      throw new Error(`Export request ${requestId} not found`);
    }

    if (request.status !== 'pending') {
      throw new Error(`Export request ${requestId} is not in pending status`);
    }

    logger.info('Processing export request', { requestId, type: request.request_type });

    try {
      await this.db.updateExportStatus(requestId, 'processing', { startedAt: new Date() });

      // Get plugins to export
      const plugins = await this.getTargetPlugins(request);
      logger.info('Target plugins', { count: plugins.length, plugins: plugins.map(p => p.plugin_name) });

      // Export data from each plugin
      const exportData: ExportData = {
        metadata: {
          exportId: requestId,
          requestType: request.request_type,
          userId: request.target_user_id ?? undefined,
          plugins: request.target_plugins ?? undefined,
          exportedAt: new Date().toISOString(),
          format: request.format,
          version: '1.0.0',
        },
        tables: {},
      };

      const tablesExported: string[] = [];
      const rowCounts: Record<string, number> = {};

      for (const plugin of plugins) {
        for (const table of plugin.tables) {
          const tableData = await this.exportTable(table, plugin, request);
          exportData.tables[table] = tableData;
          tablesExported.push(table);
          rowCounts[table] = tableData.rowCount;
        }
      }

      // Write export file
      const { filePath, fileSizeBytes } = await this.writeExportFile(requestId, exportData, request.format);

      // Generate download token
      const downloadToken = await this.generateDownloadToken();
      const downloadExpiresAt = new Date(Date.now() + this.downloadExpiryHours * 60 * 60 * 1000);

      await this.db.updateExportStatus(requestId, 'completed', {
        filePath,
        fileSizeBytes,
        downloadToken,
        downloadExpiresAt,
        tablesExported,
        rowCounts,
        completedAt: new Date(),
      });

      logger.success('Export completed', { requestId, tables: tablesExported.length, rows: Object.values(rowCounts).reduce((a, b) => a + b, 0) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export failed', { requestId, error: message });
      await this.db.updateExportStatus(requestId, 'failed', {
        errorMessage: message,
        completedAt: new Date(),
      });
      throw error;
    }
  }

  private async getTargetPlugins(request: ExportRequestRecord): Promise<PluginRegistryRecord[]> {
    const allPlugins = await this.db.listEnabledPlugins();

    if (request.target_plugins && request.target_plugins.length > 0) {
      return allPlugins.filter(p => request.target_plugins!.includes(p.plugin_name));
    }

    return allPlugins;
  }

  private async exportTable(
    tableName: string,
    plugin: PluginRegistryRecord,
    request: ExportRequestRecord
  ): Promise<TableExport> {
    logger.info('Exporting table', { table: tableName, plugin: plugin.plugin_name });

    // Use custom export query if provided
    let query: string;
    const params: unknown[] = [];

    if (plugin.export_query) {
      query = plugin.export_query;
      if (request.target_user_id) {
        params.push(request.target_user_id);
      }
    } else {
      // Default: SELECT * WHERE user_id_column = target_user_id
      if (request.target_user_id) {
        query = `SELECT * FROM ${tableName} WHERE ${plugin.user_id_column} = $1`;
        params.push(request.target_user_id);
      } else {
        query = `SELECT * FROM ${tableName}`;
      }
    }

    const result = await this.db.query<Record<string, unknown>>(query, params);
    const rows = result.rows;

    // Get column names from first row
    const columns = rows.length > 0 ? Object.keys(rows[0]) : [];

    return {
      tableName,
      rowCount: rows.length,
      columns,
      rows,
    };
  }

  private async writeExportFile(
    requestId: string,
    data: ExportData,
    format: ExportFormat
  ): Promise<{ filePath: string; fileSizeBytes: number }> {
    await fs.mkdir(this.storagePath, { recursive: true });

    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const filename = `export-${requestId}-${timestamp}.${format}`;
    const filePath = join(this.storagePath, filename);

    let content: string;

    switch (format) {
      case 'json':
        content = JSON.stringify(data, null, 2);
        break;

      case 'csv':
        // For CSV, create one CSV per table
        const csvFiles: string[] = [];
        for (const [tableName, tableData] of Object.entries(data.tables)) {
          const csv = this.convertToCSV(tableData);
          const csvPath = join(this.storagePath, `${tableName}-${requestId}.csv`);
          await fs.writeFile(csvPath, csv, 'utf-8');
          csvFiles.push(csvPath);
        }
        // Write manifest
        content = JSON.stringify({ metadata: data.metadata, files: csvFiles }, null, 2);
        break;

      case 'zip':
        // For ZIP, we'll just use JSON for now (ZIP requires additional library)
        content = JSON.stringify(data, null, 2);
        break;

      default:
        throw new Error(`Unsupported format: ${format}`);
    }

    await fs.writeFile(filePath, content, 'utf-8');
    const stats = await fs.stat(filePath);

    return {
      filePath,
      fileSizeBytes: stats.size,
    };
  }

  private convertToCSV(tableData: TableExport): string {
    if (tableData.rows.length === 0) {
      return '';
    }

    const lines: string[] = [];

    // Header
    lines.push(tableData.columns.map(col => this.escapeCSV(col)).join(','));

    // Rows
    for (const row of tableData.rows) {
      const values = tableData.columns.map(col => {
        const value = row[col];
        if (value === null || value === undefined) {
          return '';
        }
        if (typeof value === 'object') {
          return this.escapeCSV(JSON.stringify(value));
        }
        return this.escapeCSV(String(value));
      });
      lines.push(values.join(','));
    }

    return lines.join('\n');
  }

  private escapeCSV(value: string): string {
    if (value.includes(',') || value.includes('"') || value.includes('\n')) {
      return `"${value.replace(/"/g, '""')}"`;
    }
    return value;
  }

  private async generateDownloadToken(): Promise<string> {
    const buffer = await import('crypto').then(crypto => crypto.randomBytes(64));
    return buffer.toString('base64url');
  }

  async getExportFile(downloadToken: string): Promise<{ filePath: string; request: ExportRequestRecord } | null> {
    const result = await this.db.query(
      `SELECT * FROM export_requests WHERE download_token = $1 AND source_account_id = $2`,
      [downloadToken, this.db.getCurrentSourceAccountId()]
    );

    const request = result.rows[0] as ExportRequestRecord;
    if (!request) {
      return null;
    }

    // Check expiry
    if (request.download_expires_at && new Date() > new Date(request.download_expires_at)) {
      await this.db.updateExportStatus(request.id, 'expired');
      return null;
    }

    if (!request.file_path) {
      return null;
    }

    return { filePath: request.file_path, request };
  }

  // =========================================================================
  // Deletion Operations
  // =========================================================================

  async createDeletionWithVerification(requesterId: string, targetUserId: string, reason?: string): Promise<{ id: string; verificationCode: string }> {
    const verificationCode = this.generateVerificationCode();
    const id = await this.db.createDeletionRequest({ requesterId, targetUserId, reason }, verificationCode);

    logger.info('Deletion request created', { id, targetUserId });

    // In production, send verification code via email/SMS
    logger.warn('VERIFICATION CODE (send this to user)', { code: verificationCode });

    return { id, verificationCode };
  }

  private generateVerificationCode(): string {
    const digits = '0123456789';
    let code = '';
    for (let i = 0; i < this.verificationCodeLength; i++) {
      code += digits[Math.floor(Math.random() * digits.length)];
    }
    return code;
  }

  async verifyDeletion(requestId: string, code: string): Promise<boolean> {
    const request = await this.db.getDeletionRequest(requestId);
    if (!request) {
      throw new Error(`Deletion request ${requestId} not found`);
    }

    if (request.status !== 'pending') {
      throw new Error(`Deletion request ${requestId} is not in pending status`);
    }

    if (request.verification_code !== code) {
      logger.warn('Invalid verification code', { requestId });
      return false;
    }

    // Set cooldown period
    const cooldownUntil = new Date(Date.now() + this.deletionCooldownHours * 60 * 60 * 1000);
    await this.db.verifyDeletionRequest(requestId, cooldownUntil);

    logger.info('Deletion verified, cooldown period started', { requestId, cooldownUntil });
    return true;
  }

  async processDeletionRequest(requestId: string): Promise<void> {
    const request = await this.db.getDeletionRequest(requestId);
    if (!request) {
      throw new Error(`Deletion request ${requestId} not found`);
    }

    if (request.status !== 'verifying') {
      throw new Error(`Deletion request ${requestId} is not in verifying status`);
    }

    // Check cooldown
    if (request.cooldown_until && new Date() < new Date(request.cooldown_until)) {
      throw new Error(`Deletion request ${requestId} is still in cooldown period`);
    }

    logger.info('Processing deletion request', { requestId, targetUserId: request.target_user_id });

    try {
      await this.db.updateDeletionStatus(requestId, 'processing', { startedAt: new Date() });

      const plugins = await this.db.listEnabledPlugins();
      const tablesProcessed: string[] = [];
      const rowsDeleted: Record<string, number> = {};

      for (const plugin of plugins) {
        for (const table of plugin.tables) {
          const deleted = await this.deleteUserData(table, plugin, request.target_user_id);
          tablesProcessed.push(table);
          rowsDeleted[table] = deleted;
        }
      }

      await this.db.updateDeletionStatus(requestId, 'completed', {
        tablesProcessed,
        rowsDeleted,
        completedAt: new Date(),
      });

      logger.success('Deletion completed', { requestId, tables: tablesProcessed.length, rows: Object.values(rowsDeleted).reduce((a, b) => a + b, 0) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Deletion failed', { requestId, error: message });
      await this.db.updateDeletionStatus(requestId, 'failed', {
        errorMessage: message,
        completedAt: new Date(),
      });
      throw error;
    }
  }

  private async deleteUserData(tableName: string, plugin: PluginRegistryRecord, userId: string): Promise<number> {
    logger.info('Deleting user data', { table: tableName, plugin: plugin.plugin_name, userId });

    let query: string;
    const params: unknown[] = [userId];

    if (plugin.deletion_query) {
      query = plugin.deletion_query;
    } else {
      // Default: DELETE WHERE user_id_column = userId
      query = `DELETE FROM ${tableName} WHERE ${plugin.user_id_column} = $1`;
    }

    const deleted = await this.db.execute(query, params);
    logger.info('Deleted rows', { table: tableName, count: deleted });
    return deleted;
  }

  // =========================================================================
  // Import Operations
  // =========================================================================

  async processImportJob(jobId: string): Promise<void> {
    const job = await this.db.getImportJob(jobId);
    if (!job) {
      throw new Error(`Import job ${jobId} not found`);
    }

    if (job.status !== 'pending') {
      throw new Error(`Import job ${jobId} is not in pending status`);
    }

    logger.info('Processing import job', { jobId, sourcePath: job.source_path });

    try {
      await this.db.updateImportStatus(jobId, 'validating', { startedAt: new Date() });

      // Read import file
      if (!job.source_path) {
        throw new Error('Source path is required');
      }

      const fileContent = await fs.readFile(job.source_path, 'utf-8');
      const importData: ExportData = JSON.parse(fileContent);

      // Validate structure
      if (!importData.metadata || !importData.tables) {
        throw new Error('Invalid import file structure');
      }

      await this.db.updateImportStatus(jobId, 'importing');

      const tablesImported: string[] = [];
      const rowCounts: Record<string, number> = {};

      for (const [tableName, tableData] of Object.entries(importData.tables)) {
        const imported = await this.importTable(tableName, tableData);
        tablesImported.push(tableName);
        rowCounts[tableName] = imported;
      }

      await this.db.updateImportStatus(jobId, 'completed', {
        tablesImported,
        rowCounts,
        completedAt: new Date(),
      });

      logger.success('Import completed', { jobId, tables: tablesImported.length, rows: Object.values(rowCounts).reduce((a, b) => a + b, 0) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import failed', { jobId, error: message });
      await this.db.updateImportStatus(jobId, 'failed', {
        errorMessage: message,
        completedAt: new Date(),
      });
      throw error;
    }
  }

  private async importTable(tableName: string, tableData: TableExport): Promise<number> {
    logger.info('Importing table', { table: tableName, rows: tableData.rowCount });

    let imported = 0;

    for (const row of tableData.rows) {
      try {
        const columns = Object.keys(row);
        const values = Object.values(row);
        const placeholders = values.map((_, i) => `$${i + 1}`).join(', ');

        await this.db.execute(
          `INSERT INTO ${tableName} (${columns.join(', ')})
           VALUES (${placeholders})
           ON CONFLICT DO NOTHING`,
          values
        );

        imported++;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Failed to import row', { table: tableName, error: message });
      }
    }

    return imported;
  }
}
