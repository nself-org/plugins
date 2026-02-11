/**
 * Access Controls Plugin Configuration
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

  // ACL Settings
  cacheTtlSeconds: number;
  maxRoleDepth: number;
  defaultDeny: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('ACL');

  const config: Config = {
    // Server
    port: parseInt(process.env.ACL_PLUGIN_PORT ?? process.env.PORT ?? '3027', 10),
    host: process.env.ACL_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // ACL Settings
    cacheTtlSeconds: parseInt(process.env.ACL_CACHE_TTL_SECONDS ?? '300', 10),
    maxRoleDepth: parseInt(process.env.ACL_MAX_ROLE_DEPTH ?? '10', 10),
    defaultDeny: process.env.ACL_DEFAULT_DENY !== 'false',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword && process.env.NODE_ENV === 'production') {
    throw new Error('Database password is required in production');
  }

  if (config.cacheTtlSeconds < 0) {
    throw new Error('ACL_CACHE_TTL_SECONDS must be >= 0');
  }

  if (config.maxRoleDepth < 1 || config.maxRoleDepth > 50) {
    throw new Error('ACL_MAX_ROLE_DEPTH must be between 1 and 50');
  }

  return config;
}
