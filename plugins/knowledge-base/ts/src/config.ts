/**
 * Knowledge Base Plugin Configuration
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

  // Knowledge Base
  defaultLanguage: string;
  maxDocumentSize: number;
  semanticSearchEnabled: boolean;
  cacheEnabled: boolean;
  cacheTTL: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('KB');

  const config: Config = {
    // Server
    port: parseInt(process.env.KB_PLUGIN_PORT ?? process.env.PORT ?? '3713', 10),
    host: process.env.KB_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Knowledge Base
    defaultLanguage: process.env.KB_DEFAULT_LANGUAGE ?? 'en',
    maxDocumentSize: parseInt(process.env.KB_MAX_DOCUMENT_SIZE ?? '10485760', 10),
    semanticSearchEnabled: process.env.KB_SEMANTIC_SEARCH_ENABLED === 'true',
    cacheEnabled: process.env.KB_CACHE_ENABLED === 'true',
    cacheTTL: parseInt(process.env.KB_CACHE_TTL ?? '3600', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
