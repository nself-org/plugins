/**
 * 1337x Torrent Searcher
 * Web scraping-based searcher with mirror fallback
 */

import axios from 'axios';
import * as cheerio from 'cheerio';
import { BaseTorrentSearcher, TorrentSearchResult, SearchOptions } from '../base-searcher.js';
import { TorrentTitleParser } from '../../parsers/title-parser.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:1337x');

export class X1337Searcher extends BaseTorrentSearcher {
  readonly name = '1337x';
  readonly baseUrl = 'https://1337x.to';

  // Backup domains in case main is blocked
  private readonly mirrors = [
    'https://1337x.to',
    'https://www.1337x.tw',
    'https://1337x.st',
    'https://1337x.is'
  ];

  async search(options: SearchOptions): Promise<TorrentSearchResult[]> {
    const query = encodeURIComponent(options.query);
    const category = this.getCategory(options.type);
    const maxResults = options.maxResults || 50;

    const results: TorrentSearchResult[] = [];

    // Try each mirror until one works
    for (const mirror of this.mirrors) {
      try {
        const searchUrl = category
          ? `${mirror}/category-search/${query}/${category}/1/`
          : `${mirror}/search/${query}/1/`;

        logger.debug(`Searching 1337x: ${searchUrl}`);

        const response = await axios.get(searchUrl, {
          timeout: options.timeout || 10000,
          headers: {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36'
          }
        });

        const $ = cheerio.load(response.data);

        // Parse search results table
        $('.table-list tbody tr').each((i, el) => {
          if (results.length >= maxResults) return false; // Stop when max reached

          const $row = $(el);

          // Extract data
          const nameElement = $row.find('.name a:nth-child(2)');
          const name = nameElement.text().trim();
          const detailUrl = this.baseUrl + nameElement.attr('href');

          const seeders = parseInt($row.find('.seeds').text()) || 0;
          const leechers = parseInt($row.find('.leeches').text()) || 0;
          const size = $row.find('.size').text().trim();
          const uploadDate = $row.find('.coll-date').text().trim();

          if (!name) return; // Skip invalid rows

          // Skip if no seeders and filter is enabled
          if (options.minSeeders && seeders < options.minSeeders) {
            return; // Continue to next
          }

          // Parse torrent title to extract metadata
          const parsedInfo = TorrentTitleParser.parse(name);

          // Filter by quality if specified
          if (options.quality && parsedInfo.quality !== options.quality) {
            return; // Continue to next
          }

          // Add result (magnetUri will be fetched on-demand)
          results.push({
            title: name,
            normalizedTitle: this.normalizeTitle(name),
            magnetUri: '', // Will be fetched when needed
            infoHash: '',
            size: size,
            sizeBytes: this.parseSize(size),
            seeders: seeders,
            leechers: leechers,
            uploadDate: uploadDate,
            uploadDateUnix: this.parseDate(uploadDate),
            source: this.name,
            sourceUrl: detailUrl,
            parsedInfo: parsedInfo
          });
        });

        // If we got results, break (don't try other mirrors)
        if (results.length > 0) {
          logger.info(`Found ${results.length} results from 1337x`);
          break;
        }

      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn(`1337x mirror ${mirror} failed: ${message}`);
        // Continue to next mirror
        continue;
      }
    }

    return results;
  }

  /**
   * Fetch magnet link for a specific torrent
   * Called lazily when torrent is selected
   */
  async getMagnetLink(detailUrl: string): Promise<string> {
    try {
      const response = await axios.get(detailUrl, {
        timeout: 10000,
        headers: {
          'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
        }
      });

      const $ = cheerio.load(response.data);
      const magnetLink = $('a[href^="magnet:?"]').first().attr('href');

      if (!magnetLink) {
        throw new Error('Magnet link not found on detail page');
      }

      return magnetLink;
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      throw new Error(`Failed to fetch magnet link: ${message}`);
    }
  }

  private getCategory(type?: string): string | null {
    const categories: Record<string, string> = {
      'movie': 'Movies',
      'tv': 'TV'
    };
    return type ? categories[type] || null : null;
  }

  private parseDate(dateStr: string): number {
    // 1337x date formats: "2 days ago", "1 week ago", "Jan. 15th '23"
    const now = Date.now();

    // Relative dates
    if (dateStr.includes('ago')) {
      const match = dateStr.match(/(\d+)\s+(minute|min|hour|day|week|month)s?\s+ago/i);
      if (match) {
        const value = parseInt(match[1]);
        const unit = match[2].toLowerCase();

        const multipliers: Record<string, number> = {
          'min': 60 * 1000,
          'minute': 60 * 1000,
          'hour': 60 * 60 * 1000,
          'day': 24 * 60 * 60 * 1000,
          'week': 7 * 24 * 60 * 60 * 1000,
          'month': 30 * 24 * 60 * 60 * 1000,
        };

        return now - (value * (multipliers[unit] || 0));
      }
    }

    // Absolute dates: "Jan. 15th '23"
    try {
      const date = new Date(dateStr);
      return date.getTime();
    } catch {
      return now;
    }
  }
}
