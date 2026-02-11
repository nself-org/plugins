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
    redis_host: process.env.REDIS_HOST || 'localhost',
    redis_port: parseInt(process.env.REDIS_PORT || '6379', 10),
    log_level: process.env.LOG_LEVEL || 'info',
  };

  logger.info('Configuration loaded', { port: config.port });

  return config;
}

export const config = loadConfig();
