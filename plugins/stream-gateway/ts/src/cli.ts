#!/usr/bin/env node
/**
 * Stream Gateway Plugin CLI
 * Command-line interface for stream admission and governance
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { StreamGatewayDatabase } from './database.js';
import { createServer } from './server.js';
import type { StreamStatus } from './types.js';

const logger = createLogger('stream-gateway:cli');

const program = new Command();

program
  .name('nself-stream-gateway')
  .description('Stream Gateway plugin for nself - stream admission and governance service')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new StreamGatewayDatabase();
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
  .option('-p, --port <port>', 'Server port', '3601')
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
  .description('Show gateway status')
  .action(async () => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const stats = await db.getGatewayStats();

      console.log('\nStream Gateway Status');
      console.log('=====================');
      console.log(`  Total streams:        ${stats.total_streams}`);
      console.log(`  Active streams:       ${stats.active_streams}`);
      console.log(`  Total sessions:       ${stats.total_sessions}`);
      console.log(`  Active sessions:      ${stats.active_sessions}`);
      console.log(`  Denied sessions:      ${stats.denied_sessions}`);
      console.log(`  Total rules:          ${stats.total_rules}`);
      console.log(`  Active rules:         ${stats.active_rules}`);
      console.log(`  Peak concurrent:      ${stats.peak_concurrent_viewers}`);
      if (stats.last_activity) {
        console.log(`  Last activity:        ${new Date(stats.last_activity).toISOString()}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Streams commands
const streams = program
  .command('streams')
  .description('Manage streams');

streams
  .command('list')
  .description('List all streams')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .option('-s, --status <status>', 'Filter by status')
  .action(async (options: { limit: string; status?: StreamStatus }) => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const list = await db.listStreams(undefined, options.status, parseInt(options.limit, 10));

      console.log('\nStreams:');
      console.log('-'.repeat(120));
      for (const s of list) {
        console.log(
          `${s.stream_id.padEnd(30)} | ${(s.status ?? '').padEnd(10)} | ` +
          `${s.stream_type.padEnd(8)} | Viewers: ${s.current_viewers}/${s.max_viewers ?? 'unlimited'} | ` +
          `${s.title ?? 'No title'}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List streams failed', { error: message });
      process.exit(1);
    }
  });

streams
  .command('active')
  .description('List active streams with viewer counts')
  .action(async () => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const list = await db.listStreams(undefined, 'active');

      console.log('\nActive Streams:');
      console.log('-'.repeat(120));
      for (const s of list) {
        console.log(
          `${s.stream_id.padEnd(30)} | Viewers: ${s.current_viewers} (peak: ${s.peak_viewers}) | ` +
          `${s.title ?? 'No title'}`
        );
      }
      console.log(`\nTotal active: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List active streams failed', { error: message });
      process.exit(1);
    }
  });

// Sessions commands
const sessions = program
  .command('sessions')
  .description('Manage sessions');

sessions
  .command('list')
  .description('List all sessions')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .option('-s, --status <status>', 'Filter by status')
  .action(async (options) => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const list = await db.listSessions(undefined, options.status, parseInt(options.limit, 10));

      console.log('\nSessions:');
      console.log('-'.repeat(120));
      for (const s of list) {
        console.log(
          `${String(s.id).substring(0, 8)}... | ${s.stream_id.padEnd(25)} | ` +
          `${s.user_id.padEnd(20)} | ${s.status.padEnd(8)} | ` +
          `${s.device_type ?? 'unknown'}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List sessions failed', { error: message });
      process.exit(1);
    }
  });

sessions
  .command('active')
  .description('List active sessions')
  .action(async () => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const list = await db.getActiveSessions();

      console.log('\nActive Sessions:');
      console.log('-'.repeat(120));
      for (const s of list) {
        const elapsed = s.started_at ? Math.round((Date.now() - new Date(s.started_at).getTime()) / 60000) : 0;
        console.log(
          `${String(s.id).substring(0, 8)}... | ${s.stream_id.padEnd(25)} | ` +
          `${s.user_id.padEnd(20)} | ${elapsed}min | ${s.quality}`
        );
      }
      console.log(`\nTotal active: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List active sessions failed', { error: message });
      process.exit(1);
    }
  });

// Admit command
program
  .command('admit')
  .description('Manually admit user to stream')
  .requiredOption('--stream-id <streamId>', 'Stream ID')
  .requiredOption('--user-id <userId>', 'User ID')
  .option('--device-id <deviceId>', 'Device ID')
  .option('--device-type <deviceType>', 'Device type')
  .option('--quality <quality>', 'Quality', 'auto')
  .action(async (options) => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();

      const session = await db.createSession('default', {
        stream_id: options.streamId,
        user_id: options.userId,
        device_id: options.deviceId,
        device_type: options.deviceType,
        quality: options.quality,
      }, 'active');

      logger.success(`User admitted. Session ID: ${session.id}`);
      console.log(JSON.stringify(session, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Admit failed', { error: message });
      process.exit(1);
    }
  });

// Evict command
program
  .command('evict')
  .description('Evict a session')
  .requiredOption('--session-id <sessionId>', 'Session ID')
  .action(async (options) => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();

      const session = await db.evictSession(options.sessionId);
      if (!session) {
        logger.error('Active session not found');
        process.exit(1);
      }

      logger.success(`Session evicted: ${session.id}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evict failed', { error: message });
      process.exit(1);
    }
  });

// Rules commands
const rules = program
  .command('rules')
  .description('Manage admission rules');

rules
  .command('list')
  .description('List admission rules')
  .option('--active', 'Show only active rules')
  .action(async (options) => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const list = await db.listRules(undefined, options.active ?? false);

      console.log('\nAdmission Rules:');
      console.log('-'.repeat(100));
      for (const r of list) {
        console.log(
          `${String(r.id).substring(0, 8)}... | ${r.name.padEnd(30)} | ` +
          `${r.rule_type.padEnd(18)} | ${r.action.padEnd(6)} | ` +
          `Priority: ${r.priority} | ${r.active ? 'ACTIVE' : 'DISABLED'}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List rules failed', { error: message });
      process.exit(1);
    }
  });

rules
  .command('create')
  .description('Create admission rule')
  .requiredOption('--name <name>', 'Rule name')
  .requiredOption('--type <type>', 'Rule type (concurrent_limit, device_limit, geo_block, time_window, user_role)')
  .requiredOption('--conditions <json>', 'Conditions JSON')
  .option('--action <action>', 'Action (allow/deny)', 'deny')
  .option('--priority <priority>', 'Priority', '0')
  .action(async (options) => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();

      const rule = await db.createRule('default', {
        name: options.name,
        rule_type: options.type,
        conditions: JSON.parse(options.conditions),
        action: options.action,
        priority: parseInt(options.priority, 10),
      });

      logger.success(`Rule created: ${rule.id}`);
      console.log(JSON.stringify(rule, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create rule failed', { error: message });
      process.exit(1);
    }
  });

// Analytics commands
const analytics = program
  .command('analytics')
  .description('View analytics');

analytics
  .command('summary')
  .description('Show overall analytics')
  .action(async () => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const summary = await db.getAnalyticsSummary();

      console.log('\nStream Analytics Summary');
      console.log('========================');
      console.log(`  Total streams:           ${summary.total_streams}`);
      console.log(`  Total view minutes:      ${summary.total_view_minutes.toFixed(1)}`);
      console.log(`  Avg viewers per stream:  ${summary.avg_viewers_per_stream.toFixed(1)}`);
      console.log(`  Peak viewers:            ${summary.peak_viewers}`);
      console.log(`  Unique viewers:          ${summary.unique_viewers}`);

      if (summary.top_streams.length > 0) {
        console.log('\n  Top Streams:');
        for (const s of summary.top_streams) {
          console.log(`    ${s.stream_id.padEnd(30)} | ${s.total_view_minutes.toFixed(1)} min | Peak: ${s.peak_viewers}`);
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Analytics failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show gateway statistics')
  .action(async () => {
    try {
      const db = new StreamGatewayDatabase();
      await db.connect();
      const stats = await db.getGatewayStats();
      const config = loadConfig();

      console.log('\nStream Gateway Statistics');
      console.log('=========================');
      console.log(`  Heartbeat interval:   ${config.heartbeatInterval}s`);
      console.log(`  Heartbeat timeout:    ${config.heartbeatTimeout}s`);
      console.log(`  Max concurrent:       ${config.defaultMaxConcurrent}`);
      console.log(`  Max device streams:   ${config.defaultMaxDeviceStreams}`);
      console.log('\n  Gateway Stats:');
      console.log(`  Total streams:        ${stats.total_streams}`);
      console.log(`  Active streams:       ${stats.active_streams}`);
      console.log(`  Total sessions:       ${stats.total_sessions}`);
      console.log(`  Active sessions:      ${stats.active_sessions}`);
      console.log(`  Denied sessions:      ${stats.denied_sessions}`);
      console.log(`  Rules:                ${stats.active_rules}/${stats.total_rules} active`);
      console.log(`  Peak concurrent:      ${stats.peak_concurrent_viewers}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
