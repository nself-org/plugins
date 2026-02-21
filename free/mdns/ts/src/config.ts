/**
 * mDNS Plugin Configuration
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

  // mDNS settings
  defaultServiceType: string;
  instanceName: string;
  domain: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('MDNS');

  const config: Config = {
    // Server
    port: parseInt(process.env.MDNS_PLUGIN_PORT ?? process.env.PORT ?? '3216', 10),
    host: process.env.MDNS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // mDNS settings
    defaultServiceType: process.env.MDNS_SERVICE_TYPE ?? '_ntv._tcp',
    instanceName: process.env.MDNS_INSTANCE_NAME ?? 'nself-server',
    domain: process.env.MDNS_DOMAIN ?? 'local',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
