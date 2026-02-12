#!/usr/bin/env node
/**
 * Retro Gaming Plugin CLI
 * Command-line interface for managing ROMs, emulator cores, save states, and play sessions
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { RetroGamingDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('retro-gaming:cli');

const program = new Command();

program
  .name('nself-retro-gaming')
  .description('Retro gaming plugin for nself - ROM library, emulator cores, save states, and play sessions')
  .version('1.0.0');

// =========================================================================
// Server command
// =========================================================================

program
  .command('server')
  .description('Start the retro-gaming API server')
  .option('-p, --port <port>', 'Server port', '3033')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting retro-gaming server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Init command
// =========================================================================

program
  .command('init')
  .description('Initialize database schema and seed default cores')
  .action(async () => {
    try {
      loadConfig();
      const db = new RetroGamingDatabase();
      await db.connect();
      await db.initializeSchema();
      const coresSeeded = await db.seedDefaultCores();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for retro-gaming plugin');
      console.log(`Seeded ${coresSeeded} default emulator cores`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// =========================================================================
// ROMs commands
// =========================================================================

const romsCmd = program.command('roms').description('Manage ROM library');

romsCmd
  .command('list')
  .description('List ROMs in the library')
  .option('--platform <platform>', 'Filter by platform (nes, snes, gb, gba, genesis, n64, ps1, arcade)')
  .option('--genre <genre>', 'Filter by genre')
  .option('--favorites', 'Show only favorites')
  .option('--search <query>', 'Search by title')
  .option('--sort <sort>', 'Sort order: title, recent, added, most_played, platform', 'title')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();

      const roms = await db.listRoms({
        platform: options.platform,
        genre: options.genre,
        favorite: options.favorites ? true : undefined,
        search: options.search,
        sort: options.sort,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (roms.length === 0) {
        console.log('No ROMs found');
        process.exit(0);
      }

      console.log(`\nFound ${roms.length} ROM(s):\n`);
      for (const rom of roms) {
        const fav = rom.favorite ? ' [FAV]' : '';
        const plays = rom.play_count > 0 ? ` (${rom.play_count} plays)` : '';

        console.log(`  ${rom.game_title}${fav}${plays}`);
        console.log(`    ID:       ${rom.id}`);
        console.log(`    Platform: ${rom.platform}`);
        if (rom.genre) console.log(`    Genre:    ${rom.genre}`);
        if (rom.release_year) console.log(`    Year:     ${rom.release_year}`);
        if (rom.publisher) console.log(`    Publisher: ${rom.publisher}`);
        if (rom.recommended_core) console.log(`    Core:     ${rom.recommended_core}`);
        console.log(`    Path:     ${rom.rom_file_path}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list ROMs', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

romsCmd
  .command('stats')
  .description('Show ROM library statistics')
  .action(async () => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nROM Library Statistics:');
      console.log('======================');
      console.log(`Total ROMs:          ${stats.total_roms}`);
      console.log(`Platforms:           ${stats.total_platforms}`);
      console.log(`Favorites:           ${stats.total_favorites}`);
      console.log(`Total Play Sessions: ${stats.total_play_sessions}`);

      const hours = Math.floor(stats.total_play_time_seconds / 3600);
      const mins = Math.floor((stats.total_play_time_seconds % 3600) / 60);
      console.log(`Total Play Time:     ${hours}h ${mins}m`);

      console.log(`Save States:         ${stats.total_save_states}`);

      if (stats.roms_by_platform.length > 0) {
        console.log('\nROMs by Platform:');
        for (const p of stats.roms_by_platform) {
          console.log(`  ${p.platform.padEnd(12)} ${p.count}`);
        }
      }

      if (stats.most_played.length > 0) {
        console.log('\nMost Played:');
        for (const m of stats.most_played) {
          console.log(`  ${m.game_title} (${m.play_count} plays)`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get ROM stats', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// =========================================================================
// Cores commands
// =========================================================================

const coresCmd = program.command('cores').description('Manage emulator cores');

coresCmd
  .command('list')
  .description('List available emulator cores')
  .option('--platform <platform>', 'Filter by platform')
  .action(async (options) => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();
      const cores = await db.listCores(options.platform);
      await db.disconnect();

      if (cores.length === 0) {
        console.log('No emulator cores found. Run "nself-retro-gaming cores seed" to add defaults.');
        process.exit(0);
      }

      console.log(`\nFound ${cores.length} emulator core(s):\n`);
      for (const core of cores) {
        const recommended = core.is_recommended ? ' [RECOMMENDED]' : '';
        const features: string[] = [];
        if (core.supports_save_states) features.push('saves');
        if (core.supports_rewind) features.push('rewind');
        if (core.supports_fast_forward) features.push('fast-forward');
        if (core.supports_cheats) features.push('cheats');

        console.log(`  ${core.display_name}${recommended}`);
        console.log(`    Core:     ${core.core_name}`);
        console.log(`    Platform: ${core.platform}`);
        console.log(`    Version:  ${core.version}`);
        if (core.license) console.log(`    License:  ${core.license}`);
        console.log(`    Features: ${features.join(', ')}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list cores', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

coresCmd
  .command('seed')
  .description('Seed default emulator cores into the database')
  .action(async () => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();
      await db.initializeSchema();
      const count = await db.seedDefaultCores();
      await db.disconnect();

      console.log(`Seeded ${count} default emulator cores`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to seed cores', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// =========================================================================
// Save States commands
// =========================================================================

program
  .command('save-states')
  .description('Manage save states')
  .requiredOption('--rom <uuid>', 'ROM ID')
  .action(async (options) => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();

      const rom = await db.getRom(options.rom);
      if (!rom) {
        console.error('ROM not found');
        process.exit(1);
      }

      const states = await db.listSaveStates(options.rom);
      await db.disconnect();

      if (states.length === 0) {
        console.log(`No save states found for "${rom.game_title}"`);
        process.exit(0);
      }

      console.log(`\nSave States for "${rom.game_title}" (${states.length}):\n`);
      for (const state of states) {
        const playtime = state.play_time_seconds > 0
          ? `${Math.floor(state.play_time_seconds / 3600)}h ${Math.floor((state.play_time_seconds % 3600) / 60)}m`
          : 'N/A';

        console.log(`  Slot ${state.slot}`);
        console.log(`    ID:        ${state.id}`);
        console.log(`    Core:      ${state.emulator_core}`);
        console.log(`    Play Time: ${playtime}`);
        if (state.description) console.log(`    Desc:      ${state.description}`);
        console.log(`    Created:   ${state.created_at}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list save states', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// =========================================================================
// Sessions commands
// =========================================================================

const sessionsCmd = program.command('sessions').description('View play sessions');

sessionsCmd
  .command('recent')
  .description('Show recent play sessions')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (options) => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();
      const sessions = await db.listRecentSessions(parseInt(options.limit, 10));
      await db.disconnect();

      if (sessions.length === 0) {
        console.log('No play sessions found');
        process.exit(0);
      }

      console.log(`\nRecent Play Sessions (${sessions.length}):\n`);
      for (const session of sessions) {
        const duration = session.duration_seconds
          ? `${Math.floor(session.duration_seconds / 60)}m ${session.duration_seconds % 60}s`
          : 'In progress';
        const status = session.ended_at ? 'Completed' : 'Active';

        console.log(`  ${session.game_title}`);
        console.log(`    Started:  ${session.started_at}`);
        console.log(`    Duration: ${duration}`);
        console.log(`    Status:   ${status}`);
        console.log(`    Core:     ${session.emulator_core}`);
        console.log(`    Platform: ${session.platform}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list sessions', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// =========================================================================
// Stats command (top-level)
// =========================================================================

program
  .command('stats')
  .alias('status')
  .description('Show overall retro-gaming plugin statistics')
  .action(async () => {
    try {
      const db = new RetroGamingDatabase();
      await db.connect();
      const stats = await db.getStats();
      const cores = await db.listCores();
      await db.disconnect();

      console.log('\nRetro Gaming Plugin Statistics:');
      console.log('==============================');
      console.log(`Total ROMs:          ${stats.total_roms}`);
      console.log(`Platforms:           ${stats.total_platforms}`);
      console.log(`Favorites:           ${stats.total_favorites}`);
      console.log(`Emulator Cores:      ${cores.length}`);
      console.log(`Play Sessions:       ${stats.total_play_sessions}`);
      console.log(`Save States:         ${stats.total_save_states}`);

      const hours = Math.floor(stats.total_play_time_seconds / 3600);
      const mins = Math.floor((stats.total_play_time_seconds % 3600) / 60);
      console.log(`Total Play Time:     ${hours}h ${mins}m`);

      if (stats.roms_by_platform.length > 0) {
        console.log('\nROMs by Platform:');
        for (const p of stats.roms_by_platform) {
          console.log(`  ${p.platform.padEnd(12)} ${p.count}`);
        }
      }

      if (stats.most_played.length > 0) {
        console.log('\nMost Played:');
        for (const m of stats.most_played.slice(0, 5)) {
          console.log(`  ${m.game_title} (${m.play_count} plays)`);
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

program.parse();
