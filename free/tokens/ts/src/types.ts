/**
 * Tokens Plugin Types
 * All TypeScript interfaces for the secure content delivery token service
 */

// ============================================================================
// Database Record Types
// ============================================================================

export interface TokensSigningKeyRecord {
  id: string;
  source_account_id: string;
  name: string;
  algorithm: string;
  key_material_encrypted: string;
  is_active: boolean;
  rotated_from: string | null;
  created_at: Date;
  rotated_at: Date | null;
  expires_at: Date | null;
}

export interface TokensIssuedRecord {
  id: string;
  source_account_id: string;
  token_hash: string;
  token_type: string;
  signing_key_id: string | null;
  user_id: string;
  device_id: string | null;
  content_id: string;
  content_type: string | null;
  permissions: Record<string, unknown>;
  ip_address: string | null;
  issued_at: Date;
  expires_at: Date;
  revoked: boolean;
  revoked_at: Date | null;
  revoked_reason: string | null;
  last_used_at: Date | null;
  use_count: number;
}

export interface TokensEncryptionKeyRecord {
  id: string;
  source_account_id: string;
  content_id: string;
  key_material_encrypted: string;
  key_iv: string;
  key_uri: string;
  rotation_generation: number;
  is_active: boolean;
  created_at: Date;
  rotated_at: Date | null;
  expires_at: Date | null;
}

export interface TokensEntitlementRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  content_id: string;
  content_type: string | null;
  entitlement_type: string;
  granted_by: string;
  granted_at: Date;
  expires_at: Date | null;
  revoked: boolean;
  metadata: Record<string, unknown>;
}

export interface TokensWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  retry_count: number;
  created_at: Date;
}

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface IssueTokenRequest {
  userId: string;
  deviceId?: string;
  contentId: string;
  contentType?: 'video' | 'audio' | 'file' | 'document';
  tokenType?: 'playback' | 'download' | 'preview';
  ttlSeconds?: number;
  permissions?: Record<string, unknown>;
  ipRestriction?: string;
}

export interface IssueTokenResponse {
  token: string;
  signedUrl?: string;
  expiresAt: string;
  tokenId: string;
}

export interface ValidateTokenRequest {
  token: string;
  contentId?: string;
  ipAddress?: string;
}

export interface ValidateTokenResponse {
  valid: boolean;
  userId?: string;
  contentId?: string;
  permissions?: Record<string, unknown>;
  expiresAt?: string;
}

export interface RevokeTokenRequest {
  tokenId?: string;
  reason?: string;
}

export interface RevokeUserTokensRequest {
  userId: string;
  reason?: string;
}

export interface RevokeContentTokensRequest {
  contentId: string;
  reason?: string;
}

export interface CreateSigningKeyRequest {
  name: string;
  algorithm?: string;
}

export interface RotateKeyRequest {
  expireOldAfterHours?: number;
}

export interface CreateEncryptionKeyRequest {
  contentId: string;
}

export interface CreateEncryptionKeyResponse {
  keyId: string;
  keyUri: string;
}

export interface RotateEncryptionKeyRequest {
  expireOldAfterHours?: number;
}

export interface CheckEntitlementRequest {
  userId: string;
  contentId: string;
  entitlementType: 'stream' | 'download';
  deviceId?: string;
}

export interface CheckEntitlementResponse {
  allowed: boolean;
  reason?: string;
  restrictions?: Record<string, unknown>;
  expiresAt?: string;
}

export interface GrantEntitlementRequest {
  userId: string;
  contentId: string;
  contentType?: string;
  entitlementType: string;
  expiresAt?: string;
  metadata?: Record<string, unknown>;
}

export interface RevokeEntitlementRequest {
  userId: string;
  contentId: string;
  entitlementType: string;
}

// ============================================================================
// Configuration Types
// ============================================================================

export interface TokensConfig {
  port: number;
  host: string;
  logLevel: 'debug' | 'info' | 'warn' | 'error';

  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl: boolean;
  };

  appIds: string[];

  // Encryption
  encryptionKey: string;

  // Token defaults
  defaultTtlSeconds: number;
  maxTtlSeconds: number;
  signingAlgorithm: string;

  // HLS encryption
  hlsEncryptionEnabled: boolean;
  hlsKeyRotationHours: number;

  // Entitlement defaults
  defaultEntitlementCheck: boolean;
  allowAllIfNoEntitlements: boolean;

  // Cleanup
  expiredRetentionDays: number;

  // Security
  security: SecurityConfig;
}

export interface SecurityConfig {
  apiKey?: string;
  rateLimitMax?: number;
  rateLimitWindowMs?: number;
}

// ============================================================================
// Health/Status Types
// ============================================================================

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
  stats: TokensStats;
}

export interface TokensStats {
  totalSigningKeys: number;
  activeSigningKeys: number;
  totalTokensIssued: number;
  activeTokens: number;
  revokedTokens: number;
  expiredTokens: number;
  totalEncryptionKeys: number;
  totalEntitlements: number;
  activeEntitlements: number;
}
