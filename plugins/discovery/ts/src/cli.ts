#!/usr/bin/env node
/**
 * Discovery Plugin CLI
 * Command-line interface for content discovery feed management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { DiscoveryDatabase } from './database.js';
import { DiscoveryCache } from './cache.js';
import { config } from './config.js';

const logger = createLogger('discovery:cli');
const program = new Command();

// Initialize database and cache
const db = new DiscoveryDatabase(config.database_url);
const cache = new DiscoveryCache(db);

// ============================================================================
// Helper Functions
// ============================================================================

function formatDuration(seconds: number | null): string {
  if (seconds === null || seconds === undefined) return 'N/A';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${secs}s`;
  return `${secs}s`;
}

function formatScore(score: number): string {
  return score.toFixed(2);
}

function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

function truncate(str: string | null, maxLen: number): string {
  if (!str) return '';
  return str.length > maxLen ? str.substring(0, maxLen - 3) + '...' : str;
}

async function initializeCache(): Promise<void> {
  try {
    await cache.connect();
  } catch {
    // Cache connection failure is non-fatal
  }
}

// ============================================================================
// init - Initialize plugin and database schema
// ============================================================================

program
  .command('init')
  .description('Initialize discovery plugin and cache tables')
  .action(async () => {
    console.log('Initializing Discovery plugin...\n');

    try {
      await db.initializeSchema();
      console.log('  Database schema initialized\n');

      await initializeCache();
      if (cache.isConnected()) {
        console.log('  Redis cache connected\n');
      } else {
        console.log('  Redis unavailable (operating without cache)\n');
      }

      console.log('Discovery plugin initialized successfully!\n');
      console.log('Next steps:');
      console.log('  1. Ensure media_items, watch_progress, user_ratings tables exist');
      console.log('  2. Start the server: pnpm run dev');
      console.log('  3. Check trending: curl http://localhost:3022/v1/trending\n');
    } catch (error) {
      console.error('Initialization failed:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// trending - Display current trending content
// ============================================================================

program
  .command('trending')
  .description('Display current trending content')
  .option('-l, --limit <number>', 'Number of items to display', '20')
  .option('-w, --window <hours>', 'Trending window in hours', '24')
  .option('-a, --account <id>', 'Source account ID')
  .action(async (options) => {
    try {
      await initializeCache();
      const limit = parseInt(options.limit, 10);
      const windowHours = parseInt(options.window, 10);
      const { items, cached } = await cache.getTrending(limit, windowHours, options.account);

      console.log(`\nTrending Content (${items.length} items, ${windowHours}h window)${cached ? ' [CACHED]' : ''}\n`);

      if (items.length === 0) {
        console.log('  No trending content found for this window.\n');
        return;
      }

      console.log('  #   Score   Views  Rating  Compl.  Title');
      console.log('  ' + '-'.repeat(72));

      items.forEach((item, idx) => {
        const rank = String(idx + 1).padStart(3);
        const score = formatScore(Number(item.trending_score)).padStart(7);
        const views = String(item.view_count).padStart(6);
        const rating = Number(item.avg_rating).toFixed(1).padStart(6);
        const completion = formatPercent(Number(item.completion_rate)).padStart(7);
        const title = truncate(item.title, 35);
        console.log(`  ${rank} ${score} ${views} ${rating} ${completion}  ${title}`);
      });

      console.log('');
    } catch (error) {
      console.error('Error:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// popular - Display popular content
// ============================================================================

program
  .command('popular')
  .description('Display popular content')
  .option('-l, --limit <number>', 'Number of items to display', '20')
  .option('-a, --account <id>', 'Source account ID')
  .action(async (options) => {
    try {
      await initializeCache();
      const limit = parseInt(options.limit, 10);
      const { items, cached } = await cache.getPopular(limit, options.account);

      console.log(`\nPopular Content (${items.length} items)${cached ? ' [CACHED]' : ''}\n`);

      if (items.length === 0) {
        console.log('  No popular content found.\n');
        return;
      }

      console.log('  #   Views  Rating  Title');
      console.log('  ' + '-'.repeat(60));

      items.forEach((item, idx) => {
        const rank = String(idx + 1).padStart(3);
        const views = String(item.view_count).padStart(6);
        const rating = Number(item.avg_rating).toFixed(1).padStart(6);
        const title = truncate(item.title, 40);
        console.log(`  ${rank} ${views} ${rating}  ${title}`);
      });

      console.log('');
    } catch (error) {
      console.error('Error:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// recent - Display recently added content
// ============================================================================

program
  .command('recent')
  .description('Display recently added content')
  .option('-l, --limit <number>', 'Number of items to display', '20')
  .option('-a, --account <id>', 'Source account ID')
  .action(async (options) => {
    try {
      await initializeCache();
      const limit = parseInt(options.limit, 10);
      const { items, cached } = await cache.getRecent(limit, options.account);

      console.log(`\nRecently Added Content (${items.length} items)${cached ? ' [CACHED]' : ''}\n`);

      if (items.length === 0) {
        console.log('  No content found.\n');
        return;
      }

      console.log('  #   Added        Duration  Type      Title');
      console.log('  ' + '-'.repeat(65));

      items.forEach((item, idx) => {
        const rank = String(idx + 1).padStart(3);
        const added = new Date(item.created_at).toISOString().slice(0, 10);
        const duration = formatDuration(item.duration_seconds).padStart(9);
        const type = (item.type || 'unknown').padEnd(9);
        const title = truncate(item.title, 35);
        console.log(`  ${rank}   ${added} ${duration}  ${type} ${title}`);
      });

      console.log('');
    } catch (error) {
      console.error('Error:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// continue - Display continue watching for a user
// ============================================================================

program
  .command('continue <userId>')
  .description('Display continue watching items for a user')
  .option('-l, --limit <number>', 'Number of items to display', '10')
  .option('-a, --account <id>', 'Source account ID')
  .action(async (userId: string, options) => {
    try {
      await initializeCache();
      const limit = parseInt(options.limit, 10);
      const { items, cached } = await cache.getContinueWatching(userId, limit, options.account);

      console.log(`\nContinue Watching for ${userId} (${items.length} items)${cached ? ' [CACHED]' : ''}\n`);

      if (items.length === 0) {
        console.log('  No in-progress content found for this user.\n');
        return;
      }

      console.log('  #   Progress  Last Watched  Title');
      console.log('  ' + '-'.repeat(60));

      items.forEach((item, idx) => {
        const rank = String(idx + 1).padStart(3);
        const progress = `${Number(item.progress_percent).toFixed(0)}%`.padStart(8);
        const lastWatched = new Date(item.last_watched_at).toISOString().slice(0, 16).replace('T', ' ');
        const title = truncate(item.title, 30);
        console.log(`  ${rank} ${progress}  ${lastWatched}  ${title}`);
      });

      console.log('');
    } catch (error) {
      console.error('Error:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// cache-clear - Clear all cached feeds
// ============================================================================

program
  .command('cache-clear')
  .description('Clear all discovery caches (Redis and database)')
  .option('-f, --feed <type>', 'Only clear specific feed (trending, popular, recent, continue)')
  .option('--db-only', 'Only clear database cache tables')
  .option('--redis-only', 'Only clear Redis cache')
  .action(async (options) => {
    try {
      await initializeCache();

      let redisDeleted = 0;
      let dbCleared = false;

      if (!options.dbOnly) {
        if (options.feed === 'trending') {
          redisDeleted = await cache.invalidateTrending();
        } else if (options.feed === 'popular') {
          redisDeleted = await cache.invalidatePopular();
        } else if (options.feed === 'recent') {
          redisDeleted = await cache.invalidateRecent();
        } else {
          redisDeleted = await cache.invalidateAll();
        }
        console.log(`  Redis: ${redisDeleted} keys cleared`);
      }

      if (!options.redisOnly && !options.feed) {
        await db.clearCache();
        dbCleared = true;
        console.log('  Database: cache tables truncated');
      }

      console.log('\nCache cleared successfully.\n');
    } catch (error) {
      console.error('Error:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// status - Show feed and cache status
// ============================================================================

program
  .command('status')
  .description('Show discovery feed and cache status')
  .action(async () => {
    try {
      await initializeCache();

      const dbConnected = await db.isConnected();
      const redisConnected = cache.isConnected();
      const stats = await db.getStatistics();
      const cacheKeys = await cache.getCacheKeyCount();
      const trendingComputed = await db.getTrendingLastComputed();
      const popularComputed = await db.getPopularLastComputed();

      console.log('\nDiscovery Plugin Status\n');

      console.log('Infrastructure:');
      console.log(`  Database:      ${dbConnected ? 'Connected' : 'Disconnected'}`);
      console.log(`  Redis:         ${redisConnected ? 'Connected' : 'Unavailable (degraded mode)'}`);
      console.log(`  Cache Keys:    ${cacheKeys}`);
      console.log('');

      console.log('Source Tables:');
      console.log(`  media_items:     ${stats.total_media_items.toLocaleString()} records`);
      console.log(`  watch_progress:  ${stats.total_watch_progress.toLocaleString()} records`);
      console.log(`  user_ratings:    ${stats.total_user_ratings.toLocaleString()} records`);
      console.log('');

      console.log('Cache Tables:');
      console.log(`  np_disc_trending_cache: ${stats.trending_cache_entries.toLocaleString()} entries`);
      console.log(`    Last computed: ${trendingComputed ? trendingComputed.toISOString() : 'never'}`);
      console.log(`  np_disc_popular_cache:  ${stats.popular_cache_entries.toLocaleString()} entries`);
      console.log(`    Last computed: ${popularComputed ? popularComputed.toISOString() : 'never'}`);
      console.log('');

      console.log('Configuration:');
      console.log(`  Port:                ${config.port}`);
      console.log(`  Trending Window:     ${config.trending_window_hours}h`);
      console.log(`  Default Limit:       ${config.default_limit}`);
      console.log(`  Cache TTL Trending:  ${config.cache_ttl_trending}s (${Math.round(config.cache_ttl_trending / 60)}min)`);
      console.log(`  Cache TTL Popular:   ${config.cache_ttl_popular}s (${Math.round(config.cache_ttl_popular / 60)}min)`);
      console.log(`  Cache TTL Recent:    ${config.cache_ttl_recent}s (${Math.round(config.cache_ttl_recent / 60)}min)`);
      console.log(`  Cache TTL Continue:  ${config.cache_ttl_continue}s (${Math.round(config.cache_ttl_continue / 60)}min)`);
      console.log('');
    } catch (error) {
      console.error('Error:', error instanceof Error ? error.message : error);
      process.exit(1);
    } finally {
      await cache.close();
      await db.close();
    }
  });

// ============================================================================
// Program Configuration
// ============================================================================

program
  .name('discovery')
  .description('Discovery Plugin for nself - Content discovery feeds')
  .version('1.0.0');

// Parse arguments
program.parse();
