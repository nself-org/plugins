/**
 * Feature Flags Database Operations
 * Complete CRUD operations for flags, rules, segments, and evaluations
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  FlagRecord,
  CreateFlagRequest,
  UpdateFlagRequest,
  RuleRecord,
  CreateRuleRequest,
  UpdateRuleRequest,
  SegmentRecord,
  CreateSegmentRequest,
  UpdateSegmentRequest,
  EvaluationRecord,
  EvaluationResult,
  EvaluationContext,
  WebhookEventRecord,
  FlagStats,
  FlagDetail,
  ListFlagsOptions,
  ListEvaluationsOptions,
} from './types.js';

const logger = createLogger('feature-flags:db');

export class FeatureFlagsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): FeatureFlagsDatabase {
    return new FeatureFlagsDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing feature flags schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Flags
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ff_flags (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        key VARCHAR(255) NOT NULL,
        name VARCHAR(255),
        description TEXT,
        flag_type VARCHAR(16) DEFAULT 'release',
        enabled BOOLEAN DEFAULT false,
        default_value JSONB DEFAULT 'false',
        tags TEXT[] DEFAULT '{}',
        owner VARCHAR(255),
        stale_after_days INTEGER,
        last_evaluated_at TIMESTAMP WITH TIME ZONE,
        evaluation_count BIGINT DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, key)
      );
      CREATE INDEX IF NOT EXISTS idx_ff_flags_source_account ON ff_flags(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ff_flags_key ON ff_flags(key);
      CREATE INDEX IF NOT EXISTS idx_ff_flags_type ON ff_flags(flag_type);
      CREATE INDEX IF NOT EXISTS idx_ff_flags_enabled ON ff_flags(enabled);
      CREATE INDEX IF NOT EXISTS idx_ff_flags_tags ON ff_flags USING GIN(tags);

      -- =====================================================================
      -- Rules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ff_rules (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        flag_id UUID NOT NULL REFERENCES ff_flags(id) ON DELETE CASCADE,
        name VARCHAR(255),
        rule_type VARCHAR(32) NOT NULL,
        conditions JSONB NOT NULL,
        value JSONB NOT NULL DEFAULT 'true',
        priority INTEGER DEFAULT 0,
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_ff_rules_source_account ON ff_rules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ff_rules_flag_id ON ff_rules(flag_id);
      CREATE INDEX IF NOT EXISTS idx_ff_rules_priority ON ff_rules(priority DESC);

      -- =====================================================================
      -- Segments
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ff_segments (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        match_type VARCHAR(8) DEFAULT 'all',
        rules JSONB NOT NULL DEFAULT '[]',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
      CREATE INDEX IF NOT EXISTS idx_ff_segments_source_account ON ff_segments(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ff_segments_name ON ff_segments(name);

      -- =====================================================================
      -- Evaluations
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ff_evaluations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        flag_key VARCHAR(255) NOT NULL,
        user_id VARCHAR(255),
        context JSONB,
        result JSONB,
        rule_id UUID,
        reason VARCHAR(64),
        evaluated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_ff_evaluations_source_account ON ff_evaluations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ff_evaluations_flag_key ON ff_evaluations(flag_key);
      CREATE INDEX IF NOT EXISTS idx_ff_evaluations_user_id ON ff_evaluations(user_id);
      CREATE INDEX IF NOT EXISTS idx_ff_evaluations_evaluated_at ON ff_evaluations(evaluated_at DESC);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS ff_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_ff_webhook_events_source_account ON ff_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ff_webhook_events_type ON ff_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_ff_webhook_events_processed ON ff_webhook_events(processed);
    `;

    await this.execute(schema);
    logger.info('Schema initialized successfully');
  }

  // =========================================================================
  // Flags
  // =========================================================================

  async createFlag(request: CreateFlagRequest): Promise<FlagRecord> {
    const result = await this.query<FlagRecord>(
      `INSERT INTO ff_flags (
        source_account_id, key, name, description, flag_type, enabled,
        default_value, tags, owner, stale_after_days
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.key,
        request.name ?? null,
        request.description ?? null,
        request.flag_type ?? 'release',
        request.enabled ?? false,
        JSON.stringify(request.default_value ?? false),
        request.tags ?? [],
        request.owner ?? null,
        request.stale_after_days ?? null,
      ]
    );

    return result.rows[0];
  }

  async getFlag(key: string): Promise<FlagRecord | null> {
    const result = await this.query<FlagRecord>(
      `SELECT * FROM ff_flags WHERE source_account_id = $1 AND key = $2`,
      [this.sourceAccountId, key]
    );

    return result.rows[0] ?? null;
  }

  async getFlagById(id: string): Promise<FlagRecord | null> {
    const result = await this.query<FlagRecord>(
      `SELECT * FROM ff_flags WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id]
    );

    return result.rows[0] ?? null;
  }

  async listFlags(options: ListFlagsOptions = {}): Promise<FlagRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.flag_type) {
      conditions.push(`flag_type = $${paramIndex}`);
      params.push(options.flag_type);
      paramIndex++;
    }

    if (options.tag) {
      conditions.push(`$${paramIndex} = ANY(tags)`);
      params.push(options.tag);
      paramIndex++;
    }

    if (options.enabled !== undefined) {
      conditions.push(`enabled = $${paramIndex}`);
      params.push(options.enabled);
      paramIndex++;
    }

    const limit = options.limit ?? 100;
    const offset = options.offset ?? 0;

    const sql = `
      SELECT * FROM ff_flags
      WHERE ${conditions.join(' AND ')}
      ORDER BY created_at DESC
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `;

    const result = await this.query<FlagRecord>(sql, [...params, limit, offset]);
    return result.rows;
  }

  async updateFlag(key: string, request: UpdateFlagRequest): Promise<FlagRecord | null> {
    const flag = await this.getFlag(key);
    if (!flag) {
      return null;
    }

    const updates: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (request.name !== undefined) {
      updates.push(`name = $${paramIndex}`);
      params.push(request.name);
      paramIndex++;
    }

    if (request.description !== undefined) {
      updates.push(`description = $${paramIndex}`);
      params.push(request.description);
      paramIndex++;
    }

    if (request.flag_type !== undefined) {
      updates.push(`flag_type = $${paramIndex}`);
      params.push(request.flag_type);
      paramIndex++;
    }

    if (request.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex}`);
      params.push(request.enabled);
      paramIndex++;
    }

    if (request.default_value !== undefined) {
      updates.push(`default_value = $${paramIndex}`);
      params.push(JSON.stringify(request.default_value));
      paramIndex++;
    }

    if (request.tags !== undefined) {
      updates.push(`tags = $${paramIndex}`);
      params.push(request.tags);
      paramIndex++;
    }

    if (request.owner !== undefined) {
      updates.push(`owner = $${paramIndex}`);
      params.push(request.owner);
      paramIndex++;
    }

    if (request.stale_after_days !== undefined) {
      updates.push(`stale_after_days = $${paramIndex}`);
      params.push(request.stale_after_days);
      paramIndex++;
    }

    if (updates.length === 1) {
      return flag;
    }

    const result = await this.query<FlagRecord>(
      `UPDATE ff_flags SET ${updates.join(', ')}
       WHERE source_account_id = $${paramIndex} AND key = $${paramIndex + 1}
       RETURNING *`,
      [...params, this.sourceAccountId, key]
    );

    return result.rows[0] ?? null;
  }

  async deleteFlag(key: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM ff_flags WHERE source_account_id = $1 AND key = $2`,
      [this.sourceAccountId, key]
    );

    return count > 0;
  }

  async enableFlag(key: string): Promise<boolean> {
    const count = await this.execute(
      `UPDATE ff_flags SET enabled = true, updated_at = NOW()
       WHERE source_account_id = $1 AND key = $2`,
      [this.sourceAccountId, key]
    );

    return count > 0;
  }

  async disableFlag(key: string): Promise<boolean> {
    const count = await this.execute(
      `UPDATE ff_flags SET enabled = false, updated_at = NOW()
       WHERE source_account_id = $1 AND key = $2`,
      [this.sourceAccountId, key]
    );

    return count > 0;
  }

  async getFlagDetail(key: string): Promise<FlagDetail | null> {
    const flag = await this.getFlag(key);
    if (!flag) {
      return null;
    }

    const rules = await this.getRulesByFlagId(flag.id);

    return {
      ...flag,
      rules,
    };
  }

  async updateFlagEvaluation(key: string): Promise<void> {
    await this.execute(
      `UPDATE ff_flags
       SET last_evaluated_at = NOW(), evaluation_count = evaluation_count + 1
       WHERE source_account_id = $1 AND key = $2`,
      [this.sourceAccountId, key]
    );
  }

  // =========================================================================
  // Rules
  // =========================================================================

  async createRule(request: CreateRuleRequest): Promise<RuleRecord | null> {
    const flag = await this.getFlag(request.flag_key);
    if (!flag) {
      return null;
    }

    const result = await this.query<RuleRecord>(
      `INSERT INTO ff_rules (
        source_account_id, flag_id, name, rule_type, conditions, value, priority, enabled
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        flag.id,
        request.name ?? null,
        request.rule_type,
        JSON.stringify(request.conditions),
        JSON.stringify(request.value ?? true),
        request.priority ?? 0,
        request.enabled ?? true,
      ]
    );

    return result.rows[0];
  }

  async getRulesByFlagId(flagId: string): Promise<RuleRecord[]> {
    const result = await this.query<RuleRecord>(
      `SELECT * FROM ff_rules
       WHERE source_account_id = $1 AND flag_id = $2
       ORDER BY priority DESC, created_at ASC`,
      [this.sourceAccountId, flagId]
    );

    return result.rows;
  }

  async getRule(ruleId: string): Promise<RuleRecord | null> {
    const result = await this.query<RuleRecord>(
      `SELECT * FROM ff_rules WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, ruleId]
    );

    return result.rows[0] ?? null;
  }

  async updateRule(ruleId: string, request: UpdateRuleRequest): Promise<RuleRecord | null> {
    const rule = await this.getRule(ruleId);
    if (!rule) {
      return null;
    }

    const updates: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (request.name !== undefined) {
      updates.push(`name = $${paramIndex}`);
      params.push(request.name);
      paramIndex++;
    }

    if (request.conditions !== undefined) {
      updates.push(`conditions = $${paramIndex}`);
      params.push(JSON.stringify(request.conditions));
      paramIndex++;
    }

    if (request.value !== undefined) {
      updates.push(`value = $${paramIndex}`);
      params.push(JSON.stringify(request.value));
      paramIndex++;
    }

    if (request.priority !== undefined) {
      updates.push(`priority = $${paramIndex}`);
      params.push(request.priority);
      paramIndex++;
    }

    if (request.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex}`);
      params.push(request.enabled);
      paramIndex++;
    }

    if (updates.length === 1) {
      return rule;
    }

    const result = await this.query<RuleRecord>(
      `UPDATE ff_rules SET ${updates.join(', ')}
       WHERE source_account_id = $${paramIndex} AND id = $${paramIndex + 1}
       RETURNING *`,
      [...params, this.sourceAccountId, ruleId]
    );

    return result.rows[0] ?? null;
  }

  async deleteRule(ruleId: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM ff_rules WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, ruleId]
    );

    return count > 0;
  }

  // =========================================================================
  // Segments
  // =========================================================================

  async createSegment(request: CreateSegmentRequest): Promise<SegmentRecord> {
    const result = await this.query<SegmentRecord>(
      `INSERT INTO ff_segments (source_account_id, name, description, match_type, rules)
       VALUES ($1, $2, $3, $4, $5)
       RETURNING *`,
      [
        this.sourceAccountId,
        request.name,
        request.description ?? null,
        request.match_type ?? 'all',
        JSON.stringify(request.rules),
      ]
    );

    return result.rows[0];
  }

  async getSegment(id: string): Promise<SegmentRecord | null> {
    const result = await this.query<SegmentRecord>(
      `SELECT * FROM ff_segments WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id]
    );

    return result.rows[0] ?? null;
  }

  async getSegmentByName(name: string): Promise<SegmentRecord | null> {
    const result = await this.query<SegmentRecord>(
      `SELECT * FROM ff_segments WHERE source_account_id = $1 AND name = $2`,
      [this.sourceAccountId, name]
    );

    return result.rows[0] ?? null;
  }

  async listSegments(limit = 100, offset = 0): Promise<SegmentRecord[]> {
    const result = await this.query<SegmentRecord>(
      `SELECT * FROM ff_segments
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async updateSegment(id: string, request: UpdateSegmentRequest): Promise<SegmentRecord | null> {
    const segment = await this.getSegment(id);
    if (!segment) {
      return null;
    }

    const updates: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (request.name !== undefined) {
      updates.push(`name = $${paramIndex}`);
      params.push(request.name);
      paramIndex++;
    }

    if (request.description !== undefined) {
      updates.push(`description = $${paramIndex}`);
      params.push(request.description);
      paramIndex++;
    }

    if (request.match_type !== undefined) {
      updates.push(`match_type = $${paramIndex}`);
      params.push(request.match_type);
      paramIndex++;
    }

    if (request.rules !== undefined) {
      updates.push(`rules = $${paramIndex}`);
      params.push(JSON.stringify(request.rules));
      paramIndex++;
    }

    if (updates.length === 1) {
      return segment;
    }

    const result = await this.query<SegmentRecord>(
      `UPDATE ff_segments SET ${updates.join(', ')}
       WHERE source_account_id = $${paramIndex} AND id = $${paramIndex + 1}
       RETURNING *`,
      [...params, this.sourceAccountId, id]
    );

    return result.rows[0] ?? null;
  }

  async deleteSegment(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM ff_segments WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id]
    );

    return count > 0;
  }

  // =========================================================================
  // Evaluations
  // =========================================================================

  async recordEvaluation(
    flagKey: string,
    userId: string | undefined,
    context: EvaluationContext | undefined,
    result: EvaluationResult
  ): Promise<void> {
    await this.execute(
      `INSERT INTO ff_evaluations (
        source_account_id, flag_key, user_id, context, result, rule_id, reason
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
      [
        this.sourceAccountId,
        flagKey,
        userId ?? null,
        context ? JSON.stringify(context) : null,
        JSON.stringify(result.value),
        result.rule_id ?? null,
        result.reason,
      ]
    );
  }

  async listEvaluations(options: ListEvaluationsOptions = {}): Promise<EvaluationRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.flag_key) {
      conditions.push(`flag_key = $${paramIndex}`);
      params.push(options.flag_key);
      paramIndex++;
    }

    if (options.user_id) {
      conditions.push(`user_id = $${paramIndex}`);
      params.push(options.user_id);
      paramIndex++;
    }

    if (options.reason) {
      conditions.push(`reason = $${paramIndex}`);
      params.push(options.reason);
      paramIndex++;
    }

    if (options.since) {
      conditions.push(`evaluated_at >= $${paramIndex}`);
      params.push(options.since);
      paramIndex++;
    }

    const limit = options.limit ?? 100;
    const offset = options.offset ?? 0;

    const sql = `
      SELECT * FROM ff_evaluations
      WHERE ${conditions.join(' AND ')}
      ORDER BY evaluated_at DESC
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `;

    const result = await this.query<EvaluationRecord>(sql, [...params, limit, offset]);
    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<FlagStats> {
    const result = await this.query<{
      flags: string;
      rules: string;
      segments: string;
      evaluations: string;
      last_evaluated_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM ff_flags WHERE source_account_id = $1) as flags,
        (SELECT COUNT(*) FROM ff_rules WHERE source_account_id = $1) as rules,
        (SELECT COUNT(*) FROM ff_segments WHERE source_account_id = $1) as segments,
        (SELECT COUNT(*) FROM ff_evaluations WHERE source_account_id = $1) as evaluations,
        (SELECT MAX(evaluated_at) FROM ff_evaluations WHERE source_account_id = $1) as last_evaluated_at
      `,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      flags: parseInt(row.flags, 10),
      rules: parseInt(row.rules, 10),
      segments: parseInt(row.segments, 10),
      evaluations: parseInt(row.evaluations, 10),
      lastEvaluatedAt: row.last_evaluated_at,
    };
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: Omit<WebhookEventRecord, 'source_account_id' | 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO ff_webhook_events (id, source_account_id, event_type, payload, processed, processed_at, error)
       VALUES ($1, $2, $3, $4, $5, $6, $7)`,
      [
        event.id,
        this.sourceAccountId,
        event.event_type,
        JSON.stringify(event.payload),
        event.processed,
        event.processed_at ?? null,
        event.error ?? null,
      ]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE ff_webhook_events
       SET processed = true, processed_at = NOW(), error = $1
       WHERE source_account_id = $2 AND id = $3`,
      [error ?? null, this.sourceAccountId, eventId]
    );
  }
}
