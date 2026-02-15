# Media Processing Plugin

FFmpeg-based media encoding and processing with HLS streaming support for nself.

## Features

- **Multi-resolution encoding** - Encode videos in multiple resolutions (1080p, 720p, 480p, etc.)
- **HLS streaming** - Generate adaptive bitrate HLS playlists with segmented content
- **Hardware acceleration** - Support for NVENC, VAAPI, and QSV hardware encoders
- **Subtitle extraction** - Extract embedded subtitles in VTT, SRT, or ASS format
- **Thumbnail generation** - Create video thumbnails from key frames
- **Trickplay tiles** - Generate thumbnail sprite sheets for video scrubbing
- **Job queue** - Concurrent job processing with configurable limits
- **Progress tracking** - Real-time encoding progress updates
- **Multiple codecs** - Support for H.264, H.265, VP9, and AV1
- **Flexible profiles** - Customizable encoding profiles with presets
- **S3 input support** - Download and process videos directly from AWS S3 or S3-compatible storage

## Installation

```bash
cd plugins/media-processing/ts
npm install
npm run build
```

## Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

### Required Configuration

- `DATABASE_URL` - PostgreSQL connection string

### Optional Configuration

- `MP_PLUGIN_PORT` - HTTP server port (default: 3019)
- `MP_FFMPEG_PATH` - Path to ffmpeg binary (default: ffmpeg)
- `MP_FFPROBE_PATH` - Path to ffprobe binary (default: ffprobe)
- `MP_OUTPUT_BASE_PATH` - Base directory for outputs (default: /data/media-processing)
- `MP_MAX_CONCURRENT_JOBS` - Max concurrent encoding jobs (default: 2)
- `MP_MAX_INPUT_SIZE_GB` - Max input file size in GB (default: 50)
- `MP_HARDWARE_ACCEL` - Hardware acceleration (none, nvenc, vaapi, qsv)
- `MP_API_KEY` - API key for authentication
- `MP_RATE_LIMIT_MAX` - Max requests per window (default: 50)
- `MP_RATE_LIMIT_WINDOW_MS` - Rate limit window in ms (default: 60000)

### S3 Input Configuration

For S3 input support, configure AWS credentials:

- `AWS_ACCESS_KEY_ID` - AWS access key (or `MP_AWS_ACCESS_KEY_ID`)
- `AWS_SECRET_ACCESS_KEY` - AWS secret key (or `MP_AWS_SECRET_ACCESS_KEY`)
- `AWS_REGION` - AWS region (or `MP_AWS_REGION`, default: us-east-1)
- `MP_S3_ENDPOINT` - Custom S3 endpoint for MinIO or other S3-compatible services (optional)

## CLI Commands

### Initialize Database

```bash
npm run cli -- init
```

### Start Server

```bash
npm run cli -- server
# or in development
npm run dev
```

### Check Status

```bash
npm run cli -- status
```

### Submit Encoding Job

```bash
# Local file
npm run cli -- submit /path/to/video.mp4

# HTTP/HTTPS URL
npm run cli -- submit https://example.com/video.mp4 --type url

# S3 URL (s3:// format)
npm run cli -- submit s3://my-bucket/videos/input.mp4 --type s3

# S3 URL (HTTPS format)
npm run cli -- submit https://my-bucket.s3.us-east-1.amazonaws.com/videos/input.mp4 --type s3

# With custom profile and priority
npm run cli -- submit /path/to/video.mp4 --profile <profile-id> --priority 10
```

### List Jobs

```bash
npm run cli -- jobs
npm run cli -- jobs --status completed
npm run cli -- jobs --limit 50
```

### List Encoding Profiles

```bash
npm run cli -- profiles
```

### Analyze Media File

```bash
npm run cli -- analyze /path/to/video.mp4
```

### View Statistics

```bash
npm run cli -- stats
```

## API Endpoints

### Health Checks

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (includes database connectivity)
- `GET /live` - Liveness check with statistics
- `GET /v1/status` - Plugin status with configuration and queue info

### Encoding Profiles

- `POST /v1/profiles` - Create encoding profile
- `GET /v1/profiles` - List all profiles
- `GET /v1/profiles/:id` - Get profile details
- `PUT /v1/profiles/:id` - Update profile
- `DELETE /v1/profiles/:id` - Delete profile

### Jobs

- `POST /v1/jobs` - Submit new encoding job
- `GET /v1/jobs` - List jobs (supports filtering by status)
- `GET /v1/jobs/:id` - Get job details with outputs
- `POST /v1/jobs/:id/cancel` - Cancel running job
- `POST /v1/jobs/:id/retry` - Retry failed job
- `GET /v1/jobs/:id/outputs` - List job outputs
- `GET /v1/jobs/:id/hls` - Get HLS manifest info
- `GET /v1/jobs/:id/subtitles` - List extracted subtitles
- `GET /v1/jobs/:id/trickplay` - Get trickplay data

### Media Analysis

- `POST /v1/analyze` - Analyze media file (probe with ffprobe)
- `POST /v1/thumbnail` - Generate thumbnails from video URL

### Statistics

- `GET /v1/stats` - Processing statistics

## Database Schema

### Tables

1. **mp_encoding_profiles** - Encoding profile configurations
2. **mp_jobs** - Encoding job queue and status
3. **mp_job_outputs** - Generated output files
4. **mp_hls_manifests** - HLS playlist metadata
5. **mp_subtitles** - Extracted subtitle tracks
6. **mp_trickplay** - Trickplay thumbnail sprites
7. **mp_webhook_events** - Webhook event log

## Hardware Acceleration

### NVIDIA NVENC

```bash
MP_HARDWARE_ACCEL=nvenc
```

Requires NVIDIA GPU with NVENC support and appropriate drivers.

### Intel Quick Sync (QSV)

```bash
MP_HARDWARE_ACCEL=qsv
```

Requires Intel CPU with Quick Sync Video support.

### VAAPI (Linux)

```bash
MP_HARDWARE_ACCEL=vaapi
```

Requires VA-API compatible GPU and drivers.

## Example Usage

### Create Custom Encoding Profile

```bash
curl -X POST http://localhost:3019/v1/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "high-quality",
    "description": "High quality 4K encoding",
    "video_codec": "h265",
    "audio_codec": "aac",
    "preset": "slow",
    "resolutions": [
      {"width": 3840, "height": 2160, "bitrate": 20000000, "label": "4K"},
      {"width": 1920, "height": 1080, "bitrate": 8000000, "label": "1080p"},
      {"width": 1280, "height": 720, "bitrate": 5000000, "label": "720p"}
    ],
    "hls_enabled": true,
    "thumbnail_enabled": true,
    "subtitle_extract": true,
    "trickplay_enabled": true
  }'
```

### Submit Encoding Job

```bash
# Local file
curl -X POST http://localhost:3019/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input_url": "/path/to/video.mp4",
    "input_type": "file",
    "profile_id": "profile-uuid-here",
    "priority": 5
  }'

# S3 input
curl -X POST http://localhost:3019/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "input_url": "s3://my-bucket/videos/input.mp4",
    "input_type": "s3",
    "profile_id": "profile-uuid-here",
    "priority": 5
  }'
```

### Check Job Status

```bash
curl http://localhost:3019/v1/jobs/job-uuid-here
```

### Analyze Media File

```bash
curl -X POST http://localhost:3019/v1/analyze \
  -H "Content-Type: application/json" \
  -d '{"url": "/path/to/video.mp4"}'
```

## Performance Tips

1. **Hardware Acceleration** - Use NVENC, QSV, or VAAPI for 3-5x faster encoding
2. **Concurrent Jobs** - Adjust `MP_MAX_CONCURRENT_JOBS` based on CPU/GPU capacity
3. **Preset Selection** - Use faster presets (veryfast, faster) for lower quality/size ratio
4. **Resolution Limits** - Limit maximum resolution based on content requirements
5. **HLS Segment Duration** - Adjust segment duration based on network conditions

## Troubleshooting

### FFmpeg Not Found

Ensure FFmpeg is installed and in PATH:

```bash
which ffmpeg
which ffprobe
```

Or set explicit paths:

```bash
MP_FFMPEG_PATH=/usr/local/bin/ffmpeg
MP_FFPROBE_PATH=/usr/local/bin/ffprobe
```

### Hardware Acceleration Errors

Verify hardware support:

```bash
# NVENC
ffmpeg -encoders | grep nvenc

# QSV
ffmpeg -encoders | grep qsv

# VAAPI
ffmpeg -encoders | grep vaapi
```

### Job Stuck in Queue

Check server logs and ensure:
- Server is running
- Database is accessible
- Sufficient disk space for outputs
- Input file is accessible

### Out of Memory

Reduce concurrent jobs:

```bash
MP_MAX_CONCURRENT_JOBS=1
```

## License

Source-Available - See repository LICENSE file
