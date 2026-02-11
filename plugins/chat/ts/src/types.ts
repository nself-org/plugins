/**
 * Chat Plugin Types
 * Complete type definitions for all chat objects
 */

export interface ChatPluginConfig {
  port: number;
  host: string;
  maxMessageLength: number;
  maxAttachments: number;
  editWindowMinutes: number;
  maxParticipants: number;
  maxPinned: number;
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
// Conversation Types
// =============================================================================

export type ConversationType = 'direct' | 'group' | 'channel';

export interface ConversationRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  type: ConversationType;
  name: string | null;
  description: string | null;
  avatar_url: string | null;
  created_by: string | null;
  is_archived: boolean;
  is_muted: boolean;
  last_message_at: Date | null;
  last_message_preview: string | null;
  message_count: number;
  member_count: number;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateConversationInput {
  type: ConversationType;
  name?: string;
  description?: string;
  avatar_url?: string;
  created_by?: string;
  participant_ids?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateConversationInput {
  name?: string;
  description?: string;
  avatar_url?: string;
  is_archived?: boolean;
  is_muted?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Participant Types
// =============================================================================

export type ParticipantRole = 'owner' | 'admin' | 'moderator' | 'member' | 'guest';
export type NotificationLevel = 'all' | 'mentions' | 'none';

export interface ParticipantRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  conversation_id: string;
  user_id: string;
  role: ParticipantRole;
  nickname: string | null;
  muted_until: Date | null;
  joined_at: Date;
  left_at: Date | null;
  last_read_message_id: string | null;
  last_read_at: Date | null;
  notification_level: NotificationLevel;
  created_at: Date;
}

export interface AddParticipantInput {
  user_id: string;
  role?: ParticipantRole;
  nickname?: string;
  notification_level?: NotificationLevel;
}

export interface UpdateParticipantInput {
  role?: ParticipantRole;
  nickname?: string;
  muted_until?: Date | null;
  notification_level?: NotificationLevel;
}

// =============================================================================
// Message Types
// =============================================================================

export type MessageContentType = 'text' | 'image' | 'file' | 'audio' | 'video' | 'system' | 'embed';

export interface MessageAttachment {
  url: string;
  type: string;
  size?: number;
  name?: string;
  thumbnail_url?: string;
}

export interface MessageReactions {
  [emoji: string]: string[];
}

export interface MessageRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  conversation_id: string;
  sender_id: string;
  reply_to_id: string | null;
  thread_root_id: string | null;
  content: string | null;
  content_type: MessageContentType;
  attachments: MessageAttachment[];
  mentions: string[];
  reactions: MessageReactions;
  edited_at: Date | null;
  deleted_at: Date | null;
  is_pinned: boolean;
  pinned_at: Date | null;
  pinned_by: string | null;
  metadata: Record<string, unknown>;
  created_at: Date;
}

export interface SendMessageInput {
  sender_id: string;
  content?: string;
  content_type?: MessageContentType;
  reply_to_id?: string;
  thread_root_id?: string;
  attachments?: MessageAttachment[];
  mentions?: string[];
  metadata?: Record<string, unknown>;
}

export interface UpdateMessageInput {
  content?: string;
  attachments?: MessageAttachment[];
  mentions?: string[];
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Read Receipt Types
// =============================================================================

export interface ReadReceiptRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  conversation_id: string;
  user_id: string;
  last_read_message_id: string | null;
  last_read_at: Date;
  unread_count: number;
}

export interface UpdateReadReceiptInput {
  last_read_message_id: string;
}

// =============================================================================
// Moderation Types
// =============================================================================

export type ModerationAction = 'delete_message' | 'warn' | 'mute' | 'kick' | 'ban';

export interface ModerationActionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  conversation_id: string | null;
  message_id: string | null;
  target_user_id: string | null;
  moderator_id: string;
  action: ModerationAction;
  reason: string | null;
  duration_minutes: number | null;
  created_at: Date;
}

export interface CreateModerationActionInput {
  conversation_id?: string;
  message_id?: string;
  target_user_id?: string;
  moderator_id: string;
  action: ModerationAction;
  reason?: string;
  duration_minutes?: number;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface WebhookEventRecord extends Record<string, unknown> {
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
// API Response Types
// =============================================================================

export interface ConversationWithDetails extends ConversationRecord {
  participants?: ParticipantRecord[];
  unread_count?: number;
}

export interface MessageWithContext extends MessageRecord {
  sender?: {
    id: string;
    name?: string;
    avatar_url?: string;
  };
  reply_to?: MessageRecord;
  thread_count?: number;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  limit: number;
  offset?: number;
  cursor?: string;
  has_more?: boolean;
}

export interface SyncOptions {
  since?: Date;
  conversation_id?: string;
  user_id?: string;
}

export interface ChatStats {
  conversations: number;
  participants: number;
  messages: number;
  activeConversations: number;
  totalUsers: number;
  messagesLast24h: number;
  lastSyncedAt?: Date | null;
}

// =============================================================================
// Search Types
// =============================================================================

export interface MessageSearchQuery {
  conversation_id?: string;
  sender_id?: string;
  content?: string;
  content_type?: MessageContentType;
  has_attachments?: boolean;
  is_pinned?: boolean;
  from_date?: Date;
  to_date?: Date;
  limit?: number;
  offset?: number;
}

export interface MessageSearchResult {
  message: MessageRecord;
  conversation: ConversationRecord;
  score?: number;
}

// =============================================================================
// Typing Indicator Types
// =============================================================================

export interface TypingIndicator {
  conversation_id: string;
  user_id: string;
  started_at: Date;
}
