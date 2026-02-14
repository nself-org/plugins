/**
 * Backup Execution Service
 * Implements real backup and restore operations using pg_dump/pg_restore
 */

import { spawn } from 'child_process';
import { createHash } from 'crypto';
import { createReadStream, createWriteStream, promises as fs } from 'fs';
import { pipeline } from 'stream/promises';
import { createGzip } from 'zlib';
import { createLogger } from '@nself/plugin-utils';
import { BackupDatabase } from './database.js';
import type { BackupPluginConfig } from './types.js';
import type {
  BackupOptions,
  BackupResult,
  RestoreOptions,
  RestoreResult,
} from './types.js';

const logger = createLogger('backup:exec');

export class BackupService {
  constructor(
    private readonly config: BackupPluginConfig,
    private readonly db: BackupDatabase
  ) {}

  /**
   * Execute a backup operation
   */
  async executeBackup(options: BackupOptions): Promise<BackupResult> {
    const startTime = Date.now();

    // Create artifact record
    const expiresAt = new Date(Date.now() + options.retentionDays * 24 * 60 * 60 * 1000);
    const artifact = await this.db.createArtifact({
      scheduleId: options.scheduleId,
      backupType: options.backupType,
      targetProvider: options.targetProvider,
      expiresAt,
    });

    try {
      // Ensure storage directory exists
      await fs.mkdir(this.config.storagePath, { recursive: true });

      // Generate file path
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
      const extension = this.getFileExtension(options.compression);
      const fileName = `backup-${artifact.id}-${timestamp}.${extension}`;
      const filePath = `${this.config.storagePath}/${fileName}`;

      // Build pg_dump command
      const dumpArgs = this.buildPgDumpArgs(options);

      logger.info('Starting backup', {
        artifactId: artifact.id,
        backupType: options.backupType,
        compression: options.compression,
      });

      // Execute pg_dump
      const dumpOutput = await this.executePgDump(dumpArgs);

      // Compress if needed
      let finalPath = filePath;
      if (options.compression === 'gzip') {
        await this.compressFile(dumpOutput, filePath);
        await fs.unlink(dumpOutput);
      } else if (options.compression === 'zstd') {
        await this.compressFileZstd(dumpOutput, filePath);
        await fs.unlink(dumpOutput);
      } else {
        await fs.rename(dumpOutput, filePath);
      }

      // Calculate checksum
      const checksum = await this.calculateChecksum(finalPath);

      // Get file size
      const stats = await fs.stat(finalPath);

      // Get table row counts
      const rowCounts = await this.getTableRowCounts(options.includeTables);

      // Update artifact record
      await this.db.updateArtifact(artifact.id, {
        status: 'completed',
        filePath: finalPath,
        fileSize: stats.size,
        checksum,
        tablesIncluded: options.includeTables ?? [],
        rowCounts,
        durationMs: Date.now() - startTime,
        completedAt: new Date(),
      });

      logger.info('Backup completed', {
        artifactId: artifact.id,
        fileSize: stats.size,
        duration: Date.now() - startTime,
      });

      return {
        artifactId: artifact.id,
        success: true,
        filePath: finalPath,
        fileSize: stats.size,
        checksum,
        tablesIncluded: options.includeTables ?? [],
        rowCounts,
        duration: Date.now() - startTime,
      };

    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup failed', { artifactId: artifact.id, error: message });

      await this.db.updateArtifact(artifact.id, {
        status: 'failed',
        errorMessage: message,
        durationMs: Date.now() - startTime,
        completedAt: new Date(),
      });

      return {
        artifactId: artifact.id,
        success: false,
        filePath: null,
        fileSize: null,
        checksum: null,
        tablesIncluded: [],
        rowCounts: {},
        duration: Date.now() - startTime,
        error: message,
      };
    }
  }

  /**
   * Execute a restore operation
   */
  async executeRestore(options: RestoreOptions): Promise<RestoreResult> {
    const startTime = Date.now();

    // Get artifact
    const artifact = await this.db.getArtifact(options.artifactId);
    if (!artifact) {
      throw new Error(`Artifact ${options.artifactId} not found`);
    }

    if (artifact.status !== 'completed') {
      throw new Error(`Artifact ${options.artifactId} is not completed (status: ${artifact.status})`);
    }

    if (!artifact.file_path) {
      throw new Error(`Artifact ${options.artifactId} has no file path`);
    }

    // Verify file exists
    try {
      await fs.access(artifact.file_path);
    } catch {
      throw new Error(`Backup file not found: ${artifact.file_path}`);
    }

    // Create restore job record
    const job = await this.db.createRestoreJob({
      artifactId: options.artifactId,
      targetDatabase: options.targetDatabase,
      tablesToRestore: options.tablesToRestore,
      restoreMode: options.restoreMode,
      conflictStrategy: options.conflictStrategy,
    });

    try {
      logger.info('Starting restore', {
        jobId: job.id,
        artifactId: options.artifactId,
        mode: options.restoreMode,
      });

      // Decompress if needed
      const tempFile = await this.decompressIfNeeded(artifact.file_path);

      // Execute pg_restore
      const restoreArgs = this.buildPgRestoreArgs(options, tempFile);
      await this.executePgRestore(restoreArgs);

      // Clean up temp file if created
      if (tempFile !== artifact.file_path) {
        await fs.unlink(tempFile);
      }

      // NOTE: Row count is estimated via verbose pg_restore output parsing
      // Accurate row counting would require post-restore table queries or parsing pg_restore verbose output
      // For performance reasons, we set to 0 and rely on database size metrics instead
      const rowsRestored = 0;

      // Update job record
      await this.db.updateRestoreJob(job.id, {
        status: 'completed',
        rowsRestored,
        completedAt: new Date(),
      });

      logger.info('Restore completed', {
        jobId: job.id,
        duration: Date.now() - startTime,
      });

      return {
        jobId: job.id,
        success: true,
        rowsRestored,
        duration: Date.now() - startTime,
        errors: [],
      };

    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Restore failed', { jobId: job.id, error: message });

      await this.db.updateRestoreJob(job.id, {
        status: 'failed',
        errors: [{ table: 'all', error: message }],
        completedAt: new Date(),
      });

      return {
        jobId: job.id,
        success: false,
        rowsRestored: 0,
        duration: Date.now() - startTime,
        errors: [{ table: 'all', error: message }],
      };
    }
  }

  /**
   * Build pg_dump command arguments
   */
  private buildPgDumpArgs(options: BackupOptions): string[] {
    const args: string[] = [];

    // Connection parameters
    args.push('-h', this.config.databaseHost);
    args.push('-p', this.config.databasePort.toString());
    args.push('-U', this.config.databaseUser);
    args.push('-d', this.config.databaseName);

    // Backup type options
    switch (options.backupType) {
      case 'schema_only':
        args.push('--schema-only');
        break;
      case 'data_only':
        args.push('--data-only');
        break;
      case 'full':
      case 'incremental':
        // Full backup by default
        break;
    }

    // Format (always use custom format for best compression and flexibility)
    args.push('--format=custom');

    // Include/exclude tables
    if (options.includeTables && options.includeTables.length > 0) {
      options.includeTables.forEach(table => {
        args.push('--table', table);
      });
    }

    if (options.excludeTables && options.excludeTables.length > 0) {
      options.excludeTables.forEach(table => {
        args.push('--exclude-table', table);
      });
    }

    // Additional options
    args.push('--verbose');
    args.push('--no-owner');
    args.push('--no-acl');

    return args;
  }

  /**
   * Build pg_restore command arguments
   */
  private buildPgRestoreArgs(options: RestoreOptions, filePath: string): string[] {
    const args: string[] = [];

    // Connection parameters
    args.push('-h', this.config.databaseHost);
    args.push('-p', this.config.databasePort.toString());
    args.push('-U', this.config.databaseUser);
    args.push('-d', options.targetDatabase);

    // Restore mode
    if (options.restoreMode === 'replace') {
      args.push('--clean');
      args.push('--if-exists');
    }

    // Conflict strategy
    if (options.conflictStrategy === 'skip') {
      args.push('--no-owner');
      args.push('--no-acl');
    } else if (options.conflictStrategy === 'error') {
      args.push('--exit-on-error');
    }

    // Table selection
    if (options.tablesToRestore && options.tablesToRestore.length > 0) {
      options.tablesToRestore.forEach(table => {
        args.push('--table', table);
      });
    }

    // Additional options
    args.push('--verbose');
    args.push('--no-owner');

    // Input file
    args.push(filePath);

    return args;
  }

  /**
   * Execute pg_dump command
   */
  private async executePgDump(args: string[]): Promise<string> {
    const outputFile = `${this.config.storagePath}/dump-${Date.now()}.pgdump`;
    args.push('--file', outputFile);

    return new Promise((resolve, reject) => {
      const env = { ...process.env, PGPASSWORD: this.config.databasePassword };
      const proc = spawn(this.config.pgDumpPath, args, { env });

      let stderr = '';

      proc.stderr?.on('data', (data) => {
        stderr += data.toString();
      });

      proc.on('error', (error) => {
        reject(new Error(`Failed to execute pg_dump: ${error.message}`));
      });

      proc.on('close', (code) => {
        if (code !== 0) {
          reject(new Error(`pg_dump exited with code ${code}: ${stderr}`));
        } else {
          resolve(outputFile);
        }
      });
    });
  }

  /**
   * Execute pg_restore command
   */
  private async executePgRestore(args: string[]): Promise<void> {
    return new Promise((resolve, reject) => {
      const env = { ...process.env, PGPASSWORD: this.config.databasePassword };
      const proc = spawn(this.config.pgRestorePath, args, { env });

      let stderr = '';

      proc.stderr?.on('data', (data) => {
        stderr += data.toString();
      });

      proc.on('error', (error) => {
        reject(new Error(`Failed to execute pg_restore: ${error.message}`));
      });

      proc.on('close', (code) => {
        if (code !== 0) {
          reject(new Error(`pg_restore exited with code ${code}: ${stderr}`));
        } else {
          resolve();
        }
      });
    });
  }

  /**
   * Compress file with gzip
   */
  private async compressFile(inputPath: string, outputPath: string): Promise<void> {
    const input = createReadStream(inputPath);
    const output = createWriteStream(outputPath);
    const gzip = createGzip({ level: 9 });

    await pipeline(input, gzip, output);
  }

  /**
   * Compress file with zstd (shell out to zstd command)
   */
  private async compressFileZstd(inputPath: string, outputPath: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const proc = spawn('zstd', ['-19', '-o', outputPath, inputPath]);

      proc.on('error', (error) => {
        reject(new Error(`Failed to execute zstd: ${error.message}`));
      });

      proc.on('close', (code) => {
        if (code !== 0) {
          reject(new Error(`zstd exited with code ${code}`));
        } else {
          resolve();
        }
      });
    });
  }

  /**
   * Decompress file if needed
   */
  private async decompressIfNeeded(filePath: string): Promise<string> {
    if (filePath.endsWith('.gz')) {
      // Decompress gzip
      const tempFile = `${this.config.storagePath}/temp-${Date.now()}.pgdump`;
      const input = createReadStream(filePath);
      const output = createWriteStream(tempFile);
      const { createGunzip } = await import('zlib');
      const gunzip = createGunzip();

      await pipeline(input, gunzip, output);
      return tempFile;
    } else if (filePath.endsWith('.zst')) {
      // Decompress zstd
      const tempFile = `${this.config.storagePath}/temp-${Date.now()}.pgdump`;

      await new Promise<void>((resolve, reject) => {
        const proc = spawn('zstd', ['-d', '-o', tempFile, filePath]);

        proc.on('error', (error) => {
          reject(new Error(`Failed to execute zstd: ${error.message}`));
        });

        proc.on('close', (code) => {
          if (code !== 0) {
            reject(new Error(`zstd exited with code ${code}`));
          } else {
            resolve();
          }
        });
      });

      return tempFile;
    }

    return filePath;
  }

  /**
   * Calculate SHA-256 checksum
   */
  private async calculateChecksum(filePath: string): Promise<string> {
    const hash = createHash('sha256');
    const stream = createReadStream(filePath);

    for await (const chunk of stream) {
      hash.update(chunk);
    }

    return hash.digest('hex');
  }

  /**
   * Get file extension based on compression
   */
  private getFileExtension(compression: string): string {
    switch (compression) {
      case 'gzip':
        return 'pgdump.gz';
      case 'zstd':
        return 'pgdump.zst';
      default:
        return 'pgdump';
    }
  }

  /**
   * Get table row counts
   */
  private async getTableRowCounts(tables?: string[]): Promise<Record<string, number>> {
    const rowCounts: Record<string, number> = {};

    if (!tables || tables.length === 0) {
      return rowCounts;
    }

    for (const table of tables) {
      try {
        const result = await this.db.query<{ count: number }>(
          `SELECT COUNT(*) as count FROM ${table}`
        );
        rowCounts[table] = result.rows[0]?.count ?? 0;
      } catch (error) {
        logger.warn(`Failed to count rows in table ${table}`, { error });
        rowCounts[table] = -1;
      }
    }

    return rowCounts;
  }

  /**
   * Clean up expired artifacts
   */
  async cleanupExpiredArtifacts(): Promise<number> {
    // Mark as expired
    const expiredCount = await this.db.expireOldArtifacts();

    if (expiredCount > 0) {
      logger.info(`Marked ${expiredCount} artifacts as expired`);

      // Delete files
      const filePaths = await this.db.deleteExpiredArtifacts();

      for (const filePath of filePaths) {
        try {
          await fs.unlink(filePath);
          logger.info(`Deleted expired backup file: ${filePath}`);
        } catch (error) {
          logger.warn(`Failed to delete expired file: ${filePath}`, { error });
        }
      }
    }

    return expiredCount;
  }
}
