#!/usr/bin/env node
/**
 * Observability Plugin CLI
 * Command-line interface for the observability plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ObservabilityDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('observability:cli');

const program = new Command();

program
  .name('nself-observability')
  .description('Observability plugin for nself - health probes, watchdog, and service discovery')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new ObservabilityDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for observability plugin');
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
  .description('Start the observability API server')
  .option('-p, --port <port>', 'Server port', '3215')
  .option('-h, --host <host>', 'Server host', '127.0.0.1')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting observability server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .alias('status')
  .description('Show observability statistics')
  .action(async () => {
    try {
      const db = new ObservabilityDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nObservability Statistics:');
      console.log('========================');
      console.log(`Total Services:     ${stats.total_services}`);
      console.log(`Healthy Services:   ${stats.healthy_services}`);
      console.log(`Unhealthy Services: ${stats.unhealthy_services}`);
      console.log(`Degraded Services:  ${stats.degraded_services}`);
      console.log(`Health Checks:      ${stats.total_health_checks}`);
      console.log(`Watchdog Events:    ${stats.total_watchdog_events}`);
      if (stats.oldest_service) {
        console.log(`Oldest Service:     ${stats.oldest_service.toISOString()}`);
      }
      if (stats.newest_service) {
        console.log(`Newest Service:     ${stats.newest_service.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Services command
program
  .command('services')
  .description('List discovered services')
  .option('-s, --state <state>', 'Filter by state (discovered, healthy, unhealthy, degraded, unknown, removed)')
  .option('-t, --type <type>', 'Filter by service type (docker, manual)')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new ObservabilityDatabase();
      await db.connect();

      const services = await db.listServices({
        state: options.state,
        serviceType: options.type,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (services.length === 0) {
        console.log('No services found');
        process.exit(0);
      }

      console.log(`\nFound ${services.length} service(s):\n`);
      for (const svc of services) {
        const port = svc.port ? `:${svc.port}` : '';
        const container = svc.container_name ? ` (${svc.container_name})` : '';
        const stateIcon = svc.state === 'healthy' ? '[OK]'
          : svc.state === 'unhealthy' ? '[FAIL]'
          : svc.state === 'degraded' ? '[WARN]'
          : `[${svc.state.toUpperCase()}]`;

        console.log(`  ${stateIcon} ${svc.name}${container}`);
        console.log(`    Host:     ${svc.host}${port}`);
        console.log(`    Type:     ${svc.service_type}`);
        console.log(`    ID:       ${svc.id}`);
        if (svc.image) console.log(`    Image:    ${svc.image}`);
        if (svc.last_health_check) console.log(`    Checked:  ${svc.last_health_check.toISOString()}`);
        if (svc.consecutive_failures > 0) console.log(`    Failures: ${svc.consecutive_failures}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list services', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Health command
program
  .command('health')
  .description('Check all services health')
  .action(async () => {
    try {
      const config = loadConfig();
      const port = config.port;
      const host = config.host === '0.0.0.0' ? '127.0.0.1' : config.host;

      console.log('Triggering health check on all services...');

      const response = await fetch(`http://${host}:${port}/api/v1/health/check-all`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${await response.text()}`);
      }

      const data = await response.json() as {
        results: Array<{
          service_name: string;
          status: string;
          response_time_ms: number | null;
          error_message: string | null;
        }>;
        count: number;
      };

      console.log(`\nHealth check results (${data.count} services):\n`);
      for (const result of data.results) {
        const icon = result.status === 'healthy' ? '[OK]'
          : result.status === 'unhealthy' ? '[FAIL]'
          : result.status === 'degraded' ? '[WARN]'
          : `[${result.status.toUpperCase()}]`;
        const time = result.response_time_ms !== null ? ` (${result.response_time_ms}ms)` : '';
        const err = result.error_message ? ` - ${result.error_message}` : '';

        console.log(`  ${icon} ${result.service_name}${time}${err}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Health check failed', { error: message });
      console.error('Error:', message);
      console.error('Is the observability server running?');
      process.exit(1);
    }
  });

// Watchdog command
program
  .command('watchdog')
  .description('Show watchdog status')
  .action(async () => {
    try {
      const config = loadConfig();
      const port = config.port;
      const host = config.host === '0.0.0.0' ? '127.0.0.1' : config.host;

      const response = await fetch(`http://${host}:${port}/api/v1/watchdog`);

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${await response.text()}`);
      }

      const status = await response.json() as {
        enabled: boolean;
        running: boolean;
        check_interval_seconds: number;
        timeout_seconds: number;
        services_monitored: number;
        last_check: string | null;
        uptime_seconds: number;
      };

      console.log('\nWatchdog Status:');
      console.log('================');
      console.log(`Enabled:            ${status.enabled ? 'Yes' : 'No'}`);
      console.log(`Running:            ${status.running ? 'Yes' : 'No'}`);
      console.log(`Check Interval:     ${status.check_interval_seconds}s`);
      console.log(`Timeout:            ${status.timeout_seconds}s`);
      console.log(`Services Monitored: ${status.services_monitored}`);
      if (status.last_check) {
        console.log(`Last Check:         ${status.last_check}`);
      }
      if (status.uptime_seconds > 0) {
        const hours = Math.floor(status.uptime_seconds / 3600);
        const minutes = Math.floor((status.uptime_seconds % 3600) / 60);
        const seconds = status.uptime_seconds % 60;
        console.log(`Uptime:             ${hours}h ${minutes}m ${seconds}s`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get watchdog status', { error: message });
      console.error('Error:', message);
      console.error('Is the observability server running?');
      process.exit(1);
    }
  });

// Events command
program
  .command('events')
  .description('Show recent watchdog events')
  .option('-l, --limit <limit>', 'Number of events', '20')
  .option('-s, --severity <severity>', 'Filter by severity (info, warning, error, critical)')
  .option('-t, --type <type>', 'Filter by event type')
  .action(async (options) => {
    try {
      const db = new ObservabilityDatabase();
      await db.connect();

      const events = await db.listWatchdogEvents({
        severity: options.severity,
        eventType: options.type,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (events.length === 0) {
        console.log('No events found');
        process.exit(0);
      }

      console.log(`\nRecent Events (${events.length}):\n`);
      for (const event of events) {
        const time = event.created_at.toISOString();
        const sev = event.severity.toUpperCase().padEnd(8);

        console.log(`  [${sev}] ${time}`);
        console.log(`    Type:    ${event.event_type}`);
        console.log(`    Message: ${event.message}`);
        if (event.service_id) console.log(`    Service: ${event.service_id}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list events', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
