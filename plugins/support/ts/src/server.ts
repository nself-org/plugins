/**
 * Support Plugin Server
 * HTTP server for helpdesk, ticketing, SLA, canned responses, knowledge base, analytics
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { SupportDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateTicketRequest, UpdateTicketRequest,
  CreateTicketMessageRequest,
  CreateTeamRequest, UpdateTeamRequest,
  CreateTeamMemberRequest, UpdateTeamMemberRequest,
  CreateSlaPolicyRequest, UpdateSlaPolicyRequest,
  CreateCannedResponseRequest,
  CreateKbArticleRequest, UpdateKbArticleRequest,
  TicketStatus, TicketPriority,
} from './types.js';

const logger = createLogger('support:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);
  const db = new SupportDatabase();
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

  function scopedDb(request: unknown): SupportDatabase {
    return (request as Record<string, unknown>).scopedDb as SupportDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => ({ status: 'ok', plugin: 'support', timestamp: new Date().toISOString() }));

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'support', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({ ready: false, plugin: 'support', error: 'Database unavailable', timestamp: new Date().toISOString() });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'support',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'support',
      version: '1.0.0',
      status: 'running',
      csatEnabled: fullConfig.csatEnabled,
      kbEnabled: fullConfig.kbEnabled,
      autoAssignment: fullConfig.autoAssignment,
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Ticket Management
  // =========================================================================

  app.post<{ Body: CreateTicketRequest }>('/api/support/tickets', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.subject || !body.description) {
        return reply.status(400).send({ error: 'subject and description are required' });
      }
      const ticket = await scopedDb(request).createTicket(body);
      return reply.status(201).send({
        success: true,
        ticket: {
          id: ticket.id,
          ticketNumber: ticket.ticket_number,
          status: ticket.status,
          priority: ticket.priority,
          firstResponseDueAt: ticket.first_response_due_at,
          resolutionDueAt: ticket.resolution_due_at,
        },
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create ticket', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get<{ Params: { ticketId: string } }>('/api/support/tickets/:ticketId', async (request, reply) => {
    const ticket = await scopedDb(request).getTicket(request.params.ticketId);
    if (!ticket) return reply.status(404).send({ error: 'Ticket not found' });
    return { success: true, ticket };
  });

  app.get<{
    Querystring: {
      status?: TicketStatus;
      priority?: TicketPriority;
      assignedTo?: string;
      teamId?: string;
      customerId?: string;
      tags?: string;
      search?: string;
      sort?: string;
      limit?: string;
      offset?: string;
    };
  }>('/api/support/tickets', async (request) => {
    const q = request.query;
    const tickets = await scopedDb(request).listTickets({
      status: q.status,
      priority: q.priority,
      assignedTo: q.assignedTo,
      teamId: q.teamId,
      customerId: q.customerId,
      tags: q.tags ? q.tags.split(',').map(t => t.trim()) : undefined,
      search: q.search,
      sort: q.sort,
      limit: q.limit ? parseInt(q.limit, 10) : undefined,
      offset: q.offset ? parseInt(q.offset, 10) : undefined,
    });
    return { success: true, tickets, count: tickets.length };
  });

  app.patch<{ Params: { ticketId: string }; Body: UpdateTicketRequest & { userId?: string } }>(
    '/api/support/tickets/:ticketId',
    async (request, reply) => {
      try {
        const { userId, ...updates } = request.body;
        const ticket = await scopedDb(request).updateTicket(request.params.ticketId, updates, userId);
        if (!ticket) return reply.status(404).send({ error: 'Ticket not found' });
        return { success: true, ticket };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update ticket', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.post<{ Params: { ticketId: string }; Body: { rating: number; comment?: string } }>(
    '/api/support/tickets/:ticketId/satisfaction',
    async (request, reply) => {
      try {
        const { rating, comment } = request.body;
        if (typeof rating !== 'number' || rating < 1 || rating > 5) {
          return reply.status(400).send({ error: 'rating must be between 1 and 5' });
        }
        const ticket = await scopedDb(request).submitSatisfaction(request.params.ticketId, rating, comment);
        if (!ticket) return reply.status(404).send({ error: 'Ticket not found' });
        return { success: true, ticket };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to submit satisfaction', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Ticket Messages
  // =========================================================================

  app.post<{ Params: { ticketId: string }; Body: Omit<CreateTicketMessageRequest, 'ticketId'> }>(
    '/api/support/tickets/:ticketId/messages',
    async (request, reply) => {
      try {
        const body = { ...request.body, ticketId: request.params.ticketId };
        if (!body.content) {
          return reply.status(400).send({ error: 'content is required' });
        }

        // Verify ticket exists
        const ticket = await scopedDb(request).getTicket(request.params.ticketId);
        if (!ticket) return reply.status(404).send({ error: 'Ticket not found' });

        const message = await scopedDb(request).createTicketMessage(body);
        return reply.status(201).send({ success: true, message });
      } catch (error) {
        const msg = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create ticket message', { error: msg });
        return reply.status(400).send({ error: msg });
      }
    }
  );

  app.get<{ Params: { ticketId: string }; Querystring: { includeInternal?: string } }>(
    '/api/support/tickets/:ticketId/messages',
    async (request, reply) => {
      // Verify ticket exists
      const ticket = await scopedDb(request).getTicket(request.params.ticketId);
      if (!ticket) return reply.status(404).send({ error: 'Ticket not found' });

      const includeInternal = request.query.includeInternal !== 'false';
      const messages = await scopedDb(request).listTicketMessages(request.params.ticketId, includeInternal);
      return { success: true, messages, count: messages.length };
    }
  );

  // =========================================================================
  // Team Management
  // =========================================================================

  app.post<{ Body: CreateTeamRequest }>('/api/support/teams', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.name) {
        return reply.status(400).send({ error: 'name is required' });
      }
      const team = await scopedDb(request).createTeam(body);
      return reply.status(201).send({ success: true, team });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create team', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/api/support/teams', async (request) => {
    const teams = await scopedDb(request).listTeams();
    return { success: true, teams, count: teams.length };
  });

  app.get<{ Params: { teamId: string } }>('/api/support/teams/:teamId', async (request, reply) => {
    const team = await scopedDb(request).getTeam(request.params.teamId);
    if (!team) return reply.status(404).send({ error: 'Team not found' });

    const members = await scopedDb(request).listTeamMembers(request.params.teamId);
    return { success: true, team, members };
  });

  app.patch<{ Params: { teamId: string }; Body: UpdateTeamRequest }>(
    '/api/support/teams/:teamId',
    async (request, reply) => {
      try {
        const team = await scopedDb(request).updateTeam(request.params.teamId, request.body);
        if (!team) return reply.status(404).send({ error: 'Team not found' });
        return { success: true, team };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update team', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Team Members
  // =========================================================================

  app.post<{ Params: { teamId: string }; Body: Omit<CreateTeamMemberRequest, 'teamId'> }>(
    '/api/support/teams/:teamId/members',
    async (request, reply) => {
      try {
        const body = { ...request.body, teamId: request.params.teamId };
        if (!body.userId) {
          return reply.status(400).send({ error: 'userId is required' });
        }

        // Verify team exists
        const team = await scopedDb(request).getTeam(request.params.teamId);
        if (!team) return reply.status(404).send({ error: 'Team not found' });

        const member = await scopedDb(request).addTeamMember(body);
        return reply.status(201).send({ success: true, member });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to add team member', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.get<{ Params: { teamId: string } }>('/api/support/teams/:teamId/members', async (request, reply) => {
    const team = await scopedDb(request).getTeam(request.params.teamId);
    if (!team) return reply.status(404).send({ error: 'Team not found' });

    const members = await scopedDb(request).listTeamMembers(request.params.teamId);
    return { success: true, members, count: members.length };
  });

  app.patch<{ Params: { teamId: string; memberId: string }; Body: UpdateTeamMemberRequest }>(
    '/api/support/teams/:teamId/members/:memberId',
    async (request, reply) => {
      try {
        const member = await scopedDb(request).updateTeamMember(request.params.memberId, request.body);
        if (!member) return reply.status(404).send({ error: 'Team member not found' });
        return { success: true, member };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update team member', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // SLA Policies
  // =========================================================================

  app.post<{ Body: CreateSlaPolicyRequest }>('/api/support/sla-policies', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.name) {
        return reply.status(400).send({ error: 'name is required' });
      }
      const policy = await scopedDb(request).createSlaPolicy(body);
      return reply.status(201).send({ success: true, policy });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create SLA policy', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/api/support/sla-policies', async (request) => {
    const policies = await scopedDb(request).listSlaPolicies();
    return { success: true, policies, count: policies.length };
  });

  app.get<{ Params: { policyId: string } }>('/api/support/sla-policies/:policyId', async (request, reply) => {
    const policy = await scopedDb(request).getSlaPolicyById(request.params.policyId);
    if (!policy) return reply.status(404).send({ error: 'SLA policy not found' });
    return { success: true, policy };
  });

  app.patch<{ Params: { policyId: string }; Body: UpdateSlaPolicyRequest }>(
    '/api/support/sla-policies/:policyId',
    async (request, reply) => {
      try {
        const policy = await scopedDb(request).updateSlaPolicy(request.params.policyId, request.body);
        if (!policy) return reply.status(404).send({ error: 'SLA policy not found' });
        return { success: true, policy };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update SLA policy', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Canned Responses
  // =========================================================================

  app.post<{ Body: CreateCannedResponseRequest }>('/api/support/canned-responses', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.title || !body.content || !body.createdBy) {
        return reply.status(400).send({ error: 'title, content, and createdBy are required' });
      }
      const response = await scopedDb(request).createCannedResponse(body);
      return reply.status(201).send({ success: true, response });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create canned response', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get<{ Querystring: { category?: string; teamId?: string; search?: string } }>(
    '/api/support/canned-responses',
    async (request) => {
      const responses = await scopedDb(request).listCannedResponses({
        category: request.query.category,
        teamId: request.query.teamId,
        search: request.query.search,
      });
      return { success: true, responses, count: responses.length };
    }
  );

  app.get<{ Querystring: { q: string; teamId?: string } }>(
    '/api/support/canned-responses/search',
    async (request, reply) => {
      const { q, teamId } = request.query;
      if (!q) {
        return reply.status(400).send({ error: 'Query parameter "q" is required' });
      }
      const responses = await scopedDb(request).listCannedResponses({ search: q, teamId });
      return { success: true, responses, count: responses.length };
    }
  );

  app.post<{ Params: { responseId: string } }>(
    '/api/support/canned-responses/:responseId/use',
    async (request, reply) => {
      const response = await scopedDb(request).useCannedResponse(request.params.responseId);
      if (!response) return reply.status(404).send({ error: 'Canned response not found' });
      return { success: true, response };
    }
  );

  // =========================================================================
  // Knowledge Base
  // =========================================================================

  app.post<{ Body: CreateKbArticleRequest }>('/api/support/kb/articles', async (request, reply) => {
    try {
      const body = request.body;
      if (!body.title || !body.content || !body.authorId) {
        return reply.status(400).send({ error: 'title, content, and authorId are required' });
      }
      const article = await scopedDb(request).createKbArticle(body);
      return reply.status(201).send({ success: true, article });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create KB article', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get<{
    Querystring: {
      category?: string;
      search?: string;
      published?: string;
      isPublic?: string;
      limit?: string;
      offset?: string;
    };
  }>('/api/support/kb/articles', async (request) => {
    const q = request.query;
    const articles = await scopedDb(request).listKbArticles({
      category: q.category,
      search: q.search,
      published: q.published === 'true' ? true : q.published === 'false' ? false : undefined,
      isPublic: q.isPublic === 'true' ? true : q.isPublic === 'false' ? false : undefined,
      limit: q.limit ? parseInt(q.limit, 10) : undefined,
      offset: q.offset ? parseInt(q.offset, 10) : undefined,
    });
    return { success: true, articles, count: articles.length };
  });

  app.get<{ Params: { articleId: string } }>('/api/support/kb/articles/:articleId', async (request, reply) => {
    const article = await scopedDb(request).getKbArticle(request.params.articleId);
    if (!article) return reply.status(404).send({ error: 'Article not found' });

    // Increment view count
    await scopedDb(request).incrementArticleViewCount(request.params.articleId);

    return { success: true, article };
  });

  app.patch<{ Params: { articleId: string }; Body: UpdateKbArticleRequest }>(
    '/api/support/kb/articles/:articleId',
    async (request, reply) => {
      try {
        const article = await scopedDb(request).updateKbArticle(request.params.articleId, request.body);
        if (!article) return reply.status(404).send({ error: 'Article not found' });
        return { success: true, article };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to update KB article', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  app.post<{ Params: { articleId: string }; Body: { helpful: boolean } }>(
    '/api/support/kb/articles/:articleId/feedback',
    async (request, reply) => {
      try {
        const article = await scopedDb(request).getKbArticle(request.params.articleId);
        if (!article) return reply.status(404).send({ error: 'Article not found' });

        await scopedDb(request).recordArticleFeedback(request.params.articleId, request.body.helpful);
        return { success: true, recorded: true };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to record article feedback', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Analytics
  // =========================================================================

  app.get('/api/support/analytics/overview', async (request) => {
    const metrics = await scopedDb(request).getAnalyticsOverview();
    return { success: true, metrics };
  });

  app.get('/api/support/analytics/agent-performance', async (request) => {
    const agents = await scopedDb(request).getAgentPerformance();
    return { success: true, agents, count: agents.length };
  });

  app.get<{ Querystring: { limit?: string; offset?: string } }>(
    '/api/support/analytics/sla-breaches',
    async (request) => {
      const limit = request.query.limit ? parseInt(request.query.limit, 10) : 50;
      const offset = request.query.offset ? parseInt(request.query.offset, 10) : 0;

      const tickets = await scopedDb(request).listTickets({
        limit,
        offset,
      });

      // Filter to only breached tickets
      const breachedTickets = tickets.filter(
        t => t.first_response_breached || t.resolution_breached
      );

      return {
        success: true,
        breaches: breachedTickets.map(t => ({
          ticketId: t.id,
          ticketNumber: t.ticket_number,
          subject: t.subject,
          priority: t.priority,
          firstResponseBreached: t.first_response_breached,
          resolutionBreached: t.resolution_breached,
          firstResponseDueAt: t.first_response_due_at,
          firstResponseAt: t.first_response_at,
          resolutionDueAt: t.resolution_due_at,
          resolvedAt: t.resolved_at,
          createdAt: t.created_at,
        })),
        count: breachedTickets.length,
      };
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
    logger.info('Support plugin server running', { port: fullConfig.port, host: fullConfig.host });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
