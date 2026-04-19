# File Processing Plugin

Comprehensive file processing with thumbnail generation, image optimization, video thumbnails, and virus scanning. Works with any storage provider (MinIO, S3, GCS, R2, Azure, B2).

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Processing Operations](#processing-operations)
- [Integration Examples](#integration-examples)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)

---

## Overview

The File Processing plugin provides background processing for uploaded files with comprehensive support for images, videos, and documents. It supports:

- **Thumbnail Generation** - Multiple sizes (100x100, 400x400, 1200x1200 by default)
- **Image Optimization** - Compress and optimize images with Sharp
- **Video Thumbnails** - Extract frames from videos using ffmpeg
- **EXIF Stripping** - Remove metadata for privacy
- **Virus Scanning** - Scan files with ClamAV (optional)
- **Multiple Storage Providers** - MinIO, AWS S3, Google Cloud Storage, Cloudflare R2, Backblaze B2, Azure Blob
- **Queue Processing** - BullMQ-powered background processing
- **Batch Processing** - Handle multiple files concurrently
- **Webhooks** - Notify on completion
- **REST API** - HTTP endpoints for integration

### Supported Storage Providers

| Provider | Configuration |
|----------|---------------|
| MinIO | S3-compatible, self-hosted |
| AWS S3 | Native S3 support |
| Google Cloud Storage | GCS with service account |
| Cloudflare R2 | S3-compatible, zero egress fees |
| Backblaze B2 | S3-compatible, low-cost |
| Azure Blob | Native Azure Blob support |

### Processing Capabilities

| Capability | Image | Video | Audio | Document |
|------------|-------|-------|-------|----------|
| Thumbnail Generation | ✅ | ✅ | ❌ | ✅ |
| Optimization | ✅ | ❌ | ❌ | ❌ |
| EXIF Stripping | ✅ | ❌ | ❌ | ❌ |
| Metadata Extraction | ✅ | ✅ | ✅ | ✅ |
| Virus Scanning | ✅ | ✅ | ✅ | ✅ |

---

## Quick Start

```bash
# Install dependencies
cd plugins/file-processing
./install.sh

# Configure environment
echo "FILE_STORAGE_PROVIDER=minio" >> .env
echo "FILE_STORAGE_BUCKET=files" >> .env
echo "FILE_STORAGE_ENDPOINT=http://localhost:9000" >> .env
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "REDIS_URL=redis://localhost:6379" >> .env

# Initialize database schema
nself plugin file-processing init

# Start server (port 3104)
nself plugin file-processing server

# Start worker (in another terminal)
nself plugin file-processing worker
```

---

## Configuration

### Required Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgresql://user:pass@localhost:5432/nself` |
| `FILE_STORAGE_BUCKET` | Storage bucket/container name | `files` |
| `FILE_STORAGE_PROVIDER` | Storage provider | `minio`, `s3`, `gcs`, `r2`, `b2`, `azure` |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FILE_STORAGE_ENDPOINT` | - | Storage endpoint URL (MinIO/R2/B2) |
| `FILE_STORAGE_ACCESS_KEY` | - | Storage access key |
| `FILE_STORAGE_SECRET_KEY` | - | Storage secret key |
| `FILE_STORAGE_REGION` | `us-east-1` | Storage region |
| `FILE_THUMBNAIL_SIZES` | `100,400,1200` | Thumbnail sizes (comma-separated) |
| `FILE_ENABLE_VIRUS_SCAN` | `false` | Enable ClamAV virus scanning |
| `FILE_ENABLE_OPTIMIZATION` | `true` | Enable image optimization |
| `FILE_MAX_SIZE` | `104857600` | Max file size in bytes (100MB) |
| `FILE_ALLOWED_TYPES` | `*` | Allowed MIME types (comma-separated) |
| `FILE_STRIP_EXIF` | `true` | Strip EXIF metadata from images |
| `FILE_QUEUE_CONCURRENCY` | `3` | Number of concurrent processing jobs |
| `CLAMAV_HOST` | `localhost` | ClamAV host |
| `CLAMAV_PORT` | `3310` | ClamAV port |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection string |
| `PORT` | `3104` | HTTP server port |

### Storage Provider Examples

**MinIO (Self-Hosted):**
```bash
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_BUCKET=files
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_ACCESS_KEY=minioadmin
FILE_STORAGE_SECRET_KEY=minioadmin
```

**AWS S3:**
```bash
FILE_STORAGE_PROVIDER=s3
FILE_STORAGE_BUCKET=my-bucket
FILE_STORAGE_REGION=us-east-1
FILE_STORAGE_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
FILE_STORAGE_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

**Google Cloud Storage:**
```bash
FILE_STORAGE_PROVIDER=gcs
FILE_STORAGE_BUCKET=my-bucket
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
```

**Cloudflare R2:**
```bash
FILE_STORAGE_PROVIDER=r2
FILE_STORAGE_BUCKET=my-bucket
FILE_STORAGE_ENDPOINT=https://[account-id].r2.cloudflarestorage.com
FILE_STORAGE_ACCESS_KEY=your_r2_access_key
FILE_STORAGE_SECRET_KEY=your_r2_secret_key
```

**Azure Blob Storage:**
```bash
FILE_STORAGE_PROVIDER=azure
FILE_STORAGE_BUCKET=my-container
AZURE_STORAGE_CONNECTION_STRING=DefaultEndpointsProtocol=https;AccountName=...
```

**Backblaze B2:**
```bash
FILE_STORAGE_PROVIDER=b2
FILE_STORAGE_BUCKET=my-bucket
FILE_STORAGE_ENDPOINT=https://s3.us-west-000.backblazeb2.com
FILE_STORAGE_ACCESS_KEY=your_key_id
FILE_STORAGE_SECRET_KEY=your_application_key
```

---

## CLI Commands

```bash
# Initialize database schema
nself plugin file-processing init

# Start HTTP server
nself plugin file-processing server [--port 3104]

# Start background worker
nself plugin file-processing worker

# Process a file immediately
nself plugin file-processing process <file-id> <file-path>

# View processing statistics
nself plugin file-processing stats

# Clean up old jobs (default: 30 days)
nself plugin file-processing cleanup [--days 30]
```

---

## REST API

The server runs on `http://localhost:3104` by default.

### Create Processing Job

```
POST /api/jobs
Content-Type: application/json
```

**Request Body:**
```json
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

**Response:**
```json
{
  "jobId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "estimatedDuration": 3000
}
```

### Get Job Status

```
GET /api/jobs/:jobId
```

**Response:**
```json
{
  "job": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "fileId": "file_123",
    "status": "completed",
    "thumbnails": ["thumb_100.jpg", "thumb_400.jpg", "thumb_1200.jpg"],
    "durationMs": 2847
  },
  "thumbnails": [
    {
      "id": "...",
      "width": 100,
      "height": 100,
      "url": "https://storage/thumbnails/thumb_100.jpg",
      "format": "jpeg"
    }
  ],
  "metadata": {
    "width": 3000,
    "height": 2000,
    "format": "JPEG",
    "exifStripped": true
  }
}
```

### List Jobs

```
GET /api/jobs?status=completed&limit=50&offset=0
```

**Query Parameters:**
- `status` - Filter by status: `pending`, `processing`, `completed`, `failed`
- `fileId` - Filter by file ID
- `limit` - Results per page (default: 50)
- `offset` - Pagination offset (default: 0)

### Processing Statistics

```
GET /api/stats
```

**Response:**
```json
{
  "pending": 5,
  "processing": 2,
  "completed": 1247,
  "failed": 3,
  "avgDurationMs": 2340,
  "totalProcessed": 1257,
  "thumbnailsGenerated": 3771,
  "storageUsed": 524288000
}
```

### Health Check

```
GET /health
```

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2026-02-14T12:00:00Z",
  "redis": "connected",
  "database": "connected"
}
```

---

## Database Schema

### np_fileproc_jobs

Processing queue and job history.

```sql
CREATE TABLE np_fileproc_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    file_id VARCHAR(255) NOT NULL,
    file_path TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    mime_type VARCHAR(255) NOT NULL,
    storage_provider VARCHAR(50) NOT NULL,
    storage_bucket VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    priority INTEGER DEFAULT 5,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    operations JSONB DEFAULT '[]',
    thumbnails JSONB DEFAULT '[]',
    metadata JSONB,
    scan_result JSONB,
    optimization_result JSONB,
    error_message TEXT,
    error_stack TEXT,
    last_error_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_ms INTEGER,
    queue_name VARCHAR(255) DEFAULT 'default',
    scheduled_for TIMESTAMPTZ,
    webhook_url TEXT,
    webhook_secret TEXT,
    callback_data JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_np_fileproc_jobs_status ON np_fileproc_jobs(status);
CREATE INDEX idx_np_fileproc_jobs_file_id ON np_fileproc_jobs(file_id);
CREATE INDEX idx_np_fileproc_jobs_created ON np_fileproc_jobs(created_at DESC);
CREATE INDEX idx_np_fileproc_jobs_source_account ON np_fileproc_jobs(source_account_id);
```

### np_fileproc_thumbnails

Generated thumbnail metadata and URLs.

```sql
CREATE TABLE np_fileproc_thumbnails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    job_id UUID REFERENCES np_fileproc_jobs(id) ON DELETE CASCADE,
    file_id VARCHAR(255) NOT NULL,
    thumbnail_path TEXT NOT NULL,
    thumbnail_url TEXT,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    size_bytes BIGINT,
    format VARCHAR(50) NOT NULL,
    source_width INTEGER,
    source_height INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_np_fileproc_thumbnails_job ON np_fileproc_thumbnails(job_id);
CREATE INDEX idx_np_fileproc_thumbnails_file ON np_fileproc_thumbnails(file_id);
CREATE INDEX idx_np_fileproc_thumbnails_source_account ON np_fileproc_thumbnails(source_account_id);
```

### np_fileproc_scans

Virus scan results.

```sql
CREATE TABLE np_fileproc_scans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    job_id UUID REFERENCES np_fileproc_jobs(id) ON DELETE CASCADE,
    file_id VARCHAR(255) NOT NULL,
    scan_status VARCHAR(50) NOT NULL,
    infected BOOLEAN DEFAULT FALSE,
    virus_name TEXT,
    scan_duration_ms INTEGER,
    scanned_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_np_fileproc_scans_job ON np_fileproc_scans(job_id);
CREATE INDEX idx_np_fileproc_scans_file ON np_fileproc_scans(file_id);
CREATE INDEX idx_np_fileproc_scans_infected ON np_fileproc_scans(infected);
CREATE INDEX idx_np_fileproc_scans_source_account ON np_fileproc_scans(source_account_id);
```

### np_fileproc_metadata

Extracted EXIF and file metadata.

```sql
CREATE TABLE np_fileproc_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    job_id UUID REFERENCES np_fileproc_jobs(id) ON DELETE CASCADE,
    file_id VARCHAR(255) NOT NULL,
    width INTEGER,
    height INTEGER,
    duration_seconds NUMERIC,
    format VARCHAR(50),
    color_space VARCHAR(50),
    has_alpha BOOLEAN,
    exif JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_np_fileproc_metadata_job ON np_fileproc_metadata(job_id);
CREATE INDEX idx_np_fileproc_metadata_file ON np_fileproc_metadata(file_id);
CREATE INDEX idx_np_fileproc_metadata_source_account ON np_fileproc_metadata(source_account_id);
```

---

## Processing Operations

### Thumbnail Generation

Generates multiple thumbnail sizes from images and videos:

- Uses **Sharp** for image resizing (high-quality)
- Uses **ffmpeg** for video frame extraction
- Supports custom sizes via `FILE_THUMBNAIL_SIZES`
- Automatic format conversion (JPEG for thumbnails)
- Quality optimization

**Default Sizes:**
- 100x100 - Icon/avatar size
- 400x400 - Preview size
- 1200x1200 - Large preview

### Image Optimization

Reduces file size without quality loss:

- Compression with quality control
- Format conversion (e.g., PNG → JPEG)
- Progressive encoding
- Metadata stripping

**Typical Savings:**
- JPEG: 20-40% size reduction
- PNG: 30-60% size reduction

### EXIF Stripping

Removes sensitive metadata for privacy:

- GPS coordinates
- Camera information
- Software information
- Timestamps
- Copyright information

### Virus Scanning

Scans files with ClamAV:

- Detects malware, viruses, trojans
- Quarantine infected files
- Signature updates
- Scan history tracking

### Metadata Extraction

Extracts rich metadata:

- **Images**: Dimensions, color space, EXIF data, GPS location
- **Videos**: Duration, codecs, resolution, frame rate
- **Audio**: Duration, bitrate, channels, sample rate
- **Documents**: Page count, author, title, subject

---

## Integration Examples

### Node.js/TypeScript

```typescript
import fetch from 'node-fetch';

async function processFile(fileId: string, filePath: string) {
  const response = await fetch('http://localhost:3104/api/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      fileId,
      filePath,
      fileName: 'photo.jpg',
      fileSize: 1024000,
      mimeType: 'image/jpeg',
      operations: ['thumbnail', 'optimize', 'metadata'],
      webhookUrl: 'https://myapp.com/webhooks/file-processed',
    }),
  });

  const { jobId } = await response.json();
  console.log('Job created:', jobId);
}
```

### Python

```python
import requests

def process_file(file_id, file_path):
    response = requests.post('http://localhost:3104/api/jobs', json={
        'fileId': file_id,
        'filePath': file_path,
        'fileName': 'photo.jpg',
        'fileSize': 1024000,
        'mimeType': 'image/jpeg',
        'operations': ['thumbnail', 'optimize', 'metadata'],
        'webhookUrl': 'https://myapp.com/webhooks/file-processed'
    })

    job = response.json()
    print(f"Job created: {job['jobId']}")
```

### Webhooks

When a job completes, the plugin sends a webhook to your application:

```json
{
  "event": "job.completed",
  "jobId": "550e8400-e29b-41d4-a716-446655440000",
  "fileId": "file_123",
  "status": "completed",
  "thumbnails": [
    {
      "width": 100,
      "height": 100,
      "url": "https://storage/thumbnails/thumb_100.jpg"
    }
  ],
  "metadata": { /* extracted metadata */ },
  "scan": { /* virus scan result if enabled */ },
  "optimization": { /* optimization result */ },
  "durationMs": 2847,
  "callbackData": { /* your custom data */ }
}
```

Webhook requests include an `X-Signature` header with HMAC-SHA256 signature for verification.

---

## Performance

### Benchmarks

Tested on MacBook Pro M1 with Sharp and ffmpeg:

| Operation | File Type | Size | Time |
|-----------|-----------|------|------|
| Thumbnail (3 sizes) | JPEG | 5MB | ~180ms |
| Thumbnail (3 sizes) | PNG | 10MB | ~320ms |
| Video thumbnail | MP4 | 50MB | ~450ms |
| Optimization | JPEG | 5MB | ~140ms |
| EXIF extraction | JPEG | 5MB | ~25ms |
| Virus scan | Any | 10MB | ~200ms |

### Concurrency

Default concurrency: 3 files processed simultaneously

- Adjustable via `FILE_QUEUE_CONCURRENCY`
- Higher values = faster but more CPU/memory usage
- Recommended: 2-5 depending on hardware

### Storage

Thumbnail storage overhead:

- 100x100: ~3-5 KB
- 400x400: ~15-25 KB
- 1200x1200: ~80-120 KB

---

## Troubleshooting

### Sharp installation fails

```bash
# Force rebuild
cd plugins/file-processing/ts
npm rebuild sharp
```

### ffmpeg not found

```bash
# macOS
brew install ffmpeg

# Linux (Debian/Ubuntu)
sudo apt-get install ffmpeg

# Verify
which ffmpeg
```

### ClamAV not running

```bash
# macOS
brew services start clamav

# Linux
sudo systemctl start clamav-daemon

# Test connection
telnet localhost 3310
```

### Redis connection error

```bash
# Check Redis is running
redis-cli ping

# Should return: PONG
```

### Database connection error

```bash
# Test connection
psql $DATABASE_URL -c "SELECT 1"

# Check schema
psql $DATABASE_URL -c "\dt np_fileproc_*"
```

---

## License

Source-Available
