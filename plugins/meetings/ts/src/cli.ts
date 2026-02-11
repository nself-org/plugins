#!/usr/bin/env node
/**
 * CLI for meetings plugin
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { db } from './database.js';

const logger = createLogger('meetings:cli');

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
    case 'events':
      await listEvents();
      break;
    case 'rooms':
      await listRooms();
      break;
    case 'calendars':
      await listCalendars();
      break;
    case 'templates':
      await listTemplates();
      break;
    default:
      showHelp();
      break;
  }
}

async function init() {
  logger.info('Initializing meetings system...');

  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    const stats = await db.getStats();
    logger.info('Current state:');
    logger.info(`  Events: ${stats.total_events}`);
    logger.info(`  Rooms: ${stats.total_rooms}`);
    logger.info(`  Calendars: ${stats.total_calendars}`);
    logger.info(`  Templates: ${stats.total_templates}`);
    logger.info(`  Upcoming: ${stats.upcoming_events}`);

    logger.info('Configuration:');
    logger.info(`  Port: ${config.server.port}`);
    logger.info(`  Default timezone: ${config.calendar.default_timezone}`);
    logger.info(`  Business hours: ${config.calendar.business_hours_start} - ${config.calendar.business_hours_end}`);
    logger.info(`  Room buffer: ${config.rooms.default_buffer_minutes}m`);

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
  logger.info('Starting meetings server...');
  // Dynamic import to start the server
  await import('./server.js');
}

async function showStatus() {
  try {
    const stats = await db.getStats();

    logger.info('Meetings Status:');
    logger.info(`  Total events: ${stats.total_events}`);
    logger.info(`  Upcoming events: ${stats.upcoming_events}`);
    logger.info(`  Total rooms: ${stats.total_rooms}`);
    logger.info(`  Total calendars: ${stats.total_calendars}`);
    logger.info(`  Total templates: ${stats.total_templates}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting status', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listEvents() {
  try {
    const result = await db.listEvents({ limit: '20' });
    logger.info(`Events (${result.total}):`);
    for (const event of result.events) {
      logger.info(`  ${event.title} [${event.status}]`);
      logger.info(`    ${new Date(event.start_time).toLocaleString()} - ${new Date(event.end_time).toLocaleString()}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing events', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listRooms() {
  try {
    const result = await db.listRooms({});
    logger.info(`Rooms (${result.total}):`);
    for (const room of result.rooms) {
      logger.info(`  ${room.name} (capacity: ${room.capacity}) [${room.is_active ? 'active' : 'inactive'}]`);
      if (room.location) logger.info(`    Location: ${room.location}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing rooms', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listCalendars() {
  try {
    const calendars = await db.listCalendars();
    logger.info(`Calendars (${calendars.length}):`);
    for (const cal of calendars) {
      logger.info(`  ${cal.name} (owner: ${cal.owner_id}) [${cal.is_public ? 'public' : 'private'}]`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing calendars', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listTemplates() {
  try {
    const templates = await db.listTemplates();
    logger.info(`Templates (${templates.length}):`);
    for (const tmpl of templates) {
      logger.info(`  ${tmpl.name} (${tmpl.default_duration_minutes}m) [${tmpl.is_public ? 'public' : 'private'}]`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing templates', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

function showHelp() {
  logger.info('nself-meetings - Calendar and meeting management CLI');
  logger.info('');
  logger.info('Usage:');
  logger.info('  nself-meetings <command> [options]');
  logger.info('');
  logger.info('Commands:');
  logger.info('  init        Initialize and verify setup');
  logger.info('  server      Start the meetings server');
  logger.info('  status      Show system status');
  logger.info('  events      List upcoming events');
  logger.info('  rooms       List meeting rooms');
  logger.info('  calendars   List calendars');
  logger.info('  templates   List meeting templates');
  logger.info('');
  logger.info('For full functionality, use the nself plugin commands:');
  logger.info('  nself plugin meetings <action>');
}

main().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
