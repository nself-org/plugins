/**
 * The Pirate Bay Searcher
 * Web scraping-based searcher with mirror fallback
 */

import axios from 'axios';
import * as cheerio from 'cheerio';
import { BaseTorrentSearcher, TorrentSearchResult, SearchOptions } from '../base-searcher.js';
import { TorrentTitleParser } from '../../parsers/title-parser.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:tpb');

export class TPBSearcher extends BaseTorrentSearcher {
  readonly name = 'TPB';
  readonly baseUrl = 'https://thepiratebay.org';

  private readonly mirrors = [
    'https://thepiratebay.org',
    'https://tpb.party',
    'https://thepiratebay10.org',
    'https://pirateproxy.live'
  ];

  async search(options: SearchOptions): Promise<TorrentSearchResult[]> {
    const query = encodeURIComponent(options.query);
    const maxResults = options.maxResults || 50;

    const results: TorrentSearchResult[] = [];

    // Try each mirror until one works
    for (const mirror of this.mirrors) {
      try {
        const searchUrl = `${mirror}/search/${query}/1/99/0`; // Sort by seeders
        logger.debug(`Searching TPB: ${searchUrl}`);

        const response = await axios.get(searchUrl, {
          timeout: options.timeout || 10000,
          headers: {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
          }
        });

        const $ = cheerio.load(response.data);

        // Parse search results
        $('#searchResult tbody tr').each((i, el) => {
          if (results.length >= maxResults) return false;

          const $row = $(el);
          const $nameLink = $row.find('.detName a');
          const name = $nameLink.text().trim();

          if (!name) return;

          const detailUrl = mirror + $nameLink.attr('href');
          const $magnetLink = $row.find('a[href^="magnet:?"]');
          const magnetUri = $magnetLink.attr('href') || '';

          // Extract seeders and leechers
          const $td = $row.find('td');
          const seeders = parseInt($td.eq(2).text()) || 0;
          const leechers = parseInt($td.eq(3).text()) || 0;

          // Extract size
          const sizeText = $row.find('.detDesc').text();
          const sizeMatch = sizeText.match(/Size ([\d.]+ [KMG]iB)/);
          const size = sizeMatch ? sizeMatch[1] : '';

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
            size: size,
            sizeBytes: this.parseSize(size),
            seeders: seeders,
            leechers: leechers,
            uploadDate: new Date().toISOString(),
            uploadDateUnix: Date.now(),
            source: this.name,
            sourceUrl: detailUrl,
            parsedInfo: parsedInfo
          });
        });

        if (results.length > 0) {
          logger.info(`Found ${results.length} results from TPB`);
          break;
        }

      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn(`TPB mirror ${mirror} failed: ${message}`);
        continue;
      }
    }

    return results;
  }

  private extractInfoHash(magnetUri: string): string {
    const match = magnetUri.match(/btih:([a-fA-F0-9]{40})/);
    return match ? match[1].toLowerCase() : '';
  }
}
