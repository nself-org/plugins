#!/usr/bin/env node
/**
 * Donorbox Plugin CLI
 */

import 'dotenv/config';
import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { createDonorboxDatabase } from './database.js';
import {
  createDonorboxAccountContexts,
  runDonorboxAccountSync,
  runDonorboxAccountReconcile,
} from './account-sync.js';
import { createServer } from './server.js';

const logger = createLogger('donorbox:cli');
const program = new Command();

program
  .name('nself-donorbox')
  .description('Donorbox plugin for nself')
  .version('1.0.0');

program
  .command('sync')
  .description('Sync all Donorbox data to database')
  .option('-i, --incremental', 'Only sync recent changes')
  .option('-a, --account <accounts>', 'Comma-separated account labels to sync (default: all)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = createDonorboxDatabase({
        host: config.databaseHost, port: config.databasePort,
        database: config.databaseName, user: config.databaseUser,
        password: config.databasePassword, ssl: config.databaseSsl,
      });

      await db.connect();
      await db.initializeSchema();

      let contexts = createDonorboxAccountContexts(config, db);

      if (options.account) {
        const accountFilter = options.account.split(',').map((s: string) => s.trim());
        contexts = contexts.filter(c => accountFilter.includes(c.account.id));
        if (contexts.length === 0) {
          logger.error('No matching accounts found');
          process.exit(1);
        }
      }

      logger.info(`Syncing ${contexts.length} account(s)...`);
      const result = await runDonorboxAccountSync(contexts, {
        incremental: options.incremental,
      });

      for (const account of result.accounts) {
        logger.info(`[${account.accountId}]`, {
          success: account.result.success,
          duration: account.result.duration,
        });
      }

      logger.success('Sync complete', {
        campaigns: result.stats.campaigns,
        donors: result.stats.donors,
        donations: result.stats.donations,
        plans: result.stats.plans,
        events: result.stats.events,
        tickets: result.stats.tickets,
        duration: result.duration,
      });

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      logger.error('Sync failed', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  });

program
  .command('reconcile')
  .description('Re-sync recent data to catch gaps from missed webhooks')
  .option('-d, --days <days>', 'Lookback window in days', '7')
  .option('-a, --account <accounts>', 'Comma-separated account labels (default: all)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = createDonorboxDatabase({
        host: config.databaseHost, port: config.databasePort,
        database: config.databaseName, user: config.databaseUser,
        password: config.databasePassword, ssl: config.databaseSsl,
      });

      await db.connect();
      await db.initializeSchema();

      let contexts = createDonorboxAccountContexts(config, db);

      if (options.account) {
        const accountFilter = options.account.split(',').map((s: string) => s.trim());
        contexts = contexts.filter(c => accountFilter.includes(c.account.id));
        if (contexts.length === 0) {
          logger.error('No matching accounts found');
          process.exit(1);
        }
      }

      const lookbackDays = parseInt(options.days, 10);
      logger.info(`Reconciling ${contexts.length} account(s) with ${lookbackDays}-day lookback...`);
      const result = await runDonorboxAccountReconcile(contexts, lookbackDays);

      logger.success('Reconciliation complete', {
        donations: result.stats.donations,
        donors: result.stats.donors,
        duration: result.duration,
      });

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      logger.error('Reconciliation failed', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  });

program
  .command('server')
  .description('Start Donorbox plugin server')
  .option('-p, --port <port>', 'Server port', '3005')
  .action(async (options) => {
    try {
      await createServer({ port: parseInt(options.port, 10) });
    } catch (error) {
      logger.error('Server failed', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  });

program
  .command('status')
  .description('Show sync status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = createDonorboxDatabase({
        host: config.databaseHost, port: config.databasePort,
        database: config.databaseName, user: config.databaseUser,
        password: config.databasePassword, ssl: config.databaseSsl,
      });

      await db.connect();
      const stats = await db.getStats();

      console.log('\nDonorbox Plugin Status');
      console.log('═'.repeat(40));
      console.log(`Accounts: ${config.accounts.map(a => a.id).join(', ')}`);
      console.log(`\nDatabase Records:`);
      console.log(`  Campaigns:  ${stats.campaigns}`);
      console.log(`  Donors:     ${stats.donors}`);
      console.log(`  Donations:  ${stats.donations}`);
      console.log(`  Plans:      ${stats.plans}`);
      console.log(`  Events:     ${stats.events}`);
      console.log(`  Tickets:    ${stats.tickets}`);
      console.log(`  Last Synced: ${stats.lastSyncedAt ?? 'Never'}`);

      await db.disconnect();
    } catch (error) {
      logger.error('Status check failed', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  });

program.parse();
