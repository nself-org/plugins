#!/usr/bin/env node
import { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { SubtitleManagerDatabase } from './database.js';
import { OpenSubtitlesClient } from './opensubtitles-client.js';
import { SubtitleSynchronizer } from './sync.js';
import { SubtitleQC } from './qc.js';
import { SubtitleNormalizer } from './normalize.js';

const logger = createLogger('subtitle-manager:cli');
const program = new Command();

program
  .name('nself-subtitle-manager')
  .description('Subtitle search and management')
  .version('1.0.0');

program
  .command('init')
  .description('Initialize database')
  .action(async () => {
    const spinner = ora('Initializing subtitle manager').start();
    try {
      const database = new SubtitleManagerDatabase(config.database_url);
      await database.initialize();
      spinner.succeed('Database initialized');
      await database.close();
    } catch (error) {
      spinner.fail('Initialization failed');
      console.error(chalk.red(error instanceof Error ? error.message : String(error)));
      process.exit(1);
    }
  });

program
  .command('search <query>')
  .description('Search for subtitles')
  .option('-l, --language <lang>', 'Language code (en, es, fr, etc.)', 'en')
  .action(async (query, options) => {
    const spinner = ora(`Searching for subtitles: ${query}`).start();
    try {
      const client = new OpenSubtitlesClient(config.opensubtitles_api_key);
      const results = await client.searchByQuery(query, [options.language]);
      spinner.stop();

      if (results.length === 0) {
        console.log(chalk.yellow('No subtitles found'));
        return;
      }

      console.log(chalk.bold(`\nFound ${results.length} subtitles:\n`));
      results.slice(0, 10).forEach((sub, i: number) => {
        console.log(`${i + 1}. ${sub.attributes?.feature_details?.title || 'Unknown'}`);
        console.log(`   Language: ${sub.attributes?.language}`);
        console.log(`   Format: ${sub.attributes?.format}`);
        console.log('');
      });
    } catch (error) {
      spinner.fail('Search failed');
      console.error(chalk.red(error instanceof Error ? error.message : String(error)));
      process.exit(1);
    }
  });

program
  .command('server')
  .description('Start HTTP API server')
  .action(async () => {
    console.log(chalk.bold('Starting Subtitle Manager Server...\n'));
    try {
      const { SubtitleManagerServer } = await import('./server.js');
      const database = new SubtitleManagerDatabase(config.database_url);
      await database.initialize();

      const server = new SubtitleManagerServer(config, database);
      await server.initialize();
      await server.start();

      console.log(chalk.green(`✓ Server running on port ${config.port}`));

      process.on('SIGINT', async () => {
        console.log(chalk.yellow('\nShutting down...'));
        await server.stop();
        await database.close();
        process.exit(0);
      });
    } catch (error) {
      console.error(chalk.red('Failed to start server:'), error instanceof Error ? error.message : String(error));
      process.exit(1);
    }
  });

program
  .command('sync <video> <subtitle>')
  .description('Synchronize subtitle timing with video using alass and/or ffsubsync')
  .option('-o, --output <path>', 'Output path for synced subtitle')
  .option('--alass-only', 'Use only alass (skip ffsubsync)')
  .option('--ffsubsync-only', 'Use only ffsubsync (skip alass)')
  .action(async (video, subtitle, options) => {
    const spinner = ora('Synchronizing subtitle with video').start();
    try {
      const synchronizer = new SubtitleSynchronizer(config);

      const outputPath = options.output || subtitle.replace(/(\.\w+)$/, '.synced$1');

      const result = await synchronizer.syncSubtitle(video, subtitle, outputPath, {
        alassOnly: options.alassOnly,
        ffsubsyncOnly: options.ffsubsyncOnly,
      });

      spinner.succeed('Subtitle synchronized');
      console.log(chalk.bold('\nSync Results:'));
      console.log(`  Method:     ${result.method}`);
      console.log(`  Confidence: ${(result.confidence * 100).toFixed(1)}%`);
      console.log(`  Offset:     ${result.offsetMs}ms`);
      console.log(`  Output:     ${result.syncedPath}`);

      if (result.alassResult) {
        console.log(chalk.bold('\n  alass:'));
        console.log(`    Confidence:  ${(result.alassResult.confidence * 100).toFixed(1)}%`);
        console.log(`    Offset:      ${result.alassResult.offsetMs}ms`);
        console.log(`    Framerate:   ${result.alassResult.framerateAdjusted ? 'adjusted' : 'no change'}`);
      }

      if (result.ffsubsyncResult) {
        console.log(chalk.bold('\n  ffsubsync:'));
        console.log(`    Confidence:  ${(result.ffsubsyncResult.confidence * 100).toFixed(1)}%`);
        console.log(`    Offset:      ${result.ffsubsyncResult.offsetMs}ms`);
      }
    } catch (error) {
      spinner.fail('Sync failed');
      console.error(chalk.red(error instanceof Error ? error.message : String(error)));
      process.exit(1);
    }
  });

program
  .command('qc <subtitle>')
  .description('Run QC validation checks on a subtitle file')
  .option('-d, --duration <ms>', 'Video duration in milliseconds', parseInt)
  .action(async (subtitle, options) => {
    const spinner = ora('Running QC validation').start();
    try {
      const qc = new SubtitleQC();
      const result = await qc.validateSubtitle(subtitle, options.duration);
      spinner.stop();

      const statusColor = result.status === 'pass'
        ? chalk.green
        : result.status === 'warn'
          ? chalk.yellow
          : chalk.red;

      console.log(chalk.bold(`\nQC Result: ${statusColor(result.status.toUpperCase())}`));
      console.log(`  Cues:     ${result.cueCount}`);
      console.log(`  Duration: ${(result.totalDurationMs / 1000).toFixed(1)}s`);

      console.log(chalk.bold('\n  Checks:'));
      for (const check of result.checks) {
        const icon = check.passed ? chalk.green('PASS') : chalk.red('FAIL');
        console.log(`    [${icon}] ${check.name}: ${check.message}`);
      }

      if (result.issues.length > 0) {
        console.log(chalk.bold('\n  Issues:'));
        for (const issue of result.issues.slice(0, 20)) {
          const icon = issue.severity === 'error' ? chalk.red('ERR') : chalk.yellow('WRN');
          console.log(`    [${icon}] ${issue.message}`);
        }
        if (result.issues.length > 20) {
          console.log(chalk.gray(`    ... and ${result.issues.length - 20} more`));
        }
      }
    } catch (error) {
      spinner.fail('QC validation failed');
      console.error(chalk.red(error instanceof Error ? error.message : String(error)));
      process.exit(1);
    }
  });

program
  .command('normalize <subtitle>')
  .description('Convert subtitle to WebVTT format')
  .option('-o, --output <path>', 'Output path for normalized WebVTT file')
  .action(async (subtitle, options) => {
    const spinner = ora('Normalizing subtitle to WebVTT').start();
    try {
      const normalizer = new SubtitleNormalizer();
      const outputPath = await normalizer.normalizeToWebVTT(subtitle, options.output);
      spinner.succeed('Subtitle normalized to WebVTT');
      console.log(chalk.bold(`\nOutput: ${outputPath}`));
    } catch (error) {
      spinner.fail('Normalization failed');
      console.error(chalk.red(error instanceof Error ? error.message : String(error)));
      process.exit(1);
    }
  });

program.parse();
