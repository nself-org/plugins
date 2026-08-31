/**
 * FFmpeg Client
 * Real FFmpeg/FFprobe execution for media processing
 */

import { spawn } from 'child_process';
import { createLogger } from '@nself/plugin-utils';
import type { FFprobeResult, MediaMetadata, Resolution } from './types.js';
import type { Config } from './config.js';

const logger = createLogger('media-processing:ffmpeg');

export class FFmpegClient {
  constructor(private config: Config) {}

  /**
   * Probe media file to extract metadata
   */
  async probe(inputPath: string): Promise<MediaMetadata> {
    logger.debug('Probing media file', { inputPath });

    return new Promise((resolve, reject) => {
      const args = [
        '-v', 'quiet',
        '-print_format', 'json',
        '-show_format',
        '-show_streams',
        '-show_chapters',
        inputPath,
      ];

      const process = spawn(this.config.ffprobePath, args);
      let stdout = '';
      let stderr = '';

      process.stdout.on('data', (data: Buffer) => {
        stdout += data.toString();
      });

      process.stderr.on('data', (data: Buffer) => {
        stderr += data.toString();
      });

      process.on('close', (code) => {
        if (code !== 0) {
          logger.error('FFprobe failed', { code, stderr });
          reject(new Error(`FFprobe failed with code ${code}: ${stderr}`));
          return;
        }

        try {
          const result: FFprobeResult = JSON.parse(stdout);
          const metadata: MediaMetadata = {
            format: result.format.format_name,
            duration: result.format.duration ? parseFloat(result.format.duration) : undefined,
            bitrate: result.format.bit_rate ? parseInt(result.format.bit_rate, 10) : undefined,
            size: result.format.size ? parseInt(result.format.size, 10) : undefined,
            streams: result.streams,
            chapters: result.chapters,
          };

          logger.info('Media probed successfully', {
            format: metadata.format,
            duration: metadata.duration,
            streams: metadata.streams?.length,
          });

          resolve(metadata);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error('Failed to parse FFprobe output', { error: message });
          reject(new Error(`Failed to parse FFprobe output: ${message}`));
        }
      });

      process.on('error', (error) => {
        logger.error('FFprobe process error', { error: error.message });
        reject(error);
      });
    });
  }

  /**
   * Transcode video to specific resolution
   */
  async transcode(
    inputPath: string,
    outputPath: string,
    resolution: Resolution,
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      hardwareAccel?: string;
    },
    onProgress?: (progress: number) => void
  ): Promise<void> {
    logger.info('Starting transcode', {
      inputPath,
      outputPath,
      resolution: resolution.label,
      videoCodec: options.videoCodec,
    });

    return new Promise((resolve, reject) => {
      const args = this.buildTranscodeArgs(
        inputPath,
        outputPath,
        resolution,
        options
      );

      const process = spawn(this.config.ffmpegPath, args);
      let stderr = '';

      process.stderr.on('data', (data: Buffer) => {
        const chunk = data.toString();
        stderr += chunk;

        // Parse progress from stderr
        if (onProgress) {
          const progress = this.parseProgress(chunk);
          if (progress !== null) {
            onProgress(progress);
          }
        }
      });

      process.on('close', (code) => {
        if (code !== 0) {
          logger.error('Transcode failed', { code, stderr: stderr.slice(-500) });
          reject(new Error(`FFmpeg failed with code ${code}`));
          return;
        }

        logger.info('Transcode completed', { outputPath });
        resolve();
      });

      process.on('error', (error) => {
        logger.error('FFmpeg process error', { error: error.message });
        reject(error);
      });
    });
  }

  /**
   * Generate HLS playlist with multiple resolutions
   */
  async generateHls(
    inputPath: string,
    outputDir: string,
    resolutions: Resolution[],
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      segmentDuration: number;
      hardwareAccel?: string;
    },
    onProgress?: (progress: number) => void
  ): Promise<string[]> {
    logger.info('Generating HLS streams', {
      inputPath,
      outputDir,
      resolutions: resolutions.map(r => r.label),
    });

    const manifestPaths: string[] = [];

    // Generate each resolution variant
    for (const resolution of resolutions) {
      const variantDir = `${outputDir}/${resolution.label}`;
      const playlistPath = `${variantDir}/playlist.m3u8`;

      await this.generateHlsVariant(
        inputPath,
        variantDir,
        resolution,
        options,
        onProgress
      );

      manifestPaths.push(playlistPath);
    }

    // Generate master playlist
    const masterPath = `${outputDir}/master.m3u8`;
    await this.generateMasterPlaylist(masterPath, resolutions);

    return [masterPath, ...manifestPaths];
  }

  /**
   * Generate single HLS variant
   */
  private async generateHlsVariant(
    inputPath: string,
    outputDir: string,
    resolution: Resolution,
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      segmentDuration: number;
      hardwareAccel?: string;
    },
    onProgress?: (progress: number) => void
  ): Promise<void> {
    return new Promise((resolve, reject) => {
      const args = this.buildHlsArgs(inputPath, outputDir, resolution, options);

      const process = spawn(this.config.ffmpegPath, args);
      let stderr = '';

      process.stderr.on('data', (data: Buffer) => {
        const chunk = data.toString();
        stderr += chunk;

        if (onProgress) {
          const progress = this.parseProgress(chunk);
          if (progress !== null) {
            onProgress(progress);
          }
        }
      });

      process.on('close', (code) => {
        if (code !== 0) {
          logger.error('HLS variant generation failed', { code, stderr: stderr.slice(-500) });
          reject(new Error(`FFmpeg failed with code ${code}`));
          return;
        }

        logger.info('HLS variant generated', { resolution: resolution.label });
        resolve();
      });

      process.on('error', (error) => {
        logger.error('FFmpeg process error', { error: error.message });
        reject(error);
      });
    });
  }

  /**
   * Generate master HLS playlist
   */
  private async generateMasterPlaylist(
    outputPath: string,
    resolutions: Resolution[]
  ): Promise<void> {
    const fs = await import('fs/promises');

    let content = '#EXTM3U\n#EXT-X-VERSION:3\n';

    for (const resolution of resolutions) {
      const bandwidth = resolution.bitrate;
      const codecs = 'avc1.64001f,mp4a.40.2'; // H.264 + AAC

      content += `#EXT-X-STREAM-INF:BANDWIDTH=${bandwidth},RESOLUTION=${resolution.width}x${resolution.height},CODECS="${codecs}"\n`;
      content += `${resolution.label}/playlist.m3u8\n`;
    }

    await fs.writeFile(outputPath, content, 'utf-8');
    logger.info('Master playlist generated', { outputPath });
  }

  /**
   * Extract thumbnails from video
   */
  async extractThumbnails(
    inputPath: string,
    outputPattern: string,
    count: number
  ): Promise<string[]> {
    logger.info('Extracting thumbnails', { inputPath, count });

    return new Promise((resolve, reject) => {
      const args = [
        '-i', inputPath,
        '-vf', `select='eq(pict_type\\,I)',scale=320:-1`,
        '-frames:v', count.toString(),
        '-vsync', 'vfr',
        outputPattern,
      ];

      const process = spawn(this.config.ffmpegPath, args);
      let stderr = '';

      process.stderr.on('data', (data: Buffer) => {
        stderr += data.toString();
      });

      process.on('close', async (code) => {
        if (code !== 0) {
          logger.error('Thumbnail extraction failed', { code, stderr: stderr.slice(-500) });
          reject(new Error(`FFmpeg failed with code ${code}`));
          return;
        }

        // Generate list of thumbnail paths
        const paths: string[] = [];
        const baseDir = outputPattern.substring(0, outputPattern.lastIndexOf('/'));
        const pattern = outputPattern.substring(outputPattern.lastIndexOf('/') + 1);
        const fs = await import('fs/promises');

        try {
          const files = await fs.readdir(baseDir);
          const thumbFiles = files.filter(f => f.match(pattern.replace('%03d', '\\d{3}')));
          for (const file of thumbFiles.sort()) {
            paths.push(`${baseDir}/${file}`);
          }
        } catch (error) {
          logger.warn('Failed to list thumbnails', { error });
        }

        logger.info('Thumbnails extracted', { count: paths.length });
        resolve(paths);
      });

      process.on('error', (error) => {
        logger.error('FFmpeg process error', { error: error.message });
        reject(error);
      });
    });
  }

  /**
   * Generate trickplay tile sprite
   */
  async generateTrickplay(
    inputPath: string,
    outputPath: string,
    options: {
      interval: number;
      tileWidth: number;
      tileHeight: number;
      columns: number;
      rows: number;
    }
  ): Promise<void> {
    logger.info('Generating trickplay tiles', { inputPath, outputPath });

    return new Promise((resolve, reject) => {
      const fps = 1 / options.interval;
      const filter = `fps=${fps},scale=${options.tileWidth}:${options.tileHeight},tile=${options.columns}x${options.rows}`;

      const args = [
        '-i', inputPath,
        '-vf', filter,
        '-frames:v', '1',
        outputPath,
      ];

      const process = spawn(this.config.ffmpegPath, args);
      let stderr = '';

      process.stderr.on('data', (data: Buffer) => {
        stderr += data.toString();
      });

      process.on('close', (code) => {
        if (code !== 0) {
          logger.error('Trickplay generation failed', { code, stderr: stderr.slice(-500) });
          reject(new Error(`FFmpeg failed with code ${code}`));
          return;
        }

        logger.info('Trickplay tiles generated', { outputPath });
        resolve();
      });

      process.on('error', (error) => {
        logger.error('FFmpeg process error', { error: error.message });
        reject(error);
      });
    });
  }

  /**
   * Extract subtitles from video
   */
  async extractSubtitles(
    inputPath: string,
    outputPattern: string,
    format: 'vtt' | 'srt' | 'ass' = 'vtt'
  ): Promise<string[]> {
    logger.info('Extracting subtitles', { inputPath, format });

    // First probe to find subtitle streams
    const metadata = await this.probe(inputPath);
    const subtitleStreams = metadata.streams?.filter(s => s.codec_type === 'subtitle') ?? [];

    if (subtitleStreams.length === 0) {
      logger.info('No subtitle streams found');
      return [];
    }

    const paths: string[] = [];

    // Extract each subtitle stream
    for (let i = 0; i < subtitleStreams.length; i++) {
      const stream = subtitleStreams[i];
      const language = stream.tags?.language ?? 'und';
      const outputPath = outputPattern.replace('%d', i.toString()).replace('%l', language);

      await new Promise<void>((resolve, reject) => {
        const args = [
          '-i', inputPath,
          '-map', `0:s:${i}`,
          '-c:s', format,
          outputPath,
        ];

        const process = spawn(this.config.ffmpegPath, args);
        let stderr = '';

        process.stderr.on('data', (data: Buffer) => {
          stderr += data.toString();
        });

        process.on('close', (code) => {
          if (code !== 0) {
            logger.warn('Subtitle extraction failed', { code, index: i });
            reject(new Error(`FFmpeg failed with code ${code}`));
            return;
          }

          paths.push(outputPath);
          resolve();
        });

        process.on('error', (error) => {
          reject(error);
        });
      });
    }

    logger.info('Subtitles extracted', { count: paths.length });
    return paths;
  }

  /**
   * Encode to fragmented MP4 intermediate for Shaka Packager consumption.
   * Produces fMP4 with movflags suitable for CMAF packaging.
   */
  async encodeToFmp4(
    inputPath: string,
    outputPath: string,
    resolution: Resolution,
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      segmentDuration: number;
      hardwareAccel?: string;
    },
    onProgress?: (progress: number) => void
  ): Promise<void> {
    logger.info('Encoding to fMP4 intermediate', {
      inputPath,
      outputPath,
      resolution: resolution.label,
    });

    return new Promise((resolve, reject) => {
      const args = this.buildFmp4Args(inputPath, outputPath, resolution, options);

      const proc = spawn(this.config.ffmpegPath, args);
      let stderr = '';

      proc.stderr.on('data', (data: Buffer) => {
        const chunk = data.toString();
        stderr += chunk;

        if (onProgress) {
          const progress = this.parseProgress(chunk);
          if (progress !== null) {
            onProgress(progress);
          }
        }
      });

      proc.on('close', (code) => {
        if (code !== 0) {
          logger.error('fMP4 encoding failed', { code, stderr: stderr.slice(-500) });
          reject(new Error(`FFmpeg fMP4 encoding failed with code ${code}`));
          return;
        }

        logger.info('fMP4 intermediate encoded', { outputPath, resolution: resolution.label });
        resolve();
      });

      proc.on('error', (error) => {
        logger.error('FFmpeg process error', { error: error.message });
        reject(error);
      });
    });
  }

  /**
   * Build transcode arguments
   */
  private buildTranscodeArgs(
    inputPath: string,
    outputPath: string,
    resolution: Resolution,
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      hardwareAccel?: string;
    }
  ): string[] {
    const args: string[] = [];

    // Hardware acceleration
    if (options.hardwareAccel && options.hardwareAccel !== 'none') {
      if (options.hardwareAccel === 'nvenc') {
        args.push('-hwaccel', 'cuda', '-hwaccel_output_format', 'cuda');
      } else if (options.hardwareAccel === 'vaapi') {
        args.push('-hwaccel', 'vaapi', '-hwaccel_device', '/dev/dri/renderD128');
      } else if (options.hardwareAccel === 'qsv') {
        args.push('-hwaccel', 'qsv');
      }
    }

    // Input
    args.push('-i', inputPath);

    // Video encoding
    const videoCodec = this.mapVideoCodec(options.videoCodec, options.hardwareAccel);
    args.push(
      '-c:v', videoCodec,
      '-b:v', resolution.bitrate.toString(),
      '-maxrate', (resolution.bitrate * 1.5).toString(),
      '-bufsize', (resolution.bitrate * 2).toString(),
      '-vf', `scale=${resolution.width}:${resolution.height}`,
      '-r', options.framerate.toString(),
    );

    // Preset (if supported)
    if (videoCodec.includes('264') || videoCodec.includes('265')) {
      args.push('-preset', options.preset);
    }

    // Audio encoding — use per-rung audioBitrate if available, else profile-level
    const audioBitrate = resolution.audioBitrate ?? options.audioBitrate;
    args.push(
      '-c:a', options.audioCodec,
      '-b:a', `${audioBitrate}`,
      '-ar', '48000',
      '-ac', '2',
    );

    // Progress reporting
    args.push('-progress', 'pipe:2');

    // Output
    args.push('-y', outputPath);

    return args;
  }

  /**
   * Build HLS arguments
   */
  private buildHlsArgs(
    inputPath: string,
    outputDir: string,
    resolution: Resolution,
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      segmentDuration: number;
      hardwareAccel?: string;
    }
  ): string[] {
    const args: string[] = [];

    // Hardware acceleration
    if (options.hardwareAccel && options.hardwareAccel !== 'none') {
      if (options.hardwareAccel === 'nvenc') {
        args.push('-hwaccel', 'cuda', '-hwaccel_output_format', 'cuda');
      } else if (options.hardwareAccel === 'vaapi') {
        args.push('-hwaccel', 'vaapi', '-hwaccel_device', '/dev/dri/renderD128');
      } else if (options.hardwareAccel === 'qsv') {
        args.push('-hwaccel', 'qsv');
      }
    }

    // Input
    args.push('-i', inputPath);

    // Video encoding
    const videoCodec = this.mapVideoCodec(options.videoCodec, options.hardwareAccel);
    args.push(
      '-c:v', videoCodec,
      '-b:v', resolution.bitrate.toString(),
      '-maxrate', (resolution.bitrate * 1.5).toString(),
      '-bufsize', (resolution.bitrate * 2).toString(),
      '-vf', `scale=${resolution.width}:${resolution.height}`,
      '-r', options.framerate.toString(),
    );

    // Preset
    if (videoCodec.includes('264') || videoCodec.includes('265')) {
      args.push('-preset', options.preset);
    }

    // Audio encoding — use per-rung audioBitrate if available, else profile-level
    const audioBitrate = resolution.audioBitrate ?? options.audioBitrate;
    args.push(
      '-c:a', options.audioCodec,
      '-b:a', `${audioBitrate}`,
      '-ar', '48000',
      '-ac', '2',
    );

    // HLS options
    args.push(
      '-f', 'hls',
      '-hls_time', options.segmentDuration.toString(),
      '-hls_list_size', '0',
      '-hls_segment_filename', `${outputDir}/segment_%03d.ts`,
      '-hls_flags', 'independent_segments',
    );

    // Progress reporting
    args.push('-progress', 'pipe:2');

    // Output
    args.push(`${outputDir}/playlist.m3u8`);

    return args;
  }

  /**
   * Build fMP4 (fragmented MP4) arguments for Shaka Packager input.
   * Uses -movflags +frag_keyframe+empty_moov+default_base_moof for CMAF compatibility.
   */
  private buildFmp4Args(
    inputPath: string,
    outputPath: string,
    resolution: Resolution,
    options: {
      videoCodec: string;
      audioCodec: string;
      audioBitrate: number;
      preset: string;
      framerate: number;
      segmentDuration: number;
      hardwareAccel?: string;
    }
  ): string[] {
    const args: string[] = [];

    // Hardware acceleration
    if (options.hardwareAccel && options.hardwareAccel !== 'none') {
      if (options.hardwareAccel === 'nvenc') {
        args.push('-hwaccel', 'cuda', '-hwaccel_output_format', 'cuda');
      } else if (options.hardwareAccel === 'vaapi') {
        args.push('-hwaccel', 'vaapi', '-hwaccel_device', '/dev/dri/renderD128');
      } else if (options.hardwareAccel === 'qsv') {
        args.push('-hwaccel', 'qsv');
      }
    }

    // Input
    args.push('-i', inputPath);

    // Video encoding
    const videoCodec = this.mapVideoCodec(options.videoCodec, options.hardwareAccel);
    args.push(
      '-c:v', videoCodec,
      '-b:v', resolution.bitrate.toString(),
      '-maxrate', (resolution.bitrate * 1.5).toString(),
      '-bufsize', (resolution.bitrate * 2).toString(),
      '-vf', `scale=${resolution.width}:${resolution.height}`,
      '-r', options.framerate.toString(),
    );

    // Keyframe interval = segment duration * framerate (for clean segment boundaries)
    const gopSize = Math.round(options.segmentDuration * options.framerate);
    args.push(
      '-g', gopSize.toString(),
      '-keyint_min', gopSize.toString(),
    );

    // Preset
    if (videoCodec.includes('264') || videoCodec.includes('265')) {
      args.push('-preset', options.preset);
    }

    // Audio encoding — use per-rung audioBitrate if available, else profile-level
    const audioBitrate = resolution.audioBitrate ?? options.audioBitrate;
    args.push(
      '-c:a', options.audioCodec,
      '-b:a', `${audioBitrate}`,
      '-ar', '48000',
      '-ac', '2',
    );

    // fMP4 container with CMAF-compatible fragmentation flags
    const fragDuration = options.segmentDuration * 1_000_000; // microseconds
    args.push(
      '-f', 'mp4',
      '-movflags', '+frag_keyframe+empty_moov+default_base_moof',
      '-frag_duration', fragDuration.toString(),
    );

    // Progress reporting
    args.push('-progress', 'pipe:2');

    // Output
    args.push('-y', outputPath);

    return args;
  }

  /**
   * Map video codec to FFmpeg codec name with hardware acceleration
   */
  private mapVideoCodec(codec: string, hardwareAccel?: string): string {
    if (hardwareAccel === 'nvenc') {
      if (codec === 'h264') return 'h264_nvenc';
      if (codec === 'h265') return 'hevc_nvenc';
    } else if (hardwareAccel === 'vaapi') {
      if (codec === 'h264') return 'h264_vaapi';
      if (codec === 'h265') return 'hevc_vaapi';
    } else if (hardwareAccel === 'qsv') {
      if (codec === 'h264') return 'h264_qsv';
      if (codec === 'h265') return 'hevc_qsv';
    }

    // Software encoding
    if (codec === 'h264') return 'libx264';
    if (codec === 'h265') return 'libx265';
    if (codec === 'vp9') return 'libvpx-vp9';
    if (codec === 'av1') return 'libaom-av1';

    return 'libx264'; // default
  }

  /**
   * Parse FFmpeg progress from stderr
   */
  private parseProgress(stderr: string): number | null {
    // Look for time=00:01:23.45 pattern
    const timeMatch = stderr.match(/time=(\d{2}):(\d{2}):(\d{2}\.\d{2})/);
    if (!timeMatch) return null;

    const hours = parseInt(timeMatch[1], 10);
    const minutes = parseInt(timeMatch[2], 10);
    const seconds = parseFloat(timeMatch[3]);

    const totalSeconds = hours * 3600 + minutes * 60 + seconds;

    // Progress is calculated externally based on total duration
    return totalSeconds;
  }
}
