/**
 * TorrentGalaxy Searcher
 * Web scraping-based searcher for TorrentGalaxy
 */

import axios from 'axios';
import * as cheerio from 'cheerio';
import { BaseTorrentSearcher, TorrentSearchResult, SearchOptions } from '../base-searcher.js';
import { TorrentTitleParser } from '../../parsers/title-parser.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:torrentgalaxy');

export class TorrentGalaxySearcher extends BaseTorrentSearcher {
  readonly name = 'TorrentGalaxy';
  readonly baseUrl = 'https://torrentgalaxy.to';

  async search(options: SearchOptions): Promise<TorrentSearchResult[]> {
    const query = encodeURIComponent(options.query);
    const maxResults = options.maxResults || 50;

    try {
      const searchUrl = `${this.baseUrl}/torrents.php?search=${query}`;
      logger.debug(`Searching TorrentGalaxy: ${searchUrl}`);

      const response = await axios.get(searchUrl, {
        timeout: options.timeout || 10000,
        headers: {
          'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
        }
      });

      const $ = cheerio.load(response.data);
      const results: TorrentSearchResult[] = [];

      // Parse results - TorrentGalaxy uses a different HTML structure
      $('.tgxtablerow').each((i, el) => {
        if (results.length >= maxResults) return false;

        const $row = $(el);
        const $titleLink = $row.find('.txlight a:first');
        const name = $titleLink.text().trim();
        const detailUrl = this.baseUrl + $titleLink.attr('href');

        if (!name) return;

        const $magnetLink = $row.find('a[href^="magnet:?"]');
        const magnetUri = $magnetLink.attr('href') || '';

        // Extract seeders and leechers
        const $seeds = $row.find('font[color="green"]');
        const seeders = parseInt($seeds.text()) || 0;
        const leechers = 0; // TorrentGalaxy doesn't always show leechers

        // Extract size
        const sizeText = $row.find('.badge-secondary').text().trim();

        // Filter by seeders
        if (options.minSeeders && seeders < options.minSeeders) {
          return;
        }

        const parsedInfo = TorrentTitleParser.parse(name);

        // Filter by quality
        if (options.quality && parsedInfo.quality !== options.quality) {
          return;
        }

        results.push({
          title: name,
          normalizedTitle: this.normalizeTitle(name),
          magnetUri: magnetUri,
          infoHash: this.extractInfoHash(magnetUri),
          size: sizeText,
          sizeBytes: this.parseSize(sizeText),
          seeders: seeders,
          leechers: leechers,
          uploadDate: new Date().toISOString(),
          uploadDateUnix: Date.now(),
          source: this.name,
          sourceUrl: detailUrl,
          parsedInfo: parsedInfo
        });
      });

      logger.info(`Found ${results.length} results from TorrentGalaxy`);
      return results;

    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      logger.warn(`TorrentGalaxy search failed: ${message}`);
      return [];
    }
  }

  private extractInfoHash(magnetUri: string): string {
    const match = magnetUri.match(/btih:([a-fA-F0-9]{40})/);
    return match ? match[1].toLowerCase() : '';
  }
}
