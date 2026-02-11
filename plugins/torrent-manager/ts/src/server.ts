/**
 * Torrent Manager HTTP API Server
 * Complete Fastify REST API with all endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import rateLimit from '@fastify/rate-limit';
import { createLogger } from '@nself/plugin-utils';
import { TorrentDatabase } from './database.js';
import { VPNChecker } from './vpn-checker.js';
import { TransmissionClient } from './clients/transmission.js';
import { TorrentSearchAggregator } from './search/aggregator.js';
import { SmartMatcher } from './matching/smart-matcher.js';
import type { TorrentManagerConfig } from './types.js';

const logger = createLogger('torrent-manager:server');

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

    // Get client status
    this.fastify.get('/v1/clients/:id', async (request, reply) => {
      const { id } = request.params as { id: string };
      // TODO: Get specific client status
      reply.code(501).send({ error: 'Not implemented' });
    });
  }

  // ============================================================================
  // Download Routes
  // ============================================================================

  private registerDownloadRoutes(): void {
    // Add download
    this.fastify.post('/v1/downloads', async (request, reply) => {
      const { magnet_uri, category, download_path, requested_by } = request.body as any;

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
        // Add to client
        const download = await this.torrentClient.addTorrent(magnet_uri, {
          category,
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
      } catch (error: any) {
        logger.error('Failed to add download', error);
        reply.code(500).send({ error: error.message });
      }
    });

    // List downloads
    this.fastify.get('/v1/downloads', async (request) => {
      const { status, category, limit } = request.query as any;
      const downloads = await this.database.listDownloads({
        status,
        category,
        limit: limit ? parseInt(limit, 10) : undefined,
      });

      return { downloads, total: downloads.length };
    });

    // Get download details
    this.fastify.get('/v1/downloads/:id', async (request, reply) => {
      const { id } = request.params as { id: string };
      const download = await this.database.getDownload(id);

      if (!download) {
        reply.code(404).send({ error: 'Download not found' });
        return;
      }

      return { download };
    });

    // Pause download
    this.fastify.post('/v1/downloads/:id/pause', async (request, reply) => {
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
    this.fastify.post('/v1/downloads/:id/resume', async (request, reply) => {
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
    this.fastify.delete('/v1/downloads/:id', async (request, reply) => {
      const { id } = request.params as { id: string };
      const { delete_files } = request.query as any;
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
    this.fastify.post('/v1/search', async (request, reply) => {
      const { query, type, quality, minSeeders, maxResults } = request.body as any;

      if (!query) {
        reply.code(400).send({ error: 'query is required' });
        return;
      }

      try {
        const aggregator = new TorrentSearchAggregator(
          this.config.enabled_sources?.split(',')
        );

        const results = await aggregator.search({
          query,
          type,
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
      } catch (error: any) {
        logger.error('Search failed:', error);
        reply.code(500).send({ error: 'Search failed', details: error.message });
      }
    });

    // Search and return best match
    this.fastify.post('/v1/search/best-match', async (request, reply) => {
      const { title, year, season, episode, quality, minSeeders } = request.body as any;

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

        const searchResults = await aggregator.search({
          query: searchQuery,
          type: season !== undefined ? 'tv' : 'movie',
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
      } catch (error: any) {
        logger.error('Best match search failed:', error);
        reply.code(500).send({ error: 'Search failed', details: error.message });
      }
    });

    // Get magnet link for a specific torrent
    this.fastify.post('/v1/magnet', async (request, reply) => {
      const { source, sourceUrl } = request.body as any;

      if (!source || !sourceUrl) {
        reply.code(400).send({ error: 'source and sourceUrl are required' });
        return;
      }

      try {
        const aggregator = new TorrentSearchAggregator([source]);
        const magnetUri = await aggregator.getMagnetLink({
          source,
          sourceUrl,
        } as any);

        return { magnetUri };
      } catch (error: any) {
        logger.error('Magnet fetch failed:', error);
        reply.code(500).send({ error: 'Failed to fetch magnet', details: error.message });
      }
    });

    // Get search cache
    this.fastify.get('/v1/search/cache', async (request) => {
      const { query_hash } = request.query as any;
      if (!query_hash) {
        return { error: 'query_hash required' };
      }

      const cache = await this.database.getSearchCache(query_hash);
      return { cache };
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
      this.vpnChecker.startMonitoring(() => {
        logger.warn('VPN disconnected! Pausing all active downloads');
        // TODO: Pause all active downloads
      });
    } catch (error) {
      logger.error('Failed to start server', error);
      throw error;
    }
  }

  async stop(): Promise<void> {
    this.vpnChecker.stopMonitoring();
    await this.fastify.close();
    logger.info('Server stopped');
  }
}
