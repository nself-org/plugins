#!/usr/bin/env node
/**
 * Observability Plugin CLI
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { startServer } from './server.js';
import { loadConfig } from './config.js';

const logger = createLogger('observability:cli');
const program = new Command();

program
  .name('nself-observability')
  .description('Observability plugin for nself with Prometheus, Loki, and Tempo')
  .version('1.0.0');

program
  .command('server')
  .description('Start the observability server')
  .option('-p, --port <port>', 'Server port', '3215')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options: { port?: string; host?: string }) => {
    try {
      logger.info('Starting observability server...');

      const config = loadConfig({
        port: options.port ? parseInt(options.port, 10) : undefined,
        host: options.host,
      });

      await startServer(config);

      logger.info(`Server started on ${config.host}:${config.port}`);
      logger.info(`Prometheus metrics: http://${config.host}:${config.port}${config.metricsPath}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start server', { error: message });
      process.exit(1);
    }
  });

program
  .command('status')
  .description('Check observability services status')
  .action(async () => {
    try {
      const config = loadConfig();

      logger.info('Observability Plugin Status:');
      logger.info(`- Prometheus: ${config.prometheusEnabled ? 'Enabled' : 'Disabled'}`);
      logger.info(`- Loki: ${config.lokiEnabled ? 'Enabled' : 'Disabled'} (${config.lokiUrl})`);
      logger.info(`- Tempo: ${config.tempoEnabled ? 'Enabled' : 'Disabled'} (${config.tempoUrl})`);
      logger.info(`- Grafana: ${config.grafanaUrl}`);

      if (config.lokiEnabled) {
        try {
          const response = await fetch(`${config.lokiUrl}/ready`);
          logger.info(`- Loki Status: ${response.ok ? 'Ready' : 'Not Ready'}`);
        } catch {
          logger.warn('- Loki Status: Unreachable');
        }
      }

      if (config.tempoEnabled) {
        try {
          const response = await fetch(`${config.tempoUrl}/ready`);
          logger.info(`- Tempo Status: ${response.ok ? 'Ready' : 'Not Ready'}`);
        } catch {
          logger.warn('- Tempo Status: Unreachable');
        }
      }

      if (config.grafanaApiKey) {
        try {
          const response = await fetch(`${config.grafanaUrl}/api/health`, {
            headers: { Authorization: `Bearer ${config.grafanaApiKey}` },
          });
          logger.info(`- Grafana Status: ${response.ok ? 'Ready' : 'Not Ready'}`);
        } catch {
          logger.warn('- Grafana Status: Unreachable');
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to check status', { error: message });
      process.exit(1);
    }
  });

program
  .command('test-log')
  .description('Send a test log entry')
  .option('-l, --level <level>', 'Log level (debug|info|warn|error)', 'info')
  .option('-m, --message <message>', 'Log message', 'Test log message')
  .option('-p, --port <port>', 'Server port', '3215')
  .action(async (options: { level?: string; message?: string; port?: string }) => {
    try {
      const port = options.port ?? '3215';
      const level = options.level ?? 'info';
      const message = options.message ?? 'Test log message';

      logger.info('Sending test log entry...');

      const response = await fetch(`http://localhost:${port}/api/observability/logs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          level,
          message,
          timestamp: new Date().toISOString(),
          metadata: { test: true },
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${await response.text()}`);
      }

      logger.info('Test log sent successfully');
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to send test log', { error: errorMessage });
      process.exit(1);
    }
  });

program
  .command('test-trace')
  .description('Send a test trace span')
  .option('-o, --operation <name>', 'Operation name', 'test_operation')
  .option('-p, --port <port>', 'Server port', '3215')
  .action(async (options: { operation?: string; port?: string }) => {
    try {
      const port = options.port ?? '3215';
      const operation = options.operation ?? 'test_operation';

      logger.info('Sending test trace span...');

      const traceId = Array.from({ length: 32 }, () =>
        Math.floor(Math.random() * 16).toString(16)
      ).join('');
      const spanId = Array.from({ length: 16 }, () =>
        Math.floor(Math.random() * 16).toString(16)
      ).join('');

      const startTime = new Date();
      const endTime = new Date(startTime.getTime() + 1000);

      const response = await fetch(`http://localhost:${port}/api/observability/traces`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          trace_id: traceId,
          span_id: spanId,
          operation_name: operation,
          start_time: startTime.toISOString(),
          end_time: endTime.toISOString(),
          duration_ms: 1000,
          tags: { test: 'true' },
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${await response.text()}`);
      }

      logger.info(`Test trace sent successfully (trace_id: ${traceId})`);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to send test trace', { error: errorMessage });
      process.exit(1);
    }
  });

program
  .command('metrics')
  .description('Fetch current metrics')
  .option('-p, --port <port>', 'Server port', '3215')
  .action(async (options: { port?: string }) => {
    try {
      const port = options.port ?? '3215';

      const response = await fetch(`http://localhost:${port}/metrics`);

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${await response.text()}`);
      }

      const metrics = await response.text();
      console.log(metrics);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to fetch metrics', { error: errorMessage });
      process.exit(1);
    }
  });

program.parse();
