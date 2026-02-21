/**
 * GitHub Plugin Configuration
 */

import 'dotenv/config';
import { requireWebhookSecret, loadSecurityConfig, buildAccountConfigs, type SecurityConfig, type AccountConfig } from '@nself/plugin-utils';

export interface Config {
  // GitHub
  githubToken: string;
  githubWebhookSecret: string;
  githubOrg?: string;
  githubRepos?: string[];

  // Multi-app
  accounts: AccountConfig[];

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

export function loadConfig(overrides?: Partial<Config>): Config {
  const reposEnv = process.env.GITHUB_REPOS;
  const repos = reposEnv ? reposEnv.split(',').map(r => r.trim()).filter(r => r) : undefined;
  const security = loadSecurityConfig('GITHUB');

  const accounts = buildAccountConfigs(
    process.env.GITHUB_API_KEYS,
    process.env.GITHUB_ACCOUNT_LABELS,
    process.env.GITHUB_TOKEN,
    'primary'
  );

  const config: Config = {
    // GitHub
    githubToken: process.env.GITHUB_TOKEN ?? '',
    githubWebhookSecret: process.env.GITHUB_WEBHOOK_SECRET ?? '',
    githubOrg: process.env.GITHUB_ORG,
    githubRepos: repos,

    // Multi-app
    accounts,

    // Server
    port: parseInt(process.env.GITHUB_PLUGIN_PORT ?? process.env.PORT ?? '3002', 10),
    host: process.env.GITHUB_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Sync
    syncInterval: parseInt(process.env.GITHUB_SYNC_INTERVAL ?? '3600', 10),
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.githubToken) {
    throw new Error('GITHUB_TOKEN is required');
  }

  // Validate GitHub token format (PAT or fine-grained)
  if (!config.githubToken.match(/^(ghp_|github_pat_|gho_|ghu_|ghs_|ghr_)/)) {
    throw new Error('Invalid GITHUB_TOKEN format. Expected a GitHub personal access token (ghp_*, github_pat_*, etc.)');
  }

  // Enforce webhook secret in production
  requireWebhookSecret(config.githubWebhookSecret, 'github');

  return config;
}
