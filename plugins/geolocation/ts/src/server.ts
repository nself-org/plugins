#!/usr/bin/env node
/**
 * Geolocation Plugin HTTP Server
 * REST API endpoints for location sharing, history, geofencing, and proximity
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { GeolocationDatabase } from './database.js';
import {
  UpdateLocationRequest,
  BatchLocationRequest,
  GetLatestQuery,
  GetHistoryQuery,
  DeleteHistoryRequest,
  CreateGeofenceRequest,
  UpdateGeofenceRequest,
  NearbyQuery,
  DistanceQuery,
  FenceEventsQuery,
  UserFenceEventsQuery,
  HealthCheckResponse,
} from './types.js';

const logger = createLogger('geolocation:server');

const fastify = Fastify({
  logger: false,
  bodyLimit: 10485760,
});

let geoDb: GeolocationDatabase;

/**
 * Get scoped database for request
 */
function getAppContext(request: { headers: Record<string, string | string[] | undefined> }): string {
  return (request.headers['x-app-id'] as string) || 'primary';
}

function scopedDb(request: { headers: Record<string, string | string[] | undefined> }): GeolocationDatabase {
  return geoDb.forSourceAccount(getAppContext(request));
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return {
    status: 'ok',
    plugin: 'geolocation',
    timestamp: new Date().toISOString(),
    version: '1.0.0',
  };
});

fastify.get('/ready', async () => {
  try {
    await geoDb.getStats();
    return { ready: true, database: 'ok', timestamp: new Date().toISOString() };
  } catch {
    return { ready: false, database: 'error', timestamp: new Date().toISOString() };
  }
});

fastify.get('/live', async () => {
  return {
    alive: true,
    uptime: process.uptime(),
    memory: {
      used: process.memoryUsage().heapUsed,
      total: process.memoryUsage().heapTotal,
    },
  };
});

// ============================================================================
// Location Update Endpoints
// ============================================================================

fastify.post<{ Body: UpdateLocationRequest }>('/api/locations', async (request, reply) => {
  const db = scopedDb(request);
  const body = request.body;

  if (!body.userId || body.latitude === undefined || body.longitude === undefined) {
    reply.code(400);
    throw new Error('userId, latitude, and longitude are required');
  }

  // Store in history
  await db.insertLocation(body);

  // Update latest
  await db.upsertLatest(body);

  // Check geofences if enabled
  let geofenceEvents: Array<{ fenceId: string; fenceName: string; eventType: 'enter' | 'exit' }> = [];
  if (config.geofenceCheckOnUpdate) {
    geofenceEvents = await db.checkGeofences(body.userId, body.latitude, body.longitude);
  }

  const response: Record<string, unknown> = { stored: true };
  if (geofenceEvents.length > 0) {
    response.geofenceEvents = geofenceEvents;
  }

  return response;
});

fastify.post<{ Body: BatchLocationRequest }>('/api/locations/batch', async (request, reply) => {
  const db = scopedDb(request);
  const { userId, deviceId, locations } = request.body;

  if (!userId || !locations || !Array.isArray(locations)) {
    reply.code(400);
    throw new Error('userId and locations array are required');
  }

  if (locations.length > config.batchMaxPoints) {
    reply.code(400);
    throw new Error(`Maximum ${config.batchMaxPoints} locations per batch`);
  }

  let stored = 0;
  for (const loc of locations) {
    await db.insertLocation({
      userId,
      deviceId,
      latitude: loc.latitude,
      longitude: loc.longitude,
      altitude: loc.altitude,
      accuracy: loc.accuracy,
      speed: loc.speed,
      heading: loc.heading,
      batteryLevel: loc.batteryLevel,
      isCharging: loc.isCharging,
      activityType: loc.activityType,
      address: loc.address,
      recordedAt: loc.recordedAt,
      metadata: loc.metadata,
    });
    stored++;
  }

  // Update latest with the most recent location
  if (locations.length > 0) {
    const latest = locations[locations.length - 1];
    await db.upsertLatest({
      userId,
      deviceId,
      latitude: latest.latitude,
      longitude: latest.longitude,
      altitude: latest.altitude,
      accuracy: latest.accuracy,
      speed: latest.speed,
      heading: latest.heading,
      batteryLevel: latest.batteryLevel,
      isCharging: latest.isCharging,
      activityType: latest.activityType,
      address: latest.address,
      recordedAt: latest.recordedAt,
    });
  }

  return { stored, total: locations.length };
});

// ============================================================================
// Current Location Endpoints
// ============================================================================

fastify.get<{ Querystring: GetLatestQuery }>('/api/locations/latest', async (request) => {
  const db = scopedDb(request);
  const userIdsRaw = request.query.userIds || '';
  const userIds = userIdsRaw ? userIdsRaw.split(',').map((s) => s.trim()).filter(Boolean) : [];

  const locations = await db.getLatestByUserIds(userIds);
  return { locations };
});

fastify.get<{ Params: { userId: string } }>('/api/locations/latest/:userId', async (request, reply) => {
  const db = scopedDb(request);
  const location = await db.getLatestByUserId(request.params.userId);

  if (!location) {
    reply.code(404);
    throw new Error('No location found for user');
  }

  return location;
});

// ============================================================================
// History Endpoints
// ============================================================================

fastify.get<{ Querystring: GetHistoryQuery }>('/api/locations/history', async (request, reply) => {
  const db = scopedDb(request);
  const { userId, from, to, limit, offset } = request.query;

  if (!userId) {
    reply.code(400);
    throw new Error('userId is required');
  }

  const result = await db.getHistory(
    userId,
    from,
    to,
    limit ? parseInt(limit) : 1000,
    offset ? parseInt(offset) : 0,
  );

  return result;
});

fastify.delete<{ Body: DeleteHistoryRequest }>('/api/locations/history', async (request, reply) => {
  const db = scopedDb(request);
  const { userId, olderThan } = request.body;

  if (!userId) {
    reply.code(400);
    throw new Error('userId is required');
  }

  const deleted = await db.deleteHistory(userId, olderThan);
  return { deleted };
});

// ============================================================================
// Geofence Endpoints
// ============================================================================

fastify.post<{ Body: CreateGeofenceRequest }>('/api/geofences', async (request, reply) => {
  const db = scopedDb(request);
  const body = request.body;

  if (!body.ownerId || !body.name || body.latitude === undefined || body.longitude === undefined) {
    reply.code(400);
    throw new Error('ownerId, name, latitude, and longitude are required');
  }

  const fence = await db.createFence(body);
  reply.code(201);
  return fence;
});

fastify.get<{ Querystring: { ownerId?: string } }>('/api/geofences', async (request) => {
  const db = scopedDb(request);
  const fences = await db.getFences(request.query.ownerId);
  return { data: fences, total: fences.length };
});

fastify.get<{ Params: { id: string } }>('/api/geofences/:id', async (request, reply) => {
  const db = scopedDb(request);
  const fence = await db.getFenceById(request.params.id);

  if (!fence) {
    reply.code(404);
    throw new Error('Geofence not found');
  }

  return fence;
});

fastify.put<{ Params: { id: string }; Body: UpdateGeofenceRequest }>('/api/geofences/:id', async (request, reply) => {
  const db = scopedDb(request);
  const existing = await db.getFenceById(request.params.id);

  if (!existing) {
    reply.code(404);
    throw new Error('Geofence not found');
  }

  const updated = await db.updateFence(request.params.id, request.body);
  return updated;
});

fastify.delete<{ Params: { id: string } }>('/api/geofences/:id', async (request, reply) => {
  const db = scopedDb(request);
  await db.deleteFence(request.params.id);
  reply.code(204);
});

fastify.post<{ Params: { id: string } }>('/api/geofences/:id/toggle', async (request, reply) => {
  const db = scopedDb(request);
  const existing = await db.getFenceById(request.params.id);

  if (!existing) {
    reply.code(404);
    throw new Error('Geofence not found');
  }

  const toggled = await db.toggleFence(request.params.id);
  return toggled;
});

// ============================================================================
// Geofence Event Endpoints
// ============================================================================

fastify.get<{ Params: { id: string }; Querystring: FenceEventsQuery }>('/api/geofences/:id/events', async (request) => {
  const db = scopedDb(request);
  const { from, to, limit } = request.query;

  const events = await db.getFenceEvents(
    request.params.id,
    from,
    to,
    limit ? parseInt(limit) : 100,
  );

  return { data: events, total: events.length };
});

fastify.get<{ Querystring: UserFenceEventsQuery }>('/api/geofence-events', async (request, reply) => {
  const db = scopedDb(request);
  const { userId, from, to, limit } = request.query;

  if (!userId) {
    reply.code(400);
    throw new Error('userId is required');
  }

  const events = await db.getUserFenceEvents(
    userId,
    from,
    to,
    limit ? parseInt(limit) : 100,
  );

  return { data: events, total: events.length };
});

// ============================================================================
// Proximity Endpoints
// ============================================================================

fastify.get<{ Querystring: NearbyQuery }>('/api/nearby', async (request, reply) => {
  const db = scopedDb(request);
  const { latitude, longitude, radiusMeters, userIds } = request.query;

  if (!latitude || !longitude || !radiusMeters) {
    reply.code(400);
    throw new Error('latitude, longitude, and radiusMeters are required');
  }

  const userIdList = userIds ? userIds.split(',').map((s) => s.trim()).filter(Boolean) : undefined;

  const nearby = await db.findNearby(
    parseFloat(latitude),
    parseFloat(longitude),
    parseFloat(radiusMeters),
    userIdList,
  );

  return { nearby };
});

fastify.get<{ Querystring: DistanceQuery }>('/api/distance', async (request, reply) => {
  const db = scopedDb(request);
  const { userId1, userId2 } = request.query;

  if (!userId1 || !userId2) {
    reply.code(400);
    throw new Error('userId1 and userId2 are required');
  }

  const result = await db.getDistance(userId1, userId2);

  if (!result) {
    reply.code(404);
    throw new Error('Location not found for one or both users');
  }

  return {
    distanceMeters: result.distanceMeters,
    user1Location: {
      latitude: result.user1.latitude,
      longitude: result.user1.longitude,
      recordedAt: result.user1.recorded_at.toISOString(),
    },
    user2Location: {
      latitude: result.user2.latitude,
      longitude: result.user2.longitude,
      recordedAt: result.user2.recorded_at.toISOString(),
    },
  };
});

// ============================================================================
// Server Startup
// ============================================================================

async function start() {
  try {
    await fastify.register(cors, { origin: true });

    const db = createDatabase(config.database);
    await db.connect();
    geoDb = new GeolocationDatabase(db);

    logger.info('Geolocation database connection established');

    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`Geolocation plugin server listening on ${config.host}:${config.port}`);
    logger.info(`Health check: http://${config.host}:${config.port}/health`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start geolocation server', { error: message });
    process.exit(1);
  }
}

// Handle graceful shutdown
process.on('SIGINT', async () => {
  logger.info('Received SIGINT, shutting down gracefully...');
  await fastify.close();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  logger.info('Received SIGTERM, shutting down gracefully...');
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

export { fastify };
