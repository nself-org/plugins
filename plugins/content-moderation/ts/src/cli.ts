#!/usr/bin/env node
/**
 * Content Moderation Plugin CLI
 * Command-line interface for content moderation operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { ModerationDatabase } from './database.js';

const logger = createLogger('content-moderation:cli');
const program = new Command();

program
  .name('nself-content-moderation')
  .description('Content moderation plugin CLI for nself')
  .version('1.0.0');

/**
 * Initialize database
 */
program
  .command('init')
  .description('Initialize content-moderation database schema')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db);

      await modDb.initSchema();

      logger.success('Content moderation plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize content-moderation plugin', { error: message });
      process.exit(1);
    }
  });

/**
 * Start server
 */
program
  .command('server')
  .description('Start content-moderation HTTP server')
  .action(async () => {
    try {
      logger.info('Starting content-moderation server...');
      await import('./server.js');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start content-moderation server', { error: message });
      process.exit(1);
    }
  });

/**
 * View moderation queue
 */
program
  .command('queue')
  .description('View moderation queue')
  .option('--status <status>', 'Filter by status', 'pending_manual')
  .option('--content-type <type>', 'Filter by content type')
  .option('--limit <limit>', 'Limit results', '20')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db).forSourceAccount(options.appId);

      const result = await modDb.getQueue(
        options.status,
        options.contentType,
        parseInt(options.limit),
      );

      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get queue', { error: message });
      process.exit(1);
    }
  });

/**
 * Make review decision
 */
program
  .command('review')
  .description('Make a manual review decision')
  .argument('<reviewId>', 'Review ID')
  .requiredOption('--action <action>', 'Action: approve, reject, escalate')
  .option('--reason <reason>', 'Reason for decision')
  .option('--reviewer <reviewerId>', 'Reviewer ID', 'cli-user')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (reviewId, options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db).forSourceAccount(options.appId);

      const review = await modDb.getReviewById(reviewId);
      if (!review) {
        logger.error('Review not found', { reviewId });
        process.exit(1);
        return;
      }

      const updated = await modDb.updateReviewDecision(
        reviewId,
        options.action,
        options.reviewer,
        options.reason,
      );

      logger.success('Review decision recorded', {
        reviewId: updated.id,
        action: options.action,
        status: updated.status,
      });
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to review', { error: message });
      process.exit(1);
    }
  });

/**
 * View appeals
 */
program
  .command('appeals')
  .description('View moderation appeals')
  .option('--status <status>', 'Filter by status', 'pending')
  .option('--limit <limit>', 'Limit results', '20')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db).forSourceAccount(options.appId);

      const result = await modDb.getAppeals(options.status, parseInt(options.limit));
      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get appeals', { error: message });
      process.exit(1);
    }
  });

/**
 * Check user moderation status
 */
program
  .command('user-status')
  .description('Check user moderation status')
  .requiredOption('--user <userId>', 'User ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db).forSourceAccount(options.appId);

      const totalStrikes = await modDb.getTotalStrikeCount(options.user);
      const activeStrikes = await modDb.getActiveStrikeCount(options.user);
      const isBanned = activeStrikes >= config.strikeBanThreshold;
      const strikes = await modDb.getUserStrikes(options.user);

      console.log(JSON.stringify({
        userId: options.user,
        totalStrikes,
        activeStrikes,
        isBanned,
        strikes,
      }, null, 2));

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get user status', { error: message });
      process.exit(1);
    }
  });

/**
 * Manage policies
 */
program
  .command('policies')
  .description('List moderation policies')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db).forSourceAccount(options.appId);

      const policies = await modDb.getPolicies();
      console.log(JSON.stringify({ policies, total: policies.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get policies', { error: message });
      process.exit(1);
    }
  });

/**
 * Statistics
 */
program
  .command('stats')
  .description('Show moderation statistics')
  .option('--from <from>', 'Start date (ISO 8601)')
  .option('--to <to>', 'End date (ISO 8601)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const modDb = new ModerationDatabase(db).forSourceAccount(options.appId);

      const stats = await modDb.getStats(options.from, options.to);
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get statistics', { error: message });
      process.exit(1);
    }
  });

program.parse();
