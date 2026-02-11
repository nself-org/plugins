/**
 * Export/Import Plugin Configuration
 */

import 'dotenv/config';
import type { ExportImportConfig } from './types.js';

export type { ExportImportConfig };

export function loadConfig(overrides?: Partial<ExportImportConfig>): ExportImportConfig {
  const config: ExportImportConfig = {
    // Server
    port: parseInt(process.env.EI_PLUGIN_PORT ?? process.env.PORT ?? '3717', 10),
    host: process.env.EI_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Export settings
    exportStoragePath: process.env.EXPORT_STORAGE_PATH ?? '/var/nself/exports',
    exportMaxFileSizeGb: parseInt(process.env.EXPORT_MAX_FILE_SIZE_GB ?? '10', 10),
    exportRetentionDays: parseInt(process.env.EXPORT_RETENTION_DAYS ?? '30', 10),

    // Import settings
    importMaxFileSizeGb: parseInt(process.env.IMPORT_MAX_FILE_SIZE_GB ?? '10', 10),
    importTempPath: process.env.IMPORT_TEMP_PATH ?? '/tmp/nself-imports',

    // Backup settings
    backupStorageBackend: (process.env.BACKUP_STORAGE_BACKEND as ExportImportConfig['backupStorageBackend']) ?? 'local',
    backupRetentionDays: parseInt(process.env.BACKUP_RETENTION_DAYS ?? '90', 10),

    // Migration settings
    migrationBatchSize: parseInt(process.env.MIGRATION_BATCH_SIZE ?? '100', 10),
    migrationRateLimitMs: parseInt(process.env.MIGRATION_RATE_LIMIT_MS ?? '100', 10),

    // Queue settings
    queueConcurrency: parseInt(process.env.EXPORT_IMPORT_QUEUE_CONCURRENCY ?? '5', 10),
    queueTimeoutMinutes: parseInt(process.env.EXPORT_IMPORT_QUEUE_TIMEOUT_MINUTES ?? '120', 10),

    // Compression
    compressionLevel: parseInt(process.env.COMPRESSION_LEVEL ?? '6', 10),
    compressionAlgorithm: (process.env.COMPRESSION_ALGORITHM as ExportImportConfig['compressionAlgorithm']) ?? 'gzip',

    // Security
    apiKey: process.env.EI_API_KEY,
    rateLimitMax: parseInt(process.env.EI_RATE_LIMIT_MAX ?? '100', 10),
    rateLimitWindowMs: parseInt(process.env.EI_RATE_LIMIT_WINDOW_MS ?? '60000', 10),

    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword && process.env.NODE_ENV === 'production') {
    throw new Error('POSTGRES_PASSWORD must be set in production');
  }

  if (config.compressionLevel < 0 || config.compressionLevel > 9) {
    throw new Error('COMPRESSION_LEVEL must be between 0 and 9');
  }

  if (!['local', 's3', 'gcs', 'azure', 'minio'].includes(config.backupStorageBackend)) {
    throw new Error('BACKUP_STORAGE_BACKEND must be one of: local, s3, gcs, azure, minio');
  }

  return config;
}
