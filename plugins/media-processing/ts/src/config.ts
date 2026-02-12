/**
 * Media Processing Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // Server
  port: number;
  host: string;

  // FFmpeg
  ffmpegPath: string;
  ffprobePath: string;
  outputBasePath: string;
  maxConcurrentJobs: number;
  maxInputSizeGb: number;
  hardwareAccel: 'none' | 'nvenc' | 'vaapi' | 'qsv';

  // Packager (UPGRADE 1a)
  packager: 'shaka' | 'bento4' | 'ffmpeg-only';
  shakaPackagerPath: string;
  outputFormats: ('hls' | 'dash' | 'cmaf')[];

  // Drop-Folder Watcher (UPGRADE 1c)
  dropFolderPath: string;
  settleCheckSeconds: number;
  settleCheckIntervals: number;

  // Content Identification (UPGRADE 1d)
  metadataEnrichmentUrl: string;

  // Object Storage (UPGRADE 1e)
  objectStorageUrl: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseUrl(databaseUrl: string): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} {
  try {
    const url = new URL(databaseUrl);
    return {
      host: url.hostname,
      port: parseInt(url.port || '5432', 10),
      database: url.pathname.slice(1),
      user: url.username,
      password: url.password,
      ssl: url.searchParams.get('sslmode') === 'require',
    };
  } catch (error) {
    throw new Error(`Invalid DATABASE_URL format: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('MP');

  // Parse DATABASE_URL if provided
  const databaseUrl = process.env.DATABASE_URL;
  let dbConfig = {
    host: process.env.POSTGRES_HOST ?? 'localhost',
    port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: process.env.POSTGRES_DB ?? 'nself',
    user: process.env.POSTGRES_USER ?? 'postgres',
    password: process.env.POSTGRES_PASSWORD ?? '',
    ssl: process.env.POSTGRES_SSL === 'true',
  };

  if (databaseUrl) {
    dbConfig = parseUrl(databaseUrl);
  }

  // Parse output formats
  const outputFormatsRaw = process.env.MP_OUTPUT_FORMATS ?? 'hls';
  const outputFormats = outputFormatsRaw.split(',').map(f => f.trim()).filter(
    (f): f is 'hls' | 'dash' | 'cmaf' => ['hls', 'dash', 'cmaf'].includes(f)
  );

  const config: Config = {
    // Server
    port: parseInt(process.env.MP_PLUGIN_PORT ?? process.env.PORT ?? '3019', 10),
    host: process.env.MP_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // FFmpeg
    ffmpegPath: process.env.MP_FFMPEG_PATH ?? 'ffmpeg',
    ffprobePath: process.env.MP_FFPROBE_PATH ?? 'ffprobe',
    outputBasePath: process.env.MP_OUTPUT_BASE_PATH ?? '/data/media-processing',
    maxConcurrentJobs: parseInt(process.env.MP_MAX_CONCURRENT_JOBS ?? '2', 10),
    maxInputSizeGb: parseInt(process.env.MP_MAX_INPUT_SIZE_GB ?? '50', 10),
    hardwareAccel: (process.env.MP_HARDWARE_ACCEL ?? 'none') as 'none' | 'nvenc' | 'vaapi' | 'qsv',

    // Packager (UPGRADE 1a)
    packager: (process.env.MP_PACKAGER ?? 'ffmpeg-only') as 'shaka' | 'bento4' | 'ffmpeg-only',
    shakaPackagerPath: process.env.MP_SHAKA_PACKAGER_PATH ?? 'packager',
    outputFormats: outputFormats.length > 0 ? outputFormats : ['hls'],

    // Drop-Folder Watcher (UPGRADE 1c)
    dropFolderPath: process.env.MP_DROP_FOLDER_PATH ?? '',
    settleCheckSeconds: parseInt(process.env.MP_SETTLE_CHECK_SECONDS ?? '5', 10),
    settleCheckIntervals: parseInt(process.env.MP_SETTLE_CHECK_INTERVALS ?? '3', 10),

    // Content Identification (UPGRADE 1d)
    metadataEnrichmentUrl: process.env.MP_METADATA_ENRICHMENT_URL ?? 'http://localhost:3203',

    // Object Storage (UPGRADE 1e)
    objectStorageUrl: process.env.MP_OBJECT_STORAGE_URL ?? 'http://localhost:3301',

    // Database
    databaseHost: dbConfig.host,
    databasePort: dbConfig.port,
    databaseName: dbConfig.database,
    databaseUser: dbConfig.user,
    databasePassword: dbConfig.password,
    databaseSsl: dbConfig.ssl,

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databaseName) {
    throw new Error('Database name must be configured (DATABASE_URL or POSTGRES_DB)');
  }

  if (config.maxConcurrentJobs < 1) {
    throw new Error('MP_MAX_CONCURRENT_JOBS must be at least 1');
  }

  if (config.maxInputSizeGb < 1) {
    throw new Error('MP_MAX_INPUT_SIZE_GB must be at least 1');
  }

  if (!['none', 'nvenc', 'vaapi', 'qsv'].includes(config.hardwareAccel)) {
    throw new Error('MP_HARDWARE_ACCEL must be one of: none, nvenc, vaapi, qsv');
  }

  return config;
}
