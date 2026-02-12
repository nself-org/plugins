/**
 * Content Acquisition API Server
 */

import Fastify, { FastifyRequest, FastifyReply } from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext, loadSecurityConfig } from '@nself/plugin-utils';
import { ContentAcquisitionDatabase } from './database.js';
import { RSSFeedMonitor } from './rss-monitor.js';
import { PipelineOrchestrator } from './pipeline.js';
import type { ContentAcquisitionConfig, PipelineTriggerRequest } from './types.js';

const logger = createLogger('content-acquisition:server');

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

// ============================================================================
// Server
// ============================================================================

export class ContentAcquisitionServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: ContentAcquisitionDatabase;
  private config: ContentAcquisitionConfig;
  private pipeline: PipelineOrchestrator;

  constructor(config: ContentAcquisitionConfig, database: ContentAcquisitionDatabase) {
    this.config = config;
    this.database = database;
    this.fastify = Fastify({ logger: false });
    this.pipeline = new PipelineOrchestrator(database, config);
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
        subscription_type: contentType as any,
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
        feed_type: feedType as any,
      });
      return { feed };
    });

    this.fastify.get('/v1/feeds', async (request: FastifyRequest) => {
      const { sourceAccountId } = getAppContext(request);
      const feeds = await this.database.listRSSFeeds(sourceAccountId);
      return { feeds };
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
        content_type: contentType as any,
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
