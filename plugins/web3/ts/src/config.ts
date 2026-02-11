/**
 * Web3 Plugin Configuration
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

  // Web3
  defaultChainId: number;
  supportedChains: number[];
  gateCheckCacheTtl: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('WEB3');

  const config: Config = {
    // Server
    port: parseInt(process.env.WEB3_PLUGIN_PORT ?? process.env.PORT ?? '3715', 10),
    host: process.env.WEB3_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Web3
    defaultChainId: parseInt(process.env.WEB3_DEFAULT_CHAIN_ID ?? '1', 10),
    supportedChains: (process.env.WEB3_SUPPORTED_CHAINS ?? '1,137,42161,10,8453')
      .split(',')
      .map((s) => parseInt(s.trim(), 10)),
    gateCheckCacheTtl: parseInt(process.env.WEB3_GATE_CHECK_CACHE_TTL ?? '300', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
