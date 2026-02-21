/**
 * Feature Flags Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { FeatureFlagsConfig } from './types.js';

export interface Config {
  // Feature Flags
  evaluationLogEnabled: boolean;
  evaluationLogSampleRate: number;
  cacheTtlSeconds: number;

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

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseBoolean(value: string | undefined, defaultValue: boolean): boolean {
  if (value === undefined) {
    return defaultValue;
  }
  return value.toLowerCase() === 'true' || value === '1';
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('FF');

  const config: Config = {
    // Feature Flags
    evaluationLogEnabled: parseBoolean(process.env.FF_EVALUATION_LOG_ENABLED, true),
    evaluationLogSampleRate: parseInt(process.env.FF_EVALUATION_LOG_SAMPLE_RATE ?? '100', 10),
    cacheTtlSeconds: parseInt(process.env.FF_CACHE_TTL_SECONDS ?? '30', 10),

    // Server
    port: parseInt(process.env.FF_PLUGIN_PORT ?? process.env.PORT ?? '3305', 10),
    host: process.env.FF_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
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
  if (config.evaluationLogSampleRate < 0 || config.evaluationLogSampleRate > 100) {
    throw new Error('FF_EVALUATION_LOG_SAMPLE_RATE must be between 0 and 100');
  }

  if (config.cacheTtlSeconds < 0) {
    throw new Error('FF_CACHE_TTL_SECONDS must be non-negative');
  }

  return config;
}

export function toFeatureFlagsConfig(config: Config): FeatureFlagsConfig {
  return {
    port: config.port,
    host: config.host,
    evaluationLogEnabled: config.evaluationLogEnabled,
    evaluationLogSampleRate: config.evaluationLogSampleRate,
    cacheTtlSeconds: config.cacheTtlSeconds,
    apiKey: config.security.apiKey,
    rateLimitMax: config.security.rateLimitMax ?? 500,
    rateLimitWindowMs: config.security.rateLimitWindowMs ?? 60000,
    database: {
      host: config.databaseHost,
      port: config.databasePort,
      database: config.databaseName,
      user: config.databaseUser,
      password: config.databasePassword,
      ssl: config.databaseSsl,
    },
  };
}
