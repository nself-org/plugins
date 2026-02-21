/**
 * Invitations Database Operations
 * Complete CRUD operations for all invitation objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  InvitationRecord,
  TemplateRecord,
  BulkSendRecord,
  WebhookEventRecord,
  InvitationStats,
  InvitationType,
  InvitationChannel,
  InvitationStatus,
} from './types.js';

const logger = createLogger('invitations:db');

export class InvitationsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): InvitationsDatabase {
    return new InvitationsDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing invitations schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Invitations Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS inv_invitations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        type VARCHAR(64) NOT NULL DEFAULT 'app_signup',
        inviter_id VARCHAR(255) NOT NULL,
        invitee_email VARCHAR(255),
        invitee_phone VARCHAR(32),
        invitee_name VARCHAR(255),
        code VARCHAR(64) NOT NULL UNIQUE,
        status VARCHAR(32) DEFAULT 'pending',
        channel VARCHAR(16) DEFAULT 'email',
        message TEXT,
        role VARCHAR(64),
        resource_type VARCHAR(64),
        resource_id VARCHAR(255),
        expires_at TIMESTAMP WITH TIME ZONE,
        sent_at TIMESTAMP WITH TIME ZONE,
        delivered_at TIMESTAMP WITH TIME ZONE,
        accepted_at TIMESTAMP WITH TIME ZONE,
        accepted_by VARCHAR(255),
        declined_at TIMESTAMP WITH TIME ZONE,
        revoked_at TIMESTAMP WITH TIME ZONE,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CONSTRAINT inv_invitations_check_contact
          CHECK (invitee_email IS NOT NULL OR invitee_phone IS NOT NULL OR channel = 'link'),
        CONSTRAINT inv_invitations_check_status
          CHECK (status IN ('pending', 'sent', 'delivered', 'accepted', 'declined', 'expired', 'revoked')),
        CONSTRAINT inv_invitations_check_type
          CHECK (type IN ('app_signup', 'family_join', 'team_join', 'event_attend', 'share_access')),
        CONSTRAINT inv_invitations_check_channel
          CHECK (channel IN ('email', 'sms', 'link'))
      );

      CREATE INDEX IF NOT EXISTS idx_inv_invitations_source_account
        ON inv_invitations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_inviter
        ON inv_invitations(inviter_id);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_invitee_email
        ON inv_invitations(invitee_email);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_code
        ON inv_invitations(code);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_status
        ON inv_invitations(status);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_type
        ON inv_invitations(type);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_expires
        ON inv_invitations(expires_at);
      CREATE INDEX IF NOT EXISTS idx_inv_invitations_created
        ON inv_invitations(created_at DESC);

      -- =====================================================================
      -- Templates Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS inv_templates (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        type VARCHAR(64) NOT NULL,
        channel VARCHAR(16) NOT NULL,
        subject VARCHAR(500),
        body TEXT NOT NULL,
        variables TEXT[] DEFAULT '{}',
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CONSTRAINT inv_templates_unique_name
          UNIQUE(source_account_id, name),
        CONSTRAINT inv_templates_check_type
          CHECK (type IN ('app_signup', 'family_join', 'team_join', 'event_attend', 'share_access')),
        CONSTRAINT inv_templates_check_channel
          CHECK (channel IN ('email', 'sms', 'link'))
      );

      CREATE INDEX IF NOT EXISTS idx_inv_templates_source_account
        ON inv_templates(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_inv_templates_type
        ON inv_templates(type);
      CREATE INDEX IF NOT EXISTS idx_inv_templates_enabled
        ON inv_templates(enabled);

      -- =====================================================================
      -- Bulk Sends Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS inv_bulk_sends (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        inviter_id VARCHAR(255) NOT NULL,
        template_id UUID REFERENCES inv_templates(id) ON DELETE SET NULL,
        type VARCHAR(64) NOT NULL,
        total_count INTEGER DEFAULT 0,
        sent_count INTEGER DEFAULT 0,
        failed_count INTEGER DEFAULT 0,
        status VARCHAR(32) DEFAULT 'pending',
        invitees JSONB NOT NULL,
        metadata JSONB DEFAULT '{}',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CONSTRAINT inv_bulk_sends_check_status
          CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
        CONSTRAINT inv_bulk_sends_check_type
          CHECK (type IN ('app_signup', 'family_join', 'team_join', 'event_attend', 'share_access'))
      );

      CREATE INDEX IF NOT EXISTS idx_inv_bulk_sends_source_account
        ON inv_bulk_sends(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_inv_bulk_sends_inviter
        ON inv_bulk_sends(inviter_id);
      CREATE INDEX IF NOT EXISTS idx_inv_bulk_sends_status
        ON inv_bulk_sends(status);
      CREATE INDEX IF NOT EXISTS idx_inv_bulk_sends_created
        ON inv_bulk_sends(created_at DESC);

      -- =====================================================================
      -- Webhook Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS inv_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_inv_webhook_events_source_account
        ON inv_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_inv_webhook_events_type
        ON inv_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_inv_webhook_events_processed
        ON inv_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_inv_webhook_events_created
        ON inv_webhook_events(created_at DESC);
    `;

    await this.execute(schema);
    logger.success('Schema initialized');
  }

  // =========================================================================
  // Invitations CRUD
  // =========================================================================

  async createInvitation(invitation: Omit<InvitationRecord, 'id' | 'source_account_id' | 'created_at' | 'updated_at'>): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO inv_invitations (
        source_account_id, type, inviter_id, invitee_email, invitee_phone, invitee_name,
        code, status, channel, message, role, resource_type, resource_id,
        expires_at, metadata
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
      RETURNING id`,
      [
        this.sourceAccountId,
        invitation.type,
        invitation.inviter_id,
        invitation.invitee_email,
        invitation.invitee_phone,
        invitation.invitee_name,
        invitation.code,
        invitation.status,
        invitation.channel,
        invitation.message,
        invitation.role,
        invitation.resource_type,
        invitation.resource_id,
        invitation.expires_at,
        JSON.stringify(invitation.metadata),
      ]
    );

    return result.rows[0].id;
  }

  async getInvitation(id: string): Promise<InvitationRecord | null> {
    const result = await this.query<InvitationRecord>(
      'SELECT * FROM inv_invitations WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getInvitationByCode(code: string): Promise<InvitationRecord | null> {
    const result = await this.query<InvitationRecord>(
      'SELECT * FROM inv_invitations WHERE code = $1 AND source_account_id = $2',
      [code, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listInvitations(
    limit = 100,
    offset = 0,
    filters?: { type?: InvitationType; status?: InvitationStatus; inviter_id?: string }
  ): Promise<InvitationRecord[]> {
    let sql = 'SELECT * FROM inv_invitations WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.type) {
      sql += ` AND type = $${paramIndex}`;
      params.push(filters.type);
      paramIndex++;
    }

    if (filters?.status) {
      sql += ` AND status = $${paramIndex}`;
      params.push(filters.status);
      paramIndex++;
    }

    if (filters?.inviter_id) {
      sql += ` AND inviter_id = $${paramIndex}`;
      params.push(filters.inviter_id);
      paramIndex++;
    }

    sql += ` ORDER BY created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<InvitationRecord>(sql, params);
    return result.rows;
  }

  async countInvitations(filters?: { type?: InvitationType; status?: InvitationStatus; inviter_id?: string }): Promise<number> {
    let sql = 'SELECT COUNT(*) as count FROM inv_invitations WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.type) {
      sql += ` AND type = $${paramIndex}`;
      params.push(filters.type);
      paramIndex++;
    }

    if (filters?.status) {
      sql += ` AND status = $${paramIndex}`;
      params.push(filters.status);
      paramIndex++;
    }

    if (filters?.inviter_id) {
      sql += ` AND inviter_id = $${paramIndex}`;
      params.push(filters.inviter_id);
      paramIndex++;
    }

    const result = await this.query<{ count: string }>(sql, params);
    return parseInt(result.rows[0].count, 10);
  }

  async updateInvitationStatus(
    id: string,
    status: InvitationStatus,
    data?: {
      sent_at?: Date;
      delivered_at?: Date;
      accepted_at?: Date;
      accepted_by?: string;
      declined_at?: Date;
      revoked_at?: Date;
    }
  ): Promise<void> {
    const fields: string[] = ['status = $2', 'updated_at = NOW()'];
    const params: unknown[] = [id, status];
    let paramIndex = 3;

    if (data?.sent_at) {
      fields.push(`sent_at = $${paramIndex}`);
      params.push(data.sent_at);
      paramIndex++;
    }

    if (data?.delivered_at) {
      fields.push(`delivered_at = $${paramIndex}`);
      params.push(data.delivered_at);
      paramIndex++;
    }

    if (data?.accepted_at) {
      fields.push(`accepted_at = $${paramIndex}`);
      params.push(data.accepted_at);
      paramIndex++;
    }

    if (data?.accepted_by) {
      fields.push(`accepted_by = $${paramIndex}`);
      params.push(data.accepted_by);
      paramIndex++;
    }

    if (data?.declined_at) {
      fields.push(`declined_at = $${paramIndex}`);
      params.push(data.declined_at);
      paramIndex++;
    }

    if (data?.revoked_at) {
      fields.push(`revoked_at = $${paramIndex}`);
      params.push(data.revoked_at);
      paramIndex++;
    }

    await this.execute(
      `UPDATE inv_invitations SET ${fields.join(', ')} WHERE id = $1 AND source_account_id = $${paramIndex}`,
      [...params, this.sourceAccountId]
    );
  }

  async deleteInvitation(id: string): Promise<void> {
    await this.execute(
      'DELETE FROM inv_invitations WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  async markExpiredInvitations(): Promise<number> {
    return await this.execute(
      `UPDATE inv_invitations
       SET status = 'expired', updated_at = NOW()
       WHERE status IN ('pending', 'sent', 'delivered')
       AND expires_at < NOW()
       AND source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Templates CRUD
  // =========================================================================

  async createTemplate(template: Omit<TemplateRecord, 'id' | 'source_account_id' | 'created_at' | 'updated_at'>): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO inv_templates (
        source_account_id, name, type, channel, subject, body, variables, enabled
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING id`,
      [
        this.sourceAccountId,
        template.name,
        template.type,
        template.channel,
        template.subject,
        template.body,
        template.variables,
        template.enabled,
      ]
    );

    return result.rows[0].id;
  }

  async getTemplate(id: string): Promise<TemplateRecord | null> {
    const result = await this.query<TemplateRecord>(
      'SELECT * FROM inv_templates WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getTemplateByName(name: string): Promise<TemplateRecord | null> {
    const result = await this.query<TemplateRecord>(
      'SELECT * FROM inv_templates WHERE name = $1 AND source_account_id = $2',
      [name, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listTemplates(limit = 100, offset = 0, filters?: { type?: InvitationType; channel?: InvitationChannel; enabled?: boolean }): Promise<TemplateRecord[]> {
    let sql = 'SELECT * FROM inv_templates WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters?.type) {
      sql += ` AND type = $${paramIndex}`;
      params.push(filters.type);
      paramIndex++;
    }

    if (filters?.channel) {
      sql += ` AND channel = $${paramIndex}`;
      params.push(filters.channel);
      paramIndex++;
    }

    if (filters?.enabled !== undefined) {
      sql += ` AND enabled = $${paramIndex}`;
      params.push(filters.enabled);
      paramIndex++;
    }

    sql += ` ORDER BY created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<TemplateRecord>(sql, params);
    return result.rows;
  }

  async updateTemplate(
    id: string,
    updates: {
      name?: string;
      subject?: string;
      body?: string;
      variables?: string[];
      enabled?: boolean;
    }
  ): Promise<void> {
    const fields: string[] = ['updated_at = NOW()'];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.name !== undefined) {
      fields.push(`name = $${paramIndex}`);
      params.push(updates.name);
      paramIndex++;
    }

    if (updates.subject !== undefined) {
      fields.push(`subject = $${paramIndex}`);
      params.push(updates.subject);
      paramIndex++;
    }

    if (updates.body !== undefined) {
      fields.push(`body = $${paramIndex}`);
      params.push(updates.body);
      paramIndex++;
    }

    if (updates.variables !== undefined) {
      fields.push(`variables = $${paramIndex}`);
      params.push(updates.variables);
      paramIndex++;
    }

    if (updates.enabled !== undefined) {
      fields.push(`enabled = $${paramIndex}`);
      params.push(updates.enabled);
      paramIndex++;
    }

    if (fields.length === 1) {
      return; // No updates
    }

    await this.execute(
      `UPDATE inv_templates SET ${fields.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      [...params, id, this.sourceAccountId]
    );
  }

  async deleteTemplate(id: string): Promise<void> {
    await this.execute(
      'DELETE FROM inv_templates WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Bulk Sends CRUD
  // =========================================================================

  async createBulkSend(bulkSend: Omit<BulkSendRecord, 'id' | 'source_account_id' | 'created_at'>): Promise<string> {
    const result = await this.query<{ id: string }>(
      `INSERT INTO inv_bulk_sends (
        source_account_id, inviter_id, template_id, type, total_count,
        sent_count, failed_count, status, invitees, metadata, started_at, completed_at
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING id`,
      [
        this.sourceAccountId,
        bulkSend.inviter_id,
        bulkSend.template_id,
        bulkSend.type,
        bulkSend.total_count,
        bulkSend.sent_count,
        bulkSend.failed_count,
        bulkSend.status,
        JSON.stringify(bulkSend.invitees),
        JSON.stringify(bulkSend.metadata),
        bulkSend.started_at,
        bulkSend.completed_at,
      ]
    );

    return result.rows[0].id;
  }

  async getBulkSend(id: string): Promise<BulkSendRecord | null> {
    const result = await this.query<BulkSendRecord>(
      'SELECT * FROM inv_bulk_sends WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listBulkSends(limit = 100, offset = 0): Promise<BulkSendRecord[]> {
    const result = await this.query<BulkSendRecord>(
      `SELECT * FROM inv_bulk_sends
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async updateBulkSend(
    id: string,
    updates: {
      sent_count?: number;
      failed_count?: number;
      status?: string;
      started_at?: Date;
      completed_at?: Date;
    }
  ): Promise<void> {
    const fields: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.sent_count !== undefined) {
      fields.push(`sent_count = $${paramIndex}`);
      params.push(updates.sent_count);
      paramIndex++;
    }

    if (updates.failed_count !== undefined) {
      fields.push(`failed_count = $${paramIndex}`);
      params.push(updates.failed_count);
      paramIndex++;
    }

    if (updates.status !== undefined) {
      fields.push(`status = $${paramIndex}`);
      params.push(updates.status);
      paramIndex++;
    }

    if (updates.started_at !== undefined) {
      fields.push(`started_at = $${paramIndex}`);
      params.push(updates.started_at);
      paramIndex++;
    }

    if (updates.completed_at !== undefined) {
      fields.push(`completed_at = $${paramIndex}`);
      params.push(updates.completed_at);
      paramIndex++;
    }

    if (fields.length === 0) {
      return;
    }

    await this.execute(
      `UPDATE inv_bulk_sends SET ${fields.join(', ')} WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      [...params, id, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: Omit<WebhookEventRecord, 'source_account_id' | 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO inv_webhook_events (id, source_account_id, event_type, payload, processed)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (id) DO NOTHING`,
      [event.id, this.sourceAccountId, event.event_type, JSON.stringify(event.payload), event.processed]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE inv_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1 AND source_account_id = $3`,
      [id, error ?? null, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<InvitationStats> {
    const countResult = await this.query<{ status: InvitationStatus; count: string }>(
      `SELECT status, COUNT(*) as count
       FROM inv_invitations
       WHERE source_account_id = $1
       GROUP BY status`,
      [this.sourceAccountId]
    );

    const typeResult = await this.query<{ type: InvitationType; count: string }>(
      `SELECT type, COUNT(*) as count
       FROM inv_invitations
       WHERE source_account_id = $1
       GROUP BY type`,
      [this.sourceAccountId]
    );

    const channelResult = await this.query<{ channel: InvitationChannel; count: string }>(
      `SELECT channel, COUNT(*) as count
       FROM inv_invitations
       WHERE source_account_id = $1
       GROUP BY channel`,
      [this.sourceAccountId]
    );

    const stats: InvitationStats = {
      total: 0,
      pending: 0,
      sent: 0,
      delivered: 0,
      accepted: 0,
      declined: 0,
      expired: 0,
      revoked: 0,
      conversionRate: 0,
      byType: {} as Record<InvitationType, number>,
      byChannel: {} as Record<InvitationChannel, number>,
    };

    for (const row of countResult.rows) {
      const count = parseInt(row.count, 10);
      stats.total += count;
      stats[row.status] = count;
    }

    for (const row of typeResult.rows) {
      stats.byType[row.type] = parseInt(row.count, 10);
    }

    for (const row of channelResult.rows) {
      stats.byChannel[row.channel] = parseInt(row.count, 10);
    }

    // Calculate conversion rate
    const actionableInvitations = stats.sent + stats.delivered;
    if (actionableInvitations > 0) {
      stats.conversionRate = (stats.accepted / actionableInvitations) * 100;
    }

    return stats;
  }
}
