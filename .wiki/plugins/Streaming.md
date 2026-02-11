# Streaming

Live streaming and broadcasting with RTMP/HLS, viewer analytics, chat integration, multi-quality streams, DVR playback, and moderation.

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The Streaming plugin provides a complete live streaming platform for nself applications. It supports RTMP/HLS streaming, multi-bitrate adaptive streaming, DVR playback, viewer analytics, integrated chat, recording, clipping, moderation, and scheduled streams.

This plugin is essential for applications requiring live video streaming, broadcasting, webinars, or live events.

### Key Features

- **RTMP Ingestion**: Accept RTMP streams from OBS, XSplit, and other encoders
- **HLS Delivery**: Adaptive bitrate HLS streaming for optimal viewing experience
- **DVR/Time-Shift**: Viewers can pause and rewind live streams
- **Multi-Bitrate**: Automatic transcoding to multiple quality levels
- **Recording**: Automatic recording of live streams for VOD playback
- **Clipping**: Create highlight clips from live or recorded streams
- **Chat Integration**: Built-in chat synchronized with stream
- **Viewer Analytics**: Real-time and historical viewer metrics
- **Moderation**: Content moderation and stream reporting
- **Stream Keys**: Secure stream keys with usage tracking
- **Scheduled Streams**: Pre-schedule streams with notifications
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Features

- **Ingest Protocols**: RTMP, RTMPS, SRT
- **Delivery Protocols**: HLS, DASH, WebRTC (planned)
- **Video Codecs**: H.264, H.265/HEVC
- **Audio Codecs**: AAC, MP3
- **Resolutions**: 1080p, 720p, 480p, 360p, 240p
- **DVR Window**: Configurable time-shift window
- **Recording Formats**: MP4, HLS segments
- **Storage**: Local filesystem, S3-compatible storage

### Use Cases

1. **Live Events**: Conferences, concerts, sports events
2. **Webinars**: Educational content and training sessions
3. **Gaming Streams**: Live gameplay streaming with chat
4. **Product Launches**: Live product demonstrations
5. **Town Halls**: Company-wide broadcasts and Q&A

## Quick Start

```bash
# Install the plugin
nself plugin install streaming

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export STREAMING_PLUGIN_PORT=3711
export S3_BUCKET="my-recordings-bucket"

# Initialize database schema
nself plugin streaming init

# Start the streaming plugin server
nself plugin streaming server

# Check status
nself plugin streaming status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `STREAMING_PLUGIN_PORT` | No | `3711` | HTTP server port |
| `S3_BUCKET` | No | - | S3 bucket for recordings |
| `S3_REGION` | No | `us-east-1` | S3 region |
| `S3_ACCESS_KEY` | No | - | S3 access key |
| `S3_SECRET_KEY` | No | - | S3 secret key |
| `CDN_URL` | No | - | CDN base URL for streams |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
STREAMING_PLUGIN_PORT=3711

# Storage Configuration
S3_BUCKET=my-streaming-recordings
S3_REGION=us-east-1
S3_ACCESS_KEY=your-access-key
S3_SECRET_KEY=your-secret-key

# CDN Configuration
CDN_URL=https://cdn.example.com
```

## CLI Commands

### Global Commands

#### `init`
Initialize the streaming plugin database schema.

```bash
nself plugin streaming init
```

#### `server`
Start the streaming plugin HTTP server.

```bash
nself plugin streaming server
```

#### `status`
Display current streaming plugin status.

```bash
nself plugin streaming status
```

### Stream Management

#### `streams`
Manage streams.

```bash
nself plugin streaming streams list
nself plugin streaming streams info STREAM_ID
nself plugin streaming streams create "My Stream" --title "Weekly Gaming Session"
nself plugin streaming streams end STREAM_ID
```

### Recording Management

#### `recordings`
Manage recordings.

```bash
nself plugin streaming recordings list
nself plugin streaming recordings info RECORDING_ID
nself plugin streaming recordings publish RECORDING_ID
```

### Schedule Management

#### `schedule`
Manage scheduled streams.

```bash
nself plugin streaming schedule list
nself plugin streaming schedule create "Weekly Show" --start "2024-02-10T20:00:00Z"
```

## REST API

### Stream Management

#### `POST /api/streaming/streams`
Create a stream.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440000",
  "title": "Weekly Gaming Session",
  "description": "Playing the latest games",
  "category": "gaming",
  "tags": ["fps", "multiplayer"],
  "isPublic": true,
  "enableChat": true,
  "enableDvr": true,
  "dvrWindowSeconds": 7200,
  "qualities": ["1080p", "720p", "480p"]
}
```

**Response:**
```json
{
  "success": true,
  "stream": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "streamKey": "sk_live_abc123xyz...",
    "rtmpUrl": "rtmp://ingest.example.com/live",
    "playbackUrl": "https://cdn.example.com/streams/abc123/index.m3u8",
    "status": "created"
  }
}
```

#### `GET /api/streaming/streams/:streamId`
Get stream details.

#### `PATCH /api/streaming/streams/:streamId`
Update stream.

#### `POST /api/streaming/streams/:streamId/start`
Start/go live with stream.

#### `POST /api/streaming/streams/:streamId/end`
End stream.

#### `GET /api/streaming/streams`
List streams with filters.

**Query Parameters:**
- `userId` - Filter by user
- `status` - Filter by status (live, ended, created)
- `category` - Filter by category
- `limit` - Result limit
- `offset` - Result offset

### Stream Keys

#### `POST /api/streaming/streams/:streamId/keys/rotate`
Rotate stream key.

**Response:**
```json
{
  "success": true,
  "streamKey": "sk_live_new123xyz..."
}
```

### Viewer Management

#### `GET /api/streaming/streams/:streamId/viewers`
Get current viewers.

#### `GET /api/streaming/streams/:streamId/analytics`
Get stream analytics.

**Response:**
```json
{
  "success": true,
  "analytics": {
    "currentViewers": 1234,
    "peakViewers": 2567,
    "totalViews": 15678,
    "avgWatchTime": 1800,
    "chatMessages": 5432
  }
}
```

### Recording Management

#### `GET /api/streaming/recordings`
List recordings.

#### `GET /api/streaming/recordings/:recordingId`
Get recording details.

#### `POST /api/streaming/recordings/:recordingId/publish`
Publish recording as VOD.

#### `DELETE /api/streaming/recordings/:recordingId`
Delete recording.

### Clip Management

#### `POST /api/streaming/clips`
Create clip from stream or recording.

**Request:**
```json
{
  "streamId": "550e8400-e29b-41d4-a716-446655440001",
  "startOffset": 3600,
  "duration": 30,
  "title": "Epic Moment"
}
```

#### `GET /api/streaming/clips`
List clips.

#### `GET /api/streaming/clips/:clipId`
Get clip details.

### Chat Management

#### `POST /api/streaming/streams/:streamId/chat/messages`
Send chat message.

#### `GET /api/streaming/streams/:streamId/chat/messages`
Get chat messages.

#### `DELETE /api/streaming/streams/:streamId/chat/messages/:messageId`
Delete chat message (moderation).

### Moderation

#### `POST /api/streaming/streams/:streamId/moderators`
Add moderator.

#### `GET /api/streaming/streams/:streamId/moderators`
List moderators.

#### `POST /api/streaming/streams/:streamId/flag`
Flag stream for review.

#### `POST /api/streaming/streams/:streamId/takedown`
Take down stream (admin).

### Scheduled Streams

#### `POST /api/streaming/schedule`
Schedule a stream.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440000",
  "title": "Weekly Gaming Session",
  "scheduledStart": "2024-02-10T20:00:00Z",
  "estimatedDuration": 7200,
  "notifyFollowers": true
}
```

#### `GET /api/streaming/schedule`
List scheduled streams.

### Reports

#### `POST /api/streaming/reports`
Submit stream report.

**Request:**
```json
{
  "streamId": "550e8400-e29b-41d4-a716-446655440001",
  "reporterId": "550e8400-e29b-41d4-a716-446655440002",
  "reason": "inappropriate_content",
  "description": "Violates community guidelines"
}
```

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events.

## Webhook Events

### Stream Events

#### `stream.started`
A live stream started.

**Payload:**
```json
{
  "type": "stream.started",
  "stream": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "userId": "550e8400-e29b-41d4-a716-446655440000",
    "title": "Weekly Gaming Session"
  },
  "timestamp": "2024-02-10T20:00:00Z"
}
```

#### `stream.ended`
A live stream ended.

#### `stream.flagged`
A stream was flagged for moderation.

#### `stream.taken_down`
A stream was taken down.

### Recording Events

#### `recording.ready`
A recording finished processing.

**Payload:**
```json
{
  "type": "recording.ready",
  "recording": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "streamId": "550e8400-e29b-41d4-a716-446655440001",
    "duration": 7200,
    "fileUrl": "https://cdn.example.com/recordings/abc123.mp4"
  },
  "timestamp": "2024-02-10T22:00:00Z"
}
```

#### `clip.ready`
A clip finished processing.

### Viewer Events

#### `viewer.joined`
A viewer joined a stream.

#### `viewer.left`
A viewer left a stream.

### Chat Events

#### `chat.message`
A chat message was sent.

### Report Events

#### `report.created`
A stream was reported.

## Database Schema

### streaming_streams

Live streams.

```sql
CREATE TABLE streaming_streams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id UUID NOT NULL,
  title VARCHAR(500) NOT NULL,
  description TEXT,
  category VARCHAR(100),
  tags TEXT[] DEFAULT '{}',
  stream_key VARCHAR(255) NOT NULL,
  stream_key_hash VARCHAR(255) NOT NULL,
  is_public BOOLEAN NOT NULL DEFAULT true,
  enable_chat BOOLEAN NOT NULL DEFAULT true,
  enable_dvr BOOLEAN NOT NULL DEFAULT true,
  dvr_window_seconds INTEGER DEFAULT 7200,
  status VARCHAR(50) NOT NULL DEFAULT 'created',
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  current_viewers INTEGER DEFAULT 0,
  peak_viewers INTEGER DEFAULT 0,
  total_views INTEGER DEFAULT 0,
  chat_message_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, stream_key_hash)
);

CREATE INDEX idx_streaming_streams_account ON streaming_streams(source_account_id);
CREATE INDEX idx_streaming_streams_user ON streaming_streams(user_id);
CREATE INDEX idx_streaming_streams_status ON streaming_streams(status);
CREATE INDEX idx_streaming_streams_category ON streaming_streams(category);
CREATE INDEX idx_streaming_streams_started ON streaming_streams(started_at DESC);
```

### streaming_keys

Stream key management.

```sql
CREATE TABLE streaming_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  key_value VARCHAR(255) NOT NULL,
  key_hash VARCHAR(255) NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  last_used_at TIMESTAMPTZ,
  use_count INTEGER DEFAULT 0,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, key_hash)
);

CREATE INDEX idx_streaming_keys_account ON streaming_keys(source_account_id);
CREATE INDEX idx_streaming_keys_stream ON streaming_keys(stream_id);
CREATE INDEX idx_streaming_keys_active ON streaming_keys(is_active) WHERE is_active = true;
```

### streaming_viewers

Active viewers tracking.

```sql
CREATE TABLE streaming_viewers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  user_id UUID,
  session_id VARCHAR(255) NOT NULL,
  ip_address VARCHAR(45),
  user_agent TEXT,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  left_at TIMESTAMPTZ,
  watch_duration_seconds INTEGER DEFAULT 0,
  UNIQUE(source_account_id, stream_id, session_id)
);

CREATE INDEX idx_streaming_viewers_account ON streaming_viewers(source_account_id);
CREATE INDEX idx_streaming_viewers_stream ON streaming_viewers(stream_id);
CREATE INDEX idx_streaming_viewers_user ON streaming_viewers(user_id);
CREATE INDEX idx_streaming_viewers_session ON streaming_viewers(session_id);
CREATE INDEX idx_streaming_viewers_active ON streaming_viewers(left_at) WHERE left_at IS NULL;
```

### streaming_recordings

Stream recordings.

```sql
CREATE TABLE streaming_recordings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  title VARCHAR(500) NOT NULL,
  description TEXT,
  duration_seconds INTEGER,
  file_url TEXT,
  file_size_bytes BIGINT,
  thumbnail_url TEXT,
  status VARCHAR(50) NOT NULL DEFAULT 'processing',
  is_published BOOLEAN NOT NULL DEFAULT false,
  view_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  started_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_streaming_recordings_account ON streaming_recordings(source_account_id);
CREATE INDEX idx_streaming_recordings_stream ON streaming_recordings(stream_id);
CREATE INDEX idx_streaming_recordings_status ON streaming_recordings(status);
CREATE INDEX idx_streaming_recordings_published ON streaming_recordings(is_published) WHERE is_published = true;
```

### streaming_clips

Stream clips.

```sql
CREATE TABLE streaming_clips (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  recording_id UUID REFERENCES streaming_recordings(id) ON DELETE CASCADE,
  created_by UUID NOT NULL,
  title VARCHAR(500) NOT NULL,
  start_offset INTEGER NOT NULL,
  duration INTEGER NOT NULL,
  file_url TEXT,
  thumbnail_url TEXT,
  status VARCHAR(50) NOT NULL DEFAULT 'processing',
  view_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_streaming_clips_account ON streaming_clips(source_account_id);
CREATE INDEX idx_streaming_clips_stream ON streaming_clips(stream_id);
CREATE INDEX idx_streaming_clips_recording ON streaming_clips(recording_id);
CREATE INDEX idx_streaming_clips_creator ON streaming_clips(created_by);
CREATE INDEX idx_streaming_clips_status ON streaming_clips(status);
```

### streaming_analytics

Analytics data points.

```sql
CREATE TABLE streaming_analytics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  timestamp TIMESTAMPTZ NOT NULL,
  viewer_count INTEGER NOT NULL,
  chat_messages_per_minute INTEGER DEFAULT 0,
  bitrate_kbps INTEGER,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_streaming_analytics_account ON streaming_analytics(source_account_id);
CREATE INDEX idx_streaming_analytics_stream ON streaming_analytics(stream_id);
CREATE INDEX idx_streaming_analytics_timestamp ON streaming_analytics(timestamp DESC);
```

### streaming_moderators

Stream moderators.

```sql
CREATE TABLE streaming_moderators (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  granted_by UUID NOT NULL,
  permissions BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, stream_id, user_id)
);

CREATE INDEX idx_streaming_moderators_account ON streaming_moderators(source_account_id);
CREATE INDEX idx_streaming_moderators_stream ON streaming_moderators(stream_id);
CREATE INDEX idx_streaming_moderators_user ON streaming_moderators(user_id);
```

### streaming_chat_messages

Chat messages.

```sql
CREATE TABLE streaming_chat_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  message TEXT NOT NULL,
  is_deleted BOOLEAN NOT NULL DEFAULT false,
  deleted_by UUID,
  deleted_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_streaming_chat_account ON streaming_chat_messages(source_account_id);
CREATE INDEX idx_streaming_chat_stream ON streaming_chat_messages(stream_id);
CREATE INDEX idx_streaming_chat_user ON streaming_chat_messages(user_id);
CREATE INDEX idx_streaming_chat_created ON streaming_chat_messages(created_at DESC);
CREATE INDEX idx_streaming_chat_active ON streaming_chat_messages(is_deleted) WHERE is_deleted = false;
```

### streaming_reports

Stream reports.

```sql
CREATE TABLE streaming_reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  stream_id UUID NOT NULL REFERENCES streaming_streams(id) ON DELETE CASCADE,
  reporter_id UUID NOT NULL,
  reason VARCHAR(100) NOT NULL,
  description TEXT,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  reviewed_by UUID,
  reviewed_at TIMESTAMPTZ,
  action_taken VARCHAR(100),
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_streaming_reports_account ON streaming_reports(source_account_id);
CREATE INDEX idx_streaming_reports_stream ON streaming_reports(stream_id);
CREATE INDEX idx_streaming_reports_reporter ON streaming_reports(reporter_id);
CREATE INDEX idx_streaming_reports_status ON streaming_reports(status);
```

### streaming_schedule

Scheduled streams.

```sql
CREATE TABLE streaming_schedule (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id UUID NOT NULL,
  title VARCHAR(500) NOT NULL,
  description TEXT,
  scheduled_start TIMESTAMPTZ NOT NULL,
  estimated_duration INTEGER,
  stream_id UUID REFERENCES streaming_streams(id) ON DELETE SET NULL,
  status VARCHAR(50) NOT NULL DEFAULT 'scheduled',
  notify_followers BOOLEAN NOT NULL DEFAULT true,
  notified_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_streaming_schedule_account ON streaming_schedule(source_account_id);
CREATE INDEX idx_streaming_schedule_user ON streaming_schedule(user_id);
CREATE INDEX idx_streaming_schedule_start ON streaming_schedule(scheduled_start);
CREATE INDEX idx_streaming_schedule_status ON streaming_schedule(status);
```

## Examples

### Example 1: Create and Start Stream

```bash
# Create stream
curl -X POST http://localhost:3711/api/streaming/streams \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "USER_ID",
    "title": "Evening Gaming Session",
    "category": "gaming",
    "isPublic": true,
    "enableChat": true
  }'

# Save stream key from response
# Configure OBS with RTMP URL and stream key
# Start streaming in OBS

# Verify stream is live
curl http://localhost:3711/api/streaming/streams/STREAM_ID
```

### Example 2: Monitor Stream Analytics

```bash
# Get current viewers
curl http://localhost:3711/api/streaming/streams/STREAM_ID/viewers

# Get stream analytics
curl http://localhost:3711/api/streaming/streams/STREAM_ID/analytics
```

### Example 3: Create Clip

```bash
# Create 30-second clip starting at 1 hour mark
curl -X POST http://localhost:3711/api/streaming/clips \
  -H "Content-Type: application/json" \
  -d '{
    "streamId": "STREAM_ID",
    "startOffset": 3600,
    "duration": 30,
    "title": "Epic Play"
  }'
```

### Example 4: Schedule Stream

```bash
# Schedule stream for next week
curl -X POST http://localhost:3711/api/streaming/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "USER_ID",
    "title": "Weekly Show",
    "scheduledStart": "2024-02-17T20:00:00Z",
    "estimatedDuration": 7200,
    "notifyFollowers": true
  }'
```

### Example 5: Moderate Chat

```bash
# Delete inappropriate message
curl -X DELETE http://localhost:3711/api/streaming/streams/STREAM_ID/chat/messages/MESSAGE_ID

# Add moderator
curl -X POST http://localhost:3711/api/streaming/streams/STREAM_ID/moderators \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "MOD_USER_ID",
    "grantedBy": "STREAMER_USER_ID"
  }'
```

## Troubleshooting

### Stream Connection Issues

**Problem:** Cannot connect to RTMP server

**Solutions:**
1. Verify RTMP port (1935) is accessible
2. Check stream key is correct and active
3. Ensure firewall allows RTMP traffic
4. Test with a different encoder (OBS, XSplit)

### Playback Issues

**Problem:** Stream not playing or buffering

**Solutions:**
1. Check CDN configuration
2. Verify HLS segments are being generated
3. Test different quality levels
4. Review bandwidth and transcoding capacity

### Recording Problems

**Problem:** Recordings not saving

**Solutions:**
1. Verify S3 credentials and bucket permissions
2. Check available storage space
3. Review recording status for errors
4. Ensure transcoding service is running

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
