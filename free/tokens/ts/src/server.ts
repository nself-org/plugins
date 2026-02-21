#!/usr/bin/env node
/**
 * Tokens Plugin HTTP Server
 * REST API endpoints for secure content delivery tokens
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createHmac, randomBytes, createCipheriv, createDecipheriv } from 'crypto';
import {
  createLogger,
  createDatabase,
  ApiRateLimiter,
  createRateLimitHook,
  createAuthHook,
  getAppContext,
} from '@nself/plugin-utils';
import { config } from './config.js';
import { TokensDatabase } from './database.js';
import type {
  HealthCheckResponse,
  ReadyCheckResponse,
  LiveCheckResponse,
  IssueTokenRequest,
  IssueTokenResponse,
  ValidateTokenRequest,
  ValidateTokenResponse,
  RevokeTokenRequest,
  RevokeUserTokensRequest,
  RevokeContentTokensRequest,
  CreateSigningKeyRequest,
  RotateKeyRequest,
  CreateEncryptionKeyRequest,
  CreateEncryptionKeyResponse,
  RotateEncryptionKeyRequest,
  CheckEntitlementRequest,
  CheckEntitlementResponse,
  GrantEntitlementRequest,
  RevokeEntitlementRequest,
} from './types.js';

const logger = createLogger('tokens:server');
const PLUGIN_VERSION = '1.0.0';

const fastify = Fastify({ logger: false, bodyLimit: 10485760 });

let tokensDb: TokensDatabase;

// ============================================================================
// Crypto Helpers
// ============================================================================

function encryptKeyMaterial(plaintext: string): string {
  const iv = randomBytes(16);
  const key = createHmac('sha256', config.encryptionKey).update('key-material').digest();
  const cipher = createCipheriv('aes-256-cbc', key, iv);
  let encrypted = cipher.update(plaintext, 'utf8', 'hex');
  encrypted += cipher.final('hex');
  return `${iv.toString('hex')}:${encrypted}`;
}

function decryptKeyMaterial(ciphertext: string): string {
  const [ivHex, encrypted] = ciphertext.split(':');
  const iv = Buffer.from(ivHex, 'hex');
  const key = createHmac('sha256', config.encryptionKey).update('key-material').digest();
  const decipher = createDecipheriv('aes-256-cbc', key, iv);
  let decrypted = decipher.update(encrypted, 'hex', 'utf8');
  decrypted += decipher.final('utf8');
  return decrypted;
}

function generateToken(payload: Record<string, unknown>, signingKey: string): string {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const body = Buffer.from(JSON.stringify(payload)).toString('base64url');
  const signature = createHmac('sha256', signingKey).update(`${header}.${body}`).digest('base64url');
  return `${header}.${body}.${signature}`;
}

function hashToken(token: string): string {
  return createHmac('sha256', 'token-hash').update(token).digest('hex');
}

// ============================================================================
// Middleware Setup
// ============================================================================

async function setupMiddleware(): Promise<void> {
  await fastify.register(cors, { origin: true });

  const rateLimiter = new ApiRateLimiter(
    config.security.rateLimitMax ?? 100,
    config.security.rateLimitWindowMs ?? 60000
  );
  fastify.addHook('preHandler', createRateLimitHook(rateLimiter));
  fastify.addHook('preHandler', createAuthHook(config.security.apiKey));
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return { status: 'ok', plugin: 'tokens', timestamp: new Date().toISOString(), version: PLUGIN_VERSION };
});

fastify.get('/ready', async (): Promise<ReadyCheckResponse> => {
  let dbStatus: 'ok' | 'error' = 'ok';
  try { await tokensDb.getStats(); } catch { dbStatus = 'error'; }
  return { ready: dbStatus === 'ok', database: dbStatus, timestamp: new Date().toISOString() };
});

fastify.get('/live', async (): Promise<LiveCheckResponse> => {
  const stats = await tokensDb.getStats();
  return {
    alive: true, uptime: process.uptime(),
    memory: { used: process.memoryUsage().heapUsed, total: process.memoryUsage().heapTotal },
    stats,
  };
});

// ============================================================================
// Token Issuance Endpoints
// ============================================================================

fastify.post<{ Body: IssueTokenRequest }>('/api/issue', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);
  const body = request.body;

  // Check entitlements if enabled
  if (config.defaultEntitlementCheck) {
    const entitlement = await scopedDb.checkEntitlement(body.userId, body.contentId, body.tokenType ?? 'playback');
    if (!entitlement) {
      // Check if user has any entitlements at all
      const hasEntitlements = await scopedDb.hasAnyEntitlements(body.userId);
      if (hasEntitlements || !config.allowAllIfNoEntitlements) {
        await scopedDb.insertWebhookEvent(
          `tokens.access.denied-${body.userId}-${body.contentId}-${Date.now()}`,
          'tokens.access.denied',
          { userId: body.userId, contentId: body.contentId, reason: 'no_entitlement' }
        );
        reply.code(403);
        throw new Error('Access denied: no valid entitlement');
      }
    }
  }

  // Get active signing key
  const signingKey = await scopedDb.getActiveSigningKey();
  if (!signingKey) {
    reply.code(500);
    throw new Error('No active signing key configured. Create one first.');
  }

  const keyMaterial = decryptKeyMaterial(signingKey.key_material_encrypted);
  const ttl = Math.min(body.ttlSeconds ?? config.defaultTtlSeconds, config.maxTtlSeconds);
  const expiresAt = new Date(Date.now() + ttl * 1000);

  const payload: Record<string, unknown> = {
    sub: body.userId,
    cid: body.contentId,
    typ: body.tokenType ?? 'playback',
    exp: Math.floor(expiresAt.getTime() / 1000),
    iat: Math.floor(Date.now() / 1000),
    perm: body.permissions ?? {},
  };

  if (body.deviceId) payload.did = body.deviceId;
  if (body.ipRestriction) payload.ip = body.ipRestriction;
  if (body.contentType) payload.ctype = body.contentType;

  const token = generateToken(payload, keyMaterial);
  const tokenHash = hashToken(token);

  const issued = await scopedDb.insertIssuedToken({
    token_hash: tokenHash,
    token_type: body.tokenType ?? 'playback',
    signing_key_id: signingKey.id,
    user_id: body.userId,
    device_id: body.deviceId ?? null,
    content_id: body.contentId,
    content_type: body.contentType ?? null,
    permissions: body.permissions ?? {},
    ip_address: body.ipRestriction ?? null,
    expires_at: expiresAt,
  });

  await scopedDb.insertWebhookEvent(
    `tokens.issued-${issued.id}`,
    'tokens.issued',
    { tokenId: issued.id, userId: body.userId, contentId: body.contentId }
  );

  const response: IssueTokenResponse = {
    token,
    expiresAt: expiresAt.toISOString(),
    tokenId: issued.id,
  };

  return response;
});

fastify.post<{ Body: ValidateTokenRequest }>('/api/validate', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);
  const { token, contentId, ipAddress } = request.body;

  const tokenHash = hashToken(token);
  const issued = await scopedDb.getIssuedTokenByHash(tokenHash);

  if (!issued) {
    const response: ValidateTokenResponse = { valid: false };
    return response;
  }

  if (issued.revoked) {
    return { valid: false } as ValidateTokenResponse;
  }

  if (new Date() > issued.expires_at) {
    return { valid: false } as ValidateTokenResponse;
  }

  // Check content ID restriction
  if (contentId && issued.content_id !== contentId) {
    return { valid: false } as ValidateTokenResponse;
  }

  // Check IP restriction
  if (issued.ip_address && ipAddress && issued.ip_address !== ipAddress) {
    return { valid: false } as ValidateTokenResponse;
  }

  // Update last used
  await scopedDb.updateTokenLastUsed(issued.id);

  await scopedDb.insertWebhookEvent(
    `tokens.validated-${issued.id}-${Date.now()}`,
    'tokens.validated',
    { tokenId: issued.id, userId: issued.user_id }
  );

  const response: ValidateTokenResponse = {
    valid: true,
    userId: issued.user_id,
    contentId: issued.content_id,
    permissions: issued.permissions,
    expiresAt: issued.expires_at.toISOString(),
  };

  return response;
});

fastify.post<{ Body: RevokeTokenRequest }>('/api/revoke', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);
  const { tokenId, reason } = request.body;

  if (!tokenId) {
    reply.code(400);
    throw new Error('tokenId is required');
  }

  await scopedDb.revokeToken(tokenId, reason);

  await scopedDb.insertWebhookEvent(
    `tokens.revoked-${tokenId}`,
    'tokens.revoked',
    { tokenId, reason }
  );

  return { revoked: true, tokenId };
});

fastify.post<{ Body: RevokeUserTokensRequest }>('/api/revoke/user', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);
  const { userId, reason } = request.body;

  const count = await scopedDb.revokeUserTokens(userId, reason);

  await scopedDb.insertWebhookEvent(
    `tokens.revoked-user-${userId}-${Date.now()}`,
    'tokens.revoked',
    { userId, reason, count }
  );

  return { revoked: count, userId };
});

fastify.post<{ Body: RevokeContentTokensRequest }>('/api/revoke/content', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);
  const { contentId, reason } = request.body;

  const count = await scopedDb.revokeContentTokens(contentId, reason);

  await scopedDb.insertWebhookEvent(
    `tokens.revoked-content-${contentId}-${Date.now()}`,
    'tokens.revoked',
    { contentId, reason, count }
  );

  return { revoked: count, contentId };
});

// ============================================================================
// Signing Keys Endpoints
// ============================================================================

fastify.post<{ Body: CreateSigningKeyRequest }>('/api/keys', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const rawKey = randomBytes(32).toString('hex');
  const encryptedKey = encryptKeyMaterial(rawKey);

  const key = await scopedDb.createSigningKey(
    request.body.name,
    request.body.algorithm ?? config.signingAlgorithm,
    encryptedKey
  );

  return {
    id: key.id,
    name: key.name,
    algorithm: key.algorithm,
    isActive: key.is_active,
    createdAt: key.created_at.toISOString(),
  };
});

fastify.get('/api/keys', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const keys = await scopedDb.listSigningKeys();
  return {
    keys: keys.map(k => ({
      id: k.id,
      name: k.name,
      algorithm: k.algorithm,
      isActive: k.is_active,
      rotatedFrom: k.rotated_from,
      createdAt: k.created_at.toISOString(),
      rotatedAt: k.rotated_at?.toISOString() ?? null,
      expiresAt: k.expires_at?.toISOString() ?? null,
    })),
  };
});

fastify.post<{ Params: { id: string }; Body: RotateKeyRequest }>('/api/keys/:id/rotate', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const rawKey = randomBytes(32).toString('hex');
  const encryptedKey = encryptKeyMaterial(rawKey);

  try {
    const newKey = await scopedDb.rotateSigningKey(
      request.params.id,
      encryptedKey,
      request.body.expireOldAfterHours ?? 24
    );

    await scopedDb.insertWebhookEvent(
      `tokens.key.rotated-${newKey.id}`,
      'tokens.key.rotated',
      { oldKeyId: request.params.id, newKeyId: newKey.id }
    );

    return {
      id: newKey.id,
      name: newKey.name,
      algorithm: newKey.algorithm,
      isActive: newKey.is_active,
      rotatedFrom: newKey.rotated_from,
      createdAt: newKey.created_at.toISOString(),
    };
  } catch (error) {
    reply.code(404);
    throw new Error('Signing key not found');
  }
});

fastify.delete<{ Params: { id: string } }>('/api/keys/:id', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  await scopedDb.deactivateSigningKey(request.params.id);
  reply.code(204);
});

// ============================================================================
// Encryption Keys Endpoints
// ============================================================================

fastify.post<{ Body: CreateEncryptionKeyRequest }>('/api/encryption/keys', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const rawKey = randomBytes(16);
  const iv = randomBytes(16);
  const encryptedKey = encryptKeyMaterial(rawKey.toString('hex'));
  const keyUri = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}/api/encryption/keys/PLACEHOLDER/deliver`;

  const key = await scopedDb.createEncryptionKey(
    request.body.contentId,
    encryptedKey,
    iv.toString('hex'),
    keyUri
  );

  // Update key URI with actual ID
  const actualKeyUri = keyUri.replace('PLACEHOLDER', key.id);

  await scopedDb.insertWebhookEvent(
    `tokens.encryption.key.created-${key.id}`,
    'tokens.encryption.key.created',
    { keyId: key.id, contentId: request.body.contentId }
  );

  const response: CreateEncryptionKeyResponse = { keyId: key.id, keyUri: actualKeyUri };
  return response;
});

fastify.get<{ Params: { id: string } }>('/api/encryption/keys/:id/deliver', async (request, reply) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const key = await scopedDb.getEncryptionKeyById(request.params.id);
  if (!key || !key.is_active) {
    reply.code(404);
    throw new Error('Encryption key not found or inactive');
  }

  // Decrypt key material and return raw bytes
  const hexKey = decryptKeyMaterial(key.key_material_encrypted);
  const rawKey = Buffer.from(hexKey, 'hex');

  reply.header('Content-Type', 'application/octet-stream');
  reply.header('Content-Length', rawKey.length.toString());
  return reply.send(rawKey);
});

fastify.post<{ Params: { contentId: string }; Body: RotateEncryptionKeyRequest }>('/api/encryption/keys/:contentId/rotate', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const rawKey = randomBytes(16);
  const iv = randomBytes(16);
  const encryptedKey = encryptKeyMaterial(rawKey.toString('hex'));
  const keyUri = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}/api/encryption/keys/PLACEHOLDER/deliver`;

  const newKey = await scopedDb.rotateEncryptionKey(
    request.params.contentId,
    encryptedKey,
    iv.toString('hex'),
    keyUri,
    request.body.expireOldAfterHours ?? 24
  );

  const actualKeyUri = keyUri.replace('PLACEHOLDER', newKey.id);

  await scopedDb.insertWebhookEvent(
    `tokens.encryption.key.rotated-${newKey.id}`,
    'tokens.encryption.key.rotated',
    { keyId: newKey.id, contentId: request.params.contentId }
  );

  return { keyId: newKey.id, keyUri: actualKeyUri, generation: newKey.rotation_generation };
});

// ============================================================================
// Entitlements Endpoints
// ============================================================================

fastify.post<{ Body: CheckEntitlementRequest }>('/api/entitlements/check', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);
  const { userId, contentId, entitlementType } = request.body;

  const entitlement = await scopedDb.checkEntitlement(userId, contentId, entitlementType);

  if (entitlement) {
    const response: CheckEntitlementResponse = {
      allowed: true,
      reason: 'entitlement_active',
      restrictions: entitlement.metadata.restrictions as Record<string, unknown> | undefined,
      expiresAt: entitlement.expires_at?.toISOString(),
    };
    return response;
  }

  // Check if user has any entitlements
  const hasEntitlements = await scopedDb.hasAnyEntitlements(userId);
  if (!hasEntitlements && config.allowAllIfNoEntitlements) {
    return { allowed: true, reason: 'no_entitlements_mode' } as CheckEntitlementResponse;
  }

  await scopedDb.insertWebhookEvent(
    `tokens.access.denied-${userId}-${contentId}-${Date.now()}`,
    'tokens.access.denied',
    { userId, contentId, entitlementType }
  );

  return { allowed: false, reason: 'no_valid_entitlement' } as CheckEntitlementResponse;
});

fastify.post<{ Body: GrantEntitlementRequest }>('/api/entitlements', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const entitlement = await scopedDb.grantEntitlement({
    user_id: request.body.userId,
    content_id: request.body.contentId,
    content_type: request.body.contentType ?? null,
    entitlement_type: request.body.entitlementType,
    expires_at: request.body.expiresAt ? new Date(request.body.expiresAt) : null,
    metadata: request.body.metadata ?? {},
    granted_by: 'api',
  });

  await scopedDb.insertWebhookEvent(
    `tokens.entitlement.granted-${entitlement.id}`,
    'tokens.entitlement.granted',
    { entitlementId: entitlement.id, userId: request.body.userId, contentId: request.body.contentId }
  );

  return entitlement;
});

fastify.delete<{ Body: RevokeEntitlementRequest }>('/api/entitlements', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  await scopedDb.revokeEntitlement(request.body.userId, request.body.contentId, request.body.entitlementType);

  await scopedDb.insertWebhookEvent(
    `tokens.entitlement.revoked-${request.body.userId}-${request.body.contentId}-${Date.now()}`,
    'tokens.entitlement.revoked',
    { userId: request.body.userId, contentId: request.body.contentId }
  );

  return { revoked: true };
});

fastify.get<{ Params: { userId: string }; Querystring: { contentType?: string; active?: string } }>('/api/entitlements/:userId', async (request) => {
  const { sourceAccountId } = getAppContext(request);
  const scopedDb = tokensDb.forSourceAccount(sourceAccountId);

  const activeOnly = request.query.active !== 'false';
  const entitlements = await scopedDb.listUserEntitlements(
    request.params.userId,
    request.query.contentType,
    activeOnly
  );

  return { entitlements };
});

// ============================================================================
// Server Startup
// ============================================================================

async function start(): Promise<void> {
  try {
    await setupMiddleware();

    const db = createDatabase(config.database);
    await db.connect();
    tokensDb = new TokensDatabase(db);

    logger.info('Tokens database connection established');

    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`Tokens plugin server listening on ${config.host}:${config.port}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start tokens server', { error: message });
    process.exit(1);
  }
}

process.on('SIGINT', async () => {
  logger.info('Received SIGINT, shutting down...');
  await fastify.close();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  logger.info('Received SIGTERM, shutting down...');
  await fastify.close();
  process.exit(0);
});

const isMainModule = process.argv[1] && (
  process.argv[1].endsWith('server.ts') ||
  process.argv[1].endsWith('server.js')
);

if (isMainModule) {
  start();
}

export { fastify, start };
