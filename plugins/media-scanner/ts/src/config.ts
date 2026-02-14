/**
 * Media Scanner Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig } from '@nself/plugin-utils';
import type { MediaScannerConfig } from './types.js';

export type { MediaScannerConfig } from './types.js';

export function loadConfig(overrides?: Partial<MediaScannerConfig>): MediaScannerConfig {
  const security = loadSecurityConfig('MEDIA_SCANNER');

  const config: MediaScannerConfig = {
    // Server
    port: parseInt(process.env.MEDIA_SCANNER_PORT ?? process.env.PORT ?? '3021', 10),
    host: process.env.MEDIA_SCANNER_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // MeiliSearch
    meilisearchUrl: process.env.MEILISEARCH_URL ?? 'http://localhost:7700',
    meilisearchKey: process.env.MEILISEARCH_KEY ?? '',

    // TMDB
    tmdbApiKey: process.env.TMDB_API_KEY ?? '',

    // Library
    libraryPaths: parseLibraryPaths(process.env.MEDIA_LIBRARY_PATHS),
    scanIntervalHours: parseInt(process.env.SCAN_INTERVAL_HOURS ?? '24', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}

function parseLibraryPaths(value: string | undefined): string[] {
  if (!value) {
    return [];
  }
  return value
    .split(',')
    .map(p => p.trim())
    .filter(Boolean);
}
