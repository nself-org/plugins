/**
 * Devices Plugin Configuration
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

  // Device settings
  enrollmentTokenTtl: number;
  challengeTtl: number;
  heartbeatInterval: number;
  heartbeatTimeout: number;
  commandDefaultTimeout: number;
  commandMaxRetries: number;
  telemetryRetentionDays: number;

  // nTV settings
  bootstrapTokenTtl: number;
  heartbeatOfflineTimeout: number;

  // Ingest settings
  ingestHeartbeatInterval: number;
  ingestHeartbeatTimeout: number;
  ingestRetryMax: number;
  ingestRetryBackoffBase: number;

  // External service URLs
  realtimeUrl: string;
  recordingUrl: string;
  streamGatewayUrl: string;
  redisUrl: string;

  // Per-app overrides
  appTvHeartbeatInterval: number;
  appTvIngestHeartbeatInterval: number;

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
  const security = loadSecurityConfig('DEV');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.DEV_PLUGIN_PORT ?? process.env.PORT ?? '3603', 10),
    host: process.env.DEV_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // Device settings
    enrollmentTokenTtl: parseInt(process.env.DEV_ENROLLMENT_TOKEN_TTL ?? '3600', 10),
    challengeTtl: parseInt(process.env.DEV_CHALLENGE_TTL ?? '300', 10),
    heartbeatInterval: parseInt(process.env.DEV_HEARTBEAT_INTERVAL ?? '60', 10),
    heartbeatTimeout: parseInt(process.env.DEV_HEARTBEAT_TIMEOUT ?? '180', 10),
    commandDefaultTimeout: parseInt(process.env.DEV_COMMAND_DEFAULT_TIMEOUT ?? '300', 10),
    commandMaxRetries: parseInt(process.env.DEV_COMMAND_MAX_RETRIES ?? '3', 10),
    telemetryRetentionDays: parseInt(process.env.DEV_TELEMETRY_RETENTION_DAYS ?? '30', 10),

    // nTV settings
    bootstrapTokenTtl: parseInt(process.env.DEV_BOOTSTRAP_TOKEN_TTL ?? '86400', 10),
    heartbeatOfflineTimeout: parseInt(process.env.DEV_HEARTBEAT_OFFLINE_TIMEOUT ?? '90', 10),

    // Ingest settings
    ingestHeartbeatInterval: parseInt(process.env.DEV_INGEST_HEARTBEAT_INTERVAL ?? '10', 10),
    ingestHeartbeatTimeout: parseInt(process.env.DEV_INGEST_HEARTBEAT_TIMEOUT ?? '30', 10),
    ingestRetryMax: parseInt(process.env.DEV_INGEST_RETRY_MAX ?? '5', 10),
    ingestRetryBackoffBase: parseInt(process.env.DEV_INGEST_RETRY_BACKOFF_BASE ?? '5', 10),

    // External service URLs (Docker service names for container-to-container communication)
    realtimeUrl: process.env.DEV_REALTIME_URL ?? 'http://plugin-realtime:3101',
    recordingUrl: process.env.DEV_RECORDING_URL ?? 'http://plugin-recording:3602',
    streamGatewayUrl: process.env.DEV_STREAM_GATEWAY_URL ?? 'http://plugin-stream-gateway:3601',
    redisUrl: process.env.REDIS_URL ?? 'redis://redis:6379',

    // Per-app overrides
    appTvHeartbeatInterval: parseInt(process.env.DEV_APP_TV_HEARTBEAT_INTERVAL ?? '30', 10),
    appTvIngestHeartbeatInterval: parseInt(process.env.DEV_APP_TV_INGEST_HEARTBEAT_INTERVAL ?? '5', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
