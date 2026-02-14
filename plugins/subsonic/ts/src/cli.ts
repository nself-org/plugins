#!/usr/bin/env node
/**
 * Subsonic Plugin CLI
 * Command-line interface for the Subsonic plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { SubsonicDatabase } from './database.js';
import { createServer } from './server.js';
import { scanLibrary } from './library.js';

const logger = createLogger('subsonic:cli');

const program = new Command();

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m ${secs}s`;
  if (minutes > 0) return `${minutes}m ${secs}s`;
  return `${secs}s`;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

program
  .name('nself-subsonic')
  .description('Subsonic API server plugin for nself')
  .version('1.0.0');

// Server command
program
  .command('server')
  .description('Start the Subsonic API server')
  .option('-p, --port <port>', 'Server port', '3024')
  .option('-H, --host <host>', 'Server host', '0.0.0.0')
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

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new SubsonicDatabase();
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

// Scan command
program
  .command('scan')
  .description('Scan music library and index all files')
  .option('-m, --music-paths <paths>', 'Comma-separated music directory paths')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const musicPaths = options.musicPaths
        ? options.musicPaths.split(',').map((p: string) => p.trim())
        : config.musicPaths;

      logger.info('Starting library scan...');
      logger.info(`Music paths: ${musicPaths.join(', ')}`);

      const db = new SubsonicDatabase(config.sourceAccountId);
      await db.connect();
      await db.initializeSchema();

      const result = await scanLibrary(db, musicPaths, config.coverArtPath);

      console.log('\nScan Results');
      console.log('============');
      console.log(`Songs added:     ${result.songsAdded}`);
      console.log(`Songs updated:   ${result.songsUpdated}`);
      console.log(`Albums created:  ${result.albumsCreated}`);
      console.log(`Artists created: ${result.artistsCreated}`);
      console.log(`Duration:        ${(result.duration / 1000).toFixed(1)}s`);

      if (result.errors.length > 0) {
        console.log(`\nErrors (${result.errors.length}):`);
        result.errors.slice(0, 20).forEach(err => console.log(`  - ${err}`));
        if (result.errors.length > 20) {
          console.log(`  ... and ${result.errors.length - 20} more`);
        }
      }

      await db.disconnect();
      process.exit(result.errors.length > 0 ? 1 : 0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Scan failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show library statistics')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new SubsonicDatabase(config.sourceAccountId);
      await db.connect();

      const stats = await db.getStats();

      console.log('\nSubsonic Plugin Status');
      console.log('======================');
      console.log(`Music paths:    ${config.musicPaths.join(', ')}`);
      console.log(`Port:           ${config.port}`);
      console.log('');
      console.log('Library:');
      console.log(`  Artists:      ${stats.artists}`);
      console.log(`  Albums:       ${stats.albums}`);
      console.log(`  Songs:        ${stats.songs}`);
      console.log(`  Playlists:    ${stats.playlists}`);
      console.log(`  Scrobbles:    ${stats.scrobbles}`);
      console.log(`  Folders:      ${stats.musicFolders}`);
      console.log(`  Duration:     ${formatDuration(stats.totalDurationSeconds)}`);
      console.log(`  Total size:   ${formatBytes(stats.totalFileSizeBytes)}`);
      console.log(`  Last scan:    ${stats.lastScanAt?.toISOString() ?? 'Never'}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Artists command
program
  .command('artists')
  .description('List artists in the library')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new SubsonicDatabase(config.sourceAccountId);
      await db.connect();

      const artists = await db.listArtists();
      const limit = parseInt(options.limit, 10);

      console.log('\nArtists:');
      console.log('-'.repeat(60));
      artists.slice(0, limit).forEach(a => {
        const starred = a.starred ? ' *' : '';
        console.log(`${a.id.substring(0, 8)} | ${a.name} (${a.album_count} albums)${starred}`);
      });
      console.log(`\nTotal: ${artists.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Albums command
program
  .command('albums')
  .description('List albums in the library')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .option('-t, --type <type>', 'List type: newest, random, frequent, recent, alphabeticalByName', 'alphabeticalByName')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new SubsonicDatabase(config.sourceAccountId);
      await db.connect();

      const limit = parseInt(options.limit, 10);
      const albums = await db.listAlbums(options.type, limit, 0);

      console.log('\nAlbums:');
      console.log('-'.repeat(80));
      for (const album of albums) {
        let artistName = 'Unknown';
        if (album.artist_id) {
          const artist = await db.getArtistById(album.artist_id);
          if (artist) artistName = artist.name;
        }
        const year = album.year ? ` (${album.year})` : '';
        const starred = album.starred ? ' *' : '';
        console.log(`${album.id.substring(0, 8)} | ${album.title}${year} - ${artistName} [${album.song_count} songs]${starred}`);
      }
      console.log(`\nShowing: ${albums.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Playlists command
program
  .command('playlists')
  .description('List or manage playlists')
  .argument('[action]', 'Action: list, show, create, delete', 'list')
  .argument('[id]', 'Playlist ID (for show/delete)')
  .option('-n, --name <name>', 'Playlist name (for create)')
  .action(async (action, id, options) => {
    try {
      const config = loadConfig();
      const db = new SubsonicDatabase(config.sourceAccountId);
      await db.connect();

      switch (action) {
        case 'list': {
          const playlists = await db.getPlaylists();
          console.log('\nPlaylists:');
          console.log('-'.repeat(60));
          playlists.forEach(p => {
            console.log(`${p.id.substring(0, 8)} | ${p.name} [${p.song_count} songs, ${formatDuration(p.duration_seconds)}]`);
          });
          console.log(`\nTotal: ${playlists.length}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('Playlist ID required');
            process.exit(1);
          }
          const playlist = await db.getPlaylistById(id);
          if (!playlist) {
            logger.error('Playlist not found');
            process.exit(1);
          }
          const songs = await db.getPlaylistSongs(id);
          console.log(`\n${playlist.name}`);
          console.log('='.repeat(playlist.name.length));
          console.log(`Songs: ${playlist.song_count}`);
          console.log(`Duration: ${formatDuration(playlist.duration_seconds)}`);
          console.log(`Owner: ${playlist.owner}`);
          console.log(`Public: ${playlist.public}`);
          console.log('');
          songs.forEach((s, i) => {
            console.log(`${i + 1}. ${s.title} [${formatDuration(s.duration_seconds ?? 0)}]`);
          });
          break;
        }
        case 'create': {
          const name = options.name ?? 'New Playlist';
          const playlist = await db.createPlaylist(name, []);
          logger.success(`Created playlist: ${playlist.name} (${playlist.id})`);
          break;
        }
        case 'delete': {
          if (!id) {
            logger.error('Playlist ID required');
            process.exit(1);
          }
          await db.deletePlaylist(id);
          logger.success(`Deleted playlist: ${id}`);
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Genres command
program
  .command('genres')
  .description('List genres in the library')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new SubsonicDatabase(config.sourceAccountId);
      await db.connect();

      const genres = await db.getGenres();

      console.log('\nGenres:');
      console.log('-'.repeat(50));
      genres.forEach(g => {
        console.log(`${g.value} (${g.songCount} songs, ${g.albumCount} albums)`);
      });
      console.log(`\nTotal: ${genres.length}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
