/**
 * Webhooks Database Operations
 * Complete CRUD operations for webhook endpoints, deliveries, and dead letters
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import { randomBytes } from 'crypto';
import type {
  WebhookEndpointRecord,
  WebhookDeliveryRecord,
  WebhookEventTypeRecord,
  WebhookDeadLetterRecord,
  WebhookStats,
  DeliveryStatsByEndpoint,
  DeliveryStatsByEventType,
  CreateEndpointInput,
  UpdateEndpointInput,
  RegisterEventTypeInput,
  DeliveryStatus,
} from './types.js';

const logger = createLogger('webhooks:db');

export class WebhooksDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): WebhooksDatabase {
    return new WebhooksDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing webhooks schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Webhook Endpoints
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS webhook_endpoints (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        url TEXT NOT NULL,
        description TEXT,
        secret VARCHAR(255) NOT NULL,
        events TEXT[] NOT NULL,
        headers JSONB DEFAULT '{}',
        enabled BOOLEAN DEFAULT TRUE,
        failure_count INTEGER DEFAULT 0,
        last_success_at TIMESTAMP WITH TIME ZONE,
        last_failure_at TIMESTAMP WITH TIME ZONE,
        disabled_at TIMESTAMP WITH TIME ZONE,
        disabled_reason TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_source_account
        ON webhook_endpoints(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_enabled
        ON webhook_endpoints(enabled);
      CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_events
        ON webhook_endpoints USING GIN(events);
      CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_created
        ON webhook_endpoints(created_at DESC);

      -- =====================================================================
      -- Webhook Deliveries
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS webhook_deliveries (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        endpoint_id UUID NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        status VARCHAR(32) DEFAULT 'pending',
        response_status INTEGER,
        response_body TEXT,
        response_time_ms INTEGER,
        attempt_count INTEGER DEFAULT 0,
        max_attempts INTEGER DEFAULT 5,
        next_retry_at TIMESTAMP WITH TIME ZONE,
        error_message TEXT,
        signature VARCHAR(255),
        delivered_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_source_account
        ON webhook_deliveries(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint
        ON webhook_deliveries(endpoint_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status
        ON webhook_deliveries(status);
      CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event_type
        ON webhook_deliveries(event_type);
      CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_next_retry
        ON webhook_deliveries(next_retry_at) WHERE status = 'pending';
      CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created
        ON webhook_deliveries(created_at DESC);

      -- =====================================================================
      -- Webhook Event Types
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS webhook_event_types (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        source_plugin VARCHAR(128),
        schema JSONB,
        sample_payload JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_webhook_event_types_source_account
        ON webhook_event_types(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_event_types_name
        ON webhook_event_types(name);
      CREATE INDEX IF NOT EXISTS idx_webhook_event_types_source_plugin
        ON webhook_event_types(source_plugin);

      -- =====================================================================
      -- Webhook Dead Letters
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS webhook_dead_letters (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        delivery_id UUID REFERENCES webhook_deliveries(id),
        endpoint_id UUID REFERENCES webhook_endpoints(id),
        event_type VARCHAR(128),
        payload JSONB,
        last_error TEXT,
        attempt_count INTEGER,
        resolved BOOLEAN DEFAULT FALSE,
        resolved_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_source_account
        ON webhook_dead_letters(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_delivery
        ON webhook_dead_letters(delivery_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_endpoint
        ON webhook_dead_letters(endpoint_id);
      CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_resolved
        ON webhook_dead_letters(resolved);
      CREATE INDEX IF NOT EXISTS idx_webhook_dead_letters_created
        ON webhook_dead_letters(created_at DESC);
    `;

    await this.db.execute(schema);
    logger.success('Webhooks schema initialized');
  }

  // =========================================================================
  // Webhook Endpoints
  // =========================================================================

  generateSecret(): string {
    return `whsec_${randomBytes(32).toString('hex')}`;
  }

  async createEndpoint(input: CreateEndpointInput): Promise<WebhookEndpointRecord> {
    const secret = this.generateSecret();

    const result = await this.db.query<WebhookEndpointRecord>(
      `INSERT INTO webhook_endpoints (
        source_account_id, url, description, secret, events, headers, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.url,
        input.description ?? null,
        secret,
        input.events,
        JSON.stringify(input.headers ?? {}),
        JSON.stringify(input.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getEndpoint(id: string): Promise<WebhookEndpointRecord | null> {
    const result = await this.db.query<WebhookEndpointRecord>(
      'SELECT * FROM webhook_endpoints WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listEndpoints(filters?: { enabled?: boolean }): Promise<WebhookEndpointRecord[]> {
    let sql = 'SELECT * FROM webhook_endpoints WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];

    if (filters?.enabled !== undefined) {
      sql += ' AND enabled = $2';
      params.push(filters.enabled);
    }

    sql += ' ORDER BY created_at DESC';

    const result = await this.db.query<WebhookEndpointRecord>(sql, params);
    return result.rows;
  }

  async updateEndpoint(id: string, input: UpdateEndpointInput): Promise<WebhookEndpointRecord | null> {
    const updates: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (input.url !== undefined) {
      updates.push(`url = $${paramIndex++}`);
      params.push(input.url);
    }

    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      params.push(input.description);
    }

    if (input.events !== undefined) {
      updates.push(`events = $${paramIndex++}`);
      params.push(input.events);
    }

    if (input.headers !== undefined) {
      updates.push(`headers = $${paramIndex++}`);
      params.push(JSON.stringify(input.headers));
    }

    if (input.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex++}`);
      params.push(input.enabled);
    }

    if (input.metadata !== undefined) {
      updates.push(`metadata = $${paramIndex++}`);
      params.push(JSON.stringify(input.metadata));
    }

    if (updates.length === 1) {
      return this.getEndpoint(id);
    }

    params.push(id, this.sourceAccountId);

    const result = await this.db.query<WebhookEndpointRecord>(
      `UPDATE webhook_endpoints
       SET ${updates.join(', ')}
       WHERE id = $${paramIndex++} AND source_account_id = $${paramIndex}
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteEndpoint(id: string): Promise<boolean> {
    const result = await this.db.execute(
      'DELETE FROM webhook_endpoints WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async rotateEndpointSecret(id: string): Promise<string | null> {
    const newSecret = this.generateSecret();

    const result = await this.db.query<{ secret: string }>(
      `UPDATE webhook_endpoints
       SET secret = $1, updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3
       RETURNING secret`,
      [newSecret, id, this.sourceAccountId]
    );

    return result.rows[0]?.secret ?? null;
  }

  async enableEndpoint(id: string): Promise<boolean> {
    const result = await this.db.execute(
      `UPDATE webhook_endpoints
       SET enabled = TRUE, failure_count = 0, disabled_at = NULL, disabled_reason = NULL, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async disableEndpoint(id: string, reason: string): Promise<boolean> {
    const result = await this.db.execute(
      `UPDATE webhook_endpoints
       SET enabled = FALSE, disabled_at = NOW(), disabled_reason = $1, updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3`,
      [reason, id, this.sourceAccountId]
    );

    return result > 0;
  }

  async recordEndpointSuccess(id: string): Promise<void> {
    await this.db.execute(
      `UPDATE webhook_endpoints
       SET failure_count = 0, last_success_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async recordEndpointFailure(id: string, autoDisableThreshold: number): Promise<void> {
    await this.db.execute(
      `UPDATE webhook_endpoints
       SET failure_count = failure_count + 1,
           last_failure_at = NOW(),
           enabled = CASE
             WHEN failure_count + 1 >= $3 THEN FALSE
             ELSE enabled
           END,
           disabled_at = CASE
             WHEN failure_count + 1 >= $3 THEN NOW()
             ELSE disabled_at
           END,
           disabled_reason = CASE
             WHEN failure_count + 1 >= $3 THEN 'Auto-disabled after ' || $3 || ' consecutive failures'
             ELSE disabled_reason
           END,
           updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId, autoDisableThreshold]
    );
  }

  // =========================================================================
  // Webhook Deliveries
  // =========================================================================

  async createDelivery(
    endpointId: string,
    eventType: string,
    payload: Record<string, unknown>,
    signature: string,
    maxAttempts: number
  ): Promise<WebhookDeliveryRecord> {
    const result = await this.db.query<WebhookDeliveryRecord>(
      `INSERT INTO webhook_deliveries (
        source_account_id, endpoint_id, event_type, payload, signature, max_attempts
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [this.sourceAccountId, endpointId, eventType, JSON.stringify(payload), signature, maxAttempts]
    );

    return result.rows[0];
  }

  async getDelivery(id: string): Promise<WebhookDeliveryRecord | null> {
    const result = await this.db.query<WebhookDeliveryRecord>(
      'SELECT * FROM webhook_deliveries WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listDeliveries(filters?: {
    endpointId?: string;
    eventType?: string;
    status?: DeliveryStatus;
    limit?: number;
  }): Promise<WebhookDeliveryRecord[]> {
    let sql = 'SELECT * FROM webhook_deliveries WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.endpointId) {
      sql += ` AND endpoint_id = $${paramIndex++}`;
      params.push(filters.endpointId);
    }

    if (filters?.eventType) {
      sql += ` AND event_type = $${paramIndex++}`;
      params.push(filters.eventType);
    }

    if (filters?.status) {
      sql += ` AND status = $${paramIndex++}`;
      params.push(filters.status);
    }

    sql += ' ORDER BY created_at DESC';

    if (filters?.limit) {
      sql += ` LIMIT $${paramIndex++}`;
      params.push(filters.limit);
    }

    const result = await this.db.query<WebhookDeliveryRecord>(sql, params);
    return result.rows;
  }

  async getPendingDeliveries(limit: number): Promise<WebhookDeliveryRecord[]> {
    const result = await this.db.query<WebhookDeliveryRecord>(
      `SELECT * FROM webhook_deliveries
       WHERE source_account_id = $1
         AND status = 'pending'
         AND (next_retry_at IS NULL OR next_retry_at <= NOW())
       ORDER BY created_at ASC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );

    return result.rows;
  }

  async updateDeliveryStatus(
    id: string,
    status: DeliveryStatus,
    details: {
      responseStatus?: number;
      responseBody?: string;
      responseTimeMs?: number;
      errorMessage?: string;
      nextRetryAt?: Date;
    }
  ): Promise<void> {
    const updates: string[] = ['status = $2', 'attempt_count = attempt_count + 1'];
    const params: unknown[] = [id, status];
    let paramIndex = 3;

    if (status === 'delivered') {
      updates.push('delivered_at = NOW()');
    }

    if (details.responseStatus !== undefined) {
      updates.push(`response_status = $${paramIndex++}`);
      params.push(details.responseStatus);
    }

    if (details.responseBody !== undefined) {
      updates.push(`response_body = $${paramIndex++}`);
      params.push(details.responseBody);
    }

    if (details.responseTimeMs !== undefined) {
      updates.push(`response_time_ms = $${paramIndex++}`);
      params.push(details.responseTimeMs);
    }

    if (details.errorMessage !== undefined) {
      updates.push(`error_message = $${paramIndex++}`);
      params.push(details.errorMessage);
    }

    if (details.nextRetryAt !== undefined) {
      updates.push(`next_retry_at = $${paramIndex++}`);
      params.push(details.nextRetryAt);
    }

    await this.db.execute(
      `UPDATE webhook_deliveries SET ${updates.join(', ')} WHERE id = $1`,
      params
    );
  }

  async retryDelivery(id: string): Promise<boolean> {
    const result = await this.db.execute(
      `UPDATE webhook_deliveries
       SET status = 'pending', next_retry_at = NOW(), error_message = NULL
       WHERE id = $1 AND source_account_id = $2 AND status IN ('failed', 'dead_letter')`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  // =========================================================================
  // Event Types
  // =========================================================================

  async registerEventType(input: RegisterEventTypeInput): Promise<WebhookEventTypeRecord> {
    const result = await this.db.query<WebhookEventTypeRecord>(
      `INSERT INTO webhook_event_types (
        source_account_id, name, description, source_plugin, schema, sample_payload
      ) VALUES ($1, $2, $3, $4, $5, $6)
      ON CONFLICT (source_account_id, name) DO UPDATE SET
        description = EXCLUDED.description,
        source_plugin = EXCLUDED.source_plugin,
        schema = EXCLUDED.schema,
        sample_payload = EXCLUDED.sample_payload
      RETURNING *`,
      [
        this.sourceAccountId,
        input.name,
        input.description ?? null,
        input.source_plugin ?? null,
        input.schema ? JSON.stringify(input.schema) : null,
        input.sample_payload ? JSON.stringify(input.sample_payload) : null,
      ]
    );

    return result.rows[0];
  }

  async listEventTypes(): Promise<WebhookEventTypeRecord[]> {
    const result = await this.db.query<WebhookEventTypeRecord>(
      'SELECT * FROM webhook_event_types WHERE source_account_id = $1 ORDER BY name',
      [this.sourceAccountId]
    );

    return result.rows;
  }

  // =========================================================================
  // Dead Letters
  // =========================================================================

  async createDeadLetter(delivery: WebhookDeliveryRecord): Promise<WebhookDeadLetterRecord> {
    const result = await this.db.query<WebhookDeadLetterRecord>(
      `INSERT INTO webhook_dead_letters (
        source_account_id, delivery_id, endpoint_id, event_type, payload, last_error, attempt_count
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        delivery.id,
        delivery.endpoint_id,
        delivery.event_type,
        JSON.stringify(delivery.payload),
        delivery.error_message ?? 'Unknown error',
        delivery.attempt_count,
      ]
    );

    return result.rows[0];
  }

  async listDeadLetters(resolved?: boolean): Promise<WebhookDeadLetterRecord[]> {
    let sql = 'SELECT * FROM webhook_dead_letters WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];

    if (resolved !== undefined) {
      sql += ' AND resolved = $2';
      params.push(resolved);
    }

    sql += ' ORDER BY created_at DESC';

    const result = await this.db.query<WebhookDeadLetterRecord>(sql, params);
    return result.rows;
  }

  async resolveDeadLetter(id: string): Promise<boolean> {
    const result = await this.db.execute(
      `UPDATE webhook_dead_letters
       SET resolved = TRUE, resolved_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<WebhookStats> {
    const endpoints = await this.db.query<{ total: number; enabled: number; disabled: number }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE enabled = TRUE) as enabled,
        COUNT(*) FILTER (WHERE enabled = FALSE) as disabled
      FROM webhook_endpoints
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const deliveries = await this.db.query<{
      total: number;
      pending: number;
      delivered: number;
      failed: number;
      dead_letter: number;
    }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE status = 'pending') as pending,
        COUNT(*) FILTER (WHERE status = 'delivered') as delivered,
        COUNT(*) FILTER (WHERE status = 'failed') as failed,
        COUNT(*) FILTER (WHERE status = 'dead_letter') as dead_letter
      FROM webhook_deliveries
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const deadLetters = await this.db.query<{
      total: number;
      unresolved: number;
      resolved: number;
    }>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE resolved = FALSE) as unresolved,
        COUNT(*) FILTER (WHERE resolved = TRUE) as resolved
      FROM webhook_dead_letters
      WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const eventTypes = await this.db.query<{ count: number }>(
      'SELECT COUNT(*) as count FROM webhook_event_types WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    return {
      endpoints: endpoints.rows[0] ?? { total: 0, enabled: 0, disabled: 0 },
      deliveries: deliveries.rows[0] ?? { total: 0, pending: 0, delivered: 0, failed: 0, dead_letter: 0 },
      dead_letters: deadLetters.rows[0] ?? { total: 0, unresolved: 0, resolved: 0 },
      event_types: eventTypes.rows[0]?.count ?? 0,
    };
  }

  async getDeliveryStatsByEndpoint(): Promise<DeliveryStatsByEndpoint[]> {
    const result = await this.db.query<DeliveryStatsByEndpoint>(
      `SELECT
        e.id as endpoint_id,
        e.url as endpoint_url,
        COUNT(d.id) as total_deliveries,
        COUNT(*) FILTER (WHERE d.status = 'delivered') as successful,
        COUNT(*) FILTER (WHERE d.status IN ('failed', 'dead_letter')) as failed,
        ROUND(
          (COUNT(*) FILTER (WHERE d.status = 'delivered')::DECIMAL / NULLIF(COUNT(d.id), 0)) * 100,
          2
        ) as success_rate,
        ROUND(AVG(d.response_time_ms)) as avg_response_time_ms
      FROM webhook_endpoints e
      LEFT JOIN webhook_deliveries d ON e.id = d.endpoint_id AND d.source_account_id = e.source_account_id
      WHERE e.source_account_id = $1
      GROUP BY e.id, e.url
      ORDER BY total_deliveries DESC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async getDeliveryStatsByEventType(): Promise<DeliveryStatsByEventType[]> {
    const result = await this.db.query<DeliveryStatsByEventType>(
      `SELECT
        event_type,
        COUNT(*) as total_deliveries,
        COUNT(*) FILTER (WHERE status = 'delivered') as successful,
        COUNT(*) FILTER (WHERE status IN ('failed', 'dead_letter')) as failed,
        ROUND(
          (COUNT(*) FILTER (WHERE status = 'delivered')::DECIMAL / NULLIF(COUNT(*), 0)) * 100,
          2
        ) as success_rate
      FROM webhook_deliveries
      WHERE source_account_id = $1
      GROUP BY event_type
      ORDER BY total_deliveries DESC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }
}
