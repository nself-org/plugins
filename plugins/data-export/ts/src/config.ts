/**
 * Data Export Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // Server
  port: number;
  host: string;

  // Storage
  storagePath: string;
  downloadExpiryHours: number;
  deletionCooldownHours: number;
  maxExportSizeMB: number;
  verificationCodeLength: number;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('EXPORT');

  const config: Config = {
    // Server
    port: parseInt(process.env.EXPORT_PLUGIN_PORT ?? process.env.PORT ?? '3306', 10),
    host: process.env.EXPORT_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Storage
    storagePath: process.env.EXPORT_STORAGE_PATH ?? '/tmp/nself-exports',
    downloadExpiryHours: parseInt(process.env.EXPORT_DOWNLOAD_EXPIRY_HOURS ?? '24', 10),
    deletionCooldownHours: parseInt(process.env.EXPORT_DELETION_COOLDOWN_HOURS ?? '24', 10),
    maxExportSizeMB: parseInt(process.env.EXPORT_MAX_EXPORT_SIZE_MB ?? '500', 10),
    verificationCodeLength: parseInt(process.env.EXPORT_VERIFICATION_CODE_LENGTH ?? '6', 10),

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword) {
    throw new Error('POSTGRES_PASSWORD must be set');
  }

  if (config.verificationCodeLength < 4 || config.verificationCodeLength > 10) {
    throw new Error('EXPORT_VERIFICATION_CODE_LENGTH must be between 4 and 10');
  }

  if (config.maxExportSizeMB < 1 || config.maxExportSizeMB > 10000) {
    throw new Error('EXPORT_MAX_EXPORT_SIZE_MB must be between 1 and 10000');
  }

  return config;
}
