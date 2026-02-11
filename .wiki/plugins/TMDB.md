# TMDB Plugin

Media metadata enrichment from TMDB/IMDb with auto-matching and manual review queue

---

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Matching System](#matching-system)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The TMDB plugin provides comprehensive media metadata enrichment by syncing with The Movie Database (TMDB) and optionally OMDb. It features intelligent filename parsing, confidence-based auto-matching, and a manual review queue for ambiguous matches.

### Key Features
- **Intelligent Matching**: Automatic filename parsing with confidence scoring (85% threshold for auto-accept)
- **Manual Review Queue**: Queue system for matches requiring human review
- **Complete Coverage**: Movies, TV shows, seasons, episodes, and genres
- **Rich Metadata**: Cast, crew, ratings, release dates, posters, backdrops, keywords
- **IMDb Integration**: Cross-reference with IMDb IDs
- **Batch Operations**: Batch matching and refresh workflows
- **Multi-language**: Configurable language for metadata (default: en-US)
- **Cache Management**: 30-day TTL with automatic refresh

### Use Cases
- Media library metadata enrichment
- Automatic movie/TV show identification from filenames
- Content catalog management
- Streaming platform metadata backend
- Recommendation engine data source

---

## Quick Start

```bash
# Install
nself plugin install tmdb

# Configure (minimal .env)
cat > .env <<EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
TMDB_API_KEY=your_tmdb_api_key_here
EOF

# Initialize
nself plugin tmdb init

# Start server
nself plugin tmdb server

# Match a media file
nself plugin tmdb match \
  --media-id "file123" \
  --filename "The.Matrix.1999.1080p.BluRay.x264.mkv"
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `TMDB_PLUGIN_PORT` | No | `3020` | HTTP server port |
| `TMDB_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `TMDB_LOG_LEVEL` | No | `info` | Logging level (debug\|info\|warn\|error) |
| `TMDB_APP_IDS` | No | `primary` | Comma-separated app IDs for multi-tenancy |
| `TMDB_API_KEY` | **Yes** | - | TMDB API key (v3) |
| `TMDB_API_READ_ACCESS_TOKEN` | No | - | TMDB v4 read access token |
| `OMDB_API_KEY` | No | - | OMDb API key for additional metadata |
| `TMDB_AUTO_ACCEPT_THRESHOLD` | No | `0.85` | Confidence threshold for auto-accepting matches (0-1) |
| `TMDB_FILENAME_PARSING` | No | `true` | Enable intelligent filename parsing |
| `TMDB_DEFAULT_LANGUAGE` | No | `en-US` | Default language for metadata |
| `TMDB_CACHE_TTL_DAYS` | No | `30` | Days before cached metadata becomes stale |
| `TMDB_REFRESH_CRON` | No | `0 6 * * 0` | Cron for automatic metadata refresh (weekly Sunday 6am) |
| `TMDB_IMAGE_BASE_URL` | No | `https://image.tmdb.org/t/p/` | TMDB image CDN base URL |
| `TMDB_POSTER_SIZE` | No | `w500` | Poster image size (w92\|w154\|w185\|w342\|w500\|w780\|original) |
| `TMDB_BACKDROP_SIZE` | No | `w1280` | Backdrop image size |
| `TMDB_RATE_LIMIT_REQUESTS` | No | `35` | Max requests per window |
| `TMDB_RATE_LIMIT_WINDOW_MS` | No | `10000` | Rate limit window in ms (TMDB allows ~40 req/10s) |

### Example .env File
```bash
# Database
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Server
TMDB_PLUGIN_PORT=3020
TMDB_LOG_LEVEL=info

# TMDB API (Get from https://www.themoviedb.org/settings/api)
TMDB_API_KEY=your_tmdb_v3_api_key_here
TMDB_API_READ_ACCESS_TOKEN=optional_v4_token

# OMDb (Optional - Get from http://www.omdbapi.com/apikey.aspx)
OMDB_API_KEY=your_omdb_api_key

# Matching
TMDB_AUTO_ACCEPT_THRESHOLD=0.85
TMDB_DEFAULT_LANGUAGE=en-US
TMDB_CACHE_TTL_DAYS=30

# Multi-app support
TMDB_APP_IDS=app1,app2,primary
```

### Getting API Keys

#### TMDB API Key
1. Create account at https://www.themoviedb.org/
2. Go to Settings > API
3. Request an API key (v3 auth)
4. Copy the API Key (v3 auth)

#### OMDb API Key (Optional)
1. Go to http://www.omdbapi.com/apikey.aspx
2. Select free tier and enter email
3. Activate via email confirmation
4. Copy API key from email

---

## CLI Commands

### Initialize Database
```bash
nself plugin tmdb init
```
Creates all required database tables and indexes.

### Start Server
```bash
nself plugin tmdb server
```
Starts the HTTP API server on configured port (default: 3020).

### Search
Search TMDB for movies or TV shows:
```bash
# Search for movies
nself plugin tmdb search \
  --query "The Matrix" \
  --type movie \
  --year 1999

# Search for TV shows
nself plugin tmdb search \
  --query "Breaking Bad" \
  --type tv

# Multi-app context
nself plugin tmdb search \
  --query "Inception" \
  --app-id app1
```

### Match Media File
Match a media file to TMDB:
```bash
# Auto-match from filename
nself plugin tmdb match \
  --media-id "file_12345" \
  --filename "The.Matrix.1999.1080p.BluRay.x264.mkv"

# Provide explicit title and year
nself plugin tmdb match \
  --media-id "file_12345" \
  --title "The Matrix" \
  --year 1999 \
  --type movie

# TV show episode
nself plugin tmdb match \
  --media-id "ep_001" \
  --filename "Breaking.Bad.S01E01.Pilot.1080p.mkv" \
  --type tv
```

### View Match Queue
View pending matches requiring review:
```bash
# Pending matches
nself plugin tmdb queue --status pending

# Accepted matches
nself plugin tmdb queue --status accepted

# All statuses
nself plugin tmdb queue --limit 100
```

### Confirm Match
Manually confirm or reject a match:
```bash
# Confirm a match
nself plugin tmdb confirm \
  --match-id "uuid-here" \
  --tmdb-id 603 \
  --tmdb-type movie
```

### Refresh Metadata
Refresh cached metadata from TMDB:
```bash
# Refresh specific movie
nself plugin tmdb refresh \
  --type movie \
  --id 603

# Refresh specific TV show
nself plugin tmdb refresh \
  --type tv \
  --id 1396
```

### Sync Genres
Sync genre list from TMDB:
```bash
nself plugin tmdb sync
```

### Status
View plugin status and statistics:
```bash
nself plugin tmdb status
```

---

## REST API

### Base URL
```
http://localhost:3020
```

### Health Endpoints

#### GET /health
Health check
```http
GET /health
```

Response:
```json
{
  "status": "ok",
  "plugin": "tmdb",
  "timestamp": "2025-01-24T12:00:00.000Z",
  "version": "1.0.0"
}
```

#### GET /ready
Readiness check
```http
GET /ready
```

#### GET /live
Liveness with stats
```http
GET /live
```

### Search Endpoints

#### GET /api/search/movie
Search for movies
```http
GET /api/search/movie?query=The%20Matrix&year=1999&language=en-US
```

Response:
```json
{
  "results": [
    {
      "id": 603,
      "title": "The Matrix",
      "overview": "Set in the 22nd century...",
      "releaseDate": "1999-03-30",
      "posterPath": "https://image.tmdb.org/t/p/w500/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
      "voteAverage": 8.2,
      "matchScore": 0.95
    }
  ],
  "total": 47
}
```

#### GET /api/search/tv
Search for TV shows
```http
GET /api/search/tv?query=Breaking%20Bad
```

#### GET /api/search/multi
Search both movies and TV shows
```http
GET /api/search/multi?query=Matrix
```

### Metadata Endpoints

#### GET /api/movie/:id
Get movie details (with caching)
```http
GET /api/movie/603
Headers:
  X-App-Name: primary
```

Response:
```json
{
  "id": 603,
  "imdb_id": "tt0133093",
  "title": "The Matrix",
  "original_title": "The Matrix",
  "overview": "Set in the 22nd century...",
  "tagline": "Welcome to the Real World.",
  "release_date": "1999-03-30",
  "runtime": 136,
  "status": "Released",
  "poster_path": "/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
  "backdrop_path": "/fNG7i7RqMErkcqhohV2a6cV1Ehy.jpg",
  "budget": 63000000,
  "revenue": 463517383,
  "vote_average": 8.2,
  "vote_count": 24315,
  "popularity": 142.483,
  "original_language": "en",
  "genres": [{"id": 28, "name": "Action"}, {"id": 878, "name": "Science Fiction"}],
  "production_companies": [...],
  "credits": {"cast": [...], "crew": [...]},
  "keywords": [{"id": 1721, "name": "fight"}, ...],
  "content_rating": "R",
  "synced_at": "2025-01-24T12:00:00.000Z"
}
```

#### GET /api/tv/:id
Get TV show details
```http
GET /api/tv/1396
Headers:
  X-App-Name: primary
```

#### GET /api/tv/:id/season/:num
Get season details with episodes
```http
GET /api/tv/1396/season/1
Headers:
  X-App-Name: primary
```

#### GET /api/tv/:id/season/:seasonNum/episode/:episodeNum
Get episode details
```http
GET /api/tv/1396/season/1/episode/1
Headers:
  X-App-Name: primary
```

### Matching Endpoints

#### POST /api/match
Match a media file to TMDB
```http
POST /api/match
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "mediaId": "file_12345",
  "filename": "The.Matrix.1999.1080p.BluRay.x264.mkv",
  "title": "The Matrix",  // optional
  "year": 1999,           // optional
  "type": "movie"         // optional: movie|tv
}
```

Response:
```json
{
  "matchQueueId": "550e8400-e29b-41d4-a716-446655440000",
  "bestMatch": {
    "id": 603,
    "title": "The Matrix",
    "confidence": 0.95,
    "autoAccepted": true
  },
  "alternatives": [
    {
      "id": 604,
      "title": "The Matrix Reloaded",
      "confidence": 0.72
    }
  ]
}
```

#### POST /api/match/batch
Batch match multiple files
```http
POST /api/match/batch
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "items": [
    {
      "mediaId": "file_001",
      "filename": "The.Matrix.1999.1080p.mkv"
    },
    {
      "mediaId": "file_002",
      "filename": "The.Matrix.Reloaded.2003.1080p.mkv"
    }
  ]
}
```

Response:
```json
{
  "processed": 2,
  "autoAccepted": 2,
  "needsReview": 0
}
```

#### GET /api/match/queue
Get match review queue
```http
GET /api/match/queue?status=pending&limit=50&offset=0
Headers:
  X-App-Name: primary
```

#### PUT /api/match/:id/confirm
Confirm a match
```http
PUT /api/match/550e8400-e29b-41d4-a716-446655440000/confirm
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "tmdbId": 603,
  "tmdbType": "movie"
}
```

#### PUT /api/match/:id/reject
Reject a match
```http
PUT /api/match/550e8400-e29b-41d4-a716-446655440000/reject
Headers:
  X-App-Name: primary
```

### Refresh Endpoints

#### POST /api/refresh/:type/:id
Refresh cached metadata
```http
POST /api/refresh/movie/603
Headers:
  X-App-Name: primary
```

Response:
```json
{
  "refreshed": true,
  "changed": ["voteAverage", "voteCount", "popularity"]
}
```

#### POST /api/refresh/all
Queue stale metadata for refresh
```http
POST /api/refresh/all
Headers:
  Content-Type: application/json
  X-App-Name: primary
Body:
{
  "olderThanDays": 30
}
```

### Images Endpoint

#### GET /api/images/:type/:id
Get all images for media
```http
GET /api/images/movie/603
```

Response:
```json
{
  "posters": [
    {
      "path": "https://image.tmdb.org/t/p/original/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
      "width": 2000,
      "height": 3000,
      "language": "en"
    }
  ],
  "backdrops": [...],
  "logos": [...]
}
```

### Configuration Endpoint

#### GET /api/config
Get TMDB configuration (image sizes, etc.)
```http
GET /api/config
```

### Sync & Status Endpoints

#### POST /api/sync
Sync genres from TMDB
```http
POST /api/sync
Headers:
  X-App-Name: primary
```

#### GET /api/status
Get plugin status
```http
GET /api/status
Headers:
  X-App-Name: primary
```

Response:
```json
{
  "movies": 1523,
  "tvShows": 287,
  "seasons": 1849,
  "episodes": 12483,
  "genres": 38,
  "matchQueue": {
    "pending": 15,
    "accepted": 1203,
    "rejected": 42,
    "manual": 8
  },
  "lastSynced": "2025-01-24T10:30:00.000Z"
}
```

---

## Database Schema

### tmdb_movies

Full movie metadata from TMDB.

```sql
CREATE TABLE tmdb_movies (
  id INTEGER PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  imdb_id VARCHAR(20),
  title VARCHAR(500) NOT NULL,
  original_title VARCHAR(500),
  overview TEXT,
  tagline TEXT,
  release_date DATE,
  runtime INTEGER,
  status VARCHAR(50),
  poster_path VARCHAR(255),
  backdrop_path VARCHAR(255),
  budget BIGINT,
  revenue BIGINT,
  vote_average DOUBLE PRECISION,
  vote_count INTEGER,
  popularity DOUBLE PRECISION,
  original_language VARCHAR(10),
  genres JSONB DEFAULT '[]',
  production_companies JSONB DEFAULT '[]',
  production_countries JSONB DEFAULT '[]',
  spoken_languages JSONB DEFAULT '[]',
  credits JSONB DEFAULT '{}',
  keywords JSONB DEFAULT '[]',
  content_rating VARCHAR(20),
  synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_tmdb_movies_source_app ON tmdb_movies(source_account_id);
CREATE INDEX idx_tmdb_movies_imdb ON tmdb_movies(imdb_id);
CREATE INDEX idx_tmdb_movies_title ON tmdb_movies(title);
```

**Columns:**
| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| id | INTEGER | NO | TMDB movie ID (primary key) |
| source_account_id | VARCHAR(128) | NO | Multi-app isolation |
| imdb_id | VARCHAR(20) | YES | IMDb ID (e.g., tt0133093) |
| title | VARCHAR(500) | NO | Movie title |
| original_title | VARCHAR(500) | YES | Original language title |
| overview | TEXT | YES | Plot synopsis |
| tagline | TEXT | YES | Marketing tagline |
| release_date | DATE | YES | Release date |
| runtime | INTEGER | YES | Runtime in minutes |
| status | VARCHAR(50) | YES | Release status (Released, Post Production, etc.) |
| poster_path | VARCHAR(255) | YES | Poster image path |
| backdrop_path | VARCHAR(255) | YES | Backdrop image path |
| budget | BIGINT | YES | Production budget (USD) |
| revenue | BIGINT | YES | Box office revenue (USD) |
| vote_average | DOUBLE PRECISION | YES | Average rating (0-10) |
| vote_count | INTEGER | YES | Number of ratings |
| popularity | DOUBLE PRECISION | YES | TMDB popularity score |
| original_language | VARCHAR(10) | YES | ISO 639-1 language code |
| genres | JSONB | NO | Array of genre objects |
| production_companies | JSONB | NO | Array of company objects |
| production_countries | JSONB | NO | Array of country objects |
| spoken_languages | JSONB | NO | Array of language objects |
| credits | JSONB | NO | Cast and crew |
| keywords | JSONB | NO | Array of keyword objects |
| content_rating | VARCHAR(20) | YES | MPAA rating (G, PG, PG-13, R, NC-17) |
| synced_at | TIMESTAMPTZ | NO | Last sync timestamp |

### tmdb_tv_shows

Full TV show metadata from TMDB.

```sql
CREATE TABLE tmdb_tv_shows (
  id INTEGER PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  imdb_id VARCHAR(20),
  name VARCHAR(500) NOT NULL,
  original_name VARCHAR(500),
  overview TEXT,
  first_air_date DATE,
  last_air_date DATE,
  status VARCHAR(50),
  type VARCHAR(50),
  number_of_seasons INTEGER,
  number_of_episodes INTEGER,
  episode_run_time INTEGER[],
  poster_path VARCHAR(255),
  backdrop_path VARCHAR(255),
  vote_average DOUBLE PRECISION,
  vote_count INTEGER,
  popularity DOUBLE PRECISION,
  original_language VARCHAR(10),
  genres JSONB DEFAULT '[]',
  networks JSONB DEFAULT '[]',
  created_by JSONB DEFAULT '[]',
  credits JSONB DEFAULT '{}',
  content_rating VARCHAR(20),
  synced_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_tmdb_tv_source_app ON tmdb_tv_shows(source_account_id);
CREATE INDEX idx_tmdb_tv_imdb ON tmdb_tv_shows(imdb_id);
```

### tmdb_tv_seasons

TV season metadata.

```sql
CREATE TABLE tmdb_tv_seasons (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  show_id INTEGER NOT NULL REFERENCES tmdb_tv_shows(id),
  season_number INTEGER NOT NULL,
  name VARCHAR(500),
  overview TEXT,
  poster_path VARCHAR(255),
  air_date DATE,
  episode_count INTEGER,
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(show_id, season_number)
);

CREATE INDEX idx_tmdb_seasons_source_app ON tmdb_tv_seasons(source_account_id);
```

### tmdb_tv_episodes

TV episode metadata.

```sql
CREATE TABLE tmdb_tv_episodes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  show_id INTEGER NOT NULL REFERENCES tmdb_tv_shows(id),
  season_number INTEGER NOT NULL,
  episode_number INTEGER NOT NULL,
  name VARCHAR(500),
  overview TEXT,
  still_path VARCHAR(255),
  air_date DATE,
  runtime INTEGER,
  vote_average DOUBLE PRECISION,
  crew JSONB DEFAULT '[]',
  guest_stars JSONB DEFAULT '[]',
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(show_id, season_number, episode_number)
);

CREATE INDEX idx_tmdb_episodes_source_app ON tmdb_tv_episodes(source_account_id);
```

### tmdb_genres

Genre reference data.

```sql
CREATE TABLE tmdb_genres (
  id INTEGER PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(100) NOT NULL,
  media_type VARCHAR(10) NOT NULL
);

CREATE INDEX idx_tmdb_genres_source_app ON tmdb_genres(source_account_id);
```

### tmdb_match_queue

Match review queue for ambiguous matches.

```sql
CREATE TABLE tmdb_match_queue (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  media_id VARCHAR(255) NOT NULL,
  filename VARCHAR(500),
  parsed_title VARCHAR(500),
  parsed_year INTEGER,
  parsed_type VARCHAR(20),
  match_results JSONB DEFAULT '[]',
  best_match_id INTEGER,
  best_match_type VARCHAR(20),
  confidence DOUBLE PRECISION,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  reviewed_by VARCHAR(255),
  reviewed_at TIMESTAMPTZ,
  auto_accepted BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_tmdb_match_source_app ON tmdb_match_queue(source_account_id);
CREATE INDEX idx_tmdb_match_status ON tmdb_match_queue(source_account_id, status);
CREATE INDEX idx_tmdb_match_media ON tmdb_match_queue(source_account_id, media_id);
```

**Status values:** `pending`, `accepted`, `rejected`, `manual`

### tmdb_webhook_events

Internal webhook event log.

```sql
CREATE TABLE tmdb_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMPTZ,
  error TEXT,
  retry_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_tmdb_webhook_events_source_app ON tmdb_webhook_events(source_account_id);
```

---

## Matching System

### Filename Parsing

The plugin intelligently parses filenames to extract:
- **Title**: Cleaned media title
- **Year**: 4-digit year
- **Type**: Movie or TV show
- **Season/Episode**: For TV shows (S01E05, 1x05 patterns)

**Examples:**
- `The.Matrix.1999.1080p.BluRay.x264.mkv` → `{title: "The Matrix", year: 1999, type: "movie"}`
- `Breaking.Bad.S01E01.Pilot.1080p.mkv` → `{title: "Breaking Bad", season: 1, episode: 1, type: "tv"}`
- `Game.of.Thrones.1x01.Winter.is.Coming.mkv` → `{title: "Game of Thrones", season: 1, episode: 1, type: "tv"}`

### Confidence Scoring

Matches are scored 0-1 based on:
- **Title Match** (0.6): Exact match bonus, partial match penalty
- **Year Match** (0.3): Exact year, ±1 year tolerance
- **Popularity** (0.1): Boost for popular titles

**Auto-accept threshold:** 0.85 (configurable)

### Match Workflow

1. **Submit match request** with filename or explicit metadata
2. **Filename parsing** extracts title, year, type
3. **TMDB search** finds candidates
4. **Confidence scoring** ranks results
5. **Auto-accept** if best match ≥ 0.85 confidence
6. **Queue for review** if confidence < 0.85
7. **Manual review** via CLI or API
8. **Metadata fetch** on confirmation

---

## Examples

### Example 1: Auto-Match Movie
```bash
# Submit match
curl -X POST http://localhost:3020/api/match \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "mediaId": "file_12345",
    "filename": "The.Matrix.1999.1080p.BluRay.x264.mkv"
  }'

# Response shows auto-accepted
{
  "matchQueueId": "550e8400-e29b-41d4-a716-446655440000",
  "bestMatch": {
    "id": 603,
    "title": "The Matrix",
    "confidence": 0.95,
    "autoAccepted": true
  },
  "alternatives": []
}

# Fetch full metadata
curl http://localhost:3020/api/movie/603 \
  -H "X-App-Name: primary"
```

### Example 2: Manual Review Workflow
```bash
# Submit ambiguous match
curl -X POST http://localhost:3020/api/match \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "mediaId": "file_67890",
    "filename": "Matrix.2003.mkv"
  }'

# Response shows needs review (could be Matrix Reloaded or Matrix Revolutions)
{
  "matchQueueId": "661f9510-f39c-52e5-b827-557766551111",
  "bestMatch": {
    "id": 604,
    "title": "The Matrix Reloaded",
    "confidence": 0.72,
    "autoAccepted": false
  },
  "alternatives": [
    {"id": 605, "title": "The Matrix Revolutions", "confidence": 0.71}
  ]
}

# Check queue
curl http://localhost:3020/api/match/queue?status=pending \
  -H "X-App-Name: primary"

# Confirm correct match
curl -X PUT http://localhost:3020/api/match/661f9510-f39c-52e5-b827-557766551111/confirm \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{"tmdbId": 604, "tmdbType": "movie"}'
```

### Example 3: Batch Match
```bash
curl -X POST http://localhost:3020/api/match/batch \
  -H "Content-Type: application/json" \
  -H "X-App-Name: primary" \
  -d '{
    "items": [
      {
        "mediaId": "file_001",
        "filename": "The.Matrix.1999.1080p.mkv"
      },
      {
        "mediaId": "file_002",
        "filename": "The.Matrix.Reloaded.2003.1080p.mkv"
      },
      {
        "mediaId": "file_003",
        "filename": "The.Matrix.Revolutions.2003.1080p.mkv"
      }
    ]
  }'

# Response
{
  "processed": 3,
  "autoAccepted": 3,
  "needsReview": 0
}
```

### Example 4: TV Show Season Sync
```sql
-- Query all episodes for Breaking Bad Season 1
SELECT
  e.episode_number,
  e.name,
  e.air_date,
  e.runtime,
  e.vote_average
FROM tmdb_tv_episodes e
WHERE e.show_id = 1396
  AND e.season_number = 1
  AND e.source_account_id = 'primary'
ORDER BY e.episode_number;
```

### Example 5: Find Top Rated Movies
```sql
-- Top 10 highest rated movies with at least 1000 votes
SELECT
  title,
  vote_average,
  vote_count,
  release_date,
  popularity
FROM tmdb_movies
WHERE source_account_id = 'primary'
  AND vote_count >= 1000
ORDER BY vote_average DESC
LIMIT 10;
```

---

## Troubleshooting

### Issue: "No active signing key configured"
This error appears in the TMDB plugin by mistake (copy-paste from tokens plugin). Ignore this error message. TMDB doesn't use signing keys.

### Issue: "429 Too Many Requests"
**Cause:** Exceeded TMDB API rate limit (40 requests per 10 seconds).

**Solution:**
- Reduce `TMDB_RATE_LIMIT_REQUESTS` in .env
- Implement backoff in batch operations
- Upgrade to TMDB v4 API (higher limits)

### Issue: Incorrect Matches
**Cause:** Filename parsing fails or confidence threshold too low.

**Solutions:**
- Provide explicit `title` and `year` instead of relying on filename
- Increase `TMDB_AUTO_ACCEPT_THRESHOLD` to be more conservative (e.g., 0.90)
- Review pending matches in queue
- Use manual confirmation workflow

### Issue: Missing IMDb IDs
**Cause:** Not all TMDB entries have IMDb cross-references.

**Solution:**
- Use `OMDB_API_KEY` for fallback IMDb lookup
- Use TMDB external IDs endpoint (future enhancement)

### Issue: Stale Metadata
**Cause:** Cached data hasn't been refreshed.

**Solutions:**
```bash
# Refresh specific movie
nself plugin tmdb refresh --type movie --id 603

# Queue all stale metadata for refresh
curl -X POST http://localhost:3020/api/refresh/all \
  -H "Content-Type: application/json" \
  -d '{"olderThanDays": 30}'
```

### Issue: Multi-App Isolation Not Working
**Verify configuration:**
```bash
# Check app IDs
echo $TMDB_APP_IDS

# Verify X-App-Name header in requests
curl http://localhost:3020/api/status \
  -H "X-App-Name: app1" \
  -v
```

### Issue: Images Not Loading
**Check image URLs:**
- Verify `TMDB_IMAGE_BASE_URL` is set to `https://image.tmdb.org/t/p/`
- Check `poster_path` and `backdrop_path` are not null
- Ensure image size exists: `w92`, `w154`, `w185`, `w342`, `w500`, `w780`, `original`

---

**Plugin Version:** 1.0.0
**Last Updated:** February 11, 2026
**Author:** nself
**License:** Source-Available
