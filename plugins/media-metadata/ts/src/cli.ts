#!/usr/bin/env node
/**
 * TMDB Plugin CLI
 * Command-line interface for the TMDB plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { TmdbDatabase } from './database.js';
import { TmdbClient } from './client.js';
import { TmdbLookupService } from './lookup.js';
import { createServer } from './server.js';

const logger = createLogger('tmdb:cli');

const program = new Command();

program
  .name('nself-media-metadata')
  .description('TMDB media metadata plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      logger.info('Initializing TMDB database schema...');

      const db = new TmdbDatabase();
      await db.connect();
      await db.initializeSchema();

      logger.info('Schema initialized successfully');

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Init failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the TMDB API server')
  .option('-p, --port <port>', 'Server port', '3202')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting TMDB server on ${config.host}:${config.port}`);

      const server = await createServer(config);

      // Graceful shutdown
      process.on('SIGINT', async () => {
        logger.info('Shutting down...');
        await server.close();
        process.exit(0);
      });

    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show plugin status and statistics')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      const db = new TmdbDatabase();

      await db.connect();

      const stats = await db.getStats();

      console.log('\nTMDB Plugin Status');
      console.log('==================');
      console.log(`Movies:       ${stats.movies}`);
      console.log(`TV Shows:     ${stats.tvShows}`);
      console.log(`Seasons:      ${stats.seasons}`);
      console.log(`Episodes:     ${stats.episodes}`);
      console.log(`Genres:       ${stats.genres}`);
      console.log(`Match Queue:  ${stats.matchQueue}`);
      console.log(`Last Synced:  ${stats.lastSyncedAt?.toISOString() ?? 'Never'}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Search command
program
  .command('search')
  .description('Search TMDB for movies or TV shows')
  .argument('<query>', 'Search query')
  .option('-t, --type <type>', 'Media type (movie or tv)', 'movie')
  .option('-y, --year <year>', 'Release year')
  .action(async (query, options) => {
    try {
      const config = loadConfig();
      const client = new TmdbClient(config.tmdbApiKey, config.tmdbDefaultLanguage);

      logger.info(`Searching for: ${query}`);

      const result = await client.search({
        query,
        media_type: options.type as 'movie' | 'tv',
        year: options.year ? parseInt(options.year, 10) : undefined,
      });

      console.log(`\nFound ${result.total_results} results (page ${result.page}/${result.total_pages}):\n`);

      for (const item of result.results.slice(0, 10)) {
        const title = item.title ?? item.name ?? 'Unknown';
        const date = item.release_date ?? item.first_air_date ?? 'N/A';
        const year = date ? date.substring(0, 4) : 'N/A';

        console.log(`[${item.id}] ${title} (${year})`);
        console.log(`    Rating: ${item.vote_average}/10 | Popularity: ${item.popularity.toFixed(1)}`);
        if (item.overview) {
          console.log(`    ${item.overview.substring(0, 100)}...`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      process.exit(1);
    }
  });

// Lookup command
program
  .command('lookup')
  .description('Lookup media by title with confidence scoring')
  .argument('<title>', 'Media title')
  .option('-t, --type <type>', 'Media type (movie or tv)', 'movie')
  .option('-y, --year <year>', 'Release year')
  .action(async (title, options) => {
    try {
      const config = loadConfig();
      const db = new TmdbDatabase();
      await db.connect();

      const client = new TmdbClient(config.tmdbApiKey, config.tmdbDefaultLanguage);
      const lookupService = new TmdbLookupService(client, db, config.tmdbConfidenceThreshold);

      logger.info(`Looking up: ${title}`);

      const result = await lookupService.lookup({
        title,
        year: options.year ? parseInt(options.year, 10) : undefined,
        media_type: options.type as 'movie' | 'tv',
      });

      console.log('\nLookup Result:');
      console.log('==============');
      console.log(`Matched: ${result.matched ? 'Yes' : 'No'}`);
      console.log(`Confidence: ${(result.confidence * 100).toFixed(1)}%`);

      if (result.matched && result.tmdb_id) {
        console.log(`TMDB ID: ${result.tmdb_id}`);
        console.log(`Title: ${result.title}`);
        console.log(`Year: ${result.year ?? 'N/A'}`);
        console.log(`Type: ${result.media_type}`);
      }

      if (result.candidates.length > 0) {
        console.log('\nTop Candidates:');
        for (const candidate of result.candidates.slice(0, 5)) {
          console.log(`  - [${candidate.tmdb_id}] ${candidate.title} (${candidate.year ?? 'N/A'})`);
          console.log(`    Confidence: ${(candidate.confidence * 100).toFixed(1)}%`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Lookup failed', { error: message });
      process.exit(1);
    }
  });

// Enrich command
program
  .command('enrich')
  .description('Enrich media with TMDB metadata')
  .argument('<title>', 'Media title')
  .option('-t, --type <type>', 'Media type (movie or tv)', 'movie')
  .option('-y, --year <year>', 'Release year')
  .option('-f, --force', 'Force re-fetch even if cached')
  .action(async (title, options) => {
    try {
      const config = loadConfig();
      const db = new TmdbDatabase();
      await db.connect();
      await db.initializeSchema();

      const client = new TmdbClient(config.tmdbApiKey, config.tmdbDefaultLanguage);
      const lookupService = new TmdbLookupService(client, db, config.tmdbConfidenceThreshold);

      logger.info(`Enriching: ${title}`);

      const result = await lookupService.enrich({
        title,
        year: options.year ? parseInt(options.year, 10) : undefined,
        media_type: options.type as 'movie' | 'tv',
        force: options.force,
      });

      console.log('\nEnrichment Result:');
      console.log('==================');
      console.log(`Success: ${result.success ? 'Yes' : 'No'}`);
      console.log(`Cached: ${result.cached ? 'Yes' : 'No'}`);

      if (result.success && result.metadata) {
        console.log(`TMDB ID: ${result.tmdb_id}`);
        console.log(`Type: ${result.media_type}`);

        const meta = result.metadata;
        if ('title' in meta && typeof meta.title === 'string') {
          console.log(`Title: ${meta.title}`);
          const releaseDate = meta.release_date;
          if (releaseDate instanceof Date) {
            console.log(`Release Date: ${releaseDate.toISOString().substring(0, 10)}`);
          } else if (typeof releaseDate === 'string' || typeof releaseDate === 'number') {
            console.log(`Release Date: ${new Date(releaseDate).toISOString().substring(0, 10)}`);
          } else {
            console.log(`Release Date: N/A`);
          }
          console.log(`Runtime: ${meta.runtime_minutes ? `${meta.runtime_minutes} min` : 'N/A'}`);
          console.log(`Rating: ${meta.vote_average}/10 (${meta.vote_count} votes)`);
          console.log(`Popularity: ${Number(meta.popularity).toFixed(1)}`);
          console.log(`Genres: ${Array.isArray(meta.genres) ? meta.genres.join(', ') : 'N/A'}`);
        } else if ('name' in meta && typeof meta.name === 'string') {
          console.log(`Name: ${meta.name}`);
          const firstAirDate = meta.first_air_date;
          if (firstAirDate instanceof Date) {
            console.log(`First Air Date: ${firstAirDate.toISOString().substring(0, 10)}`);
          } else if (typeof firstAirDate === 'string' || typeof firstAirDate === 'number') {
            console.log(`First Air Date: ${new Date(firstAirDate).toISOString().substring(0, 10)}`);
          } else {
            console.log(`First Air Date: N/A`);
          }
          console.log(`Seasons: ${meta.number_of_seasons}`);
          console.log(`Episodes: ${meta.number_of_episodes}`);
          console.log(`Rating: ${meta.vote_average}/10 (${meta.vote_count} votes)`);
          console.log(`Popularity: ${Number(meta.popularity).toFixed(1)}`);
          console.log(`Genres: ${Array.isArray(meta.genres) ? meta.genres.join(', ') : 'N/A'}`);
        }
      } else {
        console.log('Failed to enrich media. Try manual lookup.');
      }

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Enrich failed', { error: message });
      process.exit(1);
    }
  });

// Sync genres command
program
  .command('sync-genres')
  .description('Sync genre list from TMDB')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new TmdbDatabase();
      await db.connect();
      await db.initializeSchema();

      const client = new TmdbClient(config.tmdbApiKey, config.tmdbDefaultLanguage);

      logger.info('Syncing genres from TMDB...');

      const [movieGenres, tvGenres] = await Promise.all([
        client.getMovieGenres(),
        client.getTvGenres(),
      ]);

      for (const genre of movieGenres.genres) {
        await db.upsertGenre({
          source_account_id: 'primary',
          tmdb_id: genre.id,
          name: genre.name,
          media_type: 'movie',
        });
      }

      for (const genre of tvGenres.genres) {
        await db.upsertGenre({
          source_account_id: 'primary',
          tmdb_id: genre.id,
          name: genre.name,
          media_type: 'tv',
        });
      }

      console.log(`\nSynced ${movieGenres.genres.length} movie genres`);
      console.log(`Synced ${tvGenres.genres.length} TV genres`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync genres failed', { error: message });
      process.exit(1);
    }
  });

// Match queue command
program
  .command('match-queue')
  .description('List items in match queue')
  .option('-s, --status <status>', 'Filter by status (pending, manual_review, matched, no_match)')
  .option('-l, --limit <limit>', 'Max items to show', '50')
  .action(async (options) => {
    try {
      const db = new TmdbDatabase();
      await db.connect();

      const items = await db.getMatchQueue(options.status, parseInt(options.limit, 10));

      console.log(`\nMatch Queue (${items.length} items):`);
      console.log('====================================');

      for (const item of items) {
        console.log(`\n[${item.id}] ${item.title} (${item.year ?? 'N/A'}) - ${item.media_type}`);
        console.log(`  Status: ${item.status}`);
        console.log(`  Confidence: ${item.confidence ? `${(item.confidence * 100).toFixed(1)}%` : 'N/A'}`);
        console.log(`  Source: ${item.source_plugin ?? 'Unknown'} / ${item.source_id ?? 'N/A'}`);
        console.log(`  Created: ${item.created_at.toISOString()}`);

        if (item.matched_tmdb_id) {
          console.log(`  Matched TMDB ID: ${item.matched_tmdb_id}`);
        }

        if (item.reviewed_by) {
          console.log(`  Reviewed by: ${item.reviewed_by} at ${item.reviewed_at?.toISOString() ?? 'N/A'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Match queue failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show detailed statistics')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      const db = new TmdbDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nTMDB Plugin Statistics');
      console.log('======================');
      console.log(`\nContent:`);
      console.log(`  Movies:       ${stats.movies}`);
      console.log(`  TV Shows:     ${stats.tvShows}`);
      console.log(`  Seasons:      ${stats.seasons}`);
      console.log(`  Episodes:     ${stats.episodes}`);
      console.log(`\nMetadata:`);
      console.log(`  Genres:       ${stats.genres}`);
      console.log(`\nMatching:`);
      console.log(`  Match Queue:  ${stats.matchQueue} pending`);
      console.log(`\nLast Synced:  ${stats.lastSyncedAt?.toISOString() ?? 'Never'}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
