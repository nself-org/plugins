#!/usr/bin/env node
/**
 * Media Scanner Plugin CLI
 * Command-line interface for scanning, parsing, probing, matching, and search
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { MediaScannerDatabase } from './database.js';
import { scanDirectories } from './scanner.js';
import { parseFilename } from './parser.js';
import { probeFile, checkFFprobeAvailable } from './probe.js';
import { TmdbMatcher, AUTO_ACCEPT_THRESHOLD, SUGGEST_THRESHOLD } from './matcher.js';
import { MediaSearchService } from './search.js';
import { createServer } from './server.js';
import type { ScanError } from './types.js';

const logger = createLogger('media-scanner:cli');

const program = new Command();

program
  .name('nself-media-scanner')
  .description('Media scanner plugin for nself - scan, parse, probe, match, and index media libraries')
  .version('1.0.0');

// ─── scan ───────────────────────────────────────────────────────────────────

program
  .command('scan')
  .description('Scan directories for media files')
  .argument('[paths...]', 'Directories to scan (overrides MEDIA_LIBRARY_PATHS)')
  .option('-r, --recursive', 'Scan recursively (default: true)', true)
  .option('--no-recursive', 'Do not scan recursively')
  .option('--probe', 'Also probe files with ffprobe', false)
  .option('--match', 'Also match against TMDB', false)
  .action(async (paths: string[], options) => {
    try {
      const config = loadConfig();
      const scanPaths = paths.length > 0 ? paths : config.libraryPaths;

      if (scanPaths.length === 0) {
        logger.error('No paths specified. Provide paths as arguments or set MEDIA_LIBRARY_PATHS');
        process.exit(1);
      }

      const db = new MediaScannerDatabase();
      await db.connect();
      await db.initializeSchema();

      const scan = await db.createScan(scanPaths, options.recursive);
      await db.updateScanState(scan.id, 'scanning', { started_at: new Date() });

      logger.info('Starting scan...', { paths: scanPaths, recursive: options.recursive });

      let totalFound = 0;
      let totalProcessed = 0;
      const errors: ScanError[] = [];

      const scanner = scanDirectories(scanPaths, options.recursive);
      let result = await scanner.next();

      while (!result.done) {
        const batch = result.value;
        totalFound += batch.length;

        for (const file of batch) {
          try {
            const parsed = parseFilename(file.filename);
            await db.upsertMediaFile(scan.id, file, parsed);
            totalProcessed++;

            if (totalProcessed % 100 === 0) {
              logger.info(`Processed ${totalProcessed} files...`);
            }
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push({ path: file.path, error: message, timestamp: new Date().toISOString() });
          }
        }

        result = await scanner.next();
      }

      // Probe if requested
      if (options.probe) {
        const ffprobeAvailable = await checkFFprobeAvailable();
        if (!ffprobeAvailable) {
          logger.warn('ffprobe not found in PATH, skipping probe step');
        } else {
          logger.info('Probing files...');
          const unprobed = await db.listUnprobed(500);
          let probed = 0;
          for (const file of unprobed) {
            try {
              const info = await probeFile(file.file_path);
              await db.updateMediaProbe(file.id, info);
              probed++;
              if (probed % 50 === 0) {
                logger.info(`Probed ${probed}/${unprobed.length} files...`);
              }
            } catch (error) {
              const message = error instanceof Error ? error.message : 'Unknown error';
              logger.warn('Probe failed', { path: file.file_path, error: message });
            }
          }
          logger.success(`Probed ${probed} files`);
        }
      }

      // Match if requested
      if (options.match) {
        if (!config.tmdbApiKey) {
          logger.warn('TMDB_API_KEY not set, skipping match step');
        } else {
          logger.info('Matching against TMDB...');
          const matcher = new TmdbMatcher(config.tmdbApiKey);
          const unmatched = await db.listUnmatched(200);
          let matched = 0;
          for (const file of unmatched) {
            if (!file.parsed_title) continue;
            try {
              const mediaType = file.parsed_season !== null ? 'tv' : 'movie';
              const matches = await matcher.match(file.parsed_title, file.parsed_year, mediaType);
              if (matches.length > 0 && matches[0].confidence >= SUGGEST_THRESHOLD) {
                await db.updateMediaMatch(file.id, matches[0]);
                matched++;
                const label = matches[0].confidence >= AUTO_ACCEPT_THRESHOLD ? 'auto' : 'suggest';
                logger.debug(`Matched [${label}]`, {
                  title: file.parsed_title,
                  match: matches[0].title,
                  confidence: matches[0].confidence,
                });
              }
            } catch (error) {
              const message = error instanceof Error ? error.message : 'Unknown error';
              logger.warn('Match failed', { title: file.parsed_title, error: message });
            }
          }
          logger.success(`Matched ${matched} files`);
        }
      }

      // Update scan record
      await db.updateScanState(scan.id, 'completed', {
        files_found: totalFound,
        files_processed: totalProcessed,
        completed_at: new Date(),
      });

      console.log('\nScan Results:');
      console.log('==============');
      console.log(`Scan ID:         ${scan.id}`);
      console.log(`Files found:     ${totalFound}`);
      console.log(`Files processed: ${totalProcessed}`);
      console.log(`Errors:          ${errors.length}`);

      if (errors.length > 0) {
        console.log('\nErrors:');
        errors.slice(0, 20).forEach(err => console.log(`  - ${err.path}: ${err.error}`));
        if (errors.length > 20) {
          console.log(`  ... and ${errors.length - 20} more`);
        }
      }

      await db.disconnect();
      process.exit(errors.length > 0 ? 1 : 0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Scan failed', { error: message });
      process.exit(1);
    }
  });

// ─── parse ──────────────────────────────────────────────────────────────────

program
  .command('parse')
  .description('Parse a media filename')
  .argument('<filename>', 'Filename to parse')
  .action((_filename: string) => {
    const parsed = parseFilename(_filename);
    console.log(JSON.stringify(parsed, null, 2));
  });

// ─── probe ──────────────────────────────────────────────────────────────────

program
  .command('probe')
  .description('Run FFprobe on a media file')
  .argument('<path>', 'Path to media file')
  .action(async (filePath: string) => {
    try {
      const available = await checkFFprobeAvailable();
      if (!available) {
        logger.error('ffprobe not found. Install ffmpeg to use this command.');
        process.exit(1);
      }

      const info = await probeFile(filePath);
      console.log(JSON.stringify(info, null, 2));
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Probe failed', { error: message });
      process.exit(1);
    }
  });

// ─── match ──────────────────────────────────────────────────────────────────

program
  .command('match')
  .description('Match a title against TMDB')
  .argument('<title>', 'Title to match')
  .option('-y, --year <year>', 'Release year')
  .option('-t, --type <type>', 'Media type: movie or tv', 'movie')
  .action(async (title: string, options) => {
    try {
      const config = loadConfig();
      if (!config.tmdbApiKey) {
        logger.error('TMDB_API_KEY is required for matching');
        process.exit(1);
      }

      const matcher = new TmdbMatcher(config.tmdbApiKey);
      const year = options.year ? parseInt(options.year, 10) : null;
      const type = options.type as 'movie' | 'tv';

      const matches = await matcher.match(title, year, type);

      if (matches.length === 0) {
        console.log('No matches found');
        process.exit(0);
      }

      console.log('\nMatches:');
      console.log('========');
      matches.forEach((m, i) => {
        const confidenceLabel =
          m.confidence >= AUTO_ACCEPT_THRESHOLD ? 'AUTO' :
          m.confidence >= SUGGEST_THRESHOLD ? 'SUGGEST' : 'LOW';
        console.log(`${i + 1}. [${confidenceLabel}] ${m.title} (${m.year ?? 'N/A'}) - confidence: ${(m.confidence * 100).toFixed(1)}% [tmdb:${m.id}]`);
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Match failed', { error: message });
      process.exit(1);
    }
  });

// ─── search ─────────────────────────────────────────────────────────────────

program
  .command('search')
  .description('Search indexed media')
  .argument('<query>', 'Search query')
  .option('-t, --type <type>', 'Filter by type: movie or tv')
  .option('-l, --limit <limit>', 'Number of results', '20')
  .action(async (query: string, options) => {
    try {
      const config = loadConfig();
      const searchService = new MediaSearchService(config.meilisearchUrl, config.meilisearchKey);

      const results = await searchService.search({
        q: query,
        type: options.type,
        limit: parseInt(options.limit, 10),
      });

      if (results.length === 0) {
        console.log('No results found');
        process.exit(0);
      }

      console.log('\nSearch Results:');
      console.log('===============');
      results.forEach((r, i) => {
        const year = r.year ? ` (${r.year})` : '';
        const rating = r.rating ? ` [${r.rating}/10]` : '';
        console.log(`${i + 1}. ${r.title}${year} - ${r.type}${rating}`);
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      process.exit(1);
    }
  });

// ─── stats ──────────────────────────────────────────────────────────────────

program
  .command('stats')
  .description('Show library statistics')
  .action(async () => {
    try {
      const db = new MediaScannerDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nMedia Library Statistics');
      console.log('========================');
      console.log(`Total items:    ${stats.total_items}`);
      console.log(`Movies:         ${stats.movies}`);
      console.log(`TV episodes:    ${stats.tv_shows}`);
      console.log(`Total size:     ${stats.total_size_gb} GB`);
      console.log(`Last scan:      ${stats.last_scan?.toISOString() ?? 'Never'}`);
      console.log(`Indexed:        ${stats.indexed_count}`);
      console.log(`Matched:        ${stats.matched_count}`);
      console.log(`Unmatched:      ${stats.unmatched_count}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

// ─── init ───────────────────────────────────────────────────────────────────

program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      const db = new MediaScannerDatabase();
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

// ─── server ─────────────────────────────────────────────────────────────────

program
  .command('server')
  .description('Start the HTTP server')
  .option('-p, --port <port>', 'Server port', '3021')
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

// ─── index ──────────────────────────────────────────────────────────────────

program
  .command('index')
  .description('Index matched media files into MeiliSearch')
  .option('-l, --limit <limit>', 'Number of files to index', '100')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new MediaScannerDatabase();
      await db.connect();
      await db.initializeSchema();

      const searchService = new MediaSearchService(config.meilisearchUrl, config.meilisearchKey);
      await searchService.initialize();

      const unindexed = await db.listUnindexed(parseInt(options.limit, 10));

      if (unindexed.length === 0) {
        console.log('No unindexed files found');
        await db.disconnect();
        process.exit(0);
      }

      logger.info(`Indexing ${unindexed.length} files...`);
      let indexed = 0;

      for (const file of unindexed) {
        try {
          const mediaType = file.parsed_season !== null ? 'tv' : 'movie';
          await searchService.indexItem({
            id: file.id,
            title: file.match_title ?? file.parsed_title ?? file.filename,
            type: mediaType as 'movie' | 'tv',
            year: file.parsed_year ?? undefined,
            file_path: file.file_path,
            duration_seconds: file.duration_seconds ?? undefined,
            resolution: file.video_resolution ?? undefined,
            codec: file.video_codec ?? undefined,
          });
          await db.setMediaIndexed(file.id, true);
          indexed++;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.warn('Index failed for file', { id: file.id, error: message });
        }
      }

      logger.success(`Indexed ${indexed} files`);
      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Index failed', { error: message });
      process.exit(1);
    }
  });

// ─── files ──────────────────────────────────────────────────────────────────

program
  .command('files')
  .description('List media files')
  .argument('[action]', 'Action: list, show, unmatched, unprobed', 'list')
  .argument('[id]', 'File ID (for show)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action: string, id: string | undefined, options) => {
    try {
      const db = new MediaScannerDatabase();
      await db.connect();

      const limit = parseInt(options.limit, 10);

      switch (action) {
        case 'list': {
          const files = await db.listMediaFiles(limit);
          console.log('\nMedia Files:');
          console.log('-'.repeat(100));
          files.forEach(f => {
            const match = f.match_title ? `[matched: ${f.match_title}]` : '[unmatched]';
            const size = (f.file_size / (1024 * 1024)).toFixed(0);
            console.log(`  ${f.id.substring(0, 8)} | ${f.filename} | ${size} MB | ${match}`);
          });
          console.log(`\nTotal: ${await db.countMediaFiles()}`);
          break;
        }
        case 'show': {
          if (!id) {
            logger.error('File ID required');
            process.exit(1);
          }
          const file = await db.getMediaFile(id);
          if (!file) {
            logger.error('File not found');
            process.exit(1);
          }
          console.log(JSON.stringify(file, null, 2));
          break;
        }
        case 'unmatched': {
          const files = await db.listUnmatched(limit);
          console.log('\nUnmatched Files:');
          console.log('-'.repeat(80));
          files.forEach(f => {
            console.log(`  ${f.id.substring(0, 8)} | ${f.parsed_title ?? f.filename} | ${f.parsed_year ?? 'N/A'}`);
          });
          console.log(`\nShowing: ${files.length}`);
          break;
        }
        case 'unprobed': {
          const files = await db.listUnprobed(limit);
          console.log('\nUnprobed Files:');
          console.log('-'.repeat(80));
          files.forEach(f => {
            console.log(`  ${f.id.substring(0, 8)} | ${f.filename}`);
          });
          console.log(`\nShowing: ${files.length}`);
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

program.parse();
