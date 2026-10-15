# Retro Gaming Plugin

Retro game library management with ROM cataloging, save state sync, play session tracking, emulator core management, and metadata enrichment from IGDB and MobyGames.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
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
| **Port** | 3033 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`) |

The Retro Gaming plugin provides a complete retro game library management system. It catalogs ROMs across platforms, manages emulator cores, tracks save states and play sessions, and enriches metadata from external databases like IGDB and MobyGames.

---

## Quick Start

```bash
nself plugin install retro-gaming
nself build
```

The plugin binary runs an HTTP server with no plugin-specific CLI subcommands.
Schema migrations run at service startup. Use the [REST API](#rest-api) for ROMs,
cores, save states, and sessions.

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RETROGAMING_PORT` | `3033` | Server port |
| `RETROGAMING_HOST` | `0.0.0.0` | Server host |
| `RETROGAMING_IGDB_CLIENT_ID` | - | IGDB (Twitch) Client ID for metadata |
| `RETROGAMING_IGDB_CLIENT_SECRET` | - | IGDB (Twitch) Client Secret |
| `RETROGAMING_MOBYGAMES_API_KEY` | - | MobyGames API key for metadata |
| `RETROGAMING_STORAGE_BUCKET` | - | Object storage bucket for ROMs/saves |
| `RETROGAMING_STORAGE_PATH` | - | Storage path prefix |
| `RETROGAMING_CDN_URL` | - | CDN URL for serving files |
| `RETROGAMING_API_KEY` | - | API key for authentication |
| `RETROGAMING_RATE_LIMIT_MAX` | `100` | Rate limit max requests |
| `RETROGAMING_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window |

---

## REST API

These endpoints are the public surface declared in
[`plugin.json`](../../../free/retro-gaming/plugin.json) (`apiEndpoints`).
The current Go server registers `/api/v1/...` routes instead; the declared
`/api/games/...` surface below is not yet implemented by that server.
Check the deployed version's routes before calling these endpoints.

### ROMs

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/games/roms` | List ROMs with filters |
| `POST` | `/api/games/roms` | Add a ROM to the library |
| `GET` | `/api/games/roms/:id` | Get ROM details |
| `PATCH` | `/api/games/roms/:id` | Update ROM metadata |
| `DELETE` | `/api/games/roms/:id` | Remove a ROM |
| `POST` | `/api/games/roms/scan` | Scan storage for new ROMs |
| `POST` | `/api/games/roms/:id/enrich` | Enrich ROM metadata from IGDB |

### Save States

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/games/save-states/:rom_id` | List save states for a ROM |
| `POST` | `/api/games/save-states/:rom_id` | Upload a save state |
| `GET` | `/api/games/save-states/:rom_id/:slot` | Get a specific save state |
| `DELETE` | `/api/games/save-states/:rom_id/:slot` | Delete a save state |

### Emulator Cores

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/games/cores` | List all emulator cores |
| `GET` | `/api/games/cores/:platform` | Get recommended core for a platform |
| `GET` | `/api/games/cores/:core_name/download` | Get core download URL |
| `POST` | `/api/games/cores/installed` | Record core installation |
| `GET` | `/api/games/cores/installed` | List installed cores |

### Play Sessions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/games/sessions/start` | Start a play session |
| `POST` | `/api/games/sessions/:session_id/end` | End a play session |
| `GET` | `/api/games/sessions/recent` | Get recent sessions |

### Controllers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/games/controllers` | List controller configs |
| `POST` | `/api/games/controllers` | Create controller config |
| `DELETE` | `/api/games/controllers/:id` | Delete controller config |

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |

---

## Database Schema

### `np_retrogame_roms`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `title` | VARCHAR(512) | Game title |
| `platform` | VARCHAR(64) | Platform (nes, snes, gba, n64, etc.) |
| `region` | VARCHAR(32) | Region (USA, EUR, JPN) |
| `genre` | VARCHAR(64) | Game genre |
| `developer` | VARCHAR(255) | Developer name |
| `publisher` | VARCHAR(255) | Publisher name |
| `release_year` | INTEGER | Year of release |
| `file_name` | VARCHAR(512) | ROM file name |
| `file_path` | TEXT | Storage path |
| `file_size_bytes` | BIGINT | File size |
| `file_hash_md5` | VARCHAR(32) | MD5 checksum |
| `file_hash_sha1` | VARCHAR(40) | SHA1 checksum |
| `file_hash_crc32` | VARCHAR(8) | CRC32 checksum |
| `description` | TEXT | Game description |
| `cover_url` | TEXT | Cover art URL |
| `screenshot_urls` | JSONB | Screenshot URLs array |
| `igdb_id` | INTEGER | IGDB external ID |
| `mobygames_id` | INTEGER | MobyGames external ID |
| `rating` | DECIMAL | Game rating |
| `play_count` | INTEGER | Times played |
| `total_play_time_seconds` | INTEGER | Total playtime |
| `last_played_at` | TIMESTAMPTZ | Last played timestamp |
| `is_favorite` | BOOLEAN | Favorited by user |
| `is_verified` | BOOLEAN | ROM dump verified |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Added to library |
| `updated_at` | TIMESTAMPTZ | Last update |

### `np_retrogame_save_states`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `rom_id` | UUID | FK to np_retrogame_roms |
| `slot` | INTEGER | Save slot number |
| `name` | VARCHAR(255) | Save state name |
| `file_path` | TEXT | Storage path |
| `file_size_bytes` | BIGINT | File size |
| `screenshot_url` | TEXT | Screenshot at save point |
| `core_name` | VARCHAR(128) | Emulator core used |
| `core_version` | VARCHAR(32) | Core version |
| `is_auto_save` | BOOLEAN | Auto-save vs manual |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Save timestamp |

### `np_retrogame_play_sessions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `rom_id` | UUID | FK to np_retrogame_roms |
| `core_name` | VARCHAR(128) | Emulator core used |
| `started_at` | TIMESTAMPTZ | Session start |
| `ended_at` | TIMESTAMPTZ | Session end |
| `duration_seconds` | INTEGER | Session duration |
| `metadata` | JSONB | Session metadata |

### `np_retrogame_emulator_cores`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `name` | VARCHAR(128) | Core name |
| `display_name` | VARCHAR(255) | Display name |
| `platform` | VARCHAR(64) | Target platform |
| `version` | VARCHAR(32) | Core version |
| `download_url` | TEXT | Download URL |
| `description` | TEXT | Core description |
| `supported_extensions` | JSONB | Supported file extensions |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

Core records must be populated before clients can select an emulator; startup
migrations create the tables but do not seed default cores.

### `np_retrogame_controller_configs`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `name` | VARCHAR(255) | Config name |
| `platform` | VARCHAR(64) | Target platform |
| `controller_type` | VARCHAR(64) | Controller type |
| `button_mapping` | JSONB | Button mapping configuration |
| `is_default` | BOOLEAN | Default config for platform |
| `metadata` | JSONB | Additional metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

### `np_retrogame_core_installations`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `core_id` | UUID | FK to np_retrogame_emulator_cores |
| `installed_version` | VARCHAR(32) | Installed version |
| `install_path` | TEXT | Installation path |
| `installed_at` | TIMESTAMPTZ | Install timestamp |
| `last_used_at` | TIMESTAMPTZ | Last used timestamp |

---

## Features

- **ROM library management** with platform, genre, and region filtering
- **Metadata enrichment** from IGDB and MobyGames (cover art, descriptions, ratings)
- **Save state sync** with per-slot management and auto-save support
- **Play session tracking** with duration and frequency statistics
- **Emulator core management** with per-platform core records
- **Controller configuration** per-platform with custom button mappings
- **ROM scanning** to auto-discover ROMs from storage
- **Checksum verification** (MD5, SHA1, CRC32) for ROM integrity
- **Favorites and play history** for library organization
- **Multi-app isolation** via `source_account_id` on all tables
- **CDN integration** for serving ROM files, covers, and screenshots

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| IGDB enrichment fails | Verify `RETROGAMING_IGDB_CLIENT_ID` and `RETROGAMING_IGDB_CLIENT_SECRET` are set |
| ROMs not found during scan | Check `RETROGAMING_STORAGE_PATH` points to correct directory |
| Save states not loading | Ensure core version matches the version used when saving |
| No emulator cores listed | Startup migrations create tables without default cores. The current server supports creating core records with `POST /api/v1/emulator-cores/`; list them with `GET /api/v1/emulator-cores/`. |
| Cover art not displaying | Check `RETROGAMING_CDN_URL` is configured and accessible |
