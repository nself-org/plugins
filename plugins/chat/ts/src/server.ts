/**
 * Chat Plugin Server
 * HTTP server for REST API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ChatDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateConversationInput,
  UpdateConversationInput,
  AddParticipantInput,
  UpdateParticipantInput,
  SendMessageInput,
  UpdateMessageInput,
  UpdateReadReceiptInput,
  CreateModerationActionInput,
  MessageSearchQuery,
} from './types.js';

const logger = createLogger('chat:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new ChatDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB for attachments
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  /** Extract scoped ChatDatabase from request */
  function scopedDb(request: unknown): ChatDatabase {
    return (request as Record<string, unknown>).scopedDb as ChatDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'chat', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'chat', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'chat',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'chat',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        conversations: stats.conversations,
        messages: stats.messages,
        activeConversations: stats.activeConversations,
      },
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'chat',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Conversation Endpoints
  // =========================================================================

  app.post<{ Body: CreateConversationInput }>('/v1/conversations', async (request, reply) => {
    try {
      const conversation = await scopedDb(request).createConversation(request.body);
      return reply.status(201).send(conversation);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create conversation', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.get('/v1/conversations', async (request) => {
    const { user_id, limit = 50, offset = 0 } = request.query as {
      user_id?: string;
      limit?: number;
      offset?: number;
    };

    const conversations = await scopedDb(request).listConversations(user_id, limit, offset);
    const total = await scopedDb(request).countConversations(user_id);

    return { data: conversations, total, limit, offset };
  });

  app.get<{ Params: { id: string } }>('/v1/conversations/:id', async (request, reply) => {
    const { id } = request.params;
    const { user_id } = request.query as { user_id?: string };

    const conversation = await scopedDb(request).getConversationWithDetails(id, user_id);
    if (!conversation) {
      return reply.status(404).send({ error: 'Conversation not found' });
    }

    return conversation;
  });

  app.put<{ Params: { id: string }; Body: UpdateConversationInput }>(
    '/v1/conversations/:id',
    async (request, reply) => {
      const { id } = request.params;
      const conversation = await scopedDb(request).updateConversation(id, request.body);

      if (!conversation) {
        return reply.status(404).send({ error: 'Conversation not found' });
      }

      return conversation;
    }
  );

  app.delete<{ Params: { id: string } }>('/v1/conversations/:id', async (request, reply) => {
    const { id } = request.params;

    // Archive instead of delete
    const conversation = await scopedDb(request).updateConversation(id, { is_archived: true });

    if (!conversation) {
      return reply.status(404).send({ error: 'Conversation not found' });
    }

    return { success: true, conversation };
  });

  // =========================================================================
  // Participant Endpoints
  // =========================================================================

  app.post<{ Params: { id: string }; Body: AddParticipantInput }>(
    '/v1/conversations/:id/participants',
    async (request, reply) => {
      const { id } = request.params;

      try {
        const participant = await scopedDb(request).addParticipant(id, request.body);
        return reply.status(201).send(participant);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to add participant', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.get<{ Params: { id: string } }>('/v1/conversations/:id/participants', async (request) => {
    const { id } = request.params;
    const participants = await scopedDb(request).listParticipants(id);
    return { data: participants };
  });

  app.delete<{ Params: { id: string; userId: string } }>(
    '/v1/conversations/:id/participants/:userId',
    async (request, reply) => {
      const { id, userId } = request.params;
      const success = await scopedDb(request).removeParticipant(id, userId);

      if (!success) {
        return reply.status(404).send({ error: 'Participant not found' });
      }

      return { success: true };
    }
  );

  app.put<{ Params: { id: string; userId: string }; Body: UpdateParticipantInput }>(
    '/v1/conversations/:id/participants/:userId',
    async (request, reply) => {
      const { id, userId } = request.params;
      const participant = await scopedDb(request).updateParticipant(id, userId, request.body);

      if (!participant) {
        return reply.status(404).send({ error: 'Participant not found' });
      }

      return participant;
    }
  );

  // =========================================================================
  // Message Endpoints
  // =========================================================================

  app.post<{ Params: { id: string }; Body: SendMessageInput }>(
    '/v1/conversations/:id/messages',
    async (request, reply) => {
      const { id } = request.params;

      // Validate message length
      if (request.body.content && request.body.content.length > fullConfig.maxMessageLength) {
        return reply.status(400).send({
          error: `Message content exceeds maximum length of ${fullConfig.maxMessageLength}`,
        });
      }

      // Validate attachments
      if (request.body.attachments && request.body.attachments.length > fullConfig.maxAttachments) {
        return reply.status(400).send({
          error: `Number of attachments exceeds maximum of ${fullConfig.maxAttachments}`,
        });
      }

      try {
        const message = await scopedDb(request).sendMessage(id, request.body);
        return reply.status(201).send(message);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send message', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.get<{ Params: { id: string } }>('/v1/conversations/:id/messages', async (request) => {
    const { id } = request.params;
    const { limit = 50, before } = request.query as { limit?: number; before?: string };

    const messages = await scopedDb(request).listMessages(id, limit, before);
    const hasMore = messages.length === limit;

    return {
      data: messages,
      limit,
      cursor: messages.length > 0 ? messages[messages.length - 1].id : undefined,
      has_more: hasMore,
    };
  });

  app.get<{ Params: { id: string; msgId: string } }>(
    '/v1/conversations/:id/messages/:msgId',
    async (request, reply) => {
      const { msgId } = request.params;
      const message = await scopedDb(request).getMessage(msgId);

      if (!message) {
        return reply.status(404).send({ error: 'Message not found' });
      }

      return message;
    }
  );

  app.put<{ Params: { id: string; msgId: string }; Body: UpdateMessageInput }>(
    '/v1/conversations/:id/messages/:msgId',
    async (request, reply) => {
      const { msgId } = request.params;
      const { sender_id } = request.query as { sender_id: string };

      if (!sender_id) {
        return reply.status(400).send({ error: 'sender_id query parameter required' });
      }

      // Check edit window
      const existingMessage = await scopedDb(request).getMessage(msgId);
      if (existingMessage && fullConfig.editWindowMinutes > 0) {
        const editDeadline = new Date(existingMessage.created_at.getTime() + fullConfig.editWindowMinutes * 60000);
        if (new Date() > editDeadline) {
          return reply.status(403).send({ error: 'Edit window has expired' });
        }
      }

      const message = await scopedDb(request).updateMessage(msgId, sender_id, request.body);

      if (!message) {
        return reply.status(404).send({ error: 'Message not found or permission denied' });
      }

      return message;
    }
  );

  app.delete<{ Params: { id: string; msgId: string } }>(
    '/v1/conversations/:id/messages/:msgId',
    async (request, reply) => {
      const { msgId } = request.params;
      const { sender_id } = request.query as { sender_id: string };

      if (!sender_id) {
        return reply.status(400).send({ error: 'sender_id query parameter required' });
      }

      const success = await scopedDb(request).deleteMessage(msgId, sender_id);

      if (!success) {
        return reply.status(404).send({ error: 'Message not found or permission denied' });
      }

      return { success: true };
    }
  );

  // =========================================================================
  // Message Reaction Endpoints
  // =========================================================================

  app.post<{ Params: { id: string; msgId: string }; Body: { user_id: string; emoji: string } }>(
    '/v1/conversations/:id/messages/:msgId/reactions',
    async (request, reply) => {
      const { msgId } = request.params;
      const { user_id, emoji } = request.body;

      const message = await scopedDb(request).addReaction(msgId, user_id, emoji);

      if (!message) {
        return reply.status(404).send({ error: 'Message not found' });
      }

      return message;
    }
  );

  app.delete<{ Params: { id: string; msgId: string; emoji: string } }>(
    '/v1/conversations/:id/messages/:msgId/reactions/:emoji',
    async (request, reply) => {
      const { msgId, emoji } = request.params;
      const { user_id } = request.query as { user_id: string };

      if (!user_id) {
        return reply.status(400).send({ error: 'user_id query parameter required' });
      }

      const message = await scopedDb(request).removeReaction(msgId, user_id, decodeURIComponent(emoji));

      if (!message) {
        return reply.status(404).send({ error: 'Message not found' });
      }

      return message;
    }
  );

  // =========================================================================
  // Pin/Unpin Endpoints
  // =========================================================================

  app.post<{ Params: { id: string; msgId: string }; Body: { pinned_by: string } }>(
    '/v1/conversations/:id/messages/:msgId/pin',
    async (request, reply) => {
      const { id, msgId } = request.params;
      const { pinned_by } = request.body;

      // Check max pinned limit
      const pinnedMessages = await scopedDb(request).listPinnedMessages(id);
      if (pinnedMessages.length >= fullConfig.maxPinned) {
        return reply.status(400).send({
          error: `Maximum number of pinned messages (${fullConfig.maxPinned}) reached`,
        });
      }

      const message = await scopedDb(request).pinMessage(msgId, pinned_by);

      if (!message) {
        return reply.status(404).send({ error: 'Message not found or already pinned' });
      }

      return message;
    }
  );

  app.delete<{ Params: { id: string; msgId: string } }>(
    '/v1/conversations/:id/messages/:msgId/pin',
    async (request, reply) => {
      const { msgId } = request.params;
      const message = await scopedDb(request).unpinMessage(msgId);

      if (!message) {
        return reply.status(404).send({ error: 'Message not found or not pinned' });
      }

      return message;
    }
  );

  app.get<{ Params: { id: string } }>('/v1/conversations/:id/pinned', async (request) => {
    const { id } = request.params;
    const messages = await scopedDb(request).listPinnedMessages(id);
    return { data: messages };
  });

  // =========================================================================
  // Thread Endpoints
  // =========================================================================

  app.get<{ Params: { id: string; msgId: string } }>(
    '/v1/conversations/:id/threads/:msgId',
    async (request) => {
      const { msgId } = request.params;
      const { limit = 50 } = request.query as { limit?: number };

      const messages = await scopedDb(request).listThreadMessages(msgId, limit);
      return { data: messages };
    }
  );

  // =========================================================================
  // Read Receipt Endpoints
  // =========================================================================

  app.post<{ Params: { id: string }; Body: UpdateReadReceiptInput & { user_id: string } }>(
    '/v1/conversations/:id/read',
    async (request, reply) => {
      const { id } = request.params;
      const { user_id, ...input } = request.body;

      try {
        const receipt = await scopedDb(request).updateReadReceipt(id, user_id, input);
        return receipt;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update read receipt', { error: message });
        return reply.status(500).send({ error: message });
      }
    }
  );

  app.get<{ Params: { id: string } }>('/v1/conversations/:id/unread', async (request, reply) => {
    const { id } = request.params;
    const { user_id } = request.query as { user_id?: string };

    if (!user_id) {
      return reply.status(400).send({ error: 'user_id query parameter required' });
    }

    const unreadCount = await scopedDb(request).getUnreadCount(id, user_id);
    return { conversation_id: id, user_id, unread_count: unreadCount };
  });

  // =========================================================================
  // Search Endpoint
  // =========================================================================

  app.get<{ Params: { id: string } }>('/v1/conversations/:id/search', async (request) => {
    const { id } = request.params;
    const query = request.query as MessageSearchQuery;

    const messages = await scopedDb(request).searchMessages({
      ...query,
      conversation_id: id,
    });

    return { data: messages };
  });

  // =========================================================================
  // Typing Indicator Endpoint
  // =========================================================================

  app.post<{ Params: { id: string }; Body: { user_id: string } }>(
    '/v1/conversations/:id/typing',
    async (request) => {
      // Fire-and-forget endpoint
      // In production, this would broadcast via WebSocket or realtime plugin
      const { id } = request.params;
      const { user_id } = request.body;

      logger.debug('Typing indicator received', { conversation_id: id, user_id });

      return { received: true };
    }
  );

  // =========================================================================
  // Moderation Endpoint
  // =========================================================================

  app.post<{ Body: CreateModerationActionInput }>('/v1/moderate', async (request, reply) => {
    try {
      const action = await scopedDb(request).createModerationAction(request.body);
      return reply.status(201).send(action);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create moderation action', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Sync Endpoint
  // =========================================================================

  app.get('/v1/sync', async (request) => {
    const { conversation_id, user_id } = request.query as {
      since?: string;
      conversation_id?: string;
      user_id?: string;
    };

    // Get updated conversations
    const conversations = await scopedDb(request).listConversations(user_id);

    // Get updated messages
    const messages = conversation_id
      ? await scopedDb(request).listMessages(conversation_id, 100)
      : [];

    return {
      conversations,
      messages,
      timestamp: new Date().toISOString(),
    };
  });

  // Start server
  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.info(`Chat plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start server', { error: message });
      process.exit(1);
    }
  };

  return {
    app,
    start,
    stop: async () => {
      await app.close();
      await db.disconnect();
    },
  };
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  createServer().then(server => server.start()).catch((error: unknown) => {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  });
}
