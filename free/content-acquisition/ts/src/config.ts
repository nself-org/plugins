/**
 * Content Acquisition Configuration
 */

import { config as dotenvConfig } from 'dotenv';
import { ContentAcquisitionConfig } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-acquisition:config');

dotenvConfig();

/**
 * Validate URL format
 */
function validateUrl(url: string, varName: string): void {
  try {
    new URL(url);
  } catch (error) {
    throw new Error(`${varName} must be a valid URL (got: ${url})`);
  }
}

/**
 * Validate port range
 */
function validatePort(port: number, varName: string): void {
  if (isNaN(port) || port < 1 || port > 65535) {
    throw new Error(`${varName} must be a valid port number between 1 and 65535 (got: ${port})`);
  }
}

/**
 * Validate positive integer
 */
function validatePositiveInt(value: number, varName: string): void {
  if (isNaN(value) || value < 1) {
    throw new Error(`${varName} must be a positive integer (got: ${value})`);
  }
}

export function loadConfig(): ContentAcquisitionConfig {
  // Required: DATABASE_URL
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error('DATABASE_URL environment variable is required');
  }
  if (!databaseUrl.startsWith('postgresql://') && !databaseUrl.startsWith('postgres://')) {
    throw new Error('DATABASE_URL must be a PostgreSQL connection string starting with postgresql:// or postgres://');
  }

  // Required: METADATA_ENRICHMENT_URL
  const metadataEnrichmentUrl = process.env.METADATA_ENRICHMENT_URL;
  if (!metadataEnrichmentUrl) {
    throw new Error('METADATA_ENRICHMENT_URL environment variable is required');
  }
  validateUrl(metadataEnrichmentUrl, 'METADATA_ENRICHMENT_URL');

  // Required: TORRENT_MANAGER_URL
  const torrentManagerUrl = process.env.TORRENT_MANAGER_URL;
  if (!torrentManagerUrl) {
    throw new Error('TORRENT_MANAGER_URL environment variable is required');
  }
  validateUrl(torrentManagerUrl, 'TORRENT_MANAGER_URL');

  // Required: VPN_MANAGER_URL
  const vpnManagerUrl = process.env.VPN_MANAGER_URL;
  if (!vpnManagerUrl) {
    throw new Error('VPN_MANAGER_URL environment variable is required');
  }
  validateUrl(vpnManagerUrl, 'VPN_MANAGER_URL');

  // Parse and validate port
  const port = parseInt(process.env.CONTENT_ACQUISITION_PORT || '3202', 10);
  validatePort(port, 'CONTENT_ACQUISITION_PORT');

  // Parse and validate Redis port
  const redisPort = parseInt(process.env.REDIS_PORT || '6379', 10);
  validatePort(redisPort, 'REDIS_PORT');

  // Parse and validate RSS check interval
  const rssCheckInterval = parseInt(process.env.RSS_CHECK_INTERVAL || '30', 10);
  validatePositiveInt(rssCheckInterval, 'RSS_CHECK_INTERVAL');

  // Validate optional URLs
  const subtitleManagerUrl = process.env.SUBTITLE_MANAGER_URL || 'http://plugin-subtitle-manager:3204';
  validateUrl(subtitleManagerUrl, 'SUBTITLE_MANAGER_URL');

  const mediaProcessingUrl = process.env.MEDIA_PROCESSING_URL || 'http://plugin-media-processing:3019';
  validateUrl(mediaProcessingUrl, 'MEDIA_PROCESSING_URL');

  const ntvBackendUrl = process.env.NTV_BACKEND_URL || 'http://auth:4000';
  validateUrl(ntvBackendUrl, 'NTV_BACKEND_URL');

  // Validate log level
  const logLevel = process.env.LOG_LEVEL || 'info';
  const validLogLevels = ['debug', 'info', 'warn', 'error'];
  if (!validLogLevels.includes(logLevel)) {
    throw new Error(`LOG_LEVEL must be one of: ${validLogLevels.join(', ')} (got: ${logLevel})`);
  }

  const config: ContentAcquisitionConfig = {
    database_url: databaseUrl,
    port,
    metadata_enrichment_url: metadataEnrichmentUrl,
    torrent_manager_url: torrentManagerUrl,
    vpn_manager_url: vpnManagerUrl,
    subtitle_manager_url: subtitleManagerUrl,
    media_processing_url: mediaProcessingUrl,
    ntv_backend_url: ntvBackendUrl,
    redis_host: process.env.REDIS_HOST || 'redis',
    redis_port: redisPort,
    log_level: logLevel,
    rss_check_interval: rssCheckInterval,
  };

  logger.info('Configuration loaded and validated', { port: config.port });

  return config;
}

export const config = loadConfig();
