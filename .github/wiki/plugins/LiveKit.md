# LiveKit

LiveKit voice/video infrastructure - room management, participant tracking, recording/egress, quality monitoring.

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

The LiveKit plugin provides comprehensive voice and video infrastructure for nself applications through integration with LiveKit Server. It manages rooms, participants, recordings, egress jobs, access tokens, and quality metrics, offering a complete WebRTC-based communication platform.

This plugin is essential for applications requiring real-time voice and video communication, enabling features like video conferencing, live streaming, webinars, and online collaboration.

### Key Features

- **Room Management**: Create, monitor, and close LiveKit rooms with configurable participant limits
- **Participant Tracking**: Real-time tracking of participants with status, media state, and connection quality
- **Token Generation**: Secure JWT-based access token generation with customizable grants and TTL
- **Recording & Egress**: Multiple output formats including room composite, track-based, and live streaming
- **Quality Monitoring**: Real-time quality metrics including bitrate, latency, packet loss, and connection type
- **SFU Architecture**: Leverages LiveKit's Selective Forwarding Unit for efficient media distribution
- **TURN Server Support**: Built-in NAT traversal for reliable connectivity
- **Multi-Account Isolation**: Full support for multi-tenant applications with data isolation
- **WebRTC Standards**: Full compliance with WebRTC standards for broad device compatibility
- **Scalable Infrastructure**: Handles hundreds of concurrent rooms and thousands of participants

### Supported Features

- **Room Types**: Audio-only, video, screen sharing, and mixed-mode rooms
- **Media Tracks**: Camera, microphone, screen share with individual control
- **Recording Outputs**: File (MP4, WebM), HLS streaming, RTMP streaming
- **Quality Tiers**: Adaptive bitrate with multiple resolution layers
- **Connection Types**: WebRTC, WHIP, WHEP protocols
- **Egress Destinations**: Local filesystem, S3-compatible storage, CDN endpoints

### Use Cases

1. **Video Conferencing**: Build Zoom-like conference platforms with screen sharing
2. **Live Streaming**: Stream events to large audiences with DVR capabilities
3. **Webinars**: Host interactive webinars with Q&A and participant management
4. **Online Education**: Virtual classrooms with breakout rooms and recording
5. **Telehealth**: HIPAA-compliant video consultations with recording
6. **Customer Support**: Live video support sessions with quality monitoring

## Quick Start

```bash
# Install the plugin
nself plugin install livekit

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export LIVEKIT_URL="wss://your-livekit-server:7880"
export LIVEKIT_API_KEY="your-api-key"
export LIVEKIT_API_SECRET="your-api-secret"
export LIVEKIT_PLUGIN_PORT=3707

# Initialize database schema
nself plugin livekit init

# Start the LiveKit plugin server
nself plugin livekit server

# Check status
nself plugin livekit status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `LIVEKIT_URL` | Yes | `wss://localhost:7880` | LiveKit server WebSocket URL |
| `LIVEKIT_API_KEY` | Yes | - | LiveKit API key |
| `LIVEKIT_API_SECRET` | Yes | - | LiveKit API secret |
| `LIVEKIT_PLUGIN_PORT` | No | `3707` | HTTP server port |
| `LIVEKIT_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | ` ` (empty) | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LIVEKIT_PORT` | No | `7880` | LiveKit server main port |
| `LIVEKIT_RTMP_PORT` | No | `1935` | LiveKit RTMP ingress port |
| `LIVEKIT_TURN_PORT` | No | `3478` | LiveKit TURN server port |
| `LIVEKIT_EGRESS_ENABLED` | No | `true` | Enable egress/recording features |
| `LIVEKIT_RECORDINGS_PATH` | No | `/var/livekit/recordings` | Local filesystem path for recordings |
| `LIVEKIT_RECORDINGS_S3_BUCKET` | No | - | S3 bucket name for recording storage |
| `LIVEKIT_QUALITY_MONITORING_ENABLED` | No | `true` | Enable quality metrics collection |
| `LIVEKIT_QUALITY_SAMPLE_INTERVAL` | No | `10` | Quality sampling interval (seconds) |
| `LIVEKIT_TOKEN_DEFAULT_TTL` | No | `3600` | Default token TTL (seconds) |
| `LIVEKIT_TOKEN_MAX_TTL` | No | `86400` | Maximum token TTL (seconds, 24 hours) |
| `LIVEKIT_ROOM_DEFAULT_MAX_PARTICIPANTS` | No | `100` | Default room participant limit |
| `LIVEKIT_ROOM_EMPTY_TIMEOUT` | No | `300` | Auto-close empty rooms after N seconds |
| `LIVEKIT_MAX_CONCURRENT_ROOMS` | No | `100` | Maximum concurrent active rooms |
| `LIVEKIT_MAX_PARTICIPANTS_PER_ROOM` | No | `200` | Maximum participants per room limit |
| `LIVEKIT_API_KEY_AUTH` | No | - | API key for authenticated requests |
| `LIVEKIT_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `LIVEKIT_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
POSTGRES_SSL=false

# Server Configuration
LIVEKIT_PLUGIN_PORT=3707
LIVEKIT_PLUGIN_HOST=0.0.0.0

# LiveKit Server
LIVEKIT_URL=wss://livekit.example.com:7880
LIVEKIT_API_KEY=your-api-key-here
LIVEKIT_API_SECRET=your-api-secret-here
LIVEKIT_PORT=7880
LIVEKIT_RTMP_PORT=1935
LIVEKIT_TURN_PORT=3478

# Recording/Egress
LIVEKIT_EGRESS_ENABLED=true
LIVEKIT_RECORDINGS_PATH=/var/livekit/recordings
LIVEKIT_RECORDINGS_S3_BUCKET=my-recordings-bucket

# Quality Monitoring
LIVEKIT_QUALITY_MONITORING_ENABLED=true
LIVEKIT_QUALITY_SAMPLE_INTERVAL=10

# Token Configuration
LIVEKIT_TOKEN_DEFAULT_TTL=3600
LIVEKIT_TOKEN_MAX_TTL=86400

# Room Configuration
LIVEKIT_ROOM_DEFAULT_MAX_PARTICIPANTS=100
LIVEKIT_ROOM_EMPTY_TIMEOUT=300
LIVEKIT_MAX_CONCURRENT_ROOMS=100
LIVEKIT_MAX_PARTICIPANTS_PER_ROOM=200

# Security
LIVEKIT_API_KEY_AUTH=your-secret-api-key-here
LIVEKIT_RATE_LIMIT_MAX=100
LIVEKIT_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

## CLI Commands

### Global Commands

#### `init`
Initialize the LiveKit plugin database schema.

```bash
nself plugin livekit init
```

Creates all required tables, indexes, and constraints for rooms, participants, egress, tokens, quality metrics, and webhook events.

#### `server`
Start the LiveKit plugin HTTP server.

```bash
nself plugin livekit server
nself plugin livekit server --port 3707
```

**Options:**
- `-p, --port <port>` - Server port (default: 3707)

#### `status`
Display current LiveKit plugin status and statistics.

```bash
nself plugin livekit status
```

Shows:
- LiveKit server URL and configuration
- Egress and quality monitoring status
- Total/active rooms, participants, egress jobs
- Token issuance statistics

**Example output:**
```
LiveKit Plugin Status
=====================
LiveKit URL:           wss://livekit.example.com:7880
Egress Enabled:        true
Quality Monitoring:    true
Total Rooms:           42
Active Rooms:          7
Total Participants:    156
Active Participants:   18
Total Egress Jobs:     23
Active Egress Jobs:    2
Tokens Issued:         389
```

#### `health`
Perform health check.

```bash
nself plugin livekit health
```

Verifies database connectivity and returns health status.

### Room Management

#### `rooms:list`
List LiveKit rooms with optional filters.

```bash
nself plugin livekit rooms:list
nself plugin livekit rooms:list --status active
nself plugin livekit rooms:list --type conference --limit 50
```

**Options:**
- `-s, --status <status>` - Filter by status (creating, active, closed)
- `-t, --type <type>` - Filter by room type
- `-l, --limit <limit>` - Result limit (default: 20)

**Example output:**
```
LiveKit Rooms (7):
========================
- daily-standup [active] (conference, max: 50)
  SID: RM_abc123xyz
- webinar-2024 [active] (webinar, max: 200)
  SID: RM_def456uvw
```

#### `rooms:info`
Get detailed information about a specific room.

```bash
nself plugin livekit rooms:info <room-name>
```

**Example:**
```bash
nself plugin livekit rooms:info daily-standup
```

**Output:**
```
Room: daily-standup
============================
ID:               550e8400-e29b-41d4-a716-446655440000
SID:              RM_abc123xyz
Type:             conference
Status:           active
Max Participants: 50
Empty Timeout:    300s
Created:          2024-02-10T10:30:00Z

Participants (3):
  - John Doe [joined]
  - Jane Smith [joined]
  - Bob Wilson [reconnecting]
```

#### `rooms:close`
Close a LiveKit room.

```bash
nself plugin livekit rooms:close <room-name>
```

**Example:**
```bash
nself plugin livekit rooms:close daily-standup
```

Closes the room, disconnects all participants, and updates the status to 'closed'.

#### `rooms:cleanup`
Clean up stale rooms older than specified duration.

```bash
nself plugin livekit rooms:cleanup
nself plugin livekit rooms:cleanup --older-than 2h
```

**Options:**
- `--older-than <duration>` - Clean rooms older than duration (e.g., 1h, 30m, 1d) (default: 1h)

**Example output:**
```
Cleaned up 5 stale rooms
```

### Token Management

#### `token:create`
Generate access token for room access (displays instructions).

```bash
nself plugin livekit token:create <room-name> <identity>
nself plugin livekit token:create daily-standup user@example.com --ttl 7200
```

**Arguments:**
- `<room-name>` - Room name
- `<identity>` - Participant identity (user ID or email)

**Options:**
- `--ttl <seconds>` - Token TTL in seconds (default: 3600)

**Note:** This command displays instructions. Use the REST API for actual token generation with proper JWT signing.

#### `tokens:list`
List active tokens with optional room filter.

```bash
nself plugin livekit tokens:list
nself plugin livekit tokens:list --room daily-standup
```

**Options:**
- `-r, --room <room-name>` - Filter by room name

**Example output:**
```
Active Tokens (12):
===================
- 550e8400-e29b-41d4-a716-446655440001 (room: ..., user: ...)
  Issued: 2024-02-10T10:30:00Z, Expires: 2024-02-10T11:30:00Z
  Uses: 3
```

### Recording & Egress

#### `recordings:list`
List egress jobs (recordings, streams).

```bash
nself plugin livekit recordings:list
nself plugin livekit recordings:list --status active
nself plugin livekit recordings:list --limit 50
```

**Options:**
- `-s, --status <status>` - Filter by status (pending, active, completed, failed)
- `-l, --limit <limit>` - Result limit (default: 20)

**Example output:**
```
Egress Jobs (8):
==================
- EG_abc123xyz [completed] (room/file)
  File: https://cdn.example.com/recordings/room-123.mp4
- EG_def456uvw [active] (room/stream)
  Playlist: https://cdn.example.com/streams/live.m3u8
- EG_ghi789rst [failed] (track/file)
  Error: Track not found
```

## REST API

All endpoints return JSON responses with the following structure:
```json
{
  "success": true,
  "data": { ... }
}
```

Error responses:
```json
{
  "error": "Error message"
}
```

### Authentication

If `LIVEKIT_API_KEY_AUTH` is set, include the API key in the `Authorization` header:

```
Authorization: Bearer your-api-key-here
```

### Health Endpoints

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "livekit",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `GET /ready`
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "livekit",
  "timestamp": "2024-02-10T10:30:00Z"
}
```

#### `GET /live`
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "livekit",
  "version": "1.0.0",
  "uptime": 86400,
  "memory": {
    "rss": 134217728,
    "heapTotal": 67108864,
    "heapUsed": 45088768,
    "external": 2097152
  },
  "stats": {
    "totalRooms": 42,
    "activeRooms": 7,
    "totalParticipants": 156,
    "activeParticipants": 18,
    "totalEgressJobs": 23,
    "activeEgressJobs": 2,
    "totalTokensIssued": 389
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Status Endpoint

#### `GET /v1/status`
Get plugin status and configuration.

**Response:**
```json
{
  "plugin": "livekit",
  "version": "1.0.0",
  "status": "running",
  "livekitUrl": "wss://livekit.example.com:7880",
  "egressEnabled": true,
  "qualityMonitoringEnabled": true,
  "stats": {
    "totalRooms": 42,
    "activeRooms": 7,
    "totalParticipants": 156,
    "activeParticipants": 18,
    "totalEgressJobs": 23,
    "activeEgressJobs": 2,
    "totalTokensIssued": 389
  },
  "timestamp": "2024-02-10T10:30:00Z"
}
```

### Room Management

#### `POST /api/livekit/rooms`
Create a new LiveKit room.

**Request:**
```json
{
  "roomName": "daily-standup",
  "roomType": "conference",
  "maxParticipants": 50,
  "emptyTimeout": 300,
  "callId": "550e8400-e29b-41d4-a716-446655440000",
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "room": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "source_account_id": "primary",
    "livekit_room_name": "daily-standup",
    "room_type": "conference",
    "max_participants": 50,
    "empty_timeout": 300,
    "status": "active",
    "created_at": "2024-02-10T10:30:00Z",
    "activated_at": "2024-02-10T10:30:01Z"
  }
}
```

#### `GET /api/livekit/rooms/:roomId`
Get room details by ID.

**Response:**
```json
{
  "success": true,
  "room": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "livekit_room_name": "daily-standup",
    "livekit_room_sid": "RM_abc123xyz",
    "room_type": "conference",
    "status": "active",
    "max_participants": 50
  }
}
```

#### `GET /api/livekit/rooms`
List rooms with optional filters.

**Query Parameters:**
- `status` - Filter by status
- `roomType` - Filter by room type
- `limit` - Result limit (default: 50)
- `offset` - Result offset (default: 0)

**Response:**
```json
{
  "success": true,
  "rooms": [...],
  "count": 7
}
```

#### `DELETE /api/livekit/rooms/:roomId`
Close a room.

**Response:**
```json
{
  "success": true,
  "room": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "status": "closed",
    "closed_at": "2024-02-10T11:30:00Z"
  }
}
```

### Token Management

#### `POST /api/livekit/tokens`
Generate access token for a room.

**Request:**
```json
{
  "roomName": "daily-standup",
  "participantIdentity": "user@example.com",
  "participantName": "John Doe",
  "ttl": 3600,
  "grants": {
    "canPublish": true,
    "canSubscribe": true,
    "canPublishData": true
  }
}
```

**Response:**
```json
{
  "success": true,
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "tokenId": "550e8400-e29b-41d4-a716-446655440002",
  "livekitUrl": "wss://livekit.example.com:7880",
  "expiresAt": "2024-02-10T11:30:00Z"
}
```

#### `POST /api/livekit/tokens/:tokenId/revoke`
Revoke an access token.

**Request:**
```json
{
  "revokedBy": "admin-user-id",
  "reason": "Security policy violation"
}
```

**Response:**
```json
{
  "success": true,
  "token": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "revoked_at": "2024-02-10T11:00:00Z",
    "revoked_by": "admin-user-id"
  }
}
```

#### `GET /api/livekit/tokens`
List active tokens.

**Query Parameters:**
- `roomId` - Filter by room ID
- `limit` - Result limit (default: 50)
- `offset` - Result offset (default: 0)

**Response:**
```json
{
  "success": true,
  "tokens": [...],
  "count": 12
}
```

### Recording & Egress

#### `POST /api/livekit/egress/room-composite`
Start room composite recording.

**Request:**
```json
{
  "roomName": "daily-standup",
  "layout": "grid",
  "audioOnly": false,
  "videoOptions": {
    "width": 1920,
    "height": 1080,
    "framerate": 30
  },
  "fileOutput": {
    "filepath": "/recordings/room-{room_name}-{time}.mp4"
  }
}
```

**Response:**
```json
{
  "success": true,
  "egressId": "EG_abc123xyz",
  "jobId": "550e8400-e29b-41d4-a716-446655440003",
  "status": "active"
}
```

#### `POST /api/livekit/egress/track`
Start track-based recording.

**Request:**
```json
{
  "roomName": "daily-standup",
  "trackSid": "TR_abc123xyz",
  "fileOutput": {
    "filepath": "/recordings/track-{track_id}-{time}.webm"
  }
}
```

**Response:**
```json
{
  "success": true,
  "egressId": "EG_def456uvw",
  "jobId": "550e8400-e29b-41d4-a716-446655440004",
  "status": "active"
}
```

#### `POST /api/livekit/egress/stream`
Start stream egress (RTMP/HLS).

**Request:**
```json
{
  "roomName": "daily-standup",
  "urls": ["rtmp://live.example.com/stream/key"],
  "streamProtocol": "rtmp"
}
```

**Response:**
```json
{
  "success": true,
  "egressId": "EG_ghi789rst",
  "jobId": "550e8400-e29b-41d4-a716-446655440005",
  "status": "active"
}
```

#### `GET /api/livekit/egress/:egressId`
Get egress job details.

**Response:**
```json
{
  "success": true,
  "job": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "livekit_egress_id": "EG_abc123xyz",
    "egress_type": "room",
    "output_type": "file",
    "status": "completed",
    "np_fileproc_url": "https://cdn.example.com/recordings/room-123.mp4",
    "np_fileproc_size_bytes": 524288000,
    "duration_seconds": 3600
  }
}
```

#### `GET /api/livekit/egress`
List egress jobs.

**Query Parameters:**
- `roomId` - Filter by room ID
- `status` - Filter by status
- `egressType` - Filter by egress type
- `limit` - Result limit (default: 50)
- `offset` - Result offset (default: 0)

**Response:**
```json
{
  "success": true,
  "jobs": [...],
  "count": 8
}
```

#### `DELETE /api/livekit/egress/:egressId`
Stop egress job.

**Response:**
```json
{
  "success": true,
  "job": {
    "id": "550e8400-e29b-41d4-a716-446655440003",
    "status": "ending",
    "ended_at": "2024-02-10T12:00:00Z"
  }
}
```

### Participant Management

#### `POST /api/livekit/rooms/:roomId/participants`
Add participant to room.

**Request:**
```json
{
  "userId": "550e8400-e29b-41d4-a716-446655440006",
  "livekitIdentity": "user@example.com",
  "displayName": "John Doe",
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "participant": {
    "id": "550e8400-e29b-41d4-a716-446655440007",
    "room_id": "550e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440006",
    "livekit_identity": "user@example.com",
    "display_name": "John Doe",
    "status": "joining"
  }
}
```

#### `GET /api/livekit/rooms/:roomId/participants`
List room participants.

**Query Parameters:**
- `status` - Filter by status (joining, joined, reconnecting, disconnected)

**Response:**
```json
{
  "success": true,
  "participants": [...],
  "count": 3
}
```

#### `DELETE /api/livekit/rooms/:roomId/participants/:participantId`
Remove participant from room.

**Response:**
```json
{
  "success": true,
  "removed": true
}
```

#### `POST /api/livekit/rooms/:roomId/participants/:participantId/mute`
Mute participant track.

**Request:**
```json
{
  "trackType": "microphone"
}
```

Valid track types: `microphone`, `camera`, `screen_share`

**Response:**
```json
{
  "success": true,
  "participant": {
    "id": "550e8400-e29b-41d4-a716-446655440007",
    "microphone_enabled": false
  }
}
```

### Quality Monitoring

#### `GET /api/livekit/rooms/:roomId/quality`
Get room quality metrics.

**Response:**
```json
{
  "success": true,
  "room": {
    "avgBitrate": 2500,
    "avgLatency": 45,
    "avgPacketLoss": 0.5,
    "participantCount": 3
  },
  "participants": [
    {
      "userId": "550e8400-e29b-41d4-a716-446655440006",
      "displayName": "John Doe",
      "bitrate": 2800,
      "latency": 40,
      "packetLoss": 0.3,
      "connectionType": "wifi"
    }
  ]
}
```

#### `POST /api/livekit/quality-metrics`
Record quality metric sample.

**Request:**
```json
{
  "roomId": "550e8400-e29b-41d4-a716-446655440001",
  "participantId": "550e8400-e29b-41d4-a716-446655440007",
  "metricType": "video",
  "bitrateKbps": 2800,
  "latencyMs": 40,
  "jitterMs": 5,
  "packetLossPct": 0.3,
  "resolution": "1920x1080",
  "fps": 30,
  "connectionType": "wifi",
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "metric": {
    "id": "550e8400-e29b-41d4-a716-446655440008",
    "recorded_at": "2024-02-10T10:35:00Z"
  }
}
```

### Webhook Endpoint

#### `POST /webhook`
Receive LiveKit webhook events.

**Request:**
```json
{
  "type": "participant.joined",
  "room": {
    "sid": "RM_abc123xyz",
    "name": "daily-standup"
  },
  "participant": {
    "sid": "PA_def456uvw",
    "identity": "user@example.com",
    "name": "John Doe"
  }
}
```

**Response:**
```json
{
  "received": true,
  "type": "participant.joined"
}
```

## Webhook Events

The LiveKit plugin processes the following webhook events from LiveKit Server:

### Room Events

#### `room.created`
Triggered when a LiveKit room is created.

**Payload:**
```json
{
  "type": "room.created",
  "room": {
    "sid": "RM_abc123xyz",
    "name": "daily-standup",
    "emptyTimeout": 300,
    "maxParticipants": 100,
    "creationTime": 1234567890
  }
}
```

#### `room.closed`
Triggered when a LiveKit room is closed.

**Payload:**
```json
{
  "type": "room.closed",
  "room": {
    "sid": "RM_abc123xyz",
    "name": "daily-standup"
  }
}
```

### Participant Events

#### `participant.joined`
Triggered when a participant joins a room.

**Payload:**
```json
{
  "type": "participant.joined",
  "room": {
    "sid": "RM_abc123xyz",
    "name": "daily-standup"
  },
  "participant": {
    "sid": "PA_def456uvw",
    "identity": "user@example.com",
    "name": "John Doe",
    "joinedAt": 1234567890
  }
}
```

#### `participant.left`
Triggered when a participant leaves a room.

**Payload:**
```json
{
  "type": "participant.left",
  "room": {
    "sid": "RM_abc123xyz"
  },
  "participant": {
    "sid": "PA_def456uvw",
    "identity": "user@example.com"
  }
}
```

### Egress Events

#### `egress.started`
Triggered when an egress job starts.

**Payload:**
```json
{
  "type": "egress.started",
  "egressId": "EG_abc123xyz",
  "roomName": "daily-standup",
  "startedAt": 1234567890
}
```

#### `egress.completed`
Triggered when an egress job completes successfully.

**Payload:**
```json
{
  "type": "egress.completed",
  "egressId": "EG_abc123xyz",
  "roomName": "daily-standup",
  "fileUrl": "https://cdn.example.com/recordings/room-123.mp4",
  "fileSizeBytes": 524288000,
  "duration": 3600
}
```

#### `egress.failed`
Triggered when an egress job fails.

**Payload:**
```json
{
  "type": "egress.failed",
  "egressId": "EG_abc123xyz",
  "roomName": "daily-standup",
  "error": "Track not found"
}
```

## Database Schema

### np_livekit_rooms

Stores LiveKit room configurations and states.

```sql
CREATE TABLE np_livekit_rooms (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  livekit_room_name VARCHAR(255) NOT NULL,
  livekit_room_sid VARCHAR(255),
  room_type VARCHAR(50) NOT NULL,
  max_participants INTEGER DEFAULT 100,
  empty_timeout INTEGER DEFAULT 300,
  call_id UUID,
  stream_id UUID,
  status VARCHAR(50) NOT NULL DEFAULT 'creating',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  activated_at TIMESTAMPTZ,
  closed_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, livekit_room_name)
);

CREATE INDEX idx_livekit_rooms_account ON np_livekit_rooms(source_account_id);
CREATE INDEX idx_livekit_rooms_name ON np_livekit_rooms(livekit_room_name);
CREATE INDEX idx_livekit_rooms_sid ON np_livekit_rooms(livekit_room_sid);
CREATE INDEX idx_livekit_rooms_status ON np_livekit_rooms(status);
CREATE INDEX idx_livekit_rooms_call ON np_livekit_rooms(call_id) WHERE call_id IS NOT NULL;
CREATE INDEX idx_livekit_rooms_stream ON np_livekit_rooms(stream_id) WHERE stream_id IS NOT NULL;
```

**Status values:** `creating`, `active`, `closed`

**Room types:** `conference`, `webinar`, `broadcast`, `audio`, `custom`

### np_livekit_participants

Tracks participants in LiveKit rooms.

```sql
CREATE TABLE np_livekit_participants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  room_id UUID NOT NULL REFERENCES np_livekit_rooms(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  livekit_participant_sid VARCHAR(255),
  livekit_identity VARCHAR(255) NOT NULL,
  display_name VARCHAR(255),
  metadata JSONB DEFAULT '{}'::jsonb,
  status VARCHAR(50) NOT NULL DEFAULT 'joining',
  camera_enabled BOOLEAN DEFAULT false,
  microphone_enabled BOOLEAN DEFAULT false,
  screen_share_enabled BOOLEAN DEFAULT false,
  last_bitrate_kbps INTEGER,
  last_latency_ms INTEGER,
  last_packet_loss_pct DECIMAL(5,2),
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  left_at TIMESTAMPTZ,
  total_duration_seconds INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, room_id, user_id)
);

CREATE INDEX idx_livekit_participants_account ON np_livekit_participants(source_account_id);
CREATE INDEX idx_livekit_participants_room ON np_livekit_participants(room_id);
CREATE INDEX idx_livekit_participants_user ON np_livekit_participants(user_id);
CREATE INDEX idx_livekit_participants_sid ON np_livekit_participants(livekit_participant_sid);
CREATE INDEX idx_livekit_participants_status ON np_livekit_participants(status);
```

**Status values:** `joining`, `joined`, `reconnecting`, `disconnected`

### np_livekit_egress_jobs

Manages recording and egress jobs.

```sql
CREATE TABLE np_livekit_egress_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  room_id UUID NOT NULL REFERENCES np_livekit_rooms(id) ON DELETE CASCADE,
  recording_id UUID,
  livekit_egress_id VARCHAR(255) NOT NULL,
  egress_type VARCHAR(50) NOT NULL,
  output_type VARCHAR(50) NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  status VARCHAR(50) NOT NULL DEFAULT 'pending',
  np_fileproc_url TEXT,
  np_fileproc_size_bytes BIGINT,
  duration_seconds INTEGER,
  playlist_url TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ended_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, livekit_egress_id)
);

CREATE INDEX idx_livekit_egress_account ON np_livekit_egress_jobs(source_account_id);
CREATE INDEX idx_livekit_egress_room ON np_livekit_egress_jobs(room_id);
CREATE INDEX idx_livekit_egress_recording ON np_livekit_egress_jobs(recording_id);
CREATE INDEX idx_livekit_egress_id ON np_livekit_egress_jobs(livekit_egress_id);
CREATE INDEX idx_livekit_egress_status ON np_livekit_egress_jobs(status);
CREATE INDEX idx_livekit_egress_type ON np_livekit_egress_jobs(egress_type, output_type);
```

**Status values:** `pending`, `active`, `ending`, `completed`, `failed`

**Egress types:** `room`, `track`, `stream`, `web`

**Output types:** `file`, `stream`, `segments`

### np_livekit_tokens

Tracks issued access tokens.

```sql
CREATE TABLE np_livekit_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  room_id UUID NOT NULL REFERENCES np_livekit_rooms(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  token_hash VARCHAR(255) NOT NULL,
  grants JSONB NOT NULL DEFAULT '{}'::jsonb,
  issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  revoked_by UUID,
  revoke_reason TEXT,
  first_used_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  use_count INTEGER DEFAULT 0,
  metadata JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_livekit_tokens_account ON np_livekit_tokens(source_account_id);
CREATE INDEX idx_livekit_tokens_room ON np_livekit_tokens(room_id);
CREATE INDEX idx_livekit_tokens_user ON np_livekit_tokens(user_id);
CREATE INDEX idx_livekit_tokens_expires ON np_livekit_tokens(expires_at);
CREATE INDEX idx_livekit_tokens_revoked ON np_livekit_tokens(revoked_at) WHERE revoked_at IS NOT NULL;
```

### np_livekit_quality_metrics

Stores quality metrics samples for monitoring.

```sql
CREATE TABLE np_livekit_quality_metrics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  room_id UUID NOT NULL REFERENCES np_livekit_rooms(id) ON DELETE CASCADE,
  participant_id UUID REFERENCES np_livekit_participants(id) ON DELETE CASCADE,
  metric_type VARCHAR(50) NOT NULL,
  bitrate_kbps INTEGER,
  latency_ms INTEGER,
  jitter_ms INTEGER,
  packet_loss_pct DECIMAL(5,2),
  resolution VARCHAR(20),
  fps INTEGER,
  audio_level INTEGER,
  connection_type VARCHAR(50),
  turn_server VARCHAR(255),
  metadata JSONB DEFAULT '{}'::jsonb,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_livekit_quality_account ON np_livekit_quality_metrics(source_account_id);
CREATE INDEX idx_livekit_quality_room ON np_livekit_quality_metrics(room_id);
CREATE INDEX idx_livekit_quality_participant ON np_livekit_quality_metrics(participant_id);
CREATE INDEX idx_livekit_quality_type ON np_livekit_quality_metrics(metric_type);
CREATE INDEX idx_livekit_quality_time ON np_livekit_quality_metrics(recorded_at DESC);
```

**Metric types:** `video`, `audio`, `connection`, `datachannel`

**Connection types:** `wifi`, `ethernet`, `cellular`, `unknown`

### np_livekit_webhook_events

Stores webhook events from LiveKit Server.

```sql
CREATE TABLE np_livekit_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_livekit_webhook_events_account ON np_livekit_webhook_events(source_account_id);
CREATE INDEX idx_livekit_webhook_events_type ON np_livekit_webhook_events(event_type);
CREATE INDEX idx_livekit_webhook_events_processed ON np_livekit_webhook_events(processed);
CREATE INDEX idx_livekit_webhook_events_created ON np_livekit_webhook_events(created_at);
```

## Examples

### Example 1: Create Room and Generate Token

```bash
# Create a new conference room
curl -X POST http://localhost:3707/api/livekit/rooms \
  -H "Content-Type: application/json" \
  -d '{
    "roomName": "team-meeting",
    "roomType": "conference",
    "maxParticipants": 50
  }'

# Generate access token for a participant
curl -X POST http://localhost:3707/api/livekit/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "roomName": "team-meeting",
    "participantIdentity": "john@example.com",
    "participantName": "John Doe",
    "ttl": 3600,
    "grants": {
      "canPublish": true,
      "canSubscribe": true,
      "canPublishData": true
    }
  }'
```

### Example 2: Start Room Recording

```bash
# Start recording a room with grid layout
curl -X POST http://localhost:3707/api/livekit/egress/room-composite \
  -H "Content-Type: application/json" \
  -d '{
    "roomName": "team-meeting",
    "layout": "grid",
    "videoOptions": {
      "width": 1920,
      "height": 1080,
      "framerate": 30
    },
    "fileOutput": {
      "filepath": "/recordings/team-meeting-{time}.mp4"
    }
  }'

# Check recording status
curl http://localhost:3707/api/livekit/egress/EG_abc123xyz
```

### Example 3: Monitor Room Quality

```bash
# Get quality metrics for a room
curl http://localhost:3707/api/livekit/rooms/550e8400-e29b-41d4-a716-446655440001/quality

# Response shows avg quality and per-participant metrics
{
  "success": true,
  "room": {
    "avgBitrate": 2500,
    "avgLatency": 45,
    "avgPacketLoss": 0.5,
    "participantCount": 3
  },
  "participants": [
    {
      "userId": "550e8400-e29b-41d4-a716-446655440006",
      "displayName": "John Doe",
      "bitrate": 2800,
      "latency": 40,
      "packetLoss": 0.3,
      "connectionType": "wifi"
    }
  ]
}
```

### Example 4: Live Stream to RTMP

```bash
# Stream a room to RTMP destination
curl -X POST http://localhost:3707/api/livekit/egress/stream \
  -H "Content-Type: application/json" \
  -d '{
    "roomName": "webinar-2024",
    "urls": ["rtmp://live.youtube.com/stream/your-stream-key"],
    "streamProtocol": "rtmp"
  }'

# Stop the stream
curl -X DELETE http://localhost:3707/api/livekit/egress/EG_stream123
```

### Example 5: Manage Participants

```bash
# List participants in a room
curl http://localhost:3707/api/livekit/rooms/550e8400-e29b-41d4-a716-446655440001/participants

# Mute a participant's microphone
curl -X POST http://localhost:3707/api/livekit/rooms/550e8400-e29b-41d4-a716-446655440001/participants/550e8400-e29b-41d4-a716-446655440007/mute \
  -H "Content-Type: application/json" \
  -d '{
    "trackType": "microphone"
  }'

# Remove a participant
curl -X DELETE http://localhost:3707/api/livekit/rooms/550e8400-e29b-41d4-a716-446655440001/participants/550e8400-e29b-41d4-a716-446655440007
```

## Troubleshooting

### Connection Issues

**Problem:** Participants cannot connect to rooms

**Solutions:**
1. Verify LiveKit server is running:
   ```bash
   curl http://your-livekit-server:7880/health
   ```

2. Check TURN server configuration:
   ```bash
   # Verify TURN port is accessible
   telnet your-livekit-server 3478
   ```

3. Ensure tokens are properly signed:
   ```bash
   # Check token hasn't expired
   nself plugin livekit tokens:list --room your-room
   ```

4. Verify firewall rules allow WebRTC ports (UDP 50000-60000)

### Recording Failures

**Problem:** Egress jobs fail to start or complete

**Solutions:**
1. Check egress is enabled:
   ```bash
   echo $LIVEKIT_EGRESS_ENABLED  # Should be 'true'
   ```

2. Verify output path permissions:
   ```bash
   ls -la /var/livekit/recordings
   # Should be writable by LiveKit process
   ```

3. Check S3 credentials if using cloud storage:
   ```bash
   echo $LIVEKIT_RECORDINGS_S3_BUCKET
   aws s3 ls s3://your-bucket/
   ```

4. Review egress logs:
   ```bash
   nself plugin livekit recordings:list --status failed
   ```

### Quality Issues

**Problem:** Poor video/audio quality or connection problems

**Solutions:**
1. Check quality metrics:
   ```bash
   curl http://localhost:3707/api/livekit/rooms/YOUR_ROOM_ID/quality
   ```

2. Look for high packet loss (>5%) or latency (>150ms):
   ```sql
   SELECT AVG(packet_loss_pct), AVG(latency_ms)
   FROM np_livekit_quality_metrics
   WHERE room_id = 'YOUR_ROOM_ID'
   AND recorded_at > NOW() - INTERVAL '5 minutes';
   ```

3. Verify network bandwidth:
   - Minimum: 1 Mbps per participant
   - Recommended: 3-5 Mbps per participant

4. Check TURN server is being used for NAT traversal:
   ```sql
   SELECT DISTINCT turn_server
   FROM np_livekit_quality_metrics
   WHERE room_id = 'YOUR_ROOM_ID';
   ```

### Token Errors

**Problem:** "Invalid token" or "Token expired" errors

**Solutions:**
1. Verify API credentials:
   ```bash
   echo $LIVEKIT_API_KEY
   echo $LIVEKIT_API_SECRET
   ```

2. Check token TTL configuration:
   ```bash
   echo $LIVEKIT_TOKEN_DEFAULT_TTL
   echo $LIVEKIT_TOKEN_MAX_TTL
   ```

3. Ensure server time is synchronized (NTP):
   ```bash
   timedatectl status
   ```

4. Revoke and regenerate tokens:
   ```bash
   curl -X POST http://localhost:3707/api/livekit/tokens/TOKEN_ID/revoke \
     -d '{"revokedBy":"admin","reason":"Testing"}'
   ```

### Database Connection

**Problem:** "Database unavailable" errors

**Solutions:**
1. Check database connectivity:
   ```bash
   psql $DATABASE_URL -c "SELECT 1"
   ```

2. Verify database credentials:
   ```bash
   echo $DATABASE_URL
   # Should be: postgresql://user:pass@host:5432/dbname
   ```

3. Check PostgreSQL is running:
   ```bash
   pg_isready -h localhost -p 5432
   ```

4. Review connection pool settings:
   ```bash
   # Check for connection exhaustion
   psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity"
   ```

### Performance Issues

**Problem:** Slow API responses or high latency

**Solutions:**
1. Check server resources:
   ```bash
   curl http://localhost:3707/live
   # Review memory usage
   ```

2. Monitor room count:
   ```bash
   nself plugin livekit status
   # Compare to MAX_CONCURRENT_ROOMS
   ```

3. Review database query performance:
   ```sql
   SELECT query, mean_exec_time, calls
   FROM pg_stat_statements
   WHERE query LIKE '%np_livekit%'
   ORDER BY mean_exec_time DESC
   LIMIT 10;
   ```

4. Consider scaling horizontally with multiple LiveKit servers

### Cleanup Operations

**Problem:** Stale rooms or orphaned data

**Solutions:**
1. Clean up old rooms:
   ```bash
   nself plugin livekit rooms:cleanup --older-than 24h
   ```

2. Remove old quality metrics:
   ```sql
   DELETE FROM np_livekit_quality_metrics
   WHERE recorded_at < NOW() - INTERVAL '7 days';
   ```

3. Clean up expired tokens:
   ```sql
   DELETE FROM np_livekit_tokens
   WHERE expires_at < NOW() - INTERVAL '7 days';
   ```

4. Archive completed egress jobs:
   ```sql
   UPDATE np_livekit_egress_jobs
   SET metadata = metadata || '{"archived": true}'::jsonb
   WHERE status = 'completed'
   AND ended_at < NOW() - INTERVAL '30 days';
   ```

---

**Version:** 1.0.0
**Last Updated:** February 2024
**Support:** https://github.com/acamarata/nself-plugins/issues
