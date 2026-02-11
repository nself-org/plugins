#!/usr/bin/env node
/**
 * Calendar Plugin CLI
 * Command-line interface for the Calendar plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { CalendarDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('calendar:cli');

const program = new Command();

program
  .name('nself-calendar')
  .description('Calendar plugin for nself - manage calendars and events')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      const db = new CalendarDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('✓ Calendar database schema initialized');
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
  .description('Start the calendar API server')
  .option('-p, --port <port>', 'Server port', '3505')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting calendar server on ${config.host}:${config.port}`);
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
  .description('Show calendar statistics')
  .action(async () => {
    try {
      const db = new CalendarDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nCalendar Statistics:');
      console.log('===================');
      console.log(`Calendars:       ${stats.calendars}`);
      console.log(`Events:          ${stats.events}`);
      console.log(`Attendees:       ${stats.attendees}`);
      console.log(`Reminders:       ${stats.reminders}`);
      console.log(`iCal Feeds:      ${stats.icalFeeds}`);
      console.log(`Upcoming Events: ${stats.upcomingEvents}`);
      if (stats.lastEventAt) {
        console.log(`Last Event:      ${stats.lastEventAt.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Events command
program
  .command('events')
  .description('List events')
  .option('-c, --calendar <id>', 'Filter by calendar ID')
  .option('-t, --type <type>', 'Filter by event type')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .option('--upcoming', 'Show only upcoming events')
  .option('--today', 'Show only today\'s events')
  .action(async (options) => {
    try {
      const db = new CalendarDatabase();
      await db.connect();

      let events;

      if (options.upcoming) {
        const limit = parseInt(options.limit, 10);
        events = await db.getUpcomingEvents(limit);
      } else if (options.today) {
        events = await db.getTodayEvents();
      } else {
        events = await db.listEvents({
          calendarId: options.calendar,
          type: options.type,
          limit: parseInt(options.limit, 10),
        });
      }

      await db.disconnect();

      if (events.length === 0) {
        console.log('No events found');
        process.exit(0);
      }

      console.log(`\nFound ${events.length} event(s):\n`);

      for (const event of events) {
        console.log(`${event.title}`);
        console.log(`  ID:        ${event.id}`);
        console.log(`  Type:      ${event.event_type}`);
        console.log(`  Start:     ${event.start_at.toISOString()}`);
        if (event.end_at) {
          console.log(`  End:       ${event.end_at.toISOString()}`);
        }
        console.log(`  Status:    ${event.status}`);
        if (event.location_name) {
          console.log(`  Location:  ${event.location_name}`);
        }
        if (event.recurrence_rule) {
          console.log(`  Recurring: ${event.recurrence_rule}`);
        }
        console.log(`  Attendees: ${event.attendee_count}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list events', { error: message });
      process.exit(1);
    }
  });

// Calendars command
program
  .command('calendars')
  .description('List calendars')
  .option('-o, --owner <id>', 'Filter by owner ID')
  .action(async (options) => {
    try {
      const db = new CalendarDatabase();
      await db.connect();
      const calendars = await db.listCalendars(options.owner);
      await db.disconnect();

      if (calendars.length === 0) {
        console.log('No calendars found');
        process.exit(0);
      }

      console.log(`\nFound ${calendars.length} calendar(s):\n`);

      for (const calendar of calendars) {
        console.log(`${calendar.name} ${calendar.is_default ? '(default)' : ''}`);
        console.log(`  ID:         ${calendar.id}`);
        console.log(`  Owner:      ${calendar.owner_id} (${calendar.owner_type})`);
        console.log(`  Visibility: ${calendar.visibility}`);
        console.log(`  Timezone:   ${calendar.timezone}`);
        console.log(`  Color:      ${calendar.color}`);
        if (calendar.description) {
          console.log(`  Description: ${calendar.description}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list calendars', { error: message });
      process.exit(1);
    }
  });

// Reminders command
program
  .command('reminders')
  .description('List pending reminders')
  .action(async () => {
    try {
      const db = new CalendarDatabase();
      await db.connect();
      const reminders = await db.getPendingReminders();
      await db.disconnect();

      if (reminders.length === 0) {
        console.log('No pending reminders');
        process.exit(0);
      }

      console.log(`\nFound ${reminders.length} pending reminder(s):\n`);

      for (const reminder of reminders) {
        console.log(`Reminder ${reminder.id}`);
        console.log(`  Event:   ${reminder.event_id}`);
        console.log(`  User:    ${reminder.user_id}`);
        console.log(`  Time:    ${reminder.remind_at.toISOString()}`);
        console.log(`  Channel: ${reminder.channel}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list reminders', { error: message });
      process.exit(1);
    }
  });

// iCal feeds command
program
  .command('ical')
  .description('List iCal feeds')
  .option('-c, --calendar <id>', 'Filter by calendar ID')
  .action(async (options) => {
    try {
      const db = new CalendarDatabase();
      await db.connect();
      const feeds = await db.listICalFeeds(options.calendar);
      await db.disconnect();

      if (feeds.length === 0) {
        console.log('No iCal feeds found');
        process.exit(0);
      }

      console.log(`\nFound ${feeds.length} iCal feed(s):\n`);

      const config = loadConfig();

      for (const feed of feeds) {
        const feedName = feed.name ?? 'Unnamed feed';
        console.log(`${feedName} ${feed.enabled ? '(enabled)' : '(disabled)'}`);
        console.log(`  ID:       ${feed.id}`);
        console.log(`  Calendar: ${feed.calendar_id}`);
        console.log(`  Token:    ${feed.token}`);
        console.log(`  URL:      http://localhost:${config.port}/v1/ical/${feed.token}`);
        if (feed.last_accessed_at) {
          console.log(`  Last Access: ${feed.last_accessed_at.toISOString()}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list iCal feeds', { error: message });
      process.exit(1);
    }
  });

// Stats command (alias for status)
program
  .command('stats')
  .description('Show calendar statistics (alias for status)')
  .action(async () => {
    try {
      const db = new CalendarDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nCalendar Statistics:');
      console.log('===================');
      console.log(`Calendars:       ${stats.calendars}`);
      console.log(`Events:          ${stats.events}`);
      console.log(`Attendees:       ${stats.attendees}`);
      console.log(`Reminders:       ${stats.reminders}`);
      console.log(`iCal Feeds:      ${stats.icalFeeds}`);
      console.log(`Upcoming Events: ${stats.upcomingEvents}`);
      if (stats.lastEventAt) {
        console.log(`Last Event:      ${stats.lastEventAt.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
