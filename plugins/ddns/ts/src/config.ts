/**
 * DDNS Plugin Configuration
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

  // DDNS settings
  provider: string;
  domain: string;
  token: string;
  checkInterval: number;

  // Cloudflare-specific
  cloudflareApiKey: string;
  cloudflareZoneId: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('DDNS');

  const config: Config = {
    // Server
    port: parseInt(process.env.DDNS_PLUGIN_PORT ?? process.env.PORT ?? '3217', 10),
    host: process.env.DDNS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // DDNS settings
    provider: process.env.DDNS_PROVIDER ?? '',
    domain: process.env.DDNS_DOMAIN ?? '',
    token: process.env.DDNS_TOKEN ?? '',
    checkInterval: parseInt(process.env.DDNS_CHECK_INTERVAL ?? '300', 10),

    // Cloudflare-specific
    cloudflareApiKey: process.env.DDNS_CLOUDFLARE_API_KEY ?? '',
    cloudflareZoneId: process.env.DDNS_CLOUDFLARE_ZONE_ID ?? '',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
