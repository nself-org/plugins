# subtitle-manager

Subtitle search, download, synchronization, quality control, and format conversion via the OpenSubtitles REST API. Downloads SRT files, corrects timing offsets using `alass` and `ffsubsync`, validates subtitle quality against broadcast-standard checks, and normalizes output to WebVTT. Designed to serve nself-tv's subtitle pipeline without blocking content playback.

## Installation

```bash
nself plugin install subtitle-manager
```

## Features

- Text-based subtitle search against OpenSubtitles by title query
- Hash-based subtitle search using `moviehash` and `moviebytesize` for precise file matching
- Subtitle download and storage to disk, organized by source account, media ID, and language
- Download deduplication — checks the database before downloading from OpenSubtitles
- Subtitle synchronization via `alass` (Rust-based, fast offset/split correction) with `ffsubsync` as fallback (Python audio-based sync)
- Quality control (QC) validation with seven deterministic checks: timestamp bounds, first cue timing, last cue proximity, negative durations, overlap rate, characters per second, and line length
- WebVTT normalization from SRT and ASS/SSA formats with automatic encoding detection
- Full cascade endpoint (`/v1/fetch-best`) — searches, downloads, syncs, and converts in one call per language, processing multiple languages in parallel
- QC results persisted to the database and linked to download records
- Download tracking with sync score, QC status, and file size
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting

## Configuration

| Name | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `OPENSUBTITLES_API_KEY` | No | — | OpenSubtitles REST API key. Required for search and download. |
| `SUBTITLE_STORAGE_PATH` | No | `/tmp/subtitles` | Directory for storing downloaded subtitle files |
| `SUBTITLE_MANAGER_PORT` | No | `3204` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `ALASS_PATH` | No | `alass` | Path to the `alass` binary |
| `FFSUBSYNC_PATH` | No | `ffsubsync` | Path to the `ffsubsync` binary |

### System dependencies

`python3` (3.8+) is required. Two sync tools are optional but strongly recommended:

- **alass** — `cargo install alass-cli` — fast Rust-based sync tool, tried first
- **ffsubsync** — `pip3 install ffsubsync` — audio-based Python sync, used as fallback when alass offset exceeds 500ms

## API Reference

### Health

#### GET /health

Returns `{ status: "ok", plugin: "subtitle-manager", version: "1.0.0" }`. No authentication required.

### Local Subtitle Queries

#### GET /v1/subtitles?media_id={id}&language={lang}

Searches locally stored subtitles for a given media ID and language. Returns records ordered by sync score descending.

Query parameters:

| Parameter | Required | Description |
| --------- | -------- | ----------- |
| `media_id` | Yes | Media identifier (movie ID, episode ID, etc.) |
| `language` | No | BCP-47 language code, default `en` |

Response:

```json
{
  "subtitles": [
    {
      "id": "uuid",
      "media_id": "tt1234567",
      "language": "en",
      "file_path": "/tmp/subtitles/primary/tt1234567/en.srt",
      "source": "opensubtitles",
      "sync_score": 0.95,
      "created_at": "2026-02-21T00:00:00Z"
    }
  ]
}
```

#### GET /v1/downloads?limit={n}&offset={n}

Lists all download records for the current source account, paginated.

#### GET /v1/stats

Returns subtitle and download counts, broken down by language and source.

Response:

```json
{
  "stats": {
    "total_subtitles": 42,
    "total_downloads": 38,
    "languages": [{ "language": "en", "count": 30 }, { "language": "fr", "count": 8 }],
    "sources": [{ "source": "opensubtitles", "count": 38 }]
  }
}
```

### Search

#### POST /v1/search

Searches OpenSubtitles by text query. Returns raw OpenSubtitles results including file IDs needed for download.

Request body:

```json
{
  "query": "The Dark Knight",
  "languages": ["en", "fr"]
}
```

`languages` defaults to `["en"]`.

#### POST /v1/search/hash

Searches OpenSubtitles using the OpenSubtitles movie hash algorithm for exact file matching. More accurate than text search for known video files.

Request body:

```json
{
  "moviehash": "8e245d9679d31e12",
  "moviebytesize": 733892608,
  "languages": ["en"]
}
```

### Download

#### POST /v1/download

Downloads a specific subtitle file from OpenSubtitles by `file_id` (from a prior search result) and saves it to disk. Returns the cached result if the subtitle was already downloaded for this `media_id` + `language` combination.

Request body:

```json
{
  "file_id": 1234567,
  "media_id": "tt1234567",
  "media_type": "movie",
  "media_title": "The Dark Knight",
  "language": "en",
  "run_qc": false
}
```

`media_type` accepts `movie` or `tv_episode`. `run_qc` triggers an immediate QC validation after download and stores the result.

Response:

```json
{
  "success": true,
  "download": {
    "id": "uuid",
    "media_id": "tt1234567",
    "language": "en",
    "file_path": "/tmp/subtitles/primary/tt1234567/en.srt",
    "file_size_bytes": 45320,
    "opensubtitles_file_id": 1234567,
    "source": "opensubtitles",
    "created_at": "2026-02-21T00:00:00Z"
  },
  "source": "opensubtitles"
}
```

When `run_qc: true`, the response also includes a `qc` field with the validation result.

#### DELETE /v1/downloads/:id

Deletes a download record. Returns `404` if not found.

### Synchronization

#### POST /v1/sync

Synchronizes subtitle timing to a video file. Tries `alass` first. If the detected offset exceeds 500ms, tries `ffsubsync` and keeps whichever result has the smaller absolute offset.

Request body:

```json
{
  "video_path": "/media/movies/dark-knight.mkv",
  "subtitle_path": "/tmp/subtitles/primary/tt1234567/en.srt",
  "language": "en"
}
```

The synced file is written to a `synced/` subdirectory under the account's storage path.

Response:

```json
{
  "success": true,
  "result": {
    "outputPath": "/tmp/subtitles/primary/synced/en.synced.en.srt",
    "offsetMs": 120,
    "toolUsed": "alass"
  }
}
```

### Quality Control

#### POST /v1/qc

Validates a subtitle file against seven deterministic checks. Accepts both `.srt` and `.vtt` files.

Request body:

```json
{
  "subtitle_path": "/tmp/subtitles/primary/tt1234567/en.srt",
  "video_duration_ms": 9120000,
  "download_id": "uuid"
}
```

`video_duration_ms` is optional. When provided, enables timestamp range and last-cue-near-end checks. When `download_id` is provided, the result is stored in `np_subtmgr_qc_results` and the download record's `qc_status` is updated.

QC checks performed:

| Check | Severity | Rule |
| ----- | -------- | ---- |
| `timestamps_in_range` | error | All cue timestamps must be within `[0, video_duration + 5s]` |
| `first_cue_early` | error | First cue must start within the first 10 minutes |
| `last_cue_near_end` | error | Last cue must end within 5 minutes of video end |
| `no_negative_durations` | error | No cue may have `end < start` |
| `overlap_rate` | error | Cue overlap rate must not exceed 10% |
| `cps_bounds` | warning | Characters per second must be 5–35 per cue |
| `line_length` | warning | No line may exceed 80 characters |

Response:

```json
{
  "success": true,
  "result": {
    "status": "warn",
    "cueCount": 842,
    "totalDurationMs": 5580000,
    "checks": [
      { "name": "no_negative_durations", "passed": true, "message": "No negative durations found" },
      { "name": "cps_bounds", "passed": false, "message": "3 cue(s) outside CPS bounds (5-35)" }
    ],
    "issues": [
      {
        "severity": "warning",
        "check": "cps_bounds",
        "cueIndex": 14,
        "message": "Cue 14 has 38.2 CPS (191 chars / 5.0s). Expected 5-35 CPS"
      }
    ]
  }
}
```

### Format Conversion

#### POST /v1/normalize

Converts a subtitle file to WebVTT format. Accepts `.srt`, `.ass`, and `.ssa` input with automatic encoding detection.

Request body:

```json
{
  "input_path": "/tmp/subtitles/primary/tt1234567/en.srt",
  "output_format": "vtt"
}
```

`output_format` currently only accepts `vtt`.

Response:

```json
{
  "success": true,
  "output_path": "/tmp/subtitles/primary/tt1234567/en.vtt"
}
```

### Full Cascade

#### POST /v1/fetch-best

The primary endpoint for nself-tv. Executes the full subtitle pipeline for each requested language in parallel:

1. Search OpenSubtitles by title query (or filename)
2. Rank results by download count and rating
3. Download up to `max_alternatives` candidates
4. Sync each with `alass`, fall back to `ffsubsync` if offset > 500ms
5. Keep the candidate with the smallest absolute offset
6. Normalize the best result to WebVTT
7. Track the download in the database

If a step fails for a given language, that language returns `sync_quality: "failed"` and the pipeline continues with other languages. This endpoint never blocks the content pipeline.

Request body:

```json
{
  "video_path": "/media/movies/dark-knight.mkv",
  "languages": ["en", "fr"],
  "max_alternatives": 3,
  "media_id": "tt1234567",
  "media_type": "movie",
  "media_title": "The Dark Knight"
}
```

`max_alternatives` controls how many candidates to try per language (1–10, default 3). `media_id`, `media_type`, and `media_title` are optional but enable database tracking.

Response:

```json
{
  "success": true,
  "subtitles": [
    {
      "language": "en",
      "path": "/tmp/subtitles/primary/tt1234567/en/synced_1234567.vtt",
      "format": "webvtt",
      "sync_quality": "good",
      "sync_warning": false,
      "offset_ms": 80,
      "tool_used": "alass"
    },
    {
      "language": "fr",
      "path": null,
      "format": "none",
      "sync_quality": "failed",
      "sync_warning": true,
      "offset_ms": 0,
      "tool_used": "none"
    }
  ],
  "languages_requested": 2,
  "languages_found": 1
}
```

`sync_quality` values: `good` (offset ≤ 500ms), `warning` (offset > 500ms but sync completed), `failed` (no subtitle found or all sync attempts failed).

## CLI Commands

```bash
# Initialize database schema
nself-subtitle-manager init

# Start the HTTP API server
nself-subtitle-manager server

# Search OpenSubtitles by text query
nself-subtitle-manager search "The Dark Knight" --languages en,fr

# Run QC validation on a subtitle file
nself-subtitle-manager qc /path/to/subtitle.srt

# Synchronize a subtitle to a video
nself-subtitle-manager sync /path/to/video.mkv /path/to/subtitle.srt

# Convert to WebVTT
nself-subtitle-manager normalize /path/to/subtitle.srt
```

## Database Tables

| Table | Purpose |
| ----- | ------- |
| `np_subtmgr_subtitles` | Subtitle catalog — media ID, language, file path, source, and sync score |
| `np_subtmgr_downloads` | Download log — OpenSubtitles file ID, file hash, file size, QC status, and sync score per download |
| `np_subtmgr_qc_results` | QC validation results — per-check pass/fail, issues list, cue count, and total duration |

All tables include `source_account_id` for multi-app isolation. `np_subtmgr_downloads` references `np_subtmgr_subtitles` and `np_subtmgr_qc_results` references `np_subtmgr_downloads` via foreign keys with `ON DELETE CASCADE`.

## Usage Examples

### Search and download a subtitle

```typescript
// Search
const searchRes = await fetch('http://localhost:3204/v1/search', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ query: 'The Dark Knight', languages: ['en'] }),
});
const { results } = await searchRes.json();
const fileId = results[0].attributes.files[0].file_id;

// Download
const dlRes = await fetch('http://localhost:3204/v1/download', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    file_id: fileId,
    media_id: 'tt1234567',
    media_type: 'movie',
    language: 'en',
  }),
});
const { download } = await dlRes.json();
```

### Run the full pipeline for a video

```typescript
const res = await fetch('http://localhost:3204/v1/fetch-best', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    video_path: '/media/movies/dark-knight.mkv',
    languages: ['en', 'ar'],
    media_id: 'tt1234567',
    media_title: 'The Dark Knight',
  }),
});
const { subtitles } = await res.json();
// subtitles[0].path is the ready-to-serve WebVTT file
```

### Validate a subtitle file before serving it

```typescript
const res = await fetch('http://localhost:3204/v1/qc', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    subtitle_path: '/tmp/subtitles/primary/tt1234567/en.srt',
    video_duration_ms: 9120000,
  }),
});
const { result } = await res.json();
if (result.status === 'fail') {
  // log issues and decide whether to serve anyway or try another candidate
}
```

## Integration

This plugin is the subtitle backend for **nself-tv**. When a user starts playing a video, nself-tv calls `/v1/fetch-best` with the video path and desired languages. The plugin handles the full download-sync-convert pipeline and returns ready-to-serve WebVTT paths. Failures for individual languages never block playback — the response always arrives, with nulls for languages that could not be resolved.

## Changelog

### v1.0.0

- Initial release
- OpenSubtitles text and hash-based search
- Subtitle download and disk storage with caching
- Sync via `alass` with `ffsubsync` fallback, configurable 500ms threshold
- QC validation with seven checks (timestamp bounds, CPS, overlap rate, line length)
- WebVTT normalization from SRT and ASS/SSA
- Full cascade endpoint (`/v1/fetch-best`) with parallel language processing
- QC results persisted to database
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting
