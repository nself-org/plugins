/**
 * Social Plugin Configuration
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

  // Social Settings
  maxPostLength: number;
  maxCommentLength: number;
  maxCommentDepth: number;
  editWindowMinutes: number;
  reactionsAllowed: string[];

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

function parseReactionsAllowed(value: string | undefined): string[] {
  if (!value) {
    return ['👍', '❤️', '😂', '😮', '😢', '🔥'];
  }

  return value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('SOCIAL');

  const config: Config = {
    // Server
    port: parseInt(process.env.SOCIAL_PLUGIN_PORT ?? process.env.PORT ?? '3502', 10),
    host: process.env.SOCIAL_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? 'REQUIRED',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Social Settings
    maxPostLength: parseInt(process.env.SOCIAL_MAX_POST_LENGTH ?? '5000', 10),
    maxCommentLength: parseInt(process.env.SOCIAL_MAX_COMMENT_LENGTH ?? '2000', 10),
    maxCommentDepth: parseInt(process.env.SOCIAL_MAX_COMMENT_DEPTH ?? '5', 10),
    editWindowMinutes: parseInt(process.env.SOCIAL_EDIT_WINDOW_MINUTES ?? '30', 10),
    reactionsAllowed: parseReactionsAllowed(process.env.SOCIAL_REACTIONS_ALLOWED),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (!config.databasePassword) {
    throw new Error('POSTGRES_PASSWORD must be set and cannot be empty');
  }

  if (config.databasePassword.length < 8) {
    throw new Error('POSTGRES_PASSWORD must be at least 8 characters long');
  }

  if (config.maxPostLength < 1 || config.maxPostLength > 100000) {
    throw new Error('SOCIAL_MAX_POST_LENGTH must be between 1 and 100000');
  }

  if (config.maxCommentLength < 1 || config.maxCommentLength > 10000) {
    throw new Error('SOCIAL_MAX_COMMENT_LENGTH must be between 1 and 10000');
  }

  if (config.maxCommentDepth < 1 || config.maxCommentDepth > 20) {
    throw new Error('SOCIAL_MAX_COMMENT_DEPTH must be between 1 and 20');
  }

  if (config.reactionsAllowed.length === 0) {
    throw new Error('SOCIAL_REACTIONS_ALLOWED must contain at least one reaction');
  }

  return config;
}
