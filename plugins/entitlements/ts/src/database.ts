/**
 * Entitlements Database Operations
 * Complete CRUD operations for all entitlement objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  EntitlementPlanRecord,
  CreatePlanRequest,
  UpdatePlanRequest,
  EntitlementSubscriptionRecord,
  CreateSubscriptionRequest,
  UpdateSubscriptionRequest,
  EntitlementFeatureRecord,
  CreateFeatureRequest,
  UpdateFeatureRequest,
  EntitlementQuotaRecord,
  TrackUsageRequest,
  EntitlementAddonRecord,
  AddAddonRequest,
  EntitlementGrantRecord,
  CreateGrantRequest,
  EntitlementEventRecord,
  FeatureAccessResult,
  QuotaAvailabilityResult,
  UsageTrackingResult,
  EntitlementStats,
  EntitlementEventType,
  BillingInterval,
  PlanType,
  SubscriptionStatus,
} from './types.js';

const logger = createLogger('entitlements:db');

export class EntitlementsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): EntitlementsDatabase {
    return new EntitlementsDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing entitlements schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Plans Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_plans (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name TEXT NOT NULL,
        slug TEXT NOT NULL,
        description TEXT,
        billing_interval VARCHAR(32) NOT NULL,
        price_cents INTEGER NOT NULL DEFAULT 0,
        currency VARCHAR(8) NOT NULL DEFAULT 'USD',
        trial_days INTEGER DEFAULT 0,
        trial_limits JSONB,
        plan_type VARCHAR(32) NOT NULL DEFAULT 'standard',
        is_public BOOLEAN DEFAULT true,
        is_archived BOOLEAN DEFAULT false,
        features JSONB NOT NULL DEFAULT '{}',
        quotas JSONB NOT NULL DEFAULT '{}',
        metadata JSONB,
        display_order INTEGER DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_plans_source_account ON entitlement_plans(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_plans_slug ON entitlement_plans(source_account_id, slug);
      CREATE INDEX IF NOT EXISTS idx_entitlement_plans_type ON entitlement_plans(plan_type);
      CREATE INDEX IF NOT EXISTS idx_entitlement_plans_public ON entitlement_plans(is_public);
      CREATE INDEX IF NOT EXISTS idx_entitlement_plans_features ON entitlement_plans USING GIN(features);

      -- =====================================================================
      -- Subscriptions Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_subscriptions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255),
        user_id VARCHAR(255),
        plan_id UUID NOT NULL REFERENCES entitlement_plans(id) ON DELETE RESTRICT,
        status VARCHAR(32) NOT NULL DEFAULT 'active',
        billing_interval VARCHAR(32) NOT NULL,
        price_cents INTEGER NOT NULL,
        currency VARCHAR(8) NOT NULL DEFAULT 'USD',
        is_custom_pricing BOOLEAN DEFAULT false,
        custom_quotas JSONB,
        custom_features JSONB,
        payment_provider VARCHAR(32),
        payment_provider_subscription_id TEXT,
        payment_provider_customer_id TEXT,
        trial_start TIMESTAMP WITH TIME ZONE,
        trial_end TIMESTAMP WITH TIME ZONE,
        current_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
        current_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
        cancel_at_period_end BOOLEAN DEFAULT false,
        canceled_at TIMESTAMP WITH TIME ZONE,
        cancellation_reason TEXT,
        pause_collection VARCHAR(32),
        pause_start TIMESTAMP WITH TIME ZONE,
        pause_end TIMESTAMP WITH TIME ZONE,
        metadata JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_source_account ON entitlement_subscriptions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_workspace ON entitlement_subscriptions(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_user ON entitlement_subscriptions(user_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_plan ON entitlement_subscriptions(plan_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_status ON entitlement_subscriptions(status);
      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_provider ON entitlement_subscriptions(payment_provider_subscription_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_subscriptions_period_end ON entitlement_subscriptions(current_period_end);

      -- =====================================================================
      -- Features Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_features (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        key TEXT NOT NULL,
        name TEXT NOT NULL,
        description TEXT,
        feature_type VARCHAR(32) NOT NULL,
        default_value JSONB,
        category TEXT,
        metadata JSONB,
        is_active BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_features_source_account ON entitlement_features(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_features_key ON entitlement_features(source_account_id, key);
      CREATE INDEX IF NOT EXISTS idx_entitlement_features_type ON entitlement_features(feature_type);
      CREATE INDEX IF NOT EXISTS idx_entitlement_features_category ON entitlement_features(category);
      CREATE INDEX IF NOT EXISTS idx_entitlement_features_active ON entitlement_features(is_active);

      -- =====================================================================
      -- Quotas Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_quotas (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255),
        user_id VARCHAR(255),
        subscription_id UUID NOT NULL REFERENCES entitlement_subscriptions(id) ON DELETE CASCADE,
        quota_key TEXT NOT NULL,
        quota_name TEXT NOT NULL,
        limit_value BIGINT,
        is_unlimited BOOLEAN DEFAULT false,
        current_usage BIGINT DEFAULT 0,
        reset_interval VARCHAR(32),
        last_reset_at TIMESTAMP WITH TIME ZONE,
        next_reset_at TIMESTAMP WITH TIME ZONE,
        metadata JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_quotas_source_account ON entitlement_quotas(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_quotas_workspace ON entitlement_quotas(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_quotas_user ON entitlement_quotas(user_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_quotas_subscription ON entitlement_quotas(subscription_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_quotas_key ON entitlement_quotas(quota_key);
      CREATE INDEX IF NOT EXISTS idx_entitlement_quotas_next_reset ON entitlement_quotas(next_reset_at);

      -- =====================================================================
      -- Usage Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_usage (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255),
        user_id VARCHAR(255),
        quota_id UUID NOT NULL REFERENCES entitlement_quotas(id) ON DELETE CASCADE,
        quota_key TEXT NOT NULL,
        usage_amount BIGINT NOT NULL DEFAULT 1,
        resource_type TEXT,
        resource_id VARCHAR(255),
        metadata JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_usage_source_account ON entitlement_usage(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_usage_workspace ON entitlement_usage(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_usage_user ON entitlement_usage(user_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_usage_quota ON entitlement_usage(quota_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_usage_key ON entitlement_usage(quota_key);
      CREATE INDEX IF NOT EXISTS idx_entitlement_usage_created ON entitlement_usage(created_at DESC);

      -- =====================================================================
      -- Addons Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_addons (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        addon_plan_id UUID NOT NULL REFERENCES entitlement_plans(id) ON DELETE RESTRICT,
        subscription_id UUID NOT NULL REFERENCES entitlement_subscriptions(id) ON DELETE CASCADE,
        quantity INTEGER NOT NULL DEFAULT 1,
        price_cents INTEGER NOT NULL,
        currency VARCHAR(8) NOT NULL DEFAULT 'USD',
        status VARCHAR(32) NOT NULL DEFAULT 'active',
        current_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
        current_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
        payment_provider_item_id TEXT,
        metadata JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_addons_source_account ON entitlement_addons(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_addons_subscription ON entitlement_addons(subscription_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_addons_plan ON entitlement_addons(addon_plan_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_addons_status ON entitlement_addons(status);

      -- =====================================================================
      -- Grants Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_grants (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255),
        user_id VARCHAR(255),
        feature_key TEXT NOT NULL,
        feature_value JSONB NOT NULL,
        granted_by VARCHAR(255),
        grant_reason TEXT,
        expires_at TIMESTAMP WITH TIME ZONE,
        is_active BOOLEAN DEFAULT true,
        metadata JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_grants_source_account ON entitlement_grants(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_grants_workspace ON entitlement_grants(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_grants_user ON entitlement_grants(user_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_grants_feature ON entitlement_grants(feature_key);
      CREATE INDEX IF NOT EXISTS idx_entitlement_grants_active ON entitlement_grants(is_active);
      CREATE INDEX IF NOT EXISTS idx_entitlement_grants_expires ON entitlement_grants(expires_at);

      -- =====================================================================
      -- Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS entitlement_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(64) NOT NULL,
        workspace_id VARCHAR(255),
        user_id VARCHAR(255),
        subscription_id UUID REFERENCES entitlement_subscriptions(id) ON DELETE SET NULL,
        plan_id UUID REFERENCES entitlement_plans(id) ON DELETE SET NULL,
        event_data JSONB,
        actor_user_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_entitlement_events_source_account ON entitlement_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_events_type ON entitlement_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_entitlement_events_workspace ON entitlement_events(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_events_user ON entitlement_events(user_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_events_subscription ON entitlement_events(subscription_id);
      CREATE INDEX IF NOT EXISTS idx_entitlement_events_created ON entitlement_events(created_at DESC);
    `;

    await this.execute(schema);
    logger.success('Entitlements schema initialized');
  }

  // =========================================================================
  // Plan Operations
  // =========================================================================

  async createPlan(plan: CreatePlanRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO entitlement_plans
       (id, source_account_id, name, slug, description, billing_interval, price_cents, currency,
        trial_days, trial_limits, plan_type, is_public, features, quotas, metadata, display_order,
        created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW())`,
      [
        id, this.sourceAccountId, plan.name, plan.slug, plan.description ?? null,
        plan.billing_interval, plan.price_cents, plan.currency ?? 'USD',
        plan.trial_days ?? 0, plan.trial_limits ? JSON.stringify(plan.trial_limits) : null,
        plan.plan_type, plan.is_public ?? true,
        JSON.stringify(plan.features), JSON.stringify(plan.quotas),
        plan.metadata ? JSON.stringify(plan.metadata) : null, plan.display_order ?? 0,
      ]
    );
    return id;
  }

  async getPlan(id: string): Promise<EntitlementPlanRecord | null> {
    const result = await this.query<EntitlementPlanRecord>(
      `SELECT * FROM entitlement_plans WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getPlanBySlug(slug: string): Promise<EntitlementPlanRecord | null> {
    const result = await this.query<EntitlementPlanRecord>(
      `SELECT * FROM entitlement_plans WHERE slug = $1 AND source_account_id = $2`,
      [slug, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listPlans(filters?: { plan_type?: PlanType; billing_interval?: BillingInterval; is_public?: boolean; is_archived?: boolean }): Promise<EntitlementPlanRecord[]> {
    let sql = `SELECT * FROM entitlement_plans WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.plan_type) {
      sql += ` AND plan_type = $${paramIndex}`;
      params.push(filters.plan_type);
      paramIndex++;
    }
    if (filters?.billing_interval) {
      sql += ` AND billing_interval = $${paramIndex}`;
      params.push(filters.billing_interval);
      paramIndex++;
    }
    if (filters?.is_public !== undefined) {
      sql += ` AND is_public = $${paramIndex}`;
      params.push(filters.is_public);
      paramIndex++;
    }
    if (filters?.is_archived !== undefined) {
      sql += ` AND is_archived = $${paramIndex}`;
      params.push(filters.is_archived);
      paramIndex++;
    }

    sql += ` ORDER BY display_order ASC, created_at ASC`;

    const result = await this.query<EntitlementPlanRecord>(sql, params);
    return result.rows;
  }

  async updatePlan(id: string, updates: UpdatePlanRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex}`); params.push(updates.name); paramIndex++; }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex}`); params.push(updates.description); paramIndex++; }
    if (updates.price_cents !== undefined) { sets.push(`price_cents = $${paramIndex}`); params.push(updates.price_cents); paramIndex++; }
    if (updates.trial_days !== undefined) { sets.push(`trial_days = $${paramIndex}`); params.push(updates.trial_days); paramIndex++; }
    if (updates.is_public !== undefined) { sets.push(`is_public = $${paramIndex}`); params.push(updates.is_public); paramIndex++; }
    if (updates.features !== undefined) { sets.push(`features = $${paramIndex}`); params.push(JSON.stringify(updates.features)); paramIndex++; }
    if (updates.quotas !== undefined) { sets.push(`quotas = $${paramIndex}`); params.push(JSON.stringify(updates.quotas)); paramIndex++; }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex}`); params.push(JSON.stringify(updates.metadata)); paramIndex++; }
    if (updates.display_order !== undefined) { sets.push(`display_order = $${paramIndex}`); params.push(updates.display_order); paramIndex++; }

    if (sets.length === 0) return false;
    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE entitlement_plans SET ${sets.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  async archivePlan(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_plans SET is_archived = true, updated_at = NOW() WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Subscription Operations
  // =========================================================================

  async createSubscription(sub: CreateSubscriptionRequest, _defaultTrialDays = 14): Promise<string> {
    const id = crypto.randomUUID();
    const plan = await this.getPlan(sub.plan_id);
    if (!plan) throw new Error('Plan not found');

    const billingInterval = sub.billing_interval ?? plan.billing_interval;
    const priceCents = sub.custom_price_cents ?? plan.price_cents;
    const now = new Date();
    const periodEnd = new Date(now);

    if (billingInterval === 'month') periodEnd.setMonth(periodEnd.getMonth() + 1);
    else if (billingInterval === 'year') periodEnd.setFullYear(periodEnd.getFullYear() + 1);
    else periodEnd.setMonth(periodEnd.getMonth() + 1);

    const isTrial = sub.start_trial && plan.trial_days > 0;
    const trialEnd = isTrial ? new Date(now.getTime() + (plan.trial_days * 24 * 60 * 60 * 1000)) : null;

    await this.execute(
      `INSERT INTO entitlement_subscriptions
       (id, source_account_id, workspace_id, user_id, plan_id, status, billing_interval,
        price_cents, currency, is_custom_pricing, custom_quotas, custom_features,
        payment_provider, payment_provider_subscription_id, payment_provider_customer_id,
        trial_start, trial_end, current_period_start, current_period_end,
        metadata, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW(), NOW())`,
      [
        id, this.sourceAccountId, sub.workspace_id ?? null, sub.user_id ?? null,
        sub.plan_id, isTrial ? 'trialing' : 'active', billingInterval,
        priceCents, plan.currency,
        sub.custom_price_cents !== undefined, sub.custom_quotas ? JSON.stringify(sub.custom_quotas) : null,
        sub.custom_features ? JSON.stringify(sub.custom_features) : null,
        sub.payment_provider ?? null, sub.payment_provider_subscription_id ?? null,
        sub.payment_provider_customer_id ?? null,
        isTrial ? now : null, trialEnd, now, periodEnd,
        sub.metadata ? JSON.stringify(sub.metadata) : null,
      ]
    );

    await this.logEvent('subscription_created', sub.workspace_id ?? null, sub.user_id ?? null, id, sub.plan_id);
    return id;
  }

  async getSubscription(id: string): Promise<EntitlementSubscriptionRecord | null> {
    const result = await this.query<EntitlementSubscriptionRecord>(
      `SELECT * FROM entitlement_subscriptions WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getActiveSubscription(workspaceId?: string, userId?: string): Promise<EntitlementSubscriptionRecord | null> {
    let sql = `SELECT * FROM entitlement_subscriptions WHERE source_account_id = $1 AND status IN ('active', 'trialing', 'past_due')`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (workspaceId) {
      sql += ` AND workspace_id = $${paramIndex}`;
      params.push(workspaceId);
      paramIndex++;
    }
    if (userId) {
      sql += ` AND user_id = $${paramIndex}`;
      params.push(userId);
      paramIndex++;
    }

    sql += ` ORDER BY created_at DESC LIMIT 1`;

    const result = await this.query<EntitlementSubscriptionRecord>(sql, params);
    return result.rows[0] ?? null;
  }

  async listSubscriptions(filters?: { workspace_id?: string; user_id?: string; status?: SubscriptionStatus; plan_id?: string }): Promise<EntitlementSubscriptionRecord[]> {
    let sql = `SELECT * FROM entitlement_subscriptions WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.workspace_id) { sql += ` AND workspace_id = $${paramIndex}`; params.push(filters.workspace_id); paramIndex++; }
    if (filters?.user_id) { sql += ` AND user_id = $${paramIndex}`; params.push(filters.user_id); paramIndex++; }
    if (filters?.status) { sql += ` AND status = $${paramIndex}`; params.push(filters.status); paramIndex++; }
    if (filters?.plan_id) { sql += ` AND plan_id = $${paramIndex}`; params.push(filters.plan_id); paramIndex++; }

    sql += ` ORDER BY created_at DESC`;

    const result = await this.query<EntitlementSubscriptionRecord>(sql, params);
    return result.rows;
  }

  async updateSubscription(id: string, updates: UpdateSubscriptionRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.custom_quotas !== undefined) { sets.push(`custom_quotas = $${paramIndex}`); params.push(JSON.stringify(updates.custom_quotas)); paramIndex++; }
    if (updates.custom_features !== undefined) { sets.push(`custom_features = $${paramIndex}`); params.push(JSON.stringify(updates.custom_features)); paramIndex++; }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex}`); params.push(JSON.stringify(updates.metadata)); paramIndex++; }

    if (sets.length === 0) return false;
    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE entitlement_subscriptions SET ${sets.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  async cancelSubscription(id: string, reason?: string, immediate = false): Promise<boolean> {
    const sub = await this.getSubscription(id);
    if (!sub) return false;

    if (immediate) {
      await this.execute(
        `UPDATE entitlement_subscriptions SET status = 'canceled', canceled_at = NOW(), cancellation_reason = $1, updated_at = NOW()
         WHERE id = $2 AND source_account_id = $3`,
        [reason ?? null, id, this.sourceAccountId]
      );
    } else {
      await this.execute(
        `UPDATE entitlement_subscriptions SET cancel_at_period_end = true, cancellation_reason = $1, updated_at = NOW()
         WHERE id = $2 AND source_account_id = $3`,
        [reason ?? null, id, this.sourceAccountId]
      );
    }

    await this.logEvent('subscription_canceled', sub.workspace_id, sub.user_id, id, sub.plan_id);
    return true;
  }

  async pauseSubscription(id: string, resumeAt?: Date): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_subscriptions SET status = 'paused', pause_start = NOW(), pause_end = $1, updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3`,
      [resumeAt ?? null, id, this.sourceAccountId]
    );
    return result > 0;
  }

  async resumeSubscription(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_subscriptions SET status = 'active', pause_start = NULL, pause_end = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Feature Operations
  // =========================================================================

  async createFeature(feat: CreateFeatureRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO entitlement_features
       (id, source_account_id, key, name, description, feature_type, default_value, category, metadata, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
      [
        id, this.sourceAccountId, feat.key, feat.name, feat.description ?? null,
        feat.feature_type, feat.default_value ? JSON.stringify(feat.default_value) : null,
        feat.category ?? null, feat.metadata ? JSON.stringify(feat.metadata) : null,
      ]
    );
    return id;
  }

  async getFeature(key: string): Promise<EntitlementFeatureRecord | null> {
    const result = await this.query<EntitlementFeatureRecord>(
      `SELECT * FROM entitlement_features WHERE key = $1 AND source_account_id = $2`,
      [key, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listFeatures(category?: string, isActive?: boolean): Promise<EntitlementFeatureRecord[]> {
    let sql = `SELECT * FROM entitlement_features WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (category) { sql += ` AND category = $${paramIndex}`; params.push(category); paramIndex++; }
    if (isActive !== undefined) { sql += ` AND is_active = $${paramIndex}`; params.push(isActive); paramIndex++; }

    sql += ` ORDER BY category NULLS LAST, name ASC`;

    const result = await this.query<EntitlementFeatureRecord>(sql, params);
    return result.rows;
  }

  async updateFeature(key: string, updates: UpdateFeatureRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex}`); params.push(updates.name); paramIndex++; }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex}`); params.push(updates.description); paramIndex++; }
    if (updates.default_value !== undefined) { sets.push(`default_value = $${paramIndex}`); params.push(JSON.stringify(updates.default_value)); paramIndex++; }
    if (updates.category !== undefined) { sets.push(`category = $${paramIndex}`); params.push(updates.category); paramIndex++; }
    if (updates.is_active !== undefined) { sets.push(`is_active = $${paramIndex}`); params.push(updates.is_active); paramIndex++; }

    if (sets.length === 0) return false;
    sets.push('updated_at = NOW()');
    params.push(key, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE entitlement_features SET ${sets.join(', ')} WHERE key = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  // =========================================================================
  // Feature Access Check
  // =========================================================================

  async checkFeatureAccess(featureKey: string, workspaceId?: string, userId?: string): Promise<FeatureAccessResult> {
    // Check grants first
    let grantSql = `SELECT feature_value FROM entitlement_grants
      WHERE source_account_id = $1 AND feature_key = $2 AND is_active = true
        AND (expires_at IS NULL OR expires_at > NOW())`;
    const grantParams: unknown[] = [this.sourceAccountId, featureKey];
    let gpi = 3;

    if (workspaceId) { grantSql += ` AND workspace_id = $${gpi}`; grantParams.push(workspaceId); gpi++; }
    if (userId) { grantSql += ` AND user_id = $${gpi}`; grantParams.push(userId); gpi++; }

    grantSql += ` ORDER BY created_at DESC LIMIT 1`;

    const grantResult = await this.query<{ feature_value: unknown }>(grantSql, grantParams);
    if (grantResult.rows[0]) {
      return { has_access: true, value: grantResult.rows[0].feature_value, source: 'grant' };
    }

    // Check subscription
    let subSql = `SELECT s.*, p.features FROM entitlement_subscriptions s
      JOIN entitlement_plans p ON s.plan_id = p.id
      WHERE s.source_account_id = $1 AND s.status IN ('active', 'trialing')`;
    const subParams: unknown[] = [this.sourceAccountId];
    let spi = 2;

    if (workspaceId) { subSql += ` AND s.workspace_id = $${spi}`; subParams.push(workspaceId); spi++; }
    if (userId) { subSql += ` AND s.user_id = $${spi}`; subParams.push(userId); spi++; }

    subSql += ` ORDER BY s.created_at DESC LIMIT 1`;

    const subResult = await this.query<EntitlementSubscriptionRecord & { features: Record<string, unknown> }>(subSql, subParams);
    if (!subResult.rows[0]) {
      return { has_access: false, value: null, source: 'none' };
    }

    const sub = subResult.rows[0];
    const customFeatures = sub.custom_features as Record<string, unknown> | null;
    const planFeatures = sub.features as Record<string, unknown>;
    const featureValue = customFeatures?.[featureKey] ?? planFeatures[featureKey] ?? null;

    return {
      has_access: featureValue !== null && featureValue !== false,
      value: featureValue,
      source: 'subscription',
    };
  }

  // =========================================================================
  // Quota Operations
  // =========================================================================

  async createQuota(subscriptionId: string, quotaKey: string, quotaName: string, limitValue: number | null, isUnlimited: boolean, resetInterval?: string): Promise<string> {
    const sub = await this.getSubscription(subscriptionId);
    if (!sub) throw new Error('Subscription not found');

    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO entitlement_quotas
       (id, source_account_id, workspace_id, user_id, subscription_id, quota_key, quota_name,
        limit_value, is_unlimited, reset_interval, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
      [
        id, this.sourceAccountId, sub.workspace_id, sub.user_id, subscriptionId,
        quotaKey, quotaName, limitValue, isUnlimited, resetInterval ?? null,
      ]
    );
    return id;
  }

  async getQuota(id: string): Promise<EntitlementQuotaRecord | null> {
    const result = await this.query<EntitlementQuotaRecord>(
      `SELECT * FROM entitlement_quotas WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listQuotas(filters?: { workspace_id?: string; user_id?: string; subscription_id?: string; quota_key?: string }): Promise<EntitlementQuotaRecord[]> {
    let sql = `SELECT * FROM entitlement_quotas WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.workspace_id) { sql += ` AND workspace_id = $${paramIndex}`; params.push(filters.workspace_id); paramIndex++; }
    if (filters?.user_id) { sql += ` AND user_id = $${paramIndex}`; params.push(filters.user_id); paramIndex++; }
    if (filters?.subscription_id) { sql += ` AND subscription_id = $${paramIndex}`; params.push(filters.subscription_id); paramIndex++; }
    if (filters?.quota_key) { sql += ` AND quota_key = $${paramIndex}`; params.push(filters.quota_key); paramIndex++; }

    sql += ` ORDER BY quota_key ASC`;

    const result = await this.query<EntitlementQuotaRecord>(sql, params);
    return result.rows;
  }

  async checkQuotaAvailability(quotaKey: string, requestedAmount = 1, workspaceId?: string, userId?: string): Promise<QuotaAvailabilityResult> {
    let sql = `SELECT * FROM entitlement_quotas WHERE source_account_id = $1 AND quota_key = $2`;
    const params: unknown[] = [this.sourceAccountId, quotaKey];
    let paramIndex = 3;

    if (workspaceId) { sql += ` AND workspace_id = $${paramIndex}`; params.push(workspaceId); paramIndex++; }
    if (userId) { sql += ` AND user_id = $${paramIndex}`; params.push(userId); paramIndex++; }

    sql += ` LIMIT 1`;

    const result = await this.query<EntitlementQuotaRecord>(sql, params);
    const quota = result.rows[0];

    if (!quota) return { available: false, reason: 'quota_not_found' };
    if (quota.is_unlimited) return { available: true, is_unlimited: true };

    const currentUsage = Number(quota.current_usage);
    const limitValue = Number(quota.limit_value);

    if (currentUsage + requestedAmount > limitValue) {
      return {
        available: false,
        reason: 'quota_exceeded',
        current_usage: currentUsage,
        limit_value: limitValue,
        requested: requestedAmount,
        remaining: Math.max(limitValue - currentUsage, 0),
      };
    }

    return {
      available: true,
      current_usage: currentUsage,
      limit_value: limitValue,
      remaining: limitValue - currentUsage - requestedAmount,
    };
  }

  async trackUsage(req: TrackUsageRequest): Promise<UsageTrackingResult> {
    const amount = req.usage_amount ?? 1;

    // Find matching quota
    let sql = `SELECT * FROM entitlement_quotas WHERE source_account_id = $1 AND quota_key = $2`;
    const params: unknown[] = [this.sourceAccountId, req.quota_key];
    let paramIndex = 3;

    if (req.workspace_id) { sql += ` AND workspace_id = $${paramIndex}`; params.push(req.workspace_id); paramIndex++; }
    if (req.user_id) { sql += ` AND user_id = $${paramIndex}`; params.push(req.user_id); paramIndex++; }

    sql += ` LIMIT 1`;

    const result = await this.query<EntitlementQuotaRecord>(sql, params);
    const quota = result.rows[0];

    if (!quota) return { success: false, error: 'quota_not_found' };

    const currentUsage = Number(quota.current_usage);
    const limitValue = Number(quota.limit_value);

    if (!quota.is_unlimited && currentUsage + amount > limitValue) {
      return {
        success: false,
        error: 'quota_exceeded',
        new_usage: currentUsage,
        limit_value: limitValue,
        remaining: Math.max(limitValue - currentUsage, 0),
      };
    }

    // Record usage
    const usageId = crypto.randomUUID();
    await this.execute(
      `INSERT INTO entitlement_usage
       (id, source_account_id, workspace_id, user_id, quota_id, quota_key,
        usage_amount, resource_type, resource_id, metadata, created_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
      [
        usageId, this.sourceAccountId, req.workspace_id ?? null, req.user_id ?? null,
        quota.id, req.quota_key, amount, req.resource_type ?? null, req.resource_id ?? null,
        req.metadata ? JSON.stringify(req.metadata) : null,
      ]
    );

    // Update quota usage
    await this.execute(
      `UPDATE entitlement_quotas SET current_usage = current_usage + $1, updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3`,
      [amount, quota.id, this.sourceAccountId]
    );

    const newUsage = currentUsage + amount;
    return {
      success: true,
      usage_id: usageId,
      new_usage: newUsage,
      limit_value: quota.is_unlimited ? undefined : limitValue,
      remaining: quota.is_unlimited ? undefined : limitValue - newUsage,
    };
  }

  async resetQuota(quotaId: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_quotas SET current_usage = 0, last_reset_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [quotaId, this.sourceAccountId]
    );
    return result > 0;
  }

  async updateQuotaLimit(quotaId: string, newLimit: number): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_quotas SET limit_value = $1, updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3`,
      [newLimit, quotaId, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Addon Operations
  // =========================================================================

  async addAddon(req: AddAddonRequest): Promise<string> {
    const addonPlan = await this.getPlan(req.addon_plan_id);
    if (!addonPlan) throw new Error('Addon plan not found');
    const sub = await this.getSubscription(req.subscription_id);
    if (!sub) throw new Error('Subscription not found');

    const id = crypto.randomUUID();
    const quantity = req.quantity ?? 1;

    await this.execute(
      `INSERT INTO entitlement_addons
       (id, source_account_id, addon_plan_id, subscription_id, quantity, price_cents, currency,
        current_period_start, current_period_end, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
      [
        id, this.sourceAccountId, req.addon_plan_id, req.subscription_id, quantity,
        addonPlan.price_cents * quantity, addonPlan.currency,
        sub.current_period_start, sub.current_period_end,
      ]
    );

    await this.logEvent('addon_added', sub.workspace_id, sub.user_id, sub.id, req.addon_plan_id);
    return id;
  }

  async removeAddon(addonId: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_addons SET status = 'canceled', updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [addonId, this.sourceAccountId]
    );
    return result > 0;
  }

  async listAddons(subscriptionId: string): Promise<EntitlementAddonRecord[]> {
    const result = await this.query<EntitlementAddonRecord>(
      `SELECT * FROM entitlement_addons WHERE subscription_id = $1 AND source_account_id = $2 ORDER BY created_at ASC`,
      [subscriptionId, this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Grant Operations
  // =========================================================================

  async createGrant(grant: CreateGrantRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO entitlement_grants
       (id, source_account_id, workspace_id, user_id, feature_key, feature_value,
        granted_by, grant_reason, expires_at, metadata, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
      [
        id, this.sourceAccountId, grant.workspace_id ?? null, grant.user_id ?? null,
        grant.feature_key, JSON.stringify(grant.feature_value),
        grant.granted_by ?? null, grant.grant_reason ?? null,
        grant.expires_at ? new Date(grant.expires_at) : null,
        grant.metadata ? JSON.stringify(grant.metadata) : null,
      ]
    );

    await this.logEvent('grant_created', grant.workspace_id ?? null, grant.user_id ?? null, null, null);
    return id;
  }

  async revokeGrant(grantId: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE entitlement_grants SET is_active = false, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [grantId, this.sourceAccountId]
    );
    return result > 0;
  }

  async listGrants(filters?: { workspace_id?: string; user_id?: string; feature_key?: string; is_active?: boolean }): Promise<EntitlementGrantRecord[]> {
    let sql = `SELECT * FROM entitlement_grants WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.workspace_id) { sql += ` AND workspace_id = $${paramIndex}`; params.push(filters.workspace_id); paramIndex++; }
    if (filters?.user_id) { sql += ` AND user_id = $${paramIndex}`; params.push(filters.user_id); paramIndex++; }
    if (filters?.feature_key) { sql += ` AND feature_key = $${paramIndex}`; params.push(filters.feature_key); paramIndex++; }
    if (filters?.is_active !== undefined) { sql += ` AND is_active = $${paramIndex}`; params.push(filters.is_active); paramIndex++; }

    sql += ` ORDER BY created_at DESC`;

    const result = await this.query<EntitlementGrantRecord>(sql, params);
    return result.rows;
  }

  // =========================================================================
  // Event Logging
  // =========================================================================

  async logEvent(eventType: EntitlementEventType, workspaceId: string | null, userId: string | null, subscriptionId: string | null, planId: string | null, eventData?: Record<string, unknown>, actorUserId?: string): Promise<void> {
    await this.execute(
      `INSERT INTO entitlement_events
       (source_account_id, event_type, workspace_id, user_id, subscription_id, plan_id, event_data, actor_user_id, created_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
      [
        this.sourceAccountId, eventType, workspaceId, userId, subscriptionId, planId,
        eventData ? JSON.stringify(eventData) : null, actorUserId ?? null,
      ]
    );
  }

  async listEvents(limit = 100, offset = 0, filters?: { workspace_id?: string; user_id?: string; event_type?: string; subscription_id?: string }): Promise<EntitlementEventRecord[]> {
    let sql = `SELECT * FROM entitlement_events WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.workspace_id) { sql += ` AND workspace_id = $${paramIndex}`; params.push(filters.workspace_id); paramIndex++; }
    if (filters?.user_id) { sql += ` AND user_id = $${paramIndex}`; params.push(filters.user_id); paramIndex++; }
    if (filters?.event_type) { sql += ` AND event_type = $${paramIndex}`; params.push(filters.event_type); paramIndex++; }
    if (filters?.subscription_id) { sql += ` AND subscription_id = $${paramIndex}`; params.push(filters.subscription_id); paramIndex++; }

    sql += ` ORDER BY created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<EntitlementEventRecord>(sql, params);
    return result.rows;
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(): Promise<EntitlementStats> {
    const plans = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_plans WHERE source_account_id = $1 AND is_archived = false`, [this.sourceAccountId]);
    const activeSubs = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_subscriptions WHERE source_account_id = $1 AND status = 'active'`, [this.sourceAccountId]);
    const trialSubs = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_subscriptions WHERE source_account_id = $1 AND status = 'trialing'`, [this.sourceAccountId]);
    const features = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_features WHERE source_account_id = $1 AND is_active = true`, [this.sourceAccountId]);
    const grants = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_grants WHERE source_account_id = $1 AND is_active = true`, [this.sourceAccountId]);
    const quotas = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_quotas WHERE source_account_id = $1`, [this.sourceAccountId]);
    const exceeded = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_quotas WHERE source_account_id = $1 AND is_unlimited = false AND current_usage >= limit_value`, [this.sourceAccountId]);
    const events = await this.query<{ count: string }>(`SELECT COUNT(*) as count FROM entitlement_events WHERE source_account_id = $1`, [this.sourceAccountId]);

    // MRR: sum of active monthly subscription price_cents
    const mrr = await this.query<{ total: string }>(`SELECT COALESCE(SUM(
      CASE WHEN billing_interval = 'month' THEN price_cents
           WHEN billing_interval = 'year' THEN price_cents / 12
           ELSE 0 END
    ), 0) as total FROM entitlement_subscriptions WHERE source_account_id = $1 AND status = 'active'`, [this.sourceAccountId]);

    return {
      total_plans: parseInt(plans.rows[0]?.count ?? '0', 10),
      active_subscriptions: parseInt(activeSubs.rows[0]?.count ?? '0', 10),
      trialing_subscriptions: parseInt(trialSubs.rows[0]?.count ?? '0', 10),
      total_features: parseInt(features.rows[0]?.count ?? '0', 10),
      total_grants: parseInt(grants.rows[0]?.count ?? '0', 10),
      active_quotas: parseInt(quotas.rows[0]?.count ?? '0', 10),
      exceeded_quotas: parseInt(exceeded.rows[0]?.count ?? '0', 10),
      total_events: parseInt(events.rows[0]?.count ?? '0', 10),
      mrr_cents: parseInt(mrr.rows[0]?.total ?? '0', 10),
    };
  }
}
