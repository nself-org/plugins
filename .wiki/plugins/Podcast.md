# Podcast Plugin

Podcast service with RSS feed parsing, episode management, playback position synchronization, and subscription management. Build your own podcast player or listening platform.

| Property | Value |
|----------|-------|
| **Port** | `3210` |
| **Category** | `media` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run podcast init
nself plugin run podcast server
```

---

## Features

- **RSS Feed Parsing** - Automatic parsing of podcast RSS/Atom feeds
- **Subscription Management** - Subscribe to podcasts, track new episodes
- **Episode Tracking** - Mark episodes as played, favorite, archived
- **Playback Sync** - Sync playback position across devices
- **Auto-Update** - Periodic RSS feed polling for new episodes
- **Rich Metadata** - Episode descriptions, artwork, show notes, chapters
- **Search** - Search podcasts and episodes by title, description, author
- **Categories** - Organize podcasts by category/genre

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PODCAST_PLUGIN_PORT` | `3210` | Server port |
| `PODCAST_RSS_POLL_INTERVAL` | `60` | RSS poll interval in minutes |
| `PODCAST_MAX_EPISODES` | `500` | Maximum episodes to fetch per feed |
| `PODCAST_STORAGE_BACKEND` | `database` | Episode storage backend (`database`, `s3`, `local`) |

---

## Installation

```bash
# Install plugin
nself plugin install podcast

# Initialize database
nself plugin run podcast init

# Start server
nself plugin run podcast server
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (5 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`) |
| `feeds` | List subscribed feeds (`--limit`, `--offset`) |
| `subscribe` | Subscribe to a podcast (`--url`, `--auto-download?`) |
| `episodes` | List episodes for a feed (`--feed-id`, `--unplayed?`) |
| `sync` | Force RSS feed sync for all subscriptions |
| `search` | Search podcasts (`--query`, `--limit`) |
| `stats` | Show statistics (feeds, episodes, disk usage) |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |

### Podcast Subscriptions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/podcasts/subscribe` | Subscribe to podcast (body: `rss_url`, `auto_download?`, `category?`) |
| `GET` | `/api/podcasts` | List subscriptions (query: `limit?`, `offset?`, `category?`) |
| `GET` | `/api/podcasts/:id` | Get podcast details with episode count |
| `DELETE` | `/api/podcasts/:id` | Unsubscribe from podcast |
| `PUT` | `/api/podcasts/:id` | Update subscription (body: `auto_download?`, `category?`, `is_active?`) |
| `POST` | `/api/podcasts/:id/sync` | Force sync for a specific podcast |

### Episodes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/podcasts/:id/episodes` | List episodes for a podcast (query: `limit?`, `offset?`, `status?`) |
| `GET` | `/api/episodes/:id` | Get episode details |
| `PUT` | `/api/episodes/:id/status` | Update episode status (body: `status`: `unplayed`, `played`, `archived`, `favorited`) |
| `GET` | `/api/episodes/recent` | Get recent episodes across all subscriptions (query: `limit?`, `days?`) |
| `GET` | `/api/episodes/unplayed` | Get unplayed episodes |

### Playback Position

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/episodes/:id/position` | Get playback position |
| `PUT` | `/api/episodes/:id/position` | Update playback position (body: `position_seconds`, `duration_seconds?`, `device?`) |
| `DELETE` | `/api/episodes/:id/position` | Reset playback position |

### Search

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/search/podcasts` | Search podcasts (query: `q` (required), `limit?`, `offset?`) |
| `GET` | `/api/search/episodes` | Search episodes (query: `q` (required), `podcast_id?`, `limit?`, `offset?`) |

### Categories

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/categories` | List all podcast categories |
| `GET` | `/api/categories/:id/podcasts` | List podcasts in a category |

### Sync

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/sync/all` | Sync all subscribed podcasts |
| `GET` | `/api/sync/status` | Get sync status (last sync time, queue length) |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `podcast.subscribed` | New podcast subscription created |
| `podcast.unsubscribed` | Podcast subscription removed |
| `podcast.synced` | Podcast RSS feed synced |
| `episode.new` | New episode discovered |
| `episode.played` | Episode marked as played |
| `episode.favorited` | Episode favorited |
| `playback.position.updated` | Playback position synced |

---

## Database Schema

### `np_podcast_podcasts`

Podcast subscriptions and feed metadata.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Podcast ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `rss_url` | `TEXT` | RSS feed URL |
| `title` | `VARCHAR(500)` | Podcast title |
| `description` | `TEXT` | Podcast description |
| `author` | `VARCHAR(255)` | Podcast author/publisher |
| `image_url` | `TEXT` | Podcast artwork URL |
| `language` | `VARCHAR(20)` | Language code (e.g., `en`) |
| `website` | `TEXT` | Podcast website URL |
| `category_id` | `UUID` (FK) | References `np_podcast_categories` |
| `last_synced_at` | `TIMESTAMPTZ` | Last RSS sync timestamp |
| `auto_download` | `BOOLEAN` | Whether to auto-download new episodes |
| `is_active` | `BOOLEAN` | Whether subscription is active |
| `episode_count` | `INTEGER` | Total episodes in feed |
| `metadata` | `JSONB` | Additional RSS metadata |
| `created_at` | `TIMESTAMPTZ` | Subscription timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_podcast_episodes`

Individual podcast episodes.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Episode ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `podcast_id` | `UUID` (FK) | References `np_podcast_podcasts` |
| `guid` | `VARCHAR(512)` | RSS GUID (unique identifier) |
| `title` | `VARCHAR(500)` | Episode title |
| `description` | `TEXT` | Episode description/show notes |
| `audio_url` | `TEXT` | Episode audio file URL |
| `duration_seconds` | `INTEGER` | Episode duration |
| `file_size_bytes` | `BIGINT` | Audio file size |
| `mime_type` | `VARCHAR(100)` | Audio MIME type (e.g., `audio/mpeg`) |
| `published_at` | `TIMESTAMPTZ` | Episode publish date |
| `season_number` | `INTEGER` | Season number (if applicable) |
| `episode_number` | `INTEGER` | Episode number |
| `episode_type` | `VARCHAR(50)` | `full`, `trailer`, `bonus` |
| `image_url` | `TEXT` | Episode-specific artwork |
| `chapters` | `JSONB` | Chapter markers (Podcast Namespace) |
| `transcript_url` | `TEXT` | Transcript URL |
| `metadata` | `JSONB` | Additional episode metadata |
| `created_at` | `TIMESTAMPTZ` | First seen timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_podcast_subscriptions`

Per-user subscription state and preferences.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Subscription ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | User who subscribed |
| `podcast_id` | `UUID` (FK) | References `np_podcast_podcasts` |
| `status` | `VARCHAR(20)` | `unplayed`, `played`, `archived`, `favorited` |
| `last_episode_played_id` | `UUID` | Last episode listened to |
| `auto_download` | `BOOLEAN` | User-specific auto-download preference |
| `notifications_enabled` | `BOOLEAN` | Whether to notify on new episodes |
| `sort_order` | `INTEGER` | User's custom sort order |
| `created_at` | `TIMESTAMPTZ` | Subscription timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_podcast_playback_positions`

Playback position sync across devices.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Position record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | User identifier |
| `episode_id` | `UUID` (FK) | References `np_podcast_episodes` |
| `position_seconds` | `INTEGER` | Current playback position |
| `duration_seconds` | `INTEGER` | Total episode duration |
| `progress_percent` | `DECIMAL(5,2)` | Playback progress (0-100) |
| `device_id` | `VARCHAR(255)` | Device identifier |
| `device_name` | `VARCHAR(255)` | Device display name |
| `completed` | `BOOLEAN` | Whether episode was fully played |
| `last_synced_at` | `TIMESTAMPTZ` | Last sync timestamp |
| `created_at` | `TIMESTAMPTZ` | First play timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last position update |

Unique constraint: `(source_account_id, user_id, episode_id)`

### `np_podcast_categories`

Podcast categories/genres.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Category ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(128)` | Category name |
| `slug` | `VARCHAR(100)` | URL-friendly identifier |
| `description` | `TEXT` | Category description |
| `podcast_count` | `INTEGER` | Number of podcasts in category |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

---

## Usage Examples

### Subscribe to Podcasts

```bash
# Subscribe via CLI
nself plugin run podcast subscribe --url "https://feeds.example.com/podcast.xml"

# Subscribe via API
curl -X POST http://localhost:3210/api/podcasts/subscribe \
  -H "Content-Type: application/json" \
  -d '{
    "rss_url": "https://feeds.example.com/podcast.xml",
    "auto_download": true,
    "category": "Technology"
  }'
```

### List Episodes

```bash
# Get recent unplayed episodes
curl "http://localhost:3210/api/episodes/unplayed?limit=20"

# Get episodes for a specific podcast
curl "http://localhost:3210/api/podcasts/{podcast_id}/episodes?limit=50"

# CLI list episodes
nself plugin run podcast episodes --feed-id {podcast_id}
```

### Sync Playback Position

```bash
# Update playback position
curl -X PUT http://localhost:3210/api/episodes/{episode_id}/position \
  -H "Content-Type: application/json" \
  -d '{
    "position_seconds": 1250,
    "duration_seconds": 3600,
    "device": "iPhone 14"
  }'

# Get current position
curl http://localhost:3210/api/episodes/{episode_id}/position
```

### Search

```bash
# Search podcasts
curl "http://localhost:3210/api/search/podcasts?q=technology&limit=10"

# Search episodes
curl "http://localhost:3210/api/search/episodes?q=interview&limit=20"

# CLI search
nself plugin run podcast search --query "JavaScript"
```

### Sync Feeds

```bash
# Sync all subscriptions
curl -X POST http://localhost:3210/api/sync/all

# Sync specific podcast
curl -X POST http://localhost:3210/api/podcasts/{podcast_id}/sync

# CLI sync
nself plugin run podcast sync
```

---

## RSS Feed Polling

The plugin automatically polls RSS feeds at `PODCAST_RSS_POLL_INTERVAL` (default: 60 minutes).

### Sync Behavior

1. Fetch RSS/Atom feed
2. Parse feed XML
3. Extract podcast metadata (title, author, image, etc.)
4. Parse episodes (GUID, title, audio URL, duration, etc.)
5. Insert new episodes (based on GUID uniqueness)
6. Update `last_synced_at` timestamp
7. Trigger `episode.new` webhooks for new episodes

### Manual Sync

```bash
# Force immediate sync for all feeds
nself plugin run podcast sync

# Sync specific feed
curl -X POST http://localhost:3210/api/podcasts/{podcast_id}/sync
```

---

## Playback Position Sync

Playback positions sync across devices in real-time:

```
[Mobile App] → [API: Update Position] → [Database]
                                            ↓
[Desktop App] ← [API: Get Position] ← [Database]
```

### Progress Calculation

```
progress_percent = (position_seconds / duration_seconds) * 100
completed = (progress_percent >= 95)
```

Episodes are marked complete at 95% to account for outros/credits.

---

## Episode Status Workflow

| Status | Description | User Action |
|--------|-------------|-------------|
| `unplayed` | New episode, not started | Default |
| `played` | Episode completed | Mark as played |
| `archived` | Removed from main feed | Archive |
| `favorited` | Saved for later | Favorite |

```bash
# Mark episode as played
curl -X PUT http://localhost:3210/api/episodes/{id}/status \
  -d '{"status":"played"}'

# Favorite episode
curl -X PUT http://localhost:3210/api/episodes/{id}/status \
  -d '{"status":"favorited"}'
```

---

## RSS Feed Support

### Supported Feed Formats

- **RSS 2.0** - Standard podcast format
- **Atom** - Alternative feed format
- **iTunes Tags** - iTunes podcast extensions
- **Podcast Namespace** - Modern podcast features (chapters, transcripts, funding)

### Parsed Elements

| Element | RSS Path | Description |
|---------|----------|-------------|
| Title | `channel/title` | Podcast name |
| Description | `channel/description` | Podcast summary |
| Image | `channel/image/url` or `itunes:image` | Artwork |
| Author | `itunes:author` or `channel/author` | Creator |
| Category | `itunes:category` | Genre |
| Episode GUID | `item/guid` | Unique identifier |
| Audio URL | `item/enclosure@url` | MP3/M4A file |
| Duration | `itunes:duration` | Episode length |
| Chapters | `podcast:chapters` | Chapter markers |

---

## Troubleshooting

**"Feed not syncing"** -- Check RSS URL is accessible. Verify `PODCAST_RSS_POLL_INTERVAL` is set. Manually trigger sync with `POST /api/sync/all`.

**"Episodes not appearing"** -- Ensure RSS feed contains valid `<item>` elements with `<enclosure>` audio URLs. Check `last_synced_at` timestamp.

**"Duplicate episodes"** -- Plugin uses RSS GUID for deduplication. Verify feed provides unique GUIDs for each episode.

**"Playback position not syncing"** -- Ensure `user_id` and `episode_id` are correct. Check that position updates include `position_seconds`.

**"Search returns no results"** -- Database uses full-text search. Ensure `title` and `description` fields are populated. Search is case-insensitive.

**"Auto-download not working"** -- Currently `auto_download` is a preference flag only. Implement download logic in your application using the `episode.new` webhook.

**"Large database size"** -- Set `PODCAST_MAX_EPISODES` to limit episodes per feed. Archive old episodes with `status='archived'`. Consider cleanup job for episodes older than 1 year.

---

## Performance

- **RSS parsing** - Uses streaming XML parser (low memory)
- **Sync frequency** - Default 60 minutes balances freshness vs. load
- **Search** - Indexed on `title`, `description`, `author` columns
- **Storage** - Episode metadata only (audio files not stored by default)
- **Cleanup** - Configured `cleanupOldEpisodesDays: 365` in plugin.json

### Optimization Tips

```bash
# Reduce sync frequency for large feeds
PODCAST_RSS_POLL_INTERVAL=180  # 3 hours

# Limit episodes per feed
PODCAST_MAX_EPISODES=100

# Enable selective sync (only active subscriptions)
curl -X POST http://localhost:3210/api/sync/all?active_only=true
```

---

## Advanced Configuration

### Using with nself Backend

Add to your `.env.dev`:

```bash
# Enable podcast plugin
PODCAST_PLUGIN_ENABLED=true
PODCAST_PLUGIN_PORT=3210

# Configure sync
PODCAST_RSS_POLL_INTERVAL=60
PODCAST_MAX_EPISODES=500
PODCAST_STORAGE_BACKEND=database
```

### Custom Storage Backend

While `PODCAST_STORAGE_BACKEND` supports `database`, `s3`, `local`:

```bash
# S3 storage (for episode audio files)
PODCAST_STORAGE_BACKEND=s3
AWS_S3_BUCKET=podcast-episodes
AWS_REGION=us-east-1

# Local filesystem
PODCAST_STORAGE_BACKEND=local
PODCAST_LOCAL_STORAGE_PATH=/var/podcast-episodes
```

### Building a Podcast Player

```javascript
// Frontend example (React/Vue/Svelte)
const API_BASE = 'http://localhost:3210/api';

// 1. List subscriptions
const podcasts = await fetch(`${API_BASE}/podcasts`).then(r => r.json());

// 2. Get recent episodes
const episodes = await fetch(`${API_BASE}/episodes/recent?limit=20`).then(r => r.json());

// 3. Play episode
const audio = new Audio(episodes[0].audio_url);
audio.play();

// 4. Sync position every 10 seconds
setInterval(() => {
  fetch(`${API_BASE}/episodes/${episodes[0].id}/position`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      position_seconds: Math.floor(audio.currentTime),
      duration_seconds: Math.floor(audio.duration),
      device: 'Web Player'
    })
  });
}, 10000);

// 5. Mark complete when finished
audio.addEventListener('ended', () => {
  fetch(`${API_BASE}/episodes/${episodes[0].id}/status`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status: 'played' })
  });
});
```

---

## Podcast Namespace Support

The plugin supports [Podcast Namespace](https://github.com/Podcastindex-org/podcast-namespace) tags:

| Tag | Description | Support |
|-----|-------------|---------|
| `podcast:chapters` | Chapter markers | ✅ Parsed to `episodes.chapters` JSONB |
| `podcast:transcript` | Episode transcript | ✅ Stored in `transcript_url` |
| `podcast:funding` | Donation links | ✅ Stored in `metadata` JSONB |
| `podcast:person` | Hosts/guests | ✅ Stored in `metadata` JSONB |
| `podcast:season` | Season number | ✅ Stored in `season_number` |
| `podcast:episode` | Episode number | ✅ Stored in `episode_number` |

---

## Privacy & Data

- **RSS feeds are public** - No authentication required
- **Playback positions are private** - Scoped to `user_id`
- **No telemetry** - Plugin doesn't report usage to feed publishers
- **Local metadata** - All data stored in your PostgreSQL database
- **Audio files** - Not downloaded by default (streamed from feed URLs)

---

## Future Enhancements

Potential features for future versions:

- **Offline downloads** - Download episodes for offline playback
- **Smart playlists** - Auto-generated playlists based on preferences
- **Recommendations** - Suggest podcasts based on listening history
- **Social features** - Share episodes, follow other listeners
- **Variable speed playback** - Store speed preference per podcast
- **Silence trimming** - Skip intro/outro silence automatically
