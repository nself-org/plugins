/**
 * File Processing Plugin - Webhook Handler
 * Supports inbound webhooks from storage providers (MinIO, S3, GCS, R2, B2, Azure)
 */

import crypto from 'crypto';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('file-processing:webhooks');

// =============================================================================
// Types
// =============================================================================

export interface WebhookEvent {
  provider: 'minio' | 's3' | 'gcs' | 'r2' | 'b2' | 'azure';
  eventType: string;
  bucket: string;
  key: string;
  size?: number;
  timestamp: Date;
  etag?: string;
  versionId?: string;
  metadata?: Record<string, unknown>;
}

export interface MinIOEvent {
  EventName: string;
  Key: string;
  Records: Array<{
    eventVersion: string;
    eventSource: string;
    awsRegion: string;
    eventTime: string;
    eventName: string;
    s3: {
      s3SchemaVersion: string;
      configurationId: string;
      bucket: {
        name: string;
        ownerIdentity: {
          principalId: string;
        };
        arn: string;
      };
      object: {
        key: string;
        size: number;
        eTag: string;
        versionId?: string;
        sequencer: string;
      };
    };
  }>;
}

export interface S3Event {
  Records: Array<{
    eventVersion: string;
    eventSource: string;
    awsRegion: string;
    eventTime: string;
    eventName: string;
    s3: {
      s3SchemaVersion: string;
      configurationId: string;
      bucket: {
        name: string;
        ownerIdentity: {
          principalId: string;
        };
        arn: string;
      };
      object: {
        key: string;
        size: number;
        eTag: string;
        versionId?: string;
        sequencer: string;
      };
    };
  }>;
}

export interface GCSEvent {
  kind: string;
  id: string;
  selfLink: string;
  name: string;
  bucket: string;
  generation: string;
  metageneration: string;
  contentType: string;
  timeCreated: string;
  updated: string;
  storageClass: string;
  timeStorageClassUpdated: string;
  size: string;
  md5Hash: string;
  mediaLink: string;
  metadata?: Record<string, unknown>;
}

export interface AzureEvent {
  topic: string;
  subject: string;
  eventType: string;
  eventTime: string;
  id: string;
  data: {
    api: string;
    clientRequestId: string;
    requestId: string;
    eTag: string;
    contentType: string;
    contentLength: number;
    blobType: string;
    url: string;
    sequencer: string;
    storageDiagnostics: {
      batchId: string;
    };
  };
}

// =============================================================================
// Webhook Handler
// =============================================================================

export class WebhookHandler {
  /**
   * Verify webhook signature using HMAC-SHA256
   */
  verifySignature(payload: string, signature: string, secret: string): boolean {
    try {
      const hmac = crypto.createHmac('sha256', secret);
      const expectedSignature = hmac.update(payload).digest('hex');

      // Use timing-safe comparison to prevent timing attacks
      return crypto.timingSafeEqual(
        Buffer.from(signature),
        Buffer.from(expectedSignature)
      );
    } catch (error) {
      logger.error('Signature verification failed', { error });
      return false;
    }
  }

  /**
   * Parse MinIO webhook event
   * MinIO uses AWS S3 event format
   */
  parseMinIOWebhook(body: MinIOEvent): WebhookEvent {
    const record = body.Records[0];
    if (!record) {
      throw new Error('MinIO webhook missing Records array');
    }

    return {
      provider: 'minio',
      eventType: record.eventName,
      bucket: record.s3.bucket.name,
      key: record.s3.object.key,
      size: record.s3.object.size,
      timestamp: new Date(record.eventTime),
      etag: record.s3.object.eTag,
      versionId: record.s3.object.versionId,
    };
  }

  /**
   * Parse AWS S3 webhook event
   * Same format as MinIO (S3-compatible)
   */
  parseS3Webhook(body: S3Event): WebhookEvent {
    const record = body.Records[0];
    if (!record) {
      throw new Error('S3 webhook missing Records array');
    }

    return {
      provider: 's3',
      eventType: record.eventName,
      bucket: record.s3.bucket.name,
      key: record.s3.object.key,
      size: record.s3.object.size,
      timestamp: new Date(record.eventTime),
      etag: record.s3.object.eTag,
      versionId: record.s3.object.versionId,
    };
  }

  /**
   * Parse Cloudflare R2 webhook event
   * R2 uses S3-compatible event format
   */
  parseR2Webhook(body: S3Event): WebhookEvent {
    const record = body.Records[0];
    if (!record) {
      throw new Error('R2 webhook missing Records array');
    }

    return {
      provider: 'r2',
      eventType: record.eventName,
      bucket: record.s3.bucket.name,
      key: record.s3.object.key,
      size: record.s3.object.size,
      timestamp: new Date(record.eventTime),
      etag: record.s3.object.eTag,
      versionId: record.s3.object.versionId,
    };
  }

  /**
   * Parse Backblaze B2 webhook event
   * B2 uses S3-compatible event format
   */
  parseB2Webhook(body: S3Event): WebhookEvent {
    const record = body.Records[0];
    if (!record) {
      throw new Error('B2 webhook missing Records array');
    }

    return {
      provider: 'b2',
      eventType: record.eventName,
      bucket: record.s3.bucket.name,
      key: record.s3.object.key,
      size: record.s3.object.size,
      timestamp: new Date(record.eventTime),
      etag: record.s3.object.eTag,
      versionId: record.s3.object.versionId,
    };
  }

  /**
   * Parse Google Cloud Storage webhook event
   * GCS uses Pub/Sub notification format
   */
  parseGCSWebhook(body: GCSEvent): WebhookEvent {
    return {
      provider: 'gcs',
      eventType: body.kind,
      bucket: body.bucket,
      key: body.name,
      size: body.size ? parseInt(body.size, 10) : undefined,
      timestamp: new Date(body.timeCreated),
      etag: body.md5Hash,
      metadata: body.metadata,
    };
  }

  /**
   * Parse Azure Blob Storage webhook event
   * Azure uses Event Grid format
   */
  parseAzureWebhook(body: AzureEvent): WebhookEvent {
    // Extract bucket and key from URL
    // URL format: https://{account}.blob.core.windows.net/{container}/{blob}
    const url = new URL(body.data.url);
    const pathParts = url.pathname.split('/').filter(Boolean);
    const bucket = pathParts[0] || '';
    const key = pathParts.slice(1).join('/');

    return {
      provider: 'azure',
      eventType: body.eventType,
      bucket,
      key,
      size: body.data.contentLength,
      timestamp: new Date(body.eventTime),
      etag: body.data.eTag,
    };
  }

  /**
   * Determine if event is a file creation/upload event
   */
  isUploadEvent(event: WebhookEvent): boolean {
    const uploadPatterns = [
      's3:ObjectCreated:',
      's3:ObjectCreated:Put',
      's3:ObjectCreated:Post',
      's3:ObjectCreated:CompleteMultipartUpload',
      'storage#object', // GCS
      'Microsoft.Storage.BlobCreated', // Azure
    ];

    return uploadPatterns.some(pattern => event.eventType.includes(pattern));
  }

  /**
   * Extract file ID from object key
   * Supports various key formats:
   * - uploads/{file_id}/original.jpg
   * - {file_id}.jpg
   * - files/{user_id}/{file_id}.jpg
   */
  extractFileId(key: string): string | null {
    // Try UUID pattern first
    const uuidMatch = key.match(/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/i);
    if (uuidMatch) {
      return uuidMatch[1];
    }

    // Try file_ prefix pattern
    const fileIdMatch = key.match(/file_([a-zA-Z0-9_-]+)/);
    if (fileIdMatch) {
      return `file_${fileIdMatch[1]}`;
    }

    // Use filename without extension as fallback
    const parts = key.split('/');
    const filename = parts[parts.length - 1];
    const nameWithoutExt = filename.split('.')[0];

    return nameWithoutExt || null;
  }
}

/**
 * Export singleton instance
 */
export const webhookHandler = new WebhookHandler();

/**
 * Legacy export for backwards compatibility
 */
export function getWebhookInfo(): { supported: true; endpoints: string[] } {
  return {
    supported: true,
    endpoints: [
      '/webhook/minio',
      '/webhook/s3',
      '/webhook/r2',
      '/webhook/b2',
      '/webhook/gcs',
      '/webhook/azure',
    ],
  };
}
