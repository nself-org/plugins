#!/usr/bin/env node
import { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { SubtitleManagerDatabase } from './database.js';
import { OpenSubtitlesClient } from './opensubtitles-client.js';

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
    } catch (error: any) {
      spinner.fail('Initialization failed');
      console.error(chalk.red(error.message));
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
      results.slice(0, 10).forEach((sub: any, i: number) => {
        console.log(`${i + 1}. ${sub.attributes?.feature_details?.title || 'Unknown'}`);
        console.log(`   Language: ${sub.attributes?.language}`);
        console.log(`   Format: ${sub.attributes?.format}`);
        console.log('');
      });
    } catch (error: any) {
      spinner.fail('Search failed');
      console.error(chalk.red(error.message));
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
    } catch (error: any) {
      console.error(chalk.red('Failed to start server:'), error.message);
      process.exit(1);
    }
  });

program.parse();
