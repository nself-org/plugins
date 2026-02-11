/**
 * Content Policy Plugin Configuration
 */

import 'dotenv/config';
import type { ContentPolicyConfig } from './types.js';

export type { ContentPolicyConfig };

export function loadConfig(overrides?: Partial<ContentPolicyConfig>): ContentPolicyConfig {
  const config: ContentPolicyConfig = {
    // Server
    port: parseInt(process.env.CP_PLUGIN_PORT ?? process.env.PORT ?? '3504', 10),
    host: process.env.CP_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Policy defaults
    defaultAction: (process.env.CP_DEFAULT_ACTION as 'allow' | 'deny' | 'flag' | 'quarantine') ?? 'flag',
    profanityEnabled: process.env.CP_PROFANITY_ENABLED !== 'false',
    maxContentLength: parseInt(process.env.CP_MAX_CONTENT_LENGTH ?? '100000', 10),
    evaluationLogEnabled: process.env.CP_EVALUATION_LOG_ENABLED !== 'false',

    // Security
    apiKey: process.env.CP_API_KEY,
    rateLimitMax: parseInt(process.env.CP_RATE_LIMIT_MAX ?? '200', 10),
    rateLimitWindowMs: parseInt(process.env.CP_RATE_LIMIT_WINDOW_MS ?? '60000', 10),

    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword && process.env.NODE_ENV === 'production') {
    throw new Error('POSTGRES_PASSWORD must be set in production');
  }

  if (!['allow', 'deny', 'flag', 'quarantine'].includes(config.defaultAction)) {
    throw new Error('CP_DEFAULT_ACTION must be one of: allow, deny, flag, quarantine');
  }

  if (config.maxContentLength < 1 || config.maxContentLength > 1000000) {
    throw new Error('CP_MAX_CONTENT_LENGTH must be between 1 and 1000000');
  }

  return config;
}
