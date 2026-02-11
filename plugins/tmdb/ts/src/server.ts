#!/usr/bin/env node
/**
 * TMDB Plugin HTTP Server
 * REST API endpoints for TMDB metadata enrichment
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import {
  createLogger,
  createDatabase,
  ApiRateLimiter,
  createRateLimitHook,
  createAuthHook,
  getAppContext,
} from '@nself/plugin-utils';
import { config } from './config.js';
import { TmdbDatabase } from './database.js';
import type {
  HealthCheckResponse,
  ReadyCheckResponse,
  LiveCheckResponse,
  MatchMediaRequest,
  BatchMatchRequest,
  ConfirmMatchRequest,
  TmdbSearchResult,
  TmdbSearchResponse,
  MatchMediaResponse,
  BatchMatchResponse,
  RefreshMetadataResponse,
  TmdbImagesResponse,
  TmdbImage,
  TmdbConfiguration,
  TmdbApiMovieResult,
  TmdbApiTvResult,
  TmdbApiSearchResponse,
  TmdbApiMovieDetails,
  TmdbApiTvDetails,
  TmdbApiSeasonDetails,
  TmdbApiImagesResponse,
  TmdbApiConfiguration,
  TmdbApiGenre,
} from './types.js';

const logger = createLogger('tmdb:server');
const PLUGIN_VERSION = '1.0.0';
const TMDB_API_BASE = 'https://api.themoviedb.org/3';

const fastify = Fastify({ logger: false, bodyLimit: 10485760 });

let tmdbDb: TmdbDatabase;

// Rate limiter for external TMDB API calls
const tmdbApiLimiter = new ApiRateLimiter(config.rateLimitRequests, config.rateLimitWindowMs);

// ============================================================================
// TMDB API Helper
// ============================================================================

async function tmdbFetch<T>(endpoint: string, params: Record<string, string> = {}): Promise<T> {
  const url = new URL(`${TMDB_API_BASE}${endpoint}`);
  url.searchParams.set('api_key', config.tmdbApiKey);
  for (const [key, value] of Object.entries(params)) {
    url.searchParams.set(key, value);
  }

  if (!tmdbApiLimiter.check('tmdb-api')) {
    // Wait a bit and retry
    await new Promise(resolve => setTimeout(resolve, 1000));
  }

  const response = await fetch(url.toString());
  if (!response.ok) {
    throw new Error(`TMDB API error: ${response.status} ${response.statusText}`);
  }
  return response.json() as Promise<T>;
}

// ============================================================================
// Filename Parsing
// ============================================================================

function parseFilename(filename: string): { title: string; year?: number; type?: 'movie' | 'tv'; season?: number; episode?: number } {
  let cleaned = filename.replace(/\.[^.]+$/, '');
  cleaned = cleaned.replace(/[._]/g, ' ');

  // Detect TV patterns: S01E05, 1x05
  const tvMatch = cleaned.match(/[Ss](\d{1,2})[Ee](\d{1,2})/);
  const tvMatch2 = cleaned.match(/(\d{1,2})x(\d{1,2})/);

  if (tvMatch) {
    const beforeTv = cleaned.substring(0, cleaned.indexOf(tvMatch[0])).trim();
    const yearMatch = beforeTv.match(/\b(19|20)\d{2}\b/);
    return {
      title: beforeTv.replace(/\b(19|20)\d{2}\b/, '').replace(/\s+/g, ' ').trim(),
      year: yearMatch ? parseInt(yearMatch[0], 10) : undefined,
      type: 'tv',
      season: parseInt(tvMatch[1], 10),
      episode: parseInt(tvMatch[2], 10),
    };
  }

  if (tvMatch2) {
    const beforeTv = cleaned.substring(0, cleaned.indexOf(tvMatch2[0])).trim();
    return {
      title: beforeTv.replace(/\b(19|20)\d{2}\b/, '').replace(/\s+/g, ' ').trim(),
      type: 'tv',
      season: parseInt(tvMatch2[1], 10),
      episode: parseInt(tvMatch2[2], 10),
    };
  }

  // Movie pattern: remove resolution, codec, etc.
  const yearMatch = cleaned.match(/\b(19|20)\d{2}\b/);
  const year = yearMatch ? parseInt(yearMatch[0], 10) : undefined;

  // Remove everything after year/resolution info
  let title = cleaned
    .replace(/\b(19|20)\d{2}\b.*/, '')
    .replace(/\b(720p|1080p|2160p|4K|HDR|BluRay|BRRip|WEB-DL|WEBRip|HDTV|DVDRip|x264|x265|HEVC|AAC|DTS|FLAC|Atmos)\b.*/i, '')
    .replace(/\s+/g, ' ')
    .trim();

  if (!title && yearMatch) {
    title = cleaned.substring(0, cleaned.indexOf(yearMatch[0])).trim();
  }

  return { title: title || cleaned.trim(), year, type: 'movie' };
}

function computeConfidence(query: string, result: TmdbApiMovieResult | TmdbApiTvResult, year?: number): number {
  const resultTitle = 'title' in result ? result.title : result.name;
  const normalizedQuery = query.toLowerCase().trim();
  const normalizedResult = resultTitle.toLowerCase().trim();

  let score = 0;

  // Exact title match
  if (normalizedQuery === normalizedResult) {
    score += 0.6;
  } else if (normalizedResult.includes(normalizedQuery) || normalizedQuery.includes(normalizedResult)) {
    score += 0.4;
  } else {
    score += 0.1;
  }

  // Year match
  if (year) {
    const resultDate = 'release_date' in result ? result.release_date : (result as TmdbApiTvResult).first_air_date;
    if (resultDate) {
      const resultYear = parseInt(resultDate.substring(0, 4), 10);
      if (resultYear === year) {
        score += 0.3;
      } else if (Math.abs(resultYear - year) <= 1) {
        score += 0.15;
      }
    }
  } else {
    score += 0.05;
  }

  // Popularity boost
  if (result.popularity > 100) {
    score += 0.1;
  } else if (result.popularity > 10) {
    score += 0.05;
  }

  return Math.min(score, 1.0);
}

function extractContentRating(releaseDates: TmdbApiMovieDetails['release_dates']): string | null {
  if (!releaseDates?.results) return null;
  const us = releaseDates.results.find(r => r.iso_3166_1 === 'US');
  if (!us) return null;
  // Type 3 = Theatrical
  const theatrical = us.release_dates.find(rd => rd.type === 3);
  return theatrical?.certification || us.release_dates[0]?.certification || null;
}

function extractTvContentRating(contentRatings: TmdbApiTvDetails['content_ratings']): string | null {
  if (!contentRatings?.results) return null;
  const us = contentRatings.results.find(r => r.iso_3166_1 === 'US');
  return us?.rating || null;
}

// ============================================================================
// Middleware Setup
// ============================================================================

async function setupMiddleware(): Promise<void> {
  await fastify.register(cors, { origin: true });

  const rateLimiter = new ApiRateLimiter(
    config.security.rateLimitMax ?? 100,
    config.security.rateLimitWindowMs ?? 60000
  );
  fastify.addHook('preHandler', createRateLimitHook(rateLimiter));
  fastify.addHook('preHandler', createAuthHook(config.security.apiKey));
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return { status: 'ok', plugin: 'tmdb', timestamp: new Date().toISOString(), version: PLUGIN_VERSION };
});

fastify.get('/ready', async (): Promise<ReadyCheckResponse> => {
  let dbStatus: 'ok' | 'error' = 'ok';
  let tmdbApiStatus: 'ok' | 'error' | 'unconfigured' = 'ok';
  try {
    await tmdbDb.getStats();
  } catch {
    dbStatus = 'error';
  }
  if (!config.tmdbApiKey) {
    tmdbApiStatus = 'unconfigured';
  }
  return { ready: dbStatus === 'ok', database: dbStatus, tmdbApi: tmdbApiStatus, timestamp: new Date().toISOString() };
});

fastify.get('/live', async (): Promise<LiveCheckResponse> => {
  const stats = await tmdbDb.getStats();
  return {
    alive: true,
    uptime: process.uptime(),
    memory: { used: process.memoryUsage().heapUsed, total: process.memoryUsage().heapTotal },
    stats,
  };
});

// ============================================================================
// Search Endpoints
// ============================================================================

fastify.get<{ Querystring: { query: string; year?: string; language?: string } }>('/api/search/movie', async (request) => {
  const { query, year, language } = request.query;

  const params: Record<string, string> = { query, language: language || config.defaultLanguage };
  if (year) params.year = year;

  const data = await tmdbFetch<TmdbApiSearchResponse<TmdbApiMovieResult>>('/search/movie', params);

  const results: TmdbSearchResult[] = data.results.map(r => ({
    id: r.id,
    title: r.title,
    overview: r.overview,
    releaseDate: r.release_date,
    posterPath: r.poster_path ? `${config.imageBaseUrl}${config.posterSize}${r.poster_path}` : undefined,
    voteAverage: r.vote_average,
    matchScore: computeConfidence(query, r, year ? parseInt(year, 10) : undefined),
  }));

  results.sort((a, b) => (b.matchScore ?? 0) - (a.matchScore ?? 0));

  const response: TmdbSearchResponse = { results, total: data.total_results };
  return response;
});

fastify.get<{ Querystring: { query: string; year?: string; language?: string } }>('/api/search/tv', async (request) => {
  const { query, year, language } = request.query;

  const params: Record<string, string> = { query, language: language || config.defaultLanguage };
  if (year) params.first_air_date_year = year;

  const data = await tmdbFetch<TmdbApiSearchResponse<TmdbApiTvResult>>('/search/tv', params);

  const results: TmdbSearchResult[] = data.results.map(r => ({
    id: r.id,
    name: r.name,
    overview: r.overview,
    firstAirDate: r.first_air_date,
    posterPath: r.poster_path ? `${config.imageBaseUrl}${config.posterSize}${r.poster_path}` : undefined,
    voteAverage: r.vote_average,
    matchScore: computeConfidence(query, r, year ? parseInt(year, 10) : undefined),
  }));

  results.sort((a, b) => (b.matchScore ?? 0) - (a.matchScore ?? 0));

  return { results, total: data.total_results };
});

fastify.get<{ Querystring: { query: string } }>('/api/search/multi', async (request) => {
  const { query } = request.query;

  const [movieData, tvData] = await Promise.all([
    tmdbFetch<TmdbApiSearchResponse<TmdbApiMovieResult>>('/search/movie', { query, language: config.defaultLanguage }),
    tmdbFetch<TmdbApiSearchResponse<TmdbApiTvResult>>('/search/tv', { query, language: config.defaultLanguage }),
  ]);

  const movieResults: TmdbSearchResult[] = movieData.results.map(r => ({
    id: r.id,
    title: r.title,
    overview: r.overview,
    releaseDate: r.release_date,
    posterPath: r.poster_path ? `${config.imageBaseUrl}${config.posterSize}${r.poster_path}` : undefined,
    voteAverage: r.vote_average,
  }));

  const tvResults: TmdbSearchResult[] = tvData.results.map(r => ({
    id: r.id,
    name: r.name,
    overview: r.overview,
    firstAirDate: r.first_air_date,
    posterPath: r.poster_path ? `${config.imageBaseUrl}${config.posterSize}${r.poster_path}` : undefined,
    voteAverage: r.vote_average,
  }));

  return { movies: movieResults, tvShows: tvResults, total: movieData.total_results + tvData.total_results };
});

// ============================================================================
// Metadata Retrieval Endpoints
// ============================================================================

fastify.get<{ Params: { id: string } }>('/api/movie/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const id = parseInt(request.params.id, 10);

  // Try cache first
  let cached = await scopedDb.getMovie(id);
  if (!cached) {
    // Fetch from TMDB
    const details = await tmdbFetch<TmdbApiMovieDetails>(`/movie/${id}`, {
      language: config.defaultLanguage,
      append_to_response: 'credits,keywords,release_dates',
    });

    const contentRating = extractContentRating(details.release_dates);

    await scopedDb.upsertMovie({
      id: details.id,
      source_account_id: sourceAccountId,
      imdb_id: details.imdb_id,
      title: details.title,
      original_title: details.original_title,
      overview: details.overview,
      tagline: details.tagline,
      release_date: details.release_date || null,
      runtime: details.runtime,
      status: details.status,
      poster_path: details.poster_path,
      backdrop_path: details.backdrop_path,
      budget: details.budget,
      revenue: details.revenue,
      vote_average: details.vote_average,
      vote_count: details.vote_count,
      popularity: details.popularity,
      original_language: details.original_language,
      genres: details.genres,
      production_companies: details.production_companies,
      production_countries: details.production_countries,
      spoken_languages: details.spoken_languages,
      credits: details.credits || {},
      keywords: details.keywords?.keywords || [],
      content_rating: contentRating,
    });

    cached = await scopedDb.getMovie(id);
  }

  if (!cached) {
    reply.code(404);
    throw new Error('Movie not found');
  }

  return cached;
});

fastify.get<{ Params: { id: string } }>('/api/tv/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const id = parseInt(request.params.id, 10);

  let cached = await scopedDb.getTvShow(id);
  if (!cached) {
    const details = await tmdbFetch<TmdbApiTvDetails>(`/tv/${id}`, {
      language: config.defaultLanguage,
      append_to_response: 'credits,content_ratings',
    });

    const contentRating = extractTvContentRating(details.content_ratings);

    await scopedDb.upsertTvShow({
      id: details.id,
      source_account_id: sourceAccountId,
      imdb_id: null,
      name: details.name,
      original_name: details.original_name,
      overview: details.overview,
      first_air_date: details.first_air_date || null,
      last_air_date: details.last_air_date || null,
      status: details.status,
      type: details.type,
      number_of_seasons: details.number_of_seasons,
      number_of_episodes: details.number_of_episodes,
      episode_run_time: details.episode_run_time,
      poster_path: details.poster_path,
      backdrop_path: details.backdrop_path,
      vote_average: details.vote_average,
      vote_count: details.vote_count,
      popularity: details.popularity,
      original_language: details.original_language,
      genres: details.genres,
      networks: details.networks,
      created_by: details.created_by,
      credits: details.credits || {},
      content_rating: contentRating,
    });

    cached = await scopedDb.getTvShow(id);
  }

  if (!cached) {
    reply.code(404);
    throw new Error('TV show not found');
  }

  return cached;
});

fastify.get<{ Params: { id: string; num: string } }>('/api/tv/:id/season/:num', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const showId = parseInt(request.params.id, 10);
  const seasonNum = parseInt(request.params.num, 10);

  const seasonDetails = await tmdbFetch<TmdbApiSeasonDetails>(`/tv/${showId}/season/${seasonNum}`, {
    language: config.defaultLanguage,
  });

  await scopedDb.upsertSeason({
    source_account_id: sourceAccountId,
    show_id: showId,
    season_number: seasonDetails.season_number,
    name: seasonDetails.name,
    overview: seasonDetails.overview,
    poster_path: seasonDetails.poster_path,
    air_date: seasonDetails.air_date || null,
    episode_count: seasonDetails.episodes.length,
  });

  for (const ep of seasonDetails.episodes) {
    await scopedDb.upsertEpisode({
      source_account_id: sourceAccountId,
      show_id: showId,
      season_number: seasonNum,
      episode_number: ep.episode_number,
      name: ep.name,
      overview: ep.overview,
      still_path: ep.still_path,
      air_date: ep.air_date || null,
      runtime: ep.runtime,
      vote_average: ep.vote_average,
      crew: ep.crew || [],
      guest_stars: ep.guest_stars || [],
    });
  }

  const seasons = await scopedDb.getSeasons(showId);
  const season = seasons.find(s => s.season_number === seasonNum);
  const episodes = await scopedDb.getEpisodes(showId, seasonNum);

  return { season, episodes };
});

fastify.get<{ Params: { id: string; seasonNum: string; episodeNum: string } }>('/api/tv/:id/season/:seasonNum/episode/:episodeNum', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const showId = parseInt(request.params.id, 10);
  const seasonNum = parseInt(request.params.seasonNum, 10);
  const episodeNum = parseInt(request.params.episodeNum, 10);

  const episode = await scopedDb.getEpisode(showId, seasonNum, episodeNum);
  if (!episode) {
    reply.code(404);
    throw new Error('Episode not found');
  }

  return episode;
});

// ============================================================================
// Matching Endpoints
// ============================================================================

fastify.post<{ Body: MatchMediaRequest }>('/api/match', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const body = request.body;

  let title = body.title;
  let year = body.year;
  let type = body.type;

  if (!title && body.filename && config.filenameParsing) {
    const parsed = parseFilename(body.filename);
    title = parsed.title;
    year = year ?? parsed.year;
    type = type ?? parsed.type;
  }

  if (!title) {
    throw new Error('Either title or filename is required for matching');
  }

  // Search TMDB
  const searchType = type || 'movie';
  const params: Record<string, string> = { query: title, language: config.defaultLanguage };
  if (year) params.year = String(year);

  let searchResults: Array<{ id: number; title: string; confidence: number }> = [];

  if (searchType === 'movie' || !type) {
    const movieData = await tmdbFetch<TmdbApiSearchResponse<TmdbApiMovieResult>>('/search/movie', params);
    searchResults.push(...movieData.results.slice(0, 5).map(r => ({
      id: r.id,
      title: r.title,
      confidence: computeConfidence(title!, r, year),
    })));
  }

  if (searchType === 'tv' || !type) {
    const tvParams = { ...params };
    if (year) tvParams.first_air_date_year = String(year);
    delete tvParams.year;
    const tvData = await tmdbFetch<TmdbApiSearchResponse<TmdbApiTvResult>>('/search/tv', tvParams);
    searchResults.push(...tvData.results.slice(0, 5).map(r => ({
      id: r.id,
      title: r.name,
      confidence: computeConfidence(title!, r, year),
    })));
  }

  searchResults.sort((a, b) => b.confidence - a.confidence);

  const bestMatch = searchResults[0];
  const autoAccepted = bestMatch ? bestMatch.confidence >= config.autoAcceptThreshold : false;

  const entry = await scopedDb.createMatchEntry({
    media_id: body.mediaId,
    filename: body.filename,
    parsed_title: title,
    parsed_year: year,
    parsed_type: type,
    match_results: searchResults,
    best_match_id: bestMatch?.id,
    best_match_type: type || 'movie',
    confidence: bestMatch?.confidence,
    status: autoAccepted ? 'accepted' : 'pending',
    auto_accepted: autoAccepted,
  });

  const response: MatchMediaResponse = {
    matchQueueId: entry.id,
    bestMatch: bestMatch ? {
      id: bestMatch.id,
      title: bestMatch.title,
      confidence: bestMatch.confidence,
      autoAccepted,
    } : undefined,
    alternatives: searchResults.slice(1).map(r => ({
      id: r.id,
      title: r.title,
      confidence: r.confidence,
    })),
  };

  // Emit webhook event
  const eventType = autoAccepted ? 'tmdb.match.auto_accepted' : 'tmdb.match.needs_review';
  await scopedDb.insertWebhookEvent(
    `${eventType}-${entry.id}`,
    eventType,
    { matchQueueId: entry.id, mediaId: body.mediaId, bestMatch: response.bestMatch }
  );

  return response;
});

fastify.post<{ Body: BatchMatchRequest }>('/api/match/batch', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const { items } = request.body;

  let autoAccepted = 0;
  let needsReview = 0;

  for (const item of items) {
    try {
      let title = item.title;
      let year = item.year;
      let type = item.type;

      if (!title && item.filename && config.filenameParsing) {
        const parsed = parseFilename(item.filename);
        title = parsed.title;
        year = year ?? parsed.year;
        type = type ?? parsed.type;
      }

      if (!title) continue;

      const searchType = type || 'movie';
      const params: Record<string, string> = { query: title, language: config.defaultLanguage };
      if (year) params.year = String(year);

      let results: Array<{ id: number; title: string; confidence: number }> = [];

      if (searchType === 'movie') {
        const data = await tmdbFetch<TmdbApiSearchResponse<TmdbApiMovieResult>>('/search/movie', params);
        results = data.results.slice(0, 3).map(r => ({
          id: r.id, title: r.title, confidence: computeConfidence(title!, r, year),
        }));
      } else {
        const tvParams = { ...params };
        if (year) tvParams.first_air_date_year = String(year);
        delete tvParams.year;
        const data = await tmdbFetch<TmdbApiSearchResponse<TmdbApiTvResult>>('/search/tv', tvParams);
        results = data.results.slice(0, 3).map(r => ({
          id: r.id, title: r.name, confidence: computeConfidence(title!, r, year),
        }));
      }

      results.sort((a, b) => b.confidence - a.confidence);
      const best = results[0];
      const accepted = best ? best.confidence >= config.autoAcceptThreshold : false;

      await scopedDb.createMatchEntry({
        media_id: item.mediaId,
        filename: item.filename,
        parsed_title: title,
        parsed_year: year,
        parsed_type: type,
        match_results: results,
        best_match_id: best?.id,
        best_match_type: type || 'movie',
        confidence: best?.confidence,
        status: accepted ? 'accepted' : 'pending',
        auto_accepted: accepted,
      });

      if (accepted) autoAccepted++;
      else needsReview++;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.warn('Failed to match item', { mediaId: item.mediaId, error: message });
      needsReview++;
    }
  }

  const response: BatchMatchResponse = { processed: items.length, autoAccepted, needsReview };
  return response;
});

// ============================================================================
// Match Queue Endpoints
// ============================================================================

fastify.get<{ Querystring: { status?: string; limit?: string; offset?: string } }>('/api/match/queue', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const { status, limit, offset } = request.query;

  return scopedDb.listMatchQueue(
    status,
    limit ? parseInt(limit, 10) : 50,
    offset ? parseInt(offset, 10) : 0
  );
});

fastify.put<{ Params: { id: string }; Body: ConfirmMatchRequest }>('/api/match/:id/confirm', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);

  const updated = await scopedDb.updateMatchStatus(
    request.params.id, 'accepted', 'operator',
    request.body.tmdbId, request.body.tmdbType
  );

  if (!updated) {
    reply.code(404);
    throw new Error('Match entry not found');
  }

  await scopedDb.insertWebhookEvent(
    `tmdb.match.confirmed-${request.params.id}`,
    'tmdb.match.confirmed',
    { matchQueueId: request.params.id, tmdbId: request.body.tmdbId, tmdbType: request.body.tmdbType }
  );

  return updated;
});

fastify.put<{ Params: { id: string } }>('/api/match/:id/reject', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);

  const updated = await scopedDb.updateMatchStatus(request.params.id, 'rejected', 'operator');
  if (!updated) {
    reply.code(404);
    throw new Error('Match entry not found');
  }

  await scopedDb.insertWebhookEvent(
    `tmdb.match.rejected-${request.params.id}`,
    'tmdb.match.rejected',
    { matchQueueId: request.params.id }
  );

  return updated;
});

fastify.put<{ Params: { id: string }; Body: ConfirmMatchRequest }>('/api/match/:id/manual', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);

  const updated = await scopedDb.updateMatchStatus(
    request.params.id, 'manual', 'operator',
    request.body.tmdbId, request.body.tmdbType
  );

  if (!updated) {
    reply.code(404);
    throw new Error('Match entry not found');
  }

  return updated;
});

// ============================================================================
// Refresh Endpoints
// ============================================================================

fastify.post<{ Params: { type: string; id: string } }>('/api/refresh/:type/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const { type, id } = request.params;
  const tmdbId = parseInt(id, 10);

  const changed: string[] = [];

  if (type === 'movie') {
    const old = await scopedDb.getMovie(tmdbId);
    const details = await tmdbFetch<TmdbApiMovieDetails>(`/movie/${tmdbId}`, {
      language: config.defaultLanguage,
      append_to_response: 'credits,keywords,release_dates',
    });

    if (old) {
      if (old.overview !== details.overview) changed.push('overview');
      if (old.vote_average !== details.vote_average) changed.push('voteAverage');
      if (old.vote_count !== details.vote_count) changed.push('voteCount');
      if (old.popularity !== details.popularity) changed.push('popularity');
      if (old.poster_path !== details.poster_path) changed.push('posterPath');
      if (old.status !== details.status) changed.push('status');
    }

    const contentRating = extractContentRating(details.release_dates);

    await scopedDb.upsertMovie({
      id: details.id, source_account_id: sourceAccountId, imdb_id: details.imdb_id,
      title: details.title, original_title: details.original_title, overview: details.overview,
      tagline: details.tagline, release_date: details.release_date || null, runtime: details.runtime,
      status: details.status, poster_path: details.poster_path, backdrop_path: details.backdrop_path,
      budget: details.budget, revenue: details.revenue, vote_average: details.vote_average,
      vote_count: details.vote_count, popularity: details.popularity,
      original_language: details.original_language, genres: details.genres,
      production_companies: details.production_companies,
      production_countries: details.production_countries,
      spoken_languages: details.spoken_languages, credits: details.credits || {},
      keywords: details.keywords?.keywords || [], content_rating: contentRating,
    });
  } else if (type === 'tv') {
    const old = await scopedDb.getTvShow(tmdbId);
    const details = await tmdbFetch<TmdbApiTvDetails>(`/tv/${tmdbId}`, {
      language: config.defaultLanguage,
      append_to_response: 'credits,content_ratings',
    });

    if (old) {
      if (old.overview !== details.overview) changed.push('overview');
      if (old.vote_average !== details.vote_average) changed.push('voteAverage');
      if (old.number_of_seasons !== details.number_of_seasons) changed.push('numberOfSeasons');
      if (old.status !== details.status) changed.push('status');
    }

    const contentRating = extractTvContentRating(details.content_ratings);

    await scopedDb.upsertTvShow({
      id: details.id, source_account_id: sourceAccountId, imdb_id: null,
      name: details.name, original_name: details.original_name, overview: details.overview,
      first_air_date: details.first_air_date || null, last_air_date: details.last_air_date || null,
      status: details.status, type: details.type, number_of_seasons: details.number_of_seasons,
      number_of_episodes: details.number_of_episodes, episode_run_time: details.episode_run_time,
      poster_path: details.poster_path, backdrop_path: details.backdrop_path,
      vote_average: details.vote_average, vote_count: details.vote_count,
      popularity: details.popularity, original_language: details.original_language,
      genres: details.genres, networks: details.networks, created_by: details.created_by,
      credits: details.credits || {}, content_rating: contentRating,
    });
  } else {
    reply.code(400);
    throw new Error('Invalid type. Use "movie" or "tv".');
  }

  if (changed.length > 0) {
    await scopedDb.insertWebhookEvent(
      `tmdb.metadata.changed-${type}-${id}-${Date.now()}`,
      'tmdb.metadata.changed',
      { type, id: tmdbId, changed }
    );
  }

  const response: RefreshMetadataResponse = { refreshed: true, changed };
  return response;
});

fastify.post<{ Body: { olderThanDays?: number } }>('/api/refresh/all', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  const days = request.body.olderThanDays ?? config.cacheTtlDays;

  const movies = await scopedDb.getMoviesOlderThan(days);
  const tvShows = await scopedDb.getTvShowsOlderThan(days);

  return {
    moviesQueued: movies.length,
    tvShowsQueued: tvShows.length,
    message: `Found ${movies.length} movies and ${tvShows.length} TV shows older than ${days} days for refresh`,
  };
});

// ============================================================================
// Images Endpoint
// ============================================================================

fastify.get<{ Params: { type: string; id: string } }>('/api/images/:type/:id', async (request, reply) => {
  const { type, id } = request.params;
  const tmdbId = parseInt(id, 10);

  if (type !== 'movie' && type !== 'tv') {
    reply.code(400);
    throw new Error('Invalid type. Use "movie" or "tv".');
  }

  const data = await tmdbFetch<TmdbApiImagesResponse>(`/${type}/${tmdbId}/images`);

  const mapImage = (img: TmdbApiImagesResponse['posters'][0]): TmdbImage => ({
    path: `${config.imageBaseUrl}original${img.file_path}`,
    width: img.width,
    height: img.height,
    language: img.iso_639_1 || undefined,
  });

  const response: TmdbImagesResponse = {
    posters: data.posters.map(mapImage),
    backdrops: data.backdrops.map(mapImage),
    logos: data.logos.map(mapImage),
  };

  return response;
});

// ============================================================================
// Configuration Endpoint
// ============================================================================

fastify.get('/api/config', async () => {
  const data = await tmdbFetch<TmdbApiConfiguration>('/configuration');

  const response: TmdbConfiguration = {
    imageBaseUrl: data.images.secure_base_url,
    posterSizes: data.images.poster_sizes,
    backdropSizes: data.images.backdrop_sizes,
  };

  return response;
});

// ============================================================================
// Sync & Status Endpoints
// ============================================================================

fastify.post('/api/sync', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);

  // Sync genres
  const [movieGenres, tvGenres] = await Promise.all([
    tmdbFetch<{ genres: TmdbApiGenre[] }>('/genre/movie/list', { language: config.defaultLanguage }),
    tmdbFetch<{ genres: TmdbApiGenre[] }>('/genre/tv/list', { language: config.defaultLanguage }),
  ]);

  for (const genre of movieGenres.genres) {
    await scopedDb.upsertGenre({ id: genre.id, source_account_id: sourceAccountId, name: genre.name, media_type: 'movie' });
  }
  for (const genre of tvGenres.genres) {
    await scopedDb.upsertGenre({ id: genre.id, source_account_id: sourceAccountId, name: genre.name, media_type: 'tv' });
  }

  return {
    synced: true,
    movieGenres: movieGenres.genres.length,
    tvGenres: tvGenres.genres.length,
  };
});

fastify.get('/api/status', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tmdbDb.forSourceAccount(sourceAccountId);
  return scopedDb.getStatus();
});

// ============================================================================
// Server Startup
// ============================================================================

async function start(): Promise<void> {
  try {
    await setupMiddleware();

    const db = createDatabase(config.database);
    await db.connect();
    tmdbDb = new TmdbDatabase(db);

    logger.info('TMDB database connection established');

    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`TMDB plugin server listening on ${config.host}:${config.port}`);
    logger.info(`Health check: http://${config.host}:${config.port}/health`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start TMDB server', { error: message });
    process.exit(1);
  }
}

process.on('SIGINT', async () => {
  logger.info('Received SIGINT, shutting down...');
  await fastify.close();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  logger.info('Received SIGTERM, shutting down...');
  await fastify.close();
  process.exit(0);
});

const isMainModule = process.argv[1] && (
  process.argv[1].endsWith('server.ts') ||
  process.argv[1].endsWith('server.js')
);

if (isMainModule) {
  start();
}

export { fastify, start };
