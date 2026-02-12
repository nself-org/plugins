import { execFile } from 'child_process';
import { promisify } from 'util';
import fs from 'fs/promises';
import path from 'path';
import { createLogger } from '@nself/plugin-utils';
import type { SubtitleManagerConfig, SyncResult } from './types.js';

const execFileAsync = promisify(execFile);
const logger = createLogger('subtitle-manager:sync');

export interface SyncOptions {
  /** Use only alass (skip ffsubsync) */
  alassOnly?: boolean;
  /** Use only ffsubsync (skip alass) */
  ffsubsyncOnly?: boolean;
}

export class SubtitleSynchronizer {
  private config: SubtitleManagerConfig;

  constructor(config: SubtitleManagerConfig) {
    this.config = config;
  }

  // ---------------------------------------------------------------------------
  // Public pipeline
  // ---------------------------------------------------------------------------

  /**
   * Orchestrates the full sync pipeline:
   *   alass first pass -> ffsubsync second pass -> return combined result
   */
  async syncSubtitle(
    videoPath: string,
    subtitlePath: string,
    outputPath: string,
    options?: SyncOptions,
  ): Promise<SyncResult> {
    logger.info('Starting subtitle sync pipeline', { videoPath, subtitlePath, outputPath });

    // Verify input files exist
    await fs.access(videoPath);
    await fs.access(subtitlePath);

    // Ensure output directory exists
    const outputDir = path.dirname(outputPath);
    await fs.mkdir(outputDir, { recursive: true });

    let alassResult: SyncResult['alassResult'] | undefined;
    let ffsubsyncResult: SyncResult['ffsubsyncResult'] | undefined;
    let currentSubtitlePath = subtitlePath;
    let method: SyncResult['method'] = 'both';

    const useAlass = !options?.ffsubsyncOnly;
    const useFfsubsync = !options?.alassOnly;

    // First pass: alass
    if (useAlass && await this.isAlassAvailable()) {
      const alassOutputPath = useFfsubsync
        ? outputPath + '.alass.tmp.srt'
        : outputPath;
      try {
        alassResult = await this.syncWithAlass(videoPath, currentSubtitlePath, alassOutputPath);
        currentSubtitlePath = alassOutputPath;
        logger.info('alass pass complete', { confidence: alassResult.confidence, offsetMs: alassResult.offsetMs });
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn('alass pass failed, continuing pipeline', { error: message });
      }
    } else if (useAlass) {
      logger.warn('alass binary not available, skipping first pass');
    }

    // Second pass: ffsubsync
    if (useFfsubsync && await this.isFfsubsyncAvailable()) {
      try {
        ffsubsyncResult = await this.syncWithFfsubsync(videoPath, currentSubtitlePath, outputPath);
        logger.info('ffsubsync pass complete', { confidence: ffsubsyncResult.confidence, offsetMs: ffsubsyncResult.offsetMs });
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logger.warn('ffsubsync pass failed', { error: message });

        // If ffsubsync fails but alass succeeded, copy alass output as final
        if (alassResult && currentSubtitlePath !== outputPath) {
          await fs.copyFile(currentSubtitlePath, outputPath);
        }
      }
    } else if (useFfsubsync) {
      logger.warn('ffsubsync binary not available, skipping second pass');
      // Copy alass output as final if it was written to a temp file
      if (alassResult && currentSubtitlePath !== outputPath) {
        await fs.copyFile(currentSubtitlePath, outputPath);
      }
    }

    // Clean up temp file from alass if ffsubsync ran
    if (useFfsubsync && alassResult) {
      const tmpPath = outputPath + '.alass.tmp.srt';
      try {
        await fs.unlink(tmpPath);
      } catch {
        // temp file may not exist if alass failed
      }
    }

    // Determine method used
    if (alassResult && ffsubsyncResult) {
      method = 'both';
    } else if (alassResult) {
      method = 'alass';
    } else if (ffsubsyncResult) {
      method = 'ffsubsync';
    } else {
      // Neither tool ran successfully; copy original to output
      await fs.copyFile(subtitlePath, outputPath);
      method = 'alass'; // fallback label
      logger.warn('No sync tools available; copied original subtitle to output');
    }

    // Calculate combined confidence and offset
    const confidence = this.computeAggregateConfidence(alassResult, ffsubsyncResult);
    const offsetMs = ffsubsyncResult?.offsetMs ?? alassResult?.offsetMs ?? 0;

    const result: SyncResult = {
      originalPath: subtitlePath,
      syncedPath: outputPath,
      confidence,
      offsetMs,
      method,
      alassResult,
      ffsubsyncResult,
    };

    logger.info('Sync pipeline complete', { confidence, offsetMs, method });
    return result;
  }

  // ---------------------------------------------------------------------------
  // alass
  // ---------------------------------------------------------------------------

  /**
   * Runs alass binary to correct subtitle offsets, splits, and framerate differences.
   */
  async syncWithAlass(
    videoPath: string,
    subtitlePath: string,
    outputPath: string,
  ): Promise<NonNullable<SyncResult['alassResult']>> {
    logger.debug('Running alass', { videoPath, subtitlePath, outputPath });

    const { stdout, stderr } = await execFileAsync(this.config.alass_path, [
      videoPath,
      subtitlePath,
      outputPath,
    ], { timeout: 300_000 }); // 5 minute timeout

    const combined = stdout + '\n' + stderr;
    const confidence = this.parseAlassConfidence(combined);
    const offsetMs = this.parseAlassOffset(combined);
    const framerateAdjusted = this.parseAlassFramerate(combined);

    return { confidence, offsetMs, framerateAdjusted };
  }

  // ---------------------------------------------------------------------------
  // ffsubsync
  // ---------------------------------------------------------------------------

  /**
   * Runs ffsubsync for audio-based subtitle alignment refinement.
   */
  async syncWithFfsubsync(
    videoPath: string,
    subtitlePath: string,
    outputPath: string,
  ): Promise<NonNullable<SyncResult['ffsubsyncResult']>> {
    logger.debug('Running ffsubsync', { videoPath, subtitlePath, outputPath });

    const { stdout, stderr } = await execFileAsync(this.config.ffsubsync_path, [
      videoPath,
      '-i', subtitlePath,
      '-o', outputPath,
    ], { timeout: 600_000 }); // 10 minute timeout

    const combined = stdout + '\n' + stderr;
    const confidence = this.parseFfsubsyncConfidence(combined);
    const offsetMs = this.parseFfsubsyncOffset(combined);

    return { confidence, offsetMs };
  }

  // ---------------------------------------------------------------------------
  // Binary availability checks
  // ---------------------------------------------------------------------------

  async isAlassAvailable(): Promise<boolean> {
    try {
      await execFileAsync(this.config.alass_path, ['--version'], { timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  async isFfsubsyncAvailable(): Promise<boolean> {
    try {
      await execFileAsync(this.config.ffsubsync_path, ['--version'], { timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  // ---------------------------------------------------------------------------
  // Output parsing helpers
  // ---------------------------------------------------------------------------

  private parseAlassConfidence(output: string): number {
    // alass outputs lines like "Correction confidence: 0.95" or similar metrics
    const match = output.match(/confidence[:\s]+([0-9.]+)/i);
    if (match) {
      return Math.min(1, Math.max(0, parseFloat(match[1])));
    }
    // If no explicit confidence, assume moderate confidence if output is present
    return output.trim().length > 0 ? 0.7 : 0;
  }

  private parseAlassOffset(output: string): number {
    // alass outputs lines like "Offset: 1500 ms" or "offset: -200ms"
    const match = output.match(/offset[:\s]+([+-]?[0-9.]+)\s*(?:ms)?/i);
    if (match) {
      return parseFloat(match[1]);
    }
    return 0;
  }

  private parseAlassFramerate(output: string): boolean {
    // alass mentions framerate adjustment when applied
    return /framerate|fps.*adjust|rescal/i.test(output);
  }

  private parseFfsubsyncConfidence(output: string): number {
    // ffsubsync outputs sync quality/score information
    const match = output.match(/(?:sync\s*(?:quality|score|confidence))[:\s]+([0-9.]+)/i);
    if (match) {
      return Math.min(1, Math.max(0, parseFloat(match[1])));
    }
    // Check for framerate ratio as a proxy for success
    const ratioMatch = output.match(/framerate\s*ratio[:\s]+([0-9.]+)/i);
    if (ratioMatch) {
      const ratio = parseFloat(ratioMatch[1]);
      // Ratio close to 1.0 means good sync
      return Math.max(0, 1 - Math.abs(1 - ratio));
    }
    return output.trim().length > 0 ? 0.7 : 0;
  }

  private parseFfsubsyncOffset(output: string): number {
    // ffsubsync outputs lines like "offset seconds: 1.5" or "best offset: 1500ms"
    const msMatch = output.match(/offset[:\s]+([+-]?[0-9.]+)\s*ms/i);
    if (msMatch) {
      return parseFloat(msMatch[1]);
    }
    const secMatch = output.match(/offset\s*(?:seconds)?[:\s]+([+-]?[0-9.]+)/i);
    if (secMatch) {
      return parseFloat(secMatch[1]) * 1000;
    }
    return 0;
  }

  // ---------------------------------------------------------------------------
  // Aggregate confidence
  // ---------------------------------------------------------------------------

  private computeAggregateConfidence(
    alassResult?: SyncResult['alassResult'],
    ffsubsyncResult?: SyncResult['ffsubsyncResult'],
  ): number {
    if (alassResult && ffsubsyncResult) {
      // Weighted average: ffsubsync (audio-based) gets slightly more weight
      return alassResult.confidence * 0.4 + ffsubsyncResult.confidence * 0.6;
    }
    if (alassResult) return alassResult.confidence;
    if (ffsubsyncResult) return ffsubsyncResult.confidence;
    return 0;
  }
}
