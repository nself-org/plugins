#!/usr/bin/env node
/**
 * Podcast Plugin CLI
 * Command-line interface for podcast feed management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { PodcastDatabase } from './database.js';
import { createServer } from './server.js';
import { FeedScheduler } from './scheduler.js';
import { discoverPodcasts } from './discovery.js';
import { parseOpml, extractFeedUrls, generateOpml } from './opml.js';
import { downloadEpisode } from './downloader.js';
import { readFileSync } from 'node:fs';

const logger = createLogger('podcast:cli');

const program = new Command();

program
  .name('nself-podcast')
  .description('Podcast plugin for nself - RSS/Atom feed management and episode tracking')
  .version('1.0.0');

// =========================================================================
// Init Command
// =========================================================================

program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new PodcastDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();
      logger.success('Database schema initialized');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Init failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Server Command
// =========================================================================

program
  .command('server')
  .description('Start the podcast server')
  .option('-p, --port <port>', 'Server port', '3023')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Status Command
// =========================================================================

program
  .command('status')
  .description('Show podcast statistics')
  .action(async () => {
    try {
      const db = new PodcastDatabase();
      await db.connect();
      await db.initializeSchema();

      const stats = await db.getStats();

      console.log('\nPodcast Plugin Status');
      console.log('=====================');
      console.log(`Feeds:              ${stats.feed_count}`);
      console.log(`Episodes:           ${stats.episode_count}`);
      console.log(`Unplayed:           ${stats.unplayed_count}`);
      console.log(`Downloaded:         ${stats.downloaded_count}`);
      console.log(`Total Duration:     ${stats.total_duration_hours.toFixed(1)} hours`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Feeds Command
// =========================================================================

program
  .command('feeds')
  .description('List or manage podcast feeds')
  .argument('[action]', 'Action: list, add, remove, show', 'list')
  .argument('[value]', 'Feed URL (for add) or Feed ID (for remove/show)')
  .option('-t, --title <title>', 'Feed title (for add)')
  .option('-s, --status <status>', 'Filter by status: active, paused, error')
  .action(async (action, value, options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();
      await db.initializeSchema();

      switch (action) {
        case 'list': {
          const feeds = await db.listFeeds(options.status);
          if (feeds.length === 0) {
            console.log('\nNo feeds found. Add one with: nself-podcast feeds add <url>');
            break;
          }
          console.log('\nPodcast Feeds:');
          console.log('-'.repeat(90));
          for (const feed of feeds) {
            const episodeCount = await db.countEpisodes(feed.id);
            const status = feed.status.toUpperCase().padEnd(7);
            const title = (feed.title ?? 'Untitled').slice(0, 40).padEnd(40);
            console.log(`  [${status}] ${feed.id.slice(0, 8)} | ${title} | ${episodeCount} eps`);
          }
          console.log(`\nTotal: ${feeds.length} feed(s)`);
          break;
        }
        case 'add': {
          if (!value) {
            logger.error('Feed URL required: nself-podcast feeds add <url>');
            process.exit(1);
          }
          const config = loadConfig();
          const feed = await db.insertFeed(value, options.title);
          logger.success(`Subscribed to feed: ${feed.id}`);

          // Try to fetch the feed immediately
          try {
            const scheduler = new FeedScheduler(db, config);
            const result = await scheduler.refreshFeed(feed);
            const updatedFeed = await db.getFeed(feed.id);
            console.log(`  Title:    ${updatedFeed?.title ?? 'Unknown'}`);
            console.log(`  Episodes: ${result.totalEpisodes}`);
          } catch (err) {
            const msg = err instanceof Error ? err.message : 'Unknown error';
            logger.warn(`Feed added but initial fetch failed: ${msg}`);
          }
          break;
        }
        case 'remove': {
          if (!value) {
            logger.error('Feed ID required: nself-podcast feeds remove <id>');
            process.exit(1);
          }
          const deleted = await db.deleteFeed(value);
          if (deleted) {
            logger.success('Feed removed');
          } else {
            logger.error('Feed not found');
            process.exit(1);
          }
          break;
        }
        case 'show': {
          if (!value) {
            logger.error('Feed ID required: nself-podcast feeds show <id>');
            process.exit(1);
          }
          const feed = await db.getFeed(value);
          if (!feed) {
            logger.error('Feed not found');
            process.exit(1);
          }
          console.log(JSON.stringify(feed, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}. Use: list, add, remove, show`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Episodes Command
// =========================================================================

program
  .command('episodes')
  .description('Browse and manage episodes')
  .argument('[action]', 'Action: list, new, show', 'new')
  .argument('[value]', 'Feed ID (for list) or Episode ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, value, options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();

      const limit = parseInt(options.limit, 10);

      switch (action) {
        case 'list': {
          if (!value) {
            logger.error('Feed ID required: nself-podcast episodes list <feedId>');
            process.exit(1);
          }
          const episodes = await db.listEpisodes(value, limit);
          if (episodes.length === 0) {
            console.log('\nNo episodes found for this feed.');
            break;
          }
          console.log('\nEpisodes:');
          console.log('-'.repeat(100));
          for (const ep of episodes) {
            const date = ep.pub_date ? ep.pub_date.toISOString().split('T')[0] : 'N/A';
            const duration = ep.duration_seconds
              ? formatDuration(ep.duration_seconds)
              : 'N/A';
            const played = ep.played ? 'P' : ' ';
            const downloaded = ep.downloaded ? 'D' : ' ';
            const title = ep.title.slice(0, 50).padEnd(50);
            console.log(`  [${played}${downloaded}] ${ep.id.slice(0, 8)} | ${date} | ${duration.padStart(8)} | ${title}`);
          }
          break;
        }
        case 'new': {
          const episodes = await db.getNewEpisodes(limit);
          if (episodes.length === 0) {
            console.log('\nNo new episodes. All caught up!');
            break;
          }
          console.log('\nNew Episodes:');
          console.log('-'.repeat(100));
          for (const ep of episodes) {
            const date = ep.pub_date ? ep.pub_date.toISOString().split('T')[0] : 'N/A';
            const duration = ep.duration_seconds
              ? formatDuration(ep.duration_seconds)
              : 'N/A';
            const title = ep.title.slice(0, 50).padEnd(50);
            console.log(`  ${ep.id.slice(0, 8)} | ${date} | ${duration.padStart(8)} | ${title}`);
          }
          console.log(`\nShowing ${episodes.length} unplayed episode(s)`);
          break;
        }
        case 'show': {
          if (!value) {
            logger.error('Episode ID required: nself-podcast episodes show <id>');
            process.exit(1);
          }
          const episode = await db.getEpisode(value);
          if (!episode) {
            logger.error('Episode not found');
            process.exit(1);
          }
          console.log(JSON.stringify(episode, null, 2));
          break;
        }
        default:
          logger.error(`Unknown action: ${action}. Use: list, new, show`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Discover Command
// =========================================================================

program
  .command('discover')
  .description('Search for podcasts')
  .argument('<query>', 'Search query')
  .option('-l, --limit <limit>', 'Number of results', '10')
  .action(async (query, options) => {
    try {
      const config = loadConfig();
      const limit = parseInt(options.limit, 10);

      logger.info(`Searching for "${query}"...`);
      const results = await discoverPodcasts(query, config, limit);

      if (results.length === 0) {
        console.log('\nNo podcasts found.');
        return;
      }

      console.log(`\nFound ${results.length} podcast(s):\n`);
      for (const [i, result] of results.entries()) {
        console.log(`${i + 1}. ${result.title}`);
        console.log(`   Author: ${result.author}`);
        console.log(`   Genre:  ${result.genre}`);
        console.log(`   Feed:   ${result.feedUrl}`);
        if (result.episodeCount) {
          console.log(`   Episodes: ${result.episodeCount}`);
        }
        console.log();
      }

      console.log('Subscribe with: nself-podcast feeds add <feedUrl>');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Discovery failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Refresh Command
// =========================================================================

program
  .command('refresh')
  .description('Refresh feeds')
  .argument('[feedId]', 'Specific feed ID to refresh (omit for all)')
  .action(async (feedId) => {
    try {
      const config = loadConfig();
      const db = new PodcastDatabase();
      await db.connect();
      await db.initializeSchema();

      const scheduler = new FeedScheduler(db, config);

      if (feedId) {
        const feed = await db.getFeed(feedId);
        if (!feed) {
          logger.error('Feed not found');
          await db.disconnect();
          process.exit(1);
        }
        const result = await scheduler.refreshFeed(feed);
        const updatedFeed = await db.getFeed(feedId);
        console.log(`\nRefreshed: ${updatedFeed?.title ?? feed.url}`);
        console.log(`  New episodes:   ${result.newEpisodes}`);
        console.log(`  Total episodes: ${result.totalEpisodes}`);
      } else {
        logger.info('Refreshing all feeds...');
        const result = await scheduler.refreshAllFeeds();
        console.log(`\nRefreshed: ${result.refreshed} feed(s)`);
        if (result.errors > 0) {
          console.log(`Errors:    ${result.errors} feed(s)`);
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Refresh failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Import Command
// =========================================================================

program
  .command('import')
  .description('Import OPML subscription file')
  .argument('<file>', 'Path to OPML file')
  .action(async (file) => {
    try {
      const content = readFileSync(file, 'utf-8');
      const outlines = parseOpml(content);
      const feedUrls = extractFeedUrls(outlines);

      if (feedUrls.length === 0) {
        console.log('\nNo feeds found in OPML file.');
        return;
      }

      const config = loadConfig();
      const db = new PodcastDatabase();
      await db.connect();
      await db.initializeSchema();

      let imported = 0;
      const errors: string[] = [];

      for (const { url, title } of feedUrls) {
        try {
          await db.insertFeed(url, title);
          imported++;
          logger.info(`Imported: ${title || url}`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`${url}: ${message}`);
        }
      }

      console.log(`\nImported: ${imported} feed(s)`);
      if (errors.length > 0) {
        console.log(`Errors:   ${errors.length}`);
        for (const err of errors) {
          console.log(`  - ${err}`);
        }
      }

      // Optionally refresh newly imported feeds
      console.log('\nRefreshing imported feeds...');
      const scheduler = new FeedScheduler(db, config);
      const result = await scheduler.refreshAllFeeds();
      console.log(`Refreshed: ${result.refreshed} feed(s)`);
      if (result.errors > 0) {
        console.log(`Refresh errors: ${result.errors}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Export Command
// =========================================================================

program
  .command('export')
  .description('Export subscriptions as OPML')
  .option('-o, --output <file>', 'Output file path (stdout if not specified)')
  .action(async (options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();

      const feeds = await db.listFeeds();
      const opml = generateOpml(feeds);

      if (options.output) {
        const { writeFileSync } = await import('node:fs');
        writeFileSync(options.output, opml, 'utf-8');
        logger.success(`Exported ${feeds.length} feed(s) to ${options.output}`);
      } else {
        console.log(opml);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Download Command
// =========================================================================

program
  .command('download')
  .description('Download episode audio')
  .argument('<episodeId>', 'Episode ID to download')
  .option('-p, --path <path>', 'Download directory')
  .action(async (episodeId, options) => {
    try {
      const config = loadConfig();
      const db = new PodcastDatabase();
      await db.connect();

      const episode = await db.getEpisode(episodeId);
      if (!episode) {
        logger.error('Episode not found');
        await db.disconnect();
        process.exit(1);
      }

      if (!episode.enclosure_url) {
        logger.error('Episode has no audio URL');
        await db.disconnect();
        process.exit(1);
      }

      if (episode.downloaded && episode.download_path) {
        console.log(`Already downloaded: ${episode.download_path}`);
        await db.disconnect();
        return;
      }

      const downloadPath = options.path ?? config.downloadPath;
      logger.info(`Downloading: ${episode.title}`);

      const filePath = await downloadEpisode(episode, downloadPath, db, (progress) => {
        if (progress.totalBytes) {
          const pct = ((progress.bytesDownloaded / progress.totalBytes) * 100).toFixed(1);
          process.stdout.write(`\r  Progress: ${pct}% (${formatBytes(progress.bytesDownloaded)} / ${formatBytes(progress.totalBytes)})`);
        } else {
          process.stdout.write(`\r  Downloaded: ${formatBytes(progress.bytesDownloaded)}`);
        }
      });

      console.log();
      logger.success(`Downloaded to: ${filePath}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Download failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Helpers
// =========================================================================

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${m}:${String(s).padStart(2, '0')}`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

program.parse();
