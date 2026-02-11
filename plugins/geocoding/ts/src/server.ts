/**
 * Geocoding Plugin Server
 * HTTP server for geocoding, geofencing, and place search API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { GeocodingDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  ForwardGeocodeRequest,
  ReverseGeocodeRequest,
  PlaceSearchRequest,
  AutocompleteRequest,
  BatchGeocodeRequest,
  CreateGeofenceRequest,
  UpdateGeofenceRequest,
  EvaluateGeofenceRequest,
  ClearCacheRequest,
  GeoResult,
} from './types.js';

const logger = createLogger('geocoding:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new GeocodingDatabase(undefined, 'primary', fullConfig.cacheTtlDays);

  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 5 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 500,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): GeocodingDatabase {
    return (request as Record<string, unknown>).scopedDb as GeocodingDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'geocoding', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'geocoding', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'geocoding',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return {
      alive: true,
      plugin: 'geocoding',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        cacheEntries: stats.total_cache_entries,
        geofences: stats.active_geofences,
        places: stats.total_places,
        cacheHitRate: stats.cache_hit_rate,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Geocoding Endpoints
  // =========================================================================

  app.post('/api/geocode', async (request, reply) => {
    try {
      const body = request.body as ForwardGeocodeRequest;

      if (!body.address) {
        return reply.status(400).send({ error: 'Address is required' });
      }

      const queryText = [body.address, body.city, body.state, body.country]
        .filter(Boolean)
        .join(', ');

      // Check cache first
      if (fullConfig.cacheEnabled) {
        const cached = await scopedDb(request).getCachedGeocodeAnyProvider('forward', queryText);
        if (cached) {
          const result: GeoResult = {
            lat: cached.lat ?? 0,
            lng: cached.lng ?? 0,
            formatted_address: cached.formatted_address,
            street_number: cached.street_number,
            street_name: cached.street_name,
            city: cached.city,
            state: cached.state,
            state_code: cached.state_code,
            country: cached.country,
            country_code: cached.country_code,
            postal_code: cached.postal_code,
            place_id: cached.place_id,
            place_type: cached.place_type,
            accuracy: cached.accuracy as GeoResult['accuracy'],
            provider: cached.provider,
            cached: true,
          };
          return { data: [result] };
        }
      }

      // Placeholder: would call external provider here
      // For now, return empty with a message
      return {
        data: [],
        message: 'External provider integration pending - configure GEOCODING_PROVIDERS',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Forward geocode failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/reverse', async (request, reply) => {
    try {
      const body = request.body as ReverseGeocodeRequest;

      if (body.lat === undefined || body.lng === undefined) {
        return reply.status(400).send({ error: 'lat and lng are required' });
      }

      const queryText = `${body.lat},${body.lng}`;

      // Check cache first
      if (fullConfig.cacheEnabled) {
        const cached = await scopedDb(request).getCachedGeocodeAnyProvider('reverse', queryText);
        if (cached) {
          const result: GeoResult = {
            lat: cached.lat ?? 0,
            lng: cached.lng ?? 0,
            formatted_address: cached.formatted_address,
            street_number: cached.street_number,
            street_name: cached.street_name,
            city: cached.city,
            state: cached.state,
            state_code: cached.state_code,
            country: cached.country,
            country_code: cached.country_code,
            postal_code: cached.postal_code,
            place_id: cached.place_id,
            place_type: cached.place_type,
            accuracy: cached.accuracy as GeoResult['accuracy'],
            provider: cached.provider,
            cached: true,
          };
          return { data: [result] };
        }
      }

      return {
        data: [],
        message: 'External provider integration pending - configure GEOCODING_PROVIDERS',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Reverse geocode failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/search', async (request, reply) => {
    try {
      const body = request.body as PlaceSearchRequest;

      if (!body.query) {
        return reply.status(400).send({ error: 'Query is required' });
      }

      // Search local places first
      const places = await scopedDb(request).searchPlaces({
        query: body.query,
        lat: body.lat,
        lng: body.lng,
        radius: body.radius,
        category: body.category,
        limit: body.limit,
      });

      return { data: places, total: places.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Place search failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/autocomplete', async (request, reply) => {
    try {
      const body = request.body as AutocompleteRequest;

      if (!body.input) {
        return reply.status(400).send({ error: 'Input is required' });
      }

      // Placeholder for autocomplete provider integration
      return {
        data: [],
        message: 'Autocomplete provider integration pending',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Autocomplete failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/api/batch', async (request, reply) => {
    try {
      const body = request.body as BatchGeocodeRequest;

      if (!body.addresses || !Array.isArray(body.addresses)) {
        return reply.status(400).send({ error: 'addresses array is required' });
      }

      if (body.addresses.length > fullConfig.maxBatchSize) {
        return reply.status(400).send({
          error: `Batch size exceeds maximum of ${fullConfig.maxBatchSize}`,
        });
      }

      const results: { address: string; result: GeoResult | null; error?: string }[] = [];

      for (const address of body.addresses) {
        try {
          // Check cache
          const cached = await scopedDb(request).getCachedGeocodeAnyProvider('forward', address);
          if (cached) {
            results.push({
              address,
              result: {
                lat: cached.lat ?? 0,
                lng: cached.lng ?? 0,
                formatted_address: cached.formatted_address,
                street_number: cached.street_number,
                street_name: cached.street_name,
                city: cached.city,
                state: cached.state,
                state_code: cached.state_code,
                country: cached.country,
                country_code: cached.country_code,
                postal_code: cached.postal_code,
                place_id: cached.place_id,
                place_type: cached.place_type,
                accuracy: cached.accuracy as GeoResult['accuracy'],
                provider: cached.provider,
                cached: true,
              },
            });
          } else {
            results.push({ address, result: null, error: 'Not cached and provider not configured' });
          }
        } catch (error) {
          const errMsg = error instanceof Error ? error.message : 'Unknown error';
          results.push({ address, result: null, error: errMsg });
        }
      }

      return {
        data: results,
        total: results.length,
        cached: results.filter(r => r.result?.cached).length,
        failed: results.filter(r => r.error).length,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Batch geocode failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Geofence Endpoints
  // =========================================================================

  app.get('/api/geofences', async (request) => {
    const query = request.query as {
      active?: string;
      near_lat?: string;
      near_lng?: string;
      radius?: string;
    };

    const geofences = await scopedDb(request).listGeofences({
      active: query.active !== undefined ? query.active === 'true' : undefined,
      near_lat: query.near_lat ? parseFloat(query.near_lat) : undefined,
      near_lng: query.near_lng ? parseFloat(query.near_lng) : undefined,
      radius: query.radius ? parseFloat(query.radius) : undefined,
    });

    return { data: geofences, total: geofences.length };
  });

  app.post('/api/geofences', async (request, reply) => {
    try {
      const body = request.body as CreateGeofenceRequest;

      if (!body.name || body.center_lat === undefined || body.center_lng === undefined) {
        return reply.status(400).send({ error: 'name, center_lat, and center_lng are required' });
      }

      const geofence = await scopedDb(request).createGeofence(body);
      return reply.status(201).send(geofence);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create geofence failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.put('/api/geofences/:id', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const body = request.body as UpdateGeofenceRequest;

      const geofence = await scopedDb(request).updateGeofence(id, body);
      if (!geofence) {
        return reply.status(404).send({ error: 'Geofence not found' });
      }

      return geofence;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Update geofence failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.delete('/api/geofences/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteGeofence(id);
    if (!deleted) {
      return reply.status(404).send({ error: 'Geofence not found' });
    }
    return { deleted: true };
  });

  app.post('/api/geofences/evaluate', async (request, reply) => {
    try {
      const body = request.body as EvaluateGeofenceRequest;

      if (body.lat === undefined || body.lng === undefined) {
        return reply.status(400).send({ error: 'lat and lng are required' });
      }

      const evaluations = await scopedDb(request).evaluateGeofences(body.lat, body.lng);

      // Record events for geofences the point is inside
      const insideFences = evaluations.filter(e => e.inside);
      for (const evaluation of insideFences) {
        if (body.entity_id) {
          await scopedDb(request).insertGeofenceEvent(
            evaluation.geofence.id,
            'enter',
            body.entity_id,
            body.entity_type ?? 'user',
            body.lat,
            body.lng
          );
        }
      }

      return {
        data: evaluations.map(e => ({
          geofence_id: e.geofence.id,
          geofence_name: e.geofence.name,
          inside: e.inside,
          distance_meters: e.distance_meters,
        })),
        inside_count: insideFences.length,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evaluate geofence failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/api/geofences/:id/events', async (request) => {
    const { id } = request.params as { id: string };
    const { from, to, entity_id } = request.query as {
      from?: string;
      to?: string;
      entity_id?: string;
    };

    const events = await scopedDb(request).getGeofenceEvents(id, { from, to, entity_id });
    return { data: events, total: events.length };
  });

  // =========================================================================
  // Cache Endpoints
  // =========================================================================

  app.get('/api/cache/stats', async (request) => {
    const stats = await scopedDb(request).getCacheStats();
    return stats;
  });

  app.post('/api/cache/clear', async (request) => {
    const body = (request.body as ClearCacheRequest) ?? {};
    const cleared = await scopedDb(request).clearCache(body.older_than_days);
    return { cleared, older_than_days: body.older_than_days ?? null };
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getPluginStats();
    return stats;
  });

  // =========================================================================
  // Graceful Shutdown
  // =========================================================================

  const shutdown = async () => {
    logger.info('Shutting down...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Geocoding plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info(`Providers: ${fullConfig.providers.join(', ')}`);
      logger.info(`Cache enabled: ${fullConfig.cacheEnabled}, TTL: ${fullConfig.cacheTtlDays} days`);
    },
    stop: shutdown,
  };
}

// Start server if run directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
  createServer()
    .then(server => server.start())
    .catch(error => {
      logger.error('Failed to start server', { error: error.message });
      process.exit(1);
    });
}
