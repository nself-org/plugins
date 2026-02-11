/**
 * Moderation Database Operations
 * Complete CRUD operations for content moderation
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ModerationRuleRecord,
  CreateRuleRequest,
  UpdateRuleRequest,
  WordlistRecord,
  CreateWordlistRequest,
  UpdateWordlistRequest,
  ModerationActionRecord,
  CreateActionRequest,
  RevokeActionRequest,
  ModerationFlagRecord,
  CreateFlagRequest,
  ReviewFlagRequest,
  ModerationAppealRecord,
  CreateAppealRequest,
  ReviewAppealRequest,
  ModerationReportRecord,
  CreateReportRequest,
  ToxicityScoreRecord,
  UserStatsRecord,
  AuditLogRecord,
  CreateAuditLogRequest,
  QueueItem,
  ModerationOverviewStats,
  SeverityLevel,
} from './types.js';

const logger = createLogger('moderation:db');

export class ModerationDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ModerationDatabase {
    return new ModerationDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing moderation schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Moderation Rules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_rules (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        filter_type VARCHAR(50) NOT NULL,
        severity VARCHAR(20) NOT NULL DEFAULT 'medium',
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        conditions JSONB NOT NULL DEFAULT '{}',
        actions JSONB NOT NULL DEFAULT '[]',
        threshold_config JSONB DEFAULT '{}',
        channel_id VARCHAR(255),
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_moderation_rules_account ON moderation_rules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_rules_enabled ON moderation_rules(source_account_id, is_enabled) WHERE is_enabled = true;
      CREATE INDEX IF NOT EXISTS idx_moderation_rules_type ON moderation_rules(source_account_id, filter_type, severity);

      -- =====================================================================
      -- Moderation Wordlists
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_wordlists (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        language VARCHAR(10) NOT NULL DEFAULT 'en',
        category VARCHAR(100),
        words TEXT[] NOT NULL DEFAULT '{}',
        is_regex BOOLEAN NOT NULL DEFAULT false,
        case_sensitive BOOLEAN NOT NULL DEFAULT false,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        severity VARCHAR(20) NOT NULL DEFAULT 'medium',
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
      CREATE INDEX IF NOT EXISTS idx_moderation_wordlists_account ON moderation_wordlists(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_wordlists_enabled ON moderation_wordlists(source_account_id, is_enabled) WHERE is_enabled = true;
      CREATE INDEX IF NOT EXISTS idx_moderation_wordlists_language ON moderation_wordlists(source_account_id, language);

      -- =====================================================================
      -- Moderation Actions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_actions (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        target_user_id VARCHAR(255) NOT NULL,
        target_message_id VARCHAR(255),
        target_channel_id VARCHAR(255),
        action_type VARCHAR(50) NOT NULL,
        severity VARCHAR(20) NOT NULL,
        reason TEXT NOT NULL,
        duration_minutes INTEGER,
        expires_at TIMESTAMP WITH TIME ZONE,
        triggered_by_rule_id UUID REFERENCES moderation_rules(id) ON DELETE SET NULL,
        is_automated BOOLEAN NOT NULL DEFAULT false,
        moderator_id VARCHAR(255),
        moderator_notes TEXT,
        metadata JSONB DEFAULT '{}',
        is_active BOOLEAN NOT NULL DEFAULT true,
        revoked_at TIMESTAMP WITH TIME ZONE,
        revoked_by VARCHAR(255),
        revoke_reason TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_moderation_actions_account ON moderation_actions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_actions_user ON moderation_actions(source_account_id, target_user_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_actions_message ON moderation_actions(target_message_id) WHERE target_message_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_moderation_actions_moderator ON moderation_actions(moderator_id) WHERE moderator_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_moderation_actions_expires ON moderation_actions(expires_at) WHERE expires_at IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_moderation_actions_active ON moderation_actions(source_account_id, is_active, created_at);

      -- =====================================================================
      -- Moderation Flags
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_flags (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        content_type VARCHAR(50) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        content_snapshot JSONB,
        flag_reason VARCHAR(255) NOT NULL,
        flag_category VARCHAR(100),
        severity VARCHAR(20) NOT NULL DEFAULT 'medium',
        flagged_by_user_id VARCHAR(255),
        flagged_by_rule_id UUID REFERENCES moderation_rules(id) ON DELETE SET NULL,
        is_automated BOOLEAN NOT NULL DEFAULT false,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        reviewed_by VARCHAR(255),
        reviewed_at TIMESTAMP WITH TIME ZONE,
        review_notes TEXT,
        action_id UUID REFERENCES moderation_actions(id) ON DELETE SET NULL,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_moderation_flags_account ON moderation_flags(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_flags_content ON moderation_flags(source_account_id, content_type, content_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_flags_status ON moderation_flags(source_account_id, status, created_at);
      CREATE INDEX IF NOT EXISTS idx_moderation_flags_severity ON moderation_flags(source_account_id, severity, status);

      -- =====================================================================
      -- Moderation Appeals
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_appeals (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        action_id UUID NOT NULL REFERENCES moderation_actions(id) ON DELETE CASCADE,
        appellant_user_id VARCHAR(255) NOT NULL,
        appeal_reason TEXT NOT NULL,
        supporting_evidence JSONB DEFAULT '{}',
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        reviewed_by VARCHAR(255),
        reviewed_at TIMESTAMP WITH TIME ZONE,
        review_decision TEXT,
        was_successful BOOLEAN,
        new_action_id UUID REFERENCES moderation_actions(id) ON DELETE SET NULL,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_moderation_appeals_account ON moderation_appeals(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_appeals_action ON moderation_appeals(action_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_appeals_user ON moderation_appeals(source_account_id, appellant_user_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_appeals_status ON moderation_appeals(source_account_id, status, created_at);

      -- =====================================================================
      -- Moderation Reports
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_reports (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        reporter_id VARCHAR(255) NOT NULL,
        content_type VARCHAR(50) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        report_category VARCHAR(100) NOT NULL,
        report_reason TEXT NOT NULL,
        additional_context TEXT,
        status VARCHAR(20) NOT NULL DEFAULT 'pending',
        assigned_to VARCHAR(255),
        flag_id UUID REFERENCES moderation_flags(id) ON DELETE SET NULL,
        action_id UUID REFERENCES moderation_actions(id) ON DELETE SET NULL,
        resolution_notes TEXT,
        resolved_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_moderation_reports_account ON moderation_reports(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_reports_reporter ON moderation_reports(source_account_id, reporter_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_reports_content ON moderation_reports(source_account_id, content_type, content_id);
      CREATE INDEX IF NOT EXISTS idx_moderation_reports_status ON moderation_reports(source_account_id, status, created_at);
      CREATE INDEX IF NOT EXISTS idx_moderation_reports_assigned ON moderation_reports(assigned_to) WHERE assigned_to IS NOT NULL;

      -- =====================================================================
      -- Toxicity Scores
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_toxicity_scores (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        content_type VARCHAR(50) NOT NULL,
        content_id VARCHAR(255) NOT NULL,
        content_hash VARCHAR(64),
        overall_score DECIMAL(5,4) NOT NULL,
        category_scores JSONB DEFAULT '{}',
        provider VARCHAR(50) NOT NULL,
        model_version VARCHAR(50),
        analyzed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        metadata JSONB DEFAULT '{}',
        UNIQUE(source_account_id, content_type, content_id, provider)
      );
      CREATE INDEX IF NOT EXISTS idx_toxicity_account ON moderation_toxicity_scores(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_toxicity_content ON moderation_toxicity_scores(source_account_id, content_type, content_id);
      CREATE INDEX IF NOT EXISTS idx_toxicity_score ON moderation_toxicity_scores(source_account_id, overall_score);
      CREATE INDEX IF NOT EXISTS idx_toxicity_hash ON moderation_toxicity_scores(content_hash) WHERE content_hash IS NOT NULL;

      -- =====================================================================
      -- User Moderation Stats
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_user_stats (
        user_id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        total_warnings INTEGER NOT NULL DEFAULT 0,
        total_mutes INTEGER NOT NULL DEFAULT 0,
        total_bans INTEGER NOT NULL DEFAULT 0,
        total_flags INTEGER NOT NULL DEFAULT 0,
        total_reports_filed INTEGER NOT NULL DEFAULT 0,
        total_reports_against INTEGER NOT NULL DEFAULT 0,
        average_toxicity_score DECIMAL(5,4),
        toxicity_trend DECIMAL(5,4),
        risk_level VARCHAR(20) NOT NULL DEFAULT 'low',
        risk_score DECIMAL(5,2) DEFAULT 0.0,
        is_muted BOOLEAN NOT NULL DEFAULT false,
        muted_until TIMESTAMP WITH TIME ZONE,
        is_banned BOOLEAN NOT NULL DEFAULT false,
        banned_until TIMESTAMP WITH TIME ZONE,
        first_violation_at TIMESTAMP WITH TIME ZONE,
        last_violation_at TIMESTAMP WITH TIME ZONE,
        last_calculated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        PRIMARY KEY (source_account_id, user_id)
      );
      CREATE INDEX IF NOT EXISTS idx_user_stats_account ON moderation_user_stats(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_user_stats_risk ON moderation_user_stats(source_account_id, risk_level, risk_score);
      CREATE INDEX IF NOT EXISTS idx_user_stats_muted ON moderation_user_stats(source_account_id, is_muted, muted_until) WHERE is_muted = true;
      CREATE INDEX IF NOT EXISTS idx_user_stats_banned ON moderation_user_stats(source_account_id, is_banned, banned_until) WHERE is_banned = true;

      -- =====================================================================
      -- Audit Log
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS moderation_audit_log (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(100) NOT NULL,
        event_category VARCHAR(50) NOT NULL,
        actor_id VARCHAR(255),
        actor_type VARCHAR(50) NOT NULL DEFAULT 'user',
        target_type VARCHAR(50),
        target_id VARCHAR(255),
        details JSONB NOT NULL DEFAULT '{}',
        ip_address VARCHAR(45),
        user_agent TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_audit_log_account ON moderation_audit_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_audit_log_event ON moderation_audit_log(source_account_id, event_type, created_at);
      CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON moderation_audit_log(source_account_id, actor_id, created_at);
      CREATE INDEX IF NOT EXISTS idx_audit_log_target ON moderation_audit_log(source_account_id, target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_audit_log_created ON moderation_audit_log(source_account_id, created_at DESC);
    `;

    await this.db.execute(schema);
    logger.info('Moderation schema initialized successfully');
  }

  // =========================================================================
  // Rules CRUD
  // =========================================================================

  async createRule(request: CreateRuleRequest): Promise<ModerationRuleRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_rules (
        source_account_id, name, description, filter_type, severity,
        conditions, actions, threshold_config, channel_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.name,
        request.description ?? null,
        request.filter_type,
        request.severity,
        JSON.stringify(request.conditions),
        JSON.stringify(request.actions),
        JSON.stringify(request.threshold_config ?? {}),
        request.channel_id ?? null,
      ]
    );
    return result.rows[0] as unknown as ModerationRuleRecord;
  }

  async getRule(id: string): Promise<ModerationRuleRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_rules WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as ModerationRuleRecord | null;
  }

  async listRules(enabledOnly = false): Promise<ModerationRuleRecord[]> {
    const sql = enabledOnly
      ? 'SELECT * FROM moderation_rules WHERE source_account_id = $1 AND is_enabled = true ORDER BY severity DESC, created_at DESC'
      : 'SELECT * FROM moderation_rules WHERE source_account_id = $1 ORDER BY severity DESC, created_at DESC';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as ModerationRuleRecord[];
  }

  async updateRule(id: string, updates: UpdateRuleRequest): Promise<ModerationRuleRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.severity !== undefined) { sets.push(`severity = $${paramIndex++}`); params.push(updates.severity); }
    if (updates.is_enabled !== undefined) { sets.push(`is_enabled = $${paramIndex++}`); params.push(updates.is_enabled); }
    if (updates.conditions !== undefined) { sets.push(`conditions = $${paramIndex++}`); params.push(JSON.stringify(updates.conditions)); }
    if (updates.actions !== undefined) { sets.push(`actions = $${paramIndex++}`); params.push(JSON.stringify(updates.actions)); }
    if (updates.threshold_config !== undefined) { sets.push(`threshold_config = $${paramIndex++}`); params.push(JSON.stringify(updates.threshold_config)); }

    if (sets.length === 0) return this.getRule(id);

    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE moderation_rules SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as ModerationRuleRecord | null;
  }

  async deleteRule(id: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM moderation_rules WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return count > 0;
  }

  // =========================================================================
  // Wordlists CRUD
  // =========================================================================

  async createWordlist(request: CreateWordlistRequest): Promise<WordlistRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_wordlists (
        source_account_id, name, description, language, category,
        words, is_regex, case_sensitive, severity
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.name,
        request.description ?? null,
        request.language ?? 'en',
        request.category ?? null,
        request.words,
        request.is_regex ?? false,
        request.case_sensitive ?? false,
        request.severity ?? 'medium',
      ]
    );
    return result.rows[0] as unknown as WordlistRecord;
  }

  async getWordlist(id: string): Promise<WordlistRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_wordlists WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as WordlistRecord | null;
  }

  async listWordlists(enabledOnly = false): Promise<WordlistRecord[]> {
    const sql = enabledOnly
      ? 'SELECT * FROM moderation_wordlists WHERE source_account_id = $1 AND is_enabled = true ORDER BY severity DESC, name'
      : 'SELECT * FROM moderation_wordlists WHERE source_account_id = $1 ORDER BY severity DESC, name';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as WordlistRecord[];
  }

  async updateWordlist(id: string, updates: UpdateWordlistRequest): Promise<WordlistRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.language !== undefined) { sets.push(`language = $${paramIndex++}`); params.push(updates.language); }
    if (updates.category !== undefined) { sets.push(`category = $${paramIndex++}`); params.push(updates.category); }
    if (updates.words !== undefined) { sets.push(`words = $${paramIndex++}`); params.push(updates.words); }
    if (updates.is_regex !== undefined) { sets.push(`is_regex = $${paramIndex++}`); params.push(updates.is_regex); }
    if (updates.case_sensitive !== undefined) { sets.push(`case_sensitive = $${paramIndex++}`); params.push(updates.case_sensitive); }
    if (updates.is_enabled !== undefined) { sets.push(`is_enabled = $${paramIndex++}`); params.push(updates.is_enabled); }
    if (updates.severity !== undefined) { sets.push(`severity = $${paramIndex++}`); params.push(updates.severity); }

    if (sets.length === 0) return this.getWordlist(id);

    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE moderation_wordlists SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as WordlistRecord | null;
  }

  async deleteWordlist(id: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM moderation_wordlists WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return count > 0;
  }

  // =========================================================================
  // Profanity Check
  // =========================================================================

  async checkProfanity(content: string, language?: string): Promise<{ matched_words: string[]; severity: SeverityLevel }> {
    const wordlists = await this.listWordlists(true);
    const filteredLists = language
      ? wordlists.filter(wl => wl.language === language)
      : wordlists;

    const matchedWords: string[] = [];
    let maxSeverity: SeverityLevel = 'low';
    const severityOrder: SeverityLevel[] = ['low', 'medium', 'high', 'critical'];

    for (const wl of filteredLists) {
      for (const word of wl.words) {
        let matches = false;
        if (wl.is_regex) {
          try {
            const regex = new RegExp(word, wl.case_sensitive ? '' : 'i');
            matches = regex.test(content);
          } catch {
            // Skip invalid regex
          }
        } else if (wl.case_sensitive) {
          matches = content.includes(word);
        } else {
          matches = content.toLowerCase().includes(word.toLowerCase());
        }

        if (matches) {
          matchedWords.push(word);
          if (severityOrder.indexOf(wl.severity) > severityOrder.indexOf(maxSeverity)) {
            maxSeverity = wl.severity;
          }
        }
      }
    }

    return { matched_words: [...new Set(matchedWords)], severity: maxSeverity };
  }

  // =========================================================================
  // Actions CRUD
  // =========================================================================

  async createAction(request: CreateActionRequest): Promise<ModerationActionRecord> {
    let expiresAt: Date | null = null;
    if (request.duration_minutes) {
      expiresAt = new Date(Date.now() + request.duration_minutes * 60 * 1000);
    }

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_actions (
        source_account_id, target_user_id, target_message_id, target_channel_id,
        action_type, severity, reason, duration_minutes, expires_at,
        triggered_by_rule_id, is_automated, moderator_id, moderator_notes, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.user_id,
        request.target_message_id ?? null,
        request.target_channel_id ?? null,
        request.action_type,
        request.severity ?? 'medium',
        request.reason,
        request.duration_minutes ?? null,
        expiresAt,
        request.triggered_by_rule_id ?? null,
        request.is_automated ?? false,
        request.moderator_id ?? null,
        request.moderator_notes ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    const action = result.rows[0] as unknown as ModerationActionRecord;

    // Update user stats
    await this.ensureUserStats(request.user_id);
    await this.incrementUserStatCounter(request.user_id, request.action_type);

    if (request.action_type === 'mute') {
      await this.execute(
        `UPDATE moderation_user_stats SET is_muted = true, muted_until = $3, last_violation_at = NOW()
         WHERE source_account_id = $1 AND user_id = $2`,
        [this.sourceAccountId, request.user_id, expiresAt]
      );
    } else if (request.action_type === 'ban') {
      await this.execute(
        `UPDATE moderation_user_stats SET is_banned = true, banned_until = $3, last_violation_at = NOW()
         WHERE source_account_id = $1 AND user_id = $2`,
        [this.sourceAccountId, request.user_id, expiresAt]
      );
    }

    return action;
  }

  async getAction(id: string): Promise<ModerationActionRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_actions WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as ModerationActionRecord | null;
  }

  async listActionsForUser(userId: string, activeOnly = false): Promise<ModerationActionRecord[]> {
    const sql = activeOnly
      ? 'SELECT * FROM moderation_actions WHERE source_account_id = $1 AND target_user_id = $2 AND is_active = true ORDER BY created_at DESC'
      : 'SELECT * FROM moderation_actions WHERE source_account_id = $1 AND target_user_id = $2 ORDER BY created_at DESC';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId, userId]);
    return result.rows as unknown as ModerationActionRecord[];
  }

  async revokeAction(id: string, request: RevokeActionRequest): Promise<boolean> {
    const action = await this.getAction(id);
    if (!action || !action.is_active) return false;

    await this.execute(
      `UPDATE moderation_actions SET is_active = false, revoked_at = NOW(), revoked_by = $3, revoke_reason = $4
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id, request.revoked_by ?? null, request.revoke_reason]
    );

    // Update user stats if mute or ban is revoked
    if (action.action_type === 'mute') {
      await this.execute(
        `UPDATE moderation_user_stats SET is_muted = false, muted_until = NULL
         WHERE source_account_id = $1 AND user_id = $2`,
        [this.sourceAccountId, action.target_user_id]
      );
    } else if (action.action_type === 'ban') {
      await this.execute(
        `UPDATE moderation_user_stats SET is_banned = false, banned_until = NULL
         WHERE source_account_id = $1 AND user_id = $2`,
        [this.sourceAccountId, action.target_user_id]
      );
    }

    return true;
  }

  // =========================================================================
  // Flags CRUD
  // =========================================================================

  async createFlag(request: CreateFlagRequest): Promise<ModerationFlagRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_flags (
        source_account_id, content_type, content_id, content_snapshot,
        flag_reason, flag_category, severity,
        flagged_by_user_id, flagged_by_rule_id, is_automated
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.content_type,
        request.content_id,
        request.content_snapshot ? JSON.stringify(request.content_snapshot) : null,
        request.flag_reason,
        request.flag_category ?? null,
        request.severity ?? 'medium',
        request.flagged_by_user_id ?? null,
        request.flagged_by_rule_id ?? null,
        request.is_automated ?? false,
      ]
    );
    return result.rows[0] as unknown as ModerationFlagRecord;
  }

  async getFlag(id: string): Promise<ModerationFlagRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_flags WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as ModerationFlagRecord | null;
  }

  async listFlags(options: { status?: string; severity?: string; limit?: number; offset?: number } = {}): Promise<{ flags: ModerationFlagRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }
    if (options.severity) { conditions.push(`severity = $${paramIndex++}`); params.push(options.severity); }

    const whereClause = conditions.join(' AND ');
    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const countResult = await this.query<{ total: string }>(
      `SELECT COUNT(*) as total FROM moderation_flags WHERE ${whereClause}`,
      params
    );
    const total = parseInt(countResult.rows[0]?.total ?? '0', 10);

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM moderation_flags WHERE ${whereClause} ORDER BY severity DESC, created_at ASC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`,
      [...params, limit, offset]
    );

    return {
      flags: result.rows as unknown as ModerationFlagRecord[],
      total,
    };
  }

  async reviewFlag(id: string, request: ReviewFlagRequest): Promise<ModerationFlagRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE moderation_flags SET
        status = $3, reviewed_by = $4, reviewed_at = NOW(), review_notes = $5, updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [
        this.sourceAccountId,
        id,
        request.status,
        request.reviewed_by ?? null,
        request.review_notes ?? null,
      ]
    );
    return (result.rows[0] ?? null) as unknown as ModerationFlagRecord | null;
  }

  // =========================================================================
  // Appeals CRUD
  // =========================================================================

  async createAppeal(request: CreateAppealRequest): Promise<ModerationAppealRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_appeals (
        source_account_id, action_id, appellant_user_id, appeal_reason, supporting_evidence
      ) VALUES ($1, $2, $3, $4, $5)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.action_id,
        request.appellant_user_id,
        request.appeal_reason,
        JSON.stringify(request.supporting_evidence ?? {}),
      ]
    );
    return result.rows[0] as unknown as ModerationAppealRecord;
  }

  async getAppeal(id: string): Promise<ModerationAppealRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_appeals WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as ModerationAppealRecord | null;
  }

  async listAppeals(options: { status?: string; limit?: number } = {}): Promise<ModerationAppealRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }

    const limit = options.limit ?? 50;
    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM moderation_appeals WHERE ${conditions.join(' AND ')} ORDER BY created_at DESC LIMIT $${paramIndex}`,
      [...params, limit]
    );
    return result.rows as unknown as ModerationAppealRecord[];
  }

  async reviewAppeal(id: string, request: ReviewAppealRequest): Promise<ModerationAppealRecord | null> {
    const wasSuccessful = request.status === 'approved';

    const result = await this.query<Record<string, unknown>>(
      `UPDATE moderation_appeals SET
        status = $3, reviewed_by = $4, reviewed_at = NOW(),
        review_decision = $5, was_successful = $6, updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [
        this.sourceAccountId,
        id,
        request.status,
        request.reviewed_by ?? null,
        request.review_decision,
        wasSuccessful,
      ]
    );

    const appeal = (result.rows[0] ?? null) as unknown as ModerationAppealRecord | null;

    // If approved, revoke the original action
    if (appeal && wasSuccessful) {
      await this.revokeAction(appeal.action_id, {
        revoke_reason: `Appeal approved: ${request.review_decision}`,
        revoked_by: request.reviewed_by,
      });
    }

    return appeal;
  }

  // =========================================================================
  // Reports CRUD
  // =========================================================================

  async createReport(request: CreateReportRequest): Promise<ModerationReportRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_reports (
        source_account_id, reporter_id, content_type, content_id,
        report_category, report_reason, additional_context
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.reporter_id,
        request.content_type,
        request.content_id,
        request.report_category,
        request.report_reason,
        request.additional_context ?? null,
      ]
    );
    return result.rows[0] as unknown as ModerationReportRecord;
  }

  async getReport(id: string): Promise<ModerationReportRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_reports WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as ModerationReportRecord | null;
  }

  async listReports(options: { status?: string; limit?: number; offset?: number } = {}): Promise<{ reports: ModerationReportRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }

    const whereClause = conditions.join(' AND ');
    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const countResult = await this.query<{ total: string }>(
      `SELECT COUNT(*) as total FROM moderation_reports WHERE ${whereClause}`,
      params
    );
    const total = parseInt(countResult.rows[0]?.total ?? '0', 10);

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM moderation_reports WHERE ${whereClause} ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`,
      [...params, limit, offset]
    );

    return {
      reports: result.rows as unknown as ModerationReportRecord[],
      total,
    };
  }

  // =========================================================================
  // Toxicity Scores
  // =========================================================================

  async upsertToxicityScore(
    contentType: string,
    contentId: string,
    overallScore: number,
    categoryScores: Record<string, number>,
    provider: string,
    modelVersion?: string,
    contentHash?: string
  ): Promise<ToxicityScoreRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_toxicity_scores (
        source_account_id, content_type, content_id, overall_score,
        category_scores, provider, model_version, content_hash
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      ON CONFLICT (source_account_id, content_type, content_id, provider) DO UPDATE SET
        overall_score = EXCLUDED.overall_score,
        category_scores = EXCLUDED.category_scores,
        model_version = EXCLUDED.model_version,
        content_hash = EXCLUDED.content_hash,
        analyzed_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        contentType,
        contentId,
        overallScore,
        JSON.stringify(categoryScores),
        provider,
        modelVersion ?? null,
        contentHash ?? null,
      ]
    );
    return result.rows[0] as unknown as ToxicityScoreRecord;
  }

  async getToxicityScore(contentType: string, contentId: string): Promise<ToxicityScoreRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_toxicity_scores WHERE source_account_id = $1 AND content_type = $2 AND content_id = $3 ORDER BY analyzed_at DESC LIMIT 1',
      [this.sourceAccountId, contentType, contentId]
    );
    return (result.rows[0] ?? null) as unknown as ToxicityScoreRecord | null;
  }

  // =========================================================================
  // User Stats
  // =========================================================================

  async ensureUserStats(userId: string): Promise<void> {
    await this.execute(
      `INSERT INTO moderation_user_stats (source_account_id, user_id)
       VALUES ($1, $2)
       ON CONFLICT (source_account_id, user_id) DO NOTHING`,
      [this.sourceAccountId, userId]
    );
  }

  private async incrementUserStatCounter(userId: string, actionType: string): Promise<void> {
    let column: string | null = null;
    switch (actionType) {
      case 'warn': column = 'total_warnings'; break;
      case 'mute': column = 'total_mutes'; break;
      case 'ban': column = 'total_bans'; break;
      case 'flag': column = 'total_flags'; break;
      default: return;
    }

    if (column) {
      await this.execute(
        `UPDATE moderation_user_stats SET ${column} = ${column} + 1, last_violation_at = NOW(), last_calculated_at = NOW()
         WHERE source_account_id = $1 AND user_id = $2`,
        [this.sourceAccountId, userId]
      );
    }
  }

  async getUserStats(userId: string): Promise<UserStatsRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM moderation_user_stats WHERE source_account_id = $1 AND user_id = $2',
      [this.sourceAccountId, userId]
    );
    return (result.rows[0] ?? null) as unknown as UserStatsRecord | null;
  }

  async calculateUserRiskScore(userId: string): Promise<number> {
    const recentResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM moderation_actions
       WHERE source_account_id = $1 AND target_user_id = $2 AND created_at > NOW() - INTERVAL '30 days'`,
      [this.sourceAccountId, userId]
    );
    const recentViolations = parseInt(recentResult.rows[0]?.count ?? '0', 10);

    const toxResult = await this.query<{ avg: string }>(
      `SELECT AVG(overall_score) as avg FROM moderation_toxicity_scores ts
       INNER JOIN moderation_actions ma ON ma.target_message_id = ts.content_id AND ts.content_type = 'message'
       WHERE ma.source_account_id = $1 AND ma.target_user_id = $2`,
      [this.sourceAccountId, userId]
    );
    const avgToxicity = parseFloat(toxResult.rows[0]?.avg ?? '0');

    const score = Math.min(100, (recentViolations * 5) + (avgToxicity * 50));

    let riskLevel: SeverityLevel = 'low';
    if (score >= 75) riskLevel = 'critical';
    else if (score >= 50) riskLevel = 'high';
    else if (score >= 25) riskLevel = 'medium';

    await this.execute(
      `UPDATE moderation_user_stats SET risk_score = $3, risk_level = $4, last_calculated_at = NOW()
       WHERE source_account_id = $1 AND user_id = $2`,
      [this.sourceAccountId, userId, score, riskLevel]
    );

    return score;
  }

  // =========================================================================
  // Audit Log
  // =========================================================================

  async createAuditLog(request: CreateAuditLogRequest): Promise<AuditLogRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO moderation_audit_log (
        source_account_id, event_type, event_category, actor_id, actor_type,
        target_type, target_id, details, ip_address, user_agent
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.event_type,
        request.event_category,
        request.actor_id ?? null,
        request.actor_type ?? 'user',
        request.target_type ?? null,
        request.target_id ?? null,
        JSON.stringify(request.details ?? {}),
        request.ip_address ?? null,
        request.user_agent ?? null,
      ]
    );
    return result.rows[0] as unknown as AuditLogRecord;
  }

  async listAuditLogs(options: {
    event_type?: string;
    event_category?: string;
    actor_id?: string;
    limit?: number;
    offset?: number;
  } = {}): Promise<{ logs: AuditLogRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.event_type) { conditions.push(`event_type = $${paramIndex++}`); params.push(options.event_type); }
    if (options.event_category) { conditions.push(`event_category = $${paramIndex++}`); params.push(options.event_category); }
    if (options.actor_id) { conditions.push(`actor_id = $${paramIndex++}`); params.push(options.actor_id); }

    const whereClause = conditions.join(' AND ');
    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const countResult = await this.query<{ total: string }>(
      `SELECT COUNT(*) as total FROM moderation_audit_log WHERE ${whereClause}`,
      params
    );
    const total = parseInt(countResult.rows[0]?.total ?? '0', 10);

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM moderation_audit_log WHERE ${whereClause} ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`,
      [...params, limit, offset]
    );

    return {
      logs: result.rows as unknown as AuditLogRecord[],
      total,
    };
  }

  // =========================================================================
  // Queue / Overview Stats
  // =========================================================================

  async getQueue(options: { status?: string; severity?: string; limit?: number; offset?: number } = {}): Promise<{ items: QueueItem[]; total: number }> {
    const result = await this.listFlags({ ...options, status: options.status ?? 'pending' });
    return { items: result.flags, total: result.total };
  }

  async getOverviewStats(timeframeDays = 30): Promise<ModerationOverviewStats> {
    const interval = `${timeframeDays} days`;

    const actionsResult = await this.query<{ total: string; action_type: string }>(
      `SELECT action_type, COUNT(*) as total FROM moderation_actions
       WHERE source_account_id = $1 AND created_at > NOW() - INTERVAL '${interval}'
       GROUP BY action_type`,
      [this.sourceAccountId]
    );

    const actionsByType: Record<string, number> = {};
    let totalActions = 0;
    for (const row of actionsResult.rows) {
      actionsByType[row.action_type] = parseInt(row.total, 10);
      totalActions += parseInt(row.total, 10);
    }

    const flagsResult = await this.query<{ total: string; severity: string }>(
      `SELECT severity, COUNT(*) as total FROM moderation_flags
       WHERE source_account_id = $1 AND created_at > NOW() - INTERVAL '${interval}'
       GROUP BY severity`,
      [this.sourceAccountId]
    );

    const flagsBySeverity: Record<string, number> = {};
    let totalFlags = 0;
    for (const row of flagsResult.rows) {
      flagsBySeverity[row.severity] = parseInt(row.total, 10);
      totalFlags += parseInt(row.total, 10);
    }

    const reportsResult = await this.query<{ total: string }>(
      `SELECT COUNT(*) as total FROM moderation_reports
       WHERE source_account_id = $1 AND created_at > NOW() - INTERVAL '${interval}'`,
      [this.sourceAccountId]
    );
    const totalReports = parseInt(reportsResult.rows[0]?.total ?? '0', 10);

    const toxResult = await this.query<{ avg: string }>(
      `SELECT AVG(overall_score) as avg FROM moderation_toxicity_scores
       WHERE source_account_id = $1 AND analyzed_at > NOW() - INTERVAL '${interval}'`,
      [this.sourceAccountId]
    );
    const avgToxicity = parseFloat(toxResult.rows[0]?.avg ?? '0');

    return {
      total_actions: totalActions,
      total_flags: totalFlags,
      total_reports: totalReports,
      actions_by_type: actionsByType,
      flags_by_severity: flagsBySeverity,
      average_toxicity_score: avgToxicity,
    };
  }

  // =========================================================================
  // Cleanup
  // =========================================================================

  async expireActions(): Promise<number> {
    // Expire mutes
    await this.execute(
      `UPDATE moderation_user_stats SET is_muted = false, muted_until = NULL
       WHERE source_account_id = $1 AND is_muted = true AND muted_until IS NOT NULL AND muted_until < NOW()`,
      [this.sourceAccountId]
    );

    // Expire bans
    await this.execute(
      `UPDATE moderation_user_stats SET is_banned = false, banned_until = NULL
       WHERE source_account_id = $1 AND is_banned = true AND banned_until IS NOT NULL AND banned_until < NOW()`,
      [this.sourceAccountId]
    );

    // Deactivate expired actions
    const count = await this.execute(
      `UPDATE moderation_actions SET is_active = false
       WHERE source_account_id = $1 AND is_active = true AND expires_at IS NOT NULL AND expires_at < NOW()`,
      [this.sourceAccountId]
    );

    return count;
  }
}
