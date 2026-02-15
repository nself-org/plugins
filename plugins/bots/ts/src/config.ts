/**
 * Bots Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Webhook Delivery
  webhookTimeout: number;
  webhookRetryCount: number;
  webhookRetryDelay: number;

  // Rate Limiting
  defaultRateLimitPerMinute: number;
  defaultRateLimitPerHour: number;
  defaultRateLimitPerDay: number;

  // OAuth
  oauthEnabled: boolean;
  oauthCallbackUrl: string;

  // Marketplace
  marketplaceEnabled: boolean;
  marketplaceModeration: boolean;

  // Security
  tokenExpiryDays: number;
  webhookSignatureAlgorithm: string;
  maxMessageSize: number;

  // Performance
  eventQueueSize: number;
  eventWorkerCount: number;
  commandTimeout: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('BOTS');

  const config: Config = {
    // Server
    port: parseInt(process.env.BOTS_PLUGIN_PORT ?? process.env.PORT ?? '3708', 10),
    host: process.env.BOTS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Webhook Delivery
    webhookTimeout: parseInt(process.env.BOT_WEBHOOK_TIMEOUT ?? '30', 10),
    webhookRetryCount: parseInt(process.env.BOT_WEBHOOK_RETRY_COUNT ?? '3', 10),
    webhookRetryDelay: parseInt(process.env.BOT_WEBHOOK_RETRY_DELAY ?? '5', 10),

    // Rate Limiting
    defaultRateLimitPerMinute: parseInt(process.env.BOT_DEFAULT_RATE_LIMIT_PER_MINUTE ?? '60', 10),
    defaultRateLimitPerHour: parseInt(process.env.BOT_DEFAULT_RATE_LIMIT_PER_HOUR ?? '1000', 10),
    defaultRateLimitPerDay: parseInt(process.env.BOT_DEFAULT_RATE_LIMIT_PER_DAY ?? '10000', 10),

    // OAuth
    oauthEnabled: process.env.BOT_OAUTH_ENABLED === 'true',
    oauthCallbackUrl: process.env.BOT_OAUTH_CALLBACK_URL ?? '',

    // Marketplace
    marketplaceEnabled: process.env.BOT_MARKETPLACE_ENABLED !== 'false',
    marketplaceModeration: process.env.BOT_MARKETPLACE_MODERATION !== 'false',

    // Security
    tokenExpiryDays: parseInt(process.env.BOT_TOKEN_EXPIRY_DAYS ?? '365', 10),
    webhookSignatureAlgorithm: process.env.BOT_WEBHOOK_SIGNATURE_ALGORITHM ?? 'sha256',
    maxMessageSize: parseInt(process.env.BOT_MAX_MESSAGE_SIZE ?? '10000', 10),

    // Performance
    eventQueueSize: parseInt(process.env.BOT_EVENT_QUEUE_SIZE ?? '10000', 10),
    eventWorkerCount: parseInt(process.env.BOT_EVENT_WORKER_COUNT ?? '5', 10),
    commandTimeout: parseInt(process.env.BOT_COMMAND_TIMEOUT ?? '10', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
