/**
 * Bots Plugin Server
 * HTTP server for bot framework, commands, subscriptions, marketplace
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { BotsDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateBotRequest, UpdateBotRequest,
  CreateCommandRequest,
  CreateSubscriptionRequest,
  InstallBotRequest,
  CreateReviewRequest,
  CreateApiKeyRequest,
  MarketplaceQuery,
} from './types.js';

const logger = createLogger('bots:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);
  const db = new BotsDatabase();
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({ logger: false, bodyLimit: 10 * 1024 * 1024 });
  await app.register(cors, { origin: true, credentials: true });

  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 100,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): BotsDatabase {
    return (request as Record<string, unknown>).scopedDb as BotsDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => ({ status: 'ok', plugin: 'bots', timestamp: new Date().toISOString() }));

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'bots', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({ ready: false, plugin: 'bots', error: 'Database unavailable', timestamp: new Date().toISOString() });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return { alive: true, plugin: 'bots', version: '1.0.0', uptime: process.uptime(), memory: process.memoryUsage(), stats, timestamp: new Date().toISOString() };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return { plugin: 'bots', version: '1.0.0', status: 'running', marketplaceEnabled: fullConfig.marketplaceEnabled, oauthEnabled: fullConfig.oauthEnabled, stats, timestamp: new Date().toISOString() };
  });

  // =========================================================================
  // Bot Management
  // =========================================================================

  app.post<{ Body: CreateBotRequest }>('/api/bots', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.name || !body.username || !body.ownerId) {
        return reply.status(400).send({ error: 'name, username, and ownerId are required' });
      }
      const { bot, token } = await scopedDb(request).createBot(body);
      return reply.status(201).send({ success: true, bot: { id: bot.id, name: bot.name, username: bot.username, token } });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create bot', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get<{ Params: { botId: string } }>('/api/bots/:botId', async (request, reply) => {
    const bot = await scopedDb(request).getBot(request.params.botId);
    if (!bot) return reply.status(404).send({ error: 'Bot not found' });
    return { success: true, bot };
  });

  app.get<{ Querystring: { ownerId?: string; isPublic?: string; limit?: string; offset?: string } }>(
    '/api/bots',
    async (request) => {
      const bots = await scopedDb(request).listBots({
        ownerId: request.query.ownerId,
        isPublic: request.query.isPublic === 'true' ? true : request.query.isPublic === 'false' ? false : undefined,
        limit: request.query.limit ? parseInt(request.query.limit, 10) : undefined,
        offset: request.query.offset ? parseInt(request.query.offset, 10) : undefined,
      });
      return { success: true, bots, count: bots.length };
    }
  );

  app.patch<{ Params: { botId: string }; Body: UpdateBotRequest }>('/api/bots/:botId', async (request, reply) => {
    try {
      const bot = await scopedDb(request).updateBot(request.params.botId, request.body);
      if (!bot) return reply.status(404).send({ error: 'Bot not found' });
      return { success: true, bot };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update bot', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.delete<{ Params: { botId: string } }>('/api/bots/:botId', async (request, reply) => {
    const deleted = await scopedDb(request).deleteBot(request.params.botId);
    if (!deleted) return reply.status(404).send({ error: 'Bot not found' });
    return { success: true, deleted: true };
  });

  // =========================================================================
  // Command Management
  // =========================================================================

  app.post<{ Params: { botId: string }; Body: Omit<CreateCommandRequest, 'botId'> }>(
    '/api/bots/:botId/commands',
    async (request, reply) => {
      try {
        const body = { ...request.body, botId: request.params.botId };
        if (!body.command || !body.description) {
          return reply.status(400).send({ error: 'command and description are required' });
        }
        const command = await scopedDb(request).createCommand(body);
        return reply.status(201).send({ success: true, command });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create command', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { botId: string } }>('/api/bots/:botId/commands', async (request) => {
    const commands = await scopedDb(request).listCommands(request.params.botId);
    return { success: true, commands, count: commands.length };
  });

  app.delete<{ Params: { botId: string; commandId: string } }>(
    '/api/bots/:botId/commands/:commandId',
    async (request, reply) => {
      const deleted = await scopedDb(request).deleteCommand(request.params.commandId);
      if (!deleted) return reply.status(404).send({ error: 'Command not found' });
      return { success: true, deleted: true };
    }
  );

  // =========================================================================
  // Event Subscriptions
  // =========================================================================

  app.post<{ Params: { botId: string }; Body: Omit<CreateSubscriptionRequest, 'botId'> }>(
    '/api/bots/:botId/subscriptions',
    async (request, reply) => {
      try {
        const body = { ...request.body, botId: request.params.botId };
        if (!body.eventType) {
          return reply.status(400).send({ error: 'eventType is required' });
        }
        const subscription = await scopedDb(request).createSubscription(body);
        return reply.status(201).send({ success: true, subscription });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create subscription', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { botId: string } }>('/api/bots/:botId/subscriptions', async (request) => {
    const subscriptions = await scopedDb(request).listSubscriptions(request.params.botId);
    return { success: true, subscriptions, count: subscriptions.length };
  });

  app.delete<{ Params: { botId: string; subscriptionId: string } }>(
    '/api/bots/:botId/subscriptions/:subscriptionId',
    async (request, reply) => {
      const deleted = await scopedDb(request).deleteSubscription(request.params.subscriptionId);
      if (!deleted) return reply.status(404).send({ error: 'Subscription not found' });
      return { success: true, deleted: true };
    }
  );

  // =========================================================================
  // Bot Installation
  // =========================================================================

  app.post<{ Params: { workspaceId: string; botId: string }; Body: Omit<InstallBotRequest, 'botId' | 'workspaceId'> }>(
    '/api/workspaces/:workspaceId/bots/:botId/install',
    async (request, reply) => {
      try {
        const body = { ...request.body, botId: request.params.botId, workspaceId: request.params.workspaceId };
        if (!body.installedBy || body.grantedPermissions === undefined) {
          return reply.status(400).send({ error: 'installedBy and grantedPermissions are required' });
        }
        const installation = await scopedDb(request).installBot(body);
        return reply.status(201).send({ success: true, installation });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to install bot', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.delete<{ Params: { workspaceId: string; botId: string }; Body: { uninstalledBy: string } }>(
    '/api/workspaces/:workspaceId/bots/:botId/uninstall',
    async (request, reply) => {
      try {
        // Find the installation
        const installations = await scopedDb(request).listInstallations(request.params.workspaceId);
        const installation = installations.find(i => i.bot_id === request.params.botId);
        if (!installation) return reply.status(404).send({ error: 'Installation not found' });

        const uninstalled = await scopedDb(request).uninstallBot(installation.id, request.body.uninstalledBy);
        if (!uninstalled) return reply.status(404).send({ error: 'Installation not found or already inactive' });
        return { success: true, uninstalled: true };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to uninstall bot', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { workspaceId: string } }>('/api/workspaces/:workspaceId/bots', async (request) => {
    const installations = await scopedDb(request).listInstallations(request.params.workspaceId);
    return { success: true, installations, count: installations.length };
  });

  // =========================================================================
  // Marketplace
  // =========================================================================

  app.get<{ Querystring: MarketplaceQuery & { limit?: string; offset?: string } }>(
    '/api/marketplace/bots',
    async (request) => {
      const bots = await scopedDb(request).searchMarketplace({
        category: request.query.category,
        verified: request.query.verified,
        search: request.query.search,
        sort: request.query.sort as 'installs' | 'rating' | 'recent' | undefined,
        limit: request.query.limit ? parseInt(request.query.limit as string, 10) : undefined,
        offset: request.query.offset ? parseInt(request.query.offset as string, 10) : undefined,
      });
      return { success: true, bots, count: bots.length };
    }
  );

  app.get<{ Params: { botId: string } }>('/api/marketplace/bots/:botId', async (request, reply) => {
    const bot = await scopedDb(request).getBot(request.params.botId);
    if (!bot || !bot.is_public) return reply.status(404).send({ error: 'Bot not found' });
    return { success: true, bot };
  });

  app.post<{ Params: { botId: string }; Body: Omit<CreateReviewRequest, 'botId'> }>(
    '/api/marketplace/bots/:botId/reviews',
    async (request, reply) => {
      try {
        const body = { ...request.body, botId: request.params.botId };
        if (!body.userId || !body.rating) {
          return reply.status(400).send({ error: 'userId and rating are required' });
        }
        if (body.rating < 1 || body.rating > 5) {
          return reply.status(400).send({ error: 'rating must be between 1 and 5' });
        }
        const review = await scopedDb(request).createReview(body);
        return reply.status(201).send({ success: true, review });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create review', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { botId: string }; Querystring: { limit?: string; offset?: string } }>(
    '/api/marketplace/bots/:botId/reviews',
    async (request) => {
      const reviews = await scopedDb(request).listReviews(
        request.params.botId,
        request.query.limit ? parseInt(request.query.limit, 10) : 20,
        request.query.offset ? parseInt(request.query.offset, 10) : 0,
      );
      return { success: true, reviews, count: reviews.length };
    }
  );

  // =========================================================================
  // Bot API (for bot developers)
  // =========================================================================

  app.post<{ Body: { channelId: string; content: string; messageType?: string } }>(
    '/api/bot/messages',
    async (request, reply) => {
      try {
        // Authenticate bot using Authorization header
        const authHeader = request.headers.authorization;
        if (!authHeader || !authHeader.startsWith('Bearer ')) {
          return reply.status(401).send({ error: 'Missing or invalid Authorization header. Use: Bearer nbot_...' });
        }

        const token = authHeader.substring(7); // Remove 'Bearer ' prefix
        const bot = await scopedDb(request).validateBotToken(token);
        if (!bot) {
          return reply.status(401).send({ error: 'Invalid or disabled bot token' });
        }

        const { channelId, content, messageType } = request.body;
        if (!channelId || !content) {
          return reply.status(400).send({ error: 'channelId and content are required' });
        }

        // In a real implementation, this would create the message in the chat system
        // and then track it in bot_messages
        const messageId = crypto.randomUUID();

        const botMessage = await scopedDb(request).createBotMessage(
          bot.id, messageId, channelId, messageType ?? 'text'
        );

        return reply.status(201).send({ success: true, messageId, botMessage });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send bot message', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.post<{ Params: { interactionId: string }; Body: { content: string; messageType?: string } }>(
    '/api/bot/interactions/:interactionId/respond',
    async (request, reply) => {
      try {
        const responseMessageId = crypto.randomUUID();
        await scopedDb(request).markInteractionResponded(request.params.interactionId, responseMessageId);
        return { success: true, responseMessageId };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to respond to interaction', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // API Key Management
  // =========================================================================

  app.post<{ Params: { botId: string }; Body: Omit<CreateApiKeyRequest, 'botId'> }>(
    '/api/bots/:botId/api-keys',
    async (request, reply) => {
      try {
        const body = { ...request.body, botId: request.params.botId };
        if (!body.keyName || body.permissions === undefined) {
          return reply.status(400).send({ error: 'keyName and permissions are required' });
        }
        const { apiKey, rawKey } = await scopedDb(request).createApiKey(body);
        return reply.status(201).send({ success: true, apiKey: { id: apiKey.id, keyName: apiKey.key_name, keyPrefix: apiKey.key_prefix }, rawKey });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create API key', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.post<{ Params: { botId: string; keyId: string }; Body: { revokedBy: string; reason?: string } }>(
    '/api/bots/:botId/api-keys/:keyId/revoke',
    async (request, reply) => {
      try {
        const revoked = await scopedDb(request).revokeApiKey(request.params.keyId, request.body.revokedBy, request.body.reason);
        if (!revoked) return reply.status(404).send({ error: 'API key not found or already revoked' });
        return { success: true, revoked: true };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to revoke API key', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post('/webhook', async (request, reply) => {
    try {
      const payload = request.body as Record<string, unknown>;
      const eventType = payload.type as string ?? payload.event as string;
      if (!eventType) return reply.status(400).send({ error: 'Missing event type' });
      await scopedDb(request).insertWebhookEvent(eventType, payload);
      return { received: true, type: eventType };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Webhook processing failed', { error: message });
      return reply.status(500).send({ error: 'Processing failed' });
    }
  });

  return app;
}

export async function startServer(config?: Partial<Config>): Promise<void> {
  const fullConfig = loadConfig(config);
  const app = await createServer(config);

  try {
    await app.listen({ port: fullConfig.port, host: fullConfig.host });
    logger.info('Bots plugin server running', { port: fullConfig.port, host: fullConfig.host });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
