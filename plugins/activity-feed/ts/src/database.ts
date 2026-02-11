/**
 * Activity Feed Database Operations
 * Complete CRUD operations for activity feed system in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  ActivityRecord,
  CreateActivityInput,
  FeedItemWithActivity,
  SubscriptionRecord,
  CreateSubscriptionInput,
  WebhookEventRecord,
  FeedStats,
  UserFeedStats,
  FeedQuery,
  ActivityQuery,
  EntityFeedQuery,
  AggregatedActivity,
  ActivityVerb,
} from './types.js';

const logger = createLogger('feed:db');

export class FeedDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): FeedDatabase {
    return new FeedDatabase(this.db, sourceAccountId);
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
    logger.info('Initializing activity feed schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =====================================================================
      -- Activities Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS feed_activities (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        actor_id VARCHAR(255) NOT NULL,
        actor_type VARCHAR(32) DEFAULT 'user',
        verb VARCHAR(64) NOT NULL,
        object_type VARCHAR(64) NOT NULL,
        object_id VARCHAR(255) NOT NULL,
        target_type VARCHAR(64),
        target_id VARCHAR(255),
        source_plugin VARCHAR(64),
        message TEXT,
        data JSONB DEFAULT '{}',
        is_aggregatable BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_feed_activities_source_account
        ON feed_activities(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_feed_activities_actor
        ON feed_activities(source_account_id, actor_id);
      CREATE INDEX IF NOT EXISTS idx_feed_activities_object
        ON feed_activities(source_account_id, object_type, object_id);
      CREATE INDEX IF NOT EXISTS idx_feed_activities_target
        ON feed_activities(source_account_id, target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_feed_activities_created
        ON feed_activities(source_account_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_feed_activities_verb
        ON feed_activities(source_account_id, verb);
      CREATE INDEX IF NOT EXISTS idx_feed_activities_aggregatable
        ON feed_activities(source_account_id, is_aggregatable, verb, object_type, object_id, created_at DESC)
        WHERE is_aggregatable = true;

      -- =====================================================================
      -- User Feeds Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS feed_user_feeds (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        activity_id UUID NOT NULL REFERENCES feed_activities(id) ON DELETE CASCADE,
        is_read BOOLEAN DEFAULT false,
        read_at TIMESTAMP WITH TIME ZONE,
        is_hidden BOOLEAN DEFAULT false,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, activity_id)
      );

      CREATE INDEX IF NOT EXISTS idx_feed_user_feeds_source_account
        ON feed_user_feeds(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_feed_user_feeds_user
        ON feed_user_feeds(source_account_id, user_id, created_at DESC);
      CREATE INDEX IF NOT EXISTS idx_feed_user_feeds_activity
        ON feed_user_feeds(activity_id);
      CREATE INDEX IF NOT EXISTS idx_feed_user_feeds_unread
        ON feed_user_feeds(source_account_id, user_id, is_read)
        WHERE is_read = false;
      CREATE INDEX IF NOT EXISTS idx_feed_user_feeds_visible
        ON feed_user_feeds(source_account_id, user_id, is_hidden)
        WHERE is_hidden = false;

      -- =====================================================================
      -- Subscriptions Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS feed_subscriptions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        target_type VARCHAR(64) NOT NULL,
        target_id VARCHAR(255) NOT NULL,
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, target_type, target_id)
      );

      CREATE INDEX IF NOT EXISTS idx_feed_subscriptions_source_account
        ON feed_subscriptions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_feed_subscriptions_user
        ON feed_subscriptions(source_account_id, user_id);
      CREATE INDEX IF NOT EXISTS idx_feed_subscriptions_target
        ON feed_subscriptions(source_account_id, target_type, target_id);
      CREATE INDEX IF NOT EXISTS idx_feed_subscriptions_enabled
        ON feed_subscriptions(source_account_id, enabled)
        WHERE enabled = true;

      -- =====================================================================
      -- Webhook Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS feed_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_feed_webhook_events_source_account
        ON feed_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_feed_webhook_events_type
        ON feed_webhook_events(source_account_id, event_type);
      CREATE INDEX IF NOT EXISTS idx_feed_webhook_events_processed
        ON feed_webhook_events(source_account_id, processed);
      CREATE INDEX IF NOT EXISTS idx_feed_webhook_events_created
        ON feed_webhook_events(source_account_id, created_at DESC);
    `;

    await this.execute(schema);
    logger.info('Activity feed schema initialized');
  }

  // =========================================================================
  // Activity Operations
  // =========================================================================

  async createActivity(input: CreateActivityInput): Promise<ActivityRecord> {
    const sourceAccountId = input.source_account_id ?? this.sourceAccountId;
    const actorType = input.actor_type ?? 'user';
    const data = input.data ?? {};
    const isAggregatable = input.is_aggregatable ?? true;

    const result = await this.query<ActivityRecord>(
      `INSERT INTO feed_activities (
        source_account_id, actor_id, actor_type, verb, object_type, object_id,
        target_type, target_id, source_plugin, message, data, is_aggregatable
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
      RETURNING *`,
      [
        sourceAccountId,
        input.actor_id,
        actorType,
        input.verb,
        input.object_type,
        input.object_id,
        input.target_type ?? null,
        input.target_id ?? null,
        input.source_plugin ?? null,
        input.message ?? null,
        JSON.stringify(data),
        isAggregatable,
      ]
    );

    return result.rows[0];
  }

  async getActivity(activityId: string): Promise<ActivityRecord | null> {
    const result = await this.query<ActivityRecord>(
      `SELECT * FROM feed_activities
       WHERE id = $1 AND source_account_id = $2`,
      [activityId, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listActivities(query: ActivityQuery): Promise<ActivityRecord[]> {
    const sourceAccountId = query.sourceAccountId ?? this.sourceAccountId;
    const limit = query.limit ?? 100;
    const offset = query.offset ?? 0;

    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [sourceAccountId];
    let paramIndex = 2;

    if (query.actorId) {
      conditions.push(`actor_id = $${paramIndex++}`);
      params.push(query.actorId);
    }

    if (query.verb) {
      conditions.push(`verb = $${paramIndex++}`);
      params.push(query.verb);
    }

    if (query.objectType) {
      conditions.push(`object_type = $${paramIndex++}`);
      params.push(query.objectType);
    }

    if (query.objectId) {
      conditions.push(`object_id = $${paramIndex++}`);
      params.push(query.objectId);
    }

    if (query.targetType) {
      conditions.push(`target_type = $${paramIndex++}`);
      params.push(query.targetType);
    }

    if (query.targetId) {
      conditions.push(`target_id = $${paramIndex++}`);
      params.push(query.targetId);
    }

    if (query.sinceDate) {
      conditions.push(`created_at >= $${paramIndex++}`);
      params.push(query.sinceDate);
    }

    if (query.untilDate) {
      conditions.push(`created_at <= $${paramIndex++}`);
      params.push(query.untilDate);
    }

    const sql = `
      SELECT * FROM feed_activities
      WHERE ${conditions.join(' AND ')}
      ORDER BY created_at DESC
      LIMIT $${paramIndex++} OFFSET $${paramIndex}
    `;
    params.push(limit, offset);

    const result = await this.query<ActivityRecord>(sql, params);
    return result.rows;
  }

  async countActivities(query: ActivityQuery): Promise<number> {
    const sourceAccountId = query.sourceAccountId ?? this.sourceAccountId;

    const conditions: string[] = ['source_account_id = $1'];
    const params: unknown[] = [sourceAccountId];
    let paramIndex = 2;

    if (query.actorId) {
      conditions.push(`actor_id = $${paramIndex++}`);
      params.push(query.actorId);
    }

    if (query.verb) {
      conditions.push(`verb = $${paramIndex++}`);
      params.push(query.verb);
    }

    if (query.objectType) {
      conditions.push(`object_type = $${paramIndex++}`);
      params.push(query.objectType);
    }

    if (query.objectId) {
      conditions.push(`object_id = $${paramIndex++}`);
      params.push(query.objectId);
    }

    if (query.sinceDate) {
      conditions.push(`created_at >= $${paramIndex++}`);
      params.push(query.sinceDate);
    }

    if (query.untilDate) {
      conditions.push(`created_at <= $${paramIndex++}`);
      params.push(query.untilDate);
    }

    const sql = `
      SELECT COUNT(*)::int as count FROM feed_activities
      WHERE ${conditions.join(' AND ')}
    `;

    const result = await this.query<{ count: number }>(sql, params);
    return result.rows[0]?.count ?? 0;
  }

  async getEntityFeed(query: EntityFeedQuery): Promise<ActivityRecord[]> {
    const sourceAccountId = query.sourceAccountId ?? this.sourceAccountId;
    const limit = query.limit ?? 100;
    const offset = query.offset ?? 0;

    const conditions: string[] = [
      'source_account_id = $1',
      'object_type = $2',
      'object_id = $3',
    ];
    const params: unknown[] = [sourceAccountId, query.entityType, query.entityId];
    let paramIndex = 4;

    if (query.sinceDate) {
      conditions.push(`created_at >= $${paramIndex++}`);
      params.push(query.sinceDate);
    }

    if (query.untilDate) {
      conditions.push(`created_at <= $${paramIndex++}`);
      params.push(query.untilDate);
    }

    const sql = `
      SELECT * FROM feed_activities
      WHERE ${conditions.join(' AND ')}
      ORDER BY created_at DESC
      LIMIT $${paramIndex++} OFFSET $${paramIndex}
    `;
    params.push(limit, offset);

    const result = await this.query<ActivityRecord>(sql, params);
    return result.rows;
  }

  async deleteActivity(activityId: string): Promise<void> {
    await this.execute(
      `DELETE FROM feed_activities WHERE id = $1 AND source_account_id = $2`,
      [activityId, this.sourceAccountId]
    );
  }

  // =========================================================================
  // User Feed Operations
  // =========================================================================

  async getUserFeed(query: FeedQuery): Promise<FeedItemWithActivity[]> {
    const sourceAccountId = query.sourceAccountId ?? this.sourceAccountId;
    const limit = query.limit ?? 100;
    const offset = query.offset ?? 0;
    const includeRead = query.includeRead ?? true;
    const includeHidden = query.includeHidden ?? false;

    const conditions: string[] = [
      'f.source_account_id = $1',
      'f.user_id = $2',
    ];
    const params: unknown[] = [sourceAccountId, query.userId];
    let paramIndex = 3;

    if (!includeRead) {
      conditions.push('f.is_read = false');
    }

    if (!includeHidden) {
      conditions.push('f.is_hidden = false');
    }

    if (query.sinceDate) {
      conditions.push(`f.created_at >= $${paramIndex++}`);
      params.push(query.sinceDate);
    }

    if (query.untilDate) {
      conditions.push(`f.created_at <= $${paramIndex++}`);
      params.push(query.untilDate);
    }

    const sql = `
      SELECT
        f.*,
        row_to_json(a.*) as activity
      FROM feed_user_feeds f
      INNER JOIN feed_activities a ON f.activity_id = a.id
      WHERE ${conditions.join(' AND ')}
      ORDER BY f.created_at DESC
      LIMIT $${paramIndex++} OFFSET $${paramIndex}
    `;
    params.push(limit, offset);

    const result = await this.query<FeedItemWithActivity>(sql, params);
    return result.rows;
  }

  async getUserFeedUsingSubscriptions(query: FeedQuery): Promise<ActivityRecord[]> {
    const sourceAccountId = query.sourceAccountId ?? this.sourceAccountId;
    const limit = query.limit ?? 100;
    const offset = query.offset ?? 0;

    const conditions: string[] = [
      'a.source_account_id = $1',
      'EXISTS (SELECT 1 FROM feed_subscriptions s WHERE s.source_account_id = a.source_account_id AND s.user_id = $2 AND s.enabled = true AND s.target_type = $3 AND s.target_id = a.actor_id)',
    ];
    const params: unknown[] = [sourceAccountId, query.userId, 'user'];
    let paramIndex = 4;

    if (query.sinceDate) {
      conditions.push(`a.created_at >= $${paramIndex++}`);
      params.push(query.sinceDate);
    }

    if (query.untilDate) {
      conditions.push(`a.created_at <= $${paramIndex++}`);
      params.push(query.untilDate);
    }

    const sql = `
      SELECT a.* FROM feed_activities a
      WHERE ${conditions.join(' AND ')}
      ORDER BY a.created_at DESC
      LIMIT $${paramIndex++} OFFSET $${paramIndex}
    `;
    params.push(limit, offset);

    const result = await this.query<ActivityRecord>(sql, params);
    return result.rows;
  }

  async countUserFeedItems(userId: string, includeRead = true, includeHidden = false): Promise<number> {
    const conditions: string[] = [
      'source_account_id = $1',
      'user_id = $2',
    ];
    const params: unknown[] = [this.sourceAccountId, userId];

    if (!includeRead) {
      conditions.push('is_read = false');
    }

    if (!includeHidden) {
      conditions.push('is_hidden = false');
    }

    const sql = `
      SELECT COUNT(*)::int as count FROM feed_user_feeds
      WHERE ${conditions.join(' AND ')}
    `;

    const result = await this.query<{ count: number }>(sql, params);
    return result.rows[0]?.count ?? 0;
  }

  async getUnreadCount(userId: string): Promise<number> {
    const result = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM feed_user_feeds
       WHERE source_account_id = $1 AND user_id = $2 AND is_read = false AND is_hidden = false`,
      [this.sourceAccountId, userId]
    );

    return result.rows[0]?.count ?? 0;
  }

  async markFeedItemsAsRead(userId: string, activityIds?: string[]): Promise<number> {
    if (activityIds && activityIds.length > 0) {
      return this.execute(
        `UPDATE feed_user_feeds
         SET is_read = true, read_at = NOW()
         WHERE source_account_id = $1 AND user_id = $2 AND activity_id = ANY($3) AND is_read = false`,
        [this.sourceAccountId, userId, activityIds]
      );
    } else {
      return this.execute(
        `UPDATE feed_user_feeds
         SET is_read = true, read_at = NOW()
         WHERE source_account_id = $1 AND user_id = $2 AND is_read = false`,
        [this.sourceAccountId, userId]
      );
    }
  }

  async hideFeedItem(userId: string, activityId: string): Promise<number> {
    return this.execute(
      `UPDATE feed_user_feeds
       SET is_hidden = true
       WHERE source_account_id = $1 AND user_id = $2 AND activity_id = $3`,
      [this.sourceAccountId, userId, activityId]
    );
  }

  async createFeedItem(userId: string, activityId: string): Promise<void> {
    await this.execute(
      `INSERT INTO feed_user_feeds (source_account_id, user_id, activity_id)
       VALUES ($1, $2, $3)
       ON CONFLICT (source_account_id, user_id, activity_id) DO NOTHING`,
      [this.sourceAccountId, userId, activityId]
    );
  }

  // =========================================================================
  // Subscription Operations
  // =========================================================================

  async createSubscription(input: CreateSubscriptionInput): Promise<SubscriptionRecord> {
    const sourceAccountId = input.source_account_id ?? this.sourceAccountId;
    const enabled = input.enabled ?? true;

    const result = await this.query<SubscriptionRecord>(
      `INSERT INTO feed_subscriptions (source_account_id, user_id, target_type, target_id, enabled)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (source_account_id, user_id, target_type, target_id)
       DO UPDATE SET enabled = EXCLUDED.enabled
       RETURNING *`,
      [sourceAccountId, input.user_id, input.target_type, input.target_id, enabled]
    );

    return result.rows[0];
  }

  async listUserSubscriptions(userId: string): Promise<SubscriptionRecord[]> {
    const result = await this.query<SubscriptionRecord>(
      `SELECT * FROM feed_subscriptions
       WHERE source_account_id = $1 AND user_id = $2
       ORDER BY created_at DESC`,
      [this.sourceAccountId, userId]
    );

    return result.rows;
  }

  async getSubscription(subscriptionId: string): Promise<SubscriptionRecord | null> {
    const result = await this.query<SubscriptionRecord>(
      `SELECT * FROM feed_subscriptions WHERE id = $1 AND source_account_id = $2`,
      [subscriptionId, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async deleteSubscription(subscriptionId: string): Promise<void> {
    await this.execute(
      `DELETE FROM feed_subscriptions WHERE id = $1 AND source_account_id = $2`,
      [subscriptionId, this.sourceAccountId]
    );
  }

  async getSubscribersForActor(actorId: string): Promise<string[]> {
    const result = await this.query<{ user_id: string }>(
      `SELECT DISTINCT user_id FROM feed_subscriptions
       WHERE source_account_id = $1 AND target_type = 'user' AND target_id = $2 AND enabled = true`,
      [this.sourceAccountId, actorId]
    );

    return result.rows.map(row => row.user_id);
  }

  // =========================================================================
  // Aggregation Operations
  // =========================================================================

  async getAggregatedActivities(
    userId: string,
    windowMinutes: number,
    limit = 100
  ): Promise<AggregatedActivity[]> {
    const sql = `
      SELECT
        a.verb,
        a.object_type,
        a.object_id,
        array_agg(DISTINCT a.actor_id ORDER BY a.actor_id) as actor_ids,
        COUNT(DISTINCT a.actor_id)::int as actor_count,
        MAX(a.id)::text as latest_activity_id,
        MAX(a.created_at) as created_at,
        MAX(a.message) as message
      FROM feed_activities a
      WHERE a.source_account_id = $1
        AND a.is_aggregatable = true
        AND a.created_at >= NOW() - INTERVAL '${windowMinutes} minutes'
        AND EXISTS (
          SELECT 1 FROM feed_subscriptions s
          WHERE s.source_account_id = a.source_account_id
            AND s.user_id = $2
            AND s.enabled = true
            AND s.target_type = 'user'
            AND s.target_id = a.actor_id
        )
      GROUP BY a.verb, a.object_type, a.object_id
      HAVING COUNT(DISTINCT a.actor_id) > 1
      ORDER BY created_at DESC
      LIMIT $3
    `;

    const result = await this.query<AggregatedActivity>(sql, [
      this.sourceAccountId,
      userId,
      limit,
    ]);

    return result.rows;
  }

  // =========================================================================
  // Webhook Event Operations
  // =========================================================================

  async createWebhookEvent(
    eventId: string,
    eventType: string,
    payload: Record<string, unknown>
  ): Promise<WebhookEventRecord> {
    const result = await this.query<WebhookEventRecord>(
      `INSERT INTO feed_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (id) DO NOTHING
       RETURNING *`,
      [eventId, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );

    return result.rows[0];
  }

  async markWebhookEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE feed_webhook_events
       SET processed = true, processed_at = NOW(), error = $3
       WHERE id = $1 AND source_account_id = $2`,
      [eventId, this.sourceAccountId, error ?? null]
    );
  }

  // =========================================================================
  // Statistics Operations
  // =========================================================================

  async getStats(): Promise<FeedStats> {
    const [totalActivities, totalSubscriptions, totalFeedItems, unreadFeedItems] = await Promise.all([
      this.countActivities({}),
      this.countSubscriptions(),
      this.countTotalFeedItems(),
      this.countTotalUnreadFeedItems(),
    ]);

    const activitiesByVerb = await this.getActivitiesByVerb();
    const activitiesByActorType = await this.getActivitiesByActorType();
    const recentActivityCount24h = await this.countRecentActivities(24);
    const recentActivityCount7d = await this.countRecentActivities(168);
    const lastActivityAt = await this.getLastActivityDate();

    return {
      totalActivities,
      totalSubscriptions,
      totalFeedItems,
      unreadFeedItems,
      activitiesByVerb,
      activitiesByActorType,
      recentActivityCount24h,
      recentActivityCount7d,
      lastActivityAt,
    };
  }

  async getUserFeedStats(userId: string): Promise<UserFeedStats> {
    const [totalItems, unreadCount, subscriptionCount] = await Promise.all([
      this.countUserFeedItems(userId, true, false),
      this.getUnreadCount(userId),
      this.countUserSubscriptions(userId),
    ]);

    const lastActivityAt = await this.getUserLastActivityDate(userId);

    return {
      userId,
      sourceAccountId: this.sourceAccountId,
      totalItems,
      unreadCount,
      subscriptionCount,
      lastActivityAt,
    };
  }

  private async countSubscriptions(): Promise<number> {
    const result = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM feed_subscriptions WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return result.rows[0]?.count ?? 0;
  }

  private async countUserSubscriptions(userId: string): Promise<number> {
    const result = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM feed_subscriptions
       WHERE source_account_id = $1 AND user_id = $2`,
      [this.sourceAccountId, userId]
    );
    return result.rows[0]?.count ?? 0;
  }

  private async countTotalFeedItems(): Promise<number> {
    const result = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM feed_user_feeds WHERE source_account_id = $1`,
      [this.sourceAccountId]
    );
    return result.rows[0]?.count ?? 0;
  }

  private async countTotalUnreadFeedItems(): Promise<number> {
    const result = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM feed_user_feeds
       WHERE source_account_id = $1 AND is_read = false AND is_hidden = false`,
      [this.sourceAccountId]
    );
    return result.rows[0]?.count ?? 0;
  }

  private async getActivitiesByVerb(): Promise<Record<ActivityVerb, number>> {
    const result = await this.query<{ verb: ActivityVerb; count: number }>(
      `SELECT verb, COUNT(*)::int as count FROM feed_activities
       WHERE source_account_id = $1
       GROUP BY verb`,
      [this.sourceAccountId]
    );

    const byVerb: Record<string, number> = {};
    for (const row of result.rows) {
      byVerb[row.verb] = row.count;
    }
    return byVerb as Record<ActivityVerb, number>;
  }

  private async getActivitiesByActorType(): Promise<Record<string, number>> {
    const result = await this.query<{ actor_type: string; count: number }>(
      `SELECT actor_type, COUNT(*)::int as count FROM feed_activities
       WHERE source_account_id = $1
       GROUP BY actor_type`,
      [this.sourceAccountId]
    );

    const byType: Record<string, number> = {};
    for (const row of result.rows) {
      byType[row.actor_type] = row.count;
    }
    return byType;
  }

  private async countRecentActivities(hours: number): Promise<number> {
    const result = await this.query<{ count: number }>(
      `SELECT COUNT(*)::int as count FROM feed_activities
       WHERE source_account_id = $1 AND created_at >= NOW() - INTERVAL '${hours} hours'`,
      [this.sourceAccountId]
    );
    return result.rows[0]?.count ?? 0;
  }

  private async getLastActivityDate(): Promise<Date | null> {
    const result = await this.query<{ created_at: Date }>(
      `SELECT created_at FROM feed_activities
       WHERE source_account_id = $1
       ORDER BY created_at DESC LIMIT 1`,
      [this.sourceAccountId]
    );
    return result.rows[0]?.created_at ?? null;
  }

  private async getUserLastActivityDate(userId: string): Promise<Date | null> {
    const result = await this.query<{ created_at: Date }>(
      `SELECT f.created_at FROM feed_user_feeds f
       WHERE f.source_account_id = $1 AND f.user_id = $2
       ORDER BY f.created_at DESC LIMIT 1`,
      [this.sourceAccountId, userId]
    );
    return result.rows[0]?.created_at ?? null;
  }

  // =========================================================================
  // Cleanup Operations
  // =========================================================================

  async cleanupOldActivities(retentionDays: number): Promise<number> {
    return this.execute(
      `DELETE FROM feed_activities
       WHERE source_account_id = $1 AND created_at < NOW() - INTERVAL '${retentionDays} days'`,
      [this.sourceAccountId]
    );
  }
}
