/**
 * Shopify Plugin Configuration
 * Load and validate configuration from environment variables
 */

import { config as loadEnv } from 'dotenv';
import { createLogger, requireWebhookSecret, loadSecurityConfig, buildAccountConfigs, type SecurityConfig, type AccountConfig } from '@nself/plugin-utils';

const logger = createLogger('shopify:config');

export interface ShopifyConfig {
  // Shopify credentials (legacy single account)
  shopifyShopDomain: string;
  shopifyAccessToken: string;
  shopifyApiVersion: string;
  shopifyWebhookSecret: string;

  // Multi-app
  accounts: AccountConfig[];

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Server
  port: number;
  host: string;

  // Sync options
  syncBatchSize: number;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides: Partial<ShopifyConfig> = {}): ShopifyConfig {
  // Load .env file
  loadEnv();

  const security = loadSecurityConfig('SHOPIFY');

  // Build multi-account configs from CSV env vars
  const accounts = buildAccountConfigs(
    process.env.SHOPIFY_ACCESS_TOKENS,
    process.env.SHOPIFY_ACCOUNT_LABELS,
    process.env.SHOPIFY_ACCESS_TOKEN,
    'primary'
  );

  const config: ShopifyConfig = {
    // Shopify credentials (legacy single account)
    shopifyShopDomain: overrides.shopifyShopDomain ?? process.env.SHOPIFY_SHOP_DOMAIN ?? '',
    shopifyAccessToken: overrides.shopifyAccessToken ?? process.env.SHOPIFY_ACCESS_TOKEN ?? '',
    shopifyApiVersion: overrides.shopifyApiVersion ?? process.env.SHOPIFY_API_VERSION ?? '2024-01',
    shopifyWebhookSecret: overrides.shopifyWebhookSecret ?? process.env.SHOPIFY_WEBHOOK_SECRET ?? '',

    // Multi-app
    accounts,

    // Database
    databaseHost: overrides.databaseHost ?? process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: overrides.databasePort ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: overrides.databaseName ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: overrides.databaseUser ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: overrides.databasePassword ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: overrides.databaseSsl ?? process.env.POSTGRES_SSL === 'true',

    // Server
    port: overrides.port ?? parseInt(process.env.PORT ?? '3003', 10),
    host: overrides.host ?? process.env.HOST ?? '0.0.0.0',

    // Sync options
    syncBatchSize: overrides.syncBatchSize ?? parseInt(process.env.SYNC_BATCH_SIZE ?? '250', 10),

    // Security
    security,
  };

  // Validate required fields
  const errors: string[] = [];

  if (!config.shopifyShopDomain) {
    errors.push('SHOPIFY_SHOP_DOMAIN is required');
  }

  if (!config.shopifyAccessToken) {
    errors.push('SHOPIFY_ACCESS_TOKEN is required');
  }

  // Validate Shopify access token format
  if (config.shopifyAccessToken && !config.shopifyAccessToken.match(/^shpat_/)) {
    errors.push('Invalid SHOPIFY_ACCESS_TOKEN format. Expected shpat_*');
  }

  if (errors.length > 0) {
    logger.error('Configuration errors', { errors });
    throw new Error(`Configuration invalid:\n  - ${errors.join('\n  - ')}`);
  }

  // Enforce webhook secret in production
  requireWebhookSecret(config.shopifyWebhookSecret, 'shopify');

  // Log loaded config (without secrets)
  logger.debug('Configuration loaded', {
    shopDomain: config.shopifyShopDomain,
    apiVersion: config.shopifyApiVersion,
    port: config.port,
    host: config.host,
    hasWebhookSecret: !!config.shopifyWebhookSecret,
  });

  return config;
}

export function getShopName(shopDomain: string): string {
  // Extract shop name from domain (e.g., "myshop.myshopify.com" -> "myshop")
  return shopDomain.replace('.myshopify.com', '');
}
