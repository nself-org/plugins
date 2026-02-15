/**
 * Azure Blob Storage Backend
 * Supports Azure Blob Storage with full compatibility
 */

import {
  BlobServiceClient,
  StorageSharedKeyCredential,
  ContainerClient,
  BlockBlobClient,
  BlobSASPermissions,
  generateBlobSASQueryParameters,
} from '@azure/storage-blob';
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

const logger = createLogger('object-storage:azure');

interface AzureProviderConfig extends ProviderConfig {
  accountName?: string;
  accountKey?: string;
  containerPrefix?: string;
}

export class AzureStorageBackend implements StorageBackend {
  provider: StorageProvider = 'azure';
  private client: BlobServiceClient;
  private accountName: string;
  private accountKey: string;
  private containerPrefix: string;
  private credential: StorageSharedKeyCredential;

  constructor(config: AzureProviderConfig) {
    this.containerPrefix = config.containerPrefix ?? '';

    // Azure requires account name and key
    if (!config.accountName || !config.accountKey) {
      throw new Error('Azure Blob Storage requires accountName and accountKey in provider config');
    }

    this.accountName = config.accountName;
    this.accountKey = config.accountKey;

    // Create credential
    this.credential = new StorageSharedKeyCredential(this.accountName, this.accountKey);

    // Create BlobServiceClient
    const blobServiceUrl = `https://${this.accountName}.blob.core.windows.net`;
    this.client = new BlobServiceClient(blobServiceUrl, this.credential);

    logger.info('Initialized Azure Blob Storage', { accountName: this.accountName });
  }

  private getContainerName(bucket: string): string {
    return this.containerPrefix ? `${this.containerPrefix}${bucket}` : bucket;
  }

  private getContainerClient(bucket: string): ContainerClient {
    const containerName = this.getContainerName(bucket);
    return this.client.getContainerClient(containerName);
  }

  private getBlobClient(bucket: string, key: string): BlockBlobClient {
    const containerClient = this.getContainerClient(bucket);
    return containerClient.getBlockBlobClient(key);
  }

  private mapStorageClass(storageClass?: string): string | undefined {
    if (!storageClass) return undefined;

    const mapping: Record<string, string> = {
      standard: 'Hot',
      reduced_redundancy: 'Cool',
      glacier: 'Archive',
      deep_archive: 'Archive',
    };

    return mapping[storageClass];
  }

  async createBucket(bucket: string): Promise<void> {
    const containerName = this.getContainerName(bucket);
    const containerClient = this.client.getContainerClient(containerName);

    await containerClient.create();

    logger.info('Created Azure container', { container: containerName });
  }

  async deleteBucket(bucket: string): Promise<void> {
    const containerName = this.getContainerName(bucket);
    const containerClient = this.client.getContainerClient(containerName);

    await containerClient.delete();

    logger.info('Deleted Azure container', { container: containerName });
  }

  async putObject(bucket: string, key: string, data: Buffer, options?: PutObjectOptions): Promise<PutObjectResult> {
    const blobClient = this.getBlobClient(bucket, key);

    const uploadOptions: any = {
      blobHTTPHeaders: {
        blobContentType: options?.contentType ?? 'application/octet-stream',
      },
      metadata: options?.metadata,
    };

    // Set access tier (storage class)
    const accessTier = this.mapStorageClass(options?.storageClass);
    if (accessTier) {
      uploadOptions.tier = accessTier;
    }

    const uploadResult = await blobClient.upload(data, data.length, uploadOptions);

    logger.debug('Uploaded blob to Azure', {
      container: this.getContainerName(bucket),
      key,
      size: data.length,
    });

    return {
      etag: uploadResult.etag ?? '',
      checksum_sha256: options?.checksumSHA256,
    };
  }

  async getObject(bucket: string, key: string): Promise<GetObjectResult> {
    const blobClient = this.getBlobClient(bucket, key);

    const downloadResponse = await blobClient.download();

    if (!downloadResponse.readableStreamBody) {
      throw new Error('Blob body is empty');
    }

    // Convert stream to buffer
    const chunks: Buffer[] = [];
    for await (const chunk of downloadResponse.readableStreamBody) {
      chunks.push(Buffer.from(chunk));
    }
    const data = Buffer.concat(chunks);

    return {
      data,
      contentType: downloadResponse.contentType ?? 'application/octet-stream',
      contentLength: downloadResponse.contentLength ?? data.length,
      etag: downloadResponse.etag ?? '',
      metadata: downloadResponse.metadata,
    };
  }

  async deleteObject(bucket: string, key: string): Promise<void> {
    const blobClient = this.getBlobClient(bucket, key);

    await blobClient.delete();

    logger.debug('Deleted blob from Azure', {
      container: this.getContainerName(bucket),
      key,
    });
  }

  async listObjects(bucket: string, prefix?: string, maxKeys = 1000): Promise<ListObjectsResult> {
    const containerClient = this.getContainerClient(bucket);

    const listOptions = prefix ? { prefix } : undefined;

    const objects: ListObjectsResult['objects'] = [];
    let count = 0;

    for await (const blob of containerClient.listBlobsFlat(listOptions)) {
      if (count >= maxKeys) {
        break;
      }

      objects.push({
        key: blob.name,
        size: blob.properties.contentLength ?? 0,
        etag: blob.properties.etag ?? '',
        lastModified: blob.properties.lastModified ?? new Date(),
      });

      count++;
    }

    return {
      objects,
      isTruncated: count >= maxKeys,
      nextToken: undefined, // Azure pagination works differently
    };
  }

  async presignPutObject(bucket: string, key: string, expiresIn: number, _options?: PresignOptions): Promise<string> {
    const blobClient = this.getBlobClient(bucket, key);

    const expiresOn = new Date(Date.now() + expiresIn * 1000);

    const sasToken = generateBlobSASQueryParameters(
      {
        containerName: this.getContainerName(bucket),
        blobName: key,
        permissions: BlobSASPermissions.parse('w'), // write permission
        expiresOn,
      },
      this.credential
    ).toString();

    const presignedUrl = `${blobClient.url}?${sasToken}`;

    logger.debug('Generated presigned PUT URL', {
      container: this.getContainerName(bucket),
      key,
      expiresIn,
    });

    return presignedUrl;
  }

  async presignGetObject(bucket: string, key: string, expiresIn: number): Promise<string> {
    const blobClient = this.getBlobClient(bucket, key);

    const expiresOn = new Date(Date.now() + expiresIn * 1000);

    const sasToken = generateBlobSASQueryParameters(
      {
        containerName: this.getContainerName(bucket),
        blobName: key,
        permissions: BlobSASPermissions.parse('r'), // read permission
        expiresOn,
      },
      this.credential
    ).toString();

    const presignedUrl = `${blobClient.url}?${sasToken}`;

    logger.debug('Generated presigned GET URL', {
      container: this.getContainerName(bucket),
      key,
      expiresIn,
    });

    return presignedUrl;
  }

  async createMultipartUpload(bucket: string, key: string, _options?: MultipartOptions): Promise<string> {
    // Azure uses block blobs - return the key as upload ID
    // Blocks are staged and then committed in completeMultipartUpload

    logger.debug('Created multipart upload (Azure block blob)', {
      container: this.getContainerName(bucket),
      key,
    });

    // Return key as uploadId since Azure doesn't have separate upload IDs
    return key;
  }

  async uploadPart(bucket: string, key: string, _uploadId: string, partNumber: number, data: Buffer): Promise<string> {
    const blobClient = this.getBlobClient(bucket, key);

    // Azure uses block IDs - generate from part number
    const blockId = this.generateBlockId(partNumber);

    await blobClient.stageBlock(blockId, data, data.length);

    logger.debug('Uploaded block', {
      container: this.getContainerName(bucket),
      key,
      partNumber,
      blockId,
      size: data.length,
    });

    // Return blockId as etag equivalent
    return blockId;
  }

  async completeMultipartUpload(
    bucket: string,
    key: string,
    _uploadId: string,
    parts: CompletedPart[]
  ): Promise<CompleteMultipartResult> {
    const blobClient = this.getBlobClient(bucket, key);

    // Sort parts by part number and extract block IDs (stored in etag field)
    const blockIds = parts.sort((a, b) => a.partNumber - b.partNumber).map(part => part.etag);

    // Commit the blocks
    const result = await blobClient.commitBlockList(blockIds);

    logger.info('Completed multipart upload (committed block list)', {
      container: this.getContainerName(bucket),
      key,
      parts: parts.length,
    });

    return {
      etag: result.etag ?? '',
      location: blobClient.url,
    };
  }

  async abortMultipartUpload(bucket: string, key: string, _uploadId: string): Promise<void> {
    // Azure uncommitted blocks are automatically garbage collected after 7 days
    // No explicit abort needed, but we can delete the blob if it exists

    logger.debug('Aborted multipart upload (Azure blocks auto-expire)', {
      container: this.getContainerName(bucket),
      key,
    });
  }

  /**
   * Generate a block ID from part number (Azure requires base64 block IDs)
   */
  private generateBlockId(partNumber: number): string {
    // Pad to 6 digits and base64 encode
    const paddedNumber = partNumber.toString().padStart(6, '0');
    return Buffer.from(`block-${paddedNumber}`).toString('base64');
  }
}
