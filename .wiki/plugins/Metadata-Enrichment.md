# Metadata Enrichment Plugin

TMDB metadata enrichment for movies and TV shows with intelligent caching and rate-limited API integration.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Caching System](#caching-system)
- [Rate Limiting](#rate-limiting)
- [Multi-App Support](#multi-app-support)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Metadata Enrichment plugin provides a dedicated service for looking up and caching movie and TV show metadata from The Movie Database (TMDB). It acts as a caching proxy layer between your application and the TMDB API, storing enriched metadata in PostgreSQL so that repeated lookups are served instantly from the local database rather than hitting the external API.

Unlike the full-featured [TMDB plugin](./TMDB.md) which provides match queues, filename parsing, batch operations, and genre syncing, the Metadata Enrichment plugin focuses on a streamlined, lightweight workflow: search for media, fetch details by TMDB ID, and cache the results locally with configurable staleness thresholds. This makes it ideal for applications that need fast metadata lookups without the overhead of a full media matching pipeline.

### Key Features

- **TMDB Movie Search** - Search for movies by title with optional year filtering
- **TMDB TV Show Search** - Search for TV shows by title with optional year filtering
- **Movie Detail Lookup** - Fetch comprehensive movie metadata by TMDB ID with local caching
- **TV Show Detail Lookup** - Fetch comprehensive TV show metadata by TMDB ID with local caching
- **Cache-First Architecture** - Database cache with configurable staleness (default 24 hours)
- **Rate-Limited TMDB Calls** - Queue-based rate limiter capped at 40 requests/second (TMDB allows 50)
- **Stale-While-Revalidate** - Returns stale cache if TMDB is unreachable, with source indicator
- **JSONB Raw Response Storage** - Full TMDB API response preserved for custom field extraction
- **Multi-Account Isolation** - Full `source_account_id` support for multi-tenant deployments
- **API Key Authentication** - Optional API key protection for all endpoints
- **Configurable Rate Limits** - Adjustable inbound request rate limiting via shared utilities

### Use Cases

- Media library metadata enrichment
- Streaming platform content catalog backend
- Recommendation engine data source
- Content management system media lookups
- Mobile app movie/TV show detail screens
- Search-ahead/typeahead for media titles

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                Metadata Enrichment Plugin                     │
│                                                              │
│  ┌──────────┐  ┌──────────────┐  ┌────────────────────┐     │
│  │  CLI     │  │  Fastify     │  │  TMDBClient        │     │
│  │(cli.ts)  │  │  Server      │  │  (tmdb-client.ts)  │     │
│  │          │  │  (server.ts) │  │  - Rate limiter     │     │
│  │  init    │  │              │  │  - searchMovies()   │     │
│  │  search  │  │  /health     │  │  - searchTV()       │     │
│  │  server  │  │  /v1/movies  │  │  - getMovieDetails()│     │
│  └────┬─────┘  │  /v1/tv     │  │  - getTVShowDetails()│    │
│       │        └──────┬───────┘  └─────────┬──────────┘     │
│       │               │                    │                 │
│       └───────────────┼────────────────────┘                 │
│                       │                                      │
│              ┌────────┴──────────────────┐                   │
│              │  Database (database.ts)    │                   │
│              │  - Schema initialization   │                   │
│              │  - upsertMovie()           │                   │
│              │  - upsertTVShow()          │                   │
│              │  - getMovieByTmdbId()      │                   │
│              │  - getTVShowByTmdbId()     │                   │
│              │  - searchMovies() (local)  │                   │
│              │  - searchTVShows() (local) │                   │
│              └───────────────────────────┘                   │
└─────────────────────────────────────────────────────────────┘
```

### Plugin Metadata

| Property | Value |
|----------|-------|
| **Name** | `metadata-enrichment` |
| **Version** | `1.0.0` |
| **Category** | `media` |
| **Subcategory** | `metadata` |
| **Port** | `3203` |
| **Language** | TypeScript |
| **Runtime** | Node.js (>= 18.0.0) |
| **License** | MIT |
| **Min nself Version** | `0.4.8` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install metadata-enrichment

# Configure required environment variables
cat > .env <<EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
TMDB_API_KEY=your_tmdb_api_key_here
EOF

# Initialize database schema
nself plugin metadata-enrichment init

# Start the server
nself plugin metadata-enrichment server

# Search for a movie
nself plugin metadata-enrichment search-movie "Inception"

# Search for a TV show
nself plugin metadata-enrichment search-tv "Breaking Bad"

# Query via API
curl http://localhost:3203/v1/movies/search?q=Inception
curl http://localhost:3203/v1/movies/27205
curl http://localhost:3203/v1/tv/search?q=Breaking+Bad
curl http://localhost:3203/v1/tv/1396
```

---

## Configuration

### Required Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | **Yes** | - | PostgreSQL connection string (e.g., `postgresql://user:pass@localhost:5432/nself`) |
| `TMDB_API_KEY` | **Yes** | - | TMDB API key (v3 auth) for movie and TV metadata lookups |

### Optional Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `METADATA_ENRICHMENT_PORT` | No | `3203` | HTTP server port |
| `TVDB_API_KEY` | No | - | TVDB API key for supplementary TV metadata (reserved for future use) |
| `MUSICBRAINZ_USER_AGENT` | No | `nself-tv/1.0.0` | User agent string for MusicBrainz API requests (reserved for future use) |
| `OBJECT_STORAGE_URL` | No | - | Object storage URL for media assets (reserved for future use) |
| `LOG_LEVEL` | No | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `METADATA_ENRICHMENT_API_KEY` | No | - | API key for authenticating inbound requests. Falls back to `NSELF_API_KEY` if not set. When configured, all endpoints require `Authorization: Bearer <key>` header |
| `NSELF_API_KEY` | No | - | Global nself API key; used as fallback when `METADATA_ENRICHMENT_API_KEY` is not set |
| `METADATA_ENRICHMENT_RATE_LIMIT_MAX` | No | `100` | Maximum inbound API requests per rate limit window |
| `METADATA_ENRICHMENT_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window duration in milliseconds |

### Example .env File

```bash
# Database (required)
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# TMDB API (required - Get from https://www.themoviedb.org/settings/api)
TMDB_API_KEY=your_tmdb_v3_api_key_here

# Server
METADATA_ENRICHMENT_PORT=3203

# Security (optional)
METADATA_ENRICHMENT_API_KEY=your_secret_api_key
METADATA_ENRICHMENT_RATE_LIMIT_MAX=200
METADATA_ENRICHMENT_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info

# Supplementary providers (reserved for future use)
# TVDB_API_KEY=your_tvdb_api_key
# MUSICBRAINZ_USER_AGENT=nself-tv/1.0.0
# OBJECT_STORAGE_URL=https://storage.example.com
```

### Getting a TMDB API Key

1. Create a free account at [https://www.themoviedb.org/](https://www.themoviedb.org/)
2. Go to **Settings > API** in your account dashboard
3. Request an API key (select "Developer" usage type)
4. Copy the **API Key (v3 auth)** value
5. Set it as `TMDB_API_KEY` in your `.env` file

TMDB's free tier allows up to 50 requests per second, which is more than sufficient for most use cases. The plugin's built-in rate limiter caps outbound calls at 40 requests/second to stay well within this threshold.

---

## CLI Commands

### Overview

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (create tables and indexes) |
| `search-movie <query>` | Search TMDB for movies by title |
| `search-tv <query>` | Search TMDB for TV shows by title |
| `server` | Start the HTTP API server |

### init

Initialize the database schema by creating the required tables and indexes.

```bash
nself plugin metadata-enrichment init
```

**What it does:**
- Creates `np_metaenrich_movies` table if it does not exist
- Creates `np_metaenrich_tv_shows` table if it does not exist
- Creates all required indexes for efficient lookups
- Safe to run multiple times (uses `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`)

**Example output:**
```
⠋ Initializing metadata enrichment
✔ Database initialized
```

### search-movie

Search TMDB for movies by title. Results are fetched directly from the TMDB API (not from the local cache).

```bash
nself plugin metadata-enrichment search-movie <query>
```

**Arguments:**
- `query` (required): The movie title to search for

**Example:**
```bash
nself plugin metadata-enrichment search-movie "The Dark Knight"
```

**Example output:**
```
Found 20 movies:

1. The Dark Knight (2008)
   ID: 155 | Rating: 8.5/10

2. The Dark Knight Rises (2012)
   ID: 49026 | Rating: 7.8/10

3. Batman: The Dark Knight Returns, Part 1 (2012)
   ID: 123025 | Rating: 7.8/10
```

The command displays up to 10 results, showing the title, release year, TMDB ID, and average rating. Use the TMDB ID with the REST API `/v1/movies/:id` endpoint to fetch and cache full metadata.

### search-tv

Search TMDB for TV shows by title. Results are fetched directly from the TMDB API (not from the local cache).

```bash
nself plugin metadata-enrichment search-tv <query>
```

**Arguments:**
- `query` (required): The TV show title to search for

**Example:**
```bash
nself plugin metadata-enrichment search-tv "Stranger Things"
```

**Example output:**
```
Found 5 TV shows:

1. Stranger Things (2016)
   ID: 66732 | Rating: 8.6/10

2. Stranger Things: Spotlight (2022)
   ID: 212744 | Rating: 5.5/10
```

The command displays up to 10 results, showing the name, first air date year, TMDB ID, and average rating.

### server

Start the HTTP API server on the configured port.

```bash
nself plugin metadata-enrichment server
```

**What it does:**
1. Initializes database schema (same as `init`)
2. Configures CORS, rate limiting, and optional API key authentication
3. Registers all REST API routes
4. Starts the Fastify HTTP server on `0.0.0.0:<port>`
5. Listens for SIGINT (Ctrl+C) for graceful shutdown

**Example output:**
```
Starting Metadata Enrichment Server...

✓ Server running on port 3203
```

**Graceful shutdown:**
Press Ctrl+C to gracefully stop the server. It will close the HTTP listener and the database connection pool before exiting.

---

## REST API

### Base URL

```
http://localhost:3203
```

### Authentication

When `METADATA_ENRICHMENT_API_KEY` or `NSELF_API_KEY` is configured, all endpoints require the API key in the `Authorization` header:

```
Authorization: Bearer your_secret_api_key
```

If no API key is configured, all endpoints are publicly accessible.

### Rate Limiting

All endpoints are rate-limited. The default is 100 requests per 60-second window, configurable via `METADATA_ENRICHMENT_RATE_LIMIT_MAX` and `METADATA_ENRICHMENT_RATE_LIMIT_WINDOW_MS`. When the limit is exceeded, the server returns HTTP 429 Too Many Requests.

### Health

#### GET /health

Health check endpoint. Returns basic server status.

**Request:**
```http
GET /health
```

**Response:**
```json
{
  "status": "ok",
  "plugin": "metadata-enrichment",
  "timestamp": "2026-02-12T10:00:00.000Z"
}
```

**Status Codes:**
- `200 OK` - Server is healthy

---

### Movie Endpoints

#### GET /v1/movies/search

Search TMDB for movies by title. This endpoint always queries the TMDB API directly -- search results are not cached by ID (individual movie details are cached when fetched via `/v1/movies/:id`).

**Request:**
```http
GET /v1/movies/search?q=Inception&year=2010
```

**Query Parameters:**

| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| `q` | **Yes** | string | Movie title search query |
| `year` | No | string | Release year filter (4-digit year) |

**Response (success):**
```json
{
  "results": [
    {
      "adult": false,
      "backdrop_path": "/8ZTVqvKDQ8emSGUEMjsS4yHAwrp.jpg",
      "genre_ids": [28, 878, 12],
      "id": 27205,
      "original_language": "en",
      "original_title": "Inception",
      "overview": "Cobb, a skilled thief who commits corporate espionage by infiltrating the subconscious of his targets...",
      "popularity": 108.234,
      "poster_path": "/oYuLEt3zVCKq57qu2F8dT7NIa6f.jpg",
      "release_date": "2010-07-15",
      "title": "Inception",
      "video": false,
      "vote_average": 8.369,
      "vote_count": 35241
    }
  ]
}
```

**Response (missing query):**
```json
{
  "results": [],
  "error": "q query parameter is required"
}
```

**Status Codes:**
- `200 OK` - Results returned (may be empty array)

**Notes:**
- Adult content is excluded from search results (`include_adult: false`)
- Results come directly from the TMDB search API and reflect TMDB's ranking/relevance algorithm
- The `id` field in each result is the TMDB movie ID -- use it with `/v1/movies/:id` to fetch and cache full details

---

#### GET /v1/movies/:id

Get full movie details by TMDB ID. Uses a cache-first strategy: if the movie exists in the local database and is not stale (updated within the last 24 hours), the cached version is returned. Otherwise, the plugin fetches fresh data from TMDB, upserts it into the database, and returns the enriched result.

**Request:**
```http
GET /v1/movies/27205
```

**Path Parameters:**

| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| `id` | **Yes** | string | TMDB movie ID (integer) |

**Headers:**

| Header | Required | Description |
|--------|----------|-------------|
| `X-App-Name` | No | Multi-app context identifier (defaults to `primary`) |

**Response (from cache):**
```json
{
  "movie": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "source_account_id": "primary",
    "tmdb_id": 27205,
    "imdb_id": "tt1375666",
    "title": "Inception",
    "original_title": "Inception",
    "overview": "Cobb, a skilled thief who commits corporate espionage by infiltrating the subconscious of his targets is offered a chance to regain his old life as payment for a task considered to be impossible: \"inception\", the implantation of another person's idea into a target's subconscious.",
    "release_date": "2010-07-15",
    "runtime": 148,
    "genres": ["Action", "Science Fiction", "Adventure"],
    "vote_average": 8.4,
    "vote_count": 35241,
    "poster_path": "/oYuLEt3zVCKq57qu2F8dT7NIa6f.jpg",
    "backdrop_path": "/8ZTVqvKDQ8emSGUEMjsS4yHAwrp.jpg",
    "raw_response": { "...full TMDB API response..." },
    "created_at": "2026-02-10T08:00:00.000Z",
    "updated_at": "2026-02-12T09:30:00.000Z"
  },
  "source": "cache"
}
```

**Response (from TMDB - fresh fetch):**
```json
{
  "movie": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "source_account_id": "primary",
    "tmdb_id": 27205,
    "imdb_id": "tt1375666",
    "title": "Inception",
    "...": "...",
    "created_at": "2026-02-12T10:00:00.000Z",
    "updated_at": "2026-02-12T10:00:00.000Z"
  },
  "source": "tmdb"
}
```

**Response (stale cache returned when TMDB is unreachable):**
```json
{
  "movie": {
    "...cached movie data..."
  },
  "source": "cache-stale"
}
```

**Response (not found anywhere):**
```json
{
  "movie": null,
  "source": "not-found"
}
```

**Response (invalid ID):**
```json
{
  "movie": null,
  "error": "Invalid movie ID"
}
```

**Source Field Values:**

| Value | Meaning |
|-------|---------|
| `cache` | Returned from local database (fresh, within 24 hours) |
| `tmdb` | Freshly fetched from TMDB API and cached locally |
| `cache-stale` | Stale cache returned because TMDB could not be reached |
| `not-found` | Not found in cache or TMDB |

**Status Codes:**
- `200 OK` - Response returned (check `source` field for details)

**Notes:**
- The staleness threshold is 24 hours by default (hardcoded in the `isStale()` method)
- The TMDB fetch includes appended data: `credits`, `videos`, and `release_dates`
- Genres are extracted from TMDB's genre objects and stored as a flat string array
- The full TMDB API response is preserved in `raw_response` for custom field extraction
- The upsert uses `COALESCE` to avoid overwriting existing data with null values

---

### TV Show Endpoints

#### GET /v1/tv/search

Search TMDB for TV shows by title. This endpoint always queries the TMDB API directly -- search results are not cached.

**Request:**
```http
GET /v1/tv/search?q=Breaking%20Bad&year=2008
```

**Query Parameters:**

| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| `q` | **Yes** | string | TV show title search query |
| `year` | No | string | First air date year filter (4-digit year) |

**Response (success):**
```json
{
  "results": [
    {
      "adult": false,
      "backdrop_path": "/9faGSFi5jam6pDWGNd0p8JcJgXQ.jpg",
      "genre_ids": [18, 80],
      "id": 1396,
      "name": "Breaking Bad",
      "original_language": "en",
      "original_name": "Breaking Bad",
      "overview": "Walter White, a New Mexico chemistry teacher, is diagnosed with Stage III cancer...",
      "popularity": 354.873,
      "poster_path": "/ggFHVNu6YYI5L9pCfOacjizRGt.jpg",
      "first_air_date": "2008-01-20",
      "vote_average": 8.913,
      "vote_count": 13785
    }
  ]
}
```

**Response (missing query):**
```json
{
  "results": [],
  "error": "q query parameter is required"
}
```

**Status Codes:**
- `200 OK` - Results returned (may be empty array)

**Notes:**
- Adult content is excluded (`include_adult: false`)
- The `year` parameter maps to TMDB's `first_air_date_year` filter
- The `id` field is the TMDB TV show ID -- use with `/v1/tv/:id` to fetch and cache details

---

#### GET /v1/tv/:id

Get full TV show details by TMDB ID. Uses the same cache-first strategy as the movie detail endpoint: checks local database first, fetches from TMDB if missing or stale, and caches the result.

**Request:**
```http
GET /v1/tv/1396
```

**Path Parameters:**

| Parameter | Required | Type | Description |
|-----------|----------|------|-------------|
| `id` | **Yes** | string | TMDB TV show ID (integer) |

**Headers:**

| Header | Required | Description |
|--------|----------|-------------|
| `X-App-Name` | No | Multi-app context identifier (defaults to `primary`) |

**Response (from cache):**
```json
{
  "show": {
    "id": "b2c3d4e5-f6a7-8901-bcde-f23456789012",
    "source_account_id": "primary",
    "tmdb_id": 1396,
    "tvdb_id": 81189,
    "imdb_id": "tt0903747",
    "name": "Breaking Bad",
    "original_name": "Breaking Bad",
    "overview": "Walter White, a New Mexico chemistry teacher, is diagnosed with Stage III cancer and turns to a life of crime...",
    "first_air_date": "2008-01-20",
    "last_air_date": "2013-09-29",
    "number_of_seasons": 5,
    "number_of_episodes": 62,
    "genres": ["Drama", "Crime"],
    "vote_average": 8.9,
    "vote_count": 13785,
    "poster_path": "/ggFHVNu6YYI5L9pCfOacjizRGt.jpg",
    "backdrop_path": "/9faGSFi5jam6pDWGNd0p8JcJgXQ.jpg",
    "raw_response": { "...full TMDB API response..." },
    "created_at": "2026-02-10T08:00:00.000Z",
    "updated_at": "2026-02-12T09:30:00.000Z"
  },
  "source": "cache"
}
```

**Response (from TMDB):**
```json
{
  "show": {
    "...freshly fetched TV show data..."
  },
  "source": "tmdb"
}
```

**Response (stale cache):**
```json
{
  "show": {
    "...stale cached data..."
  },
  "source": "cache-stale"
}
```

**Response (not found):**
```json
{
  "show": null,
  "source": "not-found"
}
```

**Response (invalid ID):**
```json
{
  "show": null,
  "error": "Invalid TV show ID"
}
```

**Source Field Values:**

| Value | Meaning |
|-------|---------|
| `cache` | Returned from local database (fresh, within 24 hours) |
| `tmdb` | Freshly fetched from TMDB API and cached locally |
| `cache-stale` | Stale cache returned because TMDB could not be reached |
| `not-found` | Not found in cache or TMDB |

**Status Codes:**
- `200 OK` - Response returned (check `source` field for details)

**Notes:**
- The TMDB fetch includes appended data: `credits`, `videos`, and `external_ids`
- External IDs (TVDB, IMDb) are extracted from `external_ids` in the TMDB response
- Season and episode counts come from TMDB's top-level show metadata
- Genres are flattened from TMDB genre objects to a string array

---

### Endpoint Summary

| Method | Path | Description | Hits TMDB? | Caches? |
|--------|------|-------------|------------|---------|
| `GET` | `/health` | Health check | No | No |
| `GET` | `/v1/movies/search` | Search movies by title | **Yes** (always) | No |
| `GET` | `/v1/movies/:id` | Get movie details | Only if cache miss/stale | **Yes** |
| `GET` | `/v1/tv/search` | Search TV shows by title | **Yes** (always) | No |
| `GET` | `/v1/tv/:id` | Get TV show details | Only if cache miss/stale | **Yes** |

---

## Database Schema

### np_metaenrich_movies

Stores enriched movie metadata from TMDB with cache management.

```sql
CREATE TABLE IF NOT EXISTS np_metaenrich_movies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
  tmdb_id INT NOT NULL,
  imdb_id VARCHAR(20),
  title VARCHAR(500) NOT NULL,
  original_title VARCHAR(500),
  overview TEXT,
  release_date DATE,
  runtime INT,
  genres VARCHAR(50)[],
  vote_average DECIMAL(3,1),
  vote_count INT,
  poster_path VARCHAR(500),
  backdrop_path VARCHAR(500),
  raw_response JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, tmdb_id)
);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | NO | `gen_random_uuid()` | Primary key (auto-generated UUID) |
| `source_account_id` | VARCHAR(255) | NO | `'primary'` | Multi-app isolation column |
| `tmdb_id` | INT | NO | - | TMDB movie ID (unique per source account) |
| `imdb_id` | VARCHAR(20) | YES | - | IMDb ID (e.g., `tt1375666`) |
| `title` | VARCHAR(500) | NO | - | Movie title |
| `original_title` | VARCHAR(500) | YES | - | Original language title |
| `overview` | TEXT | YES | - | Plot synopsis / description |
| `release_date` | DATE | YES | - | Theatrical release date |
| `runtime` | INT | YES | - | Runtime in minutes |
| `genres` | VARCHAR(50)[] | YES | - | PostgreSQL array of genre name strings (e.g., `{"Action","Sci-Fi"}`) |
| `vote_average` | DECIMAL(3,1) | YES | - | Average TMDB rating (0.0-10.0) |
| `vote_count` | INT | YES | - | Total number of TMDB ratings |
| `poster_path` | VARCHAR(500) | YES | - | TMDB poster image path (prepend `https://image.tmdb.org/t/p/w500`) |
| `backdrop_path` | VARCHAR(500) | YES | - | TMDB backdrop image path (prepend `https://image.tmdb.org/t/p/w1280`) |
| `raw_response` | JSONB | NO | `'{}'` | Complete TMDB API response (includes credits, videos, release_dates) |
| `created_at` | TIMESTAMPTZ | NO | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | NO | `NOW()` | Last update timestamp (used for staleness checks) |

**Constraints:**
- `UNIQUE(source_account_id, tmdb_id)` - Ensures one record per TMDB movie per tenant

**Indexes:**

| Index | Columns | Purpose |
|-------|---------|---------|
| `idx_np_metaenrich_movies_tmdb` | `(source_account_id, tmdb_id)` | Fast lookup by TMDB ID within a tenant |
| `idx_np_metaenrich_movies_title` | `(source_account_id, title)` | Fast local title search within a tenant |

**Upsert Behavior:**
On conflict (`source_account_id, tmdb_id`), the upsert uses `COALESCE(EXCLUDED.column, existing.column)` for optional fields. This means:
- Non-null incoming values overwrite existing values
- Null incoming values preserve existing values (no data loss)
- `title` is always overwritten (not wrapped in COALESCE)
- `updated_at` is always set to `NOW()`

---

### np_metaenrich_tv_shows

Stores enriched TV show metadata from TMDB with cache management.

```sql
CREATE TABLE IF NOT EXISTS np_metaenrich_tv_shows (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(255) NOT NULL DEFAULT 'primary',
  tmdb_id INT NOT NULL,
  tvdb_id INT,
  imdb_id VARCHAR(20),
  name VARCHAR(500) NOT NULL,
  original_name VARCHAR(500),
  overview TEXT,
  first_air_date DATE,
  last_air_date DATE,
  number_of_seasons INT,
  number_of_episodes INT,
  genres VARCHAR(50)[],
  vote_average DECIMAL(3,1),
  vote_count INT,
  poster_path VARCHAR(500),
  backdrop_path VARCHAR(500),
  raw_response JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(source_account_id, tmdb_id)
);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | NO | `gen_random_uuid()` | Primary key (auto-generated UUID) |
| `source_account_id` | VARCHAR(255) | NO | `'primary'` | Multi-app isolation column |
| `tmdb_id` | INT | NO | - | TMDB TV show ID (unique per source account) |
| `tvdb_id` | INT | YES | - | TheTVDB show ID (extracted from TMDB external_ids) |
| `imdb_id` | VARCHAR(20) | YES | - | IMDb ID (e.g., `tt0903747`) |
| `name` | VARCHAR(500) | NO | - | TV show name |
| `original_name` | VARCHAR(500) | YES | - | Original language name |
| `overview` | TEXT | YES | - | Show synopsis / description |
| `first_air_date` | DATE | YES | - | Date of the first episode |
| `last_air_date` | DATE | YES | - | Date of the most recent episode |
| `number_of_seasons` | INT | YES | - | Total number of seasons |
| `number_of_episodes` | INT | YES | - | Total number of episodes across all seasons |
| `genres` | VARCHAR(50)[] | YES | - | PostgreSQL array of genre name strings (e.g., `{"Drama","Crime"}`) |
| `vote_average` | DECIMAL(3,1) | YES | - | Average TMDB rating (0.0-10.0) |
| `vote_count` | INT | YES | - | Total number of TMDB ratings |
| `poster_path` | VARCHAR(500) | YES | - | TMDB poster image path |
| `backdrop_path` | VARCHAR(500) | YES | - | TMDB backdrop image path |
| `raw_response` | JSONB | NO | `'{}'` | Complete TMDB API response (includes credits, videos, external_ids) |
| `created_at` | TIMESTAMPTZ | NO | `NOW()` | Record creation timestamp |
| `updated_at` | TIMESTAMPTZ | NO | `NOW()` | Last update timestamp (used for staleness checks) |

**Constraints:**
- `UNIQUE(source_account_id, tmdb_id)` - Ensures one record per TMDB TV show per tenant

**Indexes:**

| Index | Columns | Purpose |
|-------|---------|---------|
| `idx_np_metaenrich_tv_shows_tmdb` | `(source_account_id, tmdb_id)` | Fast lookup by TMDB ID within a tenant |
| `idx_np_metaenrich_tv_shows_name` | `(source_account_id, name)` | Fast local name search within a tenant |

**Upsert Behavior:**
Same as the movies table -- uses `COALESCE` for optional fields to avoid data loss on partial updates. The `name` field is always overwritten.

---

### Image URL Construction

TMDB image paths stored in `poster_path` and `backdrop_path` are relative paths. To construct full image URLs:

```
https://image.tmdb.org/t/p/{size}{path}
```

**Common Poster Sizes:** `w92`, `w154`, `w185`, `w342`, `w500`, `w780`, `original`

**Common Backdrop Sizes:** `w300`, `w780`, `w1280`, `original`

**Examples:**
```
Poster:   https://image.tmdb.org/t/p/w500/oYuLEt3zVCKq57qu2F8dT7NIa6f.jpg
Backdrop: https://image.tmdb.org/t/p/w1280/8ZTVqvKDQ8emSGUEMjsS4yHAwrp.jpg
```

---

## Caching System

### Cache-First Architecture

The metadata enrichment plugin implements a cache-first pattern for detail endpoints (`/v1/movies/:id` and `/v1/tv/:id`):

```
Request → Check Local Cache → Fresh? → Return cached data
                                  ↓ No (stale or missing)
                            Fetch from TMDB API
                                  ↓
                            Upsert into database
                                  ↓
                            Return fresh data
                                  ↓ TMDB unreachable?
                            Return stale cache (if available)
                                  ↓ No cache at all?
                            Return null with "not-found"
```

### Staleness Threshold

A cached record is considered **stale** when its `updated_at` timestamp is more than **24 hours** old. The staleness check is implemented in the server's `isStale()` method:

```typescript
private isStale(updatedAt: Date, maxAgeHours = 24): boolean {
  return Date.now() - new Date(updatedAt).getTime() > maxAgeHours * 60 * 60 * 1000;
}
```

This means:
- Records updated less than 24 hours ago are served directly from cache
- Records older than 24 hours trigger a fresh TMDB fetch
- If the TMDB fetch fails, the stale cached record is returned with `source: "cache-stale"`

### Search vs. Detail Caching

| Endpoint Type | Caching Behavior |
|---------------|-----------------|
| Search (`/v1/movies/search`, `/v1/tv/search`) | Never cached -- always hits TMDB API |
| Detail (`/v1/movies/:id`, `/v1/tv/:id`) | Cache-first with 24-hour staleness threshold |

Search endpoints are not cached because search results depend on TMDB's evolving index and ranking algorithm. Detail endpoints are cached because movie/show metadata changes infrequently.

### Local Database Search

In addition to the TMDB-backed search endpoints, the database layer provides local search methods (`searchMovies` and `searchTVShows`) that query the cached data using case-insensitive `ILIKE` matching, ordered by vote count descending, limited to 20 results. These are available for internal use but are not currently exposed via REST endpoints.

---

## Rate Limiting

### Outbound TMDB Rate Limiting

The `TMDBClient` implements a queue-based rate limiter for outbound requests to TMDB:

- **Maximum throughput:** 40 requests per second (TMDB allows 50; the plugin leaves 10 as headroom)
- **Mechanism:** Each request acquires a slot; slots are released after 1 second
- **Queuing:** If all 40 slots are in use, requests queue and wait for a slot to become available
- **No dropped requests:** All queued requests are eventually processed in FIFO order

This ensures the plugin never exceeds TMDB's rate limits, even under heavy concurrent usage.

### Inbound API Rate Limiting

The HTTP server uses the shared `ApiRateLimiter` from `@nself/plugin-utils` to limit inbound requests:

- **Default:** 100 requests per 60-second window
- **Configurable via:**
  - `METADATA_ENRICHMENT_RATE_LIMIT_MAX` (requests per window)
  - `METADATA_ENRICHMENT_RATE_LIMIT_WINDOW_MS` (window duration in ms)
- **Behavior:** Returns HTTP 429 when the limit is exceeded

---

## Multi-App Support

The plugin fully supports multi-tenant deployments via the `source_account_id` isolation column.

### How It Works

- Every database record includes a `source_account_id` column (default: `primary`)
- All queries filter by `source_account_id` to ensure tenant isolation
- The source account is determined by the `X-App-Name` request header (via `getAppContext()` from shared utilities)
- If no `X-App-Name` header is provided, the default value `primary` is used

### Configuration

```json
{
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  }
}
```

### Usage

```bash
# Request for default tenant
curl http://localhost:3203/v1/movies/27205

# Request for specific tenant
curl http://localhost:3203/v1/movies/27205 \
  -H "X-App-Name: tenant-a"

# Each tenant has its own isolated cache
curl http://localhost:3203/v1/movies/27205 \
  -H "X-App-Name: tenant-b"
```

Tenant `tenant-a` and `tenant-b` can each have their own cached copy of the same TMDB movie, with independent `updated_at` timestamps for independent staleness management.

---

## Examples

### Example 1: Search and Fetch a Movie

```bash
# Step 1: Search for a movie
curl "http://localhost:3203/v1/movies/search?q=Interstellar&year=2014"

# Response includes TMDB ID 157336
# Step 2: Fetch full details (cached automatically)
curl http://localhost:3203/v1/movies/157336

# Response includes full metadata with source: "tmdb"
# Step 3: Fetch again (served from cache)
curl http://localhost:3203/v1/movies/157336

# Response includes same data with source: "cache"
```

### Example 2: Search and Fetch a TV Show

```bash
# Step 1: Search for a TV show
curl "http://localhost:3203/v1/tv/search?q=The+Wire"

# Response includes TMDB ID 1438
# Step 2: Fetch full details
curl http://localhost:3203/v1/tv/1438

# Response includes TVDB ID, IMDb ID, season/episode counts, etc.
```

### Example 3: Multi-Tenant Usage

```bash
# Fetch for tenant "streaming-app-1"
curl http://localhost:3203/v1/movies/603 \
  -H "X-App-Name: streaming-app-1"

# Fetch for tenant "streaming-app-2" (independent cache)
curl http://localhost:3203/v1/movies/603 \
  -H "X-App-Name: streaming-app-2"
```

### Example 4: Query Cached Data via SQL

```sql
-- Find all cached movies with high ratings
SELECT
  title,
  tmdb_id,
  vote_average,
  vote_count,
  release_date,
  genres
FROM np_metaenrich_movies
WHERE source_account_id = 'primary'
  AND vote_average >= 8.0
  AND vote_count >= 1000
ORDER BY vote_average DESC;

-- Find all TV shows with more than 5 seasons
SELECT
  name,
  tmdb_id,
  number_of_seasons,
  number_of_episodes,
  first_air_date,
  last_air_date
FROM np_metaenrich_tv_shows
WHERE source_account_id = 'primary'
  AND number_of_seasons > 5
ORDER BY number_of_episodes DESC;

-- Find all stale cached movies (older than 24 hours)
SELECT
  title,
  tmdb_id,
  updated_at,
  NOW() - updated_at AS age
FROM np_metaenrich_movies
WHERE source_account_id = 'primary'
  AND updated_at < NOW() - INTERVAL '24 hours'
ORDER BY updated_at ASC;
```

### Example 5: Extract Data from raw_response

The `raw_response` JSONB column stores the complete TMDB API response, which includes fields not broken out into dedicated columns (such as credits, videos, budget, revenue, production companies, etc.):

```sql
-- Extract director from movie credits
SELECT
  title,
  jsonb_path_query(raw_response, '$.credits.crew[*] ? (@.job == "Director")') ->> 'name' AS director
FROM np_metaenrich_movies
WHERE source_account_id = 'primary'
  AND tmdb_id = 27205;

-- Extract top 5 cast members
SELECT
  title,
  jsonb_array_element(raw_response -> 'credits' -> 'cast', i) ->> 'name' AS actor,
  jsonb_array_element(raw_response -> 'credits' -> 'cast', i) ->> 'character' AS character
FROM np_metaenrich_movies,
     generate_series(0, 4) AS i
WHERE source_account_id = 'primary'
  AND tmdb_id = 27205;

-- Extract budget and revenue from raw_response
SELECT
  title,
  (raw_response ->> 'budget')::bigint AS budget,
  (raw_response ->> 'revenue')::bigint AS revenue,
  CASE
    WHEN (raw_response ->> 'budget')::bigint > 0
    THEN ROUND(((raw_response ->> 'revenue')::numeric / (raw_response ->> 'budget')::numeric), 2)
    ELSE NULL
  END AS roi
FROM np_metaenrich_movies
WHERE source_account_id = 'primary'
  AND raw_response ->> 'budget' IS NOT NULL;

-- Extract networks for TV shows
SELECT
  name,
  jsonb_array_elements(raw_response -> 'networks') ->> 'name' AS network
FROM np_metaenrich_tv_shows
WHERE source_account_id = 'primary'
  AND tmdb_id = 1396;
```

### Example 6: CLI Workflow

```bash
# Initialize schema on a fresh database
nself plugin metadata-enrichment init

# Search for movies from the command line
nself plugin metadata-enrichment search-movie "Blade Runner"
# Output:
# Found 5 movies:
#
# 1. Blade Runner (1982)
#    ID: 78 | Rating: 7.9/10
#
# 2. Blade Runner 2049 (2017)
#    ID: 335984 | Rating: 7.5/10

# Search for TV shows
nself plugin metadata-enrichment search-tv "The Sopranos"
# Output:
# Found 1 TV shows:
#
# 1. The Sopranos (1999)
#    ID: 1398 | Rating: 8.6/10

# Start server in background and query via API
nself plugin metadata-enrichment server &
curl http://localhost:3203/v1/movies/78 | jq '.movie.title, .movie.vote_average, .source'
# "Blade Runner"
# 7.9
# "tmdb"
```

---

## Troubleshooting

### Issue: "TMDB_API_KEY is required"

**Cause:** The `TMDB_API_KEY` environment variable is not set.

**Solution:**
1. Ensure you have a valid TMDB API key (see [Getting a TMDB API Key](#getting-a-tmdb-api-key))
2. Set it in your `.env` file or environment:
   ```bash
   export TMDB_API_KEY=your_api_key_here
   ```
3. If using a `.env` file, ensure it is in the working directory where the plugin is started

---

### Issue: "DATABASE_URL is required"

**Cause:** The `DATABASE_URL` environment variable is not set.

**Solution:**
```bash
export DATABASE_URL=postgresql://user:password@localhost:5432/nself
```

Ensure PostgreSQL is running and the database exists:
```bash
psql -c "SELECT 1;" $DATABASE_URL
```

---

### Issue: Movie or TV show search returns empty results

**Cause:** TMDB API call failed silently (the `TMDBClient` catches errors and returns empty arrays).

**Solution:**
1. Verify your TMDB API key is valid:
   ```bash
   curl "https://api.themoviedb.org/3/movie/550?api_key=$TMDB_API_KEY"
   ```
2. Check the logs for error messages:
   ```bash
   LOG_LEVEL=debug nself plugin metadata-enrichment search-movie "test"
   ```
3. Verify your network can reach `api.themoviedb.org`:
   ```bash
   curl -I https://api.themoviedb.org
   ```

---

### Issue: "429 Too Many Requests" from TMDB

**Cause:** Outbound rate limit exceeded despite the plugin's built-in rate limiter. This can happen if multiple instances are running with the same API key.

**Solution:**
1. Ensure only one instance of the plugin is running per TMDB API key
2. If running multiple instances, use different API keys or implement a shared rate limiter
3. The plugin's built-in limit is 40 req/s (TMDB allows 50); this should rarely be an issue with a single instance

---

### Issue: Stale data being returned

**Cause:** The `updated_at` timestamp is within the 24-hour freshness window, but the data has changed on TMDB.

**Solution:**
1. Wait for the 24-hour staleness window to expire (data will auto-refresh on next request)
2. Manually trigger a refresh by deleting the cached record:
   ```sql
   DELETE FROM np_metaenrich_movies
   WHERE source_account_id = 'primary' AND tmdb_id = 27205;
   ```
   Then re-fetch via the API:
   ```bash
   curl http://localhost:3203/v1/movies/27205
   ```
3. Update the staleness threshold by modifying the `maxAgeHours` parameter (requires code change -- this is not currently configurable via environment variables)

---

### Issue: "Invalid movie ID" or "Invalid TV show ID"

**Cause:** The `:id` path parameter is not a valid integer.

**Solution:**
Ensure you are passing a numeric TMDB ID, not a UUID or string:
```bash
# Correct
curl http://localhost:3203/v1/movies/27205

# Incorrect
curl http://localhost:3203/v1/movies/inception
curl http://localhost:3203/v1/movies/tt1375666  # This is an IMDb ID, not TMDB
```

Use the search endpoint to find the correct TMDB ID:
```bash
curl "http://localhost:3203/v1/movies/search?q=Inception"
# Look for the "id" field in results
```

---

### Issue: API returns 429 (inbound rate limit)

**Cause:** The inbound API rate limit has been exceeded.

**Solution:**
1. Increase the rate limit:
   ```bash
   export METADATA_ENRICHMENT_RATE_LIMIT_MAX=500
   export METADATA_ENRICHMENT_RATE_LIMIT_WINDOW_MS=60000
   ```
2. Implement client-side retry with exponential backoff
3. Cache responses on the client side to reduce request volume

---

### Issue: API returns 401 Unauthorized

**Cause:** API key authentication is enabled but the request is missing or has an incorrect `Authorization` header.

**Solution:**
1. Include the API key in your requests:
   ```bash
   curl http://localhost:3203/v1/movies/27205 \
     -H "Authorization: Bearer your_api_key"
   ```
2. Verify the API key matches `METADATA_ENRICHMENT_API_KEY` or `NSELF_API_KEY`
3. To disable authentication, unset both variables:
   ```bash
   unset METADATA_ENRICHMENT_API_KEY
   unset NSELF_API_KEY
   ```

---

### Issue: Database connection refused

**Cause:** PostgreSQL is not running or the connection string is incorrect.

**Solution:**
1. Verify PostgreSQL is running:
   ```bash
   pg_isready -h localhost -p 5432
   ```
2. Test the connection:
   ```bash
   psql $DATABASE_URL -c "SELECT 1;"
   ```
3. Check the connection string format:
   ```
   postgresql://username:password@hostname:port/database
   ```
4. If using Docker:
   ```bash
   docker ps | grep postgres
   ```

---

### Issue: Multi-app data leaking between tenants

**Cause:** The `X-App-Name` header is not being sent, causing all requests to default to `primary`.

**Solution:**
1. Always include `X-App-Name` in requests for multi-tenant deployments:
   ```bash
   curl http://localhost:3203/v1/movies/27205 \
     -H "X-App-Name: tenant-a"
   ```
2. Verify tenant isolation in the database:
   ```sql
   SELECT source_account_id, COUNT(*)
   FROM np_metaenrich_movies
   GROUP BY source_account_id;
   ```
3. Ensure your API gateway or reverse proxy forwards the `X-App-Name` header

---

## Differences from the TMDB Plugin

The nself ecosystem includes two TMDB-related plugins. Here is how they compare:

| Feature | Metadata Enrichment | [TMDB Plugin](./TMDB.md) |
|---------|-------------------|------------|
| **Purpose** | Lightweight caching proxy for TMDB lookups | Full media matching and management pipeline |
| **Port** | 3203 | 3020 |
| **Tables** | 2 | 6 |
| **Movie search** | Yes | Yes |
| **TV show search** | Yes | Yes |
| **Movie details** | Yes (cache-first) | Yes (cache-first) |
| **TV show details** | Yes (cache-first) | Yes (cache-first) |
| **Season/episode details** | No | Yes |
| **Filename parsing** | No | Yes (intelligent parsing) |
| **Match queue** | No | Yes (confidence-based auto/manual) |
| **Batch matching** | No | Yes |
| **Genre sync** | No | Yes |
| **OMDb integration** | No | Optional |
| **Configurable cache TTL** | No (24h hardcoded) | Yes (`TMDB_CACHE_TTL_DAYS`) |
| **Automatic refresh cron** | No | Yes (`TMDB_REFRESH_CRON`) |

**When to use Metadata Enrichment:** You need a simple, fast metadata lookup service without the complexity of matching pipelines or review queues. Ideal for applications that already know their TMDB IDs and just need to fetch and cache metadata.

**When to use the TMDB Plugin:** You need to identify media from filenames, manage a review queue for ambiguous matches, track seasons and episodes, or need full-featured media catalog management.

---

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- nself CLI: https://github.com/acamarata/nself

---

**Plugin Version:** 1.0.0
**Last Updated:** February 12, 2026
**Author:** nself
**License:** MIT
