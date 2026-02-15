/**
 * Cloudflare Plugin Configuration
 * Environment variable loading and validation
 */

import dotenv from 'dotenv';
import { CloudflareConfig } from './types.js';
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
export function loadConfig(): CloudflareConfig {
  const appIdsRaw = getEnvOptional('CF_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const zoneIdsRaw = getEnvOptional('CF_ZONE_IDS', '');
  const zoneIds = zoneIdsRaw ? parseCsvList(zoneIdsRaw) : [];

  const config: CloudflareConfig = {
    port: getEnvInt('CF_PLUGIN_PORT', 3024),
    host: getEnvOptional('CF_PLUGIN_HOST', '0.0.0.0'),
    logLevel: getEnvOptional('CF_LOG_LEVEL', 'info') as 'debug' | 'info' | 'warn' | 'error',

    database: {
      host: getEnvOptional('POSTGRES_HOST', 'postgres'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    appIds,

    // Cloudflare API
    apiToken: getEnvOptional('CF_API_TOKEN', ''),
    apiKey: getEnvOptional('CF_API_KEY', ''),
    apiEmail: getEnvOptional('CF_API_EMAIL', ''),
    accountId: getEnvOptional('CF_ACCOUNT_ID', ''),

    // Zone filtering
    zoneIds,

    // R2
    r2AccessKey: getEnvOptional('CF_R2_ACCESS_KEY', ''),
    r2SecretKey: getEnvOptional('CF_R2_SECRET_KEY', ''),

    // Sync
    syncInterval: getEnvInt('CF_SYNC_INTERVAL', 3600),
  };

  return config;
}

// Export singleton config instance
export const config = loadConfig();
