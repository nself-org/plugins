#!/usr/bin/env node
/**
 * Jobs CLI
 * Command-line interface for job management
 */

import { Command } from 'commander';
import { Queue } from 'bullmq';
import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const IORedis = require('ioredis');
import { createLogger } from '@nself/plugin-utils';
import { getConfig } from './config.js';

const logger = createLogger('jobs:cli');

const program = new Command();
const config = getConfig();

const connection = new IORedis(config.redisUrl, {
  maxRetriesPerRequest: null,
});

program
  .name('nself-jobs')
  .description('BullMQ job queue CLI')
  .version('1.0.0');

// Retry command
program
  .command('retry')
  .description('Retry failed jobs')
  .option('--queue <queue>', 'Queue name')
  .option('--type <type>', 'Job type')
  .option('--id <id>', 'Specific job ID')
  .option('--limit <limit>', 'Limit number of jobs', '10')
  .action(async (options) => {
    logger.info('Retrying jobs...');
    // Implementation would re-queue failed jobs from database
    await connection.quit();
  });

// Add job command
program
  .command('add')
  .description('Add a job to the queue')
  .requiredOption('-t, --type <type>', 'Job type')
  .option('-q, --queue <queue>', 'Queue name', 'default')
  .requiredOption('-p, --payload <json>', 'Job payload (JSON)')
  .option('--priority <priority>', 'Priority (critical, high, normal, low)', 'normal')
  .option('--delay <ms>', 'Delay in milliseconds')
  .action(async (options) => {
    const queue = new Queue(options.queue, { connection });
    const payload = JSON.parse(options.payload);

    const job = await queue.add(options.type, payload, {
      priority: options.priority,
      delay: options.delay ? parseInt(options.delay) : undefined,
    });

    logger.info(`Job added: ${job.id}`);
    await queue.close();
    await connection.quit();
  });

program.parse();
