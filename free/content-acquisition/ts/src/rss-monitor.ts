/**
 * RSS Feed Monitor
 *
 * Checks configured RSS feeds on a schedule, parses item titles,
 * matches them against active subscriptions, searches for the best
 * torrent via torrent-manager, and queues matched items for download.
 */

import Parser from 'rss-parser';
import axios from 'axios';
import cron from 'node-cron';
import { createLogger } from '@nself/plugin-utils';
import { ContentAcquisitionDatabase } from './database.js';
import { ContentMatcher } from './matcher.js';
import type { PipelineOrchestrator } from './pipeline.js';
import type { RSSFeed, Subscription } from './types.js';

const logger = createLogger('content-acquisition:rss');

interface RSSFeedItem {
  title?: string;
  link?: string;
  pubDate?: string;
  enclosure?: {
    url?: string;
    type?: string;
    length?: string;
  };
  content?: string;
  contentSnippet?: string;
  guid?: string;
  categories?: string[];
  [key: string]: unknown;
}

export class RSSFeedMonitor {
  private parser: Parser;
  private database: ContentAcquisitionDatabase;
  private torrentManagerUrl: string;
  private pipeline: PipelineOrchestrator | null;
  private matcher: ContentMatcher;

  constructor(database: ContentAcquisitionDatabase, torrentManagerUrl: string, pipeline?: PipelineOrchestrator) {
    this.database = database;
    this.torrentManagerUrl = torrentManagerUrl;
    this.pipeline = pipeline ?? null;
    this.parser = new Parser();
    this.matcher = new ContentMatcher();
  }

  /**
   * Start a cron-based schedule that periodically checks all enabled RSS feeds.
   */
  startScheduledChecks(intervalMinutes: number = 30): void {
    const cronExpression = `*/${intervalMinutes} * * * *`;
    cron.schedule(cronExpression, async () => {
      logger.info('Running scheduled RSS feed check');
      try {
        const feeds = await this.database.listAllEnabledFeeds();
        for (const feed of feeds) {
          if (feed.enabled !== false) {
            await this.checkFeed(feed);
          }
        }
      } catch (error) {
        const metadata = error instanceof Error ? { message: error.message } : { error: String(error) };
        logger.error('Scheduled feed check failed:', metadata);
      }
    });
    logger.info(`RSS feed monitor started with ${intervalMinutes} minute interval`);
  }

  /**
   * Fetch and process all items in a single RSS feed.
   */
  async checkFeed(feed: RSSFeed): Promise<void> {
    try {
      logger.info(`Checking RSS feed: ${feed.name}`);
      const feedData = await this.parser.parseURL(feed.url);

      let processedCount = 0;
      for (const item of feedData.items) {
        const feedItem: RSSFeedItem = {
          title: item.title,
          link: item.link,
          pubDate: item.pubDate,
          enclosure: item.enclosure ? {
            url: item.enclosure.url,
            type: item.enclosure.type,
            length: item.enclosure.length ? String(item.enclosure.length) : undefined,
          } : undefined,
          content: item.content,
          contentSnippet: item.contentSnippet,
          guid: item.guid,
          categories: item.categories,
        };
        await this.processItem(feed, feedItem);
        processedCount++;
      }

      // Mark the feed as successfully checked
      await this.database.updateFeedLastChecked(feed.id);
      logger.info(`Processed ${processedCount} items from ${feed.name}`);
    } catch (error) {
      // Record the failure on the feed so consecutive_failures increments
      const errorMessage = error instanceof Error ? error.message : String(error);
      await this.database.updateFeedLastChecked(feed.id, errorMessage).catch(() => {
        // If even the error recording fails, just log it
        logger.error(`Failed to record feed check error for ${feed.name}`);
      });
      const metadata = error instanceof Error ? { message: error.message } : { error: String(error) };
      logger.error(`RSS feed check failed for ${feed.name}:`, metadata);
    }
  }

  /**
   * Process a single RSS feed item:
   * 1. Parse the title to extract structured metadata
   * 2. Store the item in the database (skip duplicates)
   * 3. Match against active subscriptions
   * 4. For matched items, search torrent-manager for the best torrent
   * 5. Add matched items to the acquisition queue
   */
  private async processItem(feed: RSSFeed, item: RSSFeedItem): Promise<void> {
    const title: string = item.title || '';
    const link: string = item.link || '';
    const magnetUri: string = item.enclosure?.url || '';
    const pubDate: Date | undefined = item.pubDate ? new Date(item.pubDate) : undefined;

    // Parse the title to extract structured data (show name, season, episode, quality)
    const parsedTitle = this.parseTitle(title);

    logger.debug(`Processing RSS item: ${title}`, parsedTitle);

    // 1. Store the item in the database
    const { feedItem, isNew } = await this.database.insertRSSFeedItem({
      feed_id: feed.id,
      source_account_id: feed.source_account_id,
      title,
      link,
      magnet_uri: magnetUri || undefined,
      pub_date: pubDate,
      parsed_title: parsedTitle.title,
      parsed_season: parsedTitle.season,
      parsed_episode: parsedTitle.episode,
      parsed_quality: parsedTitle.quality,
      status: 'pending',
    });

    // If the item was already seen, skip further processing
    if (!isNew) {
      logger.debug(`Skipping duplicate RSS item: ${title}`);
      return;
    }

    // 2. Match against active subscriptions for this account
    const matchedSubscriptions = await this.database.matchSubscriptions(
      feed.source_account_id,
      parsedTitle.title,
      feed.feed_type,
    );

    if (matchedSubscriptions.length === 0) {
      // No subscription matched; mark as rejected
      await this.database.updateRSSFeedItemStatus(feedItem.id, 'rejected', undefined, 'no_matching_subscription');
      logger.debug(`No matching subscription for: ${title}`);
      return;
    }

    // Use the first (most recently created) matching subscription
    const subscription: Subscription = matchedSubscriptions[0];
    logger.info(`RSS item matched subscription "${subscription.content_name}": ${title}`);

    // Mark the feed item as matched
    await this.database.updateRSSFeedItemStatus(feedItem.id, 'matched', subscription.id);

    // 3. Search torrent-manager for the best matching torrent
    const torrentResponse = await axios.post(
      `${this.torrentManagerUrl}/v1/search/best-match`,
      {
        query: parsedTitle.title,
        type: feed.feed_type === 'tv_shows' || feed.feed_type === 'anime' ? 'tv' : 'movie',
        season: parsedTitle.season,
        episode: parsedTitle.episode,
        quality: subscription.quality_profile_id ? undefined : parsedTitle.quality,
      },
      { timeout: 30000 }
    ).catch(err => {
      logger.error(`Torrent manager search failed: ${err.message}`);
      return null;
    });

    // 4. Add the matched item to the acquisition queue
    const contentType = feed.feed_type === 'tv_shows' || feed.feed_type === 'anime'
      ? 'tv_episode' as const
      : feed.feed_type === 'movies'
        ? 'movie' as const
        : 'other' as const;

    await this.database.addToQueue({
      source_account_id: feed.source_account_id,
      content_type: contentType,
      content_name: parsedTitle.title,
      season: parsedTitle.season,
      episode: parsedTitle.episode,
      quality_profile_id: subscription.quality_profile_id,
      requested_by: 'rss_monitor',
      request_source_id: feedItem.id,
      priority: 5,
      matched_torrent: torrentResponse?.data ?? undefined,
    });

    logger.info(`Queued "${parsedTitle.title}" (S${parsedTitle.season ?? '?'}E${parsedTitle.episode ?? '?'}) for acquisition`);

    // Trigger the full pipeline (detect -> VPN -> torrent -> metadata -> subtitles)
    if (this.pipeline && (magnetUri || link)) {
      try {
        const pipelineRun = await this.database.createPipelineRun({
          source_account_id: feed.source_account_id,
          trigger_type: 'rss_monitor',
          trigger_source: feed.name,
          content_title: parsedTitle.title,
          content_type: contentType,
          metadata: {
            magnet_url: magnetUri || undefined,
            torrent_url: link || undefined,
            feed_id: feed.id,
            feed_item_id: feedItem.id,
            season: parsedTitle.season,
            episode: parsedTitle.episode,
            quality: parsedTitle.quality,
          },
        });

        // Fire-and-forget: run pipeline asynchronously
        this.pipeline.executePipeline(pipelineRun.id).catch((err: unknown) => {
          const message = err instanceof Error ? err.message : String(err);
          logger.error(`Background pipeline execution failed for run ${pipelineRun.id}: ${message}`);
        });

        logger.info(`Pipeline ${pipelineRun.id} triggered for "${parsedTitle.title}"`);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        logger.error(`Failed to create pipeline run for "${parsedTitle.title}": ${message}`);
      }
    }
  }

  /**
   * Parse a torrent-style title into structured components.
   * Extracts: show/movie name, season, episode, and quality.
   */
  private parseTitle(title: string): { title: string; season?: number; episode?: number; quality?: string; year?: number } {
    const seasonMatch = title.match(/S(\d{2})E(\d{2})/i);

    // Use ContentMatcher for better quality detection (includes HDR, Dolby Vision, 4K, etc.)
    const qualities = this.matcher.extractQuality(title);
    const year = this.matcher.extractYear(title);

    // Normalize title using ContentMatcher
    const normalizedTitle = this.matcher.normalizeTitle(
      title.split(/\b(S\d{2}|1080p|720p|2160p|4K|HDR)\b/i)[0].trim()
    );

    return {
      title: normalizedTitle,
      season: seasonMatch ? parseInt(seasonMatch[1]) : undefined,
      episode: seasonMatch ? parseInt(seasonMatch[2]) : undefined,
      quality: qualities.length > 0 ? qualities.join(',') : undefined,
      year: year ?? undefined,
    };
  }
}
