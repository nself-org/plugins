/**
 * S3-Compatible Storage Backend
 * Supports AWS S3, MinIO, Cloudflare R2, Google Cloud Storage, Backblaze B2
 */

import {
  S3Client,
  PutObjectCommand,
  GetObjectCommand,
  DeleteObjectCommand,
  ListObjectsV2Command,
  CreateBucketCommand,
  DeleteBucketCommand,
  CreateMultipartUploadCommand,
  UploadPartCommand,
  CompleteMultipartUploadCommand,
  AbortMultipartUploadCommand,
  type StorageClass as S3StorageClass,
} from '@aws-sdk/client-s3';
import { getSignedUrl } from '@aws-sdk/s3-request-presigner';
import { createLogger } from '@nself/plugin-utils';
import type {
  StorageBackend,
  StorageProvider,
  PutObjectOptions,
  PutObjectResult,
  GetObjectResult,
  ListObjectsResult,
  CompletedPart,
  CompleteMultipartResult,
  MultipartOptions,
  PresignOptions,
  ProviderConfig,
} from './types.js';

const logger = createLogger('object-storage:s3');

export class S3StorageBackend implements StorageBackend {
  provider: StorageProvider;
  private client: S3Client;
  private bucketPrefix: string;

  constructor(provider: StorageProvider, config: ProviderConfig) {
    this.provider = provider;
    this.bucketPrefix = config.bucketPrefix ?? '';

    const clientConfig: ConstructorParameters<typeof S3Client>[0] = {
      region: config.region ?? 'us-east-1',
      credentials: config.accessKeyId && config.secretAccessKey
        ? {
            accessKeyId: config.accessKeyId,
            secretAccessKey: config.secretAccessKey,
          }
        : undefined,
    };

    // Set endpoint for non-AWS providers
    if (config.endpoint) {
      clientConfig.endpoint = config.endpoint;
    }

    // Force path-style for MinIO and some other providers
    if (config.forcePathStyle || provider === 'minio') {
      clientConfig.forcePathStyle = true;
    }

    this.client = new S3Client(clientConfig);
    logger.info('Initialized S3-compatible storage', { provider, endpoint: config.endpoint, region: config.region });
  }

  private getBucketName(bucket: string): string {
    return this.bucketPrefix ? `${this.bucketPrefix}${bucket}` : bucket;
  }

  private mapStorageClass(storageClass?: string): S3StorageClass | undefined {
    if (!storageClass) return undefined;

    const mapping: Record<string, S3StorageClass> = {
      standard: 'STANDARD',
      reduced_redundancy: 'REDUCED_REDUNDANCY',
      glacier: 'GLACIER',
      deep_archive: 'DEEP_ARCHIVE',
    };

    return mapping[storageClass];
  }

  async createBucket(bucket: string): Promise<void> {
    const bucketName = this.getBucketName(bucket);

    await this.client.send(
      new CreateBucketCommand({
        Bucket: bucketName,
      })
    );

    logger.info('Created S3 bucket', { provider: this.provider, bucket: bucketName });
  }

  async deleteBucket(bucket: string): Promise<void> {
    const bucketName = this.getBucketName(bucket);

    await this.client.send(
      new DeleteBucketCommand({
        Bucket: bucketName,
      })
    );

    logger.info('Deleted S3 bucket', { provider: this.provider, bucket: bucketName });
  }

  async putObject(bucket: string, key: string, data: Buffer, options?: PutObjectOptions): Promise<PutObjectResult> {
    const bucketName = this.getBucketName(bucket);

    const command = new PutObjectCommand({
      Bucket: bucketName,
      Key: key,
      Body: data,
      ContentType: options?.contentType,
      Metadata: options?.metadata,
      StorageClass: this.mapStorageClass(options?.storageClass),
      ChecksumSHA256: options?.checksumSHA256,
    });

    const result = await this.client.send(command);

    logger.debug('Put object to S3', { provider: this.provider, bucket: bucketName, key, size: data.length });

    return {
      etag: result.ETag ?? '',
      checksum_sha256: result.ChecksumSHA256,
    };
  }

  async getObject(bucket: string, key: string): Promise<GetObjectResult> {
    const bucketName = this.getBucketName(bucket);

    const command = new GetObjectCommand({
      Bucket: bucketName,
      Key: key,
    });

    const result = await this.client.send(command);

    if (!result.Body) {
      throw new Error('Object body is empty');
    }

    const data = Buffer.from(await result.Body.transformToByteArray());

    return {
      data,
      contentType: result.ContentType ?? 'application/octet-stream',
      contentLength: result.ContentLength ?? data.length,
      etag: result.ETag ?? '',
      metadata: result.Metadata,
    };
  }

  async deleteObject(bucket: string, key: string): Promise<void> {
    const bucketName = this.getBucketName(bucket);

    await this.client.send(
      new DeleteObjectCommand({
        Bucket: bucketName,
        Key: key,
      })
    );

    logger.debug('Deleted object from S3', { provider: this.provider, bucket: bucketName, key });
  }

  async listObjects(bucket: string, prefix?: string, maxKeys = 1000): Promise<ListObjectsResult> {
    const bucketName = this.getBucketName(bucket);

    const command = new ListObjectsV2Command({
      Bucket: bucketName,
      Prefix: prefix,
      MaxKeys: maxKeys,
    });

    const result = await this.client.send(command);

    const objects = (result.Contents ?? []).map(obj => ({
      key: obj.Key ?? '',
      size: obj.Size ?? 0,
      etag: obj.ETag ?? '',
      lastModified: obj.LastModified ?? new Date(),
    }));

    return {
      objects,
      isTruncated: result.IsTruncated ?? false,
      nextToken: result.NextContinuationToken,
    };
  }

  async presignPutObject(bucket: string, key: string, expiresIn: number, options?: PresignOptions): Promise<string> {
    const bucketName = this.getBucketName(bucket);

    const command = new PutObjectCommand({
      Bucket: bucketName,
      Key: key,
      ContentType: options?.contentType,
      Metadata: options?.metadata,
    });

    const url = await getSignedUrl(this.client, command, { expiresIn });

    logger.debug('Generated presigned PUT URL', { provider: this.provider, bucket: bucketName, key, expiresIn });

    return url;
  }

  async presignGetObject(bucket: string, key: string, expiresIn: number): Promise<string> {
    const bucketName = this.getBucketName(bucket);

    const command = new GetObjectCommand({
      Bucket: bucketName,
      Key: key,
    });

    const url = await getSignedUrl(this.client, command, { expiresIn });

    logger.debug('Generated presigned GET URL', { provider: this.provider, bucket: bucketName, key, expiresIn });

    return url;
  }

  async createMultipartUpload(bucket: string, key: string, options?: MultipartOptions): Promise<string> {
    const bucketName = this.getBucketName(bucket);

    const command = new CreateMultipartUploadCommand({
      Bucket: bucketName,
      Key: key,
      ContentType: options?.contentType,
      Metadata: options?.metadata,
      StorageClass: this.mapStorageClass(options?.storageClass),
    });

    const result = await this.client.send(command);

    if (!result.UploadId) {
      throw new Error('Failed to create multipart upload: no upload ID returned');
    }

    logger.debug('Created multipart upload', { provider: this.provider, bucket: bucketName, key, uploadId: result.UploadId });

    return result.UploadId;
  }

  async uploadPart(bucket: string, key: string, uploadId: string, partNumber: number, data: Buffer): Promise<string> {
    const bucketName = this.getBucketName(bucket);

    const command = new UploadPartCommand({
      Bucket: bucketName,
      Key: key,
      UploadId: uploadId,
      PartNumber: partNumber,
      Body: data,
    });

    const result = await this.client.send(command);

    if (!result.ETag) {
      throw new Error(`Failed to upload part ${partNumber}: no ETag returned`);
    }

    logger.debug('Uploaded part', { provider: this.provider, uploadId, partNumber, size: data.length });

    return result.ETag;
  }

  async completeMultipartUpload(
    bucket: string,
    key: string,
    uploadId: string,
    parts: CompletedPart[]
  ): Promise<CompleteMultipartResult> {
    const bucketName = this.getBucketName(bucket);

    const command = new CompleteMultipartUploadCommand({
      Bucket: bucketName,
      Key: key,
      UploadId: uploadId,
      MultipartUpload: {
        Parts: parts.map(part => ({
          PartNumber: part.partNumber,
          ETag: part.etag,
        })),
      },
    });

    const result = await this.client.send(command);

    logger.info('Completed multipart upload', {
      provider: this.provider,
      bucket: bucketName,
      key,
      uploadId,
      parts: parts.length,
    });

    return {
      etag: result.ETag ?? '',
      location: result.Location ?? `${bucketName}/${key}`,
    };
  }

  async abortMultipartUpload(bucket: string, key: string, uploadId: string): Promise<void> {
    const bucketName = this.getBucketName(bucket);

    await this.client.send(
      new AbortMultipartUploadCommand({
        Bucket: bucketName,
        Key: key,
        UploadId: uploadId,
      })
    );

    logger.debug('Aborted multipart upload', { provider: this.provider, bucket: bucketName, key, uploadId });
  }
}
