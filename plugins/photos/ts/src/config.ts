/**
 * Photos Plugin Configuration
 * Environment variable loading and validation
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import { parseCsvList, normalizeSourceAccountId } from '@nself/plugin-utils';
import type { PhotosConfig } from './types.js';

function getEnvOptional(key: string, defaultValue: string = ''): string {
  return process.env[key] || defaultValue;
}

function getEnvInt(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseInt(value, 10);
  if (isNaN(parsed)) {
    throw new Error(`Invalid integer value for ${key}: ${value}`);
  }
  return parsed;
}

function getEnvBool(key: string, defaultValue: boolean): boolean {
  const value = process.env[key];
  if (!value) return defaultValue;
  return value.toLowerCase() === 'true' || value === '1';
}

export function loadConfig(): PhotosConfig {
  const appIdsRaw = getEnvOptional('PHOTOS_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const security: SecurityConfig = loadSecurityConfig('PHOTOS');

  return {
    port: getEnvInt('PHOTOS_PLUGIN_PORT', 3023),
    host: getEnvOptional('PHOTOS_PLUGIN_HOST', '0.0.0.0'),
    logLevel: getEnvOptional('PHOTOS_LOG_LEVEL', 'info') as PhotosConfig['logLevel'],

    database: {
      host: getEnvOptional('POSTGRES_HOST', 'postgres'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    appIds,

    // Thumbnails
    thumbnailSmall: getEnvInt('PHOTOS_THUMBNAIL_SMALL', 150),
    thumbnailMedium: getEnvInt('PHOTOS_THUMBNAIL_MEDIUM', 600),
    thumbnailLarge: getEnvInt('PHOTOS_THUMBNAIL_LARGE', 1200),
    thumbnailQuality: getEnvInt('PHOTOS_THUMBNAIL_QUALITY', 85),
    thumbnailFormat: getEnvOptional('PHOTOS_THUMBNAIL_FORMAT', 'webp'),

    // Processing
    exifExtraction: getEnvBool('PHOTOS_EXIF_EXTRACTION', true),
    processingConcurrency: getEnvInt('PHOTOS_PROCESSING_CONCURRENCY', 4),

    // Search
    searchEnabled: getEnvBool('PHOTOS_SEARCH_ENABLED', true),
    maxUploadBatch: getEnvInt('PHOTOS_MAX_UPLOAD_BATCH', 100),

    security,
  };
}

export const config = loadConfig();
