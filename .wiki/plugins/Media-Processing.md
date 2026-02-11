# Media Processing Plugin

FFmpeg-based media encoding and processing with HLS streaming support, multipl

e resolutions, and hardware acceleration.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Encoding Profiles](#encoding-profiles)
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Media Processing plugin provides professional FFmpeg-based video encoding and transcoding capabilities. It supports:

- **7 Database Tables** - Jobs, profiles, outputs, manifests
- **10 Webhook Events** - Real-time job status updates
- **HLS Streaming** - Adaptive bitrate streaming with multiple resolutions
- **Hardware Acceleration** - NVENC, VA-API, QSV support
- **Full REST API** - Job management and status tracking
- **CLI Interface** - Command-line tools for all operations
- **Multipart Processing** - Handle large files efficiently

### Features

| Feature | Description |
|---------|-------------|
| Multiple Resolutions | Encode 1080p, 720p, 480p simultaneously |
| HLS Packaging | Generate adaptive streaming manifests |
| Subtitle Extraction | Extract embedded subtitles to VTT/SRT |
| Thumbnail Generation | Create preview thumbnails |
| Trickplay Tiles | Generate timeline preview images |
| Hardware Accel | GPU-accelerated encoding (NVENC, VA-API, QSV) |
| Queue Management | Priority-based job scheduling |
| Progress Tracking | Real-time encoding progress |

---

## Quick Start

```bash
# Install the plugin
nself plugin install media-processing

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "MP_OUTPUT_BASE_PATH=/data/media" >> .env

# Initialize database schema
nself plugin media-processing init

# Start server
nself plugin media-processing server --port 3019

# Submit encoding job
nself plugin media-processing submit /path/to/video.mp4
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `MP_PLUGIN_PORT` | No | `3019` | HTTP server port |
| `MP_FFMPEG_PATH` | No | `ffmpeg` | Path to ffmpeg binary |
| `MP_FFPROBE_PATH` | No | `ffprobe` | Path to ffprobe binary |
| `MP_OUTPUT_BASE_PATH` | No | `/data/media-processing` | Output directory base path |
| `MP_MAX_CONCURRENT_JOBS` | No | `2` | Maximum concurrent encoding jobs |
| `MP_MAX_INPUT_SIZE_GB` | No | `50` | Maximum input file size in GB |
| `MP_HARDWARE_ACCEL` | No | `none` | Hardware acceleration (`none`, `nvenc`, `vaapi`, `qsv`) |
| `MP_API_KEY` | No | - | API key for authentication |
| `MP_RATE_LIMIT_MAX` | No | `50` | Max API requests per window |
| `MP_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Hardware Acceleration Options

| Value | Description | Requirements |
|-------|-------------|--------------|
| `none` | CPU-only encoding (default) | None |
| `nvenc` | NVIDIA GPU acceleration | NVIDIA GPU with NVENC support |
| `vaapi` | Intel/AMD GPU acceleration | Intel iGPU or AMD GPU with VA-API |
| `qsv` | Intel Quick Sync Video | Intel CPU with Quick Sync |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Paths
MP_OUTPUT_BASE_PATH=/data/media-processing
MP_FFMPEG_PATH=/usr/bin/ffmpeg
MP_FFPROBE_PATH=/usr/bin/ffprobe

# Processing
MP_MAX_CONCURRENT_JOBS=2
MP_MAX_INPUT_SIZE_GB=50
MP_HARDWARE_ACCEL=nvenc

# Server
MP_PLUGIN_PORT=3019
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin media-processing init

# Check plugin status
nself plugin media-processing status

# View statistics
nself plugin media-processing stats
```

### Server

```bash
# Start API server (default port 3019)
nself plugin media-processing server

# Custom port
nself plugin media-processing server --port 8080
```

### Job Management

```bash
# Submit encoding job (file)
nself plugin media-processing submit /path/to/video.mp4

# Submit with specific profile
nself plugin media-processing submit video.mp4 --profile profile_id

# Submit with priority
nself plugin media-processing submit video.mp4 --priority 10

# Submit from URL
nself plugin media-processing submit https://example.com/video.mp4 --type url

# Submit from S3
nself plugin media-processing submit s3://bucket/video.mp4 --type s3

# Custom output path
nself plugin media-processing submit video.mp4 --output /custom/path

# List all jobs
nself plugin media-processing jobs

# List by status
nself plugin media-processing jobs --status pending
nself plugin media-processing jobs --status encoding
nself plugin media-processing jobs --status completed

# Limit results
nself plugin media-processing jobs --limit 10
```

### Encoding Profiles

```bash
# List all encoding profiles
nself plugin media-processing profiles
```

### Media Analysis

```bash
# Analyze media file
nself plugin media-processing analyze /path/to/video.mp4

# Example output:
# Format:     matroska,webm
# Duration:   7200s
# Bitrate:    5000 kbps
# Size:       4320.00 MB
#
# Streams (3):
# Type:       video
# Codec:      h264
# Resolution: 1920x1080
#
# Type:       audio
# Codec:      aac
# Channels:   2
# Sample Rate: 48000
```

---

## REST API

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "media-processing",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "media-processing",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "media-processing",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640
  },
  "stats": {
    "totalJobs": 150,
    "activeJobs": 2,
    "queuedJobs": 5
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/status
Plugin status and configuration.

**Response:**
```json
{
  "plugin": "media-processing",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "maxConcurrentJobs": 2,
    "hardwareAccel": "nvenc",
    "outputBasePath": "/data/media-processing"
  },
  "stats": {
    "totalJobs": 150,
    "pendingJobs": 5,
    "runningJobs": 2,
    "completedJobs": 130,
    "failedJobs": 13
  },
  "queue": {
    "pending": 5,
    "active": 2
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Encoding Profiles

#### POST /v1/profiles
Create new encoding profile.

**Request Body:**
```json
{
  "name": "high-quality",
  "description": "High quality encoding",
  "container": "mp4",
  "video_codec": "h264",
  "audio_codec": "aac",
  "resolutions": [
    { "width": 1920, "height": 1080, "bitrate": 5000000, "label": "1080p" },
    { "width": 1280, "height": 720, "bitrate": 2500000, "label": "720p" }
  ],
  "audio_bitrate": 128000,
  "framerate": 30,
  "preset": "medium",
  "hls_enabled": true,
  "hls_segment_duration": 6,
  "trickplay_enabled": false,
  "subtitle_extract": true,
  "thumbnail_enabled": true,
  "thumbnail_count": 5,
  "is_default": false
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "name": "high-quality",
    ...
  }
}
```

#### GET /v1/profiles
List all encoding profiles.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "source_account_id": "primary",
      "name": "default",
      "description": "Default encoding profile",
      "container": "mp4",
      "video_codec": "h264",
      "audio_codec": "aac",
      "resolutions": [...],
      "is_default": true
    }
  ]
}
```

#### GET /v1/profiles/:id
Get specific encoding profile.

#### PUT /v1/profiles/:id
Update encoding profile.

#### DELETE /v1/profiles/:id
Delete encoding profile.

### Jobs

#### POST /v1/jobs
Submit new encoding job.

**Request Body:**
```json
{
  "input_url": "/path/to/video.mp4",
  "input_type": "file",
  "profile_id": "uuid",
  "priority": 0,
  "output_base_path": "/custom/path"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "source_account_id": "primary",
    "input_url": "/path/to/video.mp4",
    "input_type": "file",
    "profile_id": "uuid",
    "status": "pending",
    "priority": 0,
    "progress": 0,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/jobs
List encoding jobs.

**Query Parameters:**
- `status` (optional) - Filter by status
- `limit` (optional) - Max results (default: 50)
- `offset` (optional) - Pagination offset (default: 0)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "status": "encoding",
      "progress": 45.5,
      "input_url": "/path/to/video.mp4",
      "started_at": "2026-02-11T10:00:00.000Z"
    }
  ]
}
```

#### GET /v1/jobs/:id
Get job details with outputs.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "status": "completed",
    "progress": 100,
    "outputs": [...],
    "subtitles": [...],
    "hls_manifest": {...},
    "trickplay": {...}
  }
}
```

#### POST /v1/jobs/:id/cancel
Cancel running job.

**Response:**
```json
{
  "success": true
}
```

#### POST /v1/jobs/:id/retry
Retry failed job.

**Response:**
```json
{
  "success": true
}
```

#### GET /v1/jobs/:id/outputs
Get job outputs.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "job_id": "uuid",
      "output_type": "video",
      "resolution_label": "1080p",
      "file_path": "/data/media/job_123/1080p.mp4",
      "file_size_bytes": 524288000,
      "width": 1920,
      "height": 1080,
      "bitrate": 5000000
    }
  ]
}
```

#### GET /v1/jobs/:id/hls
Get HLS manifest for job.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "job_id": "uuid",
    "master_manifest_path": "/data/media/job_123/master.m3u8",
    "variant_manifests": [
      {
        "resolution": "1080p",
        "playlist_path": "/data/media/job_123/1080p.m3u8",
        "bandwidth": 5000000
      }
    ],
    "segment_count": 120,
    "total_duration_seconds": 720
  }
}
```

#### GET /v1/jobs/:id/subtitles
Get extracted subtitles.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "job_id": "uuid",
      "language": "en",
      "label": "English",
      "format": "vtt",
      "file_path": "/data/media/job_123/en.vtt",
      "is_default": true
    }
  ]
}
```

#### GET /v1/jobs/:id/trickplay
Get trickplay tile information.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "job_id": "uuid",
    "tile_width": 320,
    "tile_height": 180,
    "columns": 10,
    "rows": 10,
    "interval_seconds": 10,
    "file_path": "/data/media/job_123/trickplay.jpg",
    "total_thumbnails": 72
  }
}
```

### Media Analysis

#### POST /v1/analyze
Analyze media file without encoding.

**Request Body:**
```json
{
  "url": "/path/to/video.mp4"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "format": "matroska,webm",
    "duration": 7200,
    "bitrate": 5000000,
    "size": 4500000000,
    "streams": [
      {
        "index": 0,
        "codec_type": "video",
        "codec_name": "h264",
        "width": 1920,
        "height": 1080,
        "bit_rate": 4500000
      },
      {
        "index": 1,
        "codec_type": "audio",
        "codec_name": "aac",
        "channels": 2,
        "sample_rate": 48000
      }
    ]
  }
}
```

### Thumbnail Generation

#### POST /v1/thumbnail
Generate thumbnails without full encoding.

**Request Body:**
```json
{
  "url": "/path/to/video.mp4",
  "count": 5
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "thumbnails": [
      "/tmp/thumb_001.jpg",
      "/tmp/thumb_002.jpg",
      "/tmp/thumb_003.jpg"
    ],
    "count": 3
  }
}
```

### Statistics

#### GET /v1/stats
Get processing statistics.

**Response:**
```json
{
  "success": true,
  "data": {
    "totalJobs": 150,
    "pendingJobs": 5,
    "runningJobs": 2,
    "completedJobs": 130,
    "failedJobs": 13,
    "totalDurationSeconds": 540000,
    "totalFileSizeBytes": 107374182400,
    "profiles": 3,
    "averageProcessingTimeSeconds": 1800,
    "lastJobCompletedAt": "2026-02-11T10:00:00.000Z",
    "activeJobs": 2,
    "queuedJobs": 5
  }
}
```

---

## Webhook Events

The plugin emits these webhook events for job lifecycle tracking:

| Event | Description | Payload |
|-------|-------------|---------|
| `job.created` | Job submitted for processing | `{ job_id, input_url, profile_id }` |
| `job.downloading` | Downloading input file | `{ job_id, progress }` |
| `job.analyzing` | Analyzing media file | `{ job_id, metadata }` |
| `job.encoding` | Encoding in progress | `{ job_id, progress }` |
| `job.packaging` | Creating HLS manifests | `{ job_id }` |
| `job.uploading` | Uploading outputs | `{ job_id, progress }` |
| `job.completed` | Job completed successfully | `{ job_id, outputs, duration }` |
| `job.failed` | Job failed | `{ job_id, error }` |
| `job.cancelled` | Job cancelled by user | `{ job_id }` |
| `job.progress` | Encoding progress update | `{ job_id, progress, eta }` |

### Webhook Payload Example

```json
{
  "id": "evt_abc123",
  "type": "job.completed",
  "created": 1707649200,
  "data": {
    "job_id": "uuid",
    "status": "completed",
    "outputs": [
      {
        "resolution": "1080p",
        "file_path": "/data/media/job_123/1080p.mp4",
        "file_size_bytes": 524288000
      }
    ],
    "duration_seconds": 1800
  }
}
```

---

## Database Schema

### mp_encoding_profiles

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `name` | VARCHAR(128) | Profile name |
| `description` | TEXT | Profile description |
| `container` | VARCHAR(16) | Container format (mp4, mkv, webm, ts) |
| `video_codec` | VARCHAR(16) | Video codec (h264, h265, vp9, av1) |
| `audio_codec` | VARCHAR(16) | Audio codec (aac, opus, mp3) |
| `resolutions` | JSONB | Array of resolution objects |
| `audio_bitrate` | INTEGER | Audio bitrate (bps) |
| `framerate` | INTEGER | Target framerate |
| `preset` | VARCHAR(16) | Encoding preset |
| `hls_enabled` | BOOLEAN | Enable HLS packaging |
| `hls_segment_duration` | INTEGER | HLS segment duration (seconds) |
| `trickplay_enabled` | BOOLEAN | Generate trickplay tiles |
| `trickplay_interval` | INTEGER | Trickplay interval (seconds) |
| `subtitle_extract` | BOOLEAN | Extract subtitles |
| `thumbnail_enabled` | BOOLEAN | Generate thumbnails |
| `thumbnail_count` | INTEGER | Number of thumbnails |
| `is_default` | BOOLEAN | Default profile flag |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Indexes:**
- `idx_mp_profiles_account` - source_account_id
- `idx_mp_profiles_default` - is_default (partial)

**Unique Constraint:**
- `(source_account_id, name)`

### mp_jobs

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `input_url` | TEXT | Input file URL/path |
| `input_type` | VARCHAR(32) | Input type (file, url, s3) |
| `profile_id` | UUID | Encoding profile reference |
| `status` | VARCHAR(32) | Job status |
| `priority` | INTEGER | Job priority (higher = first) |
| `progress` | DOUBLE PRECISION | Progress percentage (0-100) |
| `input_metadata` | JSONB | Input file metadata |
| `output_base_path` | TEXT | Output directory path |
| `error_message` | TEXT | Error message if failed |
| `duration_seconds` | DOUBLE PRECISION | Processing duration |
| `file_size_bytes` | BIGINT | Input file size |
| `started_at` | TIMESTAMPTZ | Start timestamp |
| `completed_at` | TIMESTAMPTZ | Completion timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Job Statuses:**
- `pending` - Waiting in queue
- `downloading` - Downloading input
- `analyzing` - Analyzing media
- `encoding` - Encoding in progress
- `packaging` - Creating manifests
- `uploading` - Uploading outputs
- `completed` - Successfully completed
- `failed` - Failed with error
- `cancelled` - Cancelled by user

**Indexes:**
- `idx_mp_jobs_account` - source_account_id
- `idx_mp_jobs_status` - status
- `idx_mp_jobs_profile` - profile_id
- `idx_mp_jobs_created` - created_at DESC
- `idx_mp_jobs_priority` - (priority DESC, created_at ASC) WHERE status = 'pending'

### mp_job_outputs

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `job_id` | UUID | Parent job reference |
| `output_type` | VARCHAR(32) | Output type |
| `resolution_label` | VARCHAR(16) | Resolution label (1080p, 720p, etc.) |
| `file_path` | TEXT | Output file path |
| `file_size_bytes` | BIGINT | File size |
| `content_type` | VARCHAR(128) | MIME type |
| `width` | INTEGER | Video width |
| `height` | INTEGER | Video height |
| `bitrate` | INTEGER | Bitrate (bps) |
| `duration_seconds` | DOUBLE PRECISION | Duration |
| `language` | VARCHAR(16) | Language code |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Output Types:**
- `video` - Video file
- `audio` - Audio-only file
- `thumbnail` - Preview image
- `subtitle` - Subtitle file
- `manifest` - HLS playlist

**Indexes:**
- `idx_mp_outputs_account` - source_account_id
- `idx_mp_outputs_job` - job_id
- `idx_mp_outputs_type` - output_type

### mp_hls_manifests

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `job_id` | UUID | Parent job reference |
| `master_manifest_path` | TEXT | Master playlist path |
| `variant_manifests` | JSONB | Array of variant playlists |
| `segment_count` | INTEGER | Total segment count |
| `total_duration_seconds` | DOUBLE PRECISION | Total duration |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_mp_hls_account` - source_account_id
- `idx_mp_hls_job` - job_id

### mp_subtitles

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `job_id` | UUID | Parent job reference |
| `language` | VARCHAR(16) | Language code |
| `label` | VARCHAR(64) | Display label |
| `format` | VARCHAR(8) | Subtitle format (vtt, srt, ass) |
| `file_path` | TEXT | Subtitle file path |
| `is_default` | BOOLEAN | Default subtitle flag |
| `is_forced` | BOOLEAN | Forced subtitle flag |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_mp_subtitles_account` - source_account_id
- `idx_mp_subtitles_job` - job_id

### mp_trickplay

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `job_id` | UUID | Parent job reference |
| `tile_width` | INTEGER | Tile width (pixels) |
| `tile_height` | INTEGER | Tile height (pixels) |
| `columns` | INTEGER | Columns per image |
| `rows` | INTEGER | Rows per image |
| `interval_seconds` | INTEGER | Thumbnail interval |
| `file_path` | TEXT | Tile image path |
| `index_path` | TEXT | Index file path |
| `total_thumbnails` | INTEGER | Total thumbnail count |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_mp_trickplay_account` - source_account_id
- `idx_mp_trickplay_job` - job_id

### mp_webhook_events

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (event ID) |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `event_type` | VARCHAR(128) | Event type |
| `payload` | JSONB | Event payload |
| `processed` | BOOLEAN | Processing status |
| `processed_at` | TIMESTAMPTZ | Processing timestamp |
| `error` | TEXT | Error message if failed |
| `created_at` | TIMESTAMPTZ | Event creation time |

**Indexes:**
- `idx_mp_webhook_account` - source_account_id
- `idx_mp_webhook_processed` - processed
- `idx_mp_webhook_created` - created_at DESC

---

## Encoding Profiles

### Default Profile

The plugin creates a default profile on initialization:

```json
{
  "name": "default",
  "description": "Default encoding profile with 1080p, 720p, and 480p",
  "container": "mp4",
  "video_codec": "h264",
  "audio_codec": "aac",
  "resolutions": [
    { "width": 1920, "height": 1080, "bitrate": 5000000, "label": "1080p" },
    { "width": 1280, "height": 720, "bitrate": 2500000, "label": "720p" },
    { "width": 854, "height": 480, "bitrate": 1000000, "label": "480p" }
  ],
  "audio_bitrate": 128000,
  "framerate": 30,
  "preset": "medium",
  "hls_enabled": true,
  "hls_segment_duration": 6,
  "trickplay_enabled": false,
  "subtitle_extract": true,
  "thumbnail_enabled": true,
  "thumbnail_count": 5
}
```

### Custom Profiles

Create profiles for specific use cases:

#### High Quality Profile
```json
{
  "name": "high-quality",
  "video_codec": "h265",
  "preset": "slow",
  "resolutions": [
    { "width": 3840, "height": 2160, "bitrate": 15000000, "label": "4K" },
    { "width": 1920, "height": 1080, "bitrate": 8000000, "label": "1080p" }
  ]
}
```

#### Fast Encoding Profile
```json
{
  "name": "fast",
  "preset": "ultrafast",
  "resolutions": [
    { "width": 1280, "height": 720, "bitrate": 2000000, "label": "720p" }
  ]
}
```

#### Hardware Accelerated Profile
```json
{
  "name": "nvenc",
  "video_codec": "h264",
  "preset": "fast",
  "resolutions": [
    { "width": 1920, "height": 1080, "bitrate": 5000000, "label": "1080p" }
  ]
}
```

---

## TypeScript Implementation

### File Structure

```
plugins/media-processing/ts/src/
├── types.ts          # TypeScript interfaces
├── config.ts         # Configuration loading
├── database.ts       # Database operations
├── ffmpeg.ts         # FFmpeg wrapper
├── processor.ts      # Job processing logic
├── server.ts         # HTTP server
├── cli.ts            # CLI commands
└── index.ts          # Module exports
```

### Key Components

#### MediaProcessor (processor.ts)
- Job queue management
- FFmpeg orchestration
- Progress tracking
- Error handling

#### FFmpegClient (ffmpeg.ts)
- FFmpeg/FFprobe wrapper
- Hardware acceleration
- HLS packaging
- Subtitle extraction

#### MediaProcessingDatabase (database.ts)
- Schema initialization
- Job CRUD operations
- Profile management
- Statistics

---

## Examples

### Example 1: Encode Video with Custom Settings

```bash
#!/bin/bash

# Create high-quality profile
profile_id=$(curl -X POST http://localhost:3019/v1/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-quality-hls",
    "video_codec": "h264",
    "preset": "slow",
    "resolutions": [
      {"width": 1920, "height": 1080, "bitrate": 8000000, "label": "1080p"},
      {"width": 1280, "height": 720, "bitrate": 4000000, "label": "720p"}
    ],
    "hls_enabled": true,
    "hls_segment_duration": 4
  }' | jq -r '.data.id')

# Submit job
curl -X POST http://localhost:3019/v1/jobs \
  -H "Content-Type: application/json" \
  -d "{
    \"input_url\": \"/path/to/video.mp4\",
    \"profile_id\": \"$profile_id\",
    \"priority\": 10
  }"
```

### Example 2: Monitor Job Progress

```typescript
async function monitorJob(jobId: string) {
  const pollInterval = 5000; // 5 seconds

  while (true) {
    const response = await fetch(`http://localhost:3019/v1/jobs/${jobId}`);
    const { data: job } = await response.json();

    console.log(`Status: ${job.status}, Progress: ${job.progress}%`);

    if (['completed', 'failed', 'cancelled'].includes(job.status)) {
      break;
    }

    await new Promise(resolve => setTimeout(resolve, pollInterval));
  }
}
```

### Example 3: Batch Processing

```bash
#!/bin/bash

# Process all videos in directory
for video in /path/to/videos/*.mp4; do
  echo "Submitting: $video"

  nself plugin media-processing submit "$video" --priority 5

  sleep 1 # Rate limiting
done
```

### Example 4: Query Job Statistics

```sql
-- Get encoding success rate
SELECT
  COUNT(*) FILTER (WHERE status = 'completed') as completed,
  COUNT(*) FILTER (WHERE status = 'failed') as failed,
  ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'completed') / COUNT(*), 2) as success_rate
FROM mp_jobs
WHERE source_account_id = 'primary';

-- Average processing time by resolution
SELECT
  o.resolution_label,
  COUNT(*) as job_count,
  AVG(EXTRACT(EPOCH FROM (j.completed_at - j.started_at))) as avg_seconds
FROM mp_jobs j
JOIN mp_job_outputs o ON o.job_id = j.id
WHERE j.status = 'completed'
  AND j.source_account_id = 'primary'
GROUP BY o.resolution_label
ORDER BY o.resolution_label;

-- Find largest output files
SELECT
  j.input_url,
  o.resolution_label,
  o.file_size_bytes / 1024 / 1024 as size_mb
FROM mp_job_outputs o
JOIN mp_jobs j ON j.id = o.job_id
WHERE o.source_account_id = 'primary'
ORDER BY o.file_size_bytes DESC
LIMIT 10;
```

---

## Troubleshooting

### Common Issues

#### FFmpeg Not Found

**Error:**
```
Error: spawn ffmpeg ENOENT
```

**Solution:**
Install FFmpeg or set path:
```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# macOS
brew install ffmpeg

# Or set custom path
MP_FFMPEG_PATH=/usr/local/bin/ffmpeg
MP_FFPROBE_PATH=/usr/local/bin/ffprobe
```

#### Hardware Acceleration Errors

**Error:**
```
Error: No NVENC capable devices found
```

**Solution:**
1. Verify GPU support: `nvidia-smi`
2. Check FFmpeg NVENC support: `ffmpeg -hwaccels`
3. Fall back to software encoding: `MP_HARDWARE_ACCEL=none`

#### Job Stuck in Pending

**Problem:**
Jobs remain in `pending` status.

**Solutions:**
- Check `MP_MAX_CONCURRENT_JOBS` limit
- Verify server is running
- Check logs for errors: `LOG_LEVEL=debug`
- Restart server to reset queue

#### Out of Disk Space

**Error:**
```
Error: ENOSPC: no space left on device
```

**Solution:**
1. Check disk space: `df -h`
2. Clean old outputs
3. Set `MP_OUTPUT_BASE_PATH` to larger volume
4. Reduce resolution count in profile

#### Encoding Fails on Large Files

**Error:**
```
Error: Input file too large
```

**Solution:**
Increase limit:
```bash
MP_MAX_INPUT_SIZE_GB=100
```

#### Memory Issues

**Error:**
```
Error: JavaScript heap out of memory
```

**Solution:**
Increase Node.js memory:
```bash
NODE_OPTIONS="--max-old-space-size=4096" nself plugin media-processing server
```

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug nself plugin media-processing server
```

### FFmpeg Command Inspection

The plugin logs full FFmpeg commands in debug mode. Check logs to verify encoding parameters.

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Media-Processing
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **FFmpeg**: https://ffmpeg.org/documentation.html
