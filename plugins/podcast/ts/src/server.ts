/**
 * Podcast Plugin Server
 * Fastify HTTP server for podcast feed management, episode browsing, and discovery
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
import { PodcastDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import { discoverPodcasts } from './discovery.js';
import { parseOpml, extractFeedUrls, generateOpml } from './opml.js';
import { downloadEpisode } from './downloader.js';
import { FeedScheduler } from './scheduler.js';
import type {
  SubscribeFeedRequest,
  DiscoverRequest,
  ImportOpmlRequest,
} from './types.js';

const logger = createLogger('podcast:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new PodcastDatabase();
  await db.connect();
  await db.initializeSchema();

  // Initialize scheduler
  const scheduler = new FeedScheduler(db, fullConfig);

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for OPML imports
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

  function scopedDb(request: unknown): PodcastDatabase {
    return (request as Record<string, unknown>).scopedDb as PodcastDatabase;
  }

  // =========================================================================
  // Health & Status
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'podcast', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'podcast', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'podcast',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/v1/stats', async (request) => {
    return await scopedDb(request).getStats();
  });

  // =========================================================================
  // Feed Management
  // =========================================================================

  // Subscribe to a podcast feed
  app.post('/v1/feeds', async (request, reply) => {
    const { url, title } = request.body as SubscribeFeedRequest;

    if (!url) {
      return reply.status(400).send({ error: 'url is required' });
    }

    // Validate URL format
    try {
      new URL(url);
    } catch {
      return reply.status(400).send({ error: 'Invalid URL format' });
    }

    const sdb = scopedDb(request);

    try {
      // Insert the feed record
      const feed = await sdb.insertFeed(url, title);

      // Fetch and parse the feed to populate metadata and episodes
      let episodeCount = 0;
      try {
        const feedScheduler = new FeedScheduler(sdb, fullConfig);
        const result = await feedScheduler.refreshFeed(feed);
        episodeCount = result.totalEpisodes;
      } catch (parseError) {
        const message = parseError instanceof Error ? parseError.message : 'Unknown error';
        logger.warn('Failed to fetch feed on subscribe, will retry later', { url, error: message });
      }

      // Re-fetch feed to get updated metadata
      const updatedFeed = await sdb.getFeed(feed.id);

      return reply.status(201).send({
        id: feed.id,
        title: updatedFeed?.title ?? feed.title,
        url: feed.url,
        episode_count: episodeCount,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to subscribe to feed', { url, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // List feed subscriptions
  app.get('/v1/feeds', async (request) => {
    const feeds = await scopedDb(request).listFeeds();
    return { data: feeds, total: feeds.length };
  });

  // Get feed detail
  app.get('/v1/feeds/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const sdb = scopedDb(request);
    const feed = await sdb.getFeed(id);

    if (!feed) {
      return reply.status(404).send({ error: 'Feed not found' });
    }

    // Include recent episodes
    const episodes = await sdb.listEpisodes(id, 10, 0);
    return { ...feed, recent_episodes: episodes };
  });

  // Unsubscribe from a feed
  app.delete('/v1/feeds/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteFeed(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Feed not found' });
    }

    return { success: true };
  });

  // Force refresh a feed
  app.post('/v1/feeds/:id/refresh', async (request, reply) => {
    const { id } = request.params as { id: string };
    const sdb = scopedDb(request);
    const feed = await sdb.getFeed(id);

    if (!feed) {
      return reply.status(404).send({ error: 'Feed not found' });
    }

    try {
      const feedScheduler = new FeedScheduler(sdb, fullConfig);
      const result = await feedScheduler.refreshFeed(feed);

      // Re-fetch for updated title
      const updatedFeed = await sdb.getFeed(id);

      return {
        id: feed.id,
        title: updatedFeed?.title ?? feed.title,
        new_episodes: result.newEpisodes,
        total_episodes: result.totalEpisodes,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // List episodes for a feed
  app.get('/v1/feeds/:id/episodes', async (request, reply) => {
    const { id } = request.params as { id: string };
    const { limit = 50, offset = 0 } = request.query as { limit?: number; offset?: number };
    const sdb = scopedDb(request);

    const feed = await sdb.getFeed(id);
    if (!feed) {
      return reply.status(404).send({ error: 'Feed not found' });
    }

    const episodes = await sdb.listEpisodes(id, Number(limit), Number(offset));
    const total = await sdb.countEpisodes(id);
    return { data: episodes, total, limit: Number(limit), offset: Number(offset) };
  });

  // =========================================================================
  // Episode Management
  // =========================================================================

  // Get episode detail
  app.get('/v1/episodes/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const episode = await scopedDb(request).getEpisode(id);

    if (!episode) {
      return reply.status(404).send({ error: 'Episode not found' });
    }

    return episode;
  });

  // Download episode audio
  app.post('/v1/episodes/:id/download', async (request, reply) => {
    const { id } = request.params as { id: string };
    const sdb = scopedDb(request);
    const episode = await sdb.getEpisode(id);

    if (!episode) {
      return reply.status(404).send({ error: 'Episode not found' });
    }

    if (!episode.enclosure_url) {
      return reply.status(400).send({ error: 'Episode has no audio URL' });
    }

    if (episode.downloaded && episode.download_path) {
      return { download_path: episode.download_path };
    }

    try {
      const downloadPath = await downloadEpisode(
        episode,
        fullConfig.downloadPath,
        sdb
      );
      return { download_path: downloadPath };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // Get new (unplayed) episodes across all feeds
  app.get('/v1/new-episodes', async (request) => {
    const { limit = 50 } = request.query as { limit?: number };
    const episodes = await scopedDb(request).getNewEpisodes(Number(limit));
    return { data: episodes, total: episodes.length };
  });

  // =========================================================================
  // Discovery
  // =========================================================================

  // Search for podcasts
  app.post('/v1/discover', async (request, reply) => {
    const { query, limit = 25 } = request.body as DiscoverRequest;

    if (!query || query.trim().length === 0) {
      return reply.status(400).send({ error: 'query is required' });
    }

    try {
      const results = await discoverPodcasts(query, fullConfig, limit);
      return { data: results, total: results.length };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // OPML Import/Export
  // =========================================================================

  // Import OPML
  app.post('/v1/import/opml', async (request, reply) => {
    const { opml_content } = request.body as ImportOpmlRequest;

    if (!opml_content) {
      return reply.status(400).send({ error: 'opml_content is required' });
    }

    const sdb = scopedDb(request);
    const errors: string[] = [];
    let imported = 0;

    try {
      const outlines = parseOpml(opml_content);
      const feedUrls = extractFeedUrls(outlines);

      for (const { url, title } of feedUrls) {
        try {
          await sdb.insertFeed(url, title);
          imported++;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`${url}: ${message}`);
        }
      }

      // Trigger initial fetch for newly imported feeds in background
      void (async () => {
        const feeds = await sdb.listFeeds('active');
        const feedScheduler = new FeedScheduler(sdb, fullConfig);
        for (const feed of feeds) {
          if (!feed.last_fetched_at) {
            try {
              await feedScheduler.refreshFeed(feed);
            } catch {
              // Errors already logged by scheduler
            }
          }
        }
      })();

      return { imported, errors };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return reply.status(400).send({ error: message });
    }
  });

  // Export OPML
  app.post('/v1/export/opml', async (request) => {
    const feeds = await scopedDb(request).listFeeds();
    const opml = generateOpml(feeds);
    return { opml };
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const shutdown = async () => {
    logger.info('Shutting down...');
    scheduler.stop();
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    scheduler,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      scheduler.start();
      logger.success(`Podcast plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
      logger.info('Feed refresh scheduler started');
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
