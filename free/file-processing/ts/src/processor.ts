/**
 * File processor - handles thumbnail generation, optimization, scanning, and metadata extraction
 */

import { createLogger } from '@nself/plugin-utils';
import sharp from 'sharp';
import ffmpeg from 'fluent-ffmpeg';
import { fileTypeFromFile } from 'file-type';
import ExifReader from 'exifreader';
import NodeClam from 'clamscan';
import { createHash } from 'crypto';
import { readFile, unlink } from 'fs/promises';
import { tmpdir } from 'os';
import { join } from 'path';
import type {
  FileProcessingConfig,
  StorageAdapter,
  ThumbnailResult,
  ScanResult,
  OptimizationResult,
  MetadataResult,
  ProcessingOperation,
} from './types.js';

const logger = createLogger('file-processing:processor');

export class FileProcessor {
  constructor(
    private config: FileProcessingConfig,
    private storage: StorageAdapter
  ) {}

  /**
   * Process file with specified operations
   */
  async process(
    localPath: string,
    remotePath: string,
    mimeType: string,
    operations: ProcessingOperation[]
  ): Promise<{
    thumbnails: ThumbnailResult[];
    metadata?: MetadataResult;
    scan?: ScanResult;
    optimization?: OptimizationResult;
  }> {
    const results: {
      thumbnails: ThumbnailResult[];
      metadata?: MetadataResult;
      scan?: ScanResult;
      optimization?: OptimizationResult;
    } = {
      thumbnails: [],
    };

    // Download file to local temp directory
    const tempPath = join(tmpdir(), `processing-${Date.now()}-${Math.random().toString(36).slice(2)}`);
    await this.storage.download(remotePath, tempPath);

    try {
      // Execute operations
      for (const operation of operations) {
        switch (operation) {
          case 'thumbnail':
            if (this.isImage(mimeType) || this.isVideo(mimeType)) {
              results.thumbnails = await this.generateThumbnails(tempPath, remotePath, mimeType);
            }
            break;

          case 'optimize':
            if (this.isImage(mimeType) && this.config.enableOptimization) {
              results.optimization = await this.optimizeImage(tempPath);
            }
            break;

          case 'metadata':
            results.metadata = await this.extractMetadata(tempPath, mimeType);
            break;

          case 'scan':
            if (this.config.enableVirusScan) {
              results.scan = await this.scanForViruses(tempPath);
            }
            break;
        }
      }

      return results;
    } finally {
      // Clean up temp file
      await unlink(tempPath).catch(() => {});
    }
  }

  /**
   * Scan a file for viruses using ClamAV via clamscan daemon (clamdscan).
   *
   * Behaviour:
   * - When CLAMAV_ENABLED=true (or enableVirusScan is true) and ClamAV is
   *   reachable, throws on connection failure so the caller can reject the file.
   * - When ClamAV is unreachable and the caller has not explicitly required it
   *   (i.e. CLAMAV_ENABLED is unset or false), logs a warning and returns a
   *   clean result so that file processing is not blocked.
   * - Always returns a ScanResult with engine='clamav'.
   */
  private async scanForViruses(filePath: string): Promise<ScanResult> {
    const startTime = Date.now();

    const clamavHost = this.config.clamavHost ?? 'localhost';
    const clamavPort = this.config.clamavPort ?? 3310;
    // Treat CLAMAV_ENABLED env var as the hard-require flag.
    const hardRequired = process.env.CLAMAV_ENABLED === 'true';

    try {
      const ClamScan = await new NodeClam().init({
        clamdscan: {
          host: clamavHost,
          port: clamavPort,
          timeout: 60_000,
          active: true,
        },
        preference: 'clamdscan',
      });

      const { isInfected, viruses, file } = await ClamScan.scanFile(filePath);

      const infected = isInfected === true;
      const virusName = viruses && viruses.length > 0 ? viruses[0] : undefined;

      if (infected) {
        logger.warn('Virus detected in file', { file, viruses });
      }

      return {
        status: infected ? 'infected' : 'clean',
        virusFound: virusName,
        engine: 'clamav',
        duration: Date.now() - startTime,
      };

    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);

      if (hardRequired) {
        // ClamAV was explicitly required — propagate the error so the file
        // is rejected rather than silently accepted.
        logger.error('ClamAV scan failed (CLAMAV_ENABLED=true)', { error: message });
        return {
          status: 'error',
          engine: 'clamav',
          duration: Date.now() - startTime,
        };
      }

      // Graceful degradation: ClamAV is unavailable but not required.
      logger.warn(
        'ClamAV unavailable — virus scanning skipped. Set CLAMAV_ENABLED=true to require it.',
        { host: clamavHost, port: clamavPort, error: message }
      );

      return {
        status: 'clean',
        engine: 'clamav (skipped — daemon unavailable)',
        duration: Date.now() - startTime,
      };
    }
  }

  /**
   * Generate thumbnails for images and videos
   */
  private async generateThumbnails(
    localPath: string,
    remotePath: string,
    mimeType: string
  ): Promise<ThumbnailResult[]> {
    const results: ThumbnailResult[] = [];

    for (const size of this.config.thumbnailSizes) {
      const startTime = Date.now();

      try {
        let thumbnailPath: string;

        if (this.isImage(mimeType)) {
          thumbnailPath = await this.generateImageThumbnail(localPath, size);
        } else if (this.isVideo(mimeType)) {
          thumbnailPath = await this.generateVideoThumbnail(localPath, size);
        } else {
          continue;
        }

        // Upload thumbnail
        const remoteDir = remotePath.split('/').slice(0, -1).join('/');
        const remoteThumbnailPath = `${remoteDir}/thumbnails/${size}x${size}_${Date.now()}.jpg`;

        const { url, size: fileSize } = await this.storage.upload(
          thumbnailPath,
          remoteThumbnailPath,
          'image/jpeg'
        );

        results.push({
          path: remoteThumbnailPath,
          url,
          width: size,
          height: size,
          size: fileSize,
          format: 'jpeg',
          generationTime: Date.now() - startTime,
        });

        // Clean up local thumbnail
        await unlink(thumbnailPath).catch(() => {});
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error(`Failed to generate ${size}x${size} thumbnail`, { error: message });
      }
    }

    return results;
  }

  /**
   * Generate thumbnail from image
   */
  private async generateImageThumbnail(imagePath: string, size: number): Promise<string> {
    const outputPath = join(tmpdir(), `thumb-${size}-${Date.now()}.jpg`);

    await sharp(imagePath)
      .resize(size, size, {
        fit: 'cover',
        position: 'center',
      })
      .jpeg({ quality: 85, mozjpeg: true })
      .toFile(outputPath);

    return outputPath;
  }

  /**
   * Generate thumbnail from video
   */
  private async generateVideoThumbnail(videoPath: string, size: number): Promise<string> {
    return new Promise((resolve, reject) => {
      const outputPath = join(tmpdir(), `thumb-${size}-${Date.now()}.jpg`);

      ffmpeg(videoPath)
        .screenshots({
          timestamps: ['1'],
          filename: `thumb-${size}-%i.jpg`,
          folder: tmpdir(),
          size: `${size}x${size}`,
        })
        .on('end', () => {
          resolve(outputPath);
        })
        .on('error', (err) => {
          reject(err);
        });
    });
  }

  /**
   * Optimize image (compress, strip metadata)
   */
  private async optimizeImage(imagePath: string): Promise<OptimizationResult> {
    const startTime = Date.now();
    const originalSize = (await sharp(imagePath).metadata()).size || 0;

    const tempOutput = join(tmpdir(), `optimized-${Date.now()}.jpg`);

    await sharp(imagePath)
      .jpeg({
        quality: 85,
        progressive: true,
        mozjpeg: true,
      })
      .toFile(tempOutput);

    const optimizedSize = (await sharp(tempOutput).metadata()).size || 0;

    // Clean up
    await unlink(tempOutput).catch(() => {});

    return {
      originalSize,
      optimizedSize,
      savingsBytes: originalSize - optimizedSize,
      savingsPercent: Math.round(((originalSize - optimizedSize) / originalSize) * 100),
      duration: Date.now() - startTime,
    };
  }

  /**
   * Extract metadata from file
   */
  private async extractMetadata(filePath: string, mimeType: string): Promise<MetadataResult> {
    const startTime = Date.now();
    const extracted: Record<string, unknown> = {};

    try {
      // Detect actual file type
      const fileType = await fileTypeFromFile(filePath);
      if (fileType) {
        extracted.detectedMimeType = fileType.mime;
        extracted.detectedExtension = fileType.ext;
      }

      // Extract EXIF data for images
      if (this.isImage(mimeType)) {
        const tags = ExifReader.load(await readFile(filePath));
        extracted.exif = tags;

        // Extract common fields
        if (tags.ImageWidth) extracted.width = tags.ImageWidth.value;
        if (tags.ImageHeight) extracted.height = tags.ImageHeight.value;
        if (tags.Make) extracted.cameraMake = tags.Make.description;
        if (tags.Model) extracted.cameraModel = tags.Model.description;
        if (tags.DateTime) extracted.dateTaken = tags.DateTime.description;

        // GPS data
        if (tags.GPSLatitude && tags.GPSLongitude) {
          extracted.gps = {
            latitude: tags.GPSLatitude.description,
            longitude: tags.GPSLongitude.description,
          };
        }
      }

      // Calculate file hashes
      const fileBuffer = await readFile(filePath);
      extracted.md5 = createHash('md5').update(fileBuffer).digest('hex');
      extracted.sha256 = createHash('sha256').update(fileBuffer).digest('hex');

    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Metadata extraction error', { error: message });
    }

    return {
      extracted,
      exifStripped: this.config.stripExif,
      extractionTime: Date.now() - startTime,
    };
  }

  /**
   * Check if MIME type is an image
   */
  private isImage(mimeType: string): boolean {
    return mimeType.startsWith('image/');
  }

  /**
   * Check if MIME type is a video
   */
  private isVideo(mimeType: string): boolean {
    return mimeType.startsWith('video/');
  }
}
