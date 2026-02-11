#!/usr/bin/env node
/**
 * Devices Plugin CLI
 * Command-line interface for device enrollment, commands, and fleet management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { DevicesDatabase } from './database.js';
import { createServer } from './server.js';
import type { DeviceStatus, DeviceType, TelemetryType } from './types.js';

const logger = createLogger('devices:cli');

const program = new Command();

program
  .name('nself-devices')
  .description('Devices plugin for nself - IoT device enrollment, trust management, and command dispatch')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new DevicesDatabase();
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
  .option('-p, --port <port>', 'Server port', '3603')
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
  .description('Show fleet statistics')
  .action(async () => {
    try {
      const db = new DevicesDatabase();
      await db.connect();
      const stats = await db.getFleetStats();

      console.log('\nDevice Fleet Status');
      console.log('====================');
      console.log(`  Total devices:        ${stats.total_devices}`);
      console.log(`  Enrolled:             ${stats.enrolled_devices}`);
      console.log(`  Online:               ${stats.online_devices}`);
      console.log(`  Suspended:            ${stats.suspended_devices}`);
      console.log(`  Revoked:              ${stats.revoked_devices}`);
      console.log(`  Total commands:       ${stats.total_commands}`);
      console.log(`  Pending commands:     ${stats.pending_commands}`);
      console.log(`  Succeeded commands:   ${stats.succeeded_commands}`);
      console.log(`  Failed commands:      ${stats.failed_commands}`);
      console.log(`  Active ingest:        ${stats.active_ingest_sessions}`);
      console.log(`  Telemetry records:    ${stats.total_telemetry_records}`);
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

// Devices commands
const devices = program
  .command('devices')
  .description('Manage devices');

devices
  .command('list')
  .description('List all devices')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .option('-s, --status <status>', 'Filter by status')
  .option('-t, --type <type>', 'Filter by device type')
  .action(async (options) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();
      const list = await db.listDevices(
        undefined,
        options.status as DeviceStatus | undefined,
        options.type as DeviceType | undefined,
        undefined,
        parseInt(options.limit, 10)
      );

      console.log('\nDevices:');
      console.log('-'.repeat(130));
      for (const d of list) {
        const lastSeen = d.last_seen_at ? new Date(d.last_seen_at).toLocaleString() : 'never';
        console.log(
          `${String(d.id).substring(0, 8)}... | ${d.device_id.padEnd(20)} | ` +
          `${(d.name ?? 'unnamed').padEnd(15)} | ${d.device_type.padEnd(12)} | ` +
          `${d.status.padEnd(14)} | ${d.trust_level.padEnd(10)} | Last: ${lastSeen}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List devices failed', { error: message });
      process.exit(1);
    }
  });

devices
  .command('enroll')
  .description('Start enrollment for device')
  .argument('<deviceId>', 'Device UUID')
  .action(async (deviceId) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();

      const crypto = await import('node:crypto');
      const token = crypto.randomBytes(32).toString('hex');
      const challenge = crypto.randomBytes(32).toString('hex');

      const device = await db.startEnrollment(deviceId, token, challenge);
      if (!device) {
        logger.error('Device not found');
        process.exit(1);
      }

      logger.success(`Enrollment started for device: ${device.device_id}`);
      console.log(`  Token:     ${token}`);
      console.log(`  Challenge: ${challenge}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Enrollment failed', { error: message });
      process.exit(1);
    }
  });

devices
  .command('revoke')
  .description('Revoke device trust')
  .argument('<deviceId>', 'Device UUID')
  .requiredOption('--reason <reason>', 'Revocation reason')
  .action(async (deviceId, options) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();

      const device = await db.revokeDevice(deviceId, options.reason);
      if (!device) {
        logger.error('Device not found');
        process.exit(1);
      }

      logger.success(`Device revoked: ${device.device_id}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Revoke failed', { error: message });
      process.exit(1);
    }
  });

devices
  .command('suspend')
  .description('Suspend device')
  .argument('<deviceId>', 'Device UUID')
  .action(async (deviceId) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();

      const device = await db.suspendDevice(deviceId);
      if (!device) {
        logger.error('Device not found or not enrolled');
        process.exit(1);
      }

      logger.success(`Device suspended: ${device.device_id}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Suspend failed', { error: message });
      process.exit(1);
    }
  });

// Commands subcommand
const commands = program
  .command('commands')
  .description('Manage device commands');

commands
  .command('send')
  .description('Send command to device')
  .requiredOption('--device-id <deviceId>', 'Device UUID')
  .requiredOption('--action <action>', 'Command type (tune_channel, start_recording, stop_recording, reboot, etc.)')
  .option('--payload <json>', 'Command payload JSON', '{}')
  .option('--priority <priority>', 'Priority (low, normal, high, critical)', 'normal')
  .option('--timeout <seconds>', 'Timeout in seconds', '300')
  .action(async (options) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();

      const command = await db.dispatchCommand('default', options.deviceId, {
        device_id: options.deviceId,
        command_type: options.action,
        payload: JSON.parse(options.payload),
        priority: options.priority,
        timeout_seconds: parseInt(options.timeout, 10),
      }, parseInt(options.timeout, 10));

      logger.success(`Command dispatched: ${command.id}`);
      console.log(JSON.stringify(command, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Send command failed', { error: message });
      process.exit(1);
    }
  });

// Ingest commands
const ingest = program
  .command('ingest')
  .description('Manage ingest sessions');

ingest
  .command('sessions')
  .description('List all ingest sessions')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (options) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();
      const list = await db.listIngestSessions(undefined, undefined, parseInt(options.limit, 10));

      console.log('\nIngest Sessions:');
      console.log('-'.repeat(120));
      for (const s of list) {
        const kbps = s.bitrate_kbps ? `${s.bitrate_kbps} kbps` : 'N/A';
        console.log(
          `${String(s.id).substring(0, 8)}... | ${s.stream_id.padEnd(20)} | ` +
          `${s.status.padEnd(12)} | ${s.protocol.padEnd(6)} | ` +
          `${kbps.padEnd(12)} | Errors: ${s.error_count}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List ingest sessions failed', { error: message });
      process.exit(1);
    }
  });

ingest
  .command('active')
  .description('List active ingest sessions')
  .action(async () => {
    try {
      const db = new DevicesDatabase();
      await db.connect();
      const list = await db.getActiveIngestSessions();

      console.log('\nActive Ingest Sessions:');
      console.log('-'.repeat(120));
      for (const s of list) {
        const kbps = s.bitrate_kbps ? `${s.bitrate_kbps} kbps` : 'N/A';
        const uptime = s.started_at ? Math.round((Date.now() - new Date(s.started_at).getTime()) / 60000) : 0;
        console.log(
          `${String(s.id).substring(0, 8)}... | ${s.stream_id.padEnd(20)} | ` +
          `${s.status.padEnd(12)} | ${uptime}min | ${kbps} | ` +
          `${s.resolution ?? 'N/A'}`
        );
      }
      console.log(`\nTotal active: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List active ingest failed', { error: message });
      process.exit(1);
    }
  });

// Health command
program
  .command('health')
  .description('Show device health summary')
  .requiredOption('--device-id <deviceId>', 'Device UUID')
  .option('--type <type>', 'Telemetry type filter')
  .action(async (options) => {
    try {
      const db = new DevicesDatabase();
      await db.connect();

      if (options.type) {
        const telemetry = await db.getDeviceTelemetry(options.deviceId, options.type as TelemetryType, 20);
        console.log(`\nTelemetry: ${options.type} for device ${options.deviceId}`);
        console.log('-'.repeat(80));
        for (const t of telemetry) {
          console.log(`  ${new Date(t.recorded_at).toISOString()} | ${JSON.stringify(t.data)}`);
        }
        console.log(`\nTotal: ${telemetry.length}`);
      } else {
        const health = await db.getDeviceHealth(options.deviceId);
        if (!health) {
          logger.error('Device not found');
          process.exit(1);
        }

        console.log(`\nDevice Health: ${health.device_id}`);
        console.log('='.repeat(50));
        console.log(`  Name:              ${health.name ?? 'unnamed'}`);
        console.log(`  Status:            ${health.status}`);
        console.log(`  Trust level:       ${health.trust_level}`);
        console.log(`  Last seen:         ${health.last_seen_at ? new Date(health.last_seen_at).toISOString() : 'never'}`);
        console.log(`  Pending commands:  ${health.pending_commands}`);
        console.log(`  Active ingest:     ${health.active_ingest_sessions}`);

        if (health.recent_telemetry.length > 0) {
          console.log('\n  Recent Telemetry:');
          for (const t of health.recent_telemetry.slice(0, 10)) {
            console.log(`    ${t.telemetry_type.padEnd(20)} | ${new Date(t.recorded_at).toISOString()} | ${JSON.stringify(t.data).substring(0, 50)}`);
          }
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Health check failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show fleet-wide statistics')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new DevicesDatabase();
      await db.connect();
      const stats = await db.getFleetStats();

      console.log('\nDevice Fleet Statistics');
      console.log('=======================');
      console.log(`  Heartbeat interval:     ${config.heartbeatInterval}s`);
      console.log(`  Heartbeat timeout:      ${config.heartbeatTimeout}s`);
      console.log(`  Command timeout:        ${config.commandDefaultTimeout}s`);
      console.log(`  Command max retries:    ${config.commandMaxRetries}`);
      console.log(`  Telemetry retention:    ${config.telemetryRetentionDays} days`);
      console.log('\n  Fleet Stats:');
      console.log(`  Total devices:          ${stats.total_devices}`);
      console.log(`  Enrolled:               ${stats.enrolled_devices}`);
      console.log(`  Online:                 ${stats.online_devices}`);
      console.log(`  Suspended:              ${stats.suspended_devices}`);
      console.log(`  Revoked:                ${stats.revoked_devices}`);
      console.log(`  Active ingest:          ${stats.active_ingest_sessions}`);
      console.log(`  Telemetry records:      ${stats.total_telemetry_records}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
