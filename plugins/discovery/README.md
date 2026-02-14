# Discovery Plugin for nself

**Version**: 1.0.0
**Category**: media
**Port**: 3022

Content discovery feeds with trending, popular, recently added, and continue watching functionality. Uses a Redis cache-aside pattern for fast response times with automatic database fallback.

---

## Overview

The Discovery plugin provides real-time content feed endpoints for media platforms. It reads from existing `media_items`, `watch_progress`, and `user_ratings` tables and serves precomputed discovery feeds through a Fastify REST API.

### Key Features

- **Trending feed**: Score-weighted content based on recent views, ratings, and completion rates
- **Popular feed**: All-time most-watched content weighted by average rating
- **Recent feed**: Newest content ordered by creation date
- **Continue watching**: Per-user resume feed for partially watched content
- **Redis cache-aside**: Automatic caching with configurable TTLs per feed type
- **Graceful degradation**: Operates without Redis (direct database queries as fallback)
- **Multi-app isolation**: All queries support `source_account_id` filtering

---

## Quick Start

```bash
# Navigate to plugin directory
cd plugins/discovery/ts

# Install dependencies
pnpm install

# Build TypeScript
pnpm run build

# Set environment variables
export DATABASE_URL=postgresql://user:password@localhost:5432/nself
export REDIS_URL=redis://localhost:6379

# Initialize database schema
pnpm run cli init

# Start in development mode
pnpm run dev
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection string |
| `DISCOVERY_PORT` | No | `3022` | HTTP server port |
| `TRENDING_WINDOW_HOURS` | No | `24` | Hours to look back for trending |
| `DEFAULT_LIMIT` | No | `20` | Default result limit |
| `CACHE_TTL_TRENDING` | No | `900` | Trending cache TTL (seconds) |
| `CACHE_TTL_POPULAR` | No | `3600` | Popular cache TTL (seconds) |
| `CACHE_TTL_RECENT` | No | `1800` | Recent cache TTL (seconds) |
| `CACHE_TTL_CONTINUE` | No | `300` | Continue watching cache TTL (seconds) |
| `DISCOVERY_API_KEY` | No | - | API key for authentication |
| `DISCOVERY_RATE_LIMIT_MAX` | No | `100` | Max requests per window |

---

## API Endpoints

### GET /v1/trending

Returns trending content ranked by computed score within a configurable time window.

**Trending Score Formula:**
```
trending_score = (view_count * 0.50) + (avg_rating * 0.30) + (completion_rate * 0.20)
```

**Query Parameters:**
- `limit` (int, 1-100, default: 20) - Number of items to return
- `window_hours` (int, 1-720, default: 24) - Lookback window in hours
- `source_account_id` (string) - Filter by account

**Response:**
```json
{
  "items": [
    {
      "id": "uuid",
      "title": "Content Title",
      "type": "movie",
      "trending_score": 15.42,
      "view_count": 28,
      "avg_rating": 4.2,
      "completion_rate": 0.85,
      "thumbnail_url": "https://..."
    }
  ],
  "count": 20,
  "cached": true,
  "cached_at": "2026-02-14T12:00:00Z",
  "generated_at": "2026-02-14T12:00:01Z"
}
```

### GET /v1/popular

Returns popular content by total view count weighted by average rating.

**Query Parameters:**
- `limit` (int, 1-100, default: 20)
- `source_account_id` (string)

### GET /v1/recent

Returns recently added content ordered by creation date.

**Query Parameters:**
- `limit` (int, 1-100, default: 20)
- `source_account_id` (string)

### GET /v1/continue/:userId

Returns continue watching items for a specific user. Only includes items where progress is between 5% and 95%.

**Query Parameters:**
- `limit` (int, 1-100, default: 10)
- `source_account_id` (string)

### GET /health

Health check endpoint. Returns database and Redis connectivity status.

```json
{
  "status": "ok",
  "timestamp": "2026-02-14T12:00:00Z",
  "database": true,
  "redis": true,
  "version": "1.0.0"
}
```

### GET /v1/status

Detailed status including feed counts, cache state, and source table statistics.

### POST /v1/cache/invalidate

Invalidate Redis caches. Optionally target a specific feed.

**Body:**
```json
{ "feed": "trending" }
```

### POST /v1/cache/refresh

Refresh precomputed database cache tables (np_disc_trending_cache, np_disc_popular_cache).

**Body:**
```json
{ "source_account_id": "primary" }
```

---

## CLI Commands

```bash
# Initialize plugin and database schema
pnpm run cli init

# Display trending content
pnpm run cli trending --limit 10 --window 48

# Display popular content
pnpm run cli popular --limit 20

# Display recently added content
pnpm run cli recent --limit 15

# Display continue watching for a user
pnpm run cli continue user-123 --limit 5

# Clear all caches
pnpm run cli cache-clear

# Clear specific feed cache
pnpm run cli cache-clear --feed trending

# Show plugin status
pnpm run cli status
```

---

## Database Schema

### Source Tables (READ-ONLY)

These tables must exist from the main application. The discovery plugin reads from them but never writes:

- **media_items** - Content catalog with titles, types, thumbnails
- **watch_progress** - User viewing progress per media item
- **user_ratings** - User ratings (1-5 scale) per media item

### Cache Tables (owned by this plugin)

- **np_disc_trending_cache** - Precomputed trending scores per media item
- **np_disc_popular_cache** - Precomputed popularity scores per media item

### Views

- **np_disc_trending_live** - Live trending query (bypasses cache)
- **np_disc_popular_live** - Live popular query (bypasses cache)
- **np_disc_recent_live** - Live recent query (bypasses cache)

---

## Cache Architecture

The plugin uses a cache-aside (lazy-loading) pattern:

```
Request -> Check Redis -> Cache Hit -> Return cached data
                       -> Cache Miss -> Query PostgreSQL -> Store in Redis -> Return data
```

**TTL Configuration:**
| Feed | Default TTL | Redis Key Pattern |
|------|------------|-------------------|
| Trending | 15 min | `disc:trending:{account}:{limit}:{window}` |
| Popular | 1 hour | `disc:popular:{account}:{limit}` |
| Recent | 30 min | `disc:recent:{account}:{limit}` |
| Continue | 5 min | `disc:continue:{account}:{userId}:{limit}` |

**Graceful Degradation:**
If Redis is unavailable, all queries go directly to PostgreSQL. The plugin logs a warning and continues operating in degraded mode. When Redis becomes available again, caching resumes automatically.

---

## Development

```bash
pnpm run build        # Compile TypeScript
pnpm run watch        # Watch mode
pnpm run typecheck    # Type checking only
pnpm run dev          # Development server (tsx watch)
pnpm start            # Production server
```

---

## Troubleshooting

### No trending results

Ensure `watch_progress` records exist within the configured trending window (default 24h). Check with:
```sql
SELECT COUNT(*) FROM watch_progress WHERE last_watched_at >= NOW() - INTERVAL '24 hours';
```

### Redis connection refused

The plugin operates without Redis. Check Redis is running:
```bash
redis-cli ping
```

### Source tables not found

This plugin requires `media_items`, `watch_progress`, and `user_ratings` tables. These are created by the main application, not by this plugin.

---

## License

MIT License - See LICENSE file for details
