/**
 * CDN Plugin Configuration
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

  // CDN Provider
  provider: string;
  cloudflareApiToken: string;
  cloudflareZoneIds: string[];
  bunnyCdnApiKey: string;
  bunnyCdnPullZoneIds: string[];

  // Signing
  signingKey: string;
  signedUrlTtl: number;

  // Analytics
  analyticsSyncInterval: number;

  // Purge
  purgeBatchSize: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseDatabaseUrl(url: string | undefined): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} | null {
  if (!url) {
    return null;
  }

  try {
    const match = url.match(/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/);
    if (!match) {
      return null;
    }

    const [, user, password, host, port, database, queryString] = match;
    const ssl = queryString?.includes('sslmode=require') || queryString?.includes('ssl=true') || false;

    return {
      host,
      port: parseInt(port, 10),
      database,
      user,
      password,
      ssl,
    };
  } catch {
    return null;
  }
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('CDN');
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const config: Config = {
    // Server
    port: parseInt(process.env.CDN_PLUGIN_PORT ?? process.env.PORT ?? '3036', 10),
    host: process.env.CDN_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    databaseUser: dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',

    // CDN Provider
    provider: process.env.CDN_PROVIDER ?? 'cloudflare',
    cloudflareApiToken: process.env.CDN_CLOUDFLARE_API_TOKEN ?? '',
    cloudflareZoneIds: (process.env.CDN_CLOUDFLARE_ZONE_IDS ?? '').split(',').filter(Boolean).map(s => s.trim()),
    bunnyCdnApiKey: process.env.CDN_BUNNYCDN_API_KEY ?? '',
    bunnyCdnPullZoneIds: (process.env.CDN_BUNNYCDN_PULL_ZONE_IDS ?? '').split(',').filter(Boolean).map(s => s.trim()),

    // Signing
    signingKey: process.env.CDN_SIGNING_KEY ?? '',
    signedUrlTtl: parseInt(process.env.CDN_SIGNED_URL_TTL ?? '3600', 10),

    // Analytics
    analyticsSyncInterval: parseInt(process.env.CDN_ANALYTICS_SYNC_INTERVAL ?? '86400', 10),

    // Purge
    purgeBatchSize: parseInt(process.env.CDN_PURGE_BATCH_SIZE ?? '30', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
