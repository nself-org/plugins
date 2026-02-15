/**
 * Compliance Database Operations
 * Complete CRUD for DSARs, consent, retention, breaches, and audit logs
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  DsarRecord, CreateDsarRequest, ProcessDsarRequest,
  DsarActivityRecord,
  ConsentRecord, CreateConsentRequest,
  PrivacyPolicyRecord, CreatePrivacyPolicyRequest, PolicyAcceptanceRecord,
  RetentionPolicyRecord, CreateRetentionPolicyRequest, RetentionExecutionRecord,
  DataProcessorRecord,
  DataBreachRecord, CreateBreachRequest, BreachNotificationRecord,
  ComplianceAuditLogRecord, CreateAuditLogRequest,
  DsarStatus,
} from './types.js';

const logger = createLogger('compliance:db');

export class ComplianceDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ComplianceDatabase {
    return new ComplianceDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing compliance schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- DSARs
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_dsars (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        request_type VARCHAR(50) NOT NULL,
        request_number VARCHAR(50) NOT NULL,
        user_id VARCHAR(255),
        requester_email VARCHAR(255) NOT NULL,
        requester_name VARCHAR(255),
        verification_token VARCHAR(255),
        verification_sent_at TIMESTAMP WITH TIME ZONE,
        verification_completed_at TIMESTAMP WITH TIME ZONE,
        verified_by VARCHAR(255),
        description TEXT,
        data_categories TEXT[] DEFAULT '{}',
        specific_data_requested TEXT,
        status VARCHAR(30) NOT NULL DEFAULT 'pending',
        assigned_to VARCHAR(255),
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        deadline TIMESTAMP WITH TIME ZONE NOT NULL,
        data_package_url TEXT,
        data_package_size_bytes BIGINT,
        data_package_generated_at TIMESTAMP WITH TIME ZONE,
        resolution_notes TEXT,
        rejection_reason TEXT,
        regulation VARCHAR(50) NOT NULL DEFAULT 'GDPR',
        jurisdiction VARCHAR(100),
        legal_basis TEXT,
        ip_address VARCHAR(45),
        user_agent TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, request_number)
      );
      CREATE INDEX IF NOT EXISTS idx_dsars_account ON compliance_dsars(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dsars_user ON compliance_dsars(source_account_id, user_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_dsars_status ON compliance_dsars(source_account_id, status, deadline);
      CREATE INDEX IF NOT EXISTS idx_dsars_assigned ON compliance_dsars(assigned_to) WHERE assigned_to IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_dsars_number ON compliance_dsars(source_account_id, request_number);

      -- =====================================================================
      -- DSAR Activities
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_dsar_activities (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        dsar_id UUID NOT NULL REFERENCES compliance_dsars(id) ON DELETE CASCADE,
        activity_type VARCHAR(100) NOT NULL,
        description TEXT,
        performed_by VARCHAR(255),
        performed_by_name VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_dsar_activities_account ON compliance_dsar_activities(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_dsar_activities_dsar ON compliance_dsar_activities(dsar_id, created_at);

      -- =====================================================================
      -- Consents
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_consents (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        purpose VARCHAR(255) NOT NULL,
        purpose_description TEXT,
        status VARCHAR(20) NOT NULL DEFAULT 'granted',
        granted_at TIMESTAMP WITH TIME ZONE,
        denied_at TIMESTAMP WITH TIME ZONE,
        withdrawn_at TIMESTAMP WITH TIME ZONE,
        expires_at TIMESTAMP WITH TIME ZONE,
        consent_method VARCHAR(100),
        consent_text TEXT,
        privacy_policy_version VARCHAR(50),
        ip_address VARCHAR(45),
        user_agent TEXT,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_consents_account ON compliance_consents(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_consents_user ON compliance_consents(source_account_id, user_id, purpose);
      CREATE INDEX IF NOT EXISTS idx_consents_status ON compliance_consents(source_account_id, status, purpose);
      CREATE INDEX IF NOT EXISTS idx_consents_expires ON compliance_consents(expires_at) WHERE expires_at IS NOT NULL;

      -- =====================================================================
      -- Consent History
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_consent_history (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        consent_id UUID NOT NULL REFERENCES compliance_consents(id) ON DELETE CASCADE,
        previous_status VARCHAR(20),
        new_status VARCHAR(20) NOT NULL,
        change_reason VARCHAR(255),
        changed_by VARCHAR(255),
        ip_address VARCHAR(45),
        user_agent TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_consent_history_account ON compliance_consent_history(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_consent_history_consent ON compliance_consent_history(consent_id, created_at);

      -- =====================================================================
      -- Privacy Policies
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_privacy_policies (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        version VARCHAR(50) NOT NULL,
        version_number INTEGER NOT NULL,
        title VARCHAR(255) NOT NULL,
        content TEXT NOT NULL,
        summary TEXT,
        changes_summary TEXT,
        is_active BOOLEAN NOT NULL DEFAULT false,
        requires_reacceptance BOOLEAN NOT NULL DEFAULT false,
        effective_from TIMESTAMP WITH TIME ZONE NOT NULL,
        effective_until TIMESTAMP WITH TIME ZONE,
        language VARCHAR(10) NOT NULL DEFAULT 'en',
        jurisdiction VARCHAR(100),
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, version)
      );
      CREATE INDEX IF NOT EXISTS idx_privacy_policies_account ON compliance_privacy_policies(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_privacy_policies_active ON compliance_privacy_policies(source_account_id, is_active, effective_from);

      -- =====================================================================
      -- Policy Acceptances
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_policy_acceptances (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        policy_id UUID NOT NULL REFERENCES compliance_privacy_policies(id) ON DELETE CASCADE,
        accepted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        ip_address VARCHAR(45),
        user_agent TEXT,
        metadata JSONB DEFAULT '{}',
        UNIQUE(source_account_id, user_id, policy_id)
      );
      CREATE INDEX IF NOT EXISTS idx_policy_acceptances_account ON compliance_policy_acceptances(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_policy_acceptances_user ON compliance_policy_acceptances(source_account_id, user_id, accepted_at DESC);

      -- =====================================================================
      -- Retention Policies
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_retention_policies (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        data_category VARCHAR(50) NOT NULL,
        table_name VARCHAR(255),
        retention_days INTEGER NOT NULL,
        retention_action VARCHAR(20) NOT NULL DEFAULT 'delete',
        conditions JSONB DEFAULT '{}',
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        priority INTEGER NOT NULL DEFAULT 100,
        legal_basis TEXT,
        regulation VARCHAR(50),
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_retention_policies_account ON compliance_retention_policies(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_retention_policies_enabled ON compliance_retention_policies(source_account_id, is_enabled, priority);
      CREATE INDEX IF NOT EXISTS idx_retention_policies_category ON compliance_retention_policies(source_account_id, data_category);

      -- =====================================================================
      -- Retention Executions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_retention_executions (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        policy_id UUID NOT NULL REFERENCES compliance_retention_policies(id) ON DELETE CASCADE,
        executed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        records_processed INTEGER NOT NULL DEFAULT 0,
        records_deleted INTEGER NOT NULL DEFAULT 0,
        records_anonymized INTEGER NOT NULL DEFAULT 0,
        records_archived INTEGER NOT NULL DEFAULT 0,
        status VARCHAR(50) NOT NULL DEFAULT 'completed',
        error_message TEXT,
        execution_time_ms INTEGER,
        metadata JSONB DEFAULT '{}'
      );
      CREATE INDEX IF NOT EXISTS idx_retention_executions_account ON compliance_retention_executions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_retention_executions_policy ON compliance_retention_executions(policy_id, executed_at DESC);

      -- =====================================================================
      -- Processing Records (GDPR Article 30)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_processing_records (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        activity_name VARCHAR(255) NOT NULL,
        activity_description TEXT,
        processing_purpose TEXT NOT NULL,
        legal_basis VARCHAR(100) NOT NULL,
        data_categories TEXT[] NOT NULL,
        data_subjects TEXT[],
        recipient_categories TEXT[],
        third_party_transfers BOOLEAN NOT NULL DEFAULT false,
        third_party_countries TEXT[],
        safeguards TEXT,
        retention_period VARCHAR(255),
        security_measures TEXT,
        is_active BOOLEAN NOT NULL DEFAULT true,
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_processing_records_account ON compliance_processing_records(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_processing_records_active ON compliance_processing_records(source_account_id, is_active);

      -- =====================================================================
      -- Data Processors
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_data_processors (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        processor_name VARCHAR(255) NOT NULL,
        processor_type VARCHAR(100),
        contact_name VARCHAR(255),
        contact_email VARCHAR(255),
        contact_phone VARCHAR(50),
        country VARCHAR(100),
        is_eu_based BOOLEAN NOT NULL DEFAULT false,
        dpa_signed BOOLEAN NOT NULL DEFAULT false,
        dpa_signed_date DATE,
        dpa_expiry_date DATE,
        dpa_document_url TEXT,
        processing_purposes TEXT[],
        data_categories TEXT[],
        has_privacy_shield BOOLEAN DEFAULT false,
        has_scc BOOLEAN DEFAULT false,
        has_bcr BOOLEAN DEFAULT false,
        security_certifications TEXT[],
        last_security_audit DATE,
        is_active BOOLEAN NOT NULL DEFAULT true,
        notes TEXT,
        metadata JSONB DEFAULT '{}',
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_data_processors_account ON compliance_data_processors(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_data_processors_active ON compliance_data_processors(source_account_id, is_active);

      -- =====================================================================
      -- Data Breaches
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_data_breaches (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        breach_number VARCHAR(50) NOT NULL,
        title VARCHAR(255) NOT NULL,
        description TEXT NOT NULL,
        discovered_at TIMESTAMP WITH TIME ZONE NOT NULL,
        discovered_by VARCHAR(255),
        severity VARCHAR(20) NOT NULL,
        affected_users_count INTEGER,
        data_categories TEXT[] NOT NULL,
        data_description TEXT,
        risk_assessment TEXT,
        mitigation_steps TEXT,
        notification_required BOOLEAN NOT NULL DEFAULT true,
        authority_notified_at TIMESTAMP WITH TIME ZONE,
        users_notified_at TIMESTAMP WITH TIME ZONE,
        notification_deadline TIMESTAMP WITH TIME ZONE,
        resolved_at TIMESTAMP WITH TIME ZONE,
        resolution_summary TEXT,
        root_cause TEXT,
        preventive_measures TEXT,
        status VARCHAR(50) NOT NULL DEFAULT 'investigating',
        assigned_to VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, breach_number)
      );
      CREATE INDEX IF NOT EXISTS idx_data_breaches_account ON compliance_data_breaches(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_data_breaches_status ON compliance_data_breaches(source_account_id, status, discovered_at DESC);
      CREATE INDEX IF NOT EXISTS idx_data_breaches_severity ON compliance_data_breaches(source_account_id, severity);

      -- =====================================================================
      -- Breach Notifications
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_breach_notifications (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        breach_id UUID NOT NULL REFERENCES compliance_data_breaches(id) ON DELETE CASCADE,
        notification_type VARCHAR(50) NOT NULL,
        recipient_type VARCHAR(50) NOT NULL,
        recipient_email VARCHAR(255),
        subject VARCHAR(255),
        message_body TEXT,
        sent_at TIMESTAMP WITH TIME ZONE,
        delivery_status VARCHAR(50),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_breach_notifications_account ON compliance_breach_notifications(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_breach_notifications_breach ON compliance_breach_notifications(breach_id, sent_at);

      -- =====================================================================
      -- Compliance Audit Log
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS compliance_audit_log (
        id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(100) NOT NULL,
        event_category VARCHAR(50) NOT NULL,
        actor_id VARCHAR(255),
        actor_type VARCHAR(50) NOT NULL DEFAULT 'user',
        target_type VARCHAR(50),
        target_id VARCHAR(255),
        accessed_data_categories TEXT[],
        data_subject_id VARCHAR(255),
        details JSONB NOT NULL DEFAULT '{}',
        ip_address VARCHAR(45),
        user_agent TEXT,
        legal_basis VARCHAR(100),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_compliance_audit_account ON compliance_audit_log(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_compliance_audit_event ON compliance_audit_log(source_account_id, event_type, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_compliance_audit_actor ON compliance_audit_log(source_account_id, actor_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_compliance_audit_subject ON compliance_audit_log(source_account_id, data_subject_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_compliance_audit_created ON compliance_audit_log(source_account_id, created_at DESC);
    `;

    await this.db.execute(schema);
    logger.info('Compliance schema initialized successfully');
  }

  // =========================================================================
  // DSARs CRUD
  // =========================================================================

  private generateDsarNumber(): string {
    const year = new Date().getFullYear();
    const seq = Math.floor(Math.random() * 99999).toString().padStart(5, '0');
    return `DSAR-${year}-${seq}`;
  }

  async createDsar(request: CreateDsarRequest, dsarDeadlineDays = 30): Promise<DsarRecord> {
    const requestNumber = this.generateDsarNumber();
    const deadline = new Date(Date.now() + dsarDeadlineDays * 24 * 60 * 60 * 1000);

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_dsars (
        source_account_id, request_type, request_number,
        user_id, requester_email, requester_name,
        description, data_categories, specific_data_requested,
        deadline, regulation, jurisdiction
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *`,
      [
        this.sourceAccountId, request.request_type, requestNumber,
        request.user_id ?? null, request.email, request.name ?? null,
        request.description ?? null, request.data_categories ?? [],
        request.specific_data_requested ?? null,
        deadline, request.regulation ?? 'GDPR', request.jurisdiction ?? null,
      ]
    );

    const dsar = result.rows[0] as unknown as DsarRecord;

    await this.addDsarActivity(dsar.id, 'created', 'DSAR request created');

    return dsar;
  }

  async getDsar(id: string): Promise<DsarRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_dsars WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as DsarRecord | null;
  }

  async listDsars(options: { status?: string; user_id?: string; limit?: number; offset?: number } = {}): Promise<{ dsars: DsarRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }
    if (options.user_id) { conditions.push(`user_id = $${paramIndex++}`); params.push(options.user_id); }

    const whereClause = conditions.join(' AND ');
    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const countResult = await this.query<{ total: string }>(
      `SELECT COUNT(*) as total FROM compliance_dsars WHERE ${whereClause}`, params
    );
    const total = parseInt(countResult.rows[0]?.total ?? '0', 10);

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM compliance_dsars WHERE ${whereClause} ORDER BY deadline ASC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`,
      [...params, limit, offset]
    );

    return { dsars: result.rows as unknown as DsarRecord[], total };
  }

  async processDsar(id: string, request: ProcessDsarRequest): Promise<DsarRecord | null> {
    const newStatus: DsarStatus = request.action === 'approve' ? 'approved' : 'rejected';

    const result = await this.query<Record<string, unknown>>(
      `UPDATE compliance_dsars SET
        status = $3, resolution_notes = $4, rejection_reason = $5,
        assigned_to = COALESCE($6, assigned_to),
        started_at = CASE WHEN started_at IS NULL THEN NOW() ELSE started_at END,
        updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [
        this.sourceAccountId, id, newStatus,
        request.notes ?? null, request.rejection_reason ?? null,
        request.assigned_to ?? null,
      ]
    );

    const dsar = (result.rows[0] ?? null) as unknown as DsarRecord | null;
    if (dsar) {
      await this.addDsarActivity(id, request.action === 'approve' ? 'approved' : 'rejected',
        `DSAR ${request.action === 'approve' ? 'approved' : 'rejected'}${request.notes ? ': ' + request.notes : ''}`);
    }

    return dsar;
  }

  async completeDsar(id: string, dataPackageUrl?: string): Promise<DsarRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      `UPDATE compliance_dsars SET
        status = 'completed', completed_at = NOW(),
        data_package_url = COALESCE($3, data_package_url),
        data_package_generated_at = CASE WHEN $3 IS NOT NULL THEN NOW() ELSE data_package_generated_at END,
        updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      [this.sourceAccountId, id, dataPackageUrl ?? null]
    );

    const dsar = (result.rows[0] ?? null) as unknown as DsarRecord | null;
    if (dsar) {
      await this.addDsarActivity(id, 'completed', 'DSAR processing completed');
    }

    return dsar;
  }

  async verifyDsar(id: string, token: string): Promise<boolean> {
    const dsar = await this.getDsar(id);
    if (!dsar || dsar.verification_token !== token) return false;

    await this.execute(
      `UPDATE compliance_dsars SET verification_completed_at = NOW(), status = 'in_progress', updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2`,
      [this.sourceAccountId, id]
    );

    await this.addDsarActivity(id, 'verified', 'Identity verification completed');
    return true;
  }

  // =========================================================================
  // DSAR Activities
  // =========================================================================

  async addDsarActivity(dsarId: string, activityType: string, description?: string, performedBy?: string): Promise<DsarActivityRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_dsar_activities (source_account_id, dsar_id, activity_type, description, performed_by)
       VALUES ($1, $2, $3, $4, $5) RETURNING *`,
      [this.sourceAccountId, dsarId, activityType, description ?? null, performedBy ?? null]
    );
    return result.rows[0] as unknown as DsarActivityRecord;
  }

  async getDsarActivities(dsarId: string): Promise<DsarActivityRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_dsar_activities WHERE source_account_id = $1 AND dsar_id = $2 ORDER BY created_at ASC',
      [this.sourceAccountId, dsarId]
    );
    return result.rows as unknown as DsarActivityRecord[];
  }

  // =========================================================================
  // Consent CRUD
  // =========================================================================

  async createConsent(request: CreateConsentRequest): Promise<ConsentRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_consents (
        source_account_id, user_id, purpose, purpose_description,
        status, granted_at, denied_at, consent_method, consent_text,
        privacy_policy_version, expires_at
      ) VALUES ($1, $2, $3, $4, $5,
        CASE WHEN $5 = 'granted' THEN NOW() ELSE NULL END,
        CASE WHEN $5 = 'denied' THEN NOW() ELSE NULL END,
        $6, $7, $8, $9::timestamptz)
      RETURNING *`,
      [
        this.sourceAccountId, request.user_id, request.purpose,
        request.purpose_description ?? null, request.status,
        request.consent_method ?? null, request.consent_text ?? null,
        request.privacy_policy_version ?? null, request.expires_at ?? null,
      ]
    );

    const consent = result.rows[0] as unknown as ConsentRecord;

    // Log history
    await this.execute(
      `INSERT INTO compliance_consent_history (source_account_id, consent_id, new_status, change_reason)
       VALUES ($1, $2, $3, 'Initial consent')`,
      [this.sourceAccountId, consent.id, request.status]
    );

    return consent;
  }

  async getConsent(id: string): Promise<ConsentRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_consents WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as ConsentRecord | null;
  }

  async listConsents(options: { user_id?: string; purpose?: string } = {}): Promise<ConsentRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.user_id) { conditions.push(`user_id = $${paramIndex++}`); params.push(options.user_id); }
    if (options.purpose) { conditions.push(`purpose = $${paramIndex++}`); params.push(options.purpose); }

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM compliance_consents WHERE ${conditions.join(' AND ')} ORDER BY user_id, purpose`,
      params
    );
    return result.rows as unknown as ConsentRecord[];
  }

  async withdrawConsent(id: string, reason?: string): Promise<ConsentRecord | null> {
    const consent = await this.getConsent(id);
    if (!consent) return null;

    const result = await this.query<Record<string, unknown>>(
      `UPDATE compliance_consents SET status = 'withdrawn', withdrawn_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      [this.sourceAccountId, id]
    );

    await this.execute(
      `INSERT INTO compliance_consent_history (source_account_id, consent_id, previous_status, new_status, change_reason)
       VALUES ($1, $2, $3, 'withdrawn', $4)`,
      [this.sourceAccountId, id, consent.status, reason ?? 'User withdrawal']
    );

    return (result.rows[0] ?? null) as unknown as ConsentRecord | null;
  }

  async checkUserConsent(userId: string, purpose: string): Promise<boolean> {
    const result = await this.query<{ has_consent: boolean }>(
      `SELECT EXISTS (
        SELECT 1 FROM compliance_consents
        WHERE source_account_id = $1 AND user_id = $2 AND purpose = $3
          AND status = 'granted' AND (expires_at IS NULL OR expires_at > NOW())
      ) as has_consent`,
      [this.sourceAccountId, userId, purpose]
    );
    return result.rows[0]?.has_consent ?? false;
  }

  // =========================================================================
  // Privacy Policies
  // =========================================================================

  async createPrivacyPolicy(request: CreatePrivacyPolicyRequest): Promise<PrivacyPolicyRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_privacy_policies (
        source_account_id, version, version_number, title, content,
        summary, changes_summary, requires_reacceptance,
        effective_from, language, jurisdiction
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::timestamptz, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId, request.version, request.version_number,
        request.title, request.content,
        request.summary ?? null, request.changes_summary ?? null,
        request.requires_reacceptance ?? false,
        request.effective_from, request.language ?? 'en', request.jurisdiction ?? null,
      ]
    );
    return result.rows[0] as unknown as PrivacyPolicyRecord;
  }

  async getActivePrivacyPolicy(): Promise<PrivacyPolicyRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM compliance_privacy_policies
       WHERE source_account_id = $1 AND is_active = true
       ORDER BY effective_from DESC LIMIT 1`,
      [this.sourceAccountId]
    );
    return (result.rows[0] ?? null) as unknown as PrivacyPolicyRecord | null;
  }

  async getPrivacyPolicy(version?: string): Promise<PrivacyPolicyRecord | null> {
    if (version) {
      const result = await this.query<Record<string, unknown>>(
        'SELECT * FROM compliance_privacy_policies WHERE source_account_id = $1 AND version = $2',
        [this.sourceAccountId, version]
      );
      return (result.rows[0] ?? null) as unknown as PrivacyPolicyRecord | null;
    }
    return this.getActivePrivacyPolicy();
  }

  async publishPrivacyPolicy(id: string): Promise<PrivacyPolicyRecord | null> {
    // Deactivate all others
    await this.execute(
      'UPDATE compliance_privacy_policies SET is_active = false WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    const result = await this.query<Record<string, unknown>>(
      `UPDATE compliance_privacy_policies SET is_active = true
       WHERE source_account_id = $1 AND id = $2 RETURNING *`,
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as PrivacyPolicyRecord | null;
  }

  async acceptPolicy(userId: string, policyId: string): Promise<PolicyAcceptanceRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_policy_acceptances (source_account_id, user_id, policy_id)
       VALUES ($1, $2, $3)
       ON CONFLICT (source_account_id, user_id, policy_id) DO UPDATE SET accepted_at = NOW()
       RETURNING *`,
      [this.sourceAccountId, userId, policyId]
    );
    return result.rows[0] as unknown as PolicyAcceptanceRecord;
  }

  // =========================================================================
  // Retention Policies
  // =========================================================================

  async createRetentionPolicy(request: CreateRetentionPolicyRequest): Promise<RetentionPolicyRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_retention_policies (
        source_account_id, name, description, data_category, table_name,
        retention_days, retention_action, conditions, legal_basis, regulation
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
      RETURNING *`,
      [
        this.sourceAccountId, request.name, request.description ?? null,
        request.data_category, request.table_name ?? null,
        request.retention_days, request.retention_action,
        JSON.stringify(request.conditions ?? {}),
        request.legal_basis ?? null, request.regulation ?? null,
      ]
    );
    return result.rows[0] as unknown as RetentionPolicyRecord;
  }

  async listRetentionPolicies(enabledOnly = false): Promise<RetentionPolicyRecord[]> {
    const sql = enabledOnly
      ? 'SELECT * FROM compliance_retention_policies WHERE source_account_id = $1 AND is_enabled = true ORDER BY priority ASC'
      : 'SELECT * FROM compliance_retention_policies WHERE source_account_id = $1 ORDER BY priority ASC';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as RetentionPolicyRecord[];
  }

  async executeRetentionPolicy(policyId: string): Promise<RetentionExecutionRecord> {
    const startTime = Date.now();

    // Create execution record
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_retention_executions (source_account_id, policy_id, status)
       VALUES ($1, $2, 'running') RETURNING *`,
      [this.sourceAccountId, policyId]
    );

    const execution = result.rows[0] as unknown as RetentionExecutionRecord;
    let recordsDeleted = 0;
    let recordsAnonymized = 0;
    let recordsArchived = 0;
    let status = 'completed';
    let errorMessage: string | null = null;

    try {
      // Fetch the retention policy
      const policyResult = await this.query<Record<string, unknown>>(
        'SELECT * FROM compliance_retention_policies WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, policyId]
      );

      if (policyResult.rows.length === 0) {
        throw new Error(`Retention policy ${policyId} not found`);
      }

      const policy = policyResult.rows[0] as unknown as {
        retention_days: number;
        retention_action: string;
        table_name: string | null;
        conditions: Record<string, unknown>;
        is_enabled: boolean;
      };

      if (!policy.is_enabled) {
        throw new Error(`Retention policy ${policyId} is disabled`);
      }

      // Calculate cutoff date (data older than this should be processed)
      const cutoffDate = new Date();
      cutoffDate.setDate(cutoffDate.getDate() - policy.retention_days);
      const cutoffTimestamp = cutoffDate.toISOString();

      // If no table specified, skip (cannot process without target table)
      if (!policy.table_name) {
        throw new Error('Retention policy must specify a table_name for execution');
      }

      // Build WHERE clause from conditions
      const conditionClauses: string[] = ['source_account_id = $1'];
      const conditionValues: unknown[] = [this.sourceAccountId];
      let paramIndex = 2;

      // Add timestamp condition (assuming created_at column)
      conditionClauses.push(`created_at < $${paramIndex}`);
      conditionValues.push(cutoffTimestamp);
      paramIndex++;

      // Add custom conditions from policy.conditions if any
      // Note: This is a basic implementation - production would need more sophisticated condition parsing
      if (policy.conditions && Object.keys(policy.conditions).length > 0) {
        for (const [key, value] of Object.entries(policy.conditions)) {
          conditionClauses.push(`${key} = $${paramIndex}`);
          conditionValues.push(value);
          paramIndex++;
        }
      }

      const whereClause = conditionClauses.join(' AND ');

      // Execute retention action in a transaction
      await this.execute('BEGIN');

      try {
        switch (policy.retention_action) {
          case 'delete': {
            // DELETE records matching criteria
            const deleteResult = await this.query<Record<string, unknown>>(
              `DELETE FROM ${policy.table_name} WHERE ${whereClause} RETURNING id`,
              conditionValues
            );
            recordsDeleted = deleteResult.rows.length;
            break;
          }

          case 'anonymize': {
            // UPDATE records to anonymize PII
            // This is a basic implementation - would need column mapping for full anonymization
            const anonymizeResult = await this.query<Record<string, unknown>>(
              `UPDATE ${policy.table_name}
               SET
                 email = 'anonymized-' || id || '@example.com',
                 name = 'Anonymized User',
                 phone = NULL,
                 address = NULL,
                 metadata = jsonb_set(metadata, '{anonymized}', 'true'::jsonb)
               WHERE ${whereClause}
               RETURNING id`,
              conditionValues
            );
            recordsAnonymized = anonymizeResult.rows.length;
            break;
          }

          case 'archive': {
            // Move records to archive table (assumes archive table exists with same schema)
            const archiveTableName = `${policy.table_name}_archive`;

            // Insert into archive
            await this.execute(
              `INSERT INTO ${archiveTableName}
               SELECT * FROM ${policy.table_name} WHERE ${whereClause}`,
              conditionValues
            );

            // Delete from original table
            const deleteResult = await this.query<Record<string, unknown>>(
              `DELETE FROM ${policy.table_name} WHERE ${whereClause} RETURNING id`,
              conditionValues
            );
            recordsArchived = deleteResult.rows.length;
            break;
          }

          case 'notify': {
            // Just log the notification - no data changes
            // In production, this would trigger actual notifications
            console.log(`Retention policy ${policyId} triggered notification - no data changes`);
            break;
          }

          default:
            throw new Error(`Unknown retention action: ${policy.retention_action}`);
        }

        await this.execute('COMMIT');
      } catch (txError) {
        await this.execute('ROLLBACK');
        throw txError;
      }

    } catch (error) {
      status = 'failed';
      errorMessage = error instanceof Error ? error.message : String(error);
    }

    const executionTimeMs = Date.now() - startTime;
    const recordsProcessed = recordsDeleted + recordsAnonymized + recordsArchived;

    // Update execution record with results
    await this.execute(
      `UPDATE compliance_retention_executions SET
        status = $2,
        execution_time_ms = $3,
        records_processed = $4,
        records_deleted = $5,
        records_anonymized = $6,
        records_archived = $7,
        error_message = $8
       WHERE id = $1`,
      [execution.id, status, executionTimeMs, recordsProcessed, recordsDeleted, recordsAnonymized, recordsArchived, errorMessage]
    );

    const updated = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_retention_executions WHERE id = $1', [execution.id]
    );
    return updated.rows[0] as unknown as RetentionExecutionRecord;
  }

  async getRetentionExecutions(policyId: string, limit = 20): Promise<RetentionExecutionRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_retention_executions WHERE source_account_id = $1 AND policy_id = $2 ORDER BY executed_at DESC LIMIT $3',
      [this.sourceAccountId, policyId, limit]
    );
    return result.rows as unknown as RetentionExecutionRecord[];
  }

  // =========================================================================
  // Data Processors
  // =========================================================================

  async listDataProcessors(activeOnly = true): Promise<DataProcessorRecord[]> {
    const sql = activeOnly
      ? 'SELECT * FROM compliance_data_processors WHERE source_account_id = $1 AND is_active = true ORDER BY processor_name'
      : 'SELECT * FROM compliance_data_processors WHERE source_account_id = $1 ORDER BY processor_name';
    const result = await this.query<Record<string, unknown>>(sql, [this.sourceAccountId]);
    return result.rows as unknown as DataProcessorRecord[];
  }

  // =========================================================================
  // Data Breaches
  // =========================================================================

  private generateBreachNumber(): string {
    const year = new Date().getFullYear();
    const seq = Math.floor(Math.random() * 99999).toString().padStart(5, '0');
    return `BREACH-${year}-${seq}`;
  }

  async createBreach(request: CreateBreachRequest, notificationHours = 72): Promise<DataBreachRecord> {
    const breachNumber = this.generateBreachNumber();
    const discoveredAt = request.discovered_at ? new Date(request.discovered_at) : new Date();
    const notificationDeadline = new Date(discoveredAt.getTime() + notificationHours * 60 * 60 * 1000);

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_data_breaches (
        source_account_id, breach_number, title, description,
        discovered_at, discovered_by, severity,
        affected_users_count, data_categories, data_description,
        notification_required, notification_deadline
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *`,
      [
        this.sourceAccountId, breachNumber, request.title, request.description,
        discoveredAt, request.discovered_by ?? null, request.severity,
        request.affected_users_count ?? null, request.data_categories,
        request.data_description ?? null,
        request.notification_required ?? true, notificationDeadline,
      ]
    );
    return result.rows[0] as unknown as DataBreachRecord;
  }

  async getBreach(id: string): Promise<DataBreachRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_data_breaches WHERE source_account_id = $1 AND id = $2',
      [this.sourceAccountId, id]
    );
    return (result.rows[0] ?? null) as unknown as DataBreachRecord | null;
  }

  async listBreaches(options: { status?: string; severity?: string } = {}): Promise<DataBreachRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.status) { conditions.push(`status = $${paramIndex++}`); params.push(options.status); }
    if (options.severity) { conditions.push(`severity = $${paramIndex++}`); params.push(options.severity); }

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM compliance_data_breaches WHERE ${conditions.join(' AND ')} ORDER BY discovered_at DESC`,
      params
    );
    return result.rows as unknown as DataBreachRecord[];
  }

  async addBreachNotification(
    breachId: string, notificationType: string, recipientType: string,
    recipientEmail?: string, subject?: string, messageBody?: string
  ): Promise<BreachNotificationRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_breach_notifications (
        source_account_id, breach_id, notification_type, recipient_type,
        recipient_email, subject, message_body, sent_at, delivery_status
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), 'sent')
      RETURNING *`,
      [
        this.sourceAccountId, breachId, notificationType, recipientType,
        recipientEmail ?? null, subject ?? null, messageBody ?? null,
      ]
    );

    // Update breach notification timestamps
    if (notificationType === 'authority') {
      await this.execute(
        'UPDATE compliance_data_breaches SET authority_notified_at = NOW(), updated_at = NOW() WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, breachId]
      );
    } else if (notificationType === 'user') {
      await this.execute(
        'UPDATE compliance_data_breaches SET users_notified_at = NOW(), updated_at = NOW() WHERE source_account_id = $1 AND id = $2',
        [this.sourceAccountId, breachId]
      );
    }

    return result.rows[0] as unknown as BreachNotificationRecord;
  }

  // =========================================================================
  // Audit Log
  // =========================================================================

  async createAuditLog(request: CreateAuditLogRequest): Promise<ComplianceAuditLogRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO compliance_audit_log (
        source_account_id, event_type, event_category, actor_id, actor_type,
        target_type, target_id, accessed_data_categories, data_subject_id,
        details, ip_address, user_agent, legal_basis
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
      RETURNING *`,
      [
        this.sourceAccountId, request.event_type, request.event_category,
        request.actor_id ?? null, request.actor_type ?? 'user',
        request.target_type ?? null, request.target_id ?? null,
        request.accessed_data_categories ?? [], request.data_subject_id ?? null,
        JSON.stringify(request.details ?? {}),
        request.ip_address ?? null, request.user_agent ?? null,
        request.legal_basis ?? null,
      ]
    );
    return result.rows[0] as unknown as ComplianceAuditLogRecord;
  }

  async listAuditLogs(options: {
    event_category?: string;
    actor_id?: string;
    data_subject_id?: string;
    limit?: number;
    offset?: number;
  } = {}): Promise<{ logs: ComplianceAuditLogRecord[]; total: number }> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.event_category) { conditions.push(`event_category = $${paramIndex++}`); params.push(options.event_category); }
    if (options.actor_id) { conditions.push(`actor_id = $${paramIndex++}`); params.push(options.actor_id); }
    if (options.data_subject_id) { conditions.push(`data_subject_id = $${paramIndex++}`); params.push(options.data_subject_id); }

    const whereClause = conditions.join(' AND ');
    const limit = options.limit ?? 50;
    const offset = options.offset ?? 0;

    const countResult = await this.query<{ total: string }>(
      `SELECT COUNT(*) as total FROM compliance_audit_log WHERE ${whereClause}`, params
    );
    const total = parseInt(countResult.rows[0]?.total ?? '0', 10);

    const result = await this.query<Record<string, unknown>>(
      `SELECT * FROM compliance_audit_log WHERE ${whereClause} ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`,
      [...params, limit, offset]
    );

    return { logs: result.rows as unknown as ComplianceAuditLogRecord[], total };
  }

  // =========================================================================
  // Data Export
  // =========================================================================

  async exportUserData(userId: string, _categories?: string[]): Promise<Record<string, unknown>> {
    const data: Record<string, unknown> = { user_id: userId, exported_at: new Date().toISOString() };

    // Consents
    const consents = await this.listConsents({ user_id: userId });
    data.consents = consents;

    // DSARs
    const dsarResult = await this.listDsars({ user_id: userId });
    data.dsars = dsarResult.dsars;

    // Policy acceptances
    const acceptances = await this.query<Record<string, unknown>>(
      'SELECT * FROM compliance_policy_acceptances WHERE source_account_id = $1 AND user_id = $2',
      [this.sourceAccountId, userId]
    );
    data.policy_acceptances = acceptances.rows;

    return data;
  }
}
