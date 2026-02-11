/**
 * TMDB Plugin Configuration
 * Environment variable loading and validation
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import { parseCsvList, normalizeSourceAccountId } from '@nself/plugin-utils';
import type { TmdbConfig } from './types.js';

function getEnv(key: string, defaultValue?: string): string {
  const value = process.env[key];
  if (value === undefined) {
    if (defaultValue !== undefined) {
      return defaultValue;
    }
    throw new Error(`Missing required environment variable: ${key}`);
  }
  return value;
}

function getEnvOptional(key: string, defaultValue: string = ''): string {
  return process.env[key] || defaultValue;
}

function getEnvInt(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseInt(value, 10);
  if (isNaN(parsed)) {
    throw new Error(`Invalid integer value for ${key}: ${value}`);
  }
  return parsed;
}

function getEnvBool(key: string, defaultValue: boolean): boolean {
  const value = process.env[key];
  if (!value) return defaultValue;
  return value.toLowerCase() === 'true' || value === '1';
}

function getEnvFloat(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseFloat(value);
  if (isNaN(parsed)) {
    throw new Error(`Invalid float value for ${key}: ${value}`);
  }
  return parsed;
}

export function loadConfig(): TmdbConfig {
  const appIdsRaw = getEnvOptional('TMDB_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const security: SecurityConfig = loadSecurityConfig('TMDB');

  return {
    port: getEnvInt('TMDB_PLUGIN_PORT', 3020),
    host: getEnvOptional('TMDB_PLUGIN_HOST', '0.0.0.0'),
    logLevel: getEnvOptional('TMDB_LOG_LEVEL', 'info') as TmdbConfig['logLevel'],

    database: {
      host: getEnvOptional('POSTGRES_HOST', 'localhost'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    appIds,

    // TMDB API
    tmdbApiKey: getEnv('TMDB_API_KEY'),
    tmdbApiReadAccessToken: getEnvOptional('TMDB_API_READ_ACCESS_TOKEN'),
    omdbApiKey: getEnvOptional('OMDB_API_KEY'),

    // Matching
    autoAcceptThreshold: getEnvFloat('TMDB_AUTO_ACCEPT_THRESHOLD', 0.85),
    filenameParsing: getEnvBool('TMDB_FILENAME_PARSING', true),
    defaultLanguage: getEnvOptional('TMDB_DEFAULT_LANGUAGE', 'en-US'),

    // Caching
    cacheTtlDays: getEnvInt('TMDB_CACHE_TTL_DAYS', 30),
    refreshCron: getEnvOptional('TMDB_REFRESH_CRON', '0 6 * * 0'),

    // Images
    imageBaseUrl: getEnvOptional('TMDB_IMAGE_BASE_URL', 'https://image.tmdb.org/t/p/'),
    posterSize: getEnvOptional('TMDB_POSTER_SIZE', 'w500'),
    backdropSize: getEnvOptional('TMDB_BACKDROP_SIZE', 'w1280'),

    // Rate limiting (TMDB allows ~40 req/10s)
    rateLimitRequests: getEnvInt('TMDB_RATE_LIMIT_REQUESTS', 35),
    rateLimitWindowMs: getEnvInt('TMDB_RATE_LIMIT_WINDOW_MS', 10000),

    security,
  };
}

export const config = loadConfig();
