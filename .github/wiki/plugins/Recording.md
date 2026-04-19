# Recording Plugin

Orchestrate video recordings with schedule management, encoding pipelines, sports event auto-scheduling, and device integration.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Views](#views)
- [Webhooks](#webhooks)
- [Features](#features)
- [Troubleshooting](#troubleshooting)

---

## Overview

| Field | Value |
|-------|-------|
| **Version** | 1.0.0 |
| **Category** | streaming |
| **Port** | 3602 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`) |

The Recording plugin manages the full lifecycle of video recordings: scheduling, capture, encoding, publishing, and archival. It integrates with the Sports plugin for automatic game recording and with device webhooks for hardware-triggered captures.

---

## Quick Start

```bash
nself plugin install recording
nself plugin recording init
nself plugin recording server
nself plugin recording status
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
| `RECORDING_PLUGIN_PORT` | `3602` | Server port |
| `RECORDING_STORAGE_URL` | - | Storage endpoint URL |
| `RECORDING_STORAGE_BUCKET` | - | Storage bucket name |
| `RECORDING_CDN_URL` | - | CDN URL for published recordings |
| `RECORDING_ENCODE_PROFILES` | - | JSON array of encode profile names |
| `RECORDING_LEAD_TIME_MINUTES` | `5` | Minutes before scheduled start to begin |
| `RECORDING_TRAIL_TIME_MINUTES` | `15` | Minutes after scheduled end to continue |
| `RECORDING_MAX_CONCURRENT` | `5` | Maximum concurrent recordings |
| `RECORDING_MAX_CONCURRENT_ENCODES` | `3` | Maximum concurrent encode jobs |
| `RECORDING_API_KEY` | - | API key for authentication |
| `RECORDING_RATE_LIMIT_MAX` | `100` | Rate limit max requests |
| `RECORDING_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window |

---

## CLI Commands

| Command | Description | Options |
|---------|-------------|---------|
| `init` | Initialize database schema | - |
| `server` | Start the API server | `-p, --port`, `-h, --host` |
| `status` | Show recording status and statistics | - |
| `recordings list` | List recordings | `--status`, `-l, --limit` |
| `recordings create` | Create a new recording | `--title`, `--source`, `--channel` |
| `recordings cancel <id>` | Cancel a recording | - |
| `recordings delete <id>` | Delete a recording | - |
| `schedule` | Create a schedule entry | `--sport-event-id <id>` |
| `archives list` | List archived recordings | - |
| `encode-status` | Show encoding job status | - |
| `publish <id>` | Publish a recording | - |
| `stats` | Show detailed statistics | - |

---

## REST API

### Recordings

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/recordings` | List recordings with filters |
| `POST` | `/api/recordings` | Create a new recording |
| `GET` | `/api/recordings/:id` | Get recording details |
| `PUT` | `/api/recordings/:id` | Update a recording |
| `DELETE` | `/api/recordings/:id` | Delete a recording |
| `POST` | `/api/recordings/:id/start` | Start a recording |
| `POST` | `/api/recordings/:id/stop` | Stop a recording |
| `POST` | `/api/recordings/:id/cancel` | Cancel a recording |
| `POST` | `/api/recordings/:id/encode` | Submit for encoding |
| `POST` | `/api/recordings/:id/publish` | Publish a recording |
| `POST` | `/api/recordings/:id/unpublish` | Unpublish a recording |
| `POST` | `/api/recordings/:id/enrich` | Enrich metadata |
| `GET` | `/api/recordings/:id/stream-url` | Get stream URL |

### Schedules

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/schedules` | List schedules |
| `POST` | `/api/schedules` | Create a schedule |
| `GET` | `/api/schedules/:id` | Get schedule details |
| `PUT` | `/api/schedules/:id` | Update a schedule |
| `DELETE` | `/api/schedules/:id` | Delete a schedule |

### Encode Jobs and Webhooks

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/encode-jobs` | List encode jobs |
| `POST` | `/webhooks/sports` | Receive sports event webhook |
| `POST` | `/webhooks/device` | Receive device status webhook |
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check |
| `GET` | `/live` | Liveness check with stats |
| `GET` | `/api/stats` | Recording statistics |

---

## Database Schema

### `rec_recordings`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `title` | VARCHAR(512) | Recording title |
| `description` | TEXT | Description |
| `source_type` | VARCHAR(32) | live / scheduled / manual / sports |
| `source_id` | VARCHAR(255) | Source reference ID |
| `channel_id` | VARCHAR(255) | Channel identifier |
| `channel_name` | VARCHAR(255) | Channel display name |
| `status` | VARCHAR(32) | scheduled / recording / stopping / completed / encoding / encoded / published / failed / cancelled |
| `scheduled_start` | TIMESTAMPTZ | Scheduled start time |
| `scheduled_end` | TIMESTAMPTZ | Scheduled end time |
| `actual_start` | TIMESTAMPTZ | Actual recording start |
| `actual_end` | TIMESTAMPTZ | Actual recording end |
| `duration_seconds` | INTEGER | Duration in seconds |
| `file_path` | TEXT | Storage file path |
| `file_size_bytes` | BIGINT | File size |
| `file_format` | VARCHAR(32) | File format (mp4, ts, mkv) |
| `thumbnail_url` | TEXT | Thumbnail URL |
| `publish_url` | TEXT | Published URL |
| `publish_status` | VARCHAR(32) | draft / published / unpublished |
| `enrichment_status` | VARCHAR(32) | Metadata enrichment status |
| `quality_score` | INTEGER | Quality score (0-100) |
| `error_message` | TEXT | Error description |
| `metadata` | JSONB | Custom metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

### `rec_schedules`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `title` | VARCHAR(512) | Schedule title |
| `schedule_type` | VARCHAR(32) | once / recurring / sport_event |
| `cron_expression` | VARCHAR(255) | Cron for recurring |
| `sport_event_id` | VARCHAR(255) | Linked sport event |
| `channel_id` | VARCHAR(255) | Channel to record |
| `channel_name` | VARCHAR(255) | Channel name |
| `duration_minutes` | INTEGER | Expected duration |
| `lead_time_minutes` | INTEGER | Start early by N minutes |
| `trail_time_minutes` | INTEGER | Continue after end by N minutes |
| `enabled` | BOOLEAN | Whether schedule is active |
| `last_triggered_at` | TIMESTAMPTZ | Last trigger time |
| `next_trigger_at` | TIMESTAMPTZ | Next trigger time |
| `metadata` | JSONB | Custom metadata |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

### `rec_encode_jobs`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `recording_id` | UUID | FK to rec_recordings |
| `profile` | VARCHAR(64) | Encode profile name |
| `status` | VARCHAR(32) | pending / running / completed / failed |
| `progress_percent` | INTEGER | Encode progress (0-100) |
| `input_path` | TEXT | Source file path |
| `output_path` | TEXT | Output file path |
| `output_size_bytes` | BIGINT | Output file size |
| `error_message` | TEXT | Error description |
| `started_at` | TIMESTAMPTZ | Job start time |
| `completed_at` | TIMESTAMPTZ | Job completion time |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

---

## Views

| View | Description |
|------|-------------|
| `rec_recordings_by_status` | Recording counts grouped by status |
| `rec_storage_by_type` | Storage usage grouped by source type |
| `rec_success_rate` | Recording success/failure rates |
| `rec_scheduled_vs_completed` | Scheduled vs actually completed recordings |

---

## Webhooks

### Incoming

| Path | Source | Description |
|------|--------|-------------|
| `POST /webhooks/sports` | Sports plugin | Auto-schedule recordings for game events |
| `POST /webhooks/device` | Device plugin | Hardware-triggered recording start/stop |

---

## Features

- **Full recording lifecycle**: schedule, capture, encode, publish, archive
- **Sports integration**: auto-schedule recordings from Sports plugin events with configurable lead/trail time
- **Device webhooks**: trigger recordings from hardware device events
- **Encode pipeline**: configurable encode profiles with progress tracking
- **Publishing workflow**: draft/published/unpublished states with CDN URL generation
- **Schedule management**: one-time, recurring (cron), and sport-event-linked schedules
- **Multi-app isolation**: all data scoped by `source_account_id`
- **Analytics views**: storage usage, success rates, completion tracking

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Recording stays in "scheduled" | Verify the schedule is enabled and `next_trigger_at` is set |
| Encoding fails | Check encode profile validity and storage path accessibility |
| Sports webhook not creating schedules | Ensure Sports plugin sends to `POST /webhooks/sports` |
| Storage full | Review `rec_storage_by_type` view and archive old recordings |
| Concurrent limit reached | Increase `RECORDING_MAX_CONCURRENT` or stagger schedules |
