/**
 * Tokens Plugin Configuration
 * Environment variable loading and validation
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import { parseCsvList, normalizeSourceAccountId } from '@nself/plugin-utils';
import type { TokensConfig } from './types.js';

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

export function loadConfig(): TokensConfig {
  const appIdsRaw = getEnvOptional('TOKENS_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const security: SecurityConfig = loadSecurityConfig('TOKENS');

  return {
    port: getEnvInt('TOKENS_PLUGIN_PORT', 3021),
    host: getEnvOptional('TOKENS_PLUGIN_HOST', '0.0.0.0'),
    logLevel: getEnvOptional('TOKENS_LOG_LEVEL', 'info') as TokensConfig['logLevel'],

    database: {
      host: getEnvOptional('POSTGRES_HOST', 'postgres'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    appIds,

    // Encryption
    encryptionKey: getEnv('TOKENS_ENCRYPTION_KEY'),

    // Token defaults
    defaultTtlSeconds: getEnvInt('TOKENS_DEFAULT_TTL_SECONDS', 3600),
    maxTtlSeconds: getEnvInt('TOKENS_MAX_TTL_SECONDS', 86400),
    signingAlgorithm: getEnvOptional('TOKENS_SIGNING_ALGORITHM', 'hmac-sha256'),

    // HLS encryption
    hlsEncryptionEnabled: getEnvBool('TOKENS_HLS_ENCRYPTION_ENABLED', false),
    hlsKeyRotationHours: getEnvInt('TOKENS_HLS_KEY_ROTATION_HOURS', 168),

    // Entitlement defaults
    defaultEntitlementCheck: getEnvBool('TOKENS_DEFAULT_ENTITLEMENT_CHECK', true),
    allowAllIfNoEntitlements: getEnvBool('TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS', true),

    // Cleanup
    expiredRetentionDays: getEnvInt('TOKENS_EXPIRED_RETENTION_DAYS', 7),

    security,
  };
}

export const config = loadConfig();
