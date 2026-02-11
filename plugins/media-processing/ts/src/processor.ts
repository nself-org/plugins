/**
 * Media Processing Processor
 * Orchestrates encoding jobs with FFmpeg
 */

import { createLogger } from '@nself/plugin-utils';
import { promises as fs } from 'fs';
import { join } from 'path';
import type { Config } from './config.js';
import type { MediaProcessingDatabase } from './database.js';
import { FFmpegClient } from './ffmpeg.js';
import type { JobRecord, EncodingProfileRecord } from './types.js';

const logger = createLogger('media-processing:processor');

export class MediaProcessor {
  private ffmpeg: FFmpegClient;
  private activeJobs = new Map<string, AbortController>();

  constructor(
    private config: Config,
    private db: MediaProcessingDatabase
  ) {
    this.ffmpeg = new FFmpegClient(config);
  }

  /**
   * Process a single job
   */
  async processJob(jobId: string): Promise<void> {
    logger.info('Starting job processing', { jobId });

    const abortController = new AbortController();
    this.activeJobs.set(jobId, abortController);

    try {
      // Get job details
      const job = await this.db.getJob(jobId);
      if (!job) {
        throw new Error('Job not found');
      }

      // Check if cancelled
      if (job.status === 'cancelled') {
        logger.info('Job was cancelled', { jobId });
        return;
      }

      // Get encoding profile
      let profile: EncodingProfileRecord | null = null;
      if (job.profile_id) {
        profile = await this.db.getEncodingProfile(job.profile_id);
      }
      if (!profile) {
        profile = await this.db.getDefaultEncodingProfile();
      }
      if (!profile) {
        throw new Error('No encoding profile found');
      }

      // Download input (if URL)
      await this.db.updateJobStatus(jobId, 'downloading');
      const inputPath = await this.downloadInput(job);

      // Analyze media
      await this.db.updateJobStatus(jobId, 'analyzing');
      const metadata = await this.ffmpeg.probe(inputPath);
      await this.db.updateJobMetadata(jobId, metadata);

      const duration = metadata.duration ?? 0;

      // Setup output directory
      const outputBasePath = job.output_base_path ?? join(this.config.outputBasePath, jobId);
      await fs.mkdir(outputBasePath, { recursive: true });

      // Encoding phase
      await this.db.updateJobStatus(jobId, 'encoding', 0);

      if (profile.hls_enabled) {
        // Generate HLS streams
        const hlsDir = join(outputBasePath, 'hls');
        await fs.mkdir(hlsDir, { recursive: true });

        await this.ffmpeg.generateHls(
          inputPath,
          hlsDir,
          profile.resolutions,
          {
            videoCodec: profile.video_codec,
            audioCodec: profile.audio_codec,
            audioBitrate: profile.audio_bitrate,
            preset: profile.preset,
            framerate: profile.framerate,
            segmentDuration: profile.hls_segment_duration,
            hardwareAccel: this.config.hardwareAccel,
          },
          (currentTime: number) => {
            const progress = duration > 0 ? Math.min((currentTime / duration) * 100, 100) : 0;
            this.db.updateJobStatus(jobId, 'encoding', progress).catch(err => {
              logger.error('Failed to update progress', { error: err.message });
            });
          }
        );

        // Record HLS outputs
        const masterManifestPath = join(hlsDir, 'master.m3u8');
        const variantManifests = profile.resolutions.map(r => ({
          resolution_label: r.label,
          bandwidth: r.bitrate,
          width: r.width,
          height: r.height,
          codecs: 'avc1.64001f,mp4a.40.2',
          manifest_path: join(hlsDir, r.label, 'playlist.m3u8'),
        }));

        await this.db.createHlsManifest({
          job_id: jobId,
          master_manifest_path: masterManifestPath,
          variant_manifests: variantManifests,
          segment_count: 0, // TODO: count segments
          total_duration_seconds: duration,
        });

        // Record master manifest as output
        await this.db.createJobOutput({
          job_id: jobId,
          output_type: 'hls_manifest',
          resolution_label: null,
          file_path: masterManifestPath,
          file_size_bytes: null,
          content_type: 'application/vnd.apple.mpegurl',
          width: null,
          height: null,
          bitrate: null,
          duration_seconds: duration,
          language: null,
          metadata: {},
        });
      } else {
        // Generate individual video files for each resolution
        for (const resolution of profile.resolutions) {
          const outputPath = join(outputBasePath, `${resolution.label}.${profile.container}`);

          await this.ffmpeg.transcode(
            inputPath,
            outputPath,
            resolution,
            {
              videoCodec: profile.video_codec,
              audioCodec: profile.audio_codec,
              audioBitrate: profile.audio_bitrate,
              preset: profile.preset,
              framerate: profile.framerate,
              hardwareAccel: this.config.hardwareAccel,
            },
            (currentTime: number) => {
              const progress = duration > 0 ? Math.min((currentTime / duration) * 100, 100) : 0;
              this.db.updateJobStatus(jobId, 'encoding', progress).catch(err => {
                logger.error('Failed to update progress', { error: err.message });
              });
            }
          );

          // Get file size
          const stats = await fs.stat(outputPath);

          // Record output
          await this.db.createJobOutput({
            job_id: jobId,
            output_type: 'video',
            resolution_label: resolution.label,
            file_path: outputPath,
            file_size_bytes: stats.size,
            content_type: this.getContentType(profile.container),
            width: resolution.width,
            height: resolution.height,
            bitrate: resolution.bitrate,
            duration_seconds: duration,
            language: null,
            metadata: {},
          });
        }
      }

      // Extract subtitles
      if (profile.subtitle_extract) {
        await this.db.updateJobStatus(jobId, 'packaging', 10);
        const subtitleDir = join(outputBasePath, 'subtitles');
        await fs.mkdir(subtitleDir, { recursive: true });

        try {
          const subtitlePaths = await this.ffmpeg.extractSubtitles(
            inputPath,
            join(subtitleDir, 'subtitle_%d_%l.vtt'),
            'vtt'
          );

          for (const subtitlePath of subtitlePaths) {
            const filename = subtitlePath.split('/').pop() ?? '';
            const langMatch = filename.match(/_([a-z]{2,3})\.vtt$/);
            const language = langMatch ? langMatch[1] : 'und';

            await this.db.createSubtitle({
              job_id: jobId,
              language,
              label: language.toUpperCase(),
              format: 'vtt',
              file_path: subtitlePath,
              is_default: language === 'en',
              is_forced: false,
            });
          }
        } catch (error) {
          logger.warn('Subtitle extraction failed (non-fatal)', { error });
        }
      }

      // Generate thumbnails
      if (profile.thumbnail_enabled) {
        await this.db.updateJobStatus(jobId, 'packaging', 50);
        const thumbDir = join(outputBasePath, 'thumbnails');
        await fs.mkdir(thumbDir, { recursive: true });

        try {
          const thumbPaths = await this.ffmpeg.extractThumbnails(
            inputPath,
            join(thumbDir, 'thumb_%03d.jpg'),
            profile.thumbnail_count
          );

          for (const thumbPath of thumbPaths) {
            const stats = await fs.stat(thumbPath);
            await this.db.createJobOutput({
              job_id: jobId,
              output_type: 'thumbnail',
              resolution_label: null,
              file_path: thumbPath,
              file_size_bytes: stats.size,
              content_type: 'image/jpeg',
              width: 320,
              height: null,
              bitrate: null,
              duration_seconds: null,
              language: null,
              metadata: {},
            });
          }
        } catch (error) {
          logger.warn('Thumbnail generation failed (non-fatal)', { error });
        }
      }

      // Generate trickplay tiles
      if (profile.trickplay_enabled) {
        await this.db.updateJobStatus(jobId, 'packaging', 80);
        const trickplayPath = join(outputBasePath, 'trickplay.jpg');

        try {
          await this.ffmpeg.generateTrickplay(inputPath, trickplayPath, {
            interval: profile.trickplay_interval,
            tileWidth: 320,
            tileHeight: 180,
            columns: 10,
            rows: 10,
          });

          await this.db.createTrickplay({
            job_id: jobId,
            tile_width: 320,
            tile_height: 180,
            columns: 10,
            rows: 10,
            interval_seconds: profile.trickplay_interval,
            file_path: trickplayPath,
            index_path: null,
            total_thumbnails: null,
          });

          const stats = await fs.stat(trickplayPath);
          await this.db.createJobOutput({
            job_id: jobId,
            output_type: 'trickplay',
            resolution_label: null,
            file_path: trickplayPath,
            file_size_bytes: stats.size,
            content_type: 'image/jpeg',
            width: 3200,
            height: 1800,
            bitrate: null,
            duration_seconds: null,
            language: null,
            metadata: {},
          });
        } catch (error) {
          logger.warn('Trickplay generation failed (non-fatal)', { error });
        }
      }

      // Cleanup input if it was downloaded
      if (job.input_type === 'url' || job.input_type === 's3') {
        try {
          await fs.unlink(inputPath);
        } catch (error) {
          logger.warn('Failed to cleanup input file', { error });
        }
      }

      // Mark job as completed
      await this.db.updateJobStatus(jobId, 'completed', 100);
      logger.info('Job completed successfully', { jobId });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Job processing failed', { jobId, error: message });
      await this.db.updateJobStatus(jobId, 'failed', undefined, message);
      throw error;
    } finally {
      this.activeJobs.delete(jobId);
    }
  }

  /**
   * Cancel a running job
   */
  async cancelJob(jobId: string): Promise<void> {
    const abortController = this.activeJobs.get(jobId);
    if (abortController) {
      abortController.abort();
      this.activeJobs.delete(jobId);
      logger.info('Job cancelled', { jobId });
    }

    await this.db.cancelJob(jobId);
  }

  /**
   * Download input file if needed
   */
  private async downloadInput(job: JobRecord): Promise<string> {
    if (job.input_type === 'file') {
      // Local file, use as-is
      return job.input_url;
    }

    // For URL or S3, download to temp location
    const tempDir = join(this.config.outputBasePath, 'temp');
    await fs.mkdir(tempDir, { recursive: true });

    const ext = job.input_url.split('.').pop() ?? 'mp4';
    const tempPath = join(tempDir, `${job.id}.${ext}`);

    if (job.input_type === 'url') {
      logger.info('Downloading from URL', { url: job.input_url });

      // Simple HTTP download
      const response = await fetch(job.input_url);
      if (!response.ok) {
        throw new Error(`Failed to download: ${response.statusText}`);
      }

      const arrayBuffer = await response.arrayBuffer();
      await fs.writeFile(tempPath, Buffer.from(arrayBuffer));

      logger.info('Download complete', { path: tempPath });
      return tempPath;
    }

    if (job.input_type === 's3') {
      // TODO: Implement S3 download with AWS SDK
      throw new Error('S3 input type not yet implemented');
    }

    throw new Error(`Unknown input type: ${job.input_type}`);
  }

  /**
   * Get content type for container format
   */
  private getContentType(container: string): string {
    switch (container) {
      case 'mp4':
        return 'video/mp4';
      case 'mkv':
        return 'video/x-matroska';
      case 'webm':
        return 'video/webm';
      case 'ts':
        return 'video/mp2t';
      default:
        return 'application/octet-stream';
    }
  }

  /**
   * Get active job count
   */
  getActiveJobCount(): number {
    return this.activeJobs.size;
  }

  /**
   * Check if job is active
   */
  isJobActive(jobId: string): boolean {
    return this.activeJobs.has(jobId);
  }
}
