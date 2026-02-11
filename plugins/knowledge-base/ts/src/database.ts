/**
 * Knowledge Base Database Operations
 * Complete CRUD operations for all knowledge base objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  KBDocumentRecord,
  CreateDocumentRequest,
  UpdateDocumentRequest,
  KBCollectionRecord,
  CreateCollectionRequest,
  UpdateCollectionRequest,
  KBFaqRecord,
  CreateFaqRequest,
  UpdateFaqRequest,
  KBAttachmentRecord,
  CreateAttachmentRequest,
  KBCommentRecord,
  CreateCommentRequest,
  UpdateCommentRequest,
  KBAnalyticsRecord,
  TrackAnalyticsEventRequest,
  KBTranslationRecord,
  CreateTranslationRequest,
  UpdateTranslationRequest,
  KBReviewRequestRecord,
  CreateReviewRequestRequest,
  KBSearchResult,
  KBStats,
  PopularSearch,
  DocumentStatus,
  DocumentType,
  Visibility,
  ReviewStatus,
} from './types.js';

const logger = createLogger('knowledge-base:db');

export class KBDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): KBDatabase {
    return new KBDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing knowledge base schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Collections Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_collections (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        parent_id UUID REFERENCES kb_collections(id) ON DELETE CASCADE,
        created_by VARCHAR(255) NOT NULL,
        name TEXT NOT NULL,
        slug TEXT NOT NULL,
        description TEXT,
        icon TEXT,
        color TEXT,
        display_order INTEGER DEFAULT 0,
        path TEXT[],
        depth INTEGER DEFAULT 0,
        visibility VARCHAR(32) NOT NULL DEFAULT 'private',
        default_language VARCHAR(16) DEFAULT 'en',
        allowed_languages TEXT[],
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_collections_source_account ON kb_collections(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_collections_workspace ON kb_collections(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_collections_parent ON kb_collections(parent_id);
      CREATE INDEX IF NOT EXISTS idx_kb_collections_slug ON kb_collections(workspace_id, slug);

      -- =====================================================================
      -- Documents Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_documents (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        collection_id UUID REFERENCES kb_collections(id) ON DELETE SET NULL,
        created_by VARCHAR(255) NOT NULL,
        title TEXT NOT NULL,
        slug TEXT NOT NULL,
        content TEXT NOT NULL,
        content_html TEXT,
        excerpt TEXT,
        status VARCHAR(32) NOT NULL DEFAULT 'draft',
        document_type VARCHAR(32) NOT NULL DEFAULT 'article',
        language VARCHAR(16) NOT NULL DEFAULT 'en',
        meta_title TEXT,
        meta_description TEXT,
        meta_keywords TEXT[],
        tags TEXT[],
        category TEXT,
        priority INTEGER DEFAULT 0,
        version INTEGER NOT NULL DEFAULT 1,
        parent_version_id UUID REFERENCES kb_documents(id) ON DELETE SET NULL,
        is_latest_version BOOLEAN DEFAULT true,
        visibility VARCHAR(32) NOT NULL DEFAULT 'private',
        required_role TEXT,
        view_count INTEGER DEFAULT 0,
        helpful_count INTEGER DEFAULT 0,
        not_helpful_count INTEGER DEFAULT 0,
        average_rating DECIMAL(3,2),
        published_at TIMESTAMP WITH TIME ZONE,
        last_reviewed_at TIMESTAMP WITH TIME ZONE,
        review_reminder_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_documents_source_account ON kb_documents(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_workspace ON kb_documents(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_collection ON kb_documents(collection_id);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_status ON kb_documents(status);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_type ON kb_documents(document_type);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_slug ON kb_documents(workspace_id, slug);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_visibility ON kb_documents(visibility);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_tags ON kb_documents USING GIN(tags);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_published ON kb_documents(published_at DESC);
      CREATE INDEX IF NOT EXISTS idx_kb_documents_latest ON kb_documents(workspace_id, slug, is_latest_version);

      -- =====================================================================
      -- FAQs Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_faqs (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        collection_id UUID REFERENCES kb_collections(id) ON DELETE SET NULL,
        created_by VARCHAR(255) NOT NULL,
        question TEXT NOT NULL,
        answer TEXT NOT NULL,
        answer_html TEXT,
        category TEXT,
        tags TEXT[],
        display_order INTEGER DEFAULT 0,
        status VARCHAR(32) NOT NULL DEFAULT 'published',
        language VARCHAR(16) NOT NULL DEFAULT 'en',
        view_count INTEGER DEFAULT 0,
        helpful_count INTEGER DEFAULT 0,
        not_helpful_count INTEGER DEFAULT 0,
        related_documents UUID[],
        related_faqs UUID[],
        published_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_faqs_source_account ON kb_faqs(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_faqs_workspace ON kb_faqs(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_faqs_collection ON kb_faqs(collection_id);
      CREATE INDEX IF NOT EXISTS idx_kb_faqs_status ON kb_faqs(status);
      CREATE INDEX IF NOT EXISTS idx_kb_faqs_category ON kb_faqs(category);
      CREATE INDEX IF NOT EXISTS idx_kb_faqs_tags ON kb_faqs USING GIN(tags);

      -- =====================================================================
      -- Attachments Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_attachments (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        document_id UUID REFERENCES kb_documents(id) ON DELETE CASCADE,
        uploaded_by VARCHAR(255) NOT NULL,
        filename TEXT NOT NULL,
        original_filename TEXT NOT NULL,
        mime_type TEXT NOT NULL,
        file_size BIGINT NOT NULL,
        storage_path TEXT NOT NULL,
        title TEXT,
        description TEXT,
        alt_text TEXT,
        processing_status VARCHAR(32) DEFAULT 'pending',
        thumbnail_url TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_attachments_source_account ON kb_attachments(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_attachments_workspace ON kb_attachments(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_attachments_document ON kb_attachments(document_id);

      -- =====================================================================
      -- Comments Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_comments (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        parent_id UUID REFERENCES kb_comments(id) ON DELETE CASCADE,
        content TEXT NOT NULL,
        content_html TEXT,
        status VARCHAR(32) NOT NULL DEFAULT 'published',
        is_staff_reply BOOLEAN DEFAULT false,
        helpful_count INTEGER DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_comments_source_account ON kb_comments(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_comments_workspace ON kb_comments(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_comments_document ON kb_comments(document_id);
      CREATE INDEX IF NOT EXISTS idx_kb_comments_user ON kb_comments(user_id);
      CREATE INDEX IF NOT EXISTS idx_kb_comments_parent ON kb_comments(parent_id);
      CREATE INDEX IF NOT EXISTS idx_kb_comments_created ON kb_comments(created_at DESC);

      -- =====================================================================
      -- Analytics Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_analytics (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        document_id UUID REFERENCES kb_documents(id) ON DELETE CASCADE,
        faq_id UUID REFERENCES kb_faqs(id) ON DELETE CASCADE,
        event_type VARCHAR(32) NOT NULL,
        user_id VARCHAR(255),
        session_id VARCHAR(255),
        search_query TEXT,
        referrer TEXT,
        user_agent TEXT,
        ip_address TEXT,
        metadata JSONB,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_analytics_source_account ON kb_analytics(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_workspace ON kb_analytics(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_document ON kb_analytics(document_id);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_faq ON kb_analytics(faq_id);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_event_type ON kb_analytics(event_type);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_user ON kb_analytics(user_id);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_created ON kb_analytics(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_kb_analytics_search ON kb_analytics(search_query);

      -- =====================================================================
      -- Translations Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_translations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        source_document_id UUID REFERENCES kb_documents(id) ON DELETE CASCADE,
        source_faq_id UUID REFERENCES kb_faqs(id) ON DELETE CASCADE,
        language VARCHAR(16) NOT NULL,
        translated_by VARCHAR(255),
        translation_method VARCHAR(32),
        title TEXT,
        content TEXT,
        content_html TEXT,
        answer TEXT,
        answer_html TEXT,
        status VARCHAR(32) NOT NULL DEFAULT 'draft',
        quality_score DECIMAL(3,2),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_translations_source_account ON kb_translations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_translations_workspace ON kb_translations(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_translations_document ON kb_translations(source_document_id);
      CREATE INDEX IF NOT EXISTS idx_kb_translations_faq ON kb_translations(source_faq_id);
      CREATE INDEX IF NOT EXISTS idx_kb_translations_language ON kb_translations(language);
      CREATE INDEX IF NOT EXISTS idx_kb_translations_status ON kb_translations(status);

      -- =====================================================================
      -- Review Requests Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS kb_review_requests (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        workspace_id VARCHAR(255) NOT NULL,
        document_id UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
        requested_by VARCHAR(255) NOT NULL,
        assigned_to VARCHAR(255),
        status VARCHAR(32) NOT NULL DEFAULT 'pending',
        priority VARCHAR(16) DEFAULT 'normal',
        review_notes TEXT,
        changes_requested TEXT[],
        due_date TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_kb_review_requests_source_account ON kb_review_requests(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_kb_review_requests_workspace ON kb_review_requests(workspace_id);
      CREATE INDEX IF NOT EXISTS idx_kb_review_requests_document ON kb_review_requests(document_id);
      CREATE INDEX IF NOT EXISTS idx_kb_review_requests_assigned ON kb_review_requests(assigned_to);
      CREATE INDEX IF NOT EXISTS idx_kb_review_requests_status ON kb_review_requests(status);
    `;

    await this.execute(schema);
    logger.success('Knowledge base schema initialized');
  }

  // =========================================================================
  // Document Operations
  // =========================================================================

  async createDocument(doc: CreateDocumentRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO kb_documents
       (id, source_account_id, workspace_id, collection_id, created_by,
        title, slug, content, content_html, excerpt,
        status, document_type, language,
        meta_title, meta_description, meta_keywords,
        tags, category, priority, visibility, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW(), NOW())`,
      [
        id, this.sourceAccountId, doc.workspace_id, doc.collection_id ?? null, doc.created_by,
        doc.title, doc.slug, doc.content, doc.content_html ?? null, doc.excerpt ?? null,
        doc.status ?? 'draft', doc.document_type ?? 'article', doc.language ?? 'en',
        doc.meta_title ?? null, doc.meta_description ?? null, doc.meta_keywords ?? null,
        doc.tags ?? null, doc.category ?? null, doc.priority ?? 0, doc.visibility ?? 'private',
      ]
    );
    return id;
  }

  async getDocument(id: string): Promise<KBDocumentRecord | null> {
    const result = await this.query<KBDocumentRecord>(
      `SELECT * FROM kb_documents WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getDocumentBySlug(workspaceId: string, slug: string, version?: number): Promise<KBDocumentRecord | null> {
    let sql = `SELECT * FROM kb_documents WHERE workspace_id = $1 AND slug = $2 AND source_account_id = $3`;
    const params: unknown[] = [workspaceId, slug, this.sourceAccountId];

    if (version !== undefined) {
      sql += ` AND version = $4`;
      params.push(version);
    } else {
      sql += ` AND is_latest_version = true`;
    }

    const result = await this.query<KBDocumentRecord>(sql, params);
    return result.rows[0] ?? null;
  }

  async listDocuments(workspaceId: string, limit = 100, offset = 0, filters?: {
    collection_id?: string;
    status?: DocumentStatus;
    document_type?: DocumentType;
    language?: string;
    visibility?: Visibility;
    tags?: string[];
    category?: string;
  }): Promise<KBDocumentRecord[]> {
    let sql = `SELECT * FROM kb_documents WHERE workspace_id = $1 AND source_account_id = $2 AND is_latest_version = true`;
    const params: unknown[] = [workspaceId, this.sourceAccountId];
    let paramIndex = 3;

    if (filters?.collection_id) {
      sql += ` AND collection_id = $${paramIndex}`;
      params.push(filters.collection_id);
      paramIndex++;
    }
    if (filters?.status) {
      sql += ` AND status = $${paramIndex}`;
      params.push(filters.status);
      paramIndex++;
    }
    if (filters?.document_type) {
      sql += ` AND document_type = $${paramIndex}`;
      params.push(filters.document_type);
      paramIndex++;
    }
    if (filters?.language) {
      sql += ` AND language = $${paramIndex}`;
      params.push(filters.language);
      paramIndex++;
    }
    if (filters?.visibility) {
      sql += ` AND visibility = $${paramIndex}`;
      params.push(filters.visibility);
      paramIndex++;
    }
    if (filters?.category) {
      sql += ` AND category = $${paramIndex}`;
      params.push(filters.category);
      paramIndex++;
    }
    if (filters?.tags && filters.tags.length > 0) {
      sql += ` AND tags && $${paramIndex}`;
      params.push(filters.tags);
      paramIndex++;
    }

    sql += ` ORDER BY priority DESC, created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<KBDocumentRecord>(sql, params);
    return result.rows;
  }

  async updateDocument(id: string, updates: UpdateDocumentRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    const fields: Array<{ key: keyof UpdateDocumentRequest; column: string }> = [
      { key: 'collection_id', column: 'collection_id' },
      { key: 'title', column: 'title' },
      { key: 'content', column: 'content' },
      { key: 'content_html', column: 'content_html' },
      { key: 'excerpt', column: 'excerpt' },
      { key: 'status', column: 'status' },
      { key: 'document_type', column: 'document_type' },
      { key: 'meta_title', column: 'meta_title' },
      { key: 'meta_description', column: 'meta_description' },
      { key: 'meta_keywords', column: 'meta_keywords' },
      { key: 'tags', column: 'tags' },
      { key: 'category', column: 'category' },
      { key: 'priority', column: 'priority' },
      { key: 'visibility', column: 'visibility' },
    ];

    for (const field of fields) {
      if (updates[field.key] !== undefined) {
        sets.push(`${field.column} = $${paramIndex}`);
        params.push(updates[field.key]);
        paramIndex++;
      }
    }

    if (sets.length === 0) return false;

    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE kb_documents SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  async deleteDocument(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM kb_documents WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  async publishDocument(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE kb_documents SET status = 'published', published_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  async archiveDocument(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE kb_documents SET status = 'archived', updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  async createDocumentVersion(documentId: string, userId: string): Promise<string | null> {
    const doc = await this.getDocument(documentId);
    if (!doc) return null;

    // Mark current version as not latest
    await this.execute(
      `UPDATE kb_documents SET is_latest_version = false, updated_at = NOW()
       WHERE workspace_id = $1 AND slug = $2 AND is_latest_version = true AND source_account_id = $3`,
      [doc.workspace_id, doc.slug, this.sourceAccountId]
    );

    const newId = crypto.randomUUID();
    const newVersion = doc.version + 1;

    await this.execute(
      `INSERT INTO kb_documents
       (id, source_account_id, workspace_id, collection_id, created_by,
        title, slug, content, content_html, excerpt,
        status, document_type, language,
        meta_title, meta_description, meta_keywords,
        tags, category, priority, version, parent_version_id, is_latest_version,
        visibility, required_role, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'draft', $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, true, $21, $22, NOW(), NOW())`,
      [
        newId, this.sourceAccountId, doc.workspace_id, doc.collection_id, userId,
        doc.title, doc.slug, doc.content, doc.content_html, doc.excerpt,
        doc.document_type, doc.language,
        doc.meta_title, doc.meta_description, doc.meta_keywords,
        doc.tags, doc.category, doc.priority, newVersion, documentId,
        doc.visibility, doc.required_role,
      ]
    );

    return newId;
  }

  async getDocumentVersions(workspaceId: string, slug: string): Promise<KBDocumentRecord[]> {
    const result = await this.query<KBDocumentRecord>(
      `SELECT * FROM kb_documents WHERE workspace_id = $1 AND slug = $2 AND source_account_id = $3
       ORDER BY version DESC`,
      [workspaceId, slug, this.sourceAccountId]
    );
    return result.rows;
  }

  async searchDocuments(workspaceId: string, queryText: string, limit = 20, offset = 0): Promise<KBSearchResult[]> {
    const result = await this.query<KBSearchResult>(
      `SELECT id, title, excerpt, slug, document_type,
              ts_rank(to_tsvector('english', title || ' ' || content || ' ' || COALESCE(excerpt, '')),
                      plainto_tsquery('english', $3)) AS rank
       FROM kb_documents
       WHERE workspace_id = $1 AND source_account_id = $2
         AND status = 'published' AND is_latest_version = true
         AND to_tsvector('english', title || ' ' || content || ' ' || COALESCE(excerpt, ''))
             @@ plainto_tsquery('english', $3)
       ORDER BY rank DESC
       LIMIT $4 OFFSET $5`,
      [workspaceId, this.sourceAccountId, queryText, limit, offset]
    );
    return result.rows;
  }

  async incrementDocumentViewCount(id: string): Promise<void> {
    await this.execute(
      `UPDATE kb_documents SET view_count = view_count + 1
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
  }

  async rateDocument(id: string, helpful: boolean): Promise<void> {
    if (helpful) {
      await this.execute(
        `UPDATE kb_documents SET helpful_count = helpful_count + 1, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [id, this.sourceAccountId]
      );
    } else {
      await this.execute(
        `UPDATE kb_documents SET not_helpful_count = not_helpful_count + 1, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [id, this.sourceAccountId]
      );
    }
  }

  // =========================================================================
  // Collection Operations
  // =========================================================================

  async createCollection(coll: CreateCollectionRequest): Promise<string> {
    const id = crypto.randomUUID();
    let depth = 0;
    let path: string[] = [];

    if (coll.parent_id) {
      const parent = await this.getCollection(coll.parent_id);
      if (parent) {
        depth = parent.depth + 1;
        path = [...(parent.path ?? []), parent.id];
      }
    }

    await this.execute(
      `INSERT INTO kb_collections
       (id, source_account_id, workspace_id, parent_id, created_by,
        name, slug, description, icon, color, display_order, path, depth,
        visibility, default_language, allowed_languages, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 0, $11, $12, $13, $14, $15, NOW(), NOW())`,
      [
        id, this.sourceAccountId, coll.workspace_id, coll.parent_id ?? null, coll.created_by,
        coll.name, coll.slug, coll.description ?? null, coll.icon ?? null, coll.color ?? null,
        path, depth, coll.visibility ?? 'private', coll.default_language ?? 'en', coll.allowed_languages ?? null,
      ]
    );
    return id;
  }

  async getCollection(id: string): Promise<KBCollectionRecord | null> {
    const result = await this.query<KBCollectionRecord>(
      `SELECT * FROM kb_collections WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listCollections(workspaceId: string, parentId?: string, visibility?: Visibility): Promise<KBCollectionRecord[]> {
    let sql = `SELECT * FROM kb_collections WHERE workspace_id = $1 AND source_account_id = $2`;
    const params: unknown[] = [workspaceId, this.sourceAccountId];
    let paramIndex = 3;

    if (parentId !== undefined) {
      if (parentId === '') {
        sql += ` AND parent_id IS NULL`;
      } else {
        sql += ` AND parent_id = $${paramIndex}`;
        params.push(parentId);
        paramIndex++;
      }
    }

    if (visibility) {
      sql += ` AND visibility = $${paramIndex}`;
      params.push(visibility);
      paramIndex++;
    }

    sql += ` ORDER BY display_order ASC, name ASC`;

    const result = await this.query<KBCollectionRecord>(sql, params);
    return result.rows;
  }

  async updateCollection(id: string, updates: UpdateCollectionRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    const fields: Array<{ key: keyof UpdateCollectionRequest; column: string }> = [
      { key: 'name', column: 'name' },
      { key: 'description', column: 'description' },
      { key: 'icon', column: 'icon' },
      { key: 'color', column: 'color' },
      { key: 'visibility', column: 'visibility' },
      { key: 'display_order', column: 'display_order' },
      { key: 'allowed_languages', column: 'allowed_languages' },
    ];

    for (const field of fields) {
      if (updates[field.key] !== undefined) {
        sets.push(`${field.column} = $${paramIndex}`);
        params.push(updates[field.key]);
        paramIndex++;
      }
    }

    if (sets.length === 0) return false;

    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE kb_collections SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  async deleteCollection(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM kb_collections WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // FAQ Operations
  // =========================================================================

  async createFaq(faq: CreateFaqRequest): Promise<string> {
    const id = crypto.randomUUID();
    const publishedAt = (faq.status ?? 'published') === 'published' ? new Date() : null;

    await this.execute(
      `INSERT INTO kb_faqs
       (id, source_account_id, workspace_id, collection_id, created_by,
        question, answer, answer_html, category, tags, status, language, published_at,
        created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())`,
      [
        id, this.sourceAccountId, faq.workspace_id, faq.collection_id ?? null, faq.created_by,
        faq.question, faq.answer, faq.answer_html ?? null, faq.category ?? null, faq.tags ?? null,
        faq.status ?? 'published', faq.language ?? 'en', publishedAt,
      ]
    );
    return id;
  }

  async getFaq(id: string): Promise<KBFaqRecord | null> {
    const result = await this.query<KBFaqRecord>(
      `SELECT * FROM kb_faqs WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listFaqs(workspaceId: string, limit = 100, offset = 0, filters?: {
    collection_id?: string;
    category?: string;
    status?: DocumentStatus;
    language?: string;
  }): Promise<KBFaqRecord[]> {
    let sql = `SELECT * FROM kb_faqs WHERE workspace_id = $1 AND source_account_id = $2`;
    const params: unknown[] = [workspaceId, this.sourceAccountId];
    let paramIndex = 3;

    if (filters?.collection_id) {
      sql += ` AND collection_id = $${paramIndex}`;
      params.push(filters.collection_id);
      paramIndex++;
    }
    if (filters?.category) {
      sql += ` AND category = $${paramIndex}`;
      params.push(filters.category);
      paramIndex++;
    }
    if (filters?.status) {
      sql += ` AND status = $${paramIndex}`;
      params.push(filters.status);
      paramIndex++;
    }
    if (filters?.language) {
      sql += ` AND language = $${paramIndex}`;
      params.push(filters.language);
      paramIndex++;
    }

    sql += ` ORDER BY display_order ASC, created_at DESC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<KBFaqRecord>(sql, params);
    return result.rows;
  }

  async updateFaq(id: string, updates: UpdateFaqRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    const fields: Array<{ key: keyof UpdateFaqRequest; column: string }> = [
      { key: 'question', column: 'question' },
      { key: 'answer', column: 'answer' },
      { key: 'answer_html', column: 'answer_html' },
      { key: 'category', column: 'category' },
      { key: 'tags', column: 'tags' },
      { key: 'status', column: 'status' },
      { key: 'display_order', column: 'display_order' },
    ];

    for (const field of fields) {
      if (updates[field.key] !== undefined) {
        sets.push(`${field.column} = $${paramIndex}`);
        params.push(updates[field.key]);
        paramIndex++;
      }
    }

    if (sets.length === 0) return false;

    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE kb_faqs SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  async deleteFaq(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM kb_faqs WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Attachment Operations
  // =========================================================================

  async createAttachment(att: CreateAttachmentRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO kb_attachments
       (id, source_account_id, workspace_id, document_id, uploaded_by,
        filename, original_filename, mime_type, file_size, storage_path,
        title, description, alt_text, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())`,
      [
        id, this.sourceAccountId, att.workspace_id, att.document_id ?? null, att.uploaded_by,
        att.filename, att.original_filename, att.mime_type, att.file_size, att.storage_path,
        att.title ?? null, att.description ?? null, att.alt_text ?? null,
      ]
    );
    return id;
  }

  async getAttachment(id: string): Promise<KBAttachmentRecord | null> {
    const result = await this.query<KBAttachmentRecord>(
      `SELECT * FROM kb_attachments WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listAttachments(documentId: string): Promise<KBAttachmentRecord[]> {
    const result = await this.query<KBAttachmentRecord>(
      `SELECT * FROM kb_attachments WHERE document_id = $1 AND source_account_id = $2
       ORDER BY created_at DESC`,
      [documentId, this.sourceAccountId]
    );
    return result.rows;
  }

  async deleteAttachment(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM kb_attachments WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Comment Operations
  // =========================================================================

  async createComment(comment: CreateCommentRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO kb_comments
       (id, source_account_id, workspace_id, document_id, user_id, parent_id,
        content, content_html, is_staff_reply, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
      [
        id, this.sourceAccountId, comment.workspace_id, comment.document_id, comment.user_id,
        comment.parent_id ?? null, comment.content, comment.content_html ?? null,
        comment.is_staff_reply ?? false,
      ]
    );
    return id;
  }

  async getComment(id: string): Promise<KBCommentRecord | null> {
    const result = await this.query<KBCommentRecord>(
      `SELECT * FROM kb_comments WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listComments(documentId: string, limit = 100, offset = 0, status?: string): Promise<KBCommentRecord[]> {
    let sql = `SELECT * FROM kb_comments WHERE document_id = $1 AND source_account_id = $2`;
    const params: unknown[] = [documentId, this.sourceAccountId];
    let paramIndex = 3;

    if (status) {
      sql += ` AND status = $${paramIndex}`;
      params.push(status);
      paramIndex++;
    }

    sql += ` ORDER BY created_at ASC LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`;
    params.push(limit, offset);

    const result = await this.query<KBCommentRecord>(sql, params);
    return result.rows;
  }

  async updateComment(id: string, updates: UpdateCommentRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    if (updates.content !== undefined) {
      sets.push(`content = $${paramIndex}`);
      params.push(updates.content);
      paramIndex++;
    }
    if (updates.content_html !== undefined) {
      sets.push(`content_html = $${paramIndex}`);
      params.push(updates.content_html);
      paramIndex++;
    }
    if (updates.status !== undefined) {
      sets.push(`status = $${paramIndex}`);
      params.push(updates.status);
      paramIndex++;
    }

    if (sets.length === 0) return false;

    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE kb_comments SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  async deleteComment(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM kb_comments WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  async markCommentHelpful(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE kb_comments SET helpful_count = helpful_count + 1, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Analytics Operations
  // =========================================================================

  async trackAnalyticsEvent(event: TrackAnalyticsEventRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO kb_analytics
       (id, source_account_id, workspace_id, document_id, faq_id,
        event_type, user_id, session_id, search_query,
        referrer, user_agent, ip_address, metadata, created_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())`,
      [
        id, this.sourceAccountId, event.workspace_id, event.document_id ?? null, event.faq_id ?? null,
        event.event_type, event.user_id ?? null, event.session_id ?? null, event.search_query ?? null,
        event.referrer ?? null, event.user_agent ?? null, event.ip_address ?? null,
        event.metadata ? JSON.stringify(event.metadata) : null,
      ]
    );
    return id;
  }

  async getPopularSearches(workspaceId: string, limit = 20): Promise<PopularSearch[]> {
    const result = await this.query<PopularSearch>(
      `SELECT search_query, COUNT(*) as search_count, COUNT(DISTINCT user_id) as unique_users
       FROM kb_analytics
       WHERE workspace_id = $1 AND source_account_id = $2
         AND event_type = 'search' AND search_query IS NOT NULL
       GROUP BY search_query
       ORDER BY search_count DESC
       LIMIT $3`,
      [workspaceId, this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async getDocumentAnalytics(documentId: string, startDate?: Date, endDate?: Date): Promise<KBAnalyticsRecord[]> {
    let sql = `SELECT * FROM kb_analytics WHERE document_id = $1 AND source_account_id = $2`;
    const params: unknown[] = [documentId, this.sourceAccountId];
    let paramIndex = 3;

    if (startDate) {
      sql += ` AND created_at >= $${paramIndex}`;
      params.push(startDate);
      paramIndex++;
    }
    if (endDate) {
      sql += ` AND created_at <= $${paramIndex}`;
      params.push(endDate);
      paramIndex++;
    }

    sql += ` ORDER BY created_at DESC`;

    const result = await this.query<KBAnalyticsRecord>(sql, params);
    return result.rows;
  }

  // =========================================================================
  // Translation Operations
  // =========================================================================

  async createTranslation(trans: CreateTranslationRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO kb_translations
       (id, source_account_id, workspace_id, source_document_id, source_faq_id,
        language, translated_by, translation_method,
        title, content, content_html, answer, answer_html, status, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())`,
      [
        id, this.sourceAccountId, trans.workspace_id,
        trans.source_document_id ?? null, trans.source_faq_id ?? null,
        trans.language, trans.translated_by ?? null, trans.translation_method ?? null,
        trans.title ?? null, trans.content ?? null, trans.content_html ?? null,
        trans.answer ?? null, trans.answer_html ?? null, trans.status ?? 'draft',
      ]
    );
    return id;
  }

  async getTranslation(id: string): Promise<KBTranslationRecord | null> {
    const result = await this.query<KBTranslationRecord>(
      `SELECT * FROM kb_translations WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listTranslations(filters: { document_id?: string; faq_id?: string; language?: string }): Promise<KBTranslationRecord[]> {
    let sql = `SELECT * FROM kb_translations WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.document_id) {
      sql += ` AND source_document_id = $${paramIndex}`;
      params.push(filters.document_id);
      paramIndex++;
    }
    if (filters.faq_id) {
      sql += ` AND source_faq_id = $${paramIndex}`;
      params.push(filters.faq_id);
      paramIndex++;
    }
    if (filters.language) {
      sql += ` AND language = $${paramIndex}`;
      params.push(filters.language);
      paramIndex++;
    }

    sql += ` ORDER BY created_at DESC`;

    const result = await this.query<KBTranslationRecord>(sql, params);
    return result.rows;
  }

  async updateTranslation(id: string, updates: UpdateTranslationRequest): Promise<boolean> {
    const sets: string[] = [];
    const params: unknown[] = [];
    let paramIndex = 1;

    const fields: Array<{ key: keyof UpdateTranslationRequest; column: string }> = [
      { key: 'title', column: 'title' },
      { key: 'content', column: 'content' },
      { key: 'content_html', column: 'content_html' },
      { key: 'answer', column: 'answer' },
      { key: 'answer_html', column: 'answer_html' },
      { key: 'status', column: 'status' },
      { key: 'quality_score', column: 'quality_score' },
    ];

    for (const field of fields) {
      if (updates[field.key] !== undefined) {
        sets.push(`${field.column} = $${paramIndex}`);
        params.push(updates[field.key]);
        paramIndex++;
      }
    }

    if (sets.length === 0) return false;

    sets.push('updated_at = NOW()');
    params.push(id, this.sourceAccountId);

    const result = await this.execute(
      `UPDATE kb_translations SET ${sets.join(', ')}
       WHERE id = $${paramIndex} AND source_account_id = $${paramIndex + 1}`,
      params
    );
    return result > 0;
  }

  // =========================================================================
  // Review Request Operations
  // =========================================================================

  async createReviewRequest(req: CreateReviewRequestRequest): Promise<string> {
    const id = crypto.randomUUID();
    await this.execute(
      `INSERT INTO kb_review_requests
       (id, source_account_id, workspace_id, document_id, requested_by, assigned_to,
        priority, due_date, created_at, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())`,
      [
        id, this.sourceAccountId, req.workspace_id, req.document_id, req.requested_by,
        req.assigned_to ?? null, req.priority ?? 'normal',
        req.due_date ? new Date(req.due_date) : null,
      ]
    );
    return id;
  }

  async getReviewRequest(id: string): Promise<KBReviewRequestRecord | null> {
    const result = await this.query<KBReviewRequestRecord>(
      `SELECT * FROM kb_review_requests WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listReviewRequests(workspaceId: string, status?: ReviewStatus): Promise<KBReviewRequestRecord[]> {
    let sql = `SELECT * FROM kb_review_requests WHERE workspace_id = $1 AND source_account_id = $2`;
    const params: unknown[] = [workspaceId, this.sourceAccountId];

    if (status) {
      sql += ` AND status = $3`;
      params.push(status);
    }

    sql += ` ORDER BY created_at DESC`;

    const result = await this.query<KBReviewRequestRecord>(sql, params);
    return result.rows;
  }

  async completeReview(id: string, approved: boolean, notes?: string): Promise<boolean> {
    const status = approved ? 'approved' : 'rejected';
    const result = await this.execute(
      `UPDATE kb_review_requests SET status = $1, review_notes = $2, completed_at = NOW(), updated_at = NOW()
       WHERE id = $3 AND source_account_id = $4`,
      [status, notes ?? null, id, this.sourceAccountId]
    );
    return result > 0;
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(workspaceId: string): Promise<KBStats> {
    const docs = await this.query<{ count: string; status: string }>(
      `SELECT status, COUNT(*) as count FROM kb_documents
       WHERE workspace_id = $1 AND source_account_id = $2 AND is_latest_version = true
       GROUP BY status`,
      [workspaceId, this.sourceAccountId]
    );

    const faqs = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_faqs WHERE workspace_id = $1 AND source_account_id = $2`,
      [workspaceId, this.sourceAccountId]
    );

    const collections = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_collections WHERE workspace_id = $1 AND source_account_id = $2`,
      [workspaceId, this.sourceAccountId]
    );

    const comments = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_comments WHERE workspace_id = $1 AND source_account_id = $2`,
      [workspaceId, this.sourceAccountId]
    );

    const views = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_analytics
       WHERE workspace_id = $1 AND source_account_id = $2 AND event_type = 'view'`,
      [workspaceId, this.sourceAccountId]
    );

    const searches = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_analytics
       WHERE workspace_id = $1 AND source_account_id = $2 AND event_type = 'search'`,
      [workspaceId, this.sourceAccountId]
    );

    const translations = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_translations WHERE workspace_id = $1 AND source_account_id = $2`,
      [workspaceId, this.sourceAccountId]
    );

    const pendingReviews = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM kb_review_requests
       WHERE workspace_id = $1 AND source_account_id = $2 AND status IN ('pending', 'in_progress')`,
      [workspaceId, this.sourceAccountId]
    );

    let totalDocs = 0;
    let publishedDocs = 0;
    let draftDocs = 0;
    for (const row of docs.rows) {
      const c = parseInt(row.count, 10);
      totalDocs += c;
      if (row.status === 'published') publishedDocs = c;
      if (row.status === 'draft') draftDocs = c;
    }

    return {
      total_documents: totalDocs,
      published_documents: publishedDocs,
      draft_documents: draftDocs,
      total_faqs: parseInt(faqs.rows[0]?.count ?? '0', 10),
      total_collections: parseInt(collections.rows[0]?.count ?? '0', 10),
      total_comments: parseInt(comments.rows[0]?.count ?? '0', 10),
      total_views: parseInt(views.rows[0]?.count ?? '0', 10),
      total_searches: parseInt(searches.rows[0]?.count ?? '0', 10),
      total_translations: parseInt(translations.rows[0]?.count ?? '0', 10),
      pending_reviews: parseInt(pendingReviews.rows[0]?.count ?? '0', 10),
    };
  }
}
