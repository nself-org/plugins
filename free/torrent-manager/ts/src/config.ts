/**
 * Torrent Manager Configuration
 */

import { config as dotenvConfig } from 'dotenv';
import { TorrentManagerConfig, TorrentClientType } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('torrent-manager:config');

// Load environment variables
dotenvConfig();

/**
 * Load and validate plugin configuration
 */
export function loadConfig(): TorrentManagerConfig {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error('DATABASE_URL environment variable is required');
  }

  const vpnManagerUrl = process.env.VPN_MANAGER_URL;
  if (!vpnManagerUrl) {
    throw new Error('VPN_MANAGER_URL environment variable is required');
  }

  const config: TorrentManagerConfig = {
    database_url: databaseUrl,
    port: parseInt(process.env.TORRENT_MANAGER_PORT || '3201', 10),
    vpn_manager_url: vpnManagerUrl,
    vpn_required: process.env.VPN_REQUIRED !== 'false',

    // Default Client
    default_client: (process.env.DEFAULT_TORRENT_CLIENT as TorrentClientType) || 'transmission',

    // Transmission
    transmission_host: process.env.TRANSMISSION_HOST || 'localhost',
    transmission_port: parseInt(process.env.TRANSMISSION_PORT || '9091', 10),
    transmission_username: process.env.TRANSMISSION_USERNAME,
    transmission_password: process.env.TRANSMISSION_PASSWORD,

    // qBittorrent
    qbittorrent_host: process.env.QBITTORRENT_HOST || 'localhost',
    qbittorrent_port: parseInt(process.env.QBITTORRENT_PORT || '8080', 10),
    qbittorrent_username: process.env.QBITTORRENT_USERNAME,
    qbittorrent_password: process.env.QBITTORRENT_PASSWORD,

    // Download Settings
    download_path: process.env.DOWNLOAD_PATH || '/downloads',

    // Search Settings
    enabled_sources: process.env.ENABLED_SOURCES || '1337x,yts,torrentgalaxy,tpb',
    search_enabled_sources: (process.env.SEARCH_ENABLED_SOURCES || '1337x,yts,torrentgalaxy,tpb').split(','),
    search_timeout_ms: parseInt(process.env.SEARCH_TIMEOUT_MS || '10000', 10),
    search_cache_ttl_seconds: parseInt(process.env.SEARCH_CACHE_TTL_SECONDS || '3600', 10),

    // Seeding Policy
    seeding_ratio_limit: parseFloat(process.env.SEEDING_RATIO_LIMIT || '2.0'),
    seeding_time_limit_hours: parseInt(process.env.SEEDING_TIME_LIMIT_HOURS || '168', 10),

    // Concurrency
    max_active_downloads: parseInt(process.env.MAX_ACTIVE_DOWNLOADS || '5', 10),

    log_level: process.env.LOG_LEVEL || 'info',
  };

  logger.info('Configuration loaded', {
    port: config.port,
    default_client: config.default_client,
    vpn_required: config.vpn_required,
    max_downloads: config.max_active_downloads,
  });

  return config;
}

/**
 * Validate configuration
 */
export function validateConfig(config: TorrentManagerConfig): boolean {
  const errors: string[] = [];

  if (!config.database_url) {
    errors.push('database_url is required');
  }

  if (!config.vpn_manager_url) {
    errors.push('vpn_manager_url is required');
  }

  if (config.port < 1024 || config.port > 65535) {
    errors.push('port must be between 1024 and 65535');
  }

  if (config.max_active_downloads < 1) {
    errors.push('max_active_downloads must be at least 1');
  }

  if (config.seeding_ratio_limit < 0) {
    errors.push('seeding_ratio_limit must be non-negative');
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
