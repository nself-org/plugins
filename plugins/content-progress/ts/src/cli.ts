#!/usr/bin/env node
/**
 * Content Progress Plugin CLI
 * Command-line interface for the Content Progress plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ProgressDatabase } from './database.js';
import { createServer } from './server.js';
import type { ContentType } from './types.js';

const logger = createLogger('progress:cli');

const program = new Command();

program
  .name('nself-content-progress')
  .description('Content Progress plugin for nself - track playback progress, watchlists, and favorites')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const config = loadConfig();

      const db = new ProgressDatabase(undefined, 'primary', config.completeThreshold, config.historySampleSeconds);
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

// Server command
program
  .command('server')
  .description('Start the API server')
  .option('-p, --port <port>', 'Server port', '3022')
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

// Status command
program
  .command('status')
  .description('Show plugin status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();

      const db = new ProgressDatabase(undefined, 'primary', config.completeThreshold, config.historySampleSeconds);
      await db.connect();

      const stats = await db.getPluginStats();

      console.log('\nContent Progress Plugin Status');
      console.log('==============================');
      console.log(`Complete threshold: ${config.completeThreshold}%`);
      console.log(`History sampling:   ${config.historySampleSeconds}s`);
      console.log('\nPlugin Statistics:');
      console.log(`  Total users:          ${stats.total_users}`);
      console.log(`  Total positions:      ${stats.total_positions}`);
      console.log(`  Completed:            ${stats.total_completed}`);
      console.log(`  In progress:          ${stats.total_in_progress}`);
      console.log(`  Watchlist items:      ${stats.total_watchlist}`);
      console.log(`  Favorite items:       ${stats.total_favorites}`);
      console.log(`  History events:       ${stats.total_history_events}`);
      if (stats.last_activity) {
        console.log(`  Last activity:        ${stats.last_activity.toISOString()}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Progress command
program
  .command('progress')
  .description('Manage playback progress')
  .argument('[action]', 'Action: list, show, update, delete, complete', 'list')
  .argument('[userId]', 'User ID')
  .argument('[contentType]', 'Content type')
  .argument('[contentId]', 'Content ID')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-p, --position <seconds>', 'Position in seconds')
  .option('-d, --duration <seconds>', 'Duration in seconds')
  .action(async (action, userId, contentType, contentId, options) => {
    try {
      const config = loadConfig();
      const db = new ProgressDatabase(undefined, 'primary', config.completeThreshold, config.historySampleSeconds);
      await db.connect();

      switch (action) {
        case 'list': {
          if (!userId) {
            logger.error('User ID required');
            process.exit(1);
          }
          const positions = await db.getUserProgress(userId, parseInt(options.limit, 10));
          console.log('\nProgress Positions:');
          console.log('-'.repeat(120));
          positions.forEach(p => {
            console.log(
              `${p.content_type.padEnd(10)} | ${p.content_id.padEnd(30)} | ` +
              `${p.position_seconds.toFixed(0).padStart(6)}s | ` +
              `${p.progress_percent.toFixed(1).padStart(5)}% | ` +
              `${p.completed ? 'COMPLETED' : 'IN PROGRESS'}`
            );
          });
          console.log(`\nTotal: ${positions.length}`);
          break;
        }

        case 'show': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const position = await db.getProgress(userId, contentType as ContentType, contentId);
          if (!position) {
            logger.error('Progress not found');
            process.exit(1);
          }
          console.log(JSON.stringify(position, null, 2));
          break;
        }

        case 'update': {
          if (!userId || !contentType || !contentId || !options.position) {
            logger.error('User ID, content type, content ID, and position required');
            process.exit(1);
          }
          const position = await db.updateProgress({
            user_id: userId,
            content_type: contentType as ContentType,
            content_id: contentId,
            position_seconds: parseFloat(options.position),
            duration_seconds: options.duration ? parseFloat(options.duration) : undefined,
          });
          logger.success('Progress updated');
          console.log(JSON.stringify(position, null, 2));
          break;
        }

        case 'delete': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const deleted = await db.deleteProgress(userId, contentType as ContentType, contentId);
          if (!deleted) {
            logger.error('Progress not found');
            process.exit(1);
          }
          logger.success('Progress deleted');
          break;
        }

        case 'complete': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const position = await db.markCompleted(userId, contentType as ContentType, contentId);
          if (!position) {
            logger.error('Progress not found');
            process.exit(1);
          }
          logger.success('Marked as completed');
          console.log(JSON.stringify(position, null, 2));
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Watchlist command
program
  .command('watchlist')
  .description('Manage watchlist')
  .argument('[action]', 'Action: list, add, remove', 'list')
  .argument('[userId]', 'User ID')
  .argument('[contentType]', 'Content type')
  .argument('[contentId]', 'Content ID')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('--priority <priority>', 'Priority (0-100)', '0')
  .option('--notes <notes>', 'Notes')
  .action(async (action, userId, contentType, contentId, options) => {
    try {
      const config = loadConfig();
      const db = new ProgressDatabase(undefined, 'primary', config.completeThreshold, config.historySampleSeconds);
      await db.connect();

      switch (action) {
        case 'list': {
          if (!userId) {
            logger.error('User ID required');
            process.exit(1);
          }
          const items = await db.getWatchlist(userId, parseInt(options.limit, 10));
          console.log('\nWatchlist:');
          console.log('-'.repeat(100));
          items.forEach(item => {
            console.log(
              `${item.content_type.padEnd(10)} | ${item.content_id.padEnd(30)} | ` +
              `Priority: ${item.priority.toString().padStart(3)} | ` +
              `${item.notes ?? 'No notes'}`
            );
          });
          console.log(`\nTotal: ${items.length}`);
          break;
        }

        case 'add': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const item = await db.addToWatchlist({
            user_id: userId,
            content_type: contentType as ContentType,
            content_id: contentId,
            priority: parseInt(options.priority, 10),
            notes: options.notes,
          });
          logger.success('Added to watchlist');
          console.log(JSON.stringify(item, null, 2));
          break;
        }

        case 'remove': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const deleted = await db.removeFromWatchlist(userId, contentType as ContentType, contentId);
          if (!deleted) {
            logger.error('Watchlist item not found');
            process.exit(1);
          }
          logger.success('Removed from watchlist');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Favorites command
program
  .command('favorites')
  .description('Manage favorites')
  .argument('[action]', 'Action: list, add, remove', 'list')
  .argument('[userId]', 'User ID')
  .argument('[contentType]', 'Content type')
  .argument('[contentId]', 'Content ID')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, userId, contentType, contentId, options) => {
    try {
      const config = loadConfig();
      const db = new ProgressDatabase(undefined, 'primary', config.completeThreshold, config.historySampleSeconds);
      await db.connect();

      switch (action) {
        case 'list': {
          if (!userId) {
            logger.error('User ID required');
            process.exit(1);
          }
          const items = await db.getFavorites(userId, parseInt(options.limit, 10));
          console.log('\nFavorites:');
          console.log('-'.repeat(80));
          items.forEach(item => {
            console.log(
              `${item.content_type.padEnd(10)} | ${item.content_id.padEnd(30)} | ` +
              `${item.created_at.toISOString()}`
            );
          });
          console.log(`\nTotal: ${items.length}`);
          break;
        }

        case 'add': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const item = await db.addToFavorites({
            user_id: userId,
            content_type: contentType as ContentType,
            content_id: contentId,
          });
          logger.success('Added to favorites');
          console.log(JSON.stringify(item, null, 2));
          break;
        }

        case 'remove': {
          if (!userId || !contentType || !contentId) {
            logger.error('User ID, content type, and content ID required');
            process.exit(1);
          }
          const deleted = await db.removeFromFavorites(userId, contentType as ContentType, contentId);
          if (!deleted) {
            logger.error('Favorite not found');
            process.exit(1);
          }
          logger.success('Removed from favorites');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('View user statistics')
  .argument('<userId>', 'User ID')
  .action(async (userId) => {
    try {
      const config = loadConfig();
      const db = new ProgressDatabase(undefined, 'primary', config.completeThreshold, config.historySampleSeconds);
      await db.connect();

      const stats = await db.getUserStats(userId);

      console.log(`\nUser Statistics: ${userId}`);
      console.log('='.repeat(50));
      console.log(`Total watch time:     ${stats.total_watch_time_hours.toFixed(2)} hours`);
      console.log(`                      (${stats.total_watch_time_seconds.toFixed(0)} seconds)`);
      console.log(`Content completed:    ${stats.content_completed}`);
      console.log(`Content in progress:  ${stats.content_in_progress}`);
      console.log(`Watchlist count:      ${stats.watchlist_count}`);
      console.log(`Favorites count:      ${stats.favorites_count}`);
      if (stats.most_watched_type) {
        console.log(`Most watched type:    ${stats.most_watched_type}`);
      }
      if (stats.recent_activity) {
        console.log(`Recent activity:      ${stats.recent_activity.toISOString()}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
