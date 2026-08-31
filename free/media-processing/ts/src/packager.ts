/**
 * Shaka Packager Integration (UPGRADE 1a)
 * Wraps shaka-packager binary for CMAF HLS+DASH dual output
 * Falls back to FFmpeg-only HLS if packager is not available
 */

import { spawn } from 'child_process';
import { promises as fs } from 'fs';
import { join } from 'path';
import { createLogger } from '@nself/plugin-utils';
import type { Config } from './config.js';
import type { PackagerStreamDescriptor, PackagerOptions, OutputFormat } from './types.js';

const logger = createLogger('media-processing:packager');

export class ShakaPackager {
  private available: boolean | null = null;

  constructor(private config: Config) {}

  /**
   * Check if the shaka-packager binary is available
   */
  async isAvailable(): Promise<boolean> {
    if (this.available !== null) {
      return this.available;
    }

    if (this.config.packager === 'ffmpeg-only') {
      this.available = false;
      return false;
    }

    try {
      const result = await this.execPackager(['--version']);
      this.available = result.exitCode === 0;
      if (this.available) {
        logger.info('Shaka Packager is available', { output: result.stdout.trim() });
      }
    } catch {
      this.available = false;
      logger.info('Shaka Packager not found, will use FFmpeg-only HLS');
    }

    return this.available;
  }

  /**
   * Package fMP4 intermediates into HLS + DASH using CMAF
   */
  async packageCMAF(
    inputDir: string,
    outputDir: string,
    streams: PackagerStreamDescriptor[],
    options: PackagerOptions = {}
  ): Promise<{ hlsManifest: string; dashManifest: string | null }> {
    const isReady = await this.isAvailable();
    if (!isReady) {
      throw new Error('Shaka Packager is not available. Use ffmpeg-only mode.');
    }

    await fs.mkdir(outputDir, { recursive: true });

    const hlsManifest = options.hlsMasterPlaylistOutput ?? join(outputDir, 'master.m3u8');
    const dashManifest = options.mpdOutput ?? join(outputDir, 'manifest.mpd');

    const args = this.buildPackagerArgs(inputDir, outputDir, streams, {
      ...options,
      hlsMasterPlaylistOutput: hlsManifest,
      mpdOutput: dashManifest,
    });

    logger.info('Running Shaka Packager', { args: args.join(' ') });

    const result = await this.execPackager(args);

    if (result.exitCode !== 0) {
      logger.error('Shaka Packager failed', { stderr: result.stderr });
      throw new Error(`Shaka Packager failed with code ${result.exitCode}: ${result.stderr}`);
    }

    logger.info('CMAF packaging complete', { hlsManifest, dashManifest });

    return {
      hlsManifest,
      dashManifest: this.config.outputFormats.includes('dash') ? dashManifest : null,
    };
  }

  /**
   * Get the configured output formats
   */
  getOutputFormats(): OutputFormat[] {
    return this.config.outputFormats;
  }

  /**
   * Determine whether to use Shaka Packager or FFmpeg-only
   */
  async shouldUsePackager(): Promise<boolean> {
    if (this.config.packager === 'ffmpeg-only') {
      return false;
    }

    const wantsCmaf = this.config.outputFormats.includes('cmaf') ||
      (this.config.outputFormats.includes('hls') && this.config.outputFormats.includes('dash'));

    if (!wantsCmaf && this.config.outputFormats.length === 1 && this.config.outputFormats[0] === 'hls') {
      // HLS-only can be done with FFmpeg
      return false;
    }

    return this.isAvailable();
  }

  /**
   * Build packager command-line arguments
   */
  private buildPackagerArgs(
    _inputDir: string,
    outputDir: string,
    streams: PackagerStreamDescriptor[],
    options: PackagerOptions
  ): string[] {
    const args: string[] = [];

    // Build stream descriptors
    for (const stream of streams) {
      const parts: string[] = [];
      parts.push(`in=${stream.input}`);
      parts.push(`stream=${stream.stream}`);

      if (stream.language) {
        parts.push(`language=${stream.language}`);
      }

      if (stream.bandwidth) {
        parts.push(`bandwidth=${stream.bandwidth}`);
      }

      if (stream.output) {
        parts.push(`output=${stream.output}`);
      } else {
        // Generate output path based on stream type and properties
        const suffix = stream.language ? `_${stream.language}` : '';
        const bw = stream.bandwidth ? `_${stream.bandwidth}` : '';
        parts.push(`output=${join(outputDir, `${stream.stream}${suffix}${bw}.mp4`)}`);
      }

      args.push(parts.join(','));
    }

    // HLS output
    if (options.hlsMasterPlaylistOutput) {
      args.push('--hls_master_playlist_output', options.hlsMasterPlaylistOutput);
    }

    // DASH output
    if (options.mpdOutput) {
      args.push('--mpd_output', options.mpdOutput);
    }

    // Segment duration
    if (options.segmentDuration) {
      args.push('--segment_duration', options.segmentDuration.toString());
    }

    return args;
  }

  /**
   * Execute the packager binary
   */
  private execPackager(args: string[]): Promise<{ exitCode: number; stdout: string; stderr: string }> {
    return new Promise((resolve, reject) => {
      const proc = spawn(this.config.shakaPackagerPath, args);
      let stdout = '';
      let stderr = '';

      proc.stdout.on('data', (data: Buffer) => {
        stdout += data.toString();
      });

      proc.stderr.on('data', (data: Buffer) => {
        stderr += data.toString();
      });

      proc.on('close', (code) => {
        resolve({ exitCode: code ?? 1, stdout, stderr });
      });

      proc.on('error', (error) => {
        if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
          resolve({ exitCode: 1, stdout: '', stderr: `Binary not found: ${this.config.shakaPackagerPath}` });
        } else {
          reject(error);
        }
      });
    });
  }
}
