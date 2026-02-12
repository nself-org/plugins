# Subtitle Manager Plugin

Subtitle search, download, and sync verification via the OpenSubtitles API with local file storage and download tracking.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [OpenSubtitles Client](#opensubtitles-client)
- [Features](#features)
- [Multi-App Isolation](#multi-app-isolation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Subtitle Manager plugin provides a complete subtitle management solution for media applications. It integrates with the [OpenSubtitles REST API](https://opensubtitles.stoplight.io/docs/opensubtitles-api) to search for, download, and locally cache subtitle files. Every search result and download is tracked in PostgreSQL, enabling efficient caching, analytics, and multi-tenant subtitle management.

### Key Features

- **Text-Based Search**: Search the OpenSubtitles catalog by movie or TV show title, with multi-language filtering
- **Hash-Based Search**: Match subtitles precisely using file hash and byte size for exact media file identification
- **Subtitle File Download**: Download subtitle files from OpenSubtitles and persist them to local disk storage
- **Download Caching**: Automatically detect previously downloaded subtitles to avoid redundant API calls and disk writes
- **Download Tracking**: Every download is recorded in PostgreSQL with metadata including file size, hash, language, and source
- **Sync Score Tracking**: Track subtitle quality and synchronization accuracy scores for each subtitle entry
- **Multi-App Isolation**: Full multi-tenant support via `source_account_id` column on all tables
- **REST API**: Complete HTTP API for integration with other services, frontends, and automation pipelines
- **CLI Interface**: Command-line tools for quick subtitle searching and server management
- **Rate Limiting and Auth**: Built-in API rate limiting and optional API key authentication via `@nself/plugin-utils`

### Use Cases

- Media server subtitle management (Plex, Jellyfin, Emby integrations)
- Streaming platform subtitle provisioning
- Automated subtitle acquisition pipelines
- Media library enrichment and organization
- Content localization workflows
- Subtitle quality verification and tracking

### Architecture

```
                        +-----------------------+
                        |   CLI (cli.ts)        |
                        |   - init              |
                        |   - search            |
                        |   - server            |
                        +-----------+-----------+
                                    |
                        +-----------v-----------+
                        |  Server (server.ts)   |
                        |   Fastify HTTP API    |
                        |   Port 3204           |
                        +-----------+-----------+
                                    |
                    +---------------+---------------+
                    |                               |
        +-----------v-----------+    +--------------v--------------+
        |   Database            |    |   OpenSubtitles Client      |
        |   (database.ts)       |    |   (opensubtitles-client.ts) |
        |   PostgreSQL          |    |   api.opensubtitles.com     |
        +-----------------------+    +-----------------------------+
```

---

## Quick Start

```bash
# Install the plugin
nself plugin install subtitle-manager

# Configure environment (minimal .env)
cat > .env <<EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
OPENSUBTITLES_API_KEY=your_opensubtitles_api_key_here
EOF

# Initialize database schema
nself plugin subtitle-manager init

# Search for subtitles from the CLI
nself plugin subtitle-manager search "The Matrix"

# Start the HTTP API server
nself plugin subtitle-manager server
```

The server will start on port 3204 by default. You can then use the REST API to search, download, and manage subtitles programmatically.

---

## Configuration

### Required Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | **Yes** | - | PostgreSQL connection string (e.g., `postgresql://user:pass@localhost:5432/nself`) |

### Optional Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENSUBTITLES_API_KEY` | No | - | OpenSubtitles REST API key. Required for search and download operations. Without this key, search and download endpoints will return empty results. |
| `SUBTITLE_STORAGE_PATH` | No | `/tmp/subtitles` | Local filesystem directory where downloaded subtitle files are stored. Organized as `{path}/{source_account_id}/{media_id}/{language}.srt`. |
| `SUBTITLE_MANAGER_PORT` | No | `3204` | HTTP server listen port |
| `LOG_LEVEL` | No | `info` | Logging verbosity level (`debug`, `info`, `warn`, `error`) |

### Security Environment Variables

The server uses `@nself/plugin-utils` security configuration, loaded with the `SUBTITLE_MANAGER` prefix:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SUBTITLE_MANAGER_API_KEY` | No | - | API key for authenticating requests. When set, all requests must include a valid `Authorization` header. |
| `SUBTITLE_MANAGER_RATE_LIMIT_MAX` | No | `100` | Maximum number of API requests allowed per rate limit window |
| `SUBTITLE_MANAGER_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window duration in milliseconds (default: 1 minute) |

### Example .env File

```bash
# Database (required)
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# OpenSubtitles API (get from https://www.opensubtitles.com/en/consumers)
OPENSUBTITLES_API_KEY=your_api_key_here

# Storage
SUBTITLE_STORAGE_PATH=/data/subtitles

# Server
SUBTITLE_MANAGER_PORT=3204
LOG_LEVEL=info

# Security (optional)
SUBTITLE_MANAGER_API_KEY=my-secret-api-key
SUBTITLE_MANAGER_RATE_LIMIT_MAX=100
SUBTITLE_MANAGER_RATE_LIMIT_WINDOW_MS=60000
```

### Getting an OpenSubtitles API Key

1. Create an account at [https://www.opensubtitles.com/](https://www.opensubtitles.com/)
2. Navigate to the [Consumers page](https://www.opensubtitles.com/en/consumers)
3. Register a new API consumer application
4. Copy your API key
5. Set it as `OPENSUBTITLES_API_KEY` in your `.env` file

**Note:** The OpenSubtitles API has usage limits depending on your plan. The free tier allows a limited number of downloads per day. See the [OpenSubtitles API documentation](https://opensubtitles.stoplight.io/docs/opensubtitles-api) for current rate limits.

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (create tables and indexes) |
| `search <query>` | Search OpenSubtitles for subtitles by text query |
| `server` | Start the HTTP API server |

### Initialize Database

```bash
nself plugin subtitle-manager init
```

Creates all required database tables (`np_subtmgr_subtitles`, `np_subtmgr_downloads`) and their associated indexes. This command is idempotent and safe to run multiple times -- it uses `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`.

**Output:**
```
- Initializing subtitle manager
  ✓ Database initialized
```

**On failure:**
```
✗ Initialization failed
Error: Connection refused (check DATABASE_URL)
```

### Search for Subtitles

```bash
nself plugin subtitle-manager search <query> [options]
```

Searches the OpenSubtitles API by text query and displays the top 10 results in the terminal.

**Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `-l, --language <lang>` | ISO 639-1 language code to filter results | `en` |

**Examples:**

```bash
# Search for English subtitles
nself plugin subtitle-manager search "The Matrix"

# Search for Spanish subtitles
nself plugin subtitle-manager search "The Matrix" --language es

# Search for French subtitles for a TV show
nself plugin subtitle-manager search "Breaking Bad S01E01" -l fr
```

**Output:**
```
⠋ Searching for subtitles: The Matrix

Found 25 subtitles:

1. The Matrix
   Language: en
   Format: srt

2. The Matrix
   Language: en
   Format: sub

3. The Matrix Reloaded
   Language: en
   Format: srt

...
```

**No results:**
```
No subtitles found
```

### Start Server

```bash
nself plugin subtitle-manager server
```

Starts the Fastify HTTP API server. The server initializes the database schema on startup, registers all API routes, and listens on the configured port (default: 3204).

**Output:**
```
Starting Subtitle Manager Server...

✓ Server running on port 3204
```

The server handles graceful shutdown on `SIGINT` (Ctrl+C), closing the HTTP server and database connections cleanly.

---

## REST API

### Base URL

```
http://localhost:3204
```

### Authentication

When `SUBTITLE_MANAGER_API_KEY` is configured, all requests must include an `Authorization` header:

```
Authorization: Bearer your-api-key-here
```

### Multi-App Context

All data-modifying endpoints respect the `X-App-Name` header for multi-tenant isolation:

```
X-App-Name: my-app-id
```

If omitted, the default value `primary` is used.

### Endpoints Summary

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/v1/subtitles` | Query locally stored subtitle records |
| `GET` | `/v1/downloads` | List downloaded subtitle files |
| `GET` | `/v1/stats` | Get subtitle statistics |
| `POST` | `/v1/search` | Search OpenSubtitles by text query |
| `POST` | `/v1/search/hash` | Search OpenSubtitles by file hash |
| `POST` | `/v1/download` | Download a subtitle file |
| `DELETE` | `/v1/downloads/:id` | Delete a download record |

---

### GET /health

Health check endpoint. Does not require authentication.

**Request:**
```http
GET /health
```

**Response (200):**
```json
{
  "status": "ok",
  "plugin": "subtitle-manager",
  "version": "1.0.0"
}
```

---

### GET /v1/subtitles

Query locally stored subtitle records from the `np_subtmgr_subtitles` table. Results are ordered by sync score (highest first), then by most recently updated.

**Request:**
```http
GET /v1/subtitles?media_id=tt0133093&language=en
Headers:
  X-App-Name: primary
```

**Query Parameters:**

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `media_id` | **Yes** | - | The media identifier to search for (e.g., IMDb ID, internal ID) |
| `language` | No | `en` | ISO 639-1 language code |

**Response (200):**
```json
{
  "subtitles": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "source_account_id": "primary",
      "media_id": "tt0133093",
      "media_type": "movie",
      "language": "en",
      "file_path": "/data/subtitles/primary/tt0133093/en.srt",
      "source": "opensubtitles",
      "sync_score": 9.50,
      "created_at": "2026-02-10T08:30:00.000Z",
      "updated_at": "2026-02-10T08:30:00.000Z"
    }
  ]
}
```

**Error (missing media_id):**
```json
{
  "error": "media_id query parameter is required"
}
```

---

### GET /v1/downloads

List downloaded subtitle files from the `np_subtmgr_downloads` table, with pagination. Results are ordered by most recently created first.

**Request:**
```http
GET /v1/downloads?limit=25&offset=0
Headers:
  X-App-Name: primary
```

**Query Parameters:**

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `limit` | No | `50` | Maximum number of results to return (pagination) |
| `offset` | No | `0` | Number of results to skip (pagination) |

**Response (200):**
```json
{
  "downloads": [
    {
      "id": "661f9510-f39c-52e5-b827-557766551111",
      "source_account_id": "primary",
      "subtitle_id": null,
      "media_id": "tt0133093",
      "media_type": "movie",
      "media_title": "The Matrix",
      "language": "en",
      "file_path": "/data/subtitles/primary/tt0133093/en.srt",
      "file_size_bytes": 54832,
      "opensubtitles_file_id": 1956478925,
      "file_hash": null,
      "sync_score": null,
      "source": "opensubtitles",
      "created_at": "2026-02-10T09:15:00.000Z",
      "updated_at": "2026-02-10T09:15:00.000Z"
    }
  ],
  "total": 1
}
```

---

### GET /v1/stats

Get aggregate statistics about subtitles and downloads for the current app context.

**Request:**
```http
GET /v1/stats
Headers:
  X-App-Name: primary
```

**Response (200):**
```json
{
  "stats": {
    "total_subtitles": 142,
    "total_downloads": 87,
    "languages": [
      { "language": "en", "count": 65 },
      { "language": "es", "count": 12 },
      { "language": "fr", "count": 10 }
    ],
    "sources": [
      { "source": "opensubtitles", "count": 85 },
      { "source": "manual", "count": 2 }
    ]
  }
}
```

---

### POST /v1/search

Search the OpenSubtitles API by text query. This endpoint proxies to the OpenSubtitles REST API and returns raw search results. Results are not stored in the database.

**Request:**
```http
POST /v1/search
Headers:
  Content-Type: application/json

Body:
{
  "query": "The Matrix",
  "languages": ["en", "es"]
}
```

**Body Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | **Yes** | - | Search text (movie title, TV show name, etc.). Minimum 1 character. |
| `languages` | string[] | No | `["en"]` | Array of ISO 639-1 language codes to filter results. Each code must be 2-5 characters. |

**Response (200):**
```json
{
  "results": [
    {
      "id": "1956478925",
      "type": "subtitle",
      "attributes": {
        "subtitle_id": "1956478925",
        "language": "en",
        "download_count": 2345678,
        "new_download_count": 12345,
        "hearing_impaired": false,
        "hd": true,
        "fps": 23.976,
        "votes": 42,
        "points": 10,
        "ratings": 8.5,
        "from_trusted": true,
        "foreign_parts_only": false,
        "ai_translated": false,
        "machine_translated": false,
        "upload_date": "2023-06-15T12:00:00Z",
        "release": "The.Matrix.1999.1080p.BluRay.x264-GROUP",
        "comments": "Perfect sync for BluRay release",
        "format": "srt",
        "feature_details": {
          "feature_id": 603,
          "feature_type": "Movie",
          "year": 1999,
          "title": "The Matrix",
          "movie_name": "The Matrix",
          "imdb_id": 133093,
          "tmdb_id": 603
        },
        "files": [
          {
            "file_id": 1956478925,
            "cd_number": 1,
            "file_name": "The.Matrix.1999.1080p.BluRay.x264.srt"
          }
        ]
      }
    }
  ],
  "count": 25
}
```

**Note:** The `results` array contains raw OpenSubtitles API response objects. The exact structure of each result depends on the OpenSubtitles API version. The `file_id` within `attributes.files` is used with the download endpoint.

---

### POST /v1/search/hash

Search OpenSubtitles by file hash and byte size. This provides more accurate subtitle matching than text search because it identifies the exact media file release.

**Request:**
```http
POST /v1/search/hash
Headers:
  Content-Type: application/json

Body:
{
  "moviehash": "8e245d9679d31e12",
  "moviebytesize": 733589504,
  "languages": ["en"]
}
```

**Body Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `moviehash` | string | **Yes** | - | OpenSubtitles-compatible file hash (64-bit). Minimum 1 character. |
| `moviebytesize` | number | **Yes** | - | File size in bytes. Must be >= 1. |
| `languages` | string[] | No | `["en"]` | Array of ISO 639-1 language codes. Each code must be 2-5 characters. |

**Response (200):**
```json
{
  "results": [
    {
      "id": "1956478925",
      "type": "subtitle",
      "attributes": {
        "language": "en",
        "download_count": 2345678,
        "format": "srt",
        "feature_details": {
          "title": "The Matrix",
          "year": 1999,
          "imdb_id": 133093,
          "tmdb_id": 603
        },
        "files": [
          {
            "file_id": 1956478925,
            "cd_number": 1,
            "file_name": "The.Matrix.1999.1080p.BluRay.x264.srt"
          }
        ]
      }
    }
  ],
  "count": 3
}
```

**Generating a File Hash:**

OpenSubtitles uses a specific hashing algorithm. The hash is computed from the first and last 64KB of the file combined with the file size. See the [OpenSubtitles hash documentation](https://trac.opensubtitles.org/projects/opensubtitles/wiki/HashSourceCodes) for implementation details in various languages.

---

### POST /v1/download

Download a subtitle file from OpenSubtitles and save it to local disk storage. If the subtitle for the given `media_id` and `language` has already been downloaded, the cached version is returned instead.

**Request:**
```http
POST /v1/download
Headers:
  Content-Type: application/json
  X-App-Name: primary

Body:
{
  "file_id": 1956478925,
  "media_id": "tt0133093",
  "media_type": "movie",
  "media_title": "The Matrix",
  "language": "en"
}
```

**Body Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `file_id` | number | **Yes** | - | OpenSubtitles file ID (from search results `attributes.files[].file_id`). Must be >= 1. |
| `media_id` | string | **Yes** | - | Your internal media identifier (e.g., IMDb ID, UUID, database primary key). Minimum 1 character. |
| `media_type` | string | No | `movie` | Media type. Must be `movie` or `tv_episode`. |
| `media_title` | string | No | - | Human-readable media title for reference |
| `language` | string | No | `en` | ISO 639-1 language code (2-5 characters) |

**Response (200) -- Fresh Download:**
```json
{
  "success": true,
  "download": {
    "id": "661f9510-f39c-52e5-b827-557766551111",
    "source_account_id": "primary",
    "subtitle_id": null,
    "media_id": "tt0133093",
    "media_type": "movie",
    "media_title": "The Matrix",
    "language": "en",
    "file_path": "/data/subtitles/primary/tt0133093/en.srt",
    "file_size_bytes": 54832,
    "opensubtitles_file_id": 1956478925,
    "file_hash": null,
    "sync_score": null,
    "source": "opensubtitles",
    "created_at": "2026-02-10T09:15:00.000Z",
    "updated_at": "2026-02-10T09:15:00.000Z"
  },
  "source": "opensubtitles"
}
```

**Response (200) -- Cached (Previously Downloaded):**
```json
{
  "success": true,
  "download": {
    "id": "661f9510-f39c-52e5-b827-557766551111",
    "source_account_id": "primary",
    "media_id": "tt0133093",
    "media_type": "movie",
    "media_title": "The Matrix",
    "language": "en",
    "file_path": "/data/subtitles/primary/tt0133093/en.srt",
    "file_size_bytes": 54832,
    "opensubtitles_file_id": 1956478925,
    "source": "opensubtitles",
    "created_at": "2026-02-10T09:15:00.000Z",
    "updated_at": "2026-02-10T09:15:00.000Z"
  },
  "source": "cache"
}
```

The `source` field indicates whether the subtitle was freshly downloaded (`"opensubtitles"`) or served from a previous download (`"cache"`).

**Response (404) -- Download Failed:**
```json
{
  "error": "Subtitle not found or download failed"
}
```

**File Storage Layout:**

Downloaded files are stored on disk with the following directory structure:

```
{SUBTITLE_STORAGE_PATH}/
  {source_account_id}/
    {media_id}/
      {language}.srt
```

Example:
```
/data/subtitles/primary/tt0133093/en.srt
/data/subtitles/primary/tt0133093/es.srt
/data/subtitles/app2/tt0133093/en.srt
```

---

### DELETE /v1/downloads/:id

Delete a download record from the database by its UUID. Note: this removes the database record only -- it does not delete the subtitle file from disk.

**Request:**
```http
DELETE /v1/downloads/661f9510-f39c-52e5-b827-557766551111
```

**Response (200):**
```json
{
  "success": true
}
```

**Response (404):**
```json
{
  "error": "Download not found"
}
```

---

## Database Schema

The plugin creates 2 tables, both prefixed with `np_subtmgr_` as required by the nself naming convention.

### np_subtmgr_subtitles

Stores locally cataloged subtitle records. This table tracks which subtitles are available for which media items, along with quality metadata. Records are primarily created via the `upsertSubtitle` database method.

```sql
CREATE TABLE IF NOT EXISTS np_subtmgr_subtitles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  media_id VARCHAR(255) NOT NULL,
  media_type VARCHAR(50) NOT NULL,
  language VARCHAR(10) NOT NULL,
  file_path TEXT NOT NULL,
  source VARCHAR(50) NOT NULL,
  sync_score DECIMAL(5,2),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | NO | `gen_random_uuid()` | Unique identifier (auto-generated) |
| `source_account_id` | VARCHAR(128) | NO | `'primary'` | Multi-app isolation key. Identifies which application or tenant owns this record. |
| `media_id` | VARCHAR(255) | NO | - | External media identifier (e.g., IMDb ID `tt0133093`, TMDB ID, or internal UUID) |
| `media_type` | VARCHAR(50) | NO | - | Type of media content. Typical values: `movie`, `tv_episode` |
| `language` | VARCHAR(10) | NO | - | ISO 639-1 language code (e.g., `en`, `es`, `fr`, `de`, `ar`) |
| `file_path` | TEXT | NO | - | Absolute filesystem path to the downloaded subtitle file |
| `source` | VARCHAR(50) | NO | - | Origin of the subtitle (e.g., `opensubtitles`, `manual`, `embedded`) |
| `sync_score` | DECIMAL(5,2) | YES | `NULL` | Subtitle synchronization quality score. Higher values indicate better sync with the media file. Range depends on scoring system used. |
| `created_at` | TIMESTAMPTZ | NO | `NOW()` | Timestamp when the record was first created |
| `updated_at` | TIMESTAMPTZ | NO | `NOW()` | Timestamp when the record was last modified |

**Indexes:**

| Index Name | Columns | Description |
|------------|---------|-------------|
| `idx_np_subtmgr_subtitles_media` | `(media_id, language)` | Fast lookup of subtitles by media item and language |
| `idx_np_subtmgr_subtitles_account` | `(source_account_id)` | Multi-app isolation queries |

### np_subtmgr_downloads

Tracks every subtitle download operation. Each row represents a subtitle file that was downloaded from an external source (primarily OpenSubtitles) and saved to local storage.

```sql
CREATE TABLE IF NOT EXISTS np_subtmgr_downloads (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  subtitle_id UUID REFERENCES np_subtmgr_subtitles(id) ON DELETE CASCADE,
  media_id VARCHAR(255) NOT NULL,
  media_type VARCHAR(50) NOT NULL,
  media_title VARCHAR(255),
  language VARCHAR(10) NOT NULL,
  file_path TEXT NOT NULL,
  file_size_bytes BIGINT,
  opensubtitles_file_id INT,
  file_hash VARCHAR(64),
  sync_score DECIMAL(5,2),
  source VARCHAR(50) NOT NULL DEFAULT 'opensubtitles',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | NO | `gen_random_uuid()` | Unique download record identifier (auto-generated) |
| `source_account_id` | VARCHAR(128) | NO | `'primary'` | Multi-app isolation key |
| `subtitle_id` | UUID | YES | `NULL` | Foreign key reference to `np_subtmgr_subtitles.id`. Cascades on delete. Links the download to its parent subtitle catalog entry, if applicable. |
| `media_id` | VARCHAR(255) | NO | - | External media identifier matching the subtitle |
| `media_type` | VARCHAR(50) | NO | - | Type of media (`movie`, `tv_episode`) |
| `media_title` | VARCHAR(255) | YES | `NULL` | Human-readable title of the media (e.g., "The Matrix") |
| `language` | VARCHAR(10) | NO | - | ISO 639-1 language code of the downloaded subtitle |
| `file_path` | TEXT | NO | - | Absolute filesystem path where the subtitle file is stored |
| `file_size_bytes` | BIGINT | YES | `NULL` | Size of the downloaded subtitle file in bytes |
| `opensubtitles_file_id` | INT | YES | `NULL` | The OpenSubtitles file ID used to request this download |
| `file_hash` | VARCHAR(64) | YES | `NULL` | File hash of the downloaded subtitle for integrity verification |
| `sync_score` | DECIMAL(5,2) | YES | `NULL` | Synchronization quality score for this specific download |
| `source` | VARCHAR(50) | NO | `'opensubtitles'` | Source from which the subtitle was obtained |
| `created_at` | TIMESTAMPTZ | NO | `NOW()` | Timestamp when the download was performed |
| `updated_at` | TIMESTAMPTZ | NO | `NOW()` | Timestamp of last record update |

**Indexes:**

| Index Name | Columns | Description |
|------------|---------|-------------|
| `idx_np_subtmgr_downloads_media` | `(media_id, language)` | Fast lookup for cache-hit detection (check if already downloaded) |
| `idx_np_subtmgr_downloads_account` | `(source_account_id)` | Multi-app isolation queries |

**Foreign Keys:**

| Constraint | Column | References | On Delete |
|------------|--------|------------|-----------|
| FK subtitle_id | `subtitle_id` | `np_subtmgr_subtitles(id)` | CASCADE |

### Entity Relationship

```
np_subtmgr_subtitles  1 ──── * np_subtmgr_downloads
       (id)          ←──────── (subtitle_id)
```

A single subtitle catalog entry can have multiple download records (e.g., different users downloading the same subtitle, or re-downloads after cache clearing).

---

## OpenSubtitles Client

The `OpenSubtitlesClient` class (`opensubtitles-client.ts`) handles all communication with the OpenSubtitles REST API.

### API Base URL

```
https://api.opensubtitles.com/api/v1
```

### Authentication

All requests include the API key in the `Api-Key` header:

```
Api-Key: your_opensubtitles_api_key
```

### Methods

| Method | Description |
|--------|-------------|
| `searchByQuery(query, languages)` | Text-based subtitle search |
| `searchByHash(moviehash, moviebytesize, languages)` | Hash-based subtitle search |
| `downloadSubtitle(fileId)` | Download a subtitle file as a Buffer |

### Graceful Degradation

If `OPENSUBTITLES_API_KEY` is not configured:
- `searchByQuery` returns an empty array `[]`
- `searchByHash` returns an empty array `[]`
- `downloadSubtitle` returns `null`

A warning is logged but no error is thrown. This allows the plugin to operate in a local-only mode where subtitles are managed manually without OpenSubtitles integration.

### Error Handling

All API errors are caught and logged. The methods return empty arrays or `null` on failure rather than throwing exceptions, ensuring the REST API server remains stable even during OpenSubtitles API outages or rate limiting.

---

## Features

### Text-Based Search

Search for subtitles using a text query (movie title, TV show name, etc.). The query is sent to the OpenSubtitles API and results include metadata like download count, ratings, format, and linked file IDs for downloading.

Results can be filtered by one or more language codes. The OpenSubtitles API supports ISO 639-1 codes (`en`, `es`, `fr`, `de`, `ar`, `pt`, `ja`, `ko`, etc.).

### Hash-Based Search

For more accurate subtitle matching, provide the media file's hash and byte size. OpenSubtitles uses a specific hashing algorithm based on the first and last 64KB of the file, which enables exact release matching. This method returns subtitles that are known to sync perfectly with the specific file.

### Download and Cache

The download workflow includes automatic caching:

1. Check if a subtitle for the given `media_id` + `language` + `source_account_id` combination already exists in the `np_subtmgr_downloads` table
2. If found, return the existing record with `source: "cache"` -- no API call or disk write needed
3. If not found, download the subtitle from OpenSubtitles
4. Save the file to disk at `{SUBTITLE_STORAGE_PATH}/{source_account_id}/{media_id}/{language}.srt`
5. Insert a download record into `np_subtmgr_downloads`
6. Return the new record with `source: "opensubtitles"`

### Sync Score Tracking

Both the `np_subtmgr_subtitles` and `np_subtmgr_downloads` tables support a `sync_score` column. This decimal field (up to 5 digits, 2 decimal places) can be used to track subtitle synchronization quality -- how well the subtitle timing matches the media file.

Higher scores indicate better synchronization. The score can be populated:
- Automatically by a sync verification tool
- Manually by users rating subtitle quality
- From OpenSubtitles metadata (ratings, votes)

The `GET /v1/subtitles` endpoint orders results by `sync_score DESC`, ensuring the highest-quality subtitles appear first.

### Statistics

The stats endpoint provides aggregated metrics:
- **Total subtitles**: Count of records in `np_subtmgr_subtitles`
- **Total downloads**: Count of records in `np_subtmgr_downloads`
- **Languages breakdown**: Download counts grouped by language
- **Sources breakdown**: Download counts grouped by source

---

## Multi-App Isolation

The plugin fully supports multi-tenant operation through the `source_account_id` column, which is present on both database tables.

### How It Works

- Every database query filters by `source_account_id`
- The app context is extracted from each HTTP request using `getAppContext(request)` from `@nself/plugin-utils`
- Downloaded files are stored in separate directories per account: `{storage_path}/{source_account_id}/{media_id}/`
- Statistics are scoped to the requesting app

### Setting the App Context

Include the `X-App-Name` header in your API requests:

```http
GET /v1/subtitles?media_id=tt0133093
X-App-Name: streaming-app-1
```

If the header is omitted, the default value `primary` is used.

### Data Isolation Guarantees

- App `streaming-app-1` cannot see downloads from `streaming-app-2`
- Cache lookups are scoped: the same `media_id` + `language` can have separate downloads for each app
- Statistics (language counts, source counts) are computed per-app
- File storage is physically separated into per-app directories

---

## Examples

### Example 1: Search and Download Workflow

Complete workflow from search to downloaded subtitle file:

```bash
# Step 1: Search for subtitles
curl -X POST http://localhost:3204/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "The Matrix 1999",
    "languages": ["en"]
  }'

# Step 2: Pick a file_id from the results (e.g., from attributes.files[0].file_id)
# Step 3: Download the subtitle
curl -X POST http://localhost:3204/v1/download \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "file_id": 1956478925,
    "media_id": "tt0133093",
    "media_type": "movie",
    "media_title": "The Matrix",
    "language": "en"
  }'

# Response:
# {
#   "success": true,
#   "download": {
#     "id": "...",
#     "file_path": "/tmp/subtitles/primary/tt0133093/en.srt",
#     "file_size_bytes": 54832,
#     ...
#   },
#   "source": "opensubtitles"
# }

# Step 4: Verify the file exists on disk
ls -la /tmp/subtitles/primary/tt0133093/en.srt
```

### Example 2: Hash-Based Precise Matching

Use file hashing for exact subtitle matching:

```bash
# Compute the hash for your media file (example using Python)
# python3 -c "import struct; ..."
# This gives you the hash and file size

curl -X POST http://localhost:3204/v1/search/hash \
  -H "Content-Type: application/json" \
  -d '{
    "moviehash": "8e245d9679d31e12",
    "moviebytesize": 733589504,
    "languages": ["en", "es"]
  }'
```

### Example 3: Multi-Language Download

Download subtitles in multiple languages for the same media:

```bash
# Download English subtitles
curl -X POST http://localhost:3204/v1/download \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "file_id": 1956478925,
    "media_id": "tt0133093",
    "media_type": "movie",
    "media_title": "The Matrix",
    "language": "en"
  }'

# Download Spanish subtitles
curl -X POST http://localhost:3204/v1/download \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "file_id": 2045893176,
    "media_id": "tt0133093",
    "media_type": "movie",
    "media_title": "The Matrix",
    "language": "es"
  }'

# Resulting file structure:
# /tmp/subtitles/primary/tt0133093/en.srt
# /tmp/subtitles/primary/tt0133093/es.srt
```

### Example 4: CLI Quick Search

```bash
# English subtitles for a movie
nself plugin subtitle-manager search "Inception"

# French subtitles for a TV episode
nself plugin subtitle-manager search "Breaking Bad S05E14" --language fr

# German subtitles
nself plugin subtitle-manager search "Dark" -l de
```

### Example 5: Check Statistics

```bash
curl http://localhost:3204/v1/stats \
  -H "X-App-Name: primary"

# Response:
# {
#   "stats": {
#     "total_subtitles": 142,
#     "total_downloads": 87,
#     "languages": [
#       { "language": "en", "count": 65 },
#       { "language": "es", "count": 12 }
#     ],
#     "sources": [
#       { "source": "opensubtitles", "count": 85 },
#       { "source": "manual", "count": 2 }
#     ]
#   }
# }
```

### Example 6: SQL Queries

Query the database directly for advanced reporting:

```sql
-- Find all English subtitles with high sync scores
SELECT
  s.media_id,
  s.sync_score,
  s.file_path,
  s.source,
  s.updated_at
FROM np_subtmgr_subtitles s
WHERE s.source_account_id = 'primary'
  AND s.language = 'en'
  AND s.sync_score >= 8.0
ORDER BY s.sync_score DESC;

-- Download volume by language
SELECT
  language,
  COUNT(*) AS download_count,
  SUM(file_size_bytes) AS total_bytes,
  ROUND(AVG(file_size_bytes)) AS avg_bytes
FROM np_subtmgr_downloads
WHERE source_account_id = 'primary'
GROUP BY language
ORDER BY download_count DESC;

-- Recent downloads with media titles
SELECT
  d.media_title,
  d.language,
  d.file_size_bytes,
  d.source,
  d.created_at
FROM np_subtmgr_downloads d
WHERE d.source_account_id = 'primary'
ORDER BY d.created_at DESC
LIMIT 20;

-- Media items with subtitles in multiple languages
SELECT
  media_id,
  COUNT(DISTINCT language) AS language_count,
  ARRAY_AGG(DISTINCT language) AS languages
FROM np_subtmgr_downloads
WHERE source_account_id = 'primary'
GROUP BY media_id
HAVING COUNT(DISTINCT language) > 1
ORDER BY language_count DESC;
```

### Example 7: Multi-Tenant Operation

Serve subtitles for multiple applications from a single instance:

```bash
# App 1 downloads a subtitle
curl -X POST http://localhost:3204/v1/download \
  -H "Content-Type: application/json" \
  -H "X-App-Name: streaming-service-a" \
  -d '{
    "file_id": 1956478925,
    "media_id": "tt0133093",
    "media_type": "movie",
    "language": "en"
  }'
# Stored at: /data/subtitles/streaming-service-a/tt0133093/en.srt

# App 2 downloads the same subtitle independently
curl -X POST http://localhost:3204/v1/download \
  -H "Content-Type: application/json" \
  -H "X-App-Name: streaming-service-b" \
  -d '{
    "file_id": 1956478925,
    "media_id": "tt0133093",
    "media_type": "movie",
    "language": "en"
  }'
# Stored at: /data/subtitles/streaming-service-b/tt0133093/en.srt

# Each app sees only its own downloads
curl http://localhost:3204/v1/downloads \
  -H "X-App-Name: streaming-service-a"
# Returns only streaming-service-a downloads

curl http://localhost:3204/v1/downloads \
  -H "X-App-Name: streaming-service-b"
# Returns only streaming-service-b downloads
```

---

## Troubleshooting

### Issue: "OpenSubtitles API key not configured"

**Cause:** The `OPENSUBTITLES_API_KEY` environment variable is not set or is empty.

**Solution:**
1. Obtain an API key from [OpenSubtitles](https://www.opensubtitles.com/en/consumers)
2. Set the environment variable:
   ```bash
   export OPENSUBTITLES_API_KEY=your_key_here
   ```
   Or add it to your `.env` file:
   ```
   OPENSUBTITLES_API_KEY=your_key_here
   ```

**Note:** The plugin will still start and serve local data without this key. Only search and download operations from OpenSubtitles will be affected.

### Issue: "Subtitle not found or download failed" (404)

**Cause:** The OpenSubtitles API could not find or serve the requested `file_id`.

**Solutions:**
- Verify the `file_id` is correct (obtained from a recent search result)
- The subtitle may have been removed from OpenSubtitles
- Your OpenSubtitles API key may have reached its daily download limit
- Try searching again to get a fresh file ID

### Issue: "DATABASE_URL is required"

**Cause:** The `DATABASE_URL` environment variable is missing.

**Solution:**
```bash
export DATABASE_URL=postgresql://user:password@localhost:5432/nself
```

### Issue: Connection Refused (Database)

**Cause:** PostgreSQL is not running or the connection string is incorrect.

**Solutions:**
1. Verify PostgreSQL is running:
   ```bash
   pg_isready
   ```
2. Check your connection string format:
   ```
   postgresql://username:password@hostname:port/database
   ```
3. Ensure the database exists:
   ```bash
   createdb nself
   ```

### Issue: Empty Search Results

**Cause:** Multiple possible reasons.

**Solutions:**
- Verify `OPENSUBTITLES_API_KEY` is set and valid
- Try a simpler, more common search query
- Check that the requested language has available subtitles
- OpenSubtitles may be experiencing downtime -- check the logs for error details:
  ```bash
  LOG_LEVEL=debug nself plugin subtitle-manager server
  ```

### Issue: Permission Denied Writing Subtitle Files

**Cause:** The process does not have write permissions to `SUBTITLE_STORAGE_PATH`.

**Solutions:**
1. Check current path:
   ```bash
   echo $SUBTITLE_STORAGE_PATH
   ```
2. Create the directory with proper permissions:
   ```bash
   mkdir -p /data/subtitles
   chmod 755 /data/subtitles
   ```
3. Or use a path with write access:
   ```bash
   export SUBTITLE_STORAGE_PATH=/tmp/subtitles
   ```

### Issue: Rate Limiting (429 Too Many Requests)

**Cause:** Exceeded the OpenSubtitles API rate limit or the plugin's internal rate limit.

**Solutions:**
- For OpenSubtitles rate limiting:
  - Reduce the frequency of search/download requests
  - Upgrade your OpenSubtitles API plan for higher limits
  - The free tier has strict daily download limits
- For plugin rate limiting:
  - Increase `SUBTITLE_MANAGER_RATE_LIMIT_MAX` (default: 100)
  - Increase `SUBTITLE_MANAGER_RATE_LIMIT_WINDOW_MS` (default: 60000ms)

### Issue: Cached Download Returns Stale Data

**Cause:** The cache lookup found a previous download for the same `media_id` + `language` combination.

**Solution:** Delete the existing download record and retry:
```bash
# Find the download ID
curl http://localhost:3204/v1/downloads \
  -H "X-App-Name: primary"

# Delete the cached record
curl -X DELETE http://localhost:3204/v1/downloads/661f9510-f39c-52e5-b827-557766551111

# Re-download
curl -X POST http://localhost:3204/v1/download \
  -H "Content-Type: application/json" \
  -d '{
    "file_id": 1956478925,
    "media_id": "tt0133093",
    "language": "en"
  }'
```

### Issue: Server Won't Start on Port 3204

**Cause:** Another process is already using port 3204.

**Solutions:**
1. Check what is using the port:
   ```bash
   lsof -i :3204
   ```
2. Use a different port:
   ```bash
   export SUBTITLE_MANAGER_PORT=3205
   ```
3. Kill the existing process:
   ```bash
   kill $(lsof -t -i :3204)
   ```

---

## Technical Details

### Runtime Requirements

- **Node.js**: >= 18.0.0
- **PostgreSQL**: Any version supporting `gen_random_uuid()` (PostgreSQL 13+)
- **Dependencies**: See `package.json` for full dependency list

### Plugin Metadata

| Field | Value |
|-------|-------|
| **Name** | subtitle-manager |
| **Package** | @nself/plugin-subtitle-manager |
| **Version** | 1.0.0 |
| **Category** | media |
| **Subcategory** | subtitles |
| **Port** | 3204 |
| **Language** | TypeScript |
| **Runtime** | Node.js |
| **Min nself Version** | 0.4.8 |
| **License** | MIT |
| **Author** | nself |
| **Tables** | 2 (`np_subtmgr_subtitles`, `np_subtmgr_downloads`) |
| **Views** | 0 |
| **Webhook Events** | 0 |

### Source Files

| File | Purpose |
|------|---------|
| `types.ts` | TypeScript interfaces for config, records, inputs, and stats |
| `config.ts` | Environment variable loading and validation |
| `opensubtitles-client.ts` | OpenSubtitles REST API client with search and download methods |
| `database.ts` | PostgreSQL schema initialization, CRUD operations, and statistics |
| `server.ts` | Fastify HTTP server with all REST API routes |
| `cli.ts` | Commander.js CLI with init, search, and server commands |
| `index.ts` | Module exports and standalone server entry point |

---

**Plugin Version:** 1.0.0
**Last Updated:** February 12, 2026
**Author:** nself
**License:** MIT
