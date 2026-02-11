import Fastify from 'fastify';
import cors from '@fastify/cors';
import rateLimit from '@fastify/rate-limit';
import { createLogger } from '@nself/plugin-utils';
import { SubtitleManagerDatabase } from './database.js';
import { OpenSubtitlesClient } from './opensubtitles-client.js';
import type { SubtitleManagerConfig } from './types.js';

const logger = createLogger('subtitle-manager:server');

export class SubtitleManagerServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: SubtitleManagerDatabase;
  private opensubtitles: OpenSubtitlesClient;
  private config: SubtitleManagerConfig;

  constructor(config: SubtitleManagerConfig, database: SubtitleManagerDatabase) {
    this.config = config;
    this.database = database;
    this.opensubtitles = new OpenSubtitlesClient(config.opensubtitles_api_key);
    this.fastify = Fastify({ logger: false });
  }

  async initialize(): Promise<void> {
    await this.fastify.register(cors);
    await this.fastify.register(rateLimit, { max: 100, timeWindow: '1 minute' });
    this.registerRoutes();
    logger.info('Server initialized');
  }

  private registerRoutes(): void {
    this.fastify.get('/health', async () => ({ status: 'ok' }));

    this.fastify.get('/v1/subtitles', async (request) => {
      const { media_id, language } = request.query as any;
      const subtitles = await this.database.searchSubtitles(media_id, language || 'en');
      return { subtitles };
    });

    this.fastify.post('/v1/search', async (request, reply) => {
      const { query, languages } = request.body as any;
      const results = await this.opensubtitles.searchByQuery(query, languages);
      return { results };
    });

    this.fastify.post('/v1/download', async (request, reply) => {
      const { file_id } = request.body as any;
      const subtitle = await this.opensubtitles.downloadSubtitle(file_id);
      if (!subtitle) {
        reply.code(404).send({ error: 'Subtitle not found' });
        return;
      }
      return { success: true };
    });
  }

  async start(): Promise<void> {
    await this.fastify.listen({ port: this.config.port, host: '0.0.0.0' });
    logger.info(`Server listening on port ${this.config.port}`);
  }

  async stop(): Promise<void> {
    await this.fastify.close();
  }
}
