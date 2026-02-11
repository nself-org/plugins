/**
 * Activity Feed Plugin Types
 * Complete type definitions for activity feed system
 */

// =============================================================================
// Configuration
// =============================================================================

export type FeedStrategy = 'read' | 'write';

export interface FeedPluginConfig {
  // Server
  port: number;
  host: string;

  // Database
  databaseHost: string;
  databasePort: number;
  databaseName: string;
  databaseUser: string;
  databasePassword: string;
  databaseSsl: boolean;

  // Feed settings
  strategy: FeedStrategy;
  maxFeedSize: number;
  aggregationWindowMinutes: number;
  retentionDays: number;

  // Security
  apiKey?: string;
  rateLimitMax: number;
  rateLimitWindowMs: number;

  // Logging
  logLevel: string;
}

// =============================================================================
// Activity Types
// =============================================================================

export type ActivityVerb =
  | 'created'
  | 'updated'
  | 'deleted'
  | 'liked'
  | 'commented'
  | 'followed'
  | 'shared'
  | 'joined'
  | 'left'
  | 'uploaded'
  | 'published'
  | 'mentioned'
  | 'invited'
  | 'completed'
  | 'started';

export type ActorType = 'user' | 'system' | 'bot' | 'service';

export interface ActivityRecord extends Record<string, unknown> {
  id: string; // UUID
  source_account_id: string;
  actor_id: string;
  actor_type: ActorType;
  verb: ActivityVerb;
  object_type: string;
  object_id: string;
  target_type: string | null;
  target_id: string | null;
  source_plugin: string | null;
  message: string | null;
  data: Record<string, unknown>;
  is_aggregatable: boolean;
  created_at: Date;
}

export interface CreateActivityInput extends Record<string, unknown> {
  source_account_id?: string;
  actor_id: string;
  actor_type?: ActorType;
  verb: ActivityVerb;
  object_type: string;
  object_id: string;
  target_type?: string;
  target_id?: string;
  source_plugin?: string;
  message?: string;
  data?: Record<string, unknown>;
  is_aggregatable?: boolean;
}

// =============================================================================
// User Feed Types
// =============================================================================

export interface UserFeedRecord extends Record<string, unknown> {
  id: string; // UUID
  source_account_id: string;
  user_id: string;
  activity_id: string;
  is_read: boolean;
  read_at: Date | null;
  is_hidden: boolean;
  created_at: Date;
}

export interface FeedItemWithActivity extends UserFeedRecord {
  activity: ActivityRecord;
}

// =============================================================================
// Subscription Types
// =============================================================================

export type SubscriptionTargetType = 'user' | 'group' | 'channel' | 'tag' | 'category';

export interface SubscriptionRecord extends Record<string, unknown> {
  id: string; // UUID
  source_account_id: string;
  user_id: string;
  target_type: SubscriptionTargetType;
  target_id: string;
  enabled: boolean;
  created_at: Date;
}

export interface CreateSubscriptionInput {
  source_account_id?: string;
  user_id: string;
  target_type: SubscriptionTargetType;
  target_id: string;
  enabled?: boolean;
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
// Aggregation Types
// =============================================================================

export interface AggregatedActivity extends Record<string, unknown> {
  verb: ActivityVerb;
  object_type: string;
  object_id: string;
  actor_ids: string[];
  actor_count: number;
  latest_activity_id: string;
  created_at: Date;
  message?: string;
}

// =============================================================================
// Query Types
// =============================================================================

export interface FeedQuery {
  userId: string;
  sourceAccountId?: string;
  limit?: number;
  offset?: number;
  includeRead?: boolean;
  includeHidden?: boolean;
  sinceDate?: Date;
  untilDate?: Date;
}

export interface ActivityQuery {
  sourceAccountId?: string;
  actorId?: string;
  verb?: ActivityVerb;
  objectType?: string;
  objectId?: string;
  targetType?: string;
  targetId?: string;
  limit?: number;
  offset?: number;
  sinceDate?: Date;
  untilDate?: Date;
}

export interface EntityFeedQuery {
  entityType: string;
  entityId: string;
  sourceAccountId?: string;
  limit?: number;
  offset?: number;
  sinceDate?: Date;
  untilDate?: Date;
}

// =============================================================================
// Statistics Types
// =============================================================================

export interface FeedStats {
  totalActivities: number;
  totalSubscriptions: number;
  totalFeedItems: number;
  unreadFeedItems: number;
  activitiesByVerb: Record<ActivityVerb, number>;
  activitiesByActorType: Record<ActorType, number>;
  recentActivityCount24h: number;
  recentActivityCount7d: number;
  lastActivityAt: Date | null;
}

export interface UserFeedStats {
  userId: string;
  sourceAccountId: string;
  totalItems: number;
  unreadCount: number;
  subscriptionCount: number;
  lastActivityAt: Date | null;
}

// =============================================================================
// Response Types
// =============================================================================

export interface FeedResponse {
  data: FeedItemWithActivity[];
  total: number;
  unreadCount: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

export interface ActivityListResponse {
  data: ActivityRecord[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

export interface SubscriptionListResponse {
  data: SubscriptionRecord[];
  total: number;
}

// =============================================================================
// Fan-out Types
// =============================================================================

export interface FanOutResult {
  activityId: string;
  subscribersCount: number;
  feedItemsCreated: number;
  duration: number;
  success: boolean;
  error?: string;
}

export interface FanOutOptions {
  activityId: string;
  sourceAccountId?: string;
  forceRefresh?: boolean;
}
