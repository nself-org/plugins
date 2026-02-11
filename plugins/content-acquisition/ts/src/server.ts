/**
 * Content Acquisition API Server
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import rateLimit from '@fastify/rate-limit';
import { createLogger } from '@nself/plugin-utils';
import { ContentAcquisitionDatabase } from './database.js';
import type { ContentAcquisitionConfig } from './types.js';

const logger = createLogger('content-acquisition:server');

export class ContentAcquisitionServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: ContentAcquisitionDatabase;
  private config: ContentAcquisitionConfig;

  constructor(config: ContentAcquisitionConfig, database: ContentAcquisitionDatabase) {
    this.config = config;
    this.database = database;
    this.fastify = Fastify({ logger: false });
  }

  async initialize(): Promise<void> {
    await this.fastify.register(cors);
    await this.fastify.register(rateLimit, {
      max: 100,
      timeWindow: '1 minute',
    });

    this.registerRoutes();
    logger.info('Server initialized');
  }

  private registerRoutes(): void {
    // Health check
    this.fastify.get('/health', async () => {
      return { status: 'ok', timestamp: new Date().toISOString() };
    });

    // Subscriptions
    this.fastify.post('/v1/subscriptions', async (request, reply) => {
      const { contentType, contentId, contentName, qualityProfileId } = request.body as any;
      const sub = await this.database.createSubscription({
        source_account_id: 'primary',
        subscription_type: contentType,
        content_id: contentId,
        content_name: contentName,
        quality_profile_id: qualityProfileId,
      });
      return { subscription: sub };
    });

    this.fastify.get('/v1/subscriptions', async () => {
      const subs = await this.database.listSubscriptions('primary');
      return { subscriptions: subs };
    });

    // RSS Feeds
    this.fastify.post('/v1/feeds', async (request, reply) => {
      const { name, url, feedType } = request.body as any;
      const feed = await this.database.createRSSFeed({
        source_account_id: 'primary',
        name,
        url,
        feed_type: feedType,
      });
      return { feed };
    });

    this.fastify.get('/v1/feeds', async () => {
      const feeds = await this.database.listRSSFeeds('primary');
      return { feeds };
    });

    // Queue
    this.fastify.get('/v1/queue', async () => {
      const queue = await this.database.getQueue('primary');
      return { queue };
    });

    this.fastify.post('/v1/queue', async (request, reply) => {
      const { contentType, contentName, year, season, episode } = request.body as any;
      const item = await this.database.addToQueue({
        source_account_id: 'primary',
        content_type: contentType,
        content_name: contentName,
        year,
        season,
        episode,
        requested_by: 'api',
      });
      return { item };
    });

    // Calendar
    this.fastify.get('/v1/calendar', async () => {
      return { calendar: [] }; // Simplified implementation
    });

    // Quality Profiles
    this.fastify.post('/v1/profiles', async (request, reply) => {
      const { name, preferredQualities, minSeeders } = request.body as any;
      const profile = await this.database.createQualityProfile({
        source_account_id: 'primary',
        name,
        preferred_qualities: preferredQualities || ['1080p', '720p'],
        min_seeders: minSeeders || 1,
      });
      return { profile };
    });
  }

  async start(): Promise<void> {
    try {
      await this.fastify.listen({ port: this.config.port, host: '0.0.0.0' });
      logger.info(`Server listening on port ${this.config.port}`);
    } catch (error) {
      logger.error('Failed to start server', error);
      throw error;
    }
  }

  async stop(): Promise<void> {
    await this.fastify.close();
    logger.info('Server stopped');
  }
}
