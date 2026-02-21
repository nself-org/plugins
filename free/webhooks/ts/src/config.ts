/**
 * Webhooks Plugin Configuration
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

  // Webhook Delivery Settings
  maxAttempts: number;
  requestTimeoutMs: number;
  maxPayloadSize: number;
  concurrentDeliveries: number;
  retryDelays: number[];
  autoDisableThreshold: number;

  // Security
  security: SecurityConfig;

  // Logging
  logLevel: string;
}

function parseRetryDelays(value: string | undefined): number[] {
  if (!value) {
    return [10000, 30000, 120000, 900000, 3600000]; // Default: 10s, 30s, 2min, 15min, 1hr
  }

  return value
    .split(',')
    .map(delay => parseInt(delay.trim(), 10))
    .filter(delay => !isNaN(delay) && delay > 0);
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('WEBHOOKS');

  const config: Config = {
    // Server
    port: parseInt(process.env.WEBHOOKS_PLUGIN_PORT ?? process.env.PORT ?? '3403', 10),
    host: process.env.WEBHOOKS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Webhook Delivery Settings
    maxAttempts: parseInt(process.env.WEBHOOKS_MAX_ATTEMPTS ?? '5', 10),
    requestTimeoutMs: parseInt(process.env.WEBHOOKS_REQUEST_TIMEOUT_MS ?? '30000', 10),
    maxPayloadSize: parseInt(process.env.WEBHOOKS_MAX_PAYLOAD_SIZE ?? '1048576', 10), // 1MB
    concurrentDeliveries: parseInt(process.env.WEBHOOKS_CONCURRENT_DELIVERIES ?? '10', 10),
    retryDelays: parseRetryDelays(process.env.WEBHOOKS_RETRY_DELAYS),
    autoDisableThreshold: parseInt(process.env.WEBHOOKS_AUTO_DISABLE_THRESHOLD ?? '10', 10),

    // Security
    security,

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.maxAttempts < 1) {
    throw new Error('WEBHOOKS_MAX_ATTEMPTS must be at least 1');
  }

  if (config.requestTimeoutMs < 1000) {
    throw new Error('WEBHOOKS_REQUEST_TIMEOUT_MS must be at least 1000ms');
  }

  if (config.maxPayloadSize < 1024) {
    throw new Error('WEBHOOKS_MAX_PAYLOAD_SIZE must be at least 1024 bytes');
  }

  if (config.concurrentDeliveries < 1) {
    throw new Error('WEBHOOKS_CONCURRENT_DELIVERIES must be at least 1');
  }

  if (config.retryDelays.length === 0) {
    throw new Error('WEBHOOKS_RETRY_DELAYS must contain at least one delay value');
  }

  return config;
}
