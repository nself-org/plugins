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

  // RSS settings
  rssPollIntervalMinutes: number;
  maxEpisodesPerFeed: number;
  feedTimeoutSeconds: number;

  // Storage
  storageBackend: string;

  // Cleanup
  cleanupOldEpisodesDays: number;
  cleanupCron: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('PODCAST');

  const config: Config = {
    // Server
    port: parseInt(process.env.PODCAST_PLUGIN_PORT ?? process.env.PORT ?? '3210', 10),
    host: process.env.PODCAST_PLUGIN_HOST ?? process.env.HOST ?? '127.0.0.1',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // RSS settings
    rssPollIntervalMinutes: parseInt(process.env.PODCAST_RSS_POLL_INTERVAL ?? '60', 10),
    maxEpisodesPerFeed: parseInt(process.env.PODCAST_MAX_EPISODES ?? '500', 10),
    feedTimeoutSeconds: parseInt(process.env.PODCAST_FEED_TIMEOUT ?? '30', 10),

    // Storage
    storageBackend: process.env.PODCAST_STORAGE_BACKEND ?? 'database',

    // Cleanup
    cleanupOldEpisodesDays: parseInt(process.env.PODCAST_CLEANUP_OLD_EPISODES_DAYS ?? '365', 10),
    cleanupCron: process.env.PODCAST_CLEANUP_CRON ?? '0 4 * * *',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
