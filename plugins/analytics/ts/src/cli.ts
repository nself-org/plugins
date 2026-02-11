#!/usr/bin/env node
/**
 * Analytics Plugin CLI
 * Command-line interface for the Analytics plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { AnalyticsDatabase } from './database.js';
import { createServer } from './server.js';
import type { TrackEventRequest, CreateFunnelRequest, CreateQuotaRequest } from './types.js';

const logger = createLogger('analytics:cli');

const program = new Command();

program
  .name('nself-analytics')
  .description('Analytics plugin for nself - event tracking and analytics')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new AnalyticsDatabase();
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
  .description('Start the analytics server')
  .option('-p, --port <port>', 'Server port', '3304')
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
  .description('Show analytics status and statistics')
  .action(async () => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nAnalytics Plugin Status');
      console.log('=======================');
      console.log(`Events:     ${stats.events}`);
      console.log(`Counters:   ${stats.counters}`);
      console.log(`Funnels:    ${stats.funnels}`);
      console.log(`Quotas:     ${stats.quotas}`);
      console.log(`Violations: ${stats.violations}`);
      console.log(`Last Event: ${stats.lastEventAt?.toISOString() ?? 'N/A'}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Track command
program
  .command('track')
  .description('Track an event')
  .requiredOption('-n, --name <name>', 'Event name')
  .option('-c, --category <category>', 'Event category')
  .option('-u, --user <user>', 'User ID')
  .option('-s, --session <session>', 'Session ID')
  .option('-p, --properties <json>', 'Event properties (JSON string)')
  .action(async (options) => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      const event: TrackEventRequest = {
        event_name: options.name,
        event_category: options.category,
        user_id: options.user,
        session_id: options.session,
        properties: options.properties ? JSON.parse(options.properties) : {},
      };

      const eventId = await db.trackEvent(event);
      await db.incrementCounter(event.event_name, event.user_id ?? 'total', 1);

      logger.success(`Event tracked: ${eventId}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Event tracking failed', { error: message });
      process.exit(1);
    }
  });

// Counters command
program
  .command('counters')
  .description('Manage counters')
  .argument('[action]', 'Action: list, get, increment', 'list')
  .option('-n, --name <name>', 'Counter name')
  .option('-d, --dimension <dimension>', 'Counter dimension', 'total')
  .option('-p, --period <period>', 'Counter period (hourly/daily/monthly/all_time)', 'all_time')
  .option('-i, --increment <value>', 'Increment value', '1')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, options) => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const counters = await db.listCounters(parseInt(options.limit, 10));
          console.log('\nCounters:');
          console.log('-'.repeat(100));
          counters.forEach(c => {
            console.log(`${c.counter_name} [${c.dimension}] (${c.period}): ${c.value} @ ${c.period_start.toISOString()}`);
          });
          break;
        }
        case 'get': {
          if (!options.name) {
            logger.error('Counter name required (--name)');
            process.exit(1);
          }
          const value = await db.getCounterValue(
            options.name,
            options.dimension,
            options.period as 'hourly' | 'daily' | 'monthly' | 'all_time'
          );
          if (!value) {
            logger.error('Counter not found');
            process.exit(1);
          }
          console.log(JSON.stringify(value, null, 2));
          break;
        }
        case 'increment': {
          if (!options.name) {
            logger.error('Counter name required (--name)');
            process.exit(1);
          }
          await db.incrementCounter(
            options.name,
            options.dimension,
            parseInt(options.increment, 10)
          );
          logger.success(`Counter incremented: ${options.name}`);
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

// Funnels command
program
  .command('funnels')
  .description('Manage funnels')
  .argument('[action]', 'Action: list, show, create, analyze', 'list')
  .argument('[id]', 'Funnel ID (for show/analyze)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-n, --name <name>', 'Funnel name (for create)')
  .option('-s, --steps <json>', 'Funnel steps JSON array (for create)')
  .option('-w, --window <hours>', 'Window hours (for create)', '24')
  .action(async (action, id, options) => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const funnels = await db.listFunnels(parseInt(options.limit, 10));
          console.log('\nFunnels:');
          console.log('-'.repeat(80));
          funnels.forEach(f => {
            console.log(`${f.id} | ${f.name} | ${f.steps.length} steps | ${f.enabled ? 'Enabled' : 'Disabled'}`);
          });
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Funnel ID required');
            process.exit(1);
          }
          const funnel = await db.getFunnel(id);
          if (!funnel) {
            logger.error('Funnel not found');
            process.exit(1);
          }
          console.log(JSON.stringify(funnel, null, 2));
          break;
        }
        case 'create': {
          if (!options.name || !options.steps) {
            logger.error('Name and steps required (--name, --steps)');
            process.exit(1);
          }
          const funnel: CreateFunnelRequest = {
            name: options.name,
            steps: JSON.parse(options.steps),
            window_hours: parseInt(options.window, 10),
          };
          const funnelId = await db.createFunnel(funnel);
          logger.success(`Funnel created: ${funnelId}`);
          break;
        }
        case 'analyze': {
          if (!id) {
            logger.error('Funnel ID required');
            process.exit(1);
          }
          const analysis = await db.analyzeFunnel(id);
          if (!analysis) {
            logger.error('Funnel not found');
            process.exit(1);
          }

          console.log('\nFunnel Analysis:');
          console.log('================');
          analysis.steps.forEach(step => {
            console.log(`Step ${step.step_number}: ${step.step_name}`);
            console.log(`  Users: ${step.users}`);
          });
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

// Quotas command
program
  .command('quotas')
  .description('Manage quotas')
  .argument('[action]', 'Action: list, create, check', 'list')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-n, --name <name>', 'Quota name (for create)')
  .option('-c, --counter <counter>', 'Counter name')
  .option('-m, --max <value>', 'Max value (for create)')
  .option('-p, --period <period>', 'Period (for create)')
  .option('-s, --scope <scope>', 'Scope (app/user/device)', 'app')
  .option('--scope-id <id>', 'Scope ID')
  .action(async (action, options) => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const quotas = await db.listQuotas(parseInt(options.limit, 10));
          console.log('\nQuotas:');
          console.log('-'.repeat(100));
          quotas.forEach(q => {
            console.log(`${q.name} | ${q.counter_name} | ${q.scope}:${q.scope_id ?? 'all'} | max: ${q.max_value} (${q.period}) | ${q.enabled ? 'Enabled' : 'Disabled'}`);
          });
          break;
        }
        case 'create': {
          if (!options.name || !options.counter || !options.max || !options.period) {
            logger.error('Name, counter, max, and period required');
            process.exit(1);
          }
          const quota: CreateQuotaRequest = {
            name: options.name,
            counter_name: options.counter,
            max_value: parseInt(options.max, 10),
            period: options.period as 'hourly' | 'daily' | 'monthly' | 'all_time',
            scope: options.scope as 'app' | 'user' | 'device',
            scope_id: options.scopeId,
          };
          const quotaId = await db.createQuota(quota);
          logger.success(`Quota created: ${quotaId}`);
          break;
        }
        case 'check': {
          if (!options.counter) {
            logger.error('Counter name required (--counter)');
            process.exit(1);
          }
          const result = await db.checkQuota(options.counter, options.scopeId ?? null, 1);
          console.log('\nQuota Check:');
          console.log('============');
          console.log(`Allowed: ${result.allowed ? 'Yes' : 'No'}`);
          console.log(`Current Value: ${result.currentValue}`);
          if (result.quota) {
            console.log(`Quota: ${result.quota.name}`);
            console.log(`Max Value: ${result.quota.max_value}`);
            console.log(`Action: ${result.quota.action_on_exceed}`);
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

// Rollup command
program
  .command('rollup')
  .description('Trigger counter rollup')
  .action(async () => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      logger.info('Starting counter rollup...');
      const result = await db.rollupCounters();

      console.log('\nRollup Results:');
      console.log('===============');
      console.log(`Daily:   ${result.daily} counters rolled up`);
      console.log(`Monthly: ${result.monthly} counters rolled up`);

      await db.disconnect();
      logger.success('Rollup complete');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Rollup failed', { error: message });
      process.exit(1);
    }
  });

// Dashboard command
program
  .command('dashboard')
  .description('Show analytics dashboard')
  .action(async () => {
    try {
      loadConfig();

      const db = new AnalyticsDatabase();
      await db.connect();

      const stats = await db.getDashboardStats();

      console.log('\nAnalytics Dashboard');
      console.log('===================');
      console.log(`Total Events:    ${stats.total_events}`);
      console.log(`Unique Users:    ${stats.unique_users}`);
      console.log(`Unique Sessions: ${stats.unique_sessions}`);
      console.log(`Active Quotas:   ${stats.active_quotas}`);
      console.log(`Violations (24h):${stats.quota_violations}`);

      console.log('\nTop Events:');
      stats.top_events.forEach(e => {
        console.log(`  - ${e.event_name}: ${e.count}`);
      });

      console.log('\nQuota Status:');
      stats.quota_status.forEach(q => {
        console.log(`  - ${q.quota_name}: ${q.current_value}/${q.max_value} (${q.usage_percent}%)`);
      });

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Dashboard failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
