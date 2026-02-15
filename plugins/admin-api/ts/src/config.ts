/**
 * Admin API Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig } from '@nself/plugin-utils';
import type { Config } from './types.js';

export type { Config };

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('ADMIN_API');

  const config: Config = {
    // Server
    port: parseInt(process.env.ADMIN_API_PLUGIN_PORT ?? process.env.PORT ?? '3212', 10),
    host: process.env.ADMIN_API_PLUGIN_HOST ?? process.env.HOST ?? '127.0.0.1',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Prometheus
    prometheusUrl: process.env.PROMETHEUS_URL ?? '',

    // Cache
    cacheTtlSeconds: parseInt(process.env.ADMIN_API_CACHE_TTL ?? '30', 10),

    // Metrics
    metricsRetentionDays: parseInt(process.env.ADMIN_API_METRICS_RETENTION_DAYS ?? '90', 10),
    snapshotIntervalMinutes: parseInt(process.env.ADMIN_API_SNAPSHOT_INTERVAL_MINUTES ?? '5', 10),

    // WebSocket
    wsEnabled: process.env.ADMIN_API_WS_ENABLED === 'true',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
