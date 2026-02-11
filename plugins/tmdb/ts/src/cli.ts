#!/usr/bin/env node
/**
 * TMDB Plugin CLI
 * Command-line interface for TMDB metadata operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { TmdbDatabase } from './database.js';

const logger = createLogger('tmdb:cli');
const program = new Command();

program
  .name('nself-tmdb')
  .description('TMDB metadata plugin CLI for nself')
  .version('1.0.0');

// ============================================================================
// Init Command
// ============================================================================

program
  .command('init')
  .description('Initialize TMDB database schema')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tmdbDb = new TmdbDatabase(db);
      await tmdbDb.initializeSchema();
      logger.success('TMDB plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize TMDB plugin', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Server Command
// ============================================================================

program
  .command('server')
  .description('Start TMDB HTTP server')
  .action(async () => {
    try {
      logger.info('Starting TMDB server...');
      const { start } = await import('./server.js');
      await start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start TMDB server', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Search Command
// ============================================================================

program
  .command('search')
  .description('Search TMDB for movies or TV shows')
  .requiredOption('--query <query>', 'Search query')
  .option('--type <type>', 'Media type (movie|tv)', 'movie')
  .option('--year <year>', 'Release year')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const response = await fetch(
        `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}/api/search/${options.type}?query=${encodeURIComponent(options.query)}${options.year ? `&year=${options.year}` : ''}`,
        { headers: { 'X-App-Name': options.appId } }
      );
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Match Command
// ============================================================================

program
  .command('match')
  .description('Match a media file to TMDB')
  .requiredOption('--media-id <mediaId>', 'Media ID to match')
  .option('--filename <filename>', 'Original filename for parsing')
  .option('--title <title>', 'Title to search')
  .option('--year <year>', 'Release year')
  .option('--type <type>', 'Media type (movie|tv)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const body: Record<string, unknown> = { mediaId: options.mediaId };
      if (options.filename) body.filename = options.filename;
      if (options.title) body.title = options.title;
      if (options.year) body.year = parseInt(options.year, 10);
      if (options.type) body.type = options.type;

      const response = await fetch(
        `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}/api/match`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
          body: JSON.stringify(body),
        }
      );
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Match failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Queue Command
// ============================================================================

program
  .command('queue')
  .description('View match review queue')
  .option('--status <status>', 'Filter by status (pending|accepted|rejected|manual)', 'pending')
  .option('--limit <limit>', 'Limit results', '50')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tmdbDb = new TmdbDatabase(db).forSourceAccount(options.appId);

      const result = await tmdbDb.listMatchQueue(options.status, parseInt(options.limit, 10));
      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list queue', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Confirm Command
// ============================================================================

program
  .command('confirm')
  .description('Confirm a match')
  .requiredOption('--match-id <matchId>', 'Match queue ID')
  .requiredOption('--tmdb-id <tmdbId>', 'TMDB ID to confirm')
  .option('--tmdb-type <tmdbType>', 'TMDB type (movie|tv)', 'movie')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tmdbDb = new TmdbDatabase(db).forSourceAccount(options.appId);

      const updated = await tmdbDb.updateMatchStatus(
        options.matchId, 'accepted', 'cli',
        parseInt(options.tmdbId, 10), options.tmdbType
      );

      if (!updated) {
        logger.error('Match entry not found');
        process.exit(1);
      }

      logger.success('Match confirmed', { matchId: options.matchId, tmdbId: options.tmdbId });
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to confirm match', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Refresh Command
// ============================================================================

program
  .command('refresh')
  .description('Refresh cached metadata from TMDB')
  .requiredOption('--type <type>', 'Media type (movie|tv)')
  .requiredOption('--id <id>', 'TMDB ID to refresh')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const response = await fetch(
        `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}/api/refresh/${options.type}/${options.id}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        }
      );
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Refresh failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Sync Command
// ============================================================================

program
  .command('sync')
  .description('Sync genres and configuration from TMDB')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const response = await fetch(
        `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}/api/sync`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        }
      );
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Status Command
// ============================================================================

program
  .command('status')
  .description('Show TMDB plugin status')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tmdbDb = new TmdbDatabase(db).forSourceAccount(options.appId);

      const status = await tmdbDb.getStatus();
      console.log(JSON.stringify(status, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get status', { error: message });
      process.exit(1);
    }
  });

program.parse();
