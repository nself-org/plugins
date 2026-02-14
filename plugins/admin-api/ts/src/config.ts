/**
 * Configuration loader for Admin API plugin
 */

import * as dotenv from 'dotenv';
import type { Config } from './types.js';

export type { Config };

dotenv.config();

export function loadConfig(overrides?: Partial<Config>): Config {
  const config: Config = {
    database: {
      host: process.env.POSTGRES_HOST ?? 'localhost',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    },

    server: {
      port: parseInt(process.env.ADMIN_API_PORT ?? '3214', 10),
      host: process.env.HOST ?? '0.0.0.0',
    },

    auth: {
      jwtSecret: process.env.ADMIN_JWT_SECRET ?? 'change-me-in-production',
      sessionTimeoutMinutes: parseInt(process.env.ADMIN_SESSION_TIMEOUT_MINUTES ?? '60', 10),
    },

    metrics: {
      collectionIntervalSeconds: parseInt(process.env.ADMIN_METRICS_COLLECTION_INTERVAL_SECONDS ?? '60', 10),
    },

    security: {
      apiKey: process.env.ADMIN_API_KEY,
      rateLimitMax: parseInt(process.env.ADMIN_RATE_LIMIT_MAX ?? '100', 10),
      rateLimitWindowMs: parseInt(process.env.ADMIN_RATE_LIMIT_WINDOW_MS ?? '60000', 10),
    },
  };

  return overrides ? { ...config, ...overrides } : config;
}

export const config = loadConfig();
