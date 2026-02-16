/**
 * Support Database Operations
 * Complete CRUD for tickets, teams, SLA policies, canned responses, knowledge base, analytics
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  SupportTicketRecord, CreateTicketRequest, UpdateTicketRequest, TicketListOptions,
  SupportTeamRecord, CreateTeamRequest, UpdateTeamRequest,
  SupportTeamMemberRecord, CreateTeamMemberRequest, UpdateTeamMemberRequest,
  SlaPolicyRecord, CreateSlaPolicyRequest, UpdateSlaPolicyRequest,
  CannedResponseRecord, CreateCannedResponseRequest,
  KbArticleRecord, CreateKbArticleRequest, UpdateKbArticleRequest,
  TicketMessageRecord, CreateTicketMessageRequest,
  SupportAnalytics, AgentPerformance, SupportStats,
  TicketPriority,
} from './types.js';

const logger = createLogger('support:db');

export class SupportDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): SupportDatabase {
    return new SupportDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string { return this.sourceAccountId; }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '');
    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> { await this.db.connect(); }
  async disconnect(): Promise<void> { await this.db.disconnect(); }
  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> { return this.db.query<T>(sql, params); }
  async execute(sql: string, params?: unknown[]): Promise<number> { return this.db.execute(sql, params); }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing support schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- SLA Policies (must come before tickets and teams that reference it)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_sla_policies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(100) NOT NULL,
        description TEXT,
        urgent_first_response_minutes INTEGER DEFAULT 15,
        urgent_resolution_minutes INTEGER DEFAULT 240,
        high_first_response_minutes INTEGER DEFAULT 60,
        high_resolution_minutes INTEGER DEFAULT 480,
        medium_first_response_minutes INTEGER DEFAULT 240,
        medium_resolution_minutes INTEGER DEFAULT 1440,
        low_first_response_minutes INTEGER DEFAULT 480,
        low_resolution_minutes INTEGER DEFAULT 2880,
        applies_during_business_hours_only BOOLEAN NOT NULL DEFAULT true,
        business_hours JSONB,
        timezone VARCHAR(50) DEFAULT 'UTC',
        escalation_enabled BOOLEAN NOT NULL DEFAULT true,
        escalation_threshold_minutes INTEGER DEFAULT 30,
        escalate_to_team_id UUID,
        is_active BOOLEAN NOT NULL DEFAULT true,
        is_default BOOLEAN NOT NULL DEFAULT false,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_sla_policies_account ON nchat_support_sla_policies(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_sla_policies_active ON nchat_support_sla_policies(is_active) WHERE is_active = true;
      CREATE INDEX IF NOT EXISTS idx_sla_policies_default ON nchat_support_sla_policies(is_default) WHERE is_default = true;

      -- =====================================================================
      -- Support Teams
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_teams (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(100) NOT NULL,
        description TEXT,
        email VARCHAR(255),
        is_active BOOLEAN NOT NULL DEFAULT true,
        business_hours JSONB,
        timezone VARCHAR(50) DEFAULT 'UTC',
        auto_assignment_enabled BOOLEAN NOT NULL DEFAULT true,
        assignment_method VARCHAR(50) DEFAULT 'round_robin',
        default_sla_policy_id UUID REFERENCES nchat_support_sla_policies(id) ON DELETE SET NULL,
        open_tickets_count INTEGER DEFAULT 0,
        member_count INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
      CREATE INDEX IF NOT EXISTS idx_support_teams_account ON nchat_support_teams(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_support_teams_active ON nchat_support_teams(is_active) WHERE is_active = true;

      -- =====================================================================
      -- Support Team Members
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_team_members (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        team_id UUID NOT NULL REFERENCES nchat_support_teams(id) ON DELETE CASCADE,
        user_id UUID NOT NULL,
        role VARCHAR(50) NOT NULL DEFAULT 'agent',
        skills TEXT[] DEFAULT '{}',
        skill_level INTEGER DEFAULT 1 CHECK (skill_level >= 1 AND skill_level <= 5),
        max_concurrent_tickets INTEGER DEFAULT 10,
        current_ticket_count INTEGER DEFAULT 0,
        is_active BOOLEAN NOT NULL DEFAULT true,
        is_available BOOLEAN NOT NULL DEFAULT true,
        availability_status VARCHAR(50) DEFAULT 'available',
        total_tickets_handled INTEGER DEFAULT 0,
        avg_first_response_time_seconds INTEGER,
        avg_resolution_time_seconds INTEGER,
        customer_satisfaction_avg DECIMAL(3,2),
        joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, team_id, user_id)
      );
      CREATE INDEX IF NOT EXISTS idx_support_team_members_account ON nchat_support_team_members(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_support_team_members_team ON nchat_support_team_members(team_id);
      CREATE INDEX IF NOT EXISTS idx_support_team_members_user ON nchat_support_team_members(user_id);
      CREATE INDEX IF NOT EXISTS idx_support_team_members_active ON nchat_support_team_members(is_active) WHERE is_active = true;
      CREATE INDEX IF NOT EXISTS idx_support_team_members_available ON nchat_support_team_members(is_available) WHERE is_available = true;

      -- =====================================================================
      -- Support Tickets
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_tickets (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        ticket_number VARCHAR(50) NOT NULL,
        customer_id UUID,
        customer_name VARCHAR(255),
        customer_email VARCHAR(255),
        customer_phone VARCHAR(50),
        subject VARCHAR(500) NOT NULL,
        description TEXT NOT NULL,
        status VARCHAR(50) NOT NULL DEFAULT 'new',
        priority VARCHAR(50) NOT NULL DEFAULT 'medium',
        assigned_to UUID,
        assigned_at TIMESTAMPTZ,
        team_id UUID REFERENCES nchat_support_teams(id) ON DELETE SET NULL,
        channel_id UUID,
        source VARCHAR(50) NOT NULL DEFAULT 'chat',
        category VARCHAR(100),
        tags TEXT[] DEFAULT '{}',
        sla_policy_id UUID REFERENCES nchat_support_sla_policies(id) ON DELETE SET NULL,
        first_response_due_at TIMESTAMPTZ,
        first_response_at TIMESTAMPTZ,
        resolution_due_at TIMESTAMPTZ,
        resolved_at TIMESTAMPTZ,
        first_response_breached BOOLEAN NOT NULL DEFAULT false,
        resolution_breached BOOLEAN NOT NULL DEFAULT false,
        satisfaction_rating INTEGER,
        satisfaction_comment TEXT,
        satisfaction_submitted_at TIMESTAMPTZ,
        custom_fields JSONB DEFAULT '{}'::jsonb,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        closed_at TIMESTAMPTZ,
        UNIQUE(source_account_id, ticket_number)
      );
      CREATE INDEX IF NOT EXISTS idx_support_tickets_account ON nchat_support_tickets(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_number ON nchat_support_tickets(ticket_number);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_customer ON nchat_support_tickets(customer_id);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_assigned ON nchat_support_tickets(assigned_to);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_team ON nchat_support_tickets(team_id);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_status ON nchat_support_tickets(status);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_priority ON nchat_support_tickets(priority);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_created ON nchat_support_tickets(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_tags ON nchat_support_tickets USING GIN(tags);
      CREATE INDEX IF NOT EXISTS idx_support_tickets_sla_breach ON nchat_support_tickets(first_response_breached, resolution_breached);

      -- =====================================================================
      -- Ticket Messages
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_ticket_messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        ticket_id UUID NOT NULL REFERENCES nchat_support_tickets(id) ON DELETE CASCADE,
        user_id UUID,
        content TEXT NOT NULL,
        is_internal BOOLEAN NOT NULL DEFAULT false,
        is_system BOOLEAN NOT NULL DEFAULT false,
        attachments JSONB DEFAULT '[]'::jsonb,
        email_message_id VARCHAR(255),
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_ticket_messages_account ON nchat_support_ticket_messages(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket ON nchat_support_ticket_messages(ticket_id);
      CREATE INDEX IF NOT EXISTS idx_ticket_messages_user ON nchat_support_ticket_messages(user_id);
      CREATE INDEX IF NOT EXISTS idx_ticket_messages_created ON nchat_support_ticket_messages(created_at DESC);

      -- =====================================================================
      -- Ticket Events (Audit Trail)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_ticket_events (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        ticket_id UUID NOT NULL REFERENCES nchat_support_tickets(id) ON DELETE CASCADE,
        user_id UUID,
        event_type VARCHAR(100) NOT NULL,
        field_name VARCHAR(100),
        old_value TEXT,
        new_value TEXT,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_ticket_events_account ON nchat_support_ticket_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_ticket_events_ticket ON nchat_support_ticket_events(ticket_id);
      CREATE INDEX IF NOT EXISTS idx_ticket_events_type ON nchat_support_ticket_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_ticket_events_created ON nchat_support_ticket_events(created_at DESC);

      -- =====================================================================
      -- Canned Responses
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_canned_responses (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        title VARCHAR(200) NOT NULL,
        shortcut VARCHAR(50),
        content TEXT NOT NULL,
        category VARCHAR(100),
        tags TEXT[] DEFAULT '{}',
        visibility VARCHAR(50) NOT NULL DEFAULT 'team',
        team_id UUID REFERENCES nchat_support_teams(id) ON DELETE CASCADE,
        created_by UUID NOT NULL,
        attachments JSONB DEFAULT '[]'::jsonb,
        usage_count INTEGER DEFAULT 0,
        last_used_at TIMESTAMPTZ,
        is_active BOOLEAN NOT NULL DEFAULT true,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(source_account_id, shortcut)
      );
      CREATE INDEX IF NOT EXISTS idx_canned_responses_account ON nchat_support_canned_responses(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_canned_responses_shortcut ON nchat_support_canned_responses(shortcut);
      CREATE INDEX IF NOT EXISTS idx_canned_responses_category ON nchat_support_canned_responses(category);
      CREATE INDEX IF NOT EXISTS idx_canned_responses_team ON nchat_support_canned_responses(team_id);
      CREATE INDEX IF NOT EXISTS idx_canned_responses_active ON nchat_support_canned_responses(is_active) WHERE is_active = true;

      -- =====================================================================
      -- Knowledge Base Articles
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_kb_articles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        title VARCHAR(500) NOT NULL,
        slug VARCHAR(500) NOT NULL,
        content TEXT NOT NULL,
        summary TEXT,
        author_id UUID NOT NULL,
        category VARCHAR(100),
        tags TEXT[] DEFAULT '{}',
        is_published BOOLEAN NOT NULL DEFAULT false,
        is_public BOOLEAN NOT NULL DEFAULT true,
        meta_title VARCHAR(200),
        meta_description VARCHAR(500),
        attachments JSONB DEFAULT '[]'::jsonb,
        related_articles UUID[] DEFAULT '{}',
        view_count INTEGER DEFAULT 0,
        helpful_count INTEGER DEFAULT 0,
        not_helpful_count INTEGER DEFAULT 0,
        version INTEGER DEFAULT 1,
        previous_version_id UUID,
        metadata JSONB DEFAULT '{}'::jsonb,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        published_at TIMESTAMPTZ,
        UNIQUE(source_account_id, slug)
      );
      CREATE INDEX IF NOT EXISTS idx_kb_articles_account ON nchat_support_kb_articles(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_articles_slug ON nchat_support_kb_articles(slug);
      CREATE INDEX IF NOT EXISTS idx_kb_articles_author ON nchat_support_kb_articles(author_id);
      CREATE INDEX IF NOT EXISTS idx_kb_articles_category ON nchat_support_kb_articles(category);
      CREATE INDEX IF NOT EXISTS idx_kb_articles_tags ON nchat_support_kb_articles USING GIN(tags);
      CREATE INDEX IF NOT EXISTS idx_kb_articles_published ON nchat_support_kb_articles(is_published) WHERE is_published = true;

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS nchat_support_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_support_webhook_events_account ON nchat_support_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_support_webhook_events_type ON nchat_support_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_support_webhook_events_processed ON nchat_support_webhook_events(processed);

      -- =====================================================================
      -- Ticket number sequence
      -- =====================================================================

      CREATE SEQUENCE IF NOT EXISTS nchat_ticket_number_seq START 1;
    `;

    await this.db.execute(schema);
    logger.info('Support schema initialized successfully');
  }

  // =========================================================================
  // Ticket CRUD
  // =========================================================================

  private async generateTicketNumber(): Promise<string> {
    const result = await this.query<{ nextval: string }>("SELECT nextval('nchat_ticket_number_seq')");
    const num = parseInt(result.rows[0]?.nextval ?? '1', 10);
    return `TKT-${String(num).padStart(5, '0')}`;
  }

  private getSlaMinutes(policy: SlaPolicyRecord, priority: TicketPriority, type: 'first_response' | 'resolution'): number {
    const key = `${priority}_${type}_minutes` as keyof SlaPolicyRecord;
    return policy[key] as number;
  }

  async createTicket(request: CreateTicketRequest): Promise<SupportTicketRecord> {
    const ticketNumber = await this.generateTicketNumber();

    // Get SLA policy
    let slaPolicyId = request.slaPolicyId;
    let firstResponseDueAt: Date | null = null;
    let resolutionDueAt: Date | null = null;

    if (!slaPolicyId) {
      // Get default SLA policy
      const defaultPolicy = await this.query<Record<string, unknown>>(
        'SELECT id FROM nchat_support_sla_policies WHERE source_account_id = $1 AND is_default = true AND is_active = true LIMIT 1',
        [this.sourceAccountId]
      );
      if (defaultPolicy.rows.length > 0) {
        slaPolicyId = (defaultPolicy.rows[0] as unknown as { id: string }).id;
      }
    }

    if (slaPolicyId) {
      const policy = await this.getSlaPolicyById(slaPolicyId);
      if (policy) {
        const priority = request.priority ?? 'medium';
        const firstResponseMinutes = this.getSlaMinutes(policy, priority, 'first_response');
        const resolutionMinutes = this.getSlaMinutes(policy, priority, 'resolution');
        firstResponseDueAt = new Date(Date.now() + firstResponseMinutes * 60 * 1000);
        resolutionDueAt = new Date(Date.now() + resolutionMinutes * 60 * 1000);
      }
    }

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_tickets (
        source_account_id, ticket_number, customer_id, customer_name, customer_email,
        customer_phone, subject, description, status, priority,
        assigned_to, team_id, channel_id, source, category, tags,
        sla_policy_id, first_response_due_at, resolution_due_at,
        custom_fields, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'new',$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
      RETURNING *`,
      [
        this.sourceAccountId, ticketNumber, request.customerId ?? null,
        request.customerName ?? null, request.customerEmail ?? null,
        request.customerPhone ?? null, request.subject, request.description,
        request.priority ?? 'medium', request.assignedTo ?? null,
        request.teamId ?? null, request.channelId ?? null,
        request.source ?? 'chat', request.category ?? null,
        request.tags ?? [], slaPolicyId ?? null,
        firstResponseDueAt?.toISOString() ?? null, resolutionDueAt?.toISOString() ?? null,
        JSON.stringify(request.customFields ?? {}), JSON.stringify(request.metadata ?? {}),
      ]
    );

    const ticket = result.rows[0] as unknown as SupportTicketRecord;

    // Create audit event
    await this.createTicketEvent(ticket.id, null, 'created', null, null, null);

    // If assigned, create assignment event
    if (request.assignedTo) {
      await this.execute(
        'UPDATE nchat_support_tickets SET assigned_at = NOW() WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, ticket.id]
      );
      await this.createTicketEvent(ticket.id, null, 'assigned', 'assigned_to', null, request.assignedTo);
    }

    // Update team open ticket count
    if (request.teamId) {
      await this.execute(
        'UPDATE nchat_support_teams SET open_tickets_count = open_tickets_count + 1 WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, request.teamId]
      );
    }

    return ticket;
  }

  async getTicket(ticketId: string): Promise<SupportTicketRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_tickets WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, ticketId]
    );
    return (result.rows[0] ?? null) as unknown as SupportTicketRecord | null;
  }

  async getTicketByNumber(ticketNumber: string): Promise<SupportTicketRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_tickets WHERE source_account_id = $1 AND ticket_number = $2',
      [this.sourceAccountId, ticketNumber]
    );
    return (result.rows[0] ?? null) as unknown as SupportTicketRecord | null;
  }

  async listTickets(options: TicketListOptions = {}): Promise<SupportTicketRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }
    if (options.priority) { conditions.push(`priority = $${paramIndex++}`); params.push(options.priority); }
    if (options.assignedTo) { conditions.push(`assigned_to = $${paramIndex++}`); params.push(options.assignedTo); }
    if (options.teamId) { conditions.push(`team_id = $${paramIndex++}`); params.push(options.teamId); }
    if (options.customerId) { conditions.push(`customer_id = $${paramIndex++}`); params.push(options.customerId); }
    if (options.search) {
      conditions.push(`(subject ILIKE $${paramIndex} OR description ILIKE $${paramIndex})`);
      params.push(`%${options.search}%`);
      paramIndex++;
    }
    if (options.tags && options.tags.length > 0) {
      conditions.push(`tags && $${paramIndex++}`);
      params.push(options.tags);
    }

    const sortField = options.sort ?? 'created_at';
    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_support_tickets WHERE ${conditions.join(' AND ')}
       ORDER BY ${sortField} DESC LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as SupportTicketRecord[];
  }

  async updateTicket(ticketId: string, updates: UpdateTicketRequest, userId?: string): Promise<SupportTicketRecord | null> {
    // Get current ticket for audit
    const current = await this.getTicket(ticketId);
    if (!current) return null;

    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, ticketId];
    let paramIndex = 3;

    if (updates.subject !== undefined) { sets.push(`subject = $${paramIndex++}`); params.push(updates.subject); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.status !== undefined) {
      sets.push(`status = $${paramIndex++}`); params.push(updates.status);
      if (updates.status === 'resolved') sets.push('resolved_at = NOW()');
      if (updates.status === 'closed') sets.push('closed_at = NOW()');
      await this.createTicketEvent(ticketId, userId ?? null, 'status_changed', 'status', current.status, updates.status);
    }
    if (updates.priority !== undefined) {
      sets.push(`priority = $${paramIndex++}`); params.push(updates.priority);
      await this.createTicketEvent(ticketId, userId ?? null, 'priority_changed', 'priority', current.priority, updates.priority);
    }
    if (updates.assignedTo !== undefined) {
      sets.push(`assigned_to = $${paramIndex++}`); params.push(updates.assignedTo);
      sets.push('assigned_at = NOW()');
      await this.createTicketEvent(ticketId, userId ?? null, 'assigned', 'assigned_to', current.assigned_to, updates.assignedTo);
    }
    if (updates.teamId !== undefined) { sets.push(`team_id = $${paramIndex++}`); params.push(updates.teamId); }
    if (updates.category !== undefined) { sets.push(`category = $${paramIndex++}`); params.push(updates.category); }
    if (updates.tags !== undefined) { sets.push(`tags = $${paramIndex++}`); params.push(updates.tags); }
    if (updates.slaPolicyId !== undefined) { sets.push(`sla_policy_id = $${paramIndex++}`); params.push(updates.slaPolicyId); }
    if (updates.customFields !== undefined) { sets.push(`custom_fields = $${paramIndex++}`); params.push(JSON.stringify(updates.customFields)); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return current;
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_tickets SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as SupportTicketRecord | null;
  }

  async submitSatisfaction(ticketId: string, rating: number, comment?: string): Promise<SupportTicketRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_tickets SET satisfaction_rating = $3, satisfaction_comment = $4,
       satisfaction_submitted_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      [this.sourceAccountId, ticketId, rating, comment ?? null]
    );
    return (result.rows[0] ?? null) as unknown as SupportTicketRecord | null;
  }

  // =========================================================================
  // Ticket Messages
  // =========================================================================

  async createTicketMessage(request: CreateTicketMessageRequest): Promise<TicketMessageRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_ticket_messages (
        source_account_id, ticket_id, user_id, content, is_internal,
        is_system, attachments, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *`,
      [
        this.sourceAccountId, request.ticketId, request.userId ?? null,
        request.content, request.isInternal ?? false, request.isSystem ?? false,
        JSON.stringify(request.attachments ?? []), JSON.stringify(request.metadata ?? {}),
      ]
    );

    // Check if this is first response and update SLA tracking
    if (!request.isInternal && !request.isSystem && request.userId) {
      const ticket = await this.getTicket(request.ticketId);
      if (ticket && !ticket.first_response_at) {
        const breached = ticket.first_response_due_at ? new Date() > new Date(ticket.first_response_due_at) : false;
        await this.execute(
          `UPDATE nchat_support_tickets SET first_response_at = NOW(), first_response_breached = $3,
           status = CASE WHEN status = 'new' THEN 'open' ELSE status END,
           updated_at = NOW()
           WHERE source_account_id = $1 AND id = $2`,
          [this.sourceAccountId, request.ticketId, breached]
        );
      }
    }

    // Update ticket updated_at
    await this.execute(
      'UPDATE nchat_support_tickets SET updated_at = NOW() WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, request.ticketId]
    );

    return result.rows[0] as unknown as TicketMessageRecord;
  }

  async listTicketMessages(ticketId: string, includeInternal = true): Promise<TicketMessageRecord[]> {
    const conditions = ['source_account_id = $1', 'ticket_id = $2'];
    if (!includeInternal) conditions.push('is_internal = false');

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_support_ticket_messages WHERE ${conditions.join(' AND ')} ORDER BY created_at ASC`,
      [this.sourceAccountId, ticketId]
    );
    return result.rows as unknown as TicketMessageRecord[];
  }

  // =========================================================================
  // Ticket Events
  // =========================================================================

  async createTicketEvent(ticketId: string, userId: string | null, eventType: string, fieldName: string | null, oldValue: string | null, newValue: string | null): Promise<void> {
    await this.execute(
      `INSERT INTO nchat_support_ticket_events (source_account_id, ticket_id, user_id, event_type, field_name, old_value, new_value)
       VALUES ($1,$2,$3,$4,$5,$6,$7)`,
      [this.sourceAccountId, ticketId, userId, eventType, fieldName, oldValue, newValue]
    );
  }

  // =========================================================================
  // Team CRUD
  // =========================================================================

  async createTeam(request: CreateTeamRequest): Promise<SupportTeamRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_teams (
        source_account_id, name, description, email, timezone,
        auto_assignment_enabled, assignment_method, default_sla_policy_id,
        business_hours, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *`,
      [
        this.sourceAccountId, request.name, request.description ?? null,
        request.email ?? null, request.timezone ?? 'UTC',
        request.autoAssignmentEnabled ?? true, request.assignmentMethod ?? 'round_robin',
        request.defaultSlaPolicyId ?? null,
        request.businessHours ? JSON.stringify(request.businessHours) : null,
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as SupportTeamRecord;
  }

  async getTeam(teamId: string): Promise<SupportTeamRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_teams WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, teamId]
    );
    return (result.rows[0] ?? null) as unknown as SupportTeamRecord | null;
  }

  async listTeams(): Promise<SupportTeamRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_teams WHERE source_account_id = $1 ORDER BY name',
      [this.sourceAccountId]
    );
    return result.rows as unknown as SupportTeamRecord[];
  }

  async updateTeam(teamId: string, updates: UpdateTeamRequest): Promise<SupportTeamRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, teamId];
    let paramIndex = 3;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.email !== undefined) { sets.push(`email = $${paramIndex++}`); params.push(updates.email); }
    if (updates.isActive !== undefined) { sets.push(`is_active = $${paramIndex++}`); params.push(updates.isActive); }
    if (updates.timezone !== undefined) { sets.push(`timezone = $${paramIndex++}`); params.push(updates.timezone); }
    if (updates.autoAssignmentEnabled !== undefined) { sets.push(`auto_assignment_enabled = $${paramIndex++}`); params.push(updates.autoAssignmentEnabled); }
    if (updates.assignmentMethod !== undefined) { sets.push(`assignment_method = $${paramIndex++}`); params.push(updates.assignmentMethod); }
    if (updates.defaultSlaPolicyId !== undefined) { sets.push(`default_sla_policy_id = $${paramIndex++}`); params.push(updates.defaultSlaPolicyId); }
    if (updates.businessHours !== undefined) { sets.push(`business_hours = $${paramIndex++}`); params.push(JSON.stringify(updates.businessHours)); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getTeam(teamId);
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_teams SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as SupportTeamRecord | null;
  }

  // =========================================================================
  // Team Members
  // =========================================================================

  async addTeamMember(request: CreateTeamMemberRequest): Promise<SupportTeamMemberRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_team_members (
        source_account_id, team_id, user_id, role, skills,
        skill_level, max_concurrent_tickets
      ) VALUES ($1,$2,$3,$4,$5,$6,$7)
      ON CONFLICT (source_account_id, team_id, user_id) DO UPDATE SET
        role = EXCLUDED.role, is_active = true, updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId, request.teamId, request.userId,
        request.role ?? 'agent', request.skills ?? [],
        request.skillLevel ?? 1, request.maxConcurrentTickets ?? 10,
      ]
    );

    await this.execute(
      'UPDATE nchat_support_teams SET member_count = (SELECT COUNT(*) FROM nchat_support_team_members WHERE source_account_id = $1 AND team_id = $2 AND is_active = true) WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, request.teamId]
    );

    return result.rows[0] as unknown as SupportTeamMemberRecord;
  }

  async listTeamMembers(teamId: string): Promise<SupportTeamMemberRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_team_members WHERE source_account_id = $1 AND team_id = $2 AND is_active = true ORDER BY role, joined_at',
      [this.sourceAccountId, teamId]
    );
    return result.rows as unknown as SupportTeamMemberRecord[];
  }

  async updateTeamMember(memberId: string, updates: UpdateTeamMemberRequest): Promise<SupportTeamMemberRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, memberId];
    let paramIndex = 3;

    if (updates.role !== undefined) { sets.push(`role = $${paramIndex++}`); params.push(updates.role); }
    if (updates.skills !== undefined) { sets.push(`skills = $${paramIndex++}`); params.push(updates.skills); }
    if (updates.skillLevel !== undefined) { sets.push(`skill_level = $${paramIndex++}`); params.push(updates.skillLevel); }
    if (updates.maxConcurrentTickets !== undefined) { sets.push(`max_concurrent_tickets = $${paramIndex++}`); params.push(updates.maxConcurrentTickets); }
    if (updates.isActive !== undefined) { sets.push(`is_active = $${paramIndex++}`); params.push(updates.isActive); }
    if (updates.isAvailable !== undefined) { sets.push(`is_available = $${paramIndex++}`); params.push(updates.isAvailable); }
    if (updates.availabilityStatus !== undefined) { sets.push(`availability_status = $${paramIndex++}`); params.push(updates.availabilityStatus); }

    if (sets.length === 0) return null;
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_team_members SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as SupportTeamMemberRecord | null;
  }

  // =========================================================================
  // SLA Policies
  // =========================================================================

  async createSlaPolicy(request: CreateSlaPolicyRequest): Promise<SlaPolicyRecord> {
    // If setting as default, unset current default
    if (request.isDefault) {
      await this.execute(
        'UPDATE nchat_support_sla_policies SET is_default = false WHERE source_account_id = $1 AND is_default = true',
        [this.sourceAccountId]
      );
    }

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_sla_policies (
        source_account_id, name, description,
        urgent_first_response_minutes, urgent_resolution_minutes,
        high_first_response_minutes, high_resolution_minutes,
        medium_first_response_minutes, medium_resolution_minutes,
        low_first_response_minutes, low_resolution_minutes,
        applies_during_business_hours_only, business_hours, timezone,
        escalation_enabled, escalation_threshold_minutes, escalate_to_team_id,
        is_default, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING *`,
      [
        this.sourceAccountId, request.name, request.description ?? null,
        request.urgentFirstResponseMinutes ?? 15, request.urgentResolutionMinutes ?? 240,
        request.highFirstResponseMinutes ?? 60, request.highResolutionMinutes ?? 480,
        request.mediumFirstResponseMinutes ?? 240, request.mediumResolutionMinutes ?? 1440,
        request.lowFirstResponseMinutes ?? 480, request.lowResolutionMinutes ?? 2880,
        request.appliesDuringBusinessHoursOnly ?? true,
        request.businessHours ? JSON.stringify(request.businessHours) : null,
        request.timezone ?? 'UTC', request.escalationEnabled ?? true,
        request.escalationThresholdMinutes ?? 30, request.escalateToTeamId ?? null,
        request.isDefault ?? false, JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as SlaPolicyRecord;
  }

  async getSlaPolicyById(policyId: string): Promise<SlaPolicyRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_sla_policies WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, policyId]
    );
    return (result.rows[0] ?? null) as unknown as SlaPolicyRecord | null;
  }

  async listSlaPolicies(): Promise<SlaPolicyRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_sla_policies WHERE source_account_id = $1 ORDER BY name',
      [this.sourceAccountId]
    );
    return result.rows as unknown as SlaPolicyRecord[];
  }

  async updateSlaPolicy(policyId: string, updates: UpdateSlaPolicyRequest): Promise<SlaPolicyRecord | null> {
    if (updates.isDefault) {
      await this.execute(
        'UPDATE nchat_support_sla_policies SET is_default = false WHERE source_account_id = $1 AND is_default = true',
        [this.sourceAccountId]
      );
    }

    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, policyId];
    let paramIndex = 3;

    if (updates.name !== undefined) { sets.push(`name = $${paramIndex++}`); params.push(updates.name); }
    if (updates.description !== undefined) { sets.push(`description = $${paramIndex++}`); params.push(updates.description); }
    if (updates.urgentFirstResponseMinutes !== undefined) { sets.push(`urgent_first_response_minutes = $${paramIndex++}`); params.push(updates.urgentFirstResponseMinutes); }
    if (updates.urgentResolutionMinutes !== undefined) { sets.push(`urgent_resolution_minutes = $${paramIndex++}`); params.push(updates.urgentResolutionMinutes); }
    if (updates.highFirstResponseMinutes !== undefined) { sets.push(`high_first_response_minutes = $${paramIndex++}`); params.push(updates.highFirstResponseMinutes); }
    if (updates.highResolutionMinutes !== undefined) { sets.push(`high_resolution_minutes = $${paramIndex++}`); params.push(updates.highResolutionMinutes); }
    if (updates.mediumFirstResponseMinutes !== undefined) { sets.push(`medium_first_response_minutes = $${paramIndex++}`); params.push(updates.mediumFirstResponseMinutes); }
    if (updates.mediumResolutionMinutes !== undefined) { sets.push(`medium_resolution_minutes = $${paramIndex++}`); params.push(updates.mediumResolutionMinutes); }
    if (updates.lowFirstResponseMinutes !== undefined) { sets.push(`low_first_response_minutes = $${paramIndex++}`); params.push(updates.lowFirstResponseMinutes); }
    if (updates.lowResolutionMinutes !== undefined) { sets.push(`low_resolution_minutes = $${paramIndex++}`); params.push(updates.lowResolutionMinutes); }
    if (updates.isActive !== undefined) { sets.push(`is_active = $${paramIndex++}`); params.push(updates.isActive); }
    if (updates.isDefault !== undefined) { sets.push(`is_default = $${paramIndex++}`); params.push(updates.isDefault); }
    if (updates.escalationEnabled !== undefined) { sets.push(`escalation_enabled = $${paramIndex++}`); params.push(updates.escalationEnabled); }
    if (updates.escalationThresholdMinutes !== undefined) { sets.push(`escalation_threshold_minutes = $${paramIndex++}`); params.push(updates.escalationThresholdMinutes); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getSlaPolicyById(policyId);
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_sla_policies SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as SlaPolicyRecord | null;
  }

  // =========================================================================
  // Canned Responses
  // =========================================================================

  async createCannedResponse(request: CreateCannedResponseRequest): Promise<CannedResponseRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_canned_responses (
        source_account_id, title, shortcut, content, category, tags,
        visibility, team_id, created_by, attachments, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *`,
      [
        this.sourceAccountId, request.title, request.shortcut ?? null,
        request.content, request.category ?? null, request.tags ?? [],
        request.visibility ?? 'team', request.teamId ?? null, request.createdBy,
        JSON.stringify(request.attachments ?? []), JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as CannedResponseRecord;
  }

  async listCannedResponses(options: { category?: string; search?: string; teamId?: string } = {}): Promise<CannedResponseRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'is_active = true'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.category) { conditions.push(`category = $${paramIndex++}`); params.push(options.category); }
    if (options.teamId) { conditions.push(`(team_id = $${paramIndex} OR team_id IS NULL)`); params.push(options.teamId); paramIndex++; }
    if (options.search) {
      conditions.push(`(title ILIKE $${paramIndex} OR shortcut ILIKE $${paramIndex} OR content ILIKE $${paramIndex})`);
      params.push(`%${options.search}%`);
      paramIndex++;
    }

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_support_canned_responses WHERE ${conditions.join(' AND ')} ORDER BY usage_count DESC, title`,
      params
    );
    return result.rows as unknown as CannedResponseRecord[];
  }

  async useCannedResponse(responseId: string): Promise<CannedResponseRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_canned_responses SET usage_count = usage_count + 1, last_used_at = NOW()
       WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      [this.sourceAccountId, responseId]
    );
    return (result.rows[0] ?? null) as unknown as CannedResponseRecord | null;
  }

  // =========================================================================
  // Knowledge Base Articles
  // =========================================================================

  async createKbArticle(request: CreateKbArticleRequest): Promise<KbArticleRecord> {
    const slug = request.slug ?? request.title.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO nchat_support_kb_articles (
        source_account_id, title, slug, content, summary, author_id,
        category, tags, is_public, meta_title, meta_description,
        attachments, related_articles, metadata
      ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *`,
      [
        this.sourceAccountId, request.title, slug, request.content,
        request.summary ?? null, request.authorId,
        request.category ?? null, request.tags ?? [],
        request.isPublic ?? true, request.metaTitle ?? null,
        request.metaDescription ?? null,
        JSON.stringify(request.attachments ?? []),
        request.relatedArticles ?? [],
        JSON.stringify(request.metadata ?? {}),
      ]
    );
    return result.rows[0] as unknown as KbArticleRecord;
  }

  async getKbArticle(articleId: string): Promise<KbArticleRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_kb_articles WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, articleId]
    );
    return (result.rows[0] ?? null) as unknown as KbArticleRecord | null;
  }

  async getKbArticleBySlug(slug: string): Promise<KbArticleRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM nchat_support_kb_articles WHERE source_account_id = $1 AND slug = $2',
      [this.sourceAccountId, slug]
    );
    return (result.rows[0] ?? null) as unknown as KbArticleRecord | null;
  }

  async listKbArticles(options: { category?: string; search?: string; published?: boolean; isPublic?: boolean; limit?: number; offset?: number } = {}): Promise<KbArticleRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.category) { conditions.push(`category = $${paramIndex++}`); params.push(options.category); }
    if (options.published !== undefined) { conditions.push(`is_published = $${paramIndex++}`); params.push(options.published); }
    if (options.isPublic !== undefined) { conditions.push(`is_public = $${paramIndex++}`); params.push(options.isPublic); }
    if (options.search) {
      conditions.push(`(title ILIKE $${paramIndex} OR content ILIKE $${paramIndex} OR summary ILIKE $${paramIndex})`);
      params.push(`%${options.search}%`);
      paramIndex++;
    }

    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM nchat_support_kb_articles WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      [...params, limit, offset]
    );
    return result.rows as unknown as KbArticleRecord[];
  }

  async updateKbArticle(articleId: string, updates: UpdateKbArticleRequest): Promise<KbArticleRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, articleId];
    let paramIndex = 3;

    if (updates.title !== undefined) { sets.push(`title = $${paramIndex++}`); params.push(updates.title); }
    if (updates.content !== undefined) { sets.push(`content = $${paramIndex++}`); params.push(updates.content); sets.push('version = version + 1'); }
    if (updates.summary !== undefined) { sets.push(`summary = $${paramIndex++}`); params.push(updates.summary); }
    if (updates.category !== undefined) { sets.push(`category = $${paramIndex++}`); params.push(updates.category); }
    if (updates.tags !== undefined) { sets.push(`tags = $${paramIndex++}`); params.push(updates.tags); }
    if (updates.isPublished !== undefined) {
      sets.push(`is_published = $${paramIndex++}`); params.push(updates.isPublished);
      if (updates.isPublished) sets.push('published_at = COALESCE(published_at, NOW())');
    }
    if (updates.isPublic !== undefined) { sets.push(`is_public = $${paramIndex++}`); params.push(updates.isPublic); }
    if (updates.metaTitle !== undefined) { sets.push(`meta_title = $${paramIndex++}`); params.push(updates.metaTitle); }
    if (updates.metaDescription !== undefined) { sets.push(`meta_description = $${paramIndex++}`); params.push(updates.metaDescription); }
    if (updates.metadata !== undefined) { sets.push(`metadata = $${paramIndex++}`); params.push(JSON.stringify(updates.metadata)); }

    if (sets.length === 0) return this.getKbArticle(articleId);
    sets.push('updated_at = NOW()');

    const result = await this.query<Record<string, unknown>>(
      `UPDATE nchat_support_kb_articles SET ${sets.join(', ')} WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      params
    );
    return (result.rows[0] ?? null) as unknown as KbArticleRecord | null;
  }

  async recordArticleFeedback(articleId: string, helpful: boolean): Promise<void> {
    // Use conditional query to avoid SQL injection from dynamic field names
    if (helpful) {
      await this.execute(
        'UPDATE nchat_support_kb_articles SET helpful_count = helpful_count + 1 WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, articleId]
      );
    } else {
      await this.execute(
        'UPDATE nchat_support_kb_articles SET not_helpful_count = not_helpful_count + 1 WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, articleId]
      );
    }
  }

  async incrementArticleViewCount(articleId: string): Promise<void> {
    await this.execute(
      'UPDATE nchat_support_kb_articles SET view_count = view_count + 1 WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, articleId]
    );
  }

  // =========================================================================
  // Analytics
  // =========================================================================

  async getAnalyticsOverview(): Promise<SupportAnalytics> {
    const ticketStats = await this.query<Record<string, string>>(
      `SELECT
        COUNT(*) FILTER (WHERE status NOT IN ('resolved','closed')) as open_tickets,
        AVG(EXTRACT(EPOCH FROM (first_response_at - created_at))) FILTER (WHERE first_response_at IS NOT NULL) as avg_first_response,
        AVG(EXTRACT(EPOCH FROM (resolved_at - created_at))) FILTER (WHERE resolved_at IS NOT NULL) as avg_resolution,
        (COUNT(*) FILTER (WHERE NOT first_response_breached AND NOT resolution_breached AND status IN ('resolved','closed'))::float /
         NULLIF(COUNT(*) FILTER (WHERE status IN ('resolved','closed')), 0)) as sla_compliance,
        AVG(satisfaction_rating) FILTER (WHERE satisfaction_rating IS NOT NULL) as satisfaction
      FROM nchat_support_tickets WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const byStatus = await this.query<{ status: string; count: string }>(
      'SELECT status, COUNT(*) as count FROM nchat_support_tickets WHERE source_account_id = $1 GROUP BY status',
      [this.sourceAccountId]
    );

    const byPriority = await this.query<{ priority: string; count: string }>(
      'SELECT priority, COUNT(*) as count FROM nchat_support_tickets WHERE source_account_id = $1 GROUP BY priority',
      [this.sourceAccountId]
    );

    const stats = ticketStats.rows[0] ?? {};
    return {
      openTickets: parseInt(stats.open_tickets ?? '0', 10),
      avgFirstResponseTime: parseFloat(stats.avg_first_response ?? '0'),
      avgResolutionTime: parseFloat(stats.avg_resolution ?? '0'),
      slaCompliance: parseFloat(stats.sla_compliance ?? '0'),
      customerSatisfaction: parseFloat(stats.satisfaction ?? '0'),
      ticketsByStatus: Object.fromEntries(byStatus.rows.map(r => [r.status, parseInt(r.count, 10)])),
      ticketsByPriority: Object.fromEntries(byPriority.rows.map(r => [r.priority, parseInt(r.count, 10)])),
    };
  }

  async getAgentPerformance(): Promise<AgentPerformance[]> {
    const result = await this.query<{
      user_id: string;
      tickets_handled: string;
      avg_first_response: string;
      avg_resolution: string;
      satisfaction: string;
      current_tickets: string;
    }>(
      `SELECT
        m.user_id,
        m.total_tickets_handled as tickets_handled,
        m.avg_first_response_time_seconds as avg_first_response,
        m.avg_resolution_time_seconds as avg_resolution,
        m.customer_satisfaction_avg as satisfaction,
        m.current_ticket_count as current_tickets
      FROM nchat_support_team_members m
      WHERE m.source_account_id = $1 AND m.is_active = true
      ORDER BY m.total_tickets_handled DESC`,
      [this.sourceAccountId]
    );

    return result.rows.map(r => ({
      userId: r.user_id,
      ticketsHandled: parseInt(r.tickets_handled ?? '0', 10),
      avgFirstResponseTime: parseFloat(r.avg_first_response ?? '0'),
      avgResolutionTime: parseFloat(r.avg_resolution ?? '0'),
      satisfactionAvg: parseFloat(r.satisfaction ?? '0'),
      currentTickets: parseInt(r.current_tickets ?? '0', 10),
    }));
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<SupportStats> {
    const ticketStats = await this.query<Record<string, string>>(
      `SELECT
        COUNT(*) as total,
        COUNT(*) FILTER (WHERE status IN ('new','open')) as open_count,
        COUNT(*) FILTER (WHERE status = 'pending') as pending_count,
        COUNT(*) FILTER (WHERE status = 'resolved') as resolved_count
      FROM nchat_support_tickets WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const teams = await this.query<{ count: string }>('SELECT COUNT(*) as count FROM nchat_support_teams WHERE source_account_id = $1', [this.sourceAccountId]);
    const agents = await this.query<{ count: string }>('SELECT COUNT(*) as count FROM nchat_support_team_members WHERE source_account_id = $1 AND is_active = true', [this.sourceAccountId]);
    const sla = await this.query<{ count: string }>('SELECT COUNT(*) as count FROM nchat_support_sla_policies WHERE source_account_id = $1', [this.sourceAccountId]);
    const canned = await this.query<{ count: string }>('SELECT COUNT(*) as count FROM nchat_support_canned_responses WHERE source_account_id = $1 AND is_active = true', [this.sourceAccountId]);
    const kb = await this.query<{ total: string; published: string }>(
      `SELECT COUNT(*) as total, COUNT(*) FILTER (WHERE is_published = true) as published
       FROM nchat_support_kb_articles WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const t = ticketStats.rows[0] ?? {};
    return {
      totalTickets: parseInt(t.total ?? '0', 10),
      openTickets: parseInt(t.open_count ?? '0', 10),
      pendingTickets: parseInt(t.pending_count ?? '0', 10),
      resolvedTickets: parseInt(t.resolved_count ?? '0', 10),
      totalTeams: parseInt(teams.rows[0]?.count ?? '0', 10),
      totalAgents: parseInt(agents.rows[0]?.count ?? '0', 10),
      totalSlaPolicies: parseInt(sla.rows[0]?.count ?? '0', 10),
      totalCannedResponses: parseInt(canned.rows[0]?.count ?? '0', 10),
      totalKbArticles: parseInt(kb.rows[0]?.total ?? '0', 10),
      publishedKbArticles: parseInt(kb.rows[0]?.published ?? '0', 10),
    };
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      'INSERT INTO nchat_support_webhook_events (id, source_account_id, event_type, payload) VALUES ($1,$2,$3,$4)',
      [`evt_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      'UPDATE nchat_support_webhook_events SET processed = true, processed_at = NOW(), error = $2 WHERE id = $1',
      [eventId, error ?? null]
    );
  }
}
