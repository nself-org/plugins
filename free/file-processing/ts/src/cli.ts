#!/usr/bin/env node
/**
 * File Processing Plugin - CLI
 */

import { createLogger } from '@nself/plugin-utils';
import { Command } from 'commander';
import { loadConfig, validateConfig, getDatabaseConfig } from './config.js';
import { Database } from './database.js';
import { createStorageAdapter } from './storage.js';
import { FileProcessor } from './processor.js';

const logger = createLogger('file-processing:cli');

const program = new Command();

program
  .name('nself-file-processing')
  .description('File processing plugin CLI')
  .version('1.0.0');

// Initialize command
program
  .command('init')
  .description('Initialize file processing service')
  .action(async () => {
    try {
      const config = loadConfig();
      validateConfig(config);

      logger.info('Configuration validated');
      logger.info(`Storage provider: ${config.storageProvider}`);
      logger.info(`Bucket: ${config.storageBucket}`);
      logger.info(`Thumbnail sizes: ${config.thumbnailSizes.join(', ')}`);
      logger.info(`Optimization: ${config.enableOptimization ? 'enabled' : 'disabled'}`);
      logger.info(`Virus scanning: ${config.enableVirusScan ? 'enabled' : 'disabled'}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      process.exit(1);
    }
  });

// Process file command
program
  .command('process')
  .description('Process a file')
  .argument('<file-id>', 'File ID')
  .argument('<file-path>', 'File path in storage')
  .action(async (fileId: string, filePath: string) => {
    try {
      const config = loadConfig();
      const db = new Database(getDatabaseConfig());
      const storage = createStorageAdapter(config);
      const processor = new FileProcessor(config, storage);

      logger.info(`Processing file: ${filePath}`);

      // Create job
      const jobId = await db.createJob({
        fileId,
        filePath,
        fileName: filePath.split('/').pop() || filePath,
        fileSize: 0,
        mimeType: 'application/octet-stream',
        operations: ['thumbnail', 'optimize', 'metadata'],
      });

      logger.info(`Job created: ${jobId}`);

      await db.close();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Processing failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('View processing statistics')
  .action(async () => {
    try {
      const db = new Database(getDatabaseConfig());
      const stats = await db.getStats();

      logger.info('\nFile Processing Statistics:');
      logger.info('─'.repeat(50));
      logger.info(`Pending:              ${stats.pending}`);
      logger.info(`Processing:           ${stats.processing}`);
      logger.info(`Completed:            ${stats.completed}`);
      logger.info(`Failed:               ${stats.failed}`);
      logger.info(`Avg Duration:         ${stats.avgDurationMs}ms`);
      logger.info(`Total Processed:      ${stats.totalProcessed}`);
      logger.info(`Thumbnails Generated: ${stats.thumbnailsGenerated}`);
      logger.info(`Storage Used:         ${formatBytes(stats.storageUsed)}`);
      logger.info('─'.repeat(50));

      await db.close();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get stats', { error: message });
      process.exit(1);
    }
  });

// Cleanup command
program
  .command('cleanup')
  .description('Clean up old processing jobs')
  .option('-d, --days <days>', 'Retention days', '30')
  .action(async (options: { days: string }) => {
    try {
      const db = new Database(getDatabaseConfig());
      const days = parseInt(options.days, 10);
      const deleted = await db.cleanup(days);

      logger.info(`Cleaned up ${deleted} jobs older than ${days} days`);

      await db.close();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cleanup failed', { error: message });
      process.exit(1);
    }
  });

// Helper function
function formatBytes(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unitIndex = 0;

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }

  return `${size.toFixed(2)} ${units[unitIndex]}`;
}

program.parse();
