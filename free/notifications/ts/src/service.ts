/**
 * Notification service - main business logic
 * Multi-app aware: accepts a scoped DatabaseClient for per-request isolation
 */

import { createLogger } from '@nself/plugin-utils';
import { db, DatabaseClient } from './database.js';
import { TemplateEngine } from './template.js';
import { config } from './config.js';
import { deliveryManager } from './delivery.js';
import {
  CreateNotificationInput,
  SendNotificationResult,
  Notification,
  TemplateVariables,
  DeliveryStats,
  EngagementStats,
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
          // Dry run mode - simulate delivery
          if (config.development.dry_run) {
            logger.info('[DRY RUN] Would send notification', {
              id: notification.id,
              channel: notification.channel,
              provider: provider.name,
              to: notification.recipient_email || notification.recipient_phone || notification.recipient_push_token,
            });

            await db.updateProviderHealth(provider.name, true);
            await db.updateNotificationStatus(notification.id, 'sent', {
              provider: provider.name,
              sent_at: new Date(),
            });

            return { success: true, notification_id: notification.id };
          }

          // Actual delivery using delivery manager
          let deliveryResult;

          switch (notification.channel) {
            case 'email':
              if (!notification.recipient_email) {
                throw new Error('Email recipient required');
              }
              deliveryResult = await deliveryManager.sendEmail({
                to: notification.recipient_email,
                subject: notification.subject || 'Notification',
                text: notification.body_text,
                html: notification.body_html,
              });
              break;

            case 'push':
              if (!notification.recipient_push_token) {
                throw new Error('Push token required');
              }
              deliveryResult = await deliveryManager.sendPush({
                token: notification.recipient_push_token,
                title: notification.subject || 'Notification',
                body: notification.body_text || '',
              });
              break;

            case 'sms':
              if (!notification.recipient_phone) {
                throw new Error('Phone number required');
              }
              deliveryResult = await deliveryManager.sendSMS({
                to: notification.recipient_phone,
                body: notification.body_text || '',
              });
              break;

            default:
              throw new Error(`Unsupported channel: ${notification.channel}`);
          }

          // Check delivery result
          if (deliveryResult.success) {
            await db.updateProviderHealth(provider.name, true);
            await db.updateNotificationStatus(notification.id, 'delivered', {
              provider: provider.name,
              provider_message_id: deliveryResult.message_id,
              sent_at: new Date(),
              delivered_at: new Date(),
            });

            logger.info('Notification delivered successfully', {
              id: notification.id,
              channel: notification.channel,
              provider: provider.name,
              message_id: deliveryResult.message_id,
            });

            return {
              success: true,
              notification_id: notification.id,
              provider_response: deliveryResult.provider_response,
            };
          } else {
            throw new Error(deliveryResult.error || 'Delivery failed');
          }
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error(`Provider ${provider.name} failed`, { error: message });
          await db.updateProviderHealth(provider.name, false);

          // Update notification with error
          await db.updateNotificationStatus(notification.id, 'failed', {
            provider: provider.name,
            error_message: message,
            failed_at: new Date(),
          });

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
  async getDeliveryStats(days: number = 7): Promise<DeliveryStats[]> {
    return db.getDeliveryStats(days);
  }

  /**
   * Get engagement metrics
   */
  async getEngagementStats(days: number = 7): Promise<EngagementStats[]> {
    return db.getEngagementStats(days);
  }
}

export const notificationService = new NotificationService();
