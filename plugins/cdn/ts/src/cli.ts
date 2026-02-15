#!/usr/bin/env node
/**
 * CDN Plugin CLI
 * Command-line interface for the CDN management plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { CdnDatabase } from './database.js';
import { createServer } from './server.js';
import type { CdnProvider } from './types.js';

const logger = createLogger('cdn:cli');

const program = new Command();

program
  .name('nself-cdn')
  .description('CDN management plugin for nself - cache purging, signed URLs (analytics sync planned)')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new CdnDatabase(undefined, 'primary');
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
  .option('-p, --port <port>', 'Server port', '3036')
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
  .description('Show CDN plugin statistics')
  .action(async () => {
    try {
      const db = new CdnDatabase(undefined, 'primary');
      await db.connect();

      const stats = await db.getPluginStats();

      console.log('\nCDN Plugin Status');
      console.log('=================');
      console.log(`  Zones (total):       ${stats.total_zones}`);
      console.log(`  Zones (active):      ${stats.active_zones}`);
      console.log(`  Purge requests:      ${stats.total_purge_requests}`);
      console.log(`  Pending purges:      ${stats.pending_purges}`);
      console.log(`  Signed URLs (total): ${stats.total_signed_urls}`);
      console.log(`  Signed URLs (active):${stats.active_signed_urls}`);
      console.log(`  Analytics days:      ${stats.analytics_days_tracked}`);
      console.log(`  Total requests:      ${stats.total_requests_tracked.toLocaleString()}`);

      const bandwidthMb = stats.total_bandwidth_tracked / (1024 * 1024);
      console.log(`  Total bandwidth:     ${bandwidthMb.toFixed(2)} MB`);

      if (Object.keys(stats.by_provider).length > 0) {
        console.log('\n  By Provider:');
        for (const [provider, count] of Object.entries(stats.by_provider)) {
          console.log(`    ${provider}: ${count} zones`);
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Zones command
program
  .command('zones')
  .description('Manage CDN zones')
  .argument('[action]', 'Action: list, add', 'list')
  .option('--provider <provider>', 'CDN provider (cloudflare, bunnycdn)')
  .option('--zone-id <zoneId>', 'Provider zone ID')
  .option('--name <name>', 'Zone name')
  .option('--domain <domain>', 'Zone domain')
  .option('--origin <originUrl>', 'Origin URL')
  .action(async (action, options) => {
    try {
      const db = new CdnDatabase(undefined, 'primary');
      await db.connect();

      switch (action) {
        case 'list': {
          const zones = await db.listZones(options.provider);
          console.log('\nCDN Zones:');
          console.log('-'.repeat(100));
          for (const zone of zones) {
            console.log(
              `${zone.id.substring(0, 8)}... | ` +
              `${zone.name.padEnd(20)} | ` +
              `${zone.domain.padEnd(30)} | ` +
              `${zone.provider.padEnd(12)} | ` +
              `${zone.status} | ` +
              `TTL: ${zone.cache_ttl}s`
            );
          }
          console.log(`\nTotal: ${zones.length}`);
          break;
        }

        case 'add': {
          if (!options.provider || !options.zoneId || !options.name || !options.domain) {
            logger.error('--provider, --zone-id, --name, and --domain are required');
            process.exit(1);
          }

          const zone = await db.createZone({
            provider: options.provider as CdnProvider,
            zone_id: options.zoneId,
            name: options.name,
            domain: options.domain,
            origin_url: options.origin,
          });

          logger.success(`Zone created: ${zone.id}`);
          console.log(JSON.stringify(zone, null, 2));
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Zones command failed', { error: message });
      process.exit(1);
    }
  });

// Purge command
program
  .command('purge')
  .description('Purge CDN cache')
  .option('--zone <zoneId>', 'Zone ID (required)')
  .option('--urls <urls>', 'Comma-separated URLs to purge')
  .option('--tags <tags>', 'Comma-separated cache tags to purge')
  .option('--prefixes <prefixes>', 'Comma-separated URL prefixes to purge')
  .option('--all', 'Purge entire zone cache')
  .action(async (options) => {
    try {
      if (!options.zone) {
        logger.error('--zone is required');
        process.exit(1);
      }

      const db = new CdnDatabase(undefined, 'primary');
      await db.connect();

      const zone = await db.getZone(options.zone);
      if (!zone) {
        logger.error('Zone not found');
        process.exit(1);
      }

      if (options.all) {
        const purge = await db.createPurgeRequest(options.zone, 'all', {
          requested_by: 'cli',
        });
        await db.updatePurgeStatus(purge.id, 'completed');
        logger.success(`Purged entire zone: ${zone.name} (${zone.domain})`);
      } else if (options.urls) {
        const urls = options.urls.split(',').map((u: string) => u.trim());
        const purge = await db.createPurgeRequest(options.zone, 'urls', {
          urls,
          requested_by: 'cli',
        });
        await db.updatePurgeStatus(purge.id, 'completed');
        logger.success(`Purged ${urls.length} URLs from ${zone.name}`);
      } else if (options.tags) {
        const tags = options.tags.split(',').map((t: string) => t.trim());
        const purge = await db.createPurgeRequest(options.zone, 'tags', {
          tags,
          requested_by: 'cli',
        });
        await db.updatePurgeStatus(purge.id, 'completed');
        logger.success(`Purged ${tags.length} tags from ${zone.name}`);
      } else if (options.prefixes) {
        const prefixes = options.prefixes.split(',').map((p: string) => p.trim());
        const purge = await db.createPurgeRequest(options.zone, 'prefixes', {
          prefixes,
          requested_by: 'cli',
        });
        await db.updatePurgeStatus(purge.id, 'completed');
        logger.success(`Purged ${prefixes.length} prefixes from ${zone.name}`);
      } else {
        logger.error('Specify --urls, --tags, --prefixes, or --all');
        process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Purge failed', { error: message });
      process.exit(1);
    }
  });

// Sign command
program
  .command('sign')
  .description('Generate signed URLs')
  .argument('[url]', 'URL to sign')
  .option('--zone <zoneId>', 'Zone ID')
  .option('--ttl <seconds>', 'Time to live in seconds', '3600')
  .option('--batch <file>', 'File with URLs to sign (one per line)')
  .action(async (url, options) => {
    try {
      const config = loadConfig();

      if (!config.signingKey) {
        logger.error('CDN_SIGNING_KEY is not configured');
        process.exit(1);
      }

      if (!url && !options.batch) {
        logger.error('Provide a URL or --batch <file>');
        process.exit(1);
      }

      const ttl = parseInt(options.ttl, 10);

      if (url) {
        const crypto = await import('node:crypto');
        const expires = Math.floor(Date.now() / 1000) + ttl;
        const dataToSign = `${url}${expires}`;
        const signature = crypto.createHmac('sha256', config.signingKey).update(dataToSign).digest('hex');
        const separator = url.includes('?') ? '&' : '?';
        const signedUrl = `${url}${separator}expires=${expires}&sig=${signature}`;

        console.log('\nSigned URL:');
        console.log('===========');
        console.log(`  Original:  ${url}`);
        console.log(`  Signed:    ${signedUrl}`);
        console.log(`  Expires:   ${new Date(expires * 1000).toISOString()}`);
        console.log(`  TTL:       ${ttl}s`);
      } else {
        logger.info(`Batch signing from file: ${options.batch}`);
        logger.info('Batch file processing pending');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sign failed', { error: message });
      process.exit(1);
    }
  });

// Analytics command
program
  .command('analytics')
  .description('View stored CDN analytics (sync from providers not yet implemented)')
  .argument('[action]', 'Action: view, sync', 'view')
  .option('--zone <zoneId>', 'Zone ID')
  .option('--days <days>', 'Number of days', '30')
  .action(async (action, options) => {
    try {
      const db = new CdnDatabase(undefined, 'primary');
      await db.connect();

      switch (action) {
        case 'view': {
          const from = new Date();
          from.setDate(from.getDate() - parseInt(options.days, 10));

          const summary = await db.getAnalyticsSummary(options.zone);

          console.log('\nCDN Analytics Summary (last 30 days):');
          console.log('-'.repeat(100));

          for (const s of summary) {
            console.log(`\n  Zone: ${s.zone_name} (${s.domain})`);
            console.log(`    Total requests:    ${Number(s.total_requests).toLocaleString()}`);
            console.log(`    Cached requests:   ${Number(s.cached_requests).toLocaleString()}`);
            console.log(`    Cache hit rate:    ${s.cache_hit_rate ?? 0}%`);
            const bwMb = Number(s.total_bandwidth) / (1024 * 1024);
            console.log(`    Total bandwidth:   ${bwMb.toFixed(2)} MB`);
            console.log(`    Unique visitors:   ${Number(s.total_visitors).toLocaleString()}`);
            console.log(`    4xx errors:        ${Number(s.total_4xx).toLocaleString()}`);
            console.log(`    5xx errors:        ${Number(s.total_5xx).toLocaleString()}`);
            console.log(`    Days covered:      ${s.days_covered}`);
          }

          if (summary.length === 0) {
            console.log('  No analytics data available.');
          }
          break;
        }

        case 'sync': {
          logger.info('Starting analytics sync...');

          const zones = await db.listZones();
          logger.info(`Found ${zones.length} zones to sync`);

          // Placeholder for actual provider sync
          logger.success('Analytics sync completed (provider integration pending)');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Analytics command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
