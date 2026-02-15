/**
 * PayPal Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { PayPalAccountConfig, PayPalConfig } from './types.js';

export interface Config extends PayPalConfig {
  security: SecurityConfig;
}

function parseCsvList(value: string | undefined): string[] {
  if (!value) return [];
  return value.split(',').map(item => item.trim()).filter(Boolean);
}

function normalizeAccountId(value: string, index: number): string {
  const normalized = value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return normalized.length > 0 ? normalized : `account-${index + 1}`;
}

function buildPayPalAccountsFromEnv(): PayPalAccountConfig[] {
  const multiClientIds = parseCsvList(process.env.PAYPAL_CLIENT_IDS);
  const multiClientSecrets = parseCsvList(process.env.PAYPAL_CLIENT_SECRETS);
  const multiLabels = parseCsvList(process.env.PAYPAL_ACCOUNT_LABELS);
  const multiWebhookIds = parseCsvList(process.env.PAYPAL_WEBHOOK_IDS);
  const multiWebhookSecrets = parseCsvList(process.env.PAYPAL_WEBHOOK_SECRETS);

  if (multiClientIds.length > 0) {
    if (multiClientSecrets.length !== multiClientIds.length) {
      throw new Error('PAYPAL_CLIENT_SECRETS length must match PAYPAL_CLIENT_IDS length');
    }
    if (multiLabels.length > 0 && multiLabels.length !== multiClientIds.length) {
      throw new Error('PAYPAL_ACCOUNT_LABELS length must match PAYPAL_CLIENT_IDS length');
    }
    if (multiWebhookIds.length > 0 && multiWebhookIds.length !== multiClientIds.length) {
      throw new Error('PAYPAL_WEBHOOK_IDS length must match PAYPAL_CLIENT_IDS length');
    }
    if (multiWebhookSecrets.length > 0 && multiWebhookSecrets.length !== multiClientIds.length) {
      throw new Error('PAYPAL_WEBHOOK_SECRETS length must match PAYPAL_CLIENT_IDS length');
    }

    return multiClientIds.map((clientId, index) => {
      const label = multiLabels[index] ?? `account-${index + 1}`;
      return {
        id: normalizeAccountId(label, index),
        clientId,
        clientSecret: multiClientSecrets[index],
        webhookId: multiWebhookIds[index] ?? '',
        webhookSecret: multiWebhookSecrets[index] ?? '',
      };
    });
  }

  return [{
    id: normalizeAccountId(process.env.PAYPAL_ACCOUNT_ID ?? 'primary', 0),
    clientId: process.env.PAYPAL_CLIENT_ID ?? '',
    clientSecret: process.env.PAYPAL_CLIENT_SECRET ?? '',
    webhookId: process.env.PAYPAL_WEBHOOK_ID ?? '',
    webhookSecret: process.env.PAYPAL_WEBHOOK_SECRET ?? '',
  }];
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('PAYPAL');
  const accounts = buildPayPalAccountsFromEnv();
  const primaryAccount = accounts[0];

  const config: Config = {
    clientId: primaryAccount?.clientId ?? '',
    clientSecret: primaryAccount?.clientSecret ?? '',
    environment: (process.env.PAYPAL_ENVIRONMENT as 'sandbox' | 'live') ?? 'live',
    accounts,
    port: parseInt(process.env.PAYPAL_PLUGIN_PORT ?? process.env.PORT ?? '3004', 10),
    host: process.env.PAYPAL_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',
    syncInterval: parseInt(process.env.PAYPAL_SYNC_INTERVAL ?? '3600', 10),
    logLevel: process.env.LOG_LEVEL ?? 'info',
    security,
    ...overrides,
  };

  if (!config.clientId || !config.clientSecret) {
    throw new Error('PAYPAL_CLIENT_ID and PAYPAL_CLIENT_SECRET (or PAYPAL_CLIENT_IDS and PAYPAL_CLIENT_SECRETS) must be set');
  }

  const seenIds = new Set<string>();
  for (const account of config.accounts) {
    if (!account.clientId || !account.clientSecret) {
      throw new Error(`Missing client credentials for PayPal account "${account.id}"`);
    }
    if (seenIds.has(account.id)) {
      throw new Error(`Duplicate PayPal account id "${account.id}" in configuration`);
    }
    seenIds.add(account.id);
  }

  return config;
}

export function isSandbox(config: Pick<PayPalConfig, 'environment'>): boolean {
  return config.environment === 'sandbox';
}

export function getBaseUrl(config: Pick<PayPalConfig, 'environment'>): string {
  return config.environment === 'sandbox'
    ? 'https://api-m.sandbox.paypal.com'
    : 'https://api-m.paypal.com';
}
