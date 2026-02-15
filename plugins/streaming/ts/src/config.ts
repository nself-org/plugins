/**
 * Configuration loader for Streaming plugin
 */

import * as dotenv from 'dotenv';
import { StreamingConfig } from './types.js';

// Load environment variables
dotenv.config();

export function loadConfig(): StreamingConfig {
  return {
    database: {
      host: process.env.POSTGRES_HOST ?? 'postgres',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    },

    server: {
      port: parseInt(process.env.STREAMING_PLUGIN_PORT ?? process.env.PORT ?? '3711', 10),
      host: process.env.HOST ?? '0.0.0.0',
    },

    rtmp: {
      port: parseInt(process.env.RTMP_PORT ?? '1935', 10),
    },

    recording: {
      enabled: process.env.STREAMING_RECORDING_ENABLED !== 'false',
      s3_bucket: process.env.S3_BUCKET ?? 'nself-streams',
      s3_region: process.env.S3_REGION ?? 'us-east-1',
    },

    dvr: {
      enabled: process.env.STREAMING_DVR_ENABLED !== 'false',
      window_seconds: parseInt(process.env.STREAMING_DVR_WINDOW ?? '7200', 10),
    },

    chat: {
      rate_limit_messages: parseInt(process.env.STREAMING_CHAT_RATE_LIMIT ?? '5', 10),
      rate_limit_window: parseInt(process.env.STREAMING_CHAT_RATE_WINDOW ?? '10', 10),
    },

    analytics: {
      bucket_interval_minutes: parseInt(process.env.STREAMING_ANALYTICS_INTERVAL ?? '5', 10),
      retention_days: parseInt(process.env.STREAMING_ANALYTICS_RETENTION ?? '90', 10),
    },

    cdn: {
      url: process.env.CDN_URL ?? '',
    },
  };
}

export const config = loadConfig();
