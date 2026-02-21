#!/usr/bin/env node
/**
 * HTTP server for notifications API
 * Multi-app aware: each request is scoped to a source_account_id
 */

import Fastify, { FastifyRequest } from 'fastify';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { config } from './config.js';
import { notificationService } from './service.js';
import { db, DatabaseClient } from './database.js';
import { SendNotificationRequest, SendNotificationResponse } from './types.js';

const logger = createLogger('notifications:server');

const fastify = Fastify({
  logger: {
    level: config.development.log_level,
  },
});

// =============================================================================
// Multi-app context middleware
// =============================================================================

fastify.decorateRequest('scopedDb', null);

fastify.addHook('onRequest', async (request: FastifyRequest) => {
  const ctx = getAppContext(request);
  (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
});

/** Extract the request-scoped DatabaseClient */
function scopedDb(request: unknown): DatabaseClient {
  return (request as Record<string, unknown>).scopedDb as DatabaseClient;
}

// =============================================================================
// Routes
// =============================================================================

// Health check
fastify.get('/health', async () => {
  return {
    status: 'ok',
    timestamp: new Date().toISOString(),
    service: 'notifications',
  };
});

// Send notification
fastify.post<{ Body: SendNotificationRequest }>(
  '/api/notifications/send',
  async (request, reply) => {
    const { user_id, template, channel, category, to, content, variables, priority, scheduled_at, metadata, tags } = request.body;

    // Validate required fields
    if (!user_id || !channel) {
      return reply.code(400).send({
        success: false,
        error: 'user_id and channel are required',
      });
    }

    // Validate recipient
    const hasRecipient =
      (channel === 'email' && to.email) ||
      (channel === 'push' && to.push_token) ||
      (channel === 'sms' && to.phone);

    if (!hasRecipient && !template) {
      return reply.code(400).send({
        success: false,
        error: 'Recipient required (email, phone, or push_token)',
      });
    }

    try {
      const reqDb = scopedDb(request);
      const result = await notificationService.sendWith(reqDb, {
        user_id,
        template_name: template,
        channel,
        category: category || 'transactional',
        recipient_email: to.email,
        recipient_phone: to.phone,
        recipient_push_token: to.push_token,
        subject: content?.subject,
        body_text: content?.body,
        body_html: content?.html,
        variables,
        priority,
        scheduled_at: scheduled_at ? new Date(scheduled_at) : undefined,
        metadata,
        tags,
      });

      if (result.success) {
        return {
          success: true,
          notification_id: result.notification_id,
          message: 'Notification queued for delivery',
        } as SendNotificationResponse;
      } else {
        return reply.code(400).send({
          success: false,
          error: result.error,
        } as SendNotificationResponse);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to send notification', { error: message });
      return reply.code(500).send({
        success: false,
        error: message,
      } as SendNotificationResponse);
    }
  }
);

// Get notification status
fastify.get<{ Params: { id: string } }>(
  '/api/notifications/:id',
  async (request, reply) => {
    const { id } = request.params;

    try {
      const notification = await scopedDb(request).getNotification(id);

      if (!notification) {
        return reply.code(404).send({
          error: 'Notification not found',
        });
      }

      return { notification };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get notification', { error: message });
      return reply.code(500).send({
        error: message,
      });
    }
  }
);

// List templates
fastify.get('/api/templates', async (request) => {
  try {
    const templates = await scopedDb(request).listTemplates();
    return {
      templates,
      total: templates.length,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to list templates', { error: message });
    throw error;
  }
});

// Get template
fastify.get<{ Params: { name: string } }>(
  '/api/templates/:name',
  async (request, reply) => {
    const { name } = request.params;

    try {
      const template = await scopedDb(request).getTemplate(name);

      if (!template) {
        return reply.code(404).send({
          error: 'Template not found',
        });
      }

      return { template };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get template', { error: message });
      throw error;
    }
  }
);

// Get delivery statistics
fastify.get<{ Querystring: { days?: string } }>(
  '/api/stats/delivery',
  async (request) => {
    const days = parseInt(request.query.days || '7');

    try {
      const stats = await scopedDb(request).getDeliveryStats(days);
      return { stats };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get delivery stats', { error: message });
      throw error;
    }
  }
);

// Get engagement metrics
fastify.get<{ Querystring: { days?: string } }>(
  '/api/stats/engagement',
  async (request) => {
    const days = parseInt(request.query.days || '7');

    try {
      const metrics = await scopedDb(request).getEngagementStats(days);
      return { metrics };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get engagement stats', { error: message });
      throw error;
    }
  }
);

// Webhook receiver (for provider callbacks)
fastify.post('/webhooks/notifications', async (request, reply) => {
  // NOTE: Webhook signature verification is provider-specific
  // Integration requirements:
  // 1. For SendGrid: Verify Event Webhook signature using public key
  // 2. For AWS SES: Verify SNS message signature
  // 3. For Mailgun: Verify HMAC signature using webhook signing key
  // 4. For Twilio: Verify X-Twilio-Signature header
  //
  // Delivery event processing should:
  // - Update notification status (delivered, opened, clicked, bounced, failed)
  // - Store engagement metrics in np_notif_engagement_events table
  // - Trigger analytics updates for provider health monitoring

  logger.info('Webhook received', { body: request.body });

  return { received: true };
});

// =============================================================================
// Server Lifecycle
// =============================================================================

const start = async () => {
  try {
    // Run multi-app migration on startup
    await db.migrateMultiApp();

    await fastify.listen({
      port: config.server.port,
      host: config.server.host,
    });

    logger.info(`Notifications server running on http://${config.server.host}:${config.server.port}`);
    logger.info('Endpoints: GET /health, POST /api/notifications/send, GET /api/notifications/:id, GET /api/templates, GET /api/templates/:name, GET /api/stats/delivery, GET /api/stats/engagement, POST /webhooks/notifications');
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  }
};

// Handle graceful shutdown
const gracefulShutdown = async () => {
  logger.info('Shutting down gracefully...');

  try {
    await fastify.close();
    await db.close();
    logger.info('Server closed');
    process.exit(0);
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Error during shutdown', { error: message });
    process.exit(1);
  }
};

process.on('SIGTERM', gracefulShutdown);
process.on('SIGINT', gracefulShutdown);

// Start server if run directly
if (require.main === module) {
  start();
}

export { fastify };
