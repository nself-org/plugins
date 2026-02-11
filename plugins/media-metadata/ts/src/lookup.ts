/**
 * TMDB Lookup Service
 * Intelligent matching with confidence scoring and fuzzy matching
 */

import { createLogger } from '@nself/plugin-utils';
import { TmdbClient } from './client.js';
import { TmdbDatabase } from './database.js';
import type {
  LookupRequest,
  LookupResult,
  MatchCandidate,
  EnrichRequest,
  EnrichResult,
  TmdbMovieRecord,
  TmdbTvShowRecord,
} from './types.js';

const logger = createLogger('tmdb:lookup');

export class TmdbLookupService {
  constructor(
    private client: TmdbClient,
    private db: TmdbDatabase,
    private confidenceThreshold: number
  ) {}

  /**
   * Lookup media by title and year with confidence scoring
   */
  async lookup(request: LookupRequest): Promise<LookupResult> {
    logger.debug('Looking up media', request);

    // Search TMDB
    const searchResult = await this.client.search({
      query: request.title,
      media_type: request.media_type,
      year: request.year,
      page: 1,
    });

    if (searchResult.results.length === 0) {
      return {
        matched: false,
        confidence: 0,
        tmdb_id: null,
        media_type: null,
        title: null,
        year: null,
        candidates: [],
      };
    }

    // Score candidates
    const candidates = searchResult.results
      .slice(0, 10)
      .map(item => this.scoreCandidate(request, item))
      .sort((a, b) => b.confidence - a.confidence);

    const bestMatch = candidates[0];

    // Determine if match is good enough
    const matched = bestMatch.confidence >= this.confidenceThreshold;
    const needsReview = !matched && bestMatch.confidence >= 0.5;

    if (needsReview) {
      // Add to match queue for manual review
      await this.db.addToMatchQueue({
        source_account_id: this.db.getCurrentSourceAccountId(),
        title: request.title,
        year: request.year ?? null,
        media_type: request.media_type ?? 'movie',
        source_id: null,
        source_plugin: null,
        candidates: candidates,
        status: 'manual_review',
        matched_tmdb_id: null,
        confidence: bestMatch.confidence,
      });
    }

    return {
      matched,
      confidence: bestMatch.confidence,
      tmdb_id: matched ? bestMatch.tmdb_id : null,
      media_type: matched ? bestMatch.media_type : null,
      title: matched ? bestMatch.title : null,
      year: matched ? bestMatch.year : null,
      candidates,
    };
  }

  /**
   * Batch lookup multiple titles
   */
  async batchLookup(requests: LookupRequest[]): Promise<LookupResult[]> {
    const results: LookupResult[] = [];

    for (const request of requests) {
      try {
        const result = await this.lookup(request);
        results.push(result);
      } catch (error) {
        logger.error('Batch lookup item failed', { request, error });
        results.push({
          matched: false,
          confidence: 0,
          tmdb_id: null,
          media_type: null,
          title: null,
          year: null,
          candidates: [],
        });
      }
    }

    return results;
  }

  /**
   * Enrich a media item: lookup + fetch metadata + store
   */
  async enrich(request: EnrichRequest): Promise<EnrichResult> {
    logger.info('Enriching media', request);

    // First check if we already have it cached
    if (!request.force) {
      const cached = await this.checkCache(request);
      if (cached) {
        return {
          success: true,
          tmdb_id: cached.tmdb_id,
          media_type: request.media_type,
          cached: true,
          metadata: cached.metadata,
        };
      }
    }

    // Lookup to find TMDB ID
    const lookupResult = await this.lookup({
      title: request.title,
      year: request.year,
      media_type: request.media_type,
    });

    if (!lookupResult.matched || !lookupResult.tmdb_id) {
      return {
        success: false,
        tmdb_id: null,
        media_type: null,
        cached: false,
        metadata: null,
      };
    }

    // Fetch and store metadata
    try {
      if (request.media_type === 'movie') {
        const movie = await this.client.getMovie(lookupResult.tmdb_id);
        const record = this.mapMovieToRecord(movie);
        await this.db.upsertMovie(record);

        const storedMovie = await this.db.getMovie(lookupResult.tmdb_id);
        return {
          success: true,
          tmdb_id: lookupResult.tmdb_id,
          media_type: 'movie',
          cached: false,
          metadata: storedMovie,
        };
      } else {
        const show = await this.client.getTvShow(lookupResult.tmdb_id);
        const record = this.mapTvShowToRecord(show);
        await this.db.upsertTvShow(record);

        const storedShow = await this.db.getTvShow(lookupResult.tmdb_id);
        return {
          success: true,
          tmdb_id: lookupResult.tmdb_id,
          media_type: 'tv',
          cached: false,
          metadata: storedShow,
        };
      }
    } catch (error) {
      logger.error('Failed to fetch metadata', { tmdb_id: lookupResult.tmdb_id, error });
      return {
        success: false,
        tmdb_id: lookupResult.tmdb_id,
        media_type: request.media_type,
        cached: false,
        metadata: null,
      };
    }
  }

  /**
   * Score a search result candidate against the request
   */
  private scoreCandidate(request: LookupRequest, item: {
    id: number;
    title?: string;
    name?: string;
    release_date?: string;
    first_air_date?: string;
    overview: string;
    poster_path: string | null;
  }): MatchCandidate {
    const itemTitle = item.title ?? item.name ?? '';
    const itemYear = this.client.extractYear(item.release_date ?? item.first_air_date);
    const mediaType: 'movie' | 'tv' = item.title ? 'movie' : 'tv';

    let confidence = 0;

    // Title matching (0-0.7 points)
    const titleScore = this.calculateTitleSimilarity(request.title, itemTitle);
    confidence += titleScore * 0.7;

    // Year matching (0-0.25 points)
    if (request.year && itemYear) {
      const yearDiff = Math.abs(request.year - itemYear);
      if (yearDiff === 0) {
        confidence += 0.25;
      } else if (yearDiff === 1) {
        confidence += 0.15;
      } else if (yearDiff <= 2) {
        confidence += 0.05;
      }
    } else if (!request.year) {
      // No year provided, small bonus
      confidence += 0.05;
    }

    // Media type matching (0-0.05 points)
    if (request.media_type === mediaType) {
      confidence += 0.05;
    }

    return {
      tmdb_id: item.id,
      title: itemTitle,
      year: itemYear,
      media_type: mediaType,
      confidence: Math.min(confidence, 1.0),
      overview: item.overview,
      poster_path: item.poster_path,
    };
  }

  /**
   * Calculate title similarity using multiple techniques
   */
  private calculateTitleSimilarity(query: string, target: string): number {
    const q = this.normalizeTitle(query);
    const t = this.normalizeTitle(target);

    // Exact match
    if (q === t) return 1.0;

    // Check if one contains the other
    if (q.includes(t) || t.includes(q)) return 0.9;

    // Levenshtein distance
    const distance = this.levenshteinDistance(q, t);
    const maxLength = Math.max(q.length, t.length);
    const similarity = 1 - distance / maxLength;

    return Math.max(0, similarity);
  }

  /**
   * Normalize title for comparison
   */
  private normalizeTitle(title: string): string {
    return title
      .toLowerCase()
      .replace(/[^\w\s]/g, '')
      .replace(/\s+/g, ' ')
      .trim();
  }

  /**
   * Calculate Levenshtein distance between two strings
   */
  private levenshteinDistance(a: string, b: string): number {
    const matrix: number[][] = [];

    for (let i = 0; i <= b.length; i++) {
      matrix[i] = [i];
    }

    for (let j = 0; j <= a.length; j++) {
      matrix[0][j] = j;
    }

    for (let i = 1; i <= b.length; i++) {
      for (let j = 1; j <= a.length; j++) {
        if (b.charAt(i - 1) === a.charAt(j - 1)) {
          matrix[i][j] = matrix[i - 1][j - 1];
        } else {
          matrix[i][j] = Math.min(
            matrix[i - 1][j - 1] + 1,
            matrix[i][j - 1] + 1,
            matrix[i - 1][j] + 1
          );
        }
      }
    }

    return matrix[b.length][a.length];
  }

  /**
   * Check if we have cached metadata
   */
  private async checkCache(request: EnrichRequest): Promise<{
    tmdb_id: number;
    metadata: TmdbMovieRecord | TmdbTvShowRecord;
  } | null> {
    if (request.media_type === 'movie') {
      const movies = await this.db.searchMoviesByTitle(request.title, 5);
      for (const movie of movies) {
        if (request.year && this.client.extractYear(movie.release_date?.toISOString()) === request.year) {
          return { tmdb_id: movie.tmdb_id, metadata: movie };
        }
      }
    } else {
      const shows = await this.db.searchTvShowsByName(request.title, 5);
      for (const show of shows) {
        if (request.year && this.client.extractYear(show.first_air_date?.toISOString()) === request.year) {
          return { tmdb_id: show.tmdb_id, metadata: show };
        }
      }
    }

    return null;
  }

  /**
   * Map TMDB movie to database record
   */
  private mapMovieToRecord(movie: {
    id: number;
    imdb_id?: string | null;
    title: string;
    original_title: string;
    overview: string;
    release_date: string;
    runtime?: number | null;
    vote_average: number;
    vote_count: number;
    popularity: number;
    status: string;
    tagline?: string | null;
    budget?: number;
    revenue?: number;
    genres: Array<{ name: string }>;
    spoken_languages: Array<{ english_name: string }>;
    production_countries: Array<{ name: string }>;
    poster_path: string | null;
    backdrop_path: string | null;
    credits?: { cast: unknown[]; crew: unknown[] };
    release_dates?: { results: Array<{ iso_3166_1: string; release_dates: Array<{ certification: string }> }> };
    keywords?: { keywords: Array<{ name: string }> };
  }): Omit<TmdbMovieRecord, 'id' | 'created_at' | 'updated_at'> {
    return {
      source_account_id: this.db.getCurrentSourceAccountId(),
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
      content_rating: this.client.extractUsRating(movie as never) ?? null,
      keywords: movie.keywords?.keywords.map(k => k.name) ?? [],
      synced_at: new Date(),
    };
  }

  /**
   * Map TMDB TV show to database record
   */
  private mapTvShowToRecord(show: {
    id: number;
    name: string;
    original_name: string;
    overview: string;
    first_air_date: string;
    last_air_date?: string | null;
    status: string;
    type: string;
    number_of_seasons: number;
    number_of_episodes: number;
    episode_run_time: number[];
    vote_average: number;
    vote_count: number;
    popularity: number;
    genres: Array<{ name: string }>;
    networks: Array<{ name: string }>;
    created_by: Array<{ name: string }>;
    poster_path: string | null;
    backdrop_path: string | null;
    content_ratings?: { results: Array<{ iso_3166_1: string; rating: string }> };
    keywords?: { results: Array<{ name: string }> };
    external_ids?: { imdb_id?: string | null };
  }): Omit<TmdbTvShowRecord, 'id' | 'created_at' | 'updated_at'> {
    return {
      source_account_id: this.db.getCurrentSourceAccountId(),
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
      content_rating: this.client.extractTvRating(show as never) ?? null,
      keywords: show.keywords?.results.map(k => k.name) ?? [],
      synced_at: new Date(),
    };
  }
}
