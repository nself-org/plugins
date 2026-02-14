/**
 * Stream Gateway Database Operations
 * Complete CRUD operations for stream admission, sessions, rules, and analytics
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  StreamSessionRecord,
  StreamRecord,
  AdmissionRuleRecord,
  ViewerAnalyticsRecord,
  FamilyMemberRecord,
  SessionStatus,
  StreamStatus,
  AdmitRequest,
  CreateStreamRequest,
  UpdateStreamRequest,
  CreateRuleRequest,
  UpdateRuleRequest,
  GatewayStats,
  AnalyticsSummary,
} from './types.js';

const logger = createLogger('stream-gateway:db');

export class StreamGatewayDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = sourceAccountId;
  }

  forSourceAccount(sourceAccountId: string): StreamGatewayDatabase {
    return new StreamGatewayDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing stream gateway schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Stream Sessions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sg_stream_sessions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        stream_id VARCHAR(255) NOT NULL,
        stream_type VARCHAR(32) NOT NULL DEFAULT 'live',
        user_id VARCHAR(255) NOT NULL,
        device_id VARCHAR(255),
        device_type VARCHAR(32),
        status VARCHAR(32) NOT NULL DEFAULT 'active',
        quality VARCHAR(16) DEFAULT 'auto',
        started_at TIMESTAMPTZ DEFAULT NOW(),
        last_heartbeat_at TIMESTAMPTZ DEFAULT NOW(),
        ended_at TIMESTAMPTZ,
        duration_seconds INTEGER,
        bytes_transferred BIGINT DEFAULT 0,
        denial_reason TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_sg_sessions_source_account
        ON sg_stream_sessions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sg_sessions_app
        ON sg_stream_sessions(app_id);
      CREATE INDEX IF NOT EXISTS idx_sg_sessions_stream
        ON sg_stream_sessions(stream_id);
      CREATE INDEX IF NOT EXISTS idx_sg_sessions_user
        ON sg_stream_sessions(user_id);
      CREATE INDEX IF NOT EXISTS idx_sg_sessions_status
        ON sg_stream_sessions(status) WHERE status = 'active';
      CREATE INDEX IF NOT EXISTS idx_sg_sessions_heartbeat
        ON sg_stream_sessions(last_heartbeat_at) WHERE status = 'active';

      -- =====================================================================
      -- Streams
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sg_streams (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        stream_id VARCHAR(255) NOT NULL,
        title VARCHAR(512),
        stream_type VARCHAR(32) NOT NULL DEFAULT 'live',
        status VARCHAR(32) NOT NULL DEFAULT 'inactive',
        source_device_id VARCHAR(255),
        ingest_url TEXT,
        playback_url TEXT,
        thumbnail_url TEXT,
        max_viewers INTEGER,
        current_viewers INTEGER DEFAULT 0,
        total_viewers INTEGER DEFAULT 0,
        peak_viewers INTEGER DEFAULT 0,
        started_at TIMESTAMPTZ,
        ended_at TIMESTAMPTZ,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, app_id, stream_id)
      );

      CREATE INDEX IF NOT EXISTS idx_sg_streams_source_account
        ON sg_streams(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sg_streams_app
        ON sg_streams(app_id);
      CREATE INDEX IF NOT EXISTS idx_sg_streams_status
        ON sg_streams(status);

      -- =====================================================================
      -- Admission Rules
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sg_admission_rules (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        name VARCHAR(255) NOT NULL,
        rule_type VARCHAR(32) NOT NULL,
        conditions JSONB NOT NULL DEFAULT '{}',
        action VARCHAR(16) NOT NULL DEFAULT 'deny',
        priority INTEGER DEFAULT 0,
        active BOOLEAN DEFAULT TRUE,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_sg_rules_source_account
        ON sg_admission_rules(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sg_rules_app
        ON sg_admission_rules(app_id);
      CREATE INDEX IF NOT EXISTS idx_sg_rules_active
        ON sg_admission_rules(active) WHERE active = TRUE;

      -- =====================================================================
      -- Viewer Analytics
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS sg_viewer_analytics (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        app_id VARCHAR(64) NOT NULL DEFAULT 'default',
        stream_id VARCHAR(255) NOT NULL,
        period_start TIMESTAMPTZ NOT NULL,
        period_end TIMESTAMPTZ NOT NULL,
        avg_viewers FLOAT,
        peak_viewers INTEGER,
        unique_viewers INTEGER,
        total_view_minutes FLOAT,
        quality_distribution JSONB DEFAULT '{}',
        device_distribution JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, app_id, stream_id, period_start)
      );

      CREATE INDEX IF NOT EXISTS idx_sg_analytics_source_account
        ON sg_viewer_analytics(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sg_analytics_stream
        ON sg_viewer_analytics(stream_id);
      CREATE INDEX IF NOT EXISTS idx_sg_analytics_period
        ON sg_viewer_analytics(period_start DESC);

      -- =====================================================================
      -- Family Members (nTV v1 API)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_streamgw_family_members (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        family_id VARCHAR(255) NOT NULL,
        user_id VARCHAR(255) NOT NULL,
        role VARCHAR(32) NOT NULL DEFAULT 'member',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_streamgw_family_source_account
        ON np_streamgw_family_members(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_streamgw_family_family_id
        ON np_streamgw_family_members(family_id);
      CREATE INDEX IF NOT EXISTS idx_np_streamgw_family_user_id
        ON np_streamgw_family_members(user_id);

      -- =====================================================================
      -- Analytics Views
      -- =====================================================================

      CREATE OR REPLACE VIEW sg_concurrent_viewers_over_time AS
      SELECT ss.source_account_id, ss.app_id, ss.stream_id,
             DATE_TRUNC('hour', ss.started_at) AS hour,
             COUNT(*) AS total_sessions,
             COUNT(*) FILTER (WHERE ss.status = 'active') AS concurrent_at_snapshot,
             MAX(va.peak_viewers) AS peak_viewers
      FROM sg_stream_sessions ss
      LEFT JOIN sg_viewer_analytics va
        ON ss.stream_id = va.stream_id
        AND ss.app_id = va.app_id
        AND ss.source_account_id = va.source_account_id
      GROUP BY ss.source_account_id, ss.app_id, ss.stream_id, DATE_TRUNC('hour', ss.started_at)
      ORDER BY hour DESC;

      CREATE OR REPLACE VIEW sg_denial_rates AS
      SELECT source_account_id, app_id,
             denial_reason,
             COUNT(*) AS denial_count,
             ROUND(100.0 * COUNT(*) / NULLIF(SUM(COUNT(*)) OVER (PARTITION BY source_account_id, app_id), 0), 2) AS pct_of_denials
      FROM sg_stream_sessions
      WHERE status = 'denied' AND denial_reason IS NOT NULL
      GROUP BY source_account_id, app_id, denial_reason
      ORDER BY denial_count DESC;

      CREATE OR REPLACE VIEW sg_stream_duration_distribution AS
      SELECT source_account_id, app_id,
             CASE
               WHEN duration_seconds < 300 THEN '<5min'
               WHEN duration_seconds < 1800 THEN '5-30min'
               WHEN duration_seconds < 3600 THEN '30-60min'
               WHEN duration_seconds < 7200 THEN '1-2hr'
               ELSE '2hr+'
             END AS duration_bucket,
             COUNT(*) AS session_count,
             AVG(duration_seconds) / 60.0 AS avg_minutes
      FROM sg_stream_sessions
      WHERE status IN ('ended', 'evicted') AND duration_seconds IS NOT NULL
      GROUP BY source_account_id, app_id, duration_bucket
      ORDER BY source_account_id, app_id, MIN(duration_seconds);

      CREATE OR REPLACE VIEW sg_device_type_breakdown AS
      SELECT source_account_id, app_id, device_type,
             COUNT(*) AS active_sessions,
             COUNT(DISTINCT user_id) AS unique_users,
             AVG(EXTRACT(EPOCH FROM (NOW() - started_at))) / 60.0 AS avg_session_minutes
      FROM sg_stream_sessions
      WHERE status = 'active'
      GROUP BY source_account_id, app_id, device_type
      ORDER BY active_sessions DESC;
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Stream Sessions
  // =========================================================================

  async createSession(
    appId: string,
    request: AdmitRequest,
    status: SessionStatus,
    denialReason?: string
  ): Promise<StreamSessionRecord> {
    const result = await this.query<StreamSessionRecord>(
      `INSERT INTO sg_stream_sessions (
        source_account_id, app_id, stream_id, stream_type, user_id,
        device_id, device_type, status, quality, denial_reason, metadata
      ) VALUES ($1, $2, $3, 'live', $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        request.stream_id,
        request.user_id,
        request.device_id ?? null,
        request.device_type ?? null,
        status,
        request.quality ?? 'auto',
        denialReason ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getSession(sessionId: string): Promise<StreamSessionRecord | null> {
    const result = await this.query<StreamSessionRecord>(
      `SELECT * FROM sg_stream_sessions
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, sessionId]
    );
    return result.rows[0] ?? null;
  }

  async getActiveSessions(appId?: string, limit = 100, offset = 0): Promise<StreamSessionRecord[]> {
    if (appId) {
      const result = await this.query<StreamSessionRecord>(
        `SELECT * FROM sg_stream_sessions
         WHERE source_account_id = $1 AND app_id = $2 AND status = 'active'
         ORDER BY started_at DESC
         LIMIT $3 OFFSET $4`,
        [this.sourceAccountId, appId, limit, offset]
      );
      return result.rows;
    }

    const result = await this.query<StreamSessionRecord>(
      `SELECT * FROM sg_stream_sessions
       WHERE source_account_id = $1 AND status = 'active'
       ORDER BY started_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async listSessions(appId?: string, status?: SessionStatus, limit = 100, offset = 0): Promise<StreamSessionRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (appId) {
      conditions.push(`app_id = $${params.length + 1}`);
      params.push(appId);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.query<StreamSessionRecord>(
      `SELECT * FROM sg_stream_sessions
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async getUserActiveSessions(userId: string, appId?: string): Promise<StreamSessionRecord[]> {
    if (appId) {
      const result = await this.query<StreamSessionRecord>(
        `SELECT * FROM sg_stream_sessions
         WHERE source_account_id = $1 AND user_id = $2 AND app_id = $3 AND status = 'active'
         ORDER BY started_at DESC`,
        [this.sourceAccountId, userId, appId]
      );
      return result.rows;
    }

    const result = await this.query<StreamSessionRecord>(
      `SELECT * FROM sg_stream_sessions
       WHERE source_account_id = $1 AND user_id = $2 AND status = 'active'
       ORDER BY started_at DESC`,
      [this.sourceAccountId, userId]
    );
    return result.rows;
  }

  async getDeviceActiveSessions(deviceId: string, appId?: string): Promise<StreamSessionRecord[]> {
    if (appId) {
      const result = await this.query<StreamSessionRecord>(
        `SELECT * FROM sg_stream_sessions
         WHERE source_account_id = $1 AND device_id = $2 AND app_id = $3 AND status = 'active'
         ORDER BY started_at DESC`,
        [this.sourceAccountId, deviceId, appId]
      );
      return result.rows;
    }

    const result = await this.query<StreamSessionRecord>(
      `SELECT * FROM sg_stream_sessions
       WHERE source_account_id = $1 AND device_id = $2 AND status = 'active'
       ORDER BY started_at DESC`,
      [this.sourceAccountId, deviceId]
    );
    return result.rows;
  }

  async heartbeatSession(sessionId: string, bytesTransferred?: number, quality?: string): Promise<StreamSessionRecord | null> {
    const setParts = ['last_heartbeat_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, sessionId];

    if (bytesTransferred !== undefined) {
      setParts.push(`bytes_transferred = bytes_transferred + $${params.length + 1}`);
      params.push(bytesTransferred);
    }

    if (quality) {
      setParts.push(`quality = $${params.length + 1}`);
      params.push(quality);
    }

    const result = await this.query<StreamSessionRecord>(
      `UPDATE sg_stream_sessions
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2 AND status = 'active'
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async endSession(sessionId: string, bytesTransferred?: number): Promise<StreamSessionRecord | null> {
    const result = await this.query<StreamSessionRecord>(
      `UPDATE sg_stream_sessions
       SET status = 'ended',
           ended_at = NOW(),
           duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER,
           bytes_transferred = bytes_transferred + COALESCE($3, 0)
       WHERE source_account_id = $1 AND id = $2 AND status = 'active'
       RETURNING *`,
      [this.sourceAccountId, sessionId, bytesTransferred ?? 0]
    );

    return result.rows[0] ?? null;
  }

  async evictSession(sessionId: string): Promise<StreamSessionRecord | null> {
    const result = await this.query<StreamSessionRecord>(
      `UPDATE sg_stream_sessions
       SET status = 'evicted',
           ended_at = NOW(),
           duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER
       WHERE source_account_id = $1 AND id = $2 AND status = 'active'
       RETURNING *`,
      [this.sourceAccountId, sessionId]
    );

    return result.rows[0] ?? null;
  }

  async evictUserFromStream(streamId: string, userId: string): Promise<StreamSessionRecord[]> {
    const result = await this.query<StreamSessionRecord>(
      `UPDATE sg_stream_sessions
       SET status = 'evicted',
           ended_at = NOW(),
           duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER
       WHERE source_account_id = $1 AND stream_id = $2 AND user_id = $3 AND status = 'active'
       RETURNING *`,
      [this.sourceAccountId, streamId, userId]
    );

    return result.rows;
  }

  async expireStaleSessionsByTimeout(timeoutSeconds: number): Promise<number> {
    const rowCount = await this.execute(
      `UPDATE sg_stream_sessions
       SET status = 'ended',
           ended_at = NOW(),
           duration_seconds = EXTRACT(EPOCH FROM (NOW() - started_at))::INTEGER
       WHERE source_account_id = $1
         AND status = 'active'
         AND last_heartbeat_at < NOW() - ($2 || ' seconds')::INTERVAL`,
      [this.sourceAccountId, timeoutSeconds.toString()]
    );

    return rowCount;
  }

  // =========================================================================
  // Streams
  // =========================================================================

  async createStream(appId: string, request: CreateStreamRequest): Promise<StreamRecord> {
    const result = await this.query<StreamRecord>(
      `INSERT INTO sg_streams (
        source_account_id, app_id, stream_id, title, stream_type,
        source_device_id, ingest_url, playback_url, thumbnail_url,
        max_viewers, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      ON CONFLICT (source_account_id, app_id, stream_id)
      DO UPDATE SET
        title = COALESCE(EXCLUDED.title, sg_streams.title),
        stream_type = COALESCE(EXCLUDED.stream_type, sg_streams.stream_type),
        source_device_id = COALESCE(EXCLUDED.source_device_id, sg_streams.source_device_id),
        ingest_url = COALESCE(EXCLUDED.ingest_url, sg_streams.ingest_url),
        playback_url = COALESCE(EXCLUDED.playback_url, sg_streams.playback_url),
        thumbnail_url = COALESCE(EXCLUDED.thumbnail_url, sg_streams.thumbnail_url),
        max_viewers = COALESCE(EXCLUDED.max_viewers, sg_streams.max_viewers),
        metadata = EXCLUDED.metadata,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        request.stream_id,
        request.title ?? null,
        request.stream_type ?? 'live',
        request.source_device_id ?? null,
        request.ingest_url ?? null,
        request.playback_url ?? null,
        request.thumbnail_url ?? null,
        request.max_viewers ?? null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getStream(streamId: string, appId?: string): Promise<StreamRecord | null> {
    if (appId) {
      const result = await this.query<StreamRecord>(
        `SELECT * FROM sg_streams
         WHERE source_account_id = $1 AND stream_id = $2 AND app_id = $3`,
        [this.sourceAccountId, streamId, appId]
      );
      return result.rows[0] ?? null;
    }

    const result = await this.query<StreamRecord>(
      `SELECT * FROM sg_streams
       WHERE source_account_id = $1 AND stream_id = $2`,
      [this.sourceAccountId, streamId]
    );
    return result.rows[0] ?? null;
  }

  async getStreamById(id: string): Promise<StreamRecord | null> {
    const result = await this.query<StreamRecord>(
      `SELECT * FROM sg_streams
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id]
    );
    return result.rows[0] ?? null;
  }

  async listStreams(appId?: string, status?: StreamStatus, limit = 100, offset = 0): Promise<StreamRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (appId) {
      conditions.push(`app_id = $${params.length + 1}`);
      params.push(appId);
    }

    if (status) {
      conditions.push(`status = $${params.length + 1}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.query<StreamRecord>(
      `SELECT * FROM sg_streams
       WHERE ${conditions.join(' AND ')}
       ORDER BY updated_at DESC
       LIMIT $${params.length - 1} OFFSET $${params.length}`,
      params
    );

    return result.rows;
  }

  async updateStream(streamId: string, appId: string, updates: UpdateStreamRequest): Promise<StreamRecord | null> {
    const setParts: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, streamId, appId];

    if (updates.title !== undefined) {
      setParts.push(`title = $${params.length + 1}`);
      params.push(updates.title);
    }

    if (updates.status !== undefined) {
      setParts.push(`status = $${params.length + 1}`);
      params.push(updates.status);
      if (updates.status === 'active') {
        setParts.push('started_at = COALESCE(started_at, NOW())');
      } else if (updates.status === 'ended') {
        setParts.push('ended_at = NOW()');
      }
    }

    if (updates.playback_url !== undefined) {
      setParts.push(`playback_url = $${params.length + 1}`);
      params.push(updates.playback_url);
    }

    if (updates.thumbnail_url !== undefined) {
      setParts.push(`thumbnail_url = $${params.length + 1}`);
      params.push(updates.thumbnail_url);
    }

    if (updates.max_viewers !== undefined) {
      setParts.push(`max_viewers = $${params.length + 1}`);
      params.push(updates.max_viewers);
    }

    if (updates.metadata !== undefined) {
      setParts.push(`metadata = $${params.length + 1}`);
      params.push(JSON.stringify(updates.metadata));
    }

    const result = await this.query<StreamRecord>(
      `UPDATE sg_streams
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND stream_id = $2 AND app_id = $3
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async incrementStreamViewers(streamId: string, appId: string): Promise<void> {
    await this.execute(
      `UPDATE sg_streams
       SET current_viewers = current_viewers + 1,
           total_viewers = total_viewers + 1,
           peak_viewers = GREATEST(peak_viewers, current_viewers + 1),
           updated_at = NOW()
       WHERE source_account_id = $1 AND stream_id = $2 AND app_id = $3`,
      [this.sourceAccountId, streamId, appId]
    );
  }

  async decrementStreamViewers(streamId: string, appId: string): Promise<void> {
    await this.execute(
      `UPDATE sg_streams
       SET current_viewers = GREATEST(current_viewers - 1, 0),
           updated_at = NOW()
       WHERE source_account_id = $1 AND stream_id = $2 AND app_id = $3`,
      [this.sourceAccountId, streamId, appId]
    );
  }

  async getStreamViewers(streamId: string): Promise<StreamSessionRecord[]> {
    const result = await this.query<StreamSessionRecord>(
      `SELECT * FROM sg_stream_sessions
       WHERE source_account_id = $1 AND stream_id = $2 AND status = 'active'
       ORDER BY started_at ASC`,
      [this.sourceAccountId, streamId]
    );
    return result.rows;
  }

  // =========================================================================
  // Admission Rules
  // =========================================================================

  async createRule(appId: string, request: CreateRuleRequest): Promise<AdmissionRuleRecord> {
    const result = await this.query<AdmissionRuleRecord>(
      `INSERT INTO sg_admission_rules (
        source_account_id, app_id, name, rule_type, conditions,
        action, priority, active
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        request.name,
        request.rule_type,
        JSON.stringify(request.conditions),
        request.action ?? 'deny',
        request.priority ?? 0,
        request.active ?? true,
      ]
    );

    return result.rows[0];
  }

  async getRule(ruleId: string): Promise<AdmissionRuleRecord | null> {
    const result = await this.query<AdmissionRuleRecord>(
      `SELECT * FROM sg_admission_rules
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, ruleId]
    );
    return result.rows[0] ?? null;
  }

  async listRules(appId?: string, activeOnly = false): Promise<AdmissionRuleRecord[]> {
    const conditions = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (appId) {
      conditions.push(`app_id = $${params.length + 1}`);
      params.push(appId);
    }

    if (activeOnly) {
      conditions.push('active = TRUE');
    }

    const result = await this.query<AdmissionRuleRecord>(
      `SELECT * FROM sg_admission_rules
       WHERE ${conditions.join(' AND ')}
       ORDER BY priority DESC, created_at ASC`,
      params
    );

    return result.rows;
  }

  async updateRule(ruleId: string, updates: UpdateRuleRequest): Promise<AdmissionRuleRecord | null> {
    const setParts: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [this.sourceAccountId, ruleId];

    if (updates.name !== undefined) {
      setParts.push(`name = $${params.length + 1}`);
      params.push(updates.name);
    }

    if (updates.conditions !== undefined) {
      setParts.push(`conditions = $${params.length + 1}`);
      params.push(JSON.stringify(updates.conditions));
    }

    if (updates.action !== undefined) {
      setParts.push(`action = $${params.length + 1}`);
      params.push(updates.action);
    }

    if (updates.priority !== undefined) {
      setParts.push(`priority = $${params.length + 1}`);
      params.push(updates.priority);
    }

    if (updates.active !== undefined) {
      setParts.push(`active = $${params.length + 1}`);
      params.push(updates.active);
    }

    const result = await this.query<AdmissionRuleRecord>(
      `UPDATE sg_admission_rules
       SET ${setParts.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteRule(ruleId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM sg_admission_rules
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, ruleId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Viewer Analytics
  // =========================================================================

  async upsertAnalytics(
    appId: string,
    streamId: string,
    periodStart: Date,
    periodEnd: Date,
    data: {
      avg_viewers?: number;
      peak_viewers?: number;
      unique_viewers?: number;
      total_view_minutes?: number;
      quality_distribution?: Record<string, unknown>;
      device_distribution?: Record<string, unknown>;
    }
  ): Promise<ViewerAnalyticsRecord> {
    const result = await this.query<ViewerAnalyticsRecord>(
      `INSERT INTO sg_viewer_analytics (
        source_account_id, app_id, stream_id, period_start, period_end,
        avg_viewers, peak_viewers, unique_viewers, total_view_minutes,
        quality_distribution, device_distribution
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      ON CONFLICT (source_account_id, app_id, stream_id, period_start)
      DO UPDATE SET
        avg_viewers = EXCLUDED.avg_viewers,
        peak_viewers = EXCLUDED.peak_viewers,
        unique_viewers = EXCLUDED.unique_viewers,
        total_view_minutes = EXCLUDED.total_view_minutes,
        quality_distribution = EXCLUDED.quality_distribution,
        device_distribution = EXCLUDED.device_distribution
      RETURNING *`,
      [
        this.sourceAccountId,
        appId,
        streamId,
        periodStart,
        periodEnd,
        data.avg_viewers ?? null,
        data.peak_viewers ?? null,
        data.unique_viewers ?? null,
        data.total_view_minutes ?? null,
        JSON.stringify(data.quality_distribution ?? {}),
        JSON.stringify(data.device_distribution ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getStreamAnalytics(streamId: string, limit = 100): Promise<ViewerAnalyticsRecord[]> {
    const result = await this.query<ViewerAnalyticsRecord>(
      `SELECT * FROM sg_viewer_analytics
       WHERE source_account_id = $1 AND stream_id = $2
       ORDER BY period_start DESC
       LIMIT $3`,
      [this.sourceAccountId, streamId, limit]
    );
    return result.rows;
  }

  async getAnalyticsSummary(): Promise<AnalyticsSummary> {
    const result = await this.query<{
      total_streams: number;
      total_view_minutes: number;
      avg_viewers_per_stream: number;
      peak_viewers: number;
      unique_viewers: number;
    }>(
      `SELECT
        COUNT(DISTINCT stream_id) AS total_streams,
        COALESCE(SUM(total_view_minutes), 0) AS total_view_minutes,
        COALESCE(AVG(avg_viewers), 0) AS avg_viewers_per_stream,
        COALESCE(MAX(peak_viewers), 0) AS peak_viewers,
        COALESCE(SUM(unique_viewers), 0) AS unique_viewers
       FROM sg_viewer_analytics
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const topStreamsResult = await this.query<{
      stream_id: string;
      total_view_minutes: number;
      peak_viewers: number;
    }>(
      `SELECT stream_id,
              COALESCE(SUM(total_view_minutes), 0) AS total_view_minutes,
              COALESCE(MAX(peak_viewers), 0) AS peak_viewers
       FROM sg_viewer_analytics
       WHERE source_account_id = $1
       GROUP BY stream_id
       ORDER BY total_view_minutes DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_streams: row?.total_streams ?? 0,
      total_view_minutes: row?.total_view_minutes ?? 0,
      avg_viewers_per_stream: row?.avg_viewers_per_stream ?? 0,
      peak_viewers: row?.peak_viewers ?? 0,
      unique_viewers: row?.unique_viewers ?? 0,
      top_streams: topStreamsResult.rows,
    };
  }

  // =========================================================================
  // Family Members (nTV v1 API)
  // =========================================================================

  async getFamilyMembers(familyId: string): Promise<FamilyMemberRecord[]> {
    const result = await this.query<FamilyMemberRecord>(
      `SELECT * FROM np_streamgw_family_members
       WHERE source_account_id = $1 AND family_id = $2
       ORDER BY created_at ASC`,
      [this.sourceAccountId, familyId]
    );
    return result.rows;
  }

  async getFamilySessions(familyId: string): Promise<StreamSessionRecord[]> {
    const result = await this.query<StreamSessionRecord>(
      `SELECT ss.* FROM sg_stream_sessions ss
       INNER JOIN np_streamgw_family_members fm
         ON fm.user_id = ss.user_id
         AND fm.source_account_id = ss.source_account_id
       WHERE fm.source_account_id = $1
         AND fm.family_id = $2
         AND ss.status = 'active'
       ORDER BY ss.started_at DESC`,
      [this.sourceAccountId, familyId]
    );
    return result.rows;
  }

  async addFamilyMember(familyId: string, userId: string, role = 'member'): Promise<FamilyMemberRecord> {
    const result = await this.query<FamilyMemberRecord>(
      `INSERT INTO np_streamgw_family_members (source_account_id, family_id, user_id, role)
       VALUES ($1, $2, $3, $4)
       RETURNING *`,
      [this.sourceAccountId, familyId, userId, role]
    );
    return result.rows[0];
  }

  async removeFamilyMember(familyId: string, userId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM np_streamgw_family_members
       WHERE source_account_id = $1 AND family_id = $2 AND user_id = $3`,
      [this.sourceAccountId, familyId, userId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getGatewayStats(): Promise<GatewayStats> {
    const result = await this.query<{
      total_streams: number;
      active_streams: number;
      total_sessions: number;
      active_sessions: number;
      denied_sessions: number;
      total_rules: number;
      active_rules: number;
      peak_concurrent_viewers: number;
      last_activity: Date | null;
    }>(
      `WITH np_streamgw_streams AS (
        SELECT
          COUNT(*) AS total,
          COUNT(*) FILTER (WHERE status = 'active') AS active
        FROM sg_streams
        WHERE source_account_id = $1
      ),
      sessions AS (
        SELECT
          COUNT(*) AS total,
          COUNT(*) FILTER (WHERE status = 'active') AS active,
          COUNT(*) FILTER (WHERE status = 'denied') AS denied,
          MAX(created_at) AS last_activity
        FROM sg_stream_sessions
        WHERE source_account_id = $1
      ),
      rules AS (
        SELECT
          COUNT(*) AS total,
          COUNT(*) FILTER (WHERE active = TRUE) AS active
        FROM sg_admission_rules
        WHERE source_account_id = $1
      ),
      peak AS (
        SELECT COALESCE(MAX(peak_viewers), 0) AS peak
        FROM sg_streams
        WHERE source_account_id = $1
      )
      SELECT
        s.total AS total_streams,
        s.active AS active_streams,
        se.total AS total_sessions,
        se.active AS active_sessions,
        se.denied AS denied_sessions,
        r.total AS total_rules,
        r.active AS active_rules,
        p.peak AS peak_concurrent_viewers,
        se.last_activity
      FROM np_streamgw_streams s
      CROSS JOIN sessions se
      CROSS JOIN rules r
      CROSS JOIN peak p`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_streams: row?.total_streams ?? 0,
      active_streams: row?.active_streams ?? 0,
      total_sessions: row?.total_sessions ?? 0,
      active_sessions: row?.active_sessions ?? 0,
      denied_sessions: row?.denied_sessions ?? 0,
      total_rules: row?.total_rules ?? 0,
      active_rules: row?.active_rules ?? 0,
      peak_concurrent_viewers: row?.peak_concurrent_viewers ?? 0,
      last_activity: row?.last_activity ?? null,
    };
  }
}
