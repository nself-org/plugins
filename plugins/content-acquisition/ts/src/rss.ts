/**
 * RSS Service
 * Simple RSS feed polling and matching functionality
 */

import Parser from 'rss-parser';
import { createLogger } from '@nself/plugin-utils';
import { ContentMatcher, type MatchCriteria, type RSSItem } from './matcher.js';

const logger = createLogger('content-acquisition:rss');

export interface RSSFeed {
  id: string;
  url: string;
  name: string;
  lastFetched?: Date;
  items?: RSSItem[];
}

export class RSSMonitor {
  private parser: Parser;
  private matcher: ContentMatcher;

  constructor() {
    this.parser = new Parser({
      timeout: 30000,
      headers: {
        'User-Agent': 'nself-content-acquisition/1.0',
      },
    });
    this.matcher = new ContentMatcher();
  }

  /**
   * Fetch items from an RSS feed
   */
  async fetchFeed(url: string): Promise<RSSItem[]> {
    try {
      const feed = await this.parser.parseURL(url);

      const items: RSSItem[] = feed.items.map(item => ({
        title: item.title || '',
        link: item.link || '',
        pubDate: item.pubDate || item.isoDate || new Date().toISOString(),
      }));

      logger.info('Fetched RSS feed', { url, itemCount: items.length });
      return items;
    } catch (error) {
      logger.error('RSS fetch error', { url, error });
      throw error;
    }
  }

  /**
   * Match RSS items against criteria
   */
  async matchItems(items: RSSItem[], criteria: MatchCriteria[]): Promise<RSSItem[]> {
    const matches: RSSItem[] = [];

    for (const item of items) {
      for (const rule of criteria) {
        if (this.matcher.match(item, rule)) {
          matches.push(item);
          break; // Don't match same item multiple times
        }
      }
    }

    logger.info('Matched RSS items', { total: items.length, matched: matches.length });
    return matches;
  }

  /**
   * Poll a feed and return new matching items
   */
  async pollFeed(url: string, criteria: MatchCriteria[], lastSeen?: Date): Promise<RSSItem[]> {
    const items = await this.fetchFeed(url);

    // Filter out items we've seen before
    const newItems = lastSeen
      ? items.filter(item => new Date(item.pubDate) > lastSeen)
      : items;

    // Match against criteria
    const matches = await this.matchItems(newItems, criteria);

    return matches;
  }
}
