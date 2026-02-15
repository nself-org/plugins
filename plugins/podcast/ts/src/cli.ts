#!/usr/bin/env node
/**
 * Podcast Plugin CLI
 * Command-line interface for the Podcast plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { PodcastDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('podcast:cli');

const program = new Command();

program
  .name('nself-podcast')
  .description('Podcast plugin for nself - RSS feed parsing, episode management, and subscription management')
  .version('1.0.0');

// Init command
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

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for Podcast plugin');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the Podcast API server')
  .option('-p, --port <port>', 'Server port', '3210')
  .option('-H, --host <host>', 'Server host', '127.0.0.1')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting Podcast server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .alias('status')
  .description('Show podcast statistics')
  .action(async () => {
    try {
      const db = new PodcastDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nPodcast Statistics:');
      console.log('==================');
      console.log(`Total Podcasts:       ${stats.total_podcasts}`);
      console.log(`Active Podcasts:      ${stats.active_podcasts}`);
      console.log(`Total Episodes:       ${stats.total_episodes}`);
      console.log(`Total Subscriptions:  ${stats.total_subscriptions}`);
      console.log(`Total Categories:     ${stats.total_categories}`);
      if (stats.oldest_episode) {
        console.log(`Oldest Episode:       ${stats.oldest_episode.toISOString()}`);
      }
      if (stats.newest_episode) {
        console.log(`Newest Episode:       ${stats.newest_episode.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Feeds command
program
  .command('feeds')
  .description('List podcast feeds')
  .option('-s, --status <status>', 'Filter by feed status (active, error, inactive)')
  .option('-c, --category <category>', 'Filter by category')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();

      const podcasts = await db.listPodcasts({
        feedStatus: options.status,
        category: options.category,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (podcasts.length === 0) {
        console.log('No podcast feeds found');
        process.exit(0);
      }

      console.log(`\nFound ${podcasts.length} podcast feed(s):\n`);
      for (const podcast of podcasts) {
        const status = podcast.feed_status !== 'active' ? ` [${podcast.feed_status}]` : '';
        const episodes = podcast.episode_count > 0 ? ` (${podcast.episode_count} episodes)` : '';

        console.log(`  ${podcast.title}${status}${episodes}`);
        console.log(`    ID:       ${podcast.id}`);
        console.log(`    Feed:     ${podcast.feed_url}`);
        if (podcast.author) console.log(`    Author:   ${podcast.author}`);
        if (podcast.categories.length > 0) console.log(`    Category: ${podcast.categories.join(', ')}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list feeds', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Subscribe command
program
  .command('subscribe <feed_url>')
  .description('Subscribe to a podcast by feed URL')
  .option('--user-id <userId>', 'User ID for subscription', 'cli-user')
  .action(async (feedUrl: string, options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();

      // Check if podcast already exists
      let podcast = await db.getPodcastByFeedUrl(feedUrl);
      if (!podcast) {
        // Create podcast from feed URL
        podcast = await db.createPodcast({
          source_account_id: 'primary',
          title: 'Pending Sync',
          description: null,
          author: null,
          feed_url: feedUrl,
          website_url: null,
          image_url: null,
          language: 'en',
          categories: [],
          explicit: false,
          last_fetched_at: null,
          last_published_at: null,
          etag: null,
          last_modified: null,
          feed_status: 'active',
          episode_count: 0,
          metadata: {},
        });
        console.log(`Added new podcast feed: ${feedUrl}`);
      }

      // Create subscription
      await db.createSubscription({
        source_account_id: 'primary',
        user_id: options.userId,
        podcast_id: podcast.id,
        is_active: true,
        notification_enabled: true,
        auto_download: false,
        metadata: {},
      });

      await db.disconnect();

      console.log(`Subscribed to: ${podcast.title}`);
      console.log(`Podcast ID: ${podcast.id}`);
      console.log('Run "nself-podcast sync" to fetch episodes');

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Subscribe failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Episodes command
program
  .command('episodes')
  .description('List episodes')
  .requiredOption('--podcast <uuid>', 'Podcast ID')
  .option('-s, --season <season>', 'Filter by season number')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();

      const podcast = await db.getPodcast(options.podcast);
      if (!podcast) {
        console.error('Podcast not found');
        process.exit(1);
      }

      const episodes = await db.listEpisodes({
        podcastId: options.podcast,
        seasonNumber: options.season ? parseInt(options.season, 10) : undefined,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (episodes.length === 0) {
        console.log(`No episodes found for ${podcast.title}`);
        process.exit(0);
      }

      console.log(`\nEpisodes for ${podcast.title} (${episodes.length} shown):\n`);
      for (const ep of episodes) {
        const date = ep.published_at
          ? new Date(ep.published_at).toLocaleDateString('en-US')
          : 'Unknown date';
        const duration = ep.duration_seconds
          ? formatDuration(ep.duration_seconds)
          : '';
        const seasonEp = formatSeasonEpisode(ep.season_number, ep.episode_number);

        console.log(`  ${date}  ${seasonEp}${ep.title}${duration ? ` (${duration})` : ''}`);
        console.log(`    ID: ${ep.id}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list episodes', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Sync command
program
  .command('sync')
  .description('Sync RSS feeds')
  .option('--podcast <uuid>', 'Sync specific podcast only')
  .option('--force', 'Force sync even if recently fetched')
  .action(async (options) => {
    try {
      const config = loadConfig();
      console.log('Triggering podcast sync...');

      if (options.podcast) {
        console.log(`Syncing podcast: ${options.podcast}`);
      } else {
        console.log('Syncing all active podcasts');
      }

      console.log(`Server should be running on port ${config.port}`);
      console.log('Use POST /api/sync endpoint for server-based sync');

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync trigger failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Search command
program
  .command('search <query>')
  .description('Search podcasts')
  .option('-c, --category <category>', 'Filter by category')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (query: string, options) => {
    try {
      const db = new PodcastDatabase();
      await db.connect();

      const podcasts = await db.searchPodcasts({
        query,
        category: options.category,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (podcasts.length === 0) {
        console.log(`No podcasts found for "${query}"`);
        process.exit(0);
      }

      console.log(`\nFound ${podcasts.length} podcast(s) for "${query}":\n`);
      for (const podcast of podcasts) {
        console.log(`  ${podcast.title}`);
        console.log(`    ID:       ${podcast.id}`);
        if (podcast.author) console.log(`    Author:   ${podcast.author}`);
        if (podcast.categories.length > 0) console.log(`    Category: ${podcast.categories.join(', ')}`);
        console.log(`    Episodes: ${podcast.episode_count}`);
        if (podcast.description) {
          const desc = podcast.description.length > 100
            ? podcast.description.substring(0, 100) + '...'
            : podcast.description;
          console.log(`    Desc:     ${desc}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();

// =========================================================================
// CLI Utility Functions
// =========================================================================

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

function formatSeasonEpisode(season: number | null, episode: number | null): string {
  if (season !== null && episode !== null) {
    return `S${String(season).padStart(2, '0')}E${String(episode).padStart(2, '0')} `;
  }
  if (episode !== null) {
    return `E${String(episode).padStart(2, '0')} `;
  }
  return '';
}
