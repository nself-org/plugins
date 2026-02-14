/**
 * YTS Torrent Searcher (Movies Only)
 * API-based searcher for high-quality movie torrents
 */

import axios from 'axios';
import { BaseTorrentSearcher, TorrentSearchResult, SearchOptions, ParsedTorrentInfo } from '../base-searcher.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:yts');

interface YTSMovie {
  id: number;
  title: string;
  year: number;
  rating: number;
  runtime: number;
  genres: string[];
  torrents: Array<{
    url: string;
    hash: string;
    quality: string;
    type: string;
    size: string;
    size_bytes: number;
    seeds: number;
    peers: number;
  }>;
}

interface YTSResponse {
  status: string;
  status_message: string;
  data: {
    movie_count: number;
    limit: number;
    page_number: number;
    movies?: YTSMovie[];
  };
}

export class YTSSearcher extends BaseTorrentSearcher {
  readonly name = 'YTS';
  readonly baseUrl = 'https://yts.mx';
  private readonly apiUrl = 'https://yts.mx/api/v2';

  async search(options: SearchOptions): Promise<TorrentSearchResult[]> {
    // YTS is movies only
    if (options.type === 'tv') {
      return [];
    }

    const params: Record<string, string | number> = {
      query_term: options.query,
      limit: options.maxResults || 50,
      page: 1,
      sort_by: 'seeds',
      order_by: 'desc'
    };

    // Add quality filter if specified
    if (options.quality) {
      params['quality'] = options.quality;
    }

    try {
      logger.debug(`Searching YTS for: ${options.query}`);

      const response = await axios.get<YTSResponse>(`${this.apiUrl}/list_movies.json`, {
        params,
        timeout: options.timeout || 10000
      });

      const data = response.data;

      if (data.status !== 'ok' || !data.data || !data.data.movies) {
        return [];
      }

      const results: TorrentSearchResult[] = [];

      for (const movie of data.data.movies) {
        // YTS provides multiple quality variants per movie
        for (const torrent of movie.torrents) {
          // Filter by seeders
          if (options.minSeeders && torrent.seeds < options.minSeeders) {
            continue;
          }

          // Filter by quality
          if (options.quality && torrent.quality !== options.quality) {
            continue;
          }

          // Construct torrent name
          const torrentName = `${movie.title} (${movie.year}) [${torrent.quality}] [YTS]`;

          // Construct magnet URI from hash
          const magnetUri = `magnet:?xt=urn:btih:${torrent.hash}&dn=${encodeURIComponent(torrentName)}&tr=udp://tracker.opentrackr.org:1337/announce&tr=udp://open.tracker.cl:1337/announce`;

          const parsedInfo: ParsedTorrentInfo = {
            title: movie.title,
            year: movie.year,
            quality: torrent.quality,
            source: torrent.type === 'bluray' ? 'BluRay' : 'WEB-DL',
            codec: 'x264',
            releaseGroup: 'YTS',
            type: 'movie',
            language: 'English'
          };

          results.push({
            title: torrentName,
            normalizedTitle: this.normalizeTitle(torrentName),
            infoHash: torrent.hash,
            magnetUri: magnetUri,
            size: torrent.size,
            sizeBytes: torrent.size_bytes,
            seeders: torrent.seeds,
            leechers: torrent.peers,
            uploadDate: new Date().toISOString(),
            uploadDateUnix: Date.now(),
            source: this.name,
            sourceUrl: `${this.baseUrl}/movies/${movie.title.toLowerCase().replace(/\s+/g, '-')}-${movie.year}`,
            parsedInfo: parsedInfo
          });
        }
      }

      logger.info(`Found ${results.length} results from YTS`);
      return results;

    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      logger.warn(`YTS search failed: ${message}`);
      return [];
    }
  }
}
