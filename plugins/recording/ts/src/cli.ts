#!/usr/bin/env node
/**
 * Recording Plugin CLI
 * Command-line interface for recording orchestration and archive management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { RecordingDatabase } from './database.js';
import { createServer } from './server.js';
import type { RecordingStatus, EncodeStatus } from './types.js';

const logger = createLogger('recording:cli');

const program = new Command();

program
  .name('nself-recording')
  .description('Recording plugin for nself - recording orchestration and archive management')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new RecordingDatabase();
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
  .option('-p, --port <port>', 'Server port', '3602')
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
  .description('Show recording service status')
  .action(async () => {
    try {
      const db = new RecordingDatabase();
      await db.connect();
      const stats = await db.getRecordingStats();

      console.log('\nRecording Service Status');
      console.log('========================');
      console.log(`  Total recordings:     ${stats.total_recordings}`);
      console.log(`  Scheduled:            ${stats.scheduled}`);
      console.log(`  Recording now:        ${stats.recording_now}`);
      console.log(`  Encoding:             ${stats.encoding}`);
      console.log(`  Published:            ${stats.published}`);
      console.log(`  Failed:               ${stats.failed}`);
      console.log(`  Cancelled:            ${stats.cancelled}`);
      console.log(`  Total storage:        ${stats.total_storage_gb.toFixed(2)} GB`);
      console.log(`  Total duration:       ${stats.total_duration_hours.toFixed(1)} hours`);
      console.log(`  Schedules:            ${stats.active_schedules}/${stats.total_schedules} active`);
      console.log(`  Encode jobs pending:  ${stats.pending_encode_jobs}`);
      console.log(`  Encode jobs running:  ${stats.running_encode_jobs}`);
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

// Recordings commands
const recordings = program
  .command('recordings')
  .description('Manage recordings');

recordings
  .command('list')
  .description('List all recordings')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .option('-s, --status <status>', 'Filter by status')
  .option('-c, --category <category>', 'Filter by category')
  .action(async (options) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();
      const list = await db.listRecordings(
        undefined,
        options.status as RecordingStatus | undefined,
        undefined,
        options.category,
        parseInt(options.limit, 10)
      );

      console.log('\nRecordings:');
      console.log('-'.repeat(130));
      for (const r of list) {
        const scheduled = new Date(r.scheduled_start).toLocaleString();
        const size = r.file_size ? `${(Number(r.file_size) / (1024 * 1024 * 1024)).toFixed(2)} GB` : 'N/A';
        console.log(
          `${String(r.id).substring(0, 8)}... | ${r.title.substring(0, 30).padEnd(30)} | ` +
          `${r.status.padEnd(12)} | ${r.source_type.padEnd(15)} | ` +
          `${scheduled} | ${size}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List recordings failed', { error: message });
      process.exit(1);
    }
  });

recordings
  .command('create')
  .description('Create/schedule a recording')
  .requiredOption('--title <title>', 'Recording title')
  .requiredOption('--source <sourceType>', 'Source type (live_tv, device_ingest, upload, stream_capture)')
  .option('--channel <channel>', 'Source channel')
  .requiredOption('--start <start>', 'Scheduled start (ISO 8601)')
  .requiredOption('--duration <minutes>', 'Duration in minutes')
  .option('--priority <priority>', 'Priority (low, normal, high, critical)', 'normal')
  .option('--category <category>', 'Category')
  .option('--sport-event-id <eventId>', 'Sports event ID')
  .action(async (options) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();

      const startDate = new Date(options.start);
      const endDate = new Date(startDate.getTime() + parseInt(options.duration, 10) * 60 * 1000);

      const recording = await db.createRecording('default', {
        title: options.title,
        source_type: options.source,
        source_channel: options.channel,
        scheduled_start: startDate.toISOString(),
        scheduled_end: endDate.toISOString(),
        priority: options.priority,
        category: options.category,
        sports_event_id: options.sportEventId,
      });

      logger.success(`Recording scheduled: ${recording.id}`);
      console.log(JSON.stringify(recording, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create recording failed', { error: message });
      process.exit(1);
    }
  });

recordings
  .command('cancel')
  .description('Cancel a scheduled recording')
  .argument('<recordingId>', 'Recording ID')
  .action(async (recordingId) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();

      const recording = await db.cancelRecording(recordingId);
      if (!recording) {
        logger.error('Recording not found or cannot be cancelled');
        process.exit(1);
      }

      logger.success(`Recording cancelled: ${recording.id}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cancel failed', { error: message });
      process.exit(1);
    }
  });

recordings
  .command('delete')
  .description('Delete a recording')
  .argument('<recordingId>', 'Recording ID')
  .action(async (recordingId) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();

      const deleted = await db.deleteRecording(recordingId);
      if (!deleted) {
        logger.error('Recording not found');
        process.exit(1);
      }

      logger.success('Recording deleted');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Delete failed', { error: message });
      process.exit(1);
    }
  });

// Schedule command
program
  .command('schedule')
  .description('Schedule recording from sports event')
  .requiredOption('--sport-event-id <eventId>', 'Sports event ID')
  .option('--channel <channel>', 'Source channel')
  .option('--title <title>', 'Recording title')
  .option('--duration <minutes>', 'Duration in minutes', '180')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new RecordingDatabase();
      await db.connect();

      const startDate = new Date();
      const endDate = new Date(startDate.getTime() + parseInt(options.duration, 10) * 60 * 1000);

      const recording = await db.createRecording('default', {
        title: options.title ?? `Sports Recording - ${options.sportEventId}`,
        source_type: 'live_tv',
        source_channel: options.channel,
        scheduled_start: new Date(startDate.getTime() - config.defaultLeadTimeMinutes * 60 * 1000).toISOString(),
        scheduled_end: new Date(endDate.getTime() + config.defaultTrailTimeMinutes * 60 * 1000).toISOString(),
        sports_event_id: options.sportEventId,
        category: 'sports',
      });

      logger.success(`Sports recording scheduled: ${recording.id}`);
      console.log(JSON.stringify(recording, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Schedule failed', { error: message });
      process.exit(1);
    }
  });

// Archives commands
const archives = program
  .command('archives')
  .description('Manage archived recordings');

archives
  .command('list')
  .description('List archived (published) recordings')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .option('-c, --category <category>', 'Filter by category')
  .action(async (options) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();
      const list = await db.getPublishedRecordings(undefined, options.category, parseInt(options.limit, 10));

      console.log('\nArchived Recordings:');
      console.log('-'.repeat(120));
      for (const r of list) {
        const pubDate = r.published_at ? new Date(r.published_at).toLocaleDateString() : 'N/A';
        const dur = r.duration_seconds ? `${Math.round(Number(r.duration_seconds) / 60)} min` : 'N/A';
        console.log(
          `${String(r.id).substring(0, 8)}... | ${r.title.substring(0, 35).padEnd(35)} | ` +
          `${(r.category ?? 'none').padEnd(15)} | ${dur.padEnd(8)} | Published: ${pubDate}`
        );
      }
      console.log(`\nTotal: ${list.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List archives failed', { error: message });
      process.exit(1);
    }
  });

// Encode status command
program
  .command('encode-status')
  .description('Show encode job queue status')
  .argument('[recordingId]', 'Recording ID (optional)')
  .action(async (recordingId) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();

      if (recordingId) {
        const jobs = await db.getEncodeJobsForRecording(recordingId);
        console.log(`\nEncode Jobs for Recording ${recordingId}:`);
        console.log('-'.repeat(100));
        for (const j of jobs) {
          console.log(
            `${String(j.id).substring(0, 8)}... | ${j.profile.padEnd(8)} | ` +
            `${j.status.padEnd(10)} | ${(j.progress * 100).toFixed(1)}% | ` +
            `${j.error ?? 'No errors'}`
          );
        }
      } else {
        const pending = await db.listEncodeJobs(undefined, 'pending' as EncodeStatus);
        const running = await db.listEncodeJobs(undefined, 'running' as EncodeStatus);

        console.log('\nEncode Job Queue:');
        console.log('=================');
        console.log(`  Pending: ${pending.length}`);
        console.log(`  Running: ${running.length}`);

        if (running.length > 0) {
          console.log('\n  Running Jobs:');
          for (const j of running) {
            console.log(
              `    ${String(j.id).substring(0, 8)}... | ${j.profile} | ${(j.progress * 100).toFixed(1)}%`
            );
          }
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Encode status failed', { error: message });
      process.exit(1);
    }
  });

// Publish command
program
  .command('publish')
  .description('Publish a completed recording')
  .requiredOption('--recording-id <recordingId>', 'Recording ID')
  .action(async (options) => {
    try {
      const db = new RecordingDatabase();
      await db.connect();

      const recording = await db.publishRecording(options.recordingId);
      if (!recording) {
        logger.error('Recording not found');
        process.exit(1);
      }

      logger.success(`Recording published: ${recording.id}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Publish failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show recording statistics')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new RecordingDatabase();
      await db.connect();
      const stats = await db.getRecordingStats();

      console.log('\nRecording Statistics');
      console.log('====================');
      console.log(`  Max concurrent recordings: ${config.maxConcurrentRecordings}`);
      console.log(`  Max concurrent encodes:    ${config.maxConcurrentEncodes}`);
      console.log(`  Default encode profile:    ${config.defaultEncodeProfile}`);
      console.log(`  Auto-encode:               ${config.autoEncode}`);
      console.log(`  Auto-enrich:               ${config.autoEnrich}`);
      console.log(`  Auto-publish:              ${config.autoPublish}`);
      console.log('\n  Recording Stats:');
      console.log(`  Total recordings:          ${stats.total_recordings}`);
      console.log(`  Total storage:             ${stats.total_storage_gb.toFixed(2)} GB`);
      console.log(`  Total duration:            ${stats.total_duration_hours.toFixed(1)} hours`);
      console.log(`  Published:                 ${stats.published}`);
      console.log(`  Failed:                    ${stats.failed}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
