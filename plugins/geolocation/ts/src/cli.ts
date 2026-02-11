#!/usr/bin/env node
/**
 * Geolocation Plugin CLI
 * Command-line interface for geolocation operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { GeolocationDatabase } from './database.js';

const logger = createLogger('geolocation:cli');
const program = new Command();

program
  .name('nself-geolocation')
  .description('Geolocation plugin CLI for nself')
  .version('1.0.0');

/**
 * Initialize database
 */
program
  .command('init')
  .description('Initialize geolocation database schema')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db);

      await geoDb.initSchema();

      logger.success('Geolocation plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize geolocation plugin', { error: message });
      process.exit(1);
    }
  });

/**
 * Start server
 */
program
  .command('server')
  .description('Start geolocation HTTP server')
  .action(async () => {
    try {
      logger.info('Starting geolocation server...');
      await import('./server.js');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start geolocation server', { error: message });
      process.exit(1);
    }
  });

/**
 * Locate user
 */
program
  .command('locate')
  .description('Get current location for a user')
  .requiredOption('--user <userId>', 'User ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db).forSourceAccount(options.appId);

      const location = await geoDb.getLatestByUserId(options.user);

      if (!location) {
        logger.warn('No location found for user', { userId: options.user });
      } else {
        console.log(JSON.stringify(location, null, 2));
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get location', { error: message });
      process.exit(1);
    }
  });

/**
 * Location history
 */
program
  .command('history')
  .description('View location history for a user')
  .requiredOption('--user <userId>', 'User ID')
  .option('--from <from>', 'Start date (ISO 8601)')
  .option('--to <to>', 'End date (ISO 8601)')
  .option('--limit <limit>', 'Limit results', '100')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db).forSourceAccount(options.appId);

      const result = await geoDb.getHistory(
        options.user,
        options.from,
        options.to,
        parseInt(options.limit),
      );

      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get history', { error: message });
      process.exit(1);
    }
  });

/**
 * Manage geofences
 */
program
  .command('fences')
  .description('List geofences')
  .option('--owner <ownerId>', 'Filter by owner ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db).forSourceAccount(options.appId);

      const fences = await geoDb.getFences(options.owner);
      console.log(JSON.stringify({ fences, total: fences.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list fences', { error: message });
      process.exit(1);
    }
  });

/**
 * Create geofence
 */
program
  .command('fence-create')
  .description('Create a new geofence')
  .requiredOption('--name <name>', 'Geofence name')
  .requiredOption('--lat <latitude>', 'Center latitude')
  .requiredOption('--lng <longitude>', 'Center longitude')
  .requiredOption('--radius <radius>', 'Radius in meters')
  .option('--owner <ownerId>', 'Owner ID', 'system')
  .option('--trigger <trigger>', 'Trigger on: enter, exit, both', 'both')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db).forSourceAccount(options.appId);

      const fence = await geoDb.createFence({
        ownerId: options.owner,
        name: options.name,
        latitude: parseFloat(options.lat),
        longitude: parseFloat(options.lng),
        radiusMeters: parseFloat(options.radius),
        triggerOn: options.trigger,
      });

      logger.success('Geofence created', { id: fence.id, name: fence.name });
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create geofence', { error: message });
      process.exit(1);
    }
  });

/**
 * Find nearby users
 */
program
  .command('nearby')
  .description('Find nearby users')
  .requiredOption('--lat <latitude>', 'Center latitude')
  .requiredOption('--lng <longitude>', 'Center longitude')
  .requiredOption('--radius <radius>', 'Radius in meters')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db).forSourceAccount(options.appId);

      const nearby = await geoDb.findNearby(
        parseFloat(options.lat),
        parseFloat(options.lng),
        parseFloat(options.radius),
      );

      console.log(JSON.stringify({ nearby, total: nearby.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to find nearby users', { error: message });
      process.exit(1);
    }
  });

/**
 * Statistics
 */
program
  .command('stats')
  .description('Show geolocation statistics')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const geoDb = new GeolocationDatabase(db).forSourceAccount(options.appId);

      const stats = await geoDb.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get statistics', { error: message });
      process.exit(1);
    }
  });

program.parse();
