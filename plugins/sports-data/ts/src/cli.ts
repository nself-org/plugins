#!/usr/bin/env node
/**
 * Sports Data Plugin CLI
 * Command-line interface for the Sports Data plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { SportsDataDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('sports-data:cli');

const program = new Command();

program
  .name('nself-sports-data')
  .description('Sports data plugin for nself - live scores, schedules, standings')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new SportsDataDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for sports-data plugin');
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
  .description('Start the sports-data API server')
  .option('-p, --port <port>', 'Server port', '3030')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting sports-data server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status / Stats command
program
  .command('stats')
  .alias('status')
  .description('Show sports data statistics')
  .action(async () => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nSports Data Statistics:');
      console.log('======================');
      console.log(`Leagues:         ${stats.total_leagues}`);
      console.log(`Teams:           ${stats.total_teams}`);
      console.log(`Total Games:     ${stats.total_games}`);
      console.log(`Live Games:      ${stats.live_games}`);
      console.log(`Upcoming Games:  ${stats.upcoming_games}`);
      console.log(`Completed Games: ${stats.completed_games}`);
      console.log(`Players:         ${stats.total_players}`);
      console.log(`Favorites:       ${stats.total_favorites}`);
      if (stats.last_sync_at) {
        console.log(`Last Sync:       ${stats.last_sync_at.toISOString()}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Leagues command
program
  .command('leagues')
  .description('List leagues')
  .option('-s, --sport <sport>', 'Filter by sport')
  .action(async (options) => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();
      const leagues = await db.listLeagues({ sport: options.sport });
      await db.disconnect();

      if (leagues.length === 0) {
        console.log('No leagues found');
        process.exit(0);
      }

      console.log(`\nFound ${leagues.length} league(s):\n`);
      for (const league of leagues) {
        console.log(`${league.name} (${league.abbreviation ?? league.sport})`);
        console.log(`  ID:       ${league.id}`);
        console.log(`  Sport:    ${league.sport}`);
        console.log(`  Provider: ${league.provider}`);
        console.log(`  Active:   ${league.active ? 'yes' : 'no'}`);
        if (league.season_year) {
          console.log(`  Season:   ${league.season_year} ${league.season_type ?? ''}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list leagues', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Teams command
program
  .command('teams')
  .description('List teams')
  .option('--league <uuid>', 'Filter by league ID')
  .option('--conference <name>', 'Filter by conference')
  .option('--division <name>', 'Filter by division')
  .action(async (options) => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();
      const teams = await db.listTeams({
        leagueId: options.league,
        conference: options.conference,
        division: options.division,
      });
      await db.disconnect();

      if (teams.length === 0) {
        console.log('No teams found');
        process.exit(0);
      }

      console.log(`\nFound ${teams.length} team(s):\n`);
      for (const team of teams) {
        console.log(`${team.city ? team.city + ' ' : ''}${team.name} (${team.abbreviation ?? ''})`);
        console.log(`  ID:         ${team.id}`);
        if (team.conference) console.log(`  Conference: ${team.conference}`);
        if (team.division) console.log(`  Division:   ${team.division}`);
        if (team.venue) console.log(`  Venue:      ${team.venue}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list teams', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Games command
program
  .command('games')
  .description('View games')
  .option('--today', 'Show today\'s games')
  .option('--live', 'Show live games only')
  .option('--league <uuid>', 'Filter by league ID')
  .option('--team <uuid>', 'Filter by team ID')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (options) => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();

      let games;
      if (options.live) {
        games = await db.getLiveGames();
      } else if (options.today) {
        games = await db.getTodayGames();
      } else {
        games = await db.listGames({
          leagueId: options.league,
          teamId: options.team,
          limit: parseInt(options.limit, 10),
        });
      }

      await db.disconnect();

      if (games.length === 0) {
        console.log('No games found');
        process.exit(0);
      }

      console.log(`\nFound ${games.length} game(s):\n`);
      for (const game of games) {
        const home = game.home_team_abbreviation ?? 'TBD';
        const away = game.away_team_abbreviation ?? 'TBD';
        const score = game.home_score !== null ? `${game.home_score}-${game.away_score}` : 'TBD';

        console.log(`${away} @ ${home}  ${score}  [${game.status}]`);
        console.log(`  ID:        ${game.id}`);
        console.log(`  Scheduled: ${game.scheduled_at.toISOString()}`);
        if (game.period) console.log(`  Period:    ${game.period} ${game.clock ?? ''}`);
        if (game.league_name) console.log(`  League:    ${game.league_name}`);
        if (game.venue) console.log(`  Venue:     ${game.venue}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list games', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Scores command
program
  .command('scores')
  .description('View latest scores')
  .option('--date <date>', 'Date (YYYY-MM-DD)')
  .option('--league <uuid>', 'Filter by league ID')
  .action(async (options) => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();
      const games = await db.getScores({
        leagueId: options.league,
        date: options.date ? new Date(options.date) : undefined,
      });
      await db.disconnect();

      if (games.length === 0) {
        console.log('No scores available');
        process.exit(0);
      }

      console.log(`\nScores (${games.length} games):\n`);
      for (const game of games) {
        const home = game.home_team_abbreviation ?? 'TBD';
        const away = game.away_team_abbreviation ?? 'TBD';
        const homeScore = game.home_score ?? '-';
        const awayScore = game.away_score ?? '-';
        const status = game.status === 'in_progress' ? `${game.period} ${game.clock ?? ''}` : game.status;

        console.log(`  ${away} ${awayScore}  @  ${home} ${homeScore}  [${status}]`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get scores', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Standings command
program
  .command('standings')
  .description('View standings')
  .requiredOption('--league <uuid>', 'League ID')
  .option('--season <year>', 'Season year')
  .option('--conference <name>', 'Filter by conference')
  .action(async (options) => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();
      const standings = await db.listStandings({
        leagueId: options.league,
        seasonYear: options.season ? parseInt(options.season, 10) : undefined,
        conference: options.conference,
      });
      await db.disconnect();

      if (standings.length === 0) {
        console.log('No standings found');
        process.exit(0);
      }

      console.log(`\nStandings (${standings.length} teams):\n`);
      console.log('  Rank  Team                W     L     Pct    GB    Streak');
      console.log('  ----  ----                ---   ---   -----  ----  ------');

      for (const s of standings) {
        const rank = String(s.rank_overall ?? '-').padStart(4);
        const team = (s.team_abbreviation ?? s.team_name).padEnd(20);
        const wins = String(s.wins).padStart(3);
        const losses = String(s.losses).padStart(3);
        const pct = s.win_percentage.toFixed(3).padStart(5);
        const gb = s.games_back !== null ? String(s.games_back).padStart(4) : '   -';
        const streak = (s.streak ?? '-').padStart(6);

        console.log(`  ${rank}  ${team}${wins}   ${losses}   ${pct}  ${gb}  ${streak}`);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get standings', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Sync command
program
  .command('sync')
  .description('Trigger data sync')
  .option('--provider <name>', 'Provider to sync from')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const provider = options.provider ?? config.provider;

      console.log(`Triggering sync from provider "${provider}"...`);
      console.log(`Server should be running on port ${config.port}`);
      console.log('Use POST /api/sync endpoint to trigger sync');

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync trigger failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Sync status command
program
  .command('sync-status')
  .description('Check sync status')
  .action(async () => {
    try {
      const db = new SportsDataDatabase();
      await db.connect();
      const syncStates = await db.getSyncStatus();
      await db.disconnect();

      if (syncStates.length === 0) {
        console.log('No sync state data available');
        process.exit(0);
      }

      console.log('\nSync Status:');
      console.log('============');
      for (const state of syncStates) {
        console.log(`${state.provider}/${state.resource_type}: ${state.status}`);
        if (state.last_sync_at) {
          console.log(`  Last sync: ${state.last_sync_at.toISOString()}`);
        }
        if (state.error) {
          console.log(`  Error:     ${state.error}`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync status check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
