/**
 * Social Plugin Types
 * Complete type definitions for all social objects
 */

export interface SocialPluginConfig {
  port: number;
  host: string;
  maxPostLength: number;
  maxCommentLength: number;
  maxCommentDepth: number;
  editWindowMinutes: number;
  reactionsAllowed: string[];
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
  security: {
    apiKey?: string;
    rateLimitMax: number;
    rateLimitWindowMs: number;
  };
}

// =============================================================================
// Post Types
// =============================================================================

export interface SocialPostRecord {
  id: string;
  source_account_id: string;
  author_id: string;
  content: string | null;
  content_type: 'text' | 'image' | 'video' | 'link' | 'poll';
  attachments: Attachment[];
  visibility: 'public' | 'followers' | 'private';
  hashtags: string[];
  mentions: string[];
  location: Location | null;
  comment_count: number;
  reaction_count: number;
  share_count: number;
  bookmark_count: number;
  is_pinned: boolean;
  edited_at: Date | null;
  deleted_at: Date | null;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface Attachment {
  type: 'image' | 'video' | 'audio' | 'file';
  url: string;
  thumbnail_url?: string;
  width?: number;
  height?: number;
  duration?: number;
  size?: number;
  mime_type?: string;
  alt_text?: string;
}

export interface Location {
  name?: string;
  latitude?: number;
  longitude?: number;
  address?: string;
  city?: string;
  country?: string;
}

export interface CreatePostInput {
  author_id: string;
  content?: string;
  content_type?: 'text' | 'image' | 'video' | 'link' | 'poll';
  attachments?: Attachment[];
  visibility?: 'public' | 'followers' | 'private';
  hashtags?: string[];
  mentions?: string[];
  location?: Location;
  metadata?: Record<string, unknown>;
}

export interface UpdatePostInput {
  content?: string;
  attachments?: Attachment[];
  visibility?: 'public' | 'followers' | 'private';
  hashtags?: string[];
  mentions?: string[];
  location?: Location;
  is_pinned?: boolean;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Comment Types
// =============================================================================

export interface SocialCommentRecord {
  id: string;
  source_account_id: string;
  target_type: string;
  target_id: string;
  parent_id: string | null;
  author_id: string;
  content: string;
  mentions: string[];
  reaction_count: number;
  reply_count: number;
  depth: number;
  edited_at: Date | null;
  deleted_at: Date | null;
  created_at: Date;
  updated_at: Date;
  [key: string]: unknown;
}

export interface CreateCommentInput {
  target_type: string;
  target_id: string;
  parent_id?: string;
  author_id: string;
  content: string;
  mentions?: string[];
}

export interface UpdateCommentInput {
  content?: string;
  mentions?: string[];
}

// =============================================================================
// Reaction Types
// =============================================================================

export interface SocialReactionRecord {
  id: string;
  source_account_id: string;
  target_type: string;
  target_id: string;
  user_id: string;
  reaction_type: string;
  created_at: Date;
  [key: string]: unknown;
}

export interface CreateReactionInput {
  target_type: string;
  target_id: string;
  user_id: string;
  reaction_type: string;
}

export interface ReactionSummary {
  reaction_type: string;
  count: number;
  users: string[];
}

// =============================================================================
// Follow Types
// =============================================================================

export interface SocialFollowRecord {
  id: string;
  source_account_id: string;
  follower_id: string;
  following_type: 'user' | 'tag' | 'category';
  following_id: string;
  created_at: Date;
  [key: string]: unknown;
}

export interface CreateFollowInput {
  follower_id: string;
  following_type: 'user' | 'tag' | 'category';
  following_id: string;
}

// =============================================================================
// Bookmark Types
// =============================================================================

export interface SocialBookmarkRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  target_type: string;
  target_id: string;
  collection: string;
  note: string | null;
  created_at: Date;
  [key: string]: unknown;
}

export interface CreateBookmarkInput {
  user_id: string;
  target_type: string;
  target_id: string;
  collection?: string;
  note?: string;
}

// =============================================================================
// Share Types
// =============================================================================

export interface SocialShareRecord {
  id: string;
  source_account_id: string;
  user_id: string;
  target_type: string;
  target_id: string;
  share_type: 'repost' | 'quote';
  message: string | null;
  created_at: Date;
  [key: string]: unknown;
}

export interface CreateShareInput {
  user_id: string;
  target_type: string;
  target_id: string;
  share_type: 'repost' | 'quote';
  message?: string;
}

// =============================================================================
// Webhook Types
// =============================================================================

export interface SocialWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

export interface WebhookEvent {
  type: string;
  data: Record<string, unknown>;
  timestamp: string;
}

// =============================================================================
// Statistics Types
// =============================================================================

export interface SocialStats {
  posts: number;
  comments: number;
  reactions: number;
  follows: number;
  bookmarks: number;
  shares: number;
  lastUpdatedAt?: Date | null;
}

export interface UserProfile {
  user_id: string;
  post_count: number;
  follower_count: number;
  following_count: number;
  bookmark_count: number;
}

export interface TrendingHashtag {
  hashtag: string;
  count: number;
  last_used: Date;
}

// =============================================================================
// Query Options
// =============================================================================

export interface ListPostsOptions {
  author_id?: string;
  hashtag?: string;
  visibility?: 'public' | 'followers' | 'private';
  limit?: number;
  offset?: number;
}

export interface ListCommentsOptions {
  target_type?: string;
  target_id?: string;
  author_id?: string;
  parent_id?: string;
  limit?: number;
  offset?: number;
}

export interface ListReactionsOptions {
  target_type?: string;
  target_id?: string;
  user_id?: string;
  reaction_type?: string;
}

export interface ListFollowsOptions {
  follower_id?: string;
  following_type?: 'user' | 'tag' | 'category';
  following_id?: string;
}

export interface ListBookmarksOptions {
  user_id?: string;
  target_type?: string;
  collection?: string;
  limit?: number;
  offset?: number;
}
