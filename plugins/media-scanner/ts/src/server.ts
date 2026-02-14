/**
 * Media Scanner Plugin Server
 * Fastify HTTP server for scanning, parsing, probing, matching, indexing, and search
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import {
  createLogger,
  ApiRateLimiter,
  createAuthHook,
  createRateLimitHook,
  getAppContext,
} from '@nself/plugin-utils';
import { MediaScannerDatabase } from './database.js';
import { loadConfig, type MediaScannerConfig } from './config.js';
import { scanDirectories } from './scanner.js';
import { parseFilename } from './parser.js';
import { probeFile, checkFFprobeAvailable } from './probe.js';
import { TmdbMatcher, SUGGEST_THRESHOLD } from './matcher.js';
import { MediaSearchService } from './search.js';
import type {
  ScanRequest,
  ParseRequest,
  ProbeRequest,
  MatchRequest,
  IndexRequest,
  SearchQuery,
  ScanError,
} from './types.js';

const logger = createLogger('media-scanner:server');

export async function createServer(config?: Partial<MediaScannerConfig>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new MediaScannerDatabase();
  await db.connect();
  await db.initializeSchema();

  const matcher = new TmdbMatcher(fullConfig.tmdbApiKey);
  const searchService = new MediaSearchService(fullConfig.meilisearchUrl, fullConfig.meilisearchKey);

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 100,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): MediaScannerDatabase {
    return (request as Record<string, unknown>).scopedDb as MediaScannerDatabase;
  }

  // ─── Health Check ─────────────────────────────────────────────────────

  app.get('/health', async () => {
    const ffprobeAvailable = await checkFFprobeAvailable();
    const meiliHealthy = await searchService.healthCheck();
    return {
      status: 'ok',
      plugin: 'media-scanner',
      ffprobe: ffprobeAvailable,
      meilisearch: meiliHealthy,
      timestamp: new Date().toISOString(),
    };
  });

  // ─── POST /v1/scan ────────────────────────────────────────────────────

  app.post('/v1/scan', async (request, reply) => {
    const sdb = scopedDb(request);
    const body = request.body as ScanRequest;

    if (!body.paths || !Array.isArray(body.paths) || body.paths.length === 0) {
      return reply.status(400).send({ error: 'paths is required and must be a non-empty array' });
    }

    const recursive = body.recursive !== false;

    try {
      // Create scan record
      const scan = await sdb.createScan(body.paths, recursive);

      // Start scanning in the background
      processScan(sdb, scan.id, body.paths, recursive, matcher, searchService).catch(err => {
        const message = err instanceof Error ? err.message : 'Unknown error';
        logger.error('Background scan failed', { scanId: scan.id, error: message });
      });

      return { scan_id: scan.id, files_found: 0 };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start scan', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // ─── GET /v1/scan/:id ─────────────────────────────────────────────────

  app.get('/v1/scan/:id', async (request, reply) => {
    const sdb = scopedDb(request);
    const { id } = request.params as { id: string };

    const scan = await sdb.getScan(id);
    if (!scan) {
      return reply.status(404).send({ error: 'Scan not found' });
    }

    return {
      scan_id: scan.id,
      state: scan.state,
      files_found: scan.files_found,
      files_processed: scan.files_processed,
      errors: scan.errors,
    };
  });

  // ─── POST /v1/parse ───────────────────────────────────────────────────

  app.post('/v1/parse', async (request, reply) => {
    const body = request.body as ParseRequest;

    if (!body.filename || typeof body.filename !== 'string') {
      return reply.status(400).send({ error: 'filename is required' });
    }

    const parsed = parseFilename(body.filename);
    return parsed;
  });

  // ─── POST /v1/probe ───────────────────────────────────────────────────

  app.post('/v1/probe', async (request, reply) => {
    const body = request.body as ProbeRequest;

    if (!body.path || typeof body.path !== 'string') {
      return reply.status(400).send({ error: 'path is required' });
    }

    try {
      const info = await probeFile(body.path);
      return info;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Probe failed', { path: body.path, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // ─── POST /v1/match ───────────────────────────────────────────────────

  app.post('/v1/match', async (request, reply) => {
    const body = request.body as MatchRequest;

    if (!body.title || typeof body.title !== 'string') {
      return reply.status(400).send({ error: 'title is required' });
    }
    if (!body.type || !['movie', 'tv'].includes(body.type)) {
      return reply.status(400).send({ error: 'type must be "movie" or "tv"' });
    }

    try {
      const matches = await matcher.match(body.title, body.year ?? null, body.type);
      return { matches };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Match failed', { title: body.title, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // ─── POST /v1/index ───────────────────────────────────────────────────

  app.post('/v1/index', async (request, reply) => {
    const body = request.body as IndexRequest;

    if (!body.id || !body.title || !body.type) {
      return reply.status(400).send({ error: 'id, title, and type are required' });
    }

    try {
      await searchService.indexItem(body);
      return { indexed: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Index failed', { id: body.id, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // ─── GET /v1/search ───────────────────────────────────────────────────

  app.get('/v1/search', async (request, reply) => {
    const query = request.query as SearchQuery;

    if (!query.q || typeof query.q !== 'string') {
      return reply.status(400).send({ error: 'q (search query) is required' });
    }

    try {
      const results = await searchService.search(query);
      return results;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { query: query.q, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // ─── GET /v1/stats ────────────────────────────────────────────────────

  app.get('/v1/stats', async (request) => {
    const sdb = scopedDb(request);
    const stats = await sdb.getStats();
    return stats;
  });

  // ─── GET /v1/files ────────────────────────────────────────────────────

  app.get('/v1/files', async (request) => {
    const sdb = scopedDb(request);
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const files = await sdb.listMediaFiles(limit, offset);
    const total = await sdb.countMediaFiles();
    return { data: files, total, limit, offset };
  });

  app.get('/v1/files/:id', async (request, reply) => {
    const sdb = scopedDb(request);
    const { id } = request.params as { id: string };
    const file = await sdb.getMediaFile(id);
    if (!file) {
      return reply.status(404).send({ error: 'File not found' });
    }
    return file;
  });

  // ─── GET /v1/scans ────────────────────────────────────────────────────

  app.get('/v1/scans', async (request) => {
    const sdb = scopedDb(request);
    const { limit = 20, offset = 0 } = request.query as { limit?: number; offset?: number };
    const scans = await sdb.listScans(limit, offset);
    return { data: scans, limit, offset };
  });

  // ─── Graceful Shutdown ────────────────────────────────────────────────

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
    matcher,
    searchService,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Media scanner server running on http://${fullConfig.host}:${fullConfig.port}`);
    },
    stop: shutdown,
  };
}

// ─── Background Scan Processing ─────────────────────────────────────────────

async function processScan(
  db: MediaScannerDatabase,
  scanId: string,
  paths: string[],
  recursive: boolean,
  matcher: TmdbMatcher,
  _searchService: MediaSearchService
): Promise<void> {
  const startTime = Date.now();

  // Mark scan as started
  await db.updateScanState(scanId, 'scanning', { started_at: new Date() });

  let totalFound = 0;
  let totalProcessed = 0;
  const errors: ScanError[] = [];

  try {
    const scanner = scanDirectories(paths, recursive);
    let result = await scanner.next();

    while (!result.done) {
      const batch = result.value;
      totalFound += batch.length;

      // Update files_found count
      await db.updateScanState(scanId, 'scanning', { files_found: totalFound });

      // Process each file: parse, upsert
      for (const file of batch) {
        try {
          const parsed = parseFilename(file.filename);
          await db.upsertMediaFile(scanId, file, parsed);
          totalProcessed++;
          await db.incrementScanProcessed(scanId);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          const scanError: ScanError = {
            path: file.path,
            error: message,
            timestamp: new Date().toISOString(),
          };
          errors.push(scanError);
          await db.appendScanError(scanId, scanError);
        }
      }

      result = await scanner.next();
    }

    // Get final progress from generator return value
    if (result.value) {
      errors.push(...result.value.errors);
      for (const err of result.value.errors) {
        await db.appendScanError(scanId, err);
      }
    }

    // Probe files for media info (best-effort, non-blocking per file)
    const unprobed = await db.listUnprobed(500);
    for (const file of unprobed) {
      try {
        const info = await probeFile(file.file_path);
        await db.updateMediaProbe(file.id, info);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Probe failed for file', { path: file.file_path, error: message });
      }
    }

    // Match unmatched files against TMDB
    const unmatched = await db.listUnmatched(200);
    for (const file of unmatched) {
      if (!file.parsed_title) continue;
      try {
        const mediaType = file.parsed_season !== null ? 'tv' : 'movie';
        const matches = await matcher.match(file.parsed_title, file.parsed_year, mediaType);
        if (matches.length > 0) {
          const best = matches[0];
          if (best.confidence >= SUGGEST_THRESHOLD) {
            await db.updateMediaMatch(file.id, best);
          }
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.warn('Match failed for file', { title: file.parsed_title, error: message });
      }
    }

    const duration = Date.now() - startTime;
    logger.success('Scan completed', {
      scanId,
      filesFound: totalFound,
      filesProcessed: totalProcessed,
      errors: errors.length,
      duration: `${(duration / 1000).toFixed(1)}s`,
    });

    // Mark scan as completed
    await db.updateScanState(scanId, 'completed', {
      files_found: totalFound,
      files_processed: totalProcessed,
      completed_at: new Date(),
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Scan failed', { scanId, error: message });
    await db.updateScanState(scanId, 'failed', {
      files_found: totalFound,
      files_processed: totalProcessed,
      errors: [...errors, { path: '', error: message, timestamp: new Date().toISOString() }],
      completed_at: new Date(),
    });
  }
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
