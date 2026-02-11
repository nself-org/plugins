# Recording Plugin

Recording orchestration and archive management service with DVR functionality, auto-scheduling from sports events, and media enrichment.

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
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Recording plugin provides comprehensive DVR and recording orchestration capabilities with sports event integration and automatic metadata enrichment. It supports:

- **3 Database Tables** - Recordings, schedules, encode jobs
- **4 Analytics Views** - Success rate, storage, scheduled vs completed
- **2 Webhook Sources** - Sports events, device recordings
- **Auto-Scheduling** - Create recordings from sports event data
- **Media Enrichment** - Automatic TMDB metadata lookup
- **Encode Integration** - Transcode recordings on completion
- **Storage Integration** - Upload to object storage
- **Full REST API** - Complete recording lifecycle management
- **CLI Interface** - Command-line recording operations

### Key Features

| Feature | Description |
|---------|-------------|
| Sports Integration | Auto-schedule from sports events with lead/trail time |
| Device Control | Record from network-attached tuner devices |
| Metadata Enrichment | Automatic TMDB lookup and enrichment |
| Auto-Publishing | Publish completed recordings automatically |
| Encode Orchestration | Transcode on completion with profiles |
| Storage Upload | Upload to object storage on completion |
| Conflict Detection | Prevent overlapping recordings on same device |
| Priority Scheduling | High-priority recordings take precedence |

---

## Quick Start

```bash
# Install the plugin
nself plugin install recording

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "REC_STORAGE_URL=http://localhost:3301" >> .env
echo "REC_FILE_PROCESSING_URL=http://localhost:3019" >> .env

# Initialize database schema
nself plugin recording init

# Start server
nself plugin recording server --port 3602
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PORT` | No | `3602` | HTTP server port |
| `REC_STORAGE_URL` | No | - | Object storage plugin URL |
| `REC_FILE_PROCESSING_URL` | No | - | Media processing plugin URL |
| `REC_SPORTS_URL` | No | - | Sports data plugin URL |
| `REC_MEDIA_METADATA_URL` | No | - | Media metadata plugin URL |
| `REC_DEVICES_URL` | No | - | Devices plugin URL |
| `REC_DEFAULT_LEAD_TIME_MINUTES` | No | `5` | Default recording lead time |
| `REC_DEFAULT_TRAIL_TIME_MINUTES` | No | `15` | Default recording trail time |
| `REC_ENCODE_PROFILES` | No | - | Comma-separated encode profile IDs |
| `REC_DEFAULT_ENCODE_PROFILE` | No | - | Default encode profile ID |
| `REC_AUTO_ENCODE` | No | `false` | Auto-encode on completion |
| `REC_AUTO_ENRICH` | No | `true` | Auto-enrich with metadata |
| `REC_AUTO_PUBLISH` | No | `false` | Auto-publish on completion |
| `REC_MAX_CONCURRENT_RECORDINGS` | No | `4` | Max concurrent recordings |
| `REC_MAX_CONCURRENT_ENCODES` | No | `2` | Max concurrent encode jobs |
| `REC_STORAGE_PATH_TEMPLATE` | No | `{year}/{month}/{title}` | Storage path template |
| `REC_THUMBNAIL_AT_SECONDS` | No | `10` | Generate thumbnail at N seconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### App-Specific Configuration

Per-app configuration overrides (e.g., for "tv" app):

| Variable | Description |
|----------|-------------|
| `REC_APP_TV_MAX_CONCURRENT_RECORDINGS` | Max recordings for "tv" app |
| `REC_APP_TV_AUTO_PUBLISH` | Auto-publish for "tv" app |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Service URLs
REC_STORAGE_URL=http://localhost:3301
REC_FILE_PROCESSING_URL=http://localhost:3019
REC_SPORTS_URL=http://localhost:3701
REC_MEDIA_METADATA_URL=http://localhost:3202
REC_DEVICES_URL=http://localhost:3401

# Recording Settings
REC_DEFAULT_LEAD_TIME_MINUTES=5
REC_DEFAULT_TRAIL_TIME_MINUTES=15
REC_MAX_CONCURRENT_RECORDINGS=4

# Auto-Processing
REC_AUTO_ENCODE=true
REC_AUTO_ENRICH=true
REC_AUTO_PUBLISH=false
REC_DEFAULT_ENCODE_PROFILE=default

# Server
PORT=3602
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin recording init

# Start server
nself plugin recording server

# Custom port
nself plugin recording server --port 8080

# Check status
nself plugin recording status

# View statistics
nself plugin recording stats
```

### Recording Management

```bash
# List all recordings
nself plugin recording recordings list

# Filter by status
nself plugin recording recordings list --status scheduled
nself plugin recording recordings list --status recording
nself plugin recording recordings list --status completed

# Create manual recording
nself plugin recording recordings create \
  --title "Live Event" \
  --start "2026-02-11T18:00:00Z" \
  --end "2026-02-11T20:00:00Z" \
  --device tuner-1

# Create with channel
nself plugin recording recordings create \
  --title "Game Broadcast" \
  --start "2026-02-11T19:00:00Z" \
  --duration 180 \
  --channel ESPN \
  --device tuner-1

# Cancel recording
nself plugin recording recordings cancel <recording-id>

# Delete recording and files
nself plugin recording recordings delete <recording-id>
```

### Schedule Management

```bash
# Schedule from sports event
nself plugin recording schedule <event-id>

# Custom lead/trail time
nself plugin recording schedule <event-id> \
  --lead-time 10 \
  --trail-time 20

# High priority
nself plugin recording schedule <event-id> --priority high
```

### Archive Management

```bash
# List archived recordings
nself plugin recording archives list

# Filter by date
nself plugin recording archives list --since 2026-01-01

# Filter by category
nself plugin recording archives list --category sports
```

### Encode Status

```bash
# Show encode queue status
nself plugin recording encode-status

# List pending encode jobs
nself plugin recording encode-status --status pending
```

### Publishing

```bash
# Publish recording
nself plugin recording publish <recording-id>

# Publish to specific platform
nself plugin recording publish <recording-id> --platform cms
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
  "plugin": "recording",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "recording",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/status
Plugin status with statistics.

**Response:**
```json
{
  "plugin": "recording",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "totalRecordings": 250,
    "scheduled": 15,
    "recording": 2,
    "completed": 200,
    "failed": 33,
    "successRate": 85.8,
    "storageUsedGB": 1250.5
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Recordings

#### POST /v1/recordings
Create new recording.

**Request Body:**
```json
{
  "title": "Live Event Recording",
  "description": "Recording of live event",
  "source_type": "device",
  "source_device_id": "tuner-1",
  "source_channel": "ESPN",
  "scheduled_start": "2026-02-11T18:00:00Z",
  "scheduled_end": "2026-02-11T20:00:00Z",
  "priority": "normal",
  "category": "sports",
  "tags": ["football", "nfl"],
  "auto_enrich": true,
  "auto_publish": false
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "title": "Live Event Recording",
    "status": "scheduled",
    "scheduled_start": "2026-02-11T18:00:00.000Z",
    "scheduled_end": "2026-02-11T20:00:00.000Z",
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/recordings
List recordings.

**Query Parameters:**
- `status` (optional) - Filter by status
- `app_id` (optional) - Filter by app
- `category` (optional) - Filter by category
- `start_date` (optional) - Filter by start date
- `end_date` (optional) - Filter by end date
- `limit` (optional) - Max results (default: 50)
- `offset` (optional) - Pagination offset

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "title": "Live Event Recording",
      "status": "completed",
      "duration_seconds": 7200,
      "np_fileproc_size": 15728640000,
      "scheduled_start": "2026-02-11T18:00:00.000Z",
      "actual_start": "2026-02-11T17:55:00.000Z"
    }
  ],
  "pagination": {
    "total": 250,
    "limit": 50,
    "offset": 0
  }
}
```

#### GET /v1/recordings/:id
Get recording details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "app_id": "tv",
    "title": "Live Event Recording",
    "description": "Recording of live event",
    "source_type": "device",
    "source_device_id": "tuner-1",
    "source_channel": "ESPN",
    "status": "completed",
    "priority": "normal",
    "scheduled_start": "2026-02-11T18:00:00.000Z",
    "scheduled_end": "2026-02-11T20:00:00.000Z",
    "actual_start": "2026-02-11T17:55:00.000Z",
    "actual_end": "2026-02-11T20:02:00.000Z",
    "duration_seconds": 7620,
    "np_fileproc_path": "/data/recordings/2026/02/live-event.ts",
    "np_fileproc_size": 15728640000,
    "np_fileproc_format": "mpegts",
    "thumbnail_url": "https://storage.example.com/thumbs/recording.jpg",
    "encode_status": "completed",
    "encode_progress": 100,
    "publish_status": "unpublished",
    "storage_object_id": "obj_abc123",
    "np_sports_event_id": "evt_xyz789",
    "media_metadata_id": "123456",
    "enrichment_status": "completed",
    "tags": ["football", "nfl"],
    "category": "sports",
    "metadata": {},
    "created_at": "2026-02-11T10:00:00.000Z",
    "updated_at": "2026-02-11T20:05:00.000Z"
  }
}
```

#### PUT /v1/recordings/:id
Update recording.

**Request Body:**
```json
{
  "title": "Updated Title",
  "description": "Updated description",
  "category": "sports",
  "tags": ["football", "nfl", "playoffs"]
}
```

#### POST /v1/recordings/:id/cancel
Cancel scheduled or in-progress recording.

**Response:**
```json
{
  "success": true
}
```

#### DELETE /v1/recordings/:id
Delete recording and associated files.

**Response:**
```json
{
  "success": true
}
```

#### POST /v1/recordings/:id/publish
Publish recording.

**Request Body:**
```json
{
  "platform": "cms",
  "publish_options": {
    "visibility": "public",
    "category": "sports"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "publish_status": "published",
    "published_at": "2026-02-11T20:10:00.000Z",
    "published_url": "https://cms.example.com/videos/123"
  }
}
```

### Schedules

#### POST /v1/schedules
Create recurring schedule.

**Request Body:**
```json
{
  "name": "Weekly Sports Show",
  "schedule_type": "recurring",
  "source_channel": "ESPN",
  "source_device_id": "tuner-1",
  "recurrence_rule": "FREQ=WEEKLY;BYDAY=FR",
  "duration_minutes": 120,
  "lead_time_minutes": 5,
  "trail_time_minutes": 15,
  "auto_enrich": true,
  "auto_publish": false,
  "active": true
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "name": "Weekly Sports Show",
    "schedule_type": "recurring",
    "active": true,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/schedules
List schedules.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "name": "Weekly Sports Show",
      "schedule_type": "recurring",
      "duration_minutes": 120,
      "active": true
    }
  ]
}
```

#### GET /v1/schedules/:id
Get schedule details.

#### PUT /v1/schedules/:id
Update schedule.

#### DELETE /v1/schedules/:id
Delete schedule.

### Sports Event Scheduling

#### POST /v1/schedules/from-event
Create recording from sports event.

**Request Body:**
```json
{
  "event_id": "evt_xyz789",
  "source_device_id": "tuner-1",
  "source_channel": "ESPN",
  "lead_time_minutes": 10,
  "trail_time_minutes": 20,
  "priority": "high",
  "auto_enrich": true,
  "auto_publish": false
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "title": "Team A vs Team B",
    "np_sports_event_id": "evt_xyz789",
    "scheduled_start": "2026-02-11T19:00:00.000Z",
    "scheduled_end": "2026-02-11T22:30:00.000Z"
  }
}
```

### Statistics

#### GET /v1/stats
Get recording statistics.

**Response:**
```json
{
  "success": true,
  "data": {
    "totalRecordings": 250,
    "scheduled": 15,
    "recording": 2,
    "completed": 200,
    "failed": 33,
    "cancelled": 0,
    "successRate": 85.8,
    "totalDurationHours": 1250.5,
    "storageUsedBytes": 1342177280000,
    "storageUsedGB": 1250.0,
    "averageDurationMinutes": 127,
    "encodePending": 5,
    "encodeInProgress": 2,
    "encodeCompleted": 150,
    "lastRecordingAt": "2026-02-11T09:00:00.000Z"
  }
}
```

---

## Webhook Events

The plugin consumes webhooks from other services:

### Sports Webhooks

| Event | Description | Action |
|-------|-------------|--------|
| `event.scheduled` | Sports event scheduled | Auto-create recording if configured |
| `event.rescheduled` | Event time changed | Update recording schedule |
| `event.cancelled` | Event cancelled | Cancel recording |

### Device Webhooks

| Event | Description | Action |
|-------|-------------|--------|
| `recording.started` | Device started recording | Update status to "recording" |
| `recording.stopped` | Device stopped recording | Update status, trigger post-processing |
| `recording.failed` | Device recording failed | Update status to "failed" |

---

## Database Schema

### np_rec_recordings

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `title` | VARCHAR(512) | Recording title |
| `description` | TEXT | Description |
| `source_type` | VARCHAR(32) | Source type (device, stream, url) |
| `source_id` | VARCHAR(255) | Source identifier |
| `source_channel` | VARCHAR(128) | Channel name/number |
| `source_device_id` | VARCHAR(255) | Device ID |
| `status` | VARCHAR(32) | Recording status |
| `priority` | VARCHAR(16) | Priority (low, normal, high) |
| `scheduled_start` | TIMESTAMPTZ | Scheduled start time |
| `scheduled_end` | TIMESTAMPTZ | Scheduled end time |
| `actual_start` | TIMESTAMPTZ | Actual start time |
| `actual_end` | TIMESTAMPTZ | Actual end time |
| `duration_seconds` | INTEGER | Recording duration |
| `np_fileproc_path` | TEXT | File path |
| `np_fileproc_size` | BIGINT | File size (bytes) |
| `np_fileproc_format` | VARCHAR(16) | File format |
| `thumbnail_url` | TEXT | Thumbnail URL |
| `encode_status` | VARCHAR(32) | Encoding status |
| `encode_progress` | FLOAT | Encoding progress (0-100) |
| `encode_started_at` | TIMESTAMPTZ | Encoding start time |
| `encode_completed_at` | TIMESTAMPTZ | Encoding completion time |
| `publish_status` | VARCHAR(32) | Publishing status |
| `published_at` | TIMESTAMPTZ | Publication timestamp |
| `storage_object_id` | VARCHAR(255) | Object storage ID |
| `np_sports_event_id` | VARCHAR(255) | Sports event reference |
| `media_metadata_id` | VARCHAR(255) | TMDB metadata ID |
| `enrichment_status` | VARCHAR(32) | Enrichment status |
| `tags` | JSONB | Tags array |
| `category` | VARCHAR(128) | Category |
| `content_rating` | VARCHAR(16) | Content rating |
| `custom_fields` | JSONB | Custom fields |
| `metadata` | JSONB | Additional metadata |
| `created_by` | VARCHAR(255) | Creator ID |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |
| `deleted_at` | TIMESTAMPTZ | Soft delete timestamp |

**Recording Statuses:**
- `scheduled` - Waiting to start
- `recording` - Currently recording
- `completed` - Recording finished
- `failed` - Recording failed
- `cancelled` - Cancelled by user

**Publish Statuses:**
- `unpublished` - Not published
- `pending` - Publishing in progress
- `published` - Successfully published
- `failed` - Publishing failed

**Indexes:**
- `idx_rec_source_account` - source_account_id
- `idx_rec_app` - app_id
- `idx_rec_status` - status
- `idx_rec_scheduled` - scheduled_start
- `idx_rec_source` - (source_type, source_id)
- `idx_rec_device` - source_device_id
- `idx_rec_sports` - np_sports_event_id
- `idx_rec_published` - (publish_status, published_at)
- `idx_rec_tags` - tags (GIN)
- `idx_rec_category` - category

### np_rec_schedules

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `name` | VARCHAR(255) | Schedule name |
| `schedule_type` | VARCHAR(32) | Schedule type |
| `source_channel` | VARCHAR(128) | Channel name |
| `source_device_id` | VARCHAR(255) | Device ID |
| `recurrence_rule` | VARCHAR(255) | iCal recurrence rule |
| `duration_minutes` | INTEGER | Recording duration |
| `lead_time_minutes` | INTEGER | Lead time before start |
| `trail_time_minutes` | INTEGER | Trail time after end |
| `np_sports_league` | VARCHAR(64) | Sports league filter |
| `np_sports_team_id` | VARCHAR(255) | Team filter |
| `auto_enrich` | BOOLEAN | Auto-enrich enabled |
| `auto_publish` | BOOLEAN | Auto-publish enabled |
| `priority` | VARCHAR(16) | Priority |
| `active` | BOOLEAN | Schedule active |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Schedule Types:**
- `one-time` - Single recording
- `recurring` - Recurring schedule
- `sports-event` - Sports event based

**Indexes:**
- `idx_rec_schedules_account` - source_account_id
- `idx_rec_schedules_app` - app_id
- `idx_rec_schedules_active` - active

### np_rec_encode_jobs

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `recording_id` | UUID | Recording reference |
| `profile_id` | VARCHAR(255) | Encode profile ID |
| `status` | VARCHAR(32) | Encode status |
| `progress` | FLOAT | Progress (0-100) |
| `input_path` | TEXT | Input file path |
| `output_path` | TEXT | Output file path |
| `error_message` | TEXT | Error message |
| `started_at` | TIMESTAMPTZ | Start timestamp |
| `completed_at` | TIMESTAMPTZ | Completion timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Encode Statuses:**
- `pending` - Waiting to start
- `encoding` - Encoding in progress
- `completed` - Encoding finished
- `failed` - Encoding failed

**Indexes:**
- `idx_rec_encode_account` - source_account_id
- `idx_rec_encode_recording` - recording_id
- `idx_rec_encode_status` - status

---

## Analytics Views

### np_rec_recordings_by_status

Recordings grouped by status.

```sql
SELECT
  status,
  COUNT(*) as count,
  SUM(np_fileproc_size) as total_bytes
FROM np_rec_recordings
WHERE deleted_at IS NULL
GROUP BY status;
```

### np_rec_storage_by_type

Storage usage by source type.

```sql
SELECT
  source_type,
  COUNT(*) as count,
  SUM(np_fileproc_size) / 1024 / 1024 / 1024 as total_gb
FROM np_rec_recordings
WHERE status = 'completed'
  AND deleted_at IS NULL
GROUP BY source_type;
```

### np_rec_success_rate

Recording success rate over time.

```sql
SELECT
  DATE_TRUNC('day', created_at) as date,
  COUNT(*) as total,
  COUNT(*) FILTER (WHERE status = 'completed') as completed,
  ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'completed') / COUNT(*), 2) as success_rate
FROM np_rec_recordings
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY DATE_TRUNC('day', created_at)
ORDER BY date DESC;
```

### np_rec_scheduled_vs_completed

Scheduled vs completed recordings.

```sql
SELECT
  DATE_TRUNC('day', scheduled_start) as date,
  COUNT(*) as scheduled,
  COUNT(*) FILTER (WHERE status = 'completed') as completed,
  COUNT(*) FILTER (WHERE status = 'failed') as failed
FROM np_rec_recordings
WHERE scheduled_start >= NOW() - INTERVAL '30 days'
GROUP BY DATE_TRUNC('day', scheduled_start)
ORDER BY date DESC;
```

---

## TypeScript Implementation

### File Structure

```
plugins/recording/ts/src/
├── types.ts          # TypeScript interfaces
├── config.ts         # Configuration loading
├── database.ts       # Database operations
├── scheduler.ts      # Recording scheduling logic
├── orchestrator.ts   # Recording orchestration
├── server.ts         # HTTP server
├── cli.ts            # CLI commands
└── index.ts          # Module exports
```

### Key Components

#### RecordingScheduler (scheduler.ts)
- Schedule creation and management
- Sports event integration
- Conflict detection
- Priority handling

#### RecordingOrchestrator (orchestrator.ts)
- Recording lifecycle management
- Device coordination
- Post-processing orchestration
- Error handling

#### RecordingDatabase (database.ts)
- Schema initialization
- CRUD operations
- Analytics queries
- Statistics

---

## Examples

### Example 1: Schedule from Sports Event

```typescript
async function scheduleGameRecording(eventId: string) {
  const response = await fetch('http://localhost:3602/v1/schedules/from-event', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      event_id: eventId,
      source_device_id: 'tuner-1',
      source_channel: 'ESPN',
      lead_time_minutes: 10,
      trail_time_minutes: 20,
      priority: 'high',
      auto_enrich: true
    })
  });

  const { data: recording } = await response.json();
  console.log(`Scheduled: ${recording.title}`);
}
```

### Example 2: Monitor Active Recordings

```bash
#!/bin/bash

# Check active recordings every 30 seconds
while true; do
  echo "=== Active Recordings ==="
  curl -s http://localhost:3602/v1/recordings?status=recording | jq -r '.data[] | "\(.title): \(.duration_seconds)s"'

  sleep 30
done
```

### Example 3: Auto-Publish Workflow

```sql
-- Find completed, enriched, unpublished recordings
SELECT
  id,
  title,
  np_fileproc_path,
  media_metadata_id
FROM np_rec_recordings
WHERE status = 'completed'
  AND enrichment_status = 'completed'
  AND publish_status = 'unpublished'
  AND created_at >= NOW() - INTERVAL '7 days'
ORDER BY created_at DESC;

-- Publish via API
-- curl -X POST http://localhost:3602/v1/recordings/{id}/publish \
--   -H "Content-Type: application/json" \
--   -d '{"platform": "cms"}'
```

### Example 4: Storage Usage Report

```sql
-- Storage usage by category
SELECT
  category,
  COUNT(*) as recordings,
  SUM(np_fileproc_size) / 1024 / 1024 / 1024 as size_gb,
  AVG(duration_seconds) / 60 as avg_duration_minutes
FROM np_rec_recordings
WHERE status = 'completed'
  AND deleted_at IS NULL
GROUP BY category
ORDER BY size_gb DESC;

-- Cleanup old recordings
DELETE FROM np_rec_recordings
WHERE status = 'completed'
  AND published_at < NOW() - INTERVAL '90 days'
  AND deleted_at IS NULL;
```

---

## Troubleshooting

### Common Issues

#### Device Not Available

**Error:**
```
Error: Device tuner-1 is busy
```

**Solution:**
1. Check active recordings: `nself plugin recording recordings list --status recording`
2. Cancel conflicting recording if needed
3. Use different tuner device
4. Increase `REC_MAX_CONCURRENT_RECORDINGS`

#### Recording Failed to Start

**Error:**
```
status: failed, error: "Device connection failed"
```

**Solution:**
1. Verify device is online
2. Check device plugin status
3. Test device connectivity
4. Review device plugin logs

#### Encoding Failed

**Error:**
```
encode_status: failed
```

**Solution:**
1. Check media processing plugin status
2. Verify encode profile exists
3. Check input file integrity
4. Review encoding logs

#### Enrichment Failed

**Error:**
```
enrichment_status: failed
```

**Solution:**
1. Check media metadata plugin status
2. Verify TMDB API key
3. Check title/year format
4. Manual metadata lookup

#### Storage Upload Failed

**Error:**
```
Error: Upload to storage failed
```

**Solution:**
1. Check object storage plugin status
2. Verify storage credentials
3. Check disk space
4. Review storage logs

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug nself plugin recording server
```

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Recording
- **Issues**: https://github.com/acamarata/nself-plugins/issues
