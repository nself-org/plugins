# Content Acquisition Plugin

Automated content acquisition orchestrator with RSS feed monitoring, subscription management, release calendar tracking, quality profiles, a rules engine, and a multi-stage download pipeline that coordinates VPN verification, torrent submission, metadata enrichment, and subtitle fetching.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Pipeline Architecture](#pipeline-architecture)
- [RSS Feed Monitor](#rss-feed-monitor)
- [Quality Profiles](#quality-profiles)
- [Acquisition Rules Engine](#acquisition-rules-engine)
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Content Acquisition plugin is a central orchestration layer for automated media acquisition. It monitors RSS feeds for new releases, matches them against user-defined subscriptions, enforces quality preferences through configurable profiles, and manages the complete download lifecycle through a multi-stage pipeline. The plugin coordinates with several sibling plugins (VPN Manager, Torrent Manager, Metadata Enrichment, Subtitle Manager) to form a fully automated end-to-end media acquisition workflow.

### Key Capabilities

- **Subscription Management** - Subscribe to TV shows, movie collections, artists, or podcasts with per-subscription quality preferences
- **RSS Feed Monitoring** - Scheduled cron-based monitoring of RSS feeds with automatic title parsing, duplicate detection, and subscription matching
- **Release Calendar** - Track upcoming releases across movies, TV episodes, and albums with monitoring status
- **Quality Profiles** - Define preferred resolutions, sources, release groups, language requirements, seeder thresholds, and upgrade policies
- **Acquisition Queue** - Priority-based download queue with retry logic and status tracking
- **Acquisition Rules Engine** - JSON-based conditions/actions rules for flexible download automation
- **Multi-Stage Pipeline** - Orchestrated pipeline: VPN check, torrent submission, download polling, metadata enrichment, subtitle fetch
- **Pipeline Retry** - Intelligent retry from the exact failed stage rather than restarting from scratch
- **Graceful Degradation** - Optional pipeline stages (metadata, subtitles) are skipped rather than failing the entire pipeline
- **Acquisition History** - Complete history of all acquisitions with upgrade tracking
- **Multi-App Isolation** - Full `source_account_id` isolation across all tables

### Plugin Details

| Property | Value |
|----------|-------|
| **Name** | content-acquisition |
| **Version** | 1.0.0 |
| **Category** | media |
| **Subcategory** | orchestration |
| **Port** | 3202 |
| **Language** | TypeScript |
| **Runtime** | Node.js |
| **Min nself Version** | 0.4.8 |
| **Database Tables** | 9 |
| **Webhook Events** | 6 |
| **API Endpoints** | 19 |
| **CLI Commands** | 6 |

### Sibling Plugin Dependencies

The Content Acquisition plugin integrates with these sibling plugins:

| Plugin | Purpose | Required |
|--------|---------|----------|
| VPN Manager | Verifies VPN is active before torrent downloads | Yes (pipeline) |
| Torrent Manager | Searches, submits, and monitors torrent downloads | Yes (pipeline) |
| Metadata Enrichment | Enriches downloaded content with metadata | No (graceful skip) |
| Subtitle Manager | Fetches subtitles for downloaded content | No (graceful skip) |

---

## Quick Start

```bash
# Install the plugin
nself plugin install content-acquisition

# Configure environment
cat >> .env << 'EOF'
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
METADATA_ENRICHMENT_URL=http://localhost:3203
TORRENT_MANAGER_URL=http://localhost:3100
VPN_MANAGER_URL=http://localhost:3301
EOF

# Initialize database schema
nself plugin content-acquisition init

# Subscribe to a TV show
nself plugin content-acquisition subscribe "Breaking Bad" --type tv_show

# Start server (includes RSS monitor)
nself plugin content-acquisition server
```

---

## Configuration

### Required Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | - |
| `METADATA_ENRICHMENT_URL` | URL of the Metadata Enrichment plugin API | - |
| `TORRENT_MANAGER_URL` | URL of the Torrent Manager plugin API | - |
| `VPN_MANAGER_URL` | URL of the VPN Manager plugin API | - |

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CONTENT_ACQUISITION_PORT` | HTTP server port | `3202` |
| `SUBTITLE_MANAGER_URL` | URL of the Subtitle Manager plugin API | `http://localhost:3204` |
| `MEDIA_PROCESSING_URL` | URL of the Media Processing plugin API | `http://localhost:3019` |
| `REDIS_HOST` | Redis hostname (for BullMQ job queue) | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | `info` |
| `RSS_CHECK_INTERVAL` | Minutes between scheduled RSS feed checks | `30` |
| `CONTENT_ACQUISITION_API_KEY` | API key for authenticated access (via `loadSecurityConfig`) | - |
| `CONTENT_ACQUISITION_RATE_LIMIT_MAX` | Max requests per rate limit window | `100` |
| `CONTENT_ACQUISITION_RATE_LIMIT_WINDOW_MS` | Rate limit window in milliseconds | `60000` |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Sibling Plugins (required)
METADATA_ENRICHMENT_URL=http://localhost:3203
TORRENT_MANAGER_URL=http://localhost:3100
VPN_MANAGER_URL=http://localhost:3301

# Sibling Plugins (optional)
SUBTITLE_MANAGER_URL=http://localhost:3204
MEDIA_PROCESSING_URL=http://localhost:3019

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Server
CONTENT_ACQUISITION_PORT=3202
LOG_LEVEL=info

# RSS
RSS_CHECK_INTERVAL=30

# Security (optional)
CONTENT_ACQUISITION_API_KEY=your-secret-api-key
CONTENT_ACQUISITION_RATE_LIMIT_MAX=100
CONTENT_ACQUISITION_RATE_LIMIT_WINDOW_MS=60000
```

---

## CLI Commands

### Command Summary

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (creates all tables and indexes) |
| `subscribe <name>` | Create a new content subscription |
| `feeds` | Manage RSS feeds |
| `calendar` | View release calendar |
| `queue` | View and manage the acquisition queue |
| `server` | Start the HTTP API server with RSS monitor |

### init

Initialize the database schema. Creates all 9 tables, indexes, and constraints.

```bash
nself plugin content-acquisition init
```

**Output:**
```
- Initializing content acquisition
+ Database initialized
```

### subscribe

Create a subscription to automatically acquire new content.

```bash
nself plugin content-acquisition subscribe <name> [options]
```

**Options:**

| Option | Description | Default |
|--------|-------------|---------|
| `-t, --type <type>` | Content type: `tv_show`, `movie_collection`, `artist`, `podcast` | `tv_show` |
| `-i, --id <id>` | External content ID (e.g., TMDB or TVDB identifier) | - |

**Examples:**

```bash
# Subscribe to a TV show
nself plugin content-acquisition subscribe "Breaking Bad" --type tv_show

# Subscribe with external ID
nself plugin content-acquisition subscribe "The Wire" --type tv_show --id tmdb:1396

# Subscribe to a movie collection
nself plugin content-acquisition subscribe "Marvel Cinematic Universe" --type movie_collection

# Subscribe to an artist's releases
nself plugin content-acquisition subscribe "Radiohead" --type artist
```

**Output:**
```
- Subscribing to Breaking Bad
+ Subscribed to Breaking Bad
Subscription ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

### queue

View the current acquisition queue with status and priority information.

```bash
nself plugin content-acquisition queue
```

**Output (items in queue):**
```
3 items in queue:

1. Breaking Bad S05E16
   Status: downloading
   Priority: 5

2. The Wire S01E01
   Status: searching
   Priority: 5

3. Inception (2010)
   Status: pending
   Priority: 3
```

**Output (empty queue):**
```
Queue is empty
```

### server

Start the HTTP API server. This also starts the background RSS feed monitor with cron-based scheduled checks.

```bash
nself plugin content-acquisition server
```

**Output:**
```
Starting Content Acquisition Server...

+ Server running on port 3202
```

The server binds to `0.0.0.0` and accepts connections on the configured port. Press Ctrl+C to gracefully shut down.

---

## REST API

### Endpoints Summary

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/v1/subscriptions` | Create subscription |
| `GET` | `/v1/subscriptions` | List subscriptions |
| `POST` | `/v1/feeds` | Add RSS feed |
| `GET` | `/v1/feeds` | List RSS feeds |
| `GET` | `/v1/queue` | View acquisition queue |
| `POST` | `/v1/queue` | Add item to queue |
| `GET` | `/v1/calendar` | View release calendar |
| `POST` | `/v1/profiles` | Create quality profile |
| `GET` | `/api/pipeline` | List pipeline runs |
| `GET` | `/api/pipeline/:id` | Get pipeline run details |
| `POST` | `/api/pipeline/trigger` | Trigger pipeline manually |
| `POST` | `/api/pipeline/retry/:id` | Retry failed pipeline |

### Health & Status

#### GET /health

Health check endpoint. No authentication required.

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2026-02-12T10:00:00.000Z"
}
```

### Subscriptions

#### POST /v1/subscriptions

Create a new content subscription.

**Request Body:**
```json
{
  "contentType": "tv_show",
  "contentId": "tmdb:1396",
  "contentName": "Breaking Bad",
  "qualityProfileId": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `contentType` | string | Yes | One of: `tv_show`, `movie_collection`, `artist`, `podcast` |
| `contentId` | string | No | External identifier (TMDB, TVDB, MusicBrainz, etc.) |
| `contentName` | string | Yes | Display name of the content (1-255 characters) |
| `qualityProfileId` | string (UUID) | No | UUID of a quality profile to apply |

**Response:**
```json
{
  "subscription": {
    "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "source_account_id": "primary",
    "subscription_type": "tv_show",
    "content_id": "tmdb:1396",
    "content_name": "Breaking Bad",
    "content_metadata": {},
    "quality_profile_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "enabled": true,
    "auto_upgrade": false,
    "monitor_future_seasons": true,
    "monitor_existing_seasons": false,
    "season_folder": true,
    "last_check_at": null,
    "last_download_at": null,
    "next_check_at": null,
    "created_at": "2026-02-12T10:00:00.000Z",
    "updated_at": "2026-02-12T10:00:00.000Z"
  }
}
```

#### GET /v1/subscriptions

List all subscriptions for the current account.

**Response:**
```json
{
  "subscriptions": [
    {
      "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
      "source_account_id": "primary",
      "subscription_type": "tv_show",
      "content_name": "Breaking Bad",
      "enabled": true,
      "auto_upgrade": false,
      "monitor_future_seasons": true,
      "created_at": "2026-02-12T10:00:00.000Z",
      "updated_at": "2026-02-12T10:00:00.000Z"
    }
  ]
}
```

### RSS Feeds

#### POST /v1/feeds

Add a new RSS feed to monitor.

**Request Body:**
```json
{
  "name": "EZTV TV Shows",
  "url": "https://eztv.re/ezrss.xml",
  "feedType": "tv_shows"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name for the feed (1-255 characters) |
| `url` | string (URI) | Yes | RSS feed URL |
| `feedType` | string | Yes | One of: `tv_shows`, `movies`, `anime`, `music` |

**Response:**
```json
{
  "feed": {
    "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
    "source_account_id": "primary",
    "name": "EZTV TV Shows",
    "url": "https://eztv.re/ezrss.xml",
    "feed_type": "tv_shows",
    "enabled": true,
    "check_interval_minutes": 60,
    "quality_profile_id": null,
    "last_check_at": null,
    "last_success_at": null,
    "last_error": null,
    "consecutive_failures": 0,
    "next_check_at": null,
    "created_at": "2026-02-12T10:00:00.000Z",
    "updated_at": "2026-02-12T10:00:00.000Z"
  }
}
```

#### GET /v1/feeds

List all RSS feeds for the current account.

**Response:**
```json
{
  "feeds": [
    {
      "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
      "name": "EZTV TV Shows",
      "url": "https://eztv.re/ezrss.xml",
      "feed_type": "tv_shows",
      "enabled": true,
      "check_interval_minutes": 60,
      "last_check_at": "2026-02-12T10:30:00.000Z",
      "last_success_at": "2026-02-12T10:30:00.000Z",
      "consecutive_failures": 0,
      "created_at": "2026-02-12T10:00:00.000Z"
    }
  ]
}
```

### Acquisition Queue

#### GET /v1/queue

View the active acquisition queue (items with status: pending, searching, matched, downloading).

**Response:**
```json
{
  "queue": [
    {
      "id": "d4e5f6a7-b8c9-0123-defa-234567890123",
      "source_account_id": "primary",
      "content_type": "tv_episode",
      "content_name": "Breaking Bad",
      "year": null,
      "season": 5,
      "episode": 16,
      "quality_profile_id": null,
      "requested_by": "rss_monitor",
      "request_source_id": "feed-item-uuid",
      "status": "downloading",
      "priority": 5,
      "attempts": 1,
      "max_attempts": 3,
      "matched_torrent": { "name": "Breaking.Bad.S05E16.1080p.BluRay", "seeders": 150 },
      "download_id": "torrent-download-uuid",
      "error_message": null,
      "created_at": "2026-02-12T10:30:00.000Z",
      "started_at": "2026-02-12T10:31:00.000Z",
      "completed_at": null
    }
  ]
}
```

#### POST /v1/queue

Manually add an item to the acquisition queue.

**Request Body:**
```json
{
  "contentType": "movie",
  "contentName": "Inception",
  "year": 2010
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `contentType` | string | Yes | One of: `movie`, `tv_episode`, `music`, `other` |
| `contentName` | string | Yes | Content name (1-255 characters) |
| `year` | integer | No | Release year (1900-2100) |
| `season` | integer | No | Season number (0-200) |
| `episode` | integer | No | Episode number (0-10000) |

**Response:**
```json
{
  "item": {
    "id": "e5f6a7b8-c9d0-1234-efab-345678901234",
    "source_account_id": "primary",
    "content_type": "movie",
    "content_name": "Inception",
    "year": 2010,
    "season": null,
    "episode": null,
    "quality_profile_id": null,
    "requested_by": "api",
    "status": "pending",
    "priority": 5,
    "attempts": 0,
    "max_attempts": 3,
    "created_at": "2026-02-12T10:00:00.000Z"
  }
}
```

### Calendar

#### GET /v1/calendar

View the release calendar.

**Response:**
```json
{
  "calendar": []
}
```

### Quality Profiles

#### POST /v1/profiles

Create a quality profile.

**Request Body:**
```json
{
  "name": "HD Preferred",
  "preferredQualities": ["1080p", "720p"],
  "minSeeders": 5
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Profile name (1-100 characters) |
| `preferredQualities` | string[] | No | Array of: `2160p`, `1080p`, `720p`, `480p` (min 1 item) | `["1080p", "720p"]` |
| `minSeeders` | integer | No | Minimum seeder count (0-10000) | `1` |

**Response:**
```json
{
  "profile": {
    "id": "f6a7b8c9-d0e1-2345-fabc-456789012345",
    "source_account_id": "primary",
    "name": "HD Preferred",
    "description": null,
    "preferred_qualities": ["1080p", "720p"],
    "max_size_gb": null,
    "min_size_gb": null,
    "preferred_sources": ["BluRay", "WEB-DL"],
    "excluded_sources": ["CAM", "TS", "TC"],
    "preferred_groups": null,
    "excluded_groups": null,
    "preferred_languages": ["English"],
    "require_subtitles": false,
    "min_seeders": 5,
    "wait_for_better_quality": true,
    "wait_hours": 24,
    "created_at": "2026-02-12T10:00:00.000Z",
    "updated_at": "2026-02-12T10:00:00.000Z"
  }
}
```

### Pipeline

#### GET /api/pipeline

List pipeline runs with optional filtering and pagination.

**Query Parameters:**

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `status` | string | Filter by pipeline status | - |
| `limit` | string (integer) | Maximum results to return | `50` |
| `offset` | string (integer) | Number of results to skip | `0` |

**Response:**
```json
{
  "runs": [
    {
      "id": 1,
      "source_account_id": "primary",
      "trigger_type": "rss_monitor",
      "trigger_source": "EZTV TV Shows",
      "content_title": "Breaking Bad",
      "content_type": "tv_episode",
      "status": "completed",
      "vpn_check_status": "passed",
      "torrent_status": "completed",
      "torrent_download_id": "dl-abc-123",
      "metadata_status": "completed",
      "subtitle_status": "completed",
      "encoding_status": "pending",
      "detected_at": "2026-02-12T10:30:00.000Z",
      "vpn_checked_at": "2026-02-12T10:30:01.000Z",
      "torrent_submitted_at": "2026-02-12T10:30:02.000Z",
      "download_completed_at": "2026-02-12T11:15:00.000Z",
      "metadata_enriched_at": "2026-02-12T11:15:05.000Z",
      "subtitles_fetched_at": "2026-02-12T11:15:10.000Z",
      "encoding_completed_at": null,
      "pipeline_completed_at": "2026-02-12T11:15:10.000Z",
      "error_message": null,
      "metadata": {
        "magnet_url": "magnet:?xt=urn:btih:...",
        "feed_id": "feed-uuid",
        "feed_item_id": "item-uuid",
        "season": 5,
        "episode": 16,
        "quality": "1080p"
      },
      "created_at": "2026-02-12T10:30:00.000Z",
      "updated_at": "2026-02-12T11:15:10.000Z"
    }
  ],
  "total": 42
}
```

#### GET /api/pipeline/:id

Get details for a specific pipeline run.

**Response:**
```json
{
  "run": {
    "id": 1,
    "source_account_id": "primary",
    "trigger_type": "rss_monitor",
    "trigger_source": "EZTV TV Shows",
    "content_title": "Breaking Bad",
    "content_type": "tv_episode",
    "status": "completed",
    "vpn_check_status": "passed",
    "torrent_status": "completed",
    "torrent_download_id": "dl-abc-123",
    "metadata_status": "completed",
    "subtitle_status": "completed",
    "encoding_status": "pending",
    "detected_at": "2026-02-12T10:30:00.000Z",
    "vpn_checked_at": "2026-02-12T10:30:01.000Z",
    "torrent_submitted_at": "2026-02-12T10:30:02.000Z",
    "download_completed_at": "2026-02-12T11:15:00.000Z",
    "metadata_enriched_at": "2026-02-12T11:15:05.000Z",
    "subtitles_fetched_at": "2026-02-12T11:15:10.000Z",
    "encoding_completed_at": null,
    "pipeline_completed_at": "2026-02-12T11:15:10.000Z",
    "error_message": null,
    "metadata": {},
    "created_at": "2026-02-12T10:30:00.000Z",
    "updated_at": "2026-02-12T11:15:10.000Z"
  }
}
```

**Error Responses:**
- `400` - Invalid pipeline ID (non-integer)
- `404` - Pipeline run not found

#### POST /api/pipeline/trigger

Manually trigger a new pipeline run for a piece of content.

**Request Body:**
```json
{
  "content_title": "Breaking Bad S05E16",
  "content_type": "tv_episode",
  "magnet_url": "magnet:?xt=urn:btih:abc123...",
  "torrent_url": "https://example.com/torrent/abc123.torrent"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content_title` | string | Yes | Title of the content (1-500 characters) |
| `content_type` | string | No | Content type identifier (max 100 characters) |
| `magnet_url` | string | No | Magnet URI for the torrent (max 2048 characters) |
| `torrent_url` | string | No | Direct torrent file URL (max 2048 characters) |

At least one of `magnet_url` or `torrent_url` should be provided for the pipeline to proceed past the torrent submission stage.

**Response (202 Accepted):**
```json
{
  "run": {
    "id": 42,
    "source_account_id": "primary",
    "trigger_type": "manual",
    "trigger_source": "api",
    "content_title": "Breaking Bad S05E16",
    "content_type": "tv_episode",
    "status": "detected",
    "vpn_check_status": "pending",
    "torrent_status": "pending",
    "metadata_status": "pending",
    "subtitle_status": "pending",
    "encoding_status": "pending",
    "metadata": {
      "magnet_url": "magnet:?xt=urn:btih:abc123...",
      "torrent_url": "https://example.com/torrent/abc123.torrent"
    },
    "created_at": "2026-02-12T10:00:00.000Z"
  },
  "message": "Pipeline triggered"
}
```

The pipeline executes asynchronously (fire-and-forget). Use `GET /api/pipeline/:id` to poll for progress.

#### POST /api/pipeline/retry/:id

Retry a failed pipeline run from the stage that failed.

**Response (202 Accepted):**
```json
{
  "message": "Pipeline retry triggered",
  "pipelineId": 42
}
```

**Error Responses:**
- `400` - Invalid pipeline ID or pipeline already completed
- `404` - Pipeline run not found

---

## Webhook Events

The Content Acquisition plugin emits the following webhook events:

| Event | Description |
|-------|-------------|
| `acquisition.subscribed` | A new content subscription was created |
| `acquisition.new_release_detected` | A new release was detected via RSS or calendar |
| `acquisition.queued` | Content was added to the acquisition queue |
| `acquisition.started` | Download has started for queued content |
| `acquisition.completed` | Content was successfully acquired |
| `acquisition.failed` | Content acquisition failed after all retry attempts |

---

## Database Schema

The plugin creates 9 database tables. All tables use `source_account_id` for multi-app isolation.

### quality_profiles

Defines quality preferences and constraints for content acquisition.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `name` | VARCHAR(100) | No | - | Profile display name |
| `description` | TEXT | Yes | - | Optional description |
| `preferred_qualities` | VARCHAR(10)[] | Yes | `['1080p', '720p']` | Ordered list of preferred quality levels |
| `max_size_gb` | DECIMAL(10,2) | Yes | - | Maximum file size in GB |
| `min_size_gb` | DECIMAL(10,2) | Yes | - | Minimum file size in GB |
| `preferred_sources` | VARCHAR(20)[] | Yes | `['BluRay', 'WEB-DL']` | Preferred release sources |
| `excluded_sources` | VARCHAR(20)[] | Yes | `['CAM', 'TS', 'TC']` | Excluded release sources |
| `preferred_groups` | VARCHAR(50)[] | Yes | - | Preferred release groups |
| `excluded_groups` | VARCHAR(50)[] | Yes | - | Excluded release groups |
| `preferred_languages` | VARCHAR(10)[] | Yes | `['English']` | Preferred audio languages |
| `require_subtitles` | BOOLEAN | Yes | `false` | Whether subtitles are required |
| `min_seeders` | INT | Yes | `1` | Minimum seeder count threshold |
| `wait_for_better_quality` | BOOLEAN | Yes | `true` | Wait before downloading in case a better quality appears |
| `wait_hours` | INT | Yes | `24` | Hours to wait for better quality |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Last update timestamp |

### acquisition_subscriptions

Tracks content subscriptions that drive automated acquisition.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `subscription_type` | VARCHAR(50) | No | - | Type: `tv_show`, `movie_collection`, `artist`, `podcast` |
| `content_id` | VARCHAR(255) | Yes | - | External ID (TMDB, TVDB, MusicBrainz, etc.) |
| `content_name` | VARCHAR(255) | No | - | Display name used for RSS matching |
| `content_metadata` | JSONB | Yes | - | Additional metadata about the content |
| `quality_profile_id` | UUID | Yes | - | FK to `quality_profiles(id)` |
| `enabled` | BOOLEAN | Yes | `true` | Whether this subscription is active |
| `auto_upgrade` | BOOLEAN | Yes | `false` | Automatically upgrade when better quality is found |
| `monitor_future_seasons` | BOOLEAN | Yes | `true` | Monitor for future season releases |
| `monitor_existing_seasons` | BOOLEAN | Yes | `false` | Monitor for existing season releases |
| `season_folder` | BOOLEAN | Yes | `true` | Organize downloads into season folders |
| `last_check_at` | TIMESTAMPTZ | Yes | - | Last time this subscription was checked |
| `last_download_at` | TIMESTAMPTZ | Yes | - | Last time content was downloaded |
| `next_check_at` | TIMESTAMPTZ | Yes | - | Next scheduled check time |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Last update timestamp |

**Indexes:**
- `idx_subscriptions_account` on `(source_account_id, enabled)`

### rss_feeds

Stores configured RSS feed sources for monitoring.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `name` | VARCHAR(255) | No | - | Feed display name |
| `url` | TEXT | No | - | RSS feed URL |
| `feed_type` | VARCHAR(50) | No | - | Type: `tv_shows`, `movies`, `anime`, `music` |
| `enabled` | BOOLEAN | Yes | `true` | Whether this feed is actively monitored |
| `check_interval_minutes` | INT | Yes | `60` | Minutes between checks for this feed |
| `quality_profile_id` | UUID | Yes | - | FK to `quality_profiles(id)` - default profile for items from this feed |
| `last_check_at` | TIMESTAMPTZ | Yes | - | Last time this feed was checked |
| `last_success_at` | TIMESTAMPTZ | Yes | - | Last successful check |
| `last_error` | TEXT | Yes | - | Error message from last failed check |
| `consecutive_failures` | INT | Yes | `0` | Number of consecutive check failures |
| `next_check_at` | TIMESTAMPTZ | Yes | - | Next scheduled check time |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Last update timestamp |

**Indexes:**
- `idx_rss_feeds_next_check` on `(next_check_at)` WHERE `enabled = true`

### rss_feed_items

Stores individual items parsed from RSS feeds with structured metadata.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `feed_id` | UUID | No | - | FK to `rss_feeds(id)` (CASCADE on delete) |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `title` | VARCHAR(500) | No | - | Raw RSS item title |
| `link` | TEXT | Yes | - | URL link from RSS item |
| `magnet_uri` | TEXT | Yes | - | Magnet URI extracted from enclosure |
| `info_hash` | VARCHAR(40) | Yes | - | Torrent info hash |
| `pub_date` | TIMESTAMPTZ | Yes | - | RSS item publication date |
| `parsed_title` | VARCHAR(255) | Yes | - | Show/movie name extracted from title |
| `parsed_year` | INT | Yes | - | Year extracted from title |
| `parsed_season` | INT | Yes | - | Season number extracted (e.g., S05) |
| `parsed_episode` | INT | Yes | - | Episode number extracted (e.g., E16) |
| `parsed_quality` | VARCHAR(20) | Yes | - | Quality extracted (1080p, 720p, etc.) |
| `parsed_source` | VARCHAR(50) | Yes | - | Source extracted (BluRay, WEB-DL, etc.) |
| `parsed_group` | VARCHAR(100) | Yes | - | Release group extracted |
| `size_bytes` | BIGINT | Yes | - | File size in bytes |
| `seeders` | INT | Yes | - | Seeder count |
| `leechers` | INT | Yes | - | Leecher count |
| `status` | VARCHAR(50) | Yes | `'pending'` | Processing status |
| `matched_subscription_id` | UUID | Yes | - | FK to `acquisition_subscriptions(id)` if matched |
| `rejection_reason` | TEXT | Yes | - | Reason for rejection if not matched |
| `download_id` | UUID | Yes | - | Download ID if download was initiated |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `processed_at` | TIMESTAMPTZ | Yes | - | When the item was processed |

**Item Statuses:**
- `pending` - Newly ingested, not yet processed
- `matched` - Matched to an active subscription
- `downloaded` - Successfully downloaded
- `rejected` - Did not match any subscription or failed quality checks
- `failed` - Download attempt failed

**Indexes:**
- `idx_rss_items_feed` on `(feed_id, created_at DESC)`

### release_calendar

Tracks upcoming content releases linked to subscriptions.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `content_type` | VARCHAR(50) | No | - | Type: `movie`, `tv_episode`, `album` |
| `content_id` | VARCHAR(255) | No | - | External content identifier |
| `content_name` | VARCHAR(255) | No | - | Display name |
| `season` | INT | Yes | - | Season number (TV episodes) |
| `episode` | INT | Yes | - | Episode number (TV episodes) |
| `release_date` | DATE | No | - | Primary release date |
| `digital_release_date` | DATE | Yes | - | Digital/streaming release date |
| `physical_release_date` | DATE | Yes | - | Physical media release date |
| `subscription_id` | UUID | Yes | - | FK to `acquisition_subscriptions(id)` |
| `quality_profile_id` | UUID | Yes | - | FK to `quality_profiles(id)` |
| `monitoring_enabled` | BOOLEAN | Yes | `true` | Whether to auto-search on release |
| `status` | VARCHAR(50) | Yes | `'awaiting'` | Current status |
| `first_search_at` | TIMESTAMPTZ | Yes | - | When first search was performed |
| `found_at` | TIMESTAMPTZ | Yes | - | When content was found |
| `download_id` | UUID | Yes | - | Download ID |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Last update timestamp |

**Calendar Statuses:**
- `awaiting` - Release date has not passed yet
- `searching` - Actively searching for the release
- `found` - Release found, pending download
- `downloaded` - Successfully downloaded
- `failed` - Search or download failed

**Indexes:**
- `idx_calendar_release_date` on `(release_date, monitoring_enabled)`

### acquisition_queue

Priority-based queue for content pending acquisition.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `content_type` | VARCHAR(50) | No | - | Type: `movie`, `tv_episode`, `music`, `other` |
| `content_name` | VARCHAR(255) | No | - | Content display name |
| `year` | INT | Yes | - | Release year |
| `season` | INT | Yes | - | Season number |
| `episode` | INT | Yes | - | Episode number |
| `quality_profile_id` | UUID | Yes | - | FK to `quality_profiles(id)` |
| `requested_by` | VARCHAR(100) | Yes | - | Source of request (e.g., `api`, `rss_monitor`, `calendar`) |
| `request_source_id` | UUID | Yes | - | ID of the originating feed item or calendar entry |
| `status` | VARCHAR(50) | Yes | `'pending'` | Queue item status |
| `priority` | INT | Yes | `5` | Priority level (higher = more urgent) |
| `attempts` | INT | Yes | `0` | Number of acquisition attempts |
| `max_attempts` | INT | Yes | `3` | Maximum retry attempts |
| `matched_torrent` | JSONB | Yes | - | Details of the matched torrent |
| `download_id` | UUID | Yes | - | Torrent manager download ID |
| `error_message` | TEXT | Yes | - | Last error message |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `started_at` | TIMESTAMPTZ | Yes | - | When acquisition started |
| `completed_at` | TIMESTAMPTZ | Yes | - | When acquisition completed |

**Queue Statuses:**
- `pending` - Waiting to be processed
- `searching` - Searching for matching torrent
- `matched` - Torrent found, waiting to download
- `downloading` - Download in progress
- `completed` - Successfully acquired
- `failed` - All attempts exhausted

**Indexes:**
- `idx_queue_status` on `(status, priority DESC, created_at)`

### acquisition_history

Immutable log of all completed acquisition attempts.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `content_type` | VARCHAR(50) | No | - | Content type |
| `content_name` | VARCHAR(255) | No | - | Content display name |
| `year` | INT | Yes | - | Release year |
| `season` | INT | Yes | - | Season number |
| `episode` | INT | Yes | - | Episode number |
| `torrent_title` | VARCHAR(500) | Yes | - | Full torrent title that was downloaded |
| `torrent_source` | VARCHAR(50) | Yes | - | Source of the torrent (e.g., feed name) |
| `quality` | VARCHAR(20) | Yes | - | Quality level (1080p, 720p, etc.) |
| `size_bytes` | BIGINT | Yes | - | File size in bytes |
| `download_id` | UUID | Yes | - | Torrent manager download ID |
| `status` | VARCHAR(50) | No | - | Outcome: `success`, `failed`, `upgraded` |
| `acquired_from` | VARCHAR(100) | No | - | How the content was acquired |
| `upgrade_of` | UUID | Yes | - | Self-referential FK to `acquisition_history(id)` for upgrade tracking |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Acquisition timestamp |

**History Statuses:**
- `success` - Successfully acquired
- `failed` - Acquisition failed
- `upgraded` - Replaced by a higher quality version

**Indexes:**
- `idx_history_account` on `(source_account_id, created_at DESC)`

### acquisition_rules

JSON-based rules engine for automated acquisition decisions.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | UUID | No | - | Multi-app isolation identifier |
| `name` | VARCHAR(255) | No | - | Rule display name |
| `description` | TEXT | Yes | - | Rule description |
| `conditions` | JSONB | No | - | JSON Logic conditions for rule evaluation |
| `actions` | JSONB | No | - | Actions to take when conditions match |
| `enabled` | BOOLEAN | Yes | `true` | Whether the rule is active |
| `priority` | INT | Yes | `5` | Rule evaluation priority (higher = first) |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Last update timestamp |

### np_ca_pipeline_runs

Tracks multi-stage pipeline executions that coordinate the full download workflow.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | SERIAL | No | auto-increment | Primary key |
| `source_account_id` | TEXT | No | - | Multi-app isolation identifier |
| `trigger_type` | TEXT | No | - | What initiated the pipeline (e.g., `rss_monitor`, `manual`) |
| `trigger_source` | TEXT | Yes | - | Specific source (e.g., feed name, `api`) |
| `content_title` | TEXT | No | - | Title of the content being acquired |
| `content_type` | TEXT | Yes | - | Content type (e.g., `tv_episode`, `movie`) |
| `status` | TEXT | No | `'detected'` | Overall pipeline status |
| `vpn_check_status` | TEXT | Yes | `'pending'` | VPN verification stage status |
| `torrent_status` | TEXT | Yes | `'pending'` | Torrent download stage status |
| `torrent_download_id` | TEXT | Yes | - | Download ID from torrent manager |
| `metadata_status` | TEXT | Yes | `'pending'` | Metadata enrichment stage status |
| `subtitle_status` | TEXT | Yes | `'pending'` | Subtitle fetch stage status |
| `encoding_status` | TEXT | Yes | `'pending'` | Encoding stage status |
| `detected_at` | TIMESTAMPTZ | Yes | `NOW()` | When the content was first detected |
| `vpn_checked_at` | TIMESTAMPTZ | Yes | - | When VPN check completed |
| `torrent_submitted_at` | TIMESTAMPTZ | Yes | - | When torrent was submitted |
| `download_completed_at` | TIMESTAMPTZ | Yes | - | When download completed |
| `metadata_enriched_at` | TIMESTAMPTZ | Yes | - | When metadata enrichment completed |
| `subtitles_fetched_at` | TIMESTAMPTZ | Yes | - | When subtitles were fetched |
| `encoding_completed_at` | TIMESTAMPTZ | Yes | - | When encoding completed |
| `pipeline_completed_at` | TIMESTAMPTZ | Yes | - | When the entire pipeline completed |
| `error_message` | TEXT | Yes | - | Error message if any stage failed |
| `metadata` | JSONB | Yes | `'{}'` | Additional metadata (magnet URLs, feed info, etc.) |
| `created_at` | TIMESTAMPTZ | Yes | `NOW()` | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Yes | `NOW()` | Last update timestamp |

**Pipeline Statuses:**
- `detected` - Pipeline created, not yet started
- `vpn_checking` - VPN verification in progress
- `vpn_waiting` - VPN is down, pipeline paused
- `torrent_submitting` - Submitting torrent to torrent manager
- `downloading` - Torrent download in progress
- `enriching_metadata` - Metadata enrichment in progress
- `fetching_subtitles` - Subtitle fetch in progress
- `retrying` - Pipeline is being retried from a failed stage
- `completed` - All stages completed successfully
- `failed` - Pipeline halted due to an error

**Stage Statuses (per-stage columns):**
- `pending` - Stage not yet started
- `passed` - Stage passed (VPN check)
- `downloading` - In progress (torrent)
- `completed` - Stage completed successfully
- `skipped` - Stage skipped (optional plugin unreachable)
- `failed` - Stage failed

**Indexes:**
- `idx_np_ca_pipeline_source` on `(source_account_id)`
- `idx_np_ca_pipeline_status` on `(status)`
- `idx_np_ca_pipeline_created` on `(created_at DESC)`

---

## Pipeline Architecture

The Content Acquisition plugin includes a multi-stage pipeline orchestrator that coordinates the full download workflow across multiple sibling plugins.

### Pipeline Stages

```
                    +-------------------+
                    |   1. VPN Check    |
                    |   (Required)      |
                    +--------+----------+
                             |
                    pass     |     fail
               +-------------+------------+
               |                          |
               v                          v
    +-------------------+      +-------------------+
    | 2. Torrent Submit |      | Pipeline Paused   |
    |    (Required)     |      | (vpn_waiting)     |
    +--------+----------+      +-------------------+
             |
             v
    +-------------------+
    | 3. Poll Download  |
    | (30s intervals,   |
    |  6 hour timeout)  |
    +--------+----------+
             |
             v
    +-------------------+
    | 4. Metadata       |
    |    Enrichment     |
    |    (Optional)     |
    +--------+----------+
             |
             v
    +-------------------+
    | 5. Subtitle Fetch |
    |    (Optional)     |
    +--------+----------+
             |
             v
    +-------------------+
    |   6. Complete     |
    +-------------------+
```

### Stage Details

#### Stage 1: VPN Check

Calls `GET {VPN_MANAGER_URL}/api/status` to verify the VPN is connected before initiating any torrent download. This is a **hard requirement** -- if the VPN is down or unreachable, the pipeline pauses in `vpn_waiting` status. Torrent downloads will **never** proceed without a verified VPN connection.

#### Stage 2: Torrent Submit

Calls `POST {TORRENT_MANAGER_URL}/api/downloads` with the magnet or torrent URL. The torrent manager returns a download ID that is tracked in `torrent_download_id`.

#### Stage 3: Download Polling

Polls `GET {TORRENT_MANAGER_URL}/api/downloads/{downloadId}` every 30 seconds until the download reports `completed` or `seeding` status. Maximum polling time is approximately 6 hours (720 polls at 30-second intervals). Transient network errors during polling are tolerated.

#### Stage 4: Metadata Enrichment (Optional)

Calls `POST {METADATA_ENRICHMENT_URL}/api/enrich` with the content title and type. If the metadata enrichment plugin is unreachable (network error), the stage is marked as `skipped` and the pipeline continues. If the plugin returns an error response, the stage is marked as `failed` but the pipeline still continues.

#### Stage 5: Subtitle Fetch (Optional)

Calls `POST {SUBTITLE_MANAGER_URL}/api/search` with the content title. Same graceful degradation behavior as metadata enrichment -- unreachable plugin results in `skipped`, error responses result in `failed`, but neither halts the pipeline.

### Pipeline Retry

Failed pipelines can be retried via `POST /api/pipeline/retry/:id`. The retry logic inspects individual stage statuses and resumes from the exact point of failure:

- If VPN check failed, re-run from VPN check
- If torrent submit failed, re-submit
- If downloading was interrupted, re-poll
- If metadata or subtitles failed (not skipped), re-attempt those stages
- Stages marked as `skipped` are not retried

### Timeouts

| Constant | Value | Description |
|----------|-------|-------------|
| HTTP_TIMEOUT | 30 seconds | Timeout for HTTP calls to sibling plugins |
| POLL_INTERVAL_MS | 30 seconds | Interval between download status polls |
| MAX_POLLS | 720 | Maximum polls before timeout (~6 hours) |

---

## RSS Feed Monitor

The RSS Feed Monitor is a background service that automatically checks configured RSS feeds on a cron schedule, parses item titles, matches them against active subscriptions, and triggers the download pipeline.

### How It Works

1. **Scheduled Execution** - A cron job runs every `RSS_CHECK_INTERVAL` minutes (default: 30) and iterates over all enabled RSS feeds.

2. **Feed Fetching** - Each feed URL is fetched and parsed using the `rss-parser` library.

3. **Title Parsing** - Each RSS item title is parsed to extract structured metadata:
   - **Show/Movie Name** - Text before season/quality markers
   - **Season** - Extracted from `S##` pattern (e.g., `S05`)
   - **Episode** - Extracted from `E##` pattern (e.g., `E16`)
   - **Quality** - Detected patterns: `2160p`, `1080p`, `720p`, `480p`

   Example: `Breaking.Bad.S05E16.1080p.BluRay` yields:
   - title: `Breaking.Bad.`
   - season: `5`
   - episode: `16`
   - quality: `1080p`

4. **Duplicate Detection** - Items are checked against the database by `feed_id` + `title`. Duplicate items are silently skipped.

5. **Subscription Matching** - New items are matched against active subscriptions using case-insensitive substring matching on `content_name`. Feed type is mapped to subscription type:
   - `tv_shows` / `anime` -> `tv_show`
   - `movies` -> `movie_collection`
   - `music` -> `artist`

6. **Torrent Search** - For matched items, the monitor calls the Torrent Manager's `POST /v1/search/best-match` endpoint to find the best available torrent.

7. **Queue Insertion** - Matched items are added to the acquisition queue with the matched torrent data and a priority of 5.

8. **Pipeline Trigger** - If a magnet URI or link is available, a full pipeline run is created and executed asynchronously (fire-and-forget).

### Feed Health Tracking

The monitor tracks feed health through:
- `last_check_at` - Updated on every check attempt
- `last_success_at` - Updated only on successful checks
- `last_error` - Stores the error message from the most recent failure
- `consecutive_failures` - Incremented on failure, reset to 0 on success

---

## Quality Profiles

Quality profiles define the preferences and constraints for content acquisition. They can be attached to subscriptions, feeds, or individual queue items.

### Profile Fields

| Field | Description | Default |
|-------|-------------|---------|
| **Preferred Qualities** | Ordered list of acceptable quality levels | `['1080p', '720p']` |
| **Max Size (GB)** | Maximum acceptable file size | No limit |
| **Min Size (GB)** | Minimum acceptable file size | No limit |
| **Preferred Sources** | Preferred release sources (BluRay, WEB-DL, etc.) | `['BluRay', 'WEB-DL']` |
| **Excluded Sources** | Sources to always reject | `['CAM', 'TS', 'TC']` |
| **Preferred Groups** | Preferred release groups | None |
| **Excluded Groups** | Release groups to always reject | None |
| **Preferred Languages** | Preferred audio languages | `['English']` |
| **Require Subtitles** | Only accept releases with subtitles | `false` |
| **Min Seeders** | Minimum seeder count | `1` |
| **Wait for Better Quality** | Delay download to wait for potentially better releases | `true` |
| **Wait Hours** | How long to wait for better quality | `24` |

### Quality Levels

The plugin recognizes these standard quality levels:

| Quality | Resolution | Typical Size (per episode) |
|---------|-----------|---------------------------|
| `2160p` | 3840x2160 (4K UHD) | 5-15 GB |
| `1080p` | 1920x1080 (Full HD) | 1-5 GB |
| `720p` | 1280x720 (HD) | 500 MB - 2 GB |
| `480p` | 720x480 (SD) | 200-500 MB |

### Source Types

| Source | Description |
|--------|-------------|
| `BluRay` | Blu-ray disc source |
| `WEB-DL` | Web download (streaming service) |
| `WEBRip` | Web capture/rip |
| `HDTV` | High-definition television capture |
| `DVDRip` | DVD source |
| `CAM` | Camera recording (excluded by default) |
| `TS` | Telesync (excluded by default) |
| `TC` | Telecine (excluded by default) |

---

## Acquisition Rules Engine

The rules engine uses JSON-based conditions and actions (powered by `json-logic-js`) to automate acquisition decisions.

### Rule Structure

Each rule consists of:
- **Conditions** - A JSONB object defining when the rule applies
- **Actions** - A JSONB object defining what to do when conditions match
- **Priority** - Higher priority rules are evaluated first
- **Enabled** - Rules can be toggled on/off

### Example Rules

**Auto-accept 1080p BluRay releases:**
```json
{
  "name": "Accept HD BluRay",
  "conditions": {
    "and": [
      { "==": [{ "var": "quality" }, "1080p"] },
      { "==": [{ "var": "source" }, "BluRay"] },
      { ">=": [{ "var": "seeders" }, 10] }
    ]
  },
  "actions": {
    "accept": true,
    "priority": 8
  },
  "priority": 10,
  "enabled": true
}
```

**Reject low-quality releases:**
```json
{
  "name": "Reject Low Quality",
  "conditions": {
    "or": [
      { "==": [{ "var": "quality" }, "480p"] },
      { "in": [{ "var": "source" }, ["CAM", "TS", "TC"]] }
    ]
  },
  "actions": {
    "reject": true,
    "reason": "Below minimum quality threshold"
  },
  "priority": 20,
  "enabled": true
}
```

---

## TypeScript Implementation

### File Structure

```
plugins/content-acquisition/ts/src/
+-- types.ts          # TypeScript interfaces for all data models
+-- config.ts         # Environment variable loading and validation
+-- database.ts       # PostgreSQL schema, CRUD, and query operations
+-- rss-monitor.ts    # RSS feed monitoring with cron scheduling
+-- pipeline.ts       # Multi-stage pipeline orchestrator
+-- server.ts         # Fastify HTTP server with REST API
+-- cli.ts            # Commander.js CLI interface
+-- index.ts          # Module exports and standalone entry point
```

### Key Components

#### ContentAcquisitionConfig (config.ts)

Loads and validates all environment variables. Throws on missing required variables (`DATABASE_URL`, `METADATA_ENRICHMENT_URL`, `TORRENT_MANAGER_URL`, `VPN_MANAGER_URL`).

#### ContentAcquisitionDatabase (database.ts)

Manages all PostgreSQL operations:
- Schema creation (9 tables with indexes)
- Quality profile CRUD
- Subscription CRUD
- RSS feed and feed item management
- Acquisition queue operations
- Subscription matching (case-insensitive)
- Pipeline run tracking

#### RSSFeedMonitor (rss-monitor.ts)

Background service for RSS monitoring:
- Cron-based scheduled checks
- RSS feed parsing via `rss-parser`
- Title parsing with regex (season, episode, quality extraction)
- Duplicate detection
- Subscription matching
- Torrent search integration
- Pipeline triggering

#### PipelineOrchestrator (pipeline.ts)

Multi-stage pipeline execution engine:
- VPN check (hard requirement)
- Torrent submission
- Download polling (30s intervals, 6h max)
- Metadata enrichment (graceful degradation)
- Subtitle fetching (graceful degradation)
- Intelligent retry from failed stage

#### ContentAcquisitionServer (server.ts)

Fastify HTTP server with:
- CORS support
- Rate limiting via `ApiRateLimiter`
- Optional API key authentication via `createAuthHook`
- JSON schema validation on all request bodies
- Pipeline trigger and management endpoints

### Dependencies

| Package | Purpose |
|---------|---------|
| `@nself/plugin-utils` | Shared logging, auth, rate limiting utilities |
| `fastify` | HTTP server framework |
| `@fastify/cors` | Cross-origin resource sharing |
| `@fastify/rate-limit` | Request rate limiting |
| `commander` | CLI framework |
| `rss-parser` | RSS/Atom feed parsing |
| `axios` | HTTP client for sibling plugin communication |
| `node-cron` | Cron-based job scheduling |
| `bullmq` | Redis-backed job queue |
| `ioredis` | Redis client |
| `json-logic-js` | JSON Logic rules engine |
| `pg` | PostgreSQL client |
| `dotenv` | Environment variable loading |
| `uuid` | UUID generation |
| `winston` | Logging |
| `chalk` | Terminal colors |
| `ora` | Terminal spinners |

---

## Examples

### Example 1: Complete Setup and First Subscription

```bash
# 1. Initialize the database
nself plugin content-acquisition init

# 2. Create a quality profile via API
curl -X POST http://localhost:3202/v1/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "HD Preferred",
    "preferredQualities": ["1080p", "720p"],
    "minSeeders": 5
  }'

# 3. Subscribe to a TV show
curl -X POST http://localhost:3202/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "contentType": "tv_show",
    "contentName": "Breaking Bad",
    "qualityProfileId": "<profile-uuid-from-step-2>"
  }'

# 4. Add an RSS feed
curl -X POST http://localhost:3202/v1/feeds \
  -H "Content-Type: application/json" \
  -d '{
    "name": "EZTV TV Releases",
    "url": "https://eztv.re/ezrss.xml",
    "feedType": "tv_shows"
  }'
```

### Example 2: Manual Pipeline Trigger

```typescript
// Trigger a manual download pipeline via API
const response = await fetch('http://localhost:3202/api/pipeline/trigger', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    content_title: 'Breaking Bad S05E16',
    content_type: 'tv_episode',
    magnet_url: 'magnet:?xt=urn:btih:abc123def456...'
  })
});

const { run } = await response.json();
console.log(`Pipeline ${run.id} triggered, status: ${run.status}`);

// Poll for completion
const checkStatus = async (pipelineId: number) => {
  const res = await fetch(`http://localhost:3202/api/pipeline/${pipelineId}`);
  const { run } = await res.json();

  console.log(`Pipeline ${pipelineId}:`);
  console.log(`  Status: ${run.status}`);
  console.log(`  VPN: ${run.vpn_check_status}`);
  console.log(`  Torrent: ${run.torrent_status}`);
  console.log(`  Metadata: ${run.metadata_status}`);
  console.log(`  Subtitles: ${run.subtitle_status}`);

  return run.status;
};

// Poll every 30 seconds
const interval = setInterval(async () => {
  const status = await checkStatus(run.id);
  if (status === 'completed' || status === 'failed') {
    clearInterval(interval);
    console.log(`Pipeline finished: ${status}`);
  }
}, 30000);
```

### Example 3: Monitor Pipeline Runs

```bash
# List all pipeline runs
curl http://localhost:3202/api/pipeline

# List only failed pipelines
curl "http://localhost:3202/api/pipeline?status=failed"

# Get details for a specific pipeline
curl http://localhost:3202/api/pipeline/42

# Retry a failed pipeline
curl -X POST http://localhost:3202/api/pipeline/retry/42
```

### Example 4: SQL Queries for Monitoring

```sql
-- Recent acquisition history
SELECT
  content_name,
  content_type,
  season,
  episode,
  quality,
  status,
  pg_size_pretty(size_bytes) as size,
  created_at
FROM acquisition_history
WHERE source_account_id = 'primary'
ORDER BY created_at DESC
LIMIT 20;

-- Feed health overview
SELECT
  name,
  feed_type,
  enabled,
  last_success_at,
  consecutive_failures,
  last_error
FROM rss_feeds
WHERE source_account_id = 'primary'
ORDER BY consecutive_failures DESC;

-- Queue status summary
SELECT
  status,
  COUNT(*) as count,
  AVG(priority) as avg_priority
FROM acquisition_queue
WHERE source_account_id = 'primary'
GROUP BY status
ORDER BY count DESC;

-- Pipeline success rate
SELECT
  COUNT(*) as total,
  COUNT(*) FILTER (WHERE status = 'completed') as completed,
  COUNT(*) FILTER (WHERE status = 'failed') as failed,
  COUNT(*) FILTER (WHERE status = 'vpn_waiting') as vpn_blocked,
  ROUND(
    100.0 * COUNT(*) FILTER (WHERE status = 'completed') / NULLIF(COUNT(*), 0),
    1
  ) as success_rate_pct
FROM np_ca_pipeline_runs
WHERE created_at >= NOW() - INTERVAL '7 days';

-- Average pipeline stage durations
SELECT
  ROUND(AVG(EXTRACT(EPOCH FROM (vpn_checked_at - detected_at))), 1) as avg_vpn_check_secs,
  ROUND(AVG(EXTRACT(EPOCH FROM (torrent_submitted_at - vpn_checked_at))), 1) as avg_submit_secs,
  ROUND(AVG(EXTRACT(EPOCH FROM (download_completed_at - torrent_submitted_at))) / 60, 1) as avg_download_mins,
  ROUND(AVG(EXTRACT(EPOCH FROM (metadata_enriched_at - download_completed_at))), 1) as avg_metadata_secs,
  ROUND(AVG(EXTRACT(EPOCH FROM (subtitles_fetched_at - metadata_enriched_at))), 1) as avg_subtitle_secs,
  ROUND(AVG(EXTRACT(EPOCH FROM (pipeline_completed_at - detected_at))) / 60, 1) as avg_total_mins
FROM np_ca_pipeline_runs
WHERE status = 'completed'
  AND created_at >= NOW() - INTERVAL '30 days';

-- Subscriptions with recent downloads
SELECT
  s.content_name,
  s.subscription_type,
  s.enabled,
  COUNT(h.id) as total_downloads,
  MAX(h.created_at) as last_download
FROM acquisition_subscriptions s
LEFT JOIN acquisition_history h ON h.content_name ILIKE '%' || s.content_name || '%'
  AND h.source_account_id = s.source_account_id
WHERE s.source_account_id = 'primary'
GROUP BY s.id, s.content_name, s.subscription_type, s.enabled
ORDER BY last_download DESC NULLS LAST;
```

### Example 5: Using the Rules Engine

```bash
# Create a rule that auto-accepts 4K BluRay releases
curl -X POST http://localhost:3202/v1/rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Accept 4K BluRay",
    "conditions": {
      "and": [
        { "==": [{ "var": "quality" }, "2160p"] },
        { "==": [{ "var": "source" }, "BluRay"] },
        { ">=": [{ "var": "seeders" }, 5] }
      ]
    },
    "actions": {
      "accept": true,
      "priority": 10
    },
    "priority": 15,
    "enabled": true
  }'
```

---

## Troubleshooting

### Common Issues

#### Database Connection Refused

**Error:**
```
Error: Connection refused
```

**Solution:**
1. Verify `DATABASE_URL` is set correctly
2. Ensure PostgreSQL is running: `pg_isready`
3. Check that the database exists and the user has access

#### VPN Plugin Unreachable

**Error:**
```
Pipeline paused: VPN is not active
```

**Solution:**
1. Verify `VPN_MANAGER_URL` is correct
2. Ensure the VPN Manager plugin is running
3. Check that the VPN is connected: `curl http://localhost:3301/api/status`
4. Once VPN is verified, retry the pipeline: `curl -X POST http://localhost:3202/api/pipeline/retry/{id}`

#### Torrent Manager Not Responding

**Error:**
```
Torrent submit failed: connect ECONNREFUSED
```

**Solution:**
1. Verify `TORRENT_MANAGER_URL` is correct
2. Ensure the Torrent Manager plugin is running
3. Check connectivity: `curl http://localhost:3100/health`

#### RSS Feed Check Failures

**Problem:** Feed shows increasing `consecutive_failures`.

**Solution:**
1. Check `last_error` on the feed record for the specific error message
2. Verify the feed URL is still valid: `curl -s <feed-url> | head -20`
3. Check if the feed requires authentication or has rate limiting
4. Temporarily disable the feed if the source is down

```sql
-- Check feed health
SELECT name, url, consecutive_failures, last_error
FROM rss_feeds
WHERE consecutive_failures > 0
ORDER BY consecutive_failures DESC;
```

#### No Subscriptions Matching RSS Items

**Problem:** RSS items are all being marked as `rejected` with reason `no_matching_subscription`.

**Solution:**
1. Verify you have active subscriptions: `curl http://localhost:3202/v1/subscriptions`
2. Check that the subscription `content_name` is a substring of the RSS item title (case-insensitive)
3. Ensure the feed type maps to the correct subscription type:
   - `tv_shows`/`anime` feeds require `tv_show` subscriptions
   - `movies` feeds require `movie_collection` subscriptions
   - `music` feeds require `artist` subscriptions

```sql
-- Check rejected items and why
SELECT title, parsed_title, rejection_reason
FROM rss_feed_items
WHERE status = 'rejected'
ORDER BY created_at DESC
LIMIT 20;
```

#### Pipeline Stuck in Downloading State

**Problem:** Pipeline remains in `downloading` status for an extended period.

**Solution:**
1. Check the download status with the Torrent Manager: `curl http://localhost:3100/api/downloads/{downloadId}`
2. The pipeline polls for up to 6 hours before timing out
3. If the torrent is stalled, cancel it in the Torrent Manager and retry the pipeline

#### Metadata or Subtitle Stage Failing

**Problem:** Pipeline completes but metadata or subtitles show as `failed` or `skipped`.

**Solution:**
- `skipped` means the sibling plugin was unreachable (network error). Start the plugin and future runs will succeed.
- `failed` means the plugin returned an error. Check the sibling plugin logs for details.
- Both stages are optional -- they do not prevent the pipeline from completing.

### Debug Mode

Enable verbose logging:

```bash
LOG_LEVEL=debug nself plugin content-acquisition server
```

### Useful Diagnostic Queries

```sql
-- Pipeline runs in the last 24 hours
SELECT id, content_title, status, trigger_type,
       vpn_check_status, torrent_status, metadata_status, subtitle_status,
       error_message, created_at
FROM np_ca_pipeline_runs
WHERE created_at >= NOW() - INTERVAL '24 hours'
ORDER BY created_at DESC;

-- Active queue items
SELECT content_name, content_type, status, priority, attempts, error_message
FROM acquisition_queue
WHERE status NOT IN ('completed', 'failed')
ORDER BY priority DESC, created_at;

-- Feed monitoring summary
SELECT
  name,
  feed_type,
  enabled,
  last_check_at,
  last_success_at,
  consecutive_failures,
  CASE WHEN consecutive_failures > 5 THEN 'UNHEALTHY'
       WHEN consecutive_failures > 0 THEN 'DEGRADED'
       ELSE 'HEALTHY' END as health
FROM rss_feeds
ORDER BY consecutive_failures DESC;
```

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Content-Acquisition
- **Issues**: https://github.com/acamarata/nself-plugins/issues
