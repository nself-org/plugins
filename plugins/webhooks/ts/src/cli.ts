#!/usr/bin/env node
/**
 * Webhooks Plugin CLI
 * Command-line interface for webhook management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { WebhooksDatabase } from './database.js';
import { WebhookDeliveryService } from './delivery.js';
import { createServer } from './server.js';
import { loadConfig } from './config.js';

const logger = createLogger('webhooks:cli');
const program = new Command();

program
  .name('nself-webhooks')
  .description('Outbound webhook delivery plugin for nself')
  .version('1.0.0');

// =========================================================================
// Init Command
// =========================================================================

program
  .command('init')
  .description('Initialize webhook database schema')
  .action(async () => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();
      logger.success('Webhook schema initialized successfully');
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Initialization failed:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Server Command
// =========================================================================

program
  .command('server')
  .description('Start webhook delivery server')
  .option('-p, --port <port>', 'Server port')
  .option('-h, --host <host>', 'Server host')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: options.port ? parseInt(options.port, 10) : undefined,
        host: options.host,
      });

      const app = await createServer(config);
      await app.listen({ port: config.port, host: config.host });
      logger.success(`Webhook server listening on ${config.host}:${config.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to start server:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Status Command
// =========================================================================

program
  .command('status')
  .description('Show webhook delivery status and statistics')
  .action(async () => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const stats = await db.getStats();
      const byEndpoint = await db.getDeliveryStatsByEndpoint();
      const byEventType = await db.getDeliveryStatsByEventType();

      await db.disconnect();

      console.log('\n=== Webhook Statistics ===\n');

      console.log('Endpoints:');
      console.log(`  Total: ${stats.endpoints.total}`);
      console.log(`  Enabled: ${stats.endpoints.enabled}`);
      console.log(`  Disabled: ${stats.endpoints.disabled}`);

      console.log('\nDeliveries:');
      console.log(`  Total: ${stats.deliveries.total}`);
      console.log(`  Pending: ${stats.deliveries.pending}`);
      console.log(`  Delivered: ${stats.deliveries.delivered}`);
      console.log(`  Failed: ${stats.deliveries.failed}`);
      console.log(`  Dead Letter: ${stats.deliveries.dead_letter}`);

      console.log('\nDead Letters:');
      console.log(`  Total: ${stats.dead_letters.total}`);
      console.log(`  Unresolved: ${stats.dead_letters.unresolved}`);
      console.log(`  Resolved: ${stats.dead_letters.resolved}`);

      console.log(`\nEvent Types: ${stats.event_types}`);

      if (byEndpoint.length > 0) {
        console.log('\n=== Delivery Stats by Endpoint ===\n');
        for (const stat of byEndpoint.slice(0, 10)) {
          console.log(`${stat.endpoint_url}:`);
          console.log(`  Total: ${stat.total_deliveries}, Success: ${stat.successful}, Failed: ${stat.failed}`);
          console.log(`  Success Rate: ${stat.success_rate}%, Avg Response: ${stat.avg_response_time_ms || 0}ms`);
        }
      }

      if (byEventType.length > 0) {
        console.log('\n=== Delivery Stats by Event Type ===\n');
        for (const stat of byEventType.slice(0, 10)) {
          console.log(`${stat.event_type}:`);
          console.log(`  Total: ${stat.total_deliveries}, Success: ${stat.successful}, Failed: ${stat.failed}`);
          console.log(`  Success Rate: ${stat.success_rate}%`);
        }
      }

      console.log('');
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to get status:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Endpoints Commands
// =========================================================================

const endpoints = program
  .command('endpoints')
  .description('Manage webhook endpoints');

endpoints
  .command('list')
  .description('List all webhook endpoints')
  .option('--enabled', 'Show only enabled endpoints')
  .option('--disabled', 'Show only disabled endpoints')
  .action(async (options) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      let filter: { enabled?: boolean } | undefined;
      if (options.enabled) filter = { enabled: true };
      if (options.disabled) filter = { enabled: false };

      const results = await db.listEndpoints(filter);
      await db.disconnect();

      console.log(`\nFound ${results.length} endpoint(s):\n`);
      for (const endpoint of results) {
        console.log(`ID: ${endpoint.id}`);
        console.log(`  URL: ${endpoint.url}`);
        console.log(`  Events: ${endpoint.events.join(', ')}`);
        console.log(`  Enabled: ${endpoint.enabled}`);
        console.log(`  Failures: ${endpoint.failure_count}`);
        if (endpoint.last_success_at) {
          console.log(`  Last Success: ${endpoint.last_success_at.toISOString()}`);
        }
        if (endpoint.description) {
          console.log(`  Description: ${endpoint.description}`);
        }
        console.log('');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to list endpoints:', { error: message });
      process.exit(1);
    }
  });

endpoints
  .command('create <url>')
  .description('Create a new webhook endpoint')
  .requiredOption('-e, --events <events>', 'Comma-separated list of event types')
  .option('-d, --description <description>', 'Endpoint description')
  .action(async (url, options) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const events = options.events.split(',').map((e: string) => e.trim());
      const endpoint = await db.createEndpoint({
        url,
        events,
        description: options.description,
      });

      await db.disconnect();

      console.log('\nEndpoint created successfully:\n');
      console.log(`ID: ${endpoint.id}`);
      console.log(`URL: ${endpoint.url}`);
      console.log(`Secret: ${endpoint.secret}`);
      console.log(`Events: ${endpoint.events.join(', ')}`);
      console.log('\nSave the secret securely - it will be used to sign webhook payloads.\n');
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to create endpoint:', { error: message });
      process.exit(1);
    }
  });

endpoints
  .command('delete <id>')
  .description('Delete a webhook endpoint')
  .action(async (id) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const deleted = await db.deleteEndpoint(id);
      await db.disconnect();

      if (deleted) {
        logger.success('Endpoint deleted successfully');
      } else {
        logger.error('Endpoint not found');
        process.exit(1);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to delete endpoint:', { error: message });
      process.exit(1);
    }
  });

endpoints
  .command('enable <id>')
  .description('Enable a webhook endpoint')
  .action(async (id) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const enabled = await db.enableEndpoint(id);
      await db.disconnect();

      if (enabled) {
        logger.success('Endpoint enabled successfully');
      } else {
        logger.error('Endpoint not found');
        process.exit(1);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to enable endpoint:', { error: message });
      process.exit(1);
    }
  });

endpoints
  .command('test <id>')
  .description('Send a test webhook to an endpoint')
  .action(async (id) => {
    try {
      const config = loadConfig();
      const db = new WebhooksDatabase();
      await db.connect();

      const deliveryService = new WebhookDeliveryService(db, config);
      const result = await deliveryService.testEndpoint(id);

      await db.disconnect();

      if (result.success) {
        console.log('\nTest webhook delivered successfully:');
        console.log(`  Status: ${result.status}`);
        console.log(`  Response Time: ${result.responseTime}ms\n`);
      } else {
        console.log('\nTest webhook failed:');
        console.log(`  Error: ${result.error}`);
        if (result.responseTime) {
          console.log(`  Response Time: ${result.responseTime}ms`);
        }
        console.log('');
        process.exit(1);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to test endpoint:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Deliveries Commands
// =========================================================================

const deliveries = program
  .command('deliveries')
  .description('Manage webhook deliveries');

deliveries
  .command('list')
  .description('List webhook deliveries')
  .option('-e, --endpoint <id>', 'Filter by endpoint ID')
  .option('-t, --event-type <type>', 'Filter by event type')
  .option('-s, --status <status>', 'Filter by status')
  .option('-l, --limit <limit>', 'Maximum number of results', '20')
  .action(async (options) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const results = await db.listDeliveries({
        endpointId: options.endpoint,
        eventType: options.eventType,
        status: options.status,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      console.log(`\nFound ${results.length} deliverie(s):\n`);
      for (const delivery of results) {
        console.log(`ID: ${delivery.id}`);
        console.log(`  Endpoint: ${delivery.endpoint_id}`);
        console.log(`  Event: ${delivery.event_type}`);
        console.log(`  Status: ${delivery.status}`);
        console.log(`  Attempts: ${delivery.attempt_count}/${delivery.max_attempts}`);
        if (delivery.response_status) {
          console.log(`  Response: ${delivery.response_status} (${delivery.response_time_ms}ms)`);
        }
        if (delivery.error_message) {
          console.log(`  Error: ${delivery.error_message}`);
        }
        console.log(`  Created: ${delivery.created_at.toISOString()}`);
        console.log('');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to list deliveries:', { error: message });
      process.exit(1);
    }
  });

deliveries
  .command('retry <id>')
  .description('Retry a failed delivery')
  .action(async (id) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const retried = await db.retryDelivery(id);
      await db.disconnect();

      if (retried) {
        logger.success('Delivery queued for retry');
      } else {
        logger.error('Delivery not found or cannot be retried');
        process.exit(1);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to retry delivery:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Dead Letter Commands
// =========================================================================

const deadLetter = program
  .command('dead-letter')
  .description('Manage dead letter queue');

deadLetter
  .command('list')
  .description('List dead letter items')
  .option('--resolved', 'Show only resolved items')
  .option('--unresolved', 'Show only unresolved items')
  .action(async (options) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      let filter: boolean | undefined;
      if (options.resolved) filter = true;
      if (options.unresolved) filter = false;

      const results = await db.listDeadLetters(filter);
      await db.disconnect();

      console.log(`\nFound ${results.length} dead letter(s):\n`);
      for (const item of results) {
        console.log(`ID: ${item.id}`);
        console.log(`  Endpoint: ${item.endpoint_id}`);
        console.log(`  Event: ${item.event_type}`);
        console.log(`  Attempts: ${item.attempt_count}`);
        console.log(`  Error: ${item.last_error}`);
        console.log(`  Resolved: ${item.resolved}`);
        console.log(`  Created: ${item.created_at.toISOString()}`);
        console.log('');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to list dead letters:', { error: message });
      process.exit(1);
    }
  });

deadLetter
  .command('resolve <id>')
  .description('Mark a dead letter as resolved')
  .action(async (id) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const resolved = await db.resolveDeadLetter(id);
      await db.disconnect();

      if (resolved) {
        logger.success('Dead letter marked as resolved');
      } else {
        logger.error('Dead letter not found');
        process.exit(1);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to resolve dead letter:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Event Types Commands
// =========================================================================

const eventTypes = program
  .command('event-types')
  .description('Manage event types');

eventTypes
  .command('list')
  .description('List registered event types')
  .action(async () => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const results = await db.listEventTypes();
      await db.disconnect();

      console.log(`\nFound ${results.length} event type(s):\n`);
      for (const eventType of results) {
        console.log(`Name: ${eventType.name}`);
        if (eventType.description) {
          console.log(`  Description: ${eventType.description}`);
        }
        if (eventType.source_plugin) {
          console.log(`  Source Plugin: ${eventType.source_plugin}`);
        }
        console.log('');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to list event types:', { error: message });
      process.exit(1);
    }
  });

eventTypes
  .command('register <name>')
  .description('Register a new event type')
  .option('-d, --description <description>', 'Event type description')
  .option('-p, --plugin <plugin>', 'Source plugin name')
  .action(async (name, options) => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const eventType = await db.registerEventType({
        name,
        description: options.description,
        source_plugin: options.plugin,
      });

      await db.disconnect();

      console.log('\nEvent type registered successfully:');
      console.log(`  Name: ${eventType.name}`);
      if (eventType.description) {
        console.log(`  Description: ${eventType.description}`);
      }
      console.log('');
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to register event type:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Dispatch Command
// =========================================================================

program
  .command('dispatch <event-type> <payload>')
  .description('Dispatch an event to matching endpoints')
  .option('-e, --endpoints <ids>', 'Comma-separated endpoint IDs')
  .action(async (eventType, payload, options) => {
    try {
      const config = loadConfig();
      const db = new WebhooksDatabase();
      await db.connect();

      const deliveryService = new WebhookDeliveryService(db, config);

      let parsedPayload: Record<string, unknown>;
      try {
        parsedPayload = JSON.parse(payload);
      } catch {
        logger.error('Invalid JSON payload');
        process.exit(1);
      }

      const endpoints = options.endpoints
        ? options.endpoints.split(',').map((e: string) => e.trim())
        : undefined;

      const result = await deliveryService.dispatchEvent({
        event_type: eventType,
        payload: parsedPayload,
        endpoints,
      });

      await db.disconnect();

      console.log(`\nEvent dispatched to ${result.dispatched} endpoint(s)\n`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to dispatch event:', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Stats Command
// =========================================================================

program
  .command('stats')
  .description('Show detailed delivery statistics')
  .action(async () => {
    try {
      const db = new WebhooksDatabase();
      await db.connect();

      const stats = await db.getStats();
      const byEndpoint = await db.getDeliveryStatsByEndpoint();
      const byEventType = await db.getDeliveryStatsByEventType();

      await db.disconnect();

      console.log('\n=== Webhook Statistics ===\n');

      console.log('Endpoints:');
      console.log(`  Total: ${stats.endpoints.total}`);
      console.log(`  Enabled: ${stats.endpoints.enabled}`);
      console.log(`  Disabled: ${stats.endpoints.disabled}`);

      console.log('\nDeliveries:');
      console.log(`  Total: ${stats.deliveries.total}`);
      console.log(`  Pending: ${stats.deliveries.pending}`);
      console.log(`  Delivered: ${stats.deliveries.delivered}`);
      console.log(`  Failed: ${stats.deliveries.failed}`);
      console.log(`  Dead Letter: ${stats.deliveries.dead_letter}`);

      if (stats.deliveries.total > 0) {
        const successRate = ((stats.deliveries.delivered / stats.deliveries.total) * 100).toFixed(2);
        console.log(`  Success Rate: ${successRate}%`);
      }

      console.log('\nDead Letters:');
      console.log(`  Total: ${stats.dead_letters.total}`);
      console.log(`  Unresolved: ${stats.dead_letters.unresolved}`);
      console.log(`  Resolved: ${stats.dead_letters.resolved}`);

      console.log(`\nEvent Types: ${stats.event_types}`);

      if (byEndpoint.length > 0) {
        console.log('\n=== Top Endpoints by Delivery Volume ===\n');
        for (const stat of byEndpoint.slice(0, 10)) {
          console.log(`${stat.endpoint_url}:`);
          console.log(`  Deliveries: ${stat.total_deliveries} (${stat.successful} success, ${stat.failed} failed)`);
          console.log(`  Success Rate: ${stat.success_rate}%`);
          console.log(`  Avg Response Time: ${stat.avg_response_time_ms || 0}ms`);
          console.log('');
        }
      }

      if (byEventType.length > 0) {
        console.log('=== Top Event Types by Delivery Volume ===\n');
        for (const stat of byEventType.slice(0, 10)) {
          console.log(`${stat.event_type}:`);
          console.log(`  Deliveries: ${stat.total_deliveries} (${stat.successful} success, ${stat.failed} failed)`);
          console.log(`  Success Rate: ${stat.success_rate}%`);
          console.log('');
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to get stats:', { error: message });
      process.exit(1);
    }
  });

// Parse CLI arguments
program.parse();
