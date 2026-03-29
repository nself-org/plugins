/**
 * Link Preview Database Operations
 * Complete CRUD operations for link previews, templates, oEmbed providers,
 * blocklist, settings, usage tracking, and analytics
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  LinkPreviewRecord,
  LinkPreviewUsageRecord,
  LinkPreviewTemplateRecord,
  OEmbedProviderRecord,
  UrlBlocklistRecord,
  LinkPreviewSettingsRecord,
  LinkPreviewAnalyticsRecord,
  UpsertPreviewRequest,
  CreateTemplateRequest,
  UpdateTemplateRequest,
  AddOEmbedProviderRequest,
  UpdateOEmbedProviderRequest,
  AddToBlocklistRequest,
  UpdateSettingsRequest,
  TrackUsageRequest,
  PreviewCacheStats,
  PopularPreview,
  SettingsScope,
  PreviewStatus,
} from './types.js';

const logger = createLogger('link-preview:db');

export class LinkPreviewDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): LinkPreviewDatabase {
    return new LinkPreviewDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing link preview schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
      CREATE EXTENSION IF NOT EXISTS "pgcrypto";

      -- =====================================================================
      -- Link Previews Cache
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_link_previews (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        url TEXT NOT NULL,
        url_hash VARCHAR(64) NOT NULL,
        title TEXT,
        description TEXT,
        image_url TEXT,
        video_url TEXT,
        audio_url TEXT,
        site_name TEXT,
        favicon_url TEXT,
        embed_html TEXT,
        embed_type VARCHAR(50),
        provider_name VARCHAR(255),
        provider_url TEXT,
        author_name VARCHAR(255),
        author_url TEXT,
        published_date TIMESTAMP WITH TIME ZONE,
        word_count INTEGER,
        reading_time_minutes INTEGER,
        tags TEXT[] DEFAULT '{}',
        language VARCHAR(10),
        metadata JSONB DEFAULT '{}',
        status VARCHAR(50) DEFAULT 'success',
        error_message TEXT,
        cache_expires_at TIMESTAMP WITH TIME ZONE,
        fetch_duration_ms INTEGER,
        http_status_code INTEGER,
        content_type VARCHAR(100),
        content_length BIGINT,
        is_safe BOOLEAN DEFAULT true,
        safety_check_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, url_hash)
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_source_account ON np_linkprev_link_previews(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_hash ON np_linkprev_link_previews(url_hash);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_url ON np_linkprev_link_previews(url);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_expires ON np_linkprev_link_previews(cache_expires_at);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_site ON np_linkprev_link_previews(site_name);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_created ON np_linkprev_link_previews(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_previews_status ON np_linkprev_link_previews(status);

      -- =====================================================================
      -- Link Preview Usage Tracking
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_link_preview_usage (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        preview_id UUID NOT NULL REFERENCES np_linkprev_link_previews(id) ON DELETE CASCADE,
        message_id VARCHAR(255),
        user_id VARCHAR(255),
        channel_id VARCHAR(255),
        clicked BOOLEAN DEFAULT false,
        clicked_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_usage_source_account ON np_linkprev_link_preview_usage(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_usage_preview ON np_linkprev_link_preview_usage(preview_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_usage_message ON np_linkprev_link_preview_usage(message_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_usage_user ON np_linkprev_link_preview_usage(user_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_usage_created ON np_linkprev_link_preview_usage(created_at DESC);

      -- =====================================================================
      -- Custom Preview Templates
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_preview_templates (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        url_pattern TEXT NOT NULL,
        priority INTEGER DEFAULT 0,
        template_html TEXT NOT NULL,
        css_styles TEXT,
        metadata_extractors JSONB DEFAULT '[]',
        is_active BOOLEAN DEFAULT true,
        created_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_templates_source_account ON np_linkprev_preview_templates(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_templates_active ON np_linkprev_preview_templates(is_active, priority DESC);

      -- =====================================================================
      -- oEmbed Provider Registry
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_oembed_providers (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        provider_name VARCHAR(255) NOT NULL,
        provider_url TEXT NOT NULL,
        endpoint_url TEXT NOT NULL,
        url_schemes TEXT[] NOT NULL DEFAULT '{}',
        formats VARCHAR(20)[] DEFAULT ARRAY['json'],
        discovery BOOLEAN DEFAULT true,
        max_width INTEGER,
        max_height INTEGER,
        is_active BOOLEAN DEFAULT true,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_oembed_source_account ON np_linkprev_oembed_providers(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_oembed_active ON np_linkprev_oembed_providers(is_active);

      -- =====================================================================
      -- URL Blocklist
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_url_blocklist (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        url_pattern TEXT NOT NULL,
        pattern_type VARCHAR(20) NOT NULL,
        reason VARCHAR(50) NOT NULL,
        description TEXT,
        added_by VARCHAR(255),
        expires_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_blocklist_source_account ON np_linkprev_url_blocklist(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_blocklist_pattern ON np_linkprev_url_blocklist(url_pattern);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_blocklist_expires ON np_linkprev_url_blocklist(expires_at) WHERE expires_at IS NOT NULL;

      -- =====================================================================
      -- Preview Settings (per-channel/user)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_preview_settings (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        scope VARCHAR(20) NOT NULL,
        scope_id VARCHAR(255),
        enabled BOOLEAN DEFAULT true,
        auto_expand BOOLEAN DEFAULT false,
        show_images BOOLEAN DEFAULT true,
        show_videos BOOLEAN DEFAULT true,
        max_previews_per_message INTEGER DEFAULT 3,
        preview_position VARCHAR(20) DEFAULT 'bottom',
        custom_css TEXT,
        blocked_domains TEXT[] DEFAULT '{}',
        allowed_domains TEXT[] DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, scope, scope_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_settings_source_account ON np_linkprev_preview_settings(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_settings_scope ON np_linkprev_preview_settings(scope, scope_id);

      -- =====================================================================
      -- Analytics
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS np_linkprev_preview_analytics (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        date DATE NOT NULL,
        preview_id UUID REFERENCES np_linkprev_link_previews(id) ON DELETE CASCADE,
        views_count INTEGER DEFAULT 0,
        clicks_count INTEGER DEFAULT 0,
        unique_users_count INTEGER DEFAULT 0,
        avg_click_rate DECIMAL(5,4),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, date, preview_id)
      );

      CREATE INDEX IF NOT EXISTS idx_np_linkprev_analytics_source_account ON np_linkprev_preview_analytics(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_analytics_date ON np_linkprev_preview_analytics(date DESC);
      CREATE INDEX IF NOT EXISTS idx_np_linkprev_analytics_preview ON np_linkprev_preview_analytics(preview_id);
    `;

    await this.db.execute(schema);
    logger.success('Link preview schema initialized');
  }

  // =========================================================================
  // Link Previews
  // =========================================================================

  async upsertPreview(data: UpsertPreviewRequest): Promise<LinkPreviewRecord> {
    const result = await this.db.query<LinkPreviewRecord>(
      `INSERT INTO np_linkprev_link_previews (
        source_account_id, url, url_hash, title, description, image_url,
        video_url, audio_url, site_name, favicon_url, embed_html, embed_type,
        provider_name, provider_url, author_name, author_url, published_date,
        word_count, reading_time_minutes, tags, language, metadata, status,
        error_message, fetch_duration_ms, http_status_code, content_type,
        content_length, is_safe
      ) VALUES (
        $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
        $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
        $24, $25, $26, $27, $28, $29
      )
      ON CONFLICT (source_account_id, url_hash) DO UPDATE SET
        title = COALESCE(EXCLUDED.title, np_linkprev_link_previews.title),
        description = COALESCE(EXCLUDED.description, np_linkprev_link_previews.description),
        image_url = COALESCE(EXCLUDED.image_url, np_linkprev_link_previews.image_url),
        video_url = COALESCE(EXCLUDED.video_url, np_linkprev_link_previews.video_url),
        audio_url = COALESCE(EXCLUDED.audio_url, np_linkprev_link_previews.audio_url),
        site_name = COALESCE(EXCLUDED.site_name, np_linkprev_link_previews.site_name),
        favicon_url = COALESCE(EXCLUDED.favicon_url, np_linkprev_link_previews.favicon_url),
        embed_html = COALESCE(EXCLUDED.embed_html, np_linkprev_link_previews.embed_html),
        embed_type = COALESCE(EXCLUDED.embed_type, np_linkprev_link_previews.embed_type),
        provider_name = COALESCE(EXCLUDED.provider_name, np_linkprev_link_previews.provider_name),
        provider_url = COALESCE(EXCLUDED.provider_url, np_linkprev_link_previews.provider_url),
        author_name = COALESCE(EXCLUDED.author_name, np_linkprev_link_previews.author_name),
        author_url = COALESCE(EXCLUDED.author_url, np_linkprev_link_previews.author_url),
        metadata = COALESCE(EXCLUDED.metadata, np_linkprev_link_previews.metadata),
        status = COALESCE(EXCLUDED.status, np_linkprev_link_previews.status),
        error_message = EXCLUDED.error_message,
        fetch_duration_ms = COALESCE(EXCLUDED.fetch_duration_ms, np_linkprev_link_previews.fetch_duration_ms),
        http_status_code = COALESCE(EXCLUDED.http_status_code, np_linkprev_link_previews.http_status_code),
        content_type = COALESCE(EXCLUDED.content_type, np_linkprev_link_previews.content_type),
        content_length = COALESCE(EXCLUDED.content_length, np_linkprev_link_previews.content_length),
        is_safe = COALESCE(EXCLUDED.is_safe, np_linkprev_link_previews.is_safe),
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        data.url,
        data.url_hash,
        data.title ?? null,
        data.description ?? null,
        data.image_url ?? null,
        data.video_url ?? null,
        data.audio_url ?? null,
        data.site_name ?? null,
        data.favicon_url ?? null,
        data.embed_html ?? null,
        data.embed_type ?? null,
        data.provider_name ?? null,
        data.provider_url ?? null,
        data.author_name ?? null,
        data.author_url ?? null,
        data.published_date ?? null,
        data.word_count ?? null,
        data.reading_time_minutes ?? null,
        data.tags ?? [],
        data.language ?? null,
        JSON.stringify(data.metadata ?? {}),
        data.status ?? 'success',
        data.error_message ?? null,
        data.fetch_duration_ms ?? null,
        data.http_status_code ?? null,
        data.content_type ?? null,
        data.content_length ?? null,
        data.is_safe ?? true,
      ]
    );
    return result.rows[0];
  }

  async getPreview(id: string): Promise<LinkPreviewRecord | null> {
    const result = await this.db.query<LinkPreviewRecord>(
      'SELECT * FROM np_linkprev_link_previews WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getPreviewByUrl(url: string): Promise<LinkPreviewRecord | null> {
    const result = await this.db.query<LinkPreviewRecord>(
      `SELECT * FROM np_linkprev_link_previews
       WHERE source_account_id = $1 AND url = $2
       AND (cache_expires_at IS NULL OR cache_expires_at > NOW())
       ORDER BY created_at DESC LIMIT 1`,
      [this.sourceAccountId, url]
    );
    return result.rows[0] ?? null;
  }

  async getPreviewByHash(urlHash: string): Promise<LinkPreviewRecord | null> {
    const result = await this.db.query<LinkPreviewRecord>(
      `SELECT * FROM np_linkprev_link_previews
       WHERE source_account_id = $1 AND url_hash = $2
       AND (cache_expires_at IS NULL OR cache_expires_at > NOW())`,
      [this.sourceAccountId, urlHash]
    );
    return result.rows[0] ?? null;
  }

  async listPreviews(limit = 100, offset = 0, status?: PreviewStatus): Promise<LinkPreviewRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (status) {
      conditions.push(`status = $${paramIndex++}`);
      params.push(status);
    }

    params.push(limit, offset);

    const result = await this.db.query<LinkPreviewRecord>(
      `SELECT * FROM np_linkprev_link_previews
       WHERE ${conditions.join(' AND ')}
       ORDER BY created_at DESC
       LIMIT $${paramIndex++} OFFSET $${paramIndex}`,
      params
    );
    return result.rows;
  }

  async deletePreview(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM np_linkprev_link_previews WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async getPreviewsForMessage(messageId: string): Promise<LinkPreviewRecord[]> {
    const result = await this.db.query<LinkPreviewRecord>(
      `SELECT lp.* FROM np_linkprev_link_previews lp
       INNER JOIN np_linkprev_link_preview_usage lpu ON lpu.preview_id = lp.id
       WHERE lpu.source_account_id = $1 AND lpu.message_id = $2
       ORDER BY lpu.created_at`,
      [this.sourceAccountId, messageId]
    );
    return result.rows;
  }

  async cleanupExpiredPreviews(): Promise<number> {
    const result = await this.db.query<{ count: string }>(
      `WITH deleted AS (
        DELETE FROM np_linkprev_link_previews
        WHERE source_account_id = $1
          AND cache_expires_at IS NOT NULL
          AND cache_expires_at < NOW()
        RETURNING id
      )
      SELECT COUNT(*) as count FROM deleted`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  async clearCache(): Promise<number> {
    const result = await this.db.query<{ count: string }>(
      `WITH deleted AS (
        DELETE FROM np_linkprev_link_previews
        WHERE source_account_id = $1
        RETURNING id
      )
      SELECT COUNT(*) as count FROM deleted`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  // =========================================================================
  // Usage Tracking
  // =========================================================================

  async trackUsage(data: TrackUsageRequest): Promise<LinkPreviewUsageRecord> {
    const result = await this.db.query<LinkPreviewUsageRecord>(
      `INSERT INTO np_linkprev_link_preview_usage (
        source_account_id, preview_id, message_id, user_id, channel_id
      ) VALUES ($1, $2, $3, $4, $5)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.preview_id,
        data.message_id ?? null,
        data.user_id ?? null,
        data.channel_id ?? null,
      ]
    );
    return result.rows[0];
  }

  async recordClick(usageId: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      `UPDATE np_linkprev_link_preview_usage
       SET clicked = true, clicked_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND clicked = false`,
      [usageId, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async getUsageForPreview(previewId: string, limit = 100): Promise<LinkPreviewUsageRecord[]> {
    const result = await this.db.query<LinkPreviewUsageRecord>(
      `SELECT * FROM np_linkprev_link_preview_usage
       WHERE source_account_id = $1 AND preview_id = $2
       ORDER BY created_at DESC LIMIT $3`,
      [this.sourceAccountId, previewId, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // Templates
  // =========================================================================

  async createTemplate(data: CreateTemplateRequest): Promise<LinkPreviewTemplateRecord> {
    const result = await this.db.query<LinkPreviewTemplateRecord>(
      `INSERT INTO np_linkprev_preview_templates (
        source_account_id, name, description, url_pattern, priority,
        template_html, css_styles, metadata_extractors
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.name,
        data.description ?? null,
        data.url_pattern,
        data.priority ?? 0,
        data.template_html,
        data.css_styles ?? null,
        JSON.stringify(data.metadata_extractors ?? []),
      ]
    );
    return result.rows[0];
  }

  async getTemplate(id: string): Promise<LinkPreviewTemplateRecord | null> {
    const result = await this.db.query<LinkPreviewTemplateRecord>(
      'SELECT * FROM np_linkprev_preview_templates WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listTemplates(limit = 100, offset = 0): Promise<LinkPreviewTemplateRecord[]> {
    const result = await this.db.query<LinkPreviewTemplateRecord>(
      `SELECT * FROM np_linkprev_preview_templates
       WHERE source_account_id = $1
       ORDER BY priority DESC, created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async updateTemplate(id: string, data: UpdateTemplateRequest): Promise<LinkPreviewTemplateRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      params.push(data.name);
    }
    if (data.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      params.push(data.description);
    }
    if (data.url_pattern !== undefined) {
      updates.push(`url_pattern = $${paramIndex++}`);
      params.push(data.url_pattern);
    }
    if (data.priority !== undefined) {
      updates.push(`priority = $${paramIndex++}`);
      params.push(data.priority);
    }
    if (data.template_html !== undefined) {
      updates.push(`template_html = $${paramIndex++}`);
      params.push(data.template_html);
    }
    if (data.css_styles !== undefined) {
      updates.push(`css_styles = $${paramIndex++}`);
      params.push(data.css_styles);
    }
    if (data.metadata_extractors !== undefined) {
      updates.push(`metadata_extractors = $${paramIndex++}::jsonb`);
      params.push(JSON.stringify(data.metadata_extractors));
    }
    if (data.is_active !== undefined) {
      updates.push(`is_active = $${paramIndex++}`);
      params.push(data.is_active);
    }

    if (updates.length === 0) {
      return this.getTemplate(id);
    }

    updates.push('updated_at = NOW()');

    const result = await this.db.query<LinkPreviewTemplateRecord>(
      `UPDATE np_linkprev_preview_templates SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteTemplate(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM np_linkprev_preview_templates WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async findMatchingTemplate(url: string): Promise<LinkPreviewTemplateRecord | null> {
    const templates = await this.db.query<LinkPreviewTemplateRecord>(
      `SELECT * FROM np_linkprev_preview_templates
       WHERE source_account_id = $1 AND is_active = true
       ORDER BY priority DESC`,
      [this.sourceAccountId]
    );

    for (const template of templates.rows) {
      try {
        const regex = new RegExp(template.url_pattern);
        if (regex.test(url)) {
          return template;
        }
      } catch {
        // Skip invalid regex patterns
      }
    }

    return null;
  }

  // =========================================================================
  // oEmbed Providers
  // =========================================================================

  async addOEmbedProvider(data: AddOEmbedProviderRequest): Promise<OEmbedProviderRecord> {
    const result = await this.db.query<OEmbedProviderRecord>(
      `INSERT INTO np_linkprev_oembed_providers (
        source_account_id, provider_name, provider_url, endpoint_url,
        url_schemes, formats, max_width, max_height
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.provider_name,
        data.provider_url,
        data.endpoint_url,
        data.url_schemes,
        data.formats ?? ['json'],
        data.max_width ?? null,
        data.max_height ?? null,
      ]
    );
    return result.rows[0];
  }

  async getOEmbedProvider(id: string): Promise<OEmbedProviderRecord | null> {
    const result = await this.db.query<OEmbedProviderRecord>(
      'SELECT * FROM np_linkprev_oembed_providers WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listOEmbedProviders(activeOnly = false): Promise<OEmbedProviderRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];

    if (activeOnly) {
      conditions.push('is_active = true');
    }

    const result = await this.db.query<OEmbedProviderRecord>(
      `SELECT * FROM np_linkprev_oembed_providers
       WHERE ${conditions.join(' AND ')}
       ORDER BY provider_name`,
      params
    );
    return result.rows;
  }

  async updateOEmbedProvider(id: string, data: UpdateOEmbedProviderRequest): Promise<OEmbedProviderRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (data.endpoint_url !== undefined) {
      updates.push(`endpoint_url = $${paramIndex++}`);
      params.push(data.endpoint_url);
    }
    if (data.url_schemes !== undefined) {
      updates.push(`url_schemes = $${paramIndex++}`);
      params.push(data.url_schemes);
    }
    if (data.formats !== undefined) {
      updates.push(`formats = $${paramIndex++}`);
      params.push(data.formats);
    }
    if (data.max_width !== undefined) {
      updates.push(`max_width = $${paramIndex++}`);
      params.push(data.max_width);
    }
    if (data.max_height !== undefined) {
      updates.push(`max_height = $${paramIndex++}`);
      params.push(data.max_height);
    }
    if (data.is_active !== undefined) {
      updates.push(`is_active = $${paramIndex++}`);
      params.push(data.is_active);
    }

    if (updates.length === 0) {
      return this.getOEmbedProvider(id);
    }

    updates.push('updated_at = NOW()');

    const result = await this.db.query<OEmbedProviderRecord>(
      `UPDATE np_linkprev_oembed_providers SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );
    return result.rows[0] ?? null;
  }

  async deleteOEmbedProvider(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM np_linkprev_oembed_providers WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async findOEmbedProvider(url: string): Promise<OEmbedProviderRecord | null> {
    const providers = await this.listOEmbedProviders(true);

    for (const provider of providers) {
      for (const scheme of provider.url_schemes) {
        try {
          const pattern = scheme.replace(/\*/g, '.*');
          const regex = new RegExp(`^${pattern}$`);
          if (regex.test(url)) {
            return provider;
          }
        } catch {
          // Skip invalid patterns
        }
      }
    }

    return null;
  }

  // =========================================================================
  // Blocklist
  // =========================================================================

  async addToBlocklist(data: AddToBlocklistRequest): Promise<UrlBlocklistRecord> {
    const result = await this.db.query<UrlBlocklistRecord>(
      `INSERT INTO np_linkprev_url_blocklist (
        source_account_id, url_pattern, pattern_type, reason,
        description, added_by, expires_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        data.url_pattern,
        data.pattern_type,
        data.reason,
        data.description ?? null,
        data.added_by ?? null,
        data.expires_at ?? null,
      ]
    );
    return result.rows[0];
  }

  async listBlocklist(limit = 100, offset = 0): Promise<UrlBlocklistRecord[]> {
    const result = await this.db.query<UrlBlocklistRecord>(
      `SELECT * FROM np_linkprev_url_blocklist
       WHERE source_account_id = $1
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async removeFromBlocklist(id: string): Promise<boolean> {
    const rowCount = await this.db.execute(
      'DELETE FROM np_linkprev_url_blocklist WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async isUrlBlocked(url: string): Promise<boolean> {
    // Extract domain from URL
    let domain: string;
    try {
      const parsed = new URL(url);
      domain = parsed.hostname;
    } catch {
      return false;
    }

    // Check exact match
    const exactResult = await this.db.query<{ exists: boolean }>(
      `SELECT EXISTS(
        SELECT 1 FROM np_linkprev_url_blocklist
        WHERE source_account_id = $1
          AND pattern_type = 'exact'
          AND url_pattern = $2
          AND (expires_at IS NULL OR expires_at > NOW())
      ) as exists`,
      [this.sourceAccountId, url]
    );
    if (exactResult.rows[0]?.exists) return true;

    // Check domain match
    const domainResult = await this.db.query<{ exists: boolean }>(
      `SELECT EXISTS(
        SELECT 1 FROM np_linkprev_url_blocklist
        WHERE source_account_id = $1
          AND pattern_type = 'domain'
          AND $2 LIKE '%' || url_pattern
          AND (expires_at IS NULL OR expires_at > NOW())
      ) as exists`,
      [this.sourceAccountId, domain]
    );
    if (domainResult.rows[0]?.exists) return true;

    // Check regex match
    const regexEntries = await this.db.query<UrlBlocklistRecord>(
      `SELECT * FROM np_linkprev_url_blocklist
       WHERE source_account_id = $1
         AND pattern_type = 'regex'
         AND (expires_at IS NULL OR expires_at > NOW())`,
      [this.sourceAccountId]
    );

    for (const entry of regexEntries.rows) {
      try {
        const regex = new RegExp(entry.url_pattern);
        if (regex.test(url)) return true;
      } catch {
        // Skip invalid regex
      }
    }

    return false;
  }

  // =========================================================================
  // Settings
  // =========================================================================

  async getSettings(scope: SettingsScope, scopeId?: string): Promise<LinkPreviewSettingsRecord | null> {
    const result = await this.db.query<LinkPreviewSettingsRecord>(
      `SELECT * FROM np_linkprev_preview_settings
       WHERE source_account_id = $1 AND scope = $2 AND scope_id IS NOT DISTINCT FROM $3`,
      [this.sourceAccountId, scope, scopeId ?? null]
    );
    return result.rows[0] ?? null;
  }

  async upsertSettings(data: UpdateSettingsRequest): Promise<LinkPreviewSettingsRecord> {
    const result = await this.db.query<LinkPreviewSettingsRecord>(
      `INSERT INTO np_linkprev_preview_settings (
        source_account_id, scope, scope_id, enabled, auto_expand,
        show_images, show_videos, max_previews_per_message,
        preview_position, custom_css, blocked_domains, allowed_domains
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      ON CONFLICT (source_account_id, scope, scope_id) DO UPDATE SET
        enabled = COALESCE(EXCLUDED.enabled, np_linkprev_preview_settings.enabled),
        auto_expand = COALESCE(EXCLUDED.auto_expand, np_linkprev_preview_settings.auto_expand),
        show_images = COALESCE(EXCLUDED.show_images, np_linkprev_preview_settings.show_images),
        show_videos = COALESCE(EXCLUDED.show_videos, np_linkprev_preview_settings.show_videos),
        max_previews_per_message = COALESCE(EXCLUDED.max_previews_per_message, np_linkprev_preview_settings.max_previews_per_message),
        preview_position = COALESCE(EXCLUDED.preview_position, np_linkprev_preview_settings.preview_position),
        custom_css = COALESCE(EXCLUDED.custom_css, np_linkprev_preview_settings.custom_css),
        blocked_domains = COALESCE(EXCLUDED.blocked_domains, np_linkprev_preview_settings.blocked_domains),
        allowed_domains = COALESCE(EXCLUDED.allowed_domains, np_linkprev_preview_settings.allowed_domains),
        updated_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        data.scope,
        data.scope_id ?? null,
        data.enabled ?? true,
        data.auto_expand ?? false,
        data.show_images ?? true,
        data.show_videos ?? true,
        data.max_previews_per_message ?? 3,
        data.preview_position ?? 'bottom',
        data.custom_css ?? null,
        data.blocked_domains ?? [],
        data.allowed_domains ?? [],
      ]
    );
    return result.rows[0];
  }

  // =========================================================================
  // Analytics
  // =========================================================================

  async updateAnalytics(previewId: string, date: string, viewed: boolean, clicked: boolean): Promise<void> {
    await this.db.execute(
      `INSERT INTO np_linkprev_preview_analytics (
        source_account_id, date, preview_id,
        views_count, clicks_count
      ) VALUES ($1, $2, $3, $4, $5)
      ON CONFLICT (source_account_id, date, preview_id) DO UPDATE SET
        views_count = np_linkprev_preview_analytics.views_count + $4,
        clicks_count = np_linkprev_preview_analytics.clicks_count + $5`,
      [
        this.sourceAccountId,
        date,
        previewId,
        viewed ? 1 : 0,
        clicked ? 1 : 0,
      ]
    );
  }

  async getAnalytics(startDate: string, endDate: string, previewId?: string): Promise<LinkPreviewAnalyticsRecord[]> {
    const conditions: string[] = ['source_account_id = $1', 'date >= $2', 'date <= $3'];
    const params: unknown[] = [this.sourceAccountId, startDate, endDate];
    let paramIndex = 4;

    if (previewId) {
      conditions.push(`preview_id = $${paramIndex++}`);
      params.push(previewId);
    }

    const result = await this.db.query<LinkPreviewAnalyticsRecord>(
      `SELECT * FROM np_linkprev_preview_analytics
       WHERE ${conditions.join(' AND ')}
       ORDER BY date DESC`,
      params
    );
    return result.rows;
  }

  async getPopularPreviews(limit = 20): Promise<PopularPreview[]> {
    const result = await this.db.query<PopularPreview>(
      `SELECT
        lp.*,
        COUNT(DISTINCT lpu.message_id) as usage_count,
        COUNT(DISTINCT lpu.user_id) as unique_users,
        COUNT(*) FILTER (WHERE lpu.clicked = true) as click_count,
        CASE
          WHEN COUNT(DISTINCT lpu.id) > 0
          THEN ROUND(COUNT(*) FILTER (WHERE lpu.clicked = true)::decimal / COUNT(DISTINCT lpu.id), 4)
          ELSE 0
        END as click_through_rate
       FROM np_linkprev_link_previews lp
       LEFT JOIN np_linkprev_link_preview_usage lpu ON lpu.preview_id = lp.id AND lpu.source_account_id = lp.source_account_id
       WHERE lp.source_account_id = $1
       GROUP BY lp.id
       HAVING COUNT(DISTINCT lpu.message_id) > 0
       ORDER BY usage_count DESC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );
    return result.rows;
  }

  // =========================================================================
  // Cache Statistics
  // =========================================================================

  async getCacheStats(): Promise<PreviewCacheStats> {
    const result = await this.db.query<{
      total_previews: string;
      successful: string;
      failed: string;
      expired: string;
      avg_fetch_duration_ms: string;
      oembed_count: string;
      unique_sites: string;
    }>(
      `SELECT
        COUNT(*) as total_previews,
        COUNT(*) FILTER (WHERE status = 'success') as successful,
        COUNT(*) FILTER (WHERE status = 'failed') as failed,
        COUNT(*) FILTER (WHERE cache_expires_at IS NOT NULL AND cache_expires_at < NOW()) as expired,
        COALESCE(AVG(fetch_duration_ms), 0) as avg_fetch_duration_ms,
        SUM(CASE WHEN embed_html IS NOT NULL THEN 1 ELSE 0 END) as oembed_count,
        COUNT(DISTINCT site_name) as unique_sites
       FROM np_linkprev_link_previews
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];
    return {
      total_previews: parseInt(row?.total_previews ?? '0', 10),
      successful: parseInt(row?.successful ?? '0', 10),
      failed: parseInt(row?.failed ?? '0', 10),
      expired: parseInt(row?.expired ?? '0', 10),
      avg_fetch_duration_ms: parseFloat(row?.avg_fetch_duration_ms ?? '0'),
      oembed_count: parseInt(row?.oembed_count ?? '0', 10),
      unique_sites: parseInt(row?.unique_sites ?? '0', 10),
    };
  }
}
