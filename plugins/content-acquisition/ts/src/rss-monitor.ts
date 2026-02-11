/**
 * RSS Feed Monitor
 */

import Parser from 'rss-parser';
import axios from 'axios';
import { createLogger } from '@nself/plugin-utils';
import { ContentAcquisitionDatabase } from './database.js';
import type { RSSFeed } from './types.js';

const logger = createLogger('content-acquisition:rss');

export class RSSFeedMonitor {
  private parser: Parser;
  private database: ContentAcquisitionDatabase;
  private torrentManagerUrl: string;

  constructor(database: ContentAcquisitionDatabase, torrentManagerUrl: string) {
    this.database = database;
    this.torrentManagerUrl = torrentManagerUrl;
    this.parser = new Parser();
  }

  async checkFeed(feed: RSSFeed): Promise<void> {
    try {
      logger.info(`Checking RSS feed: ${feed.name}`);
      const feedData = await this.parser.parseURL(feed.url);

      for (const item of feedData.items) {
        await this.processItem(feed, item);
      }

      logger.info(`Processed ${feedData.items.length} items from ${feed.name}`);
    } catch (error: any) {
      logger.error(`RSS feed check failed for ${feed.name}:`, error);
    }
  }

  private async processItem(feed: RSSFeed, item: any): Promise<void> {
    const title = item.title;
    const link = item.link;
    const magnetUri = item.enclosure?.url || '';

    // Simple title parsing (real implementation would use torrent-manager's parser)
    const parsedTitle = this.parseTitle(title);

    logger.debug(`Processing RSS item: ${title}`, parsedTitle);

    // Store in database
    // In real implementation, match to subscriptions and add to queue
  }

  private parseTitle(title: string): { title: string; season?: number; episode?: number; quality?: string } {
    // Simplified parsing - real implementation would import from torrent-manager
    const seasonMatch = title.match(/S(\d{2})E(\d{2})/i);
    const qualityMatch = title.match(/\b(1080p|720p|2160p|480p)\b/i);

    return {
      title: title.split(/\b(S\d{2}|1080p|720p|2160p)\b/i)[0].trim(),
      season: seasonMatch ? parseInt(seasonMatch[1]) : undefined,
      episode: seasonMatch ? parseInt(seasonMatch[2]) : undefined,
      quality: qualityMatch ? qualityMatch[1] : undefined,
    };
  }
}
