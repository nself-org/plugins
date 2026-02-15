/**
 * Compliance Plugin Configuration
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

  // GDPR
  gdprEnabled: boolean;
  dsarDeadlineDays: number;
  dsarAutoVerification: boolean;
  breachNotificationHours: number;

  // CCPA
  ccpaEnabled: boolean;
  ccpaDeadlineDays: number;

  // Consent
  consentRequired: boolean;
  consentExpiryDays: number;
  consentMethod: string;

  // Data Retention
  retentionEnabled: boolean;
  retentionGracePeriodDays: number;

  // Notifications
  notifyDsarAssigned: boolean;
  notifyDsarDeadlineDays: number;
  notifyPolicyUpdates: boolean;

  // Export
  exportFormat: string;
  exportEncryption: boolean;
  exportExpiryHours: number;

  // Audit
  auditEnabled: boolean;
  auditRetentionDays: number;

  // Logging
  logLevel: string;

  // Security
  security: SecurityConfig;
}

export function loadConfig(overrides?: Partial<Config>): Config {
  const security = loadSecurityConfig('COMPLIANCE');

  const config: Config = {
    // Server
    port: parseInt(process.env.COMPLIANCE_PLUGIN_PORT ?? process.env.PORT ?? '3706', 10),
    host: process.env.COMPLIANCE_PLUGIN_HOST ?? process.env.HOST ?? '0.0.0.0',

    // Database
    databaseHost: process.env.POSTGRES_HOST ?? 'postgres',
    databasePort: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    databaseName: process.env.POSTGRES_DB ?? 'nself',
    databaseUser: process.env.POSTGRES_USER ?? 'postgres',
    databasePassword: process.env.POSTGRES_PASSWORD ?? '',
    databaseSsl: process.env.POSTGRES_SSL === 'true',

    // GDPR
    gdprEnabled: process.env.COMPLIANCE_GDPR_ENABLED !== 'false',
    dsarDeadlineDays: parseInt(process.env.COMPLIANCE_DSAR_DEADLINE_DAYS ?? '30', 10),
    dsarAutoVerification: process.env.COMPLIANCE_DSAR_AUTO_VERIFICATION === 'true',
    breachNotificationHours: parseInt(process.env.COMPLIANCE_BREACH_NOTIFICATION_HOURS ?? '72', 10),

    // CCPA
    ccpaEnabled: process.env.COMPLIANCE_CCPA_ENABLED !== 'false',
    ccpaDeadlineDays: parseInt(process.env.COMPLIANCE_CCPA_DEADLINE_DAYS ?? '45', 10),

    // Consent
    consentRequired: process.env.COMPLIANCE_CONSENT_REQUIRED !== 'false',
    consentExpiryDays: parseInt(process.env.COMPLIANCE_CONSENT_EXPIRY_DAYS ?? '365', 10),
    consentMethod: process.env.COMPLIANCE_CONSENT_METHOD ?? 'explicit',

    // Data Retention
    retentionEnabled: process.env.COMPLIANCE_RETENTION_ENABLED !== 'false',
    retentionGracePeriodDays: parseInt(process.env.COMPLIANCE_RETENTION_GRACE_PERIOD_DAYS ?? '7', 10),

    // Notifications
    notifyDsarAssigned: process.env.COMPLIANCE_NOTIFY_DSAR_ASSIGNED !== 'false',
    notifyDsarDeadlineDays: parseInt(process.env.COMPLIANCE_NOTIFY_DSAR_DEADLINE_DAYS ?? '3', 10),
    notifyPolicyUpdates: process.env.COMPLIANCE_NOTIFY_POLICY_UPDATES !== 'false',

    // Export
    exportFormat: process.env.COMPLIANCE_EXPORT_FORMAT ?? 'json',
    exportEncryption: process.env.COMPLIANCE_EXPORT_ENCRYPTION !== 'false',
    exportExpiryHours: parseInt(process.env.COMPLIANCE_EXPORT_EXPIRY_HOURS ?? '72', 10),

    // Audit
    auditEnabled: process.env.COMPLIANCE_AUDIT_ENABLED !== 'false',
    auditRetentionDays: parseInt(process.env.COMPLIANCE_AUDIT_RETENTION_DAYS ?? '2555', 10),

    // Logging
    logLevel: process.env.LOG_LEVEL ?? 'info',

    // Security
    security,

    // Apply overrides
    ...overrides,
  };

  // Validation
  if (config.dsarDeadlineDays < 1) {
    throw new Error('COMPLIANCE_DSAR_DEADLINE_DAYS must be at least 1');
  }

  if (config.breachNotificationHours < 1) {
    throw new Error('COMPLIANCE_BREACH_NOTIFICATION_HOURS must be at least 1');
  }

  if (config.consentExpiryDays < 1) {
    throw new Error('COMPLIANCE_CONSENT_EXPIRY_DAYS must be at least 1');
  }

  return config;
}
