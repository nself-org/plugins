/**
 * Configuration loader for file processing plugin
 */

import { createLogger } from '@nself/plugin-utils';
import { config as loadEnv } from 'dotenv';
import type { FileProcessingConfig, StorageProvider } from './types.js';

const logger = createLogger('file-processing:config');

// Load environment variables
loadEnv();

/**
 * Parse thumbnail sizes from environment variable
 */
function parseThumbnailSizes(value?: string): number[] {
  if (!value) return [100, 400, 1200];

  return value
    .split(',')
    .map(s => parseInt(s.trim(), 10))
    .filter(n => !isNaN(n) && n > 0);
}

/**
 * Parse allowed file types
 */
function parseAllowedTypes(value?: string): string[] {
  if (!value) return [];

  return value
    .split(',')
    .map(s => s.trim())
    .filter(s => s.length > 0);
}

/**
 * Load configuration from environment variables
 */
export function loadConfig(): FileProcessingConfig {
  const storageProvider = (process.env.FILE_STORAGE_PROVIDER || 'minio') as StorageProvider;

  if (!['minio', 's3', 'gcs', 'r2', 'b2', 'azure'].includes(storageProvider)) {
    throw new Error(`Invalid FILE_STORAGE_PROVIDER: ${storageProvider}`);
  }

  const storageBucket = process.env.FILE_STORAGE_BUCKET;
  if (!storageBucket) {
    throw new Error('FILE_STORAGE_BUCKET is required');
  }

  return {
    // Storage
    storageProvider,
    storageBucket,
    storageEndpoint: process.env.FILE_STORAGE_ENDPOINT,
    storageRegion: process.env.FILE_STORAGE_REGION || 'us-east-1',
    storageAccessKey: process.env.FILE_STORAGE_ACCESS_KEY,
    storageSecretKey: process.env.FILE_STORAGE_SECRET_KEY,
    azureConnectionString: process.env.AZURE_STORAGE_CONNECTION_STRING,
    googleCredentials: process.env.GOOGLE_APPLICATION_CREDENTIALS,

    // Processing
    thumbnailSizes: parseThumbnailSizes(process.env.FILE_THUMBNAIL_SIZES),
    enableVirusScan: process.env.FILE_ENABLE_VIRUS_SCAN === 'true',
    enableOptimization: process.env.FILE_ENABLE_OPTIMIZATION !== 'false',
    maxFileSize: parseInt(process.env.FILE_MAX_SIZE || '104857600', 10),
    allowedTypes: parseAllowedTypes(process.env.FILE_ALLOWED_TYPES),
    stripExif: process.env.FILE_STRIP_EXIF !== 'false',
    queueConcurrency: parseInt(process.env.FILE_QUEUE_CONCURRENCY || '3', 10),

    // ClamAV
    clamavHost: process.env.CLAMAV_HOST || 'localhost',
    clamavPort: parseInt(process.env.CLAMAV_PORT || '3310', 10),

    // Queue
    redisUrl: process.env.REDIS_URL || 'redis://redis:6379',

    // Server
    port: parseInt(process.env.PORT || '3104', 10),
    host: process.env.HOST || '0.0.0.0',
    logLevel: (process.env.LOG_LEVEL || 'info') as 'debug' | 'info' | 'warn' | 'error',
  };
}

/**
 * Validate configuration
 */
export function validateConfig(config: FileProcessingConfig): void {
  // Validate storage configuration
  if (['minio', 's3', 'r2', 'b2'].includes(config.storageProvider)) {
    if (!config.storageAccessKey || !config.storageSecretKey) {
      throw new Error(`${config.storageProvider} requires FILE_STORAGE_ACCESS_KEY and FILE_STORAGE_SECRET_KEY`);
    }
  }

  if (config.storageProvider === 'azure' && !config.azureConnectionString) {
    throw new Error('Azure requires AZURE_STORAGE_CONNECTION_STRING');
  }

  if (config.storageProvider === 'gcs' && !config.googleCredentials) {
    throw new Error('GCS requires GOOGLE_APPLICATION_CREDENTIALS');
  }

  // Validate thumbnail sizes
  if (config.thumbnailSizes.length === 0) {
    throw new Error('At least one thumbnail size must be configured');
  }

  // Validate file size
  if (config.maxFileSize <= 0) {
    throw new Error('FILE_MAX_SIZE must be greater than 0');
  }

  // Validate queue concurrency
  if (config.queueConcurrency <= 0) {
    throw new Error('FILE_QUEUE_CONCURRENCY must be greater than 0');
  }
}

/**
 * Get database configuration
 */
export function getDatabaseConfig() {
  return {
    host: process.env.POSTGRES_HOST ?? 'localhost',
    port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: process.env.POSTGRES_DB ?? 'nself',
    user: process.env.POSTGRES_USER ?? 'postgres',
    password: process.env.POSTGRES_PASSWORD ?? '',
    ssl: process.env.POSTGRES_SSL === 'true',
  };
}
