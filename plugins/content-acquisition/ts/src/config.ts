/**
 * Content Acquisition Configuration
 */

import { config as dotenvConfig } from 'dotenv';
import { ContentAcquisitionConfig } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('content-acquisition:config');

dotenvConfig();

export function loadConfig(): ContentAcquisitionConfig {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error('DATABASE_URL environment variable is required');
  }

  const metadataEnrichmentUrl = process.env.METADATA_ENRICHMENT_URL;
  if (!metadataEnrichmentUrl) {
    throw new Error('METADATA_ENRICHMENT_URL environment variable is required');
  }

  const torrentManagerUrl = process.env.TORRENT_MANAGER_URL;
  if (!torrentManagerUrl) {
    throw new Error('TORRENT_MANAGER_URL environment variable is required');
  }

  const vpnManagerUrl = process.env.VPN_MANAGER_URL;
  if (!vpnManagerUrl) {
    throw new Error('VPN_MANAGER_URL environment variable is required');
  }

  const config: ContentAcquisitionConfig = {
    database_url: databaseUrl,
    port: parseInt(process.env.CONTENT_ACQUISITION_PORT || '3202', 10),
    metadata_enrichment_url: metadataEnrichmentUrl,
    torrent_manager_url: torrentManagerUrl,
    vpn_manager_url: vpnManagerUrl,
    subtitle_manager_url: process.env.SUBTITLE_MANAGER_URL || 'http://plugin-subtitle-manager:3204',
    media_processing_url: process.env.MEDIA_PROCESSING_URL || 'http://plugin-media-processing:3019',
    ntv_backend_url: process.env.NTV_BACKEND_URL || 'http://auth:4000',
    redis_host: process.env.REDIS_HOST || 'redis',
    redis_port: parseInt(process.env.REDIS_PORT || '6379', 10),
    log_level: process.env.LOG_LEVEL || 'info',
    rss_check_interval: parseInt(process.env.RSS_CHECK_INTERVAL || '30', 10),
  };

  logger.info('Configuration loaded', { port: config.port });

  return config;
}

export const config = loadConfig();
