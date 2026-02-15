/**
 * Game Metadata Plugin Configuration
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

  // IGDB Integration
  igdbClientId: string;
  igdbClientSecret: string;
  igdbRateLimitPerSecond: number;

  // Artwork
  artworkPath: string;
  maxArtworkSizeMb: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('GAME_METADATA');

  const config: Config = {
    // Server
    port: parseInt(process.env.GAME_METADATA_PLUGIN_PORT ?? process.env.PORT ?? '3211', 10),
    host: process.env.GAME_METADATA_PLUGIN_HOST ?? process.env.HOST ?? '127.0.0.1',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // IGDB Integration
    igdbClientId: process.env.IGDB_CLIENT_ID ?? '',
    igdbClientSecret: process.env.IGDB_CLIENT_SECRET ?? '',
    igdbRateLimitPerSecond: parseInt(process.env.IGDB_RATE_LIMIT ?? '4', 10),

    // Artwork
    artworkPath: process.env.GAME_METADATA_ARTWORK_PATH ?? './artwork',
    maxArtworkSizeMb: parseInt(process.env.GAME_METADATA_MAX_ARTWORK_SIZE_MB ?? '50', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
