/**
 * Object Storage Plugin for nself
 * Multi-provider object storage with S3-compatible API
 */

export { ObjectStorageDatabase } from './database.js';
export { StorageFactory } from './storage-factory.js';
export { LocalStorageBackend } from './storage-local.js';
export { S3StorageBackend } from './storage-s3.js';
export { createServer } from './server.js';
export { loadConfig, isS3Compatible, getProviderEndpoint } from './config.js';
export * from './types.js';
