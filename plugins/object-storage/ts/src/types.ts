/**
 * Object Storage Plugin Types
 * Complete type definitions for object storage operations
 */

export type StorageProvider = 'local' | 's3' | 'minio' | 'r2' | 'gcs' | 'b2' | 'azure';
export type StorageClass = 'standard' | 'reduced_redundancy' | 'glacier' | 'deep_archive';
export type UploadType = 'direct' | 'multipart' | 'presigned';
export type UploadStatus = 'initiated' | 'uploading' | 'completing' | 'completed' | 'aborted' | 'expired';
export type AccessAction = 'upload' | 'download' | 'delete' | 'list' | 'presign';

// =============================================================================
// Configuration
// =============================================================================

export interface ObjectStoragePluginConfig {
  // Server
  port: number;
  host: string;

  // Storage
  storageBasePath: string;
  defaultProvider: StorageProvider;

  // S3 Configuration
  s3Endpoint?: string;
  s3Region?: string;
  s3AccessKey?: string;
  s3SecretKey?: string;
  s3BucketPrefix?: string;

  // Upload Limits
  presignExpirySeconds: number;
  maxUploadSize: number;
  multipartThreshold: number;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Security
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;

  // Logging
  logLevel: string;
}

export interface ProviderConfig {
  endpoint?: string;
  region?: string;
  accessKeyId?: string;
  secretAccessKey?: string;
  bucketPrefix?: string;
  forcePathStyle?: boolean;
}

// =============================================================================
// Database Records
// =============================================================================

export interface BucketRecord {
  id: string;
  source_account_id: string;
  name: string;
  provider: StorageProvider;
  provider_config: ProviderConfig;
  public_read: boolean;
  cors_origins: string[];
  max_file_size_bytes: number;
  allowed_mime_types: string[];
  quota_bytes: number | null;
  used_bytes: number;
  object_count: number;
  lifecycle_rules: LifecycleRule[];
  created_at: Date;
  updated_at: Date;
}

export interface ObjectRecord {
  id: string;
  source_account_id: string;
  bucket_id: string;
  key: string;
  filename: string | null;
  content_type: string;
  size_bytes: number;
  checksum_sha256: string | null;
  etag: string | null;
  storage_class: StorageClass;
  metadata: Record<string, string>;
  tags: Record<string, string>;
  owner_id: string | null;
  is_public: boolean;
  version: number;
  deleted_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface UploadSessionRecord {
  id: string;
  source_account_id: string;
  bucket_id: string;
  key: string;
  content_type: string;
  total_size_bytes: number | null;
  upload_type: UploadType;
  status: UploadStatus;
  multipart_upload_id: string | null;
  parts_completed: number;
  parts_total: number | null;
  presigned_url: string | null;
  presigned_expires_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface AccessLogRecord {
  id: string;
  source_account_id: string;
  bucket_id: string | null;
  object_id: string | null;
  action: AccessAction;
  actor_id: string | null;
  ip_address: string | null;
  user_agent: string | null;
  status: number;
  response_time_ms: number | null;
  bytes_transferred: number | null;
  created_at: Date;
}

export interface WebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Lifecycle Rules
// =============================================================================

export interface LifecycleRule {
  id: string;
  enabled: boolean;
  prefix?: string;
  tags?: Record<string, string>;
  expiration_days?: number;
  transition?: {
    days: number;
    storage_class: StorageClass;
  };
  abort_incomplete_multipart_upload_days?: number;
}

// =============================================================================
// API Request/Response Types
// =============================================================================

export interface CreateBucketRequest {
  name: string;
  provider?: StorageProvider;
  provider_config?: ProviderConfig;
  public_read?: boolean;
  cors_origins?: string[];
  max_file_size_bytes?: number;
  allowed_mime_types?: string[];
  quota_bytes?: number;
}

export interface UpdateBucketRequest {
  public_read?: boolean;
  cors_origins?: string[];
  max_file_size_bytes?: number;
  allowed_mime_types?: string[];
  quota_bytes?: number;
  lifecycle_rules?: LifecycleRule[];
}

export interface UploadObjectRequest {
  bucket_id: string;
  key: string;
  file: Buffer;
  filename?: string;
  content_type?: string;
  metadata?: Record<string, string>;
  tags?: Record<string, string>;
  is_public?: boolean;
  storage_class?: StorageClass;
}

export interface PresignUploadRequest {
  bucket_id: string;
  key: string;
  content_type?: string;
  expires_in?: number;
  metadata?: Record<string, string>;
}

export interface PresignDownloadRequest {
  bucket_id: string;
  key: string;
  expires_in?: number;
}

export interface PresignedUrlResponse {
  url: string;
  expires_at: Date;
  method: string;
  headers?: Record<string, string>;
}

export interface MultipartUploadInitRequest {
  bucket_id: string;
  key: string;
  content_type?: string;
  total_size_bytes?: number;
  metadata?: Record<string, string>;
}

export interface MultipartUploadInitResponse {
  session_id: string;
  upload_id: string;
  bucket_id: string;
  key: string;
}

export interface MultipartUploadPartRequest {
  session_id: string;
  part_number: number;
  file: Buffer;
}

export interface MultipartUploadPartResponse {
  part_number: number;
  etag: string;
}

export interface MultipartUploadCompleteRequest {
  session_id: string;
  parts: Array<{
    part_number: number;
    etag: string;
  }>;
}

export interface ListObjectsRequest {
  bucket_id: string;
  prefix?: string;
  max_keys?: number;
  continuation_token?: string;
}

export interface ListObjectsResponse {
  objects: ObjectRecord[];
  is_truncated: boolean;
  continuation_token?: string;
}

export interface BucketUsageStats {
  bucket_id: string;
  bucket_name: string;
  object_count: number;
  total_bytes: number;
  quota_bytes: number | null;
  quota_used_percent: number | null;
}

export interface StorageStats {
  total_buckets: number;
  total_objects: number;
  total_bytes: number;
  by_provider: Record<StorageProvider, {
    buckets: number;
    objects: number;
    bytes: number;
  }>;
  by_storage_class: Record<StorageClass, {
    objects: number;
    bytes: number;
  }>;
  recent_uploads: number;
  recent_downloads: number;
}

// =============================================================================
// Storage Provider Interface
// =============================================================================

export interface StorageBackend {
  provider: StorageProvider;

  // Core operations
  putObject(bucket: string, key: string, data: Buffer, options?: PutObjectOptions): Promise<PutObjectResult>;
  getObject(bucket: string, key: string): Promise<GetObjectResult>;
  deleteObject(bucket: string, key: string): Promise<void>;
  listObjects(bucket: string, prefix?: string, maxKeys?: number): Promise<ListObjectsResult>;

  // Presigned URLs
  presignPutObject(bucket: string, key: string, expiresIn: number, options?: PresignOptions): Promise<string>;
  presignGetObject(bucket: string, key: string, expiresIn: number): Promise<string>;

  // Multipart uploads
  createMultipartUpload(bucket: string, key: string, options?: MultipartOptions): Promise<string>;
  uploadPart(bucket: string, key: string, uploadId: string, partNumber: number, data: Buffer): Promise<string>;
  completeMultipartUpload(bucket: string, key: string, uploadId: string, parts: CompletedPart[]): Promise<CompleteMultipartResult>;
  abortMultipartUpload(bucket: string, key: string, uploadId: string): Promise<void>;

  // Bucket operations (if supported)
  createBucket?(bucket: string): Promise<void>;
  deleteBucket?(bucket: string): Promise<void>;
}

export interface PutObjectOptions {
  contentType?: string;
  metadata?: Record<string, string>;
  storageClass?: StorageClass;
  checksumSHA256?: string;
}

export interface PutObjectResult {
  etag: string;
  checksum_sha256?: string;
}

export interface GetObjectResult {
  data: Buffer;
  contentType: string;
  contentLength: number;
  etag: string;
  metadata?: Record<string, string>;
}

export interface ListObjectsResult {
  objects: Array<{
    key: string;
    size: number;
    etag: string;
    lastModified: Date;
  }>;
  isTruncated: boolean;
  nextToken?: string;
}

export interface PresignOptions {
  contentType?: string;
  metadata?: Record<string, string>;
}

export interface MultipartOptions {
  contentType?: string;
  metadata?: Record<string, string>;
  storageClass?: StorageClass;
}

export interface CompletedPart {
  partNumber: number;
  etag: string;
}

export interface CompleteMultipartResult {
  etag: string;
  location: string;
}
