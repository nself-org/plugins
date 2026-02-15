/**
 * CMS Plugin Configuration
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

  // CMS Settings
  maxBodyLength: number;
  maxTitleLength: number;
  slugMaxLength: number;
  maxVersions: number;
  scheduledCheckIntervalMs: number;
  defaultContentTypes: string[];

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseCsvList(value: string | undefined): string[] {
  if (!value) {
    return [];
  }

  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('CMS');

  const config: Config = {
    // Server
    port: parseInt(process.env.CMS_PLUGIN_PORT ?? process.env.PORT ?? '3501', 10),
    host: process.env.CMS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // CMS Settings
    maxBodyLength: parseInt(process.env.CMS_MAX_BODY_LENGTH ?? '500000', 10),
    maxTitleLength: parseInt(process.env.CMS_MAX_TITLE_LENGTH ?? '500', 10),
    slugMaxLength: parseInt(process.env.CMS_SLUG_MAX_LENGTH ?? '200', 10),
    maxVersions: parseInt(process.env.CMS_MAX_VERSIONS ?? '50', 10),
    scheduledCheckIntervalMs: parseInt(process.env.CMS_SCHEDULED_CHECK_INTERVAL_MS ?? '60000', 10),
    defaultContentTypes: parseCsvList(process.env.CMS_DEFAULT_CONTENT_TYPES ?? 'post,page,recipe'),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databaseHost) {
    throw new Error('DATABASE_URL or POSTGRES_HOST must be set');
  }

  if (config.maxBodyLength < 1000) {
    throw new Error('CMS_MAX_BODY_LENGTH must be at least 1000 bytes');
  }

  if (config.maxTitleLength < 10) {
    throw new Error('CMS_MAX_TITLE_LENGTH must be at least 10 characters');
  }

  if (config.slugMaxLength < 10) {
    throw new Error('CMS_SLUG_MAX_LENGTH must be at least 10 characters');
  }

  if (config.maxVersions < 1) {
    throw new Error('CMS_MAX_VERSIONS must be at least 1');
  }

  return config;
}
