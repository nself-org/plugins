/**
 * VPN Plugin Configuration
 */

import { config as dotenvConfig } from 'dotenv';
import { VPNPluginConfig, VPNProvider } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('vpn:config');

// Load environment variables
dotenvConfig();

/**
 * Load and validate plugin configuration
 */
export function loadConfig(): VPNPluginConfig {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error('DATABASE_URL environment variable is required');
  }

  const defaultProvider = (process.env.VPN_PROVIDER as VPNProvider) || undefined;
  const validProviders = [
    'nordvpn',
    'pia',
    'mullvad',
  ];

  if (defaultProvider && !validProviders.includes(defaultProvider)) {
    logger.warn(`Invalid VPN_PROVIDER: ${defaultProvider}. Must be one of: ${validProviders.join(', ')}`);
  }

  const config: VPNPluginConfig = {
    database_url: databaseUrl,
    default_provider: defaultProvider,
    default_region: process.env.VPN_REGION,
    download_path: process.env.DOWNLOAD_PATH || '/tmp/vpn-downloads',
    enable_kill_switch: process.env.ENABLE_KILL_SWITCH !== 'false',
    enable_auto_reconnect: process.env.ENABLE_AUTO_RECONNECT !== 'false',
    server_carousel_enabled: process.env.SERVER_CAROUSEL_ENABLED === 'true',
    carousel_interval_minutes: parseInt(process.env.CAROUSEL_INTERVAL_MINUTES || '60', 10),
    port: parseInt(process.env.PORT || '3200', 10),
    log_level: (process.env.LOG_LEVEL as 'debug' | 'info' | 'warn' | 'error') || 'info',
    torrent_manager_url: process.env.TORRENT_MANAGER_URL || 'http://localhost:3210',
    internal_api_key: process.env.INTERNAL_API_KEY,
  };

  logger.info('Configuration loaded', {
    port: config.port,
    default_provider: config.default_provider || 'none',
    kill_switch: config.enable_kill_switch,
    auto_reconnect: config.enable_auto_reconnect,
    carousel: config.server_carousel_enabled,
  });

  return config;
}

/**
 * Validate configuration
 */
export function validateConfig(config: VPNPluginConfig): boolean {
  const errors: string[] = [];

  if (!config.database_url) {
    errors.push('database_url is required');
  }

  if (config.port < 1024 || config.port > 65535) {
    errors.push('port must be between 1024 and 65535');
  }

  if (config.carousel_interval_minutes < 1) {
    errors.push('carousel_interval_minutes must be at least 1');
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
