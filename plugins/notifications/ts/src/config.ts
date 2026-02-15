/**
 * Configuration loader for Notifications plugin
 */

import * as dotenv from 'dotenv';
import { NotificationConfig, EmailProvider, PushProvider, SmsProvider } from './types.js';

// Load environment variables
dotenv.config();

export function loadConfig(): NotificationConfig {
  return {
    database: {
      host: process.env.POSTGRES_HOST ?? 'postgres',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    },

    email: {
      enabled: process.env.NOTIFICATIONS_EMAIL_ENABLED === 'true',
      provider: (process.env.NOTIFICATIONS_EMAIL_PROVIDER || 'resend') as EmailProvider,
      api_key: process.env.NOTIFICATIONS_EMAIL_API_KEY,
      from: process.env.NOTIFICATIONS_EMAIL_FROM || 'noreply@example.com',
      domain: process.env.NOTIFICATIONS_EMAIL_DOMAIN,
      smtp: process.env.SMTP_HOST ? {
        host: process.env.SMTP_HOST,
        port: parseInt(process.env.SMTP_PORT || '587'),
        secure: process.env.SMTP_SECURE === 'true',
        user: process.env.SMTP_USER || '',
        pass: process.env.SMTP_PASS || '',
      } : undefined,
    },

    push: {
      enabled: process.env.NOTIFICATIONS_PUSH_ENABLED === 'true',
      provider: process.env.NOTIFICATIONS_PUSH_PROVIDER as PushProvider | undefined,
      api_key: process.env.NOTIFICATIONS_PUSH_API_KEY,
      app_id: process.env.NOTIFICATIONS_PUSH_APP_ID,
      project_id: process.env.NOTIFICATIONS_PUSH_PROJECT_ID,
      vapid: process.env.NOTIFICATIONS_PUSH_VAPID_PUBLIC_KEY ? {
        public_key: process.env.NOTIFICATIONS_PUSH_VAPID_PUBLIC_KEY,
        private_key: process.env.NOTIFICATIONS_PUSH_VAPID_PRIVATE_KEY || '',
        subject: process.env.NOTIFICATIONS_PUSH_VAPID_SUBJECT || '',
      } : undefined,
    },

    sms: {
      enabled: process.env.NOTIFICATIONS_SMS_ENABLED === 'true',
      provider: process.env.NOTIFICATIONS_SMS_PROVIDER as SmsProvider | undefined,
      account_sid: process.env.NOTIFICATIONS_SMS_ACCOUNT_SID,
      auth_token: process.env.NOTIFICATIONS_SMS_AUTH_TOKEN,
      auth_id: process.env.NOTIFICATIONS_SMS_AUTH_ID,
      from: process.env.NOTIFICATIONS_SMS_FROM,
    },

    queue: {
      backend: (process.env.NOTIFICATIONS_QUEUE_BACKEND || 'redis') as 'redis' | 'postgres',
      redis_url: process.env.REDIS_URL || 'redis://redis:6379',
    },

    worker: {
      concurrency: parseInt(process.env.WORKER_CONCURRENCY || '5'),
      poll_interval: parseInt(process.env.WORKER_POLL_INTERVAL || '1000'),
    },

    retry: {
      attempts: parseInt(process.env.NOTIFICATIONS_RETRY_ATTEMPTS || '3'),
      delay: parseInt(process.env.NOTIFICATIONS_RETRY_DELAY || '1000'),
      max_delay: parseInt(process.env.NOTIFICATIONS_MAX_RETRY_DELAY || '300000'),
    },

    rate_limits: {
      email: {
        per_user: parseInt(process.env.NOTIFICATIONS_RATE_LIMIT_EMAIL || '100'),
        window: 3600,
      },
      push: {
        per_user: parseInt(process.env.NOTIFICATIONS_RATE_LIMIT_PUSH || '200'),
        window: 3600,
      },
      sms: {
        per_user: parseInt(process.env.NOTIFICATIONS_RATE_LIMIT_SMS || '20'),
        window: 3600,
      },
    },

    batch: {
      enabled: process.env.NOTIFICATIONS_BATCH_ENABLED === 'true',
      interval: parseInt(process.env.NOTIFICATIONS_BATCH_INTERVAL || '86400'),
    },

    server: {
      port: parseInt(process.env.PORT || '3102'),
      host: process.env.HOST || '0.0.0.0',
    },

    features: {
      tracking_enabled: process.env.NOTIFICATIONS_TRACKING_ENABLED !== 'false',
      quiet_hours_enabled: process.env.NOTIFICATIONS_QUIET_HOURS_ENABLED !== 'false',
    },

    security: {
      encrypt_config: process.env.NOTIFICATIONS_ENCRYPT_CONFIG === 'true',
      encryption_key: process.env.NOTIFICATIONS_ENCRYPTION_KEY,
      webhook_secret: process.env.NOTIFICATIONS_WEBHOOK_SECRET,
      webhook_verify: process.env.NOTIFICATIONS_WEBHOOK_VERIFY !== 'false',
    },

    development: {
      dry_run: process.env.NOTIFICATIONS_DRY_RUN === 'true',
      test_mode: process.env.NOTIFICATIONS_TEST_MODE === 'true',
      log_level: process.env.LOG_LEVEL || 'info',
    },
  };
}

export const config = loadConfig();
