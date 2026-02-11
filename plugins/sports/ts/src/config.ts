/**
 * Sports Plugin Configuration
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

  // Provider configuration
  providers: string[];
  espnApiUrl: string;
  sportsdataApiKey: string;
  sportsdataApiUrl: string;

  // Sync configuration
  syncInterval: number;
  livePollInterval: number;
  enabledSports: string[];
  enabledLeagues: string[];

  // Event lock
  lockWindowMinutes: number;
  lockAuto: boolean;

  // Recording integration
  recordingPluginUrl: string;
  autoTriggerRecordings: boolean;
  recordingLeadTimeMinutes: number;
  recordingTrailTimeMinutes: number;

  // Cache
  cacheScheduleTtl: number;
  cacheLiveTtl: number;
  cacheEnabled: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseDatabaseUrl(url: string | undefined): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} | null {
  if (!url) {
    return null;
  }

  try {
    const match = url.match(/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/);
    if (!match) {
      return null;
    }

    const [, user, password, host, port, database, queryString] = match;
    const ssl = queryString?.includes('sslmode=require') || queryString?.includes('ssl=true') || false;

    return {
      host,
      port: parseInt(port, 10),
      database,
      user,
      password,
      ssl,
    };
  } catch {
    return null;
  }
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('SPORTS');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.SPORTS_PLUGIN_PORT ?? process.env.PORT ?? '3201', 10),
    host: process.env.SPORTS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // Provider configuration
    providers: (process.env.SPORTS_PROVIDERS ?? 'espn').split(',').map(p => p.trim()),
    espnApiUrl: process.env.SPORTS_ESPN_API_URL ?? 'https://site.api.espn.com',
    sportsdataApiKey: process.env.SPORTS_SPORTSDATA_API_KEY ?? '',
    sportsdataApiUrl: process.env.SPORTS_SPORTSDATA_API_URL ?? 'https://api.sportsdata.io',

    // Sync configuration
    syncInterval: parseInt(process.env.SPORTS_SYNC_INTERVAL ?? '3600', 10),
    livePollInterval: parseInt(process.env.SPORTS_LIVE_POLL_INTERVAL ?? '30', 10),
    enabledSports: (process.env.SPORTS_ENABLED_SPORTS ?? 'football,basketball,baseball,hockey').split(',').map(s => s.trim()),
    enabledLeagues: (process.env.SPORTS_ENABLED_LEAGUES ?? 'nfl,nba,mlb,nhl').split(',').map(l => l.trim()),

    // Event lock
    lockWindowMinutes: parseInt(process.env.SPORTS_LOCK_WINDOW_MINUTES ?? '120', 10),
    lockAuto: process.env.SPORTS_LOCK_AUTO !== 'false',

    // Recording integration
    recordingPluginUrl: process.env.SPORTS_RECORDING_PLUGIN_URL ?? '',
    autoTriggerRecordings: process.env.SPORTS_AUTO_TRIGGER_RECORDINGS === 'true',
    recordingLeadTimeMinutes: parseInt(process.env.SPORTS_RECORDING_LEAD_TIME_MINUTES ?? '15', 10),
    recordingTrailTimeMinutes: parseInt(process.env.SPORTS_RECORDING_TRAIL_TIME_MINUTES ?? '60', 10),

    // Cache
    cacheScheduleTtl: parseInt(process.env.SPORTS_CACHE_SCHEDULE_TTL ?? '21600', 10),
    cacheLiveTtl: parseInt(process.env.SPORTS_CACHE_LIVE_TTL ?? '30', 10),
    cacheEnabled: process.env.SPORTS_CACHE_ENABLED !== 'false',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
