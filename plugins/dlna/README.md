# DLNA Plugin

DLNA/UPnP media server plugin for nself. Serves media files to smart TVs, game consoles, and other DLNA/UPnP renderers on the local network.

## Features

- **SSDP Discovery**: Automatic device discovery via UDP multicast (239.255.255.250:1900)
- **UPnP MediaServer:1**: Full device description and service descriptions
- **ContentDirectory:1**: Browse and search media library via SOAP actions
- **ConnectionManager:1**: Protocol info and connection management
- **HTTP Streaming**: Stream media with Range request support for seeking
- **Media Scanner**: Scans directories, indexes files with metadata to PostgreSQL
- **Renderer Discovery**: Finds and tracks DLNA renderers on the network
- **REST API**: Management endpoints for status, media, renderers, and scanning

## Quick Start

```bash
# Install dependencies
cd plugins/dlna/ts
pnpm install

# Configure environment
export DATABASE_URL=postgres://postgres:password@localhost:5432/nself
export DLNA_MEDIA_PATHS=/path/to/videos,/path/to/music

# Start server
pnpm run dev
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | - | PostgreSQL connection string (required) |
| `DLNA_PORT` | `3025` | HTTP server port |
| `DLNA_SSDP_PORT` | `1900` | SSDP multicast port |
| `DLNA_FRIENDLY_NAME` | `nself-tv Media Server` | Device name shown to DLNA clients |
| `DLNA_MEDIA_PATHS` | `/media` | Comma-separated media directories to scan |
| `DLNA_UUID` | auto-generated | Persistent device UUID |
| `DLNA_ADVERTISE_INTERVAL` | `30` | SSDP advertisement interval in seconds |

## CLI Commands

```bash
# Start the DLNA server
nself-dlna server [--port 3025] [--name "My Server"]

# Initialize database schema
nself-dlna init

# Scan media directories
nself-dlna scan [--dirs /media/video,/media/audio]

# Show server status and stats
nself-dlna status

# List discovered renderers
nself-dlna renderers [--active]

# List/search media items
nself-dlna media list [--limit 20]
nself-dlna media show <id>
nself-dlna media search --query "movie name"

# Prune stale renderers
nself-dlna prune [--hours 24]
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (database connectivity) |
| `GET` | `/v1/status` | Server status, active renderers, stats |
| `GET` | `/v1/renderers` | List discovered DLNA renderers |
| `GET` | `/v1/media` | Browse media library |
| `GET` | `/v1/media/:id` | Get single media item |
| `POST` | `/v1/scan` | Rescan media directories |
| `GET` | `/v1/stats` | Media library statistics |

## UPnP Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/description.xml` | UPnP device description |
| `GET` | `/ContentDirectory.xml` | ContentDirectory SCPD |
| `GET` | `/ConnectionManager.xml` | ConnectionManager SCPD |
| `POST` | `/control/ContentDirectory` | SOAP control (Browse, Search) |
| `POST` | `/control/ConnectionManager` | SOAP control (GetProtocolInfo) |
| `GET` | `/media/:id` | Stream media file (Range support) |
| `GET` | `/thumbnails/:id` | Thumbnail image |

## Database Schema

### np_dlna_media_items

Stores all indexed media files and virtual containers.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` | Primary key |
| `source_account_id` | `VARCHAR(255)` | Multi-app isolation |
| `parent_id` | `UUID` | Parent container reference |
| `object_type` | `VARCHAR(20)` | `container` or `item` |
| `upnp_class` | `TEXT` | UPnP object class |
| `title` | `TEXT` | Display title |
| `file_path` | `TEXT` | Absolute file path |
| `file_size` | `BIGINT` | File size in bytes |
| `mime_type` | `VARCHAR(100)` | MIME type |
| `duration_seconds` | `INTEGER` | Duration for audio/video |
| `resolution` | `VARCHAR(20)` | Resolution (e.g., 1920x1080) |
| `bitrate` | `INTEGER` | Bitrate in bits/sec |
| `album` | `TEXT` | Album name (audio) |
| `artist` | `TEXT` | Artist name (audio) |
| `genre` | `TEXT` | Genre |
| `thumbnail_path` | `TEXT` | Thumbnail file path |

### np_dlna_renderers

Tracks discovered DLNA renderers on the network.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` | Primary key |
| `source_account_id` | `VARCHAR(255)` | Multi-app isolation |
| `usn` | `TEXT` | Unique Service Name |
| `friendly_name` | `TEXT` | Device display name |
| `location` | `TEXT` | Device description URL |
| `ip_address` | `VARCHAR(45)` | IP address |
| `device_type` | `TEXT` | UPnP device type |
| `manufacturer` | `TEXT` | Manufacturer name |
| `model_name` | `TEXT` | Model name |
| `last_seen_at` | `TIMESTAMPTZ` | Last SSDP advertisement |

## Supported Media Formats

### Video
mp4, mkv, avi, mov, wmv, webm, mpg, mpeg, m2ts, ts, 3gp, ogv, flv, m4v

### Audio
mp3, flac, wav, aac, ogg, wma, m4a, opus, aiff, alac

### Image
jpg, jpeg, png, gif, bmp, webp, tiff, tif, svg

## Architecture

```
DLNA Client (Smart TV)
    |
    |--- SSDP M-SEARCH (UDP 239.255.255.250:1900)
    |         |
    |    SSDPServer (ssdp.ts)
    |         |--- NOTIFY ssdp:alive (periodic)
    |         |--- M-SEARCH response (LOCATION header)
    |
    |--- GET /description.xml
    |         |--- upnp.ts
    |
    |--- SOAP POST /control/ContentDirectory
    |         |--- content-directory.ts
    |         |--- didl.ts (DIDL-Lite XML)
    |         |--- database.ts (queries)
    |
    |--- GET /media/:id (HTTP streaming)
              |--- streaming.ts (Range support)
```

## License

Source-Available
