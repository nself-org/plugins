/**
 * Configuration loader for realtime plugin
 */

import { config as loadEnv } from 'dotenv';
import type { RealtimeConfig } from './types.js';

// Load environment variables
loadEnv();

/**
 * Parse comma-separated string into array
 */
function parseArray(value: string | undefined, defaultValue: string[]): string[] {
  if (!value) return defaultValue;
  return value.split(',').map(s => s.trim()).filter(Boolean);
}

/**
 * Parse boolean from environment variable
 */
function parseBoolean(value: string | undefined, defaultValue: boolean): boolean {
  if (!value) return defaultValue;
  return value.toLowerCase() === 'true' || value === '1';
}

/**
 * Parse integer from environment variable
 */
function parseIntEnv(value: string | undefined, defaultValue: number): number {
  if (!value) return defaultValue;
  const parsed = Number(value);
  return isNaN(parsed) ? defaultValue : parsed;
}

/**
 * Load and validate configuration
 */
export function loadConfig(): RealtimeConfig {
  // Required variables
  const redisUrl = process.env.REALTIME_REDIS_URL;
  if (!redisUrl) {
    throw new Error('REALTIME_REDIS_URL is required');
  }

  const corsOrigin = process.env.REALTIME_CORS_ORIGIN;
  if (!corsOrigin) {
    throw new Error('REALTIME_CORS_ORIGIN is required');
  }

  return {
    // Server
    port: parseIntEnv(process.env.REALTIME_PORT, 3101),
    host: process.env.REALTIME_HOST || '0.0.0.0',
    redisUrl,
    corsOrigin: parseArray(corsOrigin, ['http://localhost:3000']),

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseIntEnv(process.env.POSTGRES_PORT, 5432),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Limits
    maxConnections: parseIntEnv(process.env.REALTIME_MAX_CONNECTIONS, 10000),
    pingTimeout: parseIntEnv(process.env.REALTIME_PING_TIMEOUT, 60000),
    pingInterval: parseIntEnv(process.env.REALTIME_PING_INTERVAL, 25000),

    // Authentication
    jwtSecret: process.env.REALTIME_JWT_SECRET,
    allowAnonymous: parseBoolean(process.env.REALTIME_ALLOW_ANONYMOUS, false),

    // Features
    enablePresence: parseBoolean(process.env.REALTIME_ENABLE_PRESENCE, true),
    enableTyping: parseBoolean(process.env.REALTIME_ENABLE_TYPING, true),
    typingTimeout: parseIntEnv(process.env.REALTIME_TYPING_TIMEOUT, 3000),
    presenceHeartbeat: parseIntEnv(process.env.REALTIME_PRESENCE_HEARTBEAT, 30000),

    // Performance
    enableCompression: parseBoolean(process.env.REALTIME_ENABLE_COMPRESSION, true),
    batchSize: parseIntEnv(process.env.REALTIME_BATCH_SIZE, 100),
    rateLimit: parseIntEnv(process.env.REALTIME_RATE_LIMIT, 100),

    // Logging
    logEvents: parseBoolean(process.env.REALTIME_LOG_EVENTS, true),
    logEventTypes: parseArray(process.env.REALTIME_LOG_EVENT_TYPES, ['connect', 'disconnect', 'error']),
    logLevel: (process.env.LOG_LEVEL || 'info') as RealtimeConfig['logLevel'],

    // Monitoring
    enableMetrics: parseBoolean(process.env.REALTIME_ENABLE_METRICS, true),
    metricsPath: process.env.REALTIME_METRICS_PATH || '/metrics',
    enableHealth: parseBoolean(process.env.REALTIME_ENABLE_HEALTH, true),
    healthPath: process.env.REALTIME_HEALTH_PATH || '/health',
  };
}

/**
 * Global config instance
 */
export const config = loadConfig();
