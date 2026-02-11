/**
 * Backup Scheduler
 * Evaluates cron schedules and triggers backups
 */

import { parseExpression } from 'cron-parser';
import { createLogger } from '@nself/plugin-utils';
import { BackupDatabase } from './database.js';
import { BackupService } from './backup.js';
import type { BackupOptions } from './types.js';

const logger = createLogger('backup:scheduler');

export class BackupScheduler {
  private intervalId?: NodeJS.Timeout;
  private runningBackups = new Set<string>();

  constructor(
    private readonly db: BackupDatabase,
    private readonly backupService: BackupService,
    private readonly maxConcurrent: number = 2
  ) {}

  /**
   * Start the scheduler
   */
  start(intervalMs = 60000): void {
    if (this.intervalId) {
      logger.warn('Scheduler already running');
      return;
    }

    logger.info('Starting backup scheduler', { intervalMs });

    // Run immediately
    this.tick().catch(error => {
      logger.error('Scheduler tick failed', { error });
    });

    // Run on interval
    this.intervalId = setInterval(() => {
      this.tick().catch(error => {
        logger.error('Scheduler tick failed', { error });
      });
    }, intervalMs);
  }

  /**
   * Stop the scheduler
   */
  stop(): void {
    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = undefined;
      logger.info('Scheduler stopped');
    }
  }

  /**
   * Check for due schedules and trigger backups
   */
  private async tick(): Promise<void> {
    // Check if we're at max concurrent backups
    if (this.runningBackups.size >= this.maxConcurrent) {
      logger.debug('Max concurrent backups reached, skipping tick');
      return;
    }

    try {
      // Get due schedules
      const dueSchedules = await this.db.getDueSchedules();

      if (dueSchedules.length === 0) {
        return;
      }

      logger.info(`Found ${dueSchedules.length} due schedules`);

      for (const schedule of dueSchedules) {
        // Check if we've hit concurrent limit
        if (this.runningBackups.size >= this.maxConcurrent) {
          logger.debug('Max concurrent backups reached, stopping tick');
          break;
        }

        // Skip if already running
        if (this.runningBackups.has(schedule.id)) {
          continue;
        }

        // Calculate next run time
        const nextRunAt = this.calculateNextRun(schedule.schedule_cron);

        // Update schedule
        await this.db.updateScheduleLastRun(schedule.id, new Date(), nextRunAt);

        // Trigger backup in background
        this.triggerBackup(schedule.id, {
          scheduleId: schedule.id,
          backupType: schedule.backup_type,
          includeTables: schedule.include_tables.length > 0 ? schedule.include_tables : undefined,
          excludeTables: schedule.exclude_tables.length > 0 ? schedule.exclude_tables : undefined,
          compression: schedule.compression,
          encryption: schedule.encryption_enabled ? {
            enabled: true,
            keyId: schedule.encryption_key_id ?? undefined,
          } : undefined,
          targetProvider: schedule.target_provider,
          targetConfig: schedule.target_config,
          retentionDays: schedule.retention_days,
        });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to process due schedules', { error: message });
    }
  }

  /**
   * Trigger a backup in the background
   */
  private triggerBackup(scheduleId: string, options: BackupOptions): void {
    this.runningBackups.add(scheduleId);

    logger.info('Triggering scheduled backup', { scheduleId });

    this.backupService.executeBackup(options)
      .then(result => {
        if (result.success) {
          logger.info('Scheduled backup completed', {
            scheduleId,
            artifactId: result.artifactId,
            duration: result.duration,
          });
        } else {
          logger.error('Scheduled backup failed', {
            scheduleId,
            error: result.error,
          });
        }
      })
      .catch(error => {
        logger.error('Scheduled backup threw error', {
          scheduleId,
          error: error instanceof Error ? error.message : 'Unknown error',
        });
      })
      .finally(() => {
        this.runningBackups.delete(scheduleId);
      });
  }

  /**
   * Calculate next run time from cron expression
   */
  calculateNextRun(cronExpression: string): Date | null {
    try {
      const interval = parseExpression(cronExpression);
      return interval.next().toDate();
    } catch (error) {
      logger.error('Failed to parse cron expression', {
        expression: cronExpression,
        error: error instanceof Error ? error.message : 'Unknown error',
      });
      return null;
    }
  }

  /**
   * Validate cron expression
   */
  static validateCronExpression(expression: string): boolean {
    try {
      parseExpression(expression);
      return true;
    } catch {
      return false;
    }
  }
}
