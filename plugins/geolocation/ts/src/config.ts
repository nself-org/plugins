/**
 * Geolocation Plugin Configuration
 * Environment variable loading and validation
 */

import dotenv from 'dotenv';
import { GeolocationConfig } from './types.js';
import { parseCsvList, normalizeSourceAccountId } from '@nself/plugin-utils';

// Load environment variables
dotenv.config();

/**
 * Get optional environment variable
 */
function getEnvOptional(key: string, defaultValue: string = ''): string {
  return process.env[key] || defaultValue;
}

/**
 * Get integer environment variable
 */
function getEnvInt(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseInt(value, 10);
  if (isNaN(parsed)) {
    throw new Error(`Invalid integer value for ${key}: ${value}`);
  }
  return parsed;
}

/**
 * Get boolean environment variable
 */
function getEnvBool(key: string, defaultValue: boolean): boolean {
  const value = process.env[key];
  if (!value) return defaultValue;
  return value.toLowerCase() === 'true' || value === '1';
}

/**
 * Load and validate configuration
 */
export function loadConfig(): GeolocationConfig {
  const appIdsRaw = getEnvOptional('GEO_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const config: GeolocationConfig = {
    port: getEnvInt('GEO_PLUGIN_PORT', 3026),
    host: getEnvOptional('GEO_PLUGIN_HOST', '0.0.0.0'),
    logLevel: getEnvOptional('GEO_LOG_LEVEL', 'info') as 'debug' | 'info' | 'warn' | 'error',

    database: {
      host: getEnvOptional('POSTGRES_HOST', 'localhost'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    appIds,

    postgisEnabled: getEnvBool('GEO_POSTGIS_ENABLED', true),
    historyRetentionDays: getEnvInt('GEO_HISTORY_RETENTION_DAYS', 365),
    batchMaxPoints: getEnvInt('GEO_BATCH_MAX_POINTS', 1000),
    minUpdateIntervalSeconds: getEnvInt('GEO_MIN_UPDATE_INTERVAL_SECONDS', 30),
    geofenceCheckOnUpdate: getEnvBool('GEO_GEOFENCE_CHECK_ON_UPDATE', true),
    reverseGeocodeEnabled: getEnvBool('GEO_REVERSE_GEOCODE_ENABLED', false),
    reverseGeocodeProvider: getEnvOptional('GEO_REVERSE_GEOCODE_PROVIDER', ''),
    reverseGeocodeApiKey: getEnvOptional('GEO_REVERSE_GEOCODE_API_KEY', ''),
    lowBatteryThreshold: getEnvInt('GEO_LOW_BATTERY_THRESHOLD', 15),
  };

  return config;
}

// Export singleton config instance
export const config = loadConfig();
