/**
 * Activity Feed Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { FeedStrategy } from './types.js';

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

  // Feed settings
  strategy: FeedStrategy;
  maxFeedSize: number;
  aggregationWindowMinutes: number;
  retentionDays: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseFeedStrategy(value: string | undefined): FeedStrategy {
  const normalized = (value ?? 'read').toLowerCase();
  if (normalized === 'write') {
    return 'write';
  }
  return 'read';
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('FEED');

  const config: Config = {
    // Server
    port: parseInt(process.env.FEED_PLUGIN_PORT ?? process.env.PORT ?? '3503', 10),
    host: process.env.FEED_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Feed settings
    strategy: parseFeedStrategy(process.env.FEED_STRATEGY),
    maxFeedSize: parseInt(process.env.FEED_MAX_FEED_SIZE ?? '200', 10),
    aggregationWindowMinutes: parseInt(process.env.FEED_AGGREGATION_WINDOW_MINUTES ?? '60', 10),
    retentionDays: parseInt(process.env.FEED_RETENTION_DAYS ?? '90', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.port < 1 || config.port > 65535) {
    throw new Error(`Invalid port number: ${config.port}`);
  }

  if (config.maxFeedSize < 1) {
    throw new Error(`maxFeedSize must be positive: ${config.maxFeedSize}`);
  }

  if (config.aggregationWindowMinutes < 0) {
    throw new Error(`aggregationWindowMinutes must be non-negative: ${config.aggregationWindowMinutes}`);
  }

  if (config.retentionDays < 1) {
    throw new Error(`retentionDays must be positive: ${config.retentionDays}`);
  }

  return config;
}
