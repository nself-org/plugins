/**
 * Podcast Plugin Configuration
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

  // iTunes / Podcast Index
  itunesSearchUrl: string;
  podcastIndexApiKey: string;
  podcastIndexApiSecret: string;

  // Feed refresh intervals
  refreshActiveMinutes: number;
  refreshDormantHours: number;
  refreshStaleHours: number;
  maxConsecutiveErrors: number;

  // Downloads
  downloadPath: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('PODCAST');

  const config: Config = {
    // Server
    port: parseInt(process.env.PODCAST_PORT ?? process.env.PORT ?? '3023', 10),
    host: process.env.PODCAST_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // iTunes / Podcast Index
    itunesSearchUrl: process.env.PODCAST_ITUNES_SEARCH_URL ?? 'https://itunes.apple.com/search',
    podcastIndexApiKey: process.env.PODCAST_INDEX_API_KEY ?? '',
    podcastIndexApiSecret: process.env.PODCAST_INDEX_API_SECRET ?? '',

    // Feed refresh
    refreshActiveMinutes: parseInt(process.env.PODCAST_REFRESH_ACTIVE_MINUTES ?? '60', 10),
    refreshDormantHours: parseInt(process.env.PODCAST_REFRESH_DORMANT_HOURS ?? '6', 10),
    refreshStaleHours: parseInt(process.env.PODCAST_REFRESH_STALE_HOURS ?? '24', 10),
    maxConsecutiveErrors: parseInt(process.env.PODCAST_MAX_CONSECUTIVE_ERRORS ?? '7', 10),

    // Downloads
    downloadPath: process.env.PODCAST_DOWNLOAD_PATH ?? '/data/podcasts',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
