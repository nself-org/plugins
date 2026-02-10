/**
 * Notification service - main business logic
 * Multi-app aware: accepts a scoped DatabaseClient for per-request isolation
 */

import { createLogger } from '@nself/plugin-utils';
import { db, DatabaseClient } from './database.js';
import { TemplateEngine } from './template.js';
import { config } from './config.js';
import {
  CreateNotificationInput,
  SendNotificationResult,
  Notification,
  TemplateVariables,
} from './types.js';

const logger = createLogger('notifications:service');

export class NotificationService {
  /**
   * Send a notification using the default (singleton) database client.
   * Kept for backward compatibility with CLI and worker usage.
   */
  async send(input: CreateNotificationInput): Promise<SendNotificationResult> {
    return this.sendWith(db, input);
  }

  /**
   * Send a notification using a specific (scoped) database client.
   * This is the multi-app-aware entry point used by the server.
   */
  async sendWith(scopedDb: DatabaseClient, input: CreateNotificationInput): Promise<SendNotificationResult> {
    try {
      // Check if user can receive this type of notification
      const canReceive = await scopedDb.checkUserCanReceive(
        input.user_id,
        input.channel,
        input.category || 'transactional'
      );

      if (!canReceive) {
        return {
          success: false,
          error: 'User has opted out of this notification type',
        };
      }

      // Check rate limits (skip for transactional)
      if (input.category !== 'transactional') {
        const rateLimitConfig = config.rate_limits[input.channel];
        const allowed = await scopedDb.checkRateLimit(
          input.user_id,
          input.channel,
          rateLimitConfig.window,
          rateLimitConfig.per_user
        );

        if (!allowed) {
          return {
            success: false,
            error: 'Rate limit exceeded',
          };
        }
      }

      // Render from template if specified
      let renderedContent = {
        subject: input.subject,
        body_text: input.body_text,
        body_html: input.body_html,
      };

      if (input.template_name) {
        const template = await scopedDb.getTemplate(input.template_name);
        if (!template) {
          return {
            success: false,
            error: `Template not found: ${input.template_name}`,
          };
        }

        const rendered = TemplateEngine.renderNotification(
          template,
          input.variables || {}
        );

        renderedContent = {
          subject: rendered.subject || input.subject,
          body_text: rendered.body_text || input.body_text,
          body_html: rendered.body_html || input.body_html,
        };
      }

      // Create notification record
      const notification = await scopedDb.createNotification({
        ...input,
        ...renderedContent,
      });

      // Add to processing queue
      await scopedDb.addToQueue(notification.id, input.priority || 5);

      // Update status
      await scopedDb.updateNotificationStatus(notification.id, 'queued');

      return {
        success: true,
        notification_id: notification.id,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to send notification', { error: message });
      return {
        success: false,
        error: message,
      };
    }
  }

  /**
   * Get notification status
   */
  async getStatus(notificationId: string): Promise<Notification | null> {
    return db.getNotification(notificationId);
  }

  /**
   * Process a notification (called by worker)
   */
  async process(notification: Notification): Promise<SendNotificationResult> {
    try {
      // Get enabled providers for this channel
      const providers = await db.getEnabledProviders(notification.channel);

      if (providers.length === 0) {
        return {
          success: false,
          error: `No providers enabled for channel: ${notification.channel}`,
        };
      }

      // Try each provider in priority order
      for (const provider of providers) {
        try {
          // TODO: Implement actual provider sending
          // const providerInstance = this.getProviderInstance(provider);
          // const result = await providerInstance.send(notification);

          // For now, simulate success
          if (config.development.dry_run) {
            logger.info('[DRY RUN] Would send notification', {
              id: notification.id,
              channel: notification.channel,
              provider: provider.name,
              to: notification.recipient_email || notification.recipient_phone,
            });

            await db.updateProviderHealth(provider.name, true);
            await db.updateNotificationStatus(notification.id, 'sent', {
              provider: provider.name,
              sent_at: new Date(),
            });

            return { success: true, notification_id: notification.id };
          }

          // Real implementation would send here
          // const result = await this.sendViaProvider(provider, notification);

          await db.updateProviderHealth(provider.name, true);
          await db.updateNotificationStatus(notification.id, 'delivered', {
            provider: provider.name,
            sent_at: new Date(),
            delivered_at: new Date(),
          });

          return { success: true, notification_id: notification.id };
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error(`Provider ${provider.name} failed`, { error: message });
          await db.updateProviderHealth(provider.name, false);
          // Continue to next provider
          continue;
        }
      }

      // All providers failed
      return {
        success: false,
        error: 'All providers failed',
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      return {
        success: false,
        error: message,
      };
    }
  }

  /**
   * Get delivery statistics
   */
  async getDeliveryStats(days: number = 7): Promise<any[]> {
    return db.getDeliveryStats(days);
  }

  /**
   * Get engagement metrics
   */
  async getEngagementStats(days: number = 7): Promise<any[]> {
    return db.getEngagementStats(days);
  }
}

export const notificationService = new NotificationService();
