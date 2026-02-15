/**
 * Donorbox Plugin Configuration
 */

import 'dotenv/config';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';
import type { DonorboxAccountConfig, DonorboxConfig } from './types.js';

export interface Config extends DonorboxConfig {
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

function buildDonorboxAccountsFromEnv(): DonorboxAccountConfig[] {
  const multiEmails = parseCsvList(process.env.DONORBOX_EMAILS);
  const multiApiKeys = parseCsvList(process.env.DONORBOX_API_KEYS);
  const multiLabels = parseCsvList(process.env.DONORBOX_ACCOUNT_LABELS);
  const multiWebhookSecrets = parseCsvList(process.env.DONORBOX_WEBHOOK_SECRETS);

  if (multiEmails.length > 0) {
    if (multiApiKeys.length !== multiEmails.length) {
      throw new Error('DONORBOX_API_KEYS length must match DONORBOX_EMAILS length');
    }
    if (multiLabels.length > 0 && multiLabels.length !== multiEmails.length) {
      throw new Error('DONORBOX_ACCOUNT_LABELS length must match DONORBOX_EMAILS length');
    }
    if (multiWebhookSecrets.length > 0 && multiWebhookSecrets.length !== multiEmails.length) {
      throw new Error('DONORBOX_WEBHOOK_SECRETS length must match DONORBOX_EMAILS length');
    }

    return multiEmails.map((email, index) => {
      const label = multiLabels[index] ?? `account-${index + 1}`;
      return {
        id: normalizeAccountId(label, index),
        email,
        apiKey: multiApiKeys[index],
        webhookSecret: multiWebhookSecrets[index] ?? '',
      };
    });
  }

  return [{
    id: normalizeAccountId(process.env.DONORBOX_ACCOUNT_ID ?? 'primary', 0),
    email: process.env.DONORBOX_EMAIL ?? '',
    apiKey: process.env.DONORBOX_API_KEY ?? '',
    webhookSecret: process.env.DONORBOX_WEBHOOK_SECRET ?? '',
  }];
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('DONORBOX');
  const accounts = buildDonorboxAccountsFromEnv();
  const primaryAccount = accounts[0];

  const config: Config = {
    email: primaryAccount?.email ?? '',
    apiKey: primaryAccount?.apiKey ?? '',
    accounts,
    port: parseInt(process.env.DONORBOX_PLUGIN_PORT ?? process.env.PORT ?? '3005', 10),
    host: process.env.DONORBOX_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',
    syncInterval: parseInt(process.env.DONORBOX_SYNC_INTERVAL ?? '3600', 10),
    logLevel: process.env.LOG_LEVEL ?? 'info',
    security,
    ...overrides,
  };

  if (!config.email || !config.apiKey) {
    throw new Error('DONORBOX_EMAIL and DONORBOX_API_KEY (or DONORBOX_EMAILS and DONORBOX_API_KEYS) must be set');
  }

  const seenIds = new Set<string>();
  for (const account of config.accounts) {
    if (!account.email || !account.apiKey) {
      throw new Error(`Missing credentials for Donorbox account "${account.id}"`);
    }
    if (seenIds.has(account.id)) {
      throw new Error(`Duplicate Donorbox account id "${account.id}" in configuration`);
    }
    seenIds.add(account.id);
  }

  return config;
}
