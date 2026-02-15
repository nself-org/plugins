/**
 * Entitlements Plugin Configuration
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

  // Entitlements
  defaultCurrency: string;
  defaultTrialDays: number;
  quotaWarningThreshold: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('ENTITLEMENTS');

  const config: Config = {
    // Server
    port: parseInt(process.env.ENTITLEMENTS_PLUGIN_PORT ?? process.env.PORT ?? '3714', 10),
    host: process.env.ENTITLEMENTS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Entitlements
    defaultCurrency: process.env.ENTITLEMENTS_DEFAULT_CURRENCY ?? 'USD',
    defaultTrialDays: parseInt(process.env.ENTITLEMENTS_DEFAULT_TRIAL_DAYS ?? '14', 10),
    quotaWarningThreshold: parseInt(process.env.ENTITLEMENTS_QUOTA_WARNING_THRESHOLD ?? '80', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
