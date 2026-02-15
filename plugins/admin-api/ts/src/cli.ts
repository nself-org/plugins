#!/usr/bin/env node
/**
 * Admin API Plugin CLI
 * Command-line interface for the Admin API plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { AdminApiDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('admin-api:cli');

const program = new Command();

program
  .name('nself-admin-api')
  .description('Admin API plugin for nself - system metrics, health, sessions, and storage')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new AdminApiDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for admin-api plugin');
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
  .description('Start the Admin API server')
  .option('-p, --port <port>', 'Server port', '3212')
  .option('-h, --host <host>', 'Server host', '127.0.0.1')
  .option('--ws', 'Enable WebSocket support')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
        wsEnabled: options.ws ?? false,
      });

      logger.info(`Starting Admin API server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Metrics command
program
  .command('metrics')
  .description('Show system metrics')
  .action(async () => {
    try {
      const db = new AdminApiDatabase();
      await db.connect();
      const latest = await db.getLatestSnapshot();
      await db.disconnect();

      if (!latest) {
        console.log('No metrics snapshots found. Run the server to collect metrics.');
        process.exit(0);
      }

      console.log('\nLatest Metrics Snapshot:');
      console.log('========================');
      console.log(`  Type:              ${latest.metric_type}`);
      if (latest.cpu_usage_percent !== null) {
        console.log(`  CPU Usage:         ${latest.cpu_usage_percent}%`);
      }
      if (latest.memory_used_bytes !== null && latest.memory_total_bytes !== null) {
        const memPct = ((Number(latest.memory_used_bytes) / Number(latest.memory_total_bytes)) * 100).toFixed(1);
        console.log(`  Memory:            ${memPct}% (${formatBytes(Number(latest.memory_used_bytes))} / ${formatBytes(Number(latest.memory_total_bytes))})`);
      }
      if (latest.active_connections !== null) {
        console.log(`  Active Connections: ${latest.active_connections}`);
      }
      if (latest.active_sessions !== null) {
        console.log(`  Active Sessions:   ${latest.active_sessions}`);
      }
      if (latest.request_count !== null) {
        console.log(`  Request Count:     ${latest.request_count}`);
      }
      if (latest.error_count !== null) {
        console.log(`  Error Count:       ${latest.error_count}`);
      }
      console.log(`  Captured At:       ${latest.created_at.toISOString()}`);

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Metrics check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Sessions command
program
  .command('sessions')
  .description('Show active database sessions')
  .action(async () => {
    try {
      const db = new AdminApiDatabase();
      await db.connect();
      const sessions = await db.getActiveSessions();
      await db.disconnect();

      console.log('\nActive Database Sessions:');
      console.log('=========================');
      console.log(`  Active:  ${sessions.total_active}`);
      console.log(`  Idle:    ${sessions.total_idle}`);
      console.log(`  Waiting: ${sessions.total_waiting}`);
      console.log(`  Max:     ${sessions.max_connections}`);

      if (sessions.sessions.length > 0) {
        console.log('\n  PID      State       Backend          Application');
        console.log('  -------  ----------  ---------------  ---------------');
        for (const s of sessions.sessions.slice(0, 20)) {
          const pid = String(s.pid).padEnd(7);
          const state = (s.state ?? 'unknown').padEnd(10);
          const backend = s.backend_type.padEnd(15);
          const appName = s.application_name || '-';
          console.log(`  ${pid}  ${state}  ${backend}  ${appName}`);
        }
        if (sessions.sessions.length > 20) {
          console.log(`  ... and ${sessions.sessions.length - 20} more`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sessions check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Storage command
program
  .command('storage')
  .description('Show storage breakdown')
  .action(async () => {
    try {
      const db = new AdminApiDatabase();
      await db.connect();
      const storage = await db.getStorageBreakdown();
      await db.disconnect();

      console.log('\nStorage Breakdown:');
      console.log('==================');
      console.log(`  Database: ${storage.database.name}`);
      console.log(`  Size:     ${storage.database.size_pretty}`);
      console.log(`  Tables:   ${storage.database.table_count}`);
      console.log(`  Indexes:  ${storage.database.index_count}`);

      if (storage.tables.length > 0) {
        console.log('\n  Top Tables by Size:');
        console.log('  Table                                    Size         Rows');
        console.log('  ---------------------------------------- ------------ ----------');
        for (const t of storage.tables.slice(0, 15)) {
          const name = `${t.schema_name}.${t.table_name}`.padEnd(40);
          const size = t.size_pretty.padEnd(12);
          const rows = String(t.row_estimate);
          console.log(`  ${name} ${size} ${rows}`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Storage check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Health command
program
  .command('health')
  .description('Show system health overview')
  .action(async () => {
    try {
      const db = new AdminApiDatabase();
      await db.connect();
      const dbHealth = await db.checkDatabaseHealth();
      await db.disconnect();

      console.log('\nSystem Health Overview:');
      console.log('=======================');
      console.log(`  Database Status:   ${dbHealth.status}`);
      console.log(`  Latency:           ${dbHealth.latency_ms}ms`);
      console.log(`  Connections:       ${dbHealth.connection_count} / ${dbHealth.max_connections}`);
      console.log(`  Version:           ${dbHealth.version}`);
      console.log(`  Process Uptime:    ${formatUptime(process.uptime())}`);

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Health check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .alias('status')
  .description('Show dashboard statistics')
  .action(async () => {
    try {
      const db = new AdminApiDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nAdmin API Statistics:');
      console.log('=====================');
      console.log(`  Snapshots Total:     ${stats.snapshots_total}`);
      console.log(`  Snapshots Today:     ${stats.snapshots_today}`);
      console.log(`  Config Entries:      ${stats.config_entries}`);
      if (stats.avg_cpu_24h !== null) {
        console.log(`  Avg CPU (24h):       ${stats.avg_cpu_24h.toFixed(1)}%`);
      }
      if (stats.avg_memory_24h !== null) {
        console.log(`  Avg Memory (24h):    ${stats.avg_memory_24h.toFixed(1)}%`);
      }
      if (stats.peak_connections_24h !== null) {
        console.log(`  Peak Connections:    ${stats.peak_connections_24h}`);
      }
      if (stats.total_requests_24h !== null) {
        console.log(`  Total Requests (24h): ${stats.total_requests_24h}`);
      }
      if (stats.total_errors_24h !== null) {
        console.log(`  Total Errors (24h):  ${stats.total_errors_24h}`);
      }
      if (stats.oldest_snapshot) {
        console.log(`  Oldest Snapshot:     ${stats.oldest_snapshot}`);
      }
      if (stats.newest_snapshot) {
        console.log(`  Newest Snapshot:     ${stats.newest_snapshot}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// =========================================================================
// Utility Functions
// =========================================================================

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  parts.push(`${secs}s`);

  return parts.join(' ');
}

program.parse();
