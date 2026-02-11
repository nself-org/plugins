# Stream Gateway Plugin

Stream admission and governance service with concurrency limits, viewer analytics, and intelligent access control.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Stream Gateway plugin provides intelligent stream admission control and viewer session management. It supports:

- **4 Database Tables** - Sessions, streams, admission rules, viewer analytics
- **4 Analytics Views** - Concurrent viewers, denial rates, durations, device breakdown
- **Admission Rules** - Configurable access control rules
- **Concurrency Limits** - Global and per-user stream limits
- **Session Management** - Active session tracking with heartbeats
- **Viewer Analytics** - Comprehensive viewing statistics
- **Device Tracking** - Multi-device concurrency limits
- **Real-time Monitoring** - Live viewer counts and analytics
- **Full REST API** - Complete gateway operations
- **CLI Interface** - Command-line stream and session management

### Key Features

| Feature | Description |
|---------|-------------|
| Admission Control | Rule-based access control for streams |
| Concurrency Limits | Max concurrent viewers per stream |
| Device Limits | Max concurrent streams per device |
| Session Heartbeats | Detect and evict stale sessions |
| Viewer Analytics | Detailed viewing behavior tracking |
| Quality Selection | Track viewer quality preferences |
| Denial Tracking | Monitor and analyze access denials |
| Real-time Stats | Live concurrent viewer counts |

---

## Quick Start

```bash
# Install the plugin
nself plugin install stream-gateway

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "SG_DEFAULT_MAX_CONCURRENT=100" >> .env
echo "SG_DEFAULT_MAX_DEVICE_STREAMS=2" >> .env

# Initialize database schema
nself plugin stream-gateway init

# Start server
nself plugin stream-gateway server --port 3601
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PORT` | No | `3601` | HTTP server port |
| `REDIS_URL` | No | - | Redis URL for real-time state (optional) |
| `SG_HEARTBEAT_INTERVAL` | No | `30` | Heartbeat interval (seconds) |
| `SG_HEARTBEAT_TIMEOUT` | No | `90` | Heartbeat timeout (seconds) |
| `SG_DEFAULT_MAX_CONCURRENT` | No | `100` | Default max concurrent viewers per stream |
| `SG_DEFAULT_MAX_DEVICE_STREAMS` | No | `2` | Default max concurrent streams per device |
| `SG_SESSION_MAX_DURATION_HOURS` | No | `12` | Max session duration (hours) |
| `SG_ANALYTICS_INTERVAL` | No | `60` | Analytics snapshot interval (seconds) |
| `SG_REALTIME_URL` | No | - | Realtime plugin URL for notifications |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### App-Specific Configuration

Per-app configuration overrides (e.g., for "tv" app):

| Variable | Description |
|----------|-------------|
| `SG_APP_TV_MAX_CONCURRENT` | Max concurrent viewers for "tv" app streams |
| `SG_APP_TV_MAX_DEVICE_STREAMS` | Max device streams for "tv" app |
| `SG_APP_FAMILY_MAX_CONCURRENT` | Max concurrent for "family" app |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Redis (optional)
REDIS_URL=redis://localhost:6379

# Gateway Settings
SG_DEFAULT_MAX_CONCURRENT=100
SG_DEFAULT_MAX_DEVICE_STREAMS=2
SG_SESSION_MAX_DURATION_HOURS=12

# Heartbeat
SG_HEARTBEAT_INTERVAL=30
SG_HEARTBEAT_TIMEOUT=90

# Analytics
SG_ANALYTICS_INTERVAL=60
SG_REALTIME_URL=http://localhost:3801

# App-Specific
SG_APP_TV_MAX_CONCURRENT=50
SG_APP_TV_MAX_DEVICE_STREAMS=3

# Server
PORT=3601
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin stream-gateway init

# Start server
nself plugin stream-gateway server

# Custom port
nself plugin stream-gateway server --port 8080

# Check status
nself plugin stream-gateway status

# View statistics
nself plugin stream-gateway stats
```

### Stream Management

```bash
# List all streams
nself plugin stream-gateway streams list

# List active streams with viewer counts
nself plugin stream-gateway streams active

# Get stream details
nself plugin stream-gateway streams get <stream-id>
```

### Session Management

```bash
# List all sessions
nself plugin stream-gateway sessions list

# List active sessions
nself plugin stream-gateway sessions active

# Filter by user
nself plugin stream-gateway sessions list --user user123

# Filter by stream
nself plugin stream-gateway sessions list --stream stream-abc

# Get session details
nself plugin stream-gateway sessions get <session-id>
```

### Admission Control

```bash
# Manually admit user to stream
nself plugin stream-gateway admit \
  --stream stream-abc \
  --user user123 \
  --device device-xyz

# Evict session
nself plugin stream-gateway evict <session-id>
```

### Admission Rules

```bash
# List admission rules
nself plugin stream-gateway rules list

# Create admission rule
nself plugin stream-gateway rules create \
  --name "Premium Only" \
  --type subscription \
  --action deny \
  --condition "plan != 'premium'"

# Update rule priority
nself plugin stream-gateway rules update <rule-id> --priority 10

# Disable rule
nself plugin stream-gateway rules update <rule-id> --active false

# Delete rule
nself plugin stream-gateway rules delete <rule-id>
```

### Analytics

```bash
# Show overall analytics summary
nself plugin stream-gateway analytics summary

# Analytics for specific stream
nself plugin stream-gateway analytics summary --stream stream-abc

# Analytics for date range
nself plugin stream-gateway analytics summary \
  --start 2026-02-01 \
  --end 2026-02-11
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
  "plugin": "stream-gateway",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "stream-gateway",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/status
Gateway status with statistics.

**Response:**
```json
{
  "plugin": "stream-gateway",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "activeStreams": 5,
    "activeSessions": 125,
    "totalViewers": 125,
    "admissionRules": 3,
    "denialRate": 2.5
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Streams

#### POST /v1/streams
Create/register stream.

**Request Body:**
```json
{
  "stream_id": "stream-abc",
  "title": "Live Event Stream",
  "stream_type": "live",
  "source_device_id": "device-123",
  "max_viewers": 100,
  "ingest_url": "rtmp://ingest.example.com/live/stream-abc",
  "playback_url": "https://cdn.example.com/stream-abc/master.m3u8",
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "app_id": "default",
    "stream_id": "stream-abc",
    "title": "Live Event Stream",
    "stream_type": "live",
    "status": "inactive",
    "max_viewers": 100,
    "current_viewers": 0,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/streams
List streams.

**Query Parameters:**
- `app_id` (optional) - Filter by app
- `status` (optional) - Filter by status (active, inactive)
- `stream_type` (optional) - Filter by type (live, vod)
- `limit` (optional) - Max results (default: 50)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "stream_id": "stream-abc",
      "title": "Live Event Stream",
      "status": "active",
      "current_viewers": 45,
      "max_viewers": 100,
      "started_at": "2026-02-11T10:00:00.000Z"
    }
  ]
}
```

#### GET /v1/streams/:streamId
Get stream details with viewer count.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "app_id": "default",
    "stream_id": "stream-abc",
    "title": "Live Event Stream",
    "stream_type": "live",
    "status": "active",
    "source_device_id": "device-123",
    "ingest_url": "rtmp://ingest.example.com/live/stream-abc",
    "playback_url": "https://cdn.example.com/stream-abc/master.m3u8",
    "max_viewers": 100,
    "current_viewers": 45,
    "total_viewers": 250,
    "peak_viewers": 87,
    "started_at": "2026-02-11T10:00:00.000Z",
    "metadata": {},
    "created_at": "2026-02-11T09:00:00.000Z",
    "updated_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### PUT /v1/streams/:streamId
Update stream.

**Request Body:**
```json
{
  "status": "active",
  "max_viewers": 150
}
```

#### DELETE /v1/streams/:streamId
Delete stream.

**Response:**
```json
{
  "success": true
}
```

### Admission

#### POST /v1/admit
Admit user to stream.

**Request Body:**
```json
{
  "stream_id": "stream-abc",
  "user_id": "user123",
  "device_id": "device-xyz",
  "device_type": "web",
  "quality": "auto",
  "metadata": {
    "ip_address": "192.168.1.1",
    "user_agent": "Mozilla/5.0..."
  }
}
```

**Response (Success):**
```json
{
  "success": true,
  "data": {
    "admitted": true,
    "session_id": "uuid",
    "stream_id": "stream-abc",
    "playback_url": "https://cdn.example.com/stream-abc/master.m3u8",
    "heartbeat_interval": 30
  }
}
```

**Response (Denied):**
```json
{
  "success": false,
  "data": {
    "admitted": false,
    "reason": "Stream at capacity",
    "retry_after": 30
  }
}
```

#### POST /v1/sessions/:sessionId/heartbeat
Send session heartbeat.

**Response:**
```json
{
  "success": true,
  "data": {
    "session_id": "uuid",
    "status": "active",
    "next_heartbeat_in": 30
  }
}
```

#### POST /v1/sessions/:sessionId/end
End session.

**Response:**
```json
{
  "success": true,
  "data": {
    "session_id": "uuid",
    "status": "ended",
    "duration_seconds": 3600
  }
}
```

#### POST /v1/evict/:sessionId
Evict session (admin).

**Response:**
```json
{
  "success": true
}
```

### Sessions

#### GET /v1/sessions
List sessions.

**Query Parameters:**
- `app_id` (optional) - Filter by app
- `stream_id` (optional) - Filter by stream
- `user_id` (optional) - Filter by user
- `device_id` (optional) - Filter by device
- `status` (optional) - Filter by status (active, ended, evicted)
- `limit` (optional) - Max results (default: 100)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "stream_id": "stream-abc",
      "user_id": "user123",
      "device_id": "device-xyz",
      "status": "active",
      "quality": "720p",
      "started_at": "2026-02-11T10:00:00.000Z",
      "last_heartbeat_at": "2026-02-11T10:30:00.000Z",
      "duration_seconds": 1800
    }
  ]
}
```

#### GET /v1/sessions/:id
Get session details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "app_id": "default",
    "stream_id": "stream-abc",
    "stream_type": "live",
    "user_id": "user123",
    "device_id": "device-xyz",
    "device_type": "web",
    "status": "active",
    "quality": "720p",
    "started_at": "2026-02-11T10:00:00.000Z",
    "last_heartbeat_at": "2026-02-11T10:30:00.000Z",
    "ended_at": null,
    "duration_seconds": 1800,
    "bytes_transferred": 524288000,
    "metadata": {},
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

### Admission Rules

#### POST /v1/rules
Create admission rule.

**Request Body:**
```json
{
  "name": "Premium Only",
  "rule_type": "subscription",
  "conditions": {
    "subscription_plan": ["premium", "pro"]
  },
  "action": "deny",
  "priority": 10,
  "active": true,
  "metadata": {}
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "name": "Premium Only",
    "rule_type": "subscription",
    "action": "deny",
    "priority": 10,
    "active": true,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/rules
List admission rules.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "name": "Premium Only",
      "rule_type": "subscription",
      "action": "deny",
      "priority": 10,
      "active": true
    }
  ]
}
```

#### GET /v1/rules/:id
Get rule details.

#### PUT /v1/rules/:id
Update rule.

**Request Body:**
```json
{
  "priority": 5,
  "active": false
}
```

#### DELETE /v1/rules/:id
Delete rule.

**Response:**
```json
{
  "success": true
}
```

### Analytics

#### GET /v1/analytics/summary
Get analytics summary.

**Query Parameters:**
- `stream_id` (optional) - Filter by stream
- `start_date` (optional) - Start date (ISO 8601)
- `end_date` (optional) - End date (ISO 8601)
- `period` (optional) - Grouping period (hour, day, week, month)

**Response:**
```json
{
  "success": true,
  "data": {
    "totalSessions": 1500,
    "uniqueViewers": 450,
    "averageDurationSeconds": 2400,
    "totalBytesTransferred": 107374182400,
    "peakConcurrentViewers": 87,
    "averageConcurrentViewers": 45,
    "deniedAttempts": 38,
    "denialRate": 2.5,
    "byQuality": [
      { "quality": "1080p", "count": 600, "percentage": 40 },
      { "quality": "720p", "count": 750, "percentage": 50 },
      { "quality": "480p", "count": 150, "percentage": 10 }
    ],
    "byDevice": [
      { "device_type": "web", "count": 900, "percentage": 60 },
      { "device_type": "mobile", "count": 450, "percentage": 30 },
      { "device_type": "tv", "count": 150, "percentage": 10 }
    ]
  }
}
```

#### GET /v1/analytics/concurrent
Get concurrent viewers over time.

**Query Parameters:**
- `stream_id` (optional) - Filter by stream
- `start_date` (optional) - Start date
- `end_date` (optional) - End date
- `interval` (optional) - Data point interval (minute, hour, day)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "timestamp": "2026-02-11T10:00:00.000Z",
      "concurrent_viewers": 45
    },
    {
      "timestamp": "2026-02-11T11:00:00.000Z",
      "concurrent_viewers": 67
    }
  ]
}
```

### Statistics

#### GET /v1/stats
Get gateway statistics.

**Response:**
```json
{
  "success": true,
  "data": {
    "activeStreams": 5,
    "activeSessions": 125,
    "totalViewers": 125,
    "totalStreams": 50,
    "totalSessions": 5000,
    "admissionRules": 3,
    "averageDuration": 2400,
    "denialRate": 2.5,
    "lastSessionAt": "2026-02-11T10:00:00.000Z"
  }
}
```

---

## Database Schema

### sg_stream_sessions

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `stream_id` | VARCHAR(255) | Stream identifier |
| `stream_type` | VARCHAR(32) | Stream type (live, vod) |
| `user_id` | VARCHAR(255) | User ID |
| `device_id` | VARCHAR(255) | Device ID |
| `device_type` | VARCHAR(32) | Device type |
| `status` | VARCHAR(32) | Session status |
| `quality` | VARCHAR(16) | Quality setting |
| `started_at` | TIMESTAMPTZ | Start timestamp |
| `last_heartbeat_at` | TIMESTAMPTZ | Last heartbeat |
| `ended_at` | TIMESTAMPTZ | End timestamp |
| `duration_seconds` | INTEGER | Session duration |
| `bytes_transferred` | BIGINT | Data transferred |
| `denial_reason` | TEXT | Denial reason if rejected |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Session Statuses:**
- `active` - Currently streaming
- `ended` - Normally ended
- `evicted` - Forcibly removed
- `timeout` - Heartbeat timeout
- `denied` - Admission denied

**Indexes:**
- `idx_sg_sessions_source_account` - source_account_id
- `idx_sg_sessions_app` - app_id
- `idx_sg_sessions_stream` - stream_id
- `idx_sg_sessions_user` - user_id
- `idx_sg_sessions_status` - status (partial WHERE status = 'active')
- `idx_sg_sessions_heartbeat` - last_heartbeat_at (partial WHERE status = 'active')

### sg_streams

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `stream_id` | VARCHAR(255) | Stream identifier |
| `title` | VARCHAR(512) | Stream title |
| `stream_type` | VARCHAR(32) | Stream type (live, vod) |
| `status` | VARCHAR(32) | Stream status |
| `source_device_id` | VARCHAR(255) | Source device ID |
| `ingest_url` | TEXT | Ingest URL |
| `playback_url` | TEXT | Playback URL |
| `thumbnail_url` | TEXT | Thumbnail URL |
| `max_viewers` | INTEGER | Max concurrent viewers |
| `current_viewers` | INTEGER | Current viewer count |
| `total_viewers` | INTEGER | Total unique viewers |
| `peak_viewers` | INTEGER | Peak concurrent viewers |
| `started_at` | TIMESTAMPTZ | Start timestamp |
| `ended_at` | TIMESTAMPTZ | End timestamp |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Stream Statuses:**
- `inactive` - Not streaming
- `active` - Currently streaming
- `ended` - Stream ended

**Indexes:**
- `idx_sg_streams_source_account` - source_account_id
- `idx_sg_streams_app` - app_id
- `idx_sg_streams_status` - status

**Unique Constraint:**
- `(source_account_id, app_id, stream_id)`

### sg_admission_rules

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `name` | VARCHAR(255) | Rule name |
| `rule_type` | VARCHAR(32) | Rule type |
| `conditions` | JSONB | Rule conditions |
| `action` | VARCHAR(16) | Action (allow, deny) |
| `priority` | INTEGER | Rule priority (higher = first) |
| `active` | BOOLEAN | Rule active flag |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Rule Types:**
- `subscription` - Based on subscription plan
- `geolocation` - Based on location
- `device` - Based on device type
- `time` - Based on time/date
- `capacity` - Based on viewer count
- `custom` - Custom rule logic

**Actions:**
- `allow` - Allow access
- `deny` - Deny access

**Indexes:**
- `idx_sg_rules_source_account` - source_account_id
- `idx_sg_rules_app` - app_id
- `idx_sg_rules_priority` - priority DESC
- `idx_sg_rules_active` - active

### sg_viewer_analytics

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `app_id` | VARCHAR(64) | Application ID |
| `stream_id` | VARCHAR(255) | Stream identifier |
| `timestamp` | TIMESTAMPTZ | Snapshot timestamp |
| `concurrent_viewers` | INTEGER | Concurrent viewer count |
| `quality_breakdown` | JSONB | Viewers per quality |
| `device_breakdown` | JSONB | Viewers per device type |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_sg_analytics_source_account` - source_account_id
- `idx_sg_analytics_app` - app_id
- `idx_sg_analytics_stream` - stream_id
- `idx_sg_analytics_timestamp` - timestamp DESC

---

## Analytics Views

### sg_concurrent_viewers_over_time

Concurrent viewers over time for streams.

```sql
SELECT
  stream_id,
  timestamp,
  concurrent_viewers
FROM sg_viewer_analytics
WHERE timestamp >= NOW() - INTERVAL '24 hours'
ORDER BY timestamp DESC;
```

### sg_denial_rates

Denial rates by stream.

```sql
SELECT
  stream_id,
  COUNT(*) FILTER (WHERE status = 'denied') as denied,
  COUNT(*) as total,
  ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'denied') / COUNT(*), 2) as denial_rate
FROM sg_stream_sessions
WHERE created_at >= NOW() - INTERVAL '7 days'
GROUP BY stream_id;
```

### sg_stream_duration_distribution

Distribution of viewing durations.

```sql
SELECT
  CASE
    WHEN duration_seconds < 300 THEN '< 5 min'
    WHEN duration_seconds < 1800 THEN '5-30 min'
    WHEN duration_seconds < 3600 THEN '30-60 min'
    ELSE '> 1 hour'
  END as duration_bucket,
  COUNT(*) as count
FROM sg_stream_sessions
WHERE status IN ('ended', 'evicted')
  AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY duration_bucket
ORDER BY count DESC;
```

### sg_device_type_breakdown

Viewers by device type.

```sql
SELECT
  device_type,
  COUNT(*) as sessions,
  AVG(duration_seconds) as avg_duration,
  SUM(bytes_transferred) / 1024 / 1024 / 1024 as total_gb
FROM sg_stream_sessions
WHERE created_at >= NOW() - INTERVAL '7 days'
GROUP BY device_type
ORDER BY sessions DESC;
```

---

## TypeScript Implementation

### File Structure

```
plugins/stream-gateway/ts/src/
├── types.ts          # TypeScript interfaces
├── config.ts         # Configuration loading
├── database.ts       # Database operations
├── gateway.ts        # Admission logic
├── analytics.ts      # Analytics tracking
├── server.ts         # HTTP server
├── cli.ts            # CLI commands
└── index.ts          # Module exports
```

### Key Components

#### StreamGateway (gateway.ts)
- Admission control
- Rule evaluation
- Session management
- Heartbeat processing

#### AnalyticsTracker (analytics.ts)
- Viewer tracking
- Concurrent viewers
- Duration analytics
- Quality/device breakdown

#### StreamGatewayDatabase (database.ts)
- Schema initialization
- Session CRUD
- Stream management
- Analytics queries

---

## Examples

### Example 1: Admit User to Stream

```typescript
const response = await fetch('http://localhost:3601/v1/admit', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    stream_id: 'live-event-123',
    user_id: 'user456',
    device_id: 'web-browser-xyz',
    device_type: 'web',
    quality: 'auto'
  })
});

const { data } = await response.json();

if (data.admitted) {
  console.log(`Session ID: ${data.session_id}`);
  console.log(`Playback URL: ${data.playback_url}`);

  // Send heartbeats every 30 seconds
  setInterval(async () => {
    await fetch(`http://localhost:3601/v1/sessions/${data.session_id}/heartbeat`, {
      method: 'POST'
    });
  }, 30000);
} else {
  console.log(`Denied: ${data.reason}`);
}
```

### Example 2: Monitor Active Streams

```bash
#!/bin/bash

# Monitor active streams every 10 seconds
while true; do
  clear
  echo "=== Active Streams ==="
  curl -s http://localhost:3601/v1/streams?status=active | jq -r '.data[] | "\(.stream_id): \(.current_viewers)/\(.max_viewers) viewers"'

  sleep 10
done
```

### Example 3: Viewer Analytics

```sql
-- Peak viewing times
SELECT
  DATE_TRUNC('hour', timestamp) as hour,
  MAX(concurrent_viewers) as peak_viewers,
  AVG(concurrent_viewers) as avg_viewers
FROM sg_viewer_analytics
WHERE timestamp >= NOW() - INTERVAL '7 days'
GROUP BY hour
ORDER BY hour DESC;

-- Most popular streams
SELECT
  s.stream_id,
  s.title,
  COUNT(DISTINCT ss.user_id) as unique_viewers,
  s.peak_viewers,
  AVG(ss.duration_seconds) / 60 as avg_watch_minutes
FROM sg_streams s
JOIN sg_stream_sessions ss ON ss.stream_id = s.stream_id
WHERE s.created_at >= NOW() - INTERVAL '30 days'
GROUP BY s.stream_id, s.title, s.peak_viewers
ORDER BY unique_viewers DESC
LIMIT 20;

-- Device type trends
SELECT
  DATE_TRUNC('day', created_at) as date,
  device_type,
  COUNT(*) as sessions
FROM sg_stream_sessions
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY date, device_type
ORDER BY date DESC, sessions DESC;
```

### Example 4: Admission Rule

```typescript
// Create geolocation rule
await fetch('http://localhost:3601/v1/rules', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    name: 'US Only',
    rule_type: 'geolocation',
    conditions: {
      allowed_countries: ['US']
    },
    action: 'deny',
    priority: 5,
    active: true
  })
});

// Create capacity rule
await fetch('http://localhost:3601/v1/rules', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    name: 'Capacity Limit',
    rule_type: 'capacity',
    conditions: {
      max_viewers: 100
    },
    action: 'deny',
    priority: 1,
    active: true
  })
});
```

---

## Troubleshooting

### Common Issues

#### Stream at Capacity

**Error:**
```
admitted: false, reason: "Stream at capacity"
```

**Solution:**
1. Increase stream `max_viewers`
2. Wait for viewers to leave
3. Evict stale sessions: check heartbeat timeouts

#### Too Many Device Streams

**Error:**
```
admitted: false, reason: "Device stream limit exceeded"
```

**Solution:**
User has too many concurrent streams on same device:
1. Check `SG_DEFAULT_MAX_DEVICE_STREAMS` limit
2. End inactive sessions
3. Increase per-app limit if needed

#### Heartbeat Timeout

**Problem:**
Sessions automatically evicted.

**Solution:**
1. Verify client sends heartbeats
2. Check `SG_HEARTBEAT_INTERVAL` and `SG_HEARTBEAT_TIMEOUT`
3. Ensure network stability

#### Session Not Ending

**Problem:**
Sessions remain active after user leaves.

**Solution:**
1. Ensure client calls `/sessions/:id/end` on cleanup
2. Rely on heartbeat timeout as fallback
3. Run cleanup job to evict stale sessions

### Cleanup Stale Sessions

```sql
-- Find stale sessions (no heartbeat in 2 minutes)
SELECT id, stream_id, user_id, last_heartbeat_at
FROM sg_stream_sessions
WHERE status = 'active'
  AND last_heartbeat_at < NOW() - INTERVAL '2 minutes';

-- Auto-evict via API
-- curl -X POST http://localhost:3601/v1/evict/{session-id}
```

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug nself plugin stream-gateway server
```

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Stream-Gateway
- **Issues**: https://github.com/acamarata/nself-plugins/issues
