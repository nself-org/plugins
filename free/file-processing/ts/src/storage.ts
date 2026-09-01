/**
 * Storage adapters for multiple cloud providers
 */

import { createLogger } from '@nself/plugin-utils';
import { S3Client, PutObjectCommand, GetObjectCommand, DeleteObjectCommand, HeadObjectCommand } from '@aws-sdk/client-s3';
import { Upload } from '@aws-sdk/lib-storage';
import { BlobServiceClient } from '@azure/storage-blob';
import { Storage as GoogleStorage } from '@google-cloud/storage';
import { createReadStream, createWriteStream } from 'fs';
import { pipeline } from 'stream/promises';
import type { StorageAdapter, StorageProvider, FileProcessingConfig } from './types.js';

const logger = createLogger('file-processing:storage');

/**
 * Create storage adapter based on provider
 */
export function createStorageAdapter(config: FileProcessingConfig): StorageAdapter {
  switch (config.storageProvider) {
    case 'minio':
    case 's3':
    case 'r2':
    case 'b2':
      return new S3StorageAdapter(config);
    case 'gcs':
      return new GCSStorageAdapter(config);
    case 'azure':
      return new AzureStorageAdapter(config);
    default:
      throw new Error(`Unsupported storage provider: ${config.storageProvider}`);
  }
}

/**
 * S3-compatible storage adapter (MinIO, AWS S3, Cloudflare R2, Backblaze B2)
 */
class S3StorageAdapter implements StorageAdapter {
  provider: StorageProvider;
  private client: S3Client;
  private bucket: string;

  constructor(config: FileProcessingConfig) {
    this.provider = config.storageProvider;
    this.bucket = config.storageBucket;

    this.client = new S3Client({
      region: config.storageRegion || 'us-east-1',
      endpoint: config.storageEndpoint,
      credentials: {
        accessKeyId: config.storageAccessKey!,
        secretAccessKey: config.storageSecretKey!,
      },
      forcePathStyle: config.storageProvider === 'minio',
    });
  }

  async upload(localPath: string, remotePath: string, mimeType: string, bucket?: string): Promise<{ url: string; size: number }> {
    const targetBucket = bucket || this.bucket;
    const fileStream = createReadStream(localPath);

    const upload = new Upload({
      client: this.client,
      params: {
        Bucket: targetBucket,
        Key: remotePath,
        Body: fileStream,
        ContentType: mimeType,
      },
    });

    await upload.done();

    // Get file size
    const metadata = await this.getMetadata(remotePath, targetBucket);

    return {
      url: `https://${targetBucket}.s3.amazonaws.com/${remotePath}`,
      size: metadata.size,
    };
  }

  async download(remotePath: string, localPath: string, bucket?: string): Promise<void> {
    const targetBucket = bucket || this.bucket;

    const command = new GetObjectCommand({
      Bucket: targetBucket,
      Key: remotePath,
    });

    const response = await this.client.send(command);

    if (!response.Body) {
      throw new Error('No data received from storage');
    }

    const fileStream = createWriteStream(localPath);
    await pipeline(response.Body as NodeJS.ReadableStream, fileStream);
  }

  async getTemporaryUrl(remotePath: string, expiresIn: number, bucket?: string): Promise<string> {
    // For now, return public URL (implement presigned URLs if needed)
    const targetBucket = bucket || this.bucket;
    return `https://${targetBucket}.s3.amazonaws.com/${remotePath}`;
  }

  async delete(remotePath: string, bucket?: string): Promise<void> {
    const targetBucket = bucket || this.bucket;

    const command = new DeleteObjectCommand({
      Bucket: targetBucket,
      Key: remotePath,
    });

    await this.client.send(command);
  }

  async exists(remotePath: string, bucket?: string): Promise<boolean> {
    const targetBucket = bucket || this.bucket;

    try {
      const command = new HeadObjectCommand({
        Bucket: targetBucket,
        Key: remotePath,
      });

      await this.client.send(command);
      return true;
    } catch {
      return false;
    }
  }

  async getMetadata(remotePath: string, bucket?: string): Promise<{ size: number; contentType: string; lastModified: Date }> {
    const targetBucket = bucket || this.bucket;

    const command = new HeadObjectCommand({
      Bucket: targetBucket,
      Key: remotePath,
    });

    const response = await this.client.send(command);

    return {
      size: response.ContentLength || 0,
      contentType: response.ContentType || 'application/octet-stream',
      lastModified: response.LastModified || new Date(),
    };
  }
}

/**
 * Google Cloud Storage adapter
 */
class GCSStorageAdapter implements StorageAdapter {
  provider: StorageProvider = 'gcs';
  private storage: GoogleStorage;
  private bucket: string;

  constructor(config: FileProcessingConfig) {
    this.bucket = config.storageBucket;
    this.storage = new GoogleStorage({
      keyFilename: config.googleCredentials,
    });
  }

  async upload(localPath: string, remotePath: string, mimeType: string, bucket?: string): Promise<{ url: string; size: number }> {
    const targetBucket = bucket || this.bucket;
    const bucketObj = this.storage.bucket(targetBucket);
    const file = bucketObj.file(remotePath);

    await bucketObj.upload(localPath, {
      destination: remotePath,
      metadata: {
        contentType: mimeType,
      },
    });

    const [metadata] = await file.getMetadata();

    return {
      url: `https://storage.googleapis.com/${targetBucket}/${remotePath}`,
      size: parseInt(String(metadata.size || '0'), 10),
    };
  }

  async download(remotePath: string, localPath: string, bucket?: string): Promise<void> {
    const targetBucket = bucket || this.bucket;
    const bucketObj = this.storage.bucket(targetBucket);
    const file = bucketObj.file(remotePath);

    await file.download({ destination: localPath });
  }

  async getTemporaryUrl(remotePath: string, expiresIn: number, bucket?: string): Promise<string> {
    const targetBucket = bucket || this.bucket;
    const bucketObj = this.storage.bucket(targetBucket);
    const file = bucketObj.file(remotePath);

    const [url] = await file.getSignedUrl({
      action: 'read',
      expires: Date.now() + expiresIn * 1000,
    });

    return url;
  }

  async delete(remotePath: string, bucket?: string): Promise<void> {
    const targetBucket = bucket || this.bucket;
    const bucketObj = this.storage.bucket(targetBucket);
    const file = bucketObj.file(remotePath);

    await file.delete();
  }

  async exists(remotePath: string, bucket?: string): Promise<boolean> {
    const targetBucket = bucket || this.bucket;
    const bucketObj = this.storage.bucket(targetBucket);
    const file = bucketObj.file(remotePath);

    const [exists] = await file.exists();
    return exists;
  }

  async getMetadata(remotePath: string, bucket?: string): Promise<{ size: number; contentType: string; lastModified: Date }> {
    const targetBucket = bucket || this.bucket;
    const bucketObj = this.storage.bucket(targetBucket);
    const file = bucketObj.file(remotePath);

    const [metadata] = await file.getMetadata();

    return {
      size: parseInt(String(metadata.size || '0'), 10),
      contentType: metadata.contentType || 'application/octet-stream',
      lastModified: new Date(metadata.updated || Date.now()),
    };
  }
}

/**
 * Azure Blob Storage adapter
 */
class AzureStorageAdapter implements StorageAdapter {
  provider: StorageProvider = 'azure';
  private client: BlobServiceClient;
  private bucket: string;

  constructor(config: FileProcessingConfig) {
    this.bucket = config.storageBucket;
    this.client = BlobServiceClient.fromConnectionString(config.azureConnectionString!);
  }

  async upload(localPath: string, remotePath: string, mimeType: string, bucket?: string): Promise<{ url: string; size: number }> {
    const targetBucket = bucket || this.bucket;
    const containerClient = this.client.getContainerClient(targetBucket);
    const blockBlobClient = containerClient.getBlockBlobClient(remotePath);

    await blockBlobClient.uploadFile(localPath, {
      blobHTTPHeaders: { blobContentType: mimeType },
    });

    const properties = await blockBlobClient.getProperties();

    return {
      url: blockBlobClient.url,
      size: properties.contentLength || 0,
    };
  }

  async download(remotePath: string, localPath: string, bucket?: string): Promise<void> {
    const targetBucket = bucket || this.bucket;
    const containerClient = this.client.getContainerClient(targetBucket);
    const blockBlobClient = containerClient.getBlockBlobClient(remotePath);

    await blockBlobClient.downloadToFile(localPath);
  }

  async getTemporaryUrl(remotePath: string, expiresIn: number, bucket?: string): Promise<string> {
    const targetBucket = bucket || this.bucket;
    const containerClient = this.client.getContainerClient(targetBucket);
    const blockBlobClient = containerClient.getBlockBlobClient(remotePath);

    // Generate SAS token (simplified - implement full SAS generation if needed)
    return blockBlobClient.url;
  }

  async delete(remotePath: string, bucket?: string): Promise<void> {
    const targetBucket = bucket || this.bucket;
    const containerClient = this.client.getContainerClient(targetBucket);
    const blockBlobClient = containerClient.getBlockBlobClient(remotePath);

    await blockBlobClient.delete();
  }

  async exists(remotePath: string, bucket?: string): Promise<boolean> {
    const targetBucket = bucket || this.bucket;
    const containerClient = this.client.getContainerClient(targetBucket);
    const blockBlobClient = containerClient.getBlockBlobClient(remotePath);

    return await blockBlobClient.exists();
  }

  async getMetadata(remotePath: string, bucket?: string): Promise<{ size: number; contentType: string; lastModified: Date }> {
    const targetBucket = bucket || this.bucket;
    const containerClient = this.client.getContainerClient(targetBucket);
    const blockBlobClient = containerClient.getBlockBlobClient(remotePath);

    const properties = await blockBlobClient.getProperties();

    return {
      size: properties.contentLength || 0,
      contentType: properties.contentType || 'application/octet-stream',
      lastModified: properties.lastModified || new Date(),
    };
  }
}
