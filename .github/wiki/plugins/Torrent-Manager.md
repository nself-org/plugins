# Torrent Manager Plugin

**Version**: 1.0.0 | **Status**: Production Ready | **Port**: 3201 | **Category**: media

Torrent downloading with Transmission/qBittorrent integration, multi-source search, smart matching, seeding policies, and VPN enforcement.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
  - [Required Environment Variables](#required-environment-variables)
  - [Optional Environment Variables](#optional-environment-variables)
- [CLI Commands](#cli-commands)
  - [init](#init)
  - [search](#search)
  - [best-match](#best-match)
  - [add](#add)
  - [list](#list)
  - [stats](#stats)
  - [server](#server)
- [REST API](#rest-api)
  - [Health Endpoints](#health-endpoints)
  - [Client Endpoints](#client-endpoints)
  - [Download Endpoints](#download-endpoints)
  - [Search Endpoints](#search-endpoints)
  - [Statistics Endpoints](#statistics-endpoints)
- [Database Schema](#database-schema)
  - [Tables](#tables)
  - [Views](#views)
  - [Indexes](#indexes)
- [Features](#features)
  - [VPN Enforcement](#vpn-enforcement)
  - [Multi-Client Support](#multi-client-support)
  - [Multi-Source Torrent Search](#multi-source-torrent-search)
  - [Smart Torrent Matching](#smart-torrent-matching)
  - [Torrent Title Parsing](#torrent-title-parsing)
  - [Seeding Policy Management](#seeding-policy-management)
  - [Search Result Caching](#search-result-caching)
  - [Webhook Events](#webhook-events)
- [Architecture](#architecture)
  - [Component Overview](#component-overview)
  - [Source File Reference](#source-file-reference)
- [Inter-Plugin Communication](#inter-plugin-communication)
- [Development](#development)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Torrent Manager Plugin provides a complete torrent downloading solution for the nself ecosystem. It integrates with torrent clients (Transmission and qBittorrent), searches across multiple torrent indexing sites, uses intelligent scoring to find the best matching torrent for a given title, enforces VPN connectivity before starting any downloads, and manages seeding policies to maintain healthy upload ratios.

### Key Capabilities

- **Multi-Client Support**: Supports Transmission and qBittorrent as torrent download clients with a unified adapter interface
- **VPN Enforcement**: Requires an active VPN connection (via the VPN Manager plugin) before allowing any downloads; automatically pauses all active downloads if the VPN disconnects
- **Multi-Source Search**: Searches across 4 torrent indexing sources simultaneously (1337x, YTS, TorrentGalaxy, The Pirate Bay) with parallel execution and per-source timeouts
- **Smart Torrent Matching**: Scores search results on quality, source type, seeders, file size, and release group to automatically select the best torrent for a given title
- **Torrent Title Parsing**: Extracts quality (1080p, 720p, 2160p), source (BluRay, WEB-DL), codec (x264, x265), audio format, release group, season/episode, and language from torrent names
- **Seeding Policy Management**: Configurable ratio and time limits with per-category rules to maintain healthy upload behavior
- **Category Organization**: Categorizes downloads as movie, tv, music, podcast, or other
- **Download Progress Tracking**: Full lifecycle tracking from queued through downloading, seeding, completed, and failed states
- **Search Result Caching**: Caches search results in PostgreSQL with configurable TTL to avoid redundant queries
- **Webhook Events**: Emits events for torrent lifecycle changes (added, started, progress, completed, failed, removed, vpn.disconnected)
- **REST API and CLI**: Both HTTP API and command-line interfaces for all operations

### Dependencies

**npm packages**: `@nself/plugin-utils`, `@ctrl/transmission`, `fastify`, `@fastify/cors`, `@fastify/rate-limit`, `commander`, `axios`, `cheerio`, `puppeteer`, `parse-torrent`, `webtorrent-health`, `node-cron`, `pg`, `dotenv`, `uuid`, `winston`

**System dependencies**: `transmission-daemon`, `qbittorrent-nox`

---

## Quick Start

### Prerequisites

1. A running PostgreSQL database
2. The VPN Manager plugin running (default: `http://localhost:3200`)
3. A torrent client installed and running:
   - **Transmission**: `transmission-daemon` with RPC enabled (default port 9091)
   - **qBittorrent**: `qbittorrent-nox` with Web UI enabled (default port 8080)

### Installation

```bash
# Navigate to plugin directory
cd plugins/torrent-manager/ts

# Install dependencies
npm install

# Build TypeScript
npm run build
```

### Configuration

Create a `.env` file in `plugins/torrent-manager/ts/`:

```bash
# Required
DATABASE_URL=postgresql://user:password@localhost:5432/nself
VPN_MANAGER_URL=http://localhost:3200

# Optional (shown with defaults)
TORRENT_MANAGER_PORT=3201
VPN_REQUIRED=true
DEFAULT_TORRENT_CLIENT=transmission
TRANSMISSION_HOST=localhost
TRANSMISSION_PORT=9091
DOWNLOAD_PATH=/downloads
ENABLED_SOURCES=1337x,yts,torrentgalaxy,tpb
```

### Initialize Database

```bash
npx tsx src/cli.ts init
```

This creates all database tables, indexes, and views, and registers the default torrent client.

### Start the Server

```bash
# Development mode
npm run dev

# Production mode
npm start
```

### Verify Installation

```bash
# Health check
curl http://localhost:3201/health

# Readiness check (VPN + client status)
curl http://localhost:3201/ready

# Search for a torrent
curl -X POST http://localhost:3201/v1/search \
  -H "Content-Type: application/json" \
  -d '{"query": "ubuntu 24.04"}'
```

---

## Configuration

### Required Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string (e.g., `postgresql://user:pass@localhost:5432/nself`) | *none* |
| `VPN_MANAGER_URL` | VPN Manager plugin API URL (e.g., `http://localhost:3200`) | *none* |

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TORRENT_MANAGER_PORT` | HTTP server port | `3201` |
| `VPN_REQUIRED` | Require active VPN before starting downloads (`true`/`false`) | `true` |
| `DEFAULT_TORRENT_CLIENT` | Default torrent client (`transmission` or `qbittorrent`) | `transmission` |
| `TRANSMISSION_HOST` | Transmission RPC host | `localhost` |
| `TRANSMISSION_PORT` | Transmission RPC port | `9091` |
| `TRANSMISSION_USERNAME` | Transmission RPC username | *none* |
| `TRANSMISSION_PASSWORD` | Transmission RPC password | *none* |
| `QBITTORRENT_HOST` | qBittorrent Web UI host | `localhost` |
| `QBITTORRENT_PORT` | qBittorrent Web UI port | `8080` |
| `QBITTORRENT_USERNAME` | qBittorrent Web UI username | *none* |
| `QBITTORRENT_PASSWORD` | qBittorrent Web UI password | *none* |
| `DOWNLOAD_PATH` | Default download directory | `/downloads` |
| `ENABLED_SOURCES` | Comma-separated list of enabled search sources | `1337x,yts,torrentgalaxy,tpb` |
| `SEARCH_ENABLED_SOURCES` | Comma-separated list of enabled search sources (deprecated; use `ENABLED_SOURCES`) | `1337x,yts,torrentgalaxy,tpb` |
| `SEARCH_TIMEOUT_MS` | Search timeout per source in milliseconds | `10000` |
| `SEARCH_CACHE_TTL_SECONDS` | Search cache time-to-live in seconds | `3600` |
| `SEEDING_RATIO_LIMIT` | Stop seeding after reaching this upload/download ratio | `2.0` |
| `SEEDING_TIME_LIMIT_HOURS` | Stop seeding after this many hours (168 = 1 week) | `168` |
| `MAX_ACTIVE_DOWNLOADS` | Maximum concurrent active downloads | `5` |
| `LOG_LEVEL` | Logging level (`debug`, `info`, `warn`, `error`) | `info` |

### Configuration Validation

The plugin validates configuration on startup and rejects invalid values:

- `DATABASE_URL` must be set (fatal error if missing)
- `VPN_MANAGER_URL` must be set (fatal error if missing)
- `TORRENT_MANAGER_PORT` must be between 1024 and 65535
- `MAX_ACTIVE_DOWNLOADS` must be at least 1
- `SEEDING_RATIO_LIMIT` must be non-negative

---

## CLI Commands

All CLI commands are invoked via `npx tsx src/cli.ts <command>` from the `plugins/torrent-manager/ts/` directory.

### init

Initialize the database schema and register the default torrent client.

```bash
npx tsx src/cli.ts init
```

**What it does:**
1. Creates all database tables (`torrent_clients`, `torrent_downloads`, `torrent_files`, `torrent_trackers`, `torrent_search_cache`, `torrent_seeding_policy`, `torrent_stats`)
2. Creates all indexes and views
3. Connects to the configured Transmission client
4. Registers the Transmission client as the default in the database

**Example output:**
```
Initializing torrent manager
  Database initialized
  Transmission client registered
```

---

### search

Search for torrents across all enabled sources.

```bash
npx tsx src/cli.ts search <query> [options]
```

**Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `-t, --type <type>` | Content type (`movie` or `tv`) | `movie` |
| `-q, --quality <quality>` | Quality filter (`1080p`, `720p`, `2160p`, etc.) | *none* |
| `-s, --min-seeders <number>` | Minimum number of seeders | `1` |
| `-l, --limit <number>` | Maximum results to display | `20` |

**Examples:**

```bash
# Search for a movie
npx tsx src/cli.ts search "The Matrix 1999"

# Search for a TV show episode
npx tsx src/cli.ts search "Breaking Bad S01E01" --type tv

# Search with quality filter
npx tsx src/cli.ts search "Interstellar" --quality 1080p --min-seeders 10

# Limit results
npx tsx src/cli.ts search "ubuntu iso" --limit 5
```

**Example output:**
```
Found 15 results:

1. The.Matrix.1999.1080p.BluRay.x264-SPARKS
   Source: 1337x | Seeds: 245 | Size: 2.1 GB
   Quality: 1080p | Type: BluRay

2. The.Matrix.1999.720p.WEB-DL.x264-FGT
   Source: YTS | Seeds: 180 | Size: 950 MB
   Quality: 720p | Type: WEB-DL
```

---

### best-match

Find the single best matching torrent using smart scoring, with an optional flag to download immediately.

```bash
npx tsx src/cli.ts best-match <title> [options]
```

**Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `-y, --year <year>` | Year (for movies) | *none* |
| `-s, --season <number>` | Season number (for TV shows) | *none* |
| `-e, --episode <number>` | Episode number (for TV shows) | *none* |
| `-q, --quality <quality>` | Preferred quality | *none* (defaults to 1080p, 720p) |
| `--download` | Download immediately if a match is found | `false` |

**Examples:**

```bash
# Find best match for a movie
npx tsx src/cli.ts best-match "Inception" --year 2010

# Find best match for a TV episode
npx tsx src/cli.ts best-match "Breaking Bad" --season 1 --episode 1

# Find and immediately download
npx tsx src/cli.ts best-match "Interstellar" --year 2014 --quality 1080p --download
```

**Example output:**
```
Best match found!

Best Match:

Title: Interstellar.2014.1080p.BluRay.x264-SPARKS
Source: 1337x
Quality: 1080p
Type: BluRay
Size: 2.1 GB
Seeders: 312
Score: 87.50/100

Score Breakdown:
  Quality: 25/30
  Source: 25/25
  Seeders: 17.5/20
  Size: 15.0/15
  Group: 10/10
```

When `--download` is specified, the command additionally:
1. Fetches the magnet link (if not already available)
2. Checks VPN status (if `VPN_REQUIRED=true`)
3. Adds the torrent to the configured client
4. Saves the download record to the database

---

### add

Add a torrent download by magnet link.

```bash
npx tsx src/cli.ts add <magnetUri> [options]
```

**Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `-c, --category <category>` | Category (`movie`, `tv`, `music`, `other`) | `other` |
| `-p, --path <path>` | Download path | Value of `DOWNLOAD_PATH` |

**Examples:**

```bash
# Add a torrent with default settings
npx tsx src/cli.ts add "magnet:?xt=urn:btih:abc123..."

# Add with category
npx tsx src/cli.ts add "magnet:?xt=urn:btih:abc123..." --category movie

# Add with custom download path
npx tsx src/cli.ts add "magnet:?xt=urn:btih:abc123..." --path /media/movies
```

**What it does:**
1. Checks VPN status (if `VPN_REQUIRED=true`); refuses to proceed if VPN is not active
2. Connects to the configured torrent client (Transmission)
3. Adds the magnet link to the client
4. Saves the download record to the database
5. Displays the torrent name, info hash, and size

---

### list

List all downloads with optional filters.

```bash
npx tsx src/cli.ts list [options]
```

**Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `-s, --status <status>` | Filter by status (`queued`, `downloading`, `paused`, `completed`, `seeding`, `failed`) | *none* (all) |
| `-c, --category <category>` | Filter by category (`movie`, `tv`, `music`, `podcast`, `other`) | *none* (all) |
| `-l, --limit <number>` | Limit results | `20` |

**Examples:**

```bash
# List all downloads
npx tsx src/cli.ts list

# List only active downloads
npx tsx src/cli.ts list --status downloading

# List completed movies
npx tsx src/cli.ts list --status completed --category movie

# List last 5 downloads
npx tsx src/cli.ts list --limit 5
```

**Example output:**
```
Found 3 downloads:

1. Interstellar.2014.1080p.BluRay.x264-SPARKS
   Status: downloading
   Progress: 45.2%
   Size: 2148.00 MB
   Ratio: 0.15
   ID: a1b2c3d4-e5f6-7890-abcd-ef1234567890

2. Ubuntu.24.04.Desktop.ISO
   Status: completed
   Progress: 100.0%
   Size: 4500.00 MB
   Ratio: 1.85
   ID: b2c3d4e5-f6a7-8901-bcde-f12345678901
```

---

### stats

Show download statistics.

```bash
npx tsx src/cli.ts stats
```

**Example output:**
```
Torrent Manager Statistics:

Total Downloads: 47
Active: 3
Completed: 38
Failed: 2
Seeding: 4
Downloaded: 156.78 GB
Uploaded: 245.12 GB
Overall Ratio: 1.56
```

---

### server

Start the HTTP API server.

```bash
npx tsx src/cli.ts server
```

Starts the Fastify HTTP server on the configured port (default 3201). The server:
1. Initializes the database schema
2. Connects to the default torrent client
3. Registers all API routes
4. Starts VPN monitoring (polls VPN status every 30 seconds)
5. Handles graceful shutdown on SIGINT

```
Starting Torrent Manager Server...

Server running on port 3201
  Health check: http://localhost:3201/health
  API docs: http://localhost:3201/v1
```

---

## REST API

All endpoints are available at `http://localhost:3201` (or the configured `TORRENT_MANAGER_PORT`).

### Endpoint Summary

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (VPN + client status) |
| `GET` | `/v1/clients` | List registered torrent clients |
| `POST` | `/v1/search` | Search for torrents across all sources |
| `POST` | `/v1/search/best-match` | Find best matching torrent with smart scoring |
| `POST` | `/v1/magnet` | Get magnet link for a specific torrent |
| `GET` | `/v1/search/cache` | Get cached search results |
| `POST` | `/v1/downloads` | Add a new torrent download |
| `GET` | `/v1/downloads` | List all downloads |
| `GET` | `/v1/downloads/:id` | Get download details |
| `DELETE` | `/v1/downloads/:id` | Remove a download |
| `POST` | `/v1/downloads/:id/pause` | Pause a download |
| `POST` | `/v1/downloads/:id/resume` | Resume a download |
| `GET` | `/v1/stats` | Get download statistics |
| `GET` | `/v1/seeding` | List seeding torrents |

### Security

The server uses `@nself/plugin-utils` security middleware:
- **Rate Limiting**: 100 requests per minute per client (configurable via `TORRENT_MANAGER_RATE_LIMIT_MAX` and `TORRENT_MANAGER_RATE_LIMIT_WINDOW_MS`)
- **API Key Authentication**: Optional; set `TORRENT_MANAGER_API_KEY` to require `Authorization: Bearer <key>` header on all requests

---

### Health Endpoints

#### GET /health

Returns basic health status.

```bash
curl http://localhost:3201/health
```

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2026-02-12T10:00:00.000Z"
}
```

---

#### GET /ready

Returns readiness status including VPN and torrent client connectivity.

```bash
curl http://localhost:3201/ready
```

**Response:**
```json
{
  "ready": true,
  "vpn_active": true,
  "client_connected": true,
  "timestamp": "2026-02-12T10:00:00.000Z"
}
```

The `ready` field is `true` only when the torrent client is connected. The `vpn_active` field reflects the current VPN status as reported by the VPN Manager plugin.

---

### Client Endpoints

#### GET /v1/clients

List all registered torrent clients.

```bash
curl http://localhost:3201/v1/clients
```

**Response:**
```json
{
  "clients": [
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "source_account_id": "primary",
      "client_type": "transmission",
      "host": "localhost",
      "port": 9091,
      "username": "admin",
      "is_default": true,
      "status": "connected",
      "last_connected_at": "2026-02-12T10:00:00.000Z",
      "last_error": null,
      "created_at": "2026-02-11T08:00:00.000Z",
      "updated_at": "2026-02-12T10:00:00.000Z"
    }
  ]
}
```

---

### Download Endpoints

#### POST /v1/downloads

Add a new torrent download. Requires an active VPN if `VPN_REQUIRED=true`.

```bash
curl -X POST http://localhost:3201/v1/downloads \
  -H "Content-Type: application/json" \
  -d '{
    "magnet_uri": "magnet:?xt=urn:btih:abc123...",
    "category": "movie",
    "download_path": "/media/movies",
    "requested_by": "api"
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `magnet_uri` | string | Yes | Magnet link for the torrent |
| `category` | string | No | Category: `movie`, `tv`, `music`, `podcast`, `other` |
| `download_path` | string | No | Custom download directory |
| `requested_by` | string | No | Identifier for who requested the download (default: `api`) |

**Response (200):**
```json
{
  "success": true,
  "download": {
    "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "source_account_id": "primary",
    "client_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "client_torrent_id": "42",
    "name": "Interstellar.2014.1080p.BluRay.x264-SPARKS",
    "info_hash": "abc123def456...",
    "magnet_uri": "magnet:?xt=urn:btih:abc123...",
    "status": "queued",
    "category": "movie",
    "size_bytes": 0,
    "downloaded_bytes": 0,
    "uploaded_bytes": 0,
    "progress_percent": 0,
    "ratio": 0,
    "download_path": "/media/movies",
    "requested_by": "api",
    "added_at": "2026-02-12T10:05:00.000Z",
    "created_at": "2026-02-12T10:05:00.000Z",
    "updated_at": "2026-02-12T10:05:00.000Z"
  }
}
```

**Error Response (403 - VPN Required):**
```json
{
  "error": "VPN_REQUIRED",
  "message": "VPN must be active before starting downloads"
}
```

---

#### GET /v1/downloads

List all downloads with optional filters.

```bash
# All downloads
curl http://localhost:3201/v1/downloads

# Filter by status
curl "http://localhost:3201/v1/downloads?status=downloading"

# Filter by category with limit
curl "http://localhost:3201/v1/downloads?category=movie&limit=10"
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status (`queued`, `downloading`, `paused`, `completed`, `seeding`, `failed`, `removed`) |
| `category` | string | Filter by category (`movie`, `tv`, `music`, `podcast`, `other`) |
| `limit` | number | Maximum results to return |

**Response:**
```json
{
  "downloads": [...],
  "total": 5
}
```

---

#### GET /v1/downloads/:id

Get detailed information about a specific download.

```bash
curl http://localhost:3201/v1/downloads/b2c3d4e5-f6a7-8901-bcde-f12345678901
```

**Response (200):**
```json
{
  "download": {
    "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "source_account_id": "primary",
    "client_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "client_torrent_id": "42",
    "name": "Interstellar.2014.1080p.BluRay.x264-SPARKS",
    "info_hash": "abc123def456...",
    "magnet_uri": "magnet:?xt=urn:btih:abc123...",
    "status": "downloading",
    "category": "movie",
    "size_bytes": 2254857830,
    "downloaded_bytes": 1127428915,
    "uploaded_bytes": 338228674,
    "progress_percent": 50.00,
    "ratio": 0.30,
    "download_speed_bytes": 5242880,
    "upload_speed_bytes": 1048576,
    "seeders": 45,
    "leechers": 12,
    "peers_connected": 23,
    "download_path": "/media/movies",
    "files_count": 3,
    "stop_at_ratio": 2.00,
    "vpn_ip": "185.65.134.42",
    "vpn_interface": "wg0",
    "requested_by": "api",
    "metadata": {},
    "added_at": "2026-02-12T10:05:00.000Z",
    "started_at": "2026-02-12T10:05:30.000Z",
    "created_at": "2026-02-12T10:05:00.000Z",
    "updated_at": "2026-02-12T10:30:00.000Z"
  }
}
```

**Error Response (404):**
```json
{
  "error": "Download not found"
}
```

---

#### POST /v1/downloads/:id/pause

Pause an active download.

```bash
curl -X POST http://localhost:3201/v1/downloads/b2c3d4e5-f6a7-8901-bcde-f12345678901/pause
```

**Response:**
```json
{
  "success": true
}
```

---

#### POST /v1/downloads/:id/resume

Resume a paused download. Requires an active VPN if `VPN_REQUIRED=true`.

```bash
curl -X POST http://localhost:3201/v1/downloads/b2c3d4e5-f6a7-8901-bcde-f12345678901/resume
```

**Response (200):**
```json
{
  "success": true
}
```

**Error Response (403 - VPN Required):**
```json
{
  "error": "VPN_REQUIRED",
  "message": "VPN must be active to resume downloads"
}
```

---

#### DELETE /v1/downloads/:id

Remove a download. Optionally delete downloaded files.

```bash
# Remove torrent only (keep files)
curl -X DELETE http://localhost:3201/v1/downloads/b2c3d4e5-f6a7-8901-bcde-f12345678901

# Remove torrent and delete files
curl -X DELETE "http://localhost:3201/v1/downloads/b2c3d4e5-f6a7-8901-bcde-f12345678901?delete_files=true"
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `delete_files` | string | Set to `true` to also delete downloaded files from disk |

**Response:**
```json
{
  "success": true
}
```

---

### Search Endpoints

#### POST /v1/search

Search for torrents across all enabled sources in parallel.

```bash
curl -X POST http://localhost:3201/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Inception 2010",
    "type": "movie",
    "quality": "1080p",
    "minSeeders": 10,
    "maxResults": 20
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | Yes | Search query string |
| `type` | string | No | Content type: `movie` or `tv` |
| `quality` | string | No | Quality filter: `2160p`, `1080p`, `720p`, `480p` |
| `minSeeders` | number | No | Minimum number of seeders |
| `maxResults` | number | No | Maximum results (default: 50) |

**Response:**
```json
{
  "query": "Inception 2010",
  "count": 15,
  "results": [
    {
      "title": "Inception.2010.1080p.BluRay.x264-SPARKS",
      "size": "2.1 GB",
      "seeders": 312,
      "leechers": 45,
      "source": "1337x",
      "quality": "1080p",
      "sourceType": "BluRay",
      "releaseGroup": "SPARKS",
      "magnetUri": "magnet:?xt=urn:btih:...",
      "sourceUrl": "https://1337x.to/torrent/..."
    },
    {
      "title": "Inception (2010) [1080p] [YTS]",
      "size": "1.9 GB",
      "seeders": 245,
      "leechers": 30,
      "source": "YTS",
      "quality": "1080p",
      "sourceType": "BluRay",
      "releaseGroup": "YTS",
      "magnetUri": "magnet:?xt=urn:btih:...",
      "sourceUrl": "https://yts.mx/movies/..."
    }
  ]
}
```

---

#### POST /v1/search/best-match

Find the best matching torrent for a given title using the smart scoring algorithm. Ideal for automation where you want the system to pick the best available torrent.

```bash
curl -X POST http://localhost:3201/v1/search/best-match \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Inception",
    "year": 2010,
    "quality": "1080p",
    "minSeeders": 5
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | Yes | Content title |
| `year` | number | No | Release year (for movies) |
| `season` | number | No | Season number (for TV shows) |
| `episode` | number | No | Episode number (for TV shows) |
| `quality` | string | No | Preferred quality (default: 1080p, 720p) |
| `minSeeders` | number | No | Minimum seeders (default: 1) |

**Search query construction:**
- If `season` and `episode` are provided: `"{title} S{season}E{episode}"` (e.g., "Breaking Bad S01E01")
- If `year` is provided: `"{title} {year}"` (e.g., "Inception 2010")
- Otherwise: just the title

**Response (200):**
```json
{
  "match": {
    "title": "Inception.2010.1080p.BluRay.x264-SPARKS",
    "magnetUri": "magnet:?xt=urn:btih:...",
    "size": "2.1 GB",
    "seeders": 312,
    "source": "1337x",
    "score": 87.5,
    "scoreBreakdown": {
      "qualityScore": 25,
      "sourceScore": 25,
      "seederScore": 17.5,
      "sizeScore": 15,
      "releaseGroupScore": 10
    },
    "parsedInfo": {
      "title": "Inception",
      "year": 2010,
      "quality": "1080p",
      "source": "BluRay",
      "codec": "x264",
      "releaseGroup": "SPARKS",
      "type": "movie",
      "language": "English"
    }
  }
}
```

**Error Response (404):**
```json
{
  "error": "No torrents found"
}
```
or
```json
{
  "error": "No suitable match found"
}
```

---

#### POST /v1/magnet

Fetch the magnet link for a torrent from a specific source. Some sources (like 1337x) do not include magnet links in search results and require a follow-up request to the detail page.

```bash
curl -X POST http://localhost:3201/v1/magnet \
  -H "Content-Type: application/json" \
  -d '{
    "source": "1337x",
    "sourceUrl": "https://1337x.to/torrent/12345/..."
  }'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | Yes | Search source name (e.g., `1337x`, `YTS`, `TorrentGalaxy`, `TPB`) |
| `sourceUrl` | string | Yes | Detail page URL from search results |

**Response:**
```json
{
  "magnetUri": "magnet:?xt=urn:btih:abc123..."
}
```

---

#### GET /v1/search/cache

Retrieve cached search results by query hash.

```bash
curl "http://localhost:3201/v1/search/cache?query_hash=abc123def456"
```

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query_hash` | string | Yes | SHA-256 hash of the search query |

**Response:**
```json
{
  "cache": {
    "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
    "query_hash": "abc123def456",
    "query": "Inception 2010",
    "results": [...],
    "results_count": 15,
    "sources_searched": ["1337x", "YTS", "TorrentGalaxy", "TPB"],
    "search_duration_ms": 3500,
    "cached_at": "2026-02-12T10:00:00.000Z",
    "expires_at": "2026-02-12T11:00:00.000Z"
  }
}
```

Returns `{"cache": null}` if no valid cache entry exists.

---

### Statistics Endpoints

#### GET /v1/stats

Get combined statistics from both the database and the torrent client.

```bash
curl http://localhost:3201/v1/stats
```

**Response:**
```json
{
  "database": {
    "total_downloads": 47,
    "active_downloads": 3,
    "completed_downloads": 38,
    "failed_downloads": 2,
    "seeding_torrents": 4,
    "total_downloaded_bytes": 168438398976,
    "total_uploaded_bytes": 263187218432,
    "overall_ratio": 1.56,
    "download_speed_bytes": 15728640,
    "upload_speed_bytes": 5242880,
    "disk_space_used_bytes": 0,
    "disk_space_available_bytes": 0
  },
  "client": {
    "total_torrents": 12,
    "active_torrents": 3,
    "paused_torrents": 1,
    "seeding_torrents": 4,
    "download_speed_bytes": 15728640,
    "upload_speed_bytes": 5242880,
    "downloaded_bytes": 168438398976,
    "uploaded_bytes": 263187218432
  },
  "timestamp": "2026-02-12T10:30:00.000Z"
}
```

---

#### GET /v1/seeding

List all currently seeding torrents.

```bash
curl http://localhost:3201/v1/seeding
```

**Response:**
```json
{
  "seeding": [
    {
      "id": "d4e5f6a7-b890-1234-defg-234567890123",
      "name": "Ubuntu.24.04.Desktop.ISO",
      "status": "seeding",
      "ratio": 2.45,
      "uploaded_bytes": 11265753088,
      "completed_at": "2026-02-10T15:00:00.000Z"
    }
  ],
  "total": 1
}
```

---

## Database Schema

### Tables

The plugin uses 8 database tables. All tables use the `source_account_id` column for multi-app isolation (defaults to `primary`).

---

#### torrent_clients

Registered torrent client connections.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `client_type` | VARCHAR(50) | No | | Client type: `transmission` or `qbittorrent` |
| `host` | VARCHAR(255) | No | | RPC/API host address |
| `port` | INT | No | | RPC/API port number |
| `username` | VARCHAR(255) | Yes | | Authentication username |
| `password_encrypted` | TEXT | Yes | | Encrypted authentication password |
| `is_default` | BOOLEAN | No | `FALSE` | Whether this is the default client |
| `status` | VARCHAR(50) | No | `'disconnected'` | Connection status: `connected`, `disconnected`, `error` |
| `last_connected_at` | TIMESTAMPTZ | Yes | | Timestamp of last successful connection |
| `last_error` | TEXT | Yes | | Last error message (if status is `error`) |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

---

#### torrent_sources

Torrent search source configurations.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `source_name` | VARCHAR(50) | No | | Source name: `1337x`, `thepiratebay`, `rarbg`, `yts`, `eztv`, `kickass` |
| `base_url` | VARCHAR(500) | No | | Base URL for the source |
| `is_active` | BOOLEAN | No | `TRUE` | Whether this source is active for searching |
| `priority` | INT | No | `50` | Search priority (higher = searched first) |
| `requires_proxy` | BOOLEAN | No | `FALSE` | Whether this source requires a proxy |
| `last_success_at` | TIMESTAMPTZ | Yes | | Timestamp of last successful search |
| `last_failure_at` | TIMESTAMPTZ | Yes | | Timestamp of last failed search |
| `failure_count` | INT | No | `0` | Consecutive failure count |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

---

#### torrent_downloads

Main download tracking table. Stores the full lifecycle of each torrent download.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `client_id` | UUID | No | | Foreign key to `torrent_clients(id)` (CASCADE delete) |
| `client_torrent_id` | VARCHAR(255) | No | | Torrent ID within the client (e.g., Transmission's numeric ID) |
| `name` | VARCHAR(500) | No | | Torrent name |
| `info_hash` | VARCHAR(40) | No | | BitTorrent info hash (40 hex characters) |
| `magnet_uri` | TEXT | No | | Full magnet URI |
| `status` | VARCHAR(50) | No | `'queued'` | Download status: `queued`, `downloading`, `paused`, `completed`, `seeding`, `failed`, `removed` |
| `category` | VARCHAR(50) | No | `'other'` | Content category: `movie`, `tv`, `music`, `podcast`, `other` |
| `size_bytes` | BIGINT | No | `0` | Total torrent size in bytes |
| `downloaded_bytes` | BIGINT | No | `0` | Total bytes downloaded |
| `uploaded_bytes` | BIGINT | No | `0` | Total bytes uploaded |
| `progress_percent` | DECIMAL(5,2) | No | `0` | Download progress percentage (0.00 - 100.00) |
| `ratio` | DECIMAL(5,2) | No | `0` | Upload/download ratio |
| `download_speed_bytes` | BIGINT | No | `0` | Current download speed in bytes/second |
| `upload_speed_bytes` | BIGINT | No | `0` | Current upload speed in bytes/second |
| `seeders` | INT | No | `0` | Number of seeders (peers sending to us) |
| `leechers` | INT | No | `0` | Number of leechers (peers getting from us) |
| `peers_connected` | INT | No | `0` | Total peers connected |
| `download_path` | VARCHAR(500) | Yes | | Filesystem path where files are saved |
| `files_count` | INT | No | `0` | Number of files in the torrent |
| `stop_at_ratio` | DECIMAL(5,2) | Yes | | Ratio at which to stop seeding |
| `stop_at_time_hours` | INT | Yes | | Hours after which to stop seeding |
| `vpn_ip` | VARCHAR(50) | Yes | | VPN IP address used during download |
| `vpn_interface` | VARCHAR(50) | Yes | | VPN network interface used (e.g., `wg0`) |
| `error_message` | TEXT | Yes | | Error message if status is `failed` |
| `content_id` | UUID | Yes | | Optional reference to content in another system |
| `requested_by` | VARCHAR(255) | No | | Who requested the download (`cli`, `api`, `automation`, etc.) |
| `metadata` | JSONB | No | `'{}'` | Arbitrary metadata JSON |
| `added_at` | TIMESTAMPTZ | No | `NOW()` | When the torrent was added |
| `started_at` | TIMESTAMPTZ | Yes | | When downloading actually began |
| `completed_at` | TIMESTAMPTZ | Yes | | When download completed (100%) |
| `stopped_at` | TIMESTAMPTZ | Yes | | When seeding/downloading was stopped |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

---

#### torrent_files

Individual files within a torrent.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `download_id` | UUID | No | | Foreign key to `torrent_downloads(id)` (CASCADE delete) |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `file_index` | INT | No | | Zero-based index of the file within the torrent |
| `file_name` | VARCHAR(500) | No | | File name |
| `file_path` | VARCHAR(500) | No | | Full file path on disk |
| `size_bytes` | BIGINT | No | | File size in bytes |
| `downloaded_bytes` | BIGINT | No | `0` | Bytes downloaded for this file |
| `progress_percent` | DECIMAL(5,2) | No | `0` | File download progress (0.00 - 100.00) |
| `priority` | INT | No | `0` | Download priority (higher = downloaded first) |
| `is_selected` | BOOLEAN | No | `TRUE` | Whether this file is selected for download |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

---

#### torrent_trackers

Tracker information for each download.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `download_id` | UUID | No | | Foreign key to `torrent_downloads(id)` (CASCADE delete) |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `tracker_url` | VARCHAR(500) | No | | Tracker announce URL |
| `tier` | INT | No | | Tracker tier (priority level) |
| `status` | VARCHAR(50) | No | | Tracker status (e.g., `working`, `not contacted`, `error`) |
| `seeders` | INT | Yes | | Number of seeders reported by this tracker |
| `leechers` | INT | Yes | | Number of leechers reported by this tracker |
| `last_announce_at` | TIMESTAMPTZ | Yes | | Timestamp of last successful announce |
| `last_scrape_at` | TIMESTAMPTZ | Yes | | Timestamp of last successful scrape |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

---

#### torrent_search_cache

Cached search results to avoid redundant queries.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `query_hash` | VARCHAR(64) | No | | SHA-256 hash of the search query (for lookup) |
| `query` | TEXT | No | | Original search query text |
| `results` | JSONB | No | `'[]'` | Array of search result objects |
| `results_count` | INT | No | `0` | Number of results cached |
| `sources_searched` | VARCHAR(50)[] | No | `'{}'` | Array of source names that were searched |
| `search_duration_ms` | INT | Yes | | How long the search took in milliseconds |
| `cached_at` | TIMESTAMPTZ | No | `NOW()` | When results were cached |
| `expires_at` | TIMESTAMPTZ | No | | Cache expiration timestamp |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |

---

#### torrent_seeding_policy

Configurable seeding policies with category-specific rules.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `policy_name` | VARCHAR(255) | No | | Human-readable policy name |
| `description` | TEXT | Yes | | Policy description |
| `ratio_limit` | DECIMAL(5,2) | Yes | | Stop seeding after reaching this ratio |
| `ratio_action` | VARCHAR(50) | No | `'stop'` | Action when ratio limit reached: `stop`, `pause`, `remove` |
| `time_limit_hours` | INT | Yes | | Stop seeding after this many hours |
| `time_action` | VARCHAR(50) | No | `'stop'` | Action when time limit reached: `stop`, `pause`, `remove` |
| `max_seeding_size_gb` | INT | Yes | | Maximum total seeding size in GB |
| `applies_to_categories` | VARCHAR(50)[] | No | `'{}'` | Categories this policy applies to |
| `priority` | INT | No | `50` | Policy priority (higher = applied first) |
| `is_active` | BOOLEAN | No | `TRUE` | Whether this policy is active |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | `NOW()` | Record last update timestamp |

---

#### torrent_stats

Periodic snapshots of torrent statistics.

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `source_account_id` | VARCHAR(255) | No | `'primary'` | Multi-app isolation column |
| `total_downloads` | INT | No | `0` | Total number of downloads |
| `active_downloads` | INT | No | `0` | Currently active downloads |
| `completed_downloads` | INT | No | `0` | Completed downloads |
| `failed_downloads` | INT | No | `0` | Failed downloads |
| `seeding_torrents` | INT | No | `0` | Currently seeding torrents |
| `total_downloaded_bytes` | BIGINT | No | `0` | Total bytes downloaded all-time |
| `total_uploaded_bytes` | BIGINT | No | `0` | Total bytes uploaded all-time |
| `overall_ratio` | DECIMAL(5,2) | No | `0` | Overall upload/download ratio |
| `download_speed_bytes` | BIGINT | No | `0` | Current aggregate download speed |
| `upload_speed_bytes` | BIGINT | No | `0` | Current aggregate upload speed |
| `disk_space_used_bytes` | BIGINT | No | `0` | Disk space used by downloads |
| `disk_space_available_bytes` | BIGINT | No | `0` | Available disk space |
| `snapshot_at` | TIMESTAMPTZ | No | `NOW()` | When this snapshot was taken |
| `created_at` | TIMESTAMPTZ | No | `NOW()` | Record creation timestamp |

---

### Views

#### torrent_active_downloads

Shows all torrents with status `downloading` or `paused`, ordered by most recently added first.

```sql
CREATE OR REPLACE VIEW torrent_active_downloads AS
SELECT * FROM torrent_downloads
WHERE status IN ('downloading', 'paused')
ORDER BY added_at DESC
```

#### torrent_completed_downloads

Shows all torrents with status `completed`, ordered by completion time descending.

```sql
CREATE OR REPLACE VIEW torrent_completed_downloads AS
SELECT * FROM torrent_downloads
WHERE status = 'completed'
ORDER BY completed_at DESC
```

#### torrent_seeding_torrents

Shows all torrents with status `seeding`, ordered by completion time descending.

```sql
CREATE OR REPLACE VIEW torrent_seeding_torrents AS
SELECT * FROM torrent_downloads
WHERE status = 'seeding'
ORDER BY completed_at DESC
```

---

### Indexes

| Index Name | Table | Column(s) | Description |
|------------|-------|-----------|-------------|
| `idx_torrent_clients_account` | `torrent_clients` | `source_account_id` | Fast client lookup by account |
| `idx_torrent_clients_type` | `torrent_clients` | `client_type` | Fast client lookup by type |
| `idx_torrent_sources_active` | `torrent_sources` | `is_active` (partial: WHERE `is_active = TRUE`) | Fast active source lookup |
| `idx_torrent_downloads_account` | `torrent_downloads` | `source_account_id` | Fast download lookup by account |
| `idx_torrent_downloads_status` | `torrent_downloads` | `status` | Fast download filtering by status |
| `idx_torrent_downloads_info_hash` | `torrent_downloads` | `info_hash` | Fast duplicate detection by info hash |
| `idx_torrent_files_download` | `torrent_files` | `download_id` | Fast file lookup by download |
| `idx_torrent_trackers_download` | `torrent_trackers` | `download_id` | Fast tracker lookup by download |
| `idx_torrent_search_cache_hash` | `torrent_search_cache` | `query_hash` | Fast cache lookup by query hash |
| `idx_torrent_search_cache_expires` | `torrent_search_cache` | `expires_at` | Fast expired cache cleanup |

---

## Features

### VPN Enforcement

The Torrent Manager enforces VPN connectivity to protect user privacy during torrent downloads. This feature integrates with the VPN Manager plugin (running on a separate port, default `http://localhost:3200`).

**How it works:**

1. **Before starting a download**: The plugin queries `GET {VPN_MANAGER_URL}/api/status` to check if the VPN is connected. If the VPN is not active and `VPN_REQUIRED=true`, the download request is rejected with HTTP 403 and error code `VPN_REQUIRED`.

2. **Before resuming a download**: The same VPN check is performed when resuming a paused torrent.

3. **Continuous monitoring**: When the server is running, a background VPN monitor polls the VPN Manager every 30 seconds. If the VPN disconnects:
   - All active downloads (status `downloading`) are automatically paused
   - Each paused download's `error_message` is set to `"VPN disconnected - download paused for safety"`
   - A warning is logged with the count of paused downloads
   - The `vpn.disconnected` webhook event is emitted

4. **VPN status in readiness check**: The `GET /ready` endpoint reports `vpn_active` status so external systems can verify VPN connectivity.

**Disabling VPN enforcement:**

Set `VPN_REQUIRED=false` in your environment to disable VPN checks. This is useful for downloading legal content (Linux ISOs, open-source software, etc.) where VPN protection is not needed.

---

### Multi-Client Support

The plugin supports multiple torrent clients through an adapter pattern:

| Client | Status | Library | Connection Method |
|--------|--------|---------|-------------------|
| **Transmission** | Fully implemented | `@ctrl/transmission` | RPC API (HTTP) |
| **qBittorrent** | Adapter defined | Direct API | Web UI API (HTTP) |

**Transmission Client Features:**
- Connect/disconnect with automatic status tracking
- Add torrents via magnet links with custom download path and category
- Pause, resume, and remove torrents (with optional file deletion)
- List all torrents with status filtering
- Get aggregate statistics (speeds, totals, counts)
- Automatic mapping of Transmission's numeric status codes to human-readable states

**Transmission Status Code Mapping:**

| Transmission Code | Plugin Status |
|-------------------|---------------|
| 0 (Stopped) | `paused` |
| 1 (Check Pending) | `queued` |
| 2 (Checking) | `queued` |
| 3 (Download Pending) | `queued` |
| 4 (Downloading) | `downloading` |
| 5 (Seed Pending) | `queued` |
| 6 (Seeding) | `seeding` |

**Adding a new client adapter:**

1. Create a new class extending `BaseTorrentClient` in `plugins/torrent-manager/ts/src/clients/`
2. Implement all abstract methods: `connect()`, `disconnect()`, `isConnected()`, `addTorrent()`, `getTorrent()`, `listTorrents()`, `pauseTorrent()`, `resumeTorrent()`, `removeTorrent()`, `getStats()`
3. Register the client in the server initialization logic

---

### Multi-Source Torrent Search

The plugin searches across 4 torrent indexing sources simultaneously using the `TorrentSearchAggregator`. Searches are executed in parallel with a 30-second per-source timeout.

| Source | Name | Type | Method | Movies | TV | Notes |
|--------|------|------|--------|--------|-----|-------|
| **1337x** | `1337x` | Web scraping | Cheerio + Axios | Yes | Yes | Mirror fallback (4 domains), lazy magnet fetch |
| **YTS** | `YTS` | REST API | JSON API | Yes | No | Movies only, multiple quality variants per movie |
| **TorrentGalaxy** | `TorrentGalaxy` | Web scraping | Cheerio + Axios | Yes | Yes | Direct magnet links in search results |
| **The Pirate Bay** | `TPB` | Web scraping | Cheerio + Axios | Yes | Yes | Mirror fallback (4 domains), direct magnet links |

**Mirror Fallback:**

The 1337x and TPB searchers maintain lists of mirror domains. If the primary domain fails (timeout, DNS error, HTTP error), the searcher automatically tries the next mirror:

- **1337x mirrors**: `1337x.to`, `1337x.tw`, `1337x.st`, `1337x.is`
- **TPB mirrors**: `thepiratebay.org`, `tpb.party`, `thepiratebay10.org`, `pirateproxy.live`

**Deduplication:**

After aggregating results from all sources, the aggregator deduplicates by normalized title. When duplicates are found, the result with the highest seeder count is kept.

**Result sorting:**

Results are sorted by seeder count in descending order (most seeders first).

**Configuring enabled sources:**

Set `ENABLED_SOURCES` to a comma-separated list of source names to control which sources are searched:

```bash
# Search only YTS and 1337x
ENABLED_SOURCES=yts,1337x

# Search all sources (default)
ENABLED_SOURCES=1337x,yts,torrentgalaxy,tpb
```

---

### Smart Torrent Matching

The `SmartMatcher` scores and ranks search results to automatically select the best torrent for a given title. The scoring system uses a 100-point scale across 5 categories:

#### Score Breakdown (0-100 points)

**Quality Score (0-30 points)**

| Quality | Base Score | With Preferred Bonus |
|---------|-----------|---------------------|
| 2160p / 4K | 30 | 30 (capped) |
| 1080p | 25 | 30 |
| 720p | 20 | 25 |
| 480p | 10 | 15 |
| 360p | 5 | 10 |

**Source Score (0-25 points)**

| Source | Base Score | With Preferred Bonus |
|--------|-----------|---------------------|
| BluRay | 25 | 25 (capped) |
| WEB-DL | 20 | 25 |
| WEBRip | 18 | 23 |
| HDTV | 15 | 20 |
| DVD | 10 | 15 |

**Seeder Score (0-20 points)**

Uses logarithmic scoring with diminishing returns:
- 0 seeders: 0 points
- 1-10 seeders: 5-10 points
- 10-100 seeders: 10-15 points
- 100-1000 seeders: 15-20 points
- 1000+ seeders: 20 points (maximum)

**Size Score (0-15 points)**

Scored based on expected file sizes for the quality level:

| Quality | Min (GB) | Ideal (GB) | Max (GB) |
|---------|----------|------------|----------|
| 2160p / 4K | 15 | 40 | 100 |
| 1080p | 1.5 | 8 | 25 |
| 720p | 0.7 | 4 | 15 |
| 480p | 0.3 | 1.5 | 5 |

- Below minimum: 0 points (likely poor quality)
- Min to ideal: 15 points
- Ideal to max: 5-15 points (linearly decreasing)
- Above max: 5 points (bloated)

**Release Group Score (0-10 points)**

| Group | Score |
|-------|-------|
| Trusted groups (YIFY, YTS, RARBG, FGT, EVO, SPARKS, NTb, TOMMY) | 10 |
| Preferred groups (from options) | 10 |
| Unknown groups | 5 (neutral) |
| No group detected | 5 (neutral) |

#### Matching Pipeline

1. **Title Matching**: Filters results by title similarity using Levenshtein distance (minimum 80% similarity required). Also matches year (+/- 1 year tolerance), season, and episode numbers.

2. **Hard Filters**: Removes results that fail mandatory criteria:
   - Below minimum seeder count
   - Outside size constraints (min/max GB)
   - Bad source types (CAM, TS, TC, R5, SCREENER)
   - Excluded languages
   - Excluded keywords (e.g., KORSUB, HC, BLURRED)

3. **Scoring**: Each remaining result is scored on the 5 categories above.

4. **Selection**: The result with the highest total score is returned.

---

### Torrent Title Parsing

The `TorrentTitleParser` extracts structured metadata from torrent file names using regex patterns based on Sonarr, Radarr, and Jackett conventions.

**Extracted fields:**

| Field | Examples | Description |
|-------|----------|-------------|
| `title` | "The Matrix", "Breaking Bad" | Clean content title |
| `year` | 1999, 2010 | Release year (1900-2099) |
| `season` | 1, 2, 3 | TV show season number |
| `episode` | 1, 2, 15 | TV show episode number |
| `quality` | `2160p`, `1080p`, `720p`, `480p`, `360p` | Video resolution |
| `source` | `BluRay`, `WEB-DL`, `WEBRip`, `HDTV`, `DVD`, `CAM` | Media source |
| `codec` | `x265`, `x264`, `XviD`, `DivX` | Video codec |
| `audio` | `DTS-HD MA`, `DTS`, `DD5.1`, `AC3`, `AAC`, `MP3`, `FLAC` | Audio format |
| `releaseGroup` | `SPARKS`, `YIFY`, `FGT` | Scene/P2P release group |
| `language` | `English`, `French`, `German`, `Spanish`, etc. | Content language |
| `isProper` | `true`/`false` | Whether this is a PROPER release |
| `isRepack` | `true`/`false` | Whether this is a REPACK release |
| `type` | `movie`, `tv`, `unknown` | Content type |

**TV show pattern recognition:**

| Pattern | Example |
|---------|---------|
| `S01E01` | Standard season/episode |
| `S01 E01` | With space |
| `1x01` | Alternative format |
| `Season 1 Episode 1` | Full text |

---

### Seeding Policy Management

Seeding policies define rules for how long and how much torrents should seed. Policies can be configured per-category and support multiple actions.

**Policy options:**

| Setting | Description | Actions |
|---------|-------------|---------|
| Ratio limit | Stop seeding after reaching upload/download ratio | `stop`, `pause`, `remove` |
| Time limit | Stop seeding after a number of hours | `stop`, `pause`, `remove` |
| Max seeding size | Maximum total GB being seeded | Applied globally |
| Category rules | Different policies for movies, TV, music, etc. | Per-category override |
| Priority | Higher priority policies are evaluated first | 1-100 scale |

**Default global settings (via environment variables):**

- `SEEDING_RATIO_LIMIT=2.0` - Stop seeding after 2:1 upload ratio
- `SEEDING_TIME_LIMIT_HOURS=168` - Stop seeding after 1 week (168 hours)

---

### Search Result Caching

Search results are cached in the `torrent_search_cache` table to avoid redundant queries to torrent indexing sites.

- **Cache key**: SHA-256 hash of the search query
- **Default TTL**: 3600 seconds (1 hour), configurable via `SEARCH_CACHE_TTL_SECONDS`
- **Cache includes**: Full result objects, source list, search duration
- **Expiration**: Cached entries are only returned if `expires_at > NOW()`

---

### Webhook Events

The plugin emits webhook events for torrent lifecycle changes. These can be consumed by other plugins or external systems.

| Event | Description | Trigger |
|-------|-------------|---------|
| `torrent.added` | A new torrent has been added | Download created via API or CLI |
| `torrent.started` | A torrent has started downloading | Download transitions to `downloading` state |
| `torrent.progress` | Download progress update | Periodic progress polling |
| `torrent.completed` | A torrent has finished downloading | Download reaches 100% |
| `torrent.failed` | A torrent download has failed | Error during download |
| `torrent.removed` | A torrent has been removed | Download deleted via API or CLI |
| `vpn.disconnected` | VPN connection lost | VPN monitor detects disconnection |

---

## Architecture

### Component Overview

```
+---------------------------------------------------------------+
|                    Torrent Manager Plugin                       |
|                                                                |
|  +----------+  +-----------+  +----------+  +------------+    |
|  |   CLI    |  |  Server   |  |  Search  |  |   Smart    |    |
|  | (cli.ts) |  |(server.ts)|  |Aggregator|  |  Matcher   |    |
|  +----+-----+  +----+------+  +----+-----+  +-----+------+    |
|       |             |              |               |           |
|       +------+------+------+------+-------+-------+           |
|              |             |              |                    |
|     +--------+-----+ +----+------+  +----+-------+           |
|     | VPN Checker   | | Database  |  | Searchers  |           |
|     | (vpn-checker) | |(database) |  | (1337x,    |           |
|     +-------+-------+ +----------+  |  YTS, TG,  |           |
|             |                        |  TPB)      |           |
|     +-------+-------+               +-----+------+           |
|     | VPN Manager   |                     |                   |
|     | Plugin (3200) |               +-----+------+            |
|     +---------------+               | Title      |            |
|                                     | Parser     |            |
|  +------+--------+                  +------------+            |
|  | Torrent Client |                                           |
|  | Adapters       |                                           |
|  |  +------------+|                                           |
|  |  |Transmission||                                           |
|  |  +------------+|                                           |
|  |  |qBittorrent ||                                           |
|  |  +------------+|                                           |
|  +----------------+                                           |
+---------------------------------------------------------------+
```

### Source File Reference

| File | Path | Responsibility |
|------|------|----------------|
| `types.ts` | `ts/src/types.ts` | All TypeScript interfaces for configuration, clients, downloads, search, files, trackers, policies, stats, VPN, webhooks, and API responses |
| `config.ts` | `ts/src/config.ts` | Environment variable loading, validation, and singleton export |
| `database.ts` | `ts/src/database.ts` | PostgreSQL schema initialization, all CRUD operations, statistics queries |
| `server.ts` | `ts/src/server.ts` | Fastify HTTP server with all REST API route handlers |
| `cli.ts` | `ts/src/cli.ts` | Commander.js CLI with all user commands |
| `vpn-checker.ts` | `ts/src/vpn-checker.ts` | VPN status checking, waiting, monitoring, and event subscription |
| `index.ts` | `ts/src/index.ts` | Module exports and standalone server bootstrap |
| `clients/base.ts` | `ts/src/clients/base.ts` | Abstract base class for torrent client adapters |
| `clients/transmission.ts` | `ts/src/clients/transmission.ts` | Transmission RPC client adapter using `@ctrl/transmission` |
| `search/base-searcher.ts` | `ts/src/search/base-searcher.ts` | Abstract base class for search providers, shared interfaces |
| `search/aggregator.ts` | `ts/src/search/aggregator.ts` | Parallel multi-source search, deduplication, magnet link fetching |
| `search/searchers/1337x-searcher.ts` | `ts/src/search/searchers/1337x-searcher.ts` | 1337x web scraper with mirror fallback and lazy magnet fetch |
| `search/searchers/yts-searcher.ts` | `ts/src/search/searchers/yts-searcher.ts` | YTS JSON API searcher (movies only) |
| `search/searchers/torrentgalaxy-searcher.ts` | `ts/src/search/searchers/torrentgalaxy-searcher.ts` | TorrentGalaxy web scraper |
| `search/searchers/tpb-searcher.ts` | `ts/src/search/searchers/tpb-searcher.ts` | The Pirate Bay web scraper with mirror fallback |
| `matching/smart-matcher.ts` | `ts/src/matching/smart-matcher.ts` | Multi-criteria scoring and selection algorithm |
| `parsers/title-parser.ts` | `ts/src/parsers/title-parser.ts` | Torrent title metadata extraction using regex patterns |

---

## Inter-Plugin Communication

The Torrent Manager is designed to work with other nself plugins. The primary integration point is with the VPN Manager plugin.

### VPN Manager Integration

The Torrent Manager depends on the VPN Manager plugin for VPN status. It communicates via HTTP:

```
Torrent Manager (port 3201) --> GET /api/status --> VPN Manager (port 3200)
```

**VPN Status Response (expected format):**
```json
{
  "connected": true,
  "provider": "nordvpn",
  "server": "nl928",
  "vpn_ip": "185.65.134.42",
  "interface": "wg0"
}
```

### Example: Automation Plugin Using Torrent Manager

```typescript
// In another plugin - search and download a movie
async function downloadMovie(title: string, year: number) {
  // 1. Find best match
  const searchResponse = await fetch('http://localhost:3201/v1/search/best-match', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, year, quality: '1080p' })
  });

  const { match } = await searchResponse.json();

  if (!match) {
    throw new Error('No torrent found');
  }

  // 2. Start download
  const downloadResponse = await fetch('http://localhost:3201/v1/downloads', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      magnet_uri: match.magnetUri,
      category: 'movie',
      requested_by: 'automation'
    })
  });

  const { download } = await downloadResponse.json();

  // 3. Poll for completion
  while (true) {
    const statusResponse = await fetch(`http://localhost:3201/v1/downloads/${download.id}`);
    const { download: status } = await statusResponse.json();

    if (status.status === 'completed') {
      return status.download_path;
    }

    if (status.status === 'failed') {
      throw new Error(status.error_message);
    }

    await new Promise(resolve => setTimeout(resolve, 10000));
  }
}
```

---

## Development

### Build Commands

```bash
# Navigate to plugin directory
cd plugins/torrent-manager/ts

# Install dependencies
npm install

# TypeScript compilation
npm run build

# Type checking without output
npm run typecheck

# Watch mode for development
npm run watch

# Development server (uses tsx)
npm run dev

# Production server
npm start
```

### Adding a New Search Source

1. Create a new file in `ts/src/search/searchers/` extending `BaseTorrentSearcher`
2. Implement the `search()` method returning `TorrentSearchResult[]`
3. Implement `getMagnetLink()` if the source does not include magnet links in search results
4. Register the searcher in `ts/src/search/aggregator.ts` within the `allSearchers` array
5. Add the source name to `ENABLED_SOURCES` configuration

### Adding a New Torrent Client

1. Create a new file in `ts/src/clients/` extending `BaseTorrentClient`
2. Implement all abstract methods
3. Add the client type to the `TorrentClientType` union in `types.ts`
4. Add client initialization logic in `server.ts`

### Testing

```bash
# Type check
npm run typecheck

# Build
npm run build

# Run CLI
npx tsx src/cli.ts --help

# Start dev server
npm run dev

# Manual test: health
curl http://localhost:3201/health

# Manual test: search
curl -X POST http://localhost:3201/v1/search \
  -H "Content-Type: application/json" \
  -d '{"query": "ubuntu 24.04"}'

# Manual test: best match
curl -X POST http://localhost:3201/v1/search/best-match \
  -H "Content-Type: application/json" \
  -d '{"title": "Inception", "year": 2010}'
```

---

## Troubleshooting

### VPN is not active - downloads rejected

**Symptom:** `POST /v1/downloads` returns 403 with `VPN_REQUIRED` error.

**Solutions:**
1. Start the VPN Manager plugin and connect to a VPN:
   ```bash
   # Start VPN Manager
   cd plugins/vpn/ts && npm run dev

   # Connect to VPN
   npx tsx src/cli.ts connect nordvpn --p2p
   ```
2. Verify VPN Manager is reachable:
   ```bash
   curl http://localhost:3200/api/status
   ```
3. To bypass VPN requirement for legal content:
   ```bash
   VPN_REQUIRED=false
   ```

---

### Cannot connect to Transmission

**Symptom:** `init` command shows "Could not connect to Transmission" or server logs "Failed to connect to default torrent client".

**Solutions:**
1. Verify Transmission daemon is running:
   ```bash
   systemctl status transmission-daemon
   # or
   ps aux | grep transmission
   ```
2. Verify RPC is enabled in Transmission settings:
   ```bash
   # Check settings.json (usually in /etc/transmission-daemon/ or ~/.config/transmission-daemon/)
   # Ensure:
   # "rpc-enabled": true,
   # "rpc-whitelist-enabled": false (or whitelist includes your IP)
   ```
3. Test the connection manually:
   ```bash
   curl http://localhost:9091/transmission/rpc
   # Should return a 409 with X-Transmission-Session-Id header
   ```
4. Check environment variables match your Transmission configuration:
   ```bash
   TRANSMISSION_HOST=localhost
   TRANSMISSION_PORT=9091
   TRANSMISSION_USERNAME=admin
   TRANSMISSION_PASSWORD=password
   ```

---

### Search returns no results

**Symptom:** `POST /v1/search` returns `{"query": "...", "count": 0, "results": []}`.

**Solutions:**
1. Check which sources are enabled:
   ```bash
   echo $ENABLED_SOURCES
   # Default: 1337x,yts,torrentgalaxy,tpb
   ```
2. Some sources may be blocked in your region. Check server logs for timeout or connection errors:
   ```
   WARN: 1337x mirror https://1337x.to failed: connect ETIMEDOUT
   WARN: TPB mirror https://thepiratebay.org failed: connect ECONNREFUSED
   ```
3. Try with fewer or different sources:
   ```bash
   ENABLED_SOURCES=yts
   ```
4. YTS only indexes movies. If searching for TV shows, ensure other sources are enabled:
   ```bash
   ENABLED_SOURCES=1337x,tpb,torrentgalaxy
   ```
5. Increase the search timeout:
   ```bash
   SEARCH_TIMEOUT_MS=30000
   ```

---

### Database connection failed

**Symptom:** `Error: Connection refused` or `ECONNREFUSED` on startup.

**Solutions:**
1. Verify PostgreSQL is running:
   ```bash
   systemctl status postgresql
   ```
2. Test the connection string:
   ```bash
   psql $DATABASE_URL -c "SELECT 1"
   ```
3. Verify the `DATABASE_URL` format:
   ```
   postgresql://username:password@host:port/database
   ```

---

### Downloads stuck in "queued" state

**Symptom:** Torrents are added but never start downloading.

**Solutions:**
1. Check the torrent client directly (Transmission Web UI at `http://localhost:9091/transmission/web/`)
2. Verify the torrent has seeders available
3. Check `MAX_ACTIVE_DOWNLOADS` is not exceeded:
   ```bash
   MAX_ACTIVE_DOWNLOADS=5
   ```
4. Check for VPN disconnection pausing downloads:
   ```bash
   curl http://localhost:3201/v1/downloads?status=paused
   ```

---

### Configuration validation errors on startup

**Symptom:** `Error: Invalid configuration` or specific validation messages.

**Solutions:**
1. Both `DATABASE_URL` and `VPN_MANAGER_URL` must be set
2. Port must be between 1024-65535
3. `MAX_ACTIVE_DOWNLOADS` must be at least 1
4. `SEEDING_RATIO_LIMIT` must be non-negative (0 or greater)

---

### Rate limiting errors

**Symptom:** HTTP 429 responses from the API.

**Solutions:**
1. Default rate limit is 100 requests per minute
2. Adjust with environment variables:
   ```bash
   TORRENT_MANAGER_RATE_LIMIT_MAX=200
   TORRENT_MANAGER_RATE_LIMIT_WINDOW_MS=60000
   ```

---

## Links

- **GitHub**: https://github.com/acamarata/nself-plugins
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **VPN Manager Plugin**: [VPN Plugin Wiki](VPN)
- **TMDB Plugin**: [TMDB Plugin Wiki](TMDB) (for content metadata)

---

**Version**: 1.0.0 | **Last Updated**: February 12, 2026 | **Status**: Production Ready
