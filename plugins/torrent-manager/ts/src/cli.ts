#!/usr/bin/env node
/**
 * Torrent Manager CLI
 * Complete command-line interface for torrent operations
 */

import { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { TorrentDatabase } from './database.js';
import { TransmissionClient } from './clients/transmission.js';
import { VPNChecker } from './vpn-checker.js';
import { TorrentSearchAggregator } from './search/aggregator.js';
import { SmartMatcher } from './matching/smart-matcher.js';

const logger = createLogger('torrent-manager:cli');
const program = new Command();

program
  .name('nself-torrent-manager')
  .description('Torrent downloading with VPN enforcement')
  .version('1.0.0');

// ============================================================================
// Init Command
// ============================================================================

program
  .command('init')
  .description('Initialize database and register torrent clients')
  .action(async () => {
    const spinner = ora('Initializing torrent manager').start();

    try {
      const database = new TorrentDatabase(config.database_url);
      await database.initialize();
      spinner.succeed('Database initialized');

      // Register default client
      const client = new TransmissionClient(
        config.transmission_host,
        config.transmission_port,
        config.transmission_username,
        config.transmission_password
      );

      const connected = await client.connect();
      if (connected) {
        await database.upsertClient({
          source_account_id: 'primary',
          client_type: 'transmission',
          host: config.transmission_host,
          port: config.transmission_port,
          username: config.transmission_username,
          is_default: true,
          status: 'connected',
        });
        spinner.succeed('Transmission client registered');
      } else {
        spinner.warn('Could not connect to Transmission');
      }

      await database.close();
    } catch (error: unknown) {
      spinner.fail('Initialization failed');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

// ============================================================================
// Add Command
// ============================================================================

program
  .command('add <magnetUri>')
  .description('Add torrent download')
  .option('-c, --category <category>', 'Category (movie, tv, music, other)', 'other')
  .option('-p, --path <path>', 'Download path')
  .action(async (magnetUri, options) => {
    const spinner = ora('Adding torrent').start();

    try {
      // Check VPN
      if (config.vpn_required) {
        spinner.text = 'Checking VPN status';
        const vpnChecker = new VPNChecker(config.vpn_manager_url);
        const vpnActive = await vpnChecker.isVPNActive();

        if (!vpnActive) {
          spinner.fail('VPN is not active');
          console.error(chalk.red('VPN must be active before starting downloads'));
          console.log(chalk.yellow('Start VPN with: nself-vpn-manager connect'));
          process.exit(1);
        }
        spinner.succeed('VPN is active');
      }

      // Connect to client
      spinner.text = 'Connecting to torrent client';
      const client = new TransmissionClient(
        config.transmission_host,
        config.transmission_port,
        config.transmission_username,
        config.transmission_password
      );

      const connected = await client.connect();
      if (!connected) {
        spinner.fail('Failed to connect to torrent client');
        process.exit(1);
      }

      // Add torrent
      spinner.text = 'Adding torrent';
      const download = await client.addTorrent(magnetUri, {
        category: options.category,
        download_path: options.path || config.download_path,
      });

      // Save to database
      const database = new TorrentDatabase(config.database_url);
      const defaultClient = await database.getDefaultClient();

      if (defaultClient) {
        download.client_id = defaultClient.id;
        download.requested_by = 'cli';
        await database.createDownload(download);
      }

      await database.close();

      spinner.succeed('Torrent added successfully');
      console.log(chalk.green(`Name: ${download.name}`));
      console.log(chalk.green(`Hash: ${download.info_hash}`));
      console.log(chalk.green(`Size: ${(download.size_bytes / 1024 / 1024).toFixed(2)} MB`));
    } catch (error: unknown) {
      spinner.fail('Failed to add torrent');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

// ============================================================================
// List Command
// ============================================================================

program
  .command('list')
  .description('List all downloads')
  .option('-s, --status <status>', 'Filter by status')
  .option('-c, --category <category>', 'Filter by category')
  .option('-l, --limit <number>', 'Limit results', '20')
  .action(async (options) => {
    const spinner = ora('Loading downloads').start();

    try {
      const database = new TorrentDatabase(config.database_url);
      const downloads = await database.listDownloads({
        status: options.status,
        category: options.category,
        limit: parseInt(options.limit, 10),
      });

      await database.close();
      spinner.stop();

      if (downloads.length === 0) {
        console.log(chalk.yellow('No downloads found'));
        return;
      }

      console.log(chalk.bold(`\nFound ${downloads.length} downloads:\n`));

      downloads.forEach((download, index) => {
        const statusColor =
          download.status === 'completed'
            ? chalk.green
            : download.status === 'failed'
            ? chalk.red
            : chalk.yellow;

        console.log(`${index + 1}. ${chalk.bold(download.name)}`);
        console.log(`   Status: ${statusColor(download.status)}`);
        console.log(`   Progress: ${download.progress_percent.toFixed(1)}%`);
        console.log(`   Size: ${(download.size_bytes / 1024 / 1024).toFixed(2)} MB`);
        console.log(`   Ratio: ${download.ratio.toFixed(2)}`);
        console.log(`   ID: ${chalk.gray(download.id)}`);
        console.log('');
      });
    } catch (error: unknown) {
      spinner.fail('Failed to list downloads');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

// ============================================================================
// Stats Command
// ============================================================================

program
  .command('stats')
  .description('Show statistics')
  .action(async () => {
    const spinner = ora('Loading statistics').start();

    try {
      const database = new TorrentDatabase(config.database_url);
      const stats = await database.getStats();
      await database.close();

      spinner.stop();

      console.log(chalk.bold('\nTorrent Manager Statistics:\n'));
      console.log(`Total Downloads: ${chalk.green(stats.total_downloads)}`);
      console.log(`Active: ${chalk.yellow(stats.active_downloads)}`);
      console.log(`Completed: ${chalk.green(stats.completed_downloads)}`);
      console.log(`Failed: ${chalk.red(stats.failed_downloads)}`);
      console.log(`Seeding: ${chalk.blue(stats.seeding_torrents)}`);
      console.log(
        `Downloaded: ${chalk.green((stats.total_downloaded_bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB')}`
      );
      console.log(
        `Uploaded: ${chalk.green((stats.total_uploaded_bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB')}`
      );
      console.log(`Overall Ratio: ${chalk.green(stats.overall_ratio.toFixed(2))}`);
    } catch (error: unknown) {
      spinner.fail('Failed to get statistics');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

// ============================================================================
// Server Command
// ============================================================================

program
  .command('server')
  .description('Start HTTP API server')
  .action(async () => {
    console.log(chalk.bold('Starting Torrent Manager Server...\n'));

    try {
      const { TorrentManagerServer } = await import('./server.js');
      const database = new TorrentDatabase(config.database_url);
      await database.initialize();

      const server = new TorrentManagerServer(config, database);
      await server.initialize();
      await server.start();

      console.log(chalk.green(`✓ Server running on port ${config.port}`));
      console.log(chalk.gray(`  Health check: http://localhost:${config.port}/health`));
      console.log(chalk.gray(`  API docs: http://localhost:${config.port}/v1`));

      // Handle graceful shutdown
      process.on('SIGINT', async () => {
        console.log(chalk.yellow('\nShutting down...'));
        await server.stop();
        await database.close();
        process.exit(0);
      });
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red('Failed to start server:'), message);
      process.exit(1);
    }
  });

// ============================================================================
// Search Command
// ============================================================================

program
  .command('search <query>')
  .description('Search for torrents')
  .option('-t, --type <type>', 'Content type: movie, tv', 'movie')
  .option('-q, --quality <quality>', 'Quality filter: 1080p, 720p, etc.')
  .option('-s, --min-seeders <number>', 'Minimum seeders', '1')
  .option('-l, --limit <number>', 'Max results', '20')
  .action(async (query, options) => {
    const spinner = ora('Searching torrents').start();

    try {
      const aggregator = new TorrentSearchAggregator(
        config.enabled_sources?.split(',')
      );

      spinner.text = `Searching for: ${query}`;

      const results = await aggregator.search({
        query,
        type: options.type,
        quality: options.quality,
        minSeeders: parseInt(options.minSeeders),
        maxResults: parseInt(options.limit),
      });

      spinner.stop();

      if (results.length === 0) {
        console.log(chalk.yellow('No results found'));
        return;
      }

      console.log(chalk.bold(`\nFound ${results.length} results:\n`));

      results.slice(0, parseInt(options.limit)).forEach((result, i) => {
        const statusColor = result.seeders >= 10 ? chalk.green : chalk.yellow;

        console.log(`${i + 1}. ${chalk.bold(result.title)}`);
        console.log(
          `   Source: ${chalk.cyan(result.source)} | Seeds: ${statusColor(result.seeders)} | Size: ${result.size}`
        );
        console.log(
          `   Quality: ${result.parsedInfo.quality || 'unknown'} | Type: ${result.parsedInfo.source || 'unknown'}`
        );
        console.log('');
      });
    } catch (error: unknown) {
      spinner.fail('Search failed');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

// ============================================================================
// Best Match Command
// ============================================================================

program
  .command('best-match <title>')
  .description('Find best matching torrent')
  .option('-y, --year <year>', 'Year (for movies)')
  .option('-s, --season <number>', 'Season number (for TV)')
  .option('-e, --episode <number>', 'Episode number (for TV)')
  .option('-q, --quality <quality>', 'Preferred quality')
  .option('--download', 'Download immediately if found')
  .action(async (title, options) => {
    const spinner = ora('Finding best match').start();

    try {
      const aggregator = new TorrentSearchAggregator(
        config.enabled_sources?.split(',')
      );
      const matcher = new SmartMatcher();

      // Build search query
      let searchQuery = title;
      if (options.season && options.episode) {
        searchQuery += ` S${String(options.season).padStart(2, '0')}E${String(options.episode).padStart(2, '0')}`;
      } else if (options.year) {
        searchQuery += ` ${options.year}`;
      }

      spinner.text = `Searching for: ${searchQuery}`;

      // Search
      const searchResults = await aggregator.search({
        query: searchQuery,
        type: options.season ? 'tv' : 'movie',
        quality: options.quality,
        minSeeders: 1,
        maxResults: 50,
      });

      if (searchResults.length === 0) {
        spinner.fail('No results found');
        process.exit(1);
      }

      spinner.text = `Found ${searchResults.length} results, finding best match...`;

      // Find best match
      const bestMatch = matcher.findBestMatch(searchResults, {
        title,
        year: options.year ? parseInt(options.year) : undefined,
        season: options.season ? parseInt(options.season) : undefined,
        episode: options.episode ? parseInt(options.episode) : undefined,
        preferredQualities: options.quality ? [options.quality] : ['1080p', '720p'],
        minSeeders: 1,
      });

      if (!bestMatch) {
        spinner.fail('No suitable match found');
        process.exit(1);
      }

      spinner.succeed('Best match found!\n');

      // Display result
      console.log(chalk.bold.green('✅ Best Match:\n'));
      console.log(`Title: ${chalk.bold(bestMatch.title)}`);
      console.log(`Source: ${chalk.cyan(bestMatch.source)}`);
      console.log(`Quality: ${bestMatch.parsedInfo.quality}`);
      console.log(`Type: ${bestMatch.parsedInfo.source}`);
      console.log(`Size: ${bestMatch.size}`);
      console.log(`Seeders: ${chalk.green(bestMatch.seeders)}`);
      console.log(`Score: ${chalk.yellow(bestMatch.score?.toFixed(2))}/100`);
      console.log('');
      console.log(chalk.bold('Score Breakdown:'));
      console.log(`  Quality: ${bestMatch.scoreBreakdown?.qualityScore}/30`);
      console.log(`  Source: ${bestMatch.scoreBreakdown?.sourceScore}/25`);
      console.log(`  Seeders: ${bestMatch.scoreBreakdown?.seederScore?.toFixed(1)}/20`);
      console.log(`  Size: ${bestMatch.scoreBreakdown?.sizeScore?.toFixed(1)}/15`);
      console.log(`  Group: ${bestMatch.scoreBreakdown?.releaseGroupScore}/10`);
      console.log('');

      // Download if requested
      if (options.download) {
        const downloadSpinner = ora('Fetching magnet link...').start();

        try {
          // Fetch magnet if needed
          let magnetUri = bestMatch.magnetUri;
          if (!magnetUri || !magnetUri.startsWith('magnet:')) {
            magnetUri = await aggregator.getMagnetLink(bestMatch);
          }

          downloadSpinner.text = 'Checking VPN status...';

          // Check VPN
          if (config.vpn_required) {
            const vpnChecker = new VPNChecker(config.vpn_manager_url);
            const vpnActive = await vpnChecker.isVPNActive();

            if (!vpnActive) {
              downloadSpinner.fail('VPN is not active');
              console.error(chalk.red('VPN must be active before starting downloads'));
              console.log(chalk.yellow('Start VPN with: nself-vpn-manager connect'));
              process.exit(1);
            }
          }

          downloadSpinner.text = 'Adding to downloads...';

          // Connect to client
          const client = new TransmissionClient(
            config.transmission_host,
            config.transmission_port,
            config.transmission_username,
            config.transmission_password
          );

          const connected = await client.connect();
          if (!connected) {
            downloadSpinner.fail('Failed to connect to torrent client');
            process.exit(1);
          }

          // Add torrent
          const download = await client.addTorrent(magnetUri, {
            category: options.season ? 'tv' : 'movie',
            download_path: config.download_path,
          });

          // Save to database
          const database = new TorrentDatabase(config.database_url);
          const defaultClient = await database.getDefaultClient();

          if (defaultClient) {
            download.client_id = defaultClient.id;
            download.requested_by = 'cli';
            await database.createDownload(download);
          }

          await database.close();

          downloadSpinner.succeed(`Download started: ${download.id}`);
        } catch (error: unknown) {
          downloadSpinner.fail('Download failed');
          const message = error instanceof Error ? error.message : String(error);
          console.error(chalk.red(message));
          process.exit(1);
        }
      }
    } catch (error: unknown) {
      spinner.fail('Best match search failed');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

// Parse command line
program.parse();
