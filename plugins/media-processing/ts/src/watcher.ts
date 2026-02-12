/**
 * Drop-Folder Watcher (UPGRADE 1c)
 * Watches a directory for new media files using fs.watch (Node built-in)
 * Validates file settlement before submitting jobs
 */

import { watch, promises as fs, type FSWatcher, type Stats } from 'fs';
import { join, extname } from 'path';
import { spawn } from 'child_process';
import { createLogger } from '@nself/plugin-utils';
import type { Config } from './config.js';
import type { MediaProcessingDatabase } from './database.js';
import type { WatcherStatus } from './types.js';

const logger = createLogger('media-processing:watcher');

/** Media file extensions to watch for */
const MEDIA_EXTENSIONS = new Set([
  '.mp4', '.mkv', '.avi', '.mov', '.wmv', '.flv', '.webm',
  '.ts', '.m2ts', '.mts', '.m4v', '.mpg', '.mpeg', '.vob',
  '.ogv', '.3gp', '.3g2',
]);

export class DropFolderWatcher {
  private watcher: FSWatcher | null = null;
  private running = false;
  private watchPath: string | null = null;
  private filesDetected = 0;
  private jobsSubmitted = 0;
  private errors = 0;
  private startedAt: Date | null = null;
  private pendingFiles = new Set<string>();

  constructor(
    private config: Config,
    private db: MediaProcessingDatabase
  ) {}

  /**
   * Start watching the configured drop folder
   */
  async start(watchPath?: string): Promise<void> {
    const targetPath = watchPath ?? this.config.dropFolderPath;

    if (!targetPath) {
      throw new Error('No drop folder path configured. Set MP_DROP_FOLDER_PATH or pass a path.');
    }

    // Verify directory exists
    try {
      const stat = await fs.stat(targetPath);
      if (!stat.isDirectory()) {
        throw new Error(`${targetPath} is not a directory`);
      }
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        await fs.mkdir(targetPath, { recursive: true });
        logger.info('Created watch directory', { path: targetPath });
      } else {
        throw error;
      }
    }

    if (this.running) {
      throw new Error('Watcher is already running');
    }

    this.watchPath = targetPath;
    this.running = true;
    this.startedAt = new Date();
    this.filesDetected = 0;
    this.jobsSubmitted = 0;
    this.errors = 0;

    logger.info('Starting drop-folder watcher', {
      path: targetPath,
      settleCheckSeconds: this.config.settleCheckSeconds,
      settleCheckIntervals: this.config.settleCheckIntervals,
    });

    this.watcher = watch(targetPath, { persistent: true }, (eventType, filename) => {
      if (eventType === 'rename' && filename) {
        const filePath = join(targetPath, filename);
        this.handleFileDetected(filePath).catch(err => {
          logger.error('Error handling detected file', { filePath, error: err.message });
        });
      }
    });

    this.watcher.on('error', (error) => {
      logger.error('Watcher error', { error: error.message });
      this.errors++;
    });

    // Also scan existing files in the directory
    await this.scanExistingFiles(targetPath);
  }

  /**
   * Stop the watcher
   */
  stop(): void {
    if (this.watcher) {
      this.watcher.close();
      this.watcher = null;
    }
    this.running = false;
    logger.info('Drop-folder watcher stopped');
  }

  /**
   * Get watcher status
   */
  getStatus(): WatcherStatus {
    return {
      running: this.running,
      watchPath: this.watchPath,
      filesDetected: this.filesDetected,
      jobsSubmitted: this.jobsSubmitted,
      errors: this.errors,
      startedAt: this.startedAt,
    };
  }

  /**
   * Scan existing files in the watch directory
   */
  private async scanExistingFiles(dirPath: string): Promise<void> {
    try {
      const entries = await fs.readdir(dirPath);
      for (const entry of entries) {
        const filePath = join(dirPath, entry);
        await this.handleFileDetected(filePath);
      }
    } catch (error) {
      logger.warn('Failed to scan existing files', { error });
    }
  }

  /**
   * Handle a detected file event
   */
  private async handleFileDetected(filePath: string): Promise<void> {
    // Skip if already being processed
    if (this.pendingFiles.has(filePath)) {
      return;
    }

    // Check if it's a media file extension
    const ext = extname(filePath).toLowerCase();
    if (!MEDIA_EXTENSIONS.has(ext)) {
      return;
    }

    // Check if the file exists (rename event fires for both create and delete)
    try {
      await fs.stat(filePath);
    } catch {
      return; // File was deleted, ignore
    }

    this.filesDetected++;
    this.pendingFiles.add(filePath);

    logger.info('File detected', { filePath });

    try {
      // Record detection event
      await this.db.createWatcherEvent({
        file_path: filePath,
        file_size: 0,
        event_type: 'detected',
        job_id: null,
        error_message: null,
      });

      // Wait for file to settle
      const settled = await this.waitForSettlement(filePath);
      if (!settled) {
        throw new Error('File did not settle within timeout');
      }

      const fileStat = await fs.stat(filePath);

      await this.db.createWatcherEvent({
        file_path: filePath,
        file_size: fileStat.size,
        event_type: 'settled',
        job_id: null,
        error_message: null,
      });

      // Validate with ffprobe
      const isMedia = await this.validateWithFfprobe(filePath);
      if (!isMedia) {
        throw new Error('File is not a valid media file (ffprobe validation failed)');
      }

      await this.db.createWatcherEvent({
        file_path: filePath,
        file_size: fileStat.size,
        event_type: 'validated',
        job_id: null,
        error_message: null,
      });

      // Create a job
      const job = await this.db.createJob({
        input_url: filePath,
        input_type: 'file',
        priority: 0,
      });

      await this.db.createWatcherEvent({
        file_path: filePath,
        file_size: fileStat.size,
        event_type: 'submitted',
        job_id: job.id,
        error_message: null,
      });

      this.jobsSubmitted++;
      logger.info('Job created from watched file', { filePath, jobId: job.id });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      this.errors++;
      logger.error('Failed to process watched file', { filePath, error: message });

      await this.db.createWatcherEvent({
        file_path: filePath,
        file_size: 0,
        event_type: 'error',
        job_id: null,
        error_message: message,
      }).catch(err => {
        logger.error('Failed to record watcher error event', { error: err.message });
      });
    } finally {
      this.pendingFiles.delete(filePath);
    }
  }

  /**
   * Wait for a file to settle (size unchanged across multiple checks)
   */
  private async waitForSettlement(filePath: string): Promise<boolean> {
    const intervalMs = this.config.settleCheckSeconds * 1000;
    const checks = this.config.settleCheckIntervals;

    let previousSize = -1;
    let stableCount = 0;

    for (let i = 0; i < checks + 5; i++) { // Extra iterations as buffer
      await this.sleep(intervalMs);

      let currentStat: Stats;
      try {
        currentStat = await fs.stat(filePath);
      } catch {
        return false; // File disappeared
      }

      if (currentStat.size === previousSize && previousSize >= 0) {
        stableCount++;
        if (stableCount >= checks) {
          return true;
        }
      } else {
        stableCount = 0;
      }

      previousSize = currentStat.size;
    }

    return false;
  }

  /**
   * Validate file with ffprobe
   */
  private validateWithFfprobe(filePath: string): Promise<boolean> {
    return new Promise((resolve) => {
      const proc = spawn(this.config.ffprobePath, [
        '-v', 'quiet',
        '-print_format', 'json',
        '-show_format',
        filePath,
      ]);

      let stdout = '';

      proc.stdout.on('data', (data: Buffer) => {
        stdout += data.toString();
      });

      proc.on('close', (code) => {
        if (code !== 0) {
          resolve(false);
          return;
        }

        try {
          const result = JSON.parse(stdout);
          // Must have a format with a duration (even 0 is ok)
          resolve(result.format != null);
        } catch {
          resolve(false);
        }
      });

      proc.on('error', () => {
        resolve(false);
      });
    });
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
