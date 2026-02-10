#!/usr/bin/env node
/**
 * Stripe Plugin CLI
 * Command-line interface for the Stripe plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig, isTestMode } from './config.js';
import { StripeDatabase } from './database.js';
import { createServer } from './server.js';
import { createStripeAccountContexts, runStripeAccountSync } from './account-sync.js';

const logger = createLogger('stripe:cli');

const program = new Command();

function sourceAccountLabel(record: Record<string, unknown>): string {
  const value = record.source_account_id;
  if (typeof value === 'string' && value.length > 0) {
    return value;
  }
  return 'primary';
}

program
  .name('nself-stripe')
  .description('Stripe plugin for nself - sync Stripe data to PostgreSQL')
  .version('1.0.0');

// Sync command
program
  .command('sync')
  .description('Sync Stripe data to database')
  .option('-r, --resources <resources>', 'Comma-separated list of resources to sync', 'all')
  .option('-i, --incremental', 'Only sync changes since last sync')
  .action(async (options) => {
    try {
      const config = loadConfig();

      logger.info('Starting Stripe sync...');
      logger.info(`Accounts configured: ${config.stripeAccounts.length}`);
      const db = new StripeDatabase();
      await db.connect();
      await db.initializeSchema();
      const contexts = createStripeAccountContexts(config, db);

      const resources = options.resources === 'all'
        ? undefined
        : options.resources.split(',').map((r: string) => r.trim());

      const result = await runStripeAccountSync(contexts, {
        resources: resources as Array<'customers' | 'products' | 'prices' | 'subscriptions' | 'invoices' | 'payment_intents' | 'payment_methods'>,
        incremental: options.incremental,
      });

      console.log('\nAccount Results:');
      console.log('================');
      result.accounts.forEach(accountResult => {
        const status = accountResult.result.success ? 'OK' : 'FAILED';
        console.log(`- ${accountResult.accountId} (${accountResult.mode}): ${status} in ${(accountResult.result.duration / 1000).toFixed(1)}s`);
      });

      console.log('\nSync Results:');
      console.log('=============');
      console.log(`Customers:       ${result.stats.customers}`);
      console.log(`Products:        ${result.stats.products}`);
      console.log(`Prices:          ${result.stats.prices}`);
      console.log(`Subscriptions:   ${result.stats.subscriptions}`);
      console.log(`Invoices:        ${result.stats.invoices}`);
      console.log(`Payment Intents: ${result.stats.paymentIntents}`);
      console.log(`Payment Methods: ${result.stats.paymentMethods}`);
      console.log(`\nDuration: ${(result.duration / 1000).toFixed(1)}s`);

      if (result.errors.length > 0) {
        console.log('\nErrors:');
        result.errors.forEach(err => console.log(`  - ${err}`));
      }

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the webhook server')
  .option('-p, --port <port>', 'Server port', '3001')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Mode: ${isTestMode(config.stripeApiKey) ? 'TEST' : 'LIVE'}`);

      const server = await createServer(config);
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

      const db = new StripeDatabase();
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

// Status command
program
  .command('status')
  .description('Show sync status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();

      const db = new StripeDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nStripe Plugin Status');
      console.log('====================');
      console.log(`Primary mode: ${isTestMode(config.stripeApiKey) ? 'TEST' : 'LIVE'}`);
      console.log(`Accounts: ${config.stripeAccounts.length}`);
      config.stripeAccounts.forEach(account => {
        console.log(`  - ${account.id}: ${isTestMode(account.apiKey) ? 'TEST' : 'LIVE'}`);
      });
      console.log('\nSynced Records:');
      console.log(`  Customers:       ${stats.customers}`);
      console.log(`  Products:        ${stats.products}`);
      console.log(`  Prices:          ${stats.prices}`);
      console.log(`  Subscriptions:   ${stats.subscriptions}`);
      console.log(`  Invoices:        ${stats.invoices}`);
      console.log(`  Payment Intents: ${stats.paymentIntents}`);
      console.log(`  Payment Methods: ${stats.paymentMethods}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Customers command
program
  .command('customers')
  .description('List or manage customers')
  .argument('[action]', 'Action: list, show, sync', 'list')
  .argument('[id]', 'Customer ID (for show/sync)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      const config = loadConfig();
      const db = new StripeDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const customers = await db.listCustomers(parseInt(options.limit, 10));
          console.log('\nCustomers:');
          console.log('-'.repeat(80));
          customers.forEach(c => {
            console.log(`[${sourceAccountLabel(c as unknown as Record<string, unknown>)}] ${c.id} | ${c.email ?? 'N/A'} | ${c.name ?? 'N/A'}`);
          });
          console.log(`\nTotal: ${await db.countCustomers()}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Customer ID required');
            process.exit(1);
          }
          const customer = await db.getCustomer(id);
          if (!customer) {
            logger.error('Customer not found');
            process.exit(1);
          }
          console.log(JSON.stringify(customer, null, 2));
          break;
        }
        case 'sync': {
          const contexts = createStripeAccountContexts(config, db);
          if (id) {
            let synced = false;
            for (const context of contexts) {
              const accountSynced = await context.syncService.syncSingleResource('customer', id);
              if (accountSynced) {
                logger.success(`Synced customer ${id} from account ${context.account.id}`);
                synced = true;
                break;
              }
            }

            if (!synced) {
              logger.error(`Customer ${id} not found in configured accounts`);
              process.exit(1);
            }
          } else {
            let totalCustomers = 0;
            for (const context of contexts) {
              const customers = await context.client.listAllCustomers();
              const syncedCount = await context.db.upsertCustomers(customers);
              totalCustomers += syncedCount;
              logger.success(`Synced ${syncedCount} customers from account ${context.account.id}`);
            }

            logger.success(`Synced ${totalCustomers} customers across ${contexts.length} account(s)`);
          }
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

// Subscriptions command
program
  .command('subscriptions')
  .description('List or manage subscriptions')
  .argument('[action]', 'Action: list, show, stats', 'list')
  .argument('[id]', 'Subscription ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-s, --status <status>', 'Filter by status')
  .action(async (action, id, options) => {
    try {
      const db = new StripeDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const subscriptions = await db.listSubscriptions(parseInt(options.limit, 10));
          console.log('\nSubscriptions:');
          console.log('-'.repeat(100));
          subscriptions.forEach(s => {
            console.log(`[${sourceAccountLabel(s as unknown as Record<string, unknown>)}] ${s.id} | ${s.customer_id} | ${s.status} | ${s.current_period_end.toISOString().split('T')[0]}`);
          });
          console.log(`\nTotal: ${await db.countSubscriptions(options.status)}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Subscription ID required');
            process.exit(1);
          }
          const subscription = await db.getSubscription(id);
          if (!subscription) {
            logger.error('Subscription not found');
            process.exit(1);
          }
          console.log(JSON.stringify(subscription, null, 2));
          break;
        }
        case 'stats': {
          const active = await db.countSubscriptions('active');
          const trialing = await db.countSubscriptions('trialing');
          const pastDue = await db.countSubscriptions('past_due');
          const canceled = await db.countSubscriptions('canceled');
          const total = await db.countSubscriptions();

          console.log('\nSubscription Statistics:');
          console.log('========================');
          console.log(`Active:    ${active}`);
          console.log(`Trialing:  ${trialing}`);
          console.log(`Past Due:  ${pastDue}`);
          console.log(`Canceled:  ${canceled}`);
          console.log(`Total:     ${total}`);
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

// Invoices command
program
  .command('invoices')
  .description('List or manage invoices')
  .argument('[action]', 'Action: list, show', 'list')
  .argument('[id]', 'Invoice ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-s, --status <status>', 'Filter by status')
  .action(async (action, id, options) => {
    try {
      const db = new StripeDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const invoices = await db.listInvoices(parseInt(options.limit, 10));
          console.log('\nInvoices:');
          console.log('-'.repeat(120));
          invoices.forEach(i => {
            const amount = (i.total / 100).toFixed(2);
            console.log(`[${sourceAccountLabel(i as unknown as Record<string, unknown>)}] ${i.id} | ${i.customer_email ?? 'N/A'} | ${i.status} | ${i.currency.toUpperCase()} ${amount} | ${i.created_at.toISOString().split('T')[0]}`);
          });
          console.log(`\nTotal: ${await db.countInvoices(options.status)}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Invoice ID required');
            process.exit(1);
          }
          const invoice = await db.getInvoice(id);
          if (!invoice) {
            logger.error('Invoice not found');
            process.exit(1);
          }
          console.log(JSON.stringify(invoice, null, 2));
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

// Products command
program
  .command('products')
  .description('List or manage products')
  .argument('[action]', 'Action: list, show', 'list')
  .argument('[id]', 'Product ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      const db = new StripeDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const products = await db.listProducts(parseInt(options.limit, 10));
          console.log('\nProducts:');
          console.log('-'.repeat(80));
          products.forEach(p => {
            console.log(`[${sourceAccountLabel(p as unknown as Record<string, unknown>)}] ${p.id} | ${p.name} | ${p.active ? 'Active' : 'Inactive'}`);
          });
          console.log(`\nTotal: ${await db.countProducts()}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Product ID required');
            process.exit(1);
          }
          const product = await db.getProduct(id);
          if (!product) {
            logger.error('Product not found');
            process.exit(1);
          }
          console.log(JSON.stringify(product, null, 2));
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

// Prices command
program
  .command('prices')
  .description('List or manage prices')
  .argument('[action]', 'Action: list, show', 'list')
  .argument('[id]', 'Price ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      const db = new StripeDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const prices = await db.listPrices(parseInt(options.limit, 10));
          console.log('\nPrices:');
          console.log('-'.repeat(100));
          prices.forEach(p => {
            const amount = p.unit_amount ? (p.unit_amount / 100).toFixed(2) : 'N/A';
            console.log(`[${sourceAccountLabel(p as unknown as Record<string, unknown>)}] ${p.id} | ${p.product_id} | ${p.currency.toUpperCase()} ${amount} | ${p.type} | ${p.active ? 'Active' : 'Inactive'}`);
          });
          console.log(`\nTotal: ${await db.countPrices()}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Price ID required');
            process.exit(1);
          }
          const price = await db.getPrice(id);
          if (!price) {
            logger.error('Price not found');
            process.exit(1);
          }
          console.log(JSON.stringify(price, null, 2));
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

program.parse();
