/**
 * Observability Database Operations
 * Complete CRUD operations for services, health history, and watchdog events
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ServiceRecord,
  HealthHistoryRecord,
  WatchdogEventRecord,
  ObservabilityStats,
  HealthStatus,
  ServiceState,
  WatchdogEventType,
} from './types.js';

const logger = createLogger('observability:db');

export class ObservabilityDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ObservabilityDatabase {
    return new ObservabilityDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing observability schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Services
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_observability_services (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        container_id VARCHAR(128),
        container_name VARCHAR(255),
        image VARCHAR(512),
        service_type VARCHAR(50) DEFAULT 'docker',
        host VARCHAR(255) NOT NULL,
        port INTEGER,
        health_endpoint VARCHAR(512),
        state VARCHAR(20) DEFAULT 'discovered',
        last_health_check TIMESTAMPTZ,
        last_healthy TIMESTAMPTZ,
        consecutive_failures INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_np_observability_services_source
        ON np_observability_services(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_observability_services_state
        ON np_observability_services(source_account_id, state);
      CREATE INDEX IF NOT EXISTS idx_np_observability_services_container
        ON np_observability_services(container_id) WHERE container_id IS NOT NULL;

      -- =====================================================================
      -- Health History
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_observability_health_history (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        service_id UUID NOT NULL REFERENCES np_observability_services(id) ON DELETE CASCADE,
        status VARCHAR(20) NOT NULL,
        response_time_ms INTEGER,
        status_code INTEGER,
        error_message TEXT,
        metadata JSONB DEFAULT '{}',
        checked_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_observability_health_history_source
        ON np_observability_health_history(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_observability_health_history_service
        ON np_observability_health_history(service_id, checked_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_observability_health_history_time
        ON np_observability_health_history(source_account_id, checked_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_observability_health_history_status
        ON np_observability_health_history(status);

      -- =====================================================================
      -- Watchdog Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_observability_watchdog_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        service_id UUID REFERENCES np_observability_services(id) ON DELETE SET NULL,
        event_type VARCHAR(50) NOT NULL,
        message TEXT NOT NULL,
        severity VARCHAR(20) DEFAULT 'info',
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_observability_watchdog_events_source
        ON np_observability_watchdog_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_observability_watchdog_events_service
        ON np_observability_watchdog_events(service_id) WHERE service_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_np_observability_watchdog_events_type
        ON np_observability_watchdog_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_np_observability_watchdog_events_time
        ON np_observability_watchdog_events(source_account_id, created_at DESC);
    `;

    await this.execute(schema);
    logger.info('Observability schema initialized successfully');
  }

  // =========================================================================
  // Service Operations
  // =========================================================================

  async registerService(service: Omit<ServiceRecord, 'id' | 'created_at' | 'updated_at'>): Promise<ServiceRecord> {
    const result = await this.query<ServiceRecord>(
      `INSERT INTO np_observability_services (
        source_account_id, name, container_id, container_name, image,
        service_type, host, port, health_endpoint, state,
        last_health_check, last_healthy, consecutive_failures, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      ON CONFLICT (source_account_id, name) DO UPDATE SET
        container_id = COALESCE(EXCLUDED.container_id, np_observability_services.container_id),
        container_name = COALESCE(EXCLUDED.container_name, np_observability_services.container_name),
        image = COALESCE(EXCLUDED.image, np_observability_services.image),
        host = EXCLUDED.host,
        port = COALESCE(EXCLUDED.port, np_observability_services.port),
        health_endpoint = COALESCE(EXCLUDED.health_endpoint, np_observability_services.health_endpoint),
        state = CASE
          WHEN np_observability_services.state = 'removed' THEN 'discovered'
          ELSE np_observability_services.state
        END,
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, service.name, service.container_id,
        service.container_name, service.image, service.service_type,
        service.host, service.port, service.health_endpoint,
        service.state ?? 'discovered', service.last_health_check,
        service.last_healthy, service.consecutive_failures ?? 0,
        JSON.stringify(service.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getService(id: string): Promise<ServiceRecord | null> {
    const result = await this.query<ServiceRecord>(
      `SELECT * FROM np_observability_services WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getServiceByName(name: string): Promise<ServiceRecord | null> {
    const result = await this.query<ServiceRecord>(
      `SELECT * FROM np_observability_services WHERE name = $1 AND source_account_id = $2`,
      [name, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listServices(filters: {
    state?: ServiceState;
    serviceType?: string;
    limit?: number;
    offset?: number;
  }): Promise<ServiceRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.state) {
      conditions.push(`state = $${paramIndex}`);
      values.push(filters.state);
      paramIndex++;
    }

    if (filters.serviceType) {
      conditions.push(`service_type = $${paramIndex}`);
      values.push(filters.serviceType);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    let sql = `
      SELECT * FROM np_observability_services
      WHERE ${conditions.join(' AND ')}
      ORDER BY name ASC
    `;

    if (filters.limit) sql += ` LIMIT ${filters.limit}`;
    if (filters.offset) sql += ` OFFSET ${filters.offset}`;

    const result = await this.query<ServiceRecord>(sql, values);
    return result.rows;
  }

  async updateService(id: string, updates: Partial<ServiceRecord>): Promise<ServiceRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'name', 'container_id', 'container_name', 'image',
      'service_type', 'host', 'port', 'health_endpoint',
      'state', 'last_health_check', 'last_healthy', 'consecutive_failures',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (updates.metadata !== undefined) {
      fields.push(`metadata = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.metadata));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getService(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<ServiceRecord>(
      `UPDATE np_observability_services
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteService(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM np_observability_services WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return count > 0;
  }

  async updateServiceState(id: string, state: ServiceState, consecutiveFailures?: number): Promise<ServiceRecord | null> {
    const updates: Record<string, unknown> = { state };

    if (state === 'healthy') {
      updates.last_healthy = new Date();
      updates.consecutive_failures = 0;
    }

    if (consecutiveFailures !== undefined) {
      updates.consecutive_failures = consecutiveFailures;
    }

    updates.last_health_check = new Date();

    return this.updateService(id, updates as Partial<ServiceRecord>);
  }

  // =========================================================================
  // Health History Operations
  // =========================================================================

  async recordHealthCheck(data: {
    serviceId: string;
    status: HealthStatus;
    responseTimeMs?: number;
    statusCode?: number;
    errorMessage?: string;
    metadata?: Record<string, unknown>;
  }): Promise<HealthHistoryRecord> {
    const result = await this.query<HealthHistoryRecord>(
      `INSERT INTO np_observability_health_history (
        source_account_id, service_id, status, response_time_ms,
        status_code, error_message, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId, data.serviceId, data.status,
        data.responseTimeMs ?? null, data.statusCode ?? null,
        data.errorMessage ?? null, JSON.stringify(data.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async listHealthHistory(filters: {
    serviceId?: string;
    status?: HealthStatus;
    from?: Date;
    to?: Date;
    limit?: number;
  }): Promise<HealthHistoryRecord[]> {
    const conditions: string[] = ['h.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.serviceId) {
      conditions.push(`h.service_id = $${paramIndex}`);
      values.push(filters.serviceId);
      paramIndex++;
    }

    if (filters.status) {
      conditions.push(`h.status = $${paramIndex}`);
      values.push(filters.status);
      paramIndex++;
    }

    if (filters.from) {
      conditions.push(`h.checked_at >= $${paramIndex}`);
      values.push(filters.from);
      paramIndex++;
    }

    if (filters.to) {
      conditions.push(`h.checked_at <= $${paramIndex}`);
      values.push(filters.to);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    const limit = filters.limit ?? 100;

    const result = await this.query<HealthHistoryRecord>(
      `SELECT h.* FROM np_observability_health_history h
       WHERE ${conditions.join(' AND ')}
       ORDER BY h.checked_at DESC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  async getLatestHealthForService(serviceId: string): Promise<HealthHistoryRecord | null> {
    const result = await this.query<HealthHistoryRecord>(
      `SELECT * FROM np_observability_health_history
       WHERE service_id = $1 AND source_account_id = $2
       ORDER BY checked_at DESC
       LIMIT 1`,
      [serviceId, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async cleanupOldHealthHistory(days: number): Promise<number> {
    const count = await this.execute(
      `DELETE FROM np_observability_health_history
       WHERE source_account_id = $1
         AND checked_at < NOW() - INTERVAL '${days} days'`,
      [this.sourceAccountId]
    );
    return count;
  }

  // =========================================================================
  // Watchdog Event Operations
  // =========================================================================

  async createWatchdogEvent(data: {
    serviceId?: string;
    eventType: WatchdogEventType;
    message: string;
    severity?: string;
    metadata?: Record<string, unknown>;
  }): Promise<WatchdogEventRecord> {
    const result = await this.query<WatchdogEventRecord>(
      `INSERT INTO np_observability_watchdog_events (
        source_account_id, service_id, event_type, message, severity, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId, data.serviceId ?? null, data.eventType,
        data.message, data.severity ?? 'info',
        JSON.stringify(data.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async listWatchdogEvents(filters: {
    serviceId?: string;
    eventType?: WatchdogEventType;
    severity?: string;
    from?: Date;
    to?: Date;
    limit?: number;
  }): Promise<WatchdogEventRecord[]> {
    const conditions: string[] = ['e.source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.serviceId) {
      conditions.push(`e.service_id = $${paramIndex}`);
      values.push(filters.serviceId);
      paramIndex++;
    }

    if (filters.eventType) {
      conditions.push(`e.event_type = $${paramIndex}`);
      values.push(filters.eventType);
      paramIndex++;
    }

    if (filters.severity) {
      conditions.push(`e.severity = $${paramIndex}`);
      values.push(filters.severity);
      paramIndex++;
    }

    if (filters.from) {
      conditions.push(`e.created_at >= $${paramIndex}`);
      values.push(filters.from);
      paramIndex++;
    }

    if (filters.to) {
      conditions.push(`e.created_at <= $${paramIndex}`);
      values.push(filters.to);
      paramIndex++;
    }

    // Suppress unused variable warning
    void paramIndex;

    const limit = filters.limit ?? 100;

    const result = await this.query<WatchdogEventRecord>(
      `SELECT e.* FROM np_observability_watchdog_events e
       WHERE ${conditions.join(' AND ')}
       ORDER BY e.created_at DESC
       LIMIT ${limit}`,
      values
    );

    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<ObservabilityStats> {
    const result = await this.query<{
      total_services: string;
      healthy_services: string;
      unhealthy_services: string;
      degraded_services: string;
      total_health_checks: string;
      total_watchdog_events: string;
      oldest_service: Date | null;
      newest_service: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM np_observability_services WHERE source_account_id = $1 AND state != 'removed') as total_services,
        (SELECT COUNT(*) FROM np_observability_services WHERE source_account_id = $1 AND state = 'healthy') as healthy_services,
        (SELECT COUNT(*) FROM np_observability_services WHERE source_account_id = $1 AND state = 'unhealthy') as unhealthy_services,
        (SELECT COUNT(*) FROM np_observability_services WHERE source_account_id = $1 AND state = 'degraded') as degraded_services,
        (SELECT COUNT(*) FROM np_observability_health_history WHERE source_account_id = $1) as total_health_checks,
        (SELECT COUNT(*) FROM np_observability_watchdog_events WHERE source_account_id = $1) as total_watchdog_events,
        (SELECT MIN(created_at) FROM np_observability_services WHERE source_account_id = $1) as oldest_service,
        (SELECT MAX(created_at) FROM np_observability_services WHERE source_account_id = $1) as newest_service`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_services: parseInt(row.total_services, 10),
      healthy_services: parseInt(row.healthy_services, 10),
      unhealthy_services: parseInt(row.unhealthy_services, 10),
      degraded_services: parseInt(row.degraded_services, 10),
      total_health_checks: parseInt(row.total_health_checks, 10),
      total_watchdog_events: parseInt(row.total_watchdog_events, 10),
      oldest_service: row.oldest_service,
      newest_service: row.newest_service,
    };
  }
}
