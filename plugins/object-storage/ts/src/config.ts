/**
 * Object Storage Plugin Configuration
 */

import 'dotenv/config';
import type { ObjectStoragePluginConfig, StorageProvider } from './types.js';

export interface Config extends ObjectStoragePluginConfig {}

export function loadConfig(overrides?: Partial<Config>): Config {
  const config: Config = {
    // Server
    port: parseInt(process.env.OS_PLUGIN_PORT ?? process.env.PORT ?? '3301', 10),
    host: process.env.OS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Storage
    storageBasePath: process.env.OS_STORAGE_BASE_PATH ?? '/data/object-storage',
    defaultProvider: (process.env.OS_DEFAULT_PROVIDER ?? 'local') as StorageProvider,

    // S3 Configuration
    s3Endpoint: process.env.OS_S3_ENDPOINT,
    s3Region: process.env.OS_S3_REGION ?? 'us-east-1',
    s3AccessKey: process.env.OS_S3_ACCESS_KEY,
    s3SecretKey: process.env.OS_S3_SECRET_KEY,
    s3BucketPrefix: process.env.OS_S3_BUCKET_PREFIX,

    // Upload Limits
    presignExpirySeconds: parseInt(process.env.OS_PRESIGN_EXPIRY_SECONDS ?? '3600', 10),
    maxUploadSize: parseInt(process.env.OS_MAX_UPLOAD_SIZE ?? '1073741824', 10), // 1GB
    multipartThreshold: parseInt(process.env.OS_MULTIPART_THRESHOLD ?? '104857600', 10), // 100MB

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Security
    apiKey: process.env.OS_API_KEY,
    rateLimitMax: parseInt(process.env.OS_RATE_LIMIT_MAX ?? '200', 10),
    rateLimitWindowMs: parseInt(process.env.OS_RATE_LIMIT_WINDOW_MS ?? '60000', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Apply overrides
    ...overrides,
  };

  // Validation
  const validProviders: StorageProvider[] = ['local', 's3', 'minio', 'r2', 'gcs', 'b2', 'azure'];
  if (!validProviders.includes(config.defaultProvider)) {
    throw new Error(`Invalid default provider: ${config.defaultProvider}. Must be one of: ${validProviders.join(', ')}`);
  }

  // Validate S3-compatible configuration
  if (['s3', 'minio', 'r2', 'gcs', 'b2'].includes(config.defaultProvider)) {
    if (!config.s3AccessKey || !config.s3SecretKey) {
      throw new Error(`S3-compatible provider "${config.defaultProvider}" requires OS_S3_ACCESS_KEY and OS_S3_SECRET_KEY`);
    }
  }

  // Validate local storage path
  if (config.defaultProvider === 'local') {
    if (!config.storageBasePath) {
      throw new Error('Local storage provider requires OS_STORAGE_BASE_PATH');
    }
  }

  return config;
}

export function isS3Compatible(provider: StorageProvider): boolean {
  return ['s3', 'minio', 'r2', 'gcs', 'b2'].includes(provider);
}

export function getProviderEndpoint(provider: StorageProvider, customEndpoint?: string): string | undefined {
  if (customEndpoint) {
    return customEndpoint;
  }

  switch (provider) {
    case 's3':
      return undefined; // Use AWS SDK default
    case 'minio':
      return 'http://localhost:9000';
    case 'r2':
      throw new Error('Cloudflare R2 requires explicit endpoint (e.g., https://<account-id>.r2.cloudflarestorage.com)');
    case 'gcs':
      return 'https://storage.googleapis.com';
    case 'b2':
      throw new Error('Backblaze B2 requires explicit endpoint from your account');
    case 'azure':
      throw new Error('Azure Blob Storage is planned for future release. Currently supported: s3, minio, r2, gcs, b2, local. For Azure now, use S3-compatible API with custom endpoint.');
    case 'local':
      return undefined;
    default:
      throw new Error(`Unsupported provider: ${provider}`);
  }
}
