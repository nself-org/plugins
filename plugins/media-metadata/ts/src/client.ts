/**
 * TMDB API Client
 * Complete wrapper around TMDB API v3 with rate limiting and pagination
 */

import { createLogger, RateLimiter } from '@nself/plugin-utils';
import type {
  TmdbMovie,
  TmdbTvShow,
  TmdbTvSeason,
  TmdbTvEpisode,
  TmdbGenre,
  TmdbSearchResult,
  SearchParams,
} from './types.js';

const logger = createLogger('tmdb:client');

export class TmdbClient {
  private apiKey: string;
  private baseUrl = 'https://api.themoviedb.org/3';
  private rateLimiter: RateLimiter;
  private language: string;

  constructor(apiKey: string, language = 'en-US') {
    this.apiKey = apiKey;
    this.language = language;
    // TMDB rate limit: 40 requests per 10 seconds = 4 requests per second
    this.rateLimiter = new RateLimiter(4);
    logger.info('TMDB client initialized');
  }

  /**
   * Make authenticated request to TMDB API
   */
  private async request<T>(endpoint: string, params: Record<string, string> = {}): Promise<T> {
    await this.rateLimiter.acquire();

    const url = new URL(`${this.baseUrl}${endpoint}`);
    url.searchParams.set('api_key', this.apiKey);
    url.searchParams.set('language', this.language);

    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null) {
        url.searchParams.set(key, String(value));
      }
    }

    logger.debug('TMDB API request', { endpoint, params });

    const response = await fetch(url.toString());

    if (!response.ok) {
      const error = await response.text();
      logger.error('TMDB API error', { status: response.status, error });
      throw new Error(`TMDB API error: ${response.status} - ${error}`);
    }

    return response.json() as Promise<T>;
  }

  // =========================================================================
  // Search
  // =========================================================================

  async search(params: SearchParams): Promise<TmdbSearchResult> {
    const endpoint = params.media_type === 'tv' ? '/search/tv' : '/search/movie';

    const queryParams: Record<string, string> = {
      query: params.query,
    };

    if (params.year) {
      queryParams[params.media_type === 'tv' ? 'first_air_date_year' : 'year'] = String(params.year);
    }

    if (params.page) {
      queryParams.page = String(params.page);
    }

    return this.request<TmdbSearchResult>(endpoint, queryParams);
  }

  async searchMulti(query: string, page = 1): Promise<TmdbSearchResult> {
    return this.request<TmdbSearchResult>('/search/multi', {
      query,
      page: String(page),
    });
  }

  // =========================================================================
  // Movies
  // =========================================================================

  async getMovie(tmdbId: number, includeCredits = true, includeKeywords = true): Promise<TmdbMovie> {
    const appendToResponse: string[] = [];

    if (includeCredits) {
      appendToResponse.push('credits');
    }

    if (includeKeywords) {
      appendToResponse.push('keywords');
    }

    appendToResponse.push('release_dates');

    const params: Record<string, string> = {};
    if (appendToResponse.length > 0) {
      params.append_to_response = appendToResponse.join(',');
    }

    return this.request<TmdbMovie>(`/movie/${tmdbId}`, params);
  }

  async getMovieCredits(tmdbId: number): Promise<{ cast: unknown[]; crew: unknown[] }> {
    return this.request<{ cast: unknown[]; crew: unknown[] }>(`/movie/${tmdbId}/credits`);
  }

  async getTrendingMovies(timeWindow: 'day' | 'week' = 'week', page = 1): Promise<TmdbSearchResult> {
    return this.request<TmdbSearchResult>(`/trending/movie/${timeWindow}`, {
      page: String(page),
    });
  }

  async getPopularMovies(page = 1): Promise<TmdbSearchResult> {
    return this.request<TmdbSearchResult>('/movie/popular', {
      page: String(page),
    });
  }

  // =========================================================================
  // TV Shows
  // =========================================================================

  async getTvShow(tmdbId: number, includeCredits = true, includeKeywords = true): Promise<TmdbTvShow> {
    const appendToResponse: string[] = [];

    if (includeCredits) {
      appendToResponse.push('credits');
    }

    if (includeKeywords) {
      appendToResponse.push('keywords');
    }

    appendToResponse.push('content_ratings');
    appendToResponse.push('external_ids');

    const params: Record<string, string> = {};
    if (appendToResponse.length > 0) {
      params.append_to_response = appendToResponse.join(',');
    }

    return this.request<TmdbTvShow>(`/tv/${tmdbId}`, params);
  }

  async getTvShowCredits(tmdbId: number): Promise<{ cast: unknown[]; crew: unknown[] }> {
    return this.request<{ cast: unknown[]; crew: unknown[] }>(`/tv/${tmdbId}/credits`);
  }

  async getTrendingTvShows(timeWindow: 'day' | 'week' = 'week', page = 1): Promise<TmdbSearchResult> {
    return this.request<TmdbSearchResult>(`/trending/tv/${timeWindow}`, {
      page: String(page),
    });
  }

  async getPopularTvShows(page = 1): Promise<TmdbSearchResult> {
    return this.request<TmdbSearchResult>('/tv/popular', {
      page: String(page),
    });
  }

  // =========================================================================
  // TV Seasons & Episodes
  // =========================================================================

  async getTvSeason(showTmdbId: number, seasonNumber: number): Promise<TmdbTvSeason> {
    return this.request<TmdbTvSeason>(`/tv/${showTmdbId}/season/${seasonNumber}`);
  }

  async getTvEpisode(
    showTmdbId: number,
    seasonNumber: number,
    episodeNumber: number
  ): Promise<TmdbTvEpisode> {
    return this.request<TmdbTvEpisode>(
      `/tv/${showTmdbId}/season/${seasonNumber}/episode/${episodeNumber}`
    );
  }

  // =========================================================================
  // Genres
  // =========================================================================

  async getMovieGenres(): Promise<{ genres: TmdbGenre[] }> {
    return this.request<{ genres: TmdbGenre[] }>('/genre/movie/list');
  }

  async getTvGenres(): Promise<{ genres: TmdbGenre[] }> {
    return this.request<{ genres: TmdbGenre[] }>('/genre/tv/list');
  }

  // =========================================================================
  // Helper Methods
  // =========================================================================

  getImageUrl(path: string | null, size = 'w500'): string | null {
    if (!path) return null;
    return `https://image.tmdb.org/t/p/${size}${path}`;
  }

  getOriginalImageUrl(path: string | null): string | null {
    return this.getImageUrl(path, 'original');
  }

  extractYear(dateString: string | null | undefined): number | null {
    if (!dateString) return null;
    const year = parseInt(dateString.substring(0, 4), 10);
    return isNaN(year) ? null : year;
  }

  extractUsRating(movie: TmdbMovie): string | null {
    if (!movie.release_dates?.results) return null;

    const usRelease = movie.release_dates.results.find(r => r.iso_3166_1 === 'US');
    if (!usRelease?.release_dates || usRelease.release_dates.length === 0) return null;

    // Prefer theatrical release (type 3) or premiere (type 1)
    const theatrical = usRelease.release_dates.find(rd => rd.type === 3);
    const premiere = usRelease.release_dates.find(rd => rd.type === 1);
    const any = usRelease.release_dates[0];

    return (theatrical?.certification || premiere?.certification || any?.certification) || null;
  }

  extractTvRating(show: TmdbTvShow): string | null {
    if (!show.content_ratings?.results) return null;

    const usRating = show.content_ratings.results.find(r => r.iso_3166_1 === 'US');
    return usRating?.rating || null;
  }
}
