/**
 * Social Plugin Database Operations
 * Complete CRUD operations for all social objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  SocialPostRecord,
  SocialCommentRecord,
  SocialReactionRecord,
  SocialFollowRecord,
  SocialBookmarkRecord,
  SocialShareRecord,
  SocialStats,
  UserProfile,
  TrendingHashtag,
  CreatePostInput,
  UpdatePostInput,
  CreateCommentInput,
  UpdateCommentInput,
  CreateReactionInput,
  CreateFollowInput,
  CreateBookmarkInput,
  CreateShareInput,
  ListPostsOptions,
  ListCommentsOptions,
  ListReactionsOptions,
  ListFollowsOptions,
  ListBookmarksOptions,
  ReactionSummary,
} from './types.js';

const logger = createLogger('social:db');

export class SocialDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): SocialDatabase {
    return new SocialDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing social plugin schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Posts Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_posts (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        author_id VARCHAR(255) NOT NULL,
        content TEXT,
        content_type VARCHAR(32) DEFAULT 'text',
        attachments JSONB DEFAULT '[]',
        visibility VARCHAR(16) DEFAULT 'public',
        hashtags TEXT[] DEFAULT '{}',
        mentions TEXT[] DEFAULT '{}',
        location JSONB,
        comment_count INTEGER DEFAULT 0,
        reaction_count INTEGER DEFAULT 0,
        share_count INTEGER DEFAULT 0,
        bookmark_count INTEGER DEFAULT 0,
        is_pinned BOOLEAN DEFAULT false,
        edited_at TIMESTAMP WITH TIME ZONE,
        deleted_at TIMESTAMP WITH TIME ZONE,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_social_posts_source_account ON social_posts(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_posts_author ON social_posts(author_id);
      CREATE INDEX IF NOT EXISTS idx_social_posts_created ON social_posts(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_social_posts_visibility ON social_posts(visibility);
      CREATE INDEX IF NOT EXISTS idx_social_posts_hashtags ON social_posts USING GIN(hashtags);
      CREATE INDEX IF NOT EXISTS idx_social_posts_deleted ON social_posts(deleted_at) WHERE deleted_at IS NULL;

      -- =====================================================================
      -- Comments Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_comments (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        target_type VARCHAR(64) NOT NULL,
        target_id VARCHAR(255) NOT NULL,
        parent_id UUID REFERENCES social_comments(id) ON DELETE CASCADE,
        author_id VARCHAR(255) NOT NULL,
        content TEXT NOT NULL,
        mentions TEXT[] DEFAULT '{}',
        reaction_count INTEGER DEFAULT 0,
        reply_count INTEGER DEFAULT 0,
        depth INTEGER DEFAULT 0,
        edited_at TIMESTAMP WITH TIME ZONE,
        deleted_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_social_comments_source_account ON social_comments(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_comments_target ON social_comments(target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_social_comments_parent ON social_comments(parent_id);
      CREATE INDEX IF NOT EXISTS idx_social_comments_author ON social_comments(author_id);
      CREATE INDEX IF NOT EXISTS idx_social_comments_created ON social_comments(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_social_comments_deleted ON social_comments(deleted_at) WHERE deleted_at IS NULL;

      -- =====================================================================
      -- Reactions Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_reactions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        target_type VARCHAR(64) NOT NULL,
        target_id VARCHAR(255) NOT NULL,
        user_id VARCHAR(255) NOT NULL,
        reaction_type VARCHAR(32) DEFAULT '👍',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, target_type, target_id, user_id, reaction_type)
      );

      CREATE INDEX IF NOT EXISTS idx_social_reactions_source_account ON social_reactions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_reactions_target ON social_reactions(target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_social_reactions_user ON social_reactions(user_id);
      CREATE INDEX IF NOT EXISTS idx_social_reactions_type ON social_reactions(reaction_type);

      -- =====================================================================
      -- Follows Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_follows (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        follower_id VARCHAR(255) NOT NULL,
        following_type VARCHAR(32) DEFAULT 'user',
        following_id VARCHAR(255) NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, follower_id, following_type, following_id)
      );

      CREATE INDEX IF NOT EXISTS idx_social_follows_source_account ON social_follows(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_follows_follower ON social_follows(follower_id);
      CREATE INDEX IF NOT EXISTS idx_social_follows_following ON social_follows(following_type, following_id);

      -- =====================================================================
      -- Bookmarks Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_bookmarks (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        target_type VARCHAR(64) NOT NULL,
        target_id VARCHAR(255) NOT NULL,
        collection VARCHAR(128) DEFAULT 'default',
        note TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, target_type, target_id)
      );

      CREATE INDEX IF NOT EXISTS idx_social_bookmarks_source_account ON social_bookmarks(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_bookmarks_user ON social_bookmarks(user_id);
      CREATE INDEX IF NOT EXISTS idx_social_bookmarks_target ON social_bookmarks(target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_social_bookmarks_collection ON social_bookmarks(collection);

      -- =====================================================================
      -- Shares Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_shares (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        target_type VARCHAR(64) NOT NULL,
        target_id VARCHAR(255) NOT NULL,
        share_type VARCHAR(16) DEFAULT 'repost',
        message TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_social_shares_source_account ON social_shares(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_shares_user ON social_shares(user_id);
      CREATE INDEX IF NOT EXISTS idx_social_shares_target ON social_shares(target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_social_shares_type ON social_shares(share_type);

      -- =====================================================================
      -- Webhook Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS social_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_social_webhook_events_source_account ON social_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_social_webhook_events_type ON social_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_social_webhook_events_processed ON social_webhook_events(processed);
    `;

    await this.execute(schema);
    logger.info('Social schema initialized successfully');
  }

  // =========================================================================
  // Post Operations
  // =========================================================================

  async createPost(input: CreatePostInput): Promise<SocialPostRecord> {
    const result = await this.query<SocialPostRecord>(
      `INSERT INTO social_posts (
        source_account_id, author_id, content, content_type, attachments,
        visibility, hashtags, mentions, location, metadata, created_at, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        input.author_id,
        input.content ?? null,
        input.content_type ?? 'text',
        JSON.stringify(input.attachments ?? []),
        input.visibility ?? 'public',
        input.hashtags ?? [],
        input.mentions ?? [],
        input.location ? JSON.stringify(input.location) : null,
        JSON.stringify(input.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getPost(id: string): Promise<SocialPostRecord | null> {
    const result = await this.query<SocialPostRecord>(
      `SELECT * FROM social_posts
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listPosts(options: ListPostsOptions = {}): Promise<SocialPostRecord[]> {
    const { author_id, hashtag, visibility, limit = 100, offset = 0 } = options;

    let sql = `SELECT * FROM social_posts
               WHERE source_account_id = $1 AND deleted_at IS NULL`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (author_id) {
      sql += ` AND author_id = $${paramIndex++}`;
      params.push(author_id);
    }

    if (hashtag) {
      sql += ` AND $${paramIndex++} = ANY(hashtags)`;
      params.push(hashtag);
    }

    if (visibility) {
      sql += ` AND visibility = $${paramIndex++}`;
      params.push(visibility);
    }

    sql += ` ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`;
    params.push(limit, offset);

    const result = await this.query<SocialPostRecord>(sql, params);
    return result.rows;
  }

  async updatePost(id: string, input: UpdatePostInput): Promise<SocialPostRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.content !== undefined) {
      updates.push(`content = $${paramIndex++}`);
      params.push(input.content);
    }

    if (input.attachments !== undefined) {
      updates.push(`attachments = $${paramIndex++}`);
      params.push(JSON.stringify(input.attachments));
    }

    if (input.visibility !== undefined) {
      updates.push(`visibility = $${paramIndex++}`);
      params.push(input.visibility);
    }

    if (input.hashtags !== undefined) {
      updates.push(`hashtags = $${paramIndex++}`);
      params.push(input.hashtags);
    }

    if (input.mentions !== undefined) {
      updates.push(`mentions = $${paramIndex++}`);
      params.push(input.mentions);
    }

    if (input.location !== undefined) {
      updates.push(`location = $${paramIndex++}`);
      params.push(input.location ? JSON.stringify(input.location) : null);
    }

    if (input.is_pinned !== undefined) {
      updates.push(`is_pinned = $${paramIndex++}`);
      params.push(input.is_pinned);
    }

    if (input.metadata !== undefined) {
      updates.push(`metadata = $${paramIndex++}`);
      params.push(JSON.stringify(input.metadata));
    }

    if (updates.length === 0) {
      return this.getPost(id);
    }

    updates.push(`edited_at = NOW()`, `updated_at = NOW()`);

    const sql = `UPDATE social_posts SET ${updates.join(', ')}
                 WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
                 RETURNING *`;

    const result = await this.query<SocialPostRecord>(sql, params);
    return result.rows[0] ?? null;
  }

  async deletePost(id: string): Promise<boolean> {
    const result = await this.execute(
      `UPDATE social_posts SET deleted_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async countPosts(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_posts
       WHERE source_account_id = $1 AND deleted_at IS NULL`,
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  // =========================================================================
  // Comment Operations
  // =========================================================================

  async createComment(input: CreateCommentInput): Promise<SocialCommentRecord> {
    let depth = 0;

    if (input.parent_id) {
      const parent = await this.getComment(input.parent_id);
      if (parent) {
        depth = parent.depth + 1;
        // Update parent reply count
        await this.execute(
          `UPDATE social_comments SET reply_count = reply_count + 1, updated_at = NOW()
           WHERE id = $1 AND source_account_id = $2`,
          [input.parent_id, this.sourceAccountId]
        );
      }
    }

    const result = await this.query<SocialCommentRecord>(
      `INSERT INTO social_comments (
        source_account_id, target_type, target_id, parent_id, author_id,
        content, mentions, depth, created_at, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        input.target_type,
        input.target_id,
        input.parent_id ?? null,
        input.author_id,
        input.content,
        input.mentions ?? [],
        depth,
      ]
    );

    // Update target comment count if target is a post
    if (input.target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET comment_count = comment_count + 1, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [input.target_id, this.sourceAccountId]
      );
    }

    return result.rows[0];
  }

  async getComment(id: string): Promise<SocialCommentRecord | null> {
    const result = await this.query<SocialCommentRecord>(
      `SELECT * FROM social_comments
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listComments(options: ListCommentsOptions = {}): Promise<SocialCommentRecord[]> {
    const { target_type, target_id, author_id, parent_id, limit = 100, offset = 0 } = options;

    let sql = `SELECT * FROM social_comments
               WHERE source_account_id = $1 AND deleted_at IS NULL`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (target_type) {
      sql += ` AND target_type = $${paramIndex++}`;
      params.push(target_type);
    }

    if (target_id) {
      sql += ` AND target_id = $${paramIndex++}`;
      params.push(target_id);
    }

    if (author_id) {
      sql += ` AND author_id = $${paramIndex++}`;
      params.push(author_id);
    }

    if (parent_id !== undefined) {
      if (parent_id === null) {
        sql += ` AND parent_id IS NULL`;
      } else {
        sql += ` AND parent_id = $${paramIndex++}`;
        params.push(parent_id);
      }
    }

    sql += ` ORDER BY created_at ASC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`;
    params.push(limit, offset);

    const result = await this.query<SocialCommentRecord>(sql, params);
    return result.rows;
  }

  async updateComment(id: string, input: UpdateCommentInput): Promise<SocialCommentRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.content !== undefined) {
      updates.push(`content = $${paramIndex++}`);
      params.push(input.content);
    }

    if (input.mentions !== undefined) {
      updates.push(`mentions = $${paramIndex++}`);
      params.push(input.mentions);
    }

    if (updates.length === 0) {
      return this.getComment(id);
    }

    updates.push(`edited_at = NOW()`, `updated_at = NOW()`);

    const sql = `UPDATE social_comments SET ${updates.join(', ')}
                 WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL
                 RETURNING *`;

    const result = await this.query<SocialCommentRecord>(sql, params);
    return result.rows[0] ?? null;
  }

  async deleteComment(id: string): Promise<boolean> {
    const comment = await this.getComment(id);
    if (!comment) {
      return false;
    }

    const result = await this.execute(
      `UPDATE social_comments SET deleted_at = NOW(), updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2 AND deleted_at IS NULL`,
      [id, this.sourceAccountId]
    );

    // Update parent reply count
    if (comment.parent_id) {
      await this.execute(
        `UPDATE social_comments SET reply_count = GREATEST(reply_count - 1, 0), updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [comment.parent_id, this.sourceAccountId]
      );
    }

    // Update target comment count if target is a post
    if (comment.target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET comment_count = GREATEST(comment_count - 1, 0), updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [comment.target_id, this.sourceAccountId]
      );
    }

    return result > 0;
  }

  async countComments(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_comments
       WHERE source_account_id = $1 AND deleted_at IS NULL`,
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  // =========================================================================
  // Reaction Operations
  // =========================================================================

  async addReaction(input: CreateReactionInput): Promise<SocialReactionRecord> {
    const result = await this.query<SocialReactionRecord>(
      `INSERT INTO social_reactions (
        source_account_id, target_type, target_id, user_id, reaction_type, created_at
      ) VALUES ($1, $2, $3, $4, $5, NOW())
      ON CONFLICT (source_account_id, target_type, target_id, user_id, reaction_type)
      DO UPDATE SET created_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        input.target_type,
        input.target_id,
        input.user_id,
        input.reaction_type,
      ]
    );

    // Update target reaction count
    if (input.target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET reaction_count = (
          SELECT COUNT(*) FROM social_reactions
          WHERE target_type = 'post' AND target_id = $1 AND source_account_id = $2
        ), updated_at = NOW()
        WHERE id = $1 AND source_account_id = $2`,
        [input.target_id, this.sourceAccountId]
      );
    } else if (input.target_type === 'comment') {
      await this.execute(
        `UPDATE social_comments SET reaction_count = (
          SELECT COUNT(*) FROM social_reactions
          WHERE target_type = 'comment' AND target_id = $1 AND source_account_id = $2
        ), updated_at = NOW()
        WHERE id = $1 AND source_account_id = $2`,
        [input.target_id, this.sourceAccountId]
      );
    }

    return result.rows[0];
  }

  async removeReaction(target_type: string, target_id: string, user_id: string, reaction_type?: string): Promise<boolean> {
    let sql = `DELETE FROM social_reactions
               WHERE source_account_id = $1 AND target_type = $2 AND target_id = $3 AND user_id = $4`;
    const params: unknown[] = [this.sourceAccountId, target_type, target_id, user_id];

    if (reaction_type) {
      sql += ` AND reaction_type = $5`;
      params.push(reaction_type);
    }

    const result = await this.execute(sql, params);

    // Update target reaction count
    if (target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET reaction_count = (
          SELECT COUNT(*) FROM social_reactions
          WHERE target_type = 'post' AND target_id = $1 AND source_account_id = $2
        ), updated_at = NOW()
        WHERE id = $1 AND source_account_id = $2`,
        [target_id, this.sourceAccountId]
      );
    } else if (target_type === 'comment') {
      await this.execute(
        `UPDATE social_comments SET reaction_count = (
          SELECT COUNT(*) FROM social_reactions
          WHERE target_type = 'comment' AND target_id = $1 AND source_account_id = $2
        ), updated_at = NOW()
        WHERE id = $1 AND source_account_id = $2`,
        [target_id, this.sourceAccountId]
      );
    }

    return result > 0;
  }

  async getReactions(options: ListReactionsOptions): Promise<ReactionSummary[]> {
    let sql = `SELECT reaction_type, COUNT(*) as count, ARRAY_AGG(user_id) as users
               FROM social_reactions
               WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.target_type) {
      sql += ` AND target_type = $${paramIndex++}`;
      params.push(options.target_type);
    }

    if (options.target_id) {
      sql += ` AND target_id = $${paramIndex++}`;
      params.push(options.target_id);
    }

    if (options.user_id) {
      sql += ` AND user_id = $${paramIndex++}`;
      params.push(options.user_id);
    }

    if (options.reaction_type) {
      sql += ` AND reaction_type = $${paramIndex++}`;
      params.push(options.reaction_type);
    }

    sql += ` GROUP BY reaction_type ORDER BY count DESC`;

    const result = await this.query<{ reaction_type: string; count: string; users: string[] }>(sql, params);

    return result.rows.map(row => ({
      reaction_type: row.reaction_type,
      count: parseInt(row.count, 10),
      users: row.users,
    }));
  }

  async countReactions(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_reactions WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  // =========================================================================
  // Follow Operations
  // =========================================================================

  async createFollow(input: CreateFollowInput): Promise<SocialFollowRecord> {
    const result = await this.query<SocialFollowRecord>(
      `INSERT INTO social_follows (
        source_account_id, follower_id, following_type, following_id, created_at
      ) VALUES ($1, $2, $3, $4, NOW())
      ON CONFLICT (source_account_id, follower_id, following_type, following_id)
      DO UPDATE SET created_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        input.follower_id,
        input.following_type,
        input.following_id,
      ]
    );

    return result.rows[0];
  }

  async deleteFollow(follower_id: string, following_type: string, following_id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM social_follows
       WHERE source_account_id = $1 AND follower_id = $2 AND following_type = $3 AND following_id = $4`,
      [this.sourceAccountId, follower_id, following_type, following_id]
    );

    return result > 0;
  }

  async listFollows(options: ListFollowsOptions = {}): Promise<SocialFollowRecord[]> {
    let sql = `SELECT * FROM social_follows WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (options.follower_id) {
      sql += ` AND follower_id = $${paramIndex++}`;
      params.push(options.follower_id);
    }

    if (options.following_type) {
      sql += ` AND following_type = $${paramIndex++}`;
      params.push(options.following_type);
    }

    if (options.following_id) {
      sql += ` AND following_id = $${paramIndex++}`;
      params.push(options.following_id);
    }

    sql += ` ORDER BY created_at DESC`;

    const result = await this.query<SocialFollowRecord>(sql, params);
    return result.rows;
  }

  async checkFollow(follower_id: string, following_type: string, following_id: string): Promise<boolean> {
    const result = await this.query<{ exists: boolean }>(
      `SELECT EXISTS(
        SELECT 1 FROM social_follows
        WHERE source_account_id = $1 AND follower_id = $2 AND following_type = $3 AND following_id = $4
      ) as exists`,
      [this.sourceAccountId, follower_id, following_type, following_id]
    );

    return result.rows[0].exists;
  }

  async countFollows(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_follows WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  // =========================================================================
  // Bookmark Operations
  // =========================================================================

  async createBookmark(input: CreateBookmarkInput): Promise<SocialBookmarkRecord> {
    const result = await this.query<SocialBookmarkRecord>(
      `INSERT INTO social_bookmarks (
        source_account_id, user_id, target_type, target_id, collection, note, created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW())
      ON CONFLICT (source_account_id, user_id, target_type, target_id)
      DO UPDATE SET collection = EXCLUDED.collection, note = EXCLUDED.note, created_at = NOW()
      RETURNING *`,
      [
        this.sourceAccountId,
        input.user_id,
        input.target_type,
        input.target_id,
        input.collection ?? 'default',
        input.note ?? null,
      ]
    );

    // Update target bookmark count if target is a post
    if (input.target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET bookmark_count = (
          SELECT COUNT(*) FROM social_bookmarks
          WHERE target_type = 'post' AND target_id = $1 AND source_account_id = $2
        ), updated_at = NOW()
        WHERE id = $1 AND source_account_id = $2`,
        [input.target_id, this.sourceAccountId]
      );
    }

    return result.rows[0];
  }

  async deleteBookmark(user_id: string, target_type: string, target_id: string): Promise<boolean> {
    const result = await this.execute(
      `DELETE FROM social_bookmarks
       WHERE source_account_id = $1 AND user_id = $2 AND target_type = $3 AND target_id = $4`,
      [this.sourceAccountId, user_id, target_type, target_id]
    );

    // Update target bookmark count if target is a post
    if (target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET bookmark_count = (
          SELECT COUNT(*) FROM social_bookmarks
          WHERE target_type = 'post' AND target_id = $1 AND source_account_id = $2
        ), updated_at = NOW()
        WHERE id = $1 AND source_account_id = $2`,
        [target_id, this.sourceAccountId]
      );
    }

    return result > 0;
  }

  async listBookmarks(options: ListBookmarksOptions = {}): Promise<SocialBookmarkRecord[]> {
    const { user_id, target_type, collection, limit = 100, offset = 0 } = options;

    let sql = `SELECT * FROM social_bookmarks WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (user_id) {
      sql += ` AND user_id = $${paramIndex++}`;
      params.push(user_id);
    }

    if (target_type) {
      sql += ` AND target_type = $${paramIndex++}`;
      params.push(target_type);
    }

    if (collection) {
      sql += ` AND collection = $${paramIndex++}`;
      params.push(collection);
    }

    sql += ` ORDER BY created_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex++}`;
    params.push(limit, offset);

    const result = await this.query<SocialBookmarkRecord>(sql, params);
    return result.rows;
  }

  async countBookmarks(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_bookmarks WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  // =========================================================================
  // Share Operations
  // =========================================================================

  async createShare(input: CreateShareInput): Promise<SocialShareRecord> {
    const result = await this.query<SocialShareRecord>(
      `INSERT INTO social_shares (
        source_account_id, user_id, target_type, target_id, share_type, message, created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        input.user_id,
        input.target_type,
        input.target_id,
        input.share_type,
        input.message ?? null,
      ]
    );

    // Update target share count if target is a post
    if (input.target_type === 'post') {
      await this.execute(
        `UPDATE social_posts SET share_count = share_count + 1, updated_at = NOW()
         WHERE id = $1 AND source_account_id = $2`,
        [input.target_id, this.sourceAccountId]
      );
    }

    return result.rows[0];
  }

  async countShares(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_shares WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  // =========================================================================
  // Webhook Event Operations
  // =========================================================================

  async insertWebhookEvent(event_type: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO social_webhook_events (id, source_account_id, event_type, payload, created_at)
       VALUES ($1, $2, $3, $4, NOW())
       ON CONFLICT (id) DO NOTHING`,
      [`${event_type}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`, this.sourceAccountId, event_type, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE social_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1 AND source_account_id = $3`,
      [id, error ?? null, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Analytics and Statistics
  // =========================================================================

  async getStats(): Promise<SocialStats> {
    const [posts, comments, reactions, follows, bookmarks, shares] = await Promise.all([
      this.countPosts(),
      this.countComments(),
      this.countReactions(),
      this.countFollows(),
      this.countBookmarks(),
      this.countShares(),
    ]);

    return {
      posts,
      comments,
      reactions,
      follows,
      bookmarks,
      shares,
      lastUpdatedAt: new Date(),
    };
  }

  async getUserProfile(user_id: string): Promise<UserProfile> {
    const postCount = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_posts
       WHERE source_account_id = $1 AND author_id = $2 AND deleted_at IS NULL`,
      [this.sourceAccountId, user_id]
    );

    const followerCount = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_follows
       WHERE source_account_id = $1 AND following_type = 'user' AND following_id = $2`,
      [this.sourceAccountId, user_id]
    );

    const followingCount = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_follows
       WHERE source_account_id = $1 AND follower_id = $2`,
      [this.sourceAccountId, user_id]
    );

    const bookmarkCount = await this.query<{ count: string }>(
      `SELECT COUNT(*) as count FROM social_bookmarks
       WHERE source_account_id = $1 AND user_id = $2`,
      [this.sourceAccountId, user_id]
    );

    return {
      user_id,
      post_count: parseInt(postCount.rows[0].count, 10),
      follower_count: parseInt(followerCount.rows[0].count, 10),
      following_count: parseInt(followingCount.rows[0].count, 10),
      bookmark_count: parseInt(bookmarkCount.rows[0].count, 10),
    };
  }

  async getTrendingHashtags(limit = 10): Promise<TrendingHashtag[]> {
    const result = await this.query<{ hashtag: string; count: string; last_used: Date }>(
      `SELECT unnest(hashtags) as hashtag, COUNT(*) as count, MAX(created_at) as last_used
       FROM social_posts
       WHERE source_account_id = $1
         AND deleted_at IS NULL
         AND created_at > NOW() - INTERVAL '7 days'
       GROUP BY hashtag
       ORDER BY count DESC
       LIMIT $2`,
      [this.sourceAccountId, limit]
    );

    return result.rows.map(row => ({
      hashtag: row.hashtag,
      count: parseInt(row.count, 10),
      last_used: row.last_used,
    }));
  }
}
