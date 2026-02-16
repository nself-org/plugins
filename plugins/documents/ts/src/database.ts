/**
 * Documents Database Operations
 * Complete CRUD operations for documents, templates, versions, and shares
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  DocumentRecord,
  TemplateRecord,
  VersionRecord,
  ShareRecord,
  DocumentStats,
} from './types.js';

const logger = createLogger('documents:db');

export class DocumentsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): DocumentsDatabase {
    return new DocumentsDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing documents schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Documents
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS docs_documents (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        owner_id VARCHAR(255) NOT NULL,
        title VARCHAR(500) NOT NULL,
        description TEXT,
        doc_type VARCHAR(50) NOT NULL,
        category VARCHAR(100),
        tags TEXT[] DEFAULT '{}',
        template_id UUID,
        file_url TEXT,
        file_size_bytes BIGINT,
        mime_type VARCHAR(100),
        version INTEGER DEFAULT 1,
        status VARCHAR(20) DEFAULT 'draft',
        generated_from JSONB,
        metadata JSONB DEFAULT '{}',
        search_vector tsvector,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_docs_documents_source_app
        ON docs_documents(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_docs_documents_owner
        ON docs_documents(source_account_id, owner_id);
      CREATE INDEX IF NOT EXISTS idx_docs_documents_type
        ON docs_documents(source_account_id, doc_type);
      CREATE INDEX IF NOT EXISTS idx_docs_documents_category
        ON docs_documents(source_account_id, category);
      CREATE INDEX IF NOT EXISTS idx_docs_documents_search
        ON docs_documents USING GIN(search_vector);

      -- =====================================================================
      -- Templates
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS docs_templates (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        doc_type VARCHAR(50) NOT NULL,
        output_format VARCHAR(20) DEFAULT 'pdf',
        template_engine VARCHAR(20) DEFAULT 'handlebars',
        template_content TEXT NOT NULL,
        css_content TEXT,
        header_content TEXT,
        footer_content TEXT,
        variables JSONB DEFAULT '{}',
        sample_data JSONB DEFAULT '{}',
        is_default BOOLEAN DEFAULT false,
        version INTEGER DEFAULT 1,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        updated_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, name, version)
      );

      CREATE INDEX IF NOT EXISTS idx_docs_templates_source_app
        ON docs_templates(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_docs_templates_type
        ON docs_templates(source_account_id, doc_type);

      -- =====================================================================
      -- Versions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS docs_versions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        document_id UUID NOT NULL REFERENCES docs_documents(id) ON DELETE CASCADE,
        version INTEGER NOT NULL,
        file_url TEXT NOT NULL,
        file_size_bytes BIGINT,
        change_summary TEXT,
        created_by VARCHAR(255),
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, document_id, version)
      );

      CREATE INDEX IF NOT EXISTS idx_docs_versions_source_app
        ON docs_versions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_docs_versions_document
        ON docs_versions(document_id, version DESC);

      -- =====================================================================
      -- Shares
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS docs_shares (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        document_id UUID NOT NULL REFERENCES docs_documents(id) ON DELETE CASCADE,
        shared_with_user_id VARCHAR(255),
        shared_with_email VARCHAR(255),
        share_token VARCHAR(255),
        permission VARCHAR(20) DEFAULT 'view',
        expires_at TIMESTAMPTZ,
        accessed_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ DEFAULT NOW(),
        UNIQUE(source_account_id, document_id, share_token)
      );

      CREATE INDEX IF NOT EXISTS idx_docs_shares_source_app
        ON docs_shares(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_docs_shares_document
        ON docs_shares(document_id);
      CREATE INDEX IF NOT EXISTS idx_docs_shares_user
        ON docs_shares(source_account_id, shared_with_user_id);
      CREATE INDEX IF NOT EXISTS idx_docs_shares_token
        ON docs_shares(share_token);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS docs_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMPTZ,
        error TEXT,
        retry_count INTEGER DEFAULT 0,
        created_at TIMESTAMPTZ DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_docs_webhook_events_source_app
        ON docs_webhook_events(source_account_id);
    `;

    await this.execute(schema);
    logger.info('Documents schema initialized successfully');
  }

  // =========================================================================
  // Document Operations
  // =========================================================================

  async createDocument(doc: Omit<DocumentRecord, 'id' | 'created_at' | 'updated_at'>): Promise<DocumentRecord> {
    const result = await this.query<DocumentRecord>(
      `INSERT INTO docs_documents (
        source_account_id, owner_id, title, description, doc_type, category,
        tags, template_id, file_url, file_size_bytes, mime_type, version,
        status, generated_from, metadata,
        search_vector
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
        to_tsvector('english', $3 || ' ' || COALESCE($4, '') || ' ' || COALESCE($5, ''))
      )
      RETURNING *`,
      [
        this.sourceAccountId,
        doc.owner_id,
        doc.title,
        doc.description,
        doc.doc_type,
        doc.category,
        doc.tags,
        doc.template_id,
        doc.file_url,
        doc.file_size_bytes,
        doc.mime_type,
        doc.version,
        doc.status,
        doc.generated_from ? JSON.stringify(doc.generated_from) : null,
        JSON.stringify(doc.metadata),
      ]
    );

    return result.rows[0];
  }

  async getDocument(id: string): Promise<DocumentRecord | null> {
    const result = await this.query<DocumentRecord>(
      `SELECT * FROM docs_documents WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listDocuments(filters: {
    ownerId?: string;
    docType?: string;
    category?: string;
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<DocumentRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.ownerId) {
      conditions.push(`owner_id = $${paramIndex}`);
      values.push(filters.ownerId);
      paramIndex++;
    }

    if (filters.docType) {
      conditions.push(`doc_type = $${paramIndex}`);
      values.push(filters.docType);
      paramIndex++;
    }

    if (filters.category) {
      conditions.push(`category = $${paramIndex}`);
      values.push(filters.category);
      paramIndex++;
    }

    if (filters.status) {
      conditions.push(`status = $${paramIndex}`);
      values.push(filters.status);
      paramIndex++;
    }

    let sql = `
      SELECT * FROM docs_documents
      WHERE ${conditions.join(' AND ')}
      ORDER BY updated_at DESC
    `;

    if (filters.limit) {
      sql += ` LIMIT $${paramIndex}`;
      values.push(filters.limit);
      paramIndex++;
    }

    if (filters.offset) {
      sql += ` OFFSET $${paramIndex}`;
      values.push(filters.offset);
      paramIndex++;
    }

    const result = await this.query<DocumentRecord>(sql, values);
    return result.rows;
  }

  async updateDocument(id: string, updates: Partial<DocumentRecord>): Promise<DocumentRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = ['title', 'description', 'category', 'tags', 'status', 'file_url', 'file_size_bytes', 'mime_type', 'metadata'];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        if (key === 'metadata') {
          fields.push(`${key} = $${paramIndex}::jsonb`);
          values.push(JSON.stringify(value));
        } else {
          fields.push(`${key} = $${paramIndex}`);
          values.push(value);
        }
        paramIndex++;
      }
    }

    if (fields.length === 0) {
      return this.getDocument(id);
    }

    fields.push(`updated_at = NOW()`);

    // Update search vector if title or description changed
    if (updates.title || updates.description) {
      fields.push(`search_vector = to_tsvector('english', COALESCE($${paramIndex}, title) || ' ' || COALESCE($${paramIndex + 1}, description, '') || ' ' || doc_type)`);
      values.push(updates.title ?? null, updates.description ?? null);
      paramIndex += 2;
    }

    values.push(id, this.sourceAccountId);

    const result = await this.query<DocumentRecord>(
      `UPDATE docs_documents
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteDocument(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM docs_documents WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  async searchDocuments(filters: {
    query: string;
    docType?: string;
    category?: string;
    ownerId?: string;
    dateFrom?: Date;
    dateTo?: Date;
    limit?: number;
  }): Promise<DocumentRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.query) {
      conditions.push(`search_vector @@ plainto_tsquery('english', $${paramIndex})`);
      values.push(filters.query);
      paramIndex++;
    }

    if (filters.docType) {
      conditions.push(`doc_type = $${paramIndex}`);
      values.push(filters.docType);
      paramIndex++;
    }

    if (filters.category) {
      conditions.push(`category = $${paramIndex}`);
      values.push(filters.category);
      paramIndex++;
    }

    if (filters.ownerId) {
      conditions.push(`owner_id = $${paramIndex}`);
      values.push(filters.ownerId);
      paramIndex++;
    }

    if (filters.dateFrom) {
      conditions.push(`created_at >= $${paramIndex}`);
      values.push(filters.dateFrom);
      paramIndex++;
    }

    if (filters.dateTo) {
      conditions.push(`created_at <= $${paramIndex}`);
      values.push(filters.dateTo);
      paramIndex++;
    }

    const limit = filters.limit ?? 50;
    values.push(limit);

    const result = await this.query<DocumentRecord>(
      `SELECT * FROM docs_documents
       WHERE ${conditions.join(' AND ')}
       ORDER BY updated_at DESC
       LIMIT $${paramIndex}`,
      values
    );

    return result.rows;
  }

  // =========================================================================
  // Template Operations
  // =========================================================================

  async createTemplate(template: Omit<TemplateRecord, 'id' | 'created_at' | 'updated_at'>): Promise<TemplateRecord> {
    const result = await this.query<TemplateRecord>(
      `INSERT INTO docs_templates (
        source_account_id, name, description, doc_type, output_format,
        template_engine, template_content, css_content, header_content,
        footer_content, variables, sample_data, is_default, version
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
      RETURNING *`,
      [
        this.sourceAccountId,
        template.name,
        template.description,
        template.doc_type,
        template.output_format,
        template.template_engine,
        template.template_content,
        template.css_content,
        template.header_content,
        template.footer_content,
        JSON.stringify(template.variables),
        JSON.stringify(template.sample_data),
        template.is_default,
        template.version,
      ]
    );

    return result.rows[0];
  }

  async getTemplate(id: string): Promise<TemplateRecord | null> {
    const result = await this.query<TemplateRecord>(
      `SELECT * FROM docs_templates WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getTemplateByName(name: string): Promise<TemplateRecord | null> {
    const result = await this.query<TemplateRecord>(
      `SELECT * FROM docs_templates
       WHERE source_account_id = $1 AND name = $2
       ORDER BY version DESC
       LIMIT 1`,
      [this.sourceAccountId, name]
    );

    return result.rows[0] ?? null;
  }

  async listTemplates(filters: {
    docType?: string;
    limit?: number;
    offset?: number;
  }): Promise<TemplateRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.docType) {
      conditions.push(`doc_type = $${paramIndex}`);
      values.push(filters.docType);
      paramIndex++;
    }

    let sql = `
      SELECT * FROM docs_templates
      WHERE ${conditions.join(' AND ')}
      ORDER BY name ASC, version DESC
    `;

    if (filters.limit) {
      sql += ` LIMIT $${paramIndex}`;
      values.push(filters.limit);
      paramIndex++;
    }

    if (filters.offset) {
      sql += ` OFFSET $${paramIndex}`;
      values.push(filters.offset);
      paramIndex++;
    }

    const result = await this.query<TemplateRecord>(sql, values);
    return result.rows;
  }

  async updateTemplate(id: string, updates: Partial<TemplateRecord>): Promise<TemplateRecord | null> {
    const fields: string[] = [];
    const values: unknown[] = [];
    let paramIndex = 1;

    const allowedFields = [
      'name', 'description', 'doc_type', 'output_format', 'template_engine',
      'template_content', 'css_content', 'header_content', 'footer_content',
      'is_default',
    ];

    for (const [key, value] of Object.entries(updates)) {
      if (allowedFields.includes(key)) {
        fields.push(`${key} = $${paramIndex}`);
        values.push(value);
        paramIndex++;
      }
    }

    if (updates.variables !== undefined) {
      fields.push(`variables = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.variables));
      paramIndex++;
    }

    if (updates.sample_data !== undefined) {
      fields.push(`sample_data = $${paramIndex}::jsonb`);
      values.push(JSON.stringify(updates.sample_data));
      paramIndex++;
    }

    if (fields.length === 0) {
      return this.getTemplate(id);
    }

    fields.push(`updated_at = NOW()`);
    values.push(id, this.sourceAccountId);

    const result = await this.query<TemplateRecord>(
      `UPDATE docs_templates
       SET ${fields.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteTemplate(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM docs_templates WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  // =========================================================================
  // Version Operations
  // =========================================================================

  async createVersion(version: Omit<VersionRecord, 'id' | 'created_at'>): Promise<VersionRecord> {
    const result = await this.query<VersionRecord>(
      `INSERT INTO docs_versions (
        source_account_id, document_id, version, file_url,
        file_size_bytes, change_summary, created_by
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        version.document_id,
        version.version,
        version.file_url,
        version.file_size_bytes,
        version.change_summary,
        version.created_by,
      ]
    );

    return result.rows[0];
  }

  async listVersions(documentId: string): Promise<VersionRecord[]> {
    const result = await this.query<VersionRecord>(
      `SELECT * FROM docs_versions
       WHERE document_id = $1 AND source_account_id = $2
       ORDER BY version DESC`,
      [documentId, this.sourceAccountId]
    );

    return result.rows;
  }

  async getVersion(documentId: string, version: number): Promise<VersionRecord | null> {
    const result = await this.query<VersionRecord>(
      `SELECT * FROM docs_versions
       WHERE document_id = $1 AND version = $2 AND source_account_id = $3`,
      [documentId, version, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  // =========================================================================
  // Share Operations
  // =========================================================================

  async createShare(share: Omit<ShareRecord, 'id' | 'created_at' | 'accessed_at'>): Promise<ShareRecord> {
    const result = await this.query<ShareRecord>(
      `INSERT INTO docs_shares (
        source_account_id, document_id, shared_with_user_id, shared_with_email,
        share_token, permission, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        share.document_id,
        share.shared_with_user_id,
        share.shared_with_email,
        share.share_token,
        share.permission,
        share.expires_at,
      ]
    );

    return result.rows[0];
  }

  async listShares(documentId: string): Promise<ShareRecord[]> {
    const result = await this.query<ShareRecord>(
      `SELECT * FROM docs_shares
       WHERE document_id = $1 AND source_account_id = $2
       ORDER BY created_at DESC`,
      [documentId, this.sourceAccountId]
    );

    return result.rows;
  }

  async getShareByToken(token: string): Promise<(ShareRecord & { doc_title: string; doc_file_url: string | null }) | null> {
    const result = await this.query<ShareRecord & { doc_title: string; doc_file_url: string | null }>(
      `SELECT s.*, d.title as doc_title, d.file_url as doc_file_url
       FROM docs_shares s
       JOIN docs_documents d ON s.document_id = d.id
       WHERE s.share_token = $1
         AND (s.expires_at IS NULL OR s.expires_at > NOW())`,
      [token]
    );

    if (result.rows[0]) {
      // Update accessed_at
      await this.execute(
        `UPDATE docs_shares SET accessed_at = NOW() WHERE id = $1`,
        [result.rows[0].id]
      );
    }

    return result.rows[0] ?? null;
  }

  async deleteShare(id: string): Promise<boolean> {
    const count = await this.execute(
      `DELETE FROM docs_shares WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return count > 0;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<DocumentStats> {
    const result = await this.query<{
      total_documents: string;
      total_templates: string;
      total_shares: string;
      total_versions: string;
      recent_documents: string;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM docs_documents WHERE source_account_id = $1) as total_documents,
        (SELECT COUNT(*) FROM docs_templates WHERE source_account_id = $1) as total_templates,
        (SELECT COUNT(*) FROM docs_shares WHERE source_account_id = $1) as total_shares,
        (SELECT COUNT(*) FROM docs_versions WHERE source_account_id = $1) as total_versions,
        (SELECT COUNT(*) FROM docs_documents WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '7 days') as recent_documents`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    // Get counts by type
    const typeResult = await this.query<{ doc_type: string; count: string }>(
      `SELECT doc_type, COUNT(*) as count FROM docs_documents
       WHERE source_account_id = $1
       GROUP BY doc_type`,
      [this.sourceAccountId]
    );

    const byType: Record<string, number> = {};
    for (const r of typeResult.rows) {
      byType[r.doc_type] = parseInt(r.count, 10);
    }

    // Get counts by category
    const catResult = await this.query<{ category: string; count: string }>(
      `SELECT COALESCE(category, 'uncategorized') as category, COUNT(*) as count
       FROM docs_documents
       WHERE source_account_id = $1
       GROUP BY category`,
      [this.sourceAccountId]
    );

    const byCategory: Record<string, number> = {};
    for (const r of catResult.rows) {
      byCategory[r.category] = parseInt(r.count, 10);
    }

    return {
      total_documents: parseInt(row.total_documents, 10),
      by_type: byType,
      by_category: byCategory,
      total_templates: parseInt(row.total_templates, 10),
      total_shares: parseInt(row.total_shares, 10),
      total_versions: parseInt(row.total_versions, 10),
      recent_documents: parseInt(row.recent_documents, 10),
    };
  }
}
