/**
 * Content Moderation Plugin Configuration
 * Environment variable loading and validation
 */

import dotenv from 'dotenv';
import { ContentModerationConfig } from './types.js';
import { parseCsvList, normalizeSourceAccountId } from '@nself/plugin-utils';

// Load environment variables
dotenv.config();

/**
 * Get optional environment variable
 */
function getEnvOptional(key: string, defaultValue: string = ''): string {
  return process.env[key] || defaultValue;
}

/**
 * Get integer environment variable
 */
function getEnvInt(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseInt(value, 10);
  if (isNaN(parsed)) {
    throw new Error(`Invalid integer value for ${key}: ${value}`);
  }
  return parsed;
}

/**
 * Get float environment variable
 */
function getEnvFloat(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseFloat(value);
  if (isNaN(parsed)) {
    throw new Error(`Invalid float value for ${key}: ${value}`);
  }
  return parsed;
}

/**
 * Get boolean environment variable
 */
function getEnvBool(key: string, defaultValue: boolean): boolean {
  const value = process.env[key];
  if (!value) return defaultValue;
  return value.toLowerCase() === 'true' || value === '1';
}

/**
 * Load and validate configuration
 */
export function loadConfig(): ContentModerationConfig {
  const appIdsRaw = getEnvOptional('MOD_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const config: ContentModerationConfig = {
    port: getEnvInt('MOD_PLUGIN_PORT', 3028),
    host: getEnvOptional('MOD_PLUGIN_HOST', '0.0.0.0'),
    logLevel: getEnvOptional('MOD_LOG_LEVEL', 'info') as 'debug' | 'info' | 'warn' | 'error',

    database: {
      host: getEnvOptional('POSTGRES_HOST', 'localhost'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    appIds,

    // Provider
    provider: getEnvOptional('MOD_PROVIDER', 'none'),
    openaiApiKey: getEnvOptional('MOD_OPENAI_API_KEY', ''),
    googleVisionKey: getEnvOptional('MOD_GOOGLE_VISION_KEY', ''),
    awsRekognitionKey: getEnvOptional('MOD_AWS_REKOGNITION_KEY', ''),
    awsRekognitionSecret: getEnvOptional('MOD_AWS_REKOGNITION_SECRET', ''),
    awsRekognitionRegion: getEnvOptional('MOD_AWS_REKOGNITION_REGION', 'us-east-1'),

    // Thresholds
    autoApproveBelow: getEnvFloat('MOD_AUTO_APPROVE_BELOW', 0.1),
    autoRejectAbove: getEnvFloat('MOD_AUTO_REJECT_ABOVE', 0.9),
    flagThreshold: getEnvFloat('MOD_FLAG_THRESHOLD', 0.5),

    // Strikes
    strikeWarnThreshold: getEnvInt('MOD_STRIKE_WARN_THRESHOLD', 3),
    strikeBanThreshold: getEnvInt('MOD_STRIKE_BAN_THRESHOLD', 5),
    strikeExpiryDays: getEnvInt('MOD_STRIKE_EXPIRY_DAYS', 90),

    // Queue
    reviewSlaHours: getEnvInt('MOD_REVIEW_SLA_HOURS', 24),
    queueWorkerConcurrency: getEnvInt('MOD_QUEUE_WORKER_CONCURRENCY', 5),
  };

  return config;
}

// Export singleton config instance
export const config = loadConfig();
