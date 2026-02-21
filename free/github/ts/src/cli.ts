#!/usr/bin/env node
/**
 * GitHub Plugin CLI
 * Command-line interface for the GitHub plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { GitHubClient } from './client.js';
import { GitHubDatabase } from './database.js';
import { GitHubSyncService } from './sync.js';
import { createServer } from './server.js';

const logger = createLogger('github:cli');

const program = new Command();

program
  .name('nself-github')
  .description('GitHub plugin for nself - sync GitHub data to PostgreSQL')
  .version('1.0.0');

// Sync command
program
  .command('sync')
  .description('Sync GitHub data to database')
  .option('-r, --resources <resources>', 'Comma-separated list of resources to sync', 'all')
  .option('--repos <repos>', 'Comma-separated list of repos to sync')
  .option('--since <date>', 'Only sync changes since date')
  .action(async (options) => {
    try {
      const config = loadConfig();

      logger.info('Starting GitHub sync...');
      if (config.githubOrg) {
        logger.info(`Organization: ${config.githubOrg}`);
      }
      if (config.githubRepos) {
        logger.info(`Repositories: ${config.githubRepos.join(', ')}`);
      }

      const client = new GitHubClient(config.githubToken);
      const db = new GitHubDatabase();
      await db.connect();
      await db.initializeSchema();

      const syncService = new GitHubSyncService(
        client,
        db,
        config.githubOrg,
        options.repos ? options.repos.split(',').map((r: string) => r.trim()) : config.githubRepos
      );

      const resources = options.resources === 'all'
        ? undefined
        : options.resources.split(',').map((r: string) => r.trim());

      const result = await syncService.sync({
        resources: resources as Array<'repositories' | 'issues' | 'pull_requests' | 'commits' | 'releases' | 'workflow_runs' | 'deployments'>,
        since: options.since ? new Date(options.since) : undefined,
      });

      console.log('\nSync Results:');
      console.log('=============');
      console.log(`Repositories:   ${result.stats.repositories}`);
      console.log(`Issues:         ${result.stats.issues}`);
      console.log(`Pull Requests:  ${result.stats.pullRequests}`);
      console.log(`Commits:        ${result.stats.commits}`);
      console.log(`Releases:       ${result.stats.releases}`);
      console.log(`Workflow Runs:  ${result.stats.workflowRuns}`);
      console.log(`Deployments:    ${result.stats.deployments}`);
      console.log(`\nDuration: ${(result.duration / 1000).toFixed(1)}s`);

      if (result.errors.length > 0) {
        console.log('\nErrors:');
        result.errors.forEach(err => console.log(`  - ${err}`));
      }

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Sync failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the webhook server')
  .option('-p, --port <port>', 'Server port', '3002')
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

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new GitHubDatabase();
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

// Status command
program
  .command('status')
  .description('Show sync status and statistics')
  .action(async () => {
    try {
      const config = loadConfig();

      const db = new GitHubDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nGitHub Plugin Status');
      console.log('====================');
      if (config.githubOrg) {
        console.log(`Organization: ${config.githubOrg}`);
      }
      console.log('\nSynced Records:');
      console.log(`  Repositories:   ${stats.repositories}`);
      console.log(`  Issues:         ${stats.issues}`);
      console.log(`  Pull Requests:  ${stats.pullRequests}`);
      console.log(`  Commits:        ${stats.commits}`);
      console.log(`  Releases:       ${stats.releases}`);
      console.log(`  Workflow Runs:  ${stats.workflowRuns}`);
      console.log(`  Deployments:    ${stats.deployments}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Repos command
program
  .command('repos')
  .description('List repositories')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      const db = new GitHubDatabase();
      await db.connect();

      const repos = await db.listRepositories(parseInt(options.limit, 10));
      console.log('\nRepositories:');
      console.log('-'.repeat(100));
      repos.forEach(r => {
        console.log(`${r.full_name} | ${r.language ?? 'N/A'} | ⭐${r.stargazers_count} | 🍴${r.forks_count}`);
      });
      console.log(`\nTotal: ${await db.countRepositories()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Issues command
program
  .command('issues')
  .description('List issues')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-s, --state <state>', 'Filter by state (open, closed)')
  .action(async (options) => {
    try {
      const db = new GitHubDatabase();
      await db.connect();

      const issues = await db.listIssues(undefined, options.state, parseInt(options.limit, 10));
      console.log('\nIssues:');
      console.log('-'.repeat(100));
      issues.forEach(i => {
        console.log(`#${i.number} | ${i.state} | ${i.title.substring(0, 60)}`);
      });
      console.log(`\nOpen: ${await db.countIssues('open')} | Closed: ${await db.countIssues('closed')}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// PRs command
program
  .command('prs')
  .description('List pull requests')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-s, --state <state>', 'Filter by state (open, closed)')
  .action(async (options) => {
    try {
      const db = new GitHubDatabase();
      await db.connect();

      const prs = await db.listPullRequests(undefined, options.state, parseInt(options.limit, 10));
      console.log('\nPull Requests:');
      console.log('-'.repeat(100));
      prs.forEach(p => {
        const merged = p.merged ? '✅' : (p.state === 'closed' ? '❌' : '⏳');
        console.log(`#${p.number} | ${merged} | ${p.title.substring(0, 60)}`);
      });
      console.log(`\nOpen: ${await db.countPullRequests('open')} | Closed: ${await db.countPullRequests('closed')}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
