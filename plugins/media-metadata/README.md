# TMDB Media Metadata Plugin

Production-ready TMDB integration plugin for nself. Provides intelligent media metadata enrichment with automatic matching, confidence scoring, and comprehensive database storage.

## Features

- **Smart Lookup**: Fuzzy matching with confidence scoring (Levenshtein distance algorithm)
- **Auto Enrichment**: Automatically fetch and store metadata from TMDB
- **Match Queue**: Manual review system for low-confidence matches
- **Full Data Sync**: Store movies, TV shows, seasons, episodes, and genres
- **Multi-Account Support**: Isolate data by source_account_id
- **Rate Limiting**: Automatic rate limiting (4 req/sec to respect TMDB limits)
- **REST API**: Complete HTTP API for all operations
- **CLI Tools**: Full command-line interface

## Quick Start

```bash
# Install dependencies
cd plugins/media-metadata/ts
npm install

# Build
npm run build

# Configure
cp .env.example .env
# Edit .env with your TMDB_API_KEY

# Initialize database
npm run init

# Start server
npm start
# Server runs on http://localhost:3202
```

## Configuration

### Required
- `TMDB_API_KEY` - Your TMDB API key ([Get one here](https://www.themoviedb.org/settings/api))

### Optional
- `TMDB_PLUGIN_PORT` - Server port (default: 3202)
- `TMDB_CONFIDENCE_THRESHOLD` - Match confidence threshold (default: 0.70)
- `TMDB_DEFAULT_LANGUAGE` - Language for metadata (default: en-US)
- `TMDB_AUTO_ENRICH` - Auto-fetch metadata on lookup (default: true)
- `TMDB_CACHE_TTL_DAYS` - Days to cache metadata (default: 30)

## Database Schema

### Tables (7)

1. **tmdb_movies** - Complete movie metadata
2. **tmdb_tv_shows** - TV show metadata
3. **tmdb_tv_seasons** - Season information
4. **tmdb_tv_episodes** - Episode details
5. **tmdb_genres** - Genre list (movies + TV)
6. **tmdb_match_queue** - Items pending manual matching
7. **tmdb_webhook_events** - Event log (for future webhook support)

All tables support multi-account isolation via `source_account_id`.

## CLI Commands

```bash
# Initialize schema
nself-media-metadata init

# Start server
nself-media-metadata server -p 3202

# Show status
nself-media-metadata status

# Search TMDB
nself-media-metadata search "The Matrix" -t movie -y 1999

# Lookup with confidence scoring
nself-media-metadata lookup "Inception" -t movie -y 2010

# Enrich (lookup + fetch + store)
nself-media-metadata enrich "Breaking Bad" -t tv

# Sync genres
nself-media-metadata sync-genres

# View match queue
nself-media-metadata match-queue -s manual_review

# Statistics
nself-media-metadata stats
```

## REST API Endpoints

### Search & Lookup

- `GET /v1/search?query=...&media_type=movie&year=2010` - Search TMDB
- `POST /v1/lookup` - Lookup with confidence scoring
  ```json
  { "title": "The Matrix", "year": 1999, "media_type": "movie" }
  ```
- `POST /v1/lookup/batch` - Batch lookup
- `POST /v1/enrich` - Enrich (fetch + store)

### Movies

- `GET /v1/movies/:tmdbId` - Get movie by TMDB ID
- `GET /v1/movies/trending` - Trending movies
- `GET /v1/movies/popular` - Popular movies
- `POST /v1/sync/movie/:tmdbId` - Force sync movie

### TV Shows

- `GET /v1/tv/:tmdbId` - Get TV show by TMDB ID
- `GET /v1/tv/:tmdbId/season/:num` - Get season with episodes
- `GET /v1/tv/:tmdbId/season/:num/episode/:epNum` - Get episode
- `GET /v1/tv/trending` - Trending TV shows
- `GET /v1/tv/popular` - Popular TV shows
- `POST /v1/sync/tv/:tmdbId` - Force sync TV show

### Genres & Metadata

- `GET /v1/genres?media_type=movie` - List genres
- `POST /v1/sync/genres` - Sync genre list from TMDB

### Match Queue

- `GET /v1/match-queue?status=pending` - List match queue
- `POST /v1/match-queue/:id/match` - Confirm match
- `POST /v1/match-queue/:id/reject` - Reject match

### Status

- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Plugin status
- `GET /v1/stats` - Detailed statistics

## Confidence Scoring

The lookup algorithm uses multiple factors:

- **Title Similarity (70%)**: Levenshtein distance with normalization
- **Year Match (25%)**: Exact match = 0.25, ±1 year = 0.15, ±2 years = 0.05
- **Media Type (5%)**: Bonus for matching type

### Scoring Thresholds

- **≥ 0.70**: Automatic match (configurable threshold)
- **0.50-0.69**: Manual review (added to match queue)
- **< 0.50**: No match

## Examples

### Enrich a Movie

```bash
curl -X POST http://localhost:3202/v1/enrich \
  -H "Content-Type: application/json" \
  -d '{
    "title": "The Matrix",
    "year": 1999,
    "media_type": "movie"
  }'
```

Response:
```json
{
  "success": true,
  "tmdb_id": 603,
  "media_type": "movie",
  "cached": false,
  "metadata": {
    "tmdb_id": 603,
    "imdb_id": "tt0133093",
    "title": "The Matrix",
    "release_date": "1999-03-31",
    "runtime_minutes": 136,
    "vote_average": 8.2,
    "genres": ["Action", "Science Fiction"],
    "cast": [...],
    "crew": [...]
  }
}
```

### Batch Lookup

```bash
curl -X POST http://localhost:3202/v1/lookup/batch \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      { "title": "Inception", "year": 2010, "media_type": "movie" },
      { "title": "Breaking Bad", "media_type": "tv" }
    ]
  }'
```

### Get TV Season

```bash
curl http://localhost:3202/v1/tv/1396/season/1
```

Returns season 1 of Breaking Bad with all episodes.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Media Metadata Plugin                     │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │  Client  │  │  Lookup  │  │ Database │  │  Server  │    │
│  │          │  │  Service │  │          │  │          │    │
│  │ - TMDB   │  │ - Fuzzy  │  │ - 7      │  │ - REST   │    │
│  │   API    │  │   Match  │  │   Tables │  │   API    │    │
│  │ - Rate   │  │ - Scoring│  │ - Upsert │  │ - Multi  │    │
│  │   Limit  │  │ - Queue  │  │ - Stats  │  │   Account│    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Development

```bash
# Watch mode
npm run watch

# Type checking
npm run typecheck

# Dev server (with auto-reload)
npm run dev
```

## Rate Limiting

TMDB API has a limit of **40 requests per 10 seconds**. This plugin automatically rate limits to **4 requests per second** to stay well within limits.

## Multi-Account Support

Isolate data by source account:

```bash
# Set X-Source-Account-Id header
curl -H "X-Source-Account-Id: myapp" \
  http://localhost:3202/v1/movies/603
```

All data is scoped to the source account ID automatically.

## Security

Optional API key authentication:

```bash
# Set in .env
TMDB_API_KEY_AUTH=your-secret-key

# Use in requests
curl -H "Authorization: Bearer your-secret-key" \
  http://localhost:3202/v1/search?query=Matrix
```

## License

Source-Available License

## Links

- [TMDB API Documentation](https://developers.themoviedb.org/3)
- [Plugin Repository](https://github.com/acamarata/nself-plugins)
- [nself CLI](https://github.com/acamarata/nself)
