#!/usr/bin/env node
/**
 * DLNA Plugin CLI
 * Command-line interface for the DLNA/UPnP media server plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { DlnaDatabase } from './database.js';
import { MediaScanner } from './media-scanner.js';
import { createServer } from './server.js';

const logger = createLogger('dlna:cli');

const program = new Command();

program
  .name('nself-dlna')
  .description('DLNA/UPnP media server plugin for nself')
  .version('1.0.0');

// Server command
program
  .command('server')
  .description('Start the DLNA media server with SSDP discovery')
  .option('-p, --port <port>', 'HTTP server port', '3025')
  .option('-H, --host <host>', 'Server host', '0.0.0.0')
  .option('-n, --name <name>', 'Friendly name for DLNA discovery')
  .action(async (options) => {
    try {
      const overrides: Record<string, unknown> = {
        dlnaPort: parseInt(options.port, 10),
        host: options.host,
      };
      if (options.name) {
        overrides.friendlyName = options.name;
      }

      const server = await createServer(overrides);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new DlnaDatabase();
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

// Scan command
program
  .command('scan')
  .description('Scan media directories and index files to database')
  .option('-d, --dirs <dirs>', 'Comma-separated media directories (overrides DLNA_MEDIA_PATHS)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const mediaPaths = options.dirs
        ? options.dirs.split(',').map((d: string) => d.trim())
        : config.mediaPaths;

      const db = new DlnaDatabase(config.sourceAccountId);
      await db.connect();
      await db.initializeSchema();

      const scanner = new MediaScanner(db, config.sourceAccountId);
      const result = await scanner.scan(mediaPaths);

      console.log('\nMedia Scan Results');
      console.log('==================');
      console.log(`Total Files:    ${result.totalFiles}`);
      console.log(`New Files:      ${result.newFiles}`);
      console.log(`Updated Files:  ${result.updatedFiles}`);
      console.log(`Removed Files:  ${result.removedFiles}`);
      console.log(`Duration:       ${result.duration}ms`);

      if (result.errors.length > 0) {
        console.log(`\nErrors (${result.errors.length}):`);
        result.errors.forEach(err => console.log(`  - ${err}`));
      }

      await db.disconnect();
      process.exit(result.errors.length > 0 ? 1 : 0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Scan failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show server status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();

      const db = new DlnaDatabase(config.sourceAccountId);
      await db.connect();

      const stats = await db.getStats();
      const activeRenderers = await db.getActiveRenderers();

      console.log('\nDLNA Plugin Status');
      console.log('==================');
      console.log(`Friendly Name:  ${config.friendlyName}`);
      console.log(`UUID:           ${config.uuid}`);
      console.log(`HTTP Port:      ${config.dlnaPort}`);
      console.log(`SSDP Port:      ${config.ssdpPort}`);
      console.log(`Media Paths:    ${config.mediaPaths.join(', ')}`);
      console.log('');
      console.log('Media Library:');
      console.log(`  Total Items:  ${stats.mediaItems}`);
      console.log(`  Containers:   ${stats.containers}`);
      console.log(`  Videos:       ${stats.videos}`);
      console.log(`  Audio:        ${stats.audio}`);
      console.log(`  Images:       ${stats.images}`);
      console.log(`  Total Size:   ${formatBytes(stats.totalSizeBytes as number)}`);
      console.log('');
      console.log('Network:');
      console.log(`  Active Renderers: ${activeRenderers.length}`);
      if (activeRenderers.length > 0) {
        activeRenderers.forEach(r => {
          console.log(`    - ${r.friendly_name ?? r.usn} (${r.ip_address ?? 'unknown'})`);
        });
      }
      console.log(`  Total Renderers:  ${stats.renderers}`);

      if (stats.lastSyncedAt) {
        console.log(`\nLast Scan: ${(stats.lastSyncedAt as Date).toISOString()}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Renderers command
program
  .command('renderers')
  .description('List discovered DLNA renderers on the network')
  .option('-a, --active', 'Show only recently active renderers')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (options) => {
    try {
      const config = loadConfig();

      const db = new DlnaDatabase(config.sourceAccountId);
      await db.connect();

      let renderers;
      if (options.active) {
        renderers = await db.getActiveRenderers();
      } else {
        renderers = await db.listRenderers(parseInt(options.limit, 10));
      }

      console.log('\nDiscovered Renderers');
      console.log('====================');

      if (renderers.length === 0) {
        console.log('No renderers found');
      } else {
        renderers.forEach(r => {
          const name = r.friendly_name ?? 'Unknown';
          const ip = r.ip_address ?? 'N/A';
          const lastSeen = r.last_seen_at ? new Date(r.last_seen_at).toLocaleString() : 'N/A';
          console.log(`${name}`);
          console.log(`  IP:        ${ip}`);
          console.log(`  Location:  ${r.location}`);
          console.log(`  Type:      ${r.device_type ?? 'N/A'}`);
          console.log(`  Last Seen: ${lastSeen}`);
          console.log('');
        });
      }

      console.log(`Total: ${renderers.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Media command
program
  .command('media')
  .description('List indexed media items')
  .argument('[action]', 'Action: list, show, search', 'list')
  .argument('[id]', 'Media item ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-t, --type <type>', 'Filter by object type (container, item)')
  .option('-q, --query <query>', 'Search query')
  .action(async (action, id, options) => {
    try {
      const config = loadConfig();

      const db = new DlnaDatabase(config.sourceAccountId);
      await db.connect();

      switch (action) {
        case 'list': {
          const items = await db.listMediaItems(parseInt(options.limit, 10));
          console.log('\nMedia Items');
          console.log('-'.repeat(100));
          items.forEach(item => {
            const type = item.object_type === 'container' ? '[DIR]' : '[FILE]';
            const size = item.file_size ? formatBytes(item.file_size) : '';
            const mime = item.mime_type ?? '';
            console.log(`${type} ${item.id.substring(0, 8)}... | ${item.title} | ${mime} | ${size}`);
          });
          const total = await db.countMediaItems();
          console.log(`\nTotal: ${total}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Media item ID required');
            process.exit(1);
          }
          const item = await db.getMediaItem(id);
          if (!item) {
            logger.error('Media item not found');
            process.exit(1);
          }
          console.log(JSON.stringify(item, null, 2));
          break;
        }
        case 'search': {
          const query = options.query ?? id ?? '';
          if (!query) {
            logger.error('Search query required (use --query or pass as argument)');
            process.exit(1);
          }
          const criteria = `dc:title contains "${query}"`;
          const { items, totalCount } = await db.searchMediaItems(criteria, 0, parseInt(options.limit, 10));
          console.log(`\nSearch Results for "${query}"`);
          console.log('-'.repeat(80));
          items.forEach(item => {
            const type = item.object_type === 'container' ? '[DIR]' : '[FILE]';
            console.log(`${type} ${item.id.substring(0, 8)}... | ${item.title} | ${item.mime_type ?? ''}`);
          });
          console.log(`\nResults: ${items.length} of ${totalCount}`);
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

// Prune command
program
  .command('prune')
  .description('Remove stale renderers from the database')
  .option('-h, --hours <hours>', 'Remove renderers not seen for this many hours', '24')
  .action(async (options) => {
    try {
      const config = loadConfig();

      const db = new DlnaDatabase(config.sourceAccountId);
      await db.connect();

      const hours = parseInt(options.hours, 10);
      const removed = await db.pruneStaleRenderers(hours);

      console.log(`Pruned ${removed} stale renderer(s) not seen in ${hours} hours`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Prune failed', { error: message });
      process.exit(1);
    }
  });

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(1)} ${units[i]}`;
}

program.parse();
