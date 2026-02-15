/**
 * Support Plugin Configuration
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

  // Support Email
  supportEmail: string;
  supportEmailSmtpHost: string;
  supportEmailSmtpPort: number;
  supportEmailFromName: string;

  // SLA Defaults
  defaultFirstResponseMinutes: number;
  defaultResolutionMinutes: number;
  businessHoursStart: string;
  businessHoursEnd: string;
  timezone: string;

  // Routing
  autoAssignment: boolean;
  assignmentMethod: string;
  maxTicketsPerAgent: number;

  // Satisfaction
  csatEnabled: boolean;
  csatSendDelayHours: number;
  npsEnabled: boolean;
  npsSendIntervalDays: number;

  // Knowledge Base
  kbEnabled: boolean;
  kbPublicAccess: boolean;
  kbSuggestArticles: boolean;

  // Notifications
  notifyOnNewTicket: boolean;
  notifyOnAssignment: boolean;
  notifyOnSlaBreach: boolean;
  notifyOnCustomerReply: boolean;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('SUPPORT');

  const config: Config = {
    // Server
    port: parseInt(process.env.SUPPORT_PLUGIN_PORT ?? process.env.PORT ?? '3709', 10),
    host: process.env.SUPPORT_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // Support Email
    supportEmail: process.env.SUPPORT_EMAIL ?? 'support@example.com',
    supportEmailSmtpHost: process.env.SUPPORT_EMAIL_SMTP_HOST ?? '',
    supportEmailSmtpPort: parseInt(process.env.SUPPORT_EMAIL_SMTP_PORT ?? '587', 10),
    supportEmailFromName: process.env.SUPPORT_EMAIL_FROM_NAME ?? 'Support Team',

    // SLA Defaults
    defaultFirstResponseMinutes: parseInt(process.env.SUPPORT_DEFAULT_FIRST_RESPONSE_MINUTES ?? '60', 10),
    defaultResolutionMinutes: parseInt(process.env.SUPPORT_DEFAULT_RESOLUTION_MINUTES ?? '1440', 10),
    businessHoursStart: process.env.SUPPORT_BUSINESS_HOURS_START ?? '09:00',
    businessHoursEnd: process.env.SUPPORT_BUSINESS_HOURS_END ?? '17:00',
    timezone: process.env.SUPPORT_TIMEZONE ?? 'UTC',

    // Routing
    autoAssignment: process.env.SUPPORT_AUTO_ASSIGNMENT !== 'false',
    assignmentMethod: process.env.SUPPORT_ASSIGNMENT_METHOD ?? 'round_robin',
    maxTicketsPerAgent: parseInt(process.env.SUPPORT_MAX_TICKETS_PER_AGENT ?? '10', 10),

    // Satisfaction
    csatEnabled: process.env.SUPPORT_CSAT_ENABLED !== 'false',
    csatSendDelayHours: parseInt(process.env.SUPPORT_CSAT_SEND_DELAY_HOURS ?? '24', 10),
    npsEnabled: process.env.SUPPORT_NPS_ENABLED === 'true',
    npsSendIntervalDays: parseInt(process.env.SUPPORT_NPS_SEND_INTERVAL_DAYS ?? '90', 10),

    // Knowledge Base
    kbEnabled: process.env.SUPPORT_KB_ENABLED !== 'false',
    kbPublicAccess: process.env.SUPPORT_KB_PUBLIC_ACCESS !== 'false',
    kbSuggestArticles: process.env.SUPPORT_KB_SUGGEST_ARTICLES !== 'false',

    // Notifications
    notifyOnNewTicket: process.env.SUPPORT_NOTIFY_ON_NEW_TICKET !== 'false',
    notifyOnAssignment: process.env.SUPPORT_NOTIFY_ON_ASSIGNMENT !== 'false',
    notifyOnSlaBreach: process.env.SUPPORT_NOTIFY_ON_SLA_BREACH !== 'false',
    notifyOnCustomerReply: process.env.SUPPORT_NOTIFY_ON_CUSTOMER_REPLY !== 'false',

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  return config;
}
