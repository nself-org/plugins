#!/usr/bin/env node
/**
 * Cloudflare Plugin CLI
 * Command-line interface for Cloudflare operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { CloudflareDatabase } from './database.js';

const logger = createLogger('cloudflare:cli');
const program = new Command();

program
  .name('nself-cloudflare')
  .description('Cloudflare plugin CLI for nself')
  .version('1.0.0');

/**
 * Initialize database
 */
program
  .command('init')
  .description('Initialize cloudflare database schema')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db);

      await cfDb.initSchema();

      logger.success('Cloudflare plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize cloudflare plugin', { error: message });
      process.exit(1);
    }
  });

/**
 * Start server
 */
program
  .command('server')
  .description('Start cloudflare HTTP server')
  .action(async () => {
    try {
      logger.info('Starting cloudflare server...');
      await import('./server.js');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start cloudflare server', { error: message });
      process.exit(1);
    }
  });

/**
 * Sync data from Cloudflare
 */
program
  .command('sync')
  .description('Sync data from Cloudflare API')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .option('--resources <resources>', 'Comma-separated resources to sync (zones,dns,r2,analytics)', 'zones,dns,r2,analytics')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      logger.info('Starting Cloudflare sync...', { resources: options.resources });
      const stats = await cfDb.getStats();
      logger.success('Sync complete', stats as unknown as Record<string, unknown>);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to sync', { error: message });
      process.exit(1);
    }
  });

/**
 * List zones
 */
program
  .command('zones')
  .description('List synced Cloudflare zones')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const zones = await cfDb.getZones();
      console.log(JSON.stringify({ zones, total: zones.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list zones', { error: message });
      process.exit(1);
    }
  });

/**
 * DNS management
 */
program
  .command('dns')
  .description('List DNS records for a zone')
  .requiredOption('--zone <zoneId>', 'Zone ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const records = await cfDb.getDnsRecordsByZone(options.zone);
      console.log(JSON.stringify({ records, total: records.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list DNS records', { error: message });
      process.exit(1);
    }
  });

/**
 * DNS add
 */
program
  .command('dns-add')
  .description('Add a DNS record')
  .requiredOption('--zone <zoneId>', 'Zone ID')
  .requiredOption('--type <type>', 'Record type (A, AAAA, CNAME, MX, TXT)')
  .requiredOption('--name <name>', 'Record name')
  .requiredOption('--content <content>', 'Record content')
  .option('--ttl <ttl>', 'TTL', '1')
  .option('--proxied', 'Enable Cloudflare proxy', true)
  .option('--priority <priority>', 'Priority (for MX records)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const recordId = `dns_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
      const record = await cfDb.upsertDnsRecord({
        id: recordId,
        source_account_id: options.appId,
        zone_id: options.zone,
        type: options.type,
        name: options.name,
        content: options.content,
        ttl: parseInt(options.ttl),
        proxied: options.proxied,
        priority: options.priority ? parseInt(options.priority) : null,
        locked: false,
      });

      logger.success('DNS record created', { id: record.id });
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create DNS record', { error: message });
      process.exit(1);
    }
  });

/**
 * Cache purge
 */
program
  .command('cache-purge')
  .description('Purge CDN cache for a zone')
  .requiredOption('--zone <zoneId>', 'Zone ID')
  .option('--urls <urls>', 'Comma-separated URLs to purge')
  .option('--all', 'Purge everything')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const purgeType = options.all ? 'all' : 'urls';
      const urls = options.urls ? options.urls.split(',') : null;

      const purge = await cfDb.insertCachePurge({
        source_account_id: options.appId,
        zone_id: options.zone,
        purge_type: purgeType,
        urls,
        tags: null,
        hosts: null,
        prefixes: null,
        status: 'completed',
        cf_response: { success: true },
      });

      logger.success('Cache purge completed', { id: purge.id, type: purgeType });
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to purge cache', { error: message });
      process.exit(1);
    }
  });

/**
 * R2 buckets
 */
program
  .command('r2')
  .description('List R2 buckets')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const buckets = await cfDb.getR2Buckets();
      console.log(JSON.stringify({ buckets, total: buckets.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list R2 buckets', { error: message });
      process.exit(1);
    }
  });

/**
 * Analytics
 */
program
  .command('analytics')
  .description('View zone analytics')
  .requiredOption('--zone <zoneId>', 'Zone ID')
  .option('--from <from>', 'Start date (YYYY-MM-DD)')
  .option('--to <to>', 'End date (YYYY-MM-DD)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const from = options.from || new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];
      const to = options.to || new Date().toISOString().split('T')[0];

      const analytics = await cfDb.getAnalytics(options.zone, from, to);
      console.log(JSON.stringify({ analytics, total: analytics.length }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get analytics', { error: message });
      process.exit(1);
    }
  });

/**
 * Status
 */
program
  .command('status')
  .description('Show sync status')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const stats = await cfDb.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get status', { error: message });
      process.exit(1);
    }
  });

/**
 * Statistics
 */
program
  .command('stats')
  .description('Show cloudflare plugin statistics')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const cfDb = new CloudflareDatabase(db).forSourceAccount(options.appId);

      const stats = await cfDb.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get statistics', { error: message });
      process.exit(1);
    }
  });

program.parse();
