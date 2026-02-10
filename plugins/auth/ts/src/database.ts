/**
 * Auth Plugin Database
 * Schema initialization and CRUD operations
 */

import { createDatabase, Database, createLogger } from '@nself/plugin-utils';
import {
  OAuthProviderRecord,
  PasskeyRecord,
  MfaEnrollmentRecord,
  DeviceCodeRecord,
  MagicLinkRecord,
  SessionRecord,
  LoginAttemptRecord,
  AuthStats,
} from './types.js';

const logger = createLogger('auth:database');

export class AuthDatabase {
  private db: Database;
  private currentAppId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  /**
   * Create scoped database instance for a specific app
   */
  forSourceApp(appId: string): AuthDatabase {
    const scoped = new AuthDatabase(this.db);
    scoped.currentAppId = appId;
    return scoped;
  }

  /**
   * Get current source account ID
   */
  getCurrentSourceAppId(): string {
    return this.currentAppId;
  }

  /**
   * Initialize database schema
   */
  async initSchema(): Promise<void> {
    logger.info('Initializing auth database schema...');

    // OAuth providers table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_oauth_providers (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        provider VARCHAR(50) NOT NULL,
        provider_user_id VARCHAR(255) NOT NULL,
        provider_email VARCHAR(255),
        provider_name VARCHAR(255),
        provider_avatar_url TEXT,
        access_token_encrypted TEXT,
        refresh_token_encrypted TEXT,
        token_expires_at TIMESTAMPTZ,
        scopes TEXT[],
        raw_profile JSONB,
        linked_at TIMESTAMPTZ DEFAULT NOW(),
        last_used_at TIMESTAMPTZ,
        UNIQUE(source_account_id, provider, provider_user_id),
        UNIQUE(source_account_id, user_id, provider)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_oauth_source_app
      ON auth_oauth_providers(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_oauth_user
      ON auth_oauth_providers(source_account_id, user_id);
    `);

    // Passkeys table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_passkeys (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        credential_id TEXT NOT NULL,
        public_key TEXT NOT NULL,
        counter BIGINT NOT NULL DEFAULT 0,
        device_type VARCHAR(50),
        backed_up BOOLEAN DEFAULT false,
        transports TEXT[],
        friendly_name VARCHAR(255),
        last_used_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, credential_id)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_passkeys_source_app
      ON auth_passkeys(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_passkeys_user
      ON auth_passkeys(source_account_id, user_id);
    `);

    // MFA enrollments table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_mfa_enrollments (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        method VARCHAR(50) NOT NULL DEFAULT 'totp',
        secret_encrypted TEXT NOT NULL,
        algorithm VARCHAR(10) DEFAULT 'SHA1',
        digits INTEGER DEFAULT 6,
        period INTEGER DEFAULT 30,
        verified BOOLEAN DEFAULT false,
        backup_codes_encrypted TEXT,
        backup_codes_remaining INTEGER DEFAULT 10,
        enabled BOOLEAN DEFAULT true,
        last_used_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, method)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_mfa_source_app
      ON auth_mfa_enrollments(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_mfa_user
      ON auth_mfa_enrollments(source_account_id, user_id);
    `);

    // Device codes table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_device_codes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        device_code VARCHAR(255) NOT NULL,
        user_code VARCHAR(20) NOT NULL,
        device_id VARCHAR(255),
        device_name VARCHAR(255),
        device_type VARCHAR(50),
        scopes TEXT[] DEFAULT '{openid,profile}',
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        user_id VARCHAR(255),
        authorized_at TIMESTAMPTZ,
        expires_at TIMESTAMPTZ NOT NULL,
        poll_interval INTEGER DEFAULT 5,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, device_code),
        UNIQUE(source_account_id, user_code)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_device_codes_source_app
      ON auth_device_codes(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_device_codes_user_code
      ON auth_device_codes(source_account_id, user_code);
    `);

    // Magic links table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_magic_links (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        email VARCHAR(255) NOT NULL,
        token_hash VARCHAR(128) NOT NULL,
        purpose VARCHAR(50) NOT NULL DEFAULT 'login',
        used BOOLEAN DEFAULT false,
        used_at TIMESTAMPTZ,
        expires_at TIMESTAMPTZ NOT NULL,
        ip_address VARCHAR(45),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, token_hash)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_magic_links_source_app
      ON auth_magic_links(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_magic_links_email
      ON auth_magic_links(source_account_id, email);
    `);

    // Sessions table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_sessions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        device_id VARCHAR(255),
        device_name VARCHAR(255),
        device_type VARCHAR(50),
        ip_address VARCHAR(45),
        user_agent TEXT,
        location_city VARCHAR(128),
        location_country VARCHAR(10),
        auth_method VARCHAR(50) NOT NULL,
        token_hash VARCHAR(128),
        is_active BOOLEAN DEFAULT true,
        last_activity_at TIMESTAMPTZ DEFAULT NOW(),
        expires_at TIMESTAMPTZ,
        revoked_at TIMESTAMPTZ,
        revoked_reason VARCHAR(255),
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_sessions_source_app
      ON auth_sessions(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_sessions_user
      ON auth_sessions(source_account_id, user_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_sessions_active
      ON auth_sessions(source_account_id, user_id, is_active);
    `);

    // Login attempts table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS auth_login_attempts (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        email VARCHAR(255),
        user_id VARCHAR(255),
        ip_address VARCHAR(45),
        method VARCHAR(50) NOT NULL,
        outcome VARCHAR(20) NOT NULL,
        failure_reason VARCHAR(255),
        user_agent TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_login_attempts_source_app
      ON auth_login_attempts(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_login_attempts_email
      ON auth_login_attempts(source_account_id, email, created_at);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_auth_login_attempts_ip
      ON auth_login_attempts(source_account_id, ip_address, created_at);
    `);

    logger.success('Auth database schema initialized');
  }

  // =========================================================================
  // OAuth Provider Methods
  // =========================================================================

  async upsertOAuthProvider(provider: Partial<OAuthProviderRecord>): Promise<void> {
    await this.db.execute(
      `INSERT INTO auth_oauth_providers (
        source_account_id, user_id, provider, provider_user_id, provider_email,
        provider_name, provider_avatar_url, access_token_encrypted, refresh_token_encrypted,
        token_expires_at, scopes, raw_profile, linked_at, last_used_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      ON CONFLICT (source_account_id, provider, provider_user_id) DO UPDATE SET
        provider_email = EXCLUDED.provider_email,
        provider_name = EXCLUDED.provider_name,
        provider_avatar_url = EXCLUDED.provider_avatar_url,
        access_token_encrypted = EXCLUDED.access_token_encrypted,
        refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
        token_expires_at = EXCLUDED.token_expires_at,
        scopes = EXCLUDED.scopes,
        raw_profile = EXCLUDED.raw_profile,
        last_used_at = EXCLUDED.last_used_at`,
      [
        this.currentAppId,
        provider.user_id,
        provider.provider,
        provider.provider_user_id,
        provider.provider_email || null,
        provider.provider_name || null,
        provider.provider_avatar_url || null,
        provider.access_token_encrypted || null,
        provider.refresh_token_encrypted || null,
        provider.token_expires_at || null,
        provider.scopes || [],
        provider.raw_profile || {},
        provider.linked_at || new Date(),
        provider.last_used_at || null,
      ]
    );
  }

  async getOAuthProvider(userId: string, provider: string): Promise<OAuthProviderRecord | null> {
    const result = await this.db.query<OAuthProviderRecord>(
      `SELECT * FROM auth_oauth_providers
       WHERE source_account_id = $1 AND user_id = $2 AND provider = $3`,
      [this.currentAppId, userId, provider]
    );
    return result.rows[0] || null;
  }

  async getOAuthProvidersByUser(userId: string): Promise<OAuthProviderRecord[]> {
    const result = await this.db.query<OAuthProviderRecord>(
      `SELECT * FROM auth_oauth_providers
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY linked_at DESC`,
      [this.currentAppId, userId]
    );
    return result.rows;
  }

  async deleteOAuthProvider(userId: string, provider: string): Promise<void> {
    await this.db.execute(
      `DELETE FROM auth_oauth_providers
       WHERE source_account_id = $1 AND user_id = $2 AND provider = $3`,
      [this.currentAppId, userId, provider]
    );
  }

  // =========================================================================
  // Passkey Methods
  // =========================================================================

  async insertPasskey(passkey: Partial<PasskeyRecord>): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO auth_passkeys (
        source_account_id, user_id, credential_id, public_key, counter,
        device_type, backed_up, transports, friendly_name
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING id`,
      [
        this.currentAppId,
        passkey.user_id,
        passkey.credential_id,
        passkey.public_key,
        passkey.counter || 0,
        passkey.device_type || null,
        passkey.backed_up || false,
        passkey.transports || [],
        passkey.friendly_name || null,
      ]
    );
    return result.rows[0].id;
  }

  async getPasskey(credentialId: string): Promise<PasskeyRecord | null> {
    const result = await this.db.query<PasskeyRecord>(
      `SELECT * FROM auth_passkeys
       WHERE source_account_id = $1 AND credential_id = $2`,
      [this.currentAppId, credentialId]
    );
    return result.rows[0] || null;
  }

  async getPasskeysByUser(userId: string): Promise<PasskeyRecord[]> {
    const result = await this.db.query<PasskeyRecord>(
      `SELECT * FROM auth_passkeys
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY created_at DESC`,
      [this.currentAppId, userId]
    );
    return result.rows;
  }

  async updatePasskeyCounter(credentialId: string, counter: number): Promise<void> {
    await this.db.execute(
      `UPDATE auth_passkeys
       SET counter = $1, last_used_at = NOW()
       WHERE source_account_id = $2 AND credential_id = $3`,
      [counter, this.currentAppId, credentialId]
    );
  }

  async deletePasskey(credentialId: string): Promise<void> {
    await this.db.execute(
      `DELETE FROM auth_passkeys
       WHERE source_account_id = $1 AND credential_id = $2`,
      [this.currentAppId, credentialId]
    );
  }

  // =========================================================================
  // MFA Methods
  // =========================================================================

  async insertMfaEnrollment(enrollment: Partial<MfaEnrollmentRecord>): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO auth_mfa_enrollments (
        source_account_id, user_id, method, secret_encrypted, algorithm,
        digits, period, verified, backup_codes_encrypted, backup_codes_remaining
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      ON CONFLICT (source_account_id, user_id, method) DO UPDATE SET
        secret_encrypted = EXCLUDED.secret_encrypted,
        backup_codes_encrypted = EXCLUDED.backup_codes_encrypted,
        backup_codes_remaining = EXCLUDED.backup_codes_remaining,
        verified = EXCLUDED.verified
      RETURNING id`,
      [
        this.currentAppId,
        enrollment.user_id,
        enrollment.method || 'totp',
        enrollment.secret_encrypted,
        enrollment.algorithm || 'SHA1',
        enrollment.digits || 6,
        enrollment.period || 30,
        enrollment.verified || false,
        enrollment.backup_codes_encrypted || null,
        enrollment.backup_codes_remaining || 10,
      ]
    );
    return result.rows[0].id;
  }

  async getMfaEnrollment(userId: string, method: string = 'totp'): Promise<MfaEnrollmentRecord | null> {
    const result = await this.db.query<MfaEnrollmentRecord>(
      `SELECT * FROM auth_mfa_enrollments
       WHERE source_account_id = $1 AND user_id = $2 AND method = $3`,
      [this.currentAppId, userId, method]
    );
    return result.rows[0] || null;
  }

  async updateMfaVerified(userId: string, method: string, verified: boolean): Promise<void> {
    await this.db.execute(
      `UPDATE auth_mfa_enrollments
       SET verified = $1, last_used_at = NOW()
       WHERE source_account_id = $2 AND user_id = $3 AND method = $4`,
      [verified, this.currentAppId, userId, method]
    );
  }

  async decrementBackupCodes(userId: string, method: string): Promise<number> {
    const result = await this.db.query<{ backup_codes_remaining: number }>(
      `UPDATE auth_mfa_enrollments
       SET backup_codes_remaining = GREATEST(backup_codes_remaining - 1, 0),
           last_used_at = NOW()
       WHERE source_account_id = $1 AND user_id = $2 AND method = $3
       RETURNING backup_codes_remaining`,
      [this.currentAppId, userId, method]
    );
    return result.rows[0]?.backup_codes_remaining || 0;
  }

  async deleteMfaEnrollment(userId: string, method: string): Promise<void> {
    await this.db.execute(
      `DELETE FROM auth_mfa_enrollments
       WHERE source_account_id = $1 AND user_id = $2 AND method = $3`,
      [this.currentAppId, userId, method]
    );
  }

  // =========================================================================
  // Device Code Methods
  // =========================================================================

  async insertDeviceCode(deviceCode: Partial<DeviceCodeRecord>): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO auth_device_codes (
        source_account_id, device_code, user_code, device_id, device_name,
        device_type, scopes, status, expires_at, poll_interval
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING id`,
      [
        this.currentAppId,
        deviceCode.device_code,
        deviceCode.user_code,
        deviceCode.device_id || null,
        deviceCode.device_name || null,
        deviceCode.device_type || null,
        deviceCode.scopes || ['openid', 'profile'],
        deviceCode.status || 'pending',
        deviceCode.expires_at,
        deviceCode.poll_interval || 5,
      ]
    );
    return result.rows[0].id;
  }

  async getDeviceCodeByCode(deviceCode: string): Promise<DeviceCodeRecord | null> {
    const result = await this.db.query<DeviceCodeRecord>(
      `SELECT * FROM auth_device_codes
       WHERE source_account_id = $1 AND device_code = $2`,
      [this.currentAppId, deviceCode]
    );
    return result.rows[0] || null;
  }

  async getDeviceCodeByUserCode(userCode: string): Promise<DeviceCodeRecord | null> {
    const result = await this.db.query<DeviceCodeRecord>(
      `SELECT * FROM auth_device_codes
       WHERE source_account_id = $1 AND user_code = $2`,
      [this.currentAppId, userCode]
    );
    return result.rows[0] || null;
  }

  async updateDeviceCodeStatus(
    userCode: string,
    status: string,
    userId?: string
  ): Promise<void> {
    await this.db.execute(
      `UPDATE auth_device_codes
       SET status = $1, user_id = $2, authorized_at = CASE WHEN $1 = 'authorized' THEN NOW() ELSE NULL END
       WHERE source_account_id = $3 AND user_code = $4`,
      [status, userId || null, this.currentAppId, userCode]
    );
  }

  async expireOldDeviceCodes(): Promise<number> {
    const result = await this.db.query<{ count: number }>(
      `UPDATE auth_device_codes
       SET status = 'expired'
       WHERE source_account_id = $1 AND status = 'pending' AND expires_at < NOW()
       RETURNING id`,
      [this.currentAppId]
    );
    return result.rows.length;
  }

  // =========================================================================
  // Magic Link Methods
  // =========================================================================

  async insertMagicLink(magicLink: Partial<MagicLinkRecord>): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO auth_magic_links (
        source_account_id, email, token_hash, purpose, expires_at, ip_address
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING id`,
      [
        this.currentAppId,
        magicLink.email,
        magicLink.token_hash,
        magicLink.purpose || 'login',
        magicLink.expires_at,
        magicLink.ip_address || null,
      ]
    );
    return result.rows[0].id;
  }

  async getMagicLink(tokenHash: string): Promise<MagicLinkRecord | null> {
    const result = await this.db.query<MagicLinkRecord>(
      `SELECT * FROM auth_magic_links
       WHERE source_account_id = $1 AND token_hash = $2 AND used = false AND expires_at > NOW()`,
      [this.currentAppId, tokenHash]
    );
    return result.rows[0] || null;
  }

  async markMagicLinkUsed(tokenHash: string): Promise<void> {
    await this.db.execute(
      `UPDATE auth_magic_links
       SET used = true, used_at = NOW()
       WHERE source_account_id = $1 AND token_hash = $2`,
      [this.currentAppId, tokenHash]
    );
  }

  async expireOldMagicLinks(): Promise<number> {
    const result = await this.db.query<{ id: string }>(
      `DELETE FROM auth_magic_links
       WHERE source_account_id = $1 AND (used = true OR expires_at < NOW())
       RETURNING id`,
      [this.currentAppId]
    );
    return result.rows.length;
  }

  // =========================================================================
  // Session Methods
  // =========================================================================

  async insertSession(session: Partial<SessionRecord>): Promise<string> {
    const result = await this.db.query<{ id: string }>(
      `INSERT INTO auth_sessions (
        source_account_id, user_id, device_id, device_name, device_type,
        ip_address, user_agent, location_city, location_country,
        auth_method, token_hash, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING id`,
      [
        this.currentAppId,
        session.user_id,
        session.device_id || null,
        session.device_name || null,
        session.device_type || null,
        session.ip_address || null,
        session.user_agent || null,
        session.location_city || null,
        session.location_country || null,
        session.auth_method,
        session.token_hash || null,
        session.expires_at || null,
      ]
    );
    return result.rows[0].id;
  }

  async getSession(sessionId: string): Promise<SessionRecord | null> {
    const result = await this.db.query<SessionRecord>(
      `SELECT * FROM auth_sessions
       WHERE source_account_id = $1 AND id = $2`,
      [this.currentAppId, sessionId]
    );
    return result.rows[0] || null;
  }

  async getActiveSessions(userId: string): Promise<SessionRecord[]> {
    const result = await this.db.query<SessionRecord>(
      `SELECT * FROM auth_sessions
       WHERE source_account_id = $1 AND user_id = $2 AND is_active = true
       ORDER BY last_activity_at DESC`,
      [this.currentAppId, userId]
    );
    return result.rows;
  }

  async updateSessionActivity(sessionId: string): Promise<void> {
    await this.db.execute(
      `UPDATE auth_sessions
       SET last_activity_at = NOW()
       WHERE source_account_id = $1 AND id = $2`,
      [this.currentAppId, sessionId]
    );
  }

  async revokeSession(sessionId: string, reason?: string): Promise<void> {
    await this.db.execute(
      `UPDATE auth_sessions
       SET is_active = false, revoked_at = NOW(), revoked_reason = $1
       WHERE source_account_id = $2 AND id = $3`,
      [reason || null, this.currentAppId, sessionId]
    );
  }

  async revokeAllUserSessions(userId: string, exceptSessionId?: string, reason?: string): Promise<number> {
    const query = exceptSessionId
      ? `UPDATE auth_sessions
         SET is_active = false, revoked_at = NOW(), revoked_reason = $1
         WHERE source_account_id = $2 AND user_id = $3 AND id != $4 AND is_active = true
         RETURNING id`
      : `UPDATE auth_sessions
         SET is_active = false, revoked_at = NOW(), revoked_reason = $1
         WHERE source_account_id = $2 AND user_id = $3 AND is_active = true
         RETURNING id`;

    const params = exceptSessionId
      ? [reason || null, this.currentAppId, userId, exceptSessionId]
      : [reason || null, this.currentAppId, userId];

    const result = await this.db.query<{ id: string }>(query, params);
    return result.rows.length;
  }

  async expireOldSessions(idleHours: number, absoluteHours: number): Promise<number> {
    const result = await this.db.query<{ id: string }>(
      `UPDATE auth_sessions
       SET is_active = false, revoked_at = NOW(), revoked_reason = 'expired'
       WHERE source_account_id = $1 AND is_active = true AND (
         last_activity_at < NOW() - INTERVAL '${idleHours} hours' OR
         created_at < NOW() - INTERVAL '${absoluteHours} hours'
       )
       RETURNING id`,
      [this.currentAppId]
    );
    return result.rows.length;
  }

  // =========================================================================
  // Login Attempt Methods
  // =========================================================================

  async insertLoginAttempt(attempt: Partial<LoginAttemptRecord>): Promise<void> {
    await this.db.execute(
      `INSERT INTO auth_login_attempts (
        source_account_id, email, user_id, ip_address, method, outcome, failure_reason, user_agent
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
      [
        this.currentAppId,
        attempt.email || null,
        attempt.user_id || null,
        attempt.ip_address || null,
        attempt.method,
        attempt.outcome,
        attempt.failure_reason || null,
        attempt.user_agent || null,
      ]
    );
  }

  async getLoginAttempts(userId: string, limit: number = 20): Promise<LoginAttemptRecord[]> {
    const result = await this.db.query<LoginAttemptRecord>(
      `SELECT * FROM auth_login_attempts
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY created_at DESC
       LIMIT $3`,
      [this.currentAppId, userId, limit]
    );
    return result.rows;
  }

  async getRecentFailedAttempts(emailOrIp: string, minutes: number = 15): Promise<number> {
    const result = await this.db.query<{ count: number }>(
      `SELECT COUNT(*) as count FROM auth_login_attempts
       WHERE source_account_id = $1
         AND (email = $2 OR ip_address = $2)
         AND outcome = 'failure'
         AND created_at > NOW() - INTERVAL '${minutes} minutes'`,
      [this.currentAppId, emailOrIp]
    );
    return Number(result.rows[0]?.count || 0);
  }

  async cleanupOldLoginAttempts(days: number = 90): Promise<number> {
    const result = await this.db.query<{ id: string }>(
      `DELETE FROM auth_login_attempts
       WHERE source_account_id = $1 AND created_at < NOW() - INTERVAL '${days} days'
       RETURNING id`,
      [this.currentAppId]
    );
    return result.rows.length;
  }

  // =========================================================================
  // Stats Methods
  // =========================================================================

  async getStats(): Promise<AuthStats> {
    const [
      oauthCount,
      passkeysCount,
      mfaCount,
      activeSessionsCount,
      activeDeviceCodesCount,
      pendingMagicLinksCount,
      recentAttemptsCount,
    ] = await Promise.all([
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_oauth_providers WHERE source_account_id = $1',
        [this.currentAppId]
      ),
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_passkeys WHERE source_account_id = $1',
        [this.currentAppId]
      ),
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_mfa_enrollments WHERE source_account_id = $1 AND verified = true',
        [this.currentAppId]
      ),
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_sessions WHERE source_account_id = $1 AND is_active = true',
        [this.currentAppId]
      ),
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_device_codes WHERE source_account_id = $1 AND status = \'pending\' AND expires_at > NOW()',
        [this.currentAppId]
      ),
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_magic_links WHERE source_account_id = $1 AND used = false AND expires_at > NOW()',
        [this.currentAppId]
      ),
      this.db.query<{ count: number }>(
        'SELECT COUNT(*) as count FROM auth_login_attempts WHERE source_account_id = $1 AND created_at > NOW() - INTERVAL \'24 hours\'',
        [this.currentAppId]
      ),
    ]);

    return {
      oauthProviders: Number(oauthCount.rows[0]?.count || 0),
      passkeys: Number(passkeysCount.rows[0]?.count || 0),
      mfaEnrollments: Number(mfaCount.rows[0]?.count || 0),
      activeSessions: Number(activeSessionsCount.rows[0]?.count || 0),
      activeDeviceCodes: Number(activeDeviceCodesCount.rows[0]?.count || 0),
      pendingMagicLinks: Number(pendingMagicLinksCount.rows[0]?.count || 0),
      recentLoginAttempts: Number(recentAttemptsCount.rows[0]?.count || 0),
    };
  }
}

/**
 * Create auth database instance
 */
export async function createAuthDatabase(config: any): Promise<AuthDatabase> {
  const db = await createDatabase(config.database);
  const authDb = new AuthDatabase(db);
  await authDb.initSchema();
  return authDb;
}
