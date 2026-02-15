/**
 * Recording Plugin Configuration
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

  // Recording settings
  storageUrl: string;
  fileProcessingUrl: string;
  sportsUrl: string;
  gameMetadataUrl: string;
  devicesUrl: string;
  defaultLeadTimeMinutes: number;
  defaultTrailTimeMinutes: number;
  encodeProfiles: string[];
  defaultEncodeProfile: string;
  autoEncode: boolean;
  autoEnrich: boolean;
  autoPublish: boolean;
  maxConcurrentRecordings: number;
  maxConcurrentEncodes: number;
  storagePathTemplate: string;
  thumbnailAtSeconds: number;

  // Per-app overrides
  appTvMaxConcurrentRecordings: number;
  appTvAutoPublish: boolean;

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
  const security = loadSecurityConfig('REC');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.REC_PLUGIN_PORT ?? process.env.PORT ?? '3602', 10),
    host: process.env.REC_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // Recording settings (Docker service names for plugin-to-plugin communication)
    storageUrl: process.env.REC_STORAGE_URL ?? 'http://plugin-object-storage:3301',
    fileProcessingUrl: process.env.REC_FILE_PROCESSING_URL ?? 'http://plugin-file-processing:3104',
    sportsUrl: process.env.REC_SPORTS_URL ?? 'http://plugin-sports:3035',
    gameMetadataUrl: process.env.REC_GAME_METADATA_URL ?? 'http://plugin-game-metadata:3211',
    devicesUrl: process.env.REC_DEVICES_URL ?? 'http://plugin-devices:3603',
    defaultLeadTimeMinutes: parseInt(process.env.REC_DEFAULT_LEAD_TIME_MINUTES ?? '5', 10),
    defaultTrailTimeMinutes: parseInt(process.env.REC_DEFAULT_TRAIL_TIME_MINUTES ?? '15', 10),
    encodeProfiles: (process.env.REC_ENCODE_PROFILES ?? '720p,1080p').split(','),
    defaultEncodeProfile: process.env.REC_DEFAULT_ENCODE_PROFILE ?? '1080p',
    autoEncode: process.env.REC_AUTO_ENCODE !== 'false',
    autoEnrich: process.env.REC_AUTO_ENRICH !== 'false',
    autoPublish: process.env.REC_AUTO_PUBLISH === 'true',
    maxConcurrentRecordings: parseInt(process.env.REC_MAX_CONCURRENT_RECORDINGS ?? '4', 10),
    maxConcurrentEncodes: parseInt(process.env.REC_MAX_CONCURRENT_ENCODES ?? '2', 10),
    storagePathTemplate: process.env.REC_STORAGE_PATH_TEMPLATE ?? '{{year}}/{{month}}/{{title}}',
    thumbnailAtSeconds: parseInt(process.env.REC_THUMBNAIL_AT_SECONDS ?? '30', 10),

    // Per-app overrides
    appTvMaxConcurrentRecordings: parseInt(process.env.REC_APP_TV_MAX_CONCURRENT_RECORDINGS ?? '4', 10),
    appTvAutoPublish: process.env.REC_APP_TV_AUTO_PUBLISH === 'true',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
