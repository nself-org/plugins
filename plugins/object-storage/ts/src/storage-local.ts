/**
 * Local Filesystem Storage Backend
 */

import * as fs from 'fs/promises';
import * as fsSync from 'fs';
import * as path from 'path';
import * as crypto from 'crypto';
import { createLogger } from '@nself/plugin-utils';
import type {
  StorageBackend,
  PutObjectOptions,
  PutObjectResult,
  GetObjectResult,
  ListObjectsResult,
  CompletedPart,
  CompleteMultipartResult,
  MultipartOptions,
  PresignOptions,
} from './types.js';

const logger = createLogger('object-storage:local');

export class LocalStorageBackend implements StorageBackend {
  provider = 'local' as const;
  private basePath: string;
  private multipartSessions: Map<string, MultipartSession> = new Map();

  constructor(basePath: string) {
    this.basePath = basePath;
  }

  private getBucketPath(bucket: string): string {
    return path.join(this.basePath, bucket);
  }

  private getObjectPath(bucket: string, key: string): string {
    return path.join(this.getBucketPath(bucket), key);
  }

  private getMultipartPath(_bucket: string, _key: string, uploadId: string): string {
    return path.join(this.basePath, '.multipart', uploadId);
  }

  async createBucket(bucket: string): Promise<void> {
    const bucketPath = this.getBucketPath(bucket);
    await fs.mkdir(bucketPath, { recursive: true });
    logger.info('Created local bucket', { bucket, path: bucketPath });
  }

  async deleteBucket(bucket: string): Promise<void> {
    const bucketPath = this.getBucketPath(bucket);
    await fs.rm(bucketPath, { recursive: true, force: true });
    logger.info('Deleted local bucket', { bucket, path: bucketPath });
  }

  async putObject(bucket: string, key: string, data: Buffer, options?: PutObjectOptions): Promise<PutObjectResult> {
    const objectPath = this.getObjectPath(bucket, key);
    const dir = path.dirname(objectPath);

    // Ensure directory exists
    await fs.mkdir(dir, { recursive: true });

    // Write file
    await fs.writeFile(objectPath, data);

    // Calculate checksums
    const etag = crypto.createHash('md5').update(data).digest('hex');
    const checksum_sha256 = options?.checksumSHA256 ?? crypto.createHash('sha256').update(data).digest('hex');

    // Store metadata if provided
    if (options?.metadata || options?.contentType || options?.storageClass) {
      const metadataPath = `${objectPath}.meta`;
      const metadata = {
        contentType: options.contentType,
        storageClass: options.storageClass,
        metadata: options.metadata,
        etag,
        checksum_sha256,
      };
      await fs.writeFile(metadataPath, JSON.stringify(metadata, null, 2));
    }

    logger.debug('Wrote object to local storage', { bucket, key, size: data.length });

    return { etag, checksum_sha256 };
  }

  async getObject(bucket: string, key: string): Promise<GetObjectResult> {
    const objectPath = this.getObjectPath(bucket, key);
    const metadataPath = `${objectPath}.meta`;

    // Read file
    const data = await fs.readFile(objectPath);

    // Read metadata if exists
    let metadata: Record<string, unknown> = {};
    let contentType = 'application/octet-stream';
    let etag = crypto.createHash('md5').update(data).digest('hex');

    try {
      const metaContent = await fs.readFile(metadataPath, 'utf-8');
      metadata = JSON.parse(metaContent);
      contentType = (metadata.contentType as string) ?? contentType;
      etag = (metadata.etag as string) ?? etag;
    } catch {
      // Metadata file doesn't exist, use defaults
    }

    return {
      data,
      contentType,
      contentLength: data.length,
      etag,
      metadata: metadata.metadata as Record<string, string> | undefined,
    };
  }

  async deleteObject(bucket: string, key: string): Promise<void> {
    const objectPath = this.getObjectPath(bucket, key);
    const metadataPath = `${objectPath}.meta`;

    await fs.unlink(objectPath);

    // Remove metadata if exists
    try {
      await fs.unlink(metadataPath);
    } catch {
      // Metadata file doesn't exist
    }

    logger.debug('Deleted object from local storage', { bucket, key });
  }

  async listObjects(bucket: string, prefix?: string, maxKeys = 1000): Promise<ListObjectsResult> {
    const bucketPath = this.getBucketPath(bucket);
    const objects: ListObjectsResult['objects'] = [];

    try {
      const walk = async (dir: string): Promise<void> => {
        if (objects.length >= maxKeys) return;

        const entries = await fs.readdir(dir, { withFileTypes: true });

        for (const entry of entries) {
          if (objects.length >= maxKeys) break;

          const fullPath = path.join(dir, entry.name);
          const relativePath = path.relative(bucketPath, fullPath);

          if (entry.isDirectory()) {
            await walk(fullPath);
          } else if (!entry.name.endsWith('.meta')) {
            // Skip metadata files
            if (!prefix || relativePath.startsWith(prefix)) {
              const stats = await fs.stat(fullPath);
              const data = await fs.readFile(fullPath);
              const etag = crypto.createHash('md5').update(data).digest('hex');

              objects.push({
                key: relativePath,
                size: stats.size,
                etag,
                lastModified: stats.mtime,
              });
            }
          }
        }
      };

      await walk(bucketPath);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') {
        throw error;
      }
      // Bucket doesn't exist, return empty list
    }

    return {
      objects,
      isTruncated: false,
    };
  }

  async presignPutObject(_bucket: string, _key: string, _expiresIn: number, _options?: PresignOptions): Promise<string> {
    // Local storage doesn't support true presigned URLs
    // Return a data URL or throw error requiring direct upload
    throw new Error('Local storage backend does not support presigned PUT URLs. Use direct upload instead.');
  }

  async presignGetObject(_bucket: string, _key: string, _expiresIn: number): Promise<string> {
    // Local storage doesn't support true presigned URLs
    // Return a data URL or throw error requiring direct download
    throw new Error('Local storage backend does not support presigned GET URLs. Use direct download instead.');
  }

  async createMultipartUpload(bucket: string, key: string, options?: MultipartOptions): Promise<string> {
    const uploadId = crypto.randomUUID();
    const uploadPath = this.getMultipartPath(bucket, key, uploadId);

    await fs.mkdir(uploadPath, { recursive: true });

    this.multipartSessions.set(uploadId, {
      bucket,
      key,
      uploadId,
      parts: [],
      options,
    });

    logger.debug('Created multipart upload', { bucket, key, uploadId });

    return uploadId;
  }

  async uploadPart(bucket: string, key: string, uploadId: string, partNumber: number, data: Buffer): Promise<string> {
    const session = this.multipartSessions.get(uploadId);
    if (!session) {
      throw new Error(`Multipart upload ${uploadId} not found`);
    }

    const uploadPath = this.getMultipartPath(bucket, key, uploadId);
    const partPath = path.join(uploadPath, `part-${partNumber}`);

    await fs.writeFile(partPath, data);

    const etag = crypto.createHash('md5').update(data).digest('hex');

    session.parts[partNumber - 1] = { partNumber, etag, size: data.length };

    logger.debug('Uploaded part', { uploadId, partNumber, size: data.length });

    return etag;
  }

  async completeMultipartUpload(
    bucket: string,
    key: string,
    uploadId: string,
    parts: CompletedPart[]
  ): Promise<CompleteMultipartResult> {
    const session = this.multipartSessions.get(uploadId);
    if (!session) {
      throw new Error(`Multipart upload ${uploadId} not found`);
    }

    const uploadPath = this.getMultipartPath(bucket, key, uploadId);
    const objectPath = this.getObjectPath(bucket, key);
    const dir = path.dirname(objectPath);

    await fs.mkdir(dir, { recursive: true });

    // Combine all parts
    const writeStream = fsSync.createWriteStream(objectPath);

    for (const part of parts.sort((a, b) => a.partNumber - b.partNumber)) {
      const partPath = path.join(uploadPath, `part-${part.partNumber}`);
      const partData = await fs.readFile(partPath);
      writeStream.write(partData);
    }

    await new Promise<void>((resolve, reject) => {
      writeStream.end(() => resolve());
      writeStream.on('error', reject);
    });

    // Calculate final checksums
    const finalData = await fs.readFile(objectPath);
    const etag = crypto.createHash('md5').update(finalData).digest('hex');

    // Store metadata
    if (session.options?.metadata || session.options?.contentType || session.options?.storageClass) {
      const metadataPath = `${objectPath}.meta`;
      const metadata = {
        contentType: session.options.contentType,
        storageClass: session.options.storageClass,
        metadata: session.options.metadata,
        etag,
      };
      await fs.writeFile(metadataPath, JSON.stringify(metadata, null, 2));
    }

    // Cleanup multipart session
    await fs.rm(uploadPath, { recursive: true, force: true });
    this.multipartSessions.delete(uploadId);

    logger.info('Completed multipart upload', { bucket, key, uploadId, parts: parts.length });

    return {
      etag,
      location: objectPath,
    };
  }

  async abortMultipartUpload(bucket: string, key: string, uploadId: string): Promise<void> {
    const uploadPath = this.getMultipartPath(bucket, key, uploadId);

    await fs.rm(uploadPath, { recursive: true, force: true });
    this.multipartSessions.delete(uploadId);

    logger.debug('Aborted multipart upload', { bucket, key, uploadId });
  }
}

interface MultipartSession {
  bucket: string;
  key: string;
  uploadId: string;
  parts: Array<{ partNumber: number; etag: string; size: number }>;
  options?: MultipartOptions;
}
