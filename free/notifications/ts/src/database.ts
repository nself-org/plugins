/**
 * Database client for notification operations
 * Multi-app aware: all queries are scoped by source_account_id
 */

import { Pool, PoolClient } from 'pg';
import { config } from './config.js';
import {
  Notification,
  NotificationTemplate,
  NotificationPreference,
  NotificationProvider,
  QueueItem,
  CreateNotificationInput,
  DeliveryStats,
  EngagementStats,
} from './types.js';

export class DatabaseClient {
  private pool: Pool;
  private sourceAccountId: string;

  constructor(sourceAccountId = 'primary') {
    this.pool = new Pool({
      host: config.database.host,
      port: config.database.port,
      database: config.database.database,
      user: config.database.user,
      password: config.database.password,
      ssl: config.database.ssl ? { rejectUnauthorized: false } : false,
    });
    this.sourceAccountId = sourceAccountId;
  }

  /**
   * Return a new DatabaseClient scoped to a different source account.
   * Shares the same underlying connection pool.
   */
  forSourceAccount(accountId: string): DatabaseClient {
    const scoped = Object.create(DatabaseClient.prototype) as DatabaseClient;
    scoped.pool = this.pool;
    scoped.sourceAccountId = accountId;
    return scoped;
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async getClient(): Promise<PoolClient> {
    return this.pool.connect();
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // =============================================================================
  // Schema Migration
  // =============================================================================

  /**
   * Add source_account_id column to all notification tables if it does not exist.
   * Safe to call repeatedly (idempotent).
   */
  async migrateMultiApp(): Promise<void> {
    const tables = [
      'np_notifications_templates',
      'np_notifications_messages',
      'np_notifications_queue',
      'np_notifications_preferences',
      'np_notifications_providers',
      'np_notifications_batches',
    ];

    const client = await this.getClient();
    try {
      for (const table of tables) {
        const colCheck = await client.query(
          `SELECT 1 FROM information_schema.columns
           WHERE table_schema = 'public'
             AND table_name = $1
             AND column_name = 'source_account_id'`,
          [table]
        );
        if (colCheck.rowCount === 0) {
          await client.query(
            `ALTER TABLE ${table} ADD COLUMN source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`
          );
        }
      }
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Templates
  // =============================================================================

  async getTemplate(name: string): Promise<NotificationTemplate | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_notifications_templates WHERE name = $1 AND active = true AND source_account_id = $2',
        [name, this.sourceAccountId]
      );
      return result.rows[0] || null;
    } finally {
      client.release();
    }
  }

  async listTemplates(): Promise<NotificationTemplate[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_notifications_templates WHERE active = true AND source_account_id = $1 ORDER BY category, name',
        [this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Notifications
  // =============================================================================

  async createNotification(input: CreateNotificationInput): Promise<Notification> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `INSERT INTO np_notifications_messages (
          user_id, template_name, channel, category,
          recipient_email, recipient_phone, recipient_push_token,
          subject, body_text, body_html, priority,
          scheduled_at, metadata, tags, status, source_account_id
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'pending', $15)
        RETURNING *`,
        [
          input.user_id,
          input.template_name,
          input.channel,
          input.category || 'transactional',
          input.recipient_email,
          input.recipient_phone,
          input.recipient_push_token,
          input.subject,
          input.body_text,
          input.body_html,
          input.priority || 5,
          input.scheduled_at,
          JSON.stringify(input.metadata || {}),
          JSON.stringify(input.tags || []),
          this.sourceAccountId,
        ]
      );
      return result.rows[0];
    } finally {
      client.release();
    }
  }

  async getNotification(id: string): Promise<Notification | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        'SELECT * FROM np_notifications_messages WHERE id = $1 AND source_account_id = $2',
        [id, this.sourceAccountId]
      );
      return result.rows[0] || null;
    } finally {
      client.release();
    }
  }

  async updateNotificationStatus(
    id: string,
    status: string,
    updates: Partial<Notification> = {}
  ): Promise<void> {
    const client = await this.getClient();
    try {
      const fields = Object.keys(updates);
      const values = Object.values(updates);

      let query = 'UPDATE np_notifications_messages SET status = $1, updated_at = NOW()';
      const params: unknown[] = [status];

      fields.forEach((field, index) => {
        query += `, ${field} = $${index + 2}`;
        params.push(values[index]);
      });

      query += ` WHERE id = $${params.length + 1} AND source_account_id = $${params.length + 2}`;
      params.push(id, this.sourceAccountId);

      await client.query(query, params);
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Queue
  // =============================================================================

  async addToQueue(notificationId: string, priority: number = 5): Promise<void> {
    const client = await this.getClient();
    try {
      await client.query(
        `INSERT INTO np_notifications_queue (notification_id, status, priority, next_attempt_at, source_account_id)
         VALUES ($1, 'pending', $2, NOW(), $3)
         ON CONFLICT (notification_id) DO NOTHING`,
        [notificationId, priority, this.sourceAccountId]
      );
    } finally {
      client.release();
    }
  }

  async getNextQueueItem(): Promise<QueueItem | null> {
    const client = await this.getClient();
    try {
      // Get and lock the next pending item scoped to this account
      const result = await client.query(
        `UPDATE np_notifications_queue
         SET status = 'processing', processing_started_at = NOW(), updated_at = NOW()
         WHERE id = (
           SELECT id FROM np_notifications_queue
           WHERE status = 'pending'
             AND next_attempt_at <= NOW()
             AND attempts < max_attempts
             AND source_account_id = $1
           ORDER BY priority ASC, next_attempt_at ASC
           LIMIT 1
           FOR UPDATE SKIP LOCKED
         )
         RETURNING *`,
        [this.sourceAccountId]
      );
      return result.rows[0] || null;
    } finally {
      client.release();
    }
  }

  async updateQueueItem(id: string, status: string, error?: string): Promise<void> {
    const client = await this.getClient();
    try {
      if (status === 'completed') {
        await client.query(
          `UPDATE np_notifications_queue
           SET status = $1, processing_completed_at = NOW(), updated_at = NOW()
           WHERE id = $2 AND source_account_id = $3`,
          [status, id, this.sourceAccountId]
        );
      } else if (status === 'failed') {
        await client.query(
          `UPDATE np_notifications_queue
           SET status = 'pending',
               attempts = attempts + 1,
               last_error = $1,
               next_attempt_at = NOW() + (attempts * interval '1 second'),
               updated_at = NOW()
           WHERE id = $2 AND source_account_id = $3`,
          [error, id, this.sourceAccountId]
        );
      }
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Preferences
  // =============================================================================

  async getUserPreference(
    userId: string,
    channel: string,
    category: string
  ): Promise<NotificationPreference | null> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT * FROM np_notifications_preferences
         WHERE user_id = $1 AND channel = $2 AND category = $3 AND source_account_id = $4`,
        [userId, channel, category, this.sourceAccountId]
      );
      return result.rows[0] || null;
    } finally {
      client.release();
    }
  }

  async checkUserCanReceive(
    userId: string,
    channel: string,
    category: string
  ): Promise<boolean> {
    const client = await this.getClient();
    try {
      // Inline preference check scoped by source_account_id instead of
      // relying on the unscoped SQL function get_user_notification_preference.
      const result = await client.query(
        `SELECT enabled FROM np_notifications_preferences
         WHERE user_id = $1 AND channel = $2 AND category = $3 AND source_account_id = $4`,
        [userId, channel, category, this.sourceAccountId]
      );
      // Default to enabled if no preference found
      return result.rows[0]?.enabled ?? true;
    } finally {
      client.release();
    }
  }

  async checkRateLimit(
    userId: string,
    channel: string,
    windowSeconds: number,
    maxCount: number
  ): Promise<boolean> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT COUNT(*) AS cnt FROM np_notifications_messages
         WHERE user_id = $1
           AND channel = $2
           AND created_at >= NOW() - ($3 || ' seconds')::INTERVAL
           AND source_account_id = $4`,
        [userId, channel, windowSeconds, this.sourceAccountId]
      );
      const count = parseInt(result.rows[0]?.cnt ?? '0', 10);
      return count < maxCount;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Providers
  // =============================================================================

  async getEnabledProviders(type: string): Promise<NotificationProvider[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT * FROM np_notifications_providers
         WHERE type = $1 AND enabled = true AND source_account_id = $2
         ORDER BY priority ASC`,
        [type, this.sourceAccountId]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async updateProviderHealth(
    name: string,
    success: boolean,
    _response?: unknown
  ): Promise<void> {
    const client = await this.getClient();
    try {
      if (success) {
        await client.query(
          `UPDATE np_notifications_providers
           SET success_count = success_count + 1,
               last_success_at = NOW(),
               health_status = 'healthy',
               updated_at = NOW()
           WHERE name = $1 AND source_account_id = $2`,
          [name, this.sourceAccountId]
        );
      } else {
        await client.query(
          `UPDATE np_notifications_providers
           SET failure_count = failure_count + 1,
               last_failure_at = NOW(),
               health_status = CASE
                 WHEN failure_count > 10 THEN 'unhealthy'
                 WHEN failure_count > 5 THEN 'degraded'
                 ELSE health_status
               END,
               updated_at = NOW()
           WHERE name = $1 AND source_account_id = $2`,
          [name, this.sourceAccountId]
        );
      }
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Statistics
  // =============================================================================

  async getDeliveryStats(days: number = 7): Promise<DeliveryStats[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT
           channel,
           category,
           DATE_TRUNC('day', created_at) AS date,
           COUNT(*) AS total,
           COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
           COUNT(*) FILTER (WHERE status = 'failed') AS failed,
           COUNT(*) FILTER (WHERE status = 'bounced') AS bounced,
           ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / NULLIF(COUNT(*), 0), 2) AS delivery_rate
         FROM np_notifications_messages
         WHERE source_account_id = $1
           AND created_at >= NOW() - INTERVAL '1 day' * $2
         GROUP BY channel, category, DATE_TRUNC('day', created_at)
         ORDER BY date DESC, channel`,
        [this.sourceAccountId, days]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  async getEngagementStats(days: number = 7): Promise<EngagementStats[]> {
    const client = await this.getClient();
    try {
      const result = await client.query(
        `SELECT
           channel,
           category,
           DATE_TRUNC('day', created_at) AS date,
           COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
           COUNT(*) FILTER (WHERE opened_at IS NOT NULL) AS opened,
           COUNT(*) FILTER (WHERE clicked_at IS NOT NULL) AS clicked,
           COUNT(*) FILTER (WHERE unsubscribed_at IS NOT NULL) AS unsubscribed,
           ROUND(100.0 * COUNT(*) FILTER (WHERE opened_at IS NOT NULL) / NULLIF(COUNT(*) FILTER (WHERE status = 'delivered'), 0), 2) AS open_rate,
           ROUND(100.0 * COUNT(*) FILTER (WHERE clicked_at IS NOT NULL) / NULLIF(COUNT(*) FILTER (WHERE status = 'delivered'), 0), 2) AS click_rate
         FROM np_notifications_messages
         WHERE channel = 'email'
           AND source_account_id = $1
           AND created_at >= NOW() - INTERVAL '1 day' * $2
         GROUP BY channel, category, DATE_TRUNC('day', created_at)
         ORDER BY date DESC`,
        [this.sourceAccountId, days]
      );
      return result.rows;
    } finally {
      client.release();
    }
  }

  // =============================================================================
  // Multi-App Cleanup
  // =============================================================================

  /**
   * Delete all data for a given source account across all notification tables.
   * Returns total number of deleted rows.
   */
  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    const tables = [
      'np_notifications_queue',
      'np_notifications_messages',
      'np_notifications_preferences',
      'np_notifications_templates',
      'np_notifications_providers',
      'np_notifications_batches',
    ];

    const client = await this.getClient();
    let total = 0;
    try {
      for (const table of tables) {
        const result = await client.query(
          `DELETE FROM ${table} WHERE source_account_id = $1`,
          [sourceAccountId]
        );
        total += result.rowCount ?? 0;
      }
    } finally {
      client.release();
    }
    return total;
  }
}

// Singleton instance
export const db = new DatabaseClient();
