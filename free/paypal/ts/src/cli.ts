#!/usr/bin/env node
/**
 * PayPal Plugin CLI
 */

import 'dotenv/config';
import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { createPayPalDatabase } from './database.js';
import {
  createPayPalAccountContexts,
  runPayPalAccountSync,
  runPayPalAccountReconcile,
} from './account-sync.js';
import { createServer } from './server.js';

const logger = createLogger('paypal:cli');
const program = new Command();

program
  .name('nself-paypal')
  .description('PayPal plugin for nself')
  .version('1.0.0');

program
  .command('sync')
  .description('Sync all PayPal data to database')
  .option('-i, --incremental', 'Only sync recent changes')
  .option('-a, --account <accounts>', 'Comma-separated account labels to sync (default: all)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = createPayPalDatabase({
        host: config.databaseHost, port: config.databasePort,
        database: config.databaseName, user: config.databaseUser,
        password: config.databasePassword, ssl: config.databaseSsl,
      });

      await db.connect();
      await db.initializeSchema();

      let contexts = createPayPalAccountContexts(config, db);

      if (options.account) {
        const accountFilter = options.account.split(',').map((s: string) => s.trim());
        contexts = contexts.filter(c => accountFilter.includes(c.account.id));
        if (contexts.length === 0) {
          logger.error('No matching accounts found');
          process.exit(1);
        }
      }

      logger.info(`Syncing ${contexts.length} account(s)...`);
      const result = await runPayPalAccountSync(contexts, config, {
        incremental: options.incremental,
      });

      for (const account of result.accounts) {
        logger.info(`[${account.accountId}] ${account.mode}`, {
          success: account.result.success,
          duration: account.result.duration,
        });
      }

      logger.success('Sync complete', {
        transactions: result.stats.transactions,
        products: result.stats.products,
        subscriptions: result.stats.subscriptions,
        disputes: result.stats.disputes,
        invoices: result.stats.invoices,
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
      const db = createPayPalDatabase({
        host: config.databaseHost, port: config.databasePort,
        database: config.databaseName, user: config.databaseUser,
        password: config.databasePassword, ssl: config.databaseSsl,
      });

      await db.connect();
      await db.initializeSchema();

      let contexts = createPayPalAccountContexts(config, db);

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
      const result = await runPayPalAccountReconcile(contexts, config, lookbackDays);

      logger.success('Reconciliation complete', {
        transactions: result.stats.transactions,
        disputes: result.stats.disputes,
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
  .description('Start PayPal plugin server')
  .option('-p, --port <port>', 'Server port', '3004')
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
      const db = createPayPalDatabase({
        host: config.databaseHost, port: config.databasePort,
        database: config.databaseName, user: config.databaseUser,
        password: config.databasePassword, ssl: config.databaseSsl,
      });

      await db.connect();
      const stats = await db.getStats();

      console.log('\nPayPal Plugin Status');
      console.log('═'.repeat(40));
      console.log(`Environment: ${config.environment}`);
      console.log(`Accounts: ${config.accounts.map(a => a.id).join(', ')}`);
      console.log(`\nDatabase Records:`);
      console.log(`  Transactions:      ${stats.transactions}`);
      console.log(`  Orders:            ${stats.orders}`);
      console.log(`  Captures:          ${stats.captures}`);
      console.log(`  Authorizations:    ${stats.authorizations}`);
      console.log(`  Refunds:           ${stats.refunds}`);
      console.log(`  Subscriptions:     ${stats.subscriptions}`);
      console.log(`  Subscription Plans: ${stats.subscriptionPlans}`);
      console.log(`  Products:          ${stats.products}`);
      console.log(`  Disputes:          ${stats.disputes}`);
      console.log(`  Payouts:           ${stats.payouts}`);
      console.log(`  Invoices:          ${stats.invoices}`);
      console.log(`  Payers:            ${stats.payers}`);
      console.log(`  Balances:          ${stats.balances}`);
      console.log(`  Last Synced:       ${stats.lastSyncedAt ?? 'Never'}`);

      await db.disconnect();
    } catch (error) {
      logger.error('Status check failed', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  });

program.parse();
