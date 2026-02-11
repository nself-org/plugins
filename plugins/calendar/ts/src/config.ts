/**
 * Calendar Plugin Configuration
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

  // Calendar settings
  defaultTimezone: string;
  maxAttendees: number;
  reminderCheckInterval: number;
  maxRecurrenceExpand: number;
  icalTokenLength: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('CALENDAR');

  const config: Config = {
    // Server
    port: parseInt(process.env.CALENDAR_PLUGIN_PORT ?? process.env.PORT ?? '3505', 10),
    host: process.env.CALENDAR_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Calendar settings
    defaultTimezone: process.env.CALENDAR_DEFAULT_TIMEZONE ?? 'UTC',
    maxAttendees: parseInt(process.env.CALENDAR_MAX_ATTENDEES ?? '500', 10),
    reminderCheckInterval: parseInt(process.env.CALENDAR_REMINDER_CHECK_INTERVAL_MS ?? '60000', 10),
    maxRecurrenceExpand: parseInt(process.env.CALENDAR_MAX_RECURRENCE_EXPAND ?? '365', 10),
    icalTokenLength: parseInt(process.env.CALENDAR_ICAL_TOKEN_LENGTH ?? '64', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
