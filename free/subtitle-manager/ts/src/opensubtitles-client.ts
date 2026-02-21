import axios from 'axios';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('subtitle-manager:opensubtitles');

// OpenSubtitles API response types
export interface OpenSubtitlesSearchResult {
  id: string;
  type: string;
  attributes: {
    subtitle_id: string;
    language: string;
    download_count: number;
    new_download_count: number;
    hearing_impaired: boolean;
    hd: boolean;
    format: string;
    fps: number;
    votes: number;
    points: number;
    ratings: number;
    from_trusted: boolean;
    foreign_parts_only: boolean;
    ai_translated: boolean;
    machine_translated: boolean;
    upload_date: string;
    release: string;
    comments: string;
    legacy_subtitle_id: number;
    uploader: {
      uploader_id: number;
      name: string;
      rank: string;
    };
    feature_details: {
      feature_id: number;
      feature_type: string;
      year: number;
      title: string;
      movie_name: string;
      imdb_id: number;
      tmdb_id: number;
    };
    url: string;
    related_links: Array<{
      label: string;
      url: string;
      img_url: string;
    }>;
    files: Array<{
      file_id: number;
      cd_number: number;
      file_name: string;
    }>;
  };
}

export class OpenSubtitlesClient {
  private apiKey?: string;
  private baseUrl = 'https://api.opensubtitles.com/api/v1';

  constructor(apiKey?: string) {
    this.apiKey = apiKey;
  }

  async searchByQuery(query: string, languages: string[] = ['en']): Promise<OpenSubtitlesSearchResult[]> {
    if (!this.apiKey) {
      logger.warn('OpenSubtitles API key not configured');
      return [];
    }

    try {
      const response = await axios.get(`${this.baseUrl}/subtitles`, {
        params: {
          query,
          languages: languages.join(','),
        },
        headers: {
          'Api-Key': this.apiKey,
        },
      });

      return response.data.data || [];
    } catch (error) {
      logger.error('OpenSubtitles search failed:', { error: error instanceof Error ? error.message : String(error) });
      return [];
    }
  }

  async searchByHash(
    moviehash: string,
    moviebytesize: number,
    languages: string[] = ['en'],
  ): Promise<OpenSubtitlesSearchResult[]> {
    if (!this.apiKey) {
      logger.warn('OpenSubtitles API key not configured');
      return [];
    }

    try {
      const response = await axios.get(`${this.baseUrl}/subtitles`, {
        params: {
          moviehash,
          moviebytesize: moviebytesize.toString(),
          languages: languages.join(','),
        },
        headers: {
          'Api-Key': this.apiKey,
          'Content-Type': 'application/json',
        },
      });

      return response.data.data || [];
    } catch (error) {
      logger.error('OpenSubtitles hash search failed:', { error: error instanceof Error ? error.message : String(error) });
      return [];
    }
  }

  async downloadSubtitle(fileId: number): Promise<Buffer | null> {
    if (!this.apiKey) {
      return null;
    }

    try {
      const response = await axios.post(
        `${this.baseUrl}/download`,
        { file_id: fileId },
        {
          headers: { 'Api-Key': this.apiKey },
          responseType: 'arraybuffer',
        }
      );

      return Buffer.from(response.data);
    } catch (error) {
      logger.error('Subtitle download failed:', { error: error instanceof Error ? error.message : String(error) });
      return null;
    }
  }
}
