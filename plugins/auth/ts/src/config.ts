/**
 * Auth Plugin Configuration
 * Environment variable loading and validation
 */

import dotenv from 'dotenv';
import { AuthConfig, AppAuthConfig } from './types.js';
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
 * Load per-app configuration overrides
 */
function loadAppConfigs(appIds: string[]): Map<string, AppAuthConfig> {
  const configs = new Map<string, AppAuthConfig>();

  for (const rawAppId of appIds) {
    const appId = normalizeSourceAccountId(rawAppId);
    const prefix = `AUTH_${appId.toUpperCase().replace(/-/g, '_')}_`;

    const config: AppAuthConfig = { id: appId };

    // OAuth overrides
    const googleClientId = getEnvOptional(`${prefix}GOOGLE_CLIENT_ID`);
    const appleClientId = getEnvOptional(`${prefix}APPLE_CLIENT_ID`);
    const facebookAppId = getEnvOptional(`${prefix}FACEBOOK_APP_ID`);

    if (googleClientId || appleClientId || facebookAppId) {
      config.oauth = {};
      if (googleClientId) {
        config.oauth.google = {
          clientId: googleClientId,
          clientSecret: getEnvOptional(`${prefix}GOOGLE_CLIENT_SECRET`),
          scopes: parseCsvList(getEnvOptional(`${prefix}GOOGLE_SCOPES`, 'email,profile')),
        };
      }
      if (appleClientId) {
        config.oauth.apple = {
          clientId: appleClientId,
          teamId: getEnvOptional(`${prefix}APPLE_TEAM_ID`),
          keyId: getEnvOptional(`${prefix}APPLE_KEY_ID`),
          privateKey: getEnvOptional(`${prefix}APPLE_PRIVATE_KEY`),
        };
      }
      if (facebookAppId) {
        config.oauth.facebook = {
          appId: facebookAppId,
          appSecret: getEnvOptional(`${prefix}FACEBOOK_APP_SECRET`),
        };
      }
    }

    // WebAuthn overrides
    const webauthnRpId = getEnvOptional(`${prefix}WEBAUTHN_RP_ID`);
    if (webauthnRpId) {
      config.webauthn = {
        rpId: webauthnRpId,
        origin: getEnvOptional(`${prefix}WEBAUTHN_ORIGIN`),
        rpName: getEnvOptional(`${prefix}WEBAUTHN_RP_NAME`),
      };
    }

    configs.set(appId, config);
  }

  return configs;
}

/**
 * Load and validate configuration
 */
export function loadConfig(): AuthConfig {
  // Parse app IDs
  const appIdsRaw = getEnvOptional('AUTH_APP_IDS', 'primary');
  const appIds = parseCsvList(appIdsRaw).map(normalizeSourceAccountId);

  const config: AuthConfig = {
    port: getEnvInt('AUTH_PLUGIN_PORT', 3014),
    host: getEnvOptional('AUTH_PLUGIN_HOST', '0.0.0.0'),
    logLevel: (getEnvOptional('AUTH_LOG_LEVEL', 'info') as 'debug' | 'info' | 'warn' | 'error'),

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

    // OAuth providers
    oauth: {},

    // WebAuthn
    webauthn: {
      rpName: getEnvOptional('AUTH_WEBAUTHN_RP_NAME', 'nSelf'),
      rpId: getEnvOptional('AUTH_WEBAUTHN_RP_ID', 'localhost'),
      origin: getEnvOptional('AUTH_WEBAUTHN_ORIGIN', 'http://localhost:3014'),
    },

    // TOTP
    totp: {
      issuer: getEnvOptional('AUTH_TOTP_ISSUER', 'nSelf'),
      algorithm: getEnvOptional('AUTH_TOTP_ALGORITHM', 'SHA1'),
      digits: getEnvInt('AUTH_TOTP_DIGITS', 6),
      period: getEnvInt('AUTH_TOTP_PERIOD', 30),
      backupCodeCount: getEnvInt('AUTH_TOTP_BACKUP_CODE_COUNT', 10),
    },

    // Magic Links
    magicLink: {
      expirySeconds: getEnvInt('AUTH_MAGIC_LINK_EXPIRY_SECONDS', 600),
      baseUrl: getEnvOptional('AUTH_MAGIC_LINK_BASE_URL', 'http://localhost:3014'),
    },

    // Device Code
    deviceCode: {
      expirySeconds: getEnvInt('AUTH_DEVICE_CODE_EXPIRY_SECONDS', 600),
      pollInterval: getEnvInt('AUTH_DEVICE_CODE_POLL_INTERVAL', 5),
      codeLength: getEnvInt('AUTH_DEVICE_CODE_LENGTH', 8),
    },

    // Sessions
    session: {
      maxPerUser: getEnvInt('AUTH_SESSION_MAX_PER_USER', 10),
      idleTimeoutHours: getEnvInt('AUTH_SESSION_IDLE_TIMEOUT_HOURS', 24),
      absoluteTimeoutHours: getEnvInt('AUTH_SESSION_ABSOLUTE_TIMEOUT_HOURS', 720),
    },

    // Security
    security: {
      encryptionKey: getEnv('AUTH_ENCRYPTION_KEY'),
      loginMaxAttempts: getEnvInt('AUTH_LOGIN_MAX_ATTEMPTS', 5),
      loginLockoutMinutes: getEnvInt('AUTH_LOGIN_LOCKOUT_MINUTES', 15),
    },

    // Cleanup
    cleanup: {
      cron: getEnvOptional('AUTH_CLEANUP_CRON', '0 */6 * * *'),
    },
  };

  // Load OAuth providers (global defaults)
  const googleClientId = getEnvOptional('AUTH_GOOGLE_CLIENT_ID');
  if (googleClientId) {
    config.oauth.google = {
      clientId: googleClientId,
      clientSecret: getEnvOptional('AUTH_GOOGLE_CLIENT_SECRET'),
      scopes: parseCsvList(getEnvOptional('AUTH_GOOGLE_SCOPES', 'email,profile')),
    };
  }

  const appleClientId = getEnvOptional('AUTH_APPLE_CLIENT_ID');
  if (appleClientId) {
    config.oauth.apple = {
      clientId: appleClientId,
      teamId: getEnvOptional('AUTH_APPLE_TEAM_ID'),
      keyId: getEnvOptional('AUTH_APPLE_KEY_ID'),
      privateKey: getEnvOptional('AUTH_APPLE_PRIVATE_KEY'),
    };
  }

  const facebookAppId = getEnvOptional('AUTH_FACEBOOK_APP_ID');
  if (facebookAppId) {
    config.oauth.facebook = {
      appId: facebookAppId,
      appSecret: getEnvOptional('AUTH_FACEBOOK_APP_SECRET'),
    };
  }

  const githubClientId = getEnvOptional('AUTH_GITHUB_CLIENT_ID');
  if (githubClientId) {
    config.oauth.github = {
      clientId: githubClientId,
      clientSecret: getEnvOptional('AUTH_GITHUB_CLIENT_SECRET'),
    };
  }

  const microsoftClientId = getEnvOptional('AUTH_MICROSOFT_CLIENT_ID');
  if (microsoftClientId) {
    config.oauth.microsoft = {
      clientId: microsoftClientId,
      clientSecret: getEnvOptional('AUTH_MICROSOFT_CLIENT_SECRET'),
    };
  }

  return config;
}

/**
 * Get configuration for a specific app
 */
export function getAppConfig(config: AuthConfig, appId: string): AppAuthConfig {
  const normalizedAppId = normalizeSourceAccountId(appId);

  // Load app-specific overrides
  const appConfigs = loadAppConfigs(config.appIds);
  const appConfig = appConfigs.get(normalizedAppId);

  // Merge with global config
  return {
    id: normalizedAppId,
    oauth: { ...config.oauth, ...appConfig?.oauth },
    webauthn: { ...config.webauthn, ...appConfig?.webauthn },
  };
}

/**
 * Validate encryption key format
 */
export function validateEncryptionKey(key: string): void {
  if (key.length < 32) {
    throw new Error('AUTH_ENCRYPTION_KEY must be at least 32 characters');
  }
}

// Export singleton config instance
export const config = loadConfig();

// Validate encryption key on load
validateEncryptionKey(config.security.encryptionKey);
