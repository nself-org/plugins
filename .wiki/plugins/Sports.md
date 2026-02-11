# Sports Plugin

Sports schedule and metadata synchronization plugin - ingests game schedules, team rosters, scores, and standings

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Sports plugin provides comprehensive sports data synchronization for the nself platform. It ingests game schedules, team rosters, live scores, and standings from multiple providers, enabling sports-based applications and automated recording triggers.

### Key Features

- **Multi-Provider Support** - ESPN, SportsData.io, and extensible provider architecture
- **Multi-Sport Support** - NFL, NBA, MLB, NHL, and more
- **Real-time Scores** - Live game updates with score tracking
- **Schedule Management** - Complete event schedules with broadcast information
- **Team & League Data** - Full team rosters, league structures, and venue details
- **Event Locking** - Prevent schedule changes near game time
- **Operator Overrides** - Manual schedule corrections and broadcast channel updates
- **Recording Integration** - Auto-trigger recordings for upcoming games
- **Schedule Caching** - High-performance caching layer for frequent queries
- **Webhook Events** - Real-time notifications for score updates and game starts
- **Multi-Account Support** - `source_account_id` isolation for multi-workspace deployments

### Supported Sports & Leagues

| Sport | Leagues | Provider |
|-------|---------|----------|
| Football | NFL, NCAA | ESPN, SportsData.io |
| Basketball | NBA, WNBA, NCAA | ESPN, SportsData.io |
| Baseball | MLB | ESPN, SportsData.io |
| Hockey | NHL | ESPN, SportsData.io |

---

## Quick Start

```bash
# Install the plugin
nself plugin install sports

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export SPORTS_PLUGIN_PORT=3201

# Initialize database schema
nself plugin sports init

# Start the server
nself plugin sports server --port 3201

# Sync data from providers
nself plugin sports sync

# Check status
nself plugin sports status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `SPORTS_PLUGIN_PORT` | No | `3201` | HTTP server port |
| `SPORTS_PROVIDERS` | No | `espn` | Comma-separated provider list (espn,sportsdata) |
| `SPORTS_ESPN_API_URL` | No | `https://site.api.espn.com` | ESPN API base URL |
| `SPORTS_SPORTSDATA_API_KEY` | No | - | SportsData.io API key |
| `SPORTS_SPORTSDATA_API_URL` | No | `https://api.sportsdata.io` | SportsData.io base URL |
| `SPORTS_SYNC_INTERVAL` | No | `3600` | Sync interval in seconds (1 hour) |
| `SPORTS_LIVE_POLL_INTERVAL` | No | `30` | Live game polling interval in seconds |
| `SPORTS_ENABLED_SPORTS` | No | `football,basketball,baseball,hockey` | Enabled sports |
| `SPORTS_ENABLED_LEAGUES` | No | `nfl,nba,mlb,nhl` | Enabled leagues |
| `SPORTS_LOCK_WINDOW_MINUTES` | No | `120` | Auto-lock window before game (minutes) |
| `SPORTS_LOCK_AUTO` | No | `true` | Enable automatic event locking |
| `SPORTS_RECORDING_PLUGIN_URL` | No | - | Recording plugin webhook URL |
| `SPORTS_AUTO_TRIGGER_RECORDINGS` | No | `false` | Auto-trigger recording plugin |
| `SPORTS_RECORDING_LEAD_TIME_MINUTES` | No | `15` | Recording lead time before game |
| `SPORTS_RECORDING_TRAIL_TIME_MINUTES` | No | `60` | Recording trail time after game |
| `SPORTS_CACHE_SCHEDULE_TTL` | No | `21600` | Schedule cache TTL in seconds (6 hours) |
| `SPORTS_CACHE_LIVE_TTL` | No | `30` | Live data cache TTL in seconds |
| `SPORTS_CACHE_ENABLED` | No | `true` | Enable caching layer |
| `SPORTS_API_KEY` | No | - | API key for authentication (optional) |
| `SPORTS_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `SPORTS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=nself
POSTGRES_PASSWORD=secure_password
POSTGRES_SSL=false

# Server
SPORTS_PLUGIN_PORT=3201
SPORTS_PLUGIN_HOST=0.0.0.0

# Provider Configuration
SPORTS_PROVIDERS=espn,sportsdata
SPORTS_ESPN_API_URL=https://site.api.espn.com
SPORTS_SPORTSDATA_API_KEY=your_sportsdata_api_key
SPORTS_SPORTSDATA_API_URL=https://api.sportsdata.io

# Sync Configuration
SPORTS_SYNC_INTERVAL=3600
SPORTS_LIVE_POLL_INTERVAL=30
SPORTS_ENABLED_SPORTS=football,basketball,baseball,hockey
SPORTS_ENABLED_LEAGUES=nfl,nba,mlb,nhl

# Event Lock Configuration
SPORTS_LOCK_WINDOW_MINUTES=120
SPORTS_LOCK_AUTO=true

# Recording Integration
SPORTS_RECORDING_PLUGIN_URL=http://localhost:3602
SPORTS_AUTO_TRIGGER_RECORDINGS=true
SPORTS_RECORDING_LEAD_TIME_MINUTES=15
SPORTS_RECORDING_TRAIL_TIME_MINUTES=60

# Cache Configuration
SPORTS_CACHE_SCHEDULE_TTL=21600
SPORTS_CACHE_LIVE_TTL=30
SPORTS_CACHE_ENABLED=true

# Security (optional)
SPORTS_API_KEY=your_api_key_here
SPORTS_RATE_LIMIT_MAX=100
SPORTS_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin sports init

# Start the server
nself plugin sports server
nself plugin sports server --port 3201 --host 0.0.0.0

# Check status and statistics
nself plugin sports status
```

### Sync Commands

```bash
# Sync all data from all providers
nself plugin sports sync

# Sync from specific provider
nself plugin sports sync --provider espn

# Sync specific sport
nself plugin sports sync --sport football

# Sync specific league
nself plugin sports sync --league nfl

# Sync specific season
nself plugin sports sync --season 2026

# Reconcile recent data (7 days)
nself plugin sports reconcile

# Reconcile custom timeframe
nself plugin sports reconcile --days 14
```

### League Commands

```bash
# List all leagues
nself plugin sports leagues

# Filter by sport
nself plugin sports leagues --sport football
```

### Team Commands

```bash
# List all teams
nself plugin sports teams

# Filter by league
nself plugin sports teams --league <league-id>

# Filter by sport
nself plugin sports teams --sport basketball

# Search teams
nself plugin sports teams --search "Lakers"
```

### Event Commands

```bash
# List all events
nself plugin sports events

# Show today's events
nself plugin sports events --today

# Show live events
nself plugin sports events --live

# Show upcoming events (7 days)
nself plugin sports events --upcoming

# Filter by league
nself plugin sports events --league <league-id>

# Filter by team
nself plugin sports events --team <team-id>

# Limit results
nself plugin sports events --limit 100
```

### Lock Commands

```bash
# Lock an event
nself plugin sports lock <event-id>

# Lock with custom reason
nself plugin sports lock <event-id> --reason "Broadcast conflict"

# Unlock an event
nself plugin sports unlock <event-id>
```

### Override Commands

```bash
# Override event schedule
nself plugin sports override <event-id> --time 2026-09-10T18:00:00Z

# Override broadcast channel
nself plugin sports override <event-id> --channel "NBC"

# Override with notes
nself plugin sports override <event-id> --notes "Rescheduled due to weather"
```

### Cache Commands

```bash
# Show cache status
nself plugin sports cache status

# Clear all cache
nself plugin sports cache clear
```

---

## REST API

### Base URL

```
http://localhost:3201
```

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "sports",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check endpoint (checks database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "sports",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness endpoint with runtime stats.

**Response:**
```json
{
  "alive": true,
  "plugin": "sports",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 104857600,
    "heapTotal": 52428800,
    "heapUsed": 41943040,
    "external": 1048576
  },
  "stats": {
    "leagues": 12,
    "teams": 145,
    "events": 1523,
    "liveEvents": 3
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /status
Overall plugin status and configuration.

**Response:**
```json
{
  "plugin": "sports",
  "version": "1.0.0",
  "status": "running",
  "providers": ["espn", "sportsdata"],
  "stats": {
    "leagues": 12,
    "teams": 145,
    "events": 1523,
    "upcoming_events": 234,
    "live_events": 3,
    "by_provider": {
      "espn": 1200,
      "sportsdata": 323
    },
    "last_sync": "2026-02-11T09:00:00.000Z"
  },
  "syncStats": {
    "leagues": 12,
    "teams": 145,
    "events": 1523,
    "by_provider": {
      "espn": {
        "leagues": 8,
        "teams": 100,
        "events": 1200
      },
      "sportsdata": {
        "leagues": 4,
        "teams": 45,
        "events": 323
      }
    },
    "last_sync": "2026-02-11T09:00:00.000Z"
  },
  "config": {
    "enabledSports": ["football", "basketball", "baseball", "hockey"],
    "enabledLeagues": ["nfl", "nba", "mlb", "nhl"],
    "lockWindowMinutes": 120,
    "cacheEnabled": true
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### League Endpoints

#### GET /api/leagues
List all leagues.

**Query Parameters:**
- `sport` (optional): Filter by sport (e.g., football, basketball)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "source_account_id": "primary",
      "external_id": "nfl",
      "provider": "espn",
      "name": "National Football League",
      "abbreviation": "NFL",
      "sport": "football",
      "country": "USA",
      "season_type": "regular",
      "current_season": "2026",
      "logo_url": "https://example.com/nfl-logo.png",
      "metadata": {},
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-02-11T10:00:00.000Z",
      "synced_at": "2026-02-11T09:00:00.000Z"
    }
  ],
  "total": 12
}
```

#### GET /api/leagues/:id
Get a specific league by ID.

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "external_id": "nfl",
  "provider": "espn",
  "name": "National Football League",
  "abbreviation": "NFL",
  "sport": "football",
  "country": "USA",
  "season_type": "regular",
  "current_season": "2026",
  "logo_url": "https://example.com/nfl-logo.png",
  "metadata": {},
  "created_at": "2026-01-01T00:00:00.000Z",
  "updated_at": "2026-02-11T10:00:00.000Z",
  "synced_at": "2026-02-11T09:00:00.000Z"
}
```

### Team Endpoints

#### GET /api/teams
List all teams.

**Query Parameters:**
- `league_id` (optional): Filter by league ID
- `sport` (optional): Filter by sport
- `search` (optional): Search by name, city, or abbreviation

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "source_account_id": "primary",
      "league_id": "550e8400-e29b-41d4-a716-446655440000",
      "external_id": "dal",
      "provider": "espn",
      "name": "Dallas Cowboys",
      "abbreviation": "DAL",
      "city": "Dallas",
      "conference": "NFC",
      "division": "East",
      "logo_url": "https://example.com/cowboys-logo.png",
      "primary_color": "#041E42",
      "secondary_color": "#869397",
      "venue_name": "AT&T Stadium",
      "venue_city": "Arlington",
      "venue_timezone": "America/Chicago",
      "metadata": {},
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-02-11T10:00:00.000Z",
      "synced_at": "2026-02-11T09:00:00.000Z"
    }
  ],
  "total": 145
}
```

#### GET /api/teams/:id
Get a specific team by ID.

**Response:** Same format as single team in list above.

### Event Endpoints

#### GET /api/events
List all events.

**Query Parameters:**
- `league_id` (optional): Filter by league ID
- `team_id` (optional): Filter by team ID (home or away)
- `status` (optional): Filter by status (scheduled, in_progress, final, etc.)
- `from` (optional): Start date/time (ISO 8601)
- `to` (optional): End date/time (ISO 8601)
- `season` (optional): Filter by season (e.g., 2026)
- `week` (optional): Filter by week number
- `limit` (optional): Limit results (default: 50)
- `offset` (optional): Offset for pagination (default: 0)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "source_account_id": "primary",
      "external_id": "401547410",
      "provider": "espn",
      "canonical_id": "nfl-2026-reg-1-dal-nyg",
      "league_id": "550e8400-e29b-41d4-a716-446655440000",
      "home_team_id": "550e8400-e29b-41d4-a716-446655440001",
      "away_team_id": "550e8400-e29b-41d4-a716-446655440003",
      "event_type": "regular",
      "status": "scheduled",
      "scheduled_at": "2026-09-10T20:20:00.000Z",
      "started_at": null,
      "ended_at": null,
      "venue_name": "AT&T Stadium",
      "venue_city": "Arlington",
      "venue_timezone": "America/Chicago",
      "broadcast_network": "NBC",
      "broadcast_channel": "NBC Sports",
      "season": "2026",
      "season_type": "regular",
      "week": 1,
      "home_score": null,
      "away_score": null,
      "period": null,
      "clock": null,
      "is_final": false,
      "is_locked": false,
      "lock_reason": null,
      "locked_at": null,
      "operator_override": false,
      "operator_notes": null,
      "recording_trigger_sent": false,
      "recording_trigger_sent_at": null,
      "metadata": {},
      "created_at": "2026-01-01T00:00:00.000Z",
      "updated_at": "2026-02-11T10:00:00.000Z",
      "synced_at": "2026-02-11T09:00:00.000Z",
      "deleted_at": null,
      "league_name": "National Football League",
      "sport": "football",
      "home_team_name": "Dallas Cowboys",
      "home_abbr": "DAL",
      "away_team_name": "New York Giants",
      "away_abbr": "NYG"
    }
  ],
  "total": 1523
}
```

#### GET /api/events/upcoming
List upcoming events (next 7 days).

**Query Parameters:**
- `league_id` (optional): Filter by league ID
- `team_id` (optional): Filter by team ID
- `limit` (optional): Limit results (default: 50)

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "scheduled_at": "2026-09-10T20:20:00.000Z",
      "home_team_name": "Dallas Cowboys",
      "away_team_name": "New York Giants",
      "league_name": "National Football League",
      "status": "scheduled"
    }
  ]
}
```

#### GET /api/events/live
List currently live events.

**Query Parameters:**
- `league_id` (optional): Filter by league ID

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440004",
      "home_team_name": "Los Angeles Lakers",
      "away_team_name": "Boston Celtics",
      "home_score": 98,
      "away_score": 95,
      "period": "4th Quarter",
      "clock": "3:42",
      "status": "in_progress"
    }
  ]
}
```

#### GET /api/events/today
List today's events.

**Query Parameters:**
- `league_id` (optional): Filter by league ID
- `timezone` (optional): Timezone for "today" (default: UTC)

**Response:** Same format as events list.

#### GET /api/events/:id
Get a specific event by ID.

**Response:** Same format as single event in list above.

#### POST /api/events/:id/lock
Lock an event to prevent schedule changes.

**Request Body:**
```json
{
  "reason": "Broadcast scheduling finalized"
}
```

**Response:**
```json
{
  "locked": true,
  "event": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "is_locked": true,
    "lock_reason": "Broadcast scheduling finalized",
    "locked_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### POST /api/events/:id/unlock
Unlock an event.

**Response:**
```json
{
  "locked": false,
  "event": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "is_locked": false,
    "lock_reason": null,
    "locked_at": null
  }
}
```

#### POST /api/events/:id/override
Manually override event details.

**Request Body:**
```json
{
  "scheduled_at": "2026-09-10T21:00:00.000Z",
  "broadcast_channel": "ESPN",
  "notes": "Rescheduled due to weather"
}
```

**Response:** Updated event object.

#### POST /api/events/:id/trigger-recording
Manually trigger recording for an event.

**Response:**
```json
{
  "triggered": true,
  "event_id": "550e8400-e29b-41d4-a716-446655440002"
}
```

### Sync Endpoints

#### POST /sync
Trigger a full sync from all providers.

**Request Body:**
```json
{
  "providers": ["espn", "sportsdata"]
}
```

**Response:**
```json
{
  "success": true,
  "stats": {
    "total_synced": 1523
  },
  "errors": [],
  "duration_ms": 45678
}
```

#### POST /reconcile
Reconcile recent data.

**Request Body:**
```json
{
  "lookback_days": 7
}
```

**Response:**
```json
{
  "success": true,
  "stats": {
    "lookback_days": 7
  }
}
```

### Cache Endpoints

#### GET /api/cache/status
Get cache statistics.

**Response:**
```json
{
  "entries": 4523,
  "expired": 234,
  "active": 4289
}
```

### Stats Endpoint

#### GET /api/stats
Get overall plugin statistics.

**Response:**
```json
{
  "leagues": 12,
  "teams": 145,
  "events": 1523,
  "by_provider": {
    "espn": {
      "leagues": 8,
      "teams": 100,
      "events": 1200
    },
    "sportsdata": {
      "leagues": 4,
      "teams": 45,
      "events": 323
    }
  },
  "last_sync": "2026-02-11T09:00:00.000Z"
}
```

### Webhook Endpoint

#### POST /webhooks/:provider
Receive webhooks from external providers.

**Request Body:** Provider-specific webhook payload.

**Response:**
```json
{
  "received": true
}
```

---

## Webhook Events

The Sports plugin can send webhook events to other services (e.g., the Recording plugin).

### game.score_updated
Sent when a game score is updated during live play.

**Payload:**
```json
{
  "type": "game.score_updated",
  "event_id": "550e8400-e29b-41d4-a716-446655440002",
  "home_score": 21,
  "away_score": 14,
  "period": "2nd Quarter",
  "clock": "8:34",
  "timestamp": "2026-09-10T20:45:00.000Z"
}
```

### game.started
Sent when a game officially starts.

**Payload:**
```json
{
  "type": "game.started",
  "event_id": "550e8400-e29b-41d4-a716-446655440002",
  "started_at": "2026-09-10T20:20:00.000Z",
  "home_team": "Dallas Cowboys",
  "away_team": "New York Giants",
  "venue": "AT&T Stadium"
}
```

---

## Database Schema

### sports_leagues
Stores league information.

```sql
CREATE TABLE sports_leagues (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  external_id VARCHAR(255) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  abbreviation VARCHAR(32),
  sport VARCHAR(64) NOT NULL,
  country VARCHAR(3),
  season_type VARCHAR(32),
  current_season VARCHAR(32),
  logo_url TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, provider, external_id)
);

CREATE INDEX idx_sports_leagues_source_account ON sports_leagues(source_account_id);
CREATE INDEX idx_sports_leagues_sport ON sports_leagues(sport);
CREATE INDEX idx_sports_leagues_provider ON sports_leagues(provider);
```

### sports_teams
Stores team information.

```sql
CREATE TABLE sports_teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  league_id UUID REFERENCES sports_leagues(id),
  external_id VARCHAR(255) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  name VARCHAR(255) NOT NULL,
  abbreviation VARCHAR(16),
  city VARCHAR(128),
  conference VARCHAR(128),
  division VARCHAR(128),
  logo_url TEXT,
  primary_color VARCHAR(7),
  secondary_color VARCHAR(7),
  venue_name VARCHAR(255),
  venue_city VARCHAR(128),
  venue_timezone VARCHAR(64),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, provider, external_id)
);

CREATE INDEX idx_sports_teams_source_account ON sports_teams(source_account_id);
CREATE INDEX idx_sports_teams_league ON sports_teams(league_id);
CREATE INDEX idx_sports_teams_abbr ON sports_teams(abbreviation);
```

### sports_events
Stores game/match events.

```sql
CREATE TABLE sports_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  external_id VARCHAR(255) NOT NULL,
  provider VARCHAR(64) NOT NULL,
  canonical_id VARCHAR(255),
  league_id UUID REFERENCES sports_leagues(id),
  home_team_id UUID REFERENCES sports_teams(id),
  away_team_id UUID REFERENCES sports_teams(id),
  event_type VARCHAR(32) NOT NULL DEFAULT 'regular',
  status VARCHAR(32) NOT NULL DEFAULT 'scheduled',
  scheduled_at TIMESTAMPTZ NOT NULL,
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  venue_name VARCHAR(255),
  venue_city VARCHAR(128),
  venue_timezone VARCHAR(64),
  broadcast_network VARCHAR(128),
  broadcast_channel VARCHAR(128),
  season VARCHAR(32),
  season_type VARCHAR(32),
  week INTEGER,
  home_score INTEGER,
  away_score INTEGER,
  period VARCHAR(32),
  clock VARCHAR(16),
  is_final BOOLEAN DEFAULT FALSE,
  is_locked BOOLEAN DEFAULT FALSE,
  lock_reason VARCHAR(255),
  locked_at TIMESTAMPTZ,
  operator_override BOOLEAN DEFAULT FALSE,
  operator_notes TEXT,
  recording_trigger_sent BOOLEAN DEFAULT FALSE,
  recording_trigger_sent_at TIMESTAMPTZ,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  deleted_at TIMESTAMPTZ,
  UNIQUE(source_account_id, provider, external_id)
);

CREATE INDEX idx_sports_events_source_account ON sports_events(source_account_id);
CREATE INDEX idx_sports_events_league ON sports_events(league_id);
CREATE INDEX idx_sports_events_scheduled ON sports_events(scheduled_at);
CREATE INDEX idx_sports_events_status ON sports_events(status);
CREATE INDEX idx_sports_events_home ON sports_events(home_team_id);
CREATE INDEX idx_sports_events_away ON sports_events(away_team_id);
CREATE INDEX idx_sports_events_canonical ON sports_events(canonical_id);
CREATE INDEX idx_sports_events_season ON sports_events(season, week);
```

### sports_provider_syncs
Tracks provider sync operations.

```sql
CREATE TABLE sports_provider_syncs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider VARCHAR(64) NOT NULL,
  resource_type VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  records_synced INTEGER DEFAULT 0,
  errors JSONB DEFAULT '[]',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sports_syncs_source_account ON sports_provider_syncs(source_account_id);
CREATE INDEX idx_sports_syncs_provider ON sports_provider_syncs(provider);
```

### sports_schedule_cache
High-performance schedule caching.

```sql
CREATE TABLE sports_schedule_cache (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider VARCHAR(64) NOT NULL,
  cache_key VARCHAR(255) NOT NULL,
  data JSONB NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE(source_account_id, provider, cache_key)
);

CREATE INDEX idx_sports_cache_source_account ON sports_schedule_cache(source_account_id);
CREATE INDEX idx_sports_cache_expires ON sports_schedule_cache(expires_at);
```

### sports_webhook_events
Logs incoming webhook events.

```sql
CREATE TABLE sports_webhook_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider VARCHAR(64) NOT NULL,
  event_type VARCHAR(128) NOT NULL,
  event_id VARCHAR(255),
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT FALSE,
  processed_at TIMESTAMPTZ,
  error TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sports_webhooks_source_account ON sports_webhook_events(source_account_id);
CREATE INDEX idx_sports_webhooks_type ON sports_webhook_events(event_type);
CREATE INDEX idx_sports_webhooks_processed ON sports_webhook_events(processed);
```

### Analytics Views

#### sports_upcoming_events
Pre-joined view of upcoming events (7 days).

```sql
CREATE OR REPLACE VIEW sports_upcoming_events AS
SELECT e.*, l.name AS league_name, l.sport,
       ht.name AS home_team_name, ht.abbreviation AS home_abbr,
       at2.name AS away_team_name, at2.abbreviation AS away_abbr
FROM sports_events e
JOIN sports_leagues l ON e.league_id = l.id
LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
WHERE e.status = 'scheduled'
  AND e.scheduled_at BETWEEN NOW() AND NOW() + INTERVAL '7 days'
  AND e.deleted_at IS NULL
ORDER BY e.scheduled_at ASC;
```

#### sports_live_events
Pre-joined view of live events.

```sql
CREATE OR REPLACE VIEW sports_live_events AS
SELECT e.*, l.name AS league_name, l.sport,
       ht.name AS home_team_name, ht.abbreviation AS home_abbr,
       at2.name AS away_team_name, at2.abbreviation AS away_abbr
FROM sports_events e
JOIN sports_leagues l ON e.league_id = l.id
LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
WHERE e.status IN ('in_progress', 'halftime', 'delayed')
  AND e.deleted_at IS NULL
ORDER BY e.started_at ASC;
```

#### sports_untriggered_recordings
Events that need recording triggers.

```sql
CREATE OR REPLACE VIEW sports_untriggered_recordings AS
SELECT e.*
FROM sports_events e
WHERE e.recording_trigger_sent = FALSE
  AND e.status = 'scheduled'
  AND e.scheduled_at BETWEEN NOW() AND NOW() + INTERVAL '24 hours'
  AND e.deleted_at IS NULL;
```

---

## Examples

### Example 1: Query Today's Games

```bash
# CLI
nself plugin sports events --today

# API
curl http://localhost:3201/api/events/today

# SQL
SELECT * FROM sports_events
WHERE DATE(scheduled_at AT TIME ZONE 'America/New_York') = CURRENT_DATE
  AND deleted_at IS NULL
ORDER BY scheduled_at;
```

### Example 2: Monitor Live Scores

```bash
# CLI (watch mode)
watch -n 10 'nself plugin sports events --live'

# API polling
while true; do
  curl http://localhost:3201/api/events/live | jq '.data[] | "\(.home_team_name) \(.home_score) - \(.away_score) \(.away_team_name)"'
  sleep 10
done

# SQL
SELECT
  ht.abbreviation AS home,
  e.home_score,
  at2.abbreviation AS away,
  e.away_score,
  e.period,
  e.clock,
  e.status
FROM sports_events e
LEFT JOIN sports_teams ht ON e.home_team_id = ht.id
LEFT JOIN sports_teams at2 ON e.away_team_id = at2.id
WHERE e.status IN ('in_progress', 'halftime', 'delayed')
  AND e.deleted_at IS NULL;
```

### Example 3: Auto-trigger Recordings

```javascript
// Configure in .env
SPORTS_AUTO_TRIGGER_RECORDINGS=true
SPORTS_RECORDING_PLUGIN_URL=http://localhost:3602
SPORTS_RECORDING_LEAD_TIME_MINUTES=15
SPORTS_RECORDING_TRAIL_TIME_MINUTES=60

// The plugin will automatically:
// 1. Query sports_untriggered_recordings view every sync
// 2. For each event in the next 24 hours:
//    - Calculate recording start: scheduled_at - 15 minutes
//    - Calculate recording end: scheduled_at + estimated_duration + 60 minutes
//    - POST to recording plugin webhook
//    - Mark recording_trigger_sent = TRUE
```

### Example 4: Lock Events Before Broadcast

```bash
# Manually lock critical events
nself plugin sports lock <event-id> --reason "Network finalized"

# Auto-lock via SPORTS_LOCK_AUTO=true
# Events are automatically locked 120 minutes (SPORTS_LOCK_WINDOW_MINUTES) before start
# Locked events ignore schedule updates from providers
```

### Example 5: Override Incorrect Schedule

```bash
# Correct a rescheduled game
nself plugin sports override <event-id> \
  --time "2026-09-11T18:00:00Z" \
  --channel "ESPN2" \
  --notes "Rescheduled due to weather delay"

# API version
curl -X POST http://localhost:3201/api/events/<event-id>/override \
  -H "Content-Type: application/json" \
  -d '{
    "scheduled_at": "2026-09-11T18:00:00Z",
    "broadcast_channel": "ESPN2",
    "notes": "Rescheduled due to weather delay"
  }'
```

---

## Troubleshooting

### No events synced

**Issue:** Sync completes but no events in database.

**Solution:**
1. Check provider configuration:
   ```bash
   echo $SPORTS_PROVIDERS
   echo $SPORTS_ENABLED_SPORTS
   echo $SPORTS_ENABLED_LEAGUES
   ```
2. Verify API credentials if using SportsData.io:
   ```bash
   echo $SPORTS_SPORTSDATA_API_KEY
   ```
3. Check sync logs:
   ```bash
   LOG_LEVEL=debug nself plugin sports sync
   ```
4. Verify provider is reachable:
   ```bash
   curl https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard
   ```

### Cache not working

**Issue:** High API usage despite caching enabled.

**Solution:**
1. Verify cache is enabled:
   ```bash
   echo $SPORTS_CACHE_ENABLED
   ```
2. Check cache stats:
   ```bash
   nself plugin sports cache status
   # or
   curl http://localhost:3201/api/cache/status
   ```
3. Ensure TTL is reasonable:
   ```bash
   echo $SPORTS_CACHE_SCHEDULE_TTL  # Should be 3600-21600 (1-6 hours)
   ```
4. Clear and rebuild cache:
   ```bash
   nself plugin sports cache clear
   nself plugin sports sync
   ```

### Events locked unexpectedly

**Issue:** Cannot update event details due to lock.

**Solution:**
1. Check lock status:
   ```sql
   SELECT id, home_team_name, scheduled_at, is_locked, lock_reason, locked_at
   FROM sports_events
   WHERE is_locked = TRUE;
   ```
2. Disable auto-lock if needed:
   ```bash
   export SPORTS_LOCK_AUTO=false
   ```
3. Manually unlock event:
   ```bash
   nself plugin sports unlock <event-id>
   ```
4. Adjust lock window:
   ```bash
   export SPORTS_LOCK_WINDOW_MINUTES=60  # Lock 1 hour before instead of 2
   ```

### Recording triggers not firing

**Issue:** Events not triggering recordings automatically.

**Solution:**
1. Verify recording integration is configured:
   ```bash
   echo $SPORTS_AUTO_TRIGGER_RECORDINGS
   echo $SPORTS_RECORDING_PLUGIN_URL
   ```
2. Check untriggered events:
   ```sql
   SELECT * FROM sports_untriggered_recordings;
   ```
3. Verify recording plugin is reachable:
   ```bash
   curl http://localhost:3602/health
   ```
4. Check webhook event logs:
   ```sql
   SELECT * FROM sports_webhook_events
   WHERE event_type LIKE 'recording%'
   ORDER BY created_at DESC
   LIMIT 20;
   ```
5. Manually trigger for testing:
   ```bash
   curl -X POST http://localhost:3201/api/events/<event-id>/trigger-recording
   ```

### Live scores not updating

**Issue:** Scores stuck or stale during live games.

**Solution:**
1. Verify live polling is enabled:
   ```bash
   echo $SPORTS_LIVE_POLL_INTERVAL  # Should be 15-60 seconds
   ```
2. Check last sync time:
   ```bash
   nself plugin sports status | grep last_sync
   ```
3. Force a sync:
   ```bash
   nself plugin sports reconcile --days 1
   ```
4. Check provider rate limits:
   ```bash
   # ESPN public API has soft limits
   # SportsData.io has documented rate limits per plan
   ```

### Database connection issues

**Issue:** Plugin fails to start with database errors.

**Solution:**
1. Verify DATABASE_URL is correct:
   ```bash
   echo $DATABASE_URL
   ```
2. Test connection:
   ```bash
   psql $DATABASE_URL -c "SELECT 1;"
   ```
3. Initialize schema if needed:
   ```bash
   nself plugin sports init
   ```
4. Check PostgreSQL is running:
   ```bash
   pg_isready -h localhost -p 5432
   ```

### High memory usage

**Issue:** Plugin consuming excessive memory.

**Solution:**
1. Reduce cache size:
   ```bash
   export SPORTS_CACHE_SCHEDULE_TTL=3600  # 1 hour instead of 6
   ```
2. Clear old cache entries:
   ```sql
   DELETE FROM sports_schedule_cache WHERE expires_at < NOW() - INTERVAL '1 day';
   ```
3. Limit sync scope:
   ```bash
   export SPORTS_ENABLED_SPORTS=football  # Only sync one sport
   export SPORTS_ENABLED_LEAGUES=nfl      # Only sync one league
   ```
4. Monitor memory:
   ```bash
   curl http://localhost:3201/live | jq '.memory'
   ```

---

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- nself CLI: https://github.com/acamarata/nself
