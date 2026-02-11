# Content Progress Plugin

Track video, audio, and content playback progress with continue watching, watchlists, and favorites.

## Features

- **Progress Tracking**: Track playback position for movies, TV episodes, videos, audio, articles, and courses
- **Continue Watching**: Automatically generate "continue watching" lists for users
- **Watchlists**: Let users save content to watch later with priorities and notes
- **Favorites**: Mark and retrieve favorite content
- **Cross-Device Resume**: Resume playback from the last position across devices
- **Watch History**: Track all playback events for analytics
- **User Statistics**: View total watch time, completion rates, and more
- **Multi-App Support**: Isolate data by source_account_id for multi-tenant applications

## Quick Start

### Installation

```bash
cd plugins/content-progress/ts
npm install
npm run build
```

### Configuration

Copy the example environment file and configure:

```bash
cp .env.example .env
# Edit .env with your database credentials
```

Required environment variables:
- `DATABASE_URL` or individual `POSTGRES_*` variables

Optional settings:
- `PROGRESS_PLUGIN_PORT` (default: 3022)
- `PROGRESS_COMPLETE_THRESHOLD` (default: 95%)
- `PROGRESS_HISTORY_SAMPLE_SECONDS` (default: 30)
- `PROGRESS_API_KEY` (for authentication)

### Initialize Database

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
npm run dev          # Development mode with hot reload
npm start            # Production mode
```

## CLI Commands

### Server Management

```bash
# Initialize database schema
nself-content-progress init

# Start API server
nself-content-progress server
nself-content-progress server --port 3022 --host 0.0.0.0

# Show plugin status
nself-content-progress status
```

### Progress Management

```bash
# List user's progress
nself-content-progress progress list <userId>

# Show specific progress
nself-content-progress progress show <userId> <contentType> <contentId>

# Update progress
nself-content-progress progress update <userId> <contentType> <contentId> \
  --position 120 --duration 3600

# Mark as completed
nself-content-progress progress complete <userId> <contentType> <contentId>

# Delete progress
nself-content-progress progress delete <userId> <contentType> <contentId>
```

### Watchlist Management

```bash
# List user's watchlist
nself-content-progress watchlist list <userId>

# Add to watchlist
nself-content-progress watchlist add <userId> <contentType> <contentId> \
  --priority 10 --notes "Recommended by friend"

# Remove from watchlist
nself-content-progress watchlist remove <userId> <contentType> <contentId>
```

### Favorites Management

```bash
# List user's favorites
nself-content-progress favorites list <userId>

# Add to favorites
nself-content-progress favorites add <userId> <contentType> <contentId>

# Remove from favorites
nself-content-progress favorites remove <userId> <contentType> <contentId>
```

### Statistics

```bash
# View user statistics
nself-content-progress stats <userId>
```

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health Checks

```bash
GET /health          # Basic health check
GET /ready           # Database readiness check
GET /live            # Detailed liveness with stats
GET /v1/status       # Plugin status and configuration
```

### Progress Endpoints

```bash
# Update playback position
POST /v1/progress
{
  "user_id": "user123",
  "content_type": "movie",
  "content_id": "movie-456",
  "position_seconds": 120.5,
  "duration_seconds": 7200,
  "device_id": "device-xyz",
  "audio_track": "en",
  "subtitle_track": "en",
  "quality": "1080p",
  "metadata": {}
}

# Get all progress for user
GET /v1/progress/:userId?limit=100&offset=0

# Get specific progress
GET /v1/progress/:userId/:contentType/:contentId

# Delete progress
DELETE /v1/progress/:userId/:contentType/:contentId

# Mark as completed
POST /v1/progress/:userId/:contentType/:contentId/complete

# Continue watching (in-progress, not completed, sorted by recent)
GET /v1/continue-watching/:userId?limit=20

# Recently watched (all items, sorted by recent)
GET /v1/recently-watched/:userId?limit=50
```

### History Endpoints

```bash
# Get user's watch history
GET /v1/history/:userId?limit=100&offset=0
```

### Watchlist Endpoints

```bash
# Add to watchlist
POST /v1/watchlist
{
  "user_id": "user123",
  "content_type": "movie",
  "content_id": "movie-456",
  "priority": 10,
  "added_from": "recommendations",
  "notes": "Must watch"
}

# Get user's watchlist
GET /v1/watchlist/:userId?limit=100&offset=0

# Update watchlist item
PUT /v1/watchlist/:userId/:contentType/:contentId
{
  "priority": 20,
  "notes": "Updated notes"
}

# Remove from watchlist
DELETE /v1/watchlist/:userId/:contentType/:contentId
```

### Favorites Endpoints

```bash
# Add to favorites
POST /v1/favorites
{
  "user_id": "user123",
  "content_type": "movie",
  "content_id": "movie-456"
}

# Get user's favorites
GET /v1/favorites/:userId?limit=100&offset=0

# Remove from favorites
DELETE /v1/favorites/:userId/:contentType/:contentId
```

### Statistics Endpoints

```bash
# Get user statistics
GET /v1/stats/:userId

Response:
{
  "total_watch_time_seconds": 86400,
  "total_watch_time_hours": 24,
  "content_completed": 42,
  "content_in_progress": 8,
  "watchlist_count": 15,
  "favorites_count": 23,
  "most_watched_type": "movie",
  "recent_activity": "2026-02-11T12:00:00Z"
}
```

## Database Schema

### progress_positions

Tracks current playback position for each user+content combination.

- `id`: UUID (primary key)
- `source_account_id`: VARCHAR(128) - Multi-app isolation
- `user_id`: VARCHAR(255) - User identifier
- `content_type`: VARCHAR(64) - movie, episode, video, audio, article, course
- `content_id`: VARCHAR(255) - Content identifier
- `position_seconds`: DOUBLE PRECISION - Current position
- `duration_seconds`: DOUBLE PRECISION - Total duration
- `progress_percent`: DOUBLE PRECISION - Calculated percentage
- `completed`: BOOLEAN - Auto-set when progress >= threshold
- `completed_at`: TIMESTAMPTZ - When marked completed
- `device_id`: VARCHAR(255) - Last device used
- `audio_track`: VARCHAR(16) - Audio track preference
- `subtitle_track`: VARCHAR(16) - Subtitle preference
- `quality`: VARCHAR(16) - Quality preference
- `metadata`: JSONB - Custom metadata
- `updated_at`: TIMESTAMPTZ - Last update
- `created_at`: TIMESTAMPTZ - First tracked

### progress_history

Historical playback events for analytics.

- `id`: UUID (primary key)
- `source_account_id`: VARCHAR(128)
- `user_id`: VARCHAR(255)
- `content_type`: VARCHAR(64)
- `content_id`: VARCHAR(255)
- `action`: VARCHAR(16) - play, pause, seek, complete, resume
- `position_seconds`: DOUBLE PRECISION
- `device_id`: VARCHAR(255)
- `session_id`: VARCHAR(255)
- `created_at`: TIMESTAMPTZ

### progress_watchlists

User watchlists with priorities.

- `id`: UUID (primary key)
- `source_account_id`: VARCHAR(128)
- `user_id`: VARCHAR(255)
- `content_type`: VARCHAR(64)
- `content_id`: VARCHAR(255)
- `priority`: INTEGER - Higher = more important
- `added_from`: VARCHAR(64) - Source of addition
- `notes`: TEXT - User notes
- `created_at`: TIMESTAMPTZ

### progress_favorites

User favorite content.

- `id`: UUID (primary key)
- `source_account_id`: VARCHAR(128)
- `user_id`: VARCHAR(255)
- `content_type`: VARCHAR(64)
- `content_id`: VARCHAR(255)
- `created_at`: TIMESTAMPTZ

### progress_webhook_events

Webhook event log for debugging.

- `id`: VARCHAR(255) (primary key)
- `source_account_id`: VARCHAR(128)
- `event_type`: VARCHAR(128)
- `payload`: JSONB
- `processed`: BOOLEAN
- `processed_at`: TIMESTAMPTZ
- `error`: TEXT
- `created_at`: TIMESTAMPTZ

## Configuration

### Complete Threshold

The `PROGRESS_COMPLETE_THRESHOLD` setting determines when content is marked as completed. Default is 95%, meaning if a user watches 95% or more of the content, it's marked as completed.

```bash
PROGRESS_COMPLETE_THRESHOLD=95  # 95% watched = completed
```

### History Sampling

To avoid excessive database writes, history events are sampled. The `PROGRESS_HISTORY_SAMPLE_SECONDS` setting controls the minimum time between history inserts for the same content.

```bash
PROGRESS_HISTORY_SAMPLE_SECONDS=30  # Only log every 30 seconds
```

### Multi-App Support

Use the `X-Source-Account-Id` header to isolate data:

```bash
curl -X POST http://localhost:3022/v1/progress \
  -H "X-Source-Account-Id: app-123" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user1","content_type":"movie","content_id":"abc","position_seconds":120}'
```

## Content Types

Supported content types:
- `movie` - Full-length movies
- `episode` - TV show episodes
- `video` - Generic videos
- `audio` - Audio content (podcasts, music)
- `article` - Written articles
- `course` - Educational course content

## License

Source-Available License

## Author

nself
