# File Processing Plugin for nself

Comprehensive file processing with thumbnail generation, image optimization, video thumbnails, and virus scanning. Works with any storage provider (MinIO, S3, GCS, R2, Azure, B2).

## Features

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

## Installation

```bash
# Install the plugin
cd plugins/file-processing
./install.sh
```

### Dependencies

The plugin requires:

- **Node.js 18+** (for TypeScript server)
- **PostgreSQL** (for job storage)
- **Redis** (for queue management)
- **Sharp** (for image processing - installed via npm)
- **ffmpeg** (for video thumbnails - optional)
  - macOS: `brew install ffmpeg`
  - Linux: `apt-get install ffmpeg`
- **ClamAV** (for virus scanning - optional)
  - macOS: `brew install clamav && clamd`
  - Linux: `apt-get install clamav-daemon`

## Configuration

Create `.env` in `plugins/file-processing/ts/`:

```bash
# Required
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_BUCKET=files
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# MinIO/S3-compatible
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_ACCESS_KEY=minioadmin
FILE_STORAGE_SECRET_KEY=minioadmin
FILE_STORAGE_REGION=us-east-1

# Processing options
FILE_THUMBNAIL_SIZES=100,400,1200
FILE_ENABLE_OPTIMIZATION=true
FILE_STRIP_EXIF=true
FILE_MAX_SIZE=104857600

# Optional: Virus scanning
FILE_ENABLE_VIRUS_SCAN=false
CLAMAV_HOST=localhost
CLAMAV_PORT=3310

# Queue
REDIS_URL=redis://localhost:6379
FILE_QUEUE_CONCURRENCY=3

# Server
PORT=3104
HOST=0.0.0.0
LOG_LEVEL=info
```

### Storage Provider Examples

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

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|-----------------|-----------------|-------------|---------|
| `FILE_PROCESSING_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `FILE_PROCESSING_PLUGIN_PORT` | `PORT` | Server port | `3104` |
| `MINIO_ENDPOINT` | `FILE_STORAGE_ENDPOINT` | S3 endpoint URL | `http://localhost:9000` |
| `MINIO_BUCKET_RAW` | `FILE_STORAGE_BUCKET` | Storage bucket name | `media-raw` |
| `MINIO_ACCESS_KEY` | `FILE_STORAGE_ACCESS_KEY` | S3 access key | `minioadmin` |
| `MINIO_SECRET_KEY` | `FILE_STORAGE_SECRET_KEY` | S3 secret key | `minioadmin` |
| `MINIO_REGION` | `FILE_STORAGE_REGION` | Storage region | `us-east-1` |
| `REDIS_URL` | `REDIS_URL` | Redis connection URL | `redis://localhost:6379` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| - | `FILE_STORAGE_PROVIDER` | Storage provider type | `minio` |

### Configuration Helper Script

You can generate the plugin `.env` file from your backend configuration:

```bash
#!/bin/bash
# generate-file-processing-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/file-processing/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$FILE_PROCESSING_PLUGIN_PORT
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_ENDPOINT=$MINIO_ENDPOINT
FILE_STORAGE_BUCKET=$MINIO_BUCKET_RAW
FILE_STORAGE_ACCESS_KEY=$MINIO_ACCESS_KEY
FILE_STORAGE_SECRET_KEY=$MINIO_SECRET_KEY
FILE_STORAGE_REGION=${MINIO_REGION:-us-east-1}

# Processing options
FILE_THUMBNAIL_SIZES=100,400,1200
FILE_ENABLE_OPTIMIZATION=true
FILE_STRIP_EXIF=true
FILE_MAX_SIZE=104857600

# Queue
REDIS_URL=$REDIS_URL
FILE_QUEUE_CONCURRENCY=3

# Server
HOST=0.0.0.0
LOG_LEVEL=info
EOF

echo "Created $PLUGIN_ENV"
```

### Manual Configuration

Alternatively, manually create `~/.nself/plugins/file-processing/ts/.env`:

```bash
# From nself-tv backend .env.dev
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself_tv
PORT=3104
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_BUCKET=media-raw
FILE_STORAGE_ACCESS_KEY=minioadmin
FILE_STORAGE_SECRET_KEY=minioadmin
FILE_STORAGE_REGION=us-east-1
REDIS_URL=redis://localhost:6379
```

See [CONFIGURATION.md](../../CONFIGURATION.md) for detailed mapping patterns and troubleshooting.

## Usage

### Start Services

```bash
# Start HTTP server (port 3104)
nself plugin file-processing server

# Start background worker
nself plugin file-processing worker

# Or run both in development
cd plugins/file-processing/ts
npm run dev    # Server
npm run worker # Worker (in another terminal)
```

### CLI Commands

```bash
# Process a file immediately
nself plugin file-processing process <file-id> <file-path>

# View processing statistics
nself plugin file-processing stats

# Clean up old jobs (30 days default)
nself plugin file-processing cleanup [--days 30]

# Initialize/reset schema
nself plugin file-processing init
```

### REST API

The server runs on `http://localhost:3104` by default.

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

**Response:**
```json
{
  "jobId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "estimatedDuration": 3000
}
```

#### Get Job Status

```http
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

#### List Jobs

```http
GET /api/jobs?status=completed&limit=50&offset=0
```

#### Processing Statistics

```http
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

#### Health Check

```http
GET /health
```

### Webhooks

When a job completes, the plugin can send a webhook to your application:

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

## Database Schema

### Tables

| Table | Purpose |
|-------|---------|
| `file_processing_jobs` | Processing queue and job history |
| `file_thumbnails` | Generated thumbnail metadata and URLs |
| `file_scans` | Virus scan results |
| `file_metadata` | Extracted EXIF and file metadata |

### Views

| View | Purpose |
|------|---------|
| `file_processing_queue` | Pending jobs ordered by priority |
| `file_processing_failures` | Failed jobs requiring attention |
| `file_security_alerts` | Infected files |
| `file_processing_stats` | Processing statistics by status |
| `thumbnail_generation_stats` | Thumbnail generation statistics |

### Functions

| Function | Purpose |
|----------|---------|
| `get_next_job(queue_name)` | Get and lock next job from queue |
| `cleanup_old_jobs(retention_days)` | Clean up completed jobs |

## Processing Operations

### Thumbnail Generation

Generates multiple thumbnail sizes from images and videos:

- Uses **Sharp** for image resizing (high-quality)
- Uses **ffmpeg** for video frame extraction
- Supports custom sizes via `FILE_THUMBNAIL_SIZES`
- Automatic format conversion (JPEG for thumbnails)
- Quality optimization

### Image Optimization

Reduces file size without quality loss:

- Compression with quality control
- Format conversion (e.g., PNG → JPEG)
- Progressive encoding
- Metadata stripping

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

### GraphQL (via Hasura Action)

```graphql
mutation ProcessFile($input: ProcessFileInput!) {
  processFile(input: $input) {
    jobId
    status
    estimatedDuration
  }
}
```

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
psql $DATABASE_URL -c "\dt file_*"
```

## Architecture

```
┌─────────────┐
│   Client    │
│  (Your App) │
└──────┬──────┘
       │ HTTP POST /api/jobs
       ▼
┌─────────────┐
│   Server    │
│  (Fastify)  │
└──────┬──────┘
       │ Add to queue
       ▼
┌─────────────┐     ┌──────────────┐
│    Redis    │────▶│    Worker    │
│  (BullMQ)   │     │  (BullMQ)    │
└─────────────┘     └──────┬───────┘
                           │ Process
                           ▼
                    ┌──────────────┐
                    │  Processors  │
                    │  - Thumbnail │
                    │  - Optimize  │
                    │  - Scan      │
                    │  - Metadata  │
                    └──────┬───────┘
                           │ Store results
                           ▼
                    ┌──────────────┐
                    │  PostgreSQL  │
                    └──────┬───────┘
                           │ Webhook
                           ▼
                    ┌──────────────┐
                    │  Your App    │
                    │  (Webhook)   │
                    └──────────────┘
```

## Security

- **EXIF stripping** removes GPS and camera data
- **Virus scanning** prevents malware upload
- **Webhook signatures** (HMAC-SHA256) verify authenticity
- **File type validation** restricts allowed types
- **Size limits** prevent DoS attacks

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/tree/main/plugins/file-processing
