import { MovieDb } from 'moviedb-promise';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('metadata-enrichment:tmdb');

export class TMDBClient {
  private tmdb: MovieDb;

  // Queue-based rate limiter for outbound TMDB API calls.
  // TMDB allows 50 req/s; we stay at 40 to leave headroom.
  private requestQueue: Array<() => void> = [];
  private activeRequests = 0;
  private readonly maxRequestsPerSecond = 40;

  constructor(apiKey: string) {
    this.tmdb = new MovieDb(apiKey);
  }

  /**
   * Wait until a request slot is available. Each slot is released after 1 second,
   * so at most `maxRequestsPerSecond` calls can be in-flight during any 1-second window.
   */
  private async throttle(): Promise<void> {
    if (this.activeRequests >= this.maxRequestsPerSecond) {
      await new Promise<void>((resolve) => {
        this.requestQueue.push(resolve);
      });
    }
    this.activeRequests++;
    setTimeout(() => {
      this.activeRequests--;
      const next = this.requestQueue.shift();
      if (next) next();
    }, 1000);
  }

  async searchMovies(query: string, year?: number): Promise<any[]> {
    try {
      await this.throttle();
      const results = await this.tmdb.searchMovie({
        query,
        year,
        include_adult: false,
      });
      return results.results || [];
    } catch (error: any) {
      logger.error('Movie search failed:', error);
      return [];
    }
  }

  async getMovieDetails(tmdbId: number): Promise<any> {
    try {
      await this.throttle();
      return await this.tmdb.movieInfo({
        id: tmdbId,
        append_to_response: 'credits,videos,release_dates',
      });
    } catch (error: any) {
      logger.error('Get movie details failed:', error);
      return null;
    }
  }

  async searchTV(query: string, year?: number): Promise<any[]> {
    try {
      await this.throttle();
      const results = await this.tmdb.searchTv({
        query,
        first_air_date_year: year,
        include_adult: false,
      });
      return results.results || [];
    } catch (error: any) {
      logger.error('TV search failed:', error);
      return [];
    }
  }

  async getTVShowDetails(tmdbId: number): Promise<any> {
    try {
      await this.throttle();
      return await this.tmdb.tvInfo({
        id: tmdbId,
        append_to_response: 'credits,videos,external_ids',
      });
    } catch (error: any) {
      logger.error('Get TV show details failed:', error);
      return null;
    }
  }
}
