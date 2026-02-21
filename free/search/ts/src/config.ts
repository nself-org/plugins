/**
 * Search Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { SearchEngine } from './types.js';

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

  // Search Engine
  engine: SearchEngine;
  meilisearchUrl?: string;
  meilisearchApiKey?: string;

  // Search Settings
  defaultLimit: number;
  maxResults: number;
  reindexBatchSize: number;

  // Analytics
  analyticsEnabled: boolean;
  analyticsRetentionDays: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseSearchEngine(value: string | undefined): SearchEngine {
  const normalized = (value || 'postgres').toLowerCase();
  if (normalized === 'postgres' || normalized === 'postgresql') {
    return 'postgres';
  }
  if (normalized === 'meilisearch' || normalized === 'meili') {
    return 'meilisearch';
  }
  throw new Error(`Invalid search engine: ${value}. Must be 'postgres' or 'meilisearch'`);
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('SEARCH');
  const engine = parseSearchEngine(process.env.SEARCH_ENGINE);

  const config: Config = {
    // Server
    port: parseInt(process.env.SEARCH_PLUGIN_PORT ?? process.env.PORT ?? '3302', 10),
    host: process.env.SEARCH_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Search Engine
    engine,
    meilisearchUrl: process.env.SEARCH_MEILISEARCH_URL,
    meilisearchApiKey: process.env.SEARCH_MEILISEARCH_API_KEY,

    // Search Settings
    defaultLimit: parseInt(process.env.SEARCH_DEFAULT_LIMIT ?? '20', 10),
    maxResults: parseInt(process.env.SEARCH_MAX_RESULTS ?? '1000', 10),
    reindexBatchSize: parseInt(process.env.SEARCH_REINDEX_BATCH_SIZE ?? '500', 10),

    // Analytics
    analyticsEnabled: process.env.SEARCH_ANALYTICS_ENABLED !== 'false',
    analyticsRetentionDays: parseInt(process.env.SEARCH_ANALYTICS_RETENTION_DAYS ?? '90', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.engine === 'meilisearch') {
    if (!config.meilisearchUrl) {
      throw new Error('SEARCH_MEILISEARCH_URL is required when using MeiliSearch engine');
    }
    if (!config.meilisearchApiKey) {
      throw new Error('SEARCH_MEILISEARCH_API_KEY is required when using MeiliSearch engine');
    }
  }

  if (config.defaultLimit > config.maxResults) {
    throw new Error('SEARCH_DEFAULT_LIMIT cannot exceed SEARCH_MAX_RESULTS');
  }

  if (config.reindexBatchSize < 1 || config.reindexBatchSize > 10000) {
    throw new Error('SEARCH_REINDEX_BATCH_SIZE must be between 1 and 10000');
  }

  if (config.analyticsRetentionDays < 1) {
    throw new Error('SEARCH_ANALYTICS_RETENTION_DAYS must be at least 1');
  }

  return config;
}
