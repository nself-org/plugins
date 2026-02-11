/**
 * Bots Database Operations
 * Complete CRUD operations for bots, commands, subscriptions, installations, messages, interactions, reviews, API keys
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import crypto from 'node:crypto';
import type {
  BotRecord, CreateBotRequest, UpdateBotRequest,
  BotCommandRecord, CreateCommandRequest, UpdateCommandRequest,
  BotSubscriptionRecord, CreateSubscriptionRequest,
  BotInstallationRecord, InstallBotRequest,
  BotMessageRecord,
  BotInteractionRecord, CreateInteractionRequest,
  BotReviewRecord, CreateReviewRequest,
  BotApiKeyRecord, CreateApiKeyRequest,
  BotsStats, MarketplaceQuery,
} from './types.js';

const logger = createLogger('bots:db');

export class BotsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): BotsDatabase {
    return new BotsDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> { await this.db.connect(); }
  async disconnect(): Promise<void> { await this.db.disconnect(); }
  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> { return this.db.query<T>(sql, params); }
  async execute(sql: string, params?: unknown[]): Promise<number> { return this.db.execute(sql, params); }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing bots schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Bots
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bots (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(100) NOT NULL,
        username VARCHAR(50) NOT NULL,
        description TEXT,
        avatar_url TEXT,
        bot_type VARCHAR(50) NOT NULL DEFAULT 'custom',
        owner_id UUID NOT NULL,
        workspace_id UUID,
        token_hash TEXT NOT NULL,
        oauth_client_id VARCHAR(255),
        oauth_client_secret_encrypted TEXT,
        permissions BIGINT NOT NULL DEFAULT 0,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        is_verified BOOLEAN NOT NULL DEFAULT false,
        is_public BOOLEAN NOT NULL DEFAULT false,
        category VARCHAR(50),
        tags TEXT[] DEFAULT '{}',
        website_url TEXT,
        support_url TEXT,
        privacy_policy_url TEXT,
        terms_of_service_url TEXT,
        install_count INTEGER DEFAULT 0,
        message_count INTEGER DEFAULT 0,
        command_count INTEGER DEFAULT 0,
        rating_avg DECIMAL(3,2) DEFAULT 0.0,
        rating_count INTEGER DEFAULT 0,
        last_active_at TIMESTAMPTZ,
        last_message_at TIMESTAMPTZ,
        rate_limit_per_minute INTEGER DEFAULT 60,
        rate_limit_per_hour INTEGER DEFAULT 1000,
        rate_limit_per_day INTEGER DEFAULT 10000,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, username)
      );
      CREATE INDEX IF NOT EXISTS idx_bots_account ON nchat_bots(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bots_username ON nchat_bots(username);
      CREATE INDEX IF NOT EXISTS idx_bots_owner ON nchat_bots(owner_id);
      CREATE INDEX IF NOT EXISTS idx_bots_workspace ON nchat_bots(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_bots_public ON nchat_bots(is_public) WHERE is_public = true;
      CREATE INDEX IF NOT EXISTS idx_bots_verified ON nchat_bots(is_verified) WHERE is_verified = true;
      CREATE INDEX IF NOT EXISTS idx_bots_category ON nchat_bots(category) WHERE category IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_bots_tags ON nchat_bots USING GIN(tags);

      -- =====================================================================
      -- Bot Commands
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_commands (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        command VARCHAR(50) NOT NULL,
        description TEXT NOT NULL,
        usage_hint TEXT,
        command_type VARCHAR(50) NOT NULL DEFAULT 'message',
        scope VARCHAR(50) NOT NULL DEFAULT 'all',
        parameters JSONB DEFAULT '[]'::jsonb,
        required_permissions BIGINT DEFAULT 0,
        rate_limit_per_minute INTEGER,
        rate_limit_per_hour INTEGER,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        usage_count INTEGER DEFAULT 0,
        last_used_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, bot_id, command)
      );
      CREATE INDEX IF NOT EXISTS idx_bot_commands_account ON nchat_bot_commands(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_commands_bot ON nchat_bot_commands(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_commands_command ON nchat_bot_commands(command);
      CREATE INDEX IF NOT EXISTS idx_bot_commands_enabled ON nchat_bot_commands(is_enabled) WHERE is_enabled = true;

      -- =====================================================================
      -- Bot Subscriptions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_subscriptions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        workspace_id UUID,
        channel_id UUID,
        event_type VARCHAR(100) NOT NULL,
        filters JSONB DEFAULT '{}'::jsonb,
        delivery_mode VARCHAR(50) NOT NULL DEFAULT 'webhook',
        webhook_url TEXT,
        webhook_secret VARCHAR(255),
        is_active BOOLEAN NOT NULL DEFAULT true,
        event_count INTEGER DEFAULT 0,
        last_event_at TIMESTAMPTZ,
        failed_delivery_count INTEGER DEFAULT 0,
        last_failure_at TIMESTAMPTZ,
        last_error_message TEXT,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, bot_id, workspace_id, channel_id, event_type)
      );
      CREATE INDEX IF NOT EXISTS idx_bot_subscriptions_account ON nchat_bot_subscriptions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_subscriptions_bot ON nchat_bot_subscriptions(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_subscriptions_workspace ON nchat_bot_subscriptions(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_bot_subscriptions_channel ON nchat_bot_subscriptions(channel_id);
      CREATE INDEX IF NOT EXISTS idx_bot_subscriptions_event ON nchat_bot_subscriptions(event_type);
      CREATE INDEX IF NOT EXISTS idx_bot_subscriptions_active ON nchat_bot_subscriptions(is_active) WHERE is_active = true;

      -- =====================================================================
      -- Bot Installations
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_installations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        workspace_id UUID NOT NULL,
        installed_by UUID NOT NULL,
        scope VARCHAR(50) NOT NULL DEFAULT 'workspace',
        channel_id UUID,
        config JSONB DEFAULT '{}'::jsonb,
        granted_permissions BIGINT NOT NULL,
        is_active BOOLEAN NOT NULL DEFAULT true,
        oauth_access_token_encrypted TEXT,
        oauth_refresh_token_encrypted TEXT,
        oauth_expires_at TIMESTAMPTZ,
        oauth_scope TEXT,
        message_count INTEGER DEFAULT 0,
        command_count INTEGER DEFAULT 0,
        last_used_at TIMESTAMPTZ,
        installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        uninstalled_at TIMESTAMPTZ,
        uninstalled_by UUID,
        UNIQUE(source_account_id, bot_id, workspace_id, channel_id)
      );
      CREATE INDEX IF NOT EXISTS idx_bot_installations_account ON nchat_bot_installations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_installations_bot ON nchat_bot_installations(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_installations_workspace ON nchat_bot_installations(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_bot_installations_channel ON nchat_bot_installations(channel_id);
      CREATE INDEX IF NOT EXISTS idx_bot_installations_active ON nchat_bot_installations(is_active) WHERE is_active = true;

      -- =====================================================================
      -- Bot Messages
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        message_id UUID NOT NULL,
        channel_id UUID NOT NULL,
        message_type VARCHAR(50) NOT NULL,
        interaction_count INTEGER DEFAULT 0,
        last_interaction_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_bot_messages_account ON nchat_bot_messages(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_messages_bot ON nchat_bot_messages(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_messages_message ON nchat_bot_messages(message_id);
      CREATE INDEX IF NOT EXISTS idx_bot_messages_channel ON nchat_bot_messages(channel_id);
      CREATE INDEX IF NOT EXISTS idx_bot_messages_created ON nchat_bot_messages(created_at DESC);

      -- =====================================================================
      -- Bot Interactions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_interactions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        message_id UUID NOT NULL,
        user_id UUID NOT NULL,
        interaction_type VARCHAR(50) NOT NULL,
        interaction_id VARCHAR(255) NOT NULL,
        interaction_value JSONB,
        response_sent BOOLEAN NOT NULL DEFAULT false,
        response_message_id UUID,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_bot_interactions_account ON nchat_bot_interactions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_interactions_bot ON nchat_bot_interactions(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_interactions_message ON nchat_bot_interactions(message_id);
      CREATE INDEX IF NOT EXISTS idx_bot_interactions_user ON nchat_bot_interactions(user_id);
      CREATE INDEX IF NOT EXISTS idx_bot_interactions_type ON nchat_bot_interactions(interaction_type);
      CREATE INDEX IF NOT EXISTS idx_bot_interactions_created ON nchat_bot_interactions(created_at DESC);

      -- =====================================================================
      -- Bot Reviews
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_reviews (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        user_id UUID NOT NULL,
        rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
        title VARCHAR(200),
        comment TEXT,
        is_published BOOLEAN NOT NULL DEFAULT true,
        is_flagged BOOLEAN NOT NULL DEFAULT false,
        moderated_at TIMESTAMPTZ,
        moderated_by UUID,
        moderation_reason TEXT,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, bot_id, user_id)
      );
      CREATE INDEX IF NOT EXISTS idx_bot_reviews_account ON nchat_bot_reviews(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_reviews_bot ON nchat_bot_reviews(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_reviews_user ON nchat_bot_reviews(user_id);
      CREATE INDEX IF NOT EXISTS idx_bot_reviews_rating ON nchat_bot_reviews(rating);
      CREATE INDEX IF NOT EXISTS idx_bot_reviews_published ON nchat_bot_reviews(is_published) WHERE is_published = true;

      -- =====================================================================
      -- Bot API Keys
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bot_api_keys (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        bot_id UUID NOT NULL REFERENCES nchat_bots(id) ON DELETE CASCADE,
        key_name VARCHAR(100) NOT NULL,
        key_hash VARCHAR(255) NOT NULL,
        key_prefix VARCHAR(20) NOT NULL,
        permissions BIGINT NOT NULL,
        scopes TEXT[] DEFAULT '{}',
        is_active BOOLEAN NOT NULL DEFAULT true,
        rate_limit_per_minute INTEGER,
        rate_limit_per_hour INTEGER,
        expires_at TIMESTAMPTZ,
        last_used_at TIMESTAMPTZ,
        use_count INTEGER DEFAULT 0,
        revoked_at TIMESTAMPTZ,
        revoked_by UUID,
        revoke_reason TEXT,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, key_hash)
      );
      CREATE INDEX IF NOT EXISTS idx_bot_api_keys_account ON nchat_bot_api_keys(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bot_api_keys_bot ON nchat_bot_api_keys(bot_id);
      CREATE INDEX IF NOT EXISTS idx_bot_api_keys_hash ON nchat_bot_api_keys(key_hash);
      CREATE INDEX IF NOT EXISTS idx_bot_api_keys_active ON nchat_bot_api_keys(is_active) WHERE is_active = true;
      CREATE INDEX IF NOT EXISTS idx_bot_api_keys_expires ON nchat_bot_api_keys(expires_at) WHERE expires_at IS NOT NULL;

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_bots_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_bots_webhook_events_account ON nchat_bots_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_bots_webhook_events_type ON nchat_bots_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_bots_webhook_events_processed ON nchat_bots_webhook_events(processed);
    `;

    await this.db.execute(schema);
    logger.info('Bots schema initialized successfully');
  }

  // =========================================================================
  // Bot CRUD
  // =========================================================================

  async createBot(request: CreateBotRequest): Promise<{ bot: BotRecord; token: string }> {
    const token = `nbot_${crypto.randomBytes(24).toString('hex')}`;
    const tokenHash = crypto.createHash('sha256').update(token).digest('hex');

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bots (
        source_account_id, name, username, description, avatar_url, bot_type,
        owner_id, workspace_id, token_hash, permissions, is_public,
        category, tags, website_url, support_url, privacy_policy_url,
        terms_of_service_url, rate_limit_per_minute, rate_limit_per_hour,
        rate_limit_per_day, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.name,
        request.username,
        request.description ?? null,
        request.avatarUrl ?? null,
        request.botType ?? 'custom',
        request.ownerId,
        request.workspaceId ?? null,
        tokenHash,
        request.permissions ?? 0,
        request.isPublic ?? false,
        request.category ?? null,
        request.tags ?? [],
        request.websiteUrl ?? null,
        request.supportUrl ?? null,
        request.privacyPolicyUrl ?? null,
        request.termsOfServiceUrl ?? null,
        request.rateLimitPerMinute ?? 60,
        request.rateLimitPerHour ?? 1000,
        request.rateLimitPerDay ?? 10000,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return { bot: result.rows[0] as unknown as BotRecord, token };
  }

  async getBot(botId: string): Promise<BotRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bots WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, botId]
    );
    return (result.rows[0] ?? null) as unknown as BotRecord | null;
  }

  async getBotByUsername(username: string): Promise<BotRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bots WHERE source_account_id = $1 AND username = $2',
      [this.sourceAccountId, username]
    );
    return (result.rows[0] ?? null) as unknown as BotRecord | null;
  }

  async listBots(options: { ownerId?: string; isPublic?: boolean; isEnabled?: boolean; limit?: number; offset?: number } = {}): Promise<BotRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.ownerId) { conditions.push(`owner_id = $${paramIndex++}`); params.push(options.ownerId); }
    if (options.isPublic !== undefined) { conditions.push(`is_public = $${paramIndex++}`); params.push(options.isPublic); }
    if (options.isEnabled !== undefined) { conditions.push(`is_enabled = $${paramIndex++}`); params.push(options.isEnabled); }

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_bots WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as BotRecord[];
  }

  async updateBot(botId: string, updates: UpdateBotRequest): Promise<BotRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, botId];
    let paramIndex = 3;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.avatarUrl !== undefined) { sets.push(`avatar_url = $${paramIndex++}`); params.push(updates.avatarUrl); }
    if (updates.permissions !== undefined) { sets.push(`permissions = $${paramIndex++}`); params.push(updates.permissions); }
    if (updates.isEnabled !== undefined) { sets.push(`is_enabled = $${paramIndex++}`); params.push(updates.isEnabled); }
    if (updates.isVerified !== undefined) { sets.push(`is_verified = $${paramIndex++}`); params.push(updates.isVerified); }
    if (updates.isPublic !== undefined) { sets.push(`is_public = $${paramIndex++}`); params.push(updates.isPublic); }
    if (updates.category !== undefined) { sets.push(`category = $${paramIndex++}`); params.push(updates.category); }
    if (updates.tags !== undefined) { sets.push(`tags = $${paramIndex++}`); params.push(updates.tags); }
    if (updates.websiteUrl !== undefined) { sets.push(`website_url = $${paramIndex++}`); params.push(updates.websiteUrl); }
    if (updates.supportUrl !== undefined) { sets.push(`support_url = $${paramIndex++}`); params.push(updates.supportUrl); }
    if (updates.privacyPolicyUrl !== undefined) { sets.push(`privacy_policy_url = $${paramIndex++}`); params.push(updates.privacyPolicyUrl); }
    if (updates.termsOfServiceUrl !== undefined) { sets.push(`terms_of_service_url = $${paramIndex++}`); params.push(updates.termsOfServiceUrl); }
    if (updates.rateLimitPerMinute !== undefined) { sets.push(`rate_limit_per_minute = $${paramIndex++}`); params.push(updates.rateLimitPerMinute); }
    if (updates.rateLimitPerHour !== undefined) { sets.push(`rate_limit_per_hour = $${paramIndex++}`); params.push(updates.rateLimitPerHour); }
    if (updates.rateLimitPerDay !== undefined) { sets.push(`rate_limit_per_day = $${paramIndex++}`); params.push(updates.rateLimitPerDay); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getBot(botId);
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_bots SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as BotRecord | null;
  }

  async deleteBot(botId: string): Promise<boolean> {
    const count = await this.execute('DELETE FROM nchat_bots WHERE source_account_id = $1 AND id = $2', [this.sourceAccountId, botId]);
    return count > 0;
  }

  // =========================================================================
  // Command CRUD
  // =========================================================================

  async createCommand(request: CreateCommandRequest): Promise<BotCommandRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_commands (
        source_account_id, bot_id, command, description, usage_hint,
        command_type, scope, parameters, required_permissions,
        rate_limit_per_minute, rate_limit_per_hour, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
      RETURNING *`,
      [
        this.sourceAccountId, request.botId, request.command, request.description,
        request.usageHint ?? null, request.commandType ?? 'message', request.scope ?? 'all',
        JSON.stringify(request.parameters ?? []), request.requiredPermissions ?? 0,
        request.rateLimitPerMinute ?? null, request.rateLimitPerHour ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as BotCommandRecord;
  }

  async listCommands(botId: string): Promise<BotCommandRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bot_commands WHERE source_account_id = $1 AND bot_id = $2 ORDER BY command',
      [this.sourceAccountId, botId]
    );
    return result.rows as unknown as BotCommandRecord[];
  }

  async getCommand(commandId: string): Promise<BotCommandRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bot_commands WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, commandId]
    );
    return (result.rows[0] ?? null) as unknown as BotCommandRecord | null;
  }

  async updateCommand(commandId: string, updates: UpdateCommandRequest): Promise<BotCommandRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, commandId];
    let paramIndex = 3;

    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.usageHint !== undefined) { sets.push(`usage_hint = $${paramIndex++}`); params.push(updates.usageHint); }
    if (updates.scope !== undefined) { sets.push(`scope = $${paramIndex++}`); params.push(updates.scope); }
    if (updates.parameters !== undefined) { sets.push(`parameters = $${paramIndex++}`); params.push(JSON.stringify(updates.parameters)); }
    if (updates.isEnabled !== undefined) { sets.push(`is_enabled = $${paramIndex++}`); params.push(updates.isEnabled); }
    if (updates.requiredPermissions !== undefined) { sets.push(`required_permissions = $${paramIndex++}`); params.push(updates.requiredPermissions); }
    if (updates.rateLimitPerMinute !== undefined) { sets.push(`rate_limit_per_minute = $${paramIndex++}`); params.push(updates.rateLimitPerMinute); }
    if (updates.rateLimitPerHour !== undefined) { sets.push(`rate_limit_per_hour = $${paramIndex++}`); params.push(updates.rateLimitPerHour); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getCommand(commandId);
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_bot_commands SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as BotCommandRecord | null;
  }

  async deleteCommand(commandId: string): Promise<boolean> {
    const count = await this.execute('DELETE FROM nchat_bot_commands WHERE source_account_id = $1 AND id = $2', [this.sourceAccountId, commandId]);
    return count > 0;
  }

  // =========================================================================
  // Subscription CRUD
  // =========================================================================

  async createSubscription(request: CreateSubscriptionRequest): Promise<BotSubscriptionRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_subscriptions (
        source_account_id, bot_id, workspace_id, channel_id, event_type,
        filters, delivery_mode, webhook_url, webhook_secret, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
      RETURNING *`,
      [
        this.sourceAccountId, request.botId, request.workspaceId ?? null,
        request.channelId ?? null, request.eventType,
        JSON.stringify(request.filters ?? {}), request.deliveryMode ?? 'webhook',
        request.webhookUrl ?? null, request.webhookSecret ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as BotSubscriptionRecord;
  }

  async listSubscriptions(botId: string): Promise<BotSubscriptionRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bot_subscriptions WHERE source_account_id = $1 AND bot_id = $2 ORDER BY created_at DESC',
      [this.sourceAccountId, botId]
    );
    return result.rows as unknown as BotSubscriptionRecord[];
  }

  async deleteSubscription(subscriptionId: string): Promise<boolean> {
    const count = await this.execute('DELETE FROM nchat_bot_subscriptions WHERE source_account_id = $1 AND id = $2', [this.sourceAccountId, subscriptionId]);
    return count > 0;
  }

  // =========================================================================
  // Installation CRUD
  // =========================================================================

  async installBot(request: InstallBotRequest): Promise<BotInstallationRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_installations (
        source_account_id, bot_id, workspace_id, installed_by,
        scope, channel_id, config, granted_permissions
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
      ON CONFLICT (source_account_id, bot_id, workspace_id, channel_id) DO UPDATE SET
        is_active = true, granted_permissions = EXCLUDED.granted_permissions,
        config = EXCLUDED.config, uninstalled_at = NULL, uninstalled_by = NULL,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, request.botId, request.workspaceId,
        request.installedBy, request.scope ?? 'workspace',
        request.channelId ?? null, JSON.stringify(request.config ?? {}),
        request.grantedPermissions,
      ]
    );

    // Increment install count
    await this.execute(
      'UPDATE nchat_bots SET install_count = install_count + 1 WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, request.botId]
    );

    return result.rows[0] as unknown as BotInstallationRecord;
  }

  async uninstallBot(installationId: string, uninstalledBy: string): Promise<boolean> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_bot_installations
       SET is_active = false, uninstalled_at = NOW(), uninstalled_by = $3, updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND is_active = true
       RETURNING bot_id`,
      [this.sourceAccountId, installationId, uninstalledBy]
    );

    if (result.rows.length > 0) {
      const botId = (result.rows[0] as Record<string, unknown>).bot_id as string;
      await this.execute(
        'UPDATE nchat_bots SET install_count = GREATEST(install_count - 1, 0) WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, botId]
      );
      return true;
    }
    return false;
  }

  async listInstallations(workspaceId: string, isActive = true): Promise<BotInstallationRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bot_installations WHERE source_account_id = $1 AND workspace_id = $2 AND is_active = $3 ORDER BY installed_at DESC',
      [this.sourceAccountId, workspaceId, isActive]
    );
    return result.rows as unknown as BotInstallationRecord[];
  }

  // =========================================================================
  // Bot Messages
  // =========================================================================

  async createBotMessage(botId: string, messageId: string, channelId: string, messageType: string, metadata?: Record<string, unknown>): Promise<BotMessageRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_messages (source_account_id, bot_id, message_id, channel_id, message_type, metadata)
       VALUES ($1,$2,$3,$4,$5,$6) RETURNING *`,
      [this.sourceAccountId, botId, messageId, channelId, messageType, JSON.stringify(metadata ?? {})]
    );

    await this.execute(
      'UPDATE nchat_bots SET message_count = message_count + 1, last_message_at = NOW(), last_active_at = NOW() WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, botId]
    );

    return result.rows[0] as unknown as BotMessageRecord;
  }

  // =========================================================================
  // Interactions
  // =========================================================================

  async createInteraction(request: CreateInteractionRequest): Promise<BotInteractionRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_interactions (
        source_account_id, bot_id, message_id, user_id,
        interaction_type, interaction_id, interaction_value, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *`,
      [
        this.sourceAccountId, request.botId, request.messageId, request.userId,
        request.interactionType, request.interactionId,
        request.interactionValue ? JSON.stringify(request.interactionValue) : null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    // Update interaction count on bot message
    await this.execute(
      'UPDATE nchat_bot_messages SET interaction_count = interaction_count + 1, last_interaction_at = NOW() WHERE source_account_id = $1 AND message_id = $2',
      [this.sourceAccountId, request.messageId]
    );

    return result.rows[0] as unknown as BotInteractionRecord;
  }

  async markInteractionResponded(interactionId: string, responseMessageId: string): Promise<void> {
    await this.execute(
      'UPDATE nchat_bot_interactions SET response_sent = true, response_message_id = $3 WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, interactionId, responseMessageId]
    );
  }

  // =========================================================================
  // Reviews
  // =========================================================================

  async createReview(request: CreateReviewRequest): Promise<BotReviewRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_reviews (source_account_id, bot_id, user_id, rating, title, comment)
       VALUES ($1,$2,$3,$4,$5,$6)
       ON CONFLICT (source_account_id, bot_id, user_id) DO UPDATE SET
         rating = EXCLUDED.rating, title = EXCLUDED.title, comment = EXCLUDED.comment, updated_at = NOW()
       RETURNING *`,
      [this.sourceAccountId, request.botId, request.userId, request.rating, request.title ?? null, request.comment ?? null]
    );

    // Update bot rating
    await this.execute(
      `UPDATE nchat_bots SET
        rating_avg = (SELECT AVG(rating) FROM nchat_bot_reviews WHERE source_account_id = $1 AND bot_id = $2 AND is_published = true),
        rating_count = (SELECT COUNT(*) FROM nchat_bot_reviews WHERE source_account_id = $1 AND bot_id = $2 AND is_published = true)
      WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, request.botId]
    );

    return result.rows[0] as unknown as BotReviewRecord;
  }

  async listReviews(botId: string, limit = 20, offset = 0): Promise<BotReviewRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_bot_reviews WHERE source_account_id = $1 AND bot_id = $2 AND is_published = true ORDER BY created_at DESC LIMIT $3 OFFSET $4',
      [this.sourceAccountId, botId, limit, offset]
    );
    return result.rows as unknown as BotReviewRecord[];
  }

  // =========================================================================
  // API Keys
  // =========================================================================

  async createApiKey(request: CreateApiKeyRequest): Promise<{ apiKey: BotApiKeyRecord; rawKey: string }> {
    const rawKey = `nbot_${crypto.randomBytes(24).toString('hex')}`;
    const keyHash = crypto.createHash('sha256').update(rawKey).digest('hex');
    const keyPrefix = rawKey.substring(0, 12);

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_bot_api_keys (
        source_account_id, bot_id, key_name, key_hash, key_prefix,
        permissions, scopes, rate_limit_per_minute, rate_limit_per_hour,
        expires_at, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *`,
      [
        this.sourceAccountId, request.botId, request.keyName, keyHash, keyPrefix,
        request.permissions, request.scopes ?? [],
        request.rateLimitPerMinute ?? null, request.rateLimitPerHour ?? null,
        request.expiresAt ?? null, JSON.stringify({}),
      ]
    );

    return { apiKey: result.rows[0] as unknown as BotApiKeyRecord, rawKey };
  }

  async revokeApiKey(keyId: string, revokedBy: string, reason?: string): Promise<boolean> {
    const count = await this.execute(
      `UPDATE nchat_bot_api_keys SET is_active = false, revoked_at = NOW(), revoked_by = $3, revoke_reason = $4, updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 AND is_active = true`,
      [this.sourceAccountId, keyId, revokedBy, reason ?? null]
    );
    return count > 0;
  }

  // =========================================================================
  // Marketplace
  // =========================================================================

  async searchMarketplace(query: MarketplaceQuery): Promise<BotRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'is_public = true', 'is_enabled = true'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (query.category) { conditions.push(`category = $${paramIndex++}`); params.push(query.category); }
    if (query.verified) { conditions.push('is_verified = true'); }
    if (query.search) {
      conditions.push(`(name ILIKE $${paramIndex} OR description ILIKE $${paramIndex})`);
      params.push(`%${query.search}%`);
      paramIndex++;
    }
    if (query.tags && query.tags.length > 0) {
      conditions.push(`tags && $${paramIndex++}`);
      params.push(query.tags);
    }

    let orderBy = 'install_count DESC';
    if (query.sort === 'rating') orderBy = 'rating_avg DESC';
    else if (query.sort === 'recent') orderBy = 'created_at DESC';

    const limit = query.limit ?? 20;
    const offset = query.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_bots WHERE ${conditions.join(' AND ')}
       ORDER BY ${orderBy}
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as BotRecord[];
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<BotsStats> {
    const botsResult = await this.query<{ total: string; enabled: string; public_count: string; verified: string }>(
      `SELECT COUNT(*) as total,
        COUNT(*) FILTER (WHERE is_enabled = true) as enabled,
        COUNT(*) FILTER (WHERE is_public = true) as public_count,
        COUNT(*) FILTER (WHERE is_verified = true) as verified
      FROM nchat_bots WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const commandsResult = await this.query<{ total: string }>(
      'SELECT COUNT(*) as total FROM nchat_bot_commands WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    const subsResult = await this.query<{ total: string }>(
      'SELECT COUNT(*) as total FROM nchat_bot_subscriptions WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    const installsResult = await this.query<{ total: string; active: string }>(
      `SELECT COUNT(*) as total, COUNT(*) FILTER (WHERE is_active = true) as active
      FROM nchat_bot_installations WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const keysResult = await this.query<{ total: string }>(
      'SELECT COUNT(*) as total FROM nchat_bot_api_keys WHERE source_account_id = $1 AND is_active = true',
      [this.sourceAccountId]
    );

    const b = botsResult.rows[0];
    return {
      totalBots: parseInt(b?.total ?? '0', 10),
      enabledBots: parseInt(b?.enabled ?? '0', 10),
      publicBots: parseInt(b?.public_count ?? '0', 10),
      verifiedBots: parseInt(b?.verified ?? '0', 10),
      totalCommands: parseInt(commandsResult.rows[0]?.total ?? '0', 10),
      totalSubscriptions: parseInt(subsResult.rows[0]?.total ?? '0', 10),
      totalInstallations: parseInt(installsResult.rows[0]?.total ?? '0', 10),
      activeInstallations: parseInt(installsResult.rows[0]?.active ?? '0', 10),
      totalApiKeys: parseInt(keysResult.rows[0]?.total ?? '0', 10),
    };
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO nchat_bots_webhook_events (id, source_account_id, event_type, payload) VALUES ($1,$2,$3,$4)`,
      [`evt_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      'UPDATE nchat_bots_webhook_events SET processed = true, processed_at = NOW(), error = $2 WHERE id = $1',
      [eventId, error ?? null]
    );
  }
}
