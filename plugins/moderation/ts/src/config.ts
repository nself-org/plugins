/**
 * Moderation Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { ToxicityProvider } from './types.js';

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

  // Toxicity Detection
  toxicityEnabled: boolean;
  toxicityProvider: ToxicityProvider;
  toxicityThreshold: number;

  // Auto-moderation
  autoDeleteEnabled: boolean;
  autoDeleteThreshold: number;
  autoMuteEnabled: boolean;
  autoMuteViolations: number;
  autoBanEnabled: boolean;

  // Appeals
  appealsEnabled: boolean;
  appealsTimeLimitDays: number;

  // Cleanup
  cleanupEnabled: boolean;
  cleanupIntervalMinutes: number;

  // Rate limiting per user
  maxReportsPerUserPerDay: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseToxicityProvider(value: string | undefined): ToxicityProvider {
  const normalized = (value || 'local').toLowerCase();
  if (normalized === 'perspective_api' || normalized === 'openai' || normalized === 'local') {
    return normalized;
  }
  throw new Error(`Invalid toxicity provider: ${value}. Must be 'perspective_api', 'openai', or 'local'`);
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('MODERATION');

  const config: Config = {
    // Server
    port: parseInt(process.env.MODERATION_PLUGIN_PORT ?? process.env.PORT ?? '3704', 10),
    host: process.env.MODERATION_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Toxicity Detection
    toxicityEnabled: process.env.MODERATION_TOXICITY_ENABLED === 'true',
    toxicityProvider: parseToxicityProvider(process.env.MODERATION_TOXICITY_PROVIDER),
    toxicityThreshold: parseFloat(process.env.MODERATION_TOXICITY_THRESHOLD ?? '0.8'),

    // Auto-moderation
    autoDeleteEnabled: process.env.MODERATION_AUTO_DELETE_ENABLED !== 'false',
    autoDeleteThreshold: parseFloat(process.env.MODERATION_AUTO_DELETE_THRESHOLD ?? '0.95'),
    autoMuteEnabled: process.env.MODERATION_AUTO_MUTE_ENABLED === 'true',
    autoMuteViolations: parseInt(process.env.MODERATION_AUTO_MUTE_VIOLATIONS ?? '3', 10),
    autoBanEnabled: process.env.MODERATION_AUTO_BAN_ENABLED === 'true',

    // Appeals
    appealsEnabled: process.env.MODERATION_APPEALS_ENABLED !== 'false',
    appealsTimeLimitDays: parseInt(process.env.MODERATION_APPEALS_TIME_LIMIT_DAYS ?? '7', 10),

    // Cleanup
    cleanupEnabled: process.env.MODERATION_CLEANUP_ENABLED !== 'false',
    cleanupIntervalMinutes: parseInt(process.env.MODERATION_CLEANUP_INTERVAL_MINUTES ?? '60', 10),

    // Rate limiting
    maxReportsPerUserPerDay: parseInt(process.env.MODERATION_MAX_REPORTS_PER_USER_PER_DAY ?? '10', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.toxicityThreshold < 0 || config.toxicityThreshold > 1) {
    throw new Error('MODERATION_TOXICITY_THRESHOLD must be between 0 and 1');
  }

  if (config.autoDeleteThreshold < 0 || config.autoDeleteThreshold > 1) {
    throw new Error('MODERATION_AUTO_DELETE_THRESHOLD must be between 0 and 1');
  }

  if (config.autoMuteViolations < 1) {
    throw new Error('MODERATION_AUTO_MUTE_VIOLATIONS must be at least 1');
  }

  if (config.appealsTimeLimitDays < 1) {
    throw new Error('MODERATION_APPEALS_TIME_LIMIT_DAYS must be at least 1');
  }

  return config;
}
