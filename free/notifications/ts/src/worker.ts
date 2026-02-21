#!/usr/bin/env node
/**
 * Background worker for processing notification queue
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { db } from './database.js';
import { notificationService } from './service.js';

const logger = createLogger('notifications:worker');

export class NotificationWorker {
  private running = false;
  private activeWorkers = 0;

  /**
   * Start the worker
   */
  async start(): Promise<void> {
    logger.info('Starting Notification Worker...');
    logger.info(`Concurrency: ${config.worker.concurrency}`);
    logger.info(`Poll interval: ${config.worker.poll_interval}ms`);
    logger.info(`Dry run: ${config.development.dry_run}`);

    this.running = true;

    // Start worker loops
    for (let i = 0; i < config.worker.concurrency; i++) {
      this.processLoop(i + 1);
    }

    logger.info('Worker started');
  }

  /**
   * Stop the worker
   */
  async stop(): Promise<void> {
    logger.info('Stopping worker...');
    this.running = false;

    // Wait for active workers to finish
    while (this.activeWorkers > 0) {
      await new Promise((resolve) => setTimeout(resolve, 100));
    }

    await db.close();
    logger.info('Worker stopped');
  }

  /**
   * Process queue items in a loop
   */
  private async processLoop(workerId: number): Promise<void> {
    while (this.running) {
      try {
        this.activeWorkers++;

        // Get next item from queue
        const queueItem = await db.getNextQueueItem();

        if (queueItem) {
          logger.info(`[Worker ${workerId}] Processing queue item: ${queueItem.id}`);

          // Get notification
          const notification = await db.getNotification(queueItem.notification_id);

          if (!notification) {
            logger.error(`[Worker ${workerId}] Notification not found: ${queueItem.notification_id}`);
            await db.updateQueueItem(queueItem.id, 'failed', 'Notification not found');
            continue;
          }

          // Process notification
          const result = await notificationService.process(notification);

          if (result.success) {
            logger.info(`[Worker ${workerId}] Delivered: ${notification.id}`);
            await db.updateQueueItem(queueItem.id, 'completed');
          } else {
            logger.error(`[Worker ${workerId}] Failed: ${notification.id} - ${result.error}`);

            // Update notification status
            await db.updateNotificationStatus(notification.id, 'failed', {
              error_message: result.error,
              failed_at: new Date(),
            });

            // Retry if attempts remaining
            if (queueItem.attempts < queueItem.max_attempts) {
              const retryDelay = Math.min(
                config.retry.delay * Math.pow(2, queueItem.attempts),
                config.retry.max_delay
              );

              logger.info(
                `[Worker ${workerId}] Will retry in ${retryDelay}ms (attempt ${queueItem.attempts + 1}/${queueItem.max_attempts})`
              );

              await db.updateQueueItem(queueItem.id, 'failed', result.error);
            } else {
              logger.warn(`[Worker ${workerId}] Max retries exceeded for ${notification.id}`);
              await db.updateQueueItem(queueItem.id, 'completed');
            }
          }
        } else {
          // No items in queue, wait before polling again
          await new Promise((resolve) => setTimeout(resolve, config.worker.poll_interval));
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error(`[Worker ${workerId}] Error`, { error: message });
        await new Promise((resolve) => setTimeout(resolve, 1000));
      } finally {
        this.activeWorkers--;
      }
    }
  }

  /**
   * Get worker status
   */
  getStatus(): {
    running: boolean;
    activeWorkers: number;
    maxWorkers: number;
  } {
    return {
      running: this.running,
      activeWorkers: this.activeWorkers,
      maxWorkers: config.worker.concurrency,
    };
  }
}

// =============================================================================
// Main
// =============================================================================

const worker = new NotificationWorker();

// Handle graceful shutdown
const gracefulShutdown = async () => {
  logger.info('Received shutdown signal...');
  await worker.stop();
  process.exit(0);
};

process.on('SIGTERM', gracefulShutdown);
process.on('SIGINT', gracefulShutdown);

// Start worker if run directly
if (require.main === module) {
  worker.start().catch((error) => {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start worker', { error: message });
    process.exit(1);
  });
}

export { worker };
