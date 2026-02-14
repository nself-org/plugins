/**
 * DLNA Plugin Configuration
 */

import 'dotenv/config';
import os from 'node:os';
import { v4 as uuidv4 } from 'uuid';
import { loadSecurityConfig, type SecurityConfig } from '@nself/plugin-utils';

export interface Config {
  // DLNA
  dlnaPort: number;
  ssdpPort: number;
  friendlyName: string;
  uuid: string;
  mediaPaths: string[];
  advertiseInterval: number;

  // Server
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // General
  logLevel: string;
  sourceAccountId: string;

  // Security
  security: SecurityConfig;
}

/**
 * Load and validate DLNA plugin configuration from environment variables
 */
export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('DLNA');

  const config: Config = {
    // DLNA
    dlnaPort: parseInt(process.env.DLNA_PORT ?? process.env.PORT ?? '3025', 10),
    ssdpPort: parseInt(process.env.DLNA_SSDP_PORT ?? '1900', 10),
    friendlyName: process.env.DLNA_FRIENDLY_NAME ?? 'nself-tv Media Server',
    uuid: process.env.DLNA_UUID ?? uuidv4(),
    mediaPaths: parseMediaPaths(process.env.DLNA_MEDIA_PATHS ?? '/media'),
    advertiseInterval: parseInt(process.env.DLNA_ADVERTISE_INTERVAL ?? '30', 10),

    // Server
    host: process.env.DLNA_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // General
    logLevel: process.env.LOG_LEVEL ?? 'info',
    sourceAccountId: process.env.DLNA_SOURCE_ACCOUNT_ID ?? 'primary',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.dlnaPort < 1 || config.dlnaPort > 65535) {
    throw new Error(`Invalid DLNA port: ${config.dlnaPort}`);
  }

  if (config.mediaPaths.length === 0) {
    throw new Error('At least one media path must be configured via DLNA_MEDIA_PATHS');
  }

  if (config.advertiseInterval < 5) {
    throw new Error('DLNA_ADVERTISE_INTERVAL must be at least 5 seconds');
  }

  return config;
}

/**
 * Parse comma-separated media paths from environment variable
 */
function parseMediaPaths(value: string): string[] {
  return value
    .split(',')
    .map(p => p.trim())
    .filter(Boolean);
}

/**
 * Get the local network IP address for SSDP advertisement.
 * Returns the first non-internal IPv4 address found.
 */
export function getLocalIpAddress(): string {
  const interfaces = os.networkInterfaces();

  for (const name of Object.keys(interfaces)) {
    const addrs = interfaces[name];
    if (!addrs) continue;

    for (const addr of addrs) {
      if (addr.family === 'IPv4' && !addr.internal) {
        return addr.address;
      }
    }
  }

  return '127.0.0.1';
}
