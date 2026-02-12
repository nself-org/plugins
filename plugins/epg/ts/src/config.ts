/**
 * EPG Plugin Configuration
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

  // Data Sources
  xmltvUrls: string[];
  schedulesDirectUsername: string;
  schedulesDirectPassword: string;
  schedulesDirectLineup: string;

  // Guide settings
  defaultTimezone: string;
  primetimeStart: string;
  primetimeEnd: string;
  guideDaysAhead: number;
  guideDaysRetain: number;

  // Notifications
  notifyBeforeMinutes: number;
  notifyLiveEvents: boolean;

  // Cleanup
  cleanupOldSchedulesDays: number;
  cleanupCron: string;

  // XMLTV refresh
  xmltvRefreshCron: string;

  // AntServer integration
  antserverUrl: string;
  antserverWebhookSecret: string;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('EPG');

  const config: Config = {
    // Server
    port: parseInt(process.env.EPG_PLUGIN_PORT ?? process.env.PORT ?? '3031', 10),
    host: process.env.EPG_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'localhost',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Data Sources
    xmltvUrls: (process.env.EPG_XMLTV_URLS ?? '').split(',').map(s => s.trim()).filter(Boolean),
    schedulesDirectUsername: process.env.EPG_SCHEDULES_DIRECT_USERNAME ?? '',
    schedulesDirectPassword: process.env.EPG_SCHEDULES_DIRECT_PASSWORD ?? '',
    schedulesDirectLineup: process.env.EPG_SCHEDULES_DIRECT_LINEUP ?? '',

    // Guide settings
    defaultTimezone: process.env.EPG_DEFAULT_TIMEZONE ?? 'America/New_York',
    primetimeStart: process.env.EPG_PRIMETIME_START ?? '19:00',
    primetimeEnd: process.env.EPG_PRIMETIME_END ?? '23:00',
    guideDaysAhead: parseInt(process.env.EPG_GUIDE_DAYS_AHEAD ?? '14', 10),
    guideDaysRetain: parseInt(process.env.EPG_GUIDE_DAYS_RETAIN ?? '7', 10),

    // Notifications
    notifyBeforeMinutes: parseInt(process.env.EPG_NOTIFY_BEFORE_MINUTES ?? '5', 10),
    notifyLiveEvents: process.env.EPG_NOTIFY_LIVE_EVENTS !== 'false',

    // Cleanup
    cleanupOldSchedulesDays: parseInt(process.env.EPG_CLEANUP_OLD_SCHEDULES_DAYS ?? '7', 10),
    cleanupCron: process.env.EPG_CLEANUP_CRON ?? '0 4 * * *',

    // XMLTV refresh
    xmltvRefreshCron: process.env.EPG_XMLTV_REFRESH_CRON ?? '0 3 * * *',

    // AntServer integration
    antserverUrl: process.env.ANTSERVER_URL ?? '',
    antserverWebhookSecret: process.env.ANTSERVER_WEBHOOK_SECRET ?? '',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
