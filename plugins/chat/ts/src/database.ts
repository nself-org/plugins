/**
 * Chat Database Operations
 * Complete CRUD operations for all chat objects in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ConversationRecord,
  CreateConversationInput,
  UpdateConversationInput,
  ParticipantRecord,
  AddParticipantInput,
  UpdateParticipantInput,
  MessageRecord,
  SendMessageInput,
  UpdateMessageInput,
  ReadReceiptRecord,
  UpdateReadReceiptInput,
  ModerationActionRecord,
  CreateModerationActionInput,
  ChatStats,
  MessageSearchQuery,
  ConversationWithDetails,
  MessageReactions,
} from './types.js';

const logger = createLogger('chat:db');

export class ChatDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ChatDatabase {
    return new ChatDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing chat schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Conversations
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS chat_conversations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        type VARCHAR(16) NOT NULL DEFAULT 'direct',
        name VARCHAR(255),
        description TEXT,
        avatar_url TEXT,
        created_by VARCHAR(255),
        is_archived BOOLEAN DEFAULT false,
        is_muted BOOLEAN DEFAULT false,
        last_message_at TIMESTAMP WITH TIME ZONE,
        last_message_preview TEXT,
        message_count INTEGER DEFAULT 0,
        member_count INTEGER DEFAULT 0,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CHECK (type IN ('direct', 'group', 'channel'))
      );

      CREATE INDEX IF NOT EXISTS idx_chat_conversations_source_account
        ON chat_conversations(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_chat_conversations_type
        ON chat_conversations(type);
      CREATE INDEX IF NOT EXISTS idx_chat_conversations_created_by
        ON chat_conversations(created_by);
      CREATE INDEX IF NOT EXISTS idx_chat_conversations_created_at
        ON chat_conversations(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_chat_conversations_last_message_at
        ON chat_conversations(last_message_at DESC NULLS LAST);
      CREATE INDEX IF NOT EXISTS idx_chat_conversations_archived
        ON chat_conversations(is_archived) WHERE NOT is_archived;

      -- =====================================================================
      -- Participants
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS chat_participants (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        role VARCHAR(32) DEFAULT 'member',
        nickname VARCHAR(128),
        muted_until TIMESTAMP WITH TIME ZONE,
        joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        left_at TIMESTAMP WITH TIME ZONE,
        last_read_message_id UUID,
        last_read_at TIMESTAMP WITH TIME ZONE,
        notification_level VARCHAR(32) DEFAULT 'all',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(conversation_id, user_id),
        CHECK (role IN ('owner', 'admin', 'moderator', 'member', 'guest')),
        CHECK (notification_level IN ('all', 'mentions', 'none'))
      );

      CREATE INDEX IF NOT EXISTS idx_chat_participants_source_account
        ON chat_participants(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_chat_participants_conversation
        ON chat_participants(conversation_id);
      CREATE INDEX IF NOT EXISTS idx_chat_participants_user
        ON chat_participants(user_id);
      CREATE INDEX IF NOT EXISTS idx_chat_participants_joined
        ON chat_participants(joined_at DESC);
      CREATE INDEX IF NOT EXISTS idx_chat_participants_active
        ON chat_participants(conversation_id, user_id) WHERE left_at IS NULL;

      -- =====================================================================
      -- Messages
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS chat_messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
        sender_id VARCHAR(255) NOT NULL,
        reply_to_id UUID REFERENCES chat_messages(id),
        thread_root_id UUID REFERENCES chat_messages(id),
        content TEXT,
        content_type VARCHAR(32) DEFAULT 'text',
        attachments JSONB DEFAULT '[]',
        mentions TEXT[] DEFAULT '{}',
        reactions JSONB DEFAULT '{}',
        edited_at TIMESTAMP WITH TIME ZONE,
        deleted_at TIMESTAMP WITH TIME ZONE,
        is_pinned BOOLEAN DEFAULT false,
        pinned_at TIMESTAMP WITH TIME ZONE,
        pinned_by VARCHAR(255),
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CHECK (content_type IN ('text', 'image', 'file', 'audio', 'video', 'system', 'embed'))
      );

      CREATE INDEX IF NOT EXISTS idx_chat_messages_source_account
        ON chat_messages(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation
        ON chat_messages(conversation_id);
      CREATE INDEX IF NOT EXISTS idx_chat_messages_sender
        ON chat_messages(sender_id);
      CREATE INDEX IF NOT EXISTS idx_chat_messages_created_at
        ON chat_messages(created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_created
        ON chat_messages(conversation_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_chat_messages_thread_root
        ON chat_messages(thread_root_id) WHERE thread_root_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_chat_messages_reply_to
        ON chat_messages(reply_to_id) WHERE reply_to_id IS NOT NULL;
      CREATE INDEX IF NOT EXISTS idx_chat_messages_pinned
        ON chat_messages(conversation_id, is_pinned) WHERE is_pinned;
      CREATE INDEX IF NOT EXISTS idx_chat_messages_deleted
        ON chat_messages(deleted_at) WHERE deleted_at IS NOT NULL;

      -- =====================================================================
      -- Read Receipts
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS chat_read_receipts (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
        user_id VARCHAR(255) NOT NULL,
        last_read_message_id UUID REFERENCES chat_messages(id),
        last_read_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        unread_count INTEGER DEFAULT 0,
        UNIQUE(conversation_id, user_id)
      );

      CREATE INDEX IF NOT EXISTS idx_chat_read_receipts_source_account
        ON chat_read_receipts(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_chat_read_receipts_conversation
        ON chat_read_receipts(conversation_id);
      CREATE INDEX IF NOT EXISTS idx_chat_read_receipts_user
        ON chat_read_receipts(user_id);

      -- =====================================================================
      -- Moderation Actions
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS chat_moderation_actions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        conversation_id UUID REFERENCES chat_conversations(id),
        message_id UUID REFERENCES chat_messages(id),
        target_user_id VARCHAR(255),
        moderator_id VARCHAR(255) NOT NULL,
        action VARCHAR(32) NOT NULL,
        reason TEXT,
        duration_minutes INTEGER,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        CHECK (action IN ('delete_message', 'warn', 'mute', 'kick', 'ban'))
      );

      CREATE INDEX IF NOT EXISTS idx_chat_moderation_source_account
        ON chat_moderation_actions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_chat_moderation_conversation
        ON chat_moderation_actions(conversation_id);
      CREATE INDEX IF NOT EXISTS idx_chat_moderation_message
        ON chat_moderation_actions(message_id);
      CREATE INDEX IF NOT EXISTS idx_chat_moderation_target_user
        ON chat_moderation_actions(target_user_id);
      CREATE INDEX IF NOT EXISTS idx_chat_moderation_moderator
        ON chat_moderation_actions(moderator_id);
      CREATE INDEX IF NOT EXISTS idx_chat_moderation_created_at
        ON chat_moderation_actions(created_at DESC);

      -- =====================================================================
      -- Webhook Events
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS chat_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128) NOT NULL,
        payload JSONB NOT NULL,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_chat_webhook_events_source_account
        ON chat_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_chat_webhook_events_type
        ON chat_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_chat_webhook_events_processed
        ON chat_webhook_events(processed, created_at);
      CREATE INDEX IF NOT EXISTS idx_chat_webhook_events_created_at
        ON chat_webhook_events(created_at DESC);
    `;

    await this.db.execute(schema);
    logger.info('Chat schema initialized successfully');
  }

  // =========================================================================
  // Conversation Operations
  // =========================================================================

  async createConversation(input: CreateConversationInput): Promise<ConversationRecord> {
    const result = await this.query<ConversationRecord>(
      `INSERT INTO chat_conversations (
        source_account_id, type, name, description, avatar_url,
        created_by, metadata, created_at, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        input.type,
        input.name ?? null,
        input.description ?? null,
        input.avatar_url ?? null,
        input.created_by ?? null,
        JSON.stringify(input.metadata ?? {}),
      ]
    );

    const conversation = result.rows[0];

    // Add participants if provided
    if (input.participant_ids && input.participant_ids.length > 0) {
      for (const userId of input.participant_ids) {
        await this.addParticipant(conversation.id, {
          user_id: userId,
          role: userId === input.created_by ? 'owner' : 'member',
        });
      }
    }

    return conversation;
  }

  async getConversation(id: string): Promise<ConversationRecord | null> {
    const result = await this.query<ConversationRecord>(
      `SELECT * FROM chat_conversations
       WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getConversationWithDetails(id: string, userId?: string): Promise<ConversationWithDetails | null> {
    const conversation = await this.getConversation(id);
    if (!conversation) {
      return null;
    }

    const participants = await this.listParticipants(id);

    let unread_count: number | undefined;
    if (userId) {
      const receipt = await this.getReadReceipt(id, userId);
      unread_count = receipt?.unread_count ?? 0;
    }

    return {
      ...conversation,
      participants,
      unread_count,
    };
  }

  async updateConversation(id: string, input: UpdateConversationInput): Promise<ConversationRecord | null> {
    const updates: string[] = [];
    const values: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      values.push(input.name);
    }
    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      values.push(input.description);
    }
    if (input.avatar_url !== undefined) {
      updates.push(`avatar_url = $${paramIndex++}`);
      values.push(input.avatar_url);
    }
    if (input.is_archived !== undefined) {
      updates.push(`is_archived = $${paramIndex++}`);
      values.push(input.is_archived);
    }
    if (input.is_muted !== undefined) {
      updates.push(`is_muted = $${paramIndex++}`);
      values.push(input.is_muted);
    }
    if (input.metadata !== undefined) {
      updates.push(`metadata = $${paramIndex++}`);
      values.push(JSON.stringify(input.metadata));
    }

    if (updates.length === 0) {
      return this.getConversation(id);
    }

    updates.push(`updated_at = NOW()`);

    const result = await this.query<ConversationRecord>(
      `UPDATE chat_conversations SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async listConversations(userId?: string, limit = 50, offset = 0): Promise<ConversationRecord[]> {
    let sql: string;
    let params: unknown[];

    if (userId) {
      sql = `
        SELECT DISTINCT c.*
        FROM chat_conversations c
        INNER JOIN chat_participants p ON c.id = p.conversation_id
        WHERE c.source_account_id = $1
          AND p.user_id = $2
          AND p.left_at IS NULL
        ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC
        LIMIT $3 OFFSET $4
      `;
      params = [this.sourceAccountId, userId, limit, offset];
    } else {
      sql = `
        SELECT * FROM chat_conversations
        WHERE source_account_id = $1
        ORDER BY last_message_at DESC NULLS LAST, created_at DESC
        LIMIT $2 OFFSET $3
      `;
      params = [this.sourceAccountId, limit, offset];
    }

    const result = await this.query<ConversationRecord>(sql, params);
    return result.rows;
  }

  async countConversations(userId?: string): Promise<number> {
    let sql: string;
    let params: unknown[];

    if (userId) {
      sql = `
        SELECT COUNT(DISTINCT c.id)
        FROM chat_conversations c
        INNER JOIN chat_participants p ON c.id = p.conversation_id
        WHERE c.source_account_id = $1
          AND p.user_id = $2
          AND p.left_at IS NULL
      `;
      params = [this.sourceAccountId, userId];
    } else {
      sql = `SELECT COUNT(*) FROM chat_conversations WHERE source_account_id = $1`;
      params = [this.sourceAccountId];
    }

    const result = await this.query<{ count: string }>(sql, params);
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  async deleteConversation(id: string): Promise<boolean> {
    const rowCount = await this.execute(
      `DELETE FROM chat_conversations WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  // =========================================================================
  // Participant Operations
  // =========================================================================

  async addParticipant(conversationId: string, input: AddParticipantInput): Promise<ParticipantRecord> {
    const result = await this.query<ParticipantRecord>(
      `INSERT INTO chat_participants (
        source_account_id, conversation_id, user_id, role, nickname,
        notification_level, joined_at, created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
      ON CONFLICT (conversation_id, user_id)
      DO UPDATE SET
        left_at = NULL,
        role = EXCLUDED.role,
        nickname = EXCLUDED.nickname,
        notification_level = EXCLUDED.notification_level
      RETURNING *`,
      [
        this.sourceAccountId,
        conversationId,
        input.user_id,
        input.role ?? 'member',
        input.nickname ?? null,
        input.notification_level ?? 'all',
      ]
    );

    // Update member count
    await this.updateConversationMemberCount(conversationId);

    return result.rows[0];
  }

  async updateParticipant(
    conversationId: string,
    userId: string,
    input: UpdateParticipantInput
  ): Promise<ParticipantRecord | null> {
    const updates: string[] = [];
    const values: unknown[] = [conversationId, userId, this.sourceAccountId];
    let paramIndex = 4;

    if (input.role !== undefined) {
      updates.push(`role = $${paramIndex++}`);
      values.push(input.role);
    }
    if (input.nickname !== undefined) {
      updates.push(`nickname = $${paramIndex++}`);
      values.push(input.nickname);
    }
    if (input.muted_until !== undefined) {
      updates.push(`muted_until = $${paramIndex++}`);
      values.push(input.muted_until);
    }
    if (input.notification_level !== undefined) {
      updates.push(`notification_level = $${paramIndex++}`);
      values.push(input.notification_level);
    }

    if (updates.length === 0) {
      return this.getParticipant(conversationId, userId);
    }

    const result = await this.query<ParticipantRecord>(
      `UPDATE chat_participants SET ${updates.join(', ')}
       WHERE conversation_id = $1 AND user_id = $2 AND source_account_id = $3
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async removeParticipant(conversationId: string, userId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `UPDATE chat_participants
       SET left_at = NOW()
       WHERE conversation_id = $1 AND user_id = $2 AND source_account_id = $3 AND left_at IS NULL`,
      [conversationId, userId, this.sourceAccountId]
    );

    if (rowCount > 0) {
      await this.updateConversationMemberCount(conversationId);
    }

    return rowCount > 0;
  }

  async getParticipant(conversationId: string, userId: string): Promise<ParticipantRecord | null> {
    const result = await this.query<ParticipantRecord>(
      `SELECT * FROM chat_participants
       WHERE conversation_id = $1 AND user_id = $2 AND source_account_id = $3`,
      [conversationId, userId, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async listParticipants(conversationId: string, activeOnly = true): Promise<ParticipantRecord[]> {
    let sql = `
      SELECT * FROM chat_participants
      WHERE conversation_id = $1 AND source_account_id = $2
    `;

    if (activeOnly) {
      sql += ` AND left_at IS NULL`;
    }

    sql += ` ORDER BY joined_at ASC`;

    const result = await this.query<ParticipantRecord>(sql, [conversationId, this.sourceAccountId]);
    return result.rows;
  }

  private async updateConversationMemberCount(conversationId: string): Promise<void> {
    await this.execute(
      `UPDATE chat_conversations
       SET member_count = (
         SELECT COUNT(*) FROM chat_participants
         WHERE conversation_id = $1 AND left_at IS NULL
       )
       WHERE id = $1 AND source_account_id = $2`,
      [conversationId, this.sourceAccountId]
    );
  }

  // =========================================================================
  // Message Operations
  // =========================================================================

  async sendMessage(conversationId: string, input: SendMessageInput): Promise<MessageRecord> {
    const result = await this.query<MessageRecord>(
      `INSERT INTO chat_messages (
        source_account_id, conversation_id, sender_id, content, content_type,
        reply_to_id, thread_root_id, attachments, mentions, metadata, created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        conversationId,
        input.sender_id,
        input.content ?? null,
        input.content_type ?? 'text',
        input.reply_to_id ?? null,
        input.thread_root_id ?? null,
        JSON.stringify(input.attachments ?? []),
        input.mentions ?? [],
        JSON.stringify(input.metadata ?? {}),
      ]
    );

    const message = result.rows[0];

    // Update conversation last message
    await this.updateConversationLastMessage(conversationId, message);

    // Update unread counts for all participants except sender
    await this.updateUnreadCounts(conversationId, input.sender_id);

    return message;
  }

  async getMessage(id: string): Promise<MessageRecord | null> {
    const result = await this.query<MessageRecord>(
      `SELECT * FROM chat_messages WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async updateMessage(id: string, senderId: string, input: UpdateMessageInput): Promise<MessageRecord | null> {
    const message = await this.getMessage(id);
    if (!message || message.sender_id !== senderId) {
      return null;
    }

    // Store original content in metadata if not already stored
    const metadata = message.metadata as Record<string, unknown>;
    if (!metadata.original_content && message.content) {
      metadata.original_content = message.content;
    }

    const updates: string[] = [];
    const values: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.content !== undefined) {
      updates.push(`content = $${paramIndex++}`);
      values.push(input.content);
    }
    if (input.attachments !== undefined) {
      updates.push(`attachments = $${paramIndex++}`);
      values.push(JSON.stringify(input.attachments));
    }
    if (input.mentions !== undefined) {
      updates.push(`mentions = $${paramIndex++}`);
      values.push(input.mentions);
    }
    if (input.metadata !== undefined) {
      updates.push(`metadata = $${paramIndex++}`);
      values.push(JSON.stringify({ ...metadata, ...input.metadata }));
    }

    if (updates.length === 0) {
      return message;
    }

    updates.push(`edited_at = NOW()`);
    updates.push(`metadata = $${paramIndex++}`);
    values.push(JSON.stringify(metadata));

    const result = await this.query<MessageRecord>(
      `UPDATE chat_messages SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      values
    );

    return result.rows[0] ?? null;
  }

  async deleteMessage(id: string, senderId: string): Promise<boolean> {
    const rowCount = await this.execute(
      `UPDATE chat_messages
       SET deleted_at = NOW(), content = '[deleted]'
       WHERE id = $1 AND sender_id = $2 AND source_account_id = $3 AND deleted_at IS NULL`,
      [id, senderId, this.sourceAccountId]
    );
    return rowCount > 0;
  }

  async listMessages(conversationId: string, limit = 50, beforeCursor?: string): Promise<MessageRecord[]> {
    let sql = `
      SELECT * FROM chat_messages
      WHERE conversation_id = $1 AND source_account_id = $2
    `;
    const params: unknown[] = [conversationId, this.sourceAccountId];

    if (beforeCursor) {
      sql += ` AND created_at < (SELECT created_at FROM chat_messages WHERE id = $3)`;
      params.push(beforeCursor);
    }

    sql += ` ORDER BY created_at DESC LIMIT $${params.length + 1}`;
    params.push(limit);

    const result = await this.query<MessageRecord>(sql, params);
    return result.rows;
  }

  async listThreadMessages(threadRootId: string, limit = 50): Promise<MessageRecord[]> {
    const result = await this.query<MessageRecord>(
      `SELECT * FROM chat_messages
       WHERE thread_root_id = $1 AND source_account_id = $2
       ORDER BY created_at ASC
       LIMIT $3`,
      [threadRootId, this.sourceAccountId, limit]
    );
    return result.rows;
  }

  async listPinnedMessages(conversationId: string): Promise<MessageRecord[]> {
    const result = await this.query<MessageRecord>(
      `SELECT * FROM chat_messages
       WHERE conversation_id = $1 AND source_account_id = $2 AND is_pinned = true
       ORDER BY pinned_at DESC`,
      [conversationId, this.sourceAccountId]
    );
    return result.rows;
  }

  async pinMessage(id: string, pinnedBy: string): Promise<MessageRecord | null> {
    const result = await this.query<MessageRecord>(
      `UPDATE chat_messages
       SET is_pinned = true, pinned_at = NOW(), pinned_by = $3
       WHERE id = $1 AND source_account_id = $2 AND is_pinned = false
       RETURNING *`,
      [id, this.sourceAccountId, pinnedBy]
    );
    return result.rows[0] ?? null;
  }

  async unpinMessage(id: string): Promise<MessageRecord | null> {
    const result = await this.query<MessageRecord>(
      `UPDATE chat_messages
       SET is_pinned = false, pinned_at = NULL, pinned_by = NULL
       WHERE id = $1 AND source_account_id = $2 AND is_pinned = true
       RETURNING *`,
      [id, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async addReaction(messageId: string, userId: string, emoji: string): Promise<MessageRecord | null> {
    const message = await this.getMessage(messageId);
    if (!message) {
      return null;
    }

    const reactions = (message.reactions as MessageReactions) ?? {};
    if (!reactions[emoji]) {
      reactions[emoji] = [];
    }
    if (!reactions[emoji].includes(userId)) {
      reactions[emoji].push(userId);
    }

    const result = await this.query<MessageRecord>(
      `UPDATE chat_messages
       SET reactions = $3
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      [messageId, this.sourceAccountId, JSON.stringify(reactions)]
    );

    return result.rows[0] ?? null;
  }

  async removeReaction(messageId: string, userId: string, emoji: string): Promise<MessageRecord | null> {
    const message = await this.getMessage(messageId);
    if (!message) {
      return null;
    }

    const reactions = (message.reactions as MessageReactions) ?? {};
    if (reactions[emoji]) {
      reactions[emoji] = reactions[emoji].filter(id => id !== userId);
      if (reactions[emoji].length === 0) {
        delete reactions[emoji];
      }
    }

    const result = await this.query<MessageRecord>(
      `UPDATE chat_messages
       SET reactions = $3
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      [messageId, this.sourceAccountId, JSON.stringify(reactions)]
    );

    return result.rows[0] ?? null;
  }

  async searchMessages(query: MessageSearchQuery): Promise<MessageRecord[]> {
    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (query.conversation_id) {
      conditions.push(`conversation_id = $${paramIndex++}`);
      params.push(query.conversation_id);
    }

    if (query.sender_id) {
      conditions.push(`sender_id = $${paramIndex++}`);
      params.push(query.sender_id);
    }

    if (query.content) {
      conditions.push(`content ILIKE $${paramIndex++}`);
      params.push(`%${query.content}%`);
    }

    if (query.content_type) {
      conditions.push(`content_type = $${paramIndex++}`);
      params.push(query.content_type);
    }

    if (query.has_attachments !== undefined) {
      if (query.has_attachments) {
        conditions.push(`jsonb_array_length(attachments) > 0`);
      } else {
        conditions.push(`jsonb_array_length(attachments) = 0`);
      }
    }

    if (query.is_pinned !== undefined) {
      conditions.push(`is_pinned = $${paramIndex++}`);
      params.push(query.is_pinned);
    }

    if (query.from_date) {
      conditions.push(`created_at >= $${paramIndex++}`);
      params.push(query.from_date);
    }

    if (query.to_date) {
      conditions.push(`created_at <= $${paramIndex++}`);
      params.push(query.to_date);
    }

    const limit = query.limit ?? 50;
    const offset = query.offset ?? 0;

    const sql = `
      SELECT * FROM chat_messages
      WHERE ${conditions.join(' AND ')}
      ORDER BY created_at DESC
      LIMIT $${paramIndex} OFFSET $${paramIndex + 1}
    `;

    params.push(limit, offset);

    const result = await this.query<MessageRecord>(sql, params);
    return result.rows;
  }

  private async updateConversationLastMessage(conversationId: string, message: MessageRecord): Promise<void> {
    await this.execute(
      `UPDATE chat_conversations
       SET last_message_at = $3,
           last_message_preview = $4,
           message_count = message_count + 1,
           updated_at = NOW()
       WHERE id = $1 AND source_account_id = $2`,
      [
        conversationId,
        this.sourceAccountId,
        message.created_at,
        message.content?.substring(0, 100) ?? '[media]',
      ]
    );
  }

  private async updateUnreadCounts(conversationId: string, excludeUserId: string): Promise<void> {
    await this.execute(
      `UPDATE chat_read_receipts
       SET unread_count = unread_count + 1
       WHERE conversation_id = $1
         AND source_account_id = $2
         AND user_id != $3`,
      [conversationId, this.sourceAccountId, excludeUserId]
    );
  }

  // =========================================================================
  // Read Receipt Operations
  // =========================================================================

  async updateReadReceipt(
    conversationId: string,
    userId: string,
    input: UpdateReadReceiptInput
  ): Promise<ReadReceiptRecord> {
    // Get message timestamp
    const message = await this.getMessage(input.last_read_message_id);
    const lastReadAt = message?.created_at ?? new Date();

    // Calculate unread count
    const unreadResult = await this.query<{ count: string }>(
      `SELECT COUNT(*) FROM chat_messages
       WHERE conversation_id = $1
         AND source_account_id = $2
         AND created_at > $3
         AND deleted_at IS NULL`,
      [conversationId, this.sourceAccountId, lastReadAt]
    );
    const unreadCount = parseInt(unreadResult.rows[0]?.count ?? '0', 10);

    const result = await this.query<ReadReceiptRecord>(
      `INSERT INTO chat_read_receipts (
        source_account_id, conversation_id, user_id,
        last_read_message_id, last_read_at, unread_count
      ) VALUES ($1, $2, $3, $4, $5, $6)
      ON CONFLICT (conversation_id, user_id)
      DO UPDATE SET
        last_read_message_id = EXCLUDED.last_read_message_id,
        last_read_at = EXCLUDED.last_read_at,
        unread_count = EXCLUDED.unread_count
      RETURNING *`,
      [
        this.sourceAccountId,
        conversationId,
        userId,
        input.last_read_message_id,
        lastReadAt,
        unreadCount,
      ]
    );

    return result.rows[0];
  }

  async getReadReceipt(conversationId: string, userId: string): Promise<ReadReceiptRecord | null> {
    const result = await this.query<ReadReceiptRecord>(
      `SELECT * FROM chat_read_receipts
       WHERE conversation_id = $1 AND user_id = $2 AND source_account_id = $3`,
      [conversationId, userId, this.sourceAccountId]
    );
    return result.rows[0] ?? null;
  }

  async getUnreadCount(conversationId: string, userId: string): Promise<number> {
    const receipt = await this.getReadReceipt(conversationId, userId);
    return receipt?.unread_count ?? 0;
  }

  // =========================================================================
  // Moderation Operations
  // =========================================================================

  async createModerationAction(input: CreateModerationActionInput): Promise<ModerationActionRecord> {
    const result = await this.query<ModerationActionRecord>(
      `INSERT INTO chat_moderation_actions (
        source_account_id, conversation_id, message_id, target_user_id,
        moderator_id, action, reason, duration_minutes, created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
      RETURNING *`,
      [
        this.sourceAccountId,
        input.conversation_id ?? null,
        input.message_id ?? null,
        input.target_user_id ?? null,
        input.moderator_id,
        input.action,
        input.reason ?? null,
        input.duration_minutes ?? null,
      ]
    );

    // Execute the moderation action
    await this.executeModerationAction(result.rows[0]);

    return result.rows[0];
  }

  private async executeModerationAction(action: ModerationActionRecord): Promise<void> {
    switch (action.action) {
      case 'delete_message':
        if (action.message_id) {
          await this.execute(
            `UPDATE chat_messages SET deleted_at = NOW(), content = '[deleted by moderator]'
             WHERE id = $1 AND source_account_id = $2`,
            [action.message_id, this.sourceAccountId]
          );
        }
        break;

      case 'mute':
        if (action.conversation_id && action.target_user_id && action.duration_minutes) {
          const mutedUntil = new Date(Date.now() + action.duration_minutes * 60000);
          await this.execute(
            `UPDATE chat_participants SET muted_until = $3
             WHERE conversation_id = $1 AND user_id = $2 AND source_account_id = $4`,
            [action.conversation_id, action.target_user_id, mutedUntil, this.sourceAccountId]
          );
        }
        break;

      case 'kick':
        if (action.conversation_id && action.target_user_id) {
          await this.removeParticipant(action.conversation_id, action.target_user_id);
        }
        break;

      case 'ban':
        // Ban implementation would require additional tables to track banned users
        // For now, just remove from conversation
        if (action.conversation_id && action.target_user_id) {
          await this.removeParticipant(action.conversation_id, action.target_user_id);
        }
        break;

      case 'warn':
        // Warning is just recorded in the database
        break;
    }
  }

  async listModerationActions(conversationId?: string, limit = 50): Promise<ModerationActionRecord[]> {
    let sql = `SELECT * FROM chat_moderation_actions WHERE source_account_id = $1`;
    const params: unknown[] = [this.sourceAccountId];

    if (conversationId) {
      sql += ` AND conversation_id = $2`;
      params.push(conversationId);
    }

    sql += ` ORDER BY created_at DESC LIMIT $${params.length + 1}`;
    params.push(limit);

    const result = await this.query<ModerationActionRecord>(sql, params);
    return result.rows;
  }

  // =========================================================================
  // Webhook Event Operations
  // =========================================================================

  async insertWebhookEvent(id: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO chat_webhook_events (id, source_account_id, event_type, payload, created_at)
       VALUES ($1, $2, $3, $4, NOW())
       ON CONFLICT (id) DO NOTHING`,
      [id, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE chat_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [id, error ?? null]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<ChatStats> {
    const [
      conversations,
      participants,
      messages,
      activeConversations,
      totalUsers,
      messagesLast24h,
    ] = await Promise.all([
      this.countTotal('chat_conversations'),
      this.countTotal('chat_participants'),
      this.countTotal('chat_messages'),
      this.countActiveConversations(),
      this.countTotalUsers(),
      this.countMessagesLast24h(),
    ]);

    return {
      conversations,
      participants,
      messages,
      activeConversations,
      totalUsers,
      messagesLast24h,
      lastSyncedAt: new Date(),
    };
  }

  private async countTotal(table: string): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) FROM ${table} WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  private async countActiveConversations(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) FROM chat_conversations
       WHERE source_account_id = $1 AND NOT is_archived`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  private async countTotalUsers(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(DISTINCT user_id) FROM chat_participants
       WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }

  private async countMessagesLast24h(): Promise<number> {
    const result = await this.query<{ count: string }>(
      `SELECT COUNT(*) FROM chat_messages
       WHERE source_account_id = $1
         AND created_at >= NOW() - INTERVAL '24 hours'`,
      [this.sourceAccountId]
    );
    return parseInt(result.rows[0]?.count ?? '0', 10);
  }
}
