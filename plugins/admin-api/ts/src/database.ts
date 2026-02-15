/**
 * Admin API Database Operations
 * CRUD operations for metrics snapshots and dashboard configuration
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  MetricsSnapshotRecord,
  DashboardConfigRecord,
  DashboardStats,
  MetricType,
  SessionInfo,
  SessionDetail,
  StorageBreakdown,
  TableStorageInfo,
  DatabaseStorageInfo,
} from './types.js';

const logger = createLogger('admin-api:db');

export class AdminApiDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): AdminApiDatabase {
    return new AdminApiDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing admin API schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Metrics Snapshots
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_admin_metrics_snapshots (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        metric_type VARCHAR(50) NOT NULL DEFAULT 'system',
        cpu_usage_percent DOUBLE PRECISION,
        memory_used_bytes BIGINT,
        memory_total_bytes BIGINT,
        disk_used_bytes BIGINT,
        disk_total_bytes BIGINT,
        active_connections INTEGER,
        request_count INTEGER,
        error_count INTEGER,
        avg_response_time_ms DOUBLE PRECISION,
        active_sessions INTEGER,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_source_account
        ON np_admin_metrics_snapshots(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_type
        ON np_admin_metrics_snapshots(source_account_id, metric_type);
      CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_created
        ON np_admin_metrics_snapshots(source_account_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_admin_metrics_type_created
        ON np_admin_metrics_snapshots(source_account_id, metric_type, created_at DESC);

      -- =====================================================================
      -- Dashboard Configuration
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_admin_dashboard_config (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        config_key VARCHAR(255) NOT NULL,
        config_value JSONB NOT NULL DEFAULT '{}',
        description TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, config_key)
      );

      CREATE INDEX IF NOT EXISTS idx_np_admin_dashboard_config_source_account
        ON np_admin_dashboard_config(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_admin_dashboard_config_key
        ON np_admin_dashboard_config(source_account_id, config_key);
    `;

    await this.execute(schema);
    logger.info('Admin API schema initialized successfully');
  }

  // =========================================================================
  // Metrics Snapshot Operations
  // =========================================================================

  async createSnapshot(snapshot: Omit<MetricsSnapshotRecord, 'id' | 'created_at'>): Promise<MetricsSnapshotRecord> {
    const result = await this.query<MetricsSnapshotRecord>(
      `INSERT INTO np_admin_metrics_snapshots (
        source_account_id, metric_type, cpu_usage_percent,
        memory_used_bytes, memory_total_bytes,
        disk_used_bytes, disk_total_bytes,
        active_connections, request_count, error_count,
        avg_response_time_ms, active_sessions, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
      RETURNING *`,
      [
        this.sourceAccountId, snapshot.metric_type, snapshot.cpu_usage_percent,
        snapshot.memory_used_bytes, snapshot.memory_total_bytes,
        snapshot.disk_used_bytes, snapshot.disk_total_bytes,
        snapshot.active_connections, snapshot.request_count, snapshot.error_count,
        snapshot.avg_response_time_ms, snapshot.active_sessions,
        JSON.stringify(snapshot.metadata),
      ]
    );

    return result.rows[0];
  }

  async listSnapshots(filters: {
    metricType?: MetricType;
    from?: Date;
    to?: Date;
    limit?: number;
  }): Promise<MetricsSnapshotRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.metricType) {
      conditions.push(`metric_type = $${paramIndex}`);
      values.push(filters.metricType);
      paramIndex++;
    }

    if (filters.from) {
      conditions.push(`created_at >= $${paramIndex}`);
      values.push(filters.from);
      paramIndex++;
    }

    if (filters.to) {
      conditions.push(`created_at <= $${paramIndex}`);
      values.push(filters.to);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    const limit = filters.limit ?? 100;

    const result = await this.query<MetricsSnapshotRecord>(
      `SELECT * FROM np_admin_metrics_snapshots
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  async getLatestSnapshot(metricType?: MetricType): Promise<MetricsSnapshotRecord | null> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];

    if (metricType) {
      conditions.push('metric_type = $2');
      values.push(metricType);
    }

    const result = await this.query<MetricsSnapshotRecord>(
      `SELECT * FROM np_admin_metrics_snapshots
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT 1`,
      values
    );

    return result.rows[0] ?? null;
  }

  async cleanupOldSnapshots(retentionDays: number): Promise<number> {
    const count = await this.execute(
      `DELETE FROM np_admin_metrics_snapshots
       WHERE source_account_id = $1
         AND created_at < NOW() - INTERVAL '${retentionDays} days'`,
      [this.sourceAccountId]
    );
    return count;
  }

  // =========================================================================
  // Dashboard Config Operations
  // =========================================================================

  async getConfig(configKey: string): Promise<DashboardConfigRecord | null> {
    const result = await this.query<DashboardConfigRecord>(
      `SELECT * FROM np_admin_dashboard_config
       WHERE source_account_id = $1 AND config_key = $2`,
      [this.sourceAccountId, configKey]
    );
    return result.rows[0] ?? null;
  }

  async listConfigs(): Promise<DashboardConfigRecord[]> {
    const result = await this.query<DashboardConfigRecord>(
      `SELECT * FROM np_admin_dashboard_config
       WHERE source_account_id = $1
       ORDER BY config_key ASC`,
      [this.sourceAccountId]
    );
    return result.rows;
  }

  async upsertConfig(configKey: string, configValue: Record<string, unknown>, description?: string): Promise<DashboardConfigRecord> {
    const result = await this.query<DashboardConfigRecord>(
      `INSERT INTO np_admin_dashboard_config (
        source_account_id, config_key, config_value, description
      ) VALUES ($1, $2, $3, $4)
      ON CONFLICT (source_account_id, config_key) DO UPDATE SET
        config_value = EXCLUDED.config_value,
        description = COALESCE(EXCLUDED.description, np_admin_dashboard_config.description),
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        configKey,
        JSON.stringify(configValue),
        description ?? null,
      ]
    );

    return result.rows[0];
  }

  async deleteConfig(configKey: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_admin_dashboard_config
       WHERE source_account_id = $1 AND config_key = $2`,
      [this.sourceAccountId, configKey]
    );
    return count > 0;
  }

  // =========================================================================
  // Session Queries (from pg_stat_activity)
  // =========================================================================

  async getActiveSessions(): Promise<SessionInfo> {
    const sessionsResult = await this.query<SessionDetail & Record<string, unknown>>(
      `SELECT
        pid,
        state,
        query_start::text as query_start,
        wait_event_type,
        wait_event,
        backend_type,
        COALESCE(application_name, '') as application_name,
        client_addr::text as client_addr,
        EXTRACT(EPOCH FROM (NOW() - query_start))::double precision as duration_seconds
       FROM pg_stat_activity
       WHERE datname = current_database()
         AND pid != pg_backend_pid()
       ORDER BY state, query_start DESC NULLS LAST`
    );

    const maxResult = await this.query<{ setting: string } & Record<string, unknown>>(
      `SELECT setting FROM pg_settings WHERE name = 'max_connections'`
    );

    const maxConnections = maxResult.rows[0] ? parseInt(String(maxResult.rows[0].setting), 10) : 100;

    let totalActive = 0;
    let totalIdle = 0;
    let totalWaiting = 0;

    for (const s of sessionsResult.rows) {
      if (s.state === 'active') totalActive++;
      else if (s.state === 'idle') totalIdle++;
      else totalWaiting++;
    }

    return {
      total_active: totalActive,
      total_idle: totalIdle,
      total_waiting: totalWaiting,
      max_connections: maxConnections,
      sessions: sessionsResult.rows.map(s => ({
        pid: s.pid as number,
        state: (s.state as string) ?? 'unknown',
        query_start: s.query_start as string | null,
        wait_event_type: s.wait_event_type as string | null,
        wait_event: s.wait_event as string | null,
        backend_type: s.backend_type as string,
        application_name: s.application_name as string,
        client_addr: s.client_addr as string | null,
        duration_seconds: s.duration_seconds as number | null,
      })),
      timestamp: new Date().toISOString(),
    };
  }

  // =========================================================================
  // Storage Queries (from pg_catalog)
  // =========================================================================

  async getStorageBreakdown(): Promise<StorageBreakdown> {
    // Database size
    const dbResult = await this.query<{
      db_name: string;
      db_size: string;
      db_size_pretty: string;
    } & Record<string, unknown>>(
      `SELECT
        current_database() as db_name,
        pg_database_size(current_database())::text as db_size,
        pg_size_pretty(pg_database_size(current_database())) as db_size_pretty`
    );

    // Table count
    const tableCountResult = await this.query<{ count: string } & Record<string, unknown>>(
      `SELECT COUNT(*)::text as count
       FROM information_schema.tables
       WHERE table_schema NOT IN ('pg_catalog', 'information_schema')`
    );

    // Index count
    const indexCountResult = await this.query<{ count: string } & Record<string, unknown>>(
      `SELECT COUNT(*)::text as count
       FROM pg_indexes
       WHERE schemaname NOT IN ('pg_catalog', 'information_schema')`
    );

    // Table sizes
    const tablesResult = await this.query<{
      schema_name: string;
      table_name: string;
      total_size: string;
      table_size: string;
      index_size: string;
      row_estimate: string;
      size_pretty: string;
    } & Record<string, unknown>>(
      `SELECT
        schemaname as schema_name,
        relname as table_name,
        pg_total_relation_size(relid)::text as total_size,
        pg_relation_size(relid)::text as table_size,
        pg_indexes_size(relid)::text as index_size,
        n_live_tup::text as row_estimate,
        pg_size_pretty(pg_total_relation_size(relid)) as size_pretty
       FROM pg_stat_user_tables
       ORDER BY pg_total_relation_size(relid) DESC
       LIMIT 50`
    );

    const dbRow = dbResult.rows[0];
    const dbInfo: DatabaseStorageInfo = {
      name: dbRow?.db_name ?? 'unknown',
      size_bytes: parseInt(String(dbRow?.db_size ?? '0'), 10),
      size_pretty: String(dbRow?.db_size_pretty ?? '0 bytes'),
      table_count: parseInt(String(tableCountResult.rows[0]?.count ?? '0'), 10),
      index_count: parseInt(String(indexCountResult.rows[0]?.count ?? '0'), 10),
    };

    const tables: TableStorageInfo[] = tablesResult.rows.map(t => ({
      schema_name: t.schema_name,
      table_name: t.table_name,
      total_size_bytes: parseInt(t.total_size, 10),
      table_size_bytes: parseInt(t.table_size, 10),
      index_size_bytes: parseInt(t.index_size, 10),
      row_estimate: parseInt(t.row_estimate, 10),
      size_pretty: t.size_pretty,
    }));

    return {
      database: dbInfo,
      tables,
      total_size_bytes: dbInfo.size_bytes,
      timestamp: new Date().toISOString(),
    };
  }

  // =========================================================================
  // Database Health Check
  // =========================================================================

  async checkDatabaseHealth(): Promise<{
    status: string;
    latency_ms: number;
    connection_count: number;
    max_connections: number;
    version: string;
  }> {
    const start = Date.now();

    try {
      const versionResult = await this.query<{ version: string } & Record<string, unknown>>('SELECT version()');
      const latency = Date.now() - start;

      const connResult = await this.query<{ count: string } & Record<string, unknown>>(
        `SELECT COUNT(*)::text as count FROM pg_stat_activity WHERE datname = current_database()`
      );

      const maxResult = await this.query<{ setting: string } & Record<string, unknown>>(
        `SELECT setting FROM pg_settings WHERE name = 'max_connections'`
      );

      return {
        status: 'healthy',
        latency_ms: latency,
        connection_count: parseInt(String(connResult.rows[0]?.count ?? '0'), 10),
        max_connections: parseInt(String(maxResult.rows[0]?.setting ?? '100'), 10),
        version: String(versionResult.rows[0]?.version ?? 'unknown'),
      };
    } catch {
      return {
        status: 'unhealthy',
        latency_ms: Date.now() - start,
        connection_count: 0,
        max_connections: 0,
        version: 'unavailable',
      };
    }
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<DashboardStats> {
    const result = await this.query<{
      snapshots_total: string;
      snapshots_today: string;
      oldest_snapshot: Date | null;
      newest_snapshot: Date | null;
      config_entries: string;
      avg_cpu_24h: string | null;
      avg_memory_24h: string | null;
      peak_connections_24h: string | null;
      total_requests_24h: string | null;
      total_errors_24h: string | null;
    } & Record<string, unknown>>(
      `SELECT
        (SELECT COUNT(*) FROM np_admin_metrics_snapshots WHERE source_account_id = $1)::text as snapshots_total,
        (SELECT COUNT(*) FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= CURRENT_DATE)::text as snapshots_today,
        (SELECT MIN(created_at) FROM np_admin_metrics_snapshots WHERE source_account_id = $1) as oldest_snapshot,
        (SELECT MAX(created_at) FROM np_admin_metrics_snapshots WHERE source_account_id = $1) as newest_snapshot,
        (SELECT COUNT(*) FROM np_admin_dashboard_config WHERE source_account_id = $1)::text as config_entries,
        (SELECT AVG(cpu_usage_percent)::text FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours' AND cpu_usage_percent IS NOT NULL) as avg_cpu_24h,
        (SELECT AVG(memory_used_bytes * 100.0 / NULLIF(memory_total_bytes, 0))::text FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours' AND memory_used_bytes IS NOT NULL) as avg_memory_24h,
        (SELECT MAX(active_connections)::text FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours') as peak_connections_24h,
        (SELECT SUM(request_count)::text FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours') as total_requests_24h,
        (SELECT SUM(error_count)::text FROM np_admin_metrics_snapshots WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '24 hours') as total_errors_24h`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      snapshots_total: parseInt(row.snapshots_total, 10),
      snapshots_today: parseInt(row.snapshots_today, 10),
      oldest_snapshot: row.oldest_snapshot ? row.oldest_snapshot.toISOString() : null,
      newest_snapshot: row.newest_snapshot ? row.newest_snapshot.toISOString() : null,
      config_entries: parseInt(row.config_entries, 10),
      avg_cpu_24h: row.avg_cpu_24h ? parseFloat(row.avg_cpu_24h) : null,
      avg_memory_24h: row.avg_memory_24h ? parseFloat(row.avg_memory_24h) : null,
      peak_connections_24h: row.peak_connections_24h ? parseInt(row.peak_connections_24h, 10) : null,
      total_requests_24h: row.total_requests_24h ? parseInt(row.total_requests_24h, 10) : null,
      total_errors_24h: row.total_errors_24h ? parseInt(row.total_errors_24h, 10) : null,
    };
  }
}
