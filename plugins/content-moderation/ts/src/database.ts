/**
 * Content Moderation Plugin Database
 * Schema initialization and CRUD operations
 */

import { createDatabase, Database, createLogger } from '@nself/plugin-utils';
import {
  ModReviewRecord,
  ModPolicyRecord,
  ModAppealRecord,
  ModUserStrikeRecord,
  ModWebhookEventRecord,
  ModerationStats,
  SubmitReviewRequest,
  CreatePolicyRequest,
  UpdatePolicyRequest,
  AddStrikeRequest,
} from './types.js';

const logger = createLogger('content-moderation:database');

export class ModerationDatabase {
  private db: Database;
  private currentAppId: string = 'primary';

  constructor(db: Database) {
    this.db = db;
  }

  /**
   * Create scoped database instance for a specific source account
   */
  forSourceAccount(appId: string): ModerationDatabase {
    const scoped = new ModerationDatabase(this.db);
    scoped.currentAppId = appId;
    return scoped;
  }

  /**
   * Get current source account ID
   */
  getCurrentAppId(): string {
    return this.currentAppId;
  }

  /**
   * Initialize database schema
   */
  async initSchema(): Promise<void> {
    logger.info('Initializing content-moderation database schema...');

    // Reviews table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS mod_reviews (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        content_type VARCHAR(20) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        content_source VARCHAR(50),
        content_text TEXT,
        content_url TEXT,
        author_id VARCHAR(255),
        status VARCHAR(20) DEFAULT 'pending',
        auto_result JSONB,
        auto_action VARCHAR(20),
        auto_confidence DOUBLE PRECISION,
        manual_result VARCHAR(20),
        manual_action VARCHAR(20),
        reviewer_id VARCHAR(255),
        reviewed_at TIMESTAMPTZ,
        reason TEXT,
        policy_violated VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_reviews_source_app
      ON mod_reviews(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_reviews_status
      ON mod_reviews(source_account_id, status);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_reviews_content
      ON mod_reviews(source_account_id, content_type, content_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_reviews_author
      ON mod_reviews(source_account_id, author_id);
    `);

    // Policies table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS mod_policies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        content_types TEXT[] DEFAULT '{}',
        rules JSONB NOT NULL,
        auto_action VARCHAR(20) DEFAULT 'flag',
        severity VARCHAR(20) DEFAULT 'medium',
        active BOOLEAN DEFAULT true,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_policies_source_app
      ON mod_policies(source_account_id);
    `);

    // Appeals table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS mod_appeals (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        review_id UUID NOT NULL REFERENCES mod_reviews(id) ON DELETE CASCADE,
        appellant_id VARCHAR(255) NOT NULL,
        reason TEXT NOT NULL,
        status VARCHAR(20) DEFAULT 'pending',
        resolved_by VARCHAR(255),
        resolution TEXT,
        resolved_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_appeals_source_app
      ON mod_appeals(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_appeals_review
      ON mod_appeals(review_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_appeals_status
      ON mod_appeals(source_account_id, status);
    `);

    // User strikes table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS mod_user_strikes (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        review_id UUID REFERENCES mod_reviews(id),
        strike_type VARCHAR(50) NOT NULL,
        severity VARCHAR(20) DEFAULT 'warning',
        reason TEXT,
        expires_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_strikes_source_app
      ON mod_user_strikes(source_account_id);
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_strikes_user
      ON mod_user_strikes(source_account_id, user_id);
    `);

    // Webhook events table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS mod_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );
    `);

    await this.db.execute(`
      CREATE INDEX IF NOT EXISTS idx_mod_webhook_events_source_app
      ON mod_webhook_events(source_account_id);
    `);

    logger.success('Content moderation database schema initialized');
  }

  // ============================================================================
  // Reviews
  // ============================================================================

  async createReview(req: SubmitReviewRequest, autoResult: Record<string, unknown> | null, autoAction: string, autoConfidence: number | null, status: string): Promise<ModReviewRecord> {
    const result = await this.db.query<ModReviewRecord>(`
      INSERT INTO mod_reviews (
        source_account_id, content_type, content_id, content_source,
        content_text, content_url, author_id, status,
        auto_result, auto_action, auto_confidence, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *
    `, [
      this.currentAppId,
      req.contentType,
      req.contentId,
      req.contentSource || null,
      req.contentText || null,
      req.contentUrl || null,
      req.authorId || null,
      status,
      autoResult ? JSON.stringify(autoResult) : null,
      autoAction,
      autoConfidence,
      JSON.stringify(req.metadata || {}),
    ]);

    return result.rows[0];
  }

  async getReviewById(id: string): Promise<ModReviewRecord | null> {
    const result = await this.db.query<ModReviewRecord>(`
      SELECT * FROM mod_reviews
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);

    return result.rows[0] || null;
  }

  async getReviews(filters: {
    authorId?: string;
    status?: string;
    contentType?: string;
    from?: string;
    to?: string;
    limit?: number;
    offset?: number;
  }): Promise<{ data: ModReviewRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.currentAppId];
    let paramIndex = 2;

    if (filters.authorId) {
      conditions.push(`author_id = $${paramIndex++}`);
      params.push(filters.authorId);
    }

    if (filters.status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(filters.status);
    }

    if (filters.contentType) {
      conditions.push(`content_type = $${paramIndex++}`);
      params.push(filters.contentType);
    }

    if (filters.from) {
      conditions.push(`created_at >= $${paramIndex++}`);
      params.push(filters.from);
    }

    if (filters.to) {
      conditions.push(`created_at <= $${paramIndex++}`);
      params.push(filters.to);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews WHERE ${whereClause}
    `, params);

    const total = parseInt(String(countResult.rows[0]?.count || 0));
    const limit = filters.limit || 50;
    const offset = filters.offset || 0;

    const result = await this.db.query<ModReviewRecord>(`
      SELECT * FROM mod_reviews
      WHERE ${whereClause}
      ORDER BY created_at DESC
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `, [...params, limit, offset]);

    return { data: result.rows, total };
  }

  async getQueue(status: string = 'pending_manual', contentType?: string, limit: number = 50, offset: number = 0, sortBy: string = 'oldest'): Promise<{ data: ModReviewRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1', 'status = $2'];
    const params: unknown[] = [this.currentAppId, status];
    let paramIndex = 3;

    if (contentType) {
      conditions.push(`content_type = $${paramIndex++}`);
      params.push(contentType);
    }

    const whereClause = conditions.join(' AND ');
    const orderDirection = sortBy === 'oldest' ? 'ASC' : 'DESC';

    const countResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews WHERE ${whereClause}
    `, params);

    const total = parseInt(String(countResult.rows[0]?.count || 0));

    const result = await this.db.query<ModReviewRecord>(`
      SELECT * FROM mod_reviews
      WHERE ${whereClause}
      ORDER BY created_at ${orderDirection}
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `, [...params, limit, offset]);

    return { data: result.rows, total };
  }

  async updateReviewDecision(id: string, manualAction: string, reviewerId: string, reason?: string, policyViolated?: string): Promise<ModReviewRecord> {
    const newStatus = manualAction === 'approve' ? 'approved' : manualAction === 'reject' ? 'rejected' : 'escalated';

    const result = await this.db.query<ModReviewRecord>(`
      UPDATE mod_reviews
      SET manual_action = $3,
          manual_result = $3,
          reviewer_id = $4,
          reason = $5,
          policy_violated = $6,
          status = $7,
          reviewed_at = NOW(),
          updated_at = NOW()
      WHERE source_account_id = $1 AND id = $2
      RETURNING *
    `, [
      this.currentAppId,
      id,
      manualAction,
      reviewerId,
      reason || null,
      policyViolated || null,
      newStatus,
    ]);

    return result.rows[0];
  }

  // ============================================================================
  // Policies
  // ============================================================================

  async createPolicy(req: CreatePolicyRequest): Promise<ModPolicyRecord> {
    const result = await this.db.query<ModPolicyRecord>(`
      INSERT INTO mod_policies (
        source_account_id, name, description, content_types,
        rules, auto_action, severity, active
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *
    `, [
      this.currentAppId,
      req.name,
      req.description || null,
      req.contentTypes || [],
      JSON.stringify(req.rules),
      req.autoAction || 'flag',
      req.severity || 'medium',
      req.active ?? true,
    ]);

    return result.rows[0];
  }

  async getPolicies(): Promise<ModPolicyRecord[]> {
    const result = await this.db.query<ModPolicyRecord>(`
      SELECT * FROM mod_policies
      WHERE source_account_id = $1
      ORDER BY created_at DESC
    `, [this.currentAppId]);

    return result.rows;
  }

  async getPolicyById(id: string): Promise<ModPolicyRecord | null> {
    const result = await this.db.query<ModPolicyRecord>(`
      SELECT * FROM mod_policies
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);

    return result.rows[0] || null;
  }

  async updatePolicy(id: string, updates: UpdatePolicyRequest): Promise<ModPolicyRecord> {
    const setClauses: string[] = [];
    const params: unknown[] = [this.currentAppId, id];
    let paramIndex = 3;

    if (updates.name !== undefined) {
      setClauses.push(`name = $${paramIndex++}`);
      params.push(updates.name);
    }

    if (updates.description !== undefined) {
      setClauses.push(`description = $${paramIndex++}`);
      params.push(updates.description);
    }

    if (updates.contentTypes !== undefined) {
      setClauses.push(`content_types = $${paramIndex++}`);
      params.push(updates.contentTypes);
    }

    if (updates.rules !== undefined) {
      setClauses.push(`rules = $${paramIndex++}`);
      params.push(JSON.stringify(updates.rules));
    }

    if (updates.autoAction !== undefined) {
      setClauses.push(`auto_action = $${paramIndex++}`);
      params.push(updates.autoAction);
    }

    if (updates.severity !== undefined) {
      setClauses.push(`severity = $${paramIndex++}`);
      params.push(updates.severity);
    }

    if (updates.active !== undefined) {
      setClauses.push(`active = $${paramIndex++}`);
      params.push(updates.active);
    }

    setClauses.push('updated_at = NOW()');

    const result = await this.db.query<ModPolicyRecord>(`
      UPDATE mod_policies
      SET ${setClauses.join(', ')}
      WHERE source_account_id = $1 AND id = $2
      RETURNING *
    `, params);

    return result.rows[0];
  }

  async deletePolicy(id: string): Promise<void> {
    await this.db.execute(`
      DELETE FROM mod_policies
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);
  }

  async getActivePolicies(): Promise<ModPolicyRecord[]> {
    const result = await this.db.query<ModPolicyRecord>(`
      SELECT * FROM mod_policies
      WHERE source_account_id = $1 AND active = true
      ORDER BY severity DESC
    `, [this.currentAppId]);

    return result.rows;
  }

  // ============================================================================
  // Appeals
  // ============================================================================

  async createAppeal(reviewId: string, appellantId: string, reason: string): Promise<ModAppealRecord> {
    const result = await this.db.query<ModAppealRecord>(`
      INSERT INTO mod_appeals (source_account_id, review_id, appellant_id, reason)
      VALUES ($1, $2, $3, $4)
      RETURNING *
    `, [this.currentAppId, reviewId, appellantId, reason]);

    return result.rows[0];
  }

  async getAppeals(status?: string, limit: number = 50, offset: number = 0): Promise<{ data: ModAppealRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.currentAppId];
    let paramIndex = 2;

    if (status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(status);
    }

    const whereClause = conditions.join(' AND ');

    const countResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_appeals WHERE ${whereClause}
    `, params);

    const total = parseInt(String(countResult.rows[0]?.count || 0));

    const result = await this.db.query<ModAppealRecord>(`
      SELECT * FROM mod_appeals
      WHERE ${whereClause}
      ORDER BY created_at DESC
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `, [...params, limit, offset]);

    return { data: result.rows, total };
  }

  async getAppealById(id: string): Promise<ModAppealRecord | null> {
    const result = await this.db.query<ModAppealRecord>(`
      SELECT * FROM mod_appeals
      WHERE source_account_id = $1 AND id = $2
    `, [this.currentAppId, id]);

    return result.rows[0] || null;
  }

  async resolveAppeal(id: string, status: string, resolution: string, resolvedBy: string): Promise<ModAppealRecord> {
    const result = await this.db.query<ModAppealRecord>(`
      UPDATE mod_appeals
      SET status = $3, resolution = $4, resolved_by = $5, resolved_at = NOW()
      WHERE source_account_id = $1 AND id = $2
      RETURNING *
    `, [this.currentAppId, id, status, resolution, resolvedBy]);

    return result.rows[0];
  }

  // ============================================================================
  // User Strikes
  // ============================================================================

  async addStrike(userId: string, req: AddStrikeRequest): Promise<ModUserStrikeRecord> {
    const result = await this.db.query<ModUserStrikeRecord>(`
      INSERT INTO mod_user_strikes (
        source_account_id, user_id, review_id, strike_type, severity, reason, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *
    `, [
      this.currentAppId,
      userId,
      req.reviewId || null,
      req.strikeType,
      req.severity || 'warning',
      req.reason || null,
      req.expiresAt || null,
    ]);

    return result.rows[0];
  }

  async getUserStrikes(userId: string): Promise<ModUserStrikeRecord[]> {
    const result = await this.db.query<ModUserStrikeRecord>(`
      SELECT * FROM mod_user_strikes
      WHERE source_account_id = $1 AND user_id = $2
      ORDER BY created_at DESC
    `, [this.currentAppId, userId]);

    return result.rows;
  }

  async getActiveStrikeCount(userId: string): Promise<number> {
    const result = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_user_strikes
      WHERE source_account_id = $1 AND user_id = $2
        AND (expires_at IS NULL OR expires_at > NOW())
    `, [this.currentAppId, userId]);

    return parseInt(String(result.rows[0]?.count || 0));
  }

  async getTotalStrikeCount(userId: string): Promise<number> {
    const result = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_user_strikes
      WHERE source_account_id = $1 AND user_id = $2
    `, [this.currentAppId, userId]);

    return parseInt(String(result.rows[0]?.count || 0));
  }

  // ============================================================================
  // Webhook Events
  // ============================================================================

  async insertWebhookEvent(eventId: string, eventType: string, payload: Record<string, unknown>): Promise<ModWebhookEventRecord> {
    const result = await this.db.query<ModWebhookEventRecord>(`
      INSERT INTO mod_webhook_events (id, source_account_id, event_type, payload)
      VALUES ($1, $2, $3, $4)
      RETURNING *
    `, [eventId, this.currentAppId, eventType, JSON.stringify(payload)]);

    return result.rows[0];
  }

  // ============================================================================
  // Statistics
  // ============================================================================

  async getStats(from?: string, to?: string): Promise<ModerationStats> {
    const hasDateRange = !!(from && to);
    const dateCondition = hasDateRange ? 'AND created_at >= $2 AND created_at <= $3' : '';
    const baseParams: unknown[] = hasDateRange ? [this.currentAppId, from, to] : [this.currentAppId];

    const totalResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews
      WHERE source_account_id = $1 ${dateCondition}
    `, baseParams);

    const autoApprovedResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews
      WHERE source_account_id = $1 AND auto_action = 'approve' ${dateCondition}
    `, baseParams);

    const autoRejectedResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews
      WHERE source_account_id = $1 AND auto_action = 'reject' ${dateCondition}
    `, baseParams);

    const flaggedResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews
      WHERE source_account_id = $1 AND status = 'flagged' ${dateCondition}
    `, baseParams);

    const pendingManualResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews
      WHERE source_account_id = $1 AND status = 'pending_manual' ${dateCondition}
    `, baseParams);

    const manualReviewedResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_reviews
      WHERE source_account_id = $1 AND reviewer_id IS NOT NULL ${dateCondition}
    `, baseParams);

    const appealsResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_appeals
      WHERE source_account_id = $1
    `, [this.currentAppId]);

    const pendingAppealsResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_appeals
      WHERE source_account_id = $1 AND status = 'pending'
    `, [this.currentAppId]);

    const strikesResult = await this.db.query<{ count: number }>(`
      SELECT COUNT(*) as count FROM mod_user_strikes
      WHERE source_account_id = $1
    `, [this.currentAppId]);

    return {
      totalReviews: parseInt(String(totalResult.rows[0]?.count || 0)),
      autoApproved: parseInt(String(autoApprovedResult.rows[0]?.count || 0)),
      autoRejected: parseInt(String(autoRejectedResult.rows[0]?.count || 0)),
      flagged: parseInt(String(flaggedResult.rows[0]?.count || 0)),
      pendingManual: parseInt(String(pendingManualResult.rows[0]?.count || 0)),
      manualReviewed: parseInt(String(manualReviewedResult.rows[0]?.count || 0)),
      appeals: parseInt(String(appealsResult.rows[0]?.count || 0)),
      pendingAppeals: parseInt(String(pendingAppealsResult.rows[0]?.count || 0)),
      totalStrikes: parseInt(String(strikesResult.rows[0]?.count || 0)),
    };
  }
}

/**
 * Create and initialize moderation database
 */
export async function createModerationDatabase(dbConfig: {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl?: boolean;
}): Promise<ModerationDatabase> {
  const db = createDatabase(dbConfig);
  await db.connect();
  return new ModerationDatabase(db);
}
