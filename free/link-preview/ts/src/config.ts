/**
 * Link Preview Plugin Configuration
 */

import 'dotenv/config';
import type { LinkPreviewConfig } from './types.js';

export type { LinkPreviewConfig };

export function loadConfig(overrides?: Partial<LinkPreviewConfig>): LinkPreviewConfig {
  const config: LinkPreviewConfig = {
    // Server
    port: parseInt(process.env.LP_PLUGIN_PORT ?? process.env.PORT ?? '3718', 10),
    host: process.env.LP_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Preview settings
    enabled: process.env.LINK_PREVIEW_ENABLED !== 'false',
    cacheTtlHours: parseInt(process.env.LINK_PREVIEW_CACHE_TTL_HOURS ?? '168', 10),
    timeoutSeconds: parseInt(process.env.LINK_PREVIEW_TIMEOUT_SECONDS ?? '10', 10),
    userAgent: process.env.LINK_PREVIEW_USER_AGENT ?? 'nself-bot/1.0',
    maxPreviewsPerMessage: parseInt(process.env.LINK_PREVIEW_MAX_PER_MESSAGE ?? '3', 10),

    // Fetching
    maxResponseSizeMb: parseInt(process.env.LINK_PREVIEW_MAX_RESPONSE_SIZE_MB ?? '10', 10),
    followRedirects: process.env.LINK_PREVIEW_FOLLOW_REDIRECTS !== 'false',
    maxRedirects: parseInt(process.env.LINK_PREVIEW_MAX_REDIRECTS ?? '5', 10),
    respectRobotsTxt: process.env.LINK_PREVIEW_RESPECT_ROBOTS_TXT !== 'false',

    // oEmbed
    oembedEnabled: process.env.OEMBED_ENABLED !== 'false',
    oembedDiscovery: process.env.OEMBED_DISCOVERY !== 'false',
    oembedMaxWidth: parseInt(process.env.OEMBED_MAX_WIDTH ?? '1024', 10),
    oembedMaxHeight: parseInt(process.env.OEMBED_MAX_HEIGHT ?? '768', 10),

    // Safety
    safetyCheckEnabled: process.env.LINK_PREVIEW_SAFETY_CHECK !== 'false',
    phishingDetection: process.env.LINK_PREVIEW_PHISHING_DETECTION !== 'false',

    // Rate limiting (per domain/per minute)
    rateLimitPerMinute: parseInt(process.env.LINK_PREVIEW_RATE_LIMIT_PER_MINUTE ?? '60', 10),
    rateLimitPerDomain: parseInt(process.env.LINK_PREVIEW_RATE_LIMIT_PER_DOMAIN ?? '10', 10),

    // Security (API-level)
    apiKey: process.env.LP_API_KEY,
    rateLimitMax: parseInt(process.env.LP_RATE_LIMIT_MAX ?? '200', 10),
    rateLimitWindowMs: parseInt(process.env.LP_RATE_LIMIT_WINDOW_MS ?? '60000', 10),

    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword && process.env.NODE_ENV === 'production') {
    throw new Error('POSTGRES_PASSWORD must be set in production');
  }

  if (config.cacheTtlHours < 1) {
    throw new Error('LINK_PREVIEW_CACHE_TTL_HOURS must be at least 1');
  }

  if (config.timeoutSeconds < 1 || config.timeoutSeconds > 60) {
    throw new Error('LINK_PREVIEW_TIMEOUT_SECONDS must be between 1 and 60');
  }

  return config;
}
