/**
 * Audit Plugin Configuration
 * Environment variable loading and validation
 */

import dotenv from 'dotenv';
import { AuditConfig, ComplianceFramework } from './types.js';
import { parseCsvList, normalizeSourceAccountId } from '@nself/plugin-utils';

// Load environment variables
dotenv.config();

/**
 * Get environment variable with optional default
 */
function getEnv(key: string, defaultValue?: string): string {
  const value = process.env[key];
  if (value === undefined) {
    if (defaultValue !== undefined) {
      return defaultValue;
    }
    throw new Error(`Missing required environment variable: ${key}`);
  }
  return value;
}

/**
 * Get optional environment variable
 */
function getEnvOptional(key: string, defaultValue: string = ''): string {
  return process.env[key] || defaultValue;
}

/**
 * Get integer environment variable
 */
function getEnvInt(key: string, defaultValue: number): number {
  const value = process.env[key];
  if (!value) return defaultValue;
  const parsed = parseInt(value, 10);
  if (isNaN(parsed)) {
    throw new Error(`Invalid integer value for ${key}: ${value}`);
  }
  return parsed;
}

/**
 * Get boolean environment variable
 */
function getEnvBool(key: string, defaultValue: boolean): boolean {
  const value = process.env[key];
  if (!value) return defaultValue;
  return value.toLowerCase() === 'true' || value === '1';
}

/**
 * Load and validate configuration
 */
export function loadConfig(): AuditConfig {
  // Parse app IDs
  const appIdsRaw = getEnvOptional('AUDIT_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  // Parse compliance frameworks
  const frameworksRaw = getEnvOptional('AUDIT_COMPLIANCE_FRAMEWORKS', 'SOC2,HIPAA,GDPR,PCI');
  const frameworks = parseCsvList(frameworksRaw) as ComplianceFramework[];

  const config: AuditConfig = {
    port: getEnvInt('AUDIT_PLUGIN_PORT', 3303),
    host: getEnvOptional('AUDIT_PLUGIN_HOST', '0.0.0.0'),
    logLevel: (getEnvOptional('AUDIT_LOG_LEVEL', 'info') as 'debug' | 'info' | 'warn' | 'error'),

    // Database
    database: {
      host: getEnvOptional('POSTGRES_HOST', 'localhost'),
      port: getEnvInt('POSTGRES_PORT', 5432),
      database: getEnvOptional('POSTGRES_DB', 'nself'),
      user: getEnvOptional('POSTGRES_USER', 'postgres'),
      password: getEnvOptional('POSTGRES_PASSWORD', ''),
      ssl: getEnvBool('POSTGRES_SSL', false),
    },

    // Multi-app
    appIds,

    // Fallback logging
    fallback: {
      logPath: getEnvOptional('AUDIT_FALLBACK_LOG_PATH', '/var/log/nself/audit-fallback.jsonl'),
    },

    // SIEM integration
    siem: {},

    // Retention
    retention: {
      defaultDays: getEnvInt('AUDIT_DEFAULT_RETENTION_DAYS', 2555), // 7 years default
    },

    // Compliance
    compliance: {
      frameworks,
    },

    // Alerts
    alerts: {
      webhookUrl: getEnvOptional('AUDIT_ALERT_WEBHOOK_URL') || null,
    },

    // Export
    export: {
      maxRows: getEnvInt('AUDIT_EXPORT_MAX_ROWS', 100000),
    },
  };

  // Load SIEM configurations
  const splunkHecUrl = getEnvOptional('AUDIT_SIEM_SPLUNK_HEC_URL');
  if (splunkHecUrl) {
    config.siem.splunk = {
      hecUrl: splunkHecUrl,
      hecToken: getEnvOptional('AUDIT_SIEM_SPLUNK_HEC_TOKEN'),
    };
  }

  const elkUrl = getEnvOptional('AUDIT_SIEM_ELK_URL');
  if (elkUrl) {
    config.siem.elk = {
      url: elkUrl,
      index: getEnvOptional('AUDIT_SIEM_ELK_INDEX', 'audit-logs'),
      apiKey: getEnvOptional('AUDIT_SIEM_ELK_API_KEY'),
    };
  }

  const datadogApiKey = getEnvOptional('AUDIT_SIEM_DATADOG_API_KEY');
  if (datadogApiKey) {
    config.siem.datadog = {
      apiKey: datadogApiKey,
      site: getEnvOptional('AUDIT_SIEM_DATADOG_SITE', 'datadoghq.com'),
    };
  }

  return config;
}

// Export singleton config instance
export const config = loadConfig();
