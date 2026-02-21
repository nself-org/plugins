#!/usr/bin/env node
/**
 * mDNS Plugin CLI
 * Command-line interface for the mDNS plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { MdnsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('mdns:cli');

const program = new Command();

program
  .name('nself-mdns')
  .description('mDNS plugin for nself - service discovery and zero-config LAN advertising')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new MdnsDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for mDNS plugin');
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
  .description('Start the mDNS API server')
  .option('-p, --port <port>', 'Server port', '3216')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting mDNS server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Advertise command
program
  .command('advertise')
  .description('Start advertising a service')
  .requiredOption('-n, --name <name>', 'Service name')
  .option('-t, --type <type>', 'Service type', '_ntv._tcp')
  .requiredOption('--port <port>', 'Service port')
  .option('--host <host>', 'Service host', 'localhost')
  .option('-d, --domain <domain>', 'mDNS domain', 'local')
  .action(async (options) => {
    try {
      const db = new MdnsDatabase();
      await db.connect();

      const service = await db.createService({
        source_account_id: 'primary',
        service_name: options.name,
        service_type: options.type,
        port: parseInt(options.port, 10),
        host: options.host,
        domain: options.domain,
        txt_records: {},
        is_advertised: true,
        is_active: true,
        last_seen_at: new Date(),
        metadata: {},
      });

      await db.disconnect();

      console.log(`\nService registered and advertising:`);
      console.log(`  Name:   ${service.service_name}`);
      console.log(`  Type:   ${service.service_type}`);
      console.log(`  Host:   ${service.host}`);
      console.log(`  Port:   ${service.port}`);
      console.log(`  Domain: ${service.domain}`);
      console.log(`  ID:     ${service.id}`);

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Advertise failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Discover command
program
  .command('discover')
  .description('Discover services on the network')
  .option('-t, --type <type>', 'Service type to discover', '_ntv._tcp')
  .action(async (options) => {
    try {
      const db = new MdnsDatabase();
      await db.connect();

      const discoveries = await db.listDiscoveries({
        serviceType: options.type,
        isAvailable: true,
      });

      await db.disconnect();

      if (discoveries.length === 0) {
        console.log(`No services of type "${options.type}" discovered`);
        process.exit(0);
      }

      console.log(`\nDiscovered ${discoveries.length} service(s) of type "${options.type}":\n`);
      for (const svc of discoveries) {
        console.log(`  ${svc.service_name}`);
        console.log(`    Host:      ${svc.host}:${svc.port}`);
        if (svc.addresses.length > 0) {
          console.log(`    Addresses: ${svc.addresses.join(', ')}`);
        }
        console.log(`    Last Seen: ${svc.last_seen_at.toISOString()}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Discovery failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Services command
program
  .command('services')
  .description('List advertised services')
  .option('-t, --type <type>', 'Filter by service type')
  .option('--advertised-only', 'Show only advertised services')
  .action(async (options) => {
    try {
      const db = new MdnsDatabase();
      await db.connect();

      const services = await db.listServices({
        serviceType: options.type,
        isAdvertised: options.advertisedOnly ? true : undefined,
      });

      await db.disconnect();

      if (services.length === 0) {
        console.log('No services found');
        process.exit(0);
      }

      console.log(`\nFound ${services.length} service(s):\n`);
      for (const svc of services) {
        const advertised = svc.is_advertised ? ' [ADVERTISING]' : '';
        const active = svc.is_active ? '' : ' [inactive]';

        console.log(`  ${svc.service_name}${advertised}${active}`);
        console.log(`    Type:   ${svc.service_type}`);
        console.log(`    Host:   ${svc.host}:${svc.port}`);
        console.log(`    Domain: ${svc.domain}`);
        console.log(`    ID:     ${svc.id}`);
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

// Stats command
program
  .command('stats')
  .alias('status')
  .description('Show mDNS statistics')
  .action(async () => {
    try {
      const db = new MdnsDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nmDNS Statistics:');
      console.log('================');
      console.log(`Total Services:      ${stats.total_services}`);
      console.log(`Advertised:          ${stats.advertised_services}`);
      console.log(`Active:              ${stats.active_services}`);
      console.log(`Total Discovered:    ${stats.total_discovered}`);
      console.log(`Available:           ${stats.available_discovered}`);
      if (stats.last_discovery_at) {
        console.log(`Last Discovery:      ${stats.last_discovery_at.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
