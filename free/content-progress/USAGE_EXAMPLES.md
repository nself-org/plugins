# Content Progress Plugin - Usage Examples

Complete examples for using the content-progress plugin.

## Setup

```bash
# 1. Install dependencies
cd plugins/content-progress/ts
npm install

# 2. Configure environment
cp .env.example .env
# Edit .env with your database credentials

# 3. Initialize database
npm run build
node dist/cli.js init

# 4. Start server
npm run dev
```

## API Usage Examples

### Update Progress (Video Player)

```javascript
// User watches a movie for 2 minutes (120 seconds) out of 2 hours (7200 seconds)
const response = await fetch('http://localhost:3022/v1/progress', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Source-Account-Id': 'netflix-clone'  // Optional: multi-app isolation
  },
  body: JSON.stringify({
    user_id: 'user_123',
    content_type: 'movie',
    content_id: 'inception-2010',
    position_seconds: 120,
    duration_seconds: 7200,
    device_id: 'smart-tv-living-room',
    audio_track: 'en',
    subtitle_track: 'en',
    quality: '4k',
    metadata: {
      title: 'Inception',
      year: 2010
    }
  })
});

// Response:
// {
//   "id": "550e8400-e29b-41d4-a716-446655440000",
//   "user_id": "user_123",
//   "content_type": "movie",
//   "content_id": "inception-2010",
//   "position_seconds": 120,
//   "duration_seconds": 7200,
//   "progress_percent": 1.67,
//   "completed": false,
//   "device_id": "smart-tv-living-room",
//   "updated_at": "2026-02-11T12:00:00Z"
// }
```

### Get Continue Watching

```javascript
// Get user's "continue watching" list
const response = await fetch('http://localhost:3022/v1/continue-watching/user_123?limit=10');
const data = await response.json();

// Response:
// {
//   "data": [
//     {
//       "content_type": "episode",
//       "content_id": "stranger-things-s4e1",
//       "position_seconds": 1200,
//       "progress_percent": 42.5,
//       "updated_at": "2026-02-11T11:30:00Z"
//     },
//     {
//       "content_type": "movie",
//       "content_id": "inception-2010",
//       "position_seconds": 120,
//       "progress_percent": 1.67,
//       "updated_at": "2026-02-11T10:15:00Z"
//     }
//   ]
// }
```

### Mark as Completed

```javascript
// User finishes watching (or manually marks as completed)
const response = await fetch(
  'http://localhost:3022/v1/progress/user_123/movie/inception-2010/complete',
  { method: 'POST' }
);

// Response:
// {
//   "id": "550e8400-e29b-41d4-a716-446655440000",
//   "completed": true,
//   "completed_at": "2026-02-11T12:05:00Z",
//   "progress_percent": 100
// }
```

### Add to Watchlist

```javascript
// User adds a movie to their watchlist
const response = await fetch('http://localhost:3022/v1/watchlist', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    user_id: 'user_123',
    content_type: 'movie',
    content_id: 'dune-2021',
    priority: 10,
    added_from: 'recommendations',
    notes: 'Recommended by Sarah'
  })
});
```

### Add to Favorites

```javascript
// User favorites a show
const response = await fetch('http://localhost:3022/v1/favorites', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    user_id: 'user_123',
    content_type: 'episode',
    content_id: 'breaking-bad-s5e14'
  })
});
```

### Get User Statistics

```javascript
// Get comprehensive user stats
const response = await fetch('http://localhost:3022/v1/stats/user_123');
const stats = await response.json();

// Response:
// {
//   "total_watch_time_seconds": 86400,
//   "total_watch_time_hours": 24,
//   "content_completed": 42,
//   "content_in_progress": 8,
//   "watchlist_count": 15,
//   "favorites_count": 23,
//   "most_watched_type": "episode",
//   "recent_activity": "2026-02-11T12:00:00Z"
// }
```

## CLI Usage Examples

### Initialize Database

```bash
node dist/cli.js init
```

### Start Server

```bash
# Development mode with hot reload
npm run dev

# Production mode
npm start

# Custom port and host
node dist/cli.js server --port 3022 --host 0.0.0.0
```

### Check Status

```bash
node dist/cli.js status

# Output:
# Content Progress Plugin Status
# ==============================
# Complete threshold: 95%
# History sampling:   30s
#
# Plugin Statistics:
#   Total users:          150
#   Total positions:      1,234
#   Completed:            456
#   In progress:          234
#   Watchlist items:      789
#   Favorite items:       567
#   History events:       45,678
#   Last activity:        2026-02-11T12:00:00Z
```

### Manage Progress

```bash
# List user's progress
node dist/cli.js progress list user_123

# Show specific progress
node dist/cli.js progress show user_123 movie inception-2010

# Update progress
node dist/cli.js progress update user_123 movie inception-2010 \
  --position 120 --duration 7200

# Mark as completed
node dist/cli.js progress complete user_123 movie inception-2010

# Delete progress
node dist/cli.js progress delete user_123 movie inception-2010
```

### Manage Watchlist

```bash
# List user's watchlist
node dist/cli.js watchlist list user_123

# Add to watchlist
node dist/cli.js watchlist add user_123 movie dune-2021 \
  --priority 10 --notes "Must watch this weekend"

# Remove from watchlist
node dist/cli.js watchlist remove user_123 movie dune-2021
```

### Manage Favorites

```bash
# List user's favorites
node dist/cli.js favorites list user_123

# Add to favorites
node dist/cli.js favorites add user_123 episode breaking-bad-s5e14

# Remove from favorites
node dist/cli.js favorites remove user_123 episode breaking-bad-s5e14
```

### View Statistics

```bash
node dist/cli.js stats user_123

# Output:
# User Statistics: user_123
# ==================================================
# Total watch time:     24.50 hours
#                       (88200 seconds)
# Content completed:    42
# Content in progress:  8
# Watchlist count:      15
# Favorites count:      23
# Most watched type:    episode
# Recent activity:      2026-02-11T12:00:00Z
```

## Integration Examples

### React Hook

```typescript
// useProgress.ts
import { useState, useEffect } from 'react';

interface ProgressState {
  position: number;
  progress: number;
  completed: boolean;
}

export function useProgress(userId: string, contentType: string, contentId: string) {
  const [progress, setProgress] = useState<ProgressState | null>(null);

  useEffect(() => {
    // Load initial progress
    fetch(`http://localhost:3022/v1/progress/${userId}/${contentType}/${contentId}`)
      .then(res => res.json())
      .then(data => setProgress({
        position: data.position_seconds,
        progress: data.progress_percent,
        completed: data.completed
      }))
      .catch(() => setProgress({ position: 0, progress: 0, completed: false }));
  }, [userId, contentType, contentId]);

  const updateProgress = async (positionSeconds: number, durationSeconds: number) => {
    const response = await fetch('http://localhost:3022/v1/progress', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        user_id: userId,
        content_type: contentType,
        content_id: contentId,
        position_seconds: positionSeconds,
        duration_seconds: durationSeconds
      })
    });

    const data = await response.json();
    setProgress({
      position: data.position_seconds,
      progress: data.progress_percent,
      completed: data.completed
    });
  };

  return { progress, updateProgress };
}
```

### Video Player Integration

```typescript
// VideoPlayer.tsx
import React, { useEffect, useRef } from 'react';
import { useProgress } from './useProgress';

interface Props {
  userId: string;
  videoId: string;
  videoUrl: string;
}

export function VideoPlayer({ userId, videoId, videoUrl }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const { progress, updateProgress } = useProgress(userId, 'video', videoId);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !progress) return;

    // Resume from last position
    video.currentTime = progress.position;

    // Update progress every 10 seconds
    const interval = setInterval(() => {
      updateProgress(video.currentTime, video.duration);
    }, 10000);

    return () => clearInterval(interval);
  }, [progress, updateProgress]);

  return (
    <video
      ref={videoRef}
      src={videoUrl}
      controls
      onEnded={() => {
        if (videoRef.current) {
          updateProgress(videoRef.current.duration, videoRef.current.duration);
        }
      }}
    />
  );
}
```

### Express.js Middleware

```typescript
// progressMiddleware.ts
import { Request, Response, NextFunction } from 'express';

export function trackProgress(progressApiUrl: string) {
  return async (req: Request, res: Response, next: NextFunction) => {
    if (req.method === 'POST' && req.path === '/api/watch-event') {
      const { userId, contentType, contentId, position, duration } = req.body;

      // Track progress asynchronously
      fetch(`${progressApiUrl}/v1/progress`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          user_id: userId,
          content_type: contentType,
          content_id: contentId,
          position_seconds: position,
          duration_seconds: duration
        })
      }).catch(err => console.error('Progress tracking failed:', err));
    }

    next();
  };
}
```

## Multi-App Isolation

```javascript
// App 1: Netflix Clone
fetch('http://localhost:3022/v1/progress', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Source-Account-Id': 'netflix-clone'
  },
  body: JSON.stringify({
    user_id: 'user_123',
    content_type: 'movie',
    content_id: 'movie-abc',
    position_seconds: 120
  })
});

// App 2: YouTube Clone
fetch('http://localhost:3022/v1/progress', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-Source-Account-Id': 'youtube-clone'
  },
  body: JSON.stringify({
    user_id: 'user_123',
    content_type: 'video',
    content_id: 'video-xyz',
    position_seconds: 45
  })
});

// Data is completely isolated between apps
```

## Production Configuration

```bash
# .env for production
DATABASE_URL=postgresql://user:pass@prod-db.example.com:5432/nself?sslmode=require

PROGRESS_PLUGIN_PORT=3022
PROGRESS_PLUGIN_HOST=0.0.0.0

# Complete at 98% (instead of default 95%)
PROGRESS_COMPLETE_THRESHOLD=98

# Sample history every minute (instead of 30s)
PROGRESS_HISTORY_SAMPLE_SECONDS=60

# Keep history for 2 years
PROGRESS_HISTORY_RETENTION_DAYS=730

# Enable authentication
PROGRESS_API_KEY=your-secure-api-key-here

# Rate limiting: 1000 requests per minute
PROGRESS_RATE_LIMIT_MAX=1000
PROGRESS_RATE_LIMIT_WINDOW_MS=60000

LOG_LEVEL=info
```

## Monitoring

```bash
# Health check
curl http://localhost:3022/health

# Readiness check (database connectivity)
curl http://localhost:3022/ready

# Detailed liveness
curl http://localhost:3022/live

# Plugin status
curl http://localhost:3022/v1/status
```

## Cleanup Old Data

```sql
-- Delete history events older than 1 year
DELETE FROM progress_history
WHERE created_at < NOW() - INTERVAL '365 days';

-- Delete completed items older than 6 months
DELETE FROM progress_positions
WHERE completed = TRUE
  AND completed_at < NOW() - INTERVAL '6 months';
```
