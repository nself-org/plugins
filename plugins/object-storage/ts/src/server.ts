/**
 * Object Storage Plugin Server
 * HTTP server for uploads, downloads, and API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import multipart from '@fastify/multipart';
import * as crypto from 'crypto';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ObjectStorageDatabase } from './database.js';
import { StorageFactory } from './storage-factory.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateBucketRequest,
  UpdateBucketRequest,
  PresignUploadRequest,
  PresignDownloadRequest,
  MultipartUploadInitRequest,
  ListObjectsRequest,
  AccessAction,
} from './types.js';

const logger = createLogger('object-storage:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new ObjectStorageDatabase();
  await db.connect();
  await db.initializeSchema();

  const storageFactory = new StorageFactory(
    fullConfig.defaultProvider,
    {
      endpoint: fullConfig.s3Endpoint,
      region: fullConfig.s3Region,
      accessKeyId: fullConfig.s3AccessKey,
      secretAccessKey: fullConfig.s3SecretKey,
      bucketPrefix: fullConfig.s3BucketPrefix,
    },
    fullConfig.storageBasePath
  );

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: fullConfig.maxUploadSize,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Register multipart for file uploads
  await app.register(multipart, {
    limits: {
      fileSize: fullConfig.maxUploadSize,
    },
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(fullConfig.rateLimitMax, fullConfig.rateLimitWindowMs);

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): ObjectStorageDatabase {
    return (request as Record<string, unknown>).scopedDb as ObjectStorageDatabase;
  }

  // Log access helper
  async function logAccess(
    request: unknown,
    action: AccessAction,
    bucketId: string | null,
    objectId: string | null,
    status: number,
    bytesTransferred: number | null,
    responseTimeMs: number | null
  ): Promise<void> {
    const req = request as { headers: Record<string, string | undefined>; ip?: string };

    await scopedDb(request).logAccess({
      source_account_id: scopedDb(request).getCurrentSourceAccountId(),
      bucket_id: bucketId,
      object_id: objectId,
      action,
      actor_id: null,
      ip_address: req.ip ?? null,
      user_agent: req.headers['user-agent'] ?? null,
      status,
      response_time_ms: responseTimeMs,
      bytes_transferred: bytesTransferred,
    });
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'object-storage', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'object-storage', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'object-storage',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStorageStats();
    return {
      alive: true,
      plugin: 'object-storage',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        buckets: stats.total_buckets,
        objects: stats.total_objects,
        bytes: stats.total_bytes,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStorageStats();
    return {
      plugin: 'object-storage',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Bucket Endpoints
  // =========================================================================

  app.post<{ Body: CreateBucketRequest }>('/v1/buckets', async (request, reply) => {
    const startTime = Date.now();

    try {
      const { name, provider, provider_config, public_read, cors_origins, max_file_size_bytes, allowed_mime_types, quota_bytes } =
        request.body;

      // Validate bucket name
      if (!name || !/^[a-z0-9-]{3,63}$/.test(name)) {
        return reply.status(400).send({
          error: 'Invalid bucket name. Must be 3-63 characters, lowercase alphanumeric with hyphens.',
        });
      }

      // Check if bucket exists
      const existing = await scopedDb(request).getBucketByName(name);
      if (existing) {
        return reply.status(409).send({ error: 'Bucket already exists' });
      }

      // Create bucket in database
      const bucketId = await scopedDb(request).createBucket({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        name,
        provider: provider ?? fullConfig.defaultProvider,
        provider_config: provider_config ?? {},
        public_read: public_read ?? false,
        cors_origins: cors_origins ?? [],
        max_file_size_bytes: max_file_size_bytes ?? 104857600,
        allowed_mime_types: allowed_mime_types ?? [],
        quota_bytes: quota_bytes ?? null,
        used_bytes: 0,
        object_count: 0,
        lifecycle_rules: [],
      });

      const bucket = await scopedDb(request).getBucketById(bucketId);

      if (!bucket) {
        throw new Error('Failed to retrieve created bucket');
      }

      // Create bucket in storage backend if supported
      const backend = storageFactory.getBackend(bucket);
      if (backend.createBucket) {
        await backend.createBucket(name);
      }

      await logAccess(request, 'upload', bucketId, null, 201, null, Date.now() - startTime);

      return reply.status(201).send(bucket);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create bucket', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/buckets', async (request) => {
    const buckets = await scopedDb(request).listBuckets();
    return { buckets };
  });

  app.get<{ Params: { id: string } }>('/v1/buckets/:id', async (request, reply) => {
    const bucket = await scopedDb(request).getBucketById(request.params.id);

    if (!bucket) {
      return reply.status(404).send({ error: 'Bucket not found' });
    }

    return bucket;
  });

  app.put<{ Params: { id: string }; Body: UpdateBucketRequest }>('/v1/buckets/:id', async (request, reply) => {
    const bucket = await scopedDb(request).getBucketById(request.params.id);

    if (!bucket) {
      return reply.status(404).send({ error: 'Bucket not found' });
    }

    await scopedDb(request).updateBucket(request.params.id, request.body);

    const updated = await scopedDb(request).getBucketById(request.params.id);
    return updated;
  });

  app.delete<{ Params: { id: string } }>('/v1/buckets/:id', async (request, reply) => {
    const bucket = await scopedDb(request).getBucketById(request.params.id);

    if (!bucket) {
      return reply.status(404).send({ error: 'Bucket not found' });
    }

    // Check if bucket is empty
    if (bucket.object_count > 0) {
      return reply.status(400).send({ error: 'Bucket must be empty before deletion' });
    }

    // Delete from storage backend if supported
    const backend = storageFactory.getBackend(bucket);
    if (backend.deleteBucket) {
      await backend.deleteBucket(bucket.name);
    }

    await scopedDb(request).deleteBucket(request.params.id);

    return reply.status(204).send();
  });

  // =========================================================================
  // Object Endpoints
  // =========================================================================

  app.get<{ Params: { id: string }; Querystring: ListObjectsRequest }>('/v1/buckets/:id/objects', async (request, reply) => {
    const bucket = await scopedDb(request).getBucketById(request.params.id);

    if (!bucket) {
      return reply.status(404).send({ error: 'Bucket not found' });
    }

    const objects = await scopedDb(request).listObjects(
      request.params.id,
      request.query.prefix,
      request.query.max_keys ?? 1000
    );

    return {
      objects,
      is_truncated: false,
    };
  });

  app.post<{ Params: { id: string } }>('/v1/buckets/:id/objects', async (request, reply) => {
    const startTime = Date.now();

    try {
      const bucket = await scopedDb(request).getBucketById(request.params.id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const data = await request.file();

      if (!data) {
        return reply.status(400).send({ error: 'No file uploaded' });
      }

      const buffer = await data.toBuffer();

      // Check file size
      if (buffer.length > bucket.max_file_size_bytes) {
        return reply.status(413).send({
          error: `File too large. Maximum size: ${bucket.max_file_size_bytes} bytes`,
        });
      }

      // Check quota
      if (bucket.quota_bytes && bucket.used_bytes + buffer.length > bucket.quota_bytes) {
        return reply.status(507).send({ error: 'Bucket quota exceeded' });
      }

      // Check MIME type
      if (bucket.allowed_mime_types.length > 0 && !bucket.allowed_mime_types.includes(data.mimetype)) {
        return reply.status(415).send({ error: `MIME type ${data.mimetype} not allowed` });
      }

      const key = (data.fields.key as { value: string } | undefined)?.value ?? data.filename;
      const metadata = (data.fields.metadata as { value: string } | undefined)?.value
        ? JSON.parse((data.fields.metadata as { value: string }).value)
        : {};

      // Calculate checksums
      const checksum_sha256 = crypto.createHash('sha256').update(buffer).digest('hex');

      // Upload to storage backend
      const backend = storageFactory.getBackend(bucket);
      const result = await backend.putObject(bucket.name, key, buffer, {
        contentType: data.mimetype,
        metadata,
        checksumSHA256: checksum_sha256,
      });

      // Create object in database
      const objectId = await scopedDb(request).createObject({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        bucket_id: bucket.id,
        key,
        filename: data.filename,
        content_type: data.mimetype,
        size_bytes: buffer.length,
        checksum_sha256,
        etag: result.etag,
        storage_class: 'standard',
        metadata,
        tags: {},
        owner_id: null,
        is_public: bucket.public_read,
        version: 1,
        deleted_at: null,
      });

      // Update bucket usage
      await scopedDb(request).incrementBucketUsage(bucket.id, buffer.length);

      await logAccess(request, 'upload', bucket.id, objectId, 201, buffer.length, Date.now() - startTime);

      const object = await scopedDb(request).getObjectById(objectId);
      return reply.status(201).send(object);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to upload object', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string; key: string } }>('/v1/buckets/:id/objects/:key', async (request, reply) => {
    const bucket = await scopedDb(request).getBucketById(request.params.id);

    if (!bucket) {
      return reply.status(404).send({ error: 'Bucket not found' });
    }

    const object = await scopedDb(request).getObjectByKey(request.params.id, request.params.key);

    if (!object) {
      return reply.status(404).send({ error: 'Object not found' });
    }

    return object;
  });

  app.delete<{ Params: { id: string; key: string } }>('/v1/buckets/:id/objects/:key', async (request, reply) => {
    const startTime = Date.now();

    try {
      const bucket = await scopedDb(request).getBucketById(request.params.id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const object = await scopedDb(request).getObjectByKey(request.params.id, request.params.key);

      if (!object) {
        return reply.status(404).send({ error: 'Object not found' });
      }

      // Delete from storage backend
      const backend = storageFactory.getBackend(bucket);
      await backend.deleteObject(bucket.name, object.key);

      // Mark as deleted in database
      await scopedDb(request).deleteObject(object.id);

      // Update bucket usage
      await scopedDb(request).decrementBucketUsage(bucket.id, object.size_bytes);

      await logAccess(request, 'delete', bucket.id, object.id, 204, null, Date.now() - startTime);

      return reply.status(204).send();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete object', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Presigned URL Endpoints
  // =========================================================================

  app.post<{ Body: PresignUploadRequest }>('/v1/presign/upload', async (request, reply) => {
    try {
      const { bucket_id, key, content_type, expires_in, metadata } = request.body;

      const bucket = await scopedDb(request).getBucketById(bucket_id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const backend = storageFactory.getBackend(bucket);
      const expiresIn = expires_in ?? fullConfig.presignExpirySeconds;

      const url = await backend.presignPutObject(bucket.name, key, expiresIn, {
        contentType: content_type,
        metadata,
      });

      const expiresAt = new Date(Date.now() + expiresIn * 1000);

      // Create upload session
      await scopedDb(request).createUploadSession({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        bucket_id,
        key,
        content_type: content_type ?? 'application/octet-stream',
        total_size_bytes: null,
        upload_type: 'presigned',
        status: 'initiated',
        multipart_upload_id: null,
        parts_completed: 0,
        parts_total: null,
        presigned_url: url,
        presigned_expires_at: expiresAt,
      });

      await logAccess(request, 'presign', bucket_id, null, 200, null, null);

      return {
        url,
        expires_at: expiresAt,
        method: 'PUT',
        headers: content_type ? { 'Content-Type': content_type } : {},
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate presigned upload URL', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Body: PresignDownloadRequest }>('/v1/presign/download', async (request, reply) => {
    try {
      const { bucket_id, key, expires_in } = request.body;

      const bucket = await scopedDb(request).getBucketById(bucket_id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const object = await scopedDb(request).getObjectByKey(bucket_id, key);

      if (!object) {
        return reply.status(404).send({ error: 'Object not found' });
      }

      const backend = storageFactory.getBackend(bucket);
      const expiresIn = expires_in ?? fullConfig.presignExpirySeconds;

      const url = await backend.presignGetObject(bucket.name, key, expiresIn);
      const expiresAt = new Date(Date.now() + expiresIn * 1000);

      await logAccess(request, 'presign', bucket_id, object.id, 200, null, null);

      return {
        url,
        expires_at: expiresAt,
        method: 'GET',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate presigned download URL', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Multipart Upload Endpoints
  // =========================================================================

  app.post<{ Body: MultipartUploadInitRequest }>('/v1/upload-sessions', async (request, reply) => {
    try {
      const { bucket_id, key, content_type, total_size_bytes, metadata } = request.body;

      const bucket = await scopedDb(request).getBucketById(bucket_id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const backend = storageFactory.getBackend(bucket);

      const uploadId = await backend.createMultipartUpload(bucket.name, key, {
        contentType: content_type,
        metadata,
      });

      const sessionId = await scopedDb(request).createUploadSession({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        bucket_id,
        key,
        content_type: content_type ?? 'application/octet-stream',
        total_size_bytes: total_size_bytes ?? null,
        upload_type: 'multipart',
        status: 'uploading',
        multipart_upload_id: uploadId,
        parts_completed: 0,
        parts_total: null,
        presigned_url: null,
        presigned_expires_at: null,
      });

      return {
        session_id: sessionId,
        upload_id: uploadId,
        bucket_id,
        key,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initiate multipart upload', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.put<{ Params: { id: string; partNumber: string } }>('/v1/upload-sessions/:id/parts/:partNumber', async (request, reply) => {
    try {
      const session = await scopedDb(request).getUploadSession(request.params.id);

      if (!session || !session.multipart_upload_id) {
        return reply.status(404).send({ error: 'Upload session not found' });
      }

      const bucket = await scopedDb(request).getBucketById(session.bucket_id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const data = await request.file();

      if (!data) {
        return reply.status(400).send({ error: 'No file part uploaded' });
      }

      const buffer = await data.toBuffer();
      const partNumber = parseInt(request.params.partNumber, 10);

      const backend = storageFactory.getBackend(bucket);
      const etag = await backend.uploadPart(bucket.name, session.key, session.multipart_upload_id, partNumber, buffer);

      // Update session
      await scopedDb(request).updateUploadSession(request.params.id, {
        parts_completed: session.parts_completed + 1,
      });

      return {
        part_number: partNumber,
        etag,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to upload part', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Params: { id: string }; Body: { parts: Array<{ part_number: number; etag: string }> } }>(
    '/v1/upload-sessions/:id/complete',
    async (request, reply) => {
      const startTime = Date.now();

      try {
        const session = await scopedDb(request).getUploadSession(request.params.id);

        if (!session || !session.multipart_upload_id) {
          return reply.status(404).send({ error: 'Upload session not found' });
        }

        const bucket = await scopedDb(request).getBucketById(session.bucket_id);

        if (!bucket) {
          return reply.status(404).send({ error: 'Bucket not found' });
        }

        const backend = storageFactory.getBackend(bucket);
        const result = await backend.completeMultipartUpload(
          bucket.name,
          session.key,
          session.multipart_upload_id,
          request.body.parts.map(p => ({ partNumber: p.part_number, etag: p.etag }))
        );

        // Get final object size by fetching it
        const objectData = await backend.getObject(bucket.name, session.key);

        // Create object in database
        const objectId = await scopedDb(request).createObject({
          source_account_id: scopedDb(request).getCurrentSourceAccountId(),
          bucket_id: bucket.id,
          key: session.key,
          filename: null,
          content_type: session.content_type,
          size_bytes: objectData.contentLength,
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

        // Update bucket usage
        await scopedDb(request).incrementBucketUsage(bucket.id, objectData.contentLength);

        // Update session
        await scopedDb(request).updateUploadSession(request.params.id, {
          status: 'completed',
        });

        await logAccess(request, 'upload', bucket.id, objectId, 200, objectData.contentLength, Date.now() - startTime);

        const object = await scopedDb(request).getObjectById(objectId);
        return object;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to complete multipart upload', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.post<{ Params: { id: string } }>('/v1/upload-sessions/:id/abort', async (request, reply) => {
    try {
      const session = await scopedDb(request).getUploadSession(request.params.id);

      if (!session || !session.multipart_upload_id) {
        return reply.status(404).send({ error: 'Upload session not found' });
      }

      const bucket = await scopedDb(request).getBucketById(session.bucket_id);

      if (!bucket) {
        return reply.status(404).send({ error: 'Bucket not found' });
      }

      const backend = storageFactory.getBackend(bucket);
      await backend.abortMultipartUpload(bucket.name, session.key, session.multipart_upload_id);

      await scopedDb(request).updateUploadSession(request.params.id, {
        status: 'aborted',
      });

      return reply.status(204).send();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to abort multipart upload', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Usage and Stats Endpoints
  // =========================================================================

  app.get<{ Params: { id: string } }>('/v1/buckets/:id/usage', async (request, reply) => {
    const usage = await scopedDb(request).getBucketUsage(request.params.id);

    if (!usage) {
      return reply.status(404).send({ error: 'Bucket not found' });
    }

    return usage;
  });

  app.get<{ Querystring: { bucket_id?: string; action?: string; limit?: number } }>('/v1/access-logs', async (request) => {
    const logs = await scopedDb(request).getAccessLogs({
      bucketId: request.query.bucket_id,
      action: request.query.action,
      limit: request.query.limit,
    });

    return { logs };
  });

  // =========================================================================
  // Lifecycle Endpoint
  // =========================================================================

  app.post('/v1/lifecycle/execute', async () => {
    // Placeholder for lifecycle execution
    // Would iterate through buckets, check rules, and apply them
    return { message: 'Lifecycle execution not yet implemented' };
  });

  // Start server
  const address = await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.info(`Server listening on ${address}`);

  return app;
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  createServer().catch((error) => {
    logger.error('Failed to start server', { error: error.message });
    process.exit(1);
  });
}
