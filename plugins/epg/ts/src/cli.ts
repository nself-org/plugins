#!/usr/bin/env node
/**
 * EPG Plugin CLI
 * Command-line interface for the EPG plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { EpgDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('epg:cli');

const program = new Command();

program
  .name('nself-epg')
  .description('EPG plugin for nself - electronic program guide with XMLTV import')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new EpgDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for EPG plugin');
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
  .description('Start the EPG API server')
  .option('-p, --port <port>', 'Server port', '3031')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting EPG server on ${config.host}:${config.port}`);
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
  .description('Show EPG statistics')
  .action(async () => {
    try {
      const db = new EpgDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nEPG Statistics:');
      console.log('===============');
      console.log(`Total Channels:   ${stats.total_channels}`);
      console.log(`Active Channels:  ${stats.active_channels}`);
      console.log(`Total Programs:   ${stats.total_programs}`);
      console.log(`Total Schedules:  ${stats.total_schedules}`);
      console.log(`Channel Groups:   ${stats.total_channel_groups}`);
      if (stats.oldest_schedule) {
        console.log(`Oldest Schedule:  ${stats.oldest_schedule.toISOString()}`);
      }
      if (stats.newest_schedule) {
        console.log(`Newest Schedule:  ${stats.newest_schedule.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Channels command
program
  .command('channels')
  .description('List channels')
  .option('-c, --category <category>', 'Filter by category')
  .option('--active-only', 'Show only active channels')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new EpgDatabase();
      await db.connect();

      const channels = await db.listChannels({
        category: options.category,
        isActive: options.activeOnly ? true : undefined,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (channels.length === 0) {
        console.log('No channels found');
        process.exit(0);
      }

      console.log(`\nFound ${channels.length} channel(s):\n`);
      for (const ch of channels) {
        const number = ch.channel_number ? `${ch.channel_number} ` : '';
        const callSign = ch.call_sign ? `(${ch.call_sign}) ` : '';
        const hd = ch.is_hd ? ' [HD]' : '';
        const k4 = ch.is_4k ? ' [4K]' : '';
        const active = ch.is_active ? '' : ' [inactive]';

        console.log(`  ${number}${callSign}${ch.name}${hd}${k4}${active}`);
        console.log(`    ID:       ${ch.id}`);
        if (ch.category) console.log(`    Category: ${ch.category}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list channels', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Channel groups command
program
  .command('channel-groups')
  .description('List channel groups')
  .action(async () => {
    try {
      const db = new EpgDatabase();
      await db.connect();
      const groups = await db.listChannelGroups();
      await db.disconnect();

      if (groups.length === 0) {
        console.log('No channel groups found');
        process.exit(0);
      }

      console.log(`\nFound ${groups.length} channel group(s):\n`);
      for (const group of groups) {
        console.log(`${group.name}`);
        console.log(`  ID:          ${group.id}`);
        if (group.description) console.log(`  Description: ${group.description}`);
        console.log(`  Sort Order:  ${group.sort_order}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list channel groups', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Now command
program
  .command('now')
  .description("What's on right now")
  .action(async () => {
    try {
      const db = new EpgDatabase();
      await db.connect();
      const entries = await db.getWhatsOnNow();
      await db.disconnect();

      if (entries.length === 0) {
        console.log('No channels with current programming');
        process.exit(0);
      }

      console.log("\nWhat's On Now:\n");
      for (const entry of entries) {
        const number = entry.channel_number ? `${entry.channel_number} ` : '';
        const currentTitle = entry.current_program ? entry.current_program.title : 'No data';
        const nextTitle = entry.next_program ? entry.next_program.title : 'No data';

        console.log(`  ${number}${entry.channel_name}`);
        console.log(`    Now:  ${currentTitle}`);
        console.log(`    Next: ${nextTitle}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get current listings', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Tonight command
program
  .command('tonight')
  .description("Tonight's primetime schedule")
  .option('--date <date>', 'Date (YYYY-MM-DD)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new EpgDatabase();
      await db.connect();

      const date = options.date ? new Date(options.date) : new Date();
      const [startHour, startMin] = config.primetimeStart.split(':').map(Number);
      const [endHour, endMin] = config.primetimeEnd.split(':').map(Number);

      const startTime = new Date(date);
      startTime.setHours(startHour, startMin, 0, 0);

      const endTime = new Date(date);
      endTime.setHours(endHour, endMin, 0, 0);

      if (endTime <= startTime) {
        endTime.setDate(endTime.getDate() + 1);
      }

      const schedule = await db.getScheduleGrid({ startTime, endTime });
      await db.disconnect();

      if (schedule.length === 0) {
        console.log('No primetime listings available');
        process.exit(0);
      }

      console.log(`\nTonight's Primetime (${config.primetimeStart} - ${config.primetimeEnd}):\n`);
      for (const ch of schedule) {
        const number = ch.channel_number ? `${ch.channel_number} ` : '';
        console.log(`  ${number}${ch.channel_name}:`);
        for (const prog of ch.programs) {
          const startStr = prog.start_time.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
          const flags = [
            prog.is_new ? 'NEW' : null,
            prog.is_live ? 'LIVE' : null,
          ].filter(Boolean).join(' ');

          console.log(`    ${startStr}  ${prog.title}${flags ? ` [${flags}]` : ''}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get tonight listings', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Schedule command
program
  .command('schedule')
  .description('View channel schedule')
  .requiredOption('--channel <uuid>', 'Channel ID')
  .option('--date <date>', 'Start date (YYYY-MM-DD)')
  .option('--days <days>', 'Number of days', '1')
  .action(async (options) => {
    try {
      const db = new EpgDatabase();
      await db.connect();

      const channel = await db.getChannel(options.channel);
      if (!channel) {
        console.error('Channel not found');
        process.exit(1);
      }

      const startDate = options.date ? new Date(options.date) : new Date();
      startDate.setHours(0, 0, 0, 0);
      const days = parseInt(options.days, 10);

      const schedule = await db.getScheduleForChannel(options.channel, startDate, days);
      await db.disconnect();

      if (schedule.length === 0) {
        console.log(`No schedule data for ${channel.name}`);
        process.exit(0);
      }

      console.log(`\nSchedule for ${channel.name} (${schedule.length} programs):\n`);
      for (const entry of schedule) {
        const startStr = entry.start_time.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
        const endStr = entry.end_time.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });
        const flags = [
          entry.is_new ? 'NEW' : null,
          entry.is_live ? 'LIVE' : null,
          entry.is_rerun ? 'RERUN' : null,
        ].filter(Boolean).join(' ');

        console.log(`  ${startStr} - ${endStr}  ${entry.title}${flags ? ` [${flags}]` : ''}`);
        if (entry.episode_title) console.log(`    "${entry.episode_title}"`);
        if (entry.content_rating) console.log(`    Rating: ${entry.content_rating}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get schedule', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Search command
program
  .command('search <query>')
  .description('Search programs')
  .option('--genre <genre>', 'Filter by genre')
  .option('--rating <rating>', 'Filter by content rating')
  .option('--movies', 'Search only movies')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (query: string, options) => {
    try {
      const db = new EpgDatabase();
      await db.connect();

      const programs = await db.searchPrograms({
        query,
        genre: options.genre,
        contentRating: options.rating,
        isMovie: options.movies ? true : undefined,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (programs.length === 0) {
        console.log(`No programs found for "${query}"`);
        process.exit(0);
      }

      console.log(`\nFound ${programs.length} program(s) for "${query}":\n`);
      for (const prog of programs) {
        console.log(`  ${prog.title}`);
        console.log(`    ID:       ${prog.id}`);
        if (prog.genre) console.log(`    Genre:    ${prog.genre}`);
        if (prog.content_rating) console.log(`    Rating:   ${prog.content_rating}`);
        if (prog.year) console.log(`    Year:     ${prog.year}`);
        if (prog.is_movie) console.log(`    Type:     Movie`);
        if (prog.description) {
          const desc = prog.description.length > 100
            ? prog.description.substring(0, 100) + '...'
            : prog.description;
          console.log(`    Desc:     ${desc}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Import command
program
  .command('import')
  .description('Import EPG data')
  .option('--xmltv-url <url>', 'XMLTV URL to import from')
  .option('--schedules-direct', 'Import from Schedules Direct')
  .option('--lineup <lineup>', 'Schedules Direct lineup ID')
  .action(async (options) => {
    try {
      const config = loadConfig();

      if (options.xmltvUrl) {
        console.log(`Importing XMLTV data from: ${options.xmltvUrl}`);
        console.log(`Server should be running on port ${config.port}`);
        console.log('Use POST /api/import/xmltv endpoint');
      } else if (options.schedulesDirect) {
        const lineup = options.lineup ?? config.schedulesDirectLineup;
        console.log(`Importing from Schedules Direct, lineup: ${lineup}`);
        console.log(`Server should be running on port ${config.port}`);
        console.log('Use POST /api/import/schedules-direct endpoint');
      } else {
        console.log('Specify --xmltv-url or --schedules-direct');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Sync command
program
  .command('sync')
  .description('Trigger EPG sync')
  .action(async () => {
    try {
      const config = loadConfig();
      console.log('Triggering EPG sync...');
      console.log(`Server should be running on port ${config.port}`);
      console.log('Use POST /api/sync endpoint');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync trigger failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
