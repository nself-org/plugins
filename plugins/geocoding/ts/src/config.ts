/**
 * Geocoding Plugin Configuration
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

  // Provider configuration
  providers: string[];
  googleApiKey: string;
  mapboxAccessToken: string;
  nominatimUrl: string;
  nominatimEmail: string;

  // Cache settings
  cacheTtlDays: number;
  cacheEnabled: boolean;

  // Batch settings
  maxBatchSize: number;

  // Rate limiting for external providers
  rateLimitProvider: number;

  // Geofence settings
  geofenceCheckToleranceMeters: number;
  notifyUrl: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseDatabaseUrl(url: string | undefined): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} | null {
  if (!url) {
    return null;
  }

  try {
    const match = url.match(/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/);
    if (!match) {
      return null;
    }

    const [, user, password, host, port, database, queryString] = match;
    const ssl = queryString?.includes('sslmode=require') || queryString?.includes('ssl=true') || false;

    return {
      host,
      port: parseInt(port, 10),
      database,
      user,
      password,
      ssl,
    };
  } catch {
    return null;
  }
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('GEOCODING');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.GEOCODING_PLUGIN_PORT ?? process.env.PORT ?? '3203', 10),
    host: process.env.GEOCODING_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // Provider configuration
    providers: (process.env.GEOCODING_PROVIDERS ?? 'nominatim').split(',').map(p => p.trim()),
    googleApiKey: process.env.GEOCODING_GOOGLE_API_KEY ?? '',
    mapboxAccessToken: process.env.GEOCODING_MAPBOX_ACCESS_TOKEN ?? '',
    nominatimUrl: process.env.GEOCODING_NOMINATIM_URL ?? 'https://nominatim.openstreetmap.org',
    nominatimEmail: process.env.GEOCODING_NOMINATIM_EMAIL ?? '',

    // Cache settings
    cacheTtlDays: parseInt(process.env.GEOCODING_CACHE_TTL_DAYS ?? '365', 10),
    cacheEnabled: process.env.GEOCODING_CACHE_ENABLED !== 'false',

    // Batch settings
    maxBatchSize: parseInt(process.env.GEOCODING_MAX_BATCH_SIZE ?? '100', 10),

    // Rate limiting for external providers
    rateLimitProvider: parseInt(process.env.GEOCODING_RATE_LIMIT_PROVIDER ?? '10', 10),

    // Geofence settings
    geofenceCheckToleranceMeters: parseInt(process.env.GEOCODING_GEOFENCE_CHECK_TOLERANCE_METERS ?? '50', 10),
    notifyUrl: process.env.GEOCODING_NOTIFY_URL ?? '',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
