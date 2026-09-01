/**
 * Object Storage Upload (UPGRADE 1e)
 * Uploads packaged outputs to the object-storage plugin via its API
 */

import { promises as fs } from 'fs';
import { join, extname, relative } from 'path';
import { createLogger } from '@nself/plugin-utils';
import type { Config } from './config.js';
import type { MediaProcessingDatabase } from './database.js';
import type { UploadRecord } from './types.js';

const logger = createLogger('media-processing:upload');

/** Content type mapping by file extension */
const CONTENT_TYPE_MAP: Record<string, string> = {
  '.m3u8': 'application/vnd.apple.mpegurl',
  '.mpd': 'application/dash+xml',
  '.m4s': 'video/mp4',
  '.mp4': 'video/mp4',
  '.vtt': 'text/vtt',
  '.srt': 'text/plain',
  '.ts': 'video/mp2t',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.json': 'application/json',
};

export class StorageUploader {
  constructor(
    private config: Config,
    private db: MediaProcessingDatabase
  ) {}

  /**
   * Upload all outputs from a job to object storage
   */
  async uploadJobOutputs(
    jobId: string,
    outputDir: string,
    contentId?: string
  ): Promise<UploadRecord[]> {
    logger.info('Uploading job outputs', { jobId, outputDir, contentId });

    // Determine version
    const version = contentId
      ? await this.db.getNextUploadVersion(contentId)
      : 1;

    // Collect all files in the output directory recursively
    const files = await this.collectFiles(outputDir);

    if (files.length === 0) {
      logger.warn('No files found in output directory', { outputDir });
      return [];
    }

    logger.info('Found files to upload', { count: files.length, jobId });

    const uploads: UploadRecord[] = [];

    for (const filePath of files) {
      try {
        const relativePath = relative(outputDir, filePath);
        const storagePath = contentId
          ? `content/${contentId}/v${version}/${relativePath}`
          : `jobs/${jobId}/${relativePath}`;

        const contentType = this.getContentType(filePath);
        const fileStat = await fs.stat(filePath);

        // Upload to object-storage plugin
        const storageUrl = await this.uploadFile(filePath, storagePath, contentType);

        // Record in database
        const record = await this.db.createUploadRecord({
          job_id: jobId,
          file_path: filePath,
          storage_path: storagePath,
          storage_url: storageUrl,
          content_type: contentType,
          file_size_bytes: fileStat.size,
          content_id: contentId ?? null,
          version,
        });

        uploads.push(record);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to upload file', { filePath, error: message });
        // Continue uploading remaining files
      }
    }

    logger.info('Upload complete', {
      jobId,
      totalFiles: files.length,
      uploaded: uploads.length,
      failed: files.length - uploads.length,
    });

    return uploads;
  }

  /**
   * Upload a single file to the object-storage plugin
   */
  private async uploadFile(
    filePath: string,
    storagePath: string,
    contentType: string
  ): Promise<string | null> {
    const fileData = await fs.readFile(filePath);

    const url = `${this.config.objectStorageUrl}/v1/objects/${encodeURIComponent(storagePath)}`;

    const response = await fetch(url, {
      method: 'PUT',
      headers: {
        'Content-Type': contentType,
        'Content-Length': fileData.length.toString(),
      },
      body: fileData,
      signal: AbortSignal.timeout(60000), // 60s timeout for large files
    });

    if (!response.ok) {
      const body = await response.text().catch(() => 'no body');
      throw new Error(`Upload failed (${response.status}): ${body}`);
    }

    const result = await response.json().catch(() => null) as { data?: { url?: string } } | null;
    return result?.data?.url ?? null;
  }

  /**
   * Get content type for a file based on extension
   */
  private getContentType(filePath: string): string {
    const ext = extname(filePath).toLowerCase();
    return CONTENT_TYPE_MAP[ext] ?? 'application/octet-stream';
  }

  /**
   * Recursively collect all files in a directory
   */
  private async collectFiles(dirPath: string): Promise<string[]> {
    const files: string[] = [];

    try {
      const entries = await fs.readdir(dirPath, { withFileTypes: true });

      for (const entry of entries) {
        const fullPath = join(dirPath, entry.name);
        if (entry.isDirectory()) {
          const subFiles = await this.collectFiles(fullPath);
          files.push(...subFiles);
        } else if (entry.isFile()) {
          files.push(fullPath);
        }
      }
    } catch (error) {
      logger.warn('Failed to read directory', { dirPath, error });
    }

    return files;
  }
}
