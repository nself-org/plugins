import { MovieDb } from 'moviedb-promise';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('metadata-enrichment:tmdb');

export class TMDBClient {
  private tmdb: MovieDb;

  constructor(apiKey: string) {
    this.tmdb = new MovieDb(apiKey);
  }

  async searchMovies(query: string, year?: number): Promise<any[]> {
    try {
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
      return await this.tmdb.tvInfo({
        id: tmdbId,
        append_to_response: 'credits,videos',
      });
    } catch (error: any) {
      logger.error('Get TV show details failed:', error);
      return null;
    }
  }
}
