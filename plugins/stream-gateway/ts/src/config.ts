/**
 * Stream Gateway Plugin Configuration
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

  // Stream Gateway settings
  heartbeatInterval: number;
  heartbeatTimeout: number;
  defaultMaxConcurrent: number;
  defaultMaxDeviceStreams: number;
  sessionMaxDurationHours: number;
  analyticsInterval: number;
  realtimeUrl: string;
  redisUrl: string;

  // Per-app overrides
  appTvMaxConcurrent: number;
  appTvMaxDeviceStreams: number;
  appFamilyMaxConcurrent: number;

  // URL signing (nTV v1 API)
  signingSecret: string;
  signedUrlExpirySeconds: number;

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

    return { host, port: parseInt(port, 10), database, user, password, ssl };
  } catch {
    return null;
  }
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('SG');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.SG_PLUGIN_PORT ?? process.env.PORT ?? '3601', 10),
    host: process.env.SG_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // Stream Gateway settings
    heartbeatInterval: parseInt(process.env.SG_HEARTBEAT_INTERVAL ?? '30', 10),
    heartbeatTimeout: parseInt(process.env.SG_HEARTBEAT_TIMEOUT ?? '90', 10),
    defaultMaxConcurrent: parseInt(process.env.SG_DEFAULT_MAX_CONCURRENT ?? '3', 10),
    defaultMaxDeviceStreams: parseInt(process.env.SG_DEFAULT_MAX_DEVICE_STREAMS ?? '1', 10),
    sessionMaxDurationHours: parseInt(process.env.SG_SESSION_MAX_DURATION_HOURS ?? '12', 10),
    analyticsInterval: parseInt(process.env.SG_ANALYTICS_INTERVAL ?? '300', 10),
    realtimeUrl: process.env.SG_REALTIME_URL ?? 'http://localhost:3101',
    redisUrl: process.env.REDIS_URL ?? 'redis://localhost:6379',

    // Per-app overrides
    appTvMaxConcurrent: parseInt(process.env.SG_APP_TV_MAX_CONCURRENT ?? '5', 10),
    appTvMaxDeviceStreams: parseInt(process.env.SG_APP_TV_MAX_DEVICE_STREAMS ?? '1', 10),
    appFamilyMaxConcurrent: parseInt(process.env.SG_APP_FAMILY_MAX_CONCURRENT ?? '2', 10),

    // URL signing (nTV v1 API)
    signingSecret: process.env.SG_SIGNING_SECRET ?? '',
    signedUrlExpirySeconds: parseInt(process.env.SG_SIGNED_URL_EXPIRY_SECONDS ?? '3600', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
