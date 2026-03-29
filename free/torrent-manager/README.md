# torrent-manager

Torrent downloading with Transmission/qBittorrent integration, multi-source search, seeding policies, and VPN enforcement.

The `torrent-manager` plugin connects your nself stack to one or more running torrent clients. It provides a unified HTTP API and CLI for searching across multiple torrent sources, adding downloads by magnet link or URL, tracking progress, and enforcing seeding policies. VPN enforcement prevents downloads from starting unless an active VPN connection is detected via the `vpn` plugin.

---

## Table of Contents

- [Installation](#installation)
- [Requirements](#requirements)
- [Configuration](#configuration)
- [Usage](#usage)
  - [CLI Commands](#cli-commands)
  - [API Endpoints](#api-endpoints)
- [Torrent Clients](#torrent-clients)
  - [Transmission](#transmission)
  - [qBittorrent](#qbittorrent)
- [Search Sources](#search-sources)
- [Seeding Policies](#seeding-policies)
- [VPN Enforcement](#vpn-enforcement)
- [Storage Integration (MinIO)](#storage-integration-minio)
- [Webhook Events](#webhook-events)
- [Database Tables](#database-tables)
- [Dependencies](#dependencies)
- [Troubleshooting](#troubleshooting)

---

## Installation

```bash
nself plugin install torrent-manager
```

This installs the plugin into your nself stack. The plugin runs as a Node.js service on port `3201` (configurable). It requires a running Transmission or qBittorrent daemon accessible from your server.

---

## Requirements

| Requirement | Notes |
|-------------|-------|
| nself | v0.4.8 or later |
| Node.js | v20 or later |
| PostgreSQL | Provided by your nself stack |
| Torrent client | Transmission or qBittorrent (must be running separately) |
| `vpn` plugin | Required when `VPN_REQUIRED=true` (default) |

The torrent client itself is NOT managed by this plugin. You are responsible for running Transmission or qBittorrent. The plugin connects to their existing RPC/Web UI interfaces.

---

## Configuration

All configuration is via environment variables. Place them in your nself stack `.env` file.

### Core

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string (provided by nself) |
| `TORRENT_MANAGER_PORT` | No | `3201` | HTTP server port for the plugin API |

### Torrent Client

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DEFAULT_TORRENT_CLIENT` | No | `transmission` | Active client: `transmission` or `qbittorrent` |

### Transmission

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TRANSMISSION_HOST` | No | `localhost` | Hostname or IP where Transmission RPC is running |
| `TRANSMISSION_PORT` | No | `9091` | Transmission RPC port |
| `TRANSMISSION_USERNAME` | No | — | Transmission RPC username (if authentication is enabled) |
| `TRANSMISSION_PASSWORD` | No | — | Transmission RPC password |

### qBittorrent

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `QBITTORRENT_HOST` | No | `localhost` | Hostname or IP where qBittorrent Web UI is running |
| `QBITTORRENT_PORT` | No | `8080` | qBittorrent Web UI port |
| `QBITTORRENT_USERNAME` | No | — | qBittorrent Web UI username |
| `QBITTORRENT_PASSWORD` | No | — | qBittorrent Web UI password |

### Download Directories

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DOWNLOAD_PATH` | No | `/downloads` | Default directory where torrents are downloaded |

Configure your torrent client to use the same paths so the plugin can track files correctly.

### Search

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ENABLED_SOURCES` | No | `1337x,yts,torrentgalaxy,tpb` | Comma-separated list of enabled search sources |
| `SEARCH_TIMEOUT_MS` | No | `10000` | Per-source search timeout in milliseconds |
| `SEARCH_CACHE_TTL_SECONDS` | No | `3600` | How long to cache search results (seconds) |

### Seeding

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SEEDING_RATIO_LIMIT` | No | `2.0` | Stop seeding when upload/download ratio reaches this value |
| `SEEDING_TIME_LIMIT_HOURS` | No | `168` | Stop seeding after this many hours (168 = 1 week) |
| `MAX_ACTIVE_DOWNLOADS` | No | `5` | Maximum number of concurrent active downloads |

### VPN Enforcement

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `VPN_REQUIRED` | No | `true` | Require active VPN before starting any download |
| `VPN_MANAGER_URL` | Yes* | — | Base URL of the `vpn` plugin API (e.g. `http://localhost:3200`) |

*Required when `VPN_REQUIRED=true`.

### Minimal .env example

```env
DATABASE_URL=postgresql://postgres:password@localhost:5432/mydb
TORRENT_MANAGER_PORT=3201

DEFAULT_TORRENT_CLIENT=transmission
TRANSMISSION_HOST=localhost
TRANSMISSION_PORT=9091
TRANSMISSION_USERNAME=admin
TRANSMISSION_PASSWORD=secret

DOWNLOAD_PATH=/media/downloads

VPN_REQUIRED=true
VPN_MANAGER_URL=http://localhost:3200

ENABLED_SOURCES=1337x,yts
SEEDING_RATIO_LIMIT=2.0
MAX_ACTIVE_DOWNLOADS=3
```

---

## Usage

### CLI Commands

The plugin exposes CLI commands via `nself plugin run torrent-manager <command>`.

#### Initialize

Register your torrent client with the plugin and create the database schema:

```bash
nself plugin run torrent-manager init
```

Run this once after installation before using any other commands.

#### Search

Search for torrents across all enabled sources:

```bash
nself plugin run torrent-manager search "breaking bad season 1"
```

Output includes name, size, seeders, leechers, and source for each result.

Filter by quality or category:

```bash
nself plugin run torrent-manager search "interstellar" --category movies
```

#### Best Match

Search for a title and automatically select the best result using a scoring algorithm (prefers high seeders, good quality, and verified sources):

```bash
nself plugin run torrent-manager best-match "the wire season 2"
```

Optionally download immediately:

```bash
nself plugin run torrent-manager best-match "the wire season 2" --download
```

#### Add Download

Add a torrent by magnet link:

```bash
nself plugin run torrent-manager add "magnet:?xt=urn:btih:..."
```

Add by .torrent file URL:

```bash
nself plugin run torrent-manager add "https://example.com/file.torrent"
```

#### List Downloads

List all current downloads with their status:

```bash
nself plugin run torrent-manager list
```

Example output:

```
ID                                   Name                    Status       Progress  Size
a1b2c3d4-...                         Breaking Bad S01        downloading  45%       8.2 GB
e5f6g7h8-...                         The Wire S02            seeding      100%      12.4 GB
i9j0k1l2-...                         Interstellar 4K         paused       0%        55.1 GB
```

#### Pause a Download

```bash
nself plugin run torrent-manager pause a1b2c3d4-...
```

#### Resume a Download

```bash
nself plugin run torrent-manager resume a1b2c3d4-...
```

#### Remove a Download

Remove a download record (does not delete files by default):

```bash
nself plugin run torrent-manager remove a1b2c3d4-...
```

Remove and delete the downloaded files:

```bash
nself plugin run torrent-manager remove a1b2c3d4-... --delete-files
```

#### Statistics

Show aggregate download statistics:

```bash
nself plugin run torrent-manager stats
```

Output includes total downloaded, total uploaded, active count, completed count, and average ratio.

#### Start Server

Start the HTTP API server (usually handled automatically by nself):

```bash
nself plugin run torrent-manager server
```

---

### API Endpoints

The plugin runs an HTTP server at `http://localhost:3201` (configurable via `TORRENT_MANAGER_PORT`).

#### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Returns `200 OK` when service is running |
| `GET` | `/ready` | Returns `200 OK` when database connection is established |

#### Clients

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/clients` | List registered torrent clients and their connection status |

Example response:

```json
{
  "clients": [
    {
      "id": "transmission-default",
      "type": "transmission",
      "host": "localhost",
      "port": 9091,
      "status": "connected",
      "version": "3.00"
    }
  ]
}
```

#### Search

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/search` | Search all enabled sources |
| `POST` | `/v1/search/best-match` | Return highest-scored result |
| `GET` | `/v1/search/cache` | List cached search results |

Search request body:

```json
{
  "query": "breaking bad season 1",
  "category": "tv",
  "limit": 20
}
```

Best-match request body:

```json
{
  "query": "interstellar 4k",
  "autoDownload": false
}
```

#### Downloads

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/downloads` | Add a download by magnet link or URL |
| `GET` | `/v1/downloads` | List all downloads |
| `GET` | `/v1/downloads/:id` | Get a single download by ID |
| `DELETE` | `/v1/downloads/:id` | Remove a download |
| `POST` | `/v1/downloads/:id/pause` | Pause an active download |
| `POST` | `/v1/downloads/:id/resume` | Resume a paused download |

Add download request body:

```json
{
  "magnet": "magnet:?xt=urn:btih:...",
  "category": "movies",
  "savePath": "/media/movies"
}
```

Download object (response):

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "name": "Interstellar.2014.4K.BluRay",
  "status": "downloading",
  "progress": 0.45,
  "size_bytes": 55109836800,
  "downloaded_bytes": 24799426560,
  "upload_ratio": 0.12,
  "eta_seconds": 3600,
  "added_at": "2026-01-15T10:30:00Z"
}
```

#### Statistics

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/stats` | Aggregate download statistics |

#### Seeding

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/seeding` | List all seeding torrents |
| `PUT` | `/v1/seeding/:id/policy` | Update seeding policy for a torrent |
| `GET` | `/v1/seeding/:id/policy` | Get current seeding policy for a torrent |

Seeding policy body:

```json
{
  "ratio_limit": 3.0,
  "time_limit_hours": 336
}
```

#### Sources

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/sources` | List available search sources and their status |

---

## Torrent Clients

### Transmission

Transmission must be running with RPC enabled. In your Transmission configuration (`settings.json`):

```json
{
  "rpc-enabled": true,
  "rpc-port": 9091,
  "rpc-authentication-required": true,
  "rpc-username": "admin",
  "rpc-password": "secret",
  "rpc-whitelist-enabled": false
}
```

Restart Transmission after changing settings. Verify it is accessible:

```bash
curl -u admin:secret http://localhost:9091/transmission/rpc
```

### qBittorrent

qBittorrent must be running with the Web UI enabled. Enable it in qBittorrent under Tools > Preferences > Web UI. Set a username and password, and note the port (default 8080).

Verify it is accessible:

```bash
curl -c /tmp/qb-cookie.txt -d "username=admin&password=secret" \
  http://localhost:8080/api/v2/auth/login
```

---

## Search Sources

The plugin searches across these sources simultaneously and merges results:

| Source | ID | Notes |
|--------|----|-------|
| 1337x | `1337x` | General content |
| YTS | `yts` | Movies only, smaller file sizes |
| TorrentGalaxy | `torrentgalaxy` | General content |
| The Pirate Bay | `tpb` | General content |

Enable or disable sources with `ENABLED_SOURCES`:

```env
ENABLED_SOURCES=1337x,yts
```

Results from all sources are merged and deduplicated by info hash. The best-match algorithm scores results using seeders, file size, and source reliability.

---

## Seeding Policies

Seeding policies control when a completed torrent stops uploading. Default policy applies globally; per-torrent overrides are supported.

### Global defaults (env vars)

```env
SEEDING_RATIO_LIMIT=2.0     # stop when uploaded 2x the downloaded size
SEEDING_TIME_LIMIT_HOURS=168 # stop after 1 week regardless of ratio
```

### Per-torrent override (API)

```bash
curl -X PUT http://localhost:3201/v1/seeding/a1b2c3d4-.../policy \
  -H "Content-Type: application/json" \
  -d '{"ratio_limit": 5.0, "time_limit_hours": 720}'
```

The plugin polls your torrent client periodically to enforce policies. When a torrent meets its stop condition, the plugin instructs the client to stop seeding that torrent.

---

## VPN Enforcement

When `VPN_REQUIRED=true` (the default), the plugin checks the `vpn` plugin before starting any download. If no active VPN connection is detected, the download is rejected.

The check calls `GET /v1/status` on the VPN plugin API:

```env
VPN_MANAGER_URL=http://localhost:3200
```

To disable enforcement (not recommended for privacy):

```env
VPN_REQUIRED=false
```

When the VPN disconnects while a download is active, the plugin emits a `vpn.disconnected` webhook event. It does not automatically pause downloads when the VPN drops — configure your torrent client's kill switch for that.

---

## Storage Integration (MinIO)

When MinIO is installed in your nself stack, you can move completed downloads to S3-compatible object storage using the jobs plugin or a custom workflow.

The torrent-manager tracks file paths in the `np_torrentmanager_files` table. After a download completes (trigger on `torrent.completed` webhook), you can use the `jobs` plugin's database backup processor pattern to upload files to MinIO.

Example webhook handler approach: configure the `webhooks` plugin to call an endpoint that triggers an upload job when `torrent.completed` fires with a matching category.

MinIO environment variables (from your nself stack):

```env
MINIO_ENDPOINT=http://minio:9000
MINIO_ACCESS_KEY=your-access-key
MINIO_SECRET_KEY=your-secret-key
```

---

## Webhook Events

The plugin emits these events when torrent state changes. Use the `webhooks` plugin to subscribe to them.

| Event | Trigger |
|-------|---------|
| `torrent.added` | A torrent is successfully added to the client |
| `torrent.started` | Downloading begins (leaves queued state) |
| `torrent.progress` | Progress update (emitted every 5% by default) |
| `torrent.completed` | Download finishes (100% complete) |
| `torrent.failed` | Download failed or was rejected |
| `torrent.removed` | A torrent is removed from tracking |
| `vpn.disconnected` | VPN connection was lost (detected during a download check) |

Webhook payload example (`torrent.completed`):

```json
{
  "event": "torrent.completed",
  "torrent_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "name": "Interstellar.2014.4K.BluRay",
  "size_bytes": 55109836800,
  "download_path": "/media/movies/Interstellar.2014.4K.BluRay",
  "completed_at": "2026-01-15T14:22:00Z"
}
```

---

## Database Tables

The plugin creates these PostgreSQL tables in your nself database. All tables use the `np_` prefix and include a `source_account_id` column for multi-app isolation.

| Table | Purpose |
|-------|---------|
| `np_torrentmanager_clients` | Registered torrent client connections |
| `np_torrentmanager_sources` | Available search source configurations |
| `np_torrentmanager_downloads` | All download records with status and progress |
| `np_torrentmanager_files` | Individual files within each torrent |
| `np_torrentmanager_trackers` | Tracker URLs and announce statistics |
| `np_torrentmanager_search_cache` | Cached search results (auto-expired by TTL) |
| `np_torrentmanager_stats` | Aggregate statistics snapshots |
| `np_torrentmanager_seeding_policy` | Per-torrent seeding policy overrides |
| `np_torrent_seeding_policies` | Named policy templates |

Views for common queries:

| View | Purpose |
|------|---------|
| `np_torrentmanager_active_downloads` | Downloads currently in progress |
| `np_torrentmanager_completed_downloads` | Finished downloads |
| `np_torrentmanager_seeding_torrents` | Torrents currently seeding |

---

## Dependencies

### npm packages

| Package | Purpose |
|---------|---------|
| `@nself/plugin-utils` | Shared nself plugin utilities |
| `@ctrl/transmission` | Transmission RPC client |
| `fastify` | HTTP server |
| `@fastify/cors` | CORS middleware |
| `@fastify/rate-limit` | Rate limiting middleware |
| `commander` | CLI framework |
| `axios` | HTTP requests (search source clients) |
| `cheerio` | HTML parsing for search results |
| `puppeteer` | JavaScript-rendered search pages |
| `parse-torrent` | Parse .torrent files and magnet links |
| `webtorrent-health` | Torrent health checking via DHT |
| `node-cron` | Seeding policy enforcement scheduler |
| `pg` | PostgreSQL client |
| `dotenv` | Environment variable loading |
| `uuid` | ID generation |
| `winston` | Logging |

### System packages

The following must be installed on your host system separately. They are NOT installed by `nself plugin install`:

- `transmission-daemon` — Transmission torrent client daemon
- `qbittorrent-nox` — qBittorrent headless (no UI) daemon

Install on Ubuntu/Debian:

```bash
# For Transmission
sudo apt-get install transmission-daemon

# For qBittorrent (headless)
sudo apt-get install qbittorrent-nox
```

---

## Troubleshooting

### Plugin cannot connect to Transmission

Check that Transmission is running and accessible:

```bash
curl -u admin:password http://localhost:9091/transmission/rpc
```

If you get a 409, it means Transmission RPC is running but returned a CSRF token — this is normal. The plugin handles this automatically.

If you get connection refused, Transmission is not running or is on a different port. Check your `TRANSMISSION_HOST` and `TRANSMISSION_PORT` env vars.

### VPN check failing

The plugin calls `GET /v1/status` on the VPN plugin. Verify the URL:

```bash
curl http://localhost:3200/v1/status
```

If this fails, check that the `vpn` plugin is running and `VPN_MANAGER_URL` is set correctly. You can disable VPN enforcement for testing with `VPN_REQUIRED=false`.

### Search returning no results

Check the sources are reachable from your server. Some public torrent search sites may be blocked in certain regions. Use `ENABLED_SOURCES` to configure only the sources that work in your environment.

Increase the timeout if results are slow:

```env
SEARCH_TIMEOUT_MS=20000
```

### Database errors on startup

Run `init` to ensure the schema is created:

```bash
nself plugin run torrent-manager init
```

Confirm `DATABASE_URL` points to your running PostgreSQL instance.

### Plugin port conflict

If port `3201` is in use, change it:

```env
TORRENT_MANAGER_PORT=3210
```

Check what is using the port:

```bash
lsof -i :3201
```
