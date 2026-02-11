/**
 * Storage Backend Factory
 * Creates appropriate storage backend based on provider configuration
 */

import { createLogger } from '@nself/plugin-utils';
import type { StorageBackend, StorageProvider, ProviderConfig, BucketRecord } from './types.js';
import { LocalStorageBackend } from './storage-local.js';
import { S3StorageBackend } from './storage-s3.js';
import { getProviderEndpoint, isS3Compatible } from './config.js';

const logger = createLogger('object-storage:factory');

export class StorageFactory {
  private backends: Map<string, StorageBackend> = new Map();
  private defaultProvider: StorageProvider;
  private defaultConfig: ProviderConfig;
  private localBasePath: string;

  constructor(defaultProvider: StorageProvider, defaultConfig: ProviderConfig, localBasePath: string) {
    this.defaultProvider = defaultProvider;
    this.defaultConfig = defaultConfig;
    this.localBasePath = localBasePath;
  }

  /**
   * Get storage backend for a bucket
   */
  getBackend(bucket: BucketRecord): StorageBackend {
    const cacheKey = `${bucket.provider}-${bucket.id}`;

    let backend = this.backends.get(cacheKey);

    if (!backend) {
      backend = this.createBackend(bucket.provider, bucket.provider_config);
      this.backends.set(cacheKey, backend);
    }

    return backend;
  }

  /**
   * Get default storage backend
   */
  getDefaultBackend(): StorageBackend {
    const cacheKey = `default-${this.defaultProvider}`;

    let backend = this.backends.get(cacheKey);

    if (!backend) {
      backend = this.createBackend(this.defaultProvider, this.defaultConfig);
      this.backends.set(cacheKey, backend);
    }

    return backend;
  }

  /**
   * Create a new storage backend instance
   */
  private createBackend(provider: StorageProvider, config: ProviderConfig): StorageBackend {
    logger.info('Creating storage backend', { provider });

    if (provider === 'local') {
      return new LocalStorageBackend(this.localBasePath);
    }

    if (isS3Compatible(provider)) {
      const endpoint = getProviderEndpoint(provider, config.endpoint ?? this.defaultConfig.endpoint);

      const s3Config: ProviderConfig = {
        endpoint,
        region: config.region ?? this.defaultConfig.region,
        accessKeyId: config.accessKeyId ?? this.defaultConfig.accessKeyId,
        secretAccessKey: config.secretAccessKey ?? this.defaultConfig.secretAccessKey,
        bucketPrefix: config.bucketPrefix ?? this.defaultConfig.bucketPrefix,
        forcePathStyle: config.forcePathStyle ?? this.defaultConfig.forcePathStyle,
      };

      return new S3StorageBackend(provider, s3Config);
    }

    throw new Error(`Unsupported storage provider: ${provider}`);
  }

  /**
   * Clear cached backends (useful for testing)
   */
  clearCache(): void {
    this.backends.clear();
  }
}
