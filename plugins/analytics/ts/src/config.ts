/**
 * Analytics Plugin Configuration
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

  // Analytics
  batchSize: number;
  rollupIntervalMs: number;
  eventRetentionDays: number;
  counterRetentionDays: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('ANALYTICS');

  const config: Config = {
    // Server
    port: parseInt(process.env.ANALYTICS_PLUGIN_PORT ?? process.env.PORT ?? '3304', 10),
    host: process.env.ANALYTICS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Analytics
    batchSize: parseInt(process.env.ANALYTICS_BATCH_SIZE ?? '100', 10),
    rollupIntervalMs: parseInt(process.env.ANALYTICS_ROLLUP_INTERVAL_MS ?? '3600000', 10),
    eventRetentionDays: parseInt(process.env.ANALYTICS_EVENT_RETENTION_DAYS ?? '90', 10),
    counterRetentionDays: parseInt(process.env.ANALYTICS_COUNTER_RETENTION_DAYS ?? '365', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.batchSize < 1 || config.batchSize > 1000) {
    throw new Error('ANALYTICS_BATCH_SIZE must be between 1 and 1000');
  }

  if (config.rollupIntervalMs < 60000) {
    throw new Error('ANALYTICS_ROLLUP_INTERVAL_MS must be at least 60000ms (1 minute)');
  }

  return config;
}
