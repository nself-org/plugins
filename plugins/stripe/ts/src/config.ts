/**
 * Stripe Plugin Configuration
 */

import 'dotenv/config';
import { requireWebhookSecret, loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface StripeAccountConfig {
  id: string;
  apiKey: string;
  webhookSecret: string;
}

export interface Config {
  // Stripe
  stripeApiKey: string;
  stripeApiVersion: string;
  stripeWebhookSecret: string;
  stripeAccounts: StripeAccountConfig[];

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

  // Sync
  syncInterval: number;
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseCsvList(value: string | undefined): string[] {
  if (!value) {
    return [];
  }

  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

function normalizeAccountId(value: string, index: number): string {
  const normalized = value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');

  if (normalized.length > 0) {
    return normalized;
  }

  return `account-${index + 1}`;
}

function validateStripeApiKey(apiKey: string, label: string): void {
  if (!apiKey.match(/^(sk_|rk_)(test_|live_)/)) {
    throw new Error(`Invalid Stripe API key format for ${label}. Expected sk_test_*, sk_live_*, rk_test_*, or rk_live_*`);
  }
}

function buildStripeAccountsFromEnv(): StripeAccountConfig[] {
  const multiApiKeys = parseCsvList(process.env.STRIPE_API_KEYS);
  const multiLabels = parseCsvList(process.env.STRIPE_ACCOUNT_LABELS);
  const multiWebhookSecrets = parseCsvList(process.env.STRIPE_WEBHOOK_SECRETS);

  if (multiLabels.length > 0 && multiLabels.length !== multiApiKeys.length) {
    throw new Error('STRIPE_ACCOUNT_LABELS length must match STRIPE_API_KEYS length');
  }

  if (multiWebhookSecrets.length > 0 && multiWebhookSecrets.length !== multiApiKeys.length) {
    throw new Error('STRIPE_WEBHOOK_SECRETS length must match STRIPE_API_KEYS length');
  }

  if (multiApiKeys.length > 0) {
    return multiApiKeys.map((apiKey, index) => {
      const label = multiLabels[index] ?? `account-${index + 1}`;

      return {
        id: normalizeAccountId(label, index),
        apiKey,
        webhookSecret: multiWebhookSecrets[index] ?? '',
      };
    });
  }

  const singleApiKey = process.env.STRIPE_API_KEY ?? '';
  const singleWebhookSecret = process.env.STRIPE_WEBHOOK_SECRET ?? '';
  const singleAccountId = normalizeAccountId(process.env.STRIPE_ACCOUNT_ID ?? 'primary', 0);

  return [
    {
      id: singleAccountId,
      apiKey: singleApiKey,
      webhookSecret: singleWebhookSecret,
    },
  ];
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('STRIPE');
  const stripeAccounts = buildStripeAccountsFromEnv();
  const primaryAccount = stripeAccounts[0];

  const config: Config = {
    // Stripe
    stripeApiKey: primaryAccount?.apiKey ?? '',
    stripeApiVersion: process.env.STRIPE_API_VERSION ?? '2024-12-18.acacia',
    stripeWebhookSecret: primaryAccount?.webhookSecret ?? '',
    stripeAccounts,

    // Server
    port: parseInt(process.env.STRIPE_PLUGIN_PORT ?? process.env.PORT ?? '3001', 10),
    host: process.env.STRIPE_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Sync
    syncInterval: parseInt(process.env.STRIPE_SYNC_INTERVAL ?? '3600', 10),
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.stripeApiKey) {
    throw new Error('Either STRIPE_API_KEY or STRIPE_API_KEYS must be set');
  }

  const seenAccountIds = new Set<string>();
  for (const account of config.stripeAccounts) {
    if (!account.apiKey) {
      throw new Error(`Missing API key for Stripe account "${account.id}"`);
    }

    if (seenAccountIds.has(account.id)) {
      throw new Error(`Duplicate Stripe account id "${account.id}" in configuration`);
    }
    seenAccountIds.add(account.id);

    validateStripeApiKey(account.apiKey, `account "${account.id}"`);

    // Enforce webhook secret in production
    requireWebhookSecret(account.webhookSecret, `stripe (${account.id})`);
  }

  // Keep primary account values in sync if overrides changed list-level config.
  if (config.stripeAccounts.length > 0) {
    config.stripeApiKey = config.stripeAccounts[0].apiKey;
    config.stripeWebhookSecret = config.stripeAccounts[0].webhookSecret;
  }

  return config;
}

export function isTestMode(apiKey: string): boolean {
  return apiKey.startsWith('sk_test_') || apiKey.startsWith('rk_test_');
}

export function isLiveMode(apiKey: string): boolean {
  return apiKey.startsWith('sk_live_') || apiKey.startsWith('rk_live_');
}
