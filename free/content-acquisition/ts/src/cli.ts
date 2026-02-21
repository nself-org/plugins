#!/usr/bin/env node
/**
 * Content Acquisition CLI
 */

import { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { ContentAcquisitionDatabase } from './database.js';

const logger = createLogger('content-acquisition:cli');
const program = new Command();

program
  .name('nself-content-acquisition')
  .description('Automated content acquisition')
  .version('1.0.0');

program
  .command('init')
  .description('Initialize database')
  .action(async () => {
    const spinner = ora('Initializing content acquisition').start();
    try {
      const database = new ContentAcquisitionDatabase(config.database_url);
      await database.initialize();
      spinner.succeed('Database initialized');
      await database.close();
    } catch (error) {
      spinner.fail('Initialization failed');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

program
  .command('subscribe <name>')
  .description('Subscribe to content')
  .option('-t, --type <type>', 'Content type: tv_show, movie_collection', 'tv_show')
  .option('-i, --id <id>', 'Content ID (TMDB/TVDB)')
  .action(async (name, options) => {
    const spinner = ora(`Subscribing to ${name}`).start();
    try {
      const database = new ContentAcquisitionDatabase(config.database_url);
      const sub = await database.createSubscription({
        source_account_id: 'primary',
        subscription_type: options.type,
        content_id: options.id,
        content_name: name,
      });
      await database.close();
      spinner.succeed(`Subscribed to ${name}`);
      console.log(chalk.green(`Subscription ID: ${sub.id}`));
    } catch (error) {
      spinner.fail('Subscription failed');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

program
  .command('queue')
  .description('View acquisition queue')
  .action(async () => {
    const spinner = ora('Loading queue').start();
    try {
      const database = new ContentAcquisitionDatabase(config.database_url);
      const queue = await database.getQueue('primary');
      await database.close();
      spinner.stop();

      if (queue.length === 0) {
        console.log(chalk.yellow('Queue is empty'));
        return;
      }

      console.log(chalk.bold(`\n${queue.length} items in queue:\n`));
      queue.forEach((item, i) => {
        console.log(`${i + 1}. ${item.content_name}`);
        console.log(`   Status: ${item.status}`);
        console.log(`   Priority: ${item.priority}`);
        console.log('');
      });
    } catch (error) {
      spinner.fail('Failed to load queue');
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red(message));
      process.exit(1);
    }
  });

program
  .command('server')
  .description('Start HTTP API server')
  .action(async () => {
    console.log(chalk.bold('Starting Content Acquisition Server...\n'));
    try {
      const { ContentAcquisitionServer } = await import('./server.js');
      const database = new ContentAcquisitionDatabase(config.database_url);
      await database.initialize();

      const server = new ContentAcquisitionServer(config, database);
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
      const message = error instanceof Error ? error.message : String(error);
      console.error(chalk.red('Failed to start server:'), message);
      process.exit(1);
    }
  });

program.parse();
