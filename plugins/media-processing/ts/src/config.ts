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
