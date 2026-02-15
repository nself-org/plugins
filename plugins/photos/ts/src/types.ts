/**
 * Photos Plugin Types
 * All TypeScript interfaces for the photo album management service
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface PhotosAlbumRecord {
  id: string;
  source_account_id: string;
  owner_id: string;
  name: string;
  description: string | null;
  cover_photo_id: string | null;
  visibility: string;
  visibility_user_ids: string[] | null;
  photo_count: number;
  sort_order: string;
  date_range_start: string | null;
  date_range_end: string | null;
  location_name: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface PhotosItemRecord {
  id: string;
  source_account_id: string;
  album_id: string | null;
  uploader_id: string;
  file_id: string | null;
  original_url: string;
  thumbnail_small_url: string | null;
  thumbnail_medium_url: string | null;
  thumbnail_large_url: string | null;
  width: number | null;
  height: number | null;
  file_size_bytes: number | null;
  mime_type: string | null;
  original_filename: string | null;
  caption: string | null;
  visibility: string;
  taken_at: Date | null;
  location_latitude: number | null;
  location_longitude: number | null;
  location_name: string | null;
  camera_make: string | null;
  camera_model: string | null;
  focal_length: string | null;
  aperture: string | null;
  shutter_speed: string | null;
  iso: number | null;
  orientation: number;
  processing_status: string;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface PhotosTagRecord {
  id: string;
  source_account_id: string;
  photo_id: string;
  tag_type: string;
  tag_value: string;
  tagged_user_id: string | null;
  face_region: Record<string, unknown> | null;
  confidence: number | null;
  created_by: string | null;
  created_at: Date;
}

// Face recognition removed - was placeholder
// Can be re-implemented with real face-api.js if needed

export interface PhotosWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface CreateAlbumRequest {
  name: string;
  description?: string;
  visibility?: string;
  visibilityUserIds?: string[];
  sortOrder?: string;
  metadata?: Record<string, unknown>;
}

export interface UpdateAlbumRequest {
  name?: string;
  description?: string;
  coverPhotoId?: string;
  visibility?: string;
  visibilityUserIds?: string[];
  sortOrder?: string;
}

export interface RegisterPhotoRequest {
  albumId?: string;
  fileId?: string;
  originalUrl: string;
  originalFilename?: string;
  caption?: string;
  visibility?: string;
}

export interface BatchRegisterPhotosRequest {
  albumId: string;
  photos: Array<{
    originalUrl: string;
    originalFilename?: string;
    caption?: string;
  }>;
}

export interface UpdatePhotoRequest {
  caption?: string;
  albumId?: string;
  visibility?: string;
}

export interface MovePhotoRequest {
  albumId: string;
}

export interface AddTagRequest {
  tagType: 'keyword' | 'person' | 'location' | 'event';
  tagValue: string;
  taggedUserId?: string;
  faceRegion?: Record<string, unknown>;
}

// Face requests removed - feature not implemented
// UpdateFaceRequest, MergeFacesRequest

export interface SearchPhotosRequest {
  query?: string;
  tags?: string[];
  location?: string;
  dateFrom?: string;
  dateTo?: string;
  uploaderId?: string;
  albumId?: string;
  limit?: number;
  offset?: number;
}

export interface TimelineRequest {
  userId?: string;
  granularity?: 'day' | 'week' | 'month' | 'year';
  from?: string;
  to?: string;
}

export interface TimelinePeriod {
  period: string;
  count: number;
  coverPhotoUrl: string | null;
  location: string | null;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface PhotosConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  appIds: string[];

  // Thumbnails
  thumbnailSmall: number;
  thumbnailMedium: number;
  thumbnailLarge: number;
  thumbnailQuality: number;
  thumbnailFormat: string;

  // Processing
  exifExtraction: boolean;
  processingConcurrency: number;

  // Search
  searchEnabled: boolean;
  maxUploadBatch: number;

  // Security
  security: SecurityConfig;
}

export interface SecurityConfig {
  apiKey?: string;
  rateLimitMax?: number;
  rateLimitWindowMs?: number;
}

// ============================================================================
// Health/Status Types
// ============================================================================

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}

export interface ReadyCheckResponse {
  ready: boolean;
  database: 'ok' | 'error';
  timestamp: string;
}

export interface LiveCheckResponse {
  alive: boolean;
  uptime: number;
  memory: {
    used: number;
    total: number;
  };
  stats: PhotosStats;
}

export interface PhotosStats {
  totalAlbums: number;
  totalPhotos: number;
  totalTags: number;
  pendingProcessing: number;
  processedPhotos: number;
  totalStorageBytes: number;
}
