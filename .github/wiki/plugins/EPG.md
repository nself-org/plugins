# EPG

Electronic program guide with XMLTV import, channel management, and schedule queries

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

The EPG (Electronic Program Guide) plugin provides comprehensive TV guide functionality with support for XMLTV feeds, Schedules Direct, channel management, and program schedule queries. It's ideal for IPTV applications, streaming services, or any platform that needs to display TV listings.

### Key Features

- **XMLTV Import** - Import program data from XMLTV format feeds
- **Schedules Direct Integration** - Access official broadcast schedules
- **Channel Management** - Organize channels into groups and categories
- **Program Search** - Search and filter programs by title, genre, time, and channel
- **What's On Now** - Quick queries for currently airing programs
- **Program Reminders** - Set reminders for upcoming shows
- **Primetime Highlights** - Featured programs during primetime hours
- **Multi-Day Guide** - Configurable look-ahead period for schedules
- **Timezone Support** - Handle channels from different timezones

## Quick Start

```bash
# Install the plugin
nself plugin install epg

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export EPG_PLUGIN_PORT=3031
export EPG_XMLTV_URLS="https://example.com/guide.xml"

# Initialize the database schema
nself plugin epg init

# Start the server
nself plugin epg server
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `""` | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `EPG_PLUGIN_PORT` | No | `3031` | HTTP server port |
| `EPG_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `EPG_XMLTV_URLS` | No | `""` | Comma-separated XMLTV feed URLs |
| `EPG_SCHEDULES_DIRECT_USERNAME` | No | `""` | Schedules Direct username |
| `EPG_SCHEDULES_DIRECT_PASSWORD` | No | `""` | Schedules Direct password |
| `EPG_SCHEDULES_DIRECT_LINEUP` | No | `""` | Schedules Direct lineup ID |
| `EPG_DEFAULT_TIMEZONE` | No | `America/New_York` | Default timezone for schedules |
| `EPG_PRIMETIME_START` | No | `19:00` | Primetime start time (24h format) |
| `EPG_PRIMETIME_END` | No | `23:00` | Primetime end time (24h format) |
| `EPG_GUIDE_DAYS_AHEAD` | No | `14` | Days of schedule data to maintain |
| `EPG_GUIDE_DAYS_RETAIN` | No | `7` | Days of past schedule data to keep |
| `EPG_NOTIFY_BEFORE_MINUTES` | No | `5` | Minutes before program to send reminder |
| `EPG_NOTIFY_LIVE_EVENTS` | No | `true` | Notify for live events |
| `EPG_CLEANUP_OLD_SCHEDULES_DAYS` | No | `7` | Days before cleanup of old schedules |
| `EPG_CLEANUP_CRON` | No | `0 4 * * *` | Cleanup cron (daily at 4am) |
| `EPG_XMLTV_REFRESH_CRON` | No | `0 3 * * *` | XMLTV refresh cron (daily at 3am) |
| `EPG_APP_IDS` | No | `primary` | Comma-separated application IDs |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server Configuration
EPG_PLUGIN_PORT=3031
EPG_PLUGIN_HOST=0.0.0.0

# XMLTV Data Sources
EPG_XMLTV_URLS=https://example.com/guide.xml,https://backup.com/epg.xml

# Schedules Direct (alternative/additional source)
EPG_SCHEDULES_DIRECT_USERNAME=your-username
EPG_SCHEDULES_DIRECT_PASSWORD=your-password
EPG_SCHEDULES_DIRECT_LINEUP=USA-OTA-12345

# Guide Settings
EPG_DEFAULT_TIMEZONE=America/New_York
EPG_PRIMETIME_START=19:00
EPG_PRIMETIME_END=23:00
EPG_GUIDE_DAYS_AHEAD=14
EPG_GUIDE_DAYS_RETAIN=7

# Notifications
EPG_NOTIFY_BEFORE_MINUTES=5
EPG_NOTIFY_LIVE_EVENTS=true

# Maintenance
EPG_CLEANUP_OLD_SCHEDULES_DAYS=7
EPG_CLEANUP_CRON=0 4 * * *
EPG_XMLTV_REFRESH_CRON=0 3 * * *

# Multi-App Support
EPG_APP_IDS=primary,iptv-app
```

## CLI Commands

### `init`
Initialize the EPG database schema.

```bash
nself plugin epg init
```

### `server`
Start the EPG HTTP server.

```bash
nself plugin epg server
```

### `channels`
List channels.

```bash
# List all channels
nself plugin epg channels

# Search channels
nself plugin epg channels --search "HBO"

# Filter by group
nself plugin epg channels --group "Premium Channels"
```

### `channel-groups`
Manage channel groups.

```bash
# List groups
nself plugin epg channel-groups list

# Create group
nself plugin epg channel-groups create \
  --name "News Channels" \
  --description "24/7 news networks"

# Add channel to group
nself plugin epg channel-groups add-channel \
  --group group-uuid \
  --channel channel-uuid
```

### `now`
What's on right now.

```bash
# All channels
nself plugin epg now

# Specific channel
nself plugin epg now --channel channel-uuid

# By group
nself plugin epg now --group "Entertainment"
```

### `tonight`
Tonight's primetime schedule.

```bash
# Primetime for all channels
nself plugin epg tonight

# Specific group
nself plugin epg tonight --group "Premium Channels"
```

### `schedule`
View channel schedule.

```bash
# Today's schedule for a channel
nself plugin epg schedule --channel channel-uuid

# Specific date
nself plugin epg schedule \
  --channel channel-uuid \
  --date 2025-02-15

# Date range
nself plugin epg schedule \
  --channel channel-uuid \
  --from "2025-02-15" \
  --to "2025-02-20"
```

### `search`
Search programs.

```bash
# Search by title
nself plugin epg search --query "breaking bad"

# Search with filters
nself plugin epg search \
  --query "sports" \
  --genre "Sports" \
  --date "2025-02-15"

# Search upcoming programs
nself plugin epg search \
  --query "news" \
  --from "2025-02-11T18:00:00Z" \
  --to "2025-02-11T23:00:00Z"
```

### `import`
Import EPG data from XMLTV.

```bash
# Import from configured URLs
nself plugin epg import

# Import from specific file
nself plugin epg import --file /path/to/guide.xml

# Import from URL
nself plugin epg import --url https://example.com/guide.xml
```

### `sync`
Trigger sync from data sources.

```bash
# Sync from all configured sources
nself plugin epg sync

# Sync only XMLTV
nself plugin epg sync --source xmltv

# Sync only Schedules Direct
nself plugin epg sync --source schedules-direct
```

### `stats`
Show EPG statistics.

```bash
nself plugin epg stats

# Example output:
# {
#   "totalChannels": 250,
#   "totalPrograms": 85000,
#   "totalSchedules": 125000,
#   "channelGroups": 15,
#   "oldestSchedule": "2025-02-04T00:00:00Z",
#   "newestSchedule": "2025-02-25T23:59:59Z"
# }
```

## REST API

### Channels

#### `GET /api/channels`
List channels.

**Query Parameters:**
- `search` (optional): Search channel name
- `groupId` (optional): Filter by group
- `limit` (optional, default: 100)
- `offset` (optional, default: 0)

**Response:**
```json
{
  "data": [
    {
      "id": "channel-uuid",
      "name": "HBO",
      "number": "201",
      "logoUrl": "https://...",
      "language": "en",
      "country": "US",
      "timezone": "America/New_York",
      "streamUrl": "https://...",
      "active": true
    }
  ],
  "total": 250
}
```

#### `GET /api/channels/:id`
Get channel details.

**Response:**
```json
{
  "id": "channel-uuid",
  "name": "HBO",
  "number": "201",
  "logoUrl": "https://...",
  "currentProgram": {
    "title": "Game of Thrones",
    "startTime": "2025-02-11T20:00:00Z",
    "endTime": "2025-02-11T21:00:00Z"
  },
  "nextProgram": {
    "title": "Last Week Tonight",
    "startTime": "2025-02-11T21:00:00Z",
    "endTime": "2025-02-11T21:30:00Z"
  }
}
```

### Programs

#### `GET /api/programs/now`
Get currently airing programs.

**Query Parameters:**
- `channelId` (optional): Filter by channel
- `groupId` (optional): Filter by channel group
- `genre` (optional): Filter by genre

**Response:**
```json
{
  "data": [
    {
      "programId": "program-uuid",
      "channelId": "channel-uuid",
      "channelName": "HBO",
      "title": "Game of Thrones",
      "episode": "S1E1: Winter Is Coming",
      "description": "...",
      "genre": ["Drama", "Fantasy"],
      "rating": "TV-MA",
      "startTime": "2025-02-11T20:00:00Z",
      "endTime": "2025-02-11T21:00:00Z",
      "progress": 0.45
    }
  ]
}
```

#### `GET /api/programs/tonight`
Get tonight's primetime programs.

**Query Parameters:**
- `groupId` (optional): Filter by channel group
- `genre` (optional): Filter by genre

**Response:**
```json
{
  "primetimeStart": "19:00",
  "primetimeEnd": "23:00",
  "data": [
    {
      "programId": "program-uuid",
      "channelId": "channel-uuid",
      "channelName": "NBC",
      "title": "The Tonight Show",
      "startTime": "2025-02-11T22:35:00Z",
      "endTime": "2025-02-11T23:37:00Z",
      "genre": ["Talk Show"],
      "rating": "TV-PG"
    }
  ]
}
```

#### `GET /api/programs/search`
Search programs.

**Query Parameters:**
- `q` (required): Search query
- `genre` (optional): Filter by genre
- `from` (optional): Start time (ISO 8601)
- `to` (optional): End time (ISO 8601)
- `channelId` (optional): Filter by channel
- `limit` (optional, default: 50)

**Response:**
```json
{
  "data": [
    {
      "programId": "program-uuid",
      "title": "Breaking Bad",
      "description": "...",
      "genre": ["Drama", "Thriller"],
      "schedules": [
        {
          "channelId": "channel-uuid",
          "channelName": "AMC",
          "startTime": "2025-02-11T22:00:00Z",
          "endTime": "2025-02-11T23:00:00Z"
        }
      ]
    }
  ],
  "total": 15
}
```

### Schedules

#### `GET /api/schedules`
Get program schedules.

**Query Parameters:**
- `channelId` (required): Channel ID
- `date` (optional, default: today): Date (YYYY-MM-DD)
- `from` (optional): Start time (ISO 8601)
- `to` (optional): End time (ISO 8601)

**Response:**
```json
{
  "channel": {
    "id": "channel-uuid",
    "name": "HBO",
    "number": "201"
  },
  "date": "2025-02-11",
  "data": [
    {
      "id": "schedule-uuid",
      "programId": "program-uuid",
      "title": "Game of Thrones",
      "episode": "S1E1",
      "description": "...",
      "startTime": "2025-02-11T20:00:00Z",
      "endTime": "2025-02-11T21:00:00Z",
      "genre": ["Drama", "Fantasy"],
      "rating": "TV-MA"
    }
  ]
}
```

### Channel Groups

#### `GET /api/channel-groups`
List channel groups.

**Response:**
```json
{
  "data": [
    {
      "id": "group-uuid",
      "name": "News Channels",
      "description": "24/7 news networks",
      "channelCount": 15,
      "sortOrder": 1
    }
  ]
}
```

#### `GET /api/channel-groups/:id/channels`
Get channels in a group.

**Response:**
```json
{
  "group": {
    "id": "group-uuid",
    "name": "News Channels"
  },
  "data": [
    {
      "id": "channel-uuid",
      "name": "CNN",
      "number": "200",
      "logoUrl": "https://..."
    }
  ]
}
```

## Webhook Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `epg.channel.added` | New channel added | `{ channelId, name }` |
| `epg.program.upcoming` | Program starting soon | `{ programId, channelId, title, startTime, minutesUntil }` |
| `epg.program.started` | Program started | `{ programId, channelId, title }` |
| `epg.schedule.updated` | Schedule data updated | `{ channelId, dateRange }` |
| `epg.import.completed` | XMLTV import completed | `{ source, programsImported, schedulesImported }` |

## Database Schema

### np_epg_channels

```sql
CREATE TABLE IF NOT EXISTS np_epg_channels (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  channel_id VARCHAR(255) UNIQUE NOT NULL,
  name VARCHAR(255) NOT NULL,
  number VARCHAR(20),
  logo_url TEXT,
  language VARCHAR(10),
  country VARCHAR(10),
  timezone VARCHAR(50),
  stream_url TEXT,
  metadata JSONB DEFAULT '{}',
  active BOOLEAN DEFAULT true,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### np_epg_programs

```sql
CREATE TABLE IF NOT EXISTS np_epg_programs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  program_id VARCHAR(255) UNIQUE NOT NULL,
  title VARCHAR(500) NOT NULL,
  sub_title VARCHAR(500),
  description TEXT,
  episode_num VARCHAR(50),
  season_num INTEGER,
  episode_num_total INTEGER,
  genre TEXT[],
  rating VARCHAR(20),
  year INTEGER,
  director VARCHAR(255),
  actors TEXT[],
  poster_url TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### np_epg_schedules

```sql
CREATE TABLE IF NOT EXISTS np_epg_schedules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  channel_id UUID NOT NULL REFERENCES np_epg_channels(id) ON DELETE CASCADE,
  program_id UUID NOT NULL REFERENCES np_epg_programs(id) ON DELETE CASCADE,
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ NOT NULL,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(channel_id, start_time)
);

CREATE INDEX IF NOT EXISTS idx_epg_schedules_channel_time
ON np_epg_schedules(channel_id, start_time, end_time);

CREATE INDEX IF NOT EXISTS idx_epg_schedules_time_range
ON np_epg_schedules(start_time, end_time);
```

### np_epg_channel_groups

```sql
CREATE TABLE IF NOT EXISTS np_epg_channel_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  sort_order INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);
```

### np_epg_channel_group_members

```sql
CREATE TABLE IF NOT EXISTS np_epg_channel_group_members (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  group_id UUID NOT NULL REFERENCES np_epg_channel_groups(id) ON DELETE CASCADE,
  channel_id UUID NOT NULL REFERENCES np_epg_channels(id) ON DELETE CASCADE,
  sort_order INTEGER DEFAULT 0,
  UNIQUE(group_id, channel_id)
);
```

## Examples

### Example 1: TV Guide Interface

```javascript
// Get current programs for all channels
const response = await fetch('http://localhost:3031/api/programs/now');
const { data: nowPlaying } = await response.json();

// Display in grid
nowPlaying.forEach(program => {
  console.log(`[${program.channelName}] ${program.title} (${program.progress * 100}% complete)`);
});
```

### Example 2: Tonight's Primetime Lineup

```sql
-- Get tonight's primetime shows
SELECT
  c.name as channel,
  c.number,
  p.title,
  s.start_time,
  s.end_time,
  p.genre,
  p.rating
FROM np_epg_schedules s
JOIN np_epg_channels c ON c.id = s.channel_id
JOIN np_epg_programs p ON p.id = s.program_id
WHERE s.source_account_id = 'primary'
  AND DATE(s.start_time AT TIME ZONE 'America/New_York') = CURRENT_DATE
  AND EXTRACT(HOUR FROM s.start_time AT TIME ZONE 'America/New_York') BETWEEN 19 AND 23
ORDER BY c.number, s.start_time;
```

### Example 3: Program Reminder System

```javascript
// Check for programs starting in 5 minutes
const fiveMinutesFromNow = new Date(Date.now() + 5 * 60 * 1000);

const response = await fetch(`http://localhost:3031/api/schedules?from=${fiveMinutesFromNow.toISOString()}`);
const { data: upcomingPrograms } = await response.json();

upcomingPrograms.forEach(program => {
  if (userFavorites.includes(program.programId)) {
    sendNotification(`${program.title} starts in 5 minutes on ${program.channelName}`);
  }
});
```

## Troubleshooting

### Common Issues

#### 1. XMLTV Import Fails

**Symptom:** XMLTV import errors or no data imported.

**Solutions:**
- Verify XMLTV URL is accessible: `curl -I $EPG_XMLTV_URLS`
- Check XML format is valid
- Verify sufficient disk space for large files
- Check character encoding (should be UTF-8)
- Review import logs for specific errors

#### 2. Missing Channel Data

**Symptom:** Channels appear but have no programs.

**Solutions:**
- Verify schedules are within look-ahead window
- Check timezone settings match feed timezone
- Run manual sync: `nself plugin epg sync`
- Verify XMLTV feed includes program data for channels
- Check cleanup hasn't removed current schedules

#### 3. Incorrect Times

**Symptom:** Program times don't match expected schedule.

**Solutions:**
- Verify timezone configuration: `echo $EPG_DEFAULT_TIMEZONE`
- Check channel-specific timezones in database
- Ensure XMLTV feed includes proper timezone info
- Verify system time is correct
- Use UTC in database, convert in application layer

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
