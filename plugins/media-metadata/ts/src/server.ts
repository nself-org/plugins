/**
 * TMDB Plugin Server
 * HTTP server for metadata enrichment and API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { TmdbDatabase } from './database.js';
import { TmdbClient } from './client.js';
import { TmdbLookupService } from './lookup.js';
import { loadConfig, type Config } from './config.js';
import type { LookupRequest, EnrichRequest, SearchParams } from './types.js';

const logger = createLogger('tmdb:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new TmdbDatabase();
  await db.connect();
  await db.initializeSchema();

  const client = new TmdbClient(fullConfig.tmdbApiKey, fullConfig.tmdbDefaultLanguage);
  const lookupService = new TmdbLookupService(client, db, fullConfig.tmdbConfidenceThreshold);

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.rateLimitMax,
    fullConfig.rateLimitWindowMs
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): TmdbDatabase {
    return (request as Record<string, unknown>).scopedDb as TmdbDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'media-metadata', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'media-metadata', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'media-metadata',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'media-metadata',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status & Stats
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'media-metadata',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/stats', async (request) => {
    return scopedDb(request).getStats();
  });

  // =========================================================================
  // Search
  // =========================================================================

  app.get<{ Querystring: SearchParams }>('/v1/search', async (request) => {
    const { query, media_type, year, page } = request.query;

    if (!query) {
      return { error: 'query parameter is required' };
    }

    return client.search({ query, media_type, year, page });
  });

  // =========================================================================
  // Movies
  // =========================================================================

  app.get<{ Params: { tmdbId: string } }>('/v1/movies/:tmdbId', async (request, reply) => {
    const tmdbId = parseInt(request.params.tmdbId, 10);

    if (isNaN(tmdbId)) {
      return reply.status(400).send({ error: 'Invalid TMDB ID' });
    }

    // Check database first
    let movie = await scopedDb(request).getMovie(tmdbId);

    if (!movie) {
      // Fetch from TMDB
      try {
        const tmdbMovie = await client.getMovie(tmdbId);
        const record = {
          source_account_id: scopedDb(request).getCurrentSourceAccountId(),
          tmdb_id: tmdbMovie.id,
          imdb_id: tmdbMovie.imdb_id ?? null,
          title: tmdbMovie.title,
          original_title: tmdbMovie.original_title,
          overview: tmdbMovie.overview || null,
          release_date: tmdbMovie.release_date ? new Date(tmdbMovie.release_date) : null,
          runtime_minutes: tmdbMovie.runtime ?? null,
          vote_average: tmdbMovie.vote_average,
          vote_count: tmdbMovie.vote_count,
          popularity: tmdbMovie.popularity,
          status: tmdbMovie.status,
          tagline: tmdbMovie.tagline ?? null,
          budget: tmdbMovie.budget ?? null,
          revenue: tmdbMovie.revenue ?? null,
          genres: tmdbMovie.genres.map(g => g.name),
          spoken_languages: tmdbMovie.spoken_languages.map(l => l.english_name),
          production_countries: tmdbMovie.production_countries.map(c => c.name),
          poster_path: tmdbMovie.poster_path,
          backdrop_path: tmdbMovie.backdrop_path,
          cast: tmdbMovie.credits?.cast ?? [],
          crew: tmdbMovie.credits?.crew ?? [],
          content_rating: client.extractUsRating(tmdbMovie) ?? null,
          keywords: tmdbMovie.keywords?.keywords.map(k => k.name) ?? [],
          synced_at: new Date(),
        };

        await scopedDb(request).upsertMovie(record);
        movie = await scopedDb(request).getMovie(tmdbId);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to fetch movie', { tmdbId, error: message });
        return reply.status(404).send({ error: 'Movie not found' });
      }
    }

    return movie;
  });

  app.get('/v1/movies/trending', async () => {
    return client.getTrendingMovies('week');
  });

  app.get('/v1/movies/popular', async () => {
    return client.getPopularMovies();
  });

  // =========================================================================
  // TV Shows
  // =========================================================================

  app.get<{ Params: { tmdbId: string } }>('/v1/tv/:tmdbId', async (request, reply) => {
    const tmdbId = parseInt(request.params.tmdbId, 10);

    if (isNaN(tmdbId)) {
      return reply.status(400).send({ error: 'Invalid TMDB ID' });
    }

    // Check database first
    let show = await scopedDb(request).getTvShow(tmdbId);

    if (!show) {
      // Fetch from TMDB
      try {
        const tmdbShow = await client.getTvShow(tmdbId);
        const record = {
          source_account_id: scopedDb(request).getCurrentSourceAccountId(),
          tmdb_id: tmdbShow.id,
          imdb_id: tmdbShow.external_ids?.imdb_id ?? null,
          name: tmdbShow.name,
          original_name: tmdbShow.original_name,
          overview: tmdbShow.overview || null,
          first_air_date: tmdbShow.first_air_date ? new Date(tmdbShow.first_air_date) : null,
          last_air_date: tmdbShow.last_air_date ? new Date(tmdbShow.last_air_date) : null,
          status: tmdbShow.status,
          type: tmdbShow.type,
          number_of_seasons: tmdbShow.number_of_seasons,
          number_of_episodes: tmdbShow.number_of_episodes,
          episode_run_time: tmdbShow.episode_run_time,
          vote_average: tmdbShow.vote_average,
          vote_count: tmdbShow.vote_count,
          popularity: tmdbShow.popularity,
          genres: tmdbShow.genres.map(g => g.name),
          networks: tmdbShow.networks.map(n => n.name),
          created_by: tmdbShow.created_by.map(c => c.name),
          poster_path: tmdbShow.poster_path,
          backdrop_path: tmdbShow.backdrop_path,
          content_rating: client.extractTvRating(tmdbShow) ?? null,
          keywords: tmdbShow.keywords?.results.map(k => k.name) ?? [],
          synced_at: new Date(),
        };

        await scopedDb(request).upsertTvShow(record);
        show = await scopedDb(request).getTvShow(tmdbId);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to fetch TV show', { tmdbId, error: message });
        return reply.status(404).send({ error: 'TV show not found' });
      }
    }

    return show;
  });

  app.get('/v1/tv/trending', async () => {
    return client.getTrendingTvShows('week');
  });

  app.get('/v1/tv/popular', async () => {
    return client.getPopularTvShows();
  });

  // =========================================================================
  // Seasons & Episodes
  // =========================================================================

  app.get<{ Params: { tmdbId: string; seasonNum: string } }>(
    '/v1/tv/:tmdbId/season/:seasonNum',
    async (request, reply) => {
      const tmdbId = parseInt(request.params.tmdbId, 10);
      const seasonNum = parseInt(request.params.seasonNum, 10);

      if (isNaN(tmdbId) || isNaN(seasonNum)) {
        return reply.status(400).send({ error: 'Invalid parameters' });
      }

      try {
        const season = await client.getTvSeason(tmdbId, seasonNum);

        // Store season
        await scopedDb(request).upsertTvSeason({
          source_account_id: scopedDb(request).getCurrentSourceAccountId(),
          show_tmdb_id: tmdbId,
          season_number: seasonNum,
          tmdb_id: season.id,
          name: season.name,
          overview: season.overview || null,
          air_date: season.air_date ? new Date(season.air_date) : null,
          episode_count: season.episode_count,
          poster_path: season.poster_path,
          synced_at: new Date(),
        });

        // Store episodes
        if (season.episodes) {
          for (const ep of season.episodes) {
            await scopedDb(request).upsertTvEpisode({
              source_account_id: scopedDb(request).getCurrentSourceAccountId(),
              show_tmdb_id: tmdbId,
              season_number: seasonNum,
              episode_number: ep.episode_number,
              tmdb_id: ep.id,
              name: ep.name,
              overview: ep.overview || null,
              air_date: ep.air_date ? new Date(ep.air_date) : null,
              runtime_minutes: ep.runtime ?? null,
              vote_average: ep.vote_average,
              still_path: ep.still_path,
              guest_stars: ep.guest_stars,
              crew: ep.crew,
              synced_at: new Date(),
            });
          }
        }

        return season;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to fetch season', { tmdbId, seasonNum, error: message });
        return reply.status(404).send({ error: 'Season not found' });
      }
    }
  );

  app.get<{ Params: { tmdbId: string; seasonNum: string; episodeNum: string } }>(
    '/v1/tv/:tmdbId/season/:seasonNum/episode/:episodeNum',
    async (request, reply) => {
      const tmdbId = parseInt(request.params.tmdbId, 10);
      const seasonNum = parseInt(request.params.seasonNum, 10);
      const episodeNum = parseInt(request.params.episodeNum, 10);

      if (isNaN(tmdbId) || isNaN(seasonNum) || isNaN(episodeNum)) {
        return reply.status(400).send({ error: 'Invalid parameters' });
      }

      // Check database first
      let episode = await scopedDb(request).getTvEpisode(tmdbId, seasonNum, episodeNum);

      if (!episode) {
        try {
          const tmdbEpisode = await client.getTvEpisode(tmdbId, seasonNum, episodeNum);

          await scopedDb(request).upsertTvEpisode({
            source_account_id: scopedDb(request).getCurrentSourceAccountId(),
            show_tmdb_id: tmdbId,
            season_number: seasonNum,
            episode_number: episodeNum,
            tmdb_id: tmdbEpisode.id,
            name: tmdbEpisode.name,
            overview: tmdbEpisode.overview || null,
            air_date: tmdbEpisode.air_date ? new Date(tmdbEpisode.air_date) : null,
            runtime_minutes: tmdbEpisode.runtime ?? null,
            vote_average: tmdbEpisode.vote_average,
            still_path: tmdbEpisode.still_path,
            guest_stars: tmdbEpisode.guest_stars,
            crew: tmdbEpisode.crew,
            synced_at: new Date(),
          });

          episode = await scopedDb(request).getTvEpisode(tmdbId, seasonNum, episodeNum);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error('Failed to fetch episode', { tmdbId, seasonNum, episodeNum, error: message });
          return reply.status(404).send({ error: 'Episode not found' });
        }
      }

      return episode;
    }
  );

  // =========================================================================
  // Lookup & Enrichment
  // =========================================================================

  app.post<{ Body: LookupRequest }>('/v1/lookup', async (request) => {
    return lookupService.lookup(request.body);
  });

  app.post<{ Body: { items: LookupRequest[] } }>('/v1/lookup/batch', async (request) => {
    const startTime = Date.now();
    const results = await lookupService.batchLookup(request.body.items);
    return {
      results,
      duration: Date.now() - startTime,
    };
  });

  app.post<{ Body: EnrichRequest }>('/v1/enrich', async (request) => {
    return lookupService.enrich(request.body);
  });

  // =========================================================================
  // Genres
  // =========================================================================

  app.get<{ Querystring: { media_type?: 'movie' | 'tv' } }>('/v1/genres', async (request) => {
    return scopedDb(request).getGenres(request.query.media_type);
  });

  app.post('/v1/sync/genres', async (request) => {
    const [movieGenres, tvGenres] = await Promise.all([
      client.getMovieGenres(),
      client.getTvGenres(),
    ]);

    for (const genre of movieGenres.genres) {
      await scopedDb(request).upsertGenre({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        tmdb_id: genre.id,
        name: genre.name,
        media_type: 'movie',
      });
    }

    for (const genre of tvGenres.genres) {
      await scopedDb(request).upsertGenre({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        tmdb_id: genre.id,
        name: genre.name,
        media_type: 'tv',
      });
    }

    return {
      success: true,
      movieGenres: movieGenres.genres.length,
      tvGenres: tvGenres.genres.length,
    };
  });

  // =========================================================================
  // Match Queue
  // =========================================================================

  app.get<{ Querystring: { status?: string; limit?: number } }>('/v1/match-queue', async (request) => {
    const { status, limit } = request.query;
    return scopedDb(request).getMatchQueue(status, limit);
  });

  app.post<{ Params: { id: string }; Body: { tmdb_id: number; reviewed_by?: string } }>(
    '/v1/match-queue/:id/match',
    async (request, reply) => {
      const { id } = request.params;
      const { tmdb_id, reviewed_by } = request.body;

      await scopedDb(request).updateMatchQueueItem(id, {
        status: 'matched',
        matched_tmdb_id: tmdb_id,
        reviewed_by: reviewed_by ?? 'manual',
      });

      return reply.status(200).send({ success: true });
    }
  );

  app.post<{ Params: { id: string }; Body: { reviewed_by?: string } }>(
    '/v1/match-queue/:id/reject',
    async (request, reply) => {
      const { id } = request.params;
      const { reviewed_by } = request.body;

      await scopedDb(request).updateMatchQueueItem(id, {
        status: 'no_match',
        reviewed_by: reviewed_by ?? 'manual',
      });

      return reply.status(200).send({ success: true });
    }
  );

  // =========================================================================
  // Force Sync Endpoints
  // =========================================================================

  app.post<{ Params: { tmdbId: string } }>('/v1/sync/movie/:tmdbId', async (request, reply) => {
    const tmdbId = parseInt(request.params.tmdbId, 10);

    if (isNaN(tmdbId)) {
      return reply.status(400).send({ error: 'Invalid TMDB ID' });
    }

    try {
      const movie = await client.getMovie(tmdbId);
      await scopedDb(request).upsertMovie({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        tmdb_id: movie.id,
        imdb_id: movie.imdb_id ?? null,
        title: movie.title,
        original_title: movie.original_title,
        overview: movie.overview || null,
        release_date: movie.release_date ? new Date(movie.release_date) : null,
        runtime_minutes: movie.runtime ?? null,
        vote_average: movie.vote_average,
        vote_count: movie.vote_count,
        popularity: movie.popularity,
        status: movie.status,
        tagline: movie.tagline ?? null,
        budget: movie.budget ?? null,
        revenue: movie.revenue ?? null,
        genres: movie.genres.map(g => g.name),
        spoken_languages: movie.spoken_languages.map(l => l.english_name),
        production_countries: movie.production_countries.map(c => c.name),
        poster_path: movie.poster_path,
        backdrop_path: movie.backdrop_path,
        cast: movie.credits?.cast ?? [],
        crew: movie.credits?.crew ?? [],
        content_rating: client.extractUsRating(movie) ?? null,
        keywords: movie.keywords?.keywords.map(k => k.name) ?? [],
        synced_at: new Date(),
      });

      return { success: true, tmdb_id: tmdbId };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync movie', { tmdbId, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post<{ Params: { tmdbId: string } }>('/v1/sync/tv/:tmdbId', async (request, reply) => {
    const tmdbId = parseInt(request.params.tmdbId, 10);

    if (isNaN(tmdbId)) {
      return reply.status(400).send({ error: 'Invalid TMDB ID' });
    }

    try {
      const show = await client.getTvShow(tmdbId);
      await scopedDb(request).upsertTvShow({
        source_account_id: scopedDb(request).getCurrentSourceAccountId(),
        tmdb_id: show.id,
        imdb_id: show.external_ids?.imdb_id ?? null,
        name: show.name,
        original_name: show.original_name,
        overview: show.overview || null,
        first_air_date: show.first_air_date ? new Date(show.first_air_date) : null,
        last_air_date: show.last_air_date ? new Date(show.last_air_date) : null,
        status: show.status,
        type: show.type,
        number_of_seasons: show.number_of_seasons,
        number_of_episodes: show.number_of_episodes,
        episode_run_time: show.episode_run_time,
        vote_average: show.vote_average,
        vote_count: show.vote_count,
        popularity: show.popularity,
        genres: show.genres.map(g => g.name),
        networks: show.networks.map(n => n.name),
        created_by: show.created_by.map(c => c.name),
        poster_path: show.poster_path,
        backdrop_path: show.backdrop_path,
        content_rating: client.extractTvRating(show) ?? null,
        keywords: show.keywords?.results.map(k => k.name) ?? [],
        synced_at: new Date(),
      });

      return { success: true, tmdb_id: tmdbId };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync TV show', { tmdbId, error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // Start server
  await app.listen({ port: fullConfig.port, host: fullConfig.host });
  logger.info(`TMDB plugin server started on ${fullConfig.host}:${fullConfig.port}`);

  return app;
}
