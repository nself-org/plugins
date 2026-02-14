/**
 * Discovery Plugin Configuration
 */

import { config as dotenvConfig } from 'dotenv';
import { DiscoveryConfig } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('discovery:config');

// Load environment variables
dotenvConfig();

/**
 * Load and validate plugin configuration from environment variables
 */
export function loadConfig(): DiscoveryConfig {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error('DATABASE_URL environment variable is required');
  }

  const config: DiscoveryConfig = {
    database_url: databaseUrl,
    redis_url: process.env.REDIS_URL || 'redis://localhost:6379',
    port: parseInt(process.env.DISCOVERY_PORT || '3022', 10),
    trending_window_hours: parseInt(process.env.TRENDING_WINDOW_HOURS || '24', 10),
    default_limit: parseInt(process.env.DEFAULT_LIMIT || '20', 10),
    cache_ttl_trending: parseInt(process.env.CACHE_TTL_TRENDING || '900', 10),
    cache_ttl_popular: parseInt(process.env.CACHE_TTL_POPULAR || '3600', 10),
    cache_ttl_recent: parseInt(process.env.CACHE_TTL_RECENT || '1800', 10),
    cache_ttl_continue: parseInt(process.env.CACHE_TTL_CONTINUE || '300', 10),
    log_level: (process.env.LOG_LEVEL as DiscoveryConfig['log_level']) || 'info',
  };

  logger.info('Configuration loaded', {
    port: config.port,
    trending_window_hours: config.trending_window_hours,
    default_limit: config.default_limit,
    redis_url: config.redis_url.replace(/\/\/.*@/, '//***@'),
  });

  return config;
}

/**
 * Validate configuration values
 */
export function validateConfig(config: DiscoveryConfig): boolean {
  const errors: string[] = [];

  if (!config.database_url) {
    errors.push('database_url is required');
  }

  if (config.port < 1024 || config.port > 65535) {
    errors.push('port must be between 1024 and 65535');
  }

  if (config.trending_window_hours < 1 || config.trending_window_hours > 720) {
    errors.push('trending_window_hours must be between 1 and 720');
  }

  if (config.default_limit < 1 || config.default_limit > 100) {
    errors.push('default_limit must be between 1 and 100');
  }

  if (config.cache_ttl_trending < 0) {
    errors.push('cache_ttl_trending must be non-negative');
  }

  if (config.cache_ttl_popular < 0) {
    errors.push('cache_ttl_popular must be non-negative');
  }

  if (config.cache_ttl_recent < 0) {
    errors.push('cache_ttl_recent must be non-negative');
  }

  if (config.cache_ttl_continue < 0) {
    errors.push('cache_ttl_continue must be non-negative');
  }

  if (errors.length > 0) {
    logger.error('Configuration validation failed', { errors });
    return false;
  }

  return true;
}

// Export singleton config instance
export const config = loadConfig();

if (!validateConfig(config)) {
  throw new Error('Invalid configuration');
}
