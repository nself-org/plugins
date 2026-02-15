/**
 * Observability Plugin Configuration
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

  // Health checks
  checkIntervalSeconds: number;
  watchdogTimeoutSeconds: number;
  healthHistoryRetainDays: number;

  // Docker
  dockerSocket: string;
  dockerEnabled: boolean;

  // Watchdog
  watchdogEnabled: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('OBSERVABILITY');

  const config: Config = {
    // Server
    port: parseInt(process.env.OBSERVABILITY_PLUGIN_PORT ?? process.env.PORT ?? '3215', 10),
    host: process.env.OBSERVABILITY_PLUGIN_HOST ?? process.env.HOST ?? '127.0.0.1',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Health checks
    checkIntervalSeconds: parseInt(process.env.OBSERVABILITY_CHECK_INTERVAL ?? '30', 10),
    watchdogTimeoutSeconds: parseInt(process.env.OBSERVABILITY_WATCHDOG_TIMEOUT ?? '120', 10),
    healthHistoryRetainDays: parseInt(process.env.OBSERVABILITY_HISTORY_RETAIN_DAYS ?? '30', 10),

    // Docker
    dockerSocket: process.env.OBSERVABILITY_DOCKER_SOCKET ?? '/var/run/docker.sock',
    dockerEnabled: process.env.OBSERVABILITY_DOCKER_ENABLED !== 'false',

    // Watchdog
    watchdogEnabled: process.env.OBSERVABILITY_WATCHDOG_ENABLED !== 'false',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
