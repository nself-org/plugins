# Podcast Plugin

Podcast RSS/Atom feed parsing, episode management, feed discovery, OPML import/export, and audio downloads for nself.

## Features

- **RSS/Atom Feed Parsing** - Parse RSS 2.0, Atom, and RDF feeds with podcast namespace extensions (chapters, transcripts)
- **Feed Discovery** - Search for podcasts via iTunes Search API and Podcast Index API
- **Episode Management** - Track play state, position, and download status per episode
- **OPML Import/Export** - Import and export podcast subscriptions in standard OPML format
- **Audio Downloads** - Download episode audio files with resume support
- **Adaptive Refresh** - Smart refresh intervals based on feed activity (active/dormant/stale)
- **Multi-App Isolation** - Full `source_account_id` support for multi-tenant deployments

## Quick Start

```bash
# Install dependencies
cd plugins/podcast/ts
pnpm install

# Build
pnpm run build

# Initialize database
pnpm run start -- init

# Start server
pnpm run start
# Server runs on http://localhost:3023
```

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PODCAST_PORT` | No | `3023` | Server port |
| `PODCAST_ITUNES_SEARCH_URL` | No | `https://itunes.apple.com/search` | iTunes Search API URL |
| `PODCAST_INDEX_API_KEY` | No | - | Podcast Index API key |
| `PODCAST_INDEX_API_SECRET` | No | - | Podcast Index API secret |
| `PODCAST_REFRESH_ACTIVE_MINUTES` | No | `60` | Refresh interval for active feeds (minutes) |
| `PODCAST_REFRESH_DORMANT_HOURS` | No | `6` | Refresh interval for dormant feeds (hours) |
| `PODCAST_DOWNLOAD_PATH` | No | `/data/podcasts` | Base path for episode downloads |

## CLI Commands

```bash
nself-podcast init                          # Initialize database
nself-podcast server                        # Start HTTP server
nself-podcast status                        # Show statistics
nself-podcast feeds list                    # List subscriptions
nself-podcast feeds add <url>               # Subscribe to feed
nself-podcast feeds remove <id>             # Unsubscribe
nself-podcast feeds show <id>               # Feed detail
nself-podcast episodes new                  # Unplayed episodes
nself-podcast episodes list <feedId>        # Episodes for a feed
nself-podcast episodes show <id>            # Episode detail
nself-podcast discover <query>              # Search for podcasts
nself-podcast refresh                       # Refresh all feeds
nself-podcast refresh <feedId>              # Refresh specific feed
nself-podcast import <file.opml>            # Import OPML file
nself-podcast export                        # Export OPML to stdout
nself-podcast export -o subs.opml           # Export OPML to file
nself-podcast download <episodeId>          # Download episode audio
```

## REST API

### Feeds

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/feeds` | Subscribe `{ url, title? }` |
| `GET` | `/v1/feeds` | List subscriptions |
| `GET` | `/v1/feeds/:id` | Feed detail with recent episodes |
| `DELETE` | `/v1/feeds/:id` | Unsubscribe |
| `POST` | `/v1/feeds/:id/refresh` | Force refresh feed |
| `GET` | `/v1/feeds/:id/episodes` | List episodes `?limit=50&offset=0` |

### Episodes

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/episodes/:id` | Episode detail |
| `POST` | `/v1/episodes/:id/download` | Download episode audio |
| `GET` | `/v1/new-episodes` | Unplayed episodes `?limit=50` |

### Discovery & OPML

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/discover` | Search `{ query, limit? }` |
| `POST` | `/v1/import/opml` | Import `{ opml_content }` |
| `POST` | `/v1/export/opml` | Export subscriptions as OPML |

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Liveness check |
| `GET` | `/ready` | Readiness check (DB connectivity) |
| `GET` | `/v1/stats` | Feed and episode statistics |

## Database Schema

### np_pod_feeds

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(255) | Multi-app isolation |
| `url` | TEXT | Feed URL (unique per account) |
| `title` | TEXT | Podcast title |
| `description` | TEXT | Podcast description |
| `author` | TEXT | Author name |
| `image_url` | TEXT | Cover art URL |
| `language` | VARCHAR(10) | Language code |
| `categories` | TEXT[] | iTunes categories |
| `last_fetched_at` | TIMESTAMPTZ | Last successful fetch |
| `last_episode_at` | TIMESTAMPTZ | Most recent episode date |
| `fetch_interval_minutes` | INTEGER | Adaptive refresh interval |
| `error_count` | INTEGER | Consecutive fetch errors |
| `last_error` | TEXT | Last error message |
| `status` | VARCHAR(20) | active, paused, error |
| `created_at` | TIMESTAMPTZ | Subscription date |

### np_pod_episodes

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(255) | Multi-app isolation |
| `feed_id` | UUID | FK to np_pod_feeds |
| `guid` | TEXT | Episode GUID (unique per feed) |
| `title` | TEXT | Episode title |
| `description` | TEXT | Episode description/notes |
| `pub_date` | TIMESTAMPTZ | Publication date |
| `duration_seconds` | INTEGER | Duration in seconds |
| `enclosure_url` | TEXT | Audio file URL |
| `enclosure_type` | VARCHAR(100) | MIME type |
| `enclosure_length` | BIGINT | File size in bytes |
| `season_number` | INTEGER | Season number |
| `episode_number` | INTEGER | Episode number |
| `episode_type` | VARCHAR(20) | full, trailer, bonus |
| `chapters_url` | TEXT | Chapters URL (podcast:chapters) |
| `transcript_url` | TEXT | Transcript URL (podcast:transcript) |
| `image_url` | TEXT | Episode art URL |
| `played` | BOOLEAN | Playback status |
| `play_position_seconds` | INTEGER | Playback position |
| `downloaded` | BOOLEAN | Download status |
| `download_path` | TEXT | Local file path |

## Feed Refresh Strategy

| Feed Activity | Interval | Description |
|---------------|----------|-------------|
| Active | 60 min | New content within last 7 days |
| Dormant | 6 hours | 7-30 days since last episode |
| Stale | 24 hours | Over 30 days since last episode |
| Error | Daily | After 7 consecutive failures, status set to `error` |

## License

Source-Available
