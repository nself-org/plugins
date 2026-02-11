#!/usr/bin/env node
/**
 * Entitlements Plugin CLI
 * Command-line interface for the Entitlements plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { EntitlementsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('entitlements:cli');

const program = new Command();

program
  .name('nself-entitlements')
  .description('Entitlements plugin for nself - subscriptions, feature gating, and quotas')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new EntitlementsDatabase();
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
  .description('Start the entitlements server')
  .option('-p, --port <port>', 'Server port', '3714')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });
      const { app } = await createServer(config);
      await app.listen({ port: config.port, host: config.host });
      logger.success(`Entitlements plugin listening on ${config.host}:${config.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show entitlements status and statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new EntitlementsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nEntitlements Status');
      console.log('====================');
      console.log(`Plans:                ${stats.total_plans}`);
      console.log(`Active Subscriptions: ${stats.active_subscriptions}`);
      console.log(`Trialing:             ${stats.trialing_subscriptions}`);
      console.log(`Features:             ${stats.total_features}`);
      console.log(`Active Grants:        ${stats.total_grants}`);
      console.log(`Quotas:               ${stats.active_quotas}`);
      console.log(`Exceeded Quotas:      ${stats.exceeded_quotas}`);
      console.log(`Events:               ${stats.total_events}`);
      console.log(`MRR:                  $${(stats.mrr_cents / 100).toFixed(2)}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Plans command
program
  .command('plans')
  .description('Manage entitlement plans')
  .argument('[action]', 'Action: list, get', 'list')
  .option('--id <id>', 'Plan ID (for get)')
  .option('--slug <slug>', 'Plan slug (for get)')
  .option('--type <type>', 'Filter by plan type')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new EntitlementsDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const plans = await db.listPlans({ plan_type: options.type, is_archived: false });
          console.log(`\nPlans (${plans.length}):`);
          console.log('-'.repeat(100));
          for (const p of plans) {
            console.log(`${p.id} | ${p.name} (${p.slug}) | ${p.plan_type} | $${(p.price_cents / 100).toFixed(2)}/${p.billing_interval} | ${p.is_public ? 'Public' : 'Hidden'}`);
          }
          break;
        }
        case 'get': {
          let plan = null;
          if (options.id) plan = await db.getPlan(options.id);
          else if (options.slug) plan = await db.getPlanBySlug(options.slug);
          else { logger.error('ID or slug required'); process.exit(1); }
          if (!plan) { logger.error('Plan not found'); process.exit(1); }
          console.log(JSON.stringify(plan, null, 2));
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
  .description('Manage subscriptions')
  .argument('[action]', 'Action: list, get, active', 'list')
  .option('--id <id>', 'Subscription ID')
  .option('-w, --workspace <id>', 'Workspace ID')
  .option('-u, --user <id>', 'User ID')
  .option('--status <status>', 'Filter by status')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new EntitlementsDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const subs = await db.listSubscriptions({
            workspace_id: options.workspace,
            user_id: options.user,
            status: options.status,
          });
          console.log(`\nSubscriptions (${subs.length}):`);
          console.log('-'.repeat(120));
          for (const s of subs) {
            console.log(`${s.id} | ${s.workspace_id ?? s.user_id} | ${s.status} | $${(s.price_cents / 100).toFixed(2)}/${s.billing_interval} | ${new Date(s.current_period_end).toLocaleDateString()}`);
          }
          break;
        }
        case 'get': {
          if (!options.id) { logger.error('Subscription ID required (--id)'); process.exit(1); }
          const sub = await db.getSubscription(options.id);
          if (!sub) { logger.error('Subscription not found'); process.exit(1); }
          console.log(JSON.stringify(sub, null, 2));
          break;
        }
        case 'active': {
          const sub = await db.getActiveSubscription(options.workspace, options.user);
          if (!sub) { console.log('No active subscription found'); } else { console.log(JSON.stringify(sub, null, 2)); }
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

// Check feature command
program
  .command('check-feature')
  .description('Check feature access')
  .requiredOption('-k, --key <key>', 'Feature key')
  .option('-w, --workspace <id>', 'Workspace ID')
  .option('-u, --user <id>', 'User ID')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new EntitlementsDatabase();
      await db.connect();

      const result = await db.checkFeatureAccess(options.key, options.workspace, options.user);

      console.log('\nFeature Access Check');
      console.log('====================');
      console.log(`Feature:    ${options.key}`);
      console.log(`Has Access: ${result.has_access ? 'Yes' : 'No'}`);
      console.log(`Value:      ${JSON.stringify(result.value)}`);
      console.log(`Source:     ${result.source}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Check quota command
program
  .command('check-quota')
  .description('Check quota availability')
  .requiredOption('-k, --key <key>', 'Quota key')
  .option('-w, --workspace <id>', 'Workspace ID')
  .option('-u, --user <id>', 'User ID')
  .option('-a, --amount <amount>', 'Requested amount', '1')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new EntitlementsDatabase();
      await db.connect();

      const result = await db.checkQuotaAvailability(options.key, parseInt(options.amount, 10), options.workspace, options.user);

      console.log('\nQuota Availability Check');
      console.log('========================');
      console.log(`Quota:     ${options.key}`);
      console.log(`Available: ${result.available ? 'Yes' : 'No'}`);
      if (result.reason) console.log(`Reason:    ${result.reason}`);
      if (result.current_usage !== undefined) console.log(`Usage:     ${result.current_usage}`);
      if (result.limit_value !== undefined) console.log(`Limit:     ${result.limit_value}`);
      if (result.remaining !== undefined) console.log(`Remaining: ${result.remaining}`);
      if (result.is_unlimited) console.log(`Unlimited: Yes`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
