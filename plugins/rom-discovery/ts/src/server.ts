/**
 * ROM Discovery Plugin Server
 * HTTP server for ROM metadata search, discovery, downloads, and scraper management
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import axios from 'axios';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { RomDiscoveryDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import { ScraperScheduler } from './scrapers/scraper-scheduler.js';
import { calculateQualityScore, calculatePopularityScore } from './scoring.js';
import type {
  SearchRomsQuery,
  DownloadRequest,
  UpdateScraperRequest,
  AcceptDisclaimerRequest,
  AuditLogQuery,
  AuditLogRecord,
} from './types.js';

const logger = createLogger('rom-discovery:server');

const DEFAULT_LEGAL_DISCLAIMER = `LEGAL NOTICE: By downloading ROMs through this service, you acknowledge and agree that:

1. You are responsible for ensuring you own original copies of any copyrighted games you download.
2. This service does not endorse, encourage, or facilitate software piracy.
3. All download activities are logged for compliance and legal audit purposes.
4. The service operator reserves the right to respond to DMCA takedown requests by removing content and providing download logs to rights holders.
5. You use this service at your own legal risk.

For DMCA inquiries, contact the service administrator.`;

const LEGAL_DISCLAIMER_TEXT = process.env.ROM_LEGAL_DISCLAIMER ?? DEFAULT_LEGAL_DISCLAIMER;
const LEGAL_DISCLAIMER_VERSION = process.env.ROM_LEGAL_DISCLAIMER_VERSION ?? '1.0';

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new RomDiscoveryDatabase();
  await db.connect();
  await db.initializeSchema();

  // Initialize scraper scheduler
  let scheduler: ScraperScheduler | null = null;

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
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

  function scopedDb(request: unknown): RomDiscoveryDatabase {
    return (request as Record<string, unknown>).scopedDb as RomDiscoveryDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'rom-discovery', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'rom-discovery', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'rom-discovery',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'rom-discovery',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      scrapers_running: scheduler?.isRunning() ?? false,
      stats: {
        totalRoms: stats.total_roms,
        totalPlatforms: stats.total_platforms,
        activeScrapers: stats.active_scrapers,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // ROM Search Endpoints
  // =========================================================================

  app.get<{ Querystring: SearchRomsQuery }>('/api/roms/search', async (request) => {
    const { roms, total } = await scopedDb(request).searchRoms({
      query: request.query.q,
      platform: request.query.platform,
      region: request.query.region,
      quality_min: request.query.quality_min ? parseInt(request.query.quality_min, 10) : undefined,
      popularity_min: request.query.popularity_min ? parseInt(request.query.popularity_min, 10) : undefined,
      verified_only: request.query.verified_only === 'true',
      homebrew_only: request.query.homebrew_only === 'true',
      community_only: request.query.community_only === 'true',
      show_hacks: request.query.show_hacks === 'true',
      show_translations: request.query.show_translations === 'true',
      genre: request.query.genre,
      sort: request.query.sort as 'popularity' | 'quality' | 'title' | 'year' | undefined,
      order: request.query.order as 'asc' | 'desc' | undefined,
      limit: request.query.limit ? parseInt(request.query.limit, 10) : undefined,
      offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
    });

    return {
      roms,
      total,
      limit: request.query.limit ? parseInt(request.query.limit, 10) : 50,
      offset: request.query.offset ? parseInt(request.query.offset, 10) : 0,
    };
  });

  app.get<{ Params: { id: string } }>('/api/roms/:id', async (request, reply) => {
    const rom = await scopedDb(request).getRomById(request.params.id);
    if (!rom) {
      return reply.status(404).send({ error: 'ROM not found' });
    }

    // Increment search count for popularity tracking
    await scopedDb(request).incrementSearchCount(rom.id).catch(() => {
      // Non-critical, don't fail the request
    });

    return rom;
  });

  app.get('/api/roms/platforms', async (request) => {
    const platforms = await scopedDb(request).getPlatformStats();
    return { platforms, count: platforms.length };
  });

  app.get('/api/roms/featured', async (request) => {
    const featured = await scopedDb(request).getFeaturedRoms();
    return featured;
  });

  // =========================================================================
  // Legal Compliance Endpoints
  // =========================================================================

  app.get('/api/legal/disclaimer', async () => {
    return {
      version: LEGAL_DISCLAIMER_VERSION,
      text: LEGAL_DISCLAIMER_TEXT,
      accept_url: '/api/legal/accept',
    };
  });

  app.post<{ Body: AcceptDisclaimerRequest }>('/api/legal/accept', {
    schema: {
      body: {
        type: 'object',
        required: ['user_id'],
        properties: {
          user_id: { type: 'string' },
        },
      },
    },
  }, async (request, reply) => {
    const ctx = getAppContext(request);
    const sourceAccountId = ctx.sourceAccountId;
    const userId = request.body.user_id;
    const ipAddress = request.ip;
    const userAgent = request.headers['user-agent'] ?? null;

    await scopedDb(request).recordLegalAcceptance({
      source_account_id: sourceAccountId,
      user_id: userId,
      disclaimer_version: LEGAL_DISCLAIMER_VERSION,
      ip_address: ipAddress,
      user_agent: userAgent ?? undefined,
    });

    await scopedDb(request).addAuditLog({
      source_account_id: sourceAccountId,
      user_id: userId,
      action: 'disclaimer_accepted',
      ip_address: ipAddress,
      user_agent: userAgent ?? undefined,
      details: { disclaimer_version: LEGAL_DISCLAIMER_VERSION },
    });

    return reply.status(200).send({
      accepted: true,
      disclaimer_version: LEGAL_DISCLAIMER_VERSION,
      user_id: userId,
      timestamp: new Date().toISOString(),
    });
  });

  app.get<{ Params: { userId: string }; Querystring: { requesting_user_id?: string } }>('/api/legal/status/:userId', async (request, reply) => {
    const userId = request.params.userId;

    // Authorization: require API key OR requesting user must match the userId being queried
    const apiKey = fullConfig.security.apiKey;
    const providedKey = (request.headers['x-api-key'] ?? request.headers.authorization?.replace('Bearer ', '')) as string | undefined;
    const isAdmin = apiKey && providedKey === apiKey;
    const requestingUserId = request.query.requesting_user_id;
    const isSelf = requestingUserId && requestingUserId === userId;

    if (!isAdmin && !isSelf) {
      return reply.status(403).send({
        error: 'Forbidden: provide API key for admin access, or requesting_user_id matching the userId',
      });
    }

    const accepted = await scopedDb(request).hasAcceptedDisclaimer(userId, LEGAL_DISCLAIMER_VERSION);
    return {
      user_id: userId,
      accepted,
      current_version: LEGAL_DISCLAIMER_VERSION,
      disclaimer_url: '/api/legal/disclaimer',
      accept_url: '/api/legal/accept',
    };
  });

  // =========================================================================
  // Audit Log Endpoints
  // =========================================================================

  app.get<{ Querystring: AuditLogQuery }>('/api/audit', async (request, reply) => {
    // Admin-only: require API key authentication
    const apiKey = fullConfig.security.apiKey;
    const providedKey = (request.headers['x-api-key'] ?? request.headers.authorization?.replace('Bearer ', '')) as string | undefined;
    if (!apiKey || providedKey !== apiKey) {
      return reply.status(401).send({ error: 'API key required for audit log access' });
    }

    const { entries, total } = await scopedDb(request).getAuditLog({
      user_id: request.query.user_id,
      action: request.query.action,
      from_date: request.query.from_date ? new Date(request.query.from_date) : undefined,
      to_date: request.query.to_date ? new Date(request.query.to_date) : undefined,
      limit: request.query.limit ? parseInt(request.query.limit, 10) : undefined,
      offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
    });

    return {
      entries,
      total,
      limit: request.query.limit ? parseInt(request.query.limit, 10) : 50,
      offset: request.query.offset ? parseInt(request.query.offset, 10) : 0,
    };
  });

  app.get<{ Querystring: { from_date?: string; to_date?: string; user_id?: string } }>('/api/audit/export', async (request, reply) => {
    // Admin-only: require API key authentication
    const apiKey = fullConfig.security.apiKey;
    const providedKey = (request.headers['x-api-key'] ?? request.headers.authorization?.replace('Bearer ', '')) as string | undefined;
    if (!apiKey || providedKey !== apiKey) {
      return reply.status(401).send({ error: 'API key required for audit log export' });
    }

    // Require at least one filter to prevent full-table dumps
    if (!request.query.from_date && !request.query.to_date && !request.query.user_id) {
      return reply.status(400).send({
        error: 'At least one filter is required (from_date, to_date, or user_id)',
      });
    }

    const MAX_EXPORT_ROWS = 100_000;

    const entries = await scopedDb(request).exportAuditLog({
      from_date: request.query.from_date ? new Date(request.query.from_date) : undefined,
      to_date: request.query.to_date ? new Date(request.query.to_date) : undefined,
      user_id: request.query.user_id,
      limit: MAX_EXPORT_ROWS,
    });

    // Build CSV
    const headers = [
      'id', 'source_account_id', 'user_id', 'action',
      'rom_metadata_id', 'rom_name', 'rom_platform', 'rom_source',
      'ip_address', 'user_agent', 'details', 'created_at',
    ];

    const csvRows = [headers.join(',')];
    for (const entry of entries) {
      const row = [
        entry.id,
        csvEscape(entry.source_account_id),
        csvEscape(entry.user_id),
        csvEscape(entry.action),
        csvEscape(entry.rom_metadata_id ?? ''),
        csvEscape(entry.rom_name ?? ''),
        csvEscape(entry.rom_platform ?? ''),
        csvEscape(entry.rom_source ?? ''),
        csvEscape(entry.ip_address ?? ''),
        csvEscape(entry.user_agent ?? ''),
        csvEscape(JSON.stringify(entry.details ?? {})),
        entry.created_at instanceof Date ? entry.created_at.toISOString() : String(entry.created_at),
      ];
      csvRows.push(row.join(','));
    }

    const csvText = csvRows.join('\n');
    return reply
      .header('Content-Type', 'text/csv')
      .header('Content-Disposition', 'attachment; filename="audit_log.csv"')
      .send(csvText);
  });

  // =========================================================================
  // Download Endpoints
  // =========================================================================

  app.post<{ Body: DownloadRequest }>('/api/roms/download', {
    schema: {
      body: {
        type: 'object',
        required: ['rom_metadata_id', 'user_id'],
        properties: {
          rom_metadata_id: { type: 'string', format: 'uuid' },
          user_id: { type: 'string' },
        },
      },
    },
  }, async (request, reply) => {
    try {
      const ctx = getAppContext(request);
      const sourceAccountId = ctx.sourceAccountId;
      const userId = request.body.user_id;
      const ipAddress = request.ip;
      const userAgent = request.headers['user-agent'] ?? null;

      // Verify ROM exists
      const rom = await scopedDb(request).getRomById(request.body.rom_metadata_id);
      if (!rom) {
        return reply.status(404).send({ error: 'ROM not found' });
      }

      // Audit log the download request regardless of outcome
      await scopedDb(request).addAuditLog({
        source_account_id: sourceAccountId,
        user_id: userId,
        action: 'download_requested',
        rom_metadata_id: rom.id,
        rom_name: rom.rom_title,
        rom_platform: rom.platform,
        rom_source: rom.download_source ?? undefined,
        ip_address: ipAddress,
        user_agent: userAgent ?? undefined,
        details: { rom_metadata_id: rom.id, file_name: rom.file_name },
      }).catch(() => {
        // Non-critical, don't fail the request
      });

      // Enforce legal disclaimer acceptance (HTTP 451)
      const hasAccepted = await scopedDb(request).hasAcceptedDisclaimer(userId, LEGAL_DISCLAIMER_VERSION);
      if (!hasAccepted) {
        return reply.status(451).send({
          error: 'Legal disclaimer not accepted',
          disclaimer_url: '/api/legal/disclaimer',
          accept_url: '/api/legal/accept',
        });
      }

      if (!rom.download_url || rom.download_url_dead) {
        return reply.status(400).send({ error: 'ROM download URL is not available' });
      }

      // Check file size limit
      if (rom.file_size_bytes && rom.file_size_bytes > fullConfig.maxDownloadSizeMb * 1024 * 1024) {
        return reply.status(400).send({
          error: `ROM file size (${Math.round((rom.file_size_bytes) / (1024 * 1024))}MB) exceeds limit (${fullConfig.maxDownloadSizeMb}MB)`,
        });
      }

      // Check concurrent download limit
      const activeDownloads = await db.getActiveDownloadCount();
      if (activeDownloads >= fullConfig.maxConcurrentDownloads) {
        return reply.status(429).send({
          error: `Maximum concurrent downloads (${fullConfig.maxConcurrentDownloads}) reached. Try again later.`,
        });
      }

      // Create download queue entry
      const queueEntry = await scopedDb(request).createDownloadQueueEntry({
        source_account_id: sourceAccountId,
        user_id: userId,
        rom_metadata_id: request.body.rom_metadata_id,
      });

      // Start async download with audit context
      const auditContext: DownloadAuditContext = {
        source_account_id: sourceAccountId,
        user_id: userId,
        rom_metadata_id: rom.id,
        rom_name: rom.rom_title,
        rom_platform: rom.platform,
        rom_source: rom.download_source ?? null,
        ip_address: ipAddress,
        user_agent: userAgent ?? undefined,
      };

      processDownload(queueEntry.id, rom.download_url, rom.file_hash_md5, rom.id, fullConfig.retroGamingUrl, rom, db, auditContext).catch(err => {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        logger.error('Download processing failed', { downloadId: queueEntry.id, error: msg });
      });

      return reply.status(202).send({
        download: queueEntry,
        message: 'Download queued',
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to queue download', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get<{ Params: { id: string } }>('/api/roms/download/:id/status', async (request, reply) => {
    const download = await db.getDownloadById(request.params.id);
    if (!download) {
      return reply.status(404).send({ error: 'Download not found' });
    }
    return download;
  });

  app.get('/api/roms/download/queue', async (request) => {
    const ctx = getAppContext(request);
    const queue = await db.getDownloadQueue(ctx.sourceAccountId);
    return { queue, count: queue.length };
  });

  app.delete<{ Params: { id: string } }>('/api/roms/download/:id', async (request, reply) => {
    const cancelled = await db.cancelDownload(request.params.id);
    if (!cancelled) {
      return reply.status(404).send({ error: 'Download not found or not cancellable' });
    }
    return { success: true, message: 'Download cancelled' };
  });

  // =========================================================================
  // Scraper Endpoints
  // =========================================================================

  app.get('/api/roms/scrapers', async () => {
    const scrapers = await db.getScrapers();
    return {
      scrapers,
      count: scrapers.length,
      scheduler_running: scheduler?.isRunning() ?? false,
    };
  });

  app.post<{ Params: { name: string } }>('/api/roms/scrapers/:name/run', async (request, reply) => {
    const scraperName = request.params.name;
    const scraper = await db.getScraperByName(scraperName);
    if (!scraper) {
      return reply.status(404).send({ error: `Scraper "${scraperName}" not found` });
    }

    // Run scraper asynchronously
    const ctx = getAppContext(request);
    const accountDb = db.forSourceAccount(ctx.sourceAccountId);
    const runScheduler = new ScraperScheduler(accountDb, ctx.sourceAccountId);

    // Fire and forget - the result is tracked in the database
    runScheduler.runScraper(scraperName).catch(err => {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      logger.error(`Scraper "${scraperName}" run failed`, { error: msg });
    });

    return reply.status(202).send({
      message: `Scraper "${scraperName}" started`,
      scraper_name: scraperName,
      timestamp: new Date().toISOString(),
    });
  });

  app.patch<{ Params: { name: string }; Body: UpdateScraperRequest }>('/api/roms/scrapers/:name', {
    schema: {
      body: {
        type: 'object',
        properties: {
          enabled: { type: 'boolean' },
          cron_schedule: { type: 'string' },
        },
      },
    },
  }, async (request, reply) => {
    const scraperName = request.params.name;
    const updated = await db.updateScraper(scraperName, {
      enabled: request.body.enabled,
      cron_schedule: request.body.cron_schedule,
    });

    if (!updated) {
      return reply.status(404).send({ error: `Scraper "${scraperName}" not found` });
    }

    return updated;
  });

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'rom-discovery',
      version: '1.0.0',
      stats,
      scoring: {
        quality: {
          description: 'Quality score (0-100) based on release group, verification, and metadata completeness',
          weights: {
            no_intro_bonus: 45,
            redump_bonus: 45,
            community_bonus: 35,
            tosec_bonus: 30,
            archive_org_bonus: 20,
            verified_bonus: 5,
            homebrew_bonus: 10,
            hack_penalty: -20,
            translation_penalty: -10,
            dead_url_penalty: -50,
          },
        },
        popularity: {
          description: 'Popularity score (0-100) using weighted log-scale normalization',
          weights: {
            download_count: 0.30,
            play_count: 0.25,
            archive_org_downloads: 0.25,
            search_count: 0.20,
          },
        },
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Utility functions referenced by scoring module
  // =========================================================================

  // Export scoring functions for use by scrapers
  app.get('/api/roms/scoring/quality', async (request) => {
    const query = request.query as Record<string, string>;
    const score = calculateQualityScore({
      release_group: query.release_group ?? null,
      is_verified_dump: query.is_verified_dump === 'true',
      is_hack: query.is_hack === 'true',
      is_translation: query.is_translation === 'true',
      is_homebrew: query.is_homebrew === 'true',
      is_public_domain: query.is_public_domain === 'true',
      download_url_dead: query.download_url_dead === 'true',
      download_url: query.download_url ?? null,
      file_hash_sha256: query.file_hash_sha256 ?? null,
      file_hash_md5: query.file_hash_md5 ?? null,
    });
    return { quality_score: score };
  });

  app.get('/api/roms/scoring/popularity', async (request) => {
    const query = request.query as Record<string, string>;
    const score = calculatePopularityScore({
      download_count: parseInt(query.download_count ?? '0', 10),
      play_count: parseInt(query.play_count ?? '0', 10),
      archive_org_downloads: parseInt(query.archive_org_downloads ?? '0', 10),
      search_count: parseInt(query.search_count ?? '0', 10),
    });
    return { popularity_score: score };
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const server = {
    async start() {
      try {
        await app.listen({ port: fullConfig.port, host: fullConfig.host });
        logger.info(`ROM Discovery server listening on ${fullConfig.host}:${fullConfig.port}`);

        // Start scraper scheduler if enabled
        if (fullConfig.enableScrapers) {
          scheduler = new ScraperScheduler(db, 'primary');
          await scheduler.start();
          logger.info('Scraper scheduler started');
        } else {
          logger.info('Scraper scheduler disabled (set ROM_DISCOVERY_ENABLE_SCRAPERS=true to enable)');
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Server failed to start', { error: message });
        throw error;
      }
    },

    async stop() {
      if (scheduler) {
        scheduler.stop();
      }
      await app.close();
      await db.disconnect();
      logger.info('Server stopped');
    },
  };

  return server;
}

/**
 * Audit context passed to processDownload for compliance logging
 */
interface DownloadAuditContext {
  source_account_id: string;
  user_id: string;
  rom_metadata_id: string;
  rom_name: string;
  rom_platform: string;
  rom_source: string | null;
  ip_address?: string;
  user_agent?: string;
}

/**
 * Escape a value for CSV output
 */
function csvEscape(value: string): string {
  if (value.includes(',') || value.includes('"') || value.includes('\n')) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

/**
 * Process a download asynchronously: fetch the file, verify checksum, track progress
 */
async function processDownload(
  downloadId: string,
  downloadUrl: string,
  expectedMd5: string | null,
  romMetadataId: string,
  retroGamingUrl: string,
  rom: Record<string, unknown>,
  db: RomDiscoveryDatabase,
  auditContext: DownloadAuditContext
): Promise<void> {
  try {
    // Mark as downloading
    await db.updateDownloadQueue(downloadId, {
      status: 'downloading',
      download_started_at: new Date(),
    });

    // Stream download using axios
    const response = await axios.get(downloadUrl, {
      responseType: 'arraybuffer',
      timeout: 300000, // 5 minute timeout
      maxContentLength: 2 * 1024 * 1024 * 1024, // 2GB max
      onDownloadProgress: async (progressEvent) => {
        if (progressEvent.total) {
          const percent = Math.round((progressEvent.loaded / progressEvent.total) * 100);
          // Update progress periodically (every 10%)
          if (percent % 10 === 0) {
            await db.updateDownloadQueue(downloadId, {
              download_progress_percent: percent,
              downloaded_bytes: progressEvent.loaded,
              total_bytes: progressEvent.total,
            }).catch(() => {
              // Non-critical, continue download
            });
          }
        }
      },
    });

    const downloadedBytes = response.data.byteLength ?? 0;

    // Mark as verifying
    await db.updateDownloadQueue(downloadId, {
      status: 'verifying',
      downloaded_bytes: downloadedBytes,
      total_bytes: downloadedBytes,
      download_progress_percent: 100,
    });

    // Verify checksum if we have an expected value
    let checksumVerified = false;
    if (expectedMd5) {
      const crypto = await import('crypto');
      const hash = crypto.createHash('md5');
      hash.update(Buffer.from(response.data));
      const computedMd5 = hash.digest('hex');
      checksumVerified = computedMd5 === expectedMd5;

      if (!checksumVerified) {
        logger.warn('Checksum mismatch', { downloadId, expected: expectedMd5, computed: computedMd5 });
      }
    }

    // Mark as completed
    await db.updateDownloadQueue(downloadId, {
      status: 'completed',
      download_completed_at: new Date(),
      checksum_verified: checksumVerified,
      download_progress_percent: 100,
      downloaded_bytes: downloadedBytes,
    });

    // Increment download count for popularity
    await db.incrementDownloadCount(romMetadataId);

    // Audit log: download completed
    await db.addAuditLog({
      source_account_id: auditContext.source_account_id,
      user_id: auditContext.user_id,
      action: 'download_completed',
      rom_metadata_id: auditContext.rom_metadata_id,
      rom_name: auditContext.rom_name,
      rom_platform: auditContext.rom_platform,
      rom_source: auditContext.rom_source ?? undefined,
      ip_address: auditContext.ip_address,
      user_agent: auditContext.user_agent,
      details: {
        download_id: downloadId,
        downloaded_bytes: downloadedBytes,
        checksum_verified: checksumVerified,
      },
    }).catch(() => {
      // Non-critical audit log
    });

    // Try to notify retro-gaming plugin to add ROM to library
    try {
      await axios.post(`${retroGamingUrl}/api/games/roms`, {
        title: rom.rom_title ?? rom.game_title,
        platform: rom.platform,
        file_name: rom.file_name,
        file_size_bytes: downloadedBytes,
        source: 'rom-discovery',
        rom_metadata_id: romMetadataId,
      }, {
        timeout: 10000,
      });
      logger.info('Notified retro-gaming plugin of new ROM', { romMetadataId });
    } catch {
      // Non-critical - retro-gaming plugin may not be running
      logger.debug('Could not notify retro-gaming plugin (may not be running)');
    }

    logger.info('Download completed successfully', { downloadId, romMetadataId, bytes: downloadedBytes });

  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Download failed', { downloadId, error: message });

    // Audit log: download failed
    await db.addAuditLog({
      source_account_id: auditContext.source_account_id,
      user_id: auditContext.user_id,
      action: 'download_failed',
      rom_metadata_id: auditContext.rom_metadata_id,
      rom_name: auditContext.rom_name,
      rom_platform: auditContext.rom_platform,
      rom_source: auditContext.rom_source ?? undefined,
      ip_address: auditContext.ip_address,
      user_agent: auditContext.user_agent,
      details: {
        download_id: downloadId,
        error: message,
      },
    }).catch(() => {
      // Non-critical audit log
    });

    // Check retry count
    const download = await db.getDownloadById(downloadId);
    const retryCount = (download?.retry_count ?? 0) + 1;
    const maxRetries = download?.max_retries ?? 3;

    if (retryCount < maxRetries) {
      await db.updateDownloadQueue(downloadId, {
        status: 'pending',
        error_message: message,
        retry_count: retryCount,
      });
      logger.info(`Download queued for retry (${retryCount}/${maxRetries})`, { downloadId });
    } else {
      await db.updateDownloadQueue(downloadId, {
        status: 'failed',
        error_message: message,
        retry_count: retryCount,
      });
      logger.error(`Download failed permanently after ${retryCount} retries`, { downloadId });
    }
  }
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const server = await createServer(config);
  await server.start();

  process.on('SIGTERM', async () => {
    logger.info('SIGTERM received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });

  process.on('SIGINT', async () => {
    logger.info('SIGINT received, shutting down gracefully');
    await server.stop();
    process.exit(0);
  });
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
