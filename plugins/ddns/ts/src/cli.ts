#!/usr/bin/env node
/**
 * DDNS Plugin CLI
 * Command-line interface for the DDNS plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { DdnsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('ddns:cli');

const program = new Command();

program
  .name('nself-ddns')
  .description('DDNS plugin for nself - dynamic DNS updater with multi-provider support')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new DdnsDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for DDNS plugin');
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
  .description('Start the DDNS API server')
  .option('-p, --port <port>', 'Server port', '3217')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting DDNS server on ${config.host}:${config.port}`);
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
  .description('Show current IP and domain status')
  .action(async () => {
    try {
      const db = new DdnsDatabase();
      await db.connect();
      const configs = await db.listConfigs({ isEnabled: true });
      await db.disconnect();

      if (configs.length === 0) {
        console.log('No DDNS configurations found');
        process.exit(0);
      }

      console.log('\nDDNS Status:');
      console.log('============');
      for (const cfg of configs) {
        const enabled = cfg.is_enabled ? '' : ' [disabled]';
        console.log(`\n  ${cfg.domain}${enabled}`);
        console.log(`    Provider:    ${cfg.provider}`);
        console.log(`    Current IP:  ${cfg.current_ip ?? 'unknown'}`);
        console.log(`    Record Type: ${cfg.record_type}`);
        console.log(`    Interval:    ${cfg.check_interval}s`);
        if (cfg.last_check_at) {
          console.log(`    Last Check:  ${cfg.last_check_at.toISOString()}`);
        }
        if (cfg.last_update_at) {
          console.log(`    Last Update: ${cfg.last_update_at.toISOString()}`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Update command
program
  .command('update')
  .description('Force a DNS update for all enabled configs')
  .option('--config-id <id>', 'Update specific config only')
  .action(async (options) => {
    try {
      const config = loadConfig();
      console.log('Forcing DNS update...');
      console.log(`Server should be running on port ${config.port}`);
      console.log('Use POST /api/update endpoint');
      if (options.configId) {
        console.log(`Config ID: ${options.configId}`);
      }
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Update failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Providers command
program
  .command('providers')
  .description('List available DNS providers')
  .action(async () => {
    console.log('\nAvailable DNS Providers:');
    console.log('========================\n');
    console.log('  duckdns     - DuckDNS (duckdns.org)');
    console.log('                Free dynamic DNS, supports IPv4 and IPv6');
    console.log('                Requires: token\n');
    console.log('  cloudflare  - Cloudflare (cloudflare.com)');
    console.log('                DNS management via Cloudflare API');
    console.log('                Requires: token, api_key, zone_id\n');
    console.log('  noip        - No-IP (noip.com)');
    console.log('                Dynamic DNS service');
    console.log('                Requires: token\n');
    console.log('  dynu        - Dynu (dynu.com)');
    console.log('                Free dynamic DNS, supports IPv4 and IPv6');
    console.log('                Requires: token\n');

    process.exit(0);
  });

// History command
program
  .command('history')
  .description('Show update history')
  .option('--config-id <id>', 'Filter by config ID')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (options) => {
    try {
      const db = new DdnsDatabase();
      await db.connect();

      const logs = await db.listUpdateLogs({
        configId: options.configId,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (logs.length === 0) {
        console.log('No update history found');
        process.exit(0);
      }

      console.log(`\nUpdate History (${logs.length} entries):\n`);
      for (const log of logs) {
        const statusIcon = log.status === 'success' ? '[OK]' : log.status === 'skipped' ? '[SKIP]' : '[FAIL]';
        console.log(`  ${statusIcon} ${log.domain} (${log.provider})`);
        console.log(`    ${log.old_ip ?? 'none'} -> ${log.new_ip}`);
        console.log(`    Time:     ${log.created_at.toISOString()}`);
        console.log(`    Duration: ${log.duration_ms}ms`);
        if (log.error) {
          console.log(`    Error:    ${log.error}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get history', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show DDNS statistics')
  .action(async () => {
    try {
      const db = new DdnsDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nDDNS Statistics:');
      console.log('================');
      console.log(`Total Configs:      ${stats.total_configs}`);
      console.log(`Enabled:            ${stats.enabled_configs}`);
      console.log(`Total Updates:      ${stats.total_updates}`);
      console.log(`  Successful:       ${stats.successful_updates}`);
      console.log(`  Failed:           ${stats.failed_updates}`);
      console.log(`  Skipped:          ${stats.skipped_updates}`);
      if (stats.last_update_at) {
        console.log(`Last Update:        ${stats.last_update_at.toISOString()}`);
      }
      if (stats.last_check_at) {
        console.log(`Last Check:         ${stats.last_check_at.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
