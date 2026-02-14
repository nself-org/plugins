/**
 * Subsonic Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig } from '@nself/plugin-utils';
import type { SubsonicConfig } from './types.js';

export function loadConfig(overrides?: Partial<SubsonicConfig>): SubsonicConfig {
  const security = loadSecurityConfig('SUBSONIC');

  const musicPathsRaw = process.env.SUBSONIC_MUSIC_PATHS ?? '/media/music';
  const musicPaths = musicPathsRaw
    .split(',')
    .map(p => p.trim())
    .filter(Boolean);

  const config: SubsonicConfig = {
    // Server
    port: parseInt(process.env.SUBSONIC_PORT ?? process.env.PORT ?? '3024', 10),
    host: process.env.SUBSONIC_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Music library
    musicPaths,
    adminPassword: process.env.SUBSONIC_ADMIN_PASSWORD ?? 'admin',
    transcodeEnabled: process.env.SUBSONIC_TRANSCODE_ENABLED !== 'false',
    maxBitrate: parseInt(process.env.SUBSONIC_MAX_BITRATE ?? '320', 10),
    coverArtPath: process.env.SUBSONIC_COVER_ART_PATH ?? '/data/subsonic/covers',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Misc
    logLevel: process.env.LOG_LEVEL ?? 'info',
    sourceAccountId: process.env.SOURCE_ACCOUNT_ID ?? 'primary',
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
