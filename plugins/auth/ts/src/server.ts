/**
 * Auth Plugin Server
 * Fastify HTTP server with all API endpoints
 */

import Fastify, { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { createLogger } from '@nself/plugin-utils';
import { AuthDatabase } from './database.js';
import { AuthConfig, HealthCheckResponse, ReadyCheckResponse, LiveCheckResponse } from './types.js';

const logger = createLogger('auth:server');
const PLUGIN_VERSION = '1.0.0';

export class AuthServer {
  private app: FastifyInstance;
  private db: AuthDatabase;
  private config: AuthConfig;
  private startTime: number;

  constructor(db: AuthDatabase, config: AuthConfig) {
    this.db = db;
    this.config = config;
    this.startTime = Date.now();
    this.app = Fastify({
      logger: false,
      trustProxy: true,
    });

    this.setupRoutes();
  }

  /**
   * Setup all routes
   */
  private setupRoutes(): void {
    // Health & Status endpoints
    this.app.get('/health', this.handleHealth.bind(this));
    this.app.get('/ready', this.handleReady.bind(this));
    this.app.get('/live', this.handleLive.bind(this));

    // OAuth endpoints
    this.app.get('/api/oauth/providers', this.handleOAuthListProviders.bind(this));
    this.app.get('/api/oauth/:provider/start', this.handleOAuthStart.bind(this));
    this.app.get('/api/oauth/:provider/callback', this.handleOAuthCallback.bind(this));
    this.app.post('/api/oauth/:provider/link', this.handleOAuthLink.bind(this));
    this.app.delete('/api/oauth/:provider/unlink', this.handleOAuthUnlink.bind(this));
    this.app.get('/api/oauth/connections/:userId', this.handleOAuthConnections.bind(this));

    // WebAuthn/Passkeys endpoints
    this.app.post('/api/passkeys/register/start', this.handlePasskeyRegisterStart.bind(this));
    this.app.post('/api/passkeys/register/finish', this.handlePasskeyRegisterFinish.bind(this));
    this.app.post('/api/passkeys/authenticate/start', this.handlePasskeyAuthStart.bind(this));
    this.app.post('/api/passkeys/authenticate/finish', this.handlePasskeyAuthFinish.bind(this));
    this.app.get('/api/passkeys/:userId', this.handlePasskeysList.bind(this));
    this.app.delete('/api/passkeys/:credentialId', this.handlePasskeyDelete.bind(this));

    // TOTP 2FA endpoints
    this.app.post('/api/mfa/totp/enroll', this.handleTotpEnroll.bind(this));
    this.app.post('/api/mfa/totp/verify', this.handleTotpVerify.bind(this));
    this.app.post('/api/mfa/totp/validate', this.handleTotpValidate.bind(this));
    this.app.post('/api/mfa/backup-code/validate', this.handleBackupCodeValidate.bind(this));
    this.app.delete('/api/mfa/totp/:userId', this.handleTotpDelete.bind(this));
    this.app.get('/api/mfa/status/:userId', this.handleMfaStatus.bind(this));

    // Magic Link endpoints
    this.app.post('/api/magic-link/send', this.handleMagicLinkSend.bind(this));
    this.app.post('/api/magic-link/verify', this.handleMagicLinkVerify.bind(this));

    // Device Code endpoints
    this.app.post('/api/device-code/initiate', this.handleDeviceCodeInitiate.bind(this));
    this.app.get('/api/device-code/poll', this.handleDeviceCodePoll.bind(this));
    this.app.post('/api/device-code/authorize', this.handleDeviceCodeAuthorize.bind(this));
    this.app.post('/api/device-code/deny', this.handleDeviceCodeDeny.bind(this));

    // Session endpoints
    this.app.get('/api/sessions/:userId', this.handleSessionsList.bind(this));
    this.app.delete('/api/sessions/:sessionId', this.handleSessionRevoke.bind(this));
    this.app.delete('/api/sessions/user/:userId', this.handleSessionsRevokeAll.bind(this));

    // Login Attempts endpoints
    this.app.get('/api/login-attempts/:userId', this.handleLoginAttempts.bind(this));
  }

  // =========================================================================
  // Health & Status Handlers
  // =========================================================================

  private async handleHealth(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const response: HealthCheckResponse = {
      status: 'ok',
      plugin: 'auth',
      timestamp: new Date().toISOString(),
      version: PLUGIN_VERSION,
    };
    reply.code(200).send(response);
  }

  private async handleReady(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    let dbStatus: 'ok' | 'error' = 'ok';
    try {
      await this.db.getStats();
    } catch (error) {
      dbStatus = 'error';
    }

    const response: ReadyCheckResponse = {
      ready: dbStatus === 'ok',
      database: dbStatus,
      timestamp: new Date().toISOString(),
    };
    reply.code(dbStatus === 'ok' ? 200 : 503).send(response);
  }

  private async handleLive(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const stats = await this.db.getStats();
    const memUsage = process.memoryUsage();

    const response: LiveCheckResponse = {
      alive: true,
      uptime: Math.floor((Date.now() - this.startTime) / 1000),
      memory: {
        used: memUsage.heapUsed,
        total: memUsage.heapTotal,
      },
      stats,
    };
    reply.code(200).send(response);
  }

  // =========================================================================
  // OAuth Handlers (Stubs - full implementation requires provider SDKs)
  // =========================================================================

  private async handleOAuthListProviders(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const providers = [];
    if (this.config.oauth.google) {
      providers.push({ name: 'google', displayName: 'Google', enabled: true });
    }
    if (this.config.oauth.apple) {
      providers.push({ name: 'apple', displayName: 'Apple', enabled: true });
    }
    if (this.config.oauth.facebook) {
      providers.push({ name: 'facebook', displayName: 'Facebook', enabled: true });
    }
    if (this.config.oauth.github) {
      providers.push({ name: 'github', displayName: 'GitHub', enabled: true });
    }
    if (this.config.oauth.microsoft) {
      providers.push({ name: 'microsoft', displayName: 'Microsoft', enabled: true });
    }
    reply.send({ providers });
  }

  private async handleOAuthStart(request: FastifyRequest<{ Params: { provider: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Generate OAuth authorization URL
    reply.code(501).send({ error: 'OAuth start not implemented - requires provider SDKs' });
  }

  private async handleOAuthCallback(request: FastifyRequest<{ Params: { provider: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Handle OAuth callback
    reply.code(501).send({ error: 'OAuth callback not implemented - requires provider SDKs' });
  }

  private async handleOAuthLink(request: FastifyRequest<{ Params: { provider: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Link OAuth account to existing user
    reply.code(501).send({ error: 'OAuth link not implemented' });
  }

  private async handleOAuthUnlink(request: FastifyRequest<{ Params: { provider: string }, Body: { userId: string } }>, reply: FastifyReply): Promise<void> {
    const { provider } = request.params;
    const { userId } = request.body;
    await this.db.deleteOAuthProvider(userId, provider);
    reply.send({ success: true });
  }

  private async handleOAuthConnections(request: FastifyRequest<{ Params: { userId: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    const providers = await this.db.getOAuthProvidersByUser(userId);
    const connections = providers.map(p => ({
      provider: p.provider,
      providerEmail: p.provider_email,
      providerName: p.provider_name,
      linkedAt: p.linked_at,
      lastUsedAt: p.last_used_at,
    }));
    reply.send({ connections });
  }

  // =========================================================================
  // WebAuthn/Passkeys Handlers (Stubs - requires @simplewebauthn libraries)
  // =========================================================================

  private async handlePasskeyRegisterStart(request: FastifyRequest<{ Body: { userId: string; userName: string; userDisplayName: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Generate WebAuthn creation options
    reply.code(501).send({ error: 'Passkey registration not implemented - requires @simplewebauthn library' });
  }

  private async handlePasskeyRegisterFinish(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    // Stub: Verify and store passkey
    reply.code(501).send({ error: 'Passkey registration not implemented' });
  }

  private async handlePasskeyAuthStart(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    // Stub: Generate WebAuthn request options
    reply.code(501).send({ error: 'Passkey authentication not implemented' });
  }

  private async handlePasskeyAuthFinish(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    // Stub: Verify passkey authentication
    reply.code(501).send({ error: 'Passkey authentication not implemented' });
  }

  private async handlePasskeysList(request: FastifyRequest<{ Params: { userId: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    const passkeys = await this.db.getPasskeysByUser(userId);
    const passkeyInfos = passkeys.map(p => ({
      id: p.id,
      credentialId: p.credential_id,
      deviceType: p.device_type,
      friendlyName: p.friendly_name,
      lastUsedAt: p.last_used_at,
      createdAt: p.created_at,
    }));
    reply.send({ passkeys: passkeyInfos });
  }

  private async handlePasskeyDelete(request: FastifyRequest<{ Params: { credentialId: string } }>, reply: FastifyReply): Promise<void> {
    const { credentialId } = request.params;
    await this.db.deletePasskey(credentialId);
    reply.send({ success: true });
  }

  // =========================================================================
  // TOTP 2FA Handlers (Stubs - requires otplib or speakeasy)
  // =========================================================================

  private async handleTotpEnroll(request: FastifyRequest<{ Body: { userId: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Generate TOTP secret and QR code
    reply.code(501).send({ error: 'TOTP enrollment not implemented - requires otplib or speakeasy library' });
  }

  private async handleTotpVerify(request: FastifyRequest<{ Body: { userId: string; code: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Verify TOTP code and complete enrollment
    reply.code(501).send({ error: 'TOTP verification not implemented' });
  }

  private async handleTotpValidate(request: FastifyRequest<{ Body: { userId: string; code: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Validate TOTP code during login
    reply.code(501).send({ error: 'TOTP validation not implemented' });
  }

  private async handleBackupCodeValidate(request: FastifyRequest<{ Body: { userId: string; code: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Validate backup code
    reply.code(501).send({ error: 'Backup code validation not implemented' });
  }

  private async handleTotpDelete(request: FastifyRequest<{ Params: { userId: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    await this.db.deleteMfaEnrollment(userId, 'totp');
    reply.send({ success: true });
  }

  private async handleMfaStatus(request: FastifyRequest<{ Params: { userId: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    const enrollment = await this.db.getMfaEnrollment(userId, 'totp');
    if (!enrollment) {
      reply.send({ enrolled: false, method: null, backupCodesRemaining: 0, verified: false });
      return;
    }
    reply.send({
      enrolled: true,
      method: enrollment.method,
      backupCodesRemaining: enrollment.backup_codes_remaining,
      verified: enrollment.verified,
    });
  }

  // =========================================================================
  // Magic Link Handlers (Stubs - requires crypto and notifications integration)
  // =========================================================================

  private async handleMagicLinkSend(request: FastifyRequest<{ Body: { email: string; purpose: string; redirectUrl?: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Generate magic link and send email via notifications plugin
    reply.code(501).send({ error: 'Magic link send not implemented - requires crypto and notifications plugin integration' });
  }

  private async handleMagicLinkVerify(request: FastifyRequest<{ Body: { token: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Verify magic link token
    reply.code(501).send({ error: 'Magic link verification not implemented' });
  }

  // =========================================================================
  // Device Code Handlers (Stubs - requires crypto for code generation)
  // =========================================================================

  private async handleDeviceCodeInitiate(request: FastifyRequest<{ Body: { deviceId?: string; deviceName?: string; deviceType?: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Generate device code and user code
    reply.code(501).send({ error: 'Device code initiation not implemented - requires crypto for code generation' });
  }

  private async handleDeviceCodePoll(request: FastifyRequest<{ Querystring: { deviceCode: string } }>, reply: FastifyReply): Promise<void> {
    // Stub: Poll device code status
    const { deviceCode } = request.query;
    const record = await this.db.getDeviceCodeByCode(deviceCode);
    if (!record) {
      reply.code(404).send({ error: 'Device code not found' });
      return;
    }
    if (new Date() > record.expires_at) {
      reply.send({ status: 'expired' });
      return;
    }
    reply.send({ status: record.status });
  }

  private async handleDeviceCodeAuthorize(request: FastifyRequest<{ Body: { userCode: string; userId: string } }>, reply: FastifyReply): Promise<void> {
    const { userCode, userId } = request.body;
    const record = await this.db.getDeviceCodeByUserCode(userCode);
    if (!record) {
      reply.code(404).send({ error: 'User code not found' });
      return;
    }
    if (new Date() > record.expires_at) {
      reply.code(400).send({ error: 'Device code expired' });
      return;
    }
    await this.db.updateDeviceCodeStatus(userCode, 'authorized', userId);
    reply.send({ authorized: true, deviceName: record.device_name });
  }

  private async handleDeviceCodeDeny(request: FastifyRequest<{ Body: { userCode: string } }>, reply: FastifyReply): Promise<void> {
    const { userCode } = request.body;
    await this.db.updateDeviceCodeStatus(userCode, 'denied');
    reply.send({ denied: true });
  }

  // =========================================================================
  // Session Handlers
  // =========================================================================

  private async handleSessionsList(request: FastifyRequest<{ Params: { userId: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    const sessions = await this.db.getActiveSessions(userId);
    const sessionInfos = sessions.map(s => ({
      id: s.id,
      deviceName: s.device_name,
      deviceType: s.device_type,
      ipAddress: s.ip_address,
      location: s.location_city && s.location_country
        ? `${s.location_city}, ${s.location_country}`
        : null,
      lastActivity: s.last_activity_at,
      authMethod: s.auth_method,
      createdAt: s.created_at,
    }));
    reply.send({ sessions: sessionInfos });
  }

  private async handleSessionRevoke(request: FastifyRequest<{ Params: { sessionId: string }, Body: { reason?: string } }>, reply: FastifyReply): Promise<void> {
    const { sessionId } = request.params;
    const { reason } = request.body || {};
    await this.db.revokeSession(sessionId, reason);
    reply.send({ revoked: true });
  }

  private async handleSessionsRevokeAll(request: FastifyRequest<{ Params: { userId: string }, Body?: { exceptSessionId?: string; reason?: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    const body = request.body as { exceptSessionId?: string; reason?: string } | undefined;
    const { exceptSessionId, reason } = body || {};
    const count = await this.db.revokeAllUserSessions(userId, exceptSessionId, reason);
    reply.send({ revoked: count });
  }

  // =========================================================================
  // Login Attempts Handlers
  // =========================================================================

  private async handleLoginAttempts(request: FastifyRequest<{ Params: { userId: string }, Querystring: { limit?: string } }>, reply: FastifyReply): Promise<void> {
    const { userId } = request.params;
    const limit = parseInt(request.query.limit || '20', 10);
    const attempts = await this.db.getLoginAttempts(userId, limit);
    const attemptInfos = attempts.map(a => ({
      id: a.id,
      method: a.method,
      outcome: a.outcome,
      failureReason: a.failure_reason,
      ipAddress: a.ip_address,
      userAgent: a.user_agent,
      createdAt: a.created_at,
    }));
    reply.send({ attempts: attemptInfos });
  }

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  async start(): Promise<void> {
    try {
      await this.app.listen({
        port: this.config.port,
        host: this.config.host,
      });
      logger.success(`Auth server listening on ${this.config.host}:${this.config.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to start server', { error: message });
      throw error;
    }
  }

  async stop(): Promise<void> {
    await this.app.close();
    logger.info('Auth server stopped');
  }
}

/**
 * Create and start auth server
 */
export async function createAuthServer(db: AuthDatabase, config: AuthConfig): Promise<AuthServer> {
  const server = new AuthServer(db, config);
  return server;
}
