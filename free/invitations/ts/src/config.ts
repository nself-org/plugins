/**
 * Invitations Plugin Configuration
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

  // Invitations
  defaultExpiryHours: number;
  codeLength: number;
  maxBulkSize: number;
  acceptUrlTemplate: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('INVITATIONS');

  const config: Config = {
    // Server
    port: parseInt(process.env.INVITATIONS_PLUGIN_PORT ?? process.env.PORT ?? '3402', 10),
    host: process.env.INVITATIONS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Invitations
    defaultExpiryHours: parseInt(process.env.INVITATIONS_DEFAULT_EXPIRY_HOURS ?? '168', 10),
    codeLength: parseInt(process.env.INVITATIONS_CODE_LENGTH ?? '32', 10),
    maxBulkSize: parseInt(process.env.INVITATIONS_MAX_BULK_SIZE ?? '500', 10),
    acceptUrlTemplate: process.env.INVITATIONS_ACCEPT_URL_TEMPLATE ?? 'https://app.example.com/invite/{{code}}',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.codeLength < 16 || config.codeLength > 128) {
    throw new Error('INVITATIONS_CODE_LENGTH must be between 16 and 128');
  }

  if (config.defaultExpiryHours < 1) {
    throw new Error('INVITATIONS_DEFAULT_EXPIRY_HOURS must be at least 1');
  }

  if (config.maxBulkSize < 1 || config.maxBulkSize > 10000) {
    throw new Error('INVITATIONS_MAX_BULK_SIZE must be between 1 and 10000');
  }

  if (!config.acceptUrlTemplate.includes('{{code}}')) {
    throw new Error('INVITATIONS_ACCEPT_URL_TEMPLATE must contain {{code}} placeholder');
  }

  return config;
}

export function generateInviteUrl(code: string, template: string): string {
  return template.replace('{{code}}', code);
}
