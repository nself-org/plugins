#!/usr/bin/env node
/**
 * Photos Plugin CLI
 * Command-line interface for photo album management operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { PhotosDatabase } from './database.js';

const logger = createLogger('photos:cli');
const program = new Command();

program
  .name('nself-photos')
  .description('Photos plugin CLI for nself')
  .version('1.0.0');

// ============================================================================
// Init Command
// ============================================================================

program
  .command('init')
  .description('Initialize photos database schema')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db);
      await photosDb.initializeSchema();
      logger.success('Photos plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize photos plugin', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Server Command
// ============================================================================

program
  .command('server')
  .description('Start photos HTTP server')
  .action(async () => {
    try {
      logger.info('Starting photos server...');
      const { start } = await import('./server.js');
      await start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start photos server', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Albums Command
// ============================================================================

const albumsCmd = program.command('albums').description('Manage photo albums');

albumsCmd
  .command('list')
  .description('List albums')
  .option('--owner <ownerId>', 'Filter by owner ID')
  .option('--visibility <visibility>', 'Filter by visibility (public|private|shared)')
  .option('--limit <limit>', 'Limit results', '50')
  .option('--offset <offset>', 'Offset results', '0')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);

      const result = await photosDb.listAlbums(
        options.owner, options.visibility,
        parseInt(options.limit, 10), parseInt(options.offset, 10)
      );
      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list albums', { error: message });
      process.exit(1);
    }
  });

albumsCmd
  .command('create')
  .description('Create a new album')
  .requiredOption('--name <name>', 'Album name')
  .option('--description <description>', 'Album description')
  .option('--owner <ownerId>', 'Owner ID', 'system')
  .option('--visibility <visibility>', 'Visibility (public|private|shared)', 'private')
  .option('--sort-order <sortOrder>', 'Sort order (date_asc|date_desc|name_asc)', 'date_desc')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/albums`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-App-Name': options.appId,
          'X-User-Id': options.owner,
        },
        body: JSON.stringify({
          name: options.name,
          description: options.description,
          visibility: options.visibility,
          sortOrder: options.sortOrder,
        }),
      });
      const data = await response.json();
      if (!response.ok) {
        logger.error('Failed to create album', { error: (data as Record<string, string>).message || (data as Record<string, string>).error });
        process.exit(1);
      }
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create album', { error: message });
      process.exit(1);
    }
  });

albumsCmd
  .command('get')
  .description('Get album details')
  .requiredOption('--id <albumId>', 'Album ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);

      const album = await photosDb.getAlbum(options.id);
      if (!album) {
        logger.error('Album not found');
        process.exit(1);
      }
      console.log(JSON.stringify(album, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get album', { error: message });
      process.exit(1);
    }
  });

albumsCmd
  .command('delete')
  .description('Delete an album')
  .requiredOption('--id <albumId>', 'Album ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/albums/${options.id}`, {
        method: 'DELETE',
        headers: { 'X-App-Name': options.appId },
      });
      if (!response.ok && response.status !== 204) {
        const data = await response.json();
        logger.error('Failed to delete album', { error: (data as Record<string, string>).message || (data as Record<string, string>).error });
        process.exit(1);
      }
      logger.success('Album deleted', { albumId: options.id });
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete album', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// List Photos Command
// ============================================================================

program
  .command('list')
  .description('List photos')
  .option('--album <albumId>', 'Filter by album ID')
  .option('--uploader <uploaderId>', 'Filter by uploader ID')
  .option('--limit <limit>', 'Limit results', '20')
  .option('--offset <offset>', 'Offset results', '0')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);

      const result = await photosDb.listPhotos({
        albumId: options.album,
        uploaderId: options.uploader,
        limit: parseInt(options.limit, 10),
        offset: parseInt(options.offset, 10),
      });
      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list photos', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Process Pending Command
// ============================================================================

program
  .command('process-pending')
  .description('Process pending photos (EXIF extraction, thumbnails)')
  .option('--limit <limit>', 'Maximum photos to process', String(config.processingConcurrency))
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/photos/process-pending`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
      });
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Processing failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Tags Command
// ============================================================================

const tagsCmd = program.command('tags').description('Manage photo tags');

tagsCmd
  .command('list')
  .description('List tags')
  .option('--type <tagType>', 'Filter by tag type (keyword|person|location|event)')
  .option('--top <limit>', 'Limit results', '50')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);

      const tags = await photosDb.listTags(options.type, parseInt(options.top, 10));
      console.log(JSON.stringify({ tags }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list tags', { error: message });
      process.exit(1);
    }
  });

tagsCmd
  .command('add')
  .description('Add a tag to a photo')
  .requiredOption('--photo <photoId>', 'Photo ID')
  .requiredOption('--type <tagType>', 'Tag type (keyword|person|location|event)')
  .requiredOption('--value <tagValue>', 'Tag value')
  .option('--user-id <taggedUserId>', 'Tagged user ID (for person tags)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/photos/${options.photo}/tags`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify({
          tagType: options.type,
          tagValue: options.value,
          taggedUserId: options.userId,
        }),
      });
      const data = await response.json();
      if (!response.ok) {
        logger.error('Failed to add tag', { error: (data as Record<string, string>).message || (data as Record<string, string>).error });
        process.exit(1);
      }
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to add tag', { error: message });
      process.exit(1);
    }
  });

tagsCmd
  .command('photos')
  .description('Get photos with a specific tag')
  .requiredOption('--value <tagValue>', 'Tag value')
  .option('--limit <limit>', 'Limit results', '50')
  .option('--offset <offset>', 'Offset results', '0')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);

      const result = await photosDb.getPhotosWithTag(
        options.value,
        parseInt(options.limit, 10),
        parseInt(options.offset, 10)
      );
      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get photos with tag', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Faces Command
// ============================================================================

const facesCmd = program.command('faces').description('Manage detected faces');

facesCmd
  .command('list')
  .description('List detected face groups')
  .option('--limit <limit>', 'Limit results', '50')
  .option('--offset <offset>', 'Offset results', '0')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);

      const result = await photosDb.listFaces(
        parseInt(options.limit, 10),
        parseInt(options.offset, 10)
      );
      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list faces', { error: message });
      process.exit(1);
    }
  });

facesCmd
  .command('identify')
  .description('Identify a face group')
  .requiredOption('--id <faceId>', 'Face group ID')
  .option('--name <name>', 'Person name')
  .option('--user-id <userId>', 'User ID to associate')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/faces/${options.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify({
          name: options.name,
          userId: options.userId,
        }),
      });
      const data = await response.json();
      if (!response.ok) {
        logger.error('Failed to identify face', { error: (data as Record<string, string>).message || (data as Record<string, string>).error });
        process.exit(1);
      }
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to identify face', { error: message });
      process.exit(1);
    }
  });

facesCmd
  .command('merge')
  .description('Merge two face groups')
  .requiredOption('--target <targetId>', 'Target face group ID (to keep)')
  .requiredOption('--merge-with <mergeWithId>', 'Face group ID to merge into target')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/faces/${options.target}/merge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify({ mergeWithId: options.mergeWith }),
      });
      const data = await response.json();
      if (!response.ok) {
        logger.error('Failed to merge faces', { error: (data as Record<string, string>).message || (data as Record<string, string>).error });
        process.exit(1);
      }
      logger.success('Faces merged', { targetId: options.target, mergedId: options.mergeWith });
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to merge faces', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Search Command
// ============================================================================

program
  .command('search')
  .description('Search photos')
  .requiredOption('--query <query>', 'Search query')
  .option('--tags <tags>', 'Comma-separated tag values')
  .option('--location <location>', 'Location filter')
  .option('--date-from <dateFrom>', 'Date range start (ISO)')
  .option('--date-to <dateTo>', 'Date range end (ISO)')
  .option('--uploader <uploaderId>', 'Filter by uploader')
  .option('--album <albumId>', 'Filter by album')
  .option('--limit <limit>', 'Limit results', '50')
  .option('--offset <offset>', 'Offset results', '0')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const body: Record<string, unknown> = {
        query: options.query,
        limit: parseInt(options.limit, 10),
        offset: parseInt(options.offset, 10),
      };

      if (options.tags) body.tags = options.tags.split(',').map((t: string) => t.trim());
      if (options.location) body.location = options.location;
      if (options.dateFrom) body.dateFrom = options.dateFrom;
      if (options.dateTo) body.dateTo = options.dateTo;
      if (options.uploader) body.uploaderId = options.uploader;
      if (options.album) body.albumId = options.album;

      const response = await fetch(`${baseUrl}/api/photos/search`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify(body),
      });
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
// Timeline Command
// ============================================================================

program
  .command('timeline')
  .description('View photo timeline')
  .option('--granularity <granularity>', 'Period granularity (day|week|month|year)', 'month')
  .option('--from <from>', 'Start date (ISO)')
  .option('--to <to>', 'End date (ISO)')
  .option('--user <userId>', 'Filter by user')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const params = new URLSearchParams();
      if (options.granularity) params.set('granularity', options.granularity);
      if (options.from) params.set('from', options.from);
      if (options.to) params.set('to', options.to);
      if (options.user) params.set('userId', options.user);

      const response = await fetch(`${baseUrl}/api/timeline?${params.toString()}`, {
        headers: { 'X-App-Name': options.appId },
      });
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get timeline', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Stats Command
// ============================================================================

program
  .command('stats')
  .description('Show photos statistics')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);
      const stats = await photosDb.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get stats', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Status Command
// ============================================================================

program
  .command('status')
  .description('Show photos plugin status')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const photosDb = new PhotosDatabase(db).forSourceAccount(options.appId);
      const stats = await photosDb.getStats();
      console.log(JSON.stringify({
        plugin: 'photos',
        version: '1.0.0',
        port: config.port,
        sourceAccountId: options.appId,
        stats,
      }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get status', { error: message });
      process.exit(1);
    }
  });

program.parse();
