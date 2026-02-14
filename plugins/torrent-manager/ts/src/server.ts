/**
 * Torrent Manager HTTP API Server
 * Complete Fastify REST API with all endpoints
 */

import Fastify, { FastifyRequest, FastifyReply } from 'fastify';
import cors from '@fastify/cors';
import rateLimit from '@fastify/rate-limit';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext, loadSecurityConfig } from '@nself/plugin-utils';
import { TorrentDatabase } from './database.js';
import { VPNChecker } from './vpn-checker.js';
import { TransmissionClient } from './clients/transmission.js';
import { TorrentSearchAggregator } from './search/aggregator.js';
import { SmartMatcher } from './matching/smart-matcher.js';
import { getAllSources } from './sources/registry.js';
import type { TorrentManagerConfig, TorrentCategory } from './types.js';

const logger = createLogger('torrent-manager:server');

// Request body interfaces
interface AddTorrentBody {
  magnet_uri: string;
  category?: string;
  download_path?: string;
  requested_by?: string;
}

interface SearchBody {
  query: string;
  type?: string;
  quality?: string;
  minSeeders?: number;
  maxResults?: number;
}

interface SmartSearchBody {
  title: string;
  year?: number;
  season?: number;
  episode?: number;
  quality?: string;
  minSeeders?: number;
}

interface FetchMagnetBody {
  source: string;
  sourceUrl: string;
}

interface ListQuery {
  status?: string;
  category?: string;
  limit?: string;
}

interface DeleteQuery {
  delete_files?: string;
}

interface ValidateQuery {
  query_hash?: string;
}

interface SeedingConfigBody {
  ratio_limit?: number;
  time_limit_hours?: number;
  auto_remove?: boolean;
  keep_files?: boolean;
  favorite?: boolean;
}

export class TorrentManagerServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: TorrentDatabase;
  private vpnChecker: VPNChecker;
  private config: TorrentManagerConfig;
  private torrentClient?: TransmissionClient;

  constructor(config: TorrentManagerConfig, database: TorrentDatabase) {
    this.config = config;
    this.database = database;
    this.vpnChecker = new VPNChecker(config.vpn_manager_url);
    this.fastify = Fastify({ logger: false });
  }

  async initialize(): Promise<void> {
    // Register plugins
    await this.fastify.register(cors);
    await this.fastify.register(rateLimit, {
      max: 100,
      timeWindow: '1 minute',
    });

    // Security middleware
    const security = loadSecurityConfig('TORRENT_MANAGER');
    const rateLimiter = new ApiRateLimiter(security.rateLimitMax ?? 100, security.rateLimitWindowMs ?? 60000);
    this.fastify.addHook('preHandler', createRateLimitHook(rateLimiter) as never);
    if (security.apiKey) {
      this.fastify.addHook('preHandler', createAuthHook(security.apiKey) as never);
    }

    // Initialize torrent client
    if (this.config.default_client === 'transmission') {
      this.torrentClient = new TransmissionClient(
        this.config.transmission_host,
        this.config.transmission_port,
        this.config.transmission_username,
        this.config.transmission_password
      );

      const connected = await this.torrentClient.connect();
      if (connected) {
        logger.info('Default torrent client connected');
      } else {
        logger.warn('Failed to connect to default torrent client');
      }
    }

    // Register routes
    this.registerHealthRoutes();
    this.registerClientRoutes();
    this.registerDownloadRoutes();
    this.registerSearchRoutes();
    this.registerSeedingRoutes();
    this.registerSourceRoutes();
    this.registerStatsRoutes();

    logger.info('Server initialized');
  }

  // ============================================================================
  // Health & Ready Routes
  // ============================================================================

  private registerHealthRoutes(): void {
    this.fastify.get('/health', async () => {
      return { status: 'ok', timestamp: new Date().toISOString() };
    });

    this.fastify.get('/ready', async () => {
      const vpnActive = await this.vpnChecker.isVPNActive();
      const clientConnected = this.torrentClient
        ? await this.torrentClient.isConnected()
        : false;

      return {
        ready: clientConnected,
        vpn_active: vpnActive,
        client_connected: clientConnected,
        timestamp: new Date().toISOString(),
      };
    });
  }

  // ============================================================================
  // Client Routes
  // ============================================================================

  private registerClientRoutes(): void {
    // List clients
    this.fastify.get('/v1/clients', async () => {
      const clients = await this.database.listClients();
      return { clients };
    });

  }

  // ============================================================================
  // Download Routes
  // ============================================================================

  private registerDownloadRoutes(): void {
    // Add download
    this.fastify.post('/v1/downloads', {
      schema: {
        body: {
          type: 'object',
          required: ['magnet_uri'],
          properties: {
            magnet_uri: { type: 'string' },
            category: { type: 'string' },
            download_path: { type: 'string' },
            requested_by: { type: 'string' },
          },
        },
      },
    }, async (request: FastifyRequest<{ Body: AddTorrentBody }>, reply: FastifyReply) => {
      const { magnet_uri, category, download_path, requested_by } = request.body;

      // Verify VPN if required
      if (this.config.vpn_required) {
        const vpnActive = await this.vpnChecker.isVPNActive();
        if (!vpnActive) {
          reply.code(403).send({
            error: 'VPN_REQUIRED',
            message: 'VPN must be active before starting downloads',
          });
          return;
        }
      }

      if (!this.torrentClient) {
        reply.code(500).send({ error: 'No torrent client configured' });
        return;
      }

      try {
        // Validate category if provided
        const validCategory = category as TorrentCategory | undefined;

        // Add to client
        const download = await this.torrentClient.addTorrent(magnet_uri, {
          category: validCategory,
          download_path,
        });

        // Get default client
        const client = await this.database.getDefaultClient();
        if (!client) {
          reply.code(500).send({ error: 'No default client configured' });
          return;
        }

        // Save to database
        download.client_id = client.id;
        download.requested_by = requested_by || 'api';
        const savedDownload = await this.database.createDownload(download);

        logger.info('Download added', { id: savedDownload.id, name: savedDownload.name });

        return {
          success: true,
          download: savedDownload,
        };
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Failed to add download', { error: message });
        reply.code(500).send({ error: message });
      }
    });

    // List downloads
    this.fastify.get('/v1/downloads', async (request: FastifyRequest<{ Querystring: ListQuery }>) => {
      const { status, category, limit } = request.query;
      const downloads = await this.database.listDownloads({
        status,
        category,
        limit: limit ? parseInt(limit, 10) : undefined,
      });

      return { downloads, total: downloads.length };
    });

    // Get download details
    this.fastify.get('/v1/downloads/:id', async (request: FastifyRequest, reply: FastifyReply) => {
      const { id } = request.params as { id: string };
      const download = await this.database.getDownload(id);

      if (!download) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }

      return { download };
    });

    // Pause download
    this.fastify.post('/v1/downloads/:id/pause', async (request: FastifyRequest, reply: FastifyReply) => {
      const { id } = request.params as { id: string };
      const download = await this.database.getDownload(id);

      if (!download) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }

      if (this.torrentClient) {
        await this.torrentClient.pauseTorrent(download.client_torrent_id);
        await this.database.updateDownload(id, { status: 'paused' });
      }

      return { success: true };
    });

    // Resume download
    this.fastify.post('/v1/downloads/:id/resume', async (request: FastifyRequest, reply: FastifyReply) => {
      const { id } = request.params as { id: string };
      const download = await this.database.getDownload(id);

      if (!download) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }

      // Verify VPN before resuming
      if (this.config.vpn_required) {
        const vpnActive = await this.vpnChecker.isVPNActive();
        if (!vpnActive) {
          reply.code(403).send({
            error: 'VPN_REQUIRED',
            message: 'VPN must be active to resume downloads',
          });
          return;
        }
      }

      if (this.torrentClient) {
        await this.torrentClient.resumeTorrent(download.client_torrent_id);
        await this.database.updateDownload(id, { status: 'downloading' });
      }

      return { success: true };
    });

    // Delete download
    this.fastify.delete('/v1/downloads/:id', async (request: FastifyRequest<{ Params: { id: string }; Querystring: DeleteQuery }>, reply: FastifyReply) => {
      const { id } = request.params;
      const { delete_files } = request.query;
      const download = await this.database.getDownload(id);

      if (!download) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }

      if (this.torrentClient) {
        await this.torrentClient.removeTorrent(
          download.client_torrent_id,
          delete_files === 'true'
        );
      }

      await this.database.deleteDownload(id);
      return { success: true };
    });
  }

  // ============================================================================
  // Search Routes
  // ============================================================================

  private registerSearchRoutes(): void {
    // Search torrents across all sources
    this.fastify.post('/v1/search', {
      schema: {
        body: {
          type: 'object',
          required: ['query'],
          properties: {
            query: { type: 'string' },
            type: { type: 'string' },
            quality: { type: 'string' },
            minSeeders: { type: 'number' },
            maxResults: { type: 'number' },
          },
        },
      },
    }, async (request: FastifyRequest<{ Body: SearchBody }>, reply: FastifyReply) => {
      const { query, type, quality, minSeeders, maxResults } = request.body;

      if (!query) {
        reply.code(400).send({ error: 'query is required' });
        return;
      }

      try {
        const aggregator = new TorrentSearchAggregator(
          this.config.enabled_sources?.split(',')
        );

        // Validate type is either 'movie' or 'tv'
        const searchType = type === 'tv' || type === 'movie' ? type : undefined;

        const results = await aggregator.search({
          query,
          type: searchType,
          quality,
          minSeeders,
          maxResults: maxResults || 50,
        });

        return {
          query,
          count: results.length,
          results: results.map((r) => ({
            title: r.title,
            size: r.size,
            seeders: r.seeders,
            leechers: r.leechers,
            source: r.source,
            quality: r.parsedInfo.quality,
            sourceType: r.parsedInfo.source,
            releaseGroup: r.parsedInfo.releaseGroup,
            magnetUri: r.magnetUri,
            sourceUrl: r.sourceUrl,
          })),
        };
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Search failed:', { error: message });
        reply.code(500).send({ error: 'Search failed', details: message });
      }
    });

    // Search and return best match
    this.fastify.post('/v1/search/best-match', {
      schema: {
        body: {
          type: 'object',
          required: ['title'],
          properties: {
            title: { type: 'string' },
            year: { type: 'number' },
            season: { type: 'number' },
            episode: { type: 'number' },
            quality: { type: 'string' },
            minSeeders: { type: 'number' },
          },
        },
      },
    }, async (request: FastifyRequest<{ Body: SmartSearchBody }>, reply: FastifyReply) => {
      const { title, year, season, episode, quality, minSeeders } = request.body;

      if (!title) {
        reply.code(400).send({ error: 'title is required' });
        return;
      }

      try {
        // Step 1: Search
        const aggregator = new TorrentSearchAggregator(
          this.config.enabled_sources?.split(',')
        );

        const searchQuery =
          season !== undefined && episode !== undefined
            ? `${title} S${String(season).padStart(2, '0')}E${String(episode).padStart(2, '0')}`
            : year
            ? `${title} ${year}`
            : title;

        const searchType = season !== undefined ? 'tv' : 'movie';
        const searchResults = await aggregator.search({
          query: searchQuery,
          type: searchType as 'tv' | 'movie',
          quality,
          minSeeders: minSeeders || 1,
          maxResults: 50,
        });

        if (searchResults.length === 0) {
          reply.code(404).send({ error: 'No torrents found' });
          return;
        }

        // Step 2: Smart match
        const matcher = new SmartMatcher();
        const bestMatch = matcher.findBestMatch(searchResults, {
          title,
          year,
          season,
          episode,
          preferredQualities: quality ? [quality] : ['1080p', '720p'],
          minSeeders: minSeeders || 1,
          excludeKeywords: ['KORSUB', 'HC', 'BLURRED'],
        });

        if (!bestMatch) {
          reply.code(404).send({ error: 'No suitable match found' });
          return;
        }

        // Step 3: Fetch magnet if needed
        if (!bestMatch.magnetUri || !bestMatch.magnetUri.startsWith('magnet:')) {
          bestMatch.magnetUri = await aggregator.getMagnetLink(bestMatch);
        }

        return {
          match: {
            title: bestMatch.title,
            magnetUri: bestMatch.magnetUri,
            size: bestMatch.size,
            seeders: bestMatch.seeders,
            source: bestMatch.source,
            score: bestMatch.score,
            scoreBreakdown: bestMatch.scoreBreakdown,
            parsedInfo: bestMatch.parsedInfo,
          },
        };
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Best match search failed:', { error: message });
        reply.code(500).send({ error: 'Search failed', details: message });
      }
    });

    // Get magnet link for a specific torrent
    this.fastify.post('/v1/magnet', {
      schema: {
        body: {
          type: 'object',
          required: ['source', 'sourceUrl'],
          properties: {
            source: { type: 'string' },
            sourceUrl: { type: 'string' },
          },
        },
      },
    }, async (request: FastifyRequest<{ Body: FetchMagnetBody }>, reply: FastifyReply) => {
      const { source, sourceUrl } = request.body;

      if (!source || !sourceUrl) {
        reply.code(400).send({ error: 'source and sourceUrl are required' });
        return;
      }

      try {
        const aggregator = new TorrentSearchAggregator([source]);
        const magnetUri = await aggregator.getMagnetLink({
          title: '',
          normalizedTitle: '',
          magnetUri: '',
          infoHash: '',
          size: '',
          sizeBytes: 0,
          seeders: 0,
          leechers: 0,
          uploadDate: new Date().toISOString(),
          uploadDateUnix: Date.now(),
          source,
          sourceUrl,
          parsedInfo: {
            title: '',
          },
        });

        return { magnetUri };
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Magnet fetch failed:', { error: message });
        reply.code(500).send({ error: 'Failed to fetch magnet', details: message });
      }
    });

    // Get search cache
    this.fastify.get('/v1/search/cache', async (request: FastifyRequest<{ Querystring: ValidateQuery }>) => {
      const { query_hash } = request.query;
      if (!query_hash) {
        return { error: 'query_hash required' };
      }

      const cache = await this.database.getSearchCache(query_hash);
      return { cache };
    });
  }

  // ============================================================================
  // Seeding Policy Routes
  // ============================================================================

  private registerSeedingRoutes(): void {
    // Update seeding policy for a specific download
    this.fastify.put('/v1/seeding/:id/policy', {
      schema: {
        body: {
          type: 'object',
          properties: {
            ratio_limit: { type: 'number' },
            time_limit_hours: { type: 'number' },
            auto_remove: { type: 'boolean' },
            keep_files: { type: 'boolean' },
            favorite: { type: 'boolean' },
          },
        },
      },
    }, async (request: FastifyRequest<{ Params: { id: string }; Body: SeedingConfigBody }>, reply: FastifyReply) => {
      const { id } = request.params;
      const body = request.body;

      // Verify the download exists
      const download = await this.database.getDownload(id);
      if (!download) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }

      // Favorites must never be auto-removed
      const isFavorite = body.favorite ?? false;
      const autoRemove = isFavorite ? false : (body.auto_remove ?? true);

      try {
        const appContext = getAppContext(request);
        await this.database.upsertDownloadSeedingPolicy(id, {
          source_account_id: appContext?.sourceAccountId || 'primary',
          ratio_limit: body.ratio_limit,
          time_limit_hours: body.time_limit_hours,
          auto_remove: autoRemove,
          keep_files: body.keep_files,
          favorite: isFavorite,
        });

        logger.info('Seeding policy updated', { download_id: id, favorite: isFavorite });

        return { updated: true };
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.error('Failed to update seeding policy', { error: message });
        reply.code(500).send({ error: 'Failed to update seeding policy', details: message });
      }
    });

    // Get seeding policy for a specific download
    this.fastify.get('/v1/seeding/:id/policy', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const { id } = request.params;

      const policy = await this.database.getDownloadSeedingPolicy(id);
      if (!policy) {
        reply.code(404).send({ error: 'No seeding policy found for this download' });
        return;
      }

      return { policy };
    });
  }

  // ============================================================================
  // Source Routes
  // ============================================================================

  private registerSourceRoutes(): void {
    // List all torrent sources with lifecycle info
    this.fastify.get('/v1/sources', async () => {
      const sources = getAllSources();
      return sources;
    });
  }

  // ============================================================================
  // Stats Routes
  // ============================================================================

  private registerStatsRoutes(): void {
    // Get overall stats
    this.fastify.get('/v1/stats', async () => {
      const dbStats = await this.database.getStats();
      const clientStats = this.torrentClient ? await this.torrentClient.getStats() : null;

      return {
        database: dbStats,
        client: clientStats,
        timestamp: new Date().toISOString(),
      };
    });

    // Get seeding torrents
    this.fastify.get('/v1/seeding', async () => {
      const seeding = await this.database.listDownloads({ status: 'seeding' });
      return { seeding, total: seeding.length };
    });
  }

  // ============================================================================
  // Start Server
  // ============================================================================

  async start(): Promise<void> {
    try {
      await this.fastify.listen({ port: this.config.port, host: '0.0.0.0' });
      logger.info(`Server listening on port ${this.config.port}`);

      // Start VPN monitoring
      this.vpnChecker.startMonitoring(async () => {
        logger.warn('VPN disconnected! Pausing all active downloads');
        try {
          const activeDownloads = await this.database.listDownloads({ status: 'downloading' });
          for (const download of activeDownloads) {
            try {
              if (this.torrentClient && download.client_torrent_id) {
                await this.torrentClient.pauseTorrent(download.client_torrent_id);
              }
              await this.database.updateDownload(download.id, { status: 'paused', error_message: 'VPN disconnected - download paused for safety' });
            } catch (err) {
              const msg = err instanceof Error ? err.message : 'Unknown error';
              logger.error(`Failed to pause download ${download.id}: ${msg}`);
            }
          }
          logger.warn(`Paused ${activeDownloads.length} active downloads due to VPN disconnection`);
        } catch (err) {
          const msg = err instanceof Error ? err.message : 'Unknown error';
          logger.error(`Failed to pause downloads on VPN disconnect: ${msg}`);
        }
      });
    } catch (error) {
      logger.error('Failed to start server', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async stop(): Promise<void> {
    this.vpnChecker.stopMonitoring();
    await this.fastify.close();
    logger.info('Server stopped');
  }
}
