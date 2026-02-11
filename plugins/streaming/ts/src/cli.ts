#!/usr/bin/env node
/**
 * CLI for streaming plugin
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { db } from './database.js';

const logger = createLogger('streaming:cli');

const command = process.argv[2];

async function main() {
  switch (command) {
    case 'init':
      await init();
      break;
    case 'server':
      await startServer();
      break;
    case 'status':
      await showStatus();
      break;
    case 'streams':
      await listStreams();
      break;
    case 'recordings':
      await listRecordingsCmd();
      break;
    case 'schedule':
      await listSchedule();
      break;
    default:
      showHelp();
      break;
  }
}

async function init() {
  logger.info('Initializing streaming system...');

  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    const stats = await db.getStats();
    logger.info('Current state:');
    logger.info(`  Total streams: ${stats.total_streams}`);
    logger.info(`  Live streams: ${stats.live_streams}`);
    logger.info(`  Recordings: ${stats.total_recordings}`);
    logger.info(`  Clips: ${stats.total_clips}`);

    logger.info('Configuration:');
    logger.info(`  Port: ${config.server.port}`);
    logger.info(`  RTMP Port: ${config.rtmp.port}`);
    logger.info(`  Recording: ${config.recording.enabled ? 'enabled' : 'disabled'}`);
    logger.info(`  DVR: ${config.dvr.enabled ? 'enabled' : 'disabled'} (${config.dvr.window_seconds}s)`);
    logger.info(`  Chat rate limit: ${config.chat.rate_limit_messages}/${config.chat.rate_limit_window}s`);

    logger.info('Initialization complete!');
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Initialization failed', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function startServer() {
  logger.info('Starting streaming server...');
  await import('./server.js');
}

async function showStatus() {
  try {
    const stats = await db.getStats();

    logger.info('Streaming Status:');
    logger.info(`  Total streams: ${stats.total_streams}`);
    logger.info(`  Live now: ${stats.live_streams}`);
    logger.info(`  Total recordings: ${stats.total_recordings}`);
    logger.info(`  Total clips: ${stats.total_clips}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting status', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listStreams() {
  try {
    const result = await db.listStreams({ limit: '20' });
    logger.info(`Streams (${result.total}):`);
    for (const stream of result.streams) {
      logger.info(`  ${stream.title} [${stream.status}] (${stream.visibility})`);
      logger.info(`    Broadcaster: ${stream.broadcaster_id}`);
      if (stream.started_at) logger.info(`    Started: ${new Date(stream.started_at).toLocaleString()}`);
      logger.info(`    Views: ${stream.total_views} | Peak: ${stream.peak_viewers}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing streams', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listRecordingsCmd() {
  const streamId = process.argv[3];
  if (!streamId) {
    logger.error('Usage: nself-streaming recordings <stream_id>');
    process.exit(1);
  }
  try {
    const recordings = await db.listRecordings(streamId);
    logger.info(`Recordings for stream ${streamId} (${recordings.length}):`);
    for (const rec of recordings) {
      logger.info(`  ${rec.title} [${rec.status}] (${rec.duration_seconds}s)`);
      logger.info(`    Recorded: ${new Date(rec.recorded_at).toLocaleString()}`);
      logger.info(`    Views: ${rec.views}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing recordings', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listSchedule() {
  try {
    const scheduled = await db.listScheduledStreams();
    logger.info(`Scheduled Streams (${scheduled.length}):`);
    for (const s of scheduled) {
      logger.info(`  ${s.title} [${s.status}]`);
      logger.info(`    Scheduled: ${new Date(s.scheduled_start).toLocaleString()}`);
      if (s.estimated_duration_minutes) logger.info(`    Duration: ${s.estimated_duration_minutes}m`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing schedule', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

function showHelp() {
  logger.info('nself-streaming - Live streaming and broadcasting CLI');
  logger.info('');
  logger.info('Usage:');
  logger.info('  nself-streaming <command> [options]');
  logger.info('');
  logger.info('Commands:');
  logger.info('  init              Initialize and verify setup');
  logger.info('  server            Start the streaming server');
  logger.info('  status            Show system status');
  logger.info('  streams           List streams');
  logger.info('  recordings <id>   List recordings for a stream');
  logger.info('  schedule          List scheduled streams');
  logger.info('');
  logger.info('For full functionality, use the nself plugin commands:');
  logger.info('  nself plugin streaming <action>');
}

main().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
