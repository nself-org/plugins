#!/usr/bin/env node
import { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { MetadataEnrichmentDatabase } from './database.js';
import { TMDBClient } from './tmdb-client.js';

const logger = createLogger('metadata-enrichment:cli');
const program = new Command();

program
  .name('nself-metadata-enrichment')
  .description('Metadata enrichment with TMDB')
  .version('1.0.0');

program
  .command('init')
  .description('Initialize database')
  .action(async () => {
    const spinner = ora('Initializing metadata enrichment').start();
    try {
      const database = new MetadataEnrichmentDatabase(config.database_url);
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
  .command('search-movie <query>')
  .description('Search for movies')
  .action(async (query) => {
    const spinner = ora(`Searching for: ${query}`).start();
    try {
      const tmdb = new TMDBClient(config.tmdb_api_key);
      const results = await tmdb.searchMovies(query);
      spinner.stop();

      if (results.length === 0) {
        console.log(chalk.yellow('No results found'));
        return;
      }

      console.log(chalk.bold(`\nFound ${results.length} movies:\n`));
      results.slice(0, 10).forEach((movie: any, i: number) => {
        console.log(`${i + 1}. ${movie.title} (${movie.release_date?.substring(0, 4)})`);
        console.log(`   ID: ${movie.id} | Rating: ${movie.vote_average}/10`);
        console.log('');
      });
    } catch (error: any) {
      spinner.fail('Search failed');
      console.error(chalk.red(error.message));
      process.exit(1);
    }
  });

program
  .command('search-tv <query>')
  .description('Search for TV shows')
  .action(async (query) => {
    const spinner = ora(`Searching for: ${query}`).start();
    try {
      const tmdb = new TMDBClient(config.tmdb_api_key);
      const results = await tmdb.searchTV(query);
      spinner.stop();

      if (results.length === 0) {
        console.log(chalk.yellow('No results found'));
        return;
      }

      console.log(chalk.bold(`\nFound ${results.length} TV shows:\n`));
      results.slice(0, 10).forEach((show: any, i: number) => {
        console.log(`${i + 1}. ${show.name} (${show.first_air_date?.substring(0, 4)})`);
        console.log(`   ID: ${show.id} | Rating: ${show.vote_average}/10`);
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
    console.log(chalk.bold('Starting Metadata Enrichment Server...\n'));
    try {
      const { MetadataEnrichmentServer } = await import('./server.js');
      const database = new MetadataEnrichmentDatabase(config.database_url);
      await database.initialize();

      const server = new MetadataEnrichmentServer(config, database);
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
