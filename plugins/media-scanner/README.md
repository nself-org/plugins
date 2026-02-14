# media-scanner

Media library scanning, filename parsing, FFprobe analysis, TMDB metadata matching, and MeiliSearch indexing for nself.

## Overview

The media-scanner plugin provides a complete pipeline for discovering, analyzing, and cataloging media files. It replaces the core library scanning logic from the `library_service` custom service in nself-tv with a standalone, reusable plugin.

### Pipeline

1. **Scan** - Recursively discover media files across configured directories
2. **Parse** - Extract title, year, season, episode, quality, resolution, codec, and release group from filenames
3. **Probe** - Run FFprobe to collect duration, codecs, bitrate, audio/subtitle track information
4. **Match** - Search TMDB for metadata matches using Levenshtein distance scoring
5. **Index** - Push matched media items into MeiliSearch for full-text search

## Quick Start

```bash
cd plugins/media-scanner/ts
pnpm install
pnpm run build

# Initialize database
pnpm start init

# Scan a directory
pnpm start scan /path/to/media --probe --match

# Start the HTTP server
pnpm start server
```

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `MEDIA_SCANNER_PORT` | No | 3021 | HTTP server port |
| `MEDIA_SCANNER_HOST` | No | 0.0.0.0 | HTTP server host |
| `MEILISEARCH_URL` | No | http://localhost:7700 | MeiliSearch URL |
| `MEILISEARCH_KEY` | No | - | MeiliSearch API key |
| `TMDB_API_KEY` | No | - | TMDB API key for metadata matching |
| `MEDIA_LIBRARY_PATHS` | No | - | Comma-separated media directories |
| `SCAN_INTERVAL_HOURS` | No | 24 | Automatic scan interval |

## CLI Commands

```bash
nself-media-scanner scan [paths...]       # Scan directories for media files
nself-media-scanner parse <filename>       # Parse a media filename
nself-media-scanner probe <path>           # FFprobe a media file
nself-media-scanner match <title>          # Match title against TMDB
nself-media-scanner search <query>         # Search indexed media
nself-media-scanner stats                  # Show library statistics
nself-media-scanner files [action] [id]    # List/show media files
nself-media-scanner index                  # Index matched files into MeiliSearch
nself-media-scanner init                   # Initialize database schema
nself-media-scanner server                 # Start the HTTP server
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | /v1/scan | Trigger library scan |
| GET | /v1/scan/:id | Get scan status |
| POST | /v1/parse | Parse filename |
| POST | /v1/probe | FFprobe a file |
| POST | /v1/match | Match against TMDB |
| POST | /v1/index | Index a media item |
| GET | /v1/search | Search media |
| GET | /v1/stats | Library statistics |
| GET | /v1/files | List media files |
| GET | /v1/files/:id | Get media file details |
| GET | /v1/scans | List scan history |
| GET | /health | Health check |

## Database Tables

### np_mscan_scans

Tracks scan jobs with state, progress, and errors.

### np_mscan_media_files

Stores discovered files with parsed metadata, probe results, match info, and index status.

## Supported Media Extensions

`.mkv`, `.mp4`, `.avi`, `.ts`, `.m4v`, `.webm`, `.mov`, `.wmv`, `.flv`, `.mpg`, `.mpeg`, `.m2ts`, `.vob`, `.ogv`, `.3gp`

## Filename Parser

Handles common naming patterns:

- `Show.Name.S01E02.720p.BluRay.x264-GROUP`
- `Movie.Title.2023.1080p.WEB-DL.DDP5.1.H.264-GROUP`
- `[SubGroup] Anime Title - 01 (1080p) [ABCD1234].mkv`
- And many more torrent/scene naming conventions

## Match Confidence

- **> 0.8** - Auto-accept (high confidence)
- **0.5 - 0.8** - Suggested match (manual review recommended)
- **< 0.5** - No match
