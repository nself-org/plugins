#!/usr/bin/env node
/**
 * Photos Plugin HTTP Server
 * REST API endpoints for photo album management
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import {
  createLogger,
  createDatabase,
  ApiRateLimiter,
  createRateLimitHook,
  createAuthHook,
  getAppContext,
} from '@nself/plugin-utils';
import { config } from './config.js';
import { PhotosDatabase } from './database.js';
import type {
  HealthCheckResponse,
  ReadyCheckResponse,
  LiveCheckResponse,
  CreateAlbumRequest,
  UpdateAlbumRequest,
  RegisterPhotoRequest,
  BatchRegisterPhotosRequest,
  UpdatePhotoRequest,
  MovePhotoRequest,
  AddTagRequest,
  UpdateFaceRequest,
  MergeFacesRequest,
  SearchPhotosRequest,
} from './types.js';

const logger = createLogger('photos:server');
const PLUGIN_VERSION = '1.0.0';

const fastify = Fastify({ logger: false, bodyLimit: 52428800 }); // 50MB

let photosDb: PhotosDatabase;

// ============================================================================
// Middleware Setup
// ============================================================================

async function setupMiddleware(): Promise<void> {
  await fastify.register(cors, { origin: true });

  const rateLimiter = new ApiRateLimiter(
    config.security.rateLimitMax ?? 100,
    config.security.rateLimitWindowMs ?? 60000
  );
  fastify.addHook('preHandler', createRateLimitHook(rateLimiter));
  fastify.addHook('preHandler', createAuthHook(config.security.apiKey));
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return { status: 'ok', plugin: 'photos', timestamp: new Date().toISOString(), version: PLUGIN_VERSION };
});

fastify.get('/ready', async (): Promise<ReadyCheckResponse> => {
  let dbStatus: 'ok' | 'error' = 'ok';
  try { await photosDb.getStats(); } catch { dbStatus = 'error'; }
  return { ready: dbStatus === 'ok', database: dbStatus, timestamp: new Date().toISOString() };
});

fastify.get('/live', async (): Promise<LiveCheckResponse> => {
  const stats = await photosDb.getStats();
  return {
    alive: true, uptime: process.uptime(),
    memory: { used: process.memoryUsage().heapUsed, total: process.memoryUsage().heapTotal },
    stats,
  };
});

// ============================================================================
// Albums Endpoints
// ============================================================================

fastify.post<{ Body: CreateAlbumRequest }>('/api/albums', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  // Use x-user-id header or default
  const ownerId = (request.headers['x-user-id'] as string) || 'system';

  const album = await scopedDb.createAlbum({
    owner_id: ownerId,
    name: request.body.name,
    description: request.body.description,
    visibility: request.body.visibility,
    visibility_user_ids: request.body.visibilityUserIds,
    sort_order: request.body.sortOrder,
    metadata: request.body.metadata,
  });

  await scopedDb.insertWebhookEvent(
    `photos.album.created-${album.id}`,
    'photos.album.created',
    { albumId: album.id, name: album.name, ownerId }
  );

  return album;
});

fastify.get<{ Querystring: { ownerId?: string; visibility?: string; limit?: string; offset?: string } }>('/api/albums', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);
  const { ownerId, visibility, limit, offset } = request.query;

  return scopedDb.listAlbums(
    ownerId, visibility,
    limit ? parseInt(limit, 10) : 50,
    offset ? parseInt(offset, 10) : 0
  );
});

fastify.get<{ Params: { id: string } }>('/api/albums/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const album = await scopedDb.getAlbum(request.params.id);
  if (!album) { reply.code(404); throw new Error('Album not found'); }

  return album;
});

fastify.put<{ Params: { id: string }; Body: UpdateAlbumRequest }>('/api/albums/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const album = await scopedDb.updateAlbum(request.params.id, {
    name: request.body.name,
    description: request.body.description,
    cover_photo_id: request.body.coverPhotoId,
    visibility: request.body.visibility,
    visibility_user_ids: request.body.visibilityUserIds,
    sort_order: request.body.sortOrder,
  });

  if (!album) { reply.code(404); throw new Error('Album not found'); }
  return album;
});

fastify.delete<{ Params: { id: string } }>('/api/albums/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  await scopedDb.deleteAlbum(request.params.id);

  await scopedDb.insertWebhookEvent(
    `photos.album.deleted-${request.params.id}`,
    'photos.album.deleted',
    { albumId: request.params.id }
  );

  reply.code(204);
});

// ============================================================================
// Photos Endpoints
// ============================================================================

fastify.post<{ Body: RegisterPhotoRequest }>('/api/photos', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);
  const uploaderId = (request.headers['x-user-id'] as string) || 'system';

  const photo = await scopedDb.registerPhoto({
    album_id: request.body.albumId,
    uploader_id: uploaderId,
    file_id: request.body.fileId,
    original_url: request.body.originalUrl,
    original_filename: request.body.originalFilename,
    caption: request.body.caption,
    visibility: request.body.visibility,
  });

  await scopedDb.insertWebhookEvent(
    `photos.photo.uploaded-${photo.id}`,
    'photos.photo.uploaded',
    { photoId: photo.id, uploaderId }
  );

  return { id: photo.id, processingStatus: photo.processing_status };
});

fastify.post<{ Body: BatchRegisterPhotosRequest }>('/api/photos/batch', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);
  const uploaderId = (request.headers['x-user-id'] as string) || 'system';

  if (request.body.photos.length > config.maxUploadBatch) {
    reply.code(400);
    throw new Error(`Maximum batch size is ${config.maxUploadBatch}`);
  }

  const results: Array<{ id: string; processingStatus: string }> = [];

  for (const photoData of request.body.photos) {
    const photo = await scopedDb.registerPhoto({
      album_id: request.body.albumId,
      uploader_id: uploaderId,
      original_url: photoData.originalUrl,
      original_filename: photoData.originalFilename,
      caption: photoData.caption,
    });
    results.push({ id: photo.id, processingStatus: photo.processing_status });
  }

  return { registered: results.length, photos: results };
});

fastify.get<{ Querystring: { albumId?: string; uploaderId?: string; takenFrom?: string; takenTo?: string; limit?: string; offset?: string } }>('/api/photos', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  return scopedDb.listPhotos({
    albumId: request.query.albumId,
    uploaderId: request.query.uploaderId,
    takenFrom: request.query.takenFrom,
    takenTo: request.query.takenTo,
    limit: request.query.limit ? parseInt(request.query.limit, 10) : 50,
    offset: request.query.offset ? parseInt(request.query.offset, 10) : 0,
  });
});

fastify.get<{ Params: { id: string } }>('/api/photos/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const photo = await scopedDb.getPhoto(request.params.id);
  if (!photo) { reply.code(404); throw new Error('Photo not found'); }

  const tags = await scopedDb.getPhotoTags(request.params.id);

  return { ...photo, tags };
});

fastify.put<{ Params: { id: string }; Body: UpdatePhotoRequest }>('/api/photos/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const photo = await scopedDb.updatePhoto(request.params.id, {
    caption: request.body.caption,
    album_id: request.body.albumId,
    visibility: request.body.visibility,
  });

  if (!photo) { reply.code(404); throw new Error('Photo not found'); }
  return photo;
});

fastify.delete<{ Params: { id: string } }>('/api/photos/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  await scopedDb.deletePhoto(request.params.id);

  await scopedDb.insertWebhookEvent(
    `photos.photo.deleted-${request.params.id}`,
    'photos.photo.deleted',
    { photoId: request.params.id }
  );

  reply.code(204);
});

fastify.post<{ Params: { id: string }; Body: MovePhotoRequest }>('/api/photos/:id/move', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const photo = await scopedDb.movePhoto(request.params.id, request.body.albumId);
  if (!photo) { reply.code(404); throw new Error('Photo not found'); }

  return photo;
});

// ============================================================================
// Tags Endpoints
// ============================================================================

fastify.post<{ Params: { id: string }; Body: AddTagRequest }>('/api/photos/:id/tags', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const tag = await scopedDb.addTag(request.params.id, {
    tag_type: request.body.tagType,
    tag_value: request.body.tagValue,
    tagged_user_id: request.body.taggedUserId,
    face_region: request.body.faceRegion,
  });

  await scopedDb.insertWebhookEvent(
    `photos.tag.added-${tag.id}`,
    'photos.tag.added',
    { tagId: tag.id, photoId: request.params.id, tagValue: request.body.tagValue }
  );

  return tag;
});

fastify.delete<{ Params: { id: string; tagId: string } }>('/api/photos/:id/tags/:tagId', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  await scopedDb.removeTag(request.params.tagId);
  reply.code(204);
});

fastify.get<{ Querystring: { type?: string; limit?: string } }>('/api/tags', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const tags = await scopedDb.listTags(
    request.query.type,
    request.query.limit ? parseInt(request.query.limit, 10) : 100
  );

  return { tags };
});

fastify.get<{ Params: { value: string }; Querystring: { limit?: string; offset?: string } }>('/api/tags/:value/photos', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  return scopedDb.getPhotosWithTag(
    request.params.value,
    request.query.limit ? parseInt(request.query.limit, 10) : 50,
    request.query.offset ? parseInt(request.query.offset, 10) : 0
  );
});

// ============================================================================
// Faces Endpoints
// ============================================================================

fastify.get<{ Querystring: { limit?: string; offset?: string } }>('/api/faces', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  return scopedDb.listFaces(
    request.query.limit ? parseInt(request.query.limit, 10) : 50,
    request.query.offset ? parseInt(request.query.offset, 10) : 0
  );
});

fastify.put<{ Params: { id: string }; Body: UpdateFaceRequest }>('/api/faces/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const face = await scopedDb.updateFace(request.params.id, {
    name: request.body.name,
    user_id: request.body.userId,
  });

  if (!face) { reply.code(404); throw new Error('Face not found'); }

  await scopedDb.insertWebhookEvent(
    `photos.face.identified-${face.id}`,
    'photos.face.identified',
    { faceId: face.id, name: request.body.name, userId: request.body.userId }
  );

  return face;
});

fastify.post<{ Params: { id: string }; Body: MergeFacesRequest }>('/api/faces/:id/merge', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const face = await scopedDb.mergeFaces(request.params.id, request.body.mergeWithId);
  if (!face) { reply.code(404); throw new Error('Face not found'); }

  await scopedDb.insertWebhookEvent(
    `photos.face.merged-${face.id}-${request.body.mergeWithId}`,
    'photos.face.merged',
    { targetFaceId: face.id, mergedFaceId: request.body.mergeWithId }
  );

  return face;
});

// ============================================================================
// Timeline Endpoint
// ============================================================================

fastify.get<{ Querystring: { userId?: string; granularity?: string; from?: string; to?: string } }>('/api/timeline', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const periods = await scopedDb.getTimeline(
    request.query.granularity || 'month',
    request.query.from,
    request.query.to,
    request.query.userId
  );

  return { periods };
});

// ============================================================================
// Search Endpoint
// ============================================================================

fastify.post<{ Body: SearchPhotosRequest }>('/api/photos/search', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  return scopedDb.searchPhotos(request.body.query || '', {
    tags: request.body.tags,
    location: request.body.location,
    dateFrom: request.body.dateFrom,
    dateTo: request.body.dateTo,
    uploaderId: request.body.uploaderId,
    albumId: request.body.albumId,
    limit: request.body.limit,
    offset: request.body.offset,
  });
});

// ============================================================================
// Processing Endpoints
// ============================================================================

fastify.post<{ Params: { id: string } }>('/api/photos/:id/process', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const photo = await scopedDb.getPhoto(request.params.id);
  if (!photo) { reply.code(404); throw new Error('Photo not found'); }

  // Mark as processing
  await scopedDb.updatePhotoProcessingStatus(request.params.id, 'processing');

  // In production this would trigger actual EXIF extraction and thumbnail generation.
  // For now, mark as completed with placeholder data.
  await scopedDb.updatePhotoProcessingStatus(request.params.id, 'completed', {});

  await scopedDb.insertWebhookEvent(
    `photos.photo.processed-${request.params.id}`,
    'photos.photo.processed',
    { photoId: request.params.id }
  );

  return { photoId: request.params.id, status: 'completed' };
});

fastify.post('/api/photos/process-pending', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = photosDb.forSourceAccount(sourceAccountId);

  const pending = await scopedDb.getPendingPhotos(config.processingConcurrency);

  let processed = 0;
  for (const photo of pending) {
    try {
      await scopedDb.updatePhotoProcessingStatus(photo.id, 'processing');
      // Placeholder for actual processing
      await scopedDb.updatePhotoProcessingStatus(photo.id, 'completed', {});
      processed++;

      await scopedDb.insertWebhookEvent(
        `photos.photo.processed-${photo.id}`,
        'photos.photo.processed',
        { photoId: photo.id }
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to process photo', { photoId: photo.id, error: message });
      await scopedDb.updatePhotoProcessingStatus(photo.id, 'error');
    }
  }

  return { processed, total: pending.length };
});

// ============================================================================
// Server Startup
// ============================================================================

async function start(): Promise<void> {
  try {
    await setupMiddleware();

    const db = createDatabase(config.database);
    await db.connect();
    photosDb = new PhotosDatabase(db);

    logger.info('Photos database connection established');

    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`Photos plugin server listening on ${config.host}:${config.port}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start photos server', { error: message });
    process.exit(1);
  }
}

process.on('SIGINT', async () => {
  logger.info('Received SIGINT, shutting down...');
  await fastify.close();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  logger.info('Received SIGTERM, shutting down...');
  await fastify.close();
  process.exit(0);
});

const isMainModule = process.argv[1] && (
  process.argv[1].endsWith('server.ts') ||
  process.argv[1].endsWith('server.js')
);

if (isMainModule) {
  start();
}

export { fastify, start };
