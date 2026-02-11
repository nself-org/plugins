/**
 * Chat Plugin Configuration
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

  // Chat limits
  maxMessageLength: number;
  maxAttachments: number;
  editWindowMinutes: number;
  maxParticipants: number;
  maxPinned: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('CHAT');

  const config: Config = {
    // Server
    port: parseInt(process.env.CHAT_PLUGIN_PORT ?? process.env.PORT ?? '3401', 10),
    host: process.env.CHAT_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Chat limits
    maxMessageLength: parseInt(process.env.CHAT_MAX_MESSAGE_LENGTH ?? '10000', 10),
    maxAttachments: parseInt(process.env.CHAT_MAX_ATTACHMENTS ?? '10', 10),
    editWindowMinutes: parseInt(process.env.CHAT_EDIT_WINDOW_MINUTES ?? '15', 10),
    maxParticipants: parseInt(process.env.CHAT_MAX_PARTICIPANTS ?? '100', 10),
    maxPinned: parseInt(process.env.CHAT_MAX_PINNED ?? '50', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.maxMessageLength < 1 || config.maxMessageLength > 100000) {
    throw new Error('CHAT_MAX_MESSAGE_LENGTH must be between 1 and 100000');
  }

  if (config.maxAttachments < 0 || config.maxAttachments > 50) {
    throw new Error('CHAT_MAX_ATTACHMENTS must be between 0 and 50');
  }

  if (config.editWindowMinutes < 0) {
    throw new Error('CHAT_EDIT_WINDOW_MINUTES must be non-negative');
  }

  if (config.maxParticipants < 2 || config.maxParticipants > 1000) {
    throw new Error('CHAT_MAX_PARTICIPANTS must be between 2 and 1000');
  }

  if (config.maxPinned < 0 || config.maxPinned > 100) {
    throw new Error('CHAT_MAX_PINNED must be between 0 and 100');
  }

  return config;
}
