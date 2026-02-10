# File Processing Plugin

Comprehensive file processing with thumbnail generation, image optimization, video thumbnails, and virus scanning for nself.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Performance Considerations](#performance-considerations)
- [Security Notes](#security-notes)
- [Advanced Code Examples](#advanced-code-examples)
- [Monitoring & Alerting](#monitoring--alerting)
- [Use Cases](#use-cases)
- [Troubleshooting](#troubleshooting)

---

## Overview

The File Processing plugin provides a BullMQ-powered background processing pipeline for files. It generates thumbnails, optimizes images, extracts metadata, strips EXIF data, and scans for viruses. It works with any S3-compatible storage provider.

- **4 Database Tables** - Jobs, thumbnails, scans, metadata
- **5 Analytics Views** - Queue status, failures, security alerts, processing stats, thumbnail stats
- **6 Storage Providers** - MinIO, AWS S3, Google Cloud Storage, Cloudflare R2, Azure Blob, Backblaze B2
- **5 Processing Operations** - Thumbnail generation, image optimization, EXIF stripping, virus scanning, metadata extraction
- **Queue Processing** - BullMQ-powered background processing with configurable concurrency

### Processing Operations

| Operation | Description |
|-----------|-------------|
| Thumbnail Generation | Multiple sizes (100x100, 400x400, 1200x1200) via Sharp and ffmpeg |
| Image Optimization | Compress and optimize with quality control and progressive encoding |
| EXIF Stripping | Remove GPS, camera, software, and timestamp metadata |
| Virus Scanning | ClamAV-based malware detection with quarantine support |
| Metadata Extraction | Extract dimensions, codecs, duration, EXIF data |

---

## Quick Start

```bash
# Install the plugin
cd plugins/file-processing
./install.sh

# Configure environment
cp .env.example .env
# Edit .env with storage and database credentials

# Initialize database schema
nself plugin file-processing init

# Start the HTTP server (Terminal 1)
nself plugin file-processing server

# Start the background worker (Terminal 2)
nself plugin file-processing worker
```

### Prerequisites

- Node.js 18+
- PostgreSQL
- Redis (for queue management)
- Sharp (installed via npm)
- ffmpeg (optional, for video thumbnails)
- ClamAV (optional, for virus scanning)

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `FILE_STORAGE_PROVIDER` | Yes | - | Storage provider (minio, s3, gcs, r2, azure, b2) |
| `FILE_STORAGE_BUCKET` | Yes | - | Storage bucket name |
| `FILE_STORAGE_ENDPOINT` | No | - | Storage endpoint URL (required for MinIO, R2, B2) |
| `FILE_STORAGE_ACCESS_KEY` | No | - | Storage access key |
| `FILE_STORAGE_SECRET_KEY` | No | - | Storage secret key |
| `FILE_STORAGE_REGION` | No | `us-east-1` | Storage region |
| `FILE_THUMBNAIL_SIZES` | No | `100,400,1200` | Comma-separated thumbnail sizes |
| `FILE_ENABLE_OPTIMIZATION` | No | `true` | Enable image optimization |
| `FILE_STRIP_EXIF` | No | `true` | Strip EXIF metadata from images |
| `FILE_MAX_SIZE` | No | `104857600` | Maximum file size in bytes (100MB) |
| `FILE_ENABLE_VIRUS_SCAN` | No | `false` | Enable ClamAV virus scanning |
| `CLAMAV_HOST` | No | `localhost` | ClamAV daemon host |
| `CLAMAV_PORT` | No | `3310` | ClamAV daemon port |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection string |
| `FILE_QUEUE_CONCURRENCY` | No | `3` | Concurrent file processing jobs |
| `PORT` | No | `3104` | HTTP server port |
| `HOST` | No | `0.0.0.0` | Server bind host |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Storage Provider Configuration

**MinIO / S3-compatible:**
```bash
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_ACCESS_KEY=minioadmin
FILE_STORAGE_SECRET_KEY=minioadmin
```

**AWS S3:**
```bash
FILE_STORAGE_PROVIDER=s3
FILE_STORAGE_REGION=us-east-1
FILE_STORAGE_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
FILE_STORAGE_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

**Google Cloud Storage:**
```bash
FILE_STORAGE_PROVIDER=gcs
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
```

**Cloudflare R2:**
```bash
FILE_STORAGE_PROVIDER=r2
FILE_STORAGE_ENDPOINT=https://[account-id].r2.cloudflarestorage.com
FILE_STORAGE_ACCESS_KEY=your_r2_access_key
FILE_STORAGE_SECRET_KEY=your_r2_secret_key
```

**Azure Blob Storage:**
```bash
FILE_STORAGE_PROVIDER=azure
AZURE_STORAGE_CONNECTION_STRING=DefaultEndpointsProtocol=https;AccountName=...
```

**Backblaze B2:**
```bash
FILE_STORAGE_PROVIDER=b2
FILE_STORAGE_ENDPOINT=https://s3.us-west-000.backblazeb2.com
FILE_STORAGE_ACCESS_KEY=your_key_id
FILE_STORAGE_SECRET_KEY=your_application_key
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin file-processing init

# View processing statistics
nself plugin file-processing stats

# Clean up old jobs (default 30 days)
nself plugin file-processing cleanup [--days 30]
```

### Processing

```bash
# Process a file immediately
nself plugin file-processing process <file-id> <file-path>
```

### Server & Worker

```bash
# Start HTTP server
nself plugin file-processing server

# Start background worker
nself plugin file-processing worker
```

---

## REST API

The plugin exposes a REST API when the server is running.

### Base URL

```
http://localhost:3104
```

### Endpoints

#### Health Check

```http
GET /health
```
Returns server health status.

#### Create Processing Job

```http
POST /api/jobs
Content-Type: application/json

{
  "fileId": "file_123",
  "filePath": "uploads/photo.jpg",
  "fileName": "photo.jpg",
  "fileSize": 1024000,
  "mimeType": "image/jpeg",
  "operations": ["thumbnail", "optimize", "metadata"],
  "priority": 5,
  "webhookUrl": "https://myapp.com/webhooks/file-processed",
  "webhookSecret": "secret_key",
  "callbackData": { "userId": "user_123" }
}
```

Returns `{ jobId, status, estimatedDuration }`.

#### Get Job Status

```http
GET /api/jobs/:jobId
```
Returns job details including status, thumbnails, metadata, and scan results.

#### List Jobs

```http
GET /api/jobs?status=completed&limit=50&offset=0
```
List jobs with optional status filter and pagination.

#### Processing Statistics

```http
GET /api/stats
```
Returns counts by status (pending, processing, completed, failed), average duration, total processed, thumbnails generated, and storage used.

---

## Webhook Events

When a processing job completes, the plugin sends an HTTP POST to the configured `webhookUrl` with an `X-Signature` header (HMAC-SHA256) for verification.

### Webhook Payload

| Event | Description |
|-------|-------------|
| `job.completed` | File processing completed successfully |
| `job.failed` | File processing failed |

```json
{
  "event": "job.completed",
  "jobId": "550e8400-e29b-41d4-a716-446655440000",
  "fileId": "file_123",
  "status": "completed",
  "thumbnails": [
    { "width": 100, "height": 100, "url": "https://storage/thumbnails/thumb_100.jpg" }
  ],
  "metadata": { "width": 3000, "height": 2000, "format": "JPEG" },
  "scan": { "clean": true },
  "optimization": { "originalSize": 1024000, "optimizedSize": 512000 },
  "durationMs": 2847,
  "callbackData": { "userId": "user_123" }
}
```

---

## Database Schema

### file_processing_jobs

Processing queue and job history.

```sql
CREATE TABLE file_processing_jobs (
    id UUID PRIMARY KEY,
    file_id VARCHAR(255) NOT NULL,
    file_path TEXT NOT NULL,
    file_name VARCHAR(255),
    file_size BIGINT,
    mime_type VARCHAR(255),
    operations JSONB DEFAULT '[]',         -- ["thumbnail", "optimize", "metadata"]
    priority INTEGER DEFAULT 0,
    status VARCHAR(50) NOT NULL,           -- pending, processing, completed, failed
    webhook_url TEXT,
    webhook_secret VARCHAR(255),
    callback_data JSONB,
    duration_ms INTEGER,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_file_processing_jobs_status ON file_processing_jobs(status);
CREATE INDEX idx_file_processing_jobs_file ON file_processing_jobs(file_id);
CREATE INDEX idx_file_processing_jobs_created ON file_processing_jobs(created_at DESC);
```

### file_thumbnails

Generated thumbnail metadata and URLs.

```sql
CREATE TABLE file_thumbnails (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES file_processing_jobs(id),
    file_id VARCHAR(255) NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    format VARCHAR(50),                    -- jpeg, png, webp
    size BIGINT,
    url TEXT NOT NULL,
    storage_path TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_thumbnails_job ON file_thumbnails(job_id);
CREATE INDEX idx_file_thumbnails_file ON file_thumbnails(file_id);
```

### file_scans

Virus scan results.

```sql
CREATE TABLE file_scans (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES file_processing_jobs(id),
    file_id VARCHAR(255) NOT NULL,
    clean BOOLEAN NOT NULL,
    threats JSONB DEFAULT '[]',
    scanner VARCHAR(50),                   -- clamav
    scanned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_scans_file ON file_scans(file_id);
CREATE INDEX idx_file_scans_clean ON file_scans(clean);
```

### file_metadata

Extracted EXIF and file metadata.

```sql
CREATE TABLE file_metadata (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES file_processing_jobs(id),
    file_id VARCHAR(255) NOT NULL,
    width INTEGER,
    height INTEGER,
    format VARCHAR(50),
    color_space VARCHAR(50),
    exif JSONB,
    gps JSONB,
    duration_seconds DECIMAL,
    codecs JSONB,
    frame_rate DECIMAL,
    exif_stripped BOOLEAN DEFAULT FALSE,
    extracted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_file_metadata_file ON file_metadata(file_id);
```

---

## Analytics Views

### file_processing_queue

Pending jobs ordered by priority.

```sql
CREATE VIEW file_processing_queue AS
SELECT id, file_id, file_name, priority, operations, created_at
FROM file_processing_jobs
WHERE status = 'pending'
ORDER BY priority DESC, created_at ASC;
```

### file_processing_failures

Failed jobs requiring attention.

```sql
CREATE VIEW file_processing_failures AS
SELECT id, file_id, file_name, error, created_at, completed_at
FROM file_processing_jobs
WHERE status = 'failed'
ORDER BY completed_at DESC;
```

### file_security_alerts

Infected files detected by virus scanning.

```sql
CREATE VIEW file_security_alerts AS
SELECT s.file_id, s.threats, s.scanned_at, j.file_name, j.file_path
FROM file_scans s
JOIN file_processing_jobs j ON s.job_id = j.id
WHERE s.clean = FALSE
ORDER BY s.scanned_at DESC;
```

### file_processing_stats

Processing statistics aggregated by status.

```sql
CREATE VIEW file_processing_stats AS
SELECT
    status,
    COUNT(*) AS job_count,
    AVG(duration_ms) AS avg_duration_ms,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed
FROM file_processing_jobs
GROUP BY status;
```

### thumbnail_generation_stats

Thumbnail generation statistics.

```sql
CREATE VIEW thumbnail_generation_stats AS
SELECT
    width,
    height,
    format,
    COUNT(*) AS count,
    AVG(size) AS avg_size
FROM file_thumbnails
GROUP BY width, height, format
ORDER BY count DESC;
```

---

## Performance Considerations

### Sharp.js Optimization

Sharp.js is a high-performance image processing library built on libvips. Follow these optimization strategies:

#### Memory Management

```javascript
// Configure Sharp memory limits
import sharp from 'sharp';

sharp.cache({
  memory: 50,      // Max memory cache (MB)
  files: 20,       // Max file descriptors
  items: 100       // Max cached items
});

// Disable cache for memory-constrained environments
sharp.cache(false);

// Set concurrency limit
sharp.concurrency(4); // Default: number of CPU cores
```

#### Performance Settings

```javascript
// High-speed thumbnail generation
await sharp(inputBuffer)
  .resize(400, 400, {
    fit: 'cover',
    position: 'attention',  // Smart crop using entropy detection
    kernel: 'cubic'         // Faster than 'lanczos3' with good quality
  })
  .jpeg({
    quality: 85,
    progressive: true,
    mozjpeg: true           // Better compression with libjpeg-turbo
  })
  .toBuffer();

// WebP for superior compression
await sharp(inputBuffer)
  .resize(1200, 1200, { fit: 'inside' })
  .webp({
    quality: 80,
    effort: 4,              // 0-6, higher = slower + smaller
    smartSubsample: true    // Better quality at lower bitrates
  })
  .toBuffer();

// AVIF for next-gen compression (slower, 50% smaller)
await sharp(inputBuffer)
  .resize(800, 800)
  .avif({
    quality: 70,
    effort: 4,
    chromaSubsampling: '4:2:0'
  })
  .toBuffer();
```

#### Batch Processing Optimization

```javascript
// Process multiple sizes in parallel
const sizes = [100, 400, 800, 1200];

await Promise.all(
  sizes.map(size =>
    sharp(inputBuffer)
      .resize(size, size, { fit: 'cover' })
      .jpeg({ quality: 85 })
      .toFile(`output_${size}.jpg`)
  )
);

// Pipeline pattern for multiple operations
const pipeline = sharp(inputBuffer)
  .rotate()                  // Auto-rotate based on EXIF
  .trim()                    // Remove solid-color borders
  .normalize();              // Stretch contrast

await Promise.all([
  pipeline.clone().resize(100, 100).toFile('thumb_small.jpg'),
  pipeline.clone().resize(400, 400).toFile('thumb_medium.jpg'),
  pipeline.clone().resize(1200, 1200).toFile('thumb_large.jpg')
]);
```

#### Performance Benchmarks

| Operation | Time (1000x1000 JPEG) | Notes |
|-----------|----------------------|-------|
| Resize (cubic) | ~15ms | Default quality |
| Resize (lanczos3) | ~25ms | Best quality |
| JPEG output (q85) | ~8ms | Progressive |
| WebP output (q80) | ~45ms | Effort 4 |
| AVIF output (q70) | ~180ms | Effort 4 |
| EXIF strip | ~1ms | Minimal overhead |
| Metadata extraction | ~2ms | Fast |

### FFmpeg Settings

FFmpeg video processing optimization:

#### Video Thumbnail Generation

```bash
# Fast thumbnail extraction (first frame)
ffmpeg -i input.mp4 -vframes 1 -q:v 2 output.jpg

# Thumbnail from specific time (5 seconds)
ffmpeg -ss 00:00:05 -i input.mp4 -vframes 1 -q:v 2 output.jpg

# Generate multiple thumbnails (every 10 seconds)
ffmpeg -i input.mp4 -vf fps=1/10 -q:v 2 thumb_%04d.jpg

# High-quality thumbnail with scaling
ffmpeg -i input.mp4 -vf "scale=1280:720:flags=lanczos" \
  -vframes 1 -q:v 2 output.jpg
```

#### Video Optimization Settings

```bash
# H.264 web-optimized transcoding
ffmpeg -i input.mp4 \
  -c:v libx264 -preset fast -crf 23 \
  -c:a aac -b:a 128k \
  -movflags +faststart \
  output.mp4

# Preset options: ultrafast, superfast, veryfast, faster, fast,
#                medium, slow, slower, veryslow
# CRF: 18 (near lossless) to 28 (visible compression)
# faststart: move moov atom to beginning for web streaming

# H.265/HEVC for 50% smaller files
ffmpeg -i input.mp4 \
  -c:v libx265 -preset medium -crf 28 \
  -c:a aac -b:a 128k \
  output.mp4

# AV1 for maximum compression (slow)
ffmpeg -i input.mp4 \
  -c:v libaom-av1 -crf 30 -b:v 0 \
  -cpu-used 4 \
  output.mkv
```

#### Metadata Extraction

```bash
# Extract all metadata as JSON
ffprobe -v quiet -print_format json -show_format -show_streams input.mp4

# Get video dimensions
ffprobe -v error -select_streams v:0 \
  -show_entries stream=width,height \
  -of csv=s=x:p=0 input.mp4

# Get duration in seconds
ffprobe -v error -show_entries format=duration \
  -of default=noprint_wrappers=1:nokey=1 input.mp4

# Check codec information
ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,codec_type,bit_rate \
  -of json input.mp4
```

#### Performance Tuning

```bash
# Use hardware acceleration (Intel Quick Sync)
ffmpeg -hwaccel qsv -i input.mp4 -c:v h264_qsv output.mp4

# NVIDIA GPU acceleration
ffmpeg -hwaccel cuda -i input.mp4 -c:v h264_nvenc output.mp4

# Multi-threaded encoding
ffmpeg -i input.mp4 -threads 8 -c:v libx264 output.mp4
```

### Storage Backends Comparison

Comprehensive comparison of supported storage providers:

| Provider | Egress Cost | Storage Cost | Read Latency | Write Latency | Best For |
|----------|-------------|--------------|--------------|---------------|----------|
| **MinIO** | Free (self-hosted) | Hardware cost | <10ms (LAN) | <15ms (LAN) | Development, on-premise |
| **AWS S3** | $0.09/GB | $0.023/GB/mo | 50-100ms | 100-200ms | Enterprise, high traffic |
| **Cloudflare R2** | Free | $0.015/GB/mo | 30-80ms | 80-150ms | Public assets, global CDN |
| **Backblaze B2** | Free (3x storage) | $0.005/GB/mo | 100-200ms | 150-300ms | Backups, archives |
| **Google Cloud Storage** | $0.12/GB | $0.020/GB/mo | 40-90ms | 90-180ms | ML pipelines, analytics |
| **Azure Blob** | $0.087/GB | $0.018/GB/mo | 60-120ms | 120-250ms | Microsoft ecosystem |

#### Detailed Provider Configuration

**MinIO (Self-Hosted S3-Compatible)**
```bash
# Best for: Local development, on-premise deployments
# Pros: No egress costs, full control, S3 API compatible
# Cons: Requires infrastructure management

FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_BUCKET=file-processing
FILE_STORAGE_ACCESS_KEY=minioadmin
FILE_STORAGE_SECRET_KEY=minioadmin
FILE_STORAGE_REGION=us-east-1

# Docker deployment
docker run -p 9000:9000 -p 9001:9001 \
  -e "MINIO_ROOT_USER=minioadmin" \
  -e "MINIO_ROOT_PASSWORD=minioadmin" \
  -v /data:/data \
  quay.io/minio/minio server /data --console-address ":9001"
```

**AWS S3 (Industry Standard)**
```bash
# Best for: Enterprise applications, high availability
# Pros: 99.999999999% durability, global infrastructure
# Cons: Egress costs, complexity

FILE_STORAGE_PROVIDER=s3
FILE_STORAGE_BUCKET=my-files-production
FILE_STORAGE_REGION=us-east-1
FILE_STORAGE_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
FILE_STORAGE_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

# Performance optimizations
# - Use Transfer Acceleration for global uploads
# - Enable S3 Intelligent-Tiering for cost optimization
# - Use CloudFront CDN for static assets
```

**Cloudflare R2 (Zero Egress Fees)**
```bash
# Best for: Public assets, user uploads, CDN-backed files
# Pros: Free egress, lower storage costs, global edge network
# Cons: Newer platform, fewer features than S3

FILE_STORAGE_PROVIDER=r2
FILE_STORAGE_ENDPOINT=https://[account-id].r2.cloudflarestorage.com
FILE_STORAGE_BUCKET=file-processing
FILE_STORAGE_ACCESS_KEY=your_r2_access_key
FILE_STORAGE_SECRET_KEY=your_r2_secret_key

# R2 Custom Domains (free public access)
# 1. Add custom domain in Cloudflare dashboard
# 2. Access via: https://files.yourdomain.com/thumbnails/image.jpg
# 3. No egress fees, no authentication needed for public buckets
```

**Backblaze B2 (Lowest Cost)**
```bash
# Best for: Backups, archives, infrequent access
# Pros: Cheapest storage, free egress (3x storage amount)
# Cons: Slower performance, limited regions

FILE_STORAGE_PROVIDER=b2
FILE_STORAGE_ENDPOINT=https://s3.us-west-000.backblazeb2.com
FILE_STORAGE_BUCKET=file-processing
FILE_STORAGE_ACCESS_KEY=your_key_id
FILE_STORAGE_SECRET_KEY=your_application_key
FILE_STORAGE_REGION=us-west-000

# Cost example:
# 1TB storage = $5/mo
# 3TB egress = Free
# Additional egress = $0.01/GB
```

**Google Cloud Storage (ML & Analytics)**
```bash
# Best for: Machine learning pipelines, BigQuery integration
# Pros: Integration with GCP services, strong in APAC
# Cons: Egress costs, requires service account

FILE_STORAGE_PROVIDER=gcs
FILE_STORAGE_BUCKET=file-processing-prod
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json

# Storage classes:
# - Standard: Frequent access, $0.020/GB/mo
# - Nearline: Once per month, $0.010/GB/mo
# - Coldline: Once per quarter, $0.004/GB/mo
# - Archive: Once per year, $0.0012/GB/mo
```

**Azure Blob Storage (Microsoft Ecosystem)**
```bash
# Best for: Azure-native applications, .NET shops
# Pros: Integration with Azure services, global presence
# Cons: Complexity, pricing structure

FILE_STORAGE_PROVIDER=azure
FILE_STORAGE_BUCKET=fileprocessing
AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=https;AccountName=myaccount;AccountKey=mykey;EndpointSuffix=core.windows.net"

# Access tiers:
# - Hot: Frequent access, $0.018/GB/mo
# - Cool: Infrequent access, $0.010/GB/mo
# - Archive: Rare access, $0.00099/GB/mo
```

### Performance Tuning Recommendations

#### Concurrency Settings

```bash
# Low-end server (2 cores, 4GB RAM)
FILE_QUEUE_CONCURRENCY=2
sharp.concurrency(2)

# Mid-range server (4 cores, 8GB RAM)
FILE_QUEUE_CONCURRENCY=4
sharp.concurrency(4)

# High-end server (8+ cores, 16GB+ RAM)
FILE_QUEUE_CONCURRENCY=8
sharp.concurrency(8)
```

#### Queue Priority Strategy

```javascript
// Priority levels
const PRIORITY = {
  CRITICAL: 10,   // User-uploaded profile pictures
  HIGH: 7,        // Product images
  NORMAL: 5,      // General uploads
  LOW: 3,         // Bulk imports
  BACKGROUND: 1   // Historical data backfill
};

// Create job with priority
await createJob({
  fileId: 'profile_pic_123',
  priority: PRIORITY.CRITICAL,
  operations: ['thumbnail', 'optimize']
});
```

#### Caching Strategy

```javascript
// Redis caching for processed results
const CACHE_TTL = {
  THUMBNAIL: 7 * 24 * 3600,    // 7 days
  METADATA: 30 * 24 * 3600,    // 30 days
  SCAN_RESULT: 90 * 24 * 3600  // 90 days
};

// Cache thumbnails for frequently accessed files
await redis.setex(
  `thumbnail:${fileId}:${size}`,
  CACHE_TTL.THUMBNAIL,
  thumbnailUrl
);
```

#### Storage Access Patterns

```javascript
// Use signed URLs for temporary access (no auth needed)
const signedUrl = await storage.getSignedUrl({
  bucket: 'file-processing',
  key: 'thumbnails/thumb_400.jpg',
  expiresIn: 3600  // 1 hour
});

// Batch operations for better performance
const files = await storage.listObjects({
  prefix: 'uploads/2026/01/',
  maxKeys: 1000
});

// Parallel uploads with multipart for large files
await storage.uploadLarge({
  file: largeFile,
  partSize: 5 * 1024 * 1024,  // 5MB parts
  concurrency: 4
});
```

---

## Security Notes

### File Validation

Comprehensive file validation prevents malicious uploads and processing errors.

#### MIME Type Validation

```javascript
// Strict MIME type whitelist
const ALLOWED_MIME_TYPES = {
  images: [
    'image/jpeg',
    'image/png',
    'image/gif',
    'image/webp',
    'image/avif',
    'image/tiff',
    'image/svg+xml'
  ],
  videos: [
    'video/mp4',
    'video/quicktime',
    'video/x-msvideo',
    'video/webm',
    'video/x-matroska'
  ],
  documents: [
    'application/pdf',
    'application/msword',
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
  ]
};

function validateMimeType(mimeType: string, category: string): boolean {
  return ALLOWED_MIME_TYPES[category]?.includes(mimeType) ?? false;
}

// Magic number validation (file signature)
async function verifyFileSignature(buffer: Buffer, expectedMime: string): Promise<boolean> {
  const signatures = {
    'image/jpeg': [0xFF, 0xD8, 0xFF],
    'image/png': [0x89, 0x50, 0x4E, 0x47],
    'image/gif': [0x47, 0x49, 0x46, 0x38],
    'image/webp': [0x52, 0x49, 0x46, 0x46], // RIFF
    'video/mp4': [0x00, 0x00, 0x00, null, 0x66, 0x74, 0x79, 0x70],
    'application/pdf': [0x25, 0x50, 0x44, 0x46]
  };

  const signature = signatures[expectedMime];
  if (!signature) return false;

  return signature.every((byte, i) =>
    byte === null || buffer[i] === byte
  );
}
```

#### File Size Limits

```javascript
// Configure size limits by file type
const SIZE_LIMITS = {
  'image/jpeg': 50 * 1024 * 1024,    // 50MB
  'image/png': 25 * 1024 * 1024,     // 25MB
  'video/mp4': 500 * 1024 * 1024,    // 500MB
  'application/pdf': 100 * 1024 * 1024  // 100MB
};

function validateFileSize(size: number, mimeType: string): boolean {
  const limit = SIZE_LIMITS[mimeType] ?? 10 * 1024 * 1024; // Default 10MB
  return size <= limit;
}

// Check dimensions for images
async function validateImageDimensions(buffer: Buffer): Promise<boolean> {
  const metadata = await sharp(buffer).metadata();

  const MAX_DIMENSION = 16000; // Prevent decompression bombs
  const MAX_PIXELS = 100_000_000; // 100 megapixels

  if (!metadata.width || !metadata.height) return false;

  return metadata.width <= MAX_DIMENSION &&
         metadata.height <= MAX_DIMENSION &&
         metadata.width * metadata.height <= MAX_PIXELS;
}
```

#### Content Security Policy

```javascript
// Sanitize SVG files to prevent XSS
import { DOMParser } from 'xmldom';

function sanitizeSVG(svgContent: string): string {
  const parser = new DOMParser();
  const doc = parser.parseFromString(svgContent, 'image/svg+xml');

  // Remove dangerous elements
  const dangerousElements = ['script', 'iframe', 'object', 'embed', 'foreignObject'];
  dangerousElements.forEach(tag => {
    const elements = doc.getElementsByTagName(tag);
    while (elements.length > 0) {
      elements[0].parentNode?.removeChild(elements[0]);
    }
  });

  // Remove event handlers
  const allElements = doc.getElementsByTagName('*');
  for (let i = 0; i < allElements.length; i++) {
    const element = allElements[i];
    const attributes = element.attributes;

    for (let j = attributes.length - 1; j >= 0; j--) {
      const attr = attributes[j];
      if (attr.name.startsWith('on')) {
        element.removeAttribute(attr.name);
      }
    }
  }

  return doc.toString();
}
```

### Virus Scanning with ClamAV

#### Installation & Setup

```bash
# macOS
brew install clamav
brew services start clamav

# Update virus definitions
freshclam

# Linux (Debian/Ubuntu)
sudo apt-get install clamav clamav-daemon
sudo systemctl start clamav-daemon
sudo freshclam

# Docker
docker run -d --name clamav \
  -p 3310:3310 \
  clamav/clamav:latest
```

#### Configuration

```bash
# ClamAV daemon configuration (/etc/clamav/clamd.conf)
TCPSocket 3310
TCPAddr 127.0.0.1
MaxThreads 12
MaxConnectionQueueLength 30
StreamMaxLength 100M

# Performance tuning
MaxScanSize 500M
MaxFileSize 100M
MaxRecursion 10
MaxFiles 10000
```

#### Integration Example

```typescript
import NodeClam from 'clamscan';

// Initialize ClamAV scanner
const clamscan = await new NodeClam().init({
  clamdscan: {
    host: process.env.CLAMAV_HOST || 'localhost',
    port: parseInt(process.env.CLAMAV_PORT || '3310'),
    timeout: 60000,
    multiscan: true
  },
  preference: 'clamdscan'
});

// Scan file
async function scanFile(filePath: string): Promise<ScanResult> {
  const startTime = Date.now();

  try {
    const { isInfected, viruses } = await clamscan.isInfected(filePath);

    return {
      clean: !isInfected,
      threats: viruses || [],
      scanner: 'clamav',
      durationMs: Date.now() - startTime,
      scannedAt: new Date()
    };
  } catch (error) {
    throw new Error(`Scan failed: ${error.message}`);
  }
}

// Scan buffer (for in-memory files)
async function scanBuffer(buffer: Buffer): Promise<ScanResult> {
  const stream = Readable.from(buffer);
  const { isInfected, viruses } = await clamscan.scanStream(stream);

  return {
    clean: !isInfected,
    threats: viruses || []
  };
}
```

#### Quarantine Management

```typescript
// Quarantine infected files
async function quarantineFile(fileId: string, threats: string[]): Promise<void> {
  // Move to quarantine bucket
  await storage.moveObject({
    sourceBucket: 'file-processing',
    sourceKey: `uploads/${fileId}`,
    destBucket: 'quarantine',
    destKey: `infected/${fileId}_${Date.now()}`
  });

  // Log security incident
  await db.execute(
    `INSERT INTO security_incidents (file_id, threat_type, action, created_at)
     VALUES ($1, $2, 'quarantined', NOW())`,
    [fileId, JSON.stringify(threats)]
  );

  // Send alert
  await sendSecurityAlert({
    severity: 'HIGH',
    type: 'MALWARE_DETECTED',
    fileId,
    threats
  });
}

// Periodic quarantine cleanup (delete after 90 days)
async function cleanupQuarantine(): Promise<void> {
  const files = await storage.listObjects({
    bucket: 'quarantine',
    prefix: 'infected/'
  });

  const cutoffDate = new Date();
  cutoffDate.setDate(cutoffDate.getDate() - 90);

  for (const file of files) {
    if (file.lastModified < cutoffDate) {
      await storage.deleteObject({
        bucket: 'quarantine',
        key: file.key
      });
    }
  }
}
```

### Access Control

#### API Authentication

```typescript
// JWT-based authentication
import jwt from 'jsonwebtoken';

interface AuthToken {
  userId: string;
  permissions: string[];
  expiresAt: number;
}

function generateToken(userId: string, permissions: string[]): string {
  return jwt.sign(
    { userId, permissions },
    process.env.JWT_SECRET!,
    { expiresIn: '24h' }
  );
}

// Fastify authentication hook
fastify.addHook('preHandler', async (request, reply) => {
  const authHeader = request.headers.authorization;

  if (!authHeader?.startsWith('Bearer ')) {
    return reply.code(401).send({ error: 'Missing authorization' });
  }

  try {
    const token = authHeader.substring(7);
    const decoded = jwt.verify(token, process.env.JWT_SECRET!) as AuthToken;
    request.user = decoded;
  } catch (error) {
    return reply.code(401).send({ error: 'Invalid token' });
  }
});

// Permission-based access control
function requirePermission(permission: string) {
  return async (request: FastifyRequest, reply: FastifyReply) => {
    if (!request.user?.permissions.includes(permission)) {
      return reply.code(403).send({ error: 'Insufficient permissions' });
    }
  };
}

// Usage
fastify.post('/api/jobs', {
  preHandler: requirePermission('files:create')
}, async (request, reply) => {
  // Create processing job
});
```

#### Storage Access Control

```typescript
// Signed URLs for temporary access
async function generateSignedUrl(
  fileId: string,
  expiresIn: number = 3600
): Promise<string> {
  const params = {
    Bucket: process.env.FILE_STORAGE_BUCKET,
    Key: `processed/${fileId}`,
    Expires: expiresIn
  };

  return await storage.getSignedUrl('getObject', params);
}

// Pre-signed POST for direct uploads
async function generateUploadUrl(
  fileId: string,
  contentType: string
): Promise<{ url: string; fields: Record<string, string> }> {
  const params = {
    Bucket: process.env.FILE_STORAGE_BUCKET,
    Fields: {
      key: `uploads/${fileId}`,
      'Content-Type': contentType
    },
    Conditions: [
      ['content-length-range', 0, 104857600], // Max 100MB
      ['starts-with', '$Content-Type', 'image/']
    ],
    Expires: 3600
  };

  return await storage.createPresignedPost(params);
}
```

#### Rate Limiting

```typescript
import rateLimit from '@fastify/rate-limit';

// Register rate limiter
await fastify.register(rateLimit, {
  max: 100,              // Max requests
  timeWindow: '15m',     // Per 15 minutes
  redis: redisClient,
  keyGenerator: (request) => {
    return request.user?.userId || request.ip;
  },
  errorResponseBuilder: (request, context) => {
    return {
      error: 'Rate limit exceeded',
      retryAfter: context.ttl
    };
  }
});

// Per-endpoint limits
fastify.post('/api/jobs', {
  config: {
    rateLimit: {
      max: 20,
      timeWindow: '1m'
    }
  }
}, async (request, reply) => {
  // Handle job creation
});
```

#### Audit Logging

```typescript
// Log all file operations
async function auditLog(event: AuditEvent): Promise<void> {
  await db.execute(
    `INSERT INTO audit_logs (
      user_id, action, resource_type, resource_id,
      ip_address, user_agent, metadata, created_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
    [
      event.userId,
      event.action,
      event.resourceType,
      event.resourceId,
      event.ipAddress,
      event.userAgent,
      JSON.stringify(event.metadata)
    ]
  );
}

// Audit middleware
fastify.addHook('onResponse', async (request, reply) => {
  if (request.method !== 'GET') {
    await auditLog({
      userId: request.user?.userId,
      action: `${request.method} ${request.url}`,
      resourceType: 'file',
      resourceId: request.params.fileId,
      ipAddress: request.ip,
      userAgent: request.headers['user-agent'],
      metadata: {
        statusCode: reply.statusCode,
        responseTime: reply.getResponseTime()
      }
    });
  }
});
```

---

## Advanced Code Examples

### Image Optimization Recipes

#### Responsive Image Generation

```typescript
// Generate responsive image set with srcset
async function generateResponsiveImages(
  inputPath: string,
  outputPrefix: string
): Promise<ResponsiveImageSet> {
  const widths = [320, 640, 960, 1280, 1920, 2560];
  const formats = ['webp', 'jpeg'];

  const images: ImageVariant[] = [];

  for (const width of widths) {
    for (const format of formats) {
      const outputPath = `${outputPrefix}_${width}.${format}`;

      await sharp(inputPath)
        .resize(width, null, {
          withoutEnlargement: true,
          fit: 'inside'
        })
        .toFormat(format, {
          quality: format === 'webp' ? 80 : 85,
          progressive: true
        })
        .toFile(outputPath);

      const { size } = await fs.stat(outputPath);

      images.push({
        url: outputPath,
        width,
        format,
        size
      });
    }
  }

  return {
    images,
    srcset: generateSrcSet(images)
  };
}

function generateSrcSet(images: ImageVariant[]): string {
  return images
    .filter(img => img.format === 'webp')
    .map(img => `${img.url} ${img.width}w`)
    .join(', ');
}

// HTML usage
// <picture>
//   <source type="image/webp" srcset="${srcset}">
//   <img src="${fallback}" alt="...">
// </picture>
```

#### Smart Cropping with Face Detection

```typescript
import sharp from 'sharp';

// Attention-based smart crop (entropy detection)
async function smartCrop(
  inputPath: string,
  width: number,
  height: number
): Promise<Buffer> {
  return await sharp(inputPath)
    .resize(width, height, {
      fit: 'cover',
      position: 'attention' // Focuses on high-entropy regions
    })
    .toBuffer();
}

// Manual face detection crop (using metadata)
async function cropToFace(
  inputPath: string,
  faceCoordinates: { x: number; y: number; width: number; height: number }
): Promise<Buffer> {
  // Add padding around face
  const padding = 0.3; // 30% padding
  const paddedWidth = Math.floor(faceCoordinates.width * (1 + padding));
  const paddedHeight = Math.floor(faceCoordinates.height * (1 + padding));

  const left = Math.max(0, faceCoordinates.x - Math.floor(paddedWidth * padding / 2));
  const top = Math.max(0, faceCoordinates.y - Math.floor(paddedHeight * padding / 2));

  return await sharp(inputPath)
    .extract({
      left,
      top,
      width: paddedWidth,
      height: paddedHeight
    })
    .resize(400, 400, { fit: 'cover' })
    .toBuffer();
}
```

#### Image Compression Quality Optimization

```typescript
// Find optimal JPEG quality (balance size vs quality)
async function findOptimalQuality(
  inputBuffer: Buffer,
  targetSizeKB: number = 100
): Promise<{ quality: number; size: number; buffer: Buffer }> {
  let minQuality = 50;
  let maxQuality = 95;
  let bestResult = null;

  while (minQuality <= maxQuality) {
    const quality = Math.floor((minQuality + maxQuality) / 2);

    const buffer = await sharp(inputBuffer)
      .jpeg({ quality, progressive: true })
      .toBuffer();

    const sizeKB = buffer.length / 1024;

    if (Math.abs(sizeKB - targetSizeKB) < 5) {
      return { quality, size: buffer.length, buffer };
    }

    if (sizeKB > targetSizeKB) {
      maxQuality = quality - 1;
    } else {
      minQuality = quality + 1;
      bestResult = { quality, size: buffer.length, buffer };
    }
  }

  return bestResult!;
}

// Progressive enhancement compression
async function progressiveCompress(inputPath: string): Promise<Buffer> {
  const metadata = await sharp(inputPath).metadata();

  // Choose format based on image characteristics
  if (metadata.hasAlpha) {
    // PNG or WebP for transparency
    return await sharp(inputPath)
      .webp({ quality: 90, lossless: false })
      .toBuffer();
  }

  if ((metadata.width || 0) * (metadata.height || 0) > 4_000_000) {
    // Large images: aggressive compression
    return await sharp(inputPath)
      .jpeg({ quality: 75, progressive: true, mozjpeg: true })
      .toBuffer();
  }

  // Standard compression
  return await sharp(inputPath)
    .jpeg({ quality: 85, progressive: true })
    .toBuffer();
}
```

### Video Processing Examples

#### Video Thumbnail Generation

```typescript
import { exec } from 'child_process';
import { promisify } from 'util';

const execAsync = promisify(exec);

// Extract thumbnail from middle of video
async function extractVideoThumbnail(
  videoPath: string,
  outputPath: string
): Promise<string> {
  // Get video duration
  const { stdout: durationStr } = await execAsync(
    `ffprobe -v error -show_entries format=duration ` +
    `-of default=noprint_wrappers=1:nokey=1 "${videoPath}"`
  );

  const duration = parseFloat(durationStr);
  const middleTime = duration / 2;

  // Extract frame from middle
  await execAsync(
    `ffmpeg -ss ${middleTime} -i "${videoPath}" -vframes 1 ` +
    `-vf "scale=1280:720:force_original_aspect_ratio=decrease" ` +
    `-q:v 2 "${outputPath}"`
  );

  return outputPath;
}

// Generate animated thumbnail (GIF)
async function generateAnimatedThumbnail(
  videoPath: string,
  outputPath: string,
  duration: number = 3
): Promise<string> {
  await execAsync(
    `ffmpeg -i "${videoPath}" -t ${duration} ` +
    `-vf "fps=10,scale=480:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" ` +
    `-loop 0 "${outputPath}"`
  );

  return outputPath;
}

// Generate sprite sheet (all thumbnails in one image)
async function generateSpriteSheet(
  videoPath: string,
  outputPath: string,
  interval: number = 10
): Promise<string> {
  await execAsync(
    `ffmpeg -i "${videoPath}" ` +
    `-vf "fps=1/${interval},scale=160:90,tile=10x10" ` +
    `"${outputPath}"`
  );

  return outputPath;
}
```

#### Video Optimization

```typescript
// Transcode video for web delivery
async function optimizeVideoForWeb(
  inputPath: string,
  outputPath: string,
  options: VideoOptimizationOptions = {}
): Promise<VideoMetadata> {
  const {
    maxWidth = 1920,
    maxHeight = 1080,
    targetBitrate = '2M',
    audioCodec = 'aac',
    audioBitrate = '128k'
  } = options;

  // Build ffmpeg command
  const command = [
    'ffmpeg',
    '-i', `"${inputPath}"`,
    '-c:v', 'libx264',
    '-preset', 'medium',
    '-crf', '23',
    '-maxrate', targetBitrate,
    '-bufsize', `${parseInt(targetBitrate) * 2}k`,
    `-vf "scale='min(${maxWidth},iw)':'min(${maxHeight},ih)':force_original_aspect_ratio=decrease"`,
    '-c:a', audioCodec,
    '-b:a', audioBitrate,
    '-movflags', '+faststart',
    '-y',
    `"${outputPath}"`
  ].join(' ');

  await execAsync(command);

  // Get output metadata
  const { stdout } = await execAsync(
    `ffprobe -v quiet -print_format json -show_format -show_streams "${outputPath}"`
  );

  const metadata = JSON.parse(stdout);
  const videoStream = metadata.streams.find((s: any) => s.codec_type === 'video');

  return {
    width: videoStream.width,
    height: videoStream.height,
    duration: parseFloat(metadata.format.duration),
    size: parseInt(metadata.format.size),
    codec: videoStream.codec_name,
    bitrate: parseInt(metadata.format.bit_rate)
  };
}

// Generate multiple video qualities (HLS/DASH)
async function generateMultiQualityVideo(
  inputPath: string,
  outputDir: string
): Promise<VideoQuality[]> {
  const qualities = [
    { name: '1080p', width: 1920, height: 1080, bitrate: '5000k' },
    { name: '720p', width: 1280, height: 720, bitrate: '2500k' },
    { name: '480p', width: 854, height: 480, bitrate: '1000k' },
    { name: '360p', width: 640, height: 360, bitrate: '500k' }
  ];

  const results: VideoQuality[] = [];

  for (const quality of qualities) {
    const outputPath = `${outputDir}/${quality.name}.mp4`;

    await execAsync(
      `ffmpeg -i "${inputPath}" ` +
      `-c:v libx264 -preset medium -crf 23 ` +
      `-maxrate ${quality.bitrate} -bufsize ${parseInt(quality.bitrate) * 2}k ` +
      `-vf "scale=${quality.width}:${quality.height}" ` +
      `-c:a aac -b:a 128k -movflags +faststart ` +
      `-y "${outputPath}"`
    );

    const { size } = await fs.stat(outputPath);

    results.push({
      name: quality.name,
      path: outputPath,
      width: quality.width,
      height: quality.height,
      size
    });
  }

  return results;
}
```

### Watermarking

#### Image Watermark

```typescript
// Add text watermark
async function addTextWatermark(
  inputPath: string,
  outputPath: string,
  text: string,
  options: WatermarkOptions = {}
): Promise<string> {
  const {
    position = 'southeast',
    opacity = 0.3,
    fontSize = 48,
    color = 'white'
  } = options;

  // Create text SVG
  const textSvg = `
    <svg width="500" height="100">
      <text
        x="250"
        y="50"
        text-anchor="middle"
        font-size="${fontSize}"
        font-family="Arial"
        fill="${color}"
        opacity="${opacity}"
      >${text}</text>
    </svg>
  `;

  const textBuffer = Buffer.from(textSvg);

  // Composite watermark
  await sharp(inputPath)
    .composite([{
      input: textBuffer,
      gravity: position
    }])
    .toFile(outputPath);

  return outputPath;
}

// Add logo watermark
async function addLogoWatermark(
  inputPath: string,
  logoPath: string,
  outputPath: string,
  options: LogoWatermarkOptions = {}
): Promise<string> {
  const {
    position = 'southeast',
    opacity = 0.7,
    scale = 0.1, // 10% of image width
    margin = 20
  } = options;

  // Get input dimensions
  const input = sharp(inputPath);
  const { width: inputWidth } = await input.metadata();

  // Calculate logo size
  const logoWidth = Math.floor((inputWidth || 1000) * scale);

  // Resize and adjust opacity of logo
  const logoBuffer = await sharp(logoPath)
    .resize(logoWidth, null, { withoutEnlargement: true })
    .composite([{
      input: Buffer.from([255, 255, 255, Math.floor(255 * opacity)]),
      raw: { width: 1, height: 1, channels: 4 },
      tile: true,
      blend: 'dest-in'
    }])
    .toBuffer();

  // Composite logo onto image
  await input
    .composite([{
      input: logoBuffer,
      gravity: position,
      blend: 'over'
    }])
    .toFile(outputPath);

  return outputPath;
}
```

#### Video Watermark

```typescript
// Add watermark to video
async function addVideoWatermark(
  videoPath: string,
  watermarkPath: string,
  outputPath: string,
  position: 'topleft' | 'topright' | 'bottomleft' | 'bottomright' = 'bottomright'
): Promise<string> {
  const positions = {
    topleft: '10:10',
    topright: 'main_w-overlay_w-10:10',
    bottomleft: '10:main_h-overlay_h-10',
    bottomright: 'main_w-overlay_w-10:main_h-overlay_h-10'
  };

  await execAsync(
    `ffmpeg -i "${videoPath}" -i "${watermarkPath}" ` +
    `-filter_complex "[1:v]scale=iw*0.2:-1[wm];[0:v][wm]overlay=${positions[position]}" ` +
    `-c:a copy -y "${outputPath}"`
  );

  return outputPath;
}
```

### Batch Processing Pipeline

```typescript
// Process multiple files with progress tracking
async function batchProcessFiles(
  fileIds: string[],
  operations: string[],
  progressCallback?: (progress: BatchProgress) => void
): Promise<BatchResult> {
  const results: ProcessingResult[] = [];
  let completed = 0;
  let failed = 0;

  const queue = new PQueue({ concurrency: 4 });

  for (const fileId of fileIds) {
    queue.add(async () => {
      try {
        const result = await processFile(fileId, operations);
        results.push(result);
        completed++;
      } catch (error) {
        failed++;
        results.push({
          fileId,
          status: 'failed',
          error: error.message
        });
      }

      if (progressCallback) {
        progressCallback({
          total: fileIds.length,
          completed: completed + failed,
          successful: completed,
          failed,
          percentage: ((completed + failed) / fileIds.length) * 100
        });
      }
    });
  }

  await queue.onIdle();

  return {
    total: fileIds.length,
    successful: completed,
    failed,
    results
  };
}
```

---

## Monitoring & Alerting

### Processing Queue Metrics

#### Queue Depth Monitoring

```sql
-- Real-time queue depth by status
CREATE VIEW file_queue_depth AS
SELECT
    status,
    COUNT(*) AS count,
    MIN(created_at) AS oldest_job,
    MAX(created_at) AS newest_job,
    EXTRACT(EPOCH FROM (NOW() - MIN(created_at))) AS oldest_age_seconds
FROM file_processing_jobs
WHERE status IN ('pending', 'processing')
GROUP BY status;

-- Queue wait times (percentiles)
CREATE VIEW file_queue_wait_times AS
SELECT
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (started_at - created_at))) AS p50_wait_seconds,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (started_at - created_at))) AS p95_wait_seconds,
    PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (started_at - created_at))) AS p99_wait_seconds
FROM file_processing_jobs
WHERE started_at IS NOT NULL
  AND created_at > NOW() - INTERVAL '24 hours';

-- Processing throughput (jobs per hour)
CREATE VIEW file_processing_throughput AS
SELECT
    DATE_TRUNC('hour', completed_at) AS hour,
    COUNT(*) AS jobs_completed,
    AVG(duration_ms) AS avg_duration_ms,
    SUM(file_size) AS total_bytes_processed
FROM file_processing_jobs
WHERE status = 'completed'
  AND completed_at > NOW() - INTERVAL '7 days'
GROUP BY DATE_TRUNC('hour', completed_at)
ORDER BY hour DESC;
```

#### Queue Health Checks

```typescript
// Monitor queue health
async function checkQueueHealth(): Promise<QueueHealth> {
  const [depth, waitTimes, throughput] = await Promise.all([
    db.query('SELECT * FROM file_queue_depth'),
    db.query('SELECT * FROM file_queue_wait_times'),
    db.query('SELECT * FROM file_processing_throughput WHERE hour > NOW() - INTERVAL \'1 hour\'')
  ]);

  const pendingCount = depth.rows.find(r => r.status === 'pending')?.count || 0;
  const processingCount = depth.rows.find(r => r.status === 'processing')?.count || 0;
  const oldestAge = depth.rows.find(r => r.status === 'pending')?.oldest_age_seconds || 0;

  // Health thresholds
  const healthy =
    pendingCount < 1000 &&           // Less than 1000 pending
    processingCount < 50 &&           // Less than 50 processing
    oldestAge < 300 &&                // Oldest job < 5 minutes
    (waitTimes.rows[0]?.p95_wait_seconds || 0) < 60;  // P95 wait < 1 minute

  return {
    healthy,
    metrics: {
      pendingCount,
      processingCount,
      oldestJobAgeSeconds: oldestAge,
      p95WaitSeconds: waitTimes.rows[0]?.p95_wait_seconds || 0,
      throughputLastHour: throughput.rows[0]?.jobs_completed || 0
    },
    alerts: [
      ...(pendingCount > 1000 ? ['Queue backlog: pending jobs exceeds 1000'] : []),
      ...(oldestAge > 300 ? ['Job age alert: oldest job waiting over 5 minutes'] : []),
      ...((waitTimes.rows[0]?.p95_wait_seconds || 0) > 60 ? ['Wait time alert: P95 exceeds 1 minute'] : [])
    ]
  };
}

// Automated alerting
async function monitorQueue(): Promise<void> {
  const health = await checkQueueHealth();

  if (!health.healthy) {
    await sendAlert({
      severity: 'warning',
      title: 'File Processing Queue Degraded',
      message: health.alerts.join(', '),
      metrics: health.metrics
    });
  }
}

// Run every minute
setInterval(monitorQueue, 60000);
```

### Storage Usage Tracking

#### Storage Metrics

```sql
-- Storage usage by file type
CREATE VIEW file_storage_by_type AS
SELECT
    mime_type,
    COUNT(*) AS file_count,
    SUM(file_size) AS total_bytes,
    ROUND(SUM(file_size) / 1024.0 / 1024.0 / 1024.0, 2) AS total_gb,
    AVG(file_size) AS avg_file_size
FROM file_processing_jobs
WHERE status = 'completed'
GROUP BY mime_type
ORDER BY total_bytes DESC;

-- Thumbnail storage usage
CREATE VIEW thumbnail_storage_usage AS
SELECT
    width,
    height,
    format,
    COUNT(*) AS thumbnail_count,
    SUM(size) AS total_bytes,
    ROUND(SUM(size) / 1024.0 / 1024.0, 2) AS total_mb
FROM file_thumbnails
GROUP BY width, height, format
ORDER BY total_bytes DESC;

-- Daily storage growth
CREATE VIEW storage_growth_daily AS
SELECT
    DATE(created_at) AS date,
    SUM(file_size) AS original_bytes,
    SUM((SELECT SUM(size) FROM file_thumbnails t WHERE t.job_id = j.id)) AS thumbnail_bytes,
    SUM(file_size) + COALESCE(SUM((SELECT SUM(size) FROM file_thumbnails t WHERE t.job_id = j.id)), 0) AS total_bytes
FROM file_processing_jobs j
WHERE status = 'completed'
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- Storage utilization forecast (30-day projection)
CREATE VIEW storage_forecast AS
WITH recent_growth AS (
    SELECT AVG(total_bytes) AS avg_daily_growth
    FROM storage_growth_daily
    WHERE date > CURRENT_DATE - INTERVAL '7 days'
)
SELECT
    (SELECT SUM(total_bytes) FROM storage_growth_daily) AS current_total,
    (SELECT avg_daily_growth FROM recent_growth) AS daily_growth_rate,
    (SELECT avg_daily_growth FROM recent_growth) * 30 AS projected_30day_growth,
    (SELECT SUM(total_bytes) FROM storage_growth_daily) + ((SELECT avg_daily_growth FROM recent_growth) * 30) AS projected_total;
```

#### Storage Alerts

```typescript
// Monitor storage usage
async function checkStorageUsage(): Promise<StorageAlert[]> {
  const alerts: StorageAlert[] = [];

  // Check total storage
  const { rows: [usage] } = await db.query(`
    SELECT
      SUM(file_size) AS total_bytes,
      COUNT(*) AS file_count
    FROM file_processing_jobs
    WHERE status = 'completed'
  `);

  const totalGB = usage.total_bytes / 1024 / 1024 / 1024;
  const STORAGE_LIMIT_GB = 1000;

  if (totalGB > STORAGE_LIMIT_GB * 0.9) {
    alerts.push({
      severity: 'critical',
      message: `Storage usage: ${totalGB.toFixed(2)}GB / ${STORAGE_LIMIT_GB}GB (${((totalGB / STORAGE_LIMIT_GB) * 100).toFixed(1)}%)`,
      recommendation: 'Clean up old files or increase storage quota'
    });
  }

  // Check growth rate
  const { rows: [forecast] } = await db.query('SELECT * FROM storage_forecast');
  const projectedGB = forecast.projected_total / 1024 / 1024 / 1024;

  if (projectedGB > STORAGE_LIMIT_GB) {
    alerts.push({
      severity: 'warning',
      message: `Projected storage in 30 days: ${projectedGB.toFixed(2)}GB exceeds limit`,
      recommendation: 'Review retention policy and implement cleanup strategy'
    });
  }

  return alerts;
}

// Automated cleanup based on retention policy
async function cleanupOldFiles(retentionDays: number = 90): Promise<CleanupResult> {
  const cutoffDate = new Date();
  cutoffDate.setDate(cutoffDate.getDate() - retentionDays);

  // Find old completed jobs
  const { rows: oldJobs } = await db.query(
    `SELECT id, file_id, file_path
     FROM file_processing_jobs
     WHERE completed_at < $1 AND status = 'completed'`,
    [cutoffDate]
  );

  let deletedFiles = 0;
  let deletedBytes = 0;

  for (const job of oldJobs) {
    try {
      // Delete from storage
      await storage.deleteObject({
        bucket: process.env.FILE_STORAGE_BUCKET!,
        key: job.file_path
      });

      // Delete thumbnails
      const { rows: thumbnails } = await db.query(
        'SELECT storage_path FROM file_thumbnails WHERE job_id = $1',
        [job.id]
      );

      for (const thumb of thumbnails) {
        await storage.deleteObject({
          bucket: process.env.FILE_STORAGE_BUCKET!,
          key: thumb.storage_path
        });
      }

      // Delete database records
      await db.execute('DELETE FROM file_processing_jobs WHERE id = $1', [job.id]);

      deletedFiles++;
    } catch (error) {
      console.error(`Failed to delete job ${job.id}:`, error);
    }
  }

  return {
    deletedFiles,
    deletedBytes,
    retentionDays
  };
}
```

### Failure Rate Monitoring

#### Failure Metrics

```sql
-- Failure rate by error type
CREATE VIEW failure_analysis AS
SELECT
    SUBSTRING(error FROM 1 FOR 100) AS error_type,
    COUNT(*) AS occurrence_count,
    ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM file_processing_jobs WHERE status = 'failed'), 2) AS percentage,
    MIN(created_at) AS first_seen,
    MAX(created_at) AS last_seen
FROM file_processing_jobs
WHERE status = 'failed'
  AND created_at > NOW() - INTERVAL '7 days'
GROUP BY SUBSTRING(error FROM 1 FOR 100)
ORDER BY occurrence_count DESC;

-- Success rate over time
CREATE VIEW success_rate_hourly AS
SELECT
    DATE_TRUNC('hour', completed_at) AS hour,
    COUNT(*) FILTER (WHERE status = 'completed') AS successful,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'completed') / COUNT(*), 2) AS success_rate
FROM file_processing_jobs
WHERE completed_at > NOW() - INTERVAL '24 hours'
GROUP BY DATE_TRUNC('hour', completed_at)
ORDER BY hour DESC;

-- Jobs requiring retry
CREATE VIEW jobs_needing_retry AS
SELECT
    id,
    file_id,
    file_name,
    error,
    created_at,
    CASE
        WHEN error LIKE '%timeout%' THEN 'retry'
        WHEN error LIKE '%network%' THEN 'retry'
        WHEN error LIKE '%ECONNREFUSED%' THEN 'retry'
        WHEN error LIKE '%rate limit%' THEN 'retry_delayed'
        ELSE 'manual_review'
    END AS retry_recommendation
FROM file_processing_jobs
WHERE status = 'failed'
  AND created_at > NOW() - INTERVAL '24 hours'
ORDER BY created_at DESC;
```

#### Failure Alerting

```typescript
// Monitor failure rates
async function checkFailureRate(): Promise<FailureAlert | null> {
  const { rows: [stats] } = await db.query(`
    SELECT
      COUNT(*) FILTER (WHERE status = 'completed') AS successful,
      COUNT(*) FILTER (WHERE status = 'failed') AS failed,
      ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'failed') / COUNT(*), 2) AS failure_rate
    FROM file_processing_jobs
    WHERE completed_at > NOW() - INTERVAL '1 hour'
  `);

  const FAILURE_THRESHOLD = 5; // 5% failure rate

  if (stats.failure_rate > FAILURE_THRESHOLD) {
    // Get top errors
    const { rows: topErrors } = await db.query(`
      SELECT error_type, occurrence_count
      FROM failure_analysis
      LIMIT 5
    `);

    return {
      severity: stats.failure_rate > 20 ? 'critical' : 'warning',
      failureRate: stats.failure_rate,
      failed: stats.failed,
      total: stats.successful + stats.failed,
      topErrors: topErrors.map(e => ({ type: e.error_type, count: e.occurrence_count })),
      recommendation: 'Investigate recent failures and check service dependencies'
    };
  }

  return null;
}

// Automated retry for transient failures
async function retryFailedJobs(): Promise<RetryResult> {
  const { rows: retryableJobs } = await db.query(`
    SELECT id, file_id, file_path, file_name, mime_type, operations
    FROM jobs_needing_retry
    WHERE retry_recommendation = 'retry'
    LIMIT 100
  `);

  let retried = 0;
  let succeeded = 0;

  for (const job of retryableJobs) {
    try {
      // Create new job with same parameters
      await createJob({
        fileId: job.file_id,
        filePath: job.file_path,
        fileName: job.file_name,
        mimeType: job.mime_type,
        operations: job.operations,
        priority: 7 // Higher priority for retries
      });

      // Mark original as retried
      await db.execute(
        `UPDATE file_processing_jobs
         SET error = error || ' [RETRIED]'
         WHERE id = $1`,
        [job.id]
      );

      retried++;
    } catch (error) {
      console.error(`Failed to retry job ${job.id}:`, error);
    }
  }

  return { retried, succeeded };
}
```

### Performance Dashboards

#### Grafana Dashboard Queries

```sql
-- Average processing time by operation
SELECT
    operation,
    AVG(duration_ms) AS avg_ms,
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms) AS p50_ms,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95_ms,
    PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_ms) AS p99_ms
FROM (
    SELECT
        unnest(operations::text[]) AS operation,
        duration_ms
    FROM file_processing_jobs
    WHERE status = 'completed'
      AND completed_at > NOW() - INTERVAL '24 hours'
) subquery
GROUP BY operation;

-- Concurrent processing capacity
SELECT
    DATE_TRUNC('minute', started_at) AS minute,
    COUNT(*) AS concurrent_jobs,
    MAX(COUNT(*)) OVER () AS max_concurrent
FROM file_processing_jobs
WHERE status = 'processing'
  AND started_at > NOW() - INTERVAL '1 hour'
GROUP BY DATE_TRUNC('minute', started_at)
ORDER BY minute DESC;
```

---

## Use Cases

### 1. User Profile Picture Processing

**Scenario**: Social platform requiring fast profile picture uploads with multiple sizes.

**Configuration**:
```bash
FILE_THUMBNAIL_SIZES=50,100,200,400
FILE_ENABLE_OPTIMIZATION=true
FILE_STRIP_EXIF=true
FILE_MAX_SIZE=10485760  # 10MB
```

**Implementation**:
```typescript
// Upload and process profile picture
async function uploadProfilePicture(userId: string, file: File): Promise<ProfilePicture> {
  // Upload to storage
  const fileId = `profile_${userId}_${Date.now()}`;
  await storage.upload(file, `uploads/${fileId}`);

  // Create processing job
  const job = await createJob({
    fileId,
    filePath: `uploads/${fileId}`,
    fileName: file.name,
    fileSize: file.size,
    mimeType: file.type,
    operations: ['thumbnail', 'optimize', 'strip_exif'],
    priority: 10, // High priority for user-facing content
    callbackData: { userId, type: 'profile_picture' }
  });

  return {
    jobId: job.id,
    userId,
    status: 'processing'
  };
}
```

### 2. E-commerce Product Images

**Scenario**: Online store needing optimized product images in multiple formats.

**Features**:
- Responsive images (WebP + JPEG fallback)
- Watermarking for brand protection
- Zoom-friendly high-resolution versions
- Fast thumbnail generation for listings

**Implementation**:
```typescript
async function processProductImage(productId: string, imagePath: string): Promise<ProductImageSet> {
  const job = await createJob({
    fileId: `product_${productId}`,
    filePath: imagePath,
    operations: ['thumbnail', 'optimize', 'metadata', 'watermark'],
    priority: 7,
    callbackData: {
      productId,
      category: 'product_image'
    }
  });

  return {
    jobId: job.id,
    productId,
    estimatedCompletion: 30 // seconds
  };
}
```

### 3. Video Platform Thumbnails

**Scenario**: Video streaming platform generating thumbnails and preview clips.

**Requirements**:
- Extract frame at 25% mark
- Generate sprite sheet for scrubbing
- Create animated GIF preview
- Metadata extraction (duration, resolution, codec)

**Implementation**:
```typescript
async function processVideoUpload(videoId: string, videoPath: string): Promise<VideoProcessingJob> {
  return await createJob({
    fileId: videoId,
    filePath: videoPath,
    operations: ['video_thumbnail', 'sprite_sheet', 'animated_preview', 'metadata'],
    priority: 5,
    callbackData: {
      videoId,
      generatePreview: true,
      spriteInterval: 10 // seconds
    }
  });
}
```

### 4. Document Management System

**Scenario**: Enterprise document storage with virus scanning and OCR.

**Features**:
- ClamAV virus scanning
- PDF thumbnail generation
- Metadata extraction
- Secure storage with access control

**Configuration**:
```bash
FILE_ENABLE_VIRUS_SCAN=true
FILE_ENABLE_OPTIMIZATION=false
FILE_MAX_SIZE=104857600  # 100MB
CLAMAV_HOST=localhost
CLAMAV_PORT=3310
```

**Implementation**:
```typescript
async function uploadDocument(userId: string, document: File): Promise<DocumentUpload> {
  const fileId = `doc_${userId}_${Date.now()}`;

  const job = await createJob({
    fileId,
    filePath: `documents/${fileId}`,
    fileName: document.name,
    fileSize: document.size,
    mimeType: document.type,
    operations: ['virus_scan', 'thumbnail', 'metadata'],
    priority: 8,
    callbackData: {
      userId,
      documentType: 'upload'
    }
  });

  // Wait for virus scan result
  const scanResult = await waitForScanResult(job.id, 60000);

  if (!scanResult.clean) {
    await quarantineFile(fileId, scanResult.threats);
    throw new Error('Virus detected in upload');
  }

  return {
    jobId: job.id,
    fileId,
    status: 'safe'
  };
}
```

### 5. Real Estate Listing Photos

**Scenario**: Property photos requiring HDR processing and virtual staging.

**Features**:
- High-quality optimization
- Automatic horizon leveling
- Color correction
- Multiple aspect ratios (16:9, 4:3, 1:1)

**Implementation**:
```typescript
async function processListingPhoto(listingId: string, photoPath: string): Promise<ListingPhoto> {
  return await createJob({
    fileId: `listing_${listingId}_${Date.now()}`,
    filePath: photoPath,
    operations: ['optimize', 'color_correct', 'level', 'multi_aspect_thumbnails'],
    priority: 6,
    callbackData: {
      listingId,
      photoType: 'property',
      aspectRatios: ['16:9', '4:3', '1:1']
    }
  });
}
```

### 6. Social Media Content Pipeline

**Scenario**: Automated content processing for social media posts.

**Features**:
- Platform-specific sizes (Instagram, Facebook, Twitter)
- Automatic hashtag and caption OCR
- Face detection for smart cropping
- Animated GIF optimization

**Implementation**:
```typescript
async function processSocialMediaPost(postId: string, mediaPath: string, platforms: string[]): Promise<SocialMediaJob> {
  const operations = ['thumbnail', 'optimize', 'metadata'];

  if (platforms.includes('instagram')) {
    operations.push('square_crop', 'story_crop');
  }

  if (platforms.includes('twitter')) {
    operations.push('twitter_card');
  }

  return await createJob({
    fileId: `social_${postId}`,
    filePath: mediaPath,
    operations,
    priority: 9,
    callbackData: {
      postId,
      platforms,
      generateVariants: true
    }
  });
}
```

### 7. Medical Imaging Archive

**Scenario**: HIPAA-compliant medical image storage and processing.

**Features**:
- DICOM format support
- Secure storage with encryption
- Audit logging
- High-fidelity compression

**Configuration**:
```bash
FILE_STORAGE_PROVIDER=s3
FILE_ENABLE_OPTIMIZATION=false  # Preserve original quality
FILE_STRIP_EXIF=false  # Keep medical metadata
FILE_ENABLE_VIRUS_SCAN=true
FILE_MAX_SIZE=524288000  # 500MB for large scans
```

**Implementation**:
```typescript
async function uploadMedicalImage(patientId: string, imageFile: File, studyId: string): Promise<MedicalImageJob> {
  const fileId = `medical_${patientId}_${studyId}_${Date.now()}`;

  // Encrypt before upload
  const encryptedFile = await encryptFile(imageFile);

  const job = await createJob({
    fileId,
    filePath: `medical/${patientId}/${studyId}/${fileId}`,
    fileName: imageFile.name,
    fileSize: encryptedFile.size,
    mimeType: imageFile.type,
    operations: ['virus_scan', 'metadata', 'thumbnail'],
    priority: 10,
    callbackData: {
      patientId,
      studyId,
      encrypted: true,
      hipaaCompliant: true
    }
  });

  // Audit log
  await auditLog({
    action: 'MEDICAL_IMAGE_UPLOAD',
    resourceId: fileId,
    userId: patientId,
    metadata: { studyId, jobId: job.id }
  });

  return {
    jobId: job.id,
    fileId,
    studyId,
    encrypted: true
  };
}
```

### 8. Automated Marketing Assets

**Scenario**: Bulk processing of marketing materials with brand consistency.

**Features**:
- Batch processing (1000+ images)
- Watermarking with brand logo
- Color palette validation
- Output for multiple channels

**Implementation**:
```typescript
async function processCampaignAssets(campaignId: string, assets: File[]): Promise<CampaignProcessingJob> {
  const jobs = await Promise.all(
    assets.map((asset, index) =>
      createJob({
        fileId: `campaign_${campaignId}_${index}`,
        filePath: `campaigns/${campaignId}/${asset.name}`,
        fileName: asset.name,
        fileSize: asset.size,
        mimeType: asset.type,
        operations: ['optimize', 'watermark', 'color_validate', 'multi_channel_export'],
        priority: 3, // Lower priority for batch jobs
        callbackData: {
          campaignId,
          assetIndex: index,
          totalAssets: assets.length
        }
      })
    )
  );

  return {
    campaignId,
    jobIds: jobs.map(j => j.id),
    totalAssets: assets.length,
    estimatedCompletion: Math.ceil(assets.length / 4) * 30 // Based on concurrency
  };
}
```

### 9. News Article Featured Images

**Scenario**: News website with fast image processing for breaking stories.

**Features**:
- Ultra-high priority processing
- Multiple crops for different layouts
- WebP conversion for performance
- Automatic alt text generation via OCR

**Implementation**:
```typescript
async function processNewsImage(articleId: string, imagePath: string, urgency: 'breaking' | 'normal'): Promise<NewsImageJob> {
  return await createJob({
    fileId: `news_${articleId}`,
    filePath: imagePath,
    operations: ['optimize', 'webp_convert', 'multi_crop', 'ocr_alt_text'],
    priority: urgency === 'breaking' ? 10 : 6,
    callbackData: {
      articleId,
      urgency,
      layouts: ['hero', 'thumbnail', 'mobile', 'amp']
    }
  });
}
```

### 10. Cloud Storage Backup Integration

**Scenario**: Automated backup of processed files to multiple cloud providers.

**Features**:
- Multi-cloud redundancy (S3 + R2 + B2)
- Incremental backups
- Checksum verification
- Geographic distribution

**Implementation**:
```typescript
async function backupProcessedFile(fileId: string): Promise<BackupResult> {
  const file = await getFileMetadata(fileId);

  const backupProviders = [
    { name: 's3', bucket: 'backups-primary' },
    { name: 'r2', bucket: 'backups-cdn' },
    { name: 'b2', bucket: 'backups-archive' }
  ];

  const backupResults = await Promise.all(
    backupProviders.map(async (provider) => {
      const checksum = await storage.copyObject({
        sourceBucket: process.env.FILE_STORAGE_BUCKET!,
        sourceKey: file.path,
        destProvider: provider.name,
        destBucket: provider.bucket,
        destKey: `backups/${fileId}`,
        verifyChecksum: true
      });

      return {
        provider: provider.name,
        checksum,
        timestamp: new Date()
      };
    })
  );

  return {
    fileId,
    backups: backupResults,
    redundancy: backupResults.length
  };
}
```

---

## Troubleshooting

### Common Issues

#### "Sharp installation fails"

```
Error: sharp: Installation error
```

**Solution:** Force rebuild Sharp.

```bash
cd plugins/file-processing/ts
npm rebuild sharp
```

#### "ffmpeg not found"

```
Error: ffmpeg not found in PATH
```

**Solution:** Install ffmpeg.

```bash
# macOS
brew install ffmpeg

# Linux (Debian/Ubuntu)
sudo apt-get install ffmpeg

# Verify installation
which ffmpeg
```

#### "ClamAV not running"

```
Error: ECONNREFUSED connecting to ClamAV
```

**Solutions:**
1. Start ClamAV: `brew services start clamav` (macOS) or `sudo systemctl start clamav-daemon` (Linux)
2. Test connection: `telnet localhost 3310`
3. Disable scanning if not needed: `FILE_ENABLE_VIRUS_SCAN=false`

#### "Redis Connection Error"

```
Error: Redis connection to localhost:6379 failed
```

**Solution:** Verify Redis is running.

```bash
redis-cli ping
# Should return: PONG
```

#### "Database Connection Error"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Test connection: `psql $DATABASE_URL -c "SELECT 1"`
3. Check schema exists: `psql $DATABASE_URL -c "\dt file_*"`

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
LOG_LEVEL=debug nself plugin file-processing server
```

### Health Checks

```bash
# Check server health
curl http://localhost:3104/health

# Check processing stats
curl http://localhost:3104/api/stats
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)

---

*Last Updated: January 2026*
*Plugin Version: 1.0.0*
