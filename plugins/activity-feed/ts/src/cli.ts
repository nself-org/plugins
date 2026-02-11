#!/usr/bin/env node
/**
 * Activity Feed Plugin CLI
 * Command-line interface for the Activity Feed plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { FeedDatabase } from './database.js';
import { createServer } from './server.js';
import type { CreateActivityInput, CreateSubscriptionInput } from './types.js';

const logger = createLogger('feed:cli');

const program = new Command();

program
  .name('nself-activity-feed')
  .description('Activity Feed plugin for nself - universal activity feed system')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      logger.info('Initializing activity feed database schema...');

      const db = new FeedDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.success('Schema initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the activity feed server')
  .option('-p, --port <port>', 'Server port', '3503')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting activity feed server on ${config.host}:${config.port}`);
      logger.info(`Strategy: ${config.strategy}`);

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
  .description('Show activity feed statistics')
  .action(async () => {
    try {
      loadConfig();
      logger.info('Fetching activity feed statistics...');

      const db = new FeedDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nActivity Feed Statistics:');
      console.log('=========================');
      console.log(`Total Activities:        ${stats.totalActivities}`);
      console.log(`Total Subscriptions:     ${stats.totalSubscriptions}`);
      console.log(`Total Feed Items:        ${stats.totalFeedItems}`);
      console.log(`Unread Feed Items:       ${stats.unreadFeedItems}`);
      console.log(`Recent (24h):            ${stats.recentActivityCount24h}`);
      console.log(`Recent (7d):             ${stats.recentActivityCount7d}`);

      if (stats.lastActivityAt) {
        console.log(`Last Activity:           ${stats.lastActivityAt.toISOString()}`);
      }

      console.log('\nActivities by Verb:');
      Object.entries(stats.activitiesByVerb).forEach(([verb, count]) => {
        console.log(`  ${verb.padEnd(20)} ${count}`);
      });

      console.log('\nActivities by Actor Type:');
      Object.entries(stats.activitiesByActorType).forEach(([type, count]) => {
        console.log(`  ${type.padEnd(20)} ${count}`);
      });

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Activities command
program
  .command('activities')
  .description('List recent activities')
  .option('-l, --limit <limit>', 'Number of activities to show', '20')
  .option('-a, --actor <actorId>', 'Filter by actor ID')
  .option('-v, --verb <verb>', 'Filter by verb')
  .option('-o, --object <type:id>', 'Filter by object (format: type:id)')
  .action(async (options) => {
    try {
      loadConfig();
      logger.info('Fetching activities...');

      const db = new FeedDatabase();
      await db.connect();

      let objectType: string | undefined;
      let objectId: string | undefined;

      if (options.object) {
        const parts = options.object.split(':');
        if (parts.length === 2) {
          objectType = parts[0];
          objectId = parts[1];
        }
      }

      const activities = await db.listActivities({
        limit: parseInt(options.limit, 10),
        actorId: options.actor,
        verb: options.verb,
        objectType,
        objectId,
      });

      console.log('\nRecent Activities:');
      console.log('==================\n');

      if (activities.length === 0) {
        console.log('No activities found');
      } else {
        activities.forEach(activity => {
          console.log(`[${activity.created_at.toISOString()}]`);
          console.log(`  ID:     ${activity.id}`);
          console.log(`  Actor:  ${activity.actor_id} (${activity.actor_type})`);
          console.log(`  Action: ${activity.verb} ${activity.object_type}:${activity.object_id}`);
          if (activity.target_type && activity.target_id) {
            console.log(`  Target: ${activity.target_type}:${activity.target_id}`);
          }
          if (activity.message) {
            console.log(`  Message: ${activity.message}`);
          }
          if (activity.source_plugin) {
            console.log(`  Source: ${activity.source_plugin}`);
          }
          console.log('');
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list activities', { error: message });
      process.exit(1);
    }
  });

// Feed command
program
  .command('feed <userId>')
  .description('View a user\'s activity feed')
  .option('-l, --limit <limit>', 'Number of items to show', '20')
  .option('--unread-only', 'Show only unread items')
  .action(async (userId: string, options) => {
    try {
      loadConfig();
      logger.info(`Fetching feed for user: ${userId}`);

      const db = new FeedDatabase();
      await db.connect();

      const feedItems = await db.getUserFeed({
        userId,
        limit: parseInt(options.limit, 10),
        includeRead: !options.unreadOnly,
        includeHidden: false,
      });

      const unreadCount = await db.getUnreadCount(userId);

      console.log(`\nActivity Feed for ${userId}:`);
      console.log('=============================');
      console.log(`Unread: ${unreadCount}\n`);

      if (feedItems.length === 0) {
        console.log('No feed items found');
      } else {
        feedItems.forEach(item => {
          const activity = item.activity;
          const readStatus = item.is_read ? '✓' : '○';
          console.log(`${readStatus} [${item.created_at.toISOString()}]`);
          console.log(`  ${activity.actor_id} ${activity.verb} ${activity.object_type}:${activity.object_id}`);
          if (activity.message) {
            console.log(`  ${activity.message}`);
          }
          console.log('');
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get user feed', { error: message });
      process.exit(1);
    }
  });

// Subscriptions command
program
  .command('subscriptions <userId>')
  .description('List user\'s subscriptions')
  .action(async (userId: string) => {
    try {
      loadConfig();
      logger.info(`Fetching subscriptions for user: ${userId}`);

      const db = new FeedDatabase();
      await db.connect();

      const subscriptions = await db.listUserSubscriptions(userId);

      console.log(`\nSubscriptions for ${userId}:`);
      console.log('============================\n');

      if (subscriptions.length === 0) {
        console.log('No subscriptions found');
      } else {
        subscriptions.forEach(sub => {
          const status = sub.enabled ? '✓' : '✗';
          console.log(`${status} ${sub.target_type}:${sub.target_id}`);
          console.log(`  ID: ${sub.id}`);
          console.log(`  Created: ${sub.created_at.toISOString()}`);
          console.log('');
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list subscriptions', { error: message });
      process.exit(1);
    }
  });

// Subscribe command
program
  .command('subscribe <userId> <targetType> <targetId>')
  .description('Subscribe a user to a target')
  .action(async (userId: string, targetType: string, targetId: string) => {
    try {
      loadConfig();
      logger.info(`Subscribing ${userId} to ${targetType}:${targetId}`);

      const db = new FeedDatabase();
      await db.connect();

      const subscription = await db.createSubscription({
        user_id: userId,
        target_type: targetType as CreateSubscriptionInput['target_type'],
        target_id: targetId,
        enabled: true,
      });

      console.log('\nSubscription created:');
      console.log(`  ID: ${subscription.id}`);
      console.log(`  User: ${subscription.user_id}`);
      console.log(`  Target: ${subscription.target_type}:${subscription.target_id}`);
      console.log(`  Enabled: ${subscription.enabled}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create subscription', { error: message });
      process.exit(1);
    }
  });

// Fan-out command
program
  .command('fanout <activityId>')
  .description('Manually trigger fan-out for an activity')
  .option('-f, --force', 'Force refresh existing feed items')
  .action(async (activityId: string) => {
    try {
      loadConfig();
      logger.info(`Triggering fan-out for activity: ${activityId}`);

      const db = new FeedDatabase();
      await db.connect();

      const startTime = Date.now();

      // Get activity
      const activity = await db.getActivity(activityId);
      if (!activity) {
        logger.error('Activity not found', { activityId });
        await db.disconnect();
        process.exit(1);
      }

      // Get subscribers
      const subscribers = await db.getSubscribersForActor(activity.actor_id);

      // Create feed items
      let feedItemsCreated = 0;
      for (const userId of subscribers) {
        await db.createFeedItem(userId, activityId);
        feedItemsCreated++;
      }

      const duration = Date.now() - startTime;

      console.log('\nFan-out completed:');
      console.log(`  Activity ID: ${activityId}`);
      console.log(`  Subscribers: ${subscribers.length}`);
      console.log(`  Feed items created: ${feedItemsCreated}`);
      console.log(`  Duration: ${duration}ms`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Fan-out failed', { error: message });
      process.exit(1);
    }
  });

// Create activity command
program
  .command('create-activity')
  .description('Create a new activity')
  .requiredOption('--actor <actorId>', 'Actor ID')
  .requiredOption('--verb <verb>', 'Activity verb (created, updated, liked, etc.)')
  .requiredOption('--object <type:id>', 'Object (format: type:id)')
  .option('--target <type:id>', 'Target (format: type:id)')
  .option('--message <message>', 'Activity message')
  .option('--plugin <plugin>', 'Source plugin name')
  .action(async (options) => {
    try {
      loadConfig();

      const objectParts = options.object.split(':');
      if (objectParts.length !== 2) {
        logger.error('Invalid object format. Use type:id');
        process.exit(1);
      }

      let targetType: string | undefined;
      let targetId: string | undefined;
      if (options.target) {
        const targetParts = options.target.split(':');
        if (targetParts.length === 2) {
          targetType = targetParts[0];
          targetId = targetParts[1];
        }
      }

      const input: CreateActivityInput = {
        actor_id: options.actor,
        verb: options.verb,
        object_type: objectParts[0],
        object_id: objectParts[1],
        target_type: targetType,
        target_id: targetId,
        message: options.message,
        source_plugin: options.plugin,
      };

      logger.info('Creating activity...', input as Record<string, unknown>);

      const db = new FeedDatabase();
      await db.connect();

      const activity = await db.createActivity(input);

      console.log('\nActivity created:');
      console.log(`  ID: ${activity.id}`);
      console.log(`  Actor: ${activity.actor_id}`);
      console.log(`  Action: ${activity.verb} ${activity.object_type}:${activity.object_id}`);
      if (activity.target_type && activity.target_id) {
        console.log(`  Target: ${activity.target_type}:${activity.target_id}`);
      }
      console.log(`  Created: ${activity.created_at.toISOString()}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create activity', { error: message });
      process.exit(1);
    }
  });

// Cleanup command
program
  .command('cleanup')
  .description('Clean up old activities based on retention policy')
  .option('-d, --days <days>', 'Retention days (default: from config)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const retentionDays = options.days ? parseInt(options.days, 10) : config.retentionDays;

      logger.info(`Cleaning up activities older than ${retentionDays} days...`);

      const db = new FeedDatabase();
      await db.connect();

      const deleted = await db.cleanupOldActivities(retentionDays);

      console.log(`\nCleanup completed: ${deleted} activities deleted`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cleanup failed', { error: message });
      process.exit(1);
    }
  });

// Stats command (alias for status)
program
  .command('stats')
  .description('Show activity feed statistics')
  .action(async () => {
    try {
      loadConfig();
      logger.info('Fetching activity feed statistics...');

      const db = new FeedDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nActivity Feed Statistics:');
      console.log('=========================');
      console.log(`Total Activities:        ${stats.totalActivities}`);
      console.log(`Total Subscriptions:     ${stats.totalSubscriptions}`);
      console.log(`Total Feed Items:        ${stats.totalFeedItems}`);
      console.log(`Unread Feed Items:       ${stats.unreadFeedItems}`);
      console.log(`Recent (24h):            ${stats.recentActivityCount24h}`);
      console.log(`Recent (7d):             ${stats.recentActivityCount7d}`);

      if (stats.lastActivityAt) {
        console.log(`Last Activity:           ${stats.lastActivityAt.toISOString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
