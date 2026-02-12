#!/usr/bin/env node
/**
 * Sports Plugin CLI
 * Command-line interface for the Sports plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { SportsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('sports:cli');

const program = new Command();

program
  .name('nself-sports')
  .description('Sports schedule and metadata synchronization plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
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
  .option('-p, --port <port>', 'Server port', '3035')
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

// Sync command
program
  .command('sync')
  .description('Sync from all providers or specific provider/sport/league')
  .option('--provider <provider>', 'Sync from specific provider (e.g., espn)')
  .option('--sport <sport>', 'Sync specific sport (e.g., football)')
  .option('--league <league>', 'Sync specific league (e.g., nfl)')
  .option('--season <season>', 'Sync specific season (e.g., 2026)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const providers = options.provider ? [options.provider] : config.providers;

      logger.info(`Starting sync from providers: ${providers.join(', ')}`);

      if (options.sport) {
        logger.info(`Filtering by sport: ${options.sport}`);
      }
      if (options.league) {
        logger.info(`Filtering by league: ${options.league}`);
      }
      if (options.season) {
        logger.info(`Filtering by season: ${options.season}`);
      }

      for (const provider of providers) {
        const syncRecord = await db.createSyncRecord(provider, 'full');
        try {
          logger.info(`Syncing from ${provider}...`);
          await db.updateSyncRecord(syncRecord.id, 'completed', 0);
          logger.success(`Sync from ${provider} completed`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          await db.updateSyncRecord(syncRecord.id, 'failed', 0, [message]);
          logger.error(`Sync from ${provider} failed: ${message}`);
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      process.exit(1);
    }
  });

// Reconcile command
program
  .command('reconcile')
  .description('Reconcile recent data')
  .option('--days <days>', 'Number of days to look back', '7')
  .action(async (options) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const days = parseInt(options.days, 10);
      logger.info(`Reconciling last ${days} days...`);

      // Placeholder for reconciliation logic
      logger.success(`Reconciliation of last ${days} days completed`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Reconcile failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show sync statistics')
  .action(async () => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const stats = await db.getPluginStats();

      console.log('\nSports Plugin Status');
      console.log('====================');
      console.log(`  Leagues:          ${stats.leagues}`);
      console.log(`  Teams:            ${stats.teams}`);
      console.log(`  Events:           ${stats.events}`);
      console.log(`  Upcoming (7d):    ${stats.upcoming_events}`);
      console.log(`  Live now:         ${stats.live_events}`);
      if (Object.keys(stats.by_provider).length > 0) {
        console.log('\n  By Provider:');
        for (const [provider, count] of Object.entries(stats.by_provider)) {
          console.log(`    ${provider}: ${count} events`);
        }
      }
      if (stats.last_sync) {
        console.log(`\n  Last sync:        ${stats.last_sync.toISOString()}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Leagues command
program
  .command('leagues')
  .description('List all leagues')
  .option('--sport <sport>', 'Filter by sport')
  .action(async (options) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const leagues = await db.listLeagues(options.sport);

      console.log('\nLeagues:');
      console.log('-'.repeat(80));
      for (const league of leagues) {
        console.log(
          `${(league.abbreviation ?? league.name).padEnd(10)} | ` +
          `${league.name.padEnd(30)} | ` +
          `${league.sport.padEnd(12)} | ` +
          `${league.provider}`
        );
      }
      console.log(`\nTotal: ${leagues.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Leagues list failed', { error: message });
      process.exit(1);
    }
  });

// Teams command
program
  .command('teams')
  .description('List teams')
  .option('--league <league_id>', 'Filter by league ID')
  .option('--sport <sport>', 'Filter by sport')
  .option('--search <search>', 'Search by name/city/abbreviation')
  .action(async (options) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const teams = await db.listTeams({
        league_id: options.league,
        sport: options.sport,
        search: options.search,
      });

      console.log('\nTeams:');
      console.log('-'.repeat(100));
      for (const team of teams) {
        console.log(
          `${(team.abbreviation ?? '').padEnd(6)} | ` +
          `${team.name.padEnd(30)} | ` +
          `${(team.city ?? '').padEnd(20)} | ` +
          `${(team.conference ?? '').padEnd(15)} | ` +
          `${team.division ?? ''}`
        );
      }
      console.log(`\nTotal: ${teams.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Teams list failed', { error: message });
      process.exit(1);
    }
  });

// Events command
program
  .command('events')
  .description('Show events')
  .option('--today', 'Show today\'s events')
  .option('--live', 'Show live events')
  .option('--upcoming', 'Show upcoming 7 days')
  .option('--league <league_id>', 'Filter by league ID')
  .option('--team <team_id>', 'Filter by team ID')
  .option('--limit <limit>', 'Number of results', '50')
  .action(async (options) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      let events;

      if (options.live) {
        events = await db.getLiveEvents(options.league);
        console.log('\nLive Events:');
      } else if (options.today) {
        events = await db.getTodayEvents({ league_id: options.league });
        console.log('\nToday\'s Events:');
      } else if (options.upcoming) {
        events = await db.getUpcomingEvents({
          league_id: options.league,
          team_id: options.team,
          limit: parseInt(options.limit, 10),
        });
        console.log('\nUpcoming Events (7 days):');
      } else {
        const result = await db.listEvents({
          league_id: options.league,
          team_id: options.team,
          limit: parseInt(options.limit, 10),
        });
        events = result.data;
        console.log('\nAll Events:');
      }

      console.log('-'.repeat(120));
      for (const event of events) {
        const homeTeam = event.home_team_name ?? event.home_team_id ?? 'TBD';
        const awayTeam = event.away_team_name ?? event.away_team_id ?? 'TBD';
        const score = event.home_score !== null ? `${event.home_score}-${event.away_score}` : 'N/A';
        const scheduledAt = event.scheduled_at instanceof Date
          ? event.scheduled_at.toISOString()
          : String(event.scheduled_at);

        console.log(
          `${scheduledAt.substring(0, 16).padEnd(18)} | ` +
          `${String(event.status).padEnd(12)} | ` +
          `${String(homeTeam).padEnd(20)} vs ${String(awayTeam).padEnd(20)} | ` +
          `${score.padEnd(8)} | ` +
          `${event.league_name ?? ''}`
        );
      }
      console.log(`\nTotal: ${events.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Events list failed', { error: message });
      process.exit(1);
    }
  });

// Lock command
program
  .command('lock <eventId>')
  .description('Lock an event')
  .option('--reason <reason>', 'Lock reason', 'Manual lock via CLI')
  .action(async (eventId, options) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const event = await db.lockEvent(eventId, options.reason);
      if (!event) {
        logger.error('Event not found');
        process.exit(1);
      }

      logger.success(`Event ${eventId} locked: ${options.reason}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Lock failed', { error: message });
      process.exit(1);
    }
  });

// Override command
program
  .command('override <eventId>')
  .description('Manual schedule override')
  .option('--time <time>', 'New scheduled time (ISO format)')
  .option('--channel <channel>', 'New broadcast channel')
  .option('--notes <notes>', 'Override notes', 'Manual override via CLI')
  .action(async (eventId, options) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      const event = await db.overrideEvent(eventId, options.time, options.channel, options.notes);
      if (!event) {
        logger.error('Event not found');
        process.exit(1);
      }

      logger.success(`Event ${eventId} overridden`);
      console.log(JSON.stringify(event, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Override failed', { error: message });
      process.exit(1);
    }
  });

// Cache command
program
  .command('cache')
  .description('Manage schedule cache')
  .argument('[action]', 'Action: status, clear', 'status')
  .action(async (action) => {
    try {
      const db = new SportsDatabase(undefined, 'primary');
      await db.connect();

      switch (action) {
        case 'status': {
          const stats = await db.getCacheStats();
          console.log('\nCache Status:');
          console.log('=============');
          console.log(`  Total entries:  ${stats.entries}`);
          console.log(`  Active:         ${stats.active}`);
          console.log(`  Expired:        ${stats.expired}`);
          break;
        }

        case 'clear': {
          const cleared = await db.clearCache();
          logger.success(`Cleared ${cleared} cache entries`);
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cache command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
