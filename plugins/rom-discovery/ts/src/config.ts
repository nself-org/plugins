/**
 * ROM Discovery Plugin Configuration
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

  // Scrapers
  enableScrapers: boolean;
  scraperSchedule: string;

  // Quality / Popularity defaults
  defaultQualityMin: number;
  defaultPopularityMin: number;

  // Downloads
  maxConcurrentDownloads: number;
  maxDownloadSizeMb: number;

  // Integration
  retroGamingUrl: string;
  cdnUrl: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('ROM_DISCOVERY');

  const config: Config = {
    // Server
    port: parseInt(process.env.ROM_DISCOVERY_PLUGIN_PORT ?? process.env.PORT ?? '3034', 10),
    host: process.env.ROM_DISCOVERY_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Scrapers
    enableScrapers: process.env.ROM_DISCOVERY_ENABLE_SCRAPERS === 'true',
    scraperSchedule: process.env.ROM_DISCOVERY_SCRAPER_SCHEDULE ?? '0 3 * * *',

    // Quality / Popularity
    defaultQualityMin: parseInt(process.env.ROM_DISCOVERY_DEFAULT_QUALITY_MIN ?? '50', 10),
    defaultPopularityMin: parseInt(process.env.ROM_DISCOVERY_DEFAULT_POPULARITY_MIN ?? '0', 10),

    // Downloads
    maxConcurrentDownloads: parseInt(process.env.ROM_DISCOVERY_MAX_CONCURRENT_DOWNLOADS ?? '3', 10),
    maxDownloadSizeMb: parseInt(process.env.ROM_DISCOVERY_MAX_DOWNLOAD_SIZE_MB ?? '2048', 10),

    // Integration
    retroGamingUrl: process.env.ROM_DISCOVERY_RETRO_GAMING_URL ?? 'http://localhost:3033',
    cdnUrl: process.env.ROM_DISCOVERY_CDN_URL ?? '',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
