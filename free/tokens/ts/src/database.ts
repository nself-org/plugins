/**
 * Tokens Plugin Database
 * Schema initialization and CRUD operations for content delivery tokens
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  TokensSigningKeyRecord,
  TokensIssuedRecord,
  TokensEncryptionKeyRecord,
  TokensEntitlementRecord,
  TokensStats,
} from './types.js';

const logger = createLogger('tokens:database');

export class TokensDatabase {
  private db: Database;
  private sourceAccountId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  forSourceAccount(sourceAccountId: string): TokensDatabase {
    const scoped = new TokensDatabase(this.db);
    scoped.sourceAccountId = sourceAccountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  // ============================================================================
  // Schema Initialization
  // ============================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing tokens database schema...');

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS np_tokens_signing_keys (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        algorithm VARCHAR(20) NOT NULL DEFAULT 'hmac-sha256',
        key_material_encrypted TEXT NOT NULL,
        is_active BOOLEAN DEFAULT true,
        rotated_from UUID REFERENCES np_tokens_signing_keys(id),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        rotated_at TIMESTAMPTZ,
        expires_at TIMESTAMPTZ,
        UNIQUE(source_account_id, name)
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_signing_keys_source_app ON np_tokens_signing_keys(source_account_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS np_tokens_issued (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        token_hash VARCHAR(128) NOT NULL,
        token_type VARCHAR(50) NOT NULL DEFAULT 'playback',
        signing_key_id UUID REFERENCES np_tokens_signing_keys(id),
        user_id VARCHAR(255) NOT NULL,
        device_id VARCHAR(255),
        content_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(50),
        permissions JSONB DEFAULT '{}',
        ip_address VARCHAR(45),
        issued_at TIMESTAMPTZ DEFAULT NOW(),
        expires_at TIMESTAMPTZ NOT NULL,
        revoked BOOLEAN DEFAULT false,
        revoked_at TIMESTAMPTZ,
        revoked_reason VARCHAR(255),
        last_used_at TIMESTAMPTZ,
        use_count INTEGER DEFAULT 0
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_source_app ON np_tokens_issued(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_hash ON np_tokens_issued(token_hash)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_user ON np_tokens_issued(source_account_id, user_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_content ON np_tokens_issued(source_account_id, content_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_issued_active ON np_tokens_issued(source_account_id, revoked, expires_at)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS np_tokens_encryption_keys (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        content_id VARCHAR(255) NOT NULL,
        key_material_encrypted TEXT NOT NULL,
        key_iv VARCHAR(64) NOT NULL,
        key_uri TEXT NOT NULL,
        rotation_generation INTEGER DEFAULT 1,
        is_active BOOLEAN DEFAULT true,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        rotated_at TIMESTAMPTZ,
        expires_at TIMESTAMPTZ
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tokens_enc_keys_source_app ON np_tokens_encryption_keys(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_tokens_enc_keys_content ON np_tokens_encryption_keys(source_account_id, content_id, is_active)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS np_tokens_entitlements (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(50),
        entitlement_type VARCHAR(50) NOT NULL DEFAULT 'stream',
        granted_by VARCHAR(50) DEFAULT 'system',
        granted_at TIMESTAMPTZ DEFAULT NOW(),
        expires_at TIMESTAMPTZ,
        revoked BOOLEAN DEFAULT false,
        metadata JSONB DEFAULT '{}',
        UNIQUE(source_account_id, user_id, content_id, entitlement_type)
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_entitlements_source_app ON np_tokens_entitlements(source_account_id)`);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_entitlements_user ON np_tokens_entitlements(source_account_id, user_id)`);

    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS np_tokens_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      )
    `);
    await this.db.execute(`CREATE INDEX IF NOT EXISTS idx_np_tokens_webhook_events_source_app ON np_tokens_webhook_events(source_account_id)`);

    logger.success('Tokens database schema initialized');
  }

  // ============================================================================
  // Signing Keys CRUD
  // ============================================================================

  async createSigningKey(name: string, algorithm: string, keyMaterialEncrypted: string): Promise<TokensSigningKeyRecord> {
    const result = await this.db.query<TokensSigningKeyRecord>(`
      INSERT INTO np_tokens_signing_keys (source_account_id, name, algorithm, key_material_encrypted)
      VALUES ($1, $2, $3, $4)
      RETURNING *
    `, [this.sourceAccountId, name, algorithm, keyMaterialEncrypted]);
    return result.rows[0];
  }

  async getSigningKey(id: string): Promise<TokensSigningKeyRecord | null> {
    return this.db.queryOne<TokensSigningKeyRecord>(
      `SELECT * FROM np_tokens_signing_keys WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async getSigningKeyByName(name: string): Promise<TokensSigningKeyRecord | null> {
    return this.db.queryOne<TokensSigningKeyRecord>(
      `SELECT * FROM np_tokens_signing_keys WHERE name = $1 AND source_account_id = $2 AND is_active = true`,
      [name, this.sourceAccountId]
    );
  }

  async getActiveSigningKey(): Promise<TokensSigningKeyRecord | null> {
    return this.db.queryOne<TokensSigningKeyRecord>(
      `SELECT * FROM np_tokens_signing_keys WHERE source_account_id = $1 AND is_active = true ORDER BY created_at DESC LIMIT 1`,
      [this.sourceAccountId]
    );
  }

  async listSigningKeys(): Promise<TokensSigningKeyRecord[]> {
    const result = await this.db.query<TokensSigningKeyRecord>(
      `SELECT * FROM np_tokens_signing_keys WHERE source_account_id = $1 ORDER BY created_at DESC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async rotateSigningKey(id: string, newKeyMaterial: string, expireOldAfterHours: number = 24): Promise<TokensSigningKeyRecord> {
    // Create new key rotated from old
    const oldKey = await this.getSigningKey(id);
    if (!oldKey) throw new Error('Signing key not found');

    const result = await this.db.query<TokensSigningKeyRecord>(`
      INSERT INTO np_tokens_signing_keys (source_account_id, name, algorithm, key_material_encrypted, rotated_from)
      VALUES ($1, $2, $3, $4, $5)
      RETURNING *
    `, [this.sourceAccountId, oldKey.name, oldKey.algorithm, newKeyMaterial, id]);

    // Set old key to expire
    const expiresAt = new Date(Date.now() + expireOldAfterHours * 60 * 60 * 1000);
    await this.db.execute(`
      UPDATE np_tokens_signing_keys SET rotated_at = NOW(), expires_at = $3
      WHERE id = $1 AND source_account_id = $2
    `, [id, this.sourceAccountId, expiresAt]);

    return result.rows[0];
  }

  async deactivateSigningKey(id: string): Promise<void> {
    await this.db.execute(`
      UPDATE np_tokens_signing_keys SET is_active = false
      WHERE id = $1 AND source_account_id = $2
    `, [id, this.sourceAccountId]);
  }

  // ============================================================================
  // Issued Tokens CRUD
  // ============================================================================

  async insertIssuedToken(token: {
    token_hash: string;
    token_type: string;
    signing_key_id: string | null;
    user_id: string;
    device_id: string | null;
    content_id: string;
    content_type: string | null;
    permissions: Record<string, unknown>;
    ip_address: string | null;
    expires_at: Date;
  }): Promise<TokensIssuedRecord> {
    const result = await this.db.query<TokensIssuedRecord>(`
      INSERT INTO np_tokens_issued (
        source_account_id, token_hash, token_type, signing_key_id, user_id,
        device_id, content_id, content_type, permissions, ip_address, expires_at
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
      RETURNING *
    `, [
      this.sourceAccountId, token.token_hash, token.token_type, token.signing_key_id,
      token.user_id, token.device_id, token.content_id, token.content_type,
      JSON.stringify(token.permissions), token.ip_address, token.expires_at,
    ]);
    return result.rows[0];
  }

  async getIssuedTokenByHash(tokenHash: string): Promise<TokensIssuedRecord | null> {
    return this.db.queryOne<TokensIssuedRecord>(
      `SELECT * FROM np_tokens_issued WHERE token_hash = $1`,
      [tokenHash]
    );
  }

  async getIssuedToken(id: string): Promise<TokensIssuedRecord | null> {
    return this.db.queryOne<TokensIssuedRecord>(
      `SELECT * FROM np_tokens_issued WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async updateTokenLastUsed(id: string): Promise<void> {
    await this.db.execute(`
      UPDATE np_tokens_issued SET last_used_at = NOW(), use_count = use_count + 1
      WHERE id = $1
    `, [id]);
  }

  async revokeToken(id: string, reason?: string): Promise<void> {
    await this.db.execute(`
      UPDATE np_tokens_issued SET revoked = true, revoked_at = NOW(), revoked_reason = $3
      WHERE id = $1 AND source_account_id = $2
    `, [id, this.sourceAccountId, reason || null]);
  }

  async revokeUserTokens(userId: string, reason?: string): Promise<number> {
    const result = await this.db.query(
      `UPDATE np_tokens_issued SET revoked = true, revoked_at = NOW(), revoked_reason = $3
       WHERE source_account_id = $1 AND user_id = $2 AND revoked = false
       RETURNING id`,
      [this.sourceAccountId, userId, reason || null]
    );
    return result.rowCount ?? 0;
  }

  async revokeContentTokens(contentId: string, reason?: string): Promise<number> {
    const result = await this.db.query(
      `UPDATE np_tokens_issued SET revoked = true, revoked_at = NOW(), revoked_reason = $3
       WHERE source_account_id = $1 AND content_id = $2 AND revoked = false
       RETURNING id`,
      [this.sourceAccountId, contentId, reason || null]
    );
    return result.rowCount ?? 0;
  }

  // ============================================================================
  // Encryption Keys CRUD
  // ============================================================================

  async createEncryptionKey(contentId: string, keyMaterial: string, keyIv: string, keyUri: string): Promise<TokensEncryptionKeyRecord> {
    const result = await this.db.query<TokensEncryptionKeyRecord>(`
      INSERT INTO np_tokens_encryption_keys (source_account_id, content_id, key_material_encrypted, key_iv, key_uri)
      VALUES ($1, $2, $3, $4, $5)
      RETURNING *
    `, [this.sourceAccountId, contentId, keyMaterial, keyIv, keyUri]);
    return result.rows[0];
  }

  async getActiveEncryptionKey(contentId: string): Promise<TokensEncryptionKeyRecord | null> {
    return this.db.queryOne<TokensEncryptionKeyRecord>(
      `SELECT * FROM np_tokens_encryption_keys WHERE source_account_id = $1 AND content_id = $2 AND is_active = true ORDER BY rotation_generation DESC LIMIT 1`,
      [this.sourceAccountId, contentId]
    );
  }

  async getEncryptionKeyById(id: string): Promise<TokensEncryptionKeyRecord | null> {
    return this.db.queryOne<TokensEncryptionKeyRecord>(
      `SELECT * FROM np_tokens_encryption_keys WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async rotateEncryptionKey(contentId: string, newKeyMaterial: string, newKeyIv: string, newKeyUri: string, expireOldAfterHours: number = 24): Promise<TokensEncryptionKeyRecord> {
    const current = await this.getActiveEncryptionKey(contentId);
    const nextGeneration = (current?.rotation_generation ?? 0) + 1;

    // Set old keys to expire
    const expiresAt = new Date(Date.now() + expireOldAfterHours * 60 * 60 * 1000);
    await this.db.execute(`
      UPDATE np_tokens_encryption_keys SET rotated_at = NOW(), expires_at = $3
      WHERE source_account_id = $1 AND content_id = $2 AND is_active = true
    `, [this.sourceAccountId, contentId, expiresAt]);

    const result = await this.db.query<TokensEncryptionKeyRecord>(`
      INSERT INTO np_tokens_encryption_keys (source_account_id, content_id, key_material_encrypted, key_iv, key_uri, rotation_generation)
      VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *
    `, [this.sourceAccountId, contentId, newKeyMaterial, newKeyIv, newKeyUri, nextGeneration]);

    return result.rows[0];
  }

  // ============================================================================
  // Entitlements CRUD
  // ============================================================================

  async checkEntitlement(userId: string, contentId: string, entitlementType: string): Promise<TokensEntitlementRecord | null> {
    return this.db.queryOne<TokensEntitlementRecord>(
      `SELECT * FROM np_tokens_entitlements
       WHERE source_account_id = $1 AND user_id = $2 AND content_id = $3
         AND entitlement_type = $4 AND revoked = false
         AND (expires_at IS NULL OR expires_at > NOW())`,
      [this.sourceAccountId, userId, contentId, entitlementType]
    );
  }

  async hasAnyEntitlements(userId: string): Promise<boolean> {
    const result = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_tokens_entitlements WHERE source_account_id = $1 AND user_id = $2`,
      [this.sourceAccountId, userId]
    );
    return parseInt(result?.count ?? '0', 10) > 0;
  }

  async grantEntitlement(entry: {
    user_id: string;
    content_id: string;
    content_type: string | null;
    entitlement_type: string;
    expires_at: Date | null;
    metadata: Record<string, unknown>;
    granted_by: string;
  }): Promise<TokensEntitlementRecord> {
    const result = await this.db.query<TokensEntitlementRecord>(`
      INSERT INTO np_tokens_entitlements (
        source_account_id, user_id, content_id, content_type,
        entitlement_type, expires_at, metadata, granted_by
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
      ON CONFLICT (source_account_id, user_id, content_id, entitlement_type) DO UPDATE SET
        content_type = EXCLUDED.content_type,
        expires_at = EXCLUDED.expires_at,
        metadata = EXCLUDED.metadata,
        granted_by = EXCLUDED.granted_by,
        revoked = false,
        granted_at = NOW()
      RETURNING *
    `, [
      this.sourceAccountId, entry.user_id, entry.content_id, entry.content_type,
      entry.entitlement_type, entry.expires_at, JSON.stringify(entry.metadata), entry.granted_by,
    ]);
    return result.rows[0];
  }

  async revokeEntitlement(userId: string, contentId: string, entitlementType: string): Promise<void> {
    await this.db.execute(`
      UPDATE np_tokens_entitlements SET revoked = true
      WHERE source_account_id = $1 AND user_id = $2 AND content_id = $3 AND entitlement_type = $4
    `, [this.sourceAccountId, userId, contentId, entitlementType]);
  }

  async listUserEntitlements(userId: string, contentType?: string, activeOnly: boolean = true): Promise<TokensEntitlementRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'user_id = $2'];
    const params: unknown[] = [this.sourceAccountId, userId];
    let paramIndex = 3;

    if (contentType) {
      conditions.push(`content_type = $${paramIndex++}`);
      params.push(contentType);
    }

    if (activeOnly) {
      conditions.push('revoked = false');
      conditions.push('(expires_at IS NULL OR expires_at > NOW())');
    }

    const result = await this.db.query<TokensEntitlementRecord>(
      `SELECT * FROM np_tokens_entitlements WHERE ${conditions.join(' AND ')} ORDER BY granted_at DESC`,
      params
    );
    return result.rows;
  }

  // ============================================================================
  // Webhook Events
  // ============================================================================

  async insertWebhookEvent(eventId: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.db.execute(`
      INSERT INTO np_tokens_webhook_events (id, source_account_id, event_type, payload)
      VALUES ($1, $2, $3, $4)
      ON CONFLICT (id) DO NOTHING
    `, [eventId, this.sourceAccountId, eventType, JSON.stringify(payload)]);
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(): Promise<TokensStats> {
    const totalSigning = await this.db.countScoped('np_tokens_signing_keys', this.sourceAccountId);

    const activeSigning = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_tokens_signing_keys WHERE source_account_id = $1 AND is_active = true`,
      [this.sourceAccountId]
    );

    const totalIssued = await this.db.countScoped('np_tokens_issued', this.sourceAccountId);

    const activeTokens = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_tokens_issued WHERE source_account_id = $1 AND revoked = false AND expires_at > NOW()`,
      [this.sourceAccountId]
    );

    const revokedTokens = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_tokens_issued WHERE source_account_id = $1 AND revoked = true`,
      [this.sourceAccountId]
    );

    const expiredTokens = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_tokens_issued WHERE source_account_id = $1 AND revoked = false AND expires_at <= NOW()`,
      [this.sourceAccountId]
    );

    const totalEncryption = await this.db.countScoped('np_tokens_encryption_keys', this.sourceAccountId);
    const totalEntitlements = await this.db.countScoped('np_tokens_entitlements', this.sourceAccountId);

    const activeEntitlements = await this.db.queryOne<{ count: string }>(
      `SELECT COUNT(*) as count FROM np_tokens_entitlements WHERE source_account_id = $1 AND revoked = false AND (expires_at IS NULL OR expires_at > NOW())`,
      [this.sourceAccountId]
    );

    return {
      totalSigningKeys: totalSigning,
      activeSigningKeys: parseInt(activeSigning?.count ?? '0', 10),
      totalTokensIssued: totalIssued,
      activeTokens: parseInt(activeTokens?.count ?? '0', 10),
      revokedTokens: parseInt(revokedTokens?.count ?? '0', 10),
      expiredTokens: parseInt(expiredTokens?.count ?? '0', 10),
      totalEncryptionKeys: totalEncryption,
      totalEntitlements: totalEntitlements,
      activeEntitlements: parseInt(activeEntitlements?.count ?? '0', 10),
    };
  }
}

export async function createTokensDatabase(config: {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
}): Promise<TokensDatabase> {
  const db = createDatabase(config);
  await db.connect();
  return new TokensDatabase(db);
}
