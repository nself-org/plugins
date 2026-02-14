import { MovieDb } from 'moviedb-promise';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('metadata-enrichment:tmdb');

// TMDB API response types
interface TMDBMovieSearchResult {
  id: number;
  title: string;
  original_title: string;
  release_date: string;
  poster_path: string | null;
  backdrop_path: string | null;
  overview: string;
  vote_average: number;
  vote_count: number;
  popularity: number;
  genre_ids: number[];
  adult: boolean;
  original_language: string;
  video: boolean;
}

interface TMDBTVSearchResult {
  id: number;
  name: string;
  original_name: string;
  first_air_date: string;
  poster_path: string | null;
  backdrop_path: string | null;
  overview: string;
  vote_average: number;
  vote_count: number;
  popularity: number;
  genre_ids: number[];
  origin_country: string[];
  original_language: string;
}

interface TMDBMovieDetails extends TMDBMovieSearchResult {
  imdb_id: string | null;
  runtime: number | null;
  budget: number;
  revenue: number;
  status: string;
  tagline: string | null;
  genres: Array<{ id: number; name: string }>;
  production_companies: Array<{ id: number; name: string; logo_path: string | null }>;
  credits?: {
    cast: Array<{ id: number; name: string; character: string; profile_path: string | null }>;
    crew: Array<{ id: number; name: string; job: string; department: string }>;
  };
  videos?: {
    results: Array<{ key: string; name: string; site: string; type: string }>;
  };
  release_dates?: {
    results: Array<{
      iso_3166_1: string;
      release_dates: Array<{ certification: string; type: number; release_date: string }>;
    }>;
  };
}

interface TMDBTVDetails extends TMDBTVSearchResult {
  created_by: Array<{ id: number; name: string }>;
  episode_run_time: number[];
  genres: Array<{ id: number; name: string }>;
  homepage: string | null;
  in_production: boolean;
  languages: string[];
  last_air_date: string | null;
  number_of_episodes: number;
  number_of_seasons: number;
  production_companies: Array<{ id: number; name: string; logo_path: string | null }>;
  seasons: Array<{
    id: number;
    season_number: number;
    episode_count: number;
    air_date: string | null;
    poster_path: string | null;
  }>;
  status: string;
  type: string;
  credits?: {
    cast: Array<{ id: number; name: string; character: string; profile_path: string | null }>;
    crew: Array<{ id: number; name: string; job: string; department: string }>;
  };
  videos?: {
    results: Array<{ key: string; name: string; site: string; type: string }>;
  };
  external_ids?: {
    imdb_id: string | null;
    tvdb_id: number | null;
  };
}

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

  async searchMovies(query: string, year?: number): Promise<TMDBMovieSearchResult[]> {
    try {
      await this.throttle();
      const results = await this.tmdb.searchMovie({
        query,
        year,
        include_adult: false,
      });
      return (results.results || []) as TMDBMovieSearchResult[];
    } catch (error) {
      logger.error('Movie search failed:', error instanceof Error ? error.message : String(error));
      return [];
    }
  }

  async getMovieDetails(tmdbId: number): Promise<TMDBMovieDetails | null> {
    try {
      await this.throttle();
      const details = await this.tmdb.movieInfo({
        id: tmdbId,
        append_to_response: 'credits,videos,release_dates',
      });
      return details as TMDBMovieDetails;
    } catch (error) {
      logger.error('Get movie details failed:', error instanceof Error ? error.message : String(error));
      return null;
    }
  }

  async searchTV(query: string, year?: number): Promise<TMDBTVSearchResult[]> {
    try {
      await this.throttle();
      const results = await this.tmdb.searchTv({
        query,
        first_air_date_year: year,
        include_adult: false,
      });
      return (results.results || []) as TMDBTVSearchResult[];
    } catch (error) {
      logger.error('TV search failed:', error instanceof Error ? error.message : String(error));
      return [];
    }
  }

  async getTVShowDetails(tmdbId: number): Promise<TMDBTVDetails | null> {
    try {
      await this.throttle();
      const details = await this.tmdb.tvInfo({
        id: tmdbId,
        append_to_response: 'credits,videos,external_ids',
      });
      return details as TMDBTVDetails;
    } catch (error) {
      logger.error('Get TV show details failed:', error instanceof Error ? error.message : String(error));
      return null;
    }
  }
}
