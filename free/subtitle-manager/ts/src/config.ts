import { config as dotenvConfig } from 'dotenv';
import { SubtitleManagerConfig } from './types.js';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('subtitle-manager:config');
dotenvConfig();

export function loadConfig(): SubtitleManagerConfig {
  const databaseUrl = process.env.DATABASE_URL;
  if (!databaseUrl) throw new Error('DATABASE_URL is required');

  const config: SubtitleManagerConfig = {
    database_url: databaseUrl,
    port: parseInt(process.env.SUBTITLE_MANAGER_PORT || '3204', 10),
    opensubtitles_api_key: process.env.OPENSUBTITLES_API_KEY,
    subtitle_storage_path: process.env.SUBTITLE_STORAGE_PATH || '/tmp/subtitles',
    log_level: process.env.LOG_LEVEL || 'info',
    alass_path: process.env.ALASS_PATH || 'alass',
    ffsubsync_path: process.env.FFSUBSYNC_PATH || 'ffsubsync',
  };

  logger.info('Configuration loaded', { port: config.port });
  return config;
}

export const config = loadConfig();
