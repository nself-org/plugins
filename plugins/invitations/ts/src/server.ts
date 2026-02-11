/**
 * Invitations Plugin Server
 * HTTP server for invitation management API
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import crypto from 'crypto';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { InvitationsDatabase } from './database.js';
import { loadConfig, generateInviteUrl, type Config } from './config.js';
import type {
  CreateInvitationRequest,
  CreateInvitationResponse,
  ValidateInvitationResponse,
  AcceptInvitationRequest,
  AcceptInvitationResponse,
  CreateBulkSendRequest,
  CreateBulkSendResponse,
  BulkSendStatusResponse,
  CreateTemplateRequest,
  UpdateTemplateRequest,
  InvitationType,
  InvitationStatus,
  InvitationChannel,
} from './types.js';

const logger = createLogger('invitations:server');

function generateInvitationCode(length: number): string {
  return crypto.randomBytes(length).toString('base64url').substring(0, length);
}

function calculateExpiryDate(hoursFromNow: number): Date {
  const expiry = new Date();
  expiry.setHours(expiry.getHours() + hoursFromNow);
  return expiry;
}


export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize database
  const db = new InvitationsDatabase();
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 100,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context
  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): InvitationsDatabase {
    return (request as Record<string, unknown>).scopedDb as InvitationsDatabase;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'invitations', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'invitations', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'invitations',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      alive: true,
      plugin: 'invitations',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats: {
        total: stats.total,
        accepted: stats.accepted,
        conversionRate: stats.conversionRate,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    return {
      plugin: 'invitations',
      version: '1.0.0',
      status: 'running',
      stats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Invitation Endpoints
  // =========================================================================

  app.post<{ Body: CreateInvitationRequest }>('/v1/invitations', async (request, reply) => {
    const body = request.body;

    // Validation
    if (!body.inviter_id) {
      return reply.status(400).send({ error: 'inviter_id is required' });
    }

    if (!body.invitee_email && !body.invitee_phone && body.channel !== 'link') {
      return reply.status(400).send({ error: 'invitee_email or invitee_phone required unless channel is link' });
    }

    try {
      const sdb = scopedDb(request);
      const code = generateInvitationCode(fullConfig.codeLength);
      const expiresAt = body.expires_in_hours
        ? calculateExpiryDate(body.expires_in_hours)
        : calculateExpiryDate(fullConfig.defaultExpiryHours);

      const id = await sdb.createInvitation({
        type: body.type,
        inviter_id: body.inviter_id,
        invitee_email: body.invitee_email ?? null,
        invitee_phone: body.invitee_phone ?? null,
        invitee_name: body.invitee_name ?? null,
        code,
        status: body.send_immediately ? 'sent' : 'pending',
        channel: body.channel ?? 'email',
        message: body.message ?? null,
        role: body.role ?? null,
        resource_type: body.resource_type ?? null,
        resource_id: body.resource_id ?? null,
        expires_at: expiresAt,
        sent_at: body.send_immediately ? new Date() : null,
        delivered_at: null,
        accepted_at: null,
        accepted_by: null,
        declined_at: null,
        revoked_at: null,
        metadata: body.metadata ?? {},
      });

      const inviteUrl = generateInviteUrl(code, fullConfig.acceptUrlTemplate);

      const response: CreateInvitationResponse = {
        id,
        code,
        invite_url: inviteUrl,
        status: body.send_immediately ? 'sent' : 'pending',
        expires_at: expiresAt,
        created_at: new Date(),
      };

      return reply.status(201).send(response);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create invitation', { error: message });
      return reply.status(500).send({ error: 'Failed to create invitation' });
    }
  });

  app.get('/v1/invitations', async (request) => {
    const { limit = 100, offset = 0, type, status, inviter_id } = request.query as {
      limit?: number;
      offset?: number;
      type?: InvitationType;
      status?: InvitationStatus;
      inviter_id?: string;
    };

    const sdb = scopedDb(request);
    const invitations = await sdb.listInvitations(Number(limit), Number(offset), { type, status, inviter_id });
    const total = await sdb.countInvitations({ type, status, inviter_id });

    return { data: invitations, total, limit: Number(limit), offset: Number(offset) };
  });

  app.get<{ Params: { id: string } }>('/v1/invitations/:id', async (request, reply) => {
    const { id } = request.params;

    const invitation = await scopedDb(request).getInvitation(id);
    if (!invitation) {
      return reply.status(404).send({ error: 'Invitation not found' });
    }

    return invitation;
  });

  app.delete<{ Params: { id: string } }>('/v1/invitations/:id', async (request, reply) => {
    const { id } = request.params;

    const sdb = scopedDb(request);
    const invitation = await sdb.getInvitation(id);
    if (!invitation) {
      return reply.status(404).send({ error: 'Invitation not found' });
    }

    await sdb.updateInvitationStatus(id, 'revoked', { revoked_at: new Date() });
    return { success: true, message: 'Invitation revoked' };
  });

  app.post<{ Params: { id: string } }>('/v1/invitations/:id/resend', async (request, reply) => {
    const { id } = request.params;

    const sdb = scopedDb(request);
    const invitation = await sdb.getInvitation(id);
    if (!invitation) {
      return reply.status(404).send({ error: 'Invitation not found' });
    }

    if (invitation.status === 'accepted' || invitation.status === 'revoked') {
      return reply.status(400).send({ error: 'Cannot resend accepted or revoked invitation' });
    }

    await sdb.updateInvitationStatus(id, 'sent', { sent_at: new Date() });
    return { success: true, message: 'Invitation resent' };
  });

  // =========================================================================
  // Validation & Acceptance Endpoints
  // =========================================================================

  app.get<{ Params: { code: string } }>('/v1/validate/:code', async (request, reply) => {
    const { code } = request.params;

    const sdb = scopedDb(request);
    const invitation = await sdb.getInvitationByCode(code);

    if (!invitation) {
      const response: ValidateInvitationResponse = {
        valid: false,
        error: 'Invalid invitation code',
      };
      return reply.status(404).send(response);
    }

    // Check if expired
    if (invitation.expires_at && invitation.expires_at < new Date()) {
      if (invitation.status !== 'expired') {
        await sdb.updateInvitationStatus(invitation.id, 'expired');
      }
      const response: ValidateInvitationResponse = {
        valid: false,
        error: 'Invitation has expired',
      };
      return reply.status(410).send(response);
    }

    // Check status
    if (invitation.status === 'accepted') {
      const response: ValidateInvitationResponse = {
        valid: false,
        error: 'Invitation already accepted',
      };
      return reply.status(410).send(response);
    }

    if (invitation.status === 'declined') {
      const response: ValidateInvitationResponse = {
        valid: false,
        error: 'Invitation was declined',
      };
      return reply.status(410).send(response);
    }

    if (invitation.status === 'revoked') {
      const response: ValidateInvitationResponse = {
        valid: false,
        error: 'Invitation was revoked',
      };
      return reply.status(410).send(response);
    }

    const response: ValidateInvitationResponse = {
      valid: true,
      invitation: {
        id: invitation.id,
        type: invitation.type,
        inviter_id: invitation.inviter_id,
        invitee_name: invitation.invitee_name,
        role: invitation.role,
        resource_type: invitation.resource_type,
        resource_id: invitation.resource_id,
        expires_at: invitation.expires_at,
        message: invitation.message,
      },
    };

    return response;
  });

  app.post<{ Params: { code: string }; Body: AcceptInvitationRequest }>('/v1/accept/:code', async (request, reply) => {
    const { code } = request.params;
    const { accepted_by, metadata } = request.body;

    if (!accepted_by) {
      return reply.status(400).send({ error: 'accepted_by is required' });
    }

    const sdb = scopedDb(request);
    const invitation = await sdb.getInvitationByCode(code);

    if (!invitation) {
      return reply.status(404).send({ error: 'Invalid invitation code' });
    }

    // Validate invitation status
    if (invitation.expires_at && invitation.expires_at < new Date()) {
      if (invitation.status !== 'expired') {
        await sdb.updateInvitationStatus(invitation.id, 'expired');
      }
      return reply.status(410).send({ error: 'Invitation has expired' });
    }

    if (invitation.status === 'accepted') {
      return reply.status(410).send({ error: 'Invitation already accepted' });
    }

    if (invitation.status === 'revoked') {
      return reply.status(410).send({ error: 'Invitation was revoked' });
    }

    // Accept invitation
    await sdb.updateInvitationStatus(invitation.id, 'accepted', {
      accepted_at: new Date(),
      accepted_by,
    });

    // Update metadata if provided
    if (metadata) {
      const updatedMetadata = { ...invitation.metadata, ...metadata };
      await sdb.execute(
        'UPDATE inv_invitations SET metadata = $1 WHERE id = $2',
        [JSON.stringify(updatedMetadata), invitation.id]
      );
    }

    const response: AcceptInvitationResponse = {
      id: invitation.id,
      type: invitation.type,
      inviter_id: invitation.inviter_id,
      role: invitation.role,
      resource_type: invitation.resource_type,
      resource_id: invitation.resource_id,
      accepted_at: new Date(),
      metadata: metadata ? { ...invitation.metadata, ...metadata } : invitation.metadata,
    };

    return response;
  });

  app.post<{ Params: { code: string } }>('/v1/decline/:code', async (request, reply) => {
    const { code } = request.params;

    const sdb = scopedDb(request);
    const invitation = await sdb.getInvitationByCode(code);

    if (!invitation) {
      return reply.status(404).send({ error: 'Invalid invitation code' });
    }

    if (invitation.status === 'accepted') {
      return reply.status(400).send({ error: 'Cannot decline accepted invitation' });
    }

    if (invitation.status === 'declined') {
      return reply.status(400).send({ error: 'Invitation already declined' });
    }

    await sdb.updateInvitationStatus(invitation.id, 'declined', { declined_at: new Date() });

    return { success: true, message: 'Invitation declined' };
  });

  // =========================================================================
  // Bulk Send Endpoints
  // =========================================================================

  app.post<{ Body: CreateBulkSendRequest }>('/v1/bulk', async (request, reply) => {
    const body = request.body;

    if (!body.inviter_id) {
      return reply.status(400).send({ error: 'inviter_id is required' });
    }

    if (!body.invitees || body.invitees.length === 0) {
      return reply.status(400).send({ error: 'invitees array is required and cannot be empty' });
    }

    if (body.invitees.length > fullConfig.maxBulkSize) {
      return reply.status(400).send({ error: `Maximum bulk size is ${fullConfig.maxBulkSize}` });
    }

    try {
      const sdb = scopedDb(request);
      const id = await sdb.createBulkSend({
        inviter_id: body.inviter_id,
        template_id: body.template_id ?? null,
        type: body.type,
        total_count: body.invitees.length,
        sent_count: 0,
        failed_count: 0,
        status: 'pending',
        invitees: body.invitees,
        metadata: body.metadata ?? {},
        started_at: null,
        completed_at: null,
      });

      // Process bulk send asynchronously
      setImmediate(async () => {
        await processBulkSend(sdb, id, body, fullConfig);
      });

      const response: CreateBulkSendResponse = {
        id,
        total_count: body.invitees.length,
        status: 'pending',
        created_at: new Date(),
      };

      return reply.status(201).send(response);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create bulk send', { error: message });
      return reply.status(500).send({ error: 'Failed to create bulk send' });
    }
  });

  app.get<{ Params: { id: string } }>('/v1/bulk/:id', async (request, reply) => {
    const { id } = request.params;

    const bulkSend = await scopedDb(request).getBulkSend(id);
    if (!bulkSend) {
      return reply.status(404).send({ error: 'Bulk send not found' });
    }

    const response: BulkSendStatusResponse = {
      id: bulkSend.id,
      status: bulkSend.status,
      total_count: bulkSend.total_count,
      sent_count: bulkSend.sent_count,
      failed_count: bulkSend.failed_count,
      started_at: bulkSend.started_at,
      completed_at: bulkSend.completed_at,
      created_at: bulkSend.created_at,
    };

    return response;
  });

  // =========================================================================
  // Template Endpoints
  // =========================================================================

  app.post<{ Body: CreateTemplateRequest }>('/v1/templates', async (request, reply) => {
    const body = request.body;

    if (!body.name || !body.body) {
      return reply.status(400).send({ error: 'name and body are required' });
    }

    try {
      const sdb = scopedDb(request);

      // Check for duplicate name
      const existing = await sdb.getTemplateByName(body.name);
      if (existing) {
        return reply.status(409).send({ error: 'Template with this name already exists' });
      }

      const id = await sdb.createTemplate({
        name: body.name,
        type: body.type,
        channel: body.channel,
        subject: body.subject ?? null,
        body: body.body,
        variables: body.variables ?? [],
        enabled: body.enabled ?? true,
      });

      const template = await sdb.getTemplate(id);
      return reply.status(201).send(template);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create template', { error: message });
      return reply.status(500).send({ error: 'Failed to create template' });
    }
  });

  app.get('/v1/templates', async (request) => {
    const { limit = 100, offset = 0, type, channel, enabled } = request.query as {
      limit?: number;
      offset?: number;
      type?: InvitationType;
      channel?: InvitationChannel;
      enabled?: boolean;
    };

    const templates = await scopedDb(request).listTemplates(Number(limit), Number(offset), {
      type,
      channel,
      enabled: enabled !== undefined ? (typeof enabled === 'boolean' ? enabled : enabled === 'true') : undefined,
    });

    return { data: templates, total: templates.length, limit: Number(limit), offset: Number(offset) };
  });

  app.put<{ Params: { id: string }; Body: UpdateTemplateRequest }>('/v1/templates/:id', async (request, reply) => {
    const { id } = request.params;
    const body = request.body;

    const sdb = scopedDb(request);
    const template = await sdb.getTemplate(id);
    if (!template) {
      return reply.status(404).send({ error: 'Template not found' });
    }

    await sdb.updateTemplate(id, body);
    const updated = await sdb.getTemplate(id);

    return updated;
  });

  app.delete<{ Params: { id: string } }>('/v1/templates/:id', async (request, reply) => {
    const { id } = request.params;

    const sdb = scopedDb(request);
    const template = await sdb.getTemplate(id);
    if (!template) {
      return reply.status(404).send({ error: 'Template not found' });
    }

    await sdb.deleteTemplate(id);
    return { success: true, message: 'Template deleted' };
  });

  // =========================================================================
  // Statistics Endpoint
  // =========================================================================

  app.get('/v1/stats', async (request) => {
    const stats = await scopedDb(request).getStats();
    return stats;
  });

  // =========================================================================
  // Server Lifecycle
  // =========================================================================

  const start = async () => {
    try {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Server listening on ${fullConfig.host}:${fullConfig.port}`);

      // Start cleanup job for expired invitations
      setInterval(async () => {
        try {
          const expired = await db.markExpiredInvitations();
          if (expired > 0) {
            logger.info(`Marked ${expired} invitations as expired`);
          }
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          logger.error('Failed to mark expired invitations', { error: message });
        }
      }, 60000); // Every minute
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed to start', { error: message });
      process.exit(1);
    }
  };

  return { ...app, start };
}

// =========================================================================
// Helper Functions
// =========================================================================

async function processBulkSend(
  db: InvitationsDatabase,
  bulkSendId: string,
  request: CreateBulkSendRequest,
  config: Config
): Promise<void> {
  try {
    await db.updateBulkSend(bulkSendId, {
      status: 'processing',
      started_at: new Date(),
    });

    let sentCount = 0;
    let failedCount = 0;

    for (const invitee of request.invitees) {
      try {
        const code = generateInvitationCode(config.codeLength);
        const expiresAt = request.expires_in_hours
          ? calculateExpiryDate(request.expires_in_hours)
          : calculateExpiryDate(config.defaultExpiryHours);

        await db.createInvitation({
          type: request.type,
          inviter_id: request.inviter_id,
          invitee_email: invitee.email ?? null,
          invitee_phone: invitee.phone ?? null,
          invitee_name: invitee.name ?? null,
          code,
          status: 'sent',
          channel: 'email',
          message: null,
          role: request.role ?? null,
          resource_type: request.resource_type ?? null,
          resource_id: request.resource_id ?? null,
          expires_at: expiresAt,
          sent_at: new Date(),
          delivered_at: null,
          accepted_at: null,
          accepted_by: null,
          declined_at: null,
          revoked_at: null,
          metadata: invitee.metadata ?? {},
        });

        sentCount++;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create invitation in bulk send', { error: message, invitee });
        failedCount++;
      }
    }

    await db.updateBulkSend(bulkSendId, {
      sent_count: sentCount,
      failed_count: failedCount,
      status: failedCount === request.invitees.length ? 'failed' : 'completed',
      completed_at: new Date(),
    });

    logger.info(`Bulk send completed`, { bulkSendId, sentCount, failedCount });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Bulk send processing failed', { error: message, bulkSendId });

    await db.updateBulkSend(bulkSendId, {
      status: 'failed',
      completed_at: new Date(),
    });
  }
}
