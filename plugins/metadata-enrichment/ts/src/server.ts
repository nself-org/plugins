import Fastify from 'fastify';
import cors from '@fastify/cors';
import rateLimit from '@fastify/rate-limit';
import { createLogger } from '@nself/plugin-utils';
import { MetadataEnrichmentDatabase } from './database.js';
import { TMDBClient } from './tmdb-client.js';
import type { MetadataEnrichmentConfig } from './types.js';

const logger = createLogger('metadata-enrichment:server');

export class MetadataEnrichmentServer {
  private fastify: ReturnType<typeof Fastify>;
  private database: MetadataEnrichmentDatabase;
  private tmdb: TMDBClient;
  private config: MetadataEnrichmentConfig;

  constructor(config: MetadataEnrichmentConfig, database: MetadataEnrichmentDatabase) {
    this.config = config;
    this.database = database;
    this.tmdb = new TMDBClient(config.tmdb_api_key);
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

    this.fastify.get('/v1/movies/search', async (request) => {
      const { q, year } = request.query as any;
      const results = await this.tmdb.searchMovies(q, year);
      return { results };
    });

    this.fastify.get('/v1/movies/:id', async (request) => {
      const { id } = request.params as any;
      const movie = await this.tmdb.getMovieDetails(parseInt(id));
      return { movie };
    });

    this.fastify.get('/v1/tv/search', async (request) => {
      const { q, year } = request.query as any;
      const results = await this.tmdb.searchTV(q, year);
      return { results };
    });

    this.fastify.get('/v1/tv/:id', async (request) => {
      const { id } = request.params as any;
      const show = await this.tmdb.getTVShowDetails(parseInt(id));
      return { show };
    });

    this.fastify.get('/v1/tv/:id/episodes/upcoming', async (request) => {
      return { episodes: [] }; // Simplified
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
