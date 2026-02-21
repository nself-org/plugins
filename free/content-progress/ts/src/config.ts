/**
 * Content Progress Plugin Configuration
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

  // Progress settings
  completeThreshold: number;
  historySampleSeconds: number;
  historyRetentionDays: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseDatabaseUrl(url: string | undefined): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} | null {
  if (!url) {
    return null;
  }

  try {
    // Format: postgresql://user:password@host:port/database?sslmode=require
    const match = url.match(/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/);
    if (!match) {
      return null;
    }

    const [, user, password, host, port, database, queryString] = match;
    const ssl = queryString?.includes('sslmode=require') || queryString?.includes('ssl=true') || false;

    return {
      host,
      port: parseInt(port, 10),
      database,
      user,
      password,
      ssl,
    };
  } catch {
    return null;
  }
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('PROGRESS');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.PROGRESS_PLUGIN_PORT ?? process.env.PORT ?? '3022', 10),
    host: process.env.PROGRESS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // Progress settings
    completeThreshold: parseInt(process.env.PROGRESS_COMPLETE_THRESHOLD ?? '95', 10),
    historySampleSeconds: parseInt(process.env.PROGRESS_HISTORY_SAMPLE_SECONDS ?? '30', 10),
    historyRetentionDays: parseInt(process.env.PROGRESS_HISTORY_RETENTION_DAYS ?? '365', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.completeThreshold < 1 || config.completeThreshold > 100) {
    throw new Error('PROGRESS_COMPLETE_THRESHOLD must be between 1 and 100');
  }

  if (config.historySampleSeconds < 1) {
    throw new Error('PROGRESS_HISTORY_SAMPLE_SECONDS must be at least 1');
  }

  if (config.historyRetentionDays < 1) {
    throw new Error('PROGRESS_HISTORY_RETENTION_DAYS must be at least 1');
  }

  return config;
}
