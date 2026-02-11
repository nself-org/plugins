# Sports Data

Sports data aggregation with live scores, schedules, standings, and team/player information

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

## Overview

The Sports Data plugin provides comprehensive sports data aggregation from multiple providers including ESPN, The Sports DB, and API-Sports. It tracks live scores, schedules, standings, team/player statistics, and provides real-time updates for ongoing games.

### Key Features

- **Multi-League Support** - NFL, NBA, MLB, NHL, MLS, EPL, and international leagues
- **Live Scores** - Real-time score updates during live games
- **Schedules** - Complete season schedules with game times and broadcast info
- **Standings** - Current league standings with win-loss records and rankings
- **Team Information** - Team rosters, statistics, and historical data
- **Player Statistics** - Individual player stats and performance metrics
- **Game Notifications** - Alerts for game starts, score changes, and final results
- **Data Provider Flexibility** - Support for multiple sports data APIs
- **Favorites Tracking** - Follow specific teams with personalized updates

## Quick Start

```bash
# Install the plugin
nself plugin install sports-data

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export SPORTS_PROVIDER="espn"
export SPORTS_ESPN_API_KEY="your-api-key"
export SPORTS_PLUGIN_PORT=3030
export SPORTS_LEAGUE_IDS="nfl,nba,mlb"

# Initialize the database schema
nself plugin sports-data init

# Start the server
nself plugin sports-data server
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `""` | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `SPORTS_PROVIDER` | Yes | `espn` | Data provider (espn, sportsdata, api-football, thesportsdb) |
| `SPORTS_ESPN_API_KEY` | No | `""` | ESPN API key |
| `SPORTS_SPORTSDATA_API_KEY` | No | `""` | SportsData.io API key |
| `SPORTS_API_FOOTBALL_KEY` | No | `""` | API-Football key |
| `SPORTS_THESPORTSDB_API_KEY` | No | `""` | TheSportsDB API key |
| `SPORTS_PLUGIN_PORT` | No | `3030` | HTTP server port |
| `SPORTS_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `SPORTS_LEAGUE_IDS` | No | `nfl,nba,mlb,nhl,mls,epl` | Comma-separated league IDs to track |
| `SPORTS_LIVE_GAME_POLL_SECONDS` | No | `30` | Polling interval for live games (seconds) |
| `SPORTS_SCHEDULE_SYNC_CRON` | No | `0 6 * * *` | Schedule sync cron (daily at 6am) |
| `SPORTS_STANDINGS_SYNC_CRON` | No | `0 */6 * * *` | Standings sync cron (every 6 hours) |
| `SPORTS_ROSTER_SYNC_CRON` | No | `0 0 * * 1` | Roster sync cron (weekly on Monday) |
| `SPORTS_NOTIFY_GAME_START_MINUTES_BEFORE` | No | `15` | Minutes before game start to send notification |
| `SPORTS_NOTIFY_SCORE_CHANGES` | No | `true` | Notify on score changes |
| `SPORTS_NOTIFY_GAME_END` | No | `true` | Notify when games end |
| `SPORTS_APP_IDS` | No | `primary` | Comma-separated application IDs |

### Example .env

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
SPORTS_PROVIDER=espn
SPORTS_ESPN_API_KEY=your-espn-api-key

# Server Configuration
SPORTS_PLUGIN_PORT=3030
SPORTS_PLUGIN_HOST=0.0.0.0

# Leagues to Track
SPORTS_LEAGUE_IDS=nfl,nba,mlb,nhl,mls

# Live Game Polling
SPORTS_LIVE_GAME_POLL_SECONDS=30

# Sync Schedules
SPORTS_SCHEDULE_SYNC_CRON=0 6 * * *      # Daily at 6am
SPORTS_STANDINGS_SYNC_CRON=0 */6 * * *  # Every 6 hours
SPORTS_ROSTER_SYNC_CRON=0 0 * * 1       # Weekly on Monday

# Notifications
SPORTS_NOTIFY_GAME_START_MINUTES_BEFORE=15
SPORTS_NOTIFY_SCORE_CHANGES=true
SPORTS_NOTIFY_GAME_END=true

# Multi-App Support
SPORTS_APP_IDS=primary,app1,app2
```

## CLI Commands

### `init`
Initialize the sports data database schema.

```bash
nself plugin sports-data init
```

### `server`
Start the sports data HTTP server.

```bash
nself plugin sports-data server
```

### `leagues`
List available leagues.

```bash
nself plugin sports-data leagues

# Example output:
# {
#   "leagues": [
#     { "id": "nfl", "name": "National Football League", "sport": "football" },
#     { "id": "nba", "name": "National Basketball Association", "sport": "basketball" },
#     { "id": "mlb", "name": "Major League Baseball", "sport": "baseball" }
#   ]
# }
```

### `teams`
List teams for a league.

```bash
# List NFL teams
nself plugin sports-data teams --league nfl

# Search for team
nself plugin sports-data teams --league nba --search "Lakers"
```

### `games`
View games and schedule.

```bash
# Today's games
nself plugin sports-data games --today

# Games for specific date
nself plugin sports-data games --date 2025-02-15

# Games for specific team
nself plugin sports-data games --team team-id

# Games for league
nself plugin sports-data games --league nfl
```

### `scores`
View live scores.

```bash
# All live games
nself plugin sports-data scores

# Live games for specific league
nself plugin sports-data scores --league nfl
```

### `standings`
View league standings.

```bash
# NFL standings
nself plugin sports-data standings --league nfl

# NBA standings by conference
nself plugin sports-data standings --league nba --conference eastern
```

### `sync`
Trigger data sync.

```bash
# Sync all data
nself plugin sports-data sync

# Sync specific resources
nself plugin sports-data sync --resources schedules,standings

# Sync specific league
nself plugin sports-data sync --league nfl
```

### `sync-status`
Check sync status.

```bash
nself plugin sports-data sync-status

# Example output:
# {
#   "lastScheduleSync": "2025-02-11T06:00:00Z",
#   "lastStandingsSync": "2025-02-11T12:00:00Z",
#   "lastRosterSync": "2025-02-10T00:00:00Z",
#   "liveGamesActive": 3
# }
```

### `stats`
Show sports data statistics.

```bash
nself plugin sports-data stats

# Example output:
# {
#   "totalLeagues": 6,
#   "totalTeams": 156,
#   "totalGames": 8420,
#   "totalPlayers": 4500,
#   "liveGamesNow": 3,
#   "todayGames": 15
# }
```

## REST API

### Games

#### `GET /api/games`
List games with filtering.

**Query Parameters:**
- `leagueId` (optional): Filter by league
- `teamId` (optional): Filter by team
- `date` (optional): Filter by date (YYYY-MM-DD)
- `status` (optional): Filter by status (scheduled, live, final)
- `limit` (optional, default: 50)

**Response:**
```json
{
  "data": [
    {
      "id": "game-uuid",
      "leagueId": "nfl",
      "homeTeamId": "team-1",
      "awayTeamId": "team-2",
      "homeTeamName": "Kansas City Chiefs",
      "awayTeamName": "Philadelphia Eagles",
      "homeScore": 28,
      "awayScore": 21,
      "status": "live",
      "quarter": "Q3",
      "timeRemaining": "8:42",
      "scheduledAt": "2025-02-11T18:30:00Z",
      "venue": "Arrowhead Stadium"
    }
  ]
}
```

#### `GET /api/games/:id`
Get game details.

**Response:**
```json
{
  "id": "game-uuid",
  "leagueId": "nfl",
  "homeTeam": {...},
  "awayTeam": {...},
  "homeScore": 28,
  "awayScore": 21,
  "status": "live",
  "quarter": "Q3",
  "timeRemaining": "8:42",
  "playByPlay": [...],
  "stats": {...}
}
```

#### `GET /api/games/live`
Get all live games.

**Response:**
```json
{
  "data": [
    {
      "id": "game-uuid",
      "leagueId": "nfl",
      "homeTeam": "Chiefs",
      "awayTeam": "Eagles",
      "homeScore": 28,
      "awayScore": 21,
      "quarter": "Q3",
      "timeRemaining": "8:42"
    }
  ]
}
```

### Teams

#### `GET /api/teams`
List teams.

**Query Parameters:**
- `leagueId` (optional): Filter by league
- `search` (optional): Search team name

**Response:**
```json
{
  "data": [
    {
      "id": "team-uuid",
      "leagueId": "nfl",
      "name": "Kansas City Chiefs",
      "abbreviation": "KC",
      "city": "Kansas City",
      "conference": "AFC",
      "division": "West",
      "logoUrl": "https://...",
      "founded": 1960
    }
  ]
}
```

#### `GET /api/teams/:id`
Get team details.

**Response:**
```json
{
  "id": "team-uuid",
  "name": "Kansas City Chiefs",
  "leagueId": "nfl",
  "record": "14-3",
  "wins": 14,
  "losses": 3,
  "ranking": 1,
  "roster": [...],
  "recentGames": [...],
  "upcomingGames": [...]
}
```

### Standings

#### `GET /api/standings/:leagueId`
Get league standings.

**Query Parameters:**
- `conference` (optional): Filter by conference
- `division` (optional): Filter by division

**Response:**
```json
{
  "league": "nfl",
  "season": "2024-2025",
  "data": [
    {
      "rank": 1,
      "teamId": "team-uuid",
      "teamName": "Kansas City Chiefs",
      "wins": 14,
      "losses": 3,
      "ties": 0,
      "winPercentage": 0.824,
      "pointsFor": 456,
      "pointsAgainst": 298,
      "streak": "W5"
    }
  ]
}
```

### Players

#### `GET /api/players/:id`
Get player details.

**Response:**
```json
{
  "id": "player-uuid",
  "firstName": "Patrick",
  "lastName": "Mahomes",
  "teamId": "team-uuid",
  "position": "QB",
  "jerseyNumber": 15,
  "stats": {...}
}
```

## Webhook Events

| Event Type | Description | Payload |
|------------|-------------|---------|
| `sports.game.starting` | Game about to start | `{ gameId, homeTeam, awayTeam, scheduledAt }` |
| `sports.game.started` | Game has started | `{ gameId, status }` |
| `sports.score.changed` | Score updated | `{ gameId, homeScore, awayScore }` |
| `sports.game.final` | Game ended | `{ gameId, homeScore, awayScore, winner }` |
| `sports.standings.updated` | Standings updated | `{ leagueId, season }` |

## Database Schema

### sports_leagues

```sql
CREATE TABLE IF NOT EXISTS sports_leagues (
  id VARCHAR(50) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  sport VARCHAR(50) NOT NULL,
  country VARCHAR(100),
  logo_url TEXT,
  active BOOLEAN DEFAULT true,
  synced_at TIMESTAMPTZ DEFAULT NOW()
);
```

### sports_teams

```sql
CREATE TABLE IF NOT EXISTS sports_teams (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  league_id VARCHAR(50) NOT NULL,
  name VARCHAR(255) NOT NULL,
  abbreviation VARCHAR(10),
  city VARCHAR(100),
  conference VARCHAR(50),
  division VARCHAR(50),
  logo_url TEXT,
  venue VARCHAR(255),
  founded INTEGER,
  metadata JSONB DEFAULT '{}',
  synced_at TIMESTAMPTZ DEFAULT NOW()
);
```

### sports_games

```sql
CREATE TABLE IF NOT EXISTS sports_games (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  league_id VARCHAR(50) NOT NULL,
  season VARCHAR(20),
  home_team_id VARCHAR(255) NOT NULL,
  away_team_id VARCHAR(255) NOT NULL,
  home_score INTEGER DEFAULT 0,
  away_score INTEGER DEFAULT 0,
  status VARCHAR(20) NOT NULL,
  period VARCHAR(20),
  time_remaining VARCHAR(20),
  scheduled_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  venue VARCHAR(255),
  broadcast_info JSONB,
  metadata JSONB DEFAULT '{}',
  synced_at TIMESTAMPTZ DEFAULT NOW()
);
```

### sports_standings

```sql
CREATE TABLE IF NOT EXISTS sports_standings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  league_id VARCHAR(50) NOT NULL,
  season VARCHAR(20) NOT NULL,
  team_id VARCHAR(255) NOT NULL,
  rank INTEGER,
  wins INTEGER DEFAULT 0,
  losses INTEGER DEFAULT 0,
  ties INTEGER DEFAULT 0,
  win_percentage DOUBLE PRECISION,
  points_for INTEGER DEFAULT 0,
  points_against INTEGER DEFAULT 0,
  streak VARCHAR(10),
  conference VARCHAR(50),
  division VARCHAR(50),
  synced_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(source_account_id, league_id, season, team_id)
);
```

## Examples

### Example 1: Track Live Game Scores

```javascript
// Poll for live games every 30 seconds
setInterval(async () => {
  const response = await fetch('http://localhost:3030/api/games/live');
  const { data: liveGames } = await response.json();

  liveGames.forEach(game => {
    console.log(`${game.awayTeam} ${game.awayScore} @ ${game.homeTeam} ${game.homeScore} - ${game.quarter} ${game.timeRemaining}`);

    // Check if favorite team is playing
    if (game.homeTeamId === 'my-favorite-team') {
      sendNotification(`Your team is playing! Score: ${game.homeScore}-${game.awayScore}`);
    }
  });
}, 30000);
```

### Example 2: Standings Dashboard

```sql
-- Get NFL standings ordered by division
SELECT
  s.rank,
  t.name as team,
  s.wins,
  s.losses,
  s.ties,
  ROUND(s.win_percentage, 3) as pct,
  s.points_for as pf,
  s.points_against as pa,
  s.streak
FROM sports_standings s
JOIN sports_teams t ON t.id = s.team_id
WHERE s.source_account_id = 'primary'
  AND s.league_id = 'nfl'
  AND s.season = '2024-2025'
ORDER BY t.conference, t.division, s.rank;
```

### Example 3: Upcoming Games Widget

```sql
-- Get today's games
SELECT
  g.scheduled_at,
  ht.name as home_team,
  at.name as away_team,
  g.venue,
  g.broadcast_info->>'network' as network
FROM sports_games g
JOIN sports_teams ht ON ht.id = g.home_team_id
JOIN sports_teams at ON at.id = g.away_team_id
WHERE g.source_account_id = 'primary'
  AND DATE(g.scheduled_at) = CURRENT_DATE
  AND g.status = 'scheduled'
ORDER BY g.scheduled_at;
```

## Troubleshooting

### Common Issues

#### 1. Live Scores Not Updating

**Symptom:** Live game scores remain static.

**Solutions:**
- Verify polling interval is configured: `echo $SPORTS_LIVE_GAME_POLL_SECONDS`
- Check API rate limits haven't been exceeded
- Verify API key is valid and active
- Check server logs for polling errors
- Ensure games are actually in progress

#### 2. Missing Team/League Data

**Symptom:** Teams or leagues not appearing.

**Solutions:**
- Run manual sync: `nself plugin sports-data sync`
- Verify league is in tracked list: `echo $SPORTS_LEAGUE_IDS`
- Check provider supports requested league
- Review provider API documentation for league IDs

#### 3. Sync Failures

**Symptom:** Scheduled syncs not running.

**Solutions:**
- Verify cron expressions are valid
- Check system time and timezone settings
- Review cron job logs
- Test manual sync to isolate issue
- Ensure API credentials are valid

---

**Need more help?** Check the [main documentation](https://github.com/acamarata/nself-plugins) or [open an issue](https://github.com/acamarata/nself-plugins/issues).
