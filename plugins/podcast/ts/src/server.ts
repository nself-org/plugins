/**
 * Podcast Plugin Server
 * HTTP server for podcast management API endpoints
 */

import express from 'express';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { PodcastDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreatePodcastRequest,
  UpdatePodcastRequest,
  ListPodcastsQuery,
  ListEpisodesQuery,
  SearchPodcastsRequest,
  SearchEpisodesRequest,
  SubscribeRequest,
  UpdateSubscriptionRequest,
  UpdatePlaybackPositionRequest,
  GetPlaybackPositionsQuery,
  SyncFeedRequest,
} from './types.js';

const logger = createLogger('podcast:server');

// =========================================================================
// Express middleware helpers
// =========================================================================

function asyncHandler(fn: (req: express.Request, res: express.Response) => Promise<void>) {
  return (req: express.Request, res: express.Response, next: express.NextFunction) => {
    fn(req, res).catch(next);
  };
}

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new PodcastDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Express server
  const app = express();

  app.use(express.json({ limit: '10mb' }));

  // CORS
  app.use((_req, res, next) => {
    res.header('Access-Control-Allow-Origin', '*');
    res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
    res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-App-ID, X-Source-Account-ID');
    if (_req.method === 'OPTIONS') {
      res.sendStatus(204);
      return;
    }
    next();
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.use(createRateLimitHook(rateLimiter) as express.RequestHandler);

  if (fullConfig.security.apiKey) {
    app.use(createAuthHook(fullConfig.security.apiKey) as express.RequestHandler);
    logger.info('API key authentication enabled');
  }

  // Multi-app context middleware
  app.use((req: express.Request, _res: express.Response, next: express.NextFunction) => {
    const ctx = getAppContext(req);
    (req as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    next();
  });

  function scopedDb(req: express.Request): PodcastDatabase {
    return (req as unknown as Record<string, unknown>).scopedDb as PodcastDatabase;
  }

  // =========================================================================
  // Health Endpoints
  // =========================================================================

  app.get('/health', (_req, res) => {
    res.json({ status: 'ok', plugin: 'podcast', timestamp: new Date().toISOString() });
  });

  app.get('/ready', asyncHandler(async (_req, res) => {
    try {
      await db.query('SELECT 1');
      res.json({ ready: true, plugin: 'podcast', timestamp: new Date().toISOString() });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      res.status(503).json({
        ready: false,
        plugin: 'podcast',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  }));

  app.get('/live', asyncHandler(async (req, res) => {
    const stats = await scopedDb(req).getStats();
    res.json({
      alive: true,
      plugin: 'podcast',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        totalPodcasts: stats.total_podcasts,
        activePodcasts: stats.active_podcasts,
        totalEpisodes: stats.total_episodes,
      },
      timestamp: new Date().toISOString(),
    });
  }));

  // =========================================================================
  // Podcast Endpoints
  // =========================================================================

  app.post('/api/podcasts', asyncHandler(async (req, res) => {
    try {
      const body = req.body as CreatePodcastRequest;
      if (!body.feed_url) {
        res.status(400).json({ error: 'feed_url is required' });
        return;
      }

      const existing = await scopedDb(req).getPodcastByFeedUrl(body.feed_url);
      if (existing) {
        res.status(409).json({ error: 'Podcast with this feed URL already exists', podcast: existing });
        return;
      }

      const podcast = await scopedDb(req).createPodcast({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        title: body.title ?? 'Untitled Podcast',
        description: body.description ?? null,
        author: body.author ?? null,
        feed_url: body.feed_url,
        website_url: body.website_url ?? null,
        image_url: body.image_url ?? null,
        language: body.language ?? 'en',
        categories: body.categories ?? [],
        explicit: body.explicit ?? false,
        last_fetched_at: null,
        last_published_at: null,
        etag: null,
        last_modified: null,
        feed_status: 'active',
        episode_count: 0,
        metadata: {},
      });

      res.status(201).json(podcast);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create podcast', { error: message });
      res.status(500).json({ error: message });
    }
  }));

  app.get('/api/podcasts', asyncHandler(async (req, res) => {
    const query = req.query as unknown as ListPodcastsQuery;
    const podcasts = await scopedDb(req).listPodcasts({
      category: query.category,
      language: query.language,
      feedStatus: query.feed_status,
      limit: query.limit ? parseInt(String(query.limit), 10) : 200,
      offset: query.offset ? parseInt(String(query.offset), 10) : undefined,
    });

    res.json({ podcasts, count: podcasts.length });
  }));

  app.get('/api/podcasts/:id', asyncHandler(async (req, res) => {
    const podcast = await scopedDb(req).getPodcast(req.params.id);
    if (!podcast) {
      res.status(404).json({ error: 'Podcast not found' });
      return;
    }
    res.json(podcast);
  }));

  app.put('/api/podcasts/:id', asyncHandler(async (req, res) => {
    const body = req.body as UpdatePodcastRequest;
    const podcast = await scopedDb(req).updatePodcast(req.params.id, body);
    if (!podcast) {
      res.status(404).json({ error: 'Podcast not found' });
      return;
    }
    res.json(podcast);
  }));

  app.delete('/api/podcasts/:id', asyncHandler(async (req, res) => {
    const deleted = await scopedDb(req).deletePodcast(req.params.id);
    if (!deleted) {
      res.status(404).json({ error: 'Podcast not found' });
      return;
    }
    res.status(204).send();
  }));

  app.get('/api/podcasts/search', asyncHandler(async (req, res) => {
    const query = req.query as unknown as SearchPodcastsRequest;
    if (!query.query) {
      res.status(400).json({ error: 'query parameter is required' });
      return;
    }

    const podcasts = await scopedDb(req).searchPodcasts({
      query: query.query,
      category: query.category,
      language: query.language,
      limit: query.limit ? parseInt(String(query.limit), 10) : 50,
    });

    res.json({ podcasts, count: podcasts.length });
  }));

  // =========================================================================
  // Episode Endpoints
  // =========================================================================

  app.get('/api/episodes', asyncHandler(async (req, res) => {
    const query = req.query as unknown as ListEpisodesQuery;
    const episodes = await scopedDb(req).listEpisodes({
      podcastId: query.podcast_id,
      seasonNumber: query.season_number ? parseInt(String(query.season_number), 10) : undefined,
      episodeType: query.episode_type,
      limit: query.limit ? parseInt(String(query.limit), 10) : 200,
      offset: query.offset ? parseInt(String(query.offset), 10) : undefined,
    });

    res.json({ episodes, count: episodes.length });
  }));

  app.get('/api/episodes/:id', asyncHandler(async (req, res) => {
    const episode = await scopedDb(req).getEpisode(req.params.id);
    if (!episode) {
      res.status(404).json({ error: 'Episode not found' });
      return;
    }
    res.json(episode);
  }));

  app.delete('/api/episodes/:id', asyncHandler(async (req, res) => {
    const deleted = await scopedDb(req).deleteEpisode(req.params.id);
    if (!deleted) {
      res.status(404).json({ error: 'Episode not found' });
      return;
    }
    res.status(204).send();
  }));

  app.get('/api/episodes/search', asyncHandler(async (req, res) => {
    const query = req.query as unknown as SearchEpisodesRequest;
    if (!query.query) {
      res.status(400).json({ error: 'query parameter is required' });
      return;
    }

    const episodes = await scopedDb(req).searchEpisodes({
      query: query.query,
      podcastId: query.podcast_id,
      limit: query.limit ? parseInt(String(query.limit), 10) : 50,
    });

    res.json({ episodes, count: episodes.length });
  }));

  // =========================================================================
  // Subscription Endpoints
  // =========================================================================

  app.post('/api/subscriptions', asyncHandler(async (req, res) => {
    try {
      const body = req.body as SubscribeRequest;
      if (!body.user_id || !body.podcast_id) {
        res.status(400).json({ error: 'user_id and podcast_id are required' });
        return;
      }

      const podcast = await scopedDb(req).getPodcast(body.podcast_id);
      if (!podcast) {
        res.status(404).json({ error: 'Podcast not found' });
        return;
      }

      const subscription = await scopedDb(req).createSubscription({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        user_id: body.user_id,
        podcast_id: body.podcast_id,
        is_active: true,
        notification_enabled: body.notification_enabled ?? true,
        auto_download: body.auto_download ?? false,
        metadata: {},
      });

      res.status(201).json(subscription);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create subscription', { error: message });
      res.status(500).json({ error: message });
    }
  }));

  app.get('/api/subscriptions/:userId', asyncHandler(async (req, res) => {
    const subscriptions = await scopedDb(req).listUserSubscriptions(req.params.userId);
    res.json({ subscriptions, count: subscriptions.length });
  }));

  app.put('/api/subscriptions/:id', asyncHandler(async (req, res) => {
    const body = req.body as UpdateSubscriptionRequest;
    const subscription = await scopedDb(req).updateSubscription(req.params.id, body);
    if (!subscription) {
      res.status(404).json({ error: 'Subscription not found' });
      return;
    }
    res.json(subscription);
  }));

  app.delete('/api/subscriptions/:id', asyncHandler(async (req, res) => {
    const deleted = await scopedDb(req).deleteSubscription(req.params.id);
    if (!deleted) {
      res.status(404).json({ error: 'Subscription not found' });
      return;
    }
    res.status(204).send();
  }));

  // =========================================================================
  // Playback Position Endpoints
  // =========================================================================

  app.post('/api/playback', asyncHandler(async (req, res) => {
    try {
      const body = req.body as UpdatePlaybackPositionRequest;
      if (!body.user_id || !body.episode_id || body.position_seconds === undefined) {
        res.status(400).json({ error: 'user_id, episode_id, and position_seconds are required' });
        return;
      }

      const position = await scopedDb(req).upsertPlaybackPosition({
        source_account_id: scopedDb(req).getCurrentSourceAccountId(),
        user_id: body.user_id,
        episode_id: body.episode_id,
        position_seconds: body.position_seconds,
        duration_seconds: body.duration_seconds ?? null,
        completed: body.completed ?? false,
        completed_at: null,
        device_id: body.device_id ?? null,
      });

      res.json(position);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update playback position', { error: message });
      res.status(500).json({ error: message });
    }
  }));

  app.get('/api/playback', asyncHandler(async (req, res) => {
    const query = req.query as unknown as GetPlaybackPositionsQuery;
    if (!query.user_id) {
      res.status(400).json({ error: 'user_id is required' });
      return;
    }

    if (query.episode_ids) {
      const episodeIds = query.episode_ids.split(',').map(s => s.trim()).filter(Boolean);
      const positions = await scopedDb(req).getPlaybackPositions(query.user_id, episodeIds);
      res.json({ positions, count: positions.length });
    } else {
      const inProgress = await scopedDb(req).getUserInProgressEpisodes(query.user_id);
      res.json({ in_progress: inProgress, count: inProgress.length });
    }
  }));

  // =========================================================================
  // Sync Endpoints
  // =========================================================================

  app.post('/api/sync', asyncHandler(async (req, res) => {
    try {
      const body = req.body as SyncFeedRequest;
      const podcasts = body.podcast_id
        ? [await scopedDb(req).getPodcast(body.podcast_id)].filter(Boolean) as import('./types.js').PodcastRecord[]
        : await scopedDb(req).getPodcastsForSync();

      if (podcasts.length === 0) {
        res.json({ message: 'No podcasts to sync', results: [] });
        return;
      }

      logger.info(`Starting sync for ${podcasts.length} podcast(s)`);
      const results: import('./types.js').SyncResult[] = [];

      for (const podcast of podcasts) {
        try {
          // Dynamic import of rss-parser
          const RssParser = (await import('rss-parser')).default;
          const parser = new RssParser({
            timeout: fullConfig.feedTimeoutSeconds * 1000,
            headers: {
              'User-Agent': 'nself-podcast-plugin/1.0.0',
              ...(podcast.etag ? { 'If-None-Match': podcast.etag } : {}),
              ...(podcast.last_modified ? { 'If-Modified-Since': podcast.last_modified } : {}),
            },
          });

          const feed = await parser.parseURL(podcast.feed_url);

          let newEpisodes = 0;
          let updatedEpisodes = 0;
          const errors: string[] = [];

          // Update podcast metadata from feed
          await scopedDb(req).updatePodcast(podcast.id, {
            title: feed.title ?? podcast.title,
            description: feed.description ?? podcast.description,
            image_url: feed.image?.url ?? podcast.image_url,
            last_fetched_at: new Date(),
            last_published_at: feed.lastBuildDate ? new Date(feed.lastBuildDate) : podcast.last_published_at,
            feed_status: 'active',
          });

          // Process episodes
          const maxEpisodes = fullConfig.maxEpisodesPerFeed;
          const items = (feed.items ?? []).slice(0, maxEpisodes);

          for (const item of items) {
            try {
              const enclosure = item.enclosure;
              if (!enclosure?.url) continue;

              const guid = item.guid ?? item.link ?? enclosure.url;

              await scopedDb(req).createEpisode({
                source_account_id: scopedDb(req).getCurrentSourceAccountId(),
                podcast_id: podcast.id,
                guid,
                title: item.title ?? 'Untitled Episode',
                description: item.contentSnippet ?? item.content ?? null,
                content_html: item.content ?? null,
                published_at: item.pubDate ? new Date(item.pubDate) : null,
                duration_seconds: parseDuration(item.itunes?.duration),
                audio_url: enclosure.url,
                audio_type: enclosure.type ?? null,
                audio_size_bytes: enclosure.length ? parseInt(String(enclosure.length), 10) : null,
                image_url: item.itunes?.image ?? null,
                season_number: item.itunes?.season ? parseInt(String(item.itunes.season), 10) : null,
                episode_number: item.itunes?.episode ? parseInt(String(item.itunes.episode), 10) : null,
                episode_type: item.itunes?.episodeType ?? 'full',
                explicit: item.itunes?.explicit === 'yes' || item.itunes?.explicit === 'true',
                transcript_url: null,
                chapters_url: null,
                metadata: {},
              });

              newEpisodes++;
            } catch (episodeError) {
              const msg = episodeError instanceof Error ? episodeError.message : 'Unknown error';
              errors.push(`Episode "${item.title}": ${msg}`);
            }
          }

          // Update episode count
          const episodeCount = await scopedDb(req).getEpisodeCountForPodcast(podcast.id);
          await scopedDb(req).updatePodcast(podcast.id, { episode_count: episodeCount });

          results.push({
            podcast_id: podcast.id,
            podcast_title: podcast.title,
            new_episodes: newEpisodes,
            updated_episodes: updatedEpisodes,
            errors,
          });
        } catch (feedError) {
          const msg = feedError instanceof Error ? feedError.message : 'Unknown error';
          logger.error(`Failed to sync podcast ${podcast.title}`, { error: msg });

          await scopedDb(req).updatePodcast(podcast.id, {
            feed_status: 'error',
            last_fetched_at: new Date(),
          });

          results.push({
            podcast_id: podcast.id,
            podcast_title: podcast.title,
            new_episodes: 0,
            updated_episodes: 0,
            errors: [msg],
          });
        }
      }

      const totalNew = results.reduce((sum, r) => sum + r.new_episodes, 0);
      const totalErrors = results.reduce((sum, r) => sum + r.errors.length, 0);

      logger.info(`Sync complete: ${totalNew} new episodes, ${totalErrors} errors`);
      res.json({ message: 'Sync complete', results });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      res.status(500).json({ error: message });
    }
  }));

  // =========================================================================
  // Category Endpoints
  // =========================================================================

  app.get('/api/categories', asyncHandler(async (req, res) => {
    const categories = await scopedDb(req).listCategories();
    res.json({ categories, count: categories.length });
  }));

  // =========================================================================
  // Stats Endpoint
  // =========================================================================

  app.get('/api/stats', asyncHandler(async (req, res) => {
    const stats = await scopedDb(req).getStats();
    res.json(stats);
  }));

  // =========================================================================
  // Error handler
  // =========================================================================

  app.use((err: Error, _req: express.Request, res: express.Response, _next: express.NextFunction) => {
    logger.error('Unhandled error', { error: err.message });
    res.status(500).json({ error: 'Internal server error' });
  });

  // =========================================================================
  // Server start/stop
  // =========================================================================

  let server: ReturnType<typeof app.listen> | null = null;

  return {
    app,
    db,
    config: fullConfig,

    async start() {
      return new Promise<void>((resolve) => {
        server = app.listen(fullConfig.port, fullConfig.host, () => {
          logger.info(`Podcast server listening on ${fullConfig.host}:${fullConfig.port}`);
          resolve();
        });
      });
    },

    async stop() {
      if (server) {
        await new Promise<void>((resolve) => {
          server!.close(() => resolve());
        });
      }
      await db.disconnect();
      logger.info('Podcast server stopped');
    },
  };
}

export async function startServer(config?: Partial<Config>) {
  const server = await createServer(config);
  await server.start();
  return server;
}

// =========================================================================
// Utility Functions
// =========================================================================

/**
 * Parse duration string (HH:MM:SS, MM:SS, or seconds) to seconds
 */
function parseDuration(duration: string | undefined | null): number | null {
  if (!duration) return null;

  // Already a number (seconds)
  const asNumber = parseInt(duration, 10);
  if (!isNaN(asNumber) && String(asNumber) === duration.trim()) {
    return asNumber;
  }

  // HH:MM:SS or MM:SS
  const parts = duration.split(':').map(p => parseInt(p.trim(), 10));
  if (parts.some(isNaN)) return null;

  if (parts.length === 3) {
    return parts[0] * 3600 + parts[1] * 60 + parts[2];
  }
  if (parts.length === 2) {
    return parts[0] * 60 + parts[1];
  }

  return null;
}
