/**
 * Auth Plugin Server
 * Fastify HTTP server with all API endpoints
 */

import Fastify, { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import { createLogger } from '@nself/plugin-utils';
import { AuthDatabase } from './database.js';
import { TotpService } from './totp.js';
import { TokenService } from './tokens.js';
import { DeviceCodeService } from './device-code.js';
import { MagicLinkService } from './magic-links.js';
import { WebAuthnService } from './webauthn.js';
import { OAuthService, OAuthProvider } from './oauth.js';
import { encrypt, decrypt, encryptArray, decryptArray } from './crypto.js';
import { AuthConfig, HealthCheckResponse, ReadyCheckResponse, LiveCheckResponse } from './types.js';
import type { RegistrationResponseJSON, AuthenticationResponseJSON } from '@simplewebauthn/server';

const logger = createLogger('auth:server');
const PLUGIN_VERSION = '1.0.0';

export class AuthServer {
  private app: FastifyInstance;
  private db: AuthDatabase;
  private config: AuthConfig;
  private startTime: number;
  private totpService: TotpService;
  private tokenService: TokenService;
  private deviceCodeService: DeviceCodeService;
  private magicLinkService: MagicLinkService;
  private webAuthnService: WebAuthnService;
  private oauthService: OAuthService;
  private challenges: Map<string, { challenge: string; userId?: string; timestamp: number }>;

  constructor(db: AuthDatabase, config: AuthConfig) {
    this.db = db;
    this.config = config;
    this.startTime = Date.now();
    this.totpService = new TotpService(config);
    this.tokenService = new TokenService(config.jwt);
    this.deviceCodeService = new DeviceCodeService(config);
    this.magicLinkService = new MagicLinkService(config);
    this.webAuthnService = new WebAuthnService(config);
    this.oauthService = new OAuthService(config, db);
    this.challenges = new Map();
    this.app = Fastify({
      logger: false,
      trustProxy: true,
    });

    this.setupRoutes();
    this.startChallengeCleanup();
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
  // OAuth Handlers
  // =========================================================================

  private async handleOAuthListProviders(request: FastifyRequest, reply: FastifyReply): Promise<void> {
    const providers = this.oauthService.getEnabledProviders().map(p => ({
      name: p.name,
      displayName: p.displayName,
      enabled: true,
    }));
    reply.send({ providers });
  }

  private async handleOAuthStart(request: FastifyRequest<{ Params: { provider: string }, Querystring: { redirectUri?: string; state?: string; scopes?: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { provider } = request.params;
      const { redirectUri, state, scopes } = request.query;

      if (!this.oauthService.isProviderEnabled(provider as OAuthProvider)) {
        reply.code(400).send({ error: `Provider ${provider} is not configured` });
        return;
      }

      const callbackUrl = `${this.config.magicLink.baseUrl}/api/oauth/${provider}/callback`;
      const scopeList = scopes ? scopes.split(',') : undefined;

      const authUrl = await this.oauthService.getAuthorizationUrl(
        provider as OAuthProvider,
        redirectUri || callbackUrl,
        state,
        scopeList
      );

      reply.send({
        authorizationUrl: authUrl.url,
        state: authUrl.state,
      });
    } catch (error) {
      logger.error('OAuth start error', { error });
      reply.code(500).send({ error: 'Failed to start OAuth flow' });
    }
  }

  private async handleOAuthCallback(request: FastifyRequest<{ Params: { provider: string }, Querystring: { code?: string; state?: string; error?: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { provider } = request.params;
      const { code, state, error } = request.query;

      if (error) {
        logger.warn('OAuth callback error from provider', { provider, error });
        reply.code(400).send({ error: `OAuth error: ${error}` });
        return;
      }

      if (!code) {
        reply.code(400).send({ error: 'Missing authorization code' });
        return;
      }

      if (!this.oauthService.isProviderEnabled(provider as OAuthProvider)) {
        reply.code(400).send({ error: `Provider ${provider} is not configured` });
        return;
      }

      const callbackUrl = `${this.config.magicLink.baseUrl}/api/oauth/${provider}/callback`;
      const result = await this.oauthService.handleCallback(
        provider as OAuthProvider,
        code,
        callbackUrl
      );

      // Check if user exists with this provider
      const existingProvider = await this.db.getOAuthProviderByProviderUserId(
        provider as OAuthProvider,
        result.profile.providerId
      );

      if (existingProvider) {
        // User exists - update provider info and return user
        await this.oauthService.linkProvider(
          existingProvider.user_id,
          provider as OAuthProvider,
          result.profile,
          result.tokens
        );

        reply.send({
          userId: existingProvider.user_id,
          provider,
          providerEmail: result.profile.email,
          providerName: result.profile.name,
          providerAvatarUrl: result.profile.avatarUrl,
          isNewUser: false,
        });
      } else {
        // New user - return profile for registration
        reply.send({
          provider,
          providerUserId: result.profile.providerId,
          providerEmail: result.profile.email,
          providerName: result.profile.name,
          providerAvatarUrl: result.profile.avatarUrl,
          isNewUser: true,
          // Client should create user and call /link endpoint
        });
      }
    } catch (error) {
      logger.error('OAuth callback error', { error });
      reply.code(500).send({ error: 'OAuth callback failed' });
    }
  }

  private async handleOAuthLink(request: FastifyRequest<{ Params: { provider: string }, Body: { userId: string; code: string; redirectUri?: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { provider } = request.params;
      const { userId, code, redirectUri } = request.body;

      if (!this.oauthService.isProviderEnabled(provider as OAuthProvider)) {
        reply.code(400).send({ error: `Provider ${provider} is not configured` });
        return;
      }

      const callbackUrl = redirectUri || `${this.config.magicLink.baseUrl}/api/oauth/${provider}/callback`;
      const result = await this.oauthService.handleCallback(
        provider as OAuthProvider,
        code,
        callbackUrl
      );

      await this.oauthService.linkProvider(
        userId,
        provider as OAuthProvider,
        result.profile,
        result.tokens
      );

      reply.send({
        success: true,
        provider,
        providerEmail: result.profile.email,
        providerName: result.profile.name,
      });
    } catch (error) {
      logger.error('OAuth link error', { error });
      reply.code(500).send({ error: 'Failed to link OAuth provider' });
    }
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
  // WebAuthn/Passkeys Handlers
  // =========================================================================

  /**
   * Start challenge cleanup timer
   * Removes stale challenges (older than 5 minutes)
   */
  private startChallengeCleanup(): void {
    setInterval(() => {
      const now = Date.now();
      const fiveMinutes = 5 * 60 * 1000;
      for (const [key, data] of this.challenges.entries()) {
        if (now - data.timestamp > fiveMinutes) {
          this.challenges.delete(key);
        }
      }
    }, 60000); // Run every minute
  }

  private async handlePasskeyRegisterStart(request: FastifyRequest<{ Body: { userId: string; userName: string; userDisplayName: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userId, userName, userDisplayName } = request.body;

      // Get existing passkeys to exclude
      const existingPasskeys = await this.db.getPasskeysByUser(userId);

      // Generate registration options
      const options = await this.webAuthnService.generateRegistrationOptions({
        userId,
        userName,
        userDisplayName,
        excludeCredentials: existingPasskeys,
      });

      // Store challenge for verification
      this.challenges.set(userId, {
        challenge: options.challenge,
        userId,
        timestamp: Date.now(),
      });

      reply.send(options);
    } catch (error) {
      logger.error('Passkey registration start error', { error });
      reply.code(500).send({ error: 'Failed to start passkey registration' });
    }
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

  private async handleTotpEnroll(request: FastifyRequest<{ Body: { userId: string; email?: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userId, email } = request.body;
      const enrollment = await this.totpService.generateSecret(userId, email);

      // Encrypt secret and backup codes
      const encryptionKey = this.config.security.encryptionKey;
      const secretEncrypted = encrypt(enrollment.secret, encryptionKey);
      const backupCodesEncrypted = encryptArray(enrollment.backupCodes, encryptionKey);

      // Store in database
      await this.db.insertMfaEnrollment({
        user_id: userId,
        method: 'totp',
        secret_encrypted: secretEncrypted,
        backup_codes_encrypted: backupCodesEncrypted,
        backup_codes_remaining: enrollment.backupCodes.length,
        verified: false,
        algorithm: this.config.totp.algorithm.toUpperCase(),
        digits: this.config.totp.digits,
        period: this.config.totp.period,
      });

      reply.send({
        secret: enrollment.secret,
        qrCode: enrollment.qrCode,
        backupCodes: enrollment.backupCodes,
      });
    } catch (error) {
      logger.error('TOTP enrollment error', { error });
      reply.code(500).send({ error: 'TOTP enrollment failed' });
    }
  }

  private async handleTotpVerify(request: FastifyRequest<{ Body: { userId: string; code: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userId, code } = request.body;
      const enrollment = await this.db.getMfaEnrollment(userId, 'totp');

      if (!enrollment) {
        reply.code(404).send({ error: 'TOTP not enrolled' });
        return;
      }

      // Decrypt secret
      const encryptionKey = this.config.security.encryptionKey;
      const secret = decrypt(enrollment.secret_encrypted, encryptionKey);

      const isValid = await this.totpService.verifyToken(secret, code);

      if (isValid) {
        await this.db.updateMfaVerified(userId, 'totp', true);
        reply.send({ success: true, verified: true });
      } else {
        reply.code(400).send({ error: 'Invalid TOTP code' });
      }
    } catch (error) {
      logger.error('TOTP verification error', { error });
      reply.code(500).send({ error: 'TOTP verification failed' });
    }
  }

  private async handleTotpValidate(request: FastifyRequest<{ Body: { userId: string; code: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userId, code } = request.body;
      const enrollment = await this.db.getMfaEnrollment(userId, 'totp');

      if (!enrollment || !enrollment.verified) {
        reply.code(404).send({ error: 'TOTP not configured' });
        return;
      }

      // Decrypt secret
      const encryptionKey = this.config.security.encryptionKey;
      const secret = decrypt(enrollment.secret_encrypted, encryptionKey);

      const isValid = await this.totpService.verifyToken(secret, code);
      reply.send({ valid: isValid });
    } catch (error) {
      logger.error('TOTP validation error', { error });
      reply.code(500).send({ error: 'TOTP validation failed' });
    }
  }

  private async handleBackupCodeValidate(request: FastifyRequest<{ Body: { userId: string; code: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userId, code } = request.body;
      const enrollment = await this.db.getMfaEnrollment(userId, 'totp');

      if (!enrollment || !enrollment.backup_codes_encrypted) {
        reply.code(404).send({ error: 'MFA not enrolled' });
        return;
      }

      // Decrypt backup codes
      const encryptionKey = this.config.security.encryptionKey;
      const backupCodes = decryptArray(enrollment.backup_codes_encrypted, encryptionKey);

      const result = this.totpService.verifyBackupCode(code, backupCodes);

      if (result.valid) {
        // Re-encrypt remaining backup codes
        const remainingEncrypted = encryptArray(result.remainingCodes, encryptionKey);

        // Update database with new encrypted backup codes
        await this.db.updateBackupCodes(userId, 'totp', remainingEncrypted, result.remainingCodes.length);

        reply.send({ valid: true, remainingCodes: result.remainingCodes.length });
      } else {
        reply.send({ valid: false });
      }
    } catch (error) {
      logger.error('Backup code validation error', { error });
      reply.code(500).send({ error: 'Backup code validation failed' });
    }
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
  // Magic Link Handlers
  // =========================================================================

  private async handleMagicLinkSend(request: FastifyRequest<{ Body: { email: string; purpose: 'login' | 'verify' | 'reset'; redirectUrl?: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { email, purpose } = request.body;

      // Validate purpose
      if (!['login', 'verify', 'reset'].includes(purpose)) {
        reply.code(400).send({ error: 'Invalid purpose. Must be login, verify, or reset' });
        return;
      }

      // Generate magic link
      const magicLink = await this.magicLinkService.generateMagicLink(email, purpose);
      const tokenHash = this.magicLinkService.hashToken(magicLink.token);

      // Store in database
      await this.db.insertMagicLink({
        email,
        token_hash: tokenHash,
        purpose,
        expires_at: magicLink.expiresAt,
        ip_address: request.ip || null,
      });

      // TODO: Send email via notifications plugin
      // For now, we return the URL in development mode
      // In production, this should ONLY be sent via email
      logger.info('Magic link created', { email, purpose, url: magicLink.url });

      // Calculate expiry duration
      const expiresIn = Math.floor((magicLink.expiresAt.getTime() - Date.now()) / 1000);

      reply.send({
        sent: true,
        expiresIn,
        // SECURITY: Remove this in production - only send via email!
        url: magicLink.url,
      });
    } catch (error) {
      logger.error('Magic link send error', { error });
      reply.code(500).send({ error: 'Failed to send magic link' });
    }
  }

  private async handleMagicLinkVerify(request: FastifyRequest<{ Body: { token: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { token } = request.body;

      // Hash the provided token
      const tokenHash = this.magicLinkService.hashToken(token);

      // Get magic link from database
      const magicLink = await this.db.getMagicLink(tokenHash);

      if (!magicLink) {
        reply.code(404).send({ error: 'Invalid or expired magic link' });
        return;
      }

      // Check if expired
      if (this.magicLinkService.isExpired(magicLink.expires_at)) {
        reply.code(400).send({ error: 'Magic link expired' });
        return;
      }

      // Mark as used
      await this.db.markMagicLinkUsed(tokenHash);

      logger.info('Magic link verified', { email: magicLink.email, purpose: magicLink.purpose });

      reply.send({
        valid: true,
        email: magicLink.email,
        purpose: magicLink.purpose,
      });
    } catch (error) {
      logger.error('Magic link verify error', { error });
      reply.code(500).send({ error: 'Verification failed' });
    }
  }

  // =========================================================================
  // Device Code Handlers
  // =========================================================================

  private async handleDeviceCodeInitiate(request: FastifyRequest<{ Body: { deviceId?: string; deviceName?: string; deviceType?: string; scopes?: string[] } }>, reply: FastifyReply): Promise<void> {
    try {
      const { deviceId, deviceName, deviceType, scopes } = request.body;

      // Generate device code and user code
      const codeData = await this.deviceCodeService.initiate();

      // Store in database
      await this.db.insertDeviceCode({
        device_code: codeData.deviceCode,
        user_code: codeData.userCode,
        device_id: deviceId || null,
        device_name: deviceName || null,
        device_type: deviceType || null,
        scopes: scopes || [],
        status: 'pending',
        expires_at: codeData.expiresAt,
        poll_interval: codeData.interval,
      });

      logger.info('Device code initiated', { userCode: codeData.userCode, deviceName });

      reply.send({
        deviceCode: codeData.deviceCode,
        userCode: codeData.userCode,
        verificationUrl: codeData.verificationUri,
        expiresIn: this.config.deviceCode.expirySeconds,
        pollInterval: codeData.interval,
      });
    } catch (error) {
      logger.error('Device code initiation error', { error });
      reply.code(500).send({ error: 'Device code initiation failed' });
    }
  }

  private async handleDeviceCodePoll(request: FastifyRequest<{ Querystring: { deviceCode: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { deviceCode } = request.query;
      const record = await this.db.getDeviceCodeByCode(deviceCode);

      if (!record) {
        reply.code(404).send({ error: 'Device code not found' });
        return;
      }

      // Check expiration
      if (this.deviceCodeService.isExpired(record.expires_at)) {
        // Update status if not already expired
        if (record.status !== 'expired') {
          await this.db.updateDeviceCodeStatus(record.user_code, 'expired');
        }
        reply.send({ status: 'expired' });
        return;
      }

      // Return status based on authorization state
      if (record.status === 'authorized' && record.user_id) {
        // Generate access and refresh tokens
        const tokens = this.tokenService.generateTokenPair({
          userId: record.user_id,
          sessionId: record.id,
        });

        reply.send({
          status: 'authorized',
          userId: record.user_id,
          accessToken: tokens.accessToken,
          refreshToken: tokens.refreshToken,
          expiresIn: tokens.expiresIn,
        });
        return;
      }

      reply.send({ status: record.status });
    } catch (error) {
      logger.error('Device code poll error', { error });
      reply.code(500).send({ error: 'Device code poll failed' });
    }
  }

  private async handleDeviceCodeAuthorize(request: FastifyRequest<{ Body: { userCode: string; userId: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userCode, userId } = request.body;

      const record = await this.db.getDeviceCodeByUserCode(userCode);
      if (!record) {
        reply.code(404).send({ error: 'User code not found' });
        return;
      }

      // Check expiration
      if (this.deviceCodeService.isExpired(record.expires_at)) {
        reply.code(400).send({ error: 'Device code expired' });
        return;
      }

      // Check if already authorized or denied
      if (record.status === 'authorized') {
        reply.code(400).send({ error: 'Device code already authorized' });
        return;
      }

      if (record.status === 'denied') {
        reply.code(400).send({ error: 'Device code already denied' });
        return;
      }

      // Update status to authorized
      await this.db.updateDeviceCodeStatus(userCode, 'authorized', userId);

      logger.info('Device code authorized', { userCode, userId, deviceName: record.device_name });

      reply.send({
        authorized: true,
        deviceName: record.device_name,
      });
    } catch (error) {
      logger.error('Device code authorization error', { error });
      reply.code(500).send({ error: 'Device code authorization failed' });
    }
  }

  private async handleDeviceCodeDeny(request: FastifyRequest<{ Body: { userCode: string } }>, reply: FastifyReply): Promise<void> {
    try {
      const { userCode } = request.body;

      const record = await this.db.getDeviceCodeByUserCode(userCode);
      if (!record) {
        reply.code(404).send({ error: 'User code not found' });
        return;
      }

      // Check expiration
      if (this.deviceCodeService.isExpired(record.expires_at)) {
        reply.code(400).send({ error: 'Device code expired' });
        return;
      }

      // Update status to denied
      await this.db.updateDeviceCodeStatus(userCode, 'denied');

      logger.info('Device code denied', { userCode });

      reply.send({ denied: true });
    } catch (error) {
      logger.error('Device code denial error', { error });
      reply.code(500).send({ error: 'Device code denial failed' });
    }
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
