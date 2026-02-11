/**
 * Backup Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig } from '@nself/plugin-utils';
import type { BackupPluginConfig } from './types.js';

export function loadConfig(overrides?: Partial<BackupPluginConfig>): BackupPluginConfig {
  const security = loadSecurityConfig('BACKUP');

  const config: BackupPluginConfig = {
    // Server
    port: parseInt(process.env.BACKUP_PLUGIN_PORT ?? process.env.PORT ?? '3013', 10),
    host: process.env.BACKUP_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Storage
    storagePath: process.env.BACKUP_STORAGE_PATH ?? '/tmp/nself-backups',
    s3Endpoint: process.env.BACKUP_S3_ENDPOINT,
    s3Bucket: process.env.BACKUP_S3_BUCKET,
    s3AccessKey: process.env.BACKUP_S3_ACCESS_KEY,
    s3SecretKey: process.env.BACKUP_S3_SECRET_KEY,
    s3Region: process.env.BACKUP_S3_REGION ?? 'us-east-1',

    // Encryption
    encryptionKey: process.env.BACKUP_ENCRYPTION_KEY,

    // Backup settings
    defaultRetentionDays: parseInt(process.env.BACKUP_DEFAULT_RETENTION_DAYS ?? '30', 10),
    maxConcurrent: parseInt(process.env.BACKUP_MAX_CONCURRENT ?? '2', 10),
    pgDumpPath: process.env.BACKUP_PG_DUMP_PATH ?? 'pg_dump',
    pgRestorePath: process.env.BACKUP_PG_RESTORE_PATH ?? 'pg_restore',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security: {
      apiKey: security.apiKey,
      rateLimitMax: security.rateLimitMax ?? 100,
      rateLimitWindowMs: security.rateLimitWindowMs ?? 60000,
    },

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword && !process.env.DATABASE_URL) {
    throw new Error('Either POSTGRES_PASSWORD or DATABASE_URL must be set');
  }

  return config;
}

export function getDatabaseUrl(config: BackupPluginConfig): string {
  if (process.env.DATABASE_URL) {
    return process.env.DATABASE_URL;
  }

  const sslParam = config.databaseSsl ? '?sslmode=require' : '';
  return `postgresql://${config.databaseUser}:${config.databasePassword}@${config.databaseHost}:${config.databasePort}/${config.databaseName}${sslParam}`;
}
