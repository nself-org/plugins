/**
 * Feed Refresh Scheduler
 * Manages periodic feed refresh with adaptive intervals based on feed activity
 */

import { createLogger } from '@nself/plugin-utils';
import { PodcastDatabase } from './database.js';
import { fetchAndParseFeed } from './feed-parser.js';
import type { Config } from './config.js';
import type { FeedRecord, ParsedFeed } from './types.js';

const logger = createLogger('podcast:scheduler');

export class FeedScheduler {
  private db: PodcastDatabase;
  private config: Config;
  private timer: ReturnType<typeof setInterval> | null = null;
  private running = false;

  constructor(db: PodcastDatabase, config: Config) {
    this.db = db;
    this.config = config;
  }

  /**
   * Start the scheduler. Checks for feeds needing refresh every 60 seconds.
   */
  start(): void {
    if (this.timer) {
      logger.warn('Scheduler already running');
      return;
    }

    logger.info('Starting feed refresh scheduler');

    // Run immediately on start
    void this.tick();

    // Then run every 60 seconds
    this.timer = setInterval(() => void this.tick(), 60000);
  }

  /**
   * Stop the scheduler
   */
  stop(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
      logger.info('Feed refresh scheduler stopped');
    }
  }

  /**
   * Single scheduler tick: find and refresh feeds that are due
   */
  private async tick(): Promise<void> {
    if (this.running) {
      logger.debug('Scheduler tick skipped (previous run still in progress)');
      return;
    }

    this.running = true;

    try {
      const feedsDue = await this.db.getFeedsNeedingRefresh();

      if (feedsDue.length === 0) {
        logger.debug('No feeds due for refresh');
        return;
      }

      logger.info(`Refreshing ${feedsDue.length} feed(s)`);

      for (const feed of feedsDue) {
        try {
          await this.refreshFeed(feed);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error('Feed refresh failed', { feedId: feed.id, url: feed.url, error: message });
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Scheduler tick failed', { error: message });
    } finally {
      this.running = false;
    }
  }

  /**
   * Refresh a single feed: fetch, parse, and upsert episodes
   */
  async refreshFeed(feed: FeedRecord): Promise<{ newEpisodes: number; totalEpisodes: number }> {
    logger.debug('Refreshing feed', { feedId: feed.id, url: feed.url });

    let parsed: ParsedFeed;
    try {
      parsed = await fetchAndParseFeed(feed.url);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      await this.db.markFeedError(feed.id, message);
      throw error;
    }

    // Update feed metadata
    await this.db.updateFeedMetadata(feed.id, {
      title: parsed.title,
      description: parsed.description,
      author: parsed.author,
      imageUrl: parsed.imageUrl,
      language: parsed.language,
      categories: parsed.categories.length > 0 ? parsed.categories : undefined,
      lastEpisodeAt: parsed.episodes.length > 0
        ? parsed.episodes
          .filter(e => e.pubDate !== null)
          .sort((a, b) => (b.pubDate?.getTime() ?? 0) - (a.pubDate?.getTime() ?? 0))[0]?.pubDate ?? null
        : undefined,
    });

    // Determine which episodes are new
    const existingGuids = await this.db.getExistingGuids(feed.id);
    let newEpisodeCount = 0;

    for (const episode of parsed.episodes) {
      if (!existingGuids.has(episode.guid)) {
        newEpisodeCount++;
      }
    }

    // Upsert all episodes
    const totalEpisodes = await this.db.upsertEpisodes(
      feed.id,
      parsed.episodes.map(ep => ({
        guid: ep.guid,
        title: ep.title,
        description: ep.description,
        pubDate: ep.pubDate,
        durationSeconds: ep.durationSeconds,
        enclosureUrl: ep.enclosureUrl,
        enclosureType: ep.enclosureType,
        enclosureLength: ep.enclosureLength,
        seasonNumber: ep.seasonNumber,
        episodeNumber: ep.episodeNumber,
        episodeType: ep.episodeType,
        chaptersUrl: ep.chaptersUrl,
        transcriptUrl: ep.transcriptUrl,
        imageUrl: ep.imageUrl,
      }))
    );

    // Calculate adaptive refresh interval
    const intervalMinutes = this.calculateRefreshInterval(feed, parsed);

    // Mark feed as successfully fetched
    await this.db.markFeedFetched(feed.id, intervalMinutes);

    logger.info('Feed refreshed', {
      feedId: feed.id,
      title: parsed.title,
      newEpisodes: newEpisodeCount,
      totalEpisodes,
      nextIntervalMinutes: intervalMinutes,
    });

    return { newEpisodes: newEpisodeCount, totalEpisodes };
  }

  /**
   * Calculate adaptive refresh interval based on feed activity.
   *
   * - Active feeds (new content in last 7 days): refreshActiveMinutes (default 60)
   * - Dormant feeds (7-30 days since last episode): refreshDormantHours * 60
   * - Stale feeds (>30 days): refreshStaleHours * 60
   */
  private calculateRefreshInterval(_feed: FeedRecord, parsed: ParsedFeed): number {
    const now = Date.now();
    const sevenDays = 7 * 24 * 60 * 60 * 1000;
    const thirtyDays = 30 * 24 * 60 * 60 * 1000;

    // Find the most recent episode date
    let latestEpisodeDate: Date | null = null;
    for (const episode of parsed.episodes) {
      if (episode.pubDate && (!latestEpisodeDate || episode.pubDate > latestEpisodeDate)) {
        latestEpisodeDate = episode.pubDate;
      }
    }

    if (!latestEpisodeDate) {
      // No dated episodes, use dormant interval
      return this.config.refreshDormantHours * 60;
    }

    const age = now - latestEpisodeDate.getTime();

    if (age < sevenDays) {
      // Active: new content within 7 days
      return this.config.refreshActiveMinutes;
    }

    if (age < thirtyDays) {
      // Dormant: 7-30 days since last episode
      return this.config.refreshDormantHours * 60;
    }

    // Stale: over 30 days since last episode
    return this.config.refreshStaleHours * 60;
  }

  /**
   * Force refresh all feeds regardless of schedule
   */
  async refreshAllFeeds(): Promise<{ refreshed: number; errors: number }> {
    const feeds = await this.db.listFeeds('active');
    let refreshed = 0;
    let errors = 0;

    for (const feed of feeds) {
      try {
        await this.refreshFeed(feed);
        refreshed++;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Feed refresh failed', { feedId: feed.id, error: message });
        errors++;
      }
    }

    return { refreshed, errors };
  }
}
