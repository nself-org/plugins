# Media Metadata Plugin

TMDB media metadata enrichment with automatic matching and confidence scoring for movies and TV shows.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Lookup & Matching](#lookup--matching)
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Media Metadata plugin provides comprehensive TMDB (The Movie Database) integration for enriching media libraries with professional metadata. It supports:

- **7 Database Tables** - Complete TMDB data storage
- **Automatic Matching** - Title/year-based lookup with confidence scoring
- **Manual Queue** - Review ambiguous matches
- **Full REST API** - Query and enrich media via HTTP
- **CLI Interface** - Command-line tools for all operations
- **Confidence Threshold** - Configurable auto-match threshold (default 70%)

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Movies | Full movie metadata with cast, crew, ratings | `tmdb_movies` |
| TV Shows | Complete TV show information | `tmdb_tv_shows` |
| TV Seasons | Season-level metadata | `tmdb_tv_seasons` |
| TV Episodes | Episode details with guest stars | `tmdb_tv_episodes` |
| Genres | Movie and TV genre lists | `tmdb_genres` |
| Match Queue | Items pending manual review | `tmdb_match_queue` |
| Webhook Events | Event processing log | `tmdb_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install media-metadata

# Configure environment
echo "TMDB_API_KEY=your_tmdb_api_key" >> .env
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Initialize database schema
nself plugin media-metadata init

# Sync genre list
nself plugin media-metadata sync-genres

# Start API server
nself plugin media-metadata server --port 3202
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `TMDB_API_KEY` | Yes | - | TMDB API key (from themoviedb.org) |
| `TMDB_API_READ_ACCESS_TOKEN` | No | - | TMDB read access token (v4 API) |
| `TMDB_PLUGIN_PORT` | No | `3202` | HTTP server port |
| `TMDB_IMAGE_BASE_URL` | No | `https://image.tmdb.org/t/p` | TMDB image CDN base URL |
| `TMDB_DEFAULT_LANGUAGE` | No | `en-US` | Default language for metadata |
| `TMDB_AUTO_ENRICH` | No | `false` | Auto-enrich on lookup |
| `TMDB_CONFIDENCE_THRESHOLD` | No | `0.70` | Minimum confidence for auto-match (0-1) |
| `TMDB_CACHE_TTL_DAYS` | No | `30` | Cache metadata for N days |
| `TMDB_RATE_LIMIT_MAX` | No | `100` | Max API requests per window |
| `TMDB_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Getting a TMDB API Key

1. Create a free account at https://www.themoviedb.org
2. Go to Settings > API
3. Request an API key (choose "Developer" for free access)
4. Copy the "API Key (v3 auth)" value

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# TMDB
TMDB_API_KEY=your_tmdb_api_key_here
TMDB_DEFAULT_LANGUAGE=en-US
TMDB_CONFIDENCE_THRESHOLD=0.70
TMDB_CACHE_TTL_DAYS=30

# Server
TMDB_PLUGIN_PORT=3202
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin media-metadata init

# Check plugin status
nself plugin media-metadata status

# View statistics
nself plugin media-metadata stats
```

### Server

```bash
# Start API server (default port 3202)
nself plugin media-metadata server

# Custom port
nself plugin media-metadata server --port 8080
```

### Search

```bash
# Search for movies
nself plugin media-metadata search "The Matrix"

# Search for TV shows
nself plugin media-metadata search "Breaking Bad" --type tv

# Search with year filter
nself plugin media-metadata search "The Batman" --year 2022
```

### Lookup (with Confidence Scoring)

```bash
# Lookup movie by title
nself plugin media-metadata lookup "Inception" --type movie

# Lookup with year for better accuracy
nself plugin media-metadata lookup "The Batman" --year 2022

# Lookup TV show
nself plugin media-metadata lookup "Stranger Things" --type tv --year 2016
```

### Enrich (Fetch and Store)

```bash
# Enrich a movie
nself plugin media-metadata enrich "The Matrix" --type movie

# Enrich with year
nself plugin media-metadata enrich "Dune" --year 2021

# Force re-fetch even if cached
nself plugin media-metadata enrich "Blade Runner 2049" --force
```

### Genre Management

```bash
# Sync genre list from TMDB
nself plugin media-metadata sync-genres
```

### Match Queue

```bash
# List pending matches
nself plugin media-metadata match-queue

# Filter by status
nself plugin media-metadata match-queue --status pending
nself plugin media-metadata match-queue --status manual_review

# Limit results
nself plugin media-metadata match-queue --limit 10
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
  "plugin": "media-metadata",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "media-metadata",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "media-metadata",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640
  },
  "stats": {
    "movies": 150,
    "tvShows": 45,
    "seasons": 200,
    "episodes": 1800,
    "genres": 38,
    "matchQueue": 5,
    "lastSyncedAt": "2026-02-11T09:00:00.000Z"
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/status
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "media-metadata",
  "version": "1.0.0",
  "status": "running",
  "stats": {
    "movies": 150,
    "tvShows": 45,
    "seasons": 200,
    "episodes": 1800,
    "genres": 38,
    "matchQueue": 5,
    "lastSyncedAt": "2026-02-11T09:00:00.000Z"
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/stats
Get statistics only.

**Response:**
```json
{
  "movies": 150,
  "tvShows": 45,
  "seasons": 200,
  "episodes": 1800,
  "genres": 38,
  "matchQueue": 5,
  "lastSyncedAt": "2026-02-11T09:00:00.000Z"
}
```

### Search

#### GET /v1/search
Search TMDB for movies or TV shows.

**Query Parameters:**
- `query` (required) - Search query
- `media_type` (optional) - `movie` or `tv` (default: both)
- `year` (optional) - Release/first air year
- `page` (optional) - Page number (default: 1)

**Example:**
```bash
curl "http://localhost:3202/v1/search?query=The+Matrix&media_type=movie"
```

**Response:**
```json
{
  "page": 1,
  "total_pages": 2,
  "total_results": 35,
  "results": [
    {
      "id": 603,
      "title": "The Matrix",
      "release_date": "1999-03-31",
      "vote_average": 8.2,
      "popularity": 52.4,
      "overview": "Set in the 22nd century..."
    }
  ]
}
```

### Movies

#### GET /v1/movies/:tmdbId
Get movie by TMDB ID (fetches from TMDB if not cached).

**Example:**
```bash
curl http://localhost:3202/v1/movies/603
```

**Response:**
```json
{
  "id": "uuid",
  "source_account_id": "primary",
  "tmdb_id": 603,
  "imdb_id": "tt0133093",
  "title": "The Matrix",
  "original_title": "The Matrix",
  "overview": "Set in the 22nd century...",
  "release_date": "1999-03-31T00:00:00.000Z",
  "runtime_minutes": 136,
  "vote_average": 8.2,
  "vote_count": 18500,
  "popularity": 52.4,
  "status": "Released",
  "tagline": "Welcome to the Real World",
  "budget": 63000000,
  "revenue": 463517383,
  "genres": ["Action", "Science Fiction"],
  "spoken_languages": ["English"],
  "production_countries": ["United States of America"],
  "poster_path": "/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
  "backdrop_path": "/fNG7i7RqMErkcqhohV2a6cV1Ehy.jpg",
  "cast": [...],
  "crew": [...],
  "content_rating": "R",
  "keywords": ["dystopia", "artificial intelligence", "hacker"],
  "synced_at": "2026-02-11T10:00:00.000Z",
  "created_at": "2026-02-11T10:00:00.000Z",
  "updated_at": "2026-02-11T10:00:00.000Z"
}
```

#### GET /v1/movies/trending
Get trending movies (week).

#### GET /v1/movies/popular
Get popular movies.

#### POST /v1/sync/movie/:tmdbId
Force sync specific movie by TMDB ID.

**Response:**
```json
{
  "success": true,
  "tmdb_id": 603
}
```

### TV Shows

#### GET /v1/tv/:tmdbId
Get TV show by TMDB ID.

#### GET /v1/tv/trending
Get trending TV shows (week).

#### GET /v1/tv/popular
Get popular TV shows.

#### POST /v1/sync/tv/:tmdbId
Force sync specific TV show by TMDB ID.

### Seasons & Episodes

#### GET /v1/tv/:tmdbId/season/:seasonNum
Get TV season with episodes.

**Example:**
```bash
curl http://localhost:3202/v1/tv/1396/season/1
```

#### GET /v1/tv/:tmdbId/season/:seasonNum/episode/:episodeNum
Get specific TV episode.

**Example:**
```bash
curl http://localhost:3202/v1/tv/1396/season/1/episode/1
```

### Lookup & Enrichment

#### POST /v1/lookup
Lookup media by title with confidence scoring.

**Request Body:**
```json
{
  "title": "The Matrix",
  "year": 1999,
  "media_type": "movie"
}
```

**Response:**
```json
{
  "matched": true,
  "confidence": 0.95,
  "tmdb_id": 603,
  "title": "The Matrix",
  "year": 1999,
  "media_type": "movie",
  "candidates": [
    {
      "tmdb_id": 603,
      "title": "The Matrix",
      "year": 1999,
      "confidence": 0.95
    }
  ]
}
```

#### POST /v1/lookup/batch
Batch lookup multiple items.

**Request Body:**
```json
{
  "items": [
    { "title": "The Matrix", "year": 1999, "media_type": "movie" },
    { "title": "Inception", "year": 2010, "media_type": "movie" }
  ]
}
```

**Response:**
```json
{
  "results": [...],
  "duration": 1250
}
```

#### POST /v1/enrich
Enrich media with TMDB metadata (lookup + fetch + store).

**Request Body:**
```json
{
  "title": "The Matrix",
  "year": 1999,
  "media_type": "movie",
  "force": false
}
```

**Response:**
```json
{
  "success": true,
  "cached": false,
  "tmdb_id": 603,
  "media_type": "movie",
  "metadata": {
    "title": "The Matrix",
    "release_date": "1999-03-31T00:00:00.000Z",
    "runtime_minutes": 136,
    "vote_average": 8.2,
    ...
  }
}
```

### Genres

#### GET /v1/genres
List all genres.

**Query Parameters:**
- `media_type` (optional) - `movie` or `tv`

**Response:**
```json
[
  {
    "id": "uuid",
    "source_account_id": "primary",
    "tmdb_id": 28,
    "name": "Action",
    "media_type": "movie"
  }
]
```

#### POST /v1/sync/genres
Sync genre list from TMDB.

**Response:**
```json
{
  "success": true,
  "movieGenres": 19,
  "tvGenres": 16
}
```

### Match Queue

#### GET /v1/match-queue
List items in match queue.

**Query Parameters:**
- `status` (optional) - Filter by status (pending, manual_review, matched, no_match)
- `limit` (optional) - Max items to return (default: 100)

**Response:**
```json
[
  {
    "id": "uuid",
    "source_account_id": "primary",
    "title": "Batman",
    "year": 2022,
    "media_type": "movie",
    "source_id": "file_123",
    "source_plugin": "file-scanner",
    "candidates": [
      { "tmdb_id": 414906, "title": "The Batman", "year": 2022, "confidence": 0.65 },
      { "tmdb_id": 268, "title": "Batman", "year": 1989, "confidence": 0.45 }
    ],
    "status": "manual_review",
    "matched_tmdb_id": null,
    "confidence": 0.65,
    "reviewed_by": null,
    "reviewed_at": null,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
]
```

#### POST /v1/match-queue/:id/match
Approve a match manually.

**Request Body:**
```json
{
  "tmdb_id": 414906,
  "reviewed_by": "admin"
}
```

**Response:**
```json
{
  "success": true
}
```

#### POST /v1/match-queue/:id/reject
Reject a match (no match found).

**Request Body:**
```json
{
  "reviewed_by": "admin"
}
```

**Response:**
```json
{
  "success": true
}
```

---

## Database Schema

### tmdb_movies

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `tmdb_id` | INTEGER | TMDB movie ID |
| `imdb_id` | VARCHAR(20) | IMDb ID |
| `title` | VARCHAR(500) | Movie title |
| `original_title` | VARCHAR(500) | Original title |
| `overview` | TEXT | Plot summary |
| `release_date` | DATE | Release date |
| `runtime_minutes` | INTEGER | Runtime in minutes |
| `vote_average` | DOUBLE PRECISION | Average rating |
| `vote_count` | INTEGER | Number of votes |
| `popularity` | DOUBLE PRECISION | Popularity score |
| `status` | VARCHAR(32) | Release status |
| `tagline` | TEXT | Movie tagline |
| `budget` | BIGINT | Production budget |
| `revenue` | BIGINT | Box office revenue |
| `genres` | TEXT[] | Genre names |
| `spoken_languages` | TEXT[] | Language names |
| `production_countries` | TEXT[] | Country names |
| `poster_path` | TEXT | Poster image path |
| `backdrop_path` | TEXT | Backdrop image path |
| `cast` | JSONB | Cast members |
| `crew` | JSONB | Crew members |
| `content_rating` | VARCHAR(16) | US content rating (PG, PG-13, R, etc.) |
| `keywords` | TEXT[] | Keyword tags |
| `synced_at` | TIMESTAMPTZ | Last sync timestamp |
| `created_at` | TIMESTAMPTZ | Record creation time |
| `updated_at` | TIMESTAMPTZ | Record update time |

**Indexes:**
- `idx_tmdb_movies_account` - source_account_id
- `idx_tmdb_movies_tmdb_id` - tmdb_id
- `idx_tmdb_movies_imdb_id` - imdb_id
- `idx_tmdb_movies_title` - title (trigram)
- `idx_tmdb_movies_release_date` - release_date
- `idx_tmdb_movies_popularity` - popularity DESC

**Unique Constraint:**
- `(source_account_id, tmdb_id)`

### tmdb_tv_shows

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `tmdb_id` | INTEGER | TMDB TV show ID |
| `imdb_id` | VARCHAR(20) | IMDb ID |
| `name` | VARCHAR(500) | TV show name |
| `original_name` | VARCHAR(500) | Original name |
| `overview` | TEXT | Show summary |
| `first_air_date` | DATE | First air date |
| `last_air_date` | DATE | Last air date |
| `status` | VARCHAR(32) | Show status |
| `type` | VARCHAR(32) | Show type |
| `number_of_seasons` | INTEGER | Season count |
| `number_of_episodes` | INTEGER | Episode count |
| `episode_run_time` | INTEGER[] | Episode runtimes |
| `vote_average` | DOUBLE PRECISION | Average rating |
| `vote_count` | INTEGER | Number of votes |
| `popularity` | DOUBLE PRECISION | Popularity score |
| `genres` | TEXT[] | Genre names |
| `networks` | TEXT[] | Network names |
| `created_by` | TEXT[] | Creator names |
| `poster_path` | TEXT | Poster image path |
| `backdrop_path` | TEXT | Backdrop image path |
| `content_rating` | VARCHAR(16) | US content rating |
| `keywords` | TEXT[] | Keyword tags |
| `synced_at` | TIMESTAMPTZ | Last sync timestamp |
| `created_at` | TIMESTAMPTZ | Record creation time |
| `updated_at` | TIMESTAMPTZ | Record update time |

**Indexes:**
- `idx_tmdb_tv_shows_account` - source_account_id
- `idx_tmdb_tv_shows_tmdb_id` - tmdb_id
- `idx_tmdb_tv_shows_imdb_id` - imdb_id
- `idx_tmdb_tv_shows_name` - name (trigram)
- `idx_tmdb_tv_shows_first_air_date` - first_air_date
- `idx_tmdb_tv_shows_popularity` - popularity DESC

**Unique Constraint:**
- `(source_account_id, tmdb_id)`

### tmdb_tv_seasons

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `show_tmdb_id` | INTEGER | Parent TV show TMDB ID |
| `season_number` | INTEGER | Season number |
| `tmdb_id` | INTEGER | TMDB season ID |
| `name` | VARCHAR(255) | Season name |
| `overview` | TEXT | Season summary |
| `air_date` | DATE | Air date |
| `episode_count` | INTEGER | Number of episodes |
| `poster_path` | TEXT | Poster image path |
| `synced_at` | TIMESTAMPTZ | Last sync timestamp |
| `created_at` | TIMESTAMPTZ | Record creation time |

**Indexes:**
- `idx_tmdb_tv_seasons_account` - source_account_id
- `idx_tmdb_tv_seasons_show` - (show_tmdb_id, season_number)

**Unique Constraint:**
- `(source_account_id, show_tmdb_id, season_number)`

### tmdb_tv_episodes

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `show_tmdb_id` | INTEGER | Parent TV show TMDB ID |
| `season_number` | INTEGER | Season number |
| `episode_number` | INTEGER | Episode number |
| `tmdb_id` | INTEGER | TMDB episode ID |
| `name` | VARCHAR(500) | Episode name |
| `overview` | TEXT | Episode summary |
| `air_date` | DATE | Air date |
| `runtime_minutes` | INTEGER | Runtime in minutes |
| `vote_average` | DOUBLE PRECISION | Average rating |
| `still_path` | TEXT | Still image path |
| `guest_stars` | JSONB | Guest star cast |
| `crew` | JSONB | Episode crew |
| `synced_at` | TIMESTAMPTZ | Last sync timestamp |
| `created_at` | TIMESTAMPTZ | Record creation time |

**Indexes:**
- `idx_tmdb_tv_episodes_account` - source_account_id
- `idx_tmdb_tv_episodes_show` - (show_tmdb_id, season_number, episode_number)

**Unique Constraint:**
- `(source_account_id, show_tmdb_id, season_number, episode_number)`

### tmdb_genres

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `tmdb_id` | INTEGER | TMDB genre ID |
| `name` | VARCHAR(128) | Genre name |
| `media_type` | VARCHAR(8) | `movie` or `tv` |

**Indexes:**
- `idx_tmdb_genres_account` - source_account_id
- `idx_tmdb_genres_media_type` - media_type

**Unique Constraint:**
- `(source_account_id, tmdb_id, media_type)`

### tmdb_match_queue

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `title` | VARCHAR(500) | Media title to match |
| `year` | INTEGER | Release/air year |
| `media_type` | VARCHAR(8) | `movie` or `tv` |
| `source_id` | VARCHAR(255) | Source system ID |
| `source_plugin` | VARCHAR(64) | Source plugin name |
| `candidates` | JSONB | Match candidates with confidence |
| `status` | VARCHAR(16) | `pending`, `manual_review`, `matched`, `no_match` |
| `matched_tmdb_id` | INTEGER | Selected TMDB ID |
| `confidence` | DOUBLE PRECISION | Confidence score (0-1) |
| `reviewed_by` | VARCHAR(255) | Reviewer identifier |
| `reviewed_at` | TIMESTAMPTZ | Review timestamp |
| `created_at` | TIMESTAMPTZ | Record creation time |

**Indexes:**
- `idx_tmdb_match_queue_account` - source_account_id
- `idx_tmdb_match_queue_status` - status
- `idx_tmdb_match_queue_media_type` - media_type

### tmdb_webhook_events

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
- `idx_tmdb_webhook_events_account` - source_account_id
- `idx_tmdb_webhook_events_processed` - processed

---

## Lookup & Matching

### How Matching Works

The plugin uses intelligent title and year matching with confidence scoring:

1. **Search** - Query TMDB for matching titles
2. **Score** - Calculate confidence based on:
   - Title similarity (Levenshtein distance)
   - Year proximity (+/- 1 year tolerance)
   - Popularity (tie-breaker)
3. **Threshold** - Auto-match if confidence ≥ threshold (default 70%)
4. **Queue** - If confidence < threshold, add to manual review queue

### Confidence Levels

| Confidence | Meaning |
|------------|---------|
| 0.90 - 1.00 | Exact match (title + year) |
| 0.70 - 0.89 | High confidence (close title, year match) |
| 0.50 - 0.69 | Medium confidence (similar title, year close) |
| 0.00 - 0.49 | Low confidence (manual review needed) |

### Match Queue Workflow

```bash
# 1. Lookup returns low confidence
curl -X POST http://localhost:3202/v1/lookup \
  -H "Content-Type: application/json" \
  -d '{"title": "Batman", "year": 2022, "media_type": "movie"}'

# Response: confidence 0.65, added to queue
{
  "matched": false,
  "confidence": 0.65,
  "candidates": [
    {"tmdb_id": 414906, "title": "The Batman", "year": 2022, "confidence": 0.65}
  ]
}

# 2. Review queue
curl http://localhost:3202/v1/match-queue?status=manual_review

# 3. Approve correct match
curl -X POST http://localhost:3202/v1/match-queue/{id}/match \
  -H "Content-Type: application/json" \
  -d '{"tmdb_id": 414906, "reviewed_by": "admin"}'
```

---

## TypeScript Implementation

### File Structure

```
plugins/media-metadata/ts/src/
├── types.ts          # TypeScript interfaces
├── config.ts         # Configuration loading
├── database.ts       # Database operations
├── client.ts         # TMDB API client
├── lookup.ts         # Matching logic
├── server.ts         # HTTP server
├── cli.ts            # CLI commands
└── index.ts          # Module exports
```

### Key Components

#### TmdbClient (client.ts)
- TMDB API wrapper
- Rate limiting (40 requests/10s)
- Response caching
- Type mapping

#### TmdbLookupService (lookup.ts)
- Title/year matching
- Confidence scoring
- Match queue management
- Batch lookups

#### TmdbDatabase (database.ts)
- Schema initialization
- CRUD operations
- Match queue operations
- Statistics

---

## Examples

### Example 1: Enrich Movie Library

```bash
#!/bin/bash

# Enrich a list of movies
movies=(
  "The Matrix|1999"
  "Inception|2010"
  "Interstellar|2014"
  "The Dark Knight|2008"
)

for movie in "${movies[@]}"; do
  IFS='|' read -r title year <<< "$movie"
  echo "Enriching: $title ($year)"

  nself plugin media-metadata enrich "$title" --year "$year" --type movie
done
```

### Example 2: Bulk Lookup API

```typescript
const items = [
  { title: "The Matrix", year: 1999, media_type: "movie" },
  { title: "Breaking Bad", year: 2008, media_type: "tv" },
  { title: "Game of Thrones", year: 2011, media_type: "tv" }
];

const response = await fetch('http://localhost:3202/v1/lookup/batch', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ items })
});

const { results } = await response.json();
results.forEach(r => {
  console.log(`${r.title}: ${r.matched ? 'matched' : 'needs review'} (${r.confidence})`);
});
```

### Example 3: Process Match Queue

```sql
-- Find all pending matches
SELECT
  id,
  title,
  year,
  media_type,
  candidates->0->>'title' as top_match,
  confidence
FROM tmdb_match_queue
WHERE status = 'manual_review'
ORDER BY confidence DESC;

-- Auto-approve high confidence matches
UPDATE tmdb_match_queue
SET
  status = 'matched',
  matched_tmdb_id = (candidates->0->>'tmdb_id')::integer,
  reviewed_by = 'auto',
  reviewed_at = NOW()
WHERE status = 'manual_review'
  AND confidence >= 0.75;
```

### Example 4: Query Enriched Data

```sql
-- Find all Christopher Nolan movies
SELECT
  title,
  release_date,
  vote_average,
  revenue
FROM tmdb_movies
WHERE crew @> '[{"name": "Christopher Nolan", "job": "Director"}]'
ORDER BY release_date DESC;

-- Find popular sci-fi movies
SELECT
  title,
  release_date,
  vote_average,
  popularity
FROM tmdb_movies
WHERE 'Science Fiction' = ANY(genres)
  AND vote_average >= 7.0
ORDER BY popularity DESC
LIMIT 20;

-- Get TV show episode count
SELECT
  s.name as show_name,
  s.number_of_seasons,
  s.number_of_episodes,
  COUNT(e.id) as synced_episodes
FROM tmdb_tv_shows s
LEFT JOIN tmdb_tv_episodes e ON e.show_tmdb_id = s.tmdb_id
WHERE s.source_account_id = 'primary'
GROUP BY s.id, s.name, s.number_of_seasons, s.number_of_episodes
ORDER BY s.number_of_episodes DESC;
```

---

## Troubleshooting

### Common Issues

#### API Key Invalid

**Error:**
```
Error: TMDB_API_KEY must be set
```

**Solution:**
Get an API key from https://www.themoviedb.org/settings/api

#### Rate Limit Exceeded

**Error:**
```
Error: 429 Too Many Requests
```

**Solution:**
TMDB free tier allows 40 requests per 10 seconds. The plugin handles this automatically with rate limiting, but if you see this error:
- Reduce `TMDB_RATE_LIMIT_MAX`
- Increase `TMDB_RATE_LIMIT_WINDOW_MS`

#### No Matches Found

**Problem:**
Lookup returns no candidates.

**Solutions:**
- Check title spelling
- Try without year filter
- Search directly at themoviedb.org to verify it exists
- Try alternate titles (original title vs. localized)

#### Low Confidence Matches

**Problem:**
All matches have confidence < threshold.

**Solutions:**
- Lower `TMDB_CONFIDENCE_THRESHOLD` (e.g., 0.60)
- Include year for better matching
- Use exact title from TMDB
- Review match queue and approve manually

#### Images Not Loading

**Problem:**
Poster/backdrop URLs don't work.

**Solution:**
TMDB image paths are relative. Construct full URL:
```
{TMDB_IMAGE_BASE_URL}/w500{poster_path}
```

Example:
```
https://image.tmdb.org/t/p/w500/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg
```

Image sizes available: `w92`, `w154`, `w185`, `w342`, `w500`, `w780`, `original`

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug nself plugin media-metadata server
```

### Database Connection Issues

**Error:**
```
Error: Connection refused
```

**Solution:**
1. Verify PostgreSQL is running
2. Check DATABASE_URL format: `postgresql://user:pass@host:port/dbname`
3. Ensure database exists: `createdb nself`
4. Check pg_trgm extension: `CREATE EXTENSION pg_trgm;`

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Media-Metadata
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **TMDB API**: https://developers.themoviedb.org/3
