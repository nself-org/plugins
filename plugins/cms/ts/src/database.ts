/**
 * CMS Database Operations
 * Complete CRUD operations for all CMS objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ContentTypeRecord,
  CreateContentTypeInput,
  UpdateContentTypeInput,
  PostRecord,
  PostWithRelations,
  CreatePostInput,
  UpdatePostInput,
  ListPostsFilters,
  PostVersionRecord,
  CreateVersionInput,
  CategoryRecord,
  CategoryWithChildren,
  CreateCategoryInput,
  UpdateCategoryInput,
  TagRecord,
  CreateTagInput,
  WebhookEventRecord,
  CmsStats,
} from './types.js';

const logger = createLogger('cms:db');

export class CmsDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): CmsDatabase {
    return new CmsDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing CMS schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Content Types
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_content_types (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(64) NOT NULL,
        display_name VARCHAR(255),
        description TEXT,
        icon VARCHAR(32),
        fields JSONB DEFAULT '[]',
        settings JSONB DEFAULT '{}',
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );
      CREATE INDEX IF NOT EXISTS idx_cms_content_types_account ON cms_content_types(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cms_content_types_enabled ON cms_content_types(enabled);

      -- =====================================================================
      -- Posts
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_posts (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        content_type VARCHAR(64) NOT NULL DEFAULT 'post',
        title VARCHAR(500) NOT NULL,
        slug VARCHAR(500) NOT NULL,
        excerpt TEXT,
        body TEXT,
        body_format VARCHAR(16) DEFAULT 'markdown',
        author_id VARCHAR(255) NOT NULL,
        status VARCHAR(16) DEFAULT 'draft',
        visibility VARCHAR(16) DEFAULT 'public',
        featured_image_url TEXT,
        featured_image_alt TEXT,
        cover_image_url TEXT,
        is_featured BOOLEAN DEFAULT false,
        is_pinned BOOLEAN DEFAULT false,
        pinned_at TIMESTAMP WITH TIME ZONE,
        published_at TIMESTAMP WITH TIME ZONE,
        scheduled_at TIMESTAMP WITH TIME ZONE,
        reading_time_minutes INTEGER,
        word_count INTEGER,
        view_count INTEGER DEFAULT 0,
        comment_count INTEGER DEFAULT 0,
        custom_fields JSONB DEFAULT '{}',
        seo_title VARCHAR(255),
        seo_description TEXT,
        seo_keywords TEXT[],
        canonical_url TEXT,
        metadata JSONB DEFAULT '{}',
        deleted_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, slug)
      );
      CREATE INDEX IF NOT EXISTS idx_cms_posts_account ON cms_posts(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cms_posts_slug ON cms_posts(slug);
      CREATE INDEX IF NOT EXISTS idx_cms_posts_author ON cms_posts(author_id);
      CREATE INDEX IF NOT EXISTS idx_cms_posts_status ON cms_posts(status);
      CREATE INDEX IF NOT EXISTS idx_cms_posts_content_type ON cms_posts(content_type);
      CREATE INDEX IF NOT EXISTS idx_cms_posts_published_at ON cms_posts(published_at DESC);
      CREATE INDEX IF NOT EXISTS idx_cms_posts_featured ON cms_posts(is_featured) WHERE is_featured = true;
      CREATE INDEX IF NOT EXISTS idx_cms_posts_pinned ON cms_posts(is_pinned) WHERE is_pinned = true;
      CREATE INDEX IF NOT EXISTS idx_cms_posts_scheduled ON cms_posts(scheduled_at) WHERE scheduled_at IS NOT NULL AND status = 'scheduled';
      CREATE INDEX IF NOT EXISTS idx_cms_posts_deleted ON cms_posts(deleted_at) WHERE deleted_at IS NULL;

      -- =====================================================================
      -- Post Versions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_post_versions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        post_id UUID NOT NULL REFERENCES cms_posts(id) ON DELETE CASCADE,
        version INTEGER NOT NULL,
        title VARCHAR(500),
        body TEXT,
        body_format VARCHAR(16),
        custom_fields JSONB,
        change_summary TEXT,
        changed_by VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_cms_post_versions_post ON cms_post_versions(post_id, version DESC);
      CREATE INDEX IF NOT EXISTS idx_cms_post_versions_account ON cms_post_versions(source_account_id);

      -- =====================================================================
      -- Categories
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_categories (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        slug VARCHAR(255) NOT NULL,
        description TEXT,
        parent_id UUID REFERENCES cms_categories(id) ON DELETE SET NULL,
        sort_order INTEGER DEFAULT 0,
        post_count INTEGER DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, slug)
      );
      CREATE INDEX IF NOT EXISTS idx_cms_categories_account ON cms_categories(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cms_categories_slug ON cms_categories(slug);
      CREATE INDEX IF NOT EXISTS idx_cms_categories_parent ON cms_categories(parent_id);
      CREATE INDEX IF NOT EXISTS idx_cms_categories_sort ON cms_categories(sort_order);

      -- =====================================================================
      -- Tags
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_tags (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(128) NOT NULL,
        slug VARCHAR(128) NOT NULL,
        post_count INTEGER DEFAULT 0,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, slug)
      );
      CREATE INDEX IF NOT EXISTS idx_cms_tags_account ON cms_tags(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cms_tags_slug ON cms_tags(slug);

      -- =====================================================================
      -- Post Relations
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_post_categories (
        post_id UUID NOT NULL REFERENCES cms_posts(id) ON DELETE CASCADE,
        category_id UUID NOT NULL REFERENCES cms_categories(id) ON DELETE CASCADE,
        PRIMARY KEY(post_id, category_id)
      );
      CREATE INDEX IF NOT EXISTS idx_cms_post_categories_post ON cms_post_categories(post_id);
      CREATE INDEX IF NOT EXISTS idx_cms_post_categories_category ON cms_post_categories(category_id);

      CREATE TABLE IF NOT EXISTS cms_post_tags (
        post_id UUID NOT NULL REFERENCES cms_posts(id) ON DELETE CASCADE,
        tag_id UUID NOT NULL REFERENCES cms_tags(id) ON DELETE CASCADE,
        PRIMARY KEY(post_id, tag_id)
      );
      CREATE INDEX IF NOT EXISTS idx_cms_post_tags_post ON cms_post_tags(post_id);
      CREATE INDEX IF NOT EXISTS idx_cms_post_tags_tag ON cms_post_tags(tag_id);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS cms_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );
      CREATE INDEX IF NOT EXISTS idx_cms_webhook_events_account ON cms_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_cms_webhook_events_type ON cms_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_cms_webhook_events_processed ON cms_webhook_events(processed);
    `;

    await this.execute(schema);
    logger.info('Schema initialized successfully');

    // Seed default content types
    await this.seedDefaultContentTypes();
  }

  private async seedDefaultContentTypes(): Promise<void> {
    const defaultTypes = [
      {
        name: 'post',
        display_name: 'Blog Post',
        description: 'Standard blog post with rich content',
        icon: 'article',
      },
      {
        name: 'page',
        display_name: 'Page',
        description: 'Static page content',
        icon: 'description',
      },
      {
        name: 'recipe',
        display_name: 'Recipe',
        description: 'Recipe with ingredients and instructions',
        icon: 'restaurant',
      },
    ];

    for (const type of defaultTypes) {
      try {
        await this.execute(
          `INSERT INTO cms_content_types (source_account_id, name, display_name, description, icon)
           VALUES ($1, $2, $3, $4, $5)
           ON CONFLICT (source_account_id, name) DO NOTHING`,
          [this.sourceAccountId, type.name, type.display_name, type.description, type.icon]
        );
      } catch (error) {
        logger.warn(`Failed to seed content type ${type.name}`, { error });
      }
    }
  }

  // =========================================================================
  // Content Types
  // =========================================================================

  async createContentType(input: CreateContentTypeInput): Promise<ContentTypeRecord> {
    const result = await this.query<ContentTypeRecord>(
      `INSERT INTO cms_content_types (
        source_account_id, name, display_name, description, icon, fields, settings
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.name,
        input.display_name ?? null,
        input.description ?? null,
        input.icon ?? null,
        JSON.stringify(input.fields ?? []),
        JSON.stringify(input.settings ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getContentType(id: string): Promise<ContentTypeRecord | null> {
    const result = await this.query<ContentTypeRecord>(
      `SELECT * FROM cms_content_types WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getContentTypeByName(name: string): Promise<ContentTypeRecord | null> {
    const result = await this.query<ContentTypeRecord>(
      `SELECT * FROM cms_content_types WHERE name = $1 AND source_account_id = $2`,
      [name, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listContentTypes(): Promise<ContentTypeRecord[]> {
    const result = await this.query<ContentTypeRecord>(
      `SELECT * FROM cms_content_types WHERE source_account_id = $1 ORDER BY name`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async updateContentType(id: string, input: UpdateContentTypeInput): Promise<ContentTypeRecord | null> {
    const updates: string[] = [];
    const values: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (input.display_name !== undefined) {
      updates.push(`display_name = $${paramIndex++}`);
      values.push(input.display_name);
    }
    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      values.push(input.description);
    }
    if (input.icon !== undefined) {
      updates.push(`icon = $${paramIndex++}`);
      values.push(input.icon);
    }
    if (input.fields !== undefined) {
      updates.push(`fields = $${paramIndex++}`);
      values.push(JSON.stringify(input.fields));
    }
    if (input.settings !== undefined) {
      updates.push(`settings = $${paramIndex++}`);
      values.push(JSON.stringify(input.settings));
    }
    if (input.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex++}`);
      values.push(input.enabled);
    }

    if (updates.length === 0) {
      return this.getContentType(id);
    }

    updates.push(`updated_at = NOW()`);

    const result = await this.query<ContentTypeRecord>(
      `UPDATE cms_content_types SET ${updates.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteContentType(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM cms_content_types WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  // =========================================================================
  // Posts
  // =========================================================================

  generateSlug(title: string, maxLength = 200): string {
    return title
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '')
      .substring(0, maxLength);
  }

  async ensureUniqueSlug(baseSlug: string, excludePostId?: string): Promise<string> {
    let slug = baseSlug;
    let counter = 2;

    while (true) {
      const query = excludePostId
        ? `SELECT id FROM cms_posts WHERE slug = $1 AND source_account_id = $2 AND id != $3 AND deleted_at IS NULL`
        : `SELECT id FROM cms_posts WHERE slug = $1 AND source_account_id = $2 AND deleted_at IS NULL`;

      const params = excludePostId ? [slug, this.sourceAccountId, excludePostId] : [slug, this.sourceAccountId];
      const result = await this.query(query, params);

      if (result.rows.length === 0) {
        return slug;
      }

      slug = `${baseSlug}-${counter}`;
      counter++;
    }
  }

  calculateWordCount(text: string | null): number {
    if (!text) return 0;
    return text.split(/\s+/).filter(Boolean).length;
  }

  calculateReadingTime(wordCount: number): number {
    return Math.ceil(wordCount / 200);
  }

  async createPost(input: CreatePostInput): Promise<PostRecord> {
    const slug = input.slug
      ? await this.ensureUniqueSlug(this.generateSlug(input.slug))
      : await this.ensureUniqueSlug(this.generateSlug(input.title));

    const wordCount = this.calculateWordCount(input.body ?? null);
    const readingTime = this.calculateReadingTime(wordCount);

    const result = await this.query<PostRecord>(
      `INSERT INTO cms_posts (
        source_account_id, content_type, title, slug, excerpt, body, body_format,
        author_id, status, visibility, featured_image_url, featured_image_alt,
        cover_image_url, is_featured, is_pinned, scheduled_at, reading_time_minutes,
        word_count, custom_fields, seo_title, seo_description, seo_keywords,
        canonical_url, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.content_type ?? 'post',
        input.title,
        slug,
        input.excerpt ?? null,
        input.body ?? null,
        input.body_format ?? 'markdown',
        input.author_id,
        input.status ?? 'draft',
        input.visibility ?? 'public',
        input.featured_image_url ?? null,
        input.featured_image_alt ?? null,
        input.cover_image_url ?? null,
        input.is_featured ?? false,
        input.is_pinned ?? false,
        input.scheduled_at ?? null,
        readingTime,
        wordCount,
        JSON.stringify(input.custom_fields ?? {}),
        input.seo_title ?? null,
        input.seo_description ?? null,
        input.seo_keywords ?? null,
        input.canonical_url ?? null,
        JSON.stringify(input.metadata ?? {}),
      ]
    );

    const post = result.rows[0];

    // Set categories and tags if provided
    if (input.category_ids && input.category_ids.length > 0) {
      await this.setPostCategories(post.id, input.category_ids);
    }
    if (input.tag_ids && input.tag_ids.length > 0) {
      await this.setPostTags(post.id, input.tag_ids);
    }

    return post;
  }

  async getPost(id: string): Promise<PostWithRelations | null> {
    const result = await this.query<PostRecord>(
      `SELECT * FROM cms_posts WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );

    const post = result.rows[0];
    if (!post) return null;

    // Fetch categories and tags
    const categories = await this.getPostCategories(id);
    const tags = await this.getPostTags(id);

    return { ...post, categories, tags };
  }

  async getPostBySlug(slug: string): Promise<PostWithRelations | null> {
    const result = await this.query<PostRecord>(
      `SELECT * FROM cms_posts WHERE slug = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [slug, this.sourceAccountId]
    );

    const post = result.rows[0];
    if (!post) return null;

    // Fetch categories and tags
    const categories = await this.getPostCategories(post.id);
    const tags = await this.getPostTags(post.id);

    return { ...post, categories, tags };
  }

  async listPosts(filters: ListPostsFilters = {}): Promise<PostWithRelations[]> {
    const conditions: string[] = ['source_account_id = $1', 'deleted_at IS NULL'];
    const values: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (filters.status) {
      conditions.push(`status = $${paramIndex++}`);
      values.push(filters.status);
    }
    if (filters.content_type) {
      conditions.push(`content_type = $${paramIndex++}`);
      values.push(filters.content_type);
    }
    if (filters.author_id) {
      conditions.push(`author_id = $${paramIndex++}`);
      values.push(filters.author_id);
    }
    if (filters.is_featured !== undefined) {
      conditions.push(`is_featured = $${paramIndex++}`);
      values.push(filters.is_featured);
    }
    if (filters.is_pinned !== undefined) {
      conditions.push(`is_pinned = $${paramIndex++}`);
      values.push(filters.is_pinned);
    }
    if (filters.category_id) {
      conditions.push(`id IN (SELECT post_id FROM cms_post_categories WHERE category_id = $${paramIndex++})`);
      values.push(filters.category_id);
    }
    if (filters.tag_id) {
      conditions.push(`id IN (SELECT post_id FROM cms_post_tags WHERE tag_id = $${paramIndex++})`);
      values.push(filters.tag_id);
    }
    if (filters.search) {
      conditions.push(`(title ILIKE $${paramIndex} OR body ILIKE $${paramIndex})`);
      values.push(`%${filters.search}%`);
      paramIndex++;
    }

    const limit = filters.limit ?? 50;
    const offset = filters.offset ?? 0;

    const result = await this.query<PostRecord>(
      `SELECT * FROM cms_posts
       WHERE ${conditions.join(' AND ')}
       ORDER BY
         CASE WHEN is_pinned THEN pinned_at END DESC NULLS LAST,
         published_at DESC NULLS LAST,
         created_at DESC
       LIMIT $${paramIndex} OFFSET $${paramIndex + 1}`,
      [...values, limit, offset]
    );

    // Fetch categories and tags for each post
    const posts: PostWithRelations[] = [];
    for (const post of result.rows) {
      const categories = await this.getPostCategories(post.id);
      const tags = await this.getPostTags(post.id);
      posts.push({ ...post, categories, tags });
    }

    return posts;
  }

  async updatePost(id: string, input: UpdatePostInput): Promise<PostWithRelations | null> {
    const updates: string[] = [];
    const values: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (input.title !== undefined) {
      updates.push(`title = $${paramIndex++}`);
      values.push(input.title);
    }
    if (input.slug !== undefined) {
      const uniqueSlug = await this.ensureUniqueSlug(this.generateSlug(input.slug), id);
      updates.push(`slug = $${paramIndex++}`);
      values.push(uniqueSlug);
    }
    if (input.excerpt !== undefined) {
      updates.push(`excerpt = $${paramIndex++}`);
      values.push(input.excerpt);
    }
    if (input.body !== undefined) {
      updates.push(`body = $${paramIndex++}`);
      values.push(input.body);

      const wordCount = this.calculateWordCount(input.body);
      const readingTime = this.calculateReadingTime(wordCount);
      updates.push(`word_count = $${paramIndex++}`);
      values.push(wordCount);
      updates.push(`reading_time_minutes = $${paramIndex++}`);
      values.push(readingTime);
    }
    if (input.body_format !== undefined) {
      updates.push(`body_format = $${paramIndex++}`);
      values.push(input.body_format);
    }
    if (input.status !== undefined) {
      updates.push(`status = $${paramIndex++}`);
      values.push(input.status);
    }
    if (input.visibility !== undefined) {
      updates.push(`visibility = $${paramIndex++}`);
      values.push(input.visibility);
    }
    if (input.featured_image_url !== undefined) {
      updates.push(`featured_image_url = $${paramIndex++}`);
      values.push(input.featured_image_url);
    }
    if (input.featured_image_alt !== undefined) {
      updates.push(`featured_image_alt = $${paramIndex++}`);
      values.push(input.featured_image_alt);
    }
    if (input.cover_image_url !== undefined) {
      updates.push(`cover_image_url = $${paramIndex++}`);
      values.push(input.cover_image_url);
    }
    if (input.is_featured !== undefined) {
      updates.push(`is_featured = $${paramIndex++}`);
      values.push(input.is_featured);
    }
    if (input.is_pinned !== undefined) {
      updates.push(`is_pinned = $${paramIndex++}`);
      values.push(input.is_pinned);
      if (input.is_pinned) {
        updates.push(`pinned_at = NOW()`);
      }
    }
    if (input.pinned_at !== undefined) {
      updates.push(`pinned_at = $${paramIndex++}`);
      values.push(input.pinned_at);
    }
    if (input.scheduled_at !== undefined) {
      updates.push(`scheduled_at = $${paramIndex++}`);
      values.push(input.scheduled_at);
    }
    if (input.custom_fields !== undefined) {
      updates.push(`custom_fields = $${paramIndex++}`);
      values.push(JSON.stringify(input.custom_fields));
    }
    if (input.seo_title !== undefined) {
      updates.push(`seo_title = $${paramIndex++}`);
      values.push(input.seo_title);
    }
    if (input.seo_description !== undefined) {
      updates.push(`seo_description = $${paramIndex++}`);
      values.push(input.seo_description);
    }
    if (input.seo_keywords !== undefined) {
      updates.push(`seo_keywords = $${paramIndex++}`);
      values.push(input.seo_keywords);
    }
    if (input.canonical_url !== undefined) {
      updates.push(`canonical_url = $${paramIndex++}`);
      values.push(input.canonical_url);
    }
    if (input.metadata !== undefined) {
      updates.push(`metadata = $${paramIndex++}`);
      values.push(JSON.stringify(input.metadata));
    }

    if (updates.length === 0) {
      return this.getPost(id);
    }

    updates.push(`updated_at = NOW()`);

    // Create version before updating
    const currentPost = await this.getPost(id);
    if (currentPost) {
      await this.createVersion({
        post_id: currentPost.id,
        title: currentPost.title,
        body: currentPost.body,
        body_format: currentPost.body_format,
        custom_fields: currentPost.custom_fields,
        change_summary: 'Automatic version before update',
      });
    }

    const result = await this.query<PostRecord>(
      `UPDATE cms_posts SET ${updates.join(', ')}
       WHERE source_account_id = $1 AND id = $2 AND deleted_at IS NULL
       RETURNING *`,
      values
    );

    const post = result.rows[0];
    if (!post) return null;

    const categories = await this.getPostCategories(post.id);
    const tags = await this.getPostTags(post.id);

    return { ...post, categories, tags };
  }

  async deletePost(id: string, soft = true): Promise<boolean> {
    if (soft) {
      const result = await this.execute(
        `UPDATE cms_posts SET deleted_at = NOW() WHERE id = $1 AND source_account_id = $2`,
        [id, this.sourceAccountId]
      );
      return result > 0;
    } else {
      const result = await this.execute(
        `DELETE FROM cms_posts WHERE id = $1 AND source_account_id = $2`,
        [id, this.sourceAccountId]
      );
      return result > 0;
    }
  }

  async publishPost(id: string): Promise<PostRecord | null> {
    const result = await this.query<PostRecord>(
      `UPDATE cms_posts
       SET status = 'published', published_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async unpublishPost(id: string): Promise<PostRecord | null> {
    const result = await this.query<PostRecord>(
      `UPDATE cms_posts
       SET status = 'draft', updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async schedulePost(id: string, scheduledAt: Date): Promise<PostRecord | null> {
    const result = await this.query<PostRecord>(
      `UPDATE cms_posts
       SET status = 'scheduled', scheduled_at = $3, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [id, this.sourceAccountId, scheduledAt]
    );

    return result.rows[0] ?? null;
  }

  async duplicatePost(id: string): Promise<PostRecord | null> {
    const original = await this.getPost(id);
    if (!original) return null;

    const newSlug = await this.ensureUniqueSlug(`${original.slug}-copy`);

    const result = await this.query<PostRecord>(
      `INSERT INTO cms_posts (
        source_account_id, content_type, title, slug, excerpt, body, body_format,
        author_id, status, visibility, featured_image_url, featured_image_alt,
        cover_image_url, is_featured, is_pinned, reading_time_minutes,
        word_count, custom_fields, seo_title, seo_description, seo_keywords,
        canonical_url, metadata
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
      RETURNING *`,
      [
        this.sourceAccountId,
        original.content_type,
        `${original.title} (Copy)`,
        newSlug,
        original.excerpt,
        original.body,
        original.body_format,
        original.author_id,
        'draft',
        original.visibility,
        original.featured_image_url,
        original.featured_image_alt,
        original.cover_image_url,
        false,
        false,
        original.reading_time_minutes,
        original.word_count,
        JSON.stringify(original.custom_fields),
        original.seo_title,
        original.seo_description,
        original.seo_keywords,
        original.canonical_url,
        JSON.stringify(original.metadata),
      ]
    );

    const newPost = result.rows[0];

    // Copy categories and tags
    if (original.categories && original.categories.length > 0) {
      await this.setPostCategories(newPost.id, original.categories.map(c => c.id));
    }
    if (original.tags && original.tags.length > 0) {
      await this.setPostTags(newPost.id, original.tags.map(t => t.id));
    }

    return newPost;
  }

  async processScheduledPosts(): Promise<number> {
    const result = await this.execute(
      `UPDATE cms_posts
       SET status = 'published', published_at = NOW(), updated_at = NOW()
       WHERE source_account_id = $1
         AND status = 'scheduled'
         AND scheduled_at IS NOT NULL
         AND scheduled_at <= NOW()
         AND deleted_at IS NULL`,
      [this.sourceAccountId]
    );

    if (result > 0) {
      logger.info(`Published ${result} scheduled posts`);
    }

    return result;
  }

  // =========================================================================
  // Post Versions
  // =========================================================================

  async createVersion(input: CreateVersionInput): Promise<PostVersionRecord> {
    // Get next version number
    const versionResult = await this.query<{ max_version: number }>(
      `SELECT COALESCE(MAX(version), 0) as max_version
       FROM cms_post_versions
       WHERE post_id = $1 AND source_account_id = $2`,
      [input.post_id, this.sourceAccountId]
    );

    const nextVersion = (versionResult.rows[0]?.max_version ?? 0) + 1;

    const result = await this.query<PostVersionRecord>(
      `INSERT INTO cms_post_versions (
        source_account_id, post_id, version, title, body, body_format,
        custom_fields, change_summary, changed_by
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.post_id,
        nextVersion,
        input.title,
        input.body,
        input.body_format,
        JSON.stringify(input.custom_fields),
        input.change_summary ?? null,
        input.changed_by ?? null,
      ]
    );

    // Clean up old versions if exceeding max
    await this.cleanupOldVersions(input.post_id, 50);

    return result.rows[0];
  }

  async getPostVersions(postId: string): Promise<PostVersionRecord[]> {
    const result = await this.query<PostVersionRecord>(
      `SELECT * FROM cms_post_versions
       WHERE post_id = $1 AND source_account_id = $2
       ORDER BY version DESC`,
      [postId, this.sourceAccountId]
    );

    return result.rows;
  }

  async getPostVersion(postId: string, version: number): Promise<PostVersionRecord | null> {
    const result = await this.query<PostVersionRecord>(
      `SELECT * FROM cms_post_versions
       WHERE post_id = $1 AND version = $2 AND source_account_id = $3`,
      [postId, version, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async restorePostVersion(postId: string, version: number): Promise<PostRecord | null> {
    const versionRecord = await this.getPostVersion(postId, version);
    if (!versionRecord) return null;

    const result = await this.query<PostRecord>(
      `UPDATE cms_posts
       SET title = $3, body = $4, body_format = $5, custom_fields = $6, updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
       RETURNING *`,
      [
        postId,
        this.sourceAccountId,
        versionRecord.title,
        versionRecord.body,
        versionRecord.body_format,
        JSON.stringify(versionRecord.custom_fields),
      ]
    );

    return result.rows[0] ?? null;
  }

  private async cleanupOldVersions(postId: string, maxVersions: number): Promise<void> {
    await this.execute(
      `DELETE FROM cms_post_versions
       WHERE post_id = $1 AND source_account_id = $2
         AND version < (
           SELECT MAX(version) - $3
           FROM cms_post_versions
           WHERE post_id = $1 AND source_account_id = $2
         )`,
      [postId, this.sourceAccountId, maxVersions]
    );
  }

  // =========================================================================
  // Categories
  // =========================================================================

  async createCategory(input: CreateCategoryInput): Promise<CategoryRecord> {
    const slug = input.slug
      ? await this.ensureUniqueCategorySlug(this.generateSlug(input.slug))
      : await this.ensureUniqueCategorySlug(this.generateSlug(input.name));

    const result = await this.query<CategoryRecord>(
      `INSERT INTO cms_categories (
        source_account_id, name, slug, description, parent_id, sort_order
      ) VALUES ($1, $2, $3, $4, $5, $6)
      RETURNING *`,
      [
        this.sourceAccountId,
        input.name,
        slug,
        input.description ?? null,
        input.parent_id ?? null,
        input.sort_order ?? 0,
      ]
    );

    return result.rows[0];
  }

  async ensureUniqueCategorySlug(baseSlug: string, excludeCategoryId?: string): Promise<string> {
    let slug = baseSlug;
    let counter = 2;

    while (true) {
      const query = excludeCategoryId
        ? `SELECT id FROM cms_categories WHERE slug = $1 AND source_account_id = $2 AND id != $3`
        : `SELECT id FROM cms_categories WHERE slug = $1 AND source_account_id = $2`;

      const params = excludeCategoryId ? [slug, this.sourceAccountId, excludeCategoryId] : [slug, this.sourceAccountId];
      const result = await this.query(query, params);

      if (result.rows.length === 0) {
        return slug;
      }

      slug = `${baseSlug}-${counter}`;
      counter++;
    }
  }

  async getCategory(id: string): Promise<CategoryRecord | null> {
    const result = await this.query<CategoryRecord>(
      `SELECT * FROM cms_categories WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listCategories(): Promise<CategoryRecord[]> {
    const result = await this.query<CategoryRecord>(
      `SELECT * FROM cms_categories WHERE source_account_id = $1 ORDER BY sort_order, name`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async getCategoryTree(): Promise<CategoryWithChildren[]> {
    const allCategories = await this.listCategories();

    const categoryMap = new Map<string, CategoryWithChildren>();
    const rootCategories: CategoryWithChildren[] = [];

    // First pass: create all category objects
    for (const category of allCategories) {
      categoryMap.set(category.id, { ...category, children: [] });
    }

    // Second pass: build tree structure
    for (const category of allCategories) {
      const categoryWithChildren = categoryMap.get(category.id)!;

      if (category.parent_id) {
        const parent = categoryMap.get(category.parent_id);
        if (parent) {
          parent.children!.push(categoryWithChildren);
        } else {
          rootCategories.push(categoryWithChildren);
        }
      } else {
        rootCategories.push(categoryWithChildren);
      }
    }

    return rootCategories;
  }

  async updateCategory(id: string, input: UpdateCategoryInput): Promise<CategoryRecord | null> {
    const updates: string[] = [];
    const values: unknown[] = [this.sourceAccountId, id];
    let paramIndex = 3;

    if (input.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      values.push(input.name);
    }
    if (input.slug !== undefined) {
      const uniqueSlug = await this.ensureUniqueCategorySlug(this.generateSlug(input.slug), id);
      updates.push(`slug = $${paramIndex++}`);
      values.push(uniqueSlug);
    }
    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      values.push(input.description);
    }
    if (input.parent_id !== undefined) {
      updates.push(`parent_id = $${paramIndex++}`);
      values.push(input.parent_id);
    }
    if (input.sort_order !== undefined) {
      updates.push(`sort_order = $${paramIndex++}`);
      values.push(input.sort_order);
    }

    if (updates.length === 0) {
      return this.getCategory(id);
    }

    updates.push(`updated_at = NOW()`);

    const result = await this.query<CategoryRecord>(
      `UPDATE cms_categories SET ${updates.join(', ')}
       WHERE source_account_id = $1 AND id = $2
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteCategory(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM cms_categories WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async updateCategoryPostCounts(): Promise<void> {
    await this.execute(
      `UPDATE cms_categories c
       SET post_count = (
         SELECT COUNT(*)
         FROM cms_post_categories pc
         JOIN cms_posts p ON p.id = pc.post_id
         WHERE pc.category_id = c.id
           AND p.deleted_at IS NULL
       )
       WHERE c.source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Tags
  // =========================================================================

  async createTag(input: CreateTagInput): Promise<TagRecord> {
    const slug = input.slug
      ? await this.ensureUniqueTagSlug(this.generateSlug(input.slug))
      : await this.ensureUniqueTagSlug(this.generateSlug(input.name));

    const result = await this.query<TagRecord>(
      `INSERT INTO cms_tags (source_account_id, name, slug)
       VALUES ($1, $2, $3)
       RETURNING *`,
      [this.sourceAccountId, input.name, slug]
    );

    return result.rows[0];
  }

  async ensureUniqueTagSlug(baseSlug: string, excludeTagId?: string): Promise<string> {
    let slug = baseSlug;
    let counter = 2;

    while (true) {
      const query = excludeTagId
        ? `SELECT id FROM cms_tags WHERE slug = $1 AND source_account_id = $2 AND id != $3`
        : `SELECT id FROM cms_tags WHERE slug = $1 AND source_account_id = $2`;

      const params = excludeTagId ? [slug, this.sourceAccountId, excludeTagId] : [slug, this.sourceAccountId];
      const result = await this.query(query, params);

      if (result.rows.length === 0) {
        return slug;
      }

      slug = `${baseSlug}-${counter}`;
      counter++;
    }
  }

  async getTag(id: string): Promise<TagRecord | null> {
    const result = await this.query<TagRecord>(
      `SELECT * FROM cms_tags WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getTagBySlug(slug: string): Promise<TagRecord | null> {
    const result = await this.query<TagRecord>(
      `SELECT * FROM cms_tags WHERE slug = $1 AND source_account_id = $2`,
      [slug, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listTags(): Promise<TagRecord[]> {
    const result = await this.query<TagRecord>(
      `SELECT * FROM cms_tags WHERE source_account_id = $1 ORDER BY name`,
      [this.sourceAccountId]
    );

    return result.rows;
  }

  async deleteTag(id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM cms_tags WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async updateTagPostCounts(): Promise<void> {
    await this.execute(
      `UPDATE cms_tags t
       SET post_count = (
         SELECT COUNT(*)
         FROM cms_post_tags pt
         JOIN cms_posts p ON p.id = pt.post_id
         WHERE pt.tag_id = t.id
           AND p.deleted_at IS NULL
       )
       WHERE t.source_account_id = $1`,
      [this.sourceAccountId]
    );
  }

  // =========================================================================
  // Post Relations
  // =========================================================================

  async setPostCategories(postId: string, categoryIds: string[]): Promise<void> {
    // Remove existing categories
    await this.execute(
      `DELETE FROM cms_post_categories WHERE post_id = $1`,
      [postId]
    );

    // Add new categories
    for (const categoryId of categoryIds) {
      await this.execute(
        `INSERT INTO cms_post_categories (post_id, category_id)
         VALUES ($1, $2)
         ON CONFLICT DO NOTHING`,
        [postId, categoryId]
      );
    }

    await this.updateCategoryPostCounts();
  }

  async getPostCategories(postId: string): Promise<CategoryRecord[]> {
    const result = await this.query<CategoryRecord>(
      `SELECT c.*
       FROM cms_categories c
       JOIN cms_post_categories pc ON pc.category_id = c.id
       WHERE pc.post_id = $1
       ORDER BY c.name`,
      [postId]
    );

    return result.rows;
  }

  async setPostTags(postId: string, tagIds: string[]): Promise<void> {
    // Remove existing tags
    await this.execute(
      `DELETE FROM cms_post_tags WHERE post_id = $1`,
      [postId]
    );

    // Add new tags
    for (const tagId of tagIds) {
      await this.execute(
        `INSERT INTO cms_post_tags (post_id, tag_id)
         VALUES ($1, $2)
         ON CONFLICT DO NOTHING`,
        [postId, tagId]
      );
    }

    await this.updateTagPostCounts();
  }

  async getPostTags(postId: string): Promise<TagRecord[]> {
    const result = await this.query<TagRecord>(
      `SELECT t.*
       FROM cms_tags t
       JOIN cms_post_tags pt ON pt.tag_id = t.id
       WHERE pt.post_id = $1
       ORDER BY t.name`,
      [postId]
    );

    return result.rows;
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<CmsStats> {
    const result = await this.query<{
      content_types: string;
      posts: string;
      published_posts: string;
      draft_posts: string;
      scheduled_posts: string;
      categories: string;
      tags: string;
      total_word_count: string;
      total_views: string;
      last_published_at: Date | null;
    }>(
      `SELECT
        (SELECT COUNT(*) FROM cms_content_types WHERE source_account_id = $1 AND enabled = true) as content_types,
        (SELECT COUNT(*) FROM cms_posts WHERE source_account_id = $1 AND deleted_at IS NULL) as posts,
        (SELECT COUNT(*) FROM cms_posts WHERE source_account_id = $1 AND status = 'published' AND deleted_at IS NULL) as published_posts,
        (SELECT COUNT(*) FROM cms_posts WHERE source_account_id = $1 AND status = 'draft' AND deleted_at IS NULL) as draft_posts,
        (SELECT COUNT(*) FROM cms_posts WHERE source_account_id = $1 AND status = 'scheduled' AND deleted_at IS NULL) as scheduled_posts,
        (SELECT COUNT(*) FROM cms_categories WHERE source_account_id = $1) as categories,
        (SELECT COUNT(*) FROM cms_tags WHERE source_account_id = $1) as tags,
        (SELECT COALESCE(SUM(word_count), 0) FROM cms_posts WHERE source_account_id = $1 AND deleted_at IS NULL) as total_word_count,
        (SELECT COALESCE(SUM(view_count), 0) FROM cms_posts WHERE source_account_id = $1 AND deleted_at IS NULL) as total_views,
        (SELECT MAX(published_at) FROM cms_posts WHERE source_account_id = $1 AND status = 'published' AND deleted_at IS NULL) as last_published_at`,
      [this.sourceAccountId]
    );

    const row = result.rows[0];

    return {
      contentTypes: parseInt(row.content_types, 10),
      posts: parseInt(row.posts, 10),
      publishedPosts: parseInt(row.published_posts, 10),
      draftPosts: parseInt(row.draft_posts, 10),
      scheduledPosts: parseInt(row.scheduled_posts, 10),
      categories: parseInt(row.categories, 10),
      tags: parseInt(row.tags, 10),
      totalWordCount: parseInt(row.total_word_count, 10),
      totalViews: parseInt(row.total_views, 10),
      lastPublishedAt: row.last_published_at ?? null,
    };
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: Omit<WebhookEventRecord, 'created_at'>): Promise<void> {
    await this.execute(
      `INSERT INTO cms_webhook_events (id, source_account_id, event_type, payload, processed)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (id) DO NOTHING`,
      [event.id, this.sourceAccountId, event.event_type, JSON.stringify(event.payload), event.processed]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE cms_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [eventId, error ?? null]
    );
  }
}
