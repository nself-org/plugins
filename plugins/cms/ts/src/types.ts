/**
 * CMS Plugin Types
 * Complete type definitions for CMS objects
 */

export interface CmsPluginConfig {
  port: number;
  host: string;
  maxBodyLength: number;
  maxTitleLength: number;
  slugMaxLength: number;
  maxVersions: number;
  scheduledCheckIntervalMs: number;
  defaultContentTypes: string[];
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Content Types
// =============================================================================

export interface ContentTypeRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  display_name: string | null;
  description: string | null;
  icon: string | null;
  fields: ContentTypeField[];
  settings: Record<string, unknown>;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface ContentTypeField {
  name: string;
  type: 'text' | 'textarea' | 'markdown' | 'html' | 'number' | 'boolean' | 'date' | 'datetime' | 'select' | 'multiselect' | 'json';
  label: string;
  required?: boolean;
  default?: unknown;
  options?: string[];
  validation?: {
    min?: number;
    max?: number;
    pattern?: string;
  };
}

export interface CreateContentTypeInput {
  name: string;
  display_name?: string;
  description?: string;
  icon?: string;
  fields?: ContentTypeField[];
  settings?: Record<string, unknown>;
}

export interface UpdateContentTypeInput {
  display_name?: string;
  description?: string;
  icon?: string;
  fields?: ContentTypeField[];
  settings?: Record<string, unknown>;
  enabled?: boolean;
}

// =============================================================================
// Posts
// =============================================================================

export interface PostRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  content_type: string;
  title: string;
  slug: string;
  excerpt: string | null;
  body: string | null;
  body_format: 'markdown' | 'html' | 'plaintext';
  author_id: string;
  status: 'draft' | 'review' | 'scheduled' | 'published' | 'archived';
  visibility: 'public' | 'unlisted' | 'private';
  featured_image_url: string | null;
  featured_image_alt: string | null;
  cover_image_url: string | null;
  is_featured: boolean;
  is_pinned: boolean;
  pinned_at: Date | null;
  published_at: Date | null;
  scheduled_at: Date | null;
  reading_time_minutes: number | null;
  word_count: number | null;
  view_count: number;
  comment_count: number;
  custom_fields: Record<string, unknown>;
  seo_title: string | null;
  seo_description: string | null;
  seo_keywords: string[] | null;
  canonical_url: string | null;
  metadata: Record<string, unknown>;
  deleted_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface PostWithRelations extends PostRecord {
  categories?: CategoryRecord[];
  tags?: TagRecord[];
}

export interface CreatePostInput {
  content_type?: string;
  title: string;
  slug?: string;
  excerpt?: string;
  body?: string;
  body_format?: 'markdown' | 'html' | 'plaintext';
  author_id: string;
  status?: 'draft' | 'review' | 'scheduled' | 'published' | 'archived';
  visibility?: 'public' | 'unlisted' | 'private';
  featured_image_url?: string;
  featured_image_alt?: string;
  cover_image_url?: string;
  is_featured?: boolean;
  is_pinned?: boolean;
  scheduled_at?: Date | string;
  custom_fields?: Record<string, unknown>;
  seo_title?: string;
  seo_description?: string;
  seo_keywords?: string[];
  canonical_url?: string;
  metadata?: Record<string, unknown>;
  category_ids?: string[];
  tag_ids?: string[];
}

export interface UpdatePostInput {
  title?: string;
  slug?: string;
  excerpt?: string;
  body?: string;
  body_format?: 'markdown' | 'html' | 'plaintext';
  status?: 'draft' | 'review' | 'scheduled' | 'published' | 'archived';
  visibility?: 'public' | 'unlisted' | 'private';
  featured_image_url?: string;
  featured_image_alt?: string;
  cover_image_url?: string;
  is_featured?: boolean;
  is_pinned?: boolean;
  pinned_at?: Date | string | null;
  scheduled_at?: Date | string | null;
  custom_fields?: Record<string, unknown>;
  seo_title?: string;
  seo_description?: string;
  seo_keywords?: string[];
  canonical_url?: string;
  metadata?: Record<string, unknown>;
}

export interface ListPostsFilters {
  status?: 'draft' | 'review' | 'scheduled' | 'published' | 'archived';
  content_type?: string;
  author_id?: string;
  category_id?: string;
  tag_id?: string;
  is_featured?: boolean;
  is_pinned?: boolean;
  search?: string;
  limit?: number;
  offset?: number;
}

// =============================================================================
// Post Versions
// =============================================================================

export interface PostVersionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  post_id: string;
  version: number;
  title: string | null;
  body: string | null;
  body_format: string | null;
  custom_fields: Record<string, unknown> | null;
  change_summary: string | null;
  changed_by: string | null;
  created_at: Date;
}

export interface CreateVersionInput {
  post_id: string;
  title: string;
  body: string | null;
  body_format: string;
  custom_fields: Record<string, unknown>;
  change_summary?: string;
  changed_by?: string;
}

// =============================================================================
// Categories
// =============================================================================

export interface CategoryRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  slug: string;
  description: string | null;
  parent_id: string | null;
  sort_order: number;
  post_count: number;
  created_at: Date;
  updated_at: Date;
}

export interface CategoryWithChildren extends CategoryRecord {
  children?: CategoryWithChildren[];
}

export interface CreateCategoryInput {
  name: string;
  slug?: string;
  description?: string;
  parent_id?: string;
  sort_order?: number;
}

export interface UpdateCategoryInput {
  name?: string;
  slug?: string;
  description?: string;
  parent_id?: string | null;
  sort_order?: number;
}

// =============================================================================
// Tags
// =============================================================================

export interface TagRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  slug: string;
  post_count: number;
  created_at: Date;
}

export interface CreateTagInput {
  name: string;
  slug?: string;
}

// =============================================================================
// Post Relations
// =============================================================================

export interface PostCategoryRecord {
  post_id: string;
  category_id: string;
}

export interface PostTagRecord {
  post_id: string;
  tag_id: string;
}

// =============================================================================
// Webhook Events
// =============================================================================

export interface WebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Statistics
// =============================================================================

export interface CmsStats {
  contentTypes: number;
  posts: number;
  publishedPosts: number;
  draftPosts: number;
  scheduledPosts: number;
  categories: number;
  tags: number;
  totalWordCount: number;
  totalViews: number;
  lastPublishedAt?: Date | null;
}

// =============================================================================
// Feed
// =============================================================================

export interface FeedOptions {
  format?: 'rss' | 'atom';
  limit?: number;
}

// =============================================================================
// Slug Generation
// =============================================================================

export interface SlugOptions {
  maxLength?: number;
  suffix?: string;
}
