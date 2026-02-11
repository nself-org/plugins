/**
 * TMDB Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // TMDB
  tmdbApiKey: string;
  tmdbApiReadAccessToken?: string;
  tmdbImageBaseUrl: string;
  tmdbDefaultLanguage: string;
  tmdbAutoEnrich: boolean;
  tmdbConfidenceThreshold: number;
  tmdbCacheTtlDays: number;

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

  // Rate limiting
  rateLimitMax: number;
  rateLimitWindowMs: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('TMDB');

  const config: Config = {
    // TMDB
    tmdbApiKey: process.env.TMDB_API_KEY ?? '',
    tmdbApiReadAccessToken: process.env.TMDB_API_READ_ACCESS_TOKEN,
    tmdbImageBaseUrl: process.env.TMDB_IMAGE_BASE_URL ?? 'https://image.tmdb.org/t/p',
    tmdbDefaultLanguage: process.env.TMDB_DEFAULT_LANGUAGE ?? 'en-US',
    tmdbAutoEnrich: process.env.TMDB_AUTO_ENRICH === 'true',
    tmdbConfidenceThreshold: parseFloat(process.env.TMDB_CONFIDENCE_THRESHOLD ?? '0.70'),
    tmdbCacheTtlDays: parseInt(process.env.TMDB_CACHE_TTL_DAYS ?? '30', 10),

    // Server
    port: parseInt(process.env.TMDB_PLUGIN_PORT ?? process.env.PORT ?? '3202', 10),
    host: process.env.TMDB_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Rate limiting
    rateLimitMax: parseInt(process.env.TMDB_RATE_LIMIT_MAX ?? '100', 10),
    rateLimitWindowMs: parseInt(process.env.TMDB_RATE_LIMIT_WINDOW_MS ?? '60000', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.tmdbApiKey) {
    throw new Error('TMDB_API_KEY must be set');
  }

  if (config.tmdbConfidenceThreshold < 0 || config.tmdbConfidenceThreshold > 1) {
    throw new Error('TMDB_CONFIDENCE_THRESHOLD must be between 0 and 1');
  }

  return config;
}
