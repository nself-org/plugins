/**
 * File Processing Plugin - Type Definitions
 */

export type StorageProvider = 'minio' | 's3' | 'gcs' | 'r2' | 'b2' | 'azure';

export type ProcessingStatus = 'pending' | 'processing' | 'completed' | 'failed' | 'cancelled';

export type ProcessingOperation = 'thumbnail' | 'optimize' | 'metadata';

// =============================================================================
// Configuration
// =============================================================================

export interface FileProcessingConfig {
  // Storage
  storageProvider: StorageProvider;
  storageBucket: string;
  storageEndpoint?: string;
  storageRegion?: string;
  storageAccessKey?: string;
  storageSecretKey?: string;
  azureConnectionString?: string;
  googleCredentials?: string;

  // Processing
  thumbnailSizes: number[];
  enableVirusScan: boolean;
  enableOptimization: boolean;
  maxFileSize: number;
  allowedTypes: string[];
  stripExif: boolean;
  queueConcurrency: number;

  // ClamAV
  clamavHost?: string;
  clamavPort?: number;

  // Queue
  redisUrl?: string;

  // Server
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';
}

// =============================================================================
// Database Models
// =============================================================================

export interface ProcessingJob {
  id: string;
  source_account_id: string;
  file_id: string;
  file_path: string;
  file_name: string;
  file_size: number;
  mime_type: string;
  storage_provider: StorageProvider;
  storage_bucket: string;
  status: ProcessingStatus;
  priority: number;
  attempts: number;
  max_attempts: number;
  operations: ProcessingOperation[];
  thumbnails: string[];
  metadata?: Record<string, unknown>;
  scan_result?: ScanResult;
  optimization_result?: OptimizationResult;
  error_message?: string;
  error_stack?: string;
  last_error_at?: Date;
  started_at?: Date;
  completed_at?: Date;
  duration_ms?: number;
  queue_name: string;
  scheduled_for?: Date;
  webhook_url?: string;
  webhook_secret?: string;
  callback_data?: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface FileThumbnail {
  id: string;
  source_account_id: string;
  job_id: string;
  file_id: string;
  thumbnail_path: string;
  thumbnail_url?: string;
  width: number;
  height: number;
  size_bytes?: number;
  format: 'jpeg' | 'png' | 'webp';
  source_width?: number;
  source_height?: number;
  quality?: number;
  optimization_applied: boolean;
  generation_time_ms?: number;
  storage_provider: StorageProvider;
  storage_bucket: string;
  created_at: Date;
  updated_at: Date;
}

export interface FileMetadata {
  id: string;
  source_account_id: string;
  job_id: string;
  file_id: string;
  mime_type: string;
  file_extension?: string;
  file_size: number;
  width?: number;
  height?: number;
  aspect_ratio?: number;
  color_space?: string;
  bit_depth?: number;
  has_alpha?: boolean;
  exif_data?: Record<string, unknown>;
  camera_make?: string;
  camera_model?: string;
  lens_model?: string;
  focal_length?: string;
  aperture?: string;
  shutter_speed?: string;
  iso?: number;
  flash?: string;
  orientation?: number;
  gps_latitude?: number;
  gps_longitude?: number;
  gps_altitude?: number;
  location_name?: string;
  date_taken?: Date;
  date_modified?: Date;
  duration_seconds?: number;
  video_codec?: string;
  audio_codec?: string;
  frame_rate?: number;
  bitrate?: number;
  audio_channels?: number;
  sample_rate?: number;
  page_count?: number;
  word_count?: number;
  author?: string;
  title?: string;
  subject?: string;
  md5_hash?: string;
  sha256_hash?: string;
  perceptual_hash?: string;
  exif_stripped: boolean;
  metadata_extracted_at?: Date;
  extraction_duration_ms?: number;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Processing Results
// =============================================================================

export interface ThumbnailResult {
  path: string;
  url?: string;
  width: number;
  height: number;
  size: number;
  format: 'jpeg' | 'png' | 'webp';
  generationTime: number;
}

export interface OptimizationResult {
  originalSize: number;
  optimizedSize: number;
  savingsBytes: number;
  savingsPercent: number;
  duration: number;
}

export interface MetadataResult {
  extracted: Record<string, unknown>;
  exifStripped: boolean;
  extractionTime: number;
}

export interface ProcessingResult {
  jobId: string;
  fileId: string;
  status: ProcessingStatus;
  thumbnails: ThumbnailResult[];
  metadata?: MetadataResult;
  optimization?: OptimizationResult;
  error?: string;
  duration: number;
}

// =============================================================================
// API Request/Response Types
// =============================================================================

export interface CreateJobRequest {
  fileId: string;
  filePath: string;
  fileName: string;
  fileSize: number;
  mimeType: string;
  operations?: ProcessingOperation[];
  priority?: number;
  webhookUrl?: string;
  webhookSecret?: string;
  callbackData?: Record<string, unknown>;
}

export interface CreateJobResponse {
  jobId: string;
  status: ProcessingStatus;
  estimatedDuration?: number;
}

export interface GetJobResponse {
  job: ProcessingJob;
  thumbnails: FileThumbnail[];
  metadata?: FileMetadata;
  scan?: FileScan;
}

export interface ProcessingStatsResponse {
  pending: number;
  processing: number;
  completed: number;
  failed: number;
  avgDurationMs: number;
  totalProcessed: number;
  thumbnailsGenerated: number;
  storageUsed: number;
}

// =============================================================================
// Storage Adapter Interface
// =============================================================================

export interface StorageAdapter {
  provider: StorageProvider;

  /**
   * Upload a file to storage
   */
  upload(
    localPath: string,
    remotePath: string,
    mimeType: string,
    bucket?: string
  ): Promise<{ url: string; size: number }>;

  /**
   * Download a file from storage
   */
  download(remotePath: string, localPath: string, bucket?: string): Promise<void>;

  /**
   * Get a temporary URL for a file
   */
  getTemporaryUrl(remotePath: string, expiresIn: number, bucket?: string): Promise<string>;

  /**
   * Delete a file from storage
   */
  delete(remotePath: string, bucket?: string): Promise<void>;

  /**
   * Check if a file exists
   */
  exists(remotePath: string, bucket?: string): Promise<boolean>;

  /**
   * Get file metadata
   */
  getMetadata(remotePath: string, bucket?: string): Promise<{
    size: number;
    contentType: string;
    lastModified: Date;
  }>;
}

// =============================================================================
// Image Processing Types (nTV endpoints)
// =============================================================================

export interface PosterOutput {
  width: number;
  format: string;
  path: string;
  size: number;
}

export interface SpriteOutput {
  sprite_path: string;
  vtt_path: string;
  frame_count: number;
}

export interface OptimizeOutput {
  output_path: string;
  original_size: number;
  optimized_size: number;
  savings_percent: number;
}

export interface PosterRequest {
  input_path: string;
  widths?: number[];
  formats?: string[];
}

export interface SpriteRequest {
  input_path: string;
  grid?: string;
  thumb_size?: string;
}

export interface OptimizeRequest {
  input_path: string;
  format?: string;
  quality?: number;
  strip_exif?: boolean;
}

// =============================================================================
// Queue Job Data
// =============================================================================

export interface QueueJobData {
  jobId: string;
  fileId: string;
  filePath: string;
  fileName: string;
  fileSize: number;
  mimeType: string;
  storageProvider: StorageProvider;
  storageBucket: string;
  operations: ProcessingOperation[];
}

export interface QueueJobResult {
  success: boolean;
  jobId: string;
  thumbnails?: ThumbnailResult[];
  metadata?: Record<string, unknown>;
  scan?: ScanResult;
  optimization?: OptimizationResult;
  error?: string;
  duration: number;
}
