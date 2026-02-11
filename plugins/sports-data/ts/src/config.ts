/**
 * Sports Data Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Provider settings
  provider: string;
  espnApiKey: string;
  sportsDataApiKey: string;
  apiFootballKey: string;
  theSportsDbApiKey: string;

  // Leagues to track
  leagueIds: string[];

  // Sync intervals
  liveGamePollSeconds: number;
  scheduleSyncCron: string;
  standingsSyncCron: string;
  rosterSyncCron: string;

  // Notification triggers
  notifyGameStartMinutesBefore: number;
  notifyScoreChanges: boolean;
  notifyGameEnd: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('SPORTS');

  const config: Config = {
    // Server
    port: parseInt(process.env.SPORTS_PLUGIN_PORT ?? process.env.PORT ?? '3030', 10),
    host: process.env.SPORTS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Provider settings
    provider: process.env.SPORTS_PROVIDER ?? 'espn',
    espnApiKey: process.env.SPORTS_ESPN_API_KEY ?? '',
    sportsDataApiKey: process.env.SPORTS_SPORTSDATA_API_KEY ?? '',
    apiFootballKey: process.env.SPORTS_API_FOOTBALL_KEY ?? '',
    theSportsDbApiKey: process.env.SPORTS_THESPORTSDB_API_KEY ?? '',

    // Leagues to track
    leagueIds: (process.env.SPORTS_LEAGUE_IDS ?? 'nfl,nba,mlb,nhl,mls,epl').split(',').map(s => s.trim()),

    // Sync intervals
    liveGamePollSeconds: parseInt(process.env.SPORTS_LIVE_GAME_POLL_SECONDS ?? '30', 10),
    scheduleSyncCron: process.env.SPORTS_SCHEDULE_SYNC_CRON ?? '0 6 * * *',
    standingsSyncCron: process.env.SPORTS_STANDINGS_SYNC_CRON ?? '0 */6 * * *',
    rosterSyncCron: process.env.SPORTS_ROSTER_SYNC_CRON ?? '0 0 * * 1',

    // Notification triggers
    notifyGameStartMinutesBefore: parseInt(process.env.SPORTS_NOTIFY_GAME_START_MINUTES_BEFORE ?? '15', 10),
    notifyScoreChanges: process.env.SPORTS_NOTIFY_SCORE_CHANGES !== 'false',
    notifyGameEnd: process.env.SPORTS_NOTIFY_GAME_END !== 'false',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
