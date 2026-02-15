#!/usr/bin/env node
/**
 * Game Metadata Plugin CLI
 * Command-line interface for the game metadata plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { GameMetadataDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('game-metadata:cli');

const program = new Command();

program
  .name('nself-game-metadata')
  .description('Game metadata plugin for nself - IGDB integration, ROM hash matching, and artwork management')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new GameMetadataDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for game-metadata plugin');
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
  .description('Start the game metadata API server')
  .option('-p, --port <port>', 'Server port', '3211')
  .option('-h, --host <host>', 'Server host', '127.0.0.1')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting game metadata server on ${config.host}:${config.port}`);
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
  .description('Show game metadata statistics')
  .action(async () => {
    try {
      const db = new GameMetadataDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nGame Metadata Statistics:');
      console.log('========================');
      console.log(`Total Games:      ${stats.total_games}`);
      console.log(`Verified Games:   ${stats.verified_games}`);
      console.log(`Total Platforms:  ${stats.total_platforms}`);
      console.log(`Total Genres:     ${stats.total_genres}`);
      console.log(`Total Artwork:    ${stats.total_artwork}`);
      console.log(`Total Metadata:   ${stats.total_metadata}`);
      console.log(`Games with IGDB:  ${stats.games_with_igdb}`);
      console.log(`Games with Hash:  ${stats.games_with_hashes}`);

      if (Object.keys(stats.tier_breakdown).length > 0) {
        console.log('\nTier Breakdown:');
        for (const [tier, count] of Object.entries(stats.tier_breakdown)) {
          console.log(`  ${tier}: ${count}`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Lookup command
program
  .command('lookup')
  .description('Lookup game by name or hash')
  .option('-t, --title <title>', 'Game title to search')
  .option('--hash <hash>', 'ROM hash value')
  .option('--hash-type <type>', 'Hash type (md5, sha1, sha256, crc32)', 'md5')
  .option('--platform <platform>', 'Platform slug filter')
  .action(async (options) => {
    try {
      if (!options.title && !options.hash) {
        console.error('Error: Provide --title or --hash');
        process.exit(1);
      }

      const db = new GameMetadataDatabase();
      await db.connect();

      if (options.hash) {
        const game = await db.lookupByHash(options.hash, options.hashType);
        if (game) {
          console.log(`\nFound game by ${options.hashType} hash:\n`);
          console.log(`  ${game.title}`);
          console.log(`    ID:        ${game.id}`);
          console.log(`    Developer: ${game.developer ?? 'Unknown'}`);
          console.log(`    Publisher: ${game.publisher ?? 'Unknown'}`);
          console.log(`    Tier:      ${game.tier ?? 'Untiered'}`);
          console.log(`    Verified:  ${game.is_verified ? 'Yes' : 'No'}`);
        } else {
          console.log(`No game found with ${options.hashType} hash: ${options.hash}`);
        }
      } else if (options.title) {
        const games = await db.searchGames({
          query: options.title,
          limit: 10,
        });

        if (games.length === 0) {
          console.log(`No games found matching "${options.title}"`);
        } else {
          console.log(`\nFound ${games.length} game(s) matching "${options.title}":\n`);
          for (const game of games) {
            const verified = game.is_verified ? ' [verified]' : '';
            const tier = game.tier ? ` [${game.tier}]` : '';
            console.log(`  ${game.title}${tier}${verified}`);
            console.log(`    ID:        ${game.id}`);
            console.log(`    Developer: ${game.developer ?? 'Unknown'}`);
            console.log(`    Publisher: ${game.publisher ?? 'Unknown'}`);
            console.log('');
          }
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Lookup failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Enrich command
program
  .command('enrich')
  .description('Enrich game metadata from IGDB')
  .requiredOption('--game-id <id>', 'Game ID to enrich')
  .option('--force', 'Force re-enrichment even if already enriched')
  .action(async (options) => {
    try {
      const config = loadConfig();

      if (!config.igdbClientId || !config.igdbClientSecret) {
        console.error('Error: IGDB credentials not configured.');
        console.error('Set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET environment variables.');
        process.exit(1);
      }

      console.log(`Enriching game ${options.gameId} from IGDB...`);
      console.log(`Server should be running on port ${config.port}`);
      console.log('Use POST /api/enrich endpoint for full enrichment.');

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Enrichment failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Tiers command
program
  .command('tiers')
  .description('Show tier requirements')
  .action(async () => {
    console.log('\nGame Tier Requirements:');
    console.log('======================\n');

    const tiers = [
      { tier: 'S', label: 'Masterpiece', rating: '90+', reqs: 'IGDB verified, ROM hash, full artwork, full metadata' },
      { tier: 'A', label: 'Excellent', rating: '80+', reqs: 'IGDB verified, ROM hash, cover artwork' },
      { tier: 'B', label: 'Good', rating: '70+', reqs: 'ROM hash required' },
      { tier: 'C', label: 'Average', rating: '50+', reqs: 'No special requirements' },
      { tier: 'D', label: 'Below Average', rating: 'Any', reqs: 'No special requirements' },
    ];

    for (const t of tiers) {
      console.log(`  ${t.tier}-Tier (${t.label})`);
      console.log(`    Min Rating:    ${t.rating}`);
      console.log(`    Requirements:  ${t.reqs}`);
      console.log('');
    }

    process.exit(0);
  });

// Platforms command
program
  .command('platforms')
  .description('List platforms')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new GameMetadataDatabase();
      await db.connect();

      const platforms = await db.listPlatforms({
        isActive: true,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (platforms.length === 0) {
        console.log('No platforms found');
        process.exit(0);
      }

      console.log(`\nFound ${platforms.length} platform(s):\n`);
      for (const p of platforms) {
        const abbr = p.abbreviation ? ` (${p.abbreviation})` : '';
        const gen = p.generation ? ` [Gen ${p.generation}]` : '';
        const mfg = p.manufacturer ? ` - ${p.manufacturer}` : '';

        console.log(`  ${p.name}${abbr}${gen}${mfg}`);
        console.log(`    ID:   ${p.id}`);
        console.log(`    Slug: ${p.slug}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list platforms', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
