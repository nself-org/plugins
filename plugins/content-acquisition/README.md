# content-acquisition

Automated content acquisition with RSS monitoring, release calendar, and download rules engine

## Installation

```bash
nself plugin install content-acquisition
```

## Features

### Current Features

- **RSS Feed Monitoring** - Automated polling and parsing of RSS feeds
- **Fuzzy Title Matching** - Intelligent content matching with quality and year detection
- **Subscription Management** - Track TV shows, movies, artists, and podcasts
- **Download Queue** - Prioritized acquisition queue with state management
- **Quality Profiles** - Configurable quality preferences (4K, 1080p, 720p, HDR, etc.)
- **Download Rules Engine** - Flexible rule-based automation
- **Pipeline Orchestration** - Multi-stage processing (VPN, torrent, metadata, subtitles)
- **Release Calendar** - Track upcoming releases

### Planned Features

- TMDB/TVDB Integration
- Auto-upgrade to better quality releases
- Plex/Jellyfin library integration
- Advanced scheduling
- Notification webhooks

## Configuration

See plugin.json for environment variables and configuration options.

Key environment variables:

- `CONTENT_ACQUISITION_PORT` - API server port (default: 3200)
- `CONTENT_ACQUISITION_TORRENT_MANAGER_URL` - Torrent manager URL
- `CONTENT_ACQUISITION_RSS_CHECK_INTERVAL` - RSS polling interval in minutes (default: 30)

## API Usage

### RSS Polling

Poll an RSS feed with matching criteria:

```bash
curl -X POST http://localhost:3200/api/rss/poll \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/rss",
    "criteria": [
      {
        "title": "Breaking Bad",
        "year": 2008,
        "quality": ["1080p", "720p"]
      }
    ],
    "lastSeen": "2024-01-01T00:00:00Z"
  }'
```

Response:
```json
{
  "url": "https://example.com/rss",
  "itemCount": 3,
  "matches": [
    {
      "title": "Breaking Bad S01E01 1080p",
      "link": "...",
      "pubDate": "2024-02-15T12:00:00Z"
    }
  ],
  "polledAt": "2024-02-15T15:30:00Z"
}
```

### Test RSS Feed

Test if an RSS feed is valid and parseable:

```bash
curl -X POST http://localhost:3200/api/rss/test \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/rss"
  }'
```

Response:
```json
{
  "url": "https://example.com/rss",
  "valid": true,
  "itemCount": 42,
  "sample": [
    {
      "title": "Item 1",
      "pubDate": "2024-02-15T12:00:00Z"
    }
  ],
  "testedAt": "2024-02-15T15:30:00Z"
}
```

### Subscription Management

Create a subscription:

```bash
curl -X POST http://localhost:3200/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "contentType": "tv_show",
    "contentName": "Breaking Bad",
    "qualityProfileId": "uuid-here"
  }'
```

### Download Management

Add to download queue:

```bash
curl -X POST http://localhost:3200/v1/downloads \
  -H "Content-Type: application/json" \
  -d '{
    "contentType": "tv_episode",
    "title": "Breaking Bad S01E01",
    "qualityProfile": "balanced"
  }'
```

## CLI Commands

See plugin.json for available CLI commands.

## License

See LICENSE file in repository root.
