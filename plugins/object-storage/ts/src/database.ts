/**
 * Object Storage Database Operations
 * Complete CRUD operations for object storage in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  BucketRecord,
  ObjectRecord,
  UploadSessionRecord,
  AccessLogRecord,
  WebhookEventRecord,
  BucketUsageStats,
  StorageStats,
} from './types.js';

const logger = createLogger('object-storage:db');

export class ObjectStorageDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ObjectStorageDatabase {
    return new ObjectStorageDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing object storage schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Buckets
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS os_buckets (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(128) NOT NULL,
        provider VARCHAR(32) DEFAULT 'local',
        provider_config JSONB DEFAULT '{}',
        public_read BOOLEAN DEFAULT false,
        cors_origins TEXT[] DEFAULT '{}',
        max_file_size_bytes BIGINT DEFAULT 104857600,
        allowed_mime_types TEXT[] DEFAULT '{}',
        quota_bytes BIGINT,
        used_bytes BIGINT DEFAULT 0,
        object_count INTEGER DEFAULT 0,
        lifecycle_rules JSONB DEFAULT '[]',
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_os_buckets_source_account ON os_buckets(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_os_buckets_name ON os_buckets(name);
      CREATE INDEX IF NOT EXISTS idx_os_buckets_provider ON os_buckets(provider);
      CREATE INDEX IF NOT EXISTS idx_os_buckets_created ON os_buckets(created_at);

      -- =====================================================================
      -- Objects
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS os_objects (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        bucket_id UUID NOT NULL REFERENCES os_buckets(id) ON DELETE CASCADE,
        key TEXT NOT NULL,
        filename VARCHAR(500),
        content_type VARCHAR(255),
        size_bytes BIGINT NOT NULL,
        checksum_sha256 VARCHAR(64),
        etag VARCHAR(255),
        storage_class VARCHAR(32) DEFAULT 'standard',
        metadata JSONB DEFAULT '{}',
        tags JSONB DEFAULT '{}',
        owner_id VARCHAR(255),
        is_public BOOLEAN DEFAULT false,
        version INTEGER DEFAULT 1,
        deleted_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, bucket_id, key, version)
      );

      CREATE INDEX IF NOT EXISTS idx_os_objects_source_account ON os_objects(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_os_objects_bucket ON os_objects(bucket_id);
      CREATE INDEX IF NOT EXISTS idx_os_objects_key ON os_objects(key);
      CREATE INDEX IF NOT EXISTS idx_os_objects_owner ON os_objects(owner_id);
      CREATE INDEX IF NOT EXISTS idx_os_objects_created ON os_objects(created_at);
      CREATE INDEX IF NOT EXISTS idx_os_objects_storage_class ON os_objects(storage_class);

      -- =====================================================================
      -- Upload Sessions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS os_upload_sessions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        bucket_id UUID NOT NULL REFERENCES os_buckets(id) ON DELETE CASCADE,
        key TEXT NOT NULL,
        content_type VARCHAR(255),
        total_size_bytes BIGINT,
        upload_type VARCHAR(16) DEFAULT 'direct',
        status VARCHAR(32) DEFAULT 'initiated',
        multipart_upload_id VARCHAR(255),
        parts_completed INTEGER DEFAULT 0,
        parts_total INTEGER,
        presigned_url TEXT,
        presigned_expires_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_os_upload_sessions_source_account ON os_upload_sessions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_os_upload_sessions_bucket ON os_upload_sessions(bucket_id);
      CREATE INDEX IF NOT EXISTS idx_os_upload_sessions_status ON os_upload_sessions(status);
      CREATE INDEX IF NOT EXISTS idx_os_upload_sessions_created ON os_upload_sessions(created_at);

      -- =====================================================================
      -- Access Logs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS os_access_logs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        bucket_id UUID,
        object_id UUID,
        action VARCHAR(32) NOT NULL,
        actor_id VARCHAR(255),
        ip_address VARCHAR(45),
        user_agent TEXT,
        status INTEGER,
        response_time_ms INTEGER,
        bytes_transferred BIGINT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_os_access_logs_source_account ON os_access_logs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_os_access_logs_bucket ON os_access_logs(bucket_id);
      CREATE INDEX IF NOT EXISTS idx_os_access_logs_object ON os_access_logs(object_id);
      CREATE INDEX IF NOT EXISTS idx_os_access_logs_action ON os_access_logs(action);
      CREATE INDEX IF NOT EXISTS idx_os_access_logs_created ON os_access_logs(created_at);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS os_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_os_webhook_events_source_account ON os_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_os_webhook_events_type ON os_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_os_webhook_events_processed ON os_webhook_events(processed);
    `;

    const statements = schema
      .split(';')
      .map(s => s.trim())
      .filter(s => s.length > 0);

    for (const statement of statements) {
      await this.execute(statement);
    }

    logger.info('Schema initialized successfully');
  }

  // =========================================================================
  // Bucket Operations
  // =========================================================================

  async createBucket(bucket: Omit<BucketRecord, 'id' | 'created_at' | 'updated_at'>): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO os_buckets (
        source_account_id, name, provider, provider_config, public_read,
        cors_origins, max_file_size_bytes, allowed_mime_types, quota_bytes,
        used_bytes, object_count, lifecycle_rules
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING id`,
      [
        this.sourceAccountId,
        bucket.name,
        bucket.provider,
        JSON.stringify(bucket.provider_config),
        bucket.public_read,
        bucket.cors_origins,
        bucket.max_file_size_bytes,
        bucket.allowed_mime_types,
        bucket.quota_bytes,
        bucket.used_bytes,
        bucket.object_count,
        JSON.stringify(bucket.lifecycle_rules),
      ]
    );

    return result.rows[0].id;
  }

  async getBucketById(id: string): Promise<BucketRecord | null> {
    const result = await this.query<BucketRecord & Record<string, unknown>>(
      `SELECT * FROM os_buckets WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getBucketByName(name: string): Promise<BucketRecord | null> {
    const result = await this.query<BucketRecord & Record<string, unknown>>(
      `SELECT * FROM os_buckets WHERE name = $1 AND source_account_id = $2`,
      [name, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listBuckets(): Promise<BucketRecord[]> {
    const result = await this.query<BucketRecord & Record<string, unknown>>(
      `SELECT * FROM os_buckets WHERE source_account_id = $1 ORDER BY created_at DESC`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async updateBucket(id: string, updates: Partial<Omit<BucketRecord, 'id' | 'source_account_id' | 'created_at'>>): Promise<void> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    for (const [key, value] of Object.entries(updates)) {
      if (value !== undefined) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(['provider_config', 'lifecycle_rules', 'cors_origins', 'allowed_mime_types'].includes(key)
          ? JSON.stringify(value)
          : value
        );
        paramIndex++;
      }
    }

    fields.push(`updated_at = NOW()`);

    values.push(id, this.sourceAccountId);

    await this.execute(
      `UPDATE os_buckets SET ${fields.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      values
    );
  }

  async deleteBucket(id: string): Promise<void> {
    await this.execute(
      `DELETE FROM os_buckets WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async incrementBucketUsage(bucketId: string, sizeBytes: number): Promise<void> {
    await this.execute(
      `UPDATE os_buckets
       SET used_bytes = used_bytes + $1,
           object_count = object_count + 1,
           updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3`,
      [sizeBytes, bucketId, this.sourceAccountId]
    );
  }

  async decrementBucketUsage(bucketId: string, sizeBytes: number): Promise<void> {
    await this.execute(
      `UPDATE os_buckets
       SET used_bytes = GREATEST(0, used_bytes - $1),
           object_count = GREATEST(0, object_count - 1),
           updated_at = NOW()
       WHERE id = $2 AND source_account_id = $3`,
      [sizeBytes, bucketId, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Object Operations
  // =========================================================================

  async createObject(object: Omit<ObjectRecord, 'id' | 'created_at' | 'updated_at'>): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO os_objects (
        source_account_id, bucket_id, key, filename, content_type, size_bytes,
        checksum_sha256, etag, storage_class, metadata, tags, owner_id, is_public, version
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING id`,
      [
        this.sourceAccountId,
        object.bucket_id,
        object.key,
        object.filename,
        object.content_type,
        object.size_bytes,
        object.checksum_sha256,
        object.etag,
        object.storage_class,
        JSON.stringify(object.metadata),
        JSON.stringify(object.tags),
        object.owner_id,
        object.is_public,
        object.version,
      ]
    );

    return result.rows[0].id;
  }

  async getObjectById(id: string): Promise<ObjectRecord | null> {
    const result = await this.query<ObjectRecord & Record<string, unknown>>(
      `SELECT * FROM os_objects WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getObjectByKey(bucketId: string, key: string): Promise<ObjectRecord | null> {
    const result = await this.query<ObjectRecord & Record<string, unknown>>(
      `SELECT * FROM os_objects
       WHERE bucket_id = $1 AND key = $2 AND source_account_id = $3 AND deleted_at IS NULL
       ORDER BY version DESC LIMIT 1`,
      [bucketId, key, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listObjects(bucketId: string, prefix?: string, limit = 1000): Promise<ObjectRecord[]> {
    const params: unknown[] = [this.sourceAccountId, bucketId];
    let query = `SELECT * FROM os_objects WHERE source_account_id = $1 AND bucket_id = $2 AND deleted_at IS NULL`;

    if (prefix) {
      params.push(`${prefix}%`);
      query += ` AND key LIKE $${params.length}`;
    }

    params.push(limit);
    query += ` ORDER BY key ASC LIMIT $${params.length}`;

    const result = await this.query<ObjectRecord & Record<string, unknown>>(query, params);
    return result.rows;
  }

  async deleteObject(id: string): Promise<void> {
    await this.execute(
      `UPDATE os_objects SET deleted_at = NOW() WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async permanentlyDeleteObject(id: string): Promise<void> {
    await this.execute(
      `DELETE FROM os_objects WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Upload Session Operations
  // =========================================================================

  async createUploadSession(session: Omit<UploadSessionRecord, 'id' | 'created_at' | 'updated_at'>): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO os_upload_sessions (
        source_account_id, bucket_id, key, content_type, total_size_bytes,
        upload_type, status, multipart_upload_id, parts_total, presigned_url, presigned_expires_at
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING id`,
      [
        this.sourceAccountId,
        session.bucket_id,
        session.key,
        session.content_type,
        session.total_size_bytes,
        session.upload_type,
        session.status,
        session.multipart_upload_id,
        session.parts_total,
        session.presigned_url,
        session.presigned_expires_at,
      ]
    );

    return result.rows[0].id;
  }

  async getUploadSession(id: string): Promise<UploadSessionRecord | null> {
    const result = await this.query<UploadSessionRecord & Record<string, unknown>>(
      `SELECT * FROM os_upload_sessions WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async updateUploadSession(id: string, updates: Partial<Omit<UploadSessionRecord, 'id' | 'source_account_id' | 'created_at'>>): Promise<void> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    for (const [key, value] of Object.entries(updates)) {
      if (value !== undefined) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    await this.execute(
      `UPDATE os_upload_sessions SET ${fields.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      values
    );
  }

  // =========================================================================
  // Access Log Operations
  // =========================================================================

  async logAccess(log: Omit<AccessLogRecord, 'id' | 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO os_access_logs (
        source_account_id, bucket_id, object_id, action, actor_id,
        ip_address, user_agent, status, response_time_ms, bytes_transferred
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
      [
        this.sourceAccountId,
        log.bucket_id,
        log.object_id,
        log.action,
        log.actor_id,
        log.ip_address,
        log.user_agent,
        log.status,
        log.response_time_ms,
        log.bytes_transferred,
      ]
    );
  }

  async getAccessLogs(filters: {
    bucketId?: string;
    action?: string;
    limit?: number;
  }): Promise<AccessLogRecord[]> {
    const params: unknown[] = [this.sourceAccountId];
    let query = `SELECT * FROM os_access_logs WHERE source_account_id = $1`;

    if (filters.bucketId) {
      params.push(filters.bucketId);
      query += ` AND bucket_id = $${params.length}`;
    }

    if (filters.action) {
      params.push(filters.action);
      query += ` AND action = $${params.length}`;
    }

    query += ` ORDER BY created_at DESC`;

    if (filters.limit) {
      params.push(filters.limit);
      query += ` LIMIT $${params.length}`;
    }

    const result = await this.query<AccessLogRecord & Record<string, unknown>>(query, params);
    return result.rows;
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: Omit<WebhookEventRecord, 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO os_webhook_events (id, source_account_id, event_type, payload, processed)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (id) DO NOTHING`,
      [event.id, this.sourceAccountId, event.event_type, JSON.stringify(event.payload), event.processed]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE os_webhook_events
       SET processed = true, processed_at = NOW(), error = $1
       WHERE id = $2 AND source_account_id = $3`,
      [error ?? null, id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getBucketUsage(bucketId: string): Promise<BucketUsageStats | null> {
    const result = await this.query<BucketUsageStats & Record<string, unknown>>(
      `SELECT
        id as bucket_id,
        name as bucket_name,
        object_count,
        used_bytes as total_bytes,
        quota_bytes,
        CASE
          WHEN quota_bytes IS NOT NULL AND quota_bytes > 0
          THEN (used_bytes::float / quota_bytes::float * 100)
          ELSE NULL
        END as quota_used_percent
      FROM os_buckets
      WHERE id = $1 AND source_account_id = $2`,
      [bucketId, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getStorageStats(): Promise<StorageStats> {
    const result = await this.query<{
      total_buckets: number;
      total_objects: number;
      total_bytes: number;
    }>(
      `SELECT
        COUNT(DISTINCT b.id) as total_buckets,
        COALESCE(SUM(b.object_count), 0) as total_objects,
        COALESCE(SUM(b.used_bytes), 0) as total_bytes
      FROM os_buckets b
      WHERE b.source_account_id = $1`,
      [this.sourceAccountId]
    );

    const stats = result.rows[0];

    return {
      total_buckets: Number(stats.total_buckets),
      total_objects: Number(stats.total_objects),
      total_bytes: Number(stats.total_bytes),
      by_provider: {
        local: { buckets: 0, objects: 0, bytes: 0 },
        s3: { buckets: 0, objects: 0, bytes: 0 },
        minio: { buckets: 0, objects: 0, bytes: 0 },
        r2: { buckets: 0, objects: 0, bytes: 0 },
        gcs: { buckets: 0, objects: 0, bytes: 0 },
        b2: { buckets: 0, objects: 0, bytes: 0 },
        azure: { buckets: 0, objects: 0, bytes: 0 },
      },
      by_storage_class: {
        standard: { objects: 0, bytes: 0 },
        reduced_redundancy: { objects: 0, bytes: 0 },
        glacier: { objects: 0, bytes: 0 },
        deep_archive: { objects: 0, bytes: 0 },
      },
      recent_uploads: 0,
      recent_downloads: 0,
    };
  }
}
