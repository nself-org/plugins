#!/usr/bin/env node
/**
 * Object Storage Plugin CLI
 * Command-line interface for the object storage plugin
 */

import { Command } from 'commander';
import * as fs from 'fs/promises';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ObjectStorageDatabase } from './database.js';
import { StorageFactory } from './storage-factory.js';
import { createServer } from './server.js';

const logger = createLogger('object-storage:cli');

const program = new Command();

program
  .name('nself-object-storage')
  .description('Object storage plugin for nself - S3-compatible storage with local and cloud backends')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize object storage schema')
  .action(async () => {
    try {
      logger.info('Initializing object storage schema...');

      const db = new ObjectStorageDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      console.log('✓ Schema initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize schema', { error: message });
      console.error('✗ Failed:', message);
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start HTTP server for uploads and API')
  .option('-p, --port <port>', 'Server port', '3301')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      logger.info('Starting object storage server...');

      await createServer({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      // Keep process running
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show storage status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new ObjectStorageDatabase();
      await db.connect();

      const stats = await db.getStorageStats();
      const buckets = await db.listBuckets();

      await db.disconnect();

      console.log('\nObject Storage Status');
      console.log('====================');
      console.log(`Total Buckets:  ${stats.total_buckets}`);
      console.log(`Total Objects:  ${stats.total_objects}`);
      console.log(`Total Size:     ${formatBytes(stats.total_bytes)}`);
      console.log(`Default Provider: ${config.defaultProvider}`);

      if (buckets.length > 0) {
        console.log('\nBuckets:');
        console.log('--------');
        for (const bucket of buckets) {
          const usagePercent = bucket.quota_bytes
            ? ((bucket.used_bytes / bucket.quota_bytes) * 100).toFixed(1)
            : 'N/A';
          console.log(
            `- ${bucket.name} (${bucket.provider}): ${bucket.object_count} objects, ${formatBytes(bucket.used_bytes)}${bucket.quota_bytes ? ` / ${formatBytes(bucket.quota_bytes)} (${usagePercent}%)` : ''}`
          );
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get status', { error: message });
      process.exit(1);
    }
  });

// Buckets command
program
  .command('buckets')
  .description('List all buckets')
  .action(async () => {
    try {
      const db = new ObjectStorageDatabase();
      await db.connect();

      const buckets = await db.listBuckets();

      await db.disconnect();

      if (buckets.length === 0) {
        console.log('No buckets found.');
        process.exit(0);
      }

      console.log('\nBuckets:');
      console.log('========');
      for (const bucket of buckets) {
        console.log(`\nName:     ${bucket.name}`);
        console.log(`ID:       ${bucket.id}`);
        console.log(`Provider: ${bucket.provider}`);
        console.log(`Objects:  ${bucket.object_count}`);
        console.log(`Size:     ${formatBytes(bucket.used_bytes)}`);
        console.log(`Quota:    ${bucket.quota_bytes ? formatBytes(bucket.quota_bytes) : 'Unlimited'}`);
        console.log(`Public:   ${bucket.public_read ? 'Yes' : 'No'}`);
        console.log(`Created:  ${bucket.created_at}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list buckets', { error: message });
      process.exit(1);
    }
  });

// Objects command
program
  .command('objects')
  .description('List objects in a bucket')
  .requiredOption('-b, --bucket <name>', 'Bucket name')
  .option('-p, --prefix <prefix>', 'Key prefix filter')
  .option('-l, --limit <limit>', 'Maximum number of objects', '100')
  .action(async (options) => {
    try {
      const db = new ObjectStorageDatabase();
      await db.connect();

      const bucket = await db.getBucketByName(options.bucket);

      if (!bucket) {
        console.error(`Bucket "${options.bucket}" not found.`);
        process.exit(1);
      }

      const objects = await db.listObjects(bucket.id, options.prefix, parseInt(options.limit, 10));

      await db.disconnect();

      if (objects.length === 0) {
        console.log('No objects found.');
        process.exit(0);
      }

      console.log(`\nObjects in ${bucket.name}:`);
      console.log('='.repeat(50));
      for (const obj of objects) {
        console.log(`\nKey:          ${obj.key}`);
        console.log(`Filename:     ${obj.filename ?? 'N/A'}`);
        console.log(`Size:         ${formatBytes(obj.size_bytes)}`);
        console.log(`Type:         ${obj.content_type}`);
        console.log(`Storage:      ${obj.storage_class}`);
        console.log(`Created:      ${obj.created_at}`);
      }

      console.log(`\nTotal: ${objects.length} objects`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list objects', { error: message });
      process.exit(1);
    }
  });

// Upload command
program
  .command('upload')
  .description('Upload a file to storage')
  .requiredOption('-b, --bucket <name>', 'Bucket name')
  .requiredOption('-f, --file <path>', 'File path to upload')
  .option('-k, --key <key>', 'Object key (defaults to filename)')
  .option('-t, --content-type <type>', 'Content type')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new ObjectStorageDatabase();
      await db.connect();

      const bucket = await db.getBucketByName(options.bucket);

      if (!bucket) {
        console.error(`Bucket "${options.bucket}" not found.`);
        process.exit(1);
      }

      const storageFactory = new StorageFactory(
        config.defaultProvider,
        {
          endpoint: config.s3Endpoint,
          region: config.s3Region,
          accessKeyId: config.s3AccessKey,
          secretAccessKey: config.s3SecretKey,
          bucketPrefix: config.s3BucketPrefix,
        },
        config.storageBasePath
      );

      const backend = storageFactory.getBackend(bucket);

      // Read file
      const buffer = await fs.readFile(options.file);
      const key = options.key ?? options.file.split('/').pop();

      if (!key) {
        throw new Error('Could not determine object key');
      }

      // Upload
      console.log(`Uploading ${formatBytes(buffer.length)} to ${bucket.name}/${key}...`);

      const result = await backend.putObject(bucket.name, key, buffer, {
        contentType: options.contentType,
      });

      // Create object record
      const objectId = await db.createObject({
        source_account_id: db.getCurrentSourceAccountId(),
        bucket_id: bucket.id,
        key,
        filename: options.file.split('/').pop() ?? null,
        content_type: options.contentType ?? 'application/octet-stream',
        size_bytes: buffer.length,
        checksum_sha256: null,
        etag: result.etag,
        storage_class: 'standard',
        metadata: {},
        tags: {},
        owner_id: null,
        is_public: bucket.public_read,
        version: 1,
        deleted_at: null,
      });

      await db.incrementBucketUsage(bucket.id, buffer.length);
      await db.disconnect();

      console.log(`✓ Uploaded successfully (ID: ${objectId})`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Upload failed', { error: message });
      console.error('✗ Failed:', message);
      process.exit(1);
    }
  });

// Download command
program
  .command('download')
  .description('Download a file from storage')
  .requiredOption('-b, --bucket <name>', 'Bucket name')
  .requiredOption('-k, --key <key>', 'Object key')
  .requiredOption('-o, --output <path>', 'Output file path')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new ObjectStorageDatabase();
      await db.connect();

      const bucket = await db.getBucketByName(options.bucket);

      if (!bucket) {
        console.error(`Bucket "${options.bucket}" not found.`);
        process.exit(1);
      }

      const object = await db.getObjectByKey(bucket.id, options.key);

      if (!object) {
        console.error(`Object "${options.key}" not found in bucket "${options.bucket}".`);
        process.exit(1);
      }

      const storageFactory = new StorageFactory(
        config.defaultProvider,
        {
          endpoint: config.s3Endpoint,
          region: config.s3Region,
          accessKeyId: config.s3AccessKey,
          secretAccessKey: config.s3SecretKey,
          bucketPrefix: config.s3BucketPrefix,
        },
        config.storageBasePath
      );

      const backend = storageFactory.getBackend(bucket);

      console.log(`Downloading ${formatBytes(object.size_bytes)} from ${bucket.name}/${options.key}...`);

      const result = await backend.getObject(bucket.name, options.key);

      await fs.writeFile(options.output, result.data);
      await db.disconnect();

      console.log(`✓ Downloaded successfully to ${options.output}`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Download failed', { error: message });
      console.error('✗ Failed:', message);
      process.exit(1);
    }
  });

// Presign command
program
  .command('presign')
  .description('Generate presigned URL')
  .requiredOption('-b, --bucket <name>', 'Bucket name')
  .requiredOption('-k, --key <key>', 'Object key')
  .option('-m, --method <method>', 'HTTP method (GET or PUT)', 'GET')
  .option('-e, --expires <seconds>', 'Expiry time in seconds', '3600')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new ObjectStorageDatabase();
      await db.connect();

      const bucket = await db.getBucketByName(options.bucket);

      if (!bucket) {
        console.error(`Bucket "${options.bucket}" not found.`);
        process.exit(1);
      }

      const storageFactory = new StorageFactory(
        config.defaultProvider,
        {
          endpoint: config.s3Endpoint,
          region: config.s3Region,
          accessKeyId: config.s3AccessKey,
          secretAccessKey: config.s3SecretKey,
          bucketPrefix: config.s3BucketPrefix,
        },
        config.storageBasePath
      );

      const backend = storageFactory.getBackend(bucket);
      const expiresIn = parseInt(options.expires, 10);

      let url: string;

      if (options.method.toUpperCase() === 'PUT') {
        url = await backend.presignPutObject(bucket.name, options.key, expiresIn);
      } else {
        url = await backend.presignGetObject(bucket.name, options.key, expiresIn);
      }

      await db.disconnect();

      console.log('\nPresigned URL:');
      console.log('==============');
      console.log(`Method:  ${options.method.toUpperCase()}`);
      console.log(`Bucket:  ${bucket.name}`);
      console.log(`Key:     ${options.key}`);
      console.log(`Expires: ${expiresIn} seconds`);
      console.log(`\nURL:\n${url}`);

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate presigned URL', { error: message });
      console.error('✗ Failed:', message);
      process.exit(1);
    }
  });

// Usage command
program
  .command('usage')
  .description('Show storage usage for a bucket')
  .requiredOption('-b, --bucket <name>', 'Bucket name')
  .action(async (options) => {
    try {
      const db = new ObjectStorageDatabase();
      await db.connect();

      const bucket = await db.getBucketByName(options.bucket);

      if (!bucket) {
        console.error(`Bucket "${options.bucket}" not found.`);
        process.exit(1);
      }

      const usage = await db.getBucketUsage(bucket.id);

      await db.disconnect();

      if (!usage) {
        console.error('Failed to retrieve usage stats.');
        process.exit(1);
      }

      console.log(`\nBucket Usage: ${usage.bucket_name}`);
      console.log('='.repeat(50));
      console.log(`Objects:      ${usage.object_count}`);
      console.log(`Total Size:   ${formatBytes(usage.total_bytes)}`);
      console.log(`Quota:        ${usage.quota_bytes ? formatBytes(usage.quota_bytes) : 'Unlimited'}`);

      if (usage.quota_used_percent !== null) {
        console.log(`Usage:        ${usage.quota_used_percent.toFixed(2)}%`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get usage', { error: message });
      process.exit(1);
    }
  });

// Lifecycle command (planned feature)
program
  .command('lifecycle')
  .description('Manage lifecycle rules (planned feature)')
  .action(async () => {
    console.log('Lifecycle management is planned for a future release.');
    console.log('Use provider-specific tools for now:');
    console.log('  - S3: aws s3api put-bucket-lifecycle-configuration');
    console.log('  - MinIO: mc ilm add');
    console.log('  - R2: Cloudflare dashboard > R2 > bucket settings');
    console.log('  - GCS: gsutil lifecycle set lifecycle.json gs://bucket');
    process.exit(0);
  });

program.parse();

// Helper function
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';

  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}
