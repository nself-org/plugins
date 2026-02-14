/**
 * Admin API Database Operations
 * Complete CRUD operations for admin users and audit logging
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  AdminUserRecord,
  AuditLogRecord,
  CreateAdminUserInput,
  UpdateAdminUserInput,
  CreateAuditLogInput,
  AdminStats,
  AdminAction,
} from './types.js';

const logger = createLogger('admin-api:db');

export class AdminDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): AdminDatabase {
    return new AdminDatabase(this.db, sourceAccountId);
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
      -- Admin Users
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_admin_users (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        email TEXT NOT NULL,
        password_hash TEXT NOT NULL,
        role TEXT NOT NULL,
        active BOOLEAN DEFAULT TRUE,
        last_login_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CONSTRAINT np_admin_users_email_source_unique UNIQUE (email, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_admin_users_source_account
        ON np_admin_users(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_admin_users_email
        ON np_admin_users(email);
      CREATE INDEX IF NOT EXISTS idx_np_admin_users_role
        ON np_admin_users(role);
      CREATE INDEX IF NOT EXISTS idx_np_admin_users_active
        ON np_admin_users(active);
      CREATE INDEX IF NOT EXISTS idx_np_admin_users_created
        ON np_admin_users(created_at DESC);

      -- =====================================================================
      -- Admin Audit Log (Immutable)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_admin_audit_log (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        admin_user_id UUID REFERENCES np_admin_users(id) ON DELETE SET NULL,
        action TEXT NOT NULL,
        entity_type TEXT,
        entity_id UUID,
        details JSONB DEFAULT '{}',
        ip_address INET,
        user_agent TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
      );

      CREATE INDEX IF NOT EXISTS idx_np_admin_audit_log_source_account
        ON np_admin_audit_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_admin_audit_log_admin_user
        ON np_admin_audit_log(admin_user_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_admin_audit_log_action
        ON np_admin_audit_log(action, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_admin_audit_log_entity
        ON np_admin_audit_log(entity_type, entity_id);
      CREATE INDEX IF NOT EXISTS idx_np_admin_audit_log_created
        ON np_admin_audit_log(created_at DESC);
    `;

    await this.db.execute(schema);
    logger.info('Admin API schema initialized successfully');
  }

  // =========================================================================
  // Admin Users
  // =========================================================================

  async createAdminUser(input: CreateAdminUserInput): Promise<AdminUserRecord> {
    logger.info('Creating admin user', { email: input.email, role: input.role });

    const result = await this.query<AdminUserRecord>(
      `INSERT INTO np_admin_users (source_account_id, email, password_hash, role)
       VALUES ($1, $2, $3, $4)
       RETURNING *`,
      [this.sourceAccountId, input.email, input.password, input.role]
    );

    return result.rows[0];
  }

  async getAdminUser(id: string): Promise<AdminUserRecord | null> {
    const result = await this.query<AdminUserRecord>(
      `SELECT * FROM np_admin_users
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getAdminUserByEmail(email: string): Promise<AdminUserRecord | null> {
    const result = await this.query<AdminUserRecord>(
      `SELECT * FROM np_admin_users
       WHERE email = $1 AND source_account_id = $2`,
      [email, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listAdminUsers(limit = 100, offset = 0): Promise<AdminUserRecord[]> {
    const result = await this.query<AdminUserRecord>(
      `SELECT * FROM np_admin_users
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async updateAdminUser(id: string, input: UpdateAdminUserInput): Promise<AdminUserRecord | null> {
    logger.info('Updating admin user', { id, fields: Object.keys(input) });

    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    if (input.email !== undefined) {
      fields.push(`email = $${paramIndex++}`);
      values.push(input.email);
    }
    if (input.password !== undefined) {
      fields.push(`password_hash = $${paramIndex++}`);
      values.push(input.password);
    }
    if (input.role !== undefined) {
      fields.push(`role = $${paramIndex++}`);
      values.push(input.role);
    }
    if (input.active !== undefined) {
      fields.push(`active = $${paramIndex++}`);
      values.push(input.active);
    }

    if (fields.length === 0) {
      return this.getAdminUser(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<AdminUserRecord>(
      `UPDATE np_admin_users
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteAdminUser(id: string): Promise<boolean> {
    logger.info('Deleting admin user', { id });

    const count = await this.execute(
      `DELETE FROM np_admin_users
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  async updateLastLogin(id: string): Promise<void> {
    await this.execute(
      `UPDATE np_admin_users
       SET last_login_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Audit Log (Immutable)
  // =========================================================================

  async createAuditLog(input: CreateAuditLogInput): Promise<AuditLogRecord> {
    logger.info('Creating audit log', { action: input.action, entity_type: input.entity_type });

    const result = await this.query<AuditLogRecord>(
      `INSERT INTO np_admin_audit_log (
        source_account_id,
        admin_user_id,
        action,
        entity_type,
        entity_id,
        details,
        ip_address,
        user_agent
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.admin_user_id ?? null,
        input.action,
        input.entity_type ?? null,
        input.entity_id ?? null,
        JSON.stringify(input.details ?? {}),
        input.ip_address ?? null,
        input.user_agent ?? null,
      ]
    );

    return result.rows[0];
  }

  async listAuditLogs(
    limit = 100,
    offset = 0,
    filters?: {
      admin_user_id?: string;
      action?: AdminAction;
      entity_type?: string;
      entity_id?: string;
      start_date?: Date;
      end_date?: Date;
    }
  ): Promise<AuditLogRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.admin_user_id) {
      conditions.push(`admin_user_id = $${paramIndex++}`);
      values.push(filters.admin_user_id);
    }
    if (filters?.action) {
      conditions.push(`action = $${paramIndex++}`);
      values.push(filters.action);
    }
    if (filters?.entity_type) {
      conditions.push(`entity_type = $${paramIndex++}`);
      values.push(filters.entity_type);
    }
    if (filters?.entity_id) {
      conditions.push(`entity_id = $${paramIndex++}`);
      values.push(filters.entity_id);
    }
    if (filters?.start_date) {
      conditions.push(`created_at >= $${paramIndex++}`);
      values.push(filters.start_date);
    }
    if (filters?.end_date) {
      conditions.push(`created_at <= $${paramIndex++}`);
      values.push(filters.end_date);
    }

    values.push(limit, offset);

    const result = await this.query<AuditLogRecord>(
      `SELECT * FROM np_admin_audit_log
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      values
    );

    return result.rows;
  }

  async getAuditLog(id: string): Promise<AuditLogRecord | null> {
    const result = await this.query<AuditLogRecord>(
      `SELECT * FROM np_admin_audit_log
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<AdminStats> {
    const totalUsersResult = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM np_admin_users
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const totalAuditLogsResult = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM np_admin_audit_log
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const auditLogsTodayResult = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM np_admin_audit_log
       WHERE source_account_id = $1
       AND created_at >= CURRENT_DATE`,
      [this.sourceAccountId]
    );

    const mostCommonActionsResult = await this.query<{ action: AdminAction; count: number }>(
      `SELECT action, COUNT(*)::int as count
       FROM np_admin_audit_log
       WHERE source_account_id = $1
       GROUP BY action
       ORDER BY count DESC
       LIMIT 10`,
      [this.sourceAccountId]
    );

    return {
      total_users: totalUsersResult.rows[0]?.count ?? 0,
      total_audit_logs: totalAuditLogsResult.rows[0]?.count ?? 0,
      audit_logs_today: auditLogsTodayResult.rows[0]?.count ?? 0,
      most_common_actions: mostCommonActionsResult.rows,
    };
  }
}
