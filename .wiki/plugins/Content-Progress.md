# Content Progress Plugin

Track video, audio, and content playback progress with continue watching, watchlists, favorites, and viewing history for nself applications.

## Overview

The Content Progress plugin provides comprehensive tracking of user interactions with media content. It tracks playback positions, completion status, viewing history, watchlists, and favorites, enabling features like "Continue Watching" and personalized recommendations.

### Key Features

- **Playback Tracking**: Track current playback position for videos and audio
- **Completion Detection**: Automatically mark content as completed
- **Continue Watching**: Resume playback from last position
- **Viewing History**: Complete history of content consumption
- **Watchlists**: Save content for later viewing
- **Favorites**: Mark and organize favorite content
- **Multi-Device Sync**: Sync progress across devices
- **Analytics**: Track viewing patterns and engagement
- **Configurable Thresholds**: Customize completion thresholds
- **Sampling**: Configurable history sampling intervals
- **Multi-App Support**: Isolated tracking per source account

### Use Cases

- **Video Platforms**: Netflix-style continue watching
- **E-learning**: Track course progress and completion
- **Podcasts**: Resume podcast episodes
- **Audiobooks**: Track reading progress
- **Music**: Track listening history
- **Gaming**: Save game progress
- **Reading Apps**: Track book progress
- **Fitness Apps**: Track workout progress

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install content-progress

# Initialize database schema
nself content-progress init

# Start the server
nself content-progress server
```

### Basic Usage

```bash
# Update playback progress
curl -X POST http://localhost:3022/v1/progress \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "content_id": "video456",
    "content_type": "video",
    "position_seconds": 245,
    "duration_seconds": 600
  }'

# Get continue watching
curl http://localhost:3022/v1/users/user123/continue-watching

# Add to watchlist
curl -X POST http://localhost:3022/v1/users/user123/watchlist \
  -H "Content-Type: application/json" \
  -d '{
    "content_id": "movie789",
    "content_type": "movie"
  }'

# Check status
nself content-progress status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PROGRESS_PLUGIN_PORT` | No | `3022` | HTTP server port |
| `PROGRESS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `PROGRESS_COMPLETE_THRESHOLD` | No | `95` | Percentage to mark as complete |
| `PROGRESS_HISTORY_SAMPLE_SECONDS` | No | `30` | Interval to sample position (seconds) |
| `PROGRESS_HISTORY_RETENTION_DAYS` | No | `365` | Days to retain history |
| `PROGRESS_API_KEY` | No | - | API key for authentication |
| `PROGRESS_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `PROGRESS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
PROGRESS_PLUGIN_PORT=3022
PROGRESS_COMPLETE_THRESHOLD=90
PROGRESS_HISTORY_SAMPLE_SECONDS=60
PROGRESS_HISTORY_RETENTION_DAYS=730
PROGRESS_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself content-progress init
```

### `server`
Start the HTTP API server.

```bash
nself content-progress server [options]

Options:
  -p, --port <port>    Server port (default: 3022)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

### `status`
Show plugin status and statistics.

```bash
nself content-progress status
```

**Output:**
```
Content Progress Status
=======================
Version:               1.0.0
Port:                  3022
Complete Threshold:    95%
History Sampling:      30 seconds
History Retention:     365 days

Statistics
==========
Total Users:           2341
Active Sessions:       234
Total Progress:        15234
Completed Items:       8921
Watchlist Items:       3456
Favorite Items:        1892
History Records:       125678
```

### `progress`
Manage playback progress.

```bash
nself content-progress progress [command]

Commands:
  get <userId> <contentId>    Get progress for content
  update                      Update progress
  delete <userId> <contentId> Delete progress
```

### `watchlist`
Manage watchlist items.

```bash
nself content-progress watchlist <userId> [command]

Commands:
  list                List watchlist items
  add <contentId>     Add to watchlist
  remove <contentId>  Remove from watchlist
```

### `favorites`
Manage favorite items.

```bash
nself content-progress favorites <userId> [command]

Commands:
  list                List favorites
  add <contentId>     Add to favorites
  remove <contentId>  Remove from favorites
```

### `stats`
View user statistics.

```bash
nself content-progress stats <userId>
```

---

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health & Status

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "content-progress",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/status`
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "content-progress",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "completeThreshold": 95,
    "historySampleSeconds": 30,
    "historyRetentionDays": 365
  },
  "stats": {
    "totalUsers": 2341,
    "activeSessions": 234,
    "totalProgress": 15234,
    "completedItems": 8921,
    "watchlistItems": 3456,
    "favoriteItems": 1892,
    "historyRecords": 125678,
    "progressByContentType": {
      "video": 8234,
      "audio": 3421,
      "podcast": 2134,
      "course": 1445
    }
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Progress Tracking

#### `POST /v1/progress`
Update playback progress.

**Request:**
```json
{
  "user_id": "user123",
  "content_id": "video456",
  "content_type": "video",
  "position_seconds": 245,
  "duration_seconds": 600,
  "device_id": "device789",
  "metadata": {
    "quality": "1080p",
    "player": "web"
  }
}
```

**Response:**
```json
{
  "user_id": "user123",
  "content_id": "video456",
  "content_type": "video",
  "position_seconds": 245,
  "duration_seconds": 600,
  "progress_percent": 40.83,
  "completed": false,
  "device_id": "device789",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/progress/:userId/:contentId`
Get progress for specific content.

**Response:**
```json
{
  "user_id": "user123",
  "content_id": "video456",
  "content_type": "video",
  "position_seconds": 245,
  "duration_seconds": 600,
  "progress_percent": 40.83,
  "completed": false,
  "last_watched_at": "2026-02-11T10:30:00Z",
  "device_id": "device789",
  "watch_count": 3,
  "created_at": "2026-02-10T08:00:00Z",
  "updated_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/users/:userId/progress`
Get all progress for a user.

**Query Parameters:**
- `content_type`: Filter by content type
- `completed`: Filter by completion status (true/false)
- `limit`: Results per page (default: 50)
- `offset`: Pagination offset (default: 0)

**Response:**
```json
{
  "data": [
    {
      "content_id": "video456",
      "content_type": "video",
      "position_seconds": 245,
      "duration_seconds": 600,
      "progress_percent": 40.83,
      "completed": false,
      "last_watched_at": "2026-02-11T10:30:00Z"
    }
  ],
  "total": 45,
  "limit": 50,
  "offset": 0,
  "hasMore": false
}
```

#### `DELETE /v1/progress/:userId/:contentId`
Delete progress record.

**Response:**
```json
{
  "success": true
}
```

### Continue Watching

#### `GET /v1/users/:userId/continue-watching`
Get continue watching list.

**Query Parameters:**
- `content_type`: Filter by content type
- `limit`: Results per page (default: 20)

**Response:**
```json
{
  "data": [
    {
      "content_id": "video456",
      "content_type": "video",
      "position_seconds": 245,
      "duration_seconds": 600,
      "progress_percent": 40.83,
      "last_watched_at": "2026-02-11T10:30:00Z",
      "metadata": {
        "title": "Episode 5",
        "series": "My Show",
        "thumbnail": "https://example.com/thumb.jpg"
      }
    }
  ],
  "total": 5
}
```

### Viewing History

#### `GET /v1/users/:userId/history`
Get viewing history.

**Query Parameters:**
- `content_type`: Filter by content type
- `from`: Start date (ISO 8601)
- `to`: End date (ISO 8601)
- `limit`: Results per page (default: 50)
- `offset`: Pagination offset (default: 0)

**Response:**
```json
{
  "data": [
    {
      "content_id": "video456",
      "content_type": "video",
      "position_seconds": 245,
      "timestamp": "2026-02-11T10:30:00Z",
      "device_id": "device789",
      "session_duration": 245
    }
  ],
  "total": 1234,
  "limit": 50,
  "offset": 0,
  "hasMore": true
}
```

#### `DELETE /v1/users/:userId/history`
Clear viewing history.

**Query Parameters:**
- `before`: Delete history before this date (ISO 8601)

**Response:**
```json
{
  "success": true,
  "deleted": 523
}
```

### Watchlist

#### `POST /v1/users/:userId/watchlist`
Add to watchlist.

**Request:**
```json
{
  "content_id": "movie789",
  "content_type": "movie",
  "metadata": {
    "title": "Great Movie",
    "year": 2024
  }
}
```

**Response:**
```json
{
  "user_id": "user123",
  "content_id": "movie789",
  "content_type": "movie",
  "added_at": "2026-02-11T10:30:00Z",
  "metadata": {...}
}
```

#### `GET /v1/users/:userId/watchlist`
Get watchlist.

**Query Parameters:**
- `content_type`: Filter by content type
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "content_id": "movie789",
      "content_type": "movie",
      "added_at": "2026-02-11T10:30:00Z",
      "metadata": {
        "title": "Great Movie",
        "year": 2024,
        "duration": 7200
      }
    }
  ],
  "total": 23
}
```

#### `DELETE /v1/users/:userId/watchlist/:contentId`
Remove from watchlist.

**Response:**
```json
{
  "success": true
}
```

#### `POST /v1/users/:userId/watchlist/bulk-add`
Add multiple items to watchlist.

**Request:**
```json
{
  "items": [
    {"content_id": "movie1", "content_type": "movie"},
    {"content_id": "video2", "content_type": "video"}
  ]
}
```

**Response:**
```json
{
  "success": true,
  "added": 2
}
```

### Favorites

#### `POST /v1/users/:userId/favorites`
Add to favorites.

**Request:**
```json
{
  "content_id": "series123",
  "content_type": "series",
  "metadata": {
    "title": "My Favorite Show"
  }
}
```

**Response:**
```json
{
  "user_id": "user123",
  "content_id": "series123",
  "content_type": "series",
  "added_at": "2026-02-11T10:30:00Z",
  "metadata": {...}
}
```

#### `GET /v1/users/:userId/favorites`
Get favorites.

**Query Parameters:**
- `content_type`: Filter by content type
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "content_id": "series123",
      "content_type": "series",
      "added_at": "2026-02-11T10:30:00Z",
      "metadata": {...}
    }
  ],
  "total": 18
}
```

#### `DELETE /v1/users/:userId/favorites/:contentId`
Remove from favorites.

**Response:**
```json
{
  "success": true
}
```

### Statistics

#### `GET /v1/users/:userId/stats`
Get user statistics.

**Response:**
```json
{
  "user_id": "user123",
  "total_watched": 523,
  "total_completed": 412,
  "total_watch_time_seconds": 245678,
  "watchlist_count": 23,
  "favorites_count": 18,
  "active_progress_count": 12,
  "content_types": {
    "video": 234,
    "audio": 156,
    "podcast": 89,
    "course": 44
  },
  "completion_rate": 78.8,
  "average_completion_percent": 85.3,
  "most_watched_content_type": "video",
  "current_streak_days": 7,
  "longest_streak_days": 15
}
```

#### `GET /v1/users/:userId/analytics`
Get detailed viewing analytics.

**Query Parameters:**
- `period`: Time period (day, week, month, year)
- `from`: Start date
- `to`: End date

**Response:**
```json
{
  "period": "week",
  "total_watch_time_seconds": 12345,
  "unique_content_watched": 45,
  "completion_rate": 82.5,
  "daily_breakdown": [
    {
      "date": "2026-02-05",
      "watch_time_seconds": 1800,
      "content_count": 5,
      "completed": 3
    }
  ],
  "top_content_types": [
    {"type": "video", "count": 23, "watch_time_seconds": 8234},
    {"type": "podcast", "count": 12, "watch_time_seconds": 3456}
  ],
  "peak_viewing_hours": [19, 20, 21]
}
```

---

## Webhook Events

### `progress.updated`
Triggered when playback position is updated.

### `progress.completed`
Triggered when content is marked as completed.

### `watchlist.added`
Triggered when content is added to watchlist.

### `watchlist.removed`
Triggered when content is removed from watchlist.

### `favorite.added`
Triggered when content is added to favorites.

### `favorite.removed`
Triggered when content is removed from favorites.

---

## Database Schema

### `progress_positions`
```sql
CREATE TABLE progress_positions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(64) NOT NULL,
  position_seconds INTEGER NOT NULL DEFAULT 0,
  duration_seconds INTEGER,
  progress_percent DECIMAL(5,2) GENERATED ALWAYS AS (
    CASE WHEN duration_seconds > 0
    THEN (position_seconds::decimal / duration_seconds * 100)
    ELSE 0 END
  ) STORED,
  completed BOOLEAN DEFAULT false,
  completed_at TIMESTAMP WITH TIME ZONE,
  device_id VARCHAR(255),
  watch_count INTEGER DEFAULT 1,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, user_id, content_id)
);
```

### `progress_history`
```sql
CREATE TABLE progress_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(64) NOT NULL,
  position_seconds INTEGER NOT NULL,
  session_id VARCHAR(255),
  device_id VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### `progress_watchlists`
```sql
CREATE TABLE progress_watchlists (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(64) NOT NULL,
  metadata JSONB DEFAULT '{}',
  added_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, user_id, content_id)
);
```

### `progress_favorites`
```sql
CREATE TABLE progress_favorites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  content_id VARCHAR(255) NOT NULL,
  content_type VARCHAR(64) NOT NULL,
  metadata JSONB DEFAULT '{}',
  added_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, user_id, content_id)
);
```

### `progress_webhook_events`
```sql
CREATE TABLE progress_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: Video Player Integration

```javascript
// Update progress every 30 seconds
setInterval(async () => {
  const position = videoPlayer.currentTime;
  const duration = videoPlayer.duration;

  await fetch('http://localhost:3022/v1/progress', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      user_id: currentUser.id,
      content_id: videoId,
      content_type: 'video',
      position_seconds: Math.floor(position),
      duration_seconds: Math.floor(duration)
    })
  });
}, 30000);

// Resume from last position
const progress = await fetch(
  `http://localhost:3022/v1/progress/${userId}/${videoId}`
).then(r => r.json());

if (progress.position_seconds > 0) {
  videoPlayer.currentTime = progress.position_seconds;
}
```

### Example 2: Continue Watching UI

```bash
# Get continue watching items
curl http://localhost:3022/v1/users/user123/continue-watching?limit=10

# Display in UI with progress bars
# position_seconds / duration_seconds * 100 = progress_percent
```

### Example 3: E-learning Course Progress

```bash
# Track course video completion
curl -X POST http://localhost:3022/v1/progress \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "student123",
    "content_id": "course-5-video-3",
    "content_type": "course",
    "position_seconds": 1200,
    "duration_seconds": 1200,
    "metadata": {
      "course_id": "5",
      "module": "3",
      "lesson": "Introduction to APIs"
    }
  }'

# Check if completed (position >= 95% of duration)
```

### Example 4: Podcast Resume

```bash
# Save podcast position
curl -X POST http://localhost:3022/v1/progress \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "listener123",
    "content_id": "podcast-ep-45",
    "content_type": "podcast",
    "position_seconds": 1523,
    "duration_seconds": 3600
  }'

# Later, resume from saved position
curl http://localhost:3022/v1/progress/listener123/podcast-ep-45
```

### Example 5: Watchlist Management

```bash
# Add multiple movies to watchlist
curl -X POST http://localhost:3022/v1/users/user123/watchlist/bulk-add \
  -H "Content-Type: application/json" \
  -d '{
    "items": [
      {"content_id": "movie1", "content_type": "movie"},
      {"content_id": "movie2", "content_type": "movie"},
      {"content_id": "movie3", "content_type": "movie"}
    ]
  }'

# Get watchlist for recommendations
curl http://localhost:3022/v1/users/user123/watchlist
```

---

## Troubleshooting

### Progress Not Updating

**Solution:**
- Verify content_id and user_id are correct
- Check database connectivity
- Ensure duration_seconds is provided
- Review update frequency (not too frequent)

### Completion Not Triggering

**Solution:**
- Check `PROGRESS_COMPLETE_THRESHOLD` setting (default 95%)
- Ensure position_seconds >= (duration_seconds * threshold / 100)
- Verify duration_seconds is accurate

### High Database Load

**Solution:**
- Increase `PROGRESS_HISTORY_SAMPLE_SECONDS` (reduce frequency)
- Implement client-side throttling
- Use batch updates where possible
- Consider async processing

### History Growing Too Large

**Solution:**
```bash
# Adjust retention
export PROGRESS_HISTORY_RETENTION_DAYS=180

# Manual cleanup
DELETE FROM progress_history
WHERE created_at < NOW() - INTERVAL '180 days';
```

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/content-progress
