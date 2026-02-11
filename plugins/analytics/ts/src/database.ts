/**
 * Analytics Database Operations
 * Complete CRUD operations for all analytics objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  AnalyticsEventRecord,
  AnalyticsFunnelRecord,
  AnalyticsQuotaRecord,
  AnalyticsQuotaViolationRecord,
  TrackEventRequest,
  CounterValue,
  CounterTimeseriesPoint,
  FunnelStep,
  CreateFunnelRequest,
  UpdateFunnelRequest,
  CreateQuotaRequest,
  UpdateQuotaRequest,
  DashboardStats,
  AnalyticsStats,
  CounterPeriod,
} from './types.js';

const logger = createLogger('analytics:db');

export class AnalyticsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): AnalyticsDatabase {
    return new AnalyticsDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing analytics schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS analytics_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_name VARCHAR(255) NOT NULL,
        event_category VARCHAR(128),
        user_id VARCHAR(255),
        session_id VARCHAR(255),
        properties JSONB DEFAULT '{}',
        context JSONB DEFAULT '{}',
        source_plugin VARCHAR(128),
        timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_analytics_events_source_account ON analytics_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_events_name ON analytics_events(event_name);
      CREATE INDEX IF NOT EXISTS idx_analytics_events_user ON analytics_events(user_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_events_session ON analytics_events(session_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_events_timestamp ON analytics_events(timestamp DESC);
      CREATE INDEX IF NOT EXISTS idx_analytics_events_category ON analytics_events(event_category);

      -- =====================================================================
      -- Counters Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS analytics_counters (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        counter_name VARCHAR(255) NOT NULL,
        dimension VARCHAR(255) DEFAULT 'total',
        period VARCHAR(32) NOT NULL,
        period_start TIMESTAMP WITH TIME ZONE NOT NULL,
        value BIGINT DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, counter_name, dimension, period, period_start)
      );

      CREATE INDEX IF NOT EXISTS idx_analytics_counters_source_account ON analytics_counters(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_counters_name ON analytics_counters(counter_name);
      CREATE INDEX IF NOT EXISTS idx_analytics_counters_period ON analytics_counters(period, period_start);

      -- =====================================================================
      -- Funnels Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS analytics_funnels (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        steps JSONB NOT NULL,
        window_hours INTEGER DEFAULT 24,
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_analytics_funnels_source_account ON analytics_funnels(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_funnels_enabled ON analytics_funnels(enabled);

      -- =====================================================================
      -- Quotas Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS analytics_quotas (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        scope VARCHAR(32) DEFAULT 'app',
        scope_id VARCHAR(255),
        counter_name VARCHAR(255) NOT NULL,
        max_value BIGINT NOT NULL,
        period VARCHAR(32) NOT NULL,
        action_on_exceed VARCHAR(32) DEFAULT 'warn',
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_analytics_quotas_source_account ON analytics_quotas(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_quotas_counter ON analytics_quotas(counter_name);
      CREATE INDEX IF NOT EXISTS idx_analytics_quotas_enabled ON analytics_quotas(enabled);

      -- =====================================================================
      -- Quota Violations Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS analytics_quota_violations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        quota_id UUID REFERENCES analytics_quotas(id) ON DELETE CASCADE,
        scope_id VARCHAR(255),
        current_value BIGINT,
        max_value BIGINT,
        action_taken VARCHAR(32),
        notified BOOLEAN DEFAULT false,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_analytics_quota_violations_source_account ON analytics_quota_violations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_quota_violations_quota ON analytics_quota_violations(quota_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_quota_violations_created ON analytics_quota_violations(created_at DESC);

      -- =====================================================================
      -- Webhook Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS analytics_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_analytics_webhook_events_source_account ON analytics_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_analytics_webhook_events_processed ON analytics_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_analytics_webhook_events_type ON analytics_webhook_events(event_type);
    `;

    await this.execute(schema);
    logger.success('Analytics schema initialized');
  }

  // =========================================================================
  // Events Operations
  // =========================================================================

  async trackEvent(event: TrackEventRequest): Promise<string> {
    const id = crypto.randomUUID();
    const timestamp = event.timestamp ? new Date(event.timestamp) : new Date();

    await this.execute(
      `INSERT INTO analytics_events
       (id, source_account_id, event_name, event_category, user_id, session_id,
        properties, context, source_plugin, timestamp, created_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
      [
        id,
        this.sourceAccountId,
        event.event_name,
        event.event_category ?? null,
        event.user_id ?? null,
        event.session_id ?? null,
        JSON.stringify(event.properties ?? {}),
        JSON.stringify(event.context ?? {}),
        event.source_plugin ?? null,
        timestamp,
      ]
    );

    return id;
  }

  async trackEventBatch(events: TrackEventRequest[]): Promise<number> {
    let count = 0;
    for (const event of events) {
      await this.trackEvent(event);
      count++;
    }
    return count;
  }

  async getEvent(id: string): Promise<AnalyticsEventRecord | null> {
    const result = await this.query<AnalyticsEventRecord>(
      `SELECT * FROM analytics_events
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listEvents(limit = 100, offset = 0, filters?: {
    event_name?: string;
    user_id?: string;
    session_id?: string;
    start_date?: Date;
    end_date?: Date;
  }): Promise<AnalyticsEventRecord[]> {
    let sql = `SELECT * FROM analytics_events WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.event_name) {
      sql += ` AND event_name = $${paramIndex}`;
      params.push(filters.event_name);
      paramIndex++;
    }

    if (filters?.user_id) {
      sql += ` AND user_id = $${paramIndex}`;
      params.push(filters.user_id);
      paramIndex++;
    }

    if (filters?.session_id) {
      sql += ` AND session_id = $${paramIndex}`;
      params.push(filters.session_id);
      paramIndex++;
    }

    if (filters?.start_date) {
      sql += ` AND timestamp >= $${paramIndex}`;
      params.push(filters.start_date);
      paramIndex++;
    }

    if (filters?.end_date) {
      sql += ` AND timestamp <= $${paramIndex}`;
      params.push(filters.end_date);
      paramIndex++;
    }

    sql += ` ORDER BY timestamp DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<AnalyticsEventRecord>(sql, params);
    return result.rows;
  }

  async countEvents(filters?: { event_name?: string; user_id?: string }): Promise<number> {
    let sql = `SELECT COUNT(*) as count FROM analytics_events WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.event_name) {
      sql += ` AND event_name = $${paramIndex}`;
      params.push(filters.event_name);
      paramIndex++;
    }

    if (filters?.user_id) {
      sql += ` AND user_id = $${paramIndex}`;
      params.push(filters.user_id);
      paramIndex++;
    }

    const result = await this.query<{ count: string }>(sql, params);
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Counter Operations
  // =========================================================================

  async incrementCounter(
    counterName: string,
    dimension = 'total',
    increment = 1,
    metadata?: Record<string, unknown>
  ): Promise<void> {
    const now = new Date();
    const periods: Array<{ period: CounterPeriod; periodStart: Date }> = [
      { period: 'hourly', periodStart: new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours()) },
      { period: 'daily', periodStart: new Date(now.getFullYear(), now.getMonth(), now.getDate()) },
      { period: 'monthly', periodStart: new Date(now.getFullYear(), now.getMonth(), 1) },
      { period: 'all_time', periodStart: new Date(0) },
    ];

    for (const { period, periodStart } of periods) {
      await this.execute(
        `INSERT INTO analytics_counters
         (source_account_id, counter_name, dimension, period, period_start, value, metadata, updated_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
         ON CONFLICT (source_account_id, counter_name, dimension, period, period_start)
         DO UPDATE SET
           value = analytics_counters.value + EXCLUDED.value,
           metadata = EXCLUDED.metadata,
           updated_at = NOW()`,
        [
          this.sourceAccountId,
          counterName,
          dimension,
          period,
          periodStart,
          increment,
          JSON.stringify(metadata ?? {}),
        ]
      );
    }
  }

  async getCounterValue(
    counterName: string,
    dimension = 'total',
    period: CounterPeriod = 'all_time'
  ): Promise<CounterValue | null> {
    const result = await this.query<{
      counter_name: string;
      dimension: string;
      period: string;
      period_start: Date;
      value: string;
      updated_at: Date;
    }>(
      `SELECT counter_name, dimension, period, period_start, value, updated_at
       FROM analytics_counters
       WHERE source_account_id = $1 AND counter_name = $2 AND dimension = $3 AND period = $4
       ORDER BY period_start DESC
       LIMIT 1`,
      [this.sourceAccountId, counterName, dimension, period]
    );

    if (!result.rows[0]) {
      return null;
    }

    const row = result.rows[0];
    return {
      counter_name: row.counter_name,
      dimension: row.dimension,
      period: row.period as CounterPeriod,
      period_start: row.period_start,
      value: parseInt(row.value, 10),
      updated_at: row.updated_at,
    };
  }

  async getCounterTimeseries(
    counterName: string,
    dimension = 'total',
    period: CounterPeriod = 'daily',
    startDate?: Date,
    endDate?: Date
  ): Promise<CounterTimeseriesPoint[]> {
    let sql = `SELECT period_start as timestamp, value
               FROM analytics_counters
               WHERE source_account_id = $1 AND counter_name = $2 AND dimension = $3 AND period = $4`;
    const params: unknown[] = [this.sourceAccountId, counterName, dimension, period];
    let paramIndex = 5;

    if (startDate) {
      sql += ` AND period_start >= $${paramIndex}`;
      params.push(startDate);
      paramIndex++;
    }

    if (endDate) {
      sql += ` AND period_start <= $${paramIndex}`;
      params.push(endDate);
      paramIndex++;
    }

    sql += ` ORDER BY period_start ASC`;

    const result = await this.query<{ timestamp: Date; value: string }>(sql, params);
    return result.rows.map(row => ({
      timestamp: row.timestamp,
      value: parseInt(row.value, 10),
    }));
  }

  async listCounters(limit = 100, offset = 0): Promise<CounterValue[]> {
    const result = await this.query<{
      counter_name: string;
      dimension: string;
      period: string;
      period_start: Date;
      value: string;
      updated_at: Date;
    }>(
      `SELECT DISTINCT ON (counter_name, dimension, period)
         counter_name, dimension, period, period_start, value, updated_at
       FROM analytics_counters
       WHERE source_account_id = $1
       ORDER BY counter_name, dimension, period, period_start DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows.map(row => ({
      counter_name: row.counter_name,
      dimension: row.dimension,
      period: row.period as CounterPeriod,
      period_start: row.period_start,
      value: parseInt(row.value, 10),
      updated_at: row.updated_at,
    }));
  }

  async rollupCounters(): Promise<{ hourly: number; daily: number; monthly: number }> {
    // Rollup hourly to daily
    const dailyResult = await this.execute(
      `INSERT INTO analytics_counters (source_account_id, counter_name, dimension, period, period_start, value, metadata, updated_at)
       SELECT source_account_id, counter_name, dimension, 'daily',
              DATE_TRUNC('day', period_start), SUM(value), '{}'::jsonb, NOW()
       FROM analytics_counters
       WHERE period = 'hourly' AND source_account_id = $1
       GROUP BY source_account_id, counter_name, dimension, DATE_TRUNC('day', period_start)
       ON CONFLICT (source_account_id, counter_name, dimension, period, period_start)
       DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
      [this.sourceAccountId]
    );

    // Rollup daily to monthly
    const monthlyResult = await this.execute(
      `INSERT INTO analytics_counters (source_account_id, counter_name, dimension, period, period_start, value, metadata, updated_at)
       SELECT source_account_id, counter_name, dimension, 'monthly',
              DATE_TRUNC('month', period_start), SUM(value), '{}'::jsonb, NOW()
       FROM analytics_counters
       WHERE period = 'daily' AND source_account_id = $1
       GROUP BY source_account_id, counter_name, dimension, DATE_TRUNC('month', period_start)
       ON CONFLICT (source_account_id, counter_name, dimension, period, period_start)
       DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
      [this.sourceAccountId]
    );

    return { hourly: 0, daily: dailyResult, monthly: monthlyResult };
  }

  // =========================================================================
  // Funnel Operations
  // =========================================================================

  async createFunnel(funnel: CreateFunnelRequest): Promise<string> {
    const id = crypto.randomUUID();

    await this.execute(
      `INSERT INTO analytics_funnels
       (id, source_account_id, name, description, steps, window_hours, enabled, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
      [
        id,
        this.sourceAccountId,
        funnel.name,
        funnel.description ?? null,
        JSON.stringify(funnel.steps),
        funnel.window_hours ?? 24,
        funnel.enabled ?? true,
      ]
    );

    return id;
  }

  async getFunnel(id: string): Promise<AnalyticsFunnelRecord | null> {
    const result = await this.query<AnalyticsFunnelRecord>(
      `SELECT * FROM analytics_funnels WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listFunnels(limit = 100, offset = 0): Promise<AnalyticsFunnelRecord[]> {
    const result = await this.query<AnalyticsFunnelRecord>(
      `SELECT * FROM analytics_funnels
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async updateFunnel(id: string, updates: UpdateFunnelRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.name !== undefined) {
      sets.push(`name = $${paramIndex}`);
      params.push(updates.name);
      paramIndex++;
    }

    if (updates.description !== undefined) {
      sets.push(`description = $${paramIndex}`);
      params.push(updates.description);
      paramIndex++;
    }

    if (updates.steps !== undefined) {
      sets.push(`steps = $${paramIndex}`);
      params.push(JSON.stringify(updates.steps));
      paramIndex++;
    }

    if (updates.window_hours !== undefined) {
      sets.push(`window_hours = $${paramIndex}`);
      params.push(updates.window_hours);
      paramIndex++;
    }

    if (updates.enabled !== undefined) {
      sets.push(`enabled = $${paramIndex}`);
      params.push(updates.enabled);
      paramIndex++;
    }

    if (sets.length === 0) {
      return false;
    }

    sets.push(`updated_at = NOW()`);
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE analytics_funnels SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );

    return result > 0;
  }

  async deleteFunnel(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM analytics_funnels WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  async analyzeFunnel(funnelId: string): Promise<{
    steps: Array<{ step_number: number; step_name: string; users: number }>;
  } | null> {
    const funnel = await this.getFunnel(funnelId);
    if (!funnel) {
      return null;
    }

    const steps = funnel.steps as FunnelStep[];
    const windowMs = funnel.window_hours * 60 * 60 * 1000;
    const results: Array<{ step_number: number; step_name: string; users: number }> = [];

    for (let i = 0; i < steps.length; i++) {
      const step = steps[i];

      if (i === 0) {
        // First step: count unique users who completed this step
        const result = await this.query<{ count: string }>(
          `SELECT COUNT(DISTINCT user_id) as count
           FROM analytics_events
           WHERE source_account_id = $1
             AND event_name = $2
             AND user_id IS NOT NULL`,
          [this.sourceAccountId, step.event_name]
        );

        results.push({
          step_number: i + 1,
          step_name: step.name,
          users: parseInt(result.rows[0]?.count ?? '0', 10),
        });
      } else {
        // Subsequent steps: count users who completed previous step AND this step within window
        const prevStep = steps[i - 1];

        const result = await this.query<{ count: string }>(
          `SELECT COUNT(DISTINCT e1.user_id) as count
           FROM analytics_events e1
           INNER JOIN analytics_events e2
             ON e1.user_id = e2.user_id
             AND e2.event_name = $3
             AND e2.timestamp >= e1.timestamp
             AND e2.timestamp <= e1.timestamp + INTERVAL '${windowMs} milliseconds'
           WHERE e1.source_account_id = $1
             AND e1.event_name = $2
             AND e1.user_id IS NOT NULL`,
          [this.sourceAccountId, prevStep.event_name, step.event_name]
        );

        results.push({
          step_number: i + 1,
          step_name: step.name,
          users: parseInt(result.rows[0]?.count ?? '0', 10),
        });
      }
    }

    return { steps: results };
  }

  // =========================================================================
  // Quota Operations
  // =========================================================================

  async createQuota(quota: CreateQuotaRequest): Promise<string> {
    const id = crypto.randomUUID();

    await this.execute(
      `INSERT INTO analytics_quotas
       (id, source_account_id, name, scope, scope_id, counter_name, max_value, period, action_on_exceed, enabled, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())`,
      [
        id,
        this.sourceAccountId,
        quota.name,
        quota.scope ?? 'app',
        quota.scope_id ?? null,
        quota.counter_name,
        quota.max_value,
        quota.period,
        quota.action_on_exceed ?? 'warn',
        quota.enabled ?? true,
      ]
    );

    return id;
  }

  async getQuota(id: string): Promise<AnalyticsQuotaRecord | null> {
    const result = await this.query<AnalyticsQuotaRecord>(
      `SELECT * FROM analytics_quotas WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listQuotas(limit = 100, offset = 0): Promise<AnalyticsQuotaRecord[]> {
    const result = await this.query<AnalyticsQuotaRecord>(
      `SELECT * FROM analytics_quotas
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async updateQuota(id: string, updates: UpdateQuotaRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.name !== undefined) {
      sets.push(`name = $${paramIndex}`);
      params.push(updates.name);
      paramIndex++;
    }

    if (updates.scope !== undefined) {
      sets.push(`scope = $${paramIndex}`);
      params.push(updates.scope);
      paramIndex++;
    }

    if (updates.scope_id !== undefined) {
      sets.push(`scope_id = $${paramIndex}`);
      params.push(updates.scope_id);
      paramIndex++;
    }

    if (updates.counter_name !== undefined) {
      sets.push(`counter_name = $${paramIndex}`);
      params.push(updates.counter_name);
      paramIndex++;
    }

    if (updates.max_value !== undefined) {
      sets.push(`max_value = $${paramIndex}`);
      params.push(updates.max_value);
      paramIndex++;
    }

    if (updates.period !== undefined) {
      sets.push(`period = $${paramIndex}`);
      params.push(updates.period);
      paramIndex++;
    }

    if (updates.action_on_exceed !== undefined) {
      sets.push(`action_on_exceed = $${paramIndex}`);
      params.push(updates.action_on_exceed);
      paramIndex++;
    }

    if (updates.enabled !== undefined) {
      sets.push(`enabled = $${paramIndex}`);
      params.push(updates.enabled);
      paramIndex++;
    }

    if (sets.length === 0) {
      return false;
    }

    sets.push(`updated_at = NOW()`);
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE analytics_quotas SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );

    return result > 0;
  }

  async deleteQuota(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM analytics_quotas WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  async checkQuota(
    counterName: string,
    scopeId: string | null = null,
    increment = 1
  ): Promise<{
    allowed: boolean;
    quota: AnalyticsQuotaRecord | null;
    currentValue: number;
  }> {
    // Find matching quota
    const quotas = await this.query<AnalyticsQuotaRecord>(
      `SELECT * FROM analytics_quotas
       WHERE source_account_id = $1
         AND counter_name = $2
         AND enabled = true
         AND (scope_id IS NULL OR scope_id = $3)
       ORDER BY scope_id NULLS LAST
       LIMIT 1`,
      [this.sourceAccountId, counterName, scopeId]
    );

    if (!quotas.rows[0]) {
      return { allowed: true, quota: null, currentValue: 0 };
    }

    const quota = quotas.rows[0];

    // Get current counter value
    const dimension = quota.scope_id ?? 'total';
    const counter = await this.getCounterValue(counterName, dimension, quota.period as CounterPeriod);
    const currentValue = counter?.value ?? 0;
    const newValue = currentValue + increment;

    if (newValue > Number(quota.max_value)) {
      // Record violation
      await this.execute(
        `INSERT INTO analytics_quota_violations
         (source_account_id, quota_id, scope_id, current_value, max_value, action_taken, notified, created_at)
         VALUES ($1, $2, $3, $4, $5, $6, false, NOW())`,
        [
          this.sourceAccountId,
          quota.id,
          scopeId,
          newValue,
          quota.max_value,
          quota.action_on_exceed,
        ]
      );

      return { allowed: false, quota, currentValue: newValue };
    }

    return { allowed: true, quota, currentValue: newValue };
  }

  async listViolations(limit = 100, offset = 0): Promise<AnalyticsQuotaViolationRecord[]> {
    const result = await this.query<AnalyticsQuotaViolationRecord>(
      `SELECT * FROM analytics_quota_violations
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  // =========================================================================
  // Dashboard & Stats
  // =========================================================================

  async getDashboardStats(): Promise<DashboardStats> {
    // Total events
    const eventsResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_events WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    // Unique users
    const usersResult = await this.query<{ count: string }>(
      `SELECT COUNT(DISTINCT user_id) as count FROM analytics_events
       WHERE source_account_id = $1 AND user_id IS NOT NULL`,
      [this.sourceAccountId]
    );

    // Unique sessions
    const sessionsResult = await this.query<{ count: string }>(
      `SELECT COUNT(DISTINCT session_id) as count FROM analytics_events
       WHERE source_account_id = $1 AND session_id IS NOT NULL`,
      [this.sourceAccountId]
    );

    // Active quotas
    const quotasResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_quotas
       WHERE source_account_id = $1 AND enabled = true`,
      [this.sourceAccountId]
    );

    // Quota violations (last 24h)
    const violationsResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_quota_violations
       WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours'`,
      [this.sourceAccountId]
    );

    // Top events
    const topEventsResult = await this.query<{ event_name: string; count: string }>(
      `SELECT event_name, COUNT(*) as count FROM analytics_events
       WHERE source_account_id = $1
       GROUP BY event_name
       ORDER BY count DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    // Recent events
    const recentEventsResult = await this.query<{
      event_name: string;
      timestamp: Date;
      user_id: string | null;
    }>(
      `SELECT event_name, timestamp, user_id FROM analytics_events
       WHERE source_account_id = $1
       ORDER BY timestamp DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    // Quota status
    const quotaStatusResult = await this.query<{
      quota_name: string;
      current_value: string;
      max_value: string;
    }>(
      `SELECT q.name as quota_name, COALESCE(c.value, 0) as current_value, q.max_value
       FROM analytics_quotas q
       LEFT JOIN analytics_counters c
         ON c.counter_name = q.counter_name
         AND c.period = q.period
         AND c.source_account_id = q.source_account_id
       WHERE q.source_account_id = $1 AND q.enabled = true
       ORDER BY (COALESCE(c.value, 0)::float / q.max_value::float) DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    return {
      total_events: parseInt(eventsResult.rows[0]?.count ?? '0', 10),
      unique_users: parseInt(usersResult.rows[0]?.count ?? '0', 10),
      unique_sessions: parseInt(sessionsResult.rows[0]?.count ?? '0', 10),
      active_quotas: parseInt(quotasResult.rows[0]?.count ?? '0', 10),
      quota_violations: parseInt(violationsResult.rows[0]?.count ?? '0', 10),
      top_events: topEventsResult.rows.map(row => ({
        event_name: row.event_name,
        count: parseInt(row.count, 10),
      })),
      recent_events: recentEventsResult.rows,
      quota_status: quotaStatusResult.rows.map(row => {
        const current = parseInt(row.current_value, 10);
        const max = parseInt(row.max_value, 10);
        return {
          quota_name: row.quota_name,
          current_value: current,
          max_value: max,
          usage_percent: max > 0 ? Math.round((current / max) * 100) : 0,
        };
      }),
    };
  }

  async getStats(): Promise<AnalyticsStats> {
    const eventsResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_events WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const countersResult = await this.query<{ count: string }>(
      `SELECT COUNT(DISTINCT counter_name) as count FROM analytics_counters WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const funnelsResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_funnels WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const quotasResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_quotas WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const violationsResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM analytics_quota_violations WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const lastEventResult = await this.query<{ timestamp: Date }>(
      `SELECT MAX(timestamp) as timestamp FROM analytics_events WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    return {
      events: parseInt(eventsResult.rows[0]?.count ?? '0', 10),
      counters: parseInt(countersResult.rows[0]?.count ?? '0', 10),
      funnels: parseInt(funnelsResult.rows[0]?.count ?? '0', 10),
      quotas: parseInt(quotasResult.rows[0]?.count ?? '0', 10),
      violations: parseInt(violationsResult.rows[0]?.count ?? '0', 10),
      lastEventAt: lastEventResult.rows[0]?.timestamp ?? null,
    };
  }
}
