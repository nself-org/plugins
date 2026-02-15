/**
 * Content Acquisition API Server
 */

import Fastify, { FastifyRequest, FastifyReply } from 'fastify';
import cors from '@fastify/cors';
import Parser from 'rss-parser';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext, loadSecurityConfig } from '@nself/plugin-utils';
import { ContentAcquisitionDatabase } from './database.js';
import { RSSFeedMonitor } from './rss-monitor.js';
import { RSSMonitor } from './rss.js';
import { PipelineOrchestrator } from './pipeline.js';
import { DownloadStateMachine } from './state-machine.js';
import { listQualityPresets } from './quality-profiles.js';
import type { ContentAcquisitionConfig, PipelineTriggerRequest, DownloadState } from './types.js';
import type { MatchCriteria } from './matcher.js';

const logger = createLogger('content-acquisition:server');

// Type guards for validated enums
type SubscriptionType = 'tv_show' | 'movie_collection' | 'artist' | 'podcast';
type FeedType = 'tv_shows' | 'movies' | 'anime' | 'music';
type ContentType = 'movie' | 'tv_episode' | 'music' | 'other';
type DownloadStatus = 'scheduled' | 'searching' | 'downloading' | 'downloaded' | 'failed';
type DownloadAction = 'auto-download' | 'notify' | 'skip';

// ============================================================================
// JSON Schema definitions for request validation
// ============================================================================

const subscriptionBodySchema = {
  type: 'object' as const,
  required: ['contentType', 'contentName'],
  properties: {
    contentType: { type: 'string', enum: ['tv_show', 'movie_collection', 'artist', 'podcast'] },
    contentId: { type: 'string' },
    contentName: { type: 'string', minLength: 1, maxLength: 255 },
    qualityProfileId: { type: 'string', format: 'uuid' },
  },
  additionalProperties: false,
};

const subscriptionUpdateBodySchema = {
  type: 'object' as const,
  properties: {
    contentType: { type: 'string', enum: ['tv_show', 'movie_collection', 'artist', 'podcast'] },
    contentId: { type: 'string' },
    contentName: { type: 'string', minLength: 1, maxLength: 255 },
    qualityProfileId: { type: 'string', format: 'uuid' },
    enabled: { type: 'boolean' },
    autoUpgrade: { type: 'boolean' },
  },
  additionalProperties: false,
};

const feedBodySchema = {
  type: 'object' as const,
  required: ['name', 'url', 'feedType'],
  properties: {
    name: { type: 'string', minLength: 1, maxLength: 255 },
    url: { type: 'string', minLength: 1, format: 'uri' },
    feedType: { type: 'string', enum: ['tv_shows', 'movies', 'anime', 'music'] },
  },
  additionalProperties: false,
};

const feedUpdateBodySchema = {
  type: 'object' as const,
  properties: {
    name: { type: 'string', minLength: 1, maxLength: 255 },
    url: { type: 'string', minLength: 1, format: 'uri' },
    feedType: { type: 'string', enum: ['tv_shows', 'movies', 'anime', 'music'] },
    enabled: { type: 'boolean' },
    checkIntervalMinutes: { type: 'integer', minimum: 5, maximum: 1440 },
  },
  additionalProperties: false,
};

const feedValidateBodySchema = {
  type: 'object' as const,
  required: ['url'],
  properties: {
    url: { type: 'string', minLength: 1, format: 'uri' },
  },
  additionalProperties: false,
};

const queueBodySchema = {
  type: 'object' as const,
  required: ['contentType', 'contentName'],
  properties: {
    contentType: { type: 'string', enum: ['movie', 'tv_episode', 'music', 'other'] },
    contentName: { type: 'string', minLength: 1, maxLength: 255 },
    year: { type: 'integer', minimum: 1900, maximum: 2100 },
    season: { type: 'integer', minimum: 0, maximum: 200 },
    episode: { type: 'integer', minimum: 0, maximum: 10000 },
  },
  additionalProperties: false,
};

const profileBodySchema = {
  type: 'object' as const,
  required: ['name'],
  properties: {
    name: { type: 'string', minLength: 1, maxLength: 100 },
    preferredQualities: {
      type: 'array',
      items: { type: 'string', enum: ['2160p', '1080p', '720p', '480p'] },
      minItems: 1,
    },
    minSeeders: { type: 'integer', minimum: 0, maximum: 10000 },
  },
  additionalProperties: false,
};

const pipelineTriggerBodySchema = {
  type: 'object' as const,
  required: ['content_title'],
  properties: {
    content_title: { type: 'string', minLength: 1, maxLength: 500 },
    content_type: { type: 'string', maxLength: 100 },
    magnet_url: { type: 'string', maxLength: 2048 },
    torrent_url: { type: 'string', maxLength: 2048 },
  },
  additionalProperties: false,
};

const movieBodySchema = {
  type: 'object' as const,
  required: ['title'],
  properties: {
    title: { type: 'string', minLength: 1, maxLength: 500 },
    tmdbId: { type: 'integer' },
    qualityProfile: { type: 'string', enum: ['minimal', 'balanced', '4k_premium'] },
    autoDownload: { type: 'boolean' },
    autoUpgrade: { type: 'boolean' },
  },
  additionalProperties: false,
};

const movieUpdateBodySchema = {
  type: 'object' as const,
  properties: {
    title: { type: 'string', minLength: 1, maxLength: 500 },
    tmdbId: { type: 'integer' },
    qualityProfile: { type: 'string', enum: ['minimal', 'balanced', '4k_premium'] },
    autoDownload: { type: 'boolean' },
    autoUpgrade: { type: 'boolean' },
    status: { type: 'string', enum: ['scheduled', 'searching', 'downloading', 'downloaded', 'failed'] },
  },
  additionalProperties: false,
};

const downloadBodySchema = {
  type: 'object' as const,
  required: ['contentType', 'title'],
  properties: {
    contentType: { type: 'string', minLength: 1, maxLength: 100 },
    title: { type: 'string', minLength: 1, maxLength: 500 },
    magnetUri: { type: 'string', maxLength: 2048 },
    qualityProfile: { type: 'string', enum: ['minimal', 'balanced', '4k_premium'] },
    showId: { type: 'string', format: 'uuid' },
    seasonNumber: { type: 'integer', minimum: 0 },
    episodeNumber: { type: 'integer', minimum: 0 },
    tmdbId: { type: 'integer' },
  },
  additionalProperties: false,
};

const ruleBodySchema = {
  type: 'object' as const,
  required: ['name', 'conditions', 'action'],
  properties: {
    name: { type: 'string', minLength: 1, maxLength: 255 },
    conditions: { type: 'object' },
    action: { type: 'string', enum: ['auto-download', 'notify', 'skip'] },
    priority: { type: 'integer', minimum: 0, maximum: 100 },
    enabled: { type: 'boolean' },
  },
  additionalProperties: false,
};

const ruleUpdateBodySchema = {
  type: 'object' as const,
  properties: {
    name: { type: 'string', minLength: 1, maxLength: 255 },
    conditions: { type: 'object' },
    action: { type: 'string', enum: ['auto-download', 'notify', 'skip'] },
    priority: { type: 'integer', minimum: 0, maximum: 100 },
    enabled: { type: 'boolean' },
  },
  additionalProperties: false,
};

const ruleTestBodySchema = {
  type: 'object' as const,
  required: ['sample'],
  properties: {
    sample: { type: 'object' },
  },
  additionalProperties: false,
};

const rssPollBodySchema = {
  type: 'object' as const,
  required: ['url', 'criteria'],
  properties: {
    url: { type: 'string', format: 'uri' },
    criteria: {
      type: 'array',
      items: {
        type: 'object',
        required: ['title'],
        properties: {
          title: { type: 'string' },
          year: { type: 'integer' },
          quality: { type: 'array', items: { type: 'string' } },
          category: { type: 'string' },
        },
      },
    },
    lastSeen: { type: 'string', format: 'date-time' },
  },
  additionalProperties: false,
};

const rssTestBodySchema = {
  type: 'object' as const,
  required: ['url'],
  properties: {
    url: { type: 'string', format: 'uri' },
  },
  additionalProperties: false,
};

// ============================================================================
// Server
// ============================================================================

export class ContentAcquisitionServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: ContentAcquisitionDatabase;
  private config: ContentAcquisitionConfig;
  private pipeline: PipelineOrchestrator;
  private stateMachine: DownloadStateMachine;
  private rssMonitor: RSSMonitor;

  constructor(config: ContentAcquisitionConfig, database: ContentAcquisitionDatabase) {
    this.config = config;
    this.database = database;
    this.fastify = Fastify({ logger: false });
    this.pipeline = new PipelineOrchestrator(database, config);
    this.stateMachine = new DownloadStateMachine(database.getPool());
    this.rssMonitor = new RSSMonitor();
  }

  async initialize(): Promise<void> {
    await this.fastify.register(cors);

    // Security: load config from environment and wire up auth + rate limiting hooks
    const security = loadSecurityConfig('CONTENT_ACQUISITION');
    const rateLimiter = new ApiRateLimiter(security.rateLimitMax ?? 100, security.rateLimitWindowMs ?? 60000);
    this.fastify.addHook('preHandler', createRateLimitHook(rateLimiter) as never);
    if (security.apiKey) {
      this.fastify.addHook('preHandler', createAuthHook(security.apiKey) as never);
    }

    this.registerRoutes();

    // Start the RSS feed monitor with cron-based scheduled checks
    const rssMonitor = new RSSFeedMonitor(
      this.database,
      this.config.torrent_manager_url || 'http://localhost:3100',
      this.pipeline,
    );
    rssMonitor.startScheduledChecks(this.config.rss_check_interval || 30);

    logger.info('Server initialized');
  }

  private registerRoutes(): void {
    // Health check (no auth required -- excluded by createAuthHook)
    this.fastify.get('/health', async () => {
      return { status: 'ok', timestamp: new Date().toISOString() };
    });

    // -----------------------------------------------------------------------
    // Subscriptions
    // -----------------------------------------------------------------------

    this.fastify.post('/v1/subscriptions', {
      schema: { body: subscriptionBodySchema },
    }, async (request: FastifyRequest<{ Body: { contentType: string; contentId?: string; contentName: string; qualityProfileId?: string } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { contentType, contentId, contentName, qualityProfileId } = request.body;
      const sub = await this.database.createSubscription({
        source_account_id: sourceAccountId,
        subscription_type: contentType as SubscriptionType,
        content_id: contentId,
        content_name: contentName,
        quality_profile_id: qualityProfileId,
      });
      return { subscription: sub };
    });

    this.fastify.get('/v1/subscriptions', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const subs = await this.database.listSubscriptions(sourceAccountId);
      return { subscriptions: subs };
    });

    this.fastify.get('/v1/subscriptions/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const sub = await this.database.getSubscription(request.params.id);
      if (!sub) {
        return reply.status(404).send({ error: 'Subscription not found' });
      }
      return { subscription: sub };
    });

    this.fastify.put('/v1/subscriptions/:id', {
      schema: { body: subscriptionUpdateBodySchema },
    }, async (request: FastifyRequest<{ Params: { id: string }; Body: { contentType?: string; contentId?: string; contentName?: string; qualityProfileId?: string; enabled?: boolean; autoUpgrade?: boolean } }>, reply: FastifyReply) => {
      const { contentType, contentId, contentName, qualityProfileId, enabled, autoUpgrade } = request.body;
      const sub = await this.database.updateSubscription(request.params.id, {
        subscription_type: contentType as SubscriptionType | undefined,
        content_id: contentId,
        content_name: contentName,
        quality_profile_id: qualityProfileId,
        enabled,
        auto_upgrade: autoUpgrade,
      });
      if (!sub) {
        return reply.status(404).send({ error: 'Subscription not found' });
      }
      return { subscription: sub };
    });

    this.fastify.delete('/v1/subscriptions/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const deleted = await this.database.deleteSubscription(request.params.id);
      if (!deleted) {
        return reply.status(404).send({ error: 'Subscription not found' });
      }
      return { deleted: true };
    });

    // -----------------------------------------------------------------------
    // RSS Feeds
    // -----------------------------------------------------------------------

    this.fastify.post('/v1/feeds', {
      schema: { body: feedBodySchema },
    }, async (request: FastifyRequest<{ Body: { name: string; url: string; feedType: string } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { name, url, feedType } = request.body;
      const feed = await this.database.createRSSFeed({
        source_account_id: sourceAccountId,
        name,
        url,
        feed_type: feedType as FeedType,
      });
      return { feed };
    });

    this.fastify.get('/v1/feeds', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const feeds = await this.database.listRSSFeeds(sourceAccountId);
      return { feeds };
    });

    this.fastify.post('/v1/feeds/validate', {
      schema: { body: feedValidateBodySchema },
    }, async (request: FastifyRequest<{ Body: { url: string } }>, reply: FastifyReply) => {
      const { url } = request.body;
      try {
        const parser = new Parser();
        const feedData = await parser.parseURL(url);
        const latestDate = feedData.items[0]?.pubDate
          ? new Date(feedData.items[0].pubDate).toISOString()
          : undefined;
        return {
          valid: true,
          title: feedData.title,
          item_count: feedData.items.length,
          latest_item_date: latestDate,
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        return reply.status(422).send({
          valid: false,
          error: `Failed to parse feed: ${message}`,
        });
      }
    });

    this.fastify.put('/v1/feeds/:id', {
      schema: { body: feedUpdateBodySchema },
    }, async (request: FastifyRequest<{ Params: { id: string }; Body: { name?: string; url?: string; feedType?: string; enabled?: boolean; checkIntervalMinutes?: number } }>, reply: FastifyReply) => {
      const { name, url, feedType, enabled, checkIntervalMinutes } = request.body;
      const feed = await this.database.updateRSSFeed(request.params.id, {
        name,
        url,
        feed_type: feedType as FeedType | undefined,
        enabled,
        check_interval_minutes: checkIntervalMinutes,
      });
      if (!feed) {
        return reply.status(404).send({ error: 'Feed not found' });
      }
      return { feed };
    });

    this.fastify.delete('/v1/feeds/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const deleted = await this.database.deleteRSSFeed(request.params.id);
      if (!deleted) {
        return reply.status(404).send({ error: 'Feed not found' });
      }
      return { deleted: true };
    });

    // -----------------------------------------------------------------------
    // RSS Polling & Matching
    // -----------------------------------------------------------------------

    this.fastify.post('/api/rss/poll', {
      schema: { body: rssPollBodySchema },
    }, async (request: FastifyRequest<{ Body: { url: string; criteria: MatchCriteria[]; lastSeen?: string } }>, reply: FastifyReply) => {
      const { url, criteria, lastSeen } = request.body;

      try {
        const lastSeenDate = lastSeen ? new Date(lastSeen) : undefined;
        const matches = await this.rssMonitor.pollFeed(url, criteria, lastSeenDate);

        return {
          url,
          itemCount: matches.length,
          matches,
          polledAt: new Date().toISOString(),
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        return reply.status(500).send({ error: `Failed to poll RSS feed: ${message}` });
      }
    });

    this.fastify.post('/api/rss/test', {
      schema: { body: rssTestBodySchema },
    }, async (request: FastifyRequest<{ Body: { url: string } }>, reply: FastifyReply) => {
      const { url } = request.body;

      try {
        const items = await this.rssMonitor.fetchFeed(url);

        // Return sample of items
        const sample = items.slice(0, 10).map(item => ({
          title: item.title,
          pubDate: item.pubDate,
        }));

        return {
          url,
          valid: true,
          itemCount: items.length,
          sample,
          testedAt: new Date().toISOString(),
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        return reply.status(422).send({
          url,
          valid: false,
          error: `Failed to parse RSS feed: ${message}`,
        });
      }
    });

    // -----------------------------------------------------------------------
    // Acquisition Queue
    // -----------------------------------------------------------------------

    this.fastify.get('/v1/queue', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const queue = await this.database.getQueue(sourceAccountId);
      return { queue };
    });

    this.fastify.post('/v1/queue', {
      schema: { body: queueBodySchema },
    }, async (request: FastifyRequest<{ Body: { contentType: string; contentName: string; year?: number; season?: number; episode?: number } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { contentType, contentName, year, season, episode } = request.body;
      const item = await this.database.addToQueue({
        source_account_id: sourceAccountId,
        content_type: contentType as ContentType,
        content_name: contentName,
        year,
        season,
        episode,
        requested_by: 'api',
      });
      return { item };
    });

    // -----------------------------------------------------------------------
    // Calendar
    // -----------------------------------------------------------------------

    this.fastify.get('/v1/calendar', async () => {
      return { calendar: [] }; // Simplified implementation
    });

    // -----------------------------------------------------------------------
    // Quality Profiles
    // -----------------------------------------------------------------------

    this.fastify.post('/v1/profiles', {
      schema: { body: profileBodySchema },
    }, async (request: FastifyRequest<{ Body: { name: string; preferredQualities?: string[]; minSeeders?: number } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { name, preferredQualities, minSeeders } = request.body;
      const profile = await this.database.createQualityProfile({
        source_account_id: sourceAccountId,
        name,
        preferred_qualities: preferredQualities || ['1080p', '720p'],
        min_seeders: minSeeders || 1,
      });
      return { profile };
    });

    this.fastify.get('/v1/profiles/presets', async () => {
      return { presets: listQualityPresets() };
    });

    // -----------------------------------------------------------------------
    // Movies (monitoring)
    // -----------------------------------------------------------------------

    this.fastify.post('/v1/movies', {
      schema: { body: movieBodySchema },
    }, async (request: FastifyRequest<{ Body: { title: string; tmdbId?: number; qualityProfile?: string; autoDownload?: boolean; autoUpgrade?: boolean } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { title, tmdbId, qualityProfile, autoDownload, autoUpgrade } = request.body;
      const movie = await this.database.createMovieMonitoring({
        source_account_id: sourceAccountId,
        user_id: sourceAccountId,
        movie_title: title,
        tmdb_id: tmdbId,
        quality_profile: qualityProfile ?? 'balanced',
        auto_download: autoDownload,
        auto_upgrade: autoUpgrade,
      });
      return reply.status(201).send({ movie });
    });

    this.fastify.get('/v1/movies', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const movies = await this.database.listMovieMonitoring(sourceAccountId);
      return { movies };
    });

    this.fastify.put('/v1/movies/:id', {
      schema: { body: movieUpdateBodySchema },
    }, async (request: FastifyRequest<{ Params: { id: string }; Body: { title?: string; tmdbId?: number; qualityProfile?: string; autoDownload?: boolean; autoUpgrade?: boolean; status?: string } }>, reply: FastifyReply) => {
      const { title, tmdbId, qualityProfile, autoDownload, autoUpgrade, status } = request.body;
      const movie = await this.database.updateMovieMonitoring(request.params.id, {
        movie_title: title,
        tmdb_id: tmdbId,
        quality_profile: qualityProfile,
        auto_download: autoDownload,
        auto_upgrade: autoUpgrade,
        status: status as DownloadStatus | undefined,
      });
      if (!movie) {
        return reply.status(404).send({ error: 'Movie not found' });
      }
      return { movie };
    });

    this.fastify.delete('/v1/movies/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const deleted = await this.database.deleteMovieMonitoring(request.params.id);
      if (!deleted) {
        return reply.status(404).send({ error: 'Movie not found' });
      }
      return { deleted: true };
    });

    // -----------------------------------------------------------------------
    // Downloads (state-machine driven)
    // -----------------------------------------------------------------------

    this.fastify.post('/v1/downloads', {
      schema: { body: downloadBodySchema },
    }, async (request: FastifyRequest<{ Body: { contentType: string; title: string; magnetUri?: string; qualityProfile?: string; showId?: string; seasonNumber?: number; episodeNumber?: number; tmdbId?: number } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { contentType, title, magnetUri, qualityProfile, showId, seasonNumber, episodeNumber, tmdbId } = request.body;
      const download = await this.database.createDownload({
        source_account_id: sourceAccountId,
        user_id: sourceAccountId,
        content_type: contentType,
        title,
        magnet_uri: magnetUri,
        quality_profile: qualityProfile ?? 'balanced',
        show_id: showId,
        season_number: seasonNumber,
        episode_number: episodeNumber,
        tmdb_id: tmdbId,
      });
      // Add to download queue
      await this.database.addToDownloadQueue(download.id);
      return reply.status(201).send({ download });
    });

    this.fastify.get('/v1/downloads', async (request: FastifyRequest<{ Querystring: { status?: string } }>) => {
      const { sourceAccountId } = getAppContext(request);
      const { status } = request.query;
      const downloads = await this.database.listDownloads(sourceAccountId, status);
      return { downloads };
    });

    this.fastify.get('/v1/downloads/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const download = await this.database.getDownload(request.params.id);
      if (!download) {
        return reply.status(404).send({ error: 'Download not found' });
      }
      return { download };
    });

    this.fastify.get('/v1/downloads/:id/history', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const download = await this.database.getDownload(request.params.id);
      if (!download) {
        return reply.status(404).send({ error: 'Download not found' });
      }
      const history = await this.stateMachine.getHistory(request.params.id);
      return { download_id: request.params.id, history };
    });

    this.fastify.delete('/v1/downloads/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const download = await this.database.getDownload(request.params.id);
      if (!download) {
        return reply.status(404).send({ error: 'Download not found' });
      }
      // Transition to cancelled if in an active state
      const terminalStates: DownloadState[] = ['completed', 'failed', 'cancelled'];
      if (!terminalStates.includes(download.state)) {
        try {
          await this.stateMachine.transition(request.params.id, 'cancelled', { reason: 'user_cancelled' });
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          logger.warn(`Could not transition download ${request.params.id} to cancelled: ${message}`);
        }
      }
      await this.database.removeFromDownloadQueue(request.params.id);
      return { cancelled: true, download_id: request.params.id };
    });

    this.fastify.patch('/v1/downloads/:id/pause', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const download = await this.database.getDownload(request.params.id);
      if (!download) {
        return reply.status(404).send({ error: 'Download not found' });
      }
      try {
        await this.stateMachine.transition(request.params.id, 'paused', { reason: 'user_paused' });
        const updated = await this.database.getDownload(request.params.id);
        return { download: updated };
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return reply.status(400).send({ error: message });
      }
    });

    this.fastify.patch('/v1/downloads/:id/resume', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const download = await this.database.getDownload(request.params.id);
      if (!download) {
        return reply.status(404).send({ error: 'Download not found' });
      }
      if (download.state !== 'paused') {
        return reply.status(400).send({ error: 'Download is not paused' });
      }
      // Resume to the most sensible active state based on history
      const history = await this.stateMachine.getHistory(request.params.id);
      // Find the state before paused
      let resumeState: DownloadState = 'downloading';
      for (let i = history.length - 1; i >= 0; i--) {
        if (history[i].to_state === 'paused' && history[i].from_state) {
          resumeState = history[i].from_state as DownloadState;
          break;
        }
      }
      try {
        await this.stateMachine.transition(request.params.id, resumeState, { reason: 'user_resumed' });
        const updated = await this.database.getDownload(request.params.id);
        return { download: updated };
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return reply.status(400).send({ error: message });
      }
    });

    this.fastify.post('/v1/downloads/:id/retry', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const download = await this.database.getDownload(request.params.id);
      if (!download) {
        return reply.status(404).send({ error: 'Download not found' });
      }
      if (download.state !== 'failed') {
        return reply.status(400).send({ error: 'Only failed downloads can be retried' });
      }
      try {
        await this.stateMachine.transition(request.params.id, 'created', { reason: 'user_retry' });
        await this.database.updateDownloadFields(request.params.id, {
          retry_count: download.retry_count + 1,
          error_message: undefined,
        });
        // Re-add to download queue
        await this.database.addToDownloadQueue(request.params.id);
        const updated = await this.database.getDownload(request.params.id);
        return { download: updated };
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        return reply.status(400).send({ error: message });
      }
    });

    // -----------------------------------------------------------------------
    // Download Rules
    // -----------------------------------------------------------------------

    this.fastify.post('/v1/rules', {
      schema: { body: ruleBodySchema },
    }, async (request: FastifyRequest<{ Body: { name: string; conditions: Record<string, unknown>; action: string; priority?: number; enabled?: boolean } }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { name, conditions, action, priority, enabled } = request.body;
      const rule = await this.database.createDownloadRule({
        source_account_id: sourceAccountId,
        user_id: sourceAccountId,
        name,
        conditions,
        action: action as DownloadAction,
        priority,
        enabled,
      });
      return reply.status(201).send({ rule });
    });

    this.fastify.get('/v1/rules', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const rules = await this.database.listDownloadRules(sourceAccountId);
      return { rules };
    });

    this.fastify.put('/v1/rules/:id', {
      schema: { body: ruleUpdateBodySchema },
    }, async (request: FastifyRequest<{ Params: { id: string }; Body: { name?: string; conditions?: Record<string, unknown>; action?: string; priority?: number; enabled?: boolean } }>, reply: FastifyReply) => {
      const { name, conditions, action, priority, enabled } = request.body;
      const rule = await this.database.updateDownloadRule(request.params.id, {
        name,
        conditions,
        action: action as DownloadAction | undefined,
        priority,
        enabled,
      });
      if (!rule) {
        return reply.status(404).send({ error: 'Rule not found' });
      }
      return { rule };
    });

    this.fastify.delete('/v1/rules/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const deleted = await this.database.deleteDownloadRule(request.params.id);
      if (!deleted) {
        return reply.status(404).send({ error: 'Rule not found' });
      }
      return { deleted: true };
    });

    this.fastify.post('/v1/rules/:id/test', {
      schema: { body: ruleTestBodySchema },
    }, async (request: FastifyRequest<{ Params: { id: string }; Body: { sample: Record<string, unknown> } }>, reply: FastifyReply) => {
      const rule = await this.database.getDownloadRule(request.params.id);
      if (!rule) {
        return reply.status(404).send({ error: 'Rule not found' });
      }
      // Evaluate conditions against sample data
      const { sample } = request.body;
      const conditions = rule.conditions as Record<string, unknown>;
      let matches = true;
      const results: Array<{ field: string; expected: unknown; actual: unknown; match: boolean }> = [];

      for (const [field, expected] of Object.entries(conditions)) {
        const actual = sample[field];
        let match = false;

        if (typeof expected === 'string' && typeof actual === 'string') {
          match = actual.toLowerCase().includes(expected.toLowerCase());
        } else if (typeof expected === 'number' && typeof actual === 'number') {
          match = actual >= expected;
        } else if (typeof expected === 'boolean') {
          match = actual === expected;
        } else {
          match = actual === expected;
        }

        results.push({ field, expected, actual, match });
        if (!match) matches = false;
      }

      return {
        rule_id: rule.id,
        rule_name: rule.name,
        action: rule.action,
        matches,
        results,
      };
    });

    // -----------------------------------------------------------------------
    // History
    // -----------------------------------------------------------------------

    this.fastify.get('/v1/history', async (request: FastifyRequest<{ Querystring: { days?: string } }>) => {
      const { sourceAccountId } = getAppContext(request);
      const days = request.query.days ? parseInt(request.query.days, 10) : 90;
      const history = await this.database.listAcquisitionHistory(sourceAccountId, days);
      return { history };
    });

    // -----------------------------------------------------------------------
    // Dashboard
    // -----------------------------------------------------------------------

    this.fastify.get('/v1/dashboard', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const summary = await this.database.getDashboardSummary(sourceAccountId);
      return { summary };
    });

    // -----------------------------------------------------------------------
    // Pipeline
    // -----------------------------------------------------------------------

    this.fastify.get('/api/pipeline', async (request: FastifyRequest<{ Querystring: { status?: string; limit?: string; offset?: string } }>) => {
      const { status, limit, offset } = request.query;
      const result = await this.database.listPipelineRuns({
        status,
        limit: limit ? parseInt(limit, 10) : undefined,
        offset: offset ? parseInt(offset, 10) : undefined,
      });
      return { runs: result.runs, total: result.total };
    });

    this.fastify.get('/api/pipeline/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const id = parseInt(request.params.id, 10);
      if (isNaN(id)) {
        return reply.status(400).send({ error: 'Invalid pipeline ID' });
      }
      const run = await this.database.getPipelineRun(id);
      if (!run) {
        return reply.status(404).send({ error: 'Pipeline run not found' });
      }
      return { run };
    });

    this.fastify.post('/api/pipeline/trigger', {
      schema: { body: pipelineTriggerBodySchema },
    }, async (request: FastifyRequest<{ Body: PipelineTriggerRequest }>, reply: FastifyReply) => {
      const { sourceAccountId } = getAppContext(request);
      const { content_title, content_type, magnet_url, torrent_url } = request.body;

      const run = await this.database.createPipelineRun({
        source_account_id: sourceAccountId,
        trigger_type: 'manual',
        trigger_source: 'api',
        content_title,
        content_type,
        metadata: { magnet_url, torrent_url },
      });

      // Fire-and-forget: start pipeline execution asynchronously
      this.pipeline.executePipeline(run.id).catch((err: unknown) => {
        const message = err instanceof Error ? err.message : String(err);
        logger.error(`Background pipeline execution failed for run ${run.id}: ${message}`);
      });

      return reply.status(202).send({ run, message: 'Pipeline triggered' });
    });

    this.fastify.post('/api/pipeline/retry/:id', async (request: FastifyRequest<{ Params: { id: string } }>, reply: FastifyReply) => {
      const id = parseInt(request.params.id, 10);
      if (isNaN(id)) {
        return reply.status(400).send({ error: 'Invalid pipeline ID' });
      }
      const run = await this.database.getPipelineRun(id);
      if (!run) {
        return reply.status(404).send({ error: 'Pipeline run not found' });
      }
      if (run.status === 'completed') {
        return reply.status(400).send({ error: 'Pipeline already completed' });
      }

      // Fire-and-forget: retry pipeline execution asynchronously
      this.pipeline.retryPipeline(id).catch((err: unknown) => {
        const message = err instanceof Error ? err.message : String(err);
        logger.error(`Background pipeline retry failed for run ${id}: ${message}`);
      });

      return reply.status(202).send({ message: 'Pipeline retry triggered', pipelineId: id });
    });
  }

  async start(): Promise<void> {
    try {
      await this.fastify.listen({ port: this.config.port, host: '0.0.0.0' });
      logger.info(`Server listening on port ${this.config.port}`);
    } catch (error) {
      logger.error('Failed to start server', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async stop(): Promise<void> {
    await this.fastify.close();
    logger.info('Server stopped');
  }
}
