import Fastify, { type FastifyRequest } from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext, loadSecurityConfig } from '@nself/plugin-utils';
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

    // Security middleware from shared utilities
    const securityConfig = loadSecurityConfig('METADATA_ENRICHMENT');
    const rateLimiter = new ApiRateLimiter(
      this.config.rate_limit_max ?? securityConfig.rateLimitMax ?? 100,
      this.config.rate_limit_window_ms ?? securityConfig.rateLimitWindowMs ?? 60000
    );

    this.fastify.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

    const apiKey = this.config.api_key ?? securityConfig.apiKey;
    if (apiKey) {
      this.fastify.addHook('preHandler', createAuthHook(apiKey) as never);
      logger.info('API key authentication enabled');
    }

    this.registerRoutes();
    logger.info('Server initialized');
  }

  /**
   * Check whether a cached record is stale and should be re-fetched.
   * @param updatedAt - The updated_at timestamp from the cached record
   * @param maxAgeHours - Maximum age in hours before the record is considered stale (default 24)
   */
  private isStale(updatedAt: Date, maxAgeHours = 24): boolean {
    return Date.now() - new Date(updatedAt).getTime() > maxAgeHours * 60 * 60 * 1000;
  }

  private registerRoutes(): void {
    this.fastify.get('/health', async () => ({
      status: 'ok',
      plugin: 'metadata-enrichment',
      timestamp: new Date().toISOString(),
    }));

    // -----------------------------------------------------------------------
    // Movie search -- always hits TMDB (search results are not cached by ID)
    // -----------------------------------------------------------------------
    this.fastify.get('/v1/movies/search', async (request: FastifyRequest) => {
      const { q, year } = request.query as { q?: string; year?: string };
      if (!q) {
        return { results: [], error: 'q query parameter is required' };
      }
      const yearNum = year ? parseInt(year, 10) : undefined;
      const results = await this.tmdb.searchMovies(q, yearNum);
      return { results };
    });

    // -----------------------------------------------------------------------
    // Movie detail -- cache-first with staleness check
    // -----------------------------------------------------------------------
    this.fastify.get('/v1/movies/:id', async (request: FastifyRequest) => {
      const { id } = request.params as { id: string };
      const tmdbId = parseInt(id, 10);
      if (isNaN(tmdbId)) {
        return { movie: null, error: 'Invalid movie ID' };
      }

      const ctx = getAppContext(request);

      // Check cache first
      const cached = await this.database.getMovieByTmdbId(tmdbId, ctx.sourceAccountId);
      if (cached && !this.isStale(cached.updated_at)) {
        return { movie: cached, source: 'cache' };
      }

      // Fetch from TMDB
      const tmdbResult = await this.tmdb.getMovieDetails(tmdbId);
      if (tmdbResult) {
        const genres = Array.isArray(tmdbResult.genres)
          ? tmdbResult.genres.map((g: { name?: string }) => g.name).filter((name): name is string => Boolean(name))
          : undefined;

        const movie = await this.database.upsertMovie(
          {
            tmdb_id: tmdbId,
            imdb_id: tmdbResult.imdb_id ?? undefined,
            title: tmdbResult.title ?? `Unknown (${tmdbId})`,
            original_title: tmdbResult.original_title ?? undefined,
            overview: tmdbResult.overview ?? undefined,
            release_date: tmdbResult.release_date ? new Date(tmdbResult.release_date) : undefined,
            runtime: tmdbResult.runtime ?? undefined,
            genres,
            vote_average: tmdbResult.vote_average ?? undefined,
            vote_count: tmdbResult.vote_count ?? undefined,
            poster_path: tmdbResult.poster_path ?? undefined,
            backdrop_path: tmdbResult.backdrop_path ?? undefined,
            raw_response: tmdbResult as unknown as Record<string, unknown>,
          },
          ctx.sourceAccountId
        );
        return { movie, source: 'tmdb' };
      }

      return { movie: cached ?? null, source: cached ? 'cache-stale' : 'not-found' };
    });

    // -----------------------------------------------------------------------
    // TV search -- always hits TMDB
    // -----------------------------------------------------------------------
    this.fastify.get('/v1/tv/search', async (request: FastifyRequest) => {
      const { q, year } = request.query as { q?: string; year?: string };
      if (!q) {
        return { results: [], error: 'q query parameter is required' };
      }
      const yearNum = year ? parseInt(year, 10) : undefined;
      const results = await this.tmdb.searchTV(q, yearNum);
      return { results };
    });

    // -----------------------------------------------------------------------
    // TV show detail -- cache-first with staleness check
    // -----------------------------------------------------------------------
    this.fastify.get('/v1/tv/:id', async (request: FastifyRequest) => {
      const { id } = request.params as { id: string };
      const tmdbId = parseInt(id, 10);
      if (isNaN(tmdbId)) {
        return { show: null, error: 'Invalid TV show ID' };
      }

      const ctx = getAppContext(request);

      // Check cache first
      const cached = await this.database.getTVShowByTmdbId(tmdbId, ctx.sourceAccountId);
      if (cached && !this.isStale(cached.updated_at)) {
        return { show: cached, source: 'cache' };
      }

      // Fetch from TMDB
      const tmdbResult = await this.tmdb.getTVShowDetails(tmdbId);
      if (tmdbResult) {
        const genres = Array.isArray(tmdbResult.genres)
          ? tmdbResult.genres.map((g: { name?: string }) => g.name).filter((name): name is string => Boolean(name))
          : undefined;

        const show = await this.database.upsertTVShow(
          {
            tmdb_id: tmdbId,
            imdb_id: tmdbResult.external_ids?.imdb_id ?? undefined,
            tvdb_id: tmdbResult.external_ids?.tvdb_id ?? undefined,
            name: tmdbResult.name ?? `Unknown (${tmdbId})`,
            original_name: tmdbResult.original_name ?? undefined,
            overview: tmdbResult.overview ?? undefined,
            first_air_date: tmdbResult.first_air_date ? new Date(tmdbResult.first_air_date) : undefined,
            last_air_date: tmdbResult.last_air_date ? new Date(tmdbResult.last_air_date) : undefined,
            number_of_seasons: tmdbResult.number_of_seasons ?? 0,
            number_of_episodes: tmdbResult.number_of_episodes ?? 0,
            genres,
            vote_average: tmdbResult.vote_average ?? undefined,
            vote_count: tmdbResult.vote_count ?? undefined,
            poster_path: tmdbResult.poster_path ?? undefined,
            backdrop_path: tmdbResult.backdrop_path ?? undefined,
            raw_response: tmdbResult as unknown as Record<string, unknown>,
          },
          ctx.sourceAccountId
        );
        return { show, source: 'tmdb' };
      }

      return { show: cached ?? null, source: cached ? 'cache-stale' : 'not-found' };
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
