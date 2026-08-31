# Sports Plugin - Full Implementation Guide

## Overview

The Sports plugin synchronizes sports schedules, scores, standings, and team data from **ESPN API** and **SportsData.io**. Designed for sports apps like nself-tv that need automated game recording based on live schedules.

## Current Status

**Infrastructure Status**: ✅ Complete (database, API, sync tracking, webhooks)
**Provider Integration Status**: ⚠️ Placeholder (requires API implementation)

## What's Already Built

- ✅ Complete database schema for leagues, teams, events, standings, favorites
- ✅ Full REST API with nTV v1 endpoints
- ✅ Sync tracking with success/failure logs
- ✅ Event locking system (prevent overwrite near game time)
- ✅ Operator override system
- ✅ Recording trigger integration
- ✅ Webhook ingestion endpoints
- ✅ User favorites with auto-record support

## What Needs Implementation

**Provider API Integration** - The actual sports data sync in:
- `syncLeagues()` - Fetch leagues from provider
- `syncTeams()` - Fetch team rosters
- `syncSchedule()` - Fetch game schedule
- `syncLiveScores()` - Fetch live game updates
- `syncStandings()` - Fetch team standings

---

## Required Packages

Base dependencies **already installed**:

```json
{
  "@nself/plugin-utils": "file:../../../shared",
  "fastify": "^4.24.0",
  "@fastify/cors": "^8.4.0",
  "pg": "^8.11.3"
}
```

### Additional Packages for Provider Integration

```bash
# ESPN API client (unofficial)
pnpm add espn-fantasy-football-api

# SportsData.io SDK
pnpm add sportsdata-api-node

# Generic HTTP client (works for both)
pnpm add axios

# Date handling
pnpm add date-fns
```

---

## Complete Implementation Code

### 1. Provider Integration Module

Create `ts/src/providers.ts`:

```typescript
/**
 * Sports Data Provider Integration
 * Supports ESPN API and SportsData.io
 */

import axios from 'axios';
import { format, parseISO } from 'date-fns';

export interface League {
  external_id: string;
  name: string;
  short_name: string;
  sport: string;
  season: string;
  logo_url?: string;
}

export interface Team {
  external_id: string;
  league_id: string;
  name: string;
  short_name: string;
  abbreviation: string;
  city?: string;
  logo_url?: string;
}

export interface GameEvent {
  external_id: string;
  league_id: string;
  home_team_id: string;
  away_team_id: string;
  scheduled_at: Date;
  status: 'scheduled' | 'live' | 'final' | 'postponed' | 'cancelled';
  home_score?: number;
  away_score?: number;
  broadcast_channel?: string;
  venue?: string;
  season?: string;
  week?: number;
}

export interface Standing {
  team_id: string;
  wins: number;
  losses: number;
  ties?: number;
  points?: number;
  games_played: number;
  win_percentage: number;
  rank: number;
}

/**
 * ESPN API Provider
 */
export class ESPNProvider {
  private baseUrl = 'https://site.api.espn.com/apis/site/v2/sports';

  /**
   * Fetch leagues (NFL, NBA, MLB, NHL, etc.)
   */
  async getLeagues(sport: string): Promise<League[]> {
    try {
      // ESPN has different endpoints per sport
      const response = await axios.get(`${this.baseUrl}/${sport}/scoreboard`);

      const leagues = response.data.leagues ?? [];

      return leagues.map((league: Record<string, unknown>) => ({
        external_id: league.id as string,
        name: league.name as string,
        short_name: league.abbreviation as string,
        sport,
        season: league.season?.year?.toString() ?? new Date().getFullYear().toString(),
        logo_url: league.logos?.[0]?.href as string | undefined,
      }));
    } catch (error) {
      throw new Error(`ESPN getLeagues failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Fetch teams for a league
   */
  async getTeams(sport: string, leagueId: string): Promise<Team[]> {
    try {
      const response = await axios.get(`${this.baseUrl}/${sport}/teams`);

      const teams = response.data.sports?.[0]?.leagues?.[0]?.teams ?? [];

      return teams.map((item: Record<string, unknown>) => {
        const team = item.team as Record<string, unknown>;
        return {
          external_id: team.id as string,
          league_id: leagueId,
          name: team.displayName as string,
          short_name: team.shortDisplayName as string,
          abbreviation: team.abbreviation as string,
          city: team.location as string,
          logo_url: team.logos?.[0]?.href as string | undefined,
        };
      });
    } catch (error) {
      throw new Error(`ESPN getTeams failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Fetch schedule (games)
   */
  async getSchedule(sport: string, leagueId: string, dateFrom?: Date, dateTo?: Date): Promise<GameEvent[]> {
    try {
      const params: Record<string, string> = {};
      if (dateFrom) params.dates = format(dateFrom, 'yyyyMMdd');

      const response = await axios.get(`${this.baseUrl}/${sport}/scoreboard`, { params });

      const events = response.data.events ?? [];

      return events.map((event: Record<string, unknown>) => {
        const competitions = event.competitions as Array<Record<string, unknown>>;
        const competition = competitions[0];
        const competitors = competition?.competitors as Array<Record<string, unknown>>;

        const homeTeam = competitors?.find(c => c.homeAway === 'home') as Record<string, unknown>;
        const awayTeam = competitors?.find(c => c.homeAway === 'away') as Record<string, unknown>;

        return {
          external_id: event.id as string,
          league_id: leagueId,
          home_team_id: homeTeam?.team?.id as string,
          away_team_id: awayTeam?.team?.id as string,
          scheduled_at: parseISO(event.date as string),
          status: this.mapESPNStatus(event.status?.type?.name as string),
          home_score: parseInt(homeTeam?.score as string) || undefined,
          away_score: parseInt(awayTeam?.score as string) || undefined,
          broadcast_channel: competition?.broadcasts?.[0]?.names?.[0] as string | undefined,
          venue: competition?.venue?.fullName as string | undefined,
          season: event.season?.year?.toString(),
          week: event.week?.number as number | undefined,
        };
      });
    } catch (error) {
      throw new Error(`ESPN getSchedule failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Fetch live scores
   */
  async getLiveScores(sport: string): Promise<GameEvent[]> {
    return this.getSchedule(sport, '', new Date());
  }

  /**
   * Fetch standings
   */
  async getStandings(sport: string, leagueId: string, season?: string): Promise<Standing[]> {
    try {
      const response = await axios.get(`${this.baseUrl}/${sport}/standings`);

      const standings = response.data.children?.[0]?.standings?.entries ?? [];

      return standings.map((entry: Record<string, unknown>, index: number) => {
        const team = entry.team as Record<string, unknown>;
        const stats = entry.stats as Array<Record<string, unknown>>;

        const getStat = (name: string): number => {
          return parseFloat(stats.find(s => s.name === name)?.displayValue as string) || 0;
        };

        return {
          team_id: team.id as string,
          wins: getStat('wins'),
          losses: getStat('losses'),
          ties: getStat('ties'),
          points: getStat('points'),
          games_played: getStat('gamesPlayed'),
          win_percentage: getStat('winPercent'),
          rank: index + 1,
        };
      });
    } catch (error) {
      throw new Error(`ESPN getStandings failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  private mapESPNStatus(status: string): GameEvent['status'] {
    switch (status?.toLowerCase()) {
      case 'status_scheduled': return 'scheduled';
      case 'status_in_progress': return 'live';
      case 'status_final': return 'final';
      case 'status_postponed': return 'postponed';
      case 'status_canceled': return 'cancelled';
      default: return 'scheduled';
    }
  }
}

/**
 * SportsData.io Provider
 */
export class SportsDataProvider {
  private apiKey: string;
  private baseUrl = 'https://api.sportsdata.io/v3';

  constructor(apiKey: string) {
    this.apiKey = apiKey;
  }

  /**
   * Fetch NFL teams
   */
  async getNFLTeams(): Promise<Team[]> {
    try {
      const response = await axios.get(`${this.baseUrl}/nfl/scores/json/Teams`, {
        params: { key: this.apiKey },
      });

      return response.data.map((team: Record<string, unknown>) => ({
        external_id: team.TeamID?.toString() ?? '',
        league_id: 'nfl',
        name: team.FullName as string,
        short_name: team.Name as string,
        abbreviation: team.Key as string,
        city: team.City as string,
        logo_url: team.WikipediaLogoUrl as string | undefined,
      }));
    } catch (error) {
      throw new Error(`SportsData getNFLTeams failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Fetch NFL schedule
   */
  async getNFLSchedule(season: string): Promise<GameEvent[]> {
    try {
      const response = await axios.get(`${this.baseUrl}/nfl/scores/json/Schedules/${season}`, {
        params: { key: this.apiKey },
      });

      return response.data.map((game: Record<string, unknown>) => ({
        external_id: game.GameID?.toString() ?? '',
        league_id: 'nfl',
        home_team_id: game.HomeTeamID?.toString() ?? '',
        away_team_id: game.AwayTeamID?.toString() ?? '',
        scheduled_at: parseISO(game.DateTime as string),
        status: this.mapSportsDataStatus(game.Status as string),
        home_score: game.HomeScore as number | undefined,
        away_score: game.AwayScore as number | undefined,
        broadcast_channel: game.Channel as string | undefined,
        venue: game.StadiumDetails?.Name as string | undefined,
        season: game.Season?.toString(),
        week: game.Week as number,
      }));
    } catch (error) {
      throw new Error(`SportsData getNFLSchedule failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  /**
   * Fetch NFL standings
   */
  async getNFLStandings(season: string): Promise<Standing[]> {
    try {
      const response = await axios.get(`${this.baseUrl}/nfl/scores/json/Standings/${season}`, {
        params: { key: this.apiKey },
      });

      return response.data.map((team: Record<string, unknown>, index: number) => ({
        team_id: team.TeamID?.toString() ?? '',
        wins: team.Wins as number,
        losses: team.Losses as number,
        ties: team.Ties as number,
        points: team.Points as number,
        games_played: (team.Wins as number) + (team.Losses as number) + (team.Ties as number),
        win_percentage: team.Percentage as number,
        rank: index + 1,
      }));
    } catch (error) {
      throw new Error(`SportsData getNFLStandings failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  private mapSportsDataStatus(status: string): GameEvent['status'] {
    switch (status?.toLowerCase()) {
      case 'scheduled': return 'scheduled';
      case 'inprogress': return 'live';
      case 'final': return 'final';
      case 'f/ot': return 'final';
      case 'postponed': return 'postponed';
      case 'canceled': return 'cancelled';
      default: return 'scheduled';
    }
  }
}

/**
 * Provider factory
 */
export function createSportsProvider(
  provider: string,
  config: Record<string, string>
): ESPNProvider | SportsDataProvider {
  switch (provider.toLowerCase()) {
    case 'espn':
      return new ESPNProvider();

    case 'sportsdata':
      if (!config.SPORTS_SPORTSDATA_API_KEY) {
        throw new Error('SPORTS_SPORTSDATA_API_KEY is required for SportsData.io provider');
      }
      return new SportsDataProvider(config.SPORTS_SPORTSDATA_API_KEY);

    default:
      throw new Error(`Unsupported sports provider: ${provider}`);
  }
}
```

### 2. Update Server to Use Providers

Modify `ts/src/server.ts` sync endpoint:

```typescript
import { createSportsProvider, type ESPNProvider, type SportsDataProvider } from './providers.js';

// In createServer() after config:
const sportsProviders = fullConfig.providers.map(provider =>
  createSportsProvider(provider, process.env as Record<string, string>)
);

// Update /sync endpoint (around line 337):
app.post('/sync', async (request, reply) => {
  try {
    const body = (request.body as SyncRequest) ?? {};
    const startTime = Date.now();

    const providers = body.providers ?? fullConfig.providers;
    const syncErrors: string[] = [];
    let totalSynced = 0;

    for (const providerName of providers) {
      const syncRecord = await scopedDb(request).createSyncRecord(providerName, 'full');

      try {
        logger.info(`Syncing from provider: ${providerName}`);

        const provider = createSportsProvider(providerName, process.env as Record<string, string>);

        // Sync based on enabled sports
        for (const sport of fullConfig.enabledSports) {
          if (provider instanceof ESPNProvider) {
            // Sync leagues
            const leagues = await provider.getLeagues(sport);
            for (const league of leagues) {
              await scopedDb(request).upsertLeague(league);
            }

            // Sync teams
            for (const league of leagues) {
              const teams = await provider.getTeams(sport, league.external_id);
              for (const team of teams) {
                await scopedDb(request).upsertTeam(team);
              }
            }

            // Sync schedule (next 30 days)
            const today = new Date();
            const future = new Date();
            future.setDate(future.getDate() + 30);

            for (const league of leagues) {
              const games = await provider.getSchedule(sport, league.external_id, today, future);
              for (const game of games) {
                await scopedDb(request).upsertEvent(game);
              }
            }

            // Sync standings
            for (const league of leagues) {
              const standings = await provider.getStandings(sport, league.external_id);
              for (const standing of standings) {
                await scopedDb(request).upsertStanding(league.external_id, standing);
              }
            }

            totalSynced += leagues.length;
          } else if (provider instanceof SportsDataProvider) {
            // SportsData.io specific sync
            if (sport === 'nfl') {
              const teams = await provider.getNFLTeams();
              for (const team of teams) {
                await scopedDb(request).upsertTeam(team);
              }

              const currentSeason = new Date().getFullYear().toString();
              const schedule = await provider.getNFLSchedule(currentSeason);
              for (const game of schedule) {
                await scopedDb(request).upsertEvent(game);
              }

              const standings = await provider.getNFLStandings(currentSeason);
              for (const standing of standings) {
                await scopedDb(request).upsertStanding('nfl', standing);
              }

              totalSynced++;
            }
          }
        }

        await scopedDb(request).updateSyncRecord(syncRecord.id, 'completed', totalSynced);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        syncErrors.push(`${providerName}: ${message}`);
        await scopedDb(request).updateSyncRecord(syncRecord.id, 'failed', 0, [message]);
      }
    }

    const duration = Date.now() - startTime;

    return {
      success: syncErrors.length === 0,
      stats: { total_synced: totalSynced },
      errors: syncErrors,
      duration_ms: duration,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Sync failed', { error: message });
    return reply.status(500).send({ error: message });
  }
});
```

---

## Configuration Requirements

### Environment Variables

**ESPN (Free, no API key needed)**:
```bash
SPORTS_PROVIDERS=espn
SPORTS_ENABLED_SPORTS=football,basketball,baseball
SPORTS_ENABLED_LEAGUES=nfl,nba,mlb
SPORTS_SYNC_INTERVAL=3600000  # 1 hour
SPORTS_LOCK_WINDOW_MINUTES=30
```

**SportsData.io (Paid)**:
```bash
SPORTS_PROVIDERS=sportsdata
SPORTS_SPORTSDATA_API_KEY=your_sportsdata_api_key
SPORTS_ENABLED_SPORTS=football
SPORTS_ENABLED_LEAGUES=nfl
```

**Multi-Provider**:
```bash
SPORTS_PROVIDERS=espn,sportsdata
SPORTS_SPORTSDATA_API_KEY=xxx
SPORTS_ENABLED_SPORTS=football,basketball
```

### Get API Credentials

**ESPN API**:
- **Free** public API
- No authentication required
- Rate limited (not documented, but ~100 req/min works)

**SportsData.io**:
1. Sign up at [sportsdata.io](https://sportsdata.io/)
2. Choose plan (starts at $20/month)
3. Get API key from dashboard
4. Select sports: NFL, NBA, MLB, NHL, etc.

---

## Testing Instructions

### 1. Install Dependencies

```bash
cd plugins/sports/ts
pnpm install
pnpm add axios date-fns
```

### 2. Build

```bash
pnpm build
```

### 3. Configure

Create `.env`:

```bash
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself_db
SPORTS_API_KEY=test-key
SPORTS_PORT=3035

SPORTS_PROVIDERS=espn
SPORTS_ENABLED_SPORTS=football,basketball
SPORTS_ENABLED_LEAGUES=nfl,nba
SPORTS_SYNC_INTERVAL=3600000
```

### 4. Start Server

```bash
pnpm start
```

### 5. Test API

**Sync Data**:
```bash
curl -X POST http://localhost:3035/sync \
  -H "X-API-Key: test-key"
```

**List Leagues**:
```bash
curl http://localhost:3035/api/leagues \
  -H "X-API-Key: test-key"
```

**List Teams**:
```bash
curl "http://localhost:3035/api/teams?league_id=nfl" \
  -H "X-API-Key: test-key"
```

**Get Schedule**:
```bash
curl "http://localhost:3035/api/events?league_id=nfl&status=scheduled&limit=10" \
  -H "X-API-Key: test-key"
```

**Get Live Games**:
```bash
curl http://localhost:3035/api/events/live \
  -H "X-API-Key: test-key"
```

**Get Standings**:
```bash
curl http://localhost:3035/api/v1/standings/nfl \
  -H "X-API-Key: test-key"
```

**Add Favorite Team**:
```bash
curl -X POST http://localhost:3035/api/v1/favorites \
  -H "Content-Type: application/json" \
  -H "X-API-Key: test-key" \
  -H "X-User-Id: user_123" \
  -d '{
    "team_id": "team_uuid_from_above",
    "notify_live": true,
    "auto_record": true
  }'
```

---

## Activation Checklist

- [ ] Install dependencies: `pnpm add axios date-fns`
- [ ] Create `providers.ts`
- [ ] Update `server.ts` sync endpoint
- [ ] Add database methods: `upsertLeague()`, `upsertTeam()`, `upsertEvent()`, `upsertStanding()`
- [ ] Configure providers in `.env`
- [ ] Build: `pnpm build`
- [ ] Start: `pnpm start`
- [ ] Test sync endpoint
- [ ] Set up cron job for hourly sync

---

## nTV Integration

This plugin is designed to work with **nself-tv**:

1. **Auto-Record Favorite Teams**: When users favorite a team with `auto_record: true`, the system automatically triggers recording when their games go live
2. **Event Locking**: Games lock 30 minutes before start to prevent accidental changes
3. **Recording Triggers**: Call `/api/events/:id/trigger-recording` to start recording
4. **Live Updates**: Poll `/api/events/live` to detect when games start

---

## Cost Considerations

**ESPN API**:
- **Free**
- Public endpoints, no API key
- Rate limits not documented (use responsibly)

**SportsData.io Pricing** (2024):
- **Free Tier**: 1000 calls/month
- **Starter**: $20/month (10K calls)
- **Pro**: $100/month (100K calls)
- Per-sport pricing available

**Recommendation**: Start with ESPN for free testing, upgrade to SportsData.io for production reliability.

---

## Support

- **ESPN API**: https://site.api.espn.com/apis/site/v2/sports (unofficial)
- **SportsData.io**: https://sportsdata.io/developers/api-documentation
