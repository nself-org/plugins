/**
 * Auth Plugin Types
 * All TypeScript interfaces for the auth plugin
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface OAuthProviderRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  provider: 'google' | 'apple' | 'facebook' | 'github' | 'microsoft';
  provider_user_id: string;
  provider_email: string | null;
  provider_name: string | null;
  provider_avatar_url: string | null;
  access_token_encrypted: string | null;
  refresh_token_encrypted: string | null;
  token_expires_at: Date | null;
  scopes: string[];
  raw_profile: Record<string, unknown>;
  linked_at: Date;
  last_used_at: Date | null;
}

export interface PasskeyRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  credential_id: string;
  public_key: string;
  counter: number;
  device_type: string | null;
  backed_up: boolean;
  transports: string | null;
  friendly_name: string | null;
  last_used_at: Date | null;
  created_at: Date;
}

// Alias for database row type
export type PasskeyRow = PasskeyRecord;

export interface MfaEnrollmentRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  method: 'totp' | 'sms' | 'email';
  secret_encrypted: string;
  algorithm: string;
  digits: number;
  period: number;
  verified: boolean;
  backup_codes_encrypted: string | null;
  backup_codes_remaining: number;
  enabled: boolean;
  last_used_at: Date | null;
  created_at: Date;
}

export interface DeviceCodeRecord {
  id: string;
  source_account_id: string;
  device_code: string;
  user_code: string;
  device_id: string | null;
  device_name: string | null;
  device_type: string | null;
  scopes: string[];
  status: 'pending' | 'authorized' | 'denied' | 'expired';
  user_id: string | null;
  authorized_at: Date | null;
  expires_at: Date;
  poll_interval: number;
  created_at: Date;
}

export interface MagicLinkRecord {
  id: string;
  source_account_id: string;
  email: string;
  token_hash: string;
  purpose: 'login' | 'verify' | 'reset';
  used: boolean;
  used_at: Date | null;
  expires_at: Date;
  ip_address: string | null;
  created_at: Date;
}

export interface SessionRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  device_id: string | null;
  device_name: string | null;
  device_type: string | null;
  ip_address: string | null;
  user_agent: string | null;
  location_city: string | null;
  location_country: string | null;
  auth_method: 'password' | 'oauth' | 'passkey' | 'magic_link' | 'device_code' | 'mfa';
  token_hash: string | null;
  is_active: boolean;
  last_activity_at: Date;
  expires_at: Date | null;
  revoked_at: Date | null;
  revoked_reason: string | null;
  created_at: Date;
}

export interface LoginAttemptRecord {
  id: string;
  source_account_id: string;
  email: string | null;
  user_id: string | null;
  ip_address: string | null;
  method: 'password' | 'oauth' | 'passkey' | 'magic_link' | 'device_code' | 'mfa';
  outcome: 'success' | 'failure' | 'blocked';
  failure_reason: string | null;
  user_agent: string | null;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

// OAuth
export interface OAuthProvider {
  name: string;
  displayName: string;
  authUrl: string;
  enabled: boolean;
}

export interface OAuthStartRequest {
  redirectUri: string;
  state?: string;
  scopes?: string[];
}

export interface OAuthStartResponse {
  authorizationUrl: string;
  state: string;
}

export interface OAuthCallbackRequest {
  code: string;
  state: string;
}

export interface OAuthCallbackResponse {
  userId: string;
  provider: string;
  providerEmail: string | null;
  providerName: string | null;
  providerAvatarUrl: string | null;
  accessToken?: string;
  refreshToken?: string;
}

export interface OAuthLinkRequest {
  userId: string;
  code: string;
  redirectUri: string;
}

export interface OAuthUnlinkRequest {
  userId: string;
}

export interface OAuthConnection {
  provider: string;
  providerEmail: string | null;
  providerName: string | null;
  linkedAt: Date;
  lastUsedAt: Date | null;
}

// WebAuthn/Passkeys
export interface PasskeyRegisterStartRequest {
  userId: string;
  userName: string;
  userDisplayName: string;
}

// WebAuthn type declarations (simplified - full implementation would use @simplewebauthn)
export type PublicKeyCredentialCreationOptions = Record<string, unknown>;
export type PublicKeyCredentialRequestOptions = Record<string, unknown>;
export type PublicKeyCredential = Record<string, unknown>;

export interface PasskeyRegisterStartResponse {
  options: PublicKeyCredentialCreationOptions;
}

export interface PasskeyRegisterFinishRequest {
  userId: string;
  credential: PublicKeyCredential;
  friendlyName?: string;
}

export interface PasskeyRegisterFinishResponse {
  credentialId: string;
  deviceType: string | null;
}

export interface PasskeyAuthenticateStartRequest {
  userId?: string;
}

export interface PasskeyAuthenticateStartResponse {
  options: PublicKeyCredentialRequestOptions;
}

export interface PasskeyAuthenticateFinishRequest {
  credential: PublicKeyCredential;
}

export interface PasskeyAuthenticateFinishResponse {
  userId: string;
  accessToken: string;
  refreshToken: string;
}

export interface PasskeyInfo {
  id: string;
  credentialId: string;
  deviceType: string | null;
  friendlyName: string | null;
  lastUsedAt: Date | null;
  createdAt: Date;
}

// TOTP 2FA
export interface TotpEnrollRequest {
  userId: string;
}

export interface TotpEnrollResponse {
  secret: string;
  qrCodeDataUrl: string;
  otpauthUrl: string;
  backupCodes: string[];
}

export interface TotpVerifyRequest {
  userId: string;
  code: string;
}

export interface TotpVerifyResponse {
  valid: boolean;
  enrolled?: boolean;
}

export interface BackupCodeValidateRequest {
  userId: string;
  code: string;
}

export interface BackupCodeValidateResponse {
  valid: boolean;
  remainingBackupCodes: number;
}

export interface MfaStatusResponse {
  enrolled: boolean;
  method: string | null;
  backupCodesRemaining: number;
  verified: boolean;
}

// Magic Links
export interface MagicLinkSendRequest {
  email: string;
  purpose: 'login' | 'verify' | 'reset';
  redirectUrl?: string;
}

export interface MagicLinkSendResponse {
  sent: boolean;
  expiresIn: number;
}

export interface MagicLinkVerifyRequest {
  token: string;
}

export interface MagicLinkVerifyResponse {
  valid: boolean;
  userId?: string;
  email: string;
  purpose: string;
}

// Device Code Flow
export interface DeviceCodeInitiateRequest {
  deviceId?: string;
  deviceName?: string;
  deviceType?: string;
}

export interface DeviceCodeInitiateResponse {
  deviceCode: string;
  userCode: string;
  verificationUrl: string;
  expiresIn: number;
  pollInterval: number;
}

export interface DeviceCodePollResponse {
  status: 'pending' | 'authorized' | 'expired' | 'denied';
  userId?: string;
  accessToken?: string;
  refreshToken?: string;
}

export interface DeviceCodeAuthorizeRequest {
  userCode: string;
  userId: string;
}

export interface DeviceCodeAuthorizeResponse {
  authorized: boolean;
  deviceName: string | null;
}

export interface DeviceCodeDenyRequest {
  userCode: string;
}

// Sessions
export interface SessionInfo {
  id: string;
  deviceName: string | null;
  deviceType: string | null;
  ipAddress: string | null;
  location: string | null;
  lastActivity: Date;
  authMethod: string;
  createdAt: Date;
  isCurrentSession?: boolean;
}

export interface SessionsResponse {
  sessions: SessionInfo[];
}

export interface SessionRevokeRequest {
  reason?: string;
}

export interface SessionsRevokeAllRequest {
  exceptSessionId?: string;
  reason?: string;
}

// Login Attempts
export interface LoginAttemptInfo {
  id: string;
  method: string;
  outcome: string;
  failureReason: string | null;
  ipAddress: string | null;
  userAgent: string | null;
  createdAt: Date;
}

export interface LoginAttemptsResponse {
  attempts: LoginAttemptInfo[];
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface AuthConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  // Database
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  // Multi-app
  appIds: string[];

  // OAuth providers
  oauth: {
    google?: {
      clientId: string;
      clientSecret: string;
      scopes: string[];
    };
    apple?: {
      clientId: string;
      teamId: string;
      keyId: string;
      privateKey: string;
    };
    facebook?: {
      appId: string;
      appSecret: string;
    };
    github?: {
      clientId: string;
      clientSecret: string;
    };
    microsoft?: {
      clientId: string;
      clientSecret: string;
    };
  };

  // WebAuthn
  webauthn: {
    rpName: string;
    rpId: string;
    origin: string;
    timeout: number;
  };

  // TOTP
  totp: {
    issuer: string;
    algorithm: string;
    digits: number;
    period: number;
    backupCodeCount: number;
  };

  // Magic Links
  magicLink: {
    expirySeconds: number;
    baseUrl: string;
  };

  // Device Code
  deviceCode: {
    expirySeconds: number;
    pollInterval: number;
    codeLength: number;
  };

  // Sessions
  session: {
    maxPerUser: number;
    idleTimeoutHours: number;
    absoluteTimeoutHours: number;
  };

  // Security
  security: {
    encryptionKey: string;
    loginMaxAttempts: number;
    loginLockoutMinutes: number;
  };

  // JWT Tokens
  jwt: {
    accessTokenSecret: string;
    refreshTokenSecret: string;
    accessTokenExpiresIn: string;
    refreshTokenExpiresIn: string;
  };

  // Email
  email: {
    notificationsUrl: string;
    fromEmail: string;
    fromName: string;
  };

  // Cleanup
  cleanup: {
    cron: string;
  };
}

export interface AppAuthConfig {
  id: string;
  oauth?: Partial<AuthConfig['oauth']>;
  webauthn?: Partial<AuthConfig['webauthn']>;
}

// ============================================================================
// Service Types
// ============================================================================

export interface AuthStats {
  oauthProviders: number;
  passkeys: number;
  mfaEnrollments: number;
  activeSessions: number;
  activeDeviceCodes: number;
  pendingMagicLinks: number;
  recentLoginAttempts: number;
}

export interface HealthCheckResponse {
  status: 'ok' | 'error';
  plugin: string;
  timestamp: string;
  version: string;
}

export interface ReadyCheckResponse {
  ready: boolean;
  database: 'ok' | 'error';
  timestamp: string;
}

export interface LiveCheckResponse {
  alive: boolean;
  uptime: number;
  memory: {
    used: number;
    total: number;
  };
  stats: AuthStats;
}
