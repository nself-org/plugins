import { config as dotenvConfig } from 'dotenv';
import { MetadataEnrichmentConfig } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('metadata-enrichment:config');
dotenvConfig();

export function loadConfig(): MetadataEnrichmentConfig {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) throw new Error('DATABASE_URL is required');

  const tmdbApiKey = process.env.TMDB_API_KEY;
  if (!tmdbApiKey) throw new Error('TMDB_API_KEY is required');

  const config: MetadataEnrichmentConfig = {
    database_url: databaseUrl,
    port: parseInt(process.env.METADATA_ENRICHMENT_PORT || '3203', 10),
    tmdb_api_key: tmdbApiKey,
    tvdb_api_key: process.env.TVDB_API_KEY,
    musicbrainz_user_agent: process.env.MUSICBRAINZ_USER_AGENT || 'nself-tv/1.0.0',
    object_storage_url: process.env.OBJECT_STORAGE_URL,
    log_level: process.env.LOG_LEVEL || 'info',
  };

  logger.info('Configuration loaded', { port: config.port });
  return config;
}

export const config = loadConfig();
