/**
 * Documents Plugin Configuration
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

  // Document settings
  pdfEngine: string;
  defaultTemplateEngine: string;
  storageProvider: string;
  storagePath: string;
  maxDocumentSizeMb: number;
  shareTokenLength: number;
  shareDefaultExpiryDays: number;
  versionRetention: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('DOCS');

  const config: Config = {
    // Server
    port: parseInt(process.env.DOCS_PLUGIN_PORT ?? process.env.PORT ?? '3029', 10),
    host: process.env.DOCS_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Document settings
    pdfEngine: process.env.DOCS_PDF_ENGINE ?? 'puppeteer',
    defaultTemplateEngine: process.env.DOCS_DEFAULT_TEMPLATE_ENGINE ?? 'handlebars',
    storageProvider: process.env.DOCS_STORAGE_PROVIDER ?? 'local',
    storagePath: process.env.DOCS_STORAGE_PATH ?? '/data/documents',
    maxDocumentSizeMb: parseInt(process.env.DOCS_MAX_DOCUMENT_SIZE_MB ?? '50', 10),
    shareTokenLength: parseInt(process.env.DOCS_SHARE_TOKEN_LENGTH ?? '32', 10),
    shareDefaultExpiryDays: parseInt(process.env.DOCS_SHARE_DEFAULT_EXPIRY_DAYS ?? '30', 10),
    versionRetention: parseInt(process.env.DOCS_VERSION_RETENTION ?? '10', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
