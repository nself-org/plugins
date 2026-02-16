#!/usr/bin/env node
/**
 * LiveKit Plugin CLI
 * Command-line interface for the LiveKit plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { AccessToken } from 'livekit-server-sdk';
import { loadConfig } from './config.js';
import { LiveKitDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('livekit:cli');

const program = new Command();

program
  .name('nself-livekit')
  .description('LiveKit plugin for nself - Voice/Video infrastructure management')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize LiveKit plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing LiveKit schema...');
      const db = new LiveKitDatabase();
      await db.connect();
      await db.initializeSchema();
      console.log('Schema initialized successfully');
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start LiveKit plugin server')
  .option('-p, --port <port>', 'Server port', '3707')
  .action(async (options) => {
    try {
      logger.info('Starting LiveKit server...');
      await startServer({ port: parseInt(options.port, 10) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show LiveKit plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new LiveKitDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nLiveKit Plugin Status');
      console.log('=====================');
      console.log(`LiveKit URL:           ${config.livekitUrl}`);
      console.log(`Egress Enabled:        ${config.egressEnabled}`);
      console.log(`Quality Monitoring:    ${config.qualityMonitoringEnabled}`);
      console.log(`Total Rooms:           ${stats.totalRooms}`);
      console.log(`Active Rooms:          ${stats.activeRooms}`);
      console.log(`Total Participants:    ${stats.totalParticipants}`);
      console.log(`Active Participants:   ${stats.activeParticipants}`);
      console.log(`Total Egress Jobs:     ${stats.totalEgressJobs}`);
      console.log(`Active Egress Jobs:    ${stats.activeEgressJobs}`);
      console.log(`Tokens Issued:         ${stats.totalTokensIssued}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Rooms: list
program
  .command('rooms:list')
  .description('List LiveKit rooms')
  .option('-s, --status <status>', 'Filter by status')
  .option('-t, --type <type>', 'Filter by room type')
  .option('-l, --limit <limit>', 'Result limit', '20')
  .action(async (options) => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();

      const rooms = await db.listRooms({
        status: options.status,
        roomType: options.type,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nLiveKit Rooms (${rooms.length}):`);
      console.log('========================');

      if (rooms.length === 0) {
        console.log('No rooms found.');
      } else {
        for (const room of rooms) {
          console.log(`- ${room.livekit_room_name} [${room.status}] (${room.room_type}, max: ${room.max_participants})`);
          if (room.livekit_room_sid) {
            console.log(`  SID: ${room.livekit_room_sid}`);
          }
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list rooms', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Rooms: info
program
  .command('rooms:info')
  .description('Get room details')
  .argument('<room-name>', 'Room name')
  .action(async (roomName) => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();

      const room = await db.getRoomByName(roomName);
      if (!room) {
        console.log(`Room not found: ${roomName}`);
        await db.disconnect();
        process.exit(1);
        return;
      }

      console.log(`\nRoom: ${room.livekit_room_name}`);
      console.log('============================');
      console.log(`ID:               ${room.id}`);
      console.log(`SID:              ${room.livekit_room_sid ?? 'N/A'}`);
      console.log(`Type:             ${room.room_type}`);
      console.log(`Status:           ${room.status}`);
      console.log(`Max Participants: ${room.max_participants}`);
      console.log(`Empty Timeout:    ${room.empty_timeout}s`);
      console.log(`Created:          ${room.created_at}`);

      const participants = await db.listParticipants(room.id);
      console.log(`\nParticipants (${participants.length}):`);
      for (const p of participants) {
        console.log(`  - ${p.display_name ?? p.livekit_identity} [${p.status}]`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get room info', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Rooms: close
program
  .command('rooms:close')
  .description('Close a room')
  .argument('<room-name>', 'Room name')
  .action(async (roomName) => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();

      const room = await db.getRoomByName(roomName);
      if (!room) {
        console.log(`Room not found: ${roomName}`);
        await db.disconnect();
        process.exit(1);
        return;
      }

      await db.closeRoom(room.id);
      console.log(`Room closed: ${roomName}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to close room', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Rooms: cleanup
program
  .command('rooms:cleanup')
  .description('Clean up stale rooms')
  .option('--older-than <duration>', 'Cleanup rooms older than duration (e.g., 1h)', '1h')
  .action(async (options) => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();

      const rooms = await db.listRooms({ status: 'active' });
      const durationMs = parseDuration(options.olderThan);
      const cutoff = new Date(Date.now() - durationMs);
      let closed = 0;

      for (const room of rooms) {
        if (new Date(room.created_at) < cutoff) {
          await db.closeRoom(room.id);
          closed++;
        }
      }

      console.log(`Cleaned up ${closed} stale rooms`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to cleanup rooms', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Token: create
program
  .command('token:create')
  .description('Generate access token')
  .argument('<room-name>', 'Room name')
  .argument('<identity>', 'Participant identity')
  .option('--ttl <seconds>', 'Token TTL in seconds', '3600')
  .option('--name <name>', 'Participant display name', '')
  .option('--can-publish', 'Allow publishing audio/video', true)
  .option('--can-subscribe', 'Allow subscribing to streams', true)
  .option('--can-publish-data', 'Allow publishing data messages', true)
  .action(async (roomName, identity, options) => {
    try {
      const config = loadConfig();
      const ttl = parseInt(options.ttl, 10);

      // Generate real LiveKit JWT token using livekit-server-sdk
      const at = new AccessToken(config.livekitApiKey, config.livekitApiSecret, {
        identity,
        name: options.name || identity,
        ttl,
      });

      at.addGrant({
        roomJoin: true,
        room: roomName,
        canPublish: options.canPublish,
        canSubscribe: options.canSubscribe,
        canPublishData: options.canPublishData,
      });

      const token = await at.toJwt();

      // Output token
      console.log(`\nToken generated successfully!`);
      console.log(`\nRoom:       ${roomName}`);
      console.log(`Identity:   ${identity}`);
      console.log(`Name:       ${options.name || identity}`);
      console.log(`TTL:        ${ttl}s`);
      console.log(`URL:        ${config.livekitUrl}`);
      console.log(`\nToken:`);
      console.log(token);
      console.log();

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create token', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Tokens: list
program
  .command('tokens:list')
  .description('List active tokens')
  .option('-r, --room <room-name>', 'Filter by room name')
  .action(async (options) => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();

      let roomId: string | undefined;
      if (options.room) {
        const room = await db.getRoomByName(options.room);
        if (room) roomId = room.id;
      }

      const tokens = await db.listTokens({ roomId });

      console.log(`\nActive Tokens (${tokens.length}):`);
      console.log('===================');
      for (const token of tokens) {
        console.log(`- ${token.id} (room: ${token.room_id}, user: ${token.user_id})`);
        console.log(`  Issued: ${token.issued_at}, Expires: ${token.expires_at}`);
        console.log(`  Uses: ${token.use_count}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list tokens', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Recordings: list
program
  .command('recordings:list')
  .description('List egress jobs')
  .option('-s, --status <status>', 'Filter by status')
  .option('-l, --limit <limit>', 'Result limit', '20')
  .action(async (options) => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();

      const jobs = await db.listEgressJobs({
        status: options.status,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nEgress Jobs (${jobs.length}):`);
      console.log('==================');
      for (const job of jobs) {
        console.log(`- ${job.livekit_egress_id} [${job.status}] (${job.egress_type}/${job.output_type})`);
        if (job.file_url) console.log(`  File: ${job.file_url}`);
        if (job.playlist_url) console.log(`  Playlist: ${job.playlist_url}`);
        if (job.error_message) console.log(`  Error: ${job.error_message}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list recordings', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Health command
program
  .command('health')
  .description('Health check')
  .action(async () => {
    try {
      const db = new LiveKitDatabase();
      await db.connect();
      await db.query('SELECT 1');
      console.log('Database: connected');
      console.log('Status: healthy');
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      console.error(`Health check failed: ${message}`);
      process.exit(1);
    }
  });

function parseDuration(duration: string): number {
  const match = duration.match(/^(\d+)(s|m|h|d)$/);
  if (!match) return 3600000; // default 1h
  const value = parseInt(match[1], 10);
  const unit = match[2];
  switch (unit) {
    case 's': return value * 1000;
    case 'm': return value * 60 * 1000;
    case 'h': return value * 60 * 60 * 1000;
    case 'd': return value * 24 * 60 * 60 * 1000;
    default: return 3600000;
  }
}

program.parse();
