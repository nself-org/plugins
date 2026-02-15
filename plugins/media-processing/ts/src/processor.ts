/**
 * Media Processing Processor
 * Orchestrates encoding jobs with FFmpeg
 * UPGRADE 1: Added Shaka Packager, QA validation, object storage upload, and job leasing
 */

import { createLogger } from '@nself/plugin-utils';
import { promises as fs } from 'fs';
import { join, basename } from 'path';
import { randomUUID } from 'crypto';
import type { Config } from './config.js';
import type { MediaProcessingDatabase } from './database.js';
import { FFmpegClient } from './ffmpeg.js';
import { ShakaPackager } from './packager.js';
import { QAValidator } from './qa-validator.js';
import { StorageUploader } from './upload.js';
import type { JobRecord, EncodingProfileRecord, PackagerStreamDescriptor } from './types.js';

const logger = createLogger('media-processing:processor');

/** Heartbeat interval in milliseconds (30 seconds) */
const HEARTBEAT_INTERVAL_MS = 30_000;

/** Stale job timeout in minutes */
const STALE_JOB_TIMEOUT_MINUTES = 15;

export class MediaProcessor {
  private ffmpeg: FFmpegClient;
  private packager: ShakaPackager;
  private qaValidator: QAValidator;
  private uploader: StorageUploader;
  private activeJobs = new Map<string, AbortController>();
  private heartbeatTimers = new Map<string, ReturnType<typeof setInterval>>();
  private workerId: string;

  constructor(
    private config: Config,
    private db: MediaProcessingDatabase
  ) {
    this.ffmpeg = new FFmpegClient(config);
    this.packager = new ShakaPackager(config);
    this.qaValidator = new QAValidator();
    this.uploader = new StorageUploader(config, db);
    this.workerId = `worker-${randomUUID().substring(0, 8)}`;
  }

  /**
   * Initialize the processor - reclaim stale jobs on startup
   */
  async initialize(): Promise<void> {
    logger.info('Initializing processor', { workerId: this.workerId });
    const reclaimed = await this.db.reclaimStaleJobs(STALE_JOB_TIMEOUT_MINUTES);
    if (reclaimed > 0) {
      logger.warn('Reclaimed stale jobs on startup', { count: reclaimed });
    }
  }

  /**
   * Lease and process the next available job
   */
  async leaseAndProcessNext(): Promise<boolean> {
    const leasedJob = await this.db.leaseNextJob(this.workerId);
    if (!leasedJob) {
      return false;
    }

    logger.info('Leased job', { jobId: leasedJob.id, workerId: this.workerId });

    // Process in background
    this.processJob(leasedJob.id).catch(error => {
      logger.error('Job processing error', { jobId: leasedJob.id, error: error.message });
    });

    return true;
  }

  /**
   * Process a single job
   */
  async processJob(jobId: string): Promise<void> {
    logger.info('Starting job processing', { jobId });

    const abortController = new AbortController();
    this.activeJobs.set(jobId, abortController);

    // Start heartbeat
    this.startHeartbeat(jobId);

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

      // Decide encoding path BEFORE running FFmpeg
      const usePackager = profile.hls_enabled && await this.packager.shouldUsePackager();

      if (profile.hls_enabled && usePackager) {
        // =====================================================================
        // PATH A: FFmpeg → fMP4 intermediates → Shaka Packager → CMAF (HLS + DASH)
        // =====================================================================
        const fmp4Dir = join(outputBasePath, 'intermediates');
        await fs.mkdir(fmp4Dir, { recursive: true });

        const streams: PackagerStreamDescriptor[] = [];

        // Step 1: Encode each resolution to fragmented MP4
        for (let i = 0; i < profile.resolutions.length; i++) {
          const resolution = profile.resolutions[i];
          const fmp4Path = join(fmp4Dir, `${resolution.label}.mp4`);

          await this.ffmpeg.encodeToFmp4(
            inputPath,
            fmp4Path,
            resolution,
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
              // Spread progress across all resolutions (e.g. 5 rungs = 20% each)
              const rungProgress = duration > 0 ? Math.min((currentTime / duration) * 100, 100) : 0;
              const overallProgress = ((i / profile.resolutions.length) * 80) +
                (rungProgress / profile.resolutions.length * 0.8);
              this.db.updateJobStatus(jobId, 'encoding', Math.min(overallProgress, 80)).catch(err => {
                logger.error('Failed to update progress', { error: err.message });
              });
            }
          );

          streams.push({
            input: fmp4Path,
            stream: 'video',
            bandwidth: resolution.bitrate,
          });
        }

        // Step 2: Package with Shaka Packager → CMAF (HLS + DASH)
        await this.db.updateJobStatus(jobId, 'packaging', 0);
        const cmafDir = join(outputBasePath, 'cmaf');
        await fs.mkdir(cmafDir, { recursive: true });

        let cmafResult: { hlsManifest: string; dashManifest: string | null };
        try {
          cmafResult = await this.packager.packageCMAF(fmp4Dir, cmafDir, streams, {
            segmentDuration: profile.hls_segment_duration,
          });

          logger.info('CMAF packaging complete', {
            jobId,
            hlsManifest: cmafResult.hlsManifest,
            dashManifest: cmafResult.dashManifest,
          });
        } catch (error) {
          // Shaka Packager failed — fall back to FFmpeg HLS
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.warn('CMAF packaging failed, falling back to FFmpeg-only HLS', { jobId, error: message });

          // Re-encode using FFmpeg HLS path as fallback
          const hlsDir = join(outputBasePath, 'hls');
          await fs.mkdir(hlsDir, { recursive: true });

          await this.ffmpeg.generateHls(inputPath, hlsDir, profile.resolutions, {
            videoCodec: profile.video_codec,
            audioCodec: profile.audio_codec,
            audioBitrate: profile.audio_bitrate,
            preset: profile.preset,
            framerate: profile.framerate,
            segmentDuration: profile.hls_segment_duration,
            hardwareAccel: this.config.hardwareAccel,
          });

          cmafResult = {
            hlsManifest: join(hlsDir, 'master.m3u8'),
            dashManifest: null,
          };
        }

        // Record manifests
        const masterManifestPath = cmafResult.hlsManifest;
        const variantManifests = profile.resolutions.map(r => ({
          resolution_label: r.label,
          bandwidth: r.bitrate,
          width: r.width,
          height: r.height,
          codecs: 'avc1.64001f,mp4a.40.2',
          manifest_path: join(cmafDir, `video_${r.bitrate}.mp4`),
        }));

        await this.db.createHlsManifest({
          job_id: jobId,
          master_manifest_path: masterManifestPath,
          variant_manifests: variantManifests,
          segment_count: 0,
          total_duration_seconds: duration,
        });

        // Record HLS manifest output
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

        // Record DASH manifest output if produced
        if (cmafResult.dashManifest) {
          await this.db.createJobOutput({
            job_id: jobId,
            output_type: 'hls_manifest', // reusing type for DASH manifest
            resolution_label: null,
            file_path: cmafResult.dashManifest,
            file_size_bytes: null,
            content_type: 'application/dash+xml',
            width: null,
            height: null,
            bitrate: null,
            duration_seconds: duration,
            language: null,
            metadata: { format: 'dash' },
          });
        }

        // Clean up intermediates
        try {
          await fs.rm(fmp4Dir, { recursive: true, force: true });
          logger.debug('Cleaned up fMP4 intermediates', { fmp4Dir });
        } catch {
          logger.warn('Failed to clean up fMP4 intermediates (non-fatal)', { fmp4Dir });
        }

      } else if (profile.hls_enabled) {
        // =====================================================================
        // PATH B: FFmpeg → HLS directly (no Shaka Packager)
        // =====================================================================
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
          segment_count: 0,
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

      // QA Validation (UPGRADE 1f)
      logger.info('Running QA validation', { jobId });
      const qaResult = await this.qaValidator.validateOutput(outputBasePath);

      if (qaResult.status === 'fail') {
        logger.error('QA validation failed', { jobId, issues: qaResult.issues });
        await this.db.updateJobStatus(jobId, 'qa_failed', undefined, `QA failed: ${qaResult.issues.join('; ')}`);
        return; // Block upload on QA failure
      }

      if (qaResult.status === 'warn') {
        logger.warn('QA validation passed with warnings', { jobId, issues: qaResult.issues });
      }

      // Object Storage Upload (UPGRADE 1e)
      if (this.config.objectStorageUrl) {
        try {
          await this.db.updateJobStatus(jobId, 'uploading', 90);

          // Derive content ID from filename
          const inputFilename = basename(job.input_url);
          const contentId = inputFilename.replace(/\.[^.]+$/, '').replace(/[^a-zA-Z0-9_-]/g, '_');

          await this.uploader.uploadJobOutputs(jobId, outputBasePath, contentId);
          logger.info('Upload complete', { jobId, contentId });
        } catch (error) {
          logger.warn('Upload to object storage failed (non-fatal)', { error });
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
      this.stopHeartbeat(jobId);
      this.activeJobs.delete(jobId);
      await this.db.releaseJobLease(jobId).catch(err => {
        logger.error('Failed to release job lease', { jobId, error: err.message });
      });
    }
  }

  /**
   * Start heartbeat for a job
   */
  private startHeartbeat(jobId: string): void {
    const timer = setInterval(() => {
      this.db.heartbeatJob(jobId, this.workerId).catch(err => {
        logger.error('Heartbeat failed', { jobId, error: err.message });
      });
    }, HEARTBEAT_INTERVAL_MS);

    this.heartbeatTimers.set(jobId, timer);
  }

  /**
   * Stop heartbeat for a job
   */
  private stopHeartbeat(jobId: string): void {
    const timer = this.heartbeatTimers.get(jobId);
    if (timer) {
      clearInterval(timer);
      this.heartbeatTimers.delete(jobId);
    }
  }

  /**
   * Cancel a running job
   */
  async cancelJob(jobId: string): Promise<void> {
    const abortController = this.activeJobs.get(jobId);
    if (abortController) {
      abortController.abort();
      this.stopHeartbeat(jobId);
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
      // NOTE: S3 download requires AWS SDK integration and S3 credentials
      // Integration point: Install @aws-sdk/client-s3 and implement:
      // const s3 = new S3Client({ region: 'us-east-1' });
      // const command = new GetObjectCommand({ Bucket: bucket, Key: key });
      // const response = await s3.send(command);
      // await pipeline(response.Body, fs.createWriteStream(tempPath));
      // Requires AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION environment variables
      throw new Error('S3 input type requires AWS SDK integration (planned feature). Currently supported: url, local. See inline comments for integration requirements.');
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

  /**
   * Get the worker ID for this processor instance
   */
  getWorkerId(): string {
    return this.workerId;
  }
}
