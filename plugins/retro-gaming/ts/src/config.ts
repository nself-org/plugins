/**
 * Retro Gaming Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { RetroGamingConfig } from './types.js';

export type { RetroGamingConfig };
export type { SecurityConfig };

export function loadConfig(overrides?: Partial<RetroGamingConfig>): RetroGamingConfig {
  const security = loadSecurityConfig('RETRO_GAMING');

  const config: RetroGamingConfig = {
    // Server
    port: parseInt(process.env.RETRO_GAMING_PLUGIN_PORT ?? process.env.PORT ?? '3033', 10),
    host: process.env.RETRO_GAMING_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database URL (combined)
    databaseUrl: process.env.DATABASE_URL ?? '',

    // Database connection params (individual)
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // IGDB
    igdbClientId: process.env.IGDB_CLIENT_ID ?? '',
    igdbClientSecret: process.env.IGDB_CLIENT_SECRET ?? '',

    // MobyGames
    mobyGamesApiKey: process.env.MOBYGAMES_API_KEY ?? '',

    // Storage
    storageBucket: process.env.RETRO_GAMING_STORAGE_BUCKET ?? '',
    romPathPrefix: process.env.RETRO_GAMING_ROM_PATH_PREFIX ?? '/roms',
    saveStatePathPrefix: process.env.RETRO_GAMING_SAVE_STATE_PATH_PREFIX ?? '/save-states',
    corePathPrefix: process.env.RETRO_GAMING_CORE_PATH_PREFIX ?? '/cores',
    cdnUrl: process.env.RETRO_GAMING_CDN_URL ?? '',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
