/**
 * File Processing Plugin - Background Worker
 */

import { createLogger } from '@nself/plugin-utils';
import { Worker, Queue } from 'bullmq';
import { loadConfig, validateConfig, getDatabaseConfig } from './config.js';
import { Database } from './database.js';
import { createStorageAdapter } from './storage.js';
import { FileProcessor } from './processor.js';
import type { QueueJobData, QueueJobResult } from './types.js';

const logger = createLogger('file-processing:worker');

async function startWorker() {
  const config = loadConfig();
  validateConfig(config);

  const db = new Database(getDatabaseConfig());
  const storage = createStorageAdapter(config);
  const processor = new FileProcessor(config, storage);

  // Parse Redis URL
  const redisUrl = new URL(config.redisUrl || 'redis://localhost:6379');
  const redisConnection = {
    host: redisUrl.hostname,
    port: parseInt(redisUrl.port, 10) || 6379,
  };

  // Create queue
  const queue = new Queue('file-processing', {
    connection: redisConnection,
  });

  // Create worker
  const worker = new Worker<QueueJobData, QueueJobResult>(
    'file-processing',
    async (job) => {
      const { jobId, filePath, mimeType, operations } = job.data;

      logger.info(`Processing job ${jobId}: ${filePath}`);

      try {
        // Update job status to processing
        await db.updateJobStatus(jobId, 'processing');

        // Process file
        const startTime = Date.now();
        const results = await processor.process(
          filePath,
          filePath,
          mimeType,
          operations
        );

        // Save results to database
        for (const thumbnail of results.thumbnails) {
          await db.saveThumbnail(jobId, thumbnail);
        }

        if (results.metadata) {
          await db.saveMetadata(jobId, results.metadata);
        }

        if (results.scan) {
          await db.saveScan(jobId, results.scan);
        }

        // Update job status to completed
        await db.updateJobStatus(jobId, 'completed');

        const duration = Date.now() - startTime;
        logger.info(`Job ${jobId} completed in ${duration}ms`);

        return {
          success: true,
          jobId,
          thumbnails: results.thumbnails,
          metadata: results.metadata?.extracted,
          scan: results.scan,
          optimization: results.optimization,
          duration,
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error(`Job ${jobId} failed`, { error: message });

        // Update job status to failed
        await db.updateJobStatus(jobId, 'failed', {
          message,
          stack: error instanceof Error ? error.stack : undefined,
        });

        throw error;
      }
    },
    {
      connection: redisConnection,
      concurrency: config.queueConcurrency,
    }
  );

  // Worker event handlers
  worker.on('completed', (job) => {
    logger.info(`Job ${job.id} completed`);
  });

  worker.on('failed', (job, error) => {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error(`Job ${job?.id} failed`, { error: message });
  });

  worker.on('error', (error) => {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Worker error', { error: message });
  });

  logger.info(`Worker started (concurrency: ${config.queueConcurrency})`);

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down worker...');
    await worker.close();
    await queue.close();
    await db.close();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);
}

startWorker().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
