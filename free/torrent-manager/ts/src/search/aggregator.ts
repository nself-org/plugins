/**
 * Torrent Search Aggregator
 * Combines results from multiple torrent sources
 */

import { BaseTorrentSearcher, TorrentSearchResult, SearchOptions } from './base-searcher.js';
import { X1337Searcher } from './searchers/1337x-searcher.js';
import { YTSSearcher } from './searchers/yts-searcher.js';
import { TorrentGalaxySearcher } from './searchers/torrentgalaxy-searcher.js';
import { TPBSearcher } from './searchers/tpb-searcher.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:aggregator');

export class TorrentSearchAggregator {
  private searchers: BaseTorrentSearcher[] = [];
  private enabledSources: Set<string>;

  constructor(enabledSources?: string[]) {
    // Initialize all searchers
    const allSearchers = [
      new X1337Searcher(),
      new YTSSearcher(),
      new TorrentGalaxySearcher(),
      new TPBSearcher()
    ];

    // Filter to enabled sources only
    if (enabledSources && enabledSources.length > 0) {
      this.enabledSources = new Set(enabledSources.map(s => s.toLowerCase()));
      this.searchers = allSearchers.filter(s =>
        this.enabledSources.has(s.name.toLowerCase())
      );
    } else {
      // All sources enabled by default
      this.searchers = allSearchers;
      this.enabledSources = new Set(allSearchers.map(s => s.name.toLowerCase()));
    }

    logger.info(`Initialized aggregator with sources: ${this.searchers.map(s => s.name).join(', ')}`);
  }

  /**
   * Search all enabled torrent sources in parallel
   */
  async search(options: SearchOptions): Promise<TorrentSearchResult[]> {
    logger.info(`Searching ${this.searchers.length} sources for: ${options.query}`);

    // Execute searches in parallel with timeout
    const SEARCH_TIMEOUT_MS = 30000;

    const searchPromises = this.searchers.map(searcher =>
      Promise.race([
        searcher.search(options),
        new Promise<TorrentSearchResult[]>((_, reject) =>
          setTimeout(() => reject(new Error(`Search timeout for ${searcher.name} after ${SEARCH_TIMEOUT_MS}ms`)), SEARCH_TIMEOUT_MS)
        )
      ]).catch(error => {
        logger.error(`Search failed for ${searcher.name}: ${error.message}`);
        return [] as TorrentSearchResult[];
      })
    );

    const resultsArrays = await Promise.all(searchPromises);

    // Flatten results
    const allResults = resultsArrays.flat();

    logger.info(`Found ${allResults.length} total results from ${this.searchers.length} sources`);

    // Remove duplicates based on normalized title
    const uniqueResults = this.deduplicateResults(allResults);

    logger.info(`${uniqueResults.length} unique results after deduplication`);

    // Sort by seeders (descending)
    uniqueResults.sort((a, b) => b.seeders - a.seeders);

    return uniqueResults;
  }

  /**
   * Remove duplicate torrents
   * Keeps the one with most seeders
   */
  private deduplicateResults(results: TorrentSearchResult[]): TorrentSearchResult[] {
    const seen = new Map<string, TorrentSearchResult>();

    for (const result of results) {
      const key = result.normalizedTitle;

      // If not seen before, add it
      if (!seen.has(key)) {
        seen.set(key, result);
        continue;
      }

      // If seen before, keep the one with more seeders
      const existing = seen.get(key)!;
      if (result.seeders > existing.seeders) {
        seen.set(key, result);
      }
    }

    return Array.from(seen.values());
  }

  /**
   * Get magnet link for a specific result
   * Some searchers (like 1337x) require a second request
   */
  async getMagnetLink(result: TorrentSearchResult): Promise<string> {
    // If magnet already present, return it
    if (result.magnetUri && result.magnetUri.startsWith('magnet:')) {
      return result.magnetUri;
    }

    // Otherwise, fetch from source
    const searcher = this.searchers.find(s => s.name === result.source);
    if (!searcher) {
      throw new Error(`Searcher ${result.source} not found`);
    }

    // Call searcher-specific method to fetch magnet
    // Type-safe check for getMagnetLink method
    interface SearcherWithMagnetLink {
      getMagnetLink(sourceUrl: string): Promise<string>;
    }

    if ('getMagnetLink' in searcher && typeof (searcher as SearcherWithMagnetLink).getMagnetLink === 'function') {
      return await (searcher as SearcherWithMagnetLink).getMagnetLink(result.sourceUrl);
    }

    throw new Error(`Searcher ${result.source} does not support magnet fetching`);
  }

  /**
   * Get list of enabled source names
   */
  getEnabledSources(): string[] {
    return this.searchers.map(s => s.name);
  }
}
