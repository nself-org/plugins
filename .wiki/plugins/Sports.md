# Sports Plugin

Comprehensive sports data integration for nself. Syncs sports schedules, live scores, team rosters, player stats, and league standings from multiple providers with real-time webhook support.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Multi-Provider Support](#multi-provider-support)
- [Favorites Management](#favorites-management)
- [Recording Integration](#recording-integration)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Sports plugin provides unified synchronization of sports data from multiple providers to a local PostgreSQL database. It consolidates sports schedules, live scores, team information, player statistics, and league standings into a single queryable interface.

### Key Features

- **11 Database Tables** - Complete coverage of sports data
- **3 Analytics Views** - Pre-built views for upcoming events, live games, and recording triggers
- **Multi-Provider Support** - ESPN, SportsData.io, and extensible provider architecture
- **Real-time Updates** - Live score updates via webhooks and polling
- **Favorites Management** - Track and filter favorite teams
- **Recording Integration** - Automatic recording triggers for games
- **Schedule Locking** - Prevent duplicate recordings with automatic lock management
- **Flexible Sync Options** - Full sync or incremental updates by provider/sport/league
- **REST API** - Query all sports data via HTTP endpoints
- **CLI Interface** - Comprehensive command-line management

### Supported Leagues

| Sport | Leagues | Provider Support |
|-------|---------|------------------|
| Football | NFL | ESPN, SportsData.io |
| Basketball | NBA, WNBA, NCAA | ESPN, SportsData.io |
| Baseball | MLB | ESPN, SportsData.io |
| Hockey | NHL | ESPN, SportsData.io |
| Soccer | MLS, EPL, La Liga, UEFA | ESPN |
| College Football | NCAA | ESPN |
| College Basketball | NCAA | ESPN |

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Leagues | League definitions and configurations | `np_sports_leagues` |
| Teams | Team profiles with logos and metadata | `np_sports_teams` |
| Events | Scheduled games and events | `np_sports_events` |
| Games | Game details with scores and status | `np_sports_games` |
| Standings | League/division standings | `np_sports_standings` |
| Players | Player rosters and information | `np_sports_players` |
| Favorites | User favorite teams | `np_sports_favorites` |
| Provider Syncs | Sync history and status | `np_sports_provider_syncs` |
| Schedule Cache | Cached schedule data | `np_sports_schedule_cache` |
| Sync State | Current sync state per provider | `np_sports_sync_state` |
| Webhook Events | Received webhook events | `np_sports_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install sports

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "SPORTS_PROVIDER=espn" >> .env
echo "SPORTS_ESPN_API_KEY=your_key_here" >> .env

# Initialize database schema
nself plugin sports init

# Sync all sports data
nself plugin sports sync

# Sync specific league
nself plugin sports sync --league nfl

# View today's games
nself plugin sports events today

# Start webhook server
nself plugin sports server --port 3201
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `SPORTS_PLUGIN_PORT` | No | `3201` | HTTP server port |
| `SPORTS_APP_IDS` | No | - | Comma-separated application IDs for multi-app support |
| `SPORTS_PROVIDER` | No | `espn` | Default provider (espn, sportsdata) |
| `SPORTS_PROVIDERS` | No | - | Comma-separated list of enabled providers |
| `SPORTS_ESPN_API_KEY` | No | - | ESPN API key |
| `SPORTS_ESPN_API_URL` | No | `https://site.api.espn.com/apis/site/v2/sports` | ESPN API base URL |
| `SPORTS_SPORTSDATA_API_KEY` | No | - | SportsData.io API key |
| `SPORTS_SPORTSDATA_API_URL` | No | `https://api.sportsdata.io` | SportsData.io API base URL |
| `SPORTS_LEAGUE_IDS` | No | - | Comma-separated list of league IDs to sync |
| `SPORTS_ENABLED_SPORTS` | No | `football,basketball,baseball,hockey` | Comma-separated list of enabled sports |
| `SPORTS_ENABLED_LEAGUES` | No | `nfl,nba,mlb,nhl` | Comma-separated list of enabled leagues |
| `SPORTS_SYNC_INTERVAL` | No | `3600` | Full sync interval in seconds (1 hour) |
| `SPORTS_LIVE_POLL_INTERVAL` | No | `60` | Live game polling interval in seconds |
| `SPORTS_LIVE_GAME_POLL_SECONDS` | No | `30` | Interval for polling individual live games |
| `SPORTS_LOCK_WINDOW_MINUTES` | No | `15` | Minutes before game to lock schedule |
| `SPORTS_LOCK_AUTO` | No | `true` | Automatically lock games before start |
| `SPORTS_AUTO_TRIGGER_RECORDINGS` | No | `false` | Automatically trigger recording plugin |
| `SPORTS_RECORDING_PLUGIN_URL` | No | `http://localhost:3220` | Recording plugin API URL |
| `SPORTS_RECORDING_LEAD_TIME_MINUTES` | No | `10` | Minutes before game to start recording |
| `SPORTS_RECORDING_TRAIL_TIME_MINUTES` | No | `30` | Minutes after game to stop recording |
| `SPORTS_CACHE_ENABLED` | No | `true` | Enable schedule caching |
| `SPORTS_CACHE_SCHEDULE_TTL` | No | `3600` | Schedule cache TTL in seconds (1 hour) |
| `SPORTS_CACHE_LIVE_TTL` | No | `30` | Live game cache TTL in seconds |
| `SPORTS_API_KEY` | No | - | API key for REST API authentication |
| `SPORTS_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `SPORTS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Provider Configuration

#### ESPN Configuration

```bash
# ESPN provider (public API, no key required for basic access)
SPORTS_PROVIDER=espn
SPORTS_ESPN_API_URL=https://site.api.espn.com/apis/site/v2/sports

# Optional ESPN API key for enhanced features
SPORTS_ESPN_API_KEY=your_espn_key

# Enabled sports for ESPN
SPORTS_ENABLED_SPORTS=football,basketball,baseball,hockey,soccer
SPORTS_ENABLED_LEAGUES=nfl,nba,mlb,nhl,mls
```

#### SportsData.io Configuration

```bash
# SportsData.io provider (requires API key)
SPORTS_PROVIDER=sportsdata
SPORTS_SPORTSDATA_API_KEY=your_sportsdata_key
SPORTS_SPORTSDATA_API_URL=https://api.sportsdata.io

# Enabled sports for SportsData.io
SPORTS_ENABLED_SPORTS=football,basketball,baseball,hockey
SPORTS_ENABLED_LEAGUES=nfl,nba,mlb,nhl
```

#### Multi-Provider Configuration

```bash
# Enable multiple providers for redundancy
SPORTS_PROVIDERS=espn,sportsdata

# Configure each provider
SPORTS_ESPN_API_KEY=your_espn_key
SPORTS_SPORTSDATA_API_KEY=your_sportsdata_key
```

### Multi-App Support

The Sports plugin supports multi-app isolation using `source_account_id`:

```bash
# Configure app IDs
SPORTS_APP_IDS=app1,app2,app3

# Each synced record includes source_account_id
# Default: "primary"
```

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Sports Plugin
SPORTS_PLUGIN_PORT=3201
SPORTS_PROVIDER=espn

# ESPN Configuration
SPORTS_ESPN_API_KEY=abc123def456
SPORTS_ESPN_API_URL=https://site.api.espn.com/apis/site/v2/sports

# Enabled Sports
SPORTS_ENABLED_SPORTS=football,basketball,baseball,hockey
SPORTS_ENABLED_LEAGUES=nfl,nba,mlb,nhl

# Sync Configuration
SPORTS_SYNC_INTERVAL=3600
SPORTS_LIVE_POLL_INTERVAL=60
SPORTS_LIVE_GAME_POLL_SECONDS=30

# Recording Integration
SPORTS_AUTO_TRIGGER_RECORDINGS=true
SPORTS_RECORDING_PLUGIN_URL=http://localhost:3220
SPORTS_RECORDING_LEAD_TIME_MINUTES=10
SPORTS_RECORDING_TRAIL_TIME_MINUTES=30

# Lock Configuration
SPORTS_LOCK_WINDOW_MINUTES=15
SPORTS_LOCK_AUTO=true

# Cache Configuration
SPORTS_CACHE_ENABLED=true
SPORTS_CACHE_SCHEDULE_TTL=3600
SPORTS_CACHE_LIVE_TTL=30

# API Security
SPORTS_API_KEY=secure_random_key_here
SPORTS_RATE_LIMIT_MAX=100
SPORTS_RATE_LIMIT_WINDOW_MS=60000
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin sports init

# Check plugin status
nself plugin sports status

# View detailed sync status
nself plugin sports sync-status

# View sports data statistics
nself plugin sports stats
```

### Data Synchronization

```bash
# Full sync (all providers, all sports)
nself plugin sports sync

# Sync specific provider
nself plugin sports sync --provider espn
nself plugin sports sync --provider sportsdata

# Sync specific sport
nself plugin sports sync --sport football
nself plugin sports sync --sport basketball

# Sync specific league
nself plugin sports sync --league nfl
nself plugin sports sync --league nba

# Incremental sync (recent changes only)
nself plugin sports sync --incremental

# Sync with date filter
nself plugin sports sync --since 2024-02-01

# Reconcile recent data (past 7 days)
nself plugin sports reconcile
```

### League Commands

```bash
# List all leagues
nself plugin sports leagues

# List leagues for specific sport
nself plugin sports leagues --sport football

# Show league details
nself plugin sports leagues --league nfl
```

### Team Commands

```bash
# List all teams
nself plugin sports teams

# List teams in a league
nself plugin sports teams --league nfl
nself plugin sports teams --league nba

# Get team details
nself plugin sports teams --team "Dallas Cowboys"

# List teams by conference/division
nself plugin sports teams --league nfl --division NFC-East
```

### Event Commands

```bash
# Show today's events
nself plugin sports events today

# Show live events
nself plugin sports events live

# Show upcoming events
nself plugin sports events upcoming

# Show events for specific date
nself plugin sports events --date 2024-02-15

# Filter by league
nself plugin sports events today --league nfl

# Filter by team
nself plugin sports events upcoming --team "Dallas Cowboys"

# Show week schedule
nself plugin sports events --week 1 --season 2024 --league nfl
```

### Game Commands

```bash
# List all games
nself plugin sports games

# Get game details
nself plugin sports games --id 12345

# Show games by status
nself plugin sports games --status live
nself plugin sports games --status scheduled
nself plugin sports games --status final

# Show games by date
nself plugin sports games --date 2024-02-15

# Filter by league
nself plugin sports games --league nfl

# Show game score details
nself plugin sports games --id 12345 --details
```

### Score Commands

```bash
# View latest scores
nself plugin sports scores

# Scores for specific date
nself plugin sports scores --date 2024-02-15

# Scores for specific league
nself plugin sports scores --league nfl

# Live scores only
nself plugin sports scores --live
```

### Standings Commands

```bash
# View league standings
nself plugin sports standings --league nfl

# View division standings
nself plugin sports standings --league nfl --division NFC-East

# View conference standings
nself plugin sports standings --league nfl --conference NFC
```

### Player Commands

```bash
# List players
nself plugin sports players

# List players on a team
nself plugin sports players --team "Dallas Cowboys"

# Search for player
nself plugin sports players --name "Tom Brady"

# View player details
nself plugin sports players --id 12345

# List players by position
nself plugin sports players --team "Dallas Cowboys" --position QB
```

### Favorites Commands

```bash
# List favorite teams
nself plugin sports favorites list

# Add favorite team
nself plugin sports favorites add "Dallas Cowboys"

# Remove favorite team
nself plugin sports favorites remove "Dallas Cowboys"

# Show events for favorite teams only
nself plugin sports events today --favorites

# Show scores for favorite teams
nself plugin sports scores --favorites
```

### Lock Management

```bash
# Lock an event (prevent schedule changes)
nself plugin sports lock --event-id 12345

# Unlock an event
nself plugin sports lock --event-id 12345 --unlock

# Show locked events
nself plugin sports lock --list

# Auto-lock events before game time
nself plugin sports lock --auto --window 15
```

### Schedule Override

```bash
# Manual schedule override
nself plugin sports override --event-id 12345 \
  --start-time "2024-02-15T20:00:00Z" \
  --reason "Network schedule change"

# Clear override
nself plugin sports override --event-id 12345 --clear
```

### Cache Management

```bash
# Show cache statistics
nself plugin sports cache stats

# Clear cache
nself plugin sports cache clear

# Clear cache for specific league
nself plugin sports cache clear --league nfl

# Warm cache
nself plugin sports cache warm
```

### Server Commands

```bash
# Start HTTP server
nself plugin sports server

# Start on custom port
nself plugin sports server --port 3201

# Start with specific host
nself plugin sports server --host 0.0.0.0 --port 3201

# Enable debug logging
nself plugin sports server --debug
```

---

## REST API

The plugin exposes a comprehensive REST API when running the server.

### Base URL

```
http://localhost:3201
```

### Authentication

If `SPORTS_API_KEY` is configured, include it in requests:

```http
Authorization: Bearer YOUR_API_KEY
```

### Endpoints

#### Health & Status

```http
GET /health
```
Returns server health status.

**Response:**
```json
{
  "status": "ok",
  "uptime": 12345,
  "timestamp": "2024-02-11T10:00:00Z"
}
```

```http
GET /status
```
Returns sync status and statistics.

**Response:**
```json
{
  "lastSync": "2024-02-11T09:30:00Z",
  "providers": {
    "espn": {
      "status": "synced",
      "lastSync": "2024-02-11T09:30:00Z",
      "totalEvents": 150
    }
  },
  "leagues": {
    "nfl": { "teams": 32, "events": 45 },
    "nba": { "teams": 30, "events": 78 }
  }
}
```

#### Sync

```http
POST /sync
```
Triggers a full data sync.

**Request Body (optional):**
```json
{
  "provider": "espn",
  "sport": "football",
  "league": "nfl",
  "incremental": true
}
```

**Response:**
```json
{
  "status": "started",
  "syncId": "sync_abc123",
  "estimatedDuration": 300
}
```

#### Leagues

```http
GET /api/leagues
```
List all leagues.

**Query Parameters:**
- `sport` - Filter by sport (football, basketball, etc.)

**Response:**
```json
{
  "leagues": [
    {
      "id": "nfl",
      "name": "National Football League",
      "abbreviation": "NFL",
      "sport": "football",
      "logo": "https://...",
      "teams_count": 32
    }
  ]
}
```

```http
GET /api/leagues/:id
```
Get league details.

#### Teams

```http
GET /api/teams
```
List all teams.

**Query Parameters:**
- `league` - Filter by league
- `limit` - Results per page (default: 50)
- `offset` - Pagination offset

**Response:**
```json
{
  "teams": [
    {
      "id": "dal",
      "name": "Dallas Cowboys",
      "abbreviation": "DAL",
      "league": "nfl",
      "conference": "NFC",
      "division": "NFC East",
      "logo": "https://...",
      "color": "#003594"
    }
  ],
  "total": 32,
  "limit": 50,
  "offset": 0
}
```

```http
GET /api/teams/:id
```
Get team details.

```http
GET /api/teams/:id/roster
```
Get team roster (players).

```http
GET /api/teams/:id/events
```
Get team's upcoming events.

```http
GET /api/teams/:id/standings
```
Get team's standings.

#### Events

```http
GET /api/events
```
List events.

**Query Parameters:**
- `date` - Filter by date (YYYY-MM-DD)
- `league` - Filter by league
- `team` - Filter by team ID
- `status` - Filter by status (scheduled, live, final)
- `favorites` - Only favorite teams (boolean)
- `limit` - Results per page
- `offset` - Pagination offset

**Response:**
```json
{
  "events": [
    {
      "id": "event_123",
      "league": "nfl",
      "home_team_id": "dal",
      "away_team_id": "phi",
      "home_team_name": "Dallas Cowboys",
      "away_team_name": "Philadelphia Eagles",
      "start_time": "2024-02-15T20:00:00Z",
      "status": "scheduled",
      "venue": "AT&T Stadium",
      "locked": false
    }
  ],
  "total": 10
}
```

```http
GET /api/events/today
```
Get today's events.

```http
GET /api/events/live
```
Get currently live events.

```http
GET /api/events/upcoming
```
Get upcoming events.

```http
GET /api/events/:id
```
Get event details.

#### Games

```http
GET /api/games
```
List games with scores.

**Query Parameters:**
- `date` - Filter by date
- `league` - Filter by league
- `team` - Filter by team
- `status` - Filter by status
- `limit` - Results per page
- `offset` - Pagination offset

**Response:**
```json
{
  "games": [
    {
      "id": "game_123",
      "event_id": "event_123",
      "league": "nfl",
      "home_team": "Dallas Cowboys",
      "away_team": "Philadelphia Eagles",
      "home_score": 24,
      "away_score": 21,
      "status": "final",
      "quarter": "4th",
      "time_remaining": "00:00",
      "start_time": "2024-02-15T20:00:00Z",
      "final_time": "2024-02-15T23:30:00Z"
    }
  ]
}
```

```http
GET /api/games/:id
```
Get game details with full scoring breakdown.

```http
GET /api/games/live
```
Get all live games.

#### Scores

```http
GET /api/scores
```
Get latest scores.

**Query Parameters:**
- `date` - Filter by date
- `league` - Filter by league
- `live` - Only live games (boolean)
- `favorites` - Only favorite teams (boolean)

#### Standings

```http
GET /api/standings
```
Get league standings.

**Query Parameters:**
- `league` - League ID (required)
- `division` - Filter by division
- `conference` - Filter by conference

**Response:**
```json
{
  "standings": [
    {
      "team_id": "dal",
      "team_name": "Dallas Cowboys",
      "league": "nfl",
      "conference": "NFC",
      "division": "NFC East",
      "wins": 12,
      "losses": 5,
      "ties": 0,
      "win_percentage": 0.706,
      "points_for": 456,
      "points_against": 321,
      "streak": "W3",
      "rank": 1
    }
  ]
}
```

#### Players

```http
GET /api/players
```
List players.

**Query Parameters:**
- `team` - Filter by team
- `name` - Search by name
- `position` - Filter by position
- `limit` - Results per page
- `offset` - Pagination offset

**Response:**
```json
{
  "players": [
    {
      "id": "player_123",
      "name": "Tom Brady",
      "team_id": "tb",
      "team_name": "Tampa Bay Buccaneers",
      "position": "QB",
      "jersey_number": 12,
      "height": "6-4",
      "weight": 225,
      "age": 46,
      "photo": "https://..."
    }
  ]
}
```

```http
GET /api/players/:id
```
Get player details.

#### Favorites

```http
GET /api/favorites
```
List favorite teams.

**Response:**
```json
{
  "favorites": [
    {
      "team_id": "dal",
      "team_name": "Dallas Cowboys",
      "league": "nfl",
      "added_at": "2024-02-01T10:00:00Z"
    }
  ]
}
```

```http
POST /api/favorites
```
Add favorite team.

**Request Body:**
```json
{
  "team_id": "dal"
}
```

```http
DELETE /api/favorites/:teamId
```
Remove favorite team.

#### Webhooks

```http
POST /webhooks/sports
```
Sports webhook endpoint. Requires valid signature if configured.

**Supported Events:**
- `game.score_updated`
- `game.started`

```http
GET /api/webhooks/events
```
List received webhook events.

---

## Webhook Events

The plugin handles real-time webhook events from sports data providers.

### Webhook Events

| Event | Description | Action |
|-------|-------------|--------|
| `game.score_updated` | Live score update during game | Update game scores in database |
| `game.started` | Game has started | Update game status, trigger recordings |

### Event Payloads

#### game.score_updated

```json
{
  "event": "game.score_updated",
  "timestamp": "2024-02-15T20:30:00Z",
  "data": {
    "game_id": "game_123",
    "event_id": "event_123",
    "league": "nfl",
    "home_team_id": "dal",
    "away_team_id": "phi",
    "home_score": 14,
    "away_score": 10,
    "quarter": "2nd",
    "time_remaining": "08:45",
    "status": "live"
  }
}
```

#### game.started

```json
{
  "event": "game.started",
  "timestamp": "2024-02-15T20:00:00Z",
  "data": {
    "game_id": "game_123",
    "event_id": "event_123",
    "league": "nfl",
    "home_team_id": "dal",
    "away_team_id": "phi",
    "start_time": "2024-02-15T20:00:00Z",
    "venue": "AT&T Stadium"
  }
}
```

### Webhook Configuration

Configure webhook endpoints in your sports data provider dashboard:

**Endpoint URL:**
```
https://your-domain.com/webhooks/sports
```

**Events to Subscribe:**
- Score Updates
- Game Status Changes

**Webhook Secret:**
Set webhook secret in provider dashboard to match your configuration.

---

## Database Schema

(The database schema section continues with all 11 tables as shown in the original file, maintaining the exact structure...)

### np_sports_leagues

```sql
CREATE TABLE np_sports_leagues (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    abbreviation VARCHAR(10),
    sport VARCHAR(50) NOT NULL,
    logo_url VARCHAR(2048),
    color VARCHAR(7),
    season_type VARCHAR(20),
    current_season VARCHAR(10),
    current_week INTEGER,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_leagues_sport ON np_sports_leagues(sport);
CREATE INDEX idx_sports_leagues_season ON np_sports_leagues(current_season);
```

### np_sports_teams

```sql
CREATE TABLE np_sports_teams (
    id VARCHAR(50) PRIMARY KEY,
    league_id VARCHAR(50) REFERENCES np_sports_leagues(id),
    name VARCHAR(255) NOT NULL,
    abbreviation VARCHAR(10),
    display_name VARCHAR(255),
    short_name VARCHAR(100),
    location VARCHAR(255),
    conference VARCHAR(50),
    division VARCHAR(50),
    color VARCHAR(7),
    alternate_color VARCHAR(7),
    logo_url VARCHAR(2048),
    logo_dark_url VARCHAR(2048),
    venue_id VARCHAR(50),
    venue_name VARCHAR(255),
    venue_city VARCHAR(255),
    venue_state VARCHAR(50),
    venue_capacity INTEGER,
    is_active BOOLEAN DEFAULT TRUE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_teams_league ON np_sports_teams(league_id);
CREATE INDEX idx_sports_teams_name ON np_sports_teams(name);
CREATE INDEX idx_sports_teams_conference ON np_sports_teams(conference);
CREATE INDEX idx_sports_teams_division ON np_sports_teams(division);
```

### np_sports_events

```sql
CREATE TABLE np_sports_events (
    id VARCHAR(100) PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    provider_event_id VARCHAR(255),
    league_id VARCHAR(50) REFERENCES np_sports_leagues(id),
    home_team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    away_team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    season VARCHAR(10),
    season_type VARCHAR(20),
    week INTEGER,
    event_name VARCHAR(255),
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) NOT NULL,
    venue_id VARCHAR(50),
    venue_name VARCHAR(255),
    broadcast_network VARCHAR(100),
    broadcast_info JSONB,
    locked BOOLEAN DEFAULT FALSE,
    locked_at TIMESTAMP WITH TIME ZONE,
    override_start_time TIMESTAMP WITH TIME ZONE,
    override_reason TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_events_league ON np_sports_events(league_id);
CREATE INDEX idx_sports_events_home_team ON np_sports_events(home_team_id);
CREATE INDEX idx_sports_events_away_team ON np_sports_events(away_team_id);
CREATE INDEX idx_sports_events_start_time ON np_sports_events(start_time);
CREATE INDEX idx_sports_events_status ON np_sports_events(status);
CREATE INDEX idx_sports_events_locked ON np_sports_events(locked);
CREATE INDEX idx_sports_events_provider ON np_sports_events(provider, provider_event_id);
```

### np_sports_games

```sql
CREATE TABLE np_sports_games (
    id VARCHAR(100) PRIMARY KEY,
    event_id VARCHAR(100) REFERENCES np_sports_events(id),
    league_id VARCHAR(50) REFERENCES np_sports_leagues(id),
    home_team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    away_team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    home_score INTEGER DEFAULT 0,
    away_score INTEGER DEFAULT 0,
    home_score_by_period JSONB DEFAULT '[]',
    away_score_by_period JSONB DEFAULT '[]',
    status VARCHAR(20) NOT NULL,
    period VARCHAR(20),
    clock VARCHAR(10),
    is_overtime BOOLEAN DEFAULT FALSE,
    home_timeouts_remaining INTEGER,
    away_timeouts_remaining INTEGER,
    possession VARCHAR(50),
    down_distance VARCHAR(20),
    last_play TEXT,
    game_stats JSONB DEFAULT '{}',
    attendance INTEGER,
    game_duration INTEGER,
    weather JSONB,
    officials JSONB DEFAULT '[]',
    notes TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    finished_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_games_event ON np_sports_games(event_id);
CREATE INDEX idx_sports_games_league ON np_sports_games(league_id);
CREATE INDEX idx_sports_games_status ON np_sports_games(status);
CREATE INDEX idx_sports_games_started ON np_sports_games(started_at);
```

### np_sports_standings

```sql
CREATE TABLE np_sports_standings (
    id SERIAL PRIMARY KEY,
    league_id VARCHAR(50) REFERENCES np_sports_leagues(id),
    team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    season VARCHAR(10) NOT NULL,
    season_type VARCHAR(20),
    conference VARCHAR(50),
    division VARCHAR(50),
    wins INTEGER DEFAULT 0,
    losses INTEGER DEFAULT 0,
    ties INTEGER DEFAULT 0,
    overtime_losses INTEGER DEFAULT 0,
    win_percentage DECIMAL(5,4),
    games_back DECIMAL(4,1),
    points INTEGER,
    points_for INTEGER,
    points_against INTEGER,
    point_differential INTEGER,
    streak VARCHAR(10),
    home_record VARCHAR(10),
    away_record VARCHAR(10),
    conference_record VARCHAR(10),
    division_record VARCHAR(10),
    last_ten_record VARCHAR(10),
    rank INTEGER,
    conference_rank INTEGER,
    division_rank INTEGER,
    playoff_seed INTEGER,
    clinched VARCHAR(20),
    eliminated BOOLEAN DEFAULT FALSE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(league_id, team_id, season, season_type)
);

CREATE INDEX idx_sports_standings_league ON np_sports_standings(league_id);
CREATE INDEX idx_sports_standings_team ON np_sports_standings(team_id);
CREATE INDEX idx_sports_standings_season ON np_sports_standings(season);
CREATE INDEX idx_sports_standings_rank ON np_sports_standings(rank);
```

### np_sports_players

```sql
CREATE TABLE np_sports_players (
    id VARCHAR(100) PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    provider_player_id VARCHAR(255),
    team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    full_name VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    jersey_number VARCHAR(10),
    position VARCHAR(20),
    position_abbreviation VARCHAR(5),
    height VARCHAR(10),
    weight INTEGER,
    age INTEGER,
    birth_date DATE,
    birth_place VARCHAR(255),
    college VARCHAR(255),
    experience INTEGER,
    status VARCHAR(20),
    photo_url VARCHAR(2048),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_players_team ON np_sports_players(team_id);
CREATE INDEX idx_sports_players_name ON np_sports_players(full_name);
CREATE INDEX idx_sports_players_position ON np_sports_players(position);
CREATE INDEX idx_sports_players_provider ON np_sports_players(provider, provider_player_id);
```

### np_sports_favorites

```sql
CREATE TABLE np_sports_favorites (
    id SERIAL PRIMARY KEY,
    team_id VARCHAR(50) REFERENCES np_sports_teams(id),
    user_id VARCHAR(100),
    priority INTEGER DEFAULT 0,
    notifications_enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(team_id, user_id)
);

CREATE INDEX idx_sports_favorites_team ON np_sports_favorites(team_id);
CREATE INDEX idx_sports_favorites_user ON np_sports_favorites(user_id);
CREATE INDEX idx_sports_favorites_priority ON np_sports_favorites(priority DESC);
```

### np_sports_provider_syncs

```sql
CREATE TABLE np_sports_provider_syncs (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    sport VARCHAR(50),
    league VARCHAR(50),
    sync_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_seconds INTEGER,
    records_synced INTEGER DEFAULT 0,
    records_updated INTEGER DEFAULT 0,
    records_created INTEGER DEFAULT 0,
    errors JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_provider_syncs_provider ON np_sports_provider_syncs(provider);
CREATE INDEX idx_sports_provider_syncs_status ON np_sports_provider_syncs(status);
CREATE INDEX idx_sports_provider_syncs_started ON np_sports_provider_syncs(started_at DESC);
```

### np_sports_schedule_cache

```sql
CREATE TABLE np_sports_schedule_cache (
    id SERIAL PRIMARY KEY,
    cache_key VARCHAR(255) NOT NULL UNIQUE,
    league VARCHAR(50),
    team_id VARCHAR(50),
    date DATE,
    data JSONB NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_schedule_cache_key ON np_sports_schedule_cache(cache_key);
CREATE INDEX idx_sports_schedule_cache_expires ON np_sports_schedule_cache(expires_at);
CREATE INDEX idx_sports_schedule_cache_league_date ON np_sports_schedule_cache(league, date);
```

### np_sports_sync_state

```sql
CREATE TABLE np_sports_sync_state (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(50) NOT NULL,
    sport VARCHAR(50),
    league VARCHAR(50),
    last_sync_time TIMESTAMP WITH TIME ZONE,
    last_successful_sync TIMESTAMP WITH TIME ZONE,
    sync_cursor VARCHAR(255),
    status VARCHAR(20),
    error_message TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(provider, sport, league)
);

CREATE INDEX idx_sports_sync_state_provider ON np_sports_sync_state(provider);
CREATE INDEX idx_sports_sync_state_status ON np_sports_sync_state(status);
```

### np_sports_webhook_events

```sql
CREATE TABLE np_sports_webhook_events (
    id SERIAL PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    signature VARCHAR(255),
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_sports_webhook_events_type ON np_sports_webhook_events(event_type);
CREATE INDEX idx_sports_webhook_events_processed ON np_sports_webhook_events(processed);
CREATE INDEX idx_sports_webhook_events_received ON np_sports_webhook_events(received_at DESC);
```

---

## Analytics Views

### np_sports_upcoming_events

Upcoming events within the next 7 days.

```sql
CREATE VIEW np_sports_upcoming_events AS
SELECT
    e.id,
    e.league_id,
    l.name AS league_name,
    l.abbreviation AS league_abbr,
    e.home_team_id,
    ht.name AS home_team_name,
    ht.abbreviation AS home_team_abbr,
    e.away_team_id,
    at.name AS away_team_name,
    at.abbreviation AS away_team_abbr,
    e.start_time,
    e.status,
    e.venue_name,
    e.broadcast_network,
    e.locked
FROM np_sports_events e
JOIN np_sports_leagues l ON e.league_id = l.id
JOIN np_sports_teams ht ON e.home_team_id = ht.id
JOIN np_sports_teams at ON e.away_team_id = at.id
WHERE e.status = 'scheduled'
  AND e.start_time BETWEEN NOW() AND NOW() + INTERVAL '7 days'
ORDER BY e.start_time;
```

### np_sports_live_events

Currently live games.

```sql
CREATE VIEW np_sports_live_events AS
SELECT
    e.id AS event_id,
    e.league_id,
    l.name AS league_name,
    g.id AS game_id,
    ht.name AS home_team_name,
    at.name AS away_team_name,
    g.home_score,
    g.away_score,
    g.period,
    g.clock,
    e.venue_name,
    e.broadcast_network
FROM np_sports_events e
JOIN np_sports_leagues l ON e.league_id = l.id
JOIN np_sports_teams ht ON e.home_team_id = ht.id
JOIN np_sports_teams at ON e.away_team_id = at.id
JOIN np_sports_games g ON e.id = g.event_id
WHERE e.status = 'live'
  AND g.status = 'live'
ORDER BY e.start_time;
```

### np_sports_untriggered_recordings

Events needing recording triggers.

```sql
CREATE VIEW np_sports_untriggered_recordings AS
SELECT
    e.id AS event_id,
    e.league_id,
    l.abbreviation AS league,
    ht.name AS home_team,
    at.name AS away_team,
    e.start_time,
    e.venue_name,
    e.broadcast_network,
    e.locked
FROM np_sports_events e
JOIN np_sports_leagues l ON e.league_id = l.id
JOIN np_sports_teams ht ON e.home_team_id = ht.id
JOIN np_sports_teams at ON e.away_team_id = at.id
WHERE e.status = 'scheduled'
  AND e.locked = FALSE
  AND e.start_time > NOW()
  AND e.start_time <= NOW() + INTERVAL '2 hours'
ORDER BY e.start_time;
```

---

## Examples

### Example: Get Today's NFL Games

```bash
# Using CLI
nself plugin sports events today --league nfl
```

```sql
-- Using SQL
SELECT
    ht.name AS home_team,
    at.name AS away_team,
    e.start_time,
    e.venue_name,
    e.broadcast_network
FROM np_sports_events e
JOIN np_sports_teams ht ON e.home_team_id = ht.id
JOIN np_sports_teams at ON e.away_team_id = at.id
WHERE e.league_id = 'nfl'
  AND DATE(e.start_time) = CURRENT_DATE
ORDER BY e.start_time;
```

### Example: Get Live Scores

```bash
# Using CLI
nself plugin sports scores --live
```

```sql
-- Using SQL
SELECT * FROM np_sports_live_events;
```

### Example: Auto-trigger Recordings

```bash
# Configure in .env
SPORTS_AUTO_TRIGGER_RECORDINGS=true
SPORTS_RECORDING_PLUGIN_URL=http://localhost:3220
SPORTS_RECORDING_LEAD_TIME_MINUTES=10
SPORTS_RECORDING_TRAIL_TIME_MINUTES=30
```

---

## Troubleshooting

### No Events Synced

**Issue:** Sync completes but no events in database.

**Solution:**
1. Check provider configuration
2. Verify API credentials
3. Check enabled sports and leagues
4. Review sync logs with DEBUG mode

### Cache Not Working

**Issue:** High API usage despite caching.

**Solution:**
1. Verify SPORTS_CACHE_ENABLED=true
2. Check cache stats
3. Adjust TTL values
4. Clear and rebuild cache

### Recording Triggers Not Firing

**Issue:** Events not triggering recordings.

**Solution:**
1. Verify SPORTS_AUTO_TRIGGER_RECORDINGS=true
2. Check SPORTS_RECORDING_PLUGIN_URL
3. Verify recording plugin is reachable
4. Check webhook event logs

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)
- **Plugin Documentation:** [github.com/acamarata/nself-plugins/wiki/Sports](https://github.com/acamarata/nself-plugins/wiki/Sports)
- **ESPN API:** [site.api.espn.com](https://site.api.espn.com)
- **SportsData.io:** [sportsdata.io/developers](https://sportsdata.io/developers)

---

*Last Updated: February 11, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
