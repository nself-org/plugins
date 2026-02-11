/**
 * Content Policy Database Operations
 * Complete CRUD operations for all content policy objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  PolicyRecord,
  RuleRecord,
  EvaluationRecord,
  WordListRecord,
  OverrideRecord,
  PolicyStats,
  QueueItem,
  CreatePolicyRequest,
  UpdatePolicyRequest,
  CreateRuleRequest,
  UpdateRuleRequest,
  CreateWordListRequest,
  UpdateWordListRequest,
  CreateOverrideRequest,
  EvaluationResult,
} from './types.js';

const logger = createLogger('content-policy:db');

export class ContentPolicyDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ContentPolicyDatabase {
    return new ContentPolicyDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing content policy schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Policies
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cp_policies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        content_types TEXT[] DEFAULT '{}',
        enabled BOOLEAN DEFAULT true,
        priority INTEGER DEFAULT 0,
        mode VARCHAR(16) DEFAULT 'enforce',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_cp_policies_source_account ON cp_policies(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cp_policies_enabled ON cp_policies(enabled);
      CREATE INDEX IF NOT EXISTS idx_cp_policies_priority ON cp_policies(priority DESC);

      -- =====================================================================
      -- Rules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cp_rules (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        policy_id UUID NOT NULL REFERENCES cp_policies(id) ON DELETE CASCADE,
        name VARCHAR(255) NOT NULL,
        rule_type VARCHAR(32) NOT NULL,
        config JSONB NOT NULL,
        action VARCHAR(16) DEFAULT 'flag',
        severity VARCHAR(16) DEFAULT 'medium',
        message TEXT,
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_cp_rules_source_account ON cp_rules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cp_rules_policy ON cp_rules(policy_id);
      CREATE INDEX IF NOT EXISTS idx_cp_rules_type ON cp_rules(rule_type);
      CREATE INDEX IF NOT EXISTS idx_cp_rules_enabled ON cp_rules(enabled);

      -- =====================================================================
      -- Evaluations
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cp_evaluations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        content_type VARCHAR(64) NOT NULL,
        content_id VARCHAR(255),
        content_text TEXT,
        submitter_id VARCHAR(255),
        policy_id UUID REFERENCES cp_policies(id),
        rule_id UUID REFERENCES cp_rules(id),
        result VARCHAR(16) NOT NULL,
        matched_rules JSONB DEFAULT '[]',
        score DOUBLE PRECISION,
        processing_time_ms INTEGER,
        override_id UUID,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_cp_evaluations_source_account ON cp_evaluations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cp_evaluations_content_type ON cp_evaluations(content_type);
      CREATE INDEX IF NOT EXISTS idx_cp_evaluations_content_id ON cp_evaluations(content_id);
      CREATE INDEX IF NOT EXISTS idx_cp_evaluations_result ON cp_evaluations(result);
      CREATE INDEX IF NOT EXISTS idx_cp_evaluations_submitter ON cp_evaluations(submitter_id);
      CREATE INDEX IF NOT EXISTS idx_cp_evaluations_created ON cp_evaluations(created_at DESC);

      -- =====================================================================
      -- Word Lists
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cp_word_lists (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        list_type VARCHAR(16) NOT NULL DEFAULT 'blocklist',
        words TEXT[] NOT NULL DEFAULT '{}',
        case_sensitive BOOLEAN DEFAULT false,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_cp_word_lists_source_account ON cp_word_lists(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cp_word_lists_type ON cp_word_lists(list_type);

      -- =====================================================================
      -- Overrides
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cp_overrides (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        evaluation_id UUID REFERENCES cp_evaluations(id),
        content_type VARCHAR(64),
        content_id VARCHAR(255),
        original_result VARCHAR(16),
        override_result VARCHAR(16) NOT NULL,
        moderator_id VARCHAR(255) NOT NULL,
        reason TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_cp_overrides_source_account ON cp_overrides(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cp_overrides_evaluation ON cp_overrides(evaluation_id);
      CREATE INDEX IF NOT EXISTS idx_cp_overrides_moderator ON cp_overrides(moderator_id);
      CREATE INDEX IF NOT EXISTS idx_cp_overrides_created ON cp_overrides(created_at DESC);
    `;

    await this.db.execute(schema);
    logger.success('Content policy schema initialized');
  }

  // =========================================================================
  // Policies
  // =========================================================================

  async createPolicy(data: CreatePolicyRequest): Promise<PolicyRecord> {
    const result = await this.db.query<PolicyRecord>(
      `INSERT INTO cp_policies (
        source_account_id, name, description, content_types, enabled, priority, mode
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.name,
        data.description ?? null,
        data.content_types ?? [],
        data.enabled ?? true,
        data.priority ?? 0,
        data.mode ?? 'enforce',
      ]
    );
    return result.rows[0];
  }

  async getPolicy(id: string): Promise<PolicyRecord | null> {
    const result = await this.db.query<PolicyRecord>(
      'SELECT * FROM cp_policies WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listPolicies(limit = 100, offset = 0): Promise<PolicyRecord[]> {
    const result = await this.db.query<PolicyRecord>(
      `SELECT * FROM cp_policies
       WHERE source_account_id = $1
       ORDER BY priority DESC, created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async updatePolicy(id: string, data: UpdatePolicyRequest): Promise<PolicyRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      params.push(data.name);
    }
    if (data.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      params.push(data.description);
    }
    if (data.content_types !== undefined) {
      updates.push(`content_types = $${paramIndex++}`);
      params.push(data.content_types);
    }
    if (data.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex++}`);
      params.push(data.enabled);
    }
    if (data.priority !== undefined) {
      updates.push(`priority = $${paramIndex++}`);
      params.push(data.priority);
    }
    if (data.mode !== undefined) {
      updates.push(`mode = $${paramIndex++}`);
      params.push(data.mode);
    }

    if (updates.length === 0) {
      return this.getPolicy(id);
    }

    updates.push('updated_at = NOW()');

    const result = await this.db.query<PolicyRecord>(
      `UPDATE cp_policies SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deletePolicy(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM cp_policies WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async getPoliciesForContentType(contentType: string): Promise<PolicyRecord[]> {
    const result = await this.db.query<PolicyRecord>(
      `SELECT * FROM cp_policies
       WHERE source_account_id = $1
         AND enabled = true
         AND (content_types = '{}' OR $2 = ANY(content_types))
       ORDER BY priority DESC, created_at`,
      [this.sourceAccountId, contentType]
    );
    return result.rows;
  }

  // =========================================================================
  // Rules
  // =========================================================================

  async createRule(policyId: string, data: CreateRuleRequest): Promise<RuleRecord> {
    const result = await this.db.query<RuleRecord>(
      `INSERT INTO cp_rules (
        source_account_id, policy_id, name, rule_type, config, action, severity, message, enabled
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId,
        policyId,
        data.name,
        data.rule_type,
        JSON.stringify(data.config),
        data.action ?? 'flag',
        data.severity ?? 'medium',
        data.message ?? null,
        data.enabled ?? true,
      ]
    );
    return result.rows[0];
  }

  async getRule(id: string): Promise<RuleRecord | null> {
    const result = await this.db.query<RuleRecord>(
      'SELECT * FROM cp_rules WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listRules(policyId: string): Promise<RuleRecord[]> {
    const result = await this.db.query<RuleRecord>(
      `SELECT * FROM cp_rules
       WHERE policy_id = $1 AND source_account_id = $2
       ORDER BY created_at`,
      [policyId, this.sourceAccountId]
    );
    return result.rows;
  }

  async updateRule(id: string, data: UpdateRuleRequest): Promise<RuleRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      params.push(data.name);
    }
    if (data.config !== undefined) {
      updates.push(`config = $${paramIndex++}`);
      params.push(JSON.stringify(data.config));
    }
    if (data.action !== undefined) {
      updates.push(`action = $${paramIndex++}`);
      params.push(data.action);
    }
    if (data.severity !== undefined) {
      updates.push(`severity = $${paramIndex++}`);
      params.push(data.severity);
    }
    if (data.message !== undefined) {
      updates.push(`message = $${paramIndex++}`);
      params.push(data.message);
    }
    if (data.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex++}`);
      params.push(data.enabled);
    }

    if (updates.length === 0) {
      return this.getRule(id);
    }

    updates.push('updated_at = NOW()');

    const result = await this.db.query<RuleRecord>(
      `UPDATE cp_rules SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteRule(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM cp_rules WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Evaluations
  // =========================================================================

  async createEvaluation(data: Partial<EvaluationRecord>): Promise<EvaluationRecord> {
    const result = await this.db.query<EvaluationRecord>(
      `INSERT INTO cp_evaluations (
        source_account_id, content_type, content_id, content_text, submitter_id,
        policy_id, rule_id, result, matched_rules, score, processing_time_ms
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.content_type,
        data.content_id ?? null,
        data.content_text ?? null,
        data.submitter_id ?? null,
        data.policy_id ?? null,
        data.rule_id ?? null,
        data.result,
        JSON.stringify(data.matched_rules ?? []),
        data.score ?? 0,
        data.processing_time_ms ?? 0,
      ]
    );
    return result.rows[0];
  }

  async getEvaluation(id: string): Promise<EvaluationRecord | null> {
    const result = await this.db.query<EvaluationRecord>(
      'SELECT * FROM cp_evaluations WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listEvaluations(
    limit = 100,
    offset = 0,
    filters?: {
      result?: EvaluationResult;
      content_type?: string;
      submitter_id?: string;
      since?: Date;
    }
  ): Promise<EvaluationRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.result) {
      conditions.push(`result = $${paramIndex++}`);
      params.push(filters.result);
    }
    if (filters?.content_type) {
      conditions.push(`content_type = $${paramIndex++}`);
      params.push(filters.content_type);
    }
    if (filters?.submitter_id) {
      conditions.push(`submitter_id = $${paramIndex++}`);
      params.push(filters.submitter_id);
    }
    if (filters?.since) {
      conditions.push(`created_at >= $${paramIndex++}`);
      params.push(filters.since);
    }

    params.push(limit, offset);

    const result = await this.db.query<EvaluationRecord>(
      `SELECT * FROM cp_evaluations
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async countEvaluations(filters?: { result?: EvaluationResult; content_type?: string }): Promise<number> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.result) {
      conditions.push(`result = $${paramIndex++}`);
      params.push(filters.result);
    }
    if (filters?.content_type) {
      conditions.push(`content_type = $${paramIndex++}`);
      params.push(filters.content_type);
    }

    const result = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM cp_evaluations WHERE ${conditions.join(' AND ')}`,
      params
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Word Lists
  // =========================================================================

  async createWordList(data: CreateWordListRequest): Promise<WordListRecord> {
    const result = await this.db.query<WordListRecord>(
      `INSERT INTO cp_word_lists (
        source_account_id, name, list_type, words, case_sensitive
      ) VALUES ($1, $2, $3, $4, $5)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.name,
        data.list_type,
        data.words,
        data.case_sensitive ?? false,
      ]
    );
    return result.rows[0];
  }

  async getWordList(id: string): Promise<WordListRecord | null> {
    const result = await this.db.query<WordListRecord>(
      'SELECT * FROM cp_word_lists WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listWordLists(limit = 100, offset = 0): Promise<WordListRecord[]> {
    const result = await this.db.query<WordListRecord>(
      `SELECT * FROM cp_word_lists
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async updateWordList(id: string, data: UpdateWordListRequest): Promise<WordListRecord | null> {
    const current = await this.getWordList(id);
    if (!current) return null;

    let words = data.words ?? current.words;

    if (data.add_words && data.add_words.length > 0) {
      const currentSet = new Set(words);
      data.add_words.forEach(word => currentSet.add(word));
      words = Array.from(currentSet);
    }

    if (data.remove_words && data.remove_words.length > 0) {
      const removeSet = new Set(data.remove_words);
      words = words.filter(word => !removeSet.has(word));
    }

    const result = await this.db.query<WordListRecord>(
      `UPDATE cp_word_lists
       SET name = $3, words = $4, case_sensitive = $5, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      [id, this.sourceAccountId, data.name ?? current.name, words, data.case_sensitive ?? current.case_sensitive]
    );
    return result.rows[0] ?? null;
  }

  async deleteWordList(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM cp_word_lists WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async getAllWordLists(): Promise<WordListRecord[]> {
    const result = await this.db.query<WordListRecord>(
      'SELECT * FROM cp_word_lists WHERE source_account_id = $1',
      [this.sourceAccountId]
    );
    return result.rows;
  }

  // =========================================================================
  // Overrides
  // =========================================================================

  async createOverride(data: CreateOverrideRequest): Promise<OverrideRecord> {
    const evaluation = await this.getEvaluation(data.evaluation_id);
    if (!evaluation) {
      throw new Error('Evaluation not found');
    }

    const result = await this.db.query<OverrideRecord>(
      `INSERT INTO cp_overrides (
        source_account_id, evaluation_id, content_type, content_id,
        original_result, override_result, moderator_id, reason
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.evaluation_id,
        evaluation.content_type,
        evaluation.content_id,
        evaluation.result,
        data.override_result,
        data.moderator_id,
        data.reason ?? null,
      ]
    );

    // Update evaluation with override
    await this.db.execute(
      'UPDATE cp_evaluations SET override_id = $1 WHERE id = $2',
      [result.rows[0].id, data.evaluation_id]
    );

    return result.rows[0];
  }

  async listOverrides(limit = 100, offset = 0): Promise<OverrideRecord[]> {
    const result = await this.db.query<OverrideRecord>(
      `SELECT * FROM cp_overrides
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  // =========================================================================
  // Queue & Statistics
  // =========================================================================

  async getQueue(result?: 'flagged' | 'quarantined', limit = 100): Promise<QueueItem[]> {
    const conditions = ['source_account_id = $1', 'override_id IS NULL'];
    const params: unknown[] = [this.sourceAccountId];

    if (result) {
      conditions.push(`result = $${params.length + 1}`);
      params.push(result);
    } else {
      conditions.push("result IN ('flagged', 'quarantined')");
    }

    params.push(limit);

    const rows = await this.db.query<EvaluationRecord>(
      `SELECT * FROM cp_evaluations
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length}`,
      params
    );

    return rows.rows.map(row => ({
      evaluation_id: row.id,
      content_type: row.content_type,
      content_id: row.content_id,
      content_text: row.content_text,
      submitter_id: row.submitter_id,
      result: row.result,
      matched_rules: row.matched_rules,
      score: row.score,
      created_at: row.created_at,
    }));
  }

  async getStats(since?: Date): Promise<PolicyStats> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (since) {
      conditions.push('created_at >= $2');
      params.push(since);
    }

    const whereClause = conditions.join(' AND ');

    // Total evaluations
    const totalResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM cp_evaluations WHERE ${whereClause}`,
      params
    );

    // By result
    const byResult = await this.db.query<{ result: EvaluationResult; count: string }>(
      `SELECT result, COUNT(*) as count FROM cp_evaluations WHERE ${whereClause} GROUP BY result`,
      params
    );

    // Override count
    const overrideResult = await this.db.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM cp_overrides WHERE source_account_id = $1${since ? ' AND created_at >= $2' : ''}`,
      params
    );

    // Average processing time
    const avgTimeResult = await this.db.query<{ avg: string }>(
      `SELECT AVG(processing_time_ms) as avg FROM cp_evaluations WHERE ${whereClause}`,
      params
    );

    // Top violations
    const topViolations = await this.db.query<{ rule_name: string; count: string }>(
      `SELECT
        (matched_rules->0->>'rule_name') as rule_name,
        COUNT(*) as count
       FROM cp_evaluations
       WHERE ${whereClause} AND jsonb_array_length(matched_rules) > 0
       GROUP BY rule_name
       ORDER BY count DESC
       LIMIT 10`,
      params
    );

    // By content type
    const byContentType = await this.db.query<{ content_type: string; count: string }>(
      `SELECT content_type, COUNT(*) as count
       FROM cp_evaluations
       WHERE ${whereClause}
       GROUP BY content_type
       ORDER BY count DESC`,
      params
    );

    const resultMap = new Map(byResult.rows.map(r => [r.result, parseInt(r.count, 10)]));

    return {
      total_evaluations: parseInt(totalResult.rows[0]?.count ?? '0', 10),
      allowed: resultMap.get('allowed') ?? 0,
      denied: resultMap.get('denied') ?? 0,
      flagged: resultMap.get('flagged') ?? 0,
      quarantined: resultMap.get('quarantined') ?? 0,
      override_count: parseInt(overrideResult.rows[0]?.count ?? '0', 10),
      avg_processing_time_ms: parseFloat(avgTimeResult.rows[0]?.avg ?? '0'),
      top_violations: topViolations.rows
        .filter(v => v.rule_name)
        .map(v => ({
          rule_name: v.rule_name,
          count: parseInt(v.count, 10),
        })),
      evaluations_by_content_type: byContentType.rows.map(ct => ({
        content_type: ct.content_type,
        count: parseInt(ct.count, 10),
      })),
      evaluations_by_result: byResult.rows.map(r => ({
        result: r.result,
        count: parseInt(r.count, 10),
      })),
    };
  }
}
