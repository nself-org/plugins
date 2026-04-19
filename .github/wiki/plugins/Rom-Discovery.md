# ROM Discovery Plugin

ROM metadata search, discovery, and scraping engine with full-text search, legal compliance, download queue management, and automated scraper scheduling.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Features](#features)
- [Troubleshooting](#troubleshooting)

---

## Overview

| Field | Value |
|-------|-------|
| **Version** | 1.0.0 |
| **Category** | media |
| **Port** | 3034 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`) |

The ROM Discovery plugin provides a searchable ROM metadata database populated by automated scrapers. It includes full-text search with PostgreSQL tsvector, quality and popularity scoring, legal compliance with DMCA-aware disclaimer and acceptance workflows, an auditable download queue, and CSV audit log export.

---

## Quick Start

```bash
nself plugin install rom-discovery
nself plugin rom-discovery init
nself plugin rom-discovery server --enable-scrapers
nself plugin rom-discovery search "Super Mario"
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ROM_DISCOVERY_PORT` | `3034` | Server port |
| `ROM_DISCOVERY_HOST` | `0.0.0.0` | Server host |
| `ROM_DISCOVERY_ENABLE_SCRAPERS` | `false` | Enable automated scrapers |
| `ROM_DISCOVERY_SCRAPER_SCHEDULE` | `0 2 * * *` | Cron schedule for scrapers |
| `ROM_DISCOVERY_DEFAULT_QUALITY` | `50` | Default quality score for new ROMs |
| `ROM_DISCOVERY_DEFAULT_POPULARITY` | `0` | Default popularity score |
| `ROM_DISCOVERY_MAX_CONCURRENT_DOWNLOADS` | `3` | Max simultaneous downloads |
| `ROM_DISCOVERY_MAX_DOWNLOAD_SIZE_MB` | `2048` | Max download size in MB |
| `ROM_DISCOVERY_RETROGAMING_URL` | - | Retro Gaming plugin URL for integration |
| `ROM_DISCOVERY_API_KEY` | - | API key for authentication |
| `ROM_DISCOVERY_RATE_LIMIT_MAX` | `100` | Rate limit max requests |
| `ROM_DISCOVERY_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window |

---

## CLI Commands

| Command | Description | Options |
|---------|-------------|---------|
| `init` | Initialize database schema and seed default scrapers | - |
| `server` | Start the API server | `-p, --port`, `-h, --host`, `--enable-scrapers` |
| `search <query>` | Search ROM database | `--platform`, `--region`, `--quality-min`, `--verified`, `--homebrew`, `--show-hacks`, `--sort`, `-l, --limit` |
| `platforms` | List platforms with ROM counts | - |
| `scrapers list` | List all scrapers and their status | - |
| `scrapers run <name>` | Manually trigger a scraper | - |
| `downloads` / `queue` | List download queue | `--status` |
| `stats` / `status` | Show ROM Discovery statistics | - |

---

## REST API

### Search and Discovery

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/roms/search` | Full-text search with filters |
| `GET` | `/api/roms/:id` | Get ROM metadata details |
| `GET` | `/api/roms/platforms` | List platforms with stats |
| `GET` | `/api/roms/featured` | Get featured ROMs |

### Legal Compliance

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/legal/disclaimer` | Get legal disclaimer text |
| `POST` | `/api/legal/accept` | Accept legal disclaimer |
| `GET` | `/api/legal/status/:userId` | Check user acceptance status |

### Downloads

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/roms/download` | Queue a download (requires legal acceptance, returns HTTP 451 if not accepted) |
| `GET` | `/api/roms/download/:id/status` | Check download status |
| `GET` | `/api/roms/download/queue` | List download queue |

### Scrapers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/roms/scrapers` | List all scrapers |
| `POST` | `/api/roms/scrapers/:name/run` | Manually trigger a scraper |

### Scoring

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/roms/scoring/quality` | Update quality scores |
| `POST` | `/api/roms/scoring/popularity` | Update popularity scores |

### Audit

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/audit` | View audit log entries |
| `GET` | `/api/audit/export` | Export audit log as CSV |

### Stats and Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/stats` | ROM Discovery statistics |
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |

---

## Database Schema

### `np_romdisc_metadata`

Full-text search enabled with tsvector trigger.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `rom_title` | VARCHAR(512) | ROM title |
| `platform` | VARCHAR(64) | Platform (nes, snes, gba, etc.) |
| `region` | VARCHAR(32) | Region (USA, Europe, Japan) |
| `language` | VARCHAR(32) | Language |
| `developer` | VARCHAR(255) | Developer |
| `publisher` | VARCHAR(255) | Publisher |
| `release_year` | INTEGER | Release year |
| `genre` | VARCHAR(64) | Genre |
| `description` | TEXT | Game description |
| `file_name` | VARCHAR(512) | ROM file name |
| `file_size_bytes` | BIGINT | File size |
| `file_hash_md5` | VARCHAR(32) | MD5 hash |
| `file_hash_sha1` | VARCHAR(40) | SHA1 hash |
| `file_hash_crc32` | VARCHAR(8) | CRC32 hash |
| `download_url` | TEXT | Download URL |
| `cover_url` | TEXT | Cover art URL |
| `screenshots` | JSONB | Screenshot URLs |
| `quality_score` | INTEGER | Quality score (0-100) |
| `popularity_score` | INTEGER | Popularity score |
| `is_verified_dump` | BOOLEAN | Verified dump flag |
| `is_homebrew` | BOOLEAN | Homebrew flag |
| `is_hack` | BOOLEAN | ROM hack flag |
| `is_translation` | BOOLEAN | Translation flag |
| `release_group` | VARCHAR(128) | Release group name |
| `source_scraper` | VARCHAR(128) | Scraper that found it |
| `source_url` | TEXT | Original source URL |
| `search_vector` | TSVECTOR | Full-text search vector (auto-updated by trigger) |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | First discovered |
| `updated_at` | TIMESTAMPTZ | Last updated |

**Indexes:** GIN index on `search_vector`, trigram index on `rom_title` for similarity search, indexes on platform, region, quality_score, popularity_score, is_verified_dump, is_homebrew.

### `np_romdisc_download_queue`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `rom_metadata_id` | UUID | FK to np_romdisc_metadata |
| `user_id` | VARCHAR(255) | Requesting user |
| `status` | VARCHAR(32) | pending / downloading / completed / failed / cancelled |
| `download_url` | TEXT | URL to download from |
| `destination_path` | TEXT | Local destination path |
| `total_bytes` | BIGINT | Total file size |
| `downloaded_bytes` | BIGINT | Bytes downloaded so far |
| `download_progress_percent` | INTEGER | Progress percentage |
| `checksum_verified` | BOOLEAN | Checksum verification passed |
| `error_message` | TEXT | Error description |
| `retry_count` | INTEGER | Number of retries |
| `max_retries` | INTEGER | Maximum retries allowed |
| `created_at` | TIMESTAMPTZ | Queue timestamp |
| `started_at` | TIMESTAMPTZ | Download start |
| `completed_at` | TIMESTAMPTZ | Download completion |

### `np_romdisc_scraper_jobs`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `scraper_name` | VARCHAR(128) | Scraper identifier |
| `scraper_type` | VARCHAR(64) | Scraper type |
| `cron_schedule` | VARCHAR(64) | Cron schedule expression |
| `enabled` | BOOLEAN | Whether scraper is active |
| `last_run_at` | TIMESTAMPTZ | Last execution time |
| `last_run_status` | VARCHAR(32) | Last run status |
| `last_run_duration_seconds` | INTEGER | Last run duration |
| `roms_found` | INTEGER | ROMs found in last run |
| `roms_added` | INTEGER | ROMs added in last run |
| `roms_updated` | INTEGER | ROMs updated in last run |
| `errors` | JSONB | Error list from last run |
| `config` | JSONB | Scraper configuration |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

**Default seeded scrapers:** 8 default scrapers are created on init.

### `np_romdisc_popularity`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `rom_metadata_id` | UUID | FK to np_romdisc_metadata |
| `search_count` | INTEGER | Times searched |
| `download_count` | INTEGER | Times downloaded |
| `view_count` | INTEGER | Times viewed |
| `score` | INTEGER | Computed popularity score |
| `updated_at` | TIMESTAMPTZ | Last score update |

### `np_romdisc_audit_log`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `action` | VARCHAR(64) | Action type (search, download, accept_legal, etc.) |
| `user_id` | VARCHAR(255) | Acting user |
| `rom_metadata_id` | UUID | Related ROM |
| `details` | JSONB | Action details |
| `ip_address` | INET | Client IP |
| `user_agent` | TEXT | Client user agent |
| `created_at` | TIMESTAMPTZ | Action timestamp |

### `np_romdisc_legal_acceptance`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `user_id` | VARCHAR(255) | User who accepted |
| `disclaimer_version` | VARCHAR(32) | Disclaimer version accepted |
| `accepted_at` | TIMESTAMPTZ | Acceptance timestamp |
| `ip_address` | INET | Client IP at acceptance |

---

## Features

- **Full-text search** with PostgreSQL tsvector and trigram similarity for fuzzy matching
- **Quality and popularity scoring** for ROM ranking and discovery
- **Legal compliance** with DMCA-aware disclaimer, acceptance tracking, and HTTP 451 responses
- **Audit logging** with CSV export for compliance reporting
- **Automated scrapers** with cron scheduling and 8 pre-seeded scraper configurations
- **Download queue** with progress tracking, checksum verification, and retry logic
- **Platform statistics** with ROM counts, verified counts, homebrew counts, and average quality
- **ROM classification**: verified dumps, homebrew, hacks, translations, release groups
- **Integration with Retro Gaming plugin** for seamless library import
- **Multi-app isolation** via `source_account_id` on all tables

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Search returns no results | Run scrapers first to populate the database: `scrapers run <name>` |
| Full-text search not working | Ensure `pg_trgm` extension is installed in PostgreSQL |
| Download returns HTTP 451 | User must accept legal disclaimer first via `POST /api/legal/accept` |
| Scraper fails with timeout | Check network access to scraper source URLs |
| Download queue stuck | Check `ROM_DISCOVERY_MAX_CONCURRENT_DOWNLOADS` and retry failed items |
| Audit export empty | Verify audit log entries exist with `GET /api/audit` |
