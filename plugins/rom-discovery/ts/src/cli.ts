#!/usr/bin/env node
/**
 * ROM Discovery Plugin CLI
 * Command-line interface for ROM metadata search, scraper management, and downloads
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { RomDiscoveryDatabase } from './database.js';
import { createServer } from './server.js';
import { ScraperScheduler } from './scrapers/scraper-scheduler.js';

const logger = createLogger('rom-discovery:cli');

const program = new Command();

program
  .name('nself-rom-discovery')
  .description('ROM Discovery plugin for nself - ROM metadata search, discovery, and scraping')
  .version('1.0.0');

// Server command
program
  .command('server')
  .description('Start the ROM Discovery API server')
  .option('-p, --port <port>', 'Server port', '3034')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .option('--enable-scrapers', 'Enable automated scrapers')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
        enableScrapers: options.enableScrapers ?? false,
      });

      logger.info(`Starting ROM Discovery server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Search command
program
  .command('search <query>')
  .description('Search ROM database')
  .option('--platform <platform>', 'Filter by platform (nes, snes, gba, genesis, n64, etc.)')
  .option('--region <region>', 'Filter by region (USA, Europe, Japan)')
  .option('--quality-min <min>', 'Minimum quality score', '0')
  .option('--verified', 'Only verified dumps')
  .option('--homebrew', 'Only homebrew ROMs')
  .option('--show-hacks', 'Include ROM hacks')
  .option('--sort <field>', 'Sort by: popularity, quality, title, year', 'popularity')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (query: string, options) => {
    try {
      const db = new RomDiscoveryDatabase();
      await db.connect();

      const { roms, total } = await db.searchRoms({
        query,
        platform: options.platform,
        region: options.region,
        quality_min: parseInt(options.qualityMin, 10),
        verified_only: options.verified ?? false,
        homebrew_only: options.homebrew ?? false,
        show_hacks: options.showHacks ?? false,
        show_translations: false,
        sort: options.sort,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (roms.length === 0) {
        console.log(`No ROMs found for "${query}"`);
        process.exit(0);
      }

      console.log(`\nFound ${total} ROM(s) for "${query}" (showing ${roms.length}):\n`);
      for (const rom of roms) {
        const verified = rom.is_verified_dump ? ' [VERIFIED]' : '';
        const homebrew = rom.is_homebrew ? ' [HOMEBREW]' : '';
        const hack = rom.is_hack ? ' [HACK]' : '';
        const region = rom.region ? ` (${rom.region})` : '';
        const quality = `Q:${rom.quality_score}`;
        const popularity = `P:${rom.popularity_score}`;
        const size = rom.file_size_bytes
          ? formatBytes(rom.file_size_bytes)
          : 'Size unknown';

        console.log(`  ${rom.rom_title}${region}${verified}${homebrew}${hack}`);
        console.log(`    ID:       ${rom.id}`);
        console.log(`    Platform: ${rom.platform}`);
        console.log(`    Size:     ${size}`);
        console.log(`    Scores:   ${quality} ${popularity}`);
        if (rom.release_group) console.log(`    Source:   ${rom.release_group}`);
        if (rom.download_url) console.log(`    URL:      ${rom.download_url}`);
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

// Platforms command
program
  .command('platforms')
  .description('List platforms with ROM counts')
  .action(async () => {
    try {
      const db = new RomDiscoveryDatabase();
      await db.connect();
      const platforms = await db.getPlatformStats();
      await db.disconnect();

      if (platforms.length === 0) {
        console.log('No platforms found. Run scrapers to populate the database.');
        process.exit(0);
      }

      console.log('\nPlatform Statistics:\n');
      console.log('  Platform                          ROMs    Verified   Homebrew   Avg Quality');
      console.log('  ' + '-'.repeat(80));

      for (const p of platforms) {
        const name = p.platform.padEnd(32);
        const roms = p.rom_count.toString().padStart(6);
        const verified = p.verified_count.toString().padStart(10);
        const homebrew = p.homebrew_count.toString().padStart(10);
        const quality = p.avg_quality.toFixed(0).padStart(13);

        console.log(`  ${name}${roms}${verified}${homebrew}${quality}`);
      }

      const totalRoms = platforms.reduce((sum, p) => sum + p.rom_count, 0);
      console.log('  ' + '-'.repeat(80));
      console.log(`  Total: ${totalRoms} ROMs across ${platforms.length} platforms\n`);

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list platforms', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Scrapers command group
const scrapersCmd = program
  .command('scrapers')
  .description('Manage ROM scrapers');

scrapersCmd
  .command('list')
  .description('List all scrapers and their status')
  .action(async () => {
    try {
      const db = new RomDiscoveryDatabase();
      await db.connect();
      const scrapers = await db.getScrapers();
      await db.disconnect();

      if (scrapers.length === 0) {
        console.log('No scrapers configured.');
        process.exit(0);
      }

      console.log('\nScraper Status:\n');
      for (const s of scrapers) {
        const enabled = s.enabled ? 'ENABLED' : 'DISABLED';
        const status = s.last_run_status ?? 'never run';
        const lastRun = s.last_run_at
          ? new Date(s.last_run_at).toISOString()
          : 'never';

        console.log(`  ${s.scraper_name} [${enabled}]`);
        console.log(`    Type:      ${s.scraper_type}`);
        console.log(`    Schedule:  ${s.cron_schedule}`);
        console.log(`    Status:    ${status}`);
        console.log(`    Last Run:  ${lastRun}`);
        if (s.last_run_duration_seconds) {
          console.log(`    Duration:  ${s.last_run_duration_seconds}s`);
        }
        console.log(`    Found:     ${s.roms_found} | Added: ${s.roms_added} | Updated: ${s.roms_updated}`);
        if (s.errors.length > 0) {
          console.log(`    Errors:    ${s.errors.length}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list scrapers', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

scrapersCmd
  .command('run <name>')
  .description('Manually trigger a scraper')
  .action(async (name: string) => {
    try {
      const db = new RomDiscoveryDatabase();
      await db.connect();

      const scraper = await db.getScraperByName(name);
      if (!scraper) {
        console.error(`Scraper "${name}" not found`);
        await db.disconnect();
        process.exit(1);
      }

      console.log(`Running scraper: ${name}...`);
      const scheduler = new ScraperScheduler(db, 'primary');
      const result = await scheduler.runScraper(name);

      await db.disconnect();

      console.log('\nScraper Results:');
      console.log(`  Found:     ${result.roms_found}`);
      console.log(`  Added:     ${result.roms_added}`);
      console.log(`  Updated:   ${result.roms_updated}`);
      console.log(`  Removed:   ${result.roms_removed}`);
      console.log(`  Errors:    ${result.errors.length}`);
      console.log(`  Duration:  ${result.duration_seconds}s`);

      if (result.errors.length > 0) {
        console.log('\nErrors:');
        for (const err of result.errors.slice(0, 10)) {
          console.log(`  - ${err}`);
        }
        if (result.errors.length > 10) {
          console.log(`  ... and ${result.errors.length - 10} more`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Scraper run failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Downloads command
program
  .command('downloads')
  .alias('queue')
  .description('List download queue')
  .option('--status <status>', 'Filter by status (pending, downloading, completed, failed)')
  .action(async () => {
    try {
      const db = new RomDiscoveryDatabase();
      await db.connect();
      const queue = await db.getDownloadQueue();
      await db.disconnect();

      if (queue.length === 0) {
        console.log('Download queue is empty.');
        process.exit(0);
      }

      console.log('\nDownload Queue:\n');
      for (const d of queue) {
        const progress = d.status === 'downloading'
          ? ` ${d.download_progress_percent}%`
          : '';
        const size = d.total_bytes > 0
          ? ` (${formatBytes(d.downloaded_bytes)}/${formatBytes(d.total_bytes)})`
          : '';
        const romTitle = (d as Record<string, unknown>).rom_title ?? d.rom_metadata_id;

        console.log(`  [${d.status.toUpperCase()}${progress}] ${romTitle}${size}`);
        console.log(`    ID:        ${d.id}`);
        console.log(`    ROM ID:    ${d.rom_metadata_id}`);
        if (d.error_message) console.log(`    Error:     ${d.error_message}`);
        if (d.checksum_verified) console.log(`    Checksum:  verified`);
        console.log(`    Retries:   ${d.retry_count}/${d.max_retries}`);
        console.log(`    Created:   ${new Date(d.created_at).toISOString()}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list downloads', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .alias('status')
  .description('Show ROM Discovery statistics')
  .action(async () => {
    try {
      const db = new RomDiscoveryDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nROM Discovery Statistics:');
      console.log('========================');
      console.log(`Total ROMs:              ${stats.total_roms}`);
      console.log(`Total Platforms:         ${stats.total_platforms}`);
      console.log(`Verified Dumps:          ${stats.total_verified}`);
      console.log(`Homebrew ROMs:           ${stats.total_homebrew}`);
      console.log(`Community ROMs:          ${stats.total_community}`);
      console.log(`Downloads Queued:        ${stats.total_downloads_queued}`);
      console.log(`Downloads Completed:     ${stats.total_downloads_completed}`);
      console.log(`Active Scrapers:         ${stats.active_scrapers}`);
      console.log(`Avg Quality Score:       ${stats.avg_quality_score}`);
      console.log(`Avg Popularity Score:    ${stats.avg_popularity_score}`);

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Init command (initialize database schema)
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new RomDiscoveryDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for ROM Discovery plugin');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

/**
 * Format bytes into human-readable string
 */
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${value.toFixed(1)} ${units[i]}`;
}

program.parse();
