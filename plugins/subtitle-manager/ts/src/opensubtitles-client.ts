import axios from 'axios';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('subtitle-manager:opensubtitles');

export class OpenSubtitlesClient {
  private apiKey?: string;
  private baseUrl = 'https://api.opensubtitles.com/api/v1';

  constructor(apiKey?: string) {
    this.apiKey = apiKey;
  }

  async searchByQuery(query: string, languages: string[] = ['en']): Promise<any[]> {
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
    } catch (error: any) {
      logger.error('OpenSubtitles search failed:', error);
      return [];
    }
  }

  async searchByHash(
    moviehash: string,
    moviebytesize: number,
    languages: string[] = ['en'],
  ): Promise<any[]> {
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
    } catch (error: any) {
      logger.error('OpenSubtitles hash search failed:', error.message);
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
    } catch (error: any) {
      logger.error('Subtitle download failed:', error);
      return null;
    }
  }
}
