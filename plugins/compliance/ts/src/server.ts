/**
 * Compliance Plugin Server
 * HTTP server for GDPR/CCPA compliance API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ComplianceDatabase } from './database.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateDsarRequest,
  ProcessDsarRequest,
  CreateConsentRequest,
  CreatePrivacyPolicyRequest,
  CreateRetentionPolicyRequest,
  CreateBreachRequest,
  CreateAuditLogRequest,
  ExportDataRequest,
} from './types.js';

const logger = createLogger('compliance:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new ComplianceDatabase();

  // Connect to database
  await db.connect();
  await db.initializeSchema();

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024, // 10MB
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

  /** Extract scoped ComplianceDatabase from request */
  function scopedDb(request: unknown): ComplianceDatabase {
    return (request as Record<string, unknown>).scopedDb as ComplianceDatabase;
  }

  // =========================================================================
  // Health Checks
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'compliance', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'compliance', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'compliance',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const dsarResult = await scopedDb(request).listDsars({ limit: 0 });
    return {
      alive: true,
      plugin: 'compliance',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      config: {
        gdprEnabled: fullConfig.gdprEnabled,
        ccpaEnabled: fullConfig.ccpaEnabled,
        retentionEnabled: fullConfig.retentionEnabled,
        auditEnabled: fullConfig.auditEnabled,
      },
      stats: {
        totalDsars: dsarResult.total,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/v1/status', async (request) => {
    const dsarResult = await scopedDb(request).listDsars({ limit: 1 });
    const retentionPolicies = await scopedDb(request).listRetentionPolicies();
    const breaches = await scopedDb(request).listBreaches();

    return {
      plugin: 'compliance',
      version: '1.0.0',
      status: 'running',
      config: {
        gdprEnabled: fullConfig.gdprEnabled,
        ccpaEnabled: fullConfig.ccpaEnabled,
        dsarDeadlineDays: fullConfig.dsarDeadlineDays,
        breachNotificationHours: fullConfig.breachNotificationHours,
        retentionEnabled: fullConfig.retentionEnabled,
        auditEnabled: fullConfig.auditEnabled,
      },
      stats: {
        totalDsars: dsarResult.total,
        retentionPolicies: retentionPolicies.length,
        activeBreaches: breaches.filter(b => b.status !== 'resolved').length,
      },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // DSARs
  // =========================================================================

  // POST /api/compliance/dsars - Create a new DSAR
  app.post<{ Body: CreateDsarRequest }>('/api/compliance/dsars', async (request, reply) => {
    try {
      const body = request.body;

      if (!body.request_type) {
        return reply.status(400).send({ error: 'request_type is required' });
      }
      if (!body.email) {
        return reply.status(400).send({ error: 'email is required' });
      }

      const deadlineDays = body.request_type.startsWith('ccpa_')
        ? fullConfig.ccpaDeadlineDays
        : fullConfig.dsarDeadlineDays;

      const dsar = await scopedDb(request).createDsar(body, deadlineDays);

      // Log audit event
      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: 'dsar.created',
          event_category: 'dsar',
          target_type: 'dsar',
          target_id: dsar.id,
          details: { request_type: body.request_type, email: body.email },
        });
      }

      return reply.status(201).send({
        dsar_id: dsar.id,
        request_number: dsar.request_number,
        status: dsar.status,
        deadline: dsar.deadline,
        verification_required: !fullConfig.dsarAutoVerification,
        created_at: dsar.created_at,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create DSAR', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // GET /api/compliance/dsars - List DSARs
  app.get<{ Querystring: { status?: string; user_id?: string; limit?: string; offset?: string } }>(
    '/api/compliance/dsars',
    async (request) => {
      const { status, user_id, limit, offset } = request.query;
      const result = await scopedDb(request).listDsars({
        status,
        user_id,
        limit: limit ? parseInt(limit, 10) : undefined,
        offset: offset ? parseInt(offset, 10) : undefined,
      });

      return {
        dsars: result.dsars,
        total: result.total,
        limit: limit ? parseInt(limit, 10) : 50,
        offset: offset ? parseInt(offset, 10) : 0,
      };
    }
  );

  // GET /api/compliance/dsars/:id - Get a specific DSAR
  app.get<{ Params: { id: string } }>('/api/compliance/dsars/:id', async (request, reply) => {
    const dsar = await scopedDb(request).getDsar(request.params.id);
    if (!dsar) {
      return reply.status(404).send({ error: 'DSAR not found' });
    }

    const activities = await scopedDb(request).getDsarActivities(dsar.id);

    return { ...dsar, activities };
  });

  // POST /api/compliance/dsars/:id/verify - Verify DSAR identity
  app.post<{ Params: { id: string }; Body: { verification_token: string } }>(
    '/api/compliance/dsars/:id/verify',
    async (request, reply) => {
      try {
        const { verification_token } = request.body;
        if (!verification_token) {
          return reply.status(400).send({ error: 'verification_token is required' });
        }

        const verified = await scopedDb(request).verifyDsar(request.params.id, verification_token);

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'dsar.verified',
            event_category: 'dsar',
            target_type: 'dsar',
            target_id: request.params.id,
            details: { verified },
          });
        }

        return { verified };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to verify DSAR', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // POST /api/compliance/dsars/:id/process - Approve or reject a DSAR
  app.post<{ Params: { id: string }; Body: ProcessDsarRequest }>(
    '/api/compliance/dsars/:id/process',
    async (request, reply) => {
      try {
        const body = request.body;
        if (!body.action || !['approve', 'reject'].includes(body.action)) {
          return reply.status(400).send({ error: 'action must be "approve" or "reject"' });
        }

        const dsar = await scopedDb(request).processDsar(request.params.id, body);
        if (!dsar) {
          return reply.status(404).send({ error: 'DSAR not found' });
        }

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: `dsar.${body.action === 'approve' ? 'approved' : 'rejected'}`,
            event_category: 'dsar',
            target_type: 'dsar',
            target_id: dsar.id,
            details: { action: body.action, notes: body.notes },
          });
        }

        return { success: true, dsar };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to process DSAR', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // POST /api/compliance/dsars/:id/complete - Complete a DSAR
  app.post<{ Params: { id: string }; Body: { data_package_url?: string } }>(
    '/api/compliance/dsars/:id/complete',
    async (request, reply) => {
      try {
        const dsar = await scopedDb(request).completeDsar(
          request.params.id,
          request.body.data_package_url
        );

        if (!dsar) {
          return reply.status(404).send({ error: 'DSAR not found' });
        }

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'dsar.completed',
            event_category: 'dsar',
            target_type: 'dsar',
            target_id: dsar.id,
          });
        }

        return { success: true, dsar };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to complete DSAR', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // GET /api/compliance/dsars/:id/activities - Get DSAR activity log
  app.get<{ Params: { id: string } }>('/api/compliance/dsars/:id/activities', async (request, reply) => {
    const dsar = await scopedDb(request).getDsar(request.params.id);
    if (!dsar) {
      return reply.status(404).send({ error: 'DSAR not found' });
    }

    const activities = await scopedDb(request).getDsarActivities(request.params.id);
    return { activities, count: activities.length };
  });

  // =========================================================================
  // Consent Management
  // =========================================================================

  // POST /api/compliance/consent - Create or update consent
  app.post<{ Body: CreateConsentRequest }>('/api/compliance/consent', async (request, reply) => {
    try {
      const body = request.body;

      if (!body.user_id) {
        return reply.status(400).send({ error: 'user_id is required' });
      }
      if (!body.purpose) {
        return reply.status(400).send({ error: 'purpose is required' });
      }
      if (!body.status || !['granted', 'denied'].includes(body.status)) {
        return reply.status(400).send({ error: 'status must be "granted" or "denied"' });
      }

      const consent = await scopedDb(request).createConsent(body);

      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: `consent.${body.status}`,
          event_category: 'consent',
          data_subject_id: body.user_id,
          target_type: 'consent',
          target_id: consent.id,
          details: { purpose: body.purpose, status: body.status },
        });
      }

      return reply.status(201).send({
        consent_id: consent.id,
        user_id: consent.user_id,
        purpose: consent.purpose,
        status: consent.status,
        granted_at: consent.granted_at,
        expires_at: consent.expires_at,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create consent', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // GET /api/compliance/consent - List consents
  app.get<{ Querystring: { user_id?: string; purpose?: string } }>(
    '/api/compliance/consent',
    async (request) => {
      const consents = await scopedDb(request).listConsents({
        user_id: request.query.user_id,
        purpose: request.query.purpose,
      });

      return { consents, count: consents.length };
    }
  );

  // GET /api/compliance/consent/:id - Get specific consent
  app.get<{ Params: { id: string } }>('/api/compliance/consent/:id', async (request, reply) => {
    const consent = await scopedDb(request).getConsent(request.params.id);
    if (!consent) {
      return reply.status(404).send({ error: 'Consent record not found' });
    }
    return consent;
  });

  // POST /api/compliance/consent/:id/withdraw - Withdraw consent
  app.post<{ Params: { id: string }; Body: { reason?: string } }>(
    '/api/compliance/consent/:id/withdraw',
    async (request, reply) => {
      try {
        const consent = await scopedDb(request).withdrawConsent(
          request.params.id,
          request.body.reason
        );

        if (!consent) {
          return reply.status(404).send({ error: 'Consent record not found' });
        }

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'consent.withdrawn',
            event_category: 'consent',
            data_subject_id: consent.user_id,
            target_type: 'consent',
            target_id: consent.id,
            details: { purpose: consent.purpose, reason: request.body.reason },
          });
        }

        return { success: true, consent };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to withdraw consent', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // GET /api/compliance/consent/check - Check if user has valid consent for a purpose
  app.get<{ Querystring: { user_id: string; purpose: string } }>(
    '/api/compliance/consent/check',
    async (request, reply) => {
      const { user_id, purpose } = request.query;

      if (!user_id || !purpose) {
        return reply.status(400).send({ error: 'user_id and purpose are required' });
      }

      const hasConsent = await scopedDb(request).checkUserConsent(user_id, purpose);

      return { user_id, purpose, has_consent: hasConsent };
    }
  );

  // =========================================================================
  // Privacy Policies
  // =========================================================================

  // GET /api/compliance/privacy-policy - Get active privacy policy
  app.get<{ Querystring: { version?: string } }>(
    '/api/compliance/privacy-policy',
    async (request, reply) => {
      const policy = await scopedDb(request).getPrivacyPolicy(request.query.version);
      if (!policy) {
        return reply.status(404).send({ error: 'No active privacy policy found' });
      }
      return policy;
    }
  );

  // GET /api/compliance/privacy-policies - List all privacy policies
  app.get('/api/compliance/privacy-policies', async (request) => {
    const result = await scopedDb(request).query<Record<string, unknown>>(
      `SELECT id, version, version_number, title, summary, is_active, effective_from, language, created_at
       FROM compliance_privacy_policies
       WHERE source_account_id = $1
       ORDER BY version_number DESC`,
      [scopedDb(request).getCurrentSourceAccountId()]
    );

    return { policies: result.rows, count: result.rows.length };
  });

  // POST /api/compliance/privacy-policies - Create a new privacy policy version
  app.post<{ Body: CreatePrivacyPolicyRequest }>('/api/compliance/privacy-policies', async (request, reply) => {
    try {
      const body = request.body;

      if (!body.version || !body.title || !body.content || !body.effective_from) {
        return reply.status(400).send({
          error: 'version, title, content, and effective_from are required',
        });
      }

      const policy = await scopedDb(request).createPrivacyPolicy(body);

      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: 'policy.created',
          event_category: 'policy',
          target_type: 'privacy_policy',
          target_id: policy.id,
          details: { version: body.version },
        });
      }

      return reply.status(201).send(policy);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create privacy policy', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // POST /api/compliance/privacy-policies/:id/publish - Publish a privacy policy
  app.post<{ Params: { id: string } }>('/api/compliance/privacy-policies/:id/publish', async (request, reply) => {
    try {
      const policy = await scopedDb(request).publishPrivacyPolicy(request.params.id);
      if (!policy) {
        return reply.status(404).send({ error: 'Privacy policy not found' });
      }

      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: 'policy.published',
          event_category: 'policy',
          target_type: 'privacy_policy',
          target_id: policy.id,
          details: { version: policy.version },
        });
      }

      return { success: true, policy };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to publish privacy policy', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // POST /api/compliance/privacy-policy/accept - Accept a privacy policy
  app.post<{ Body: { user_id: string; policy_id: string } }>(
    '/api/compliance/privacy-policy/accept',
    async (request, reply) => {
      try {
        const { user_id, policy_id } = request.body;

        if (!user_id || !policy_id) {
          return reply.status(400).send({ error: 'user_id and policy_id are required' });
        }

        const acceptance = await scopedDb(request).acceptPolicy(user_id, policy_id);

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'policy.accepted',
            event_category: 'policy',
            data_subject_id: user_id,
            target_type: 'privacy_policy',
            target_id: policy_id,
          });
        }

        return reply.status(201).send({
          acceptance_id: acceptance.id,
          user_id: acceptance.user_id,
          policy_id: acceptance.policy_id,
          accepted_at: acceptance.accepted_at,
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to accept privacy policy', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Data Retention
  // =========================================================================

  // GET /api/compliance/retention/policies - List retention policies
  app.get<{ Querystring: { enabled_only?: string } }>(
    '/api/compliance/retention/policies',
    async (request) => {
      const enabledOnly = request.query.enabled_only === 'true';
      const policies = await scopedDb(request).listRetentionPolicies(enabledOnly);
      return { policies, count: policies.length };
    }
  );

  // POST /api/compliance/retention/policies - Create a retention policy
  app.post<{ Body: CreateRetentionPolicyRequest }>(
    '/api/compliance/retention/policies',
    async (request, reply) => {
      try {
        const body = request.body;

        if (!body.name || !body.data_category || body.retention_days === undefined || !body.retention_action) {
          return reply.status(400).send({
            error: 'name, data_category, retention_days, and retention_action are required',
          });
        }

        const policy = await scopedDb(request).createRetentionPolicy(body);

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'retention.policy_created',
            event_category: 'retention',
            target_type: 'retention_policy',
            target_id: policy.id,
            details: { name: body.name, data_category: body.data_category, retention_days: body.retention_days },
          });
        }

        return reply.status(201).send(policy);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to create retention policy', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // POST /api/compliance/retention/execute - Execute a retention policy
  app.post<{ Body: { policy_id: string } }>(
    '/api/compliance/retention/execute',
    async (request, reply) => {
      try {
        const { policy_id } = request.body;

        if (!policy_id) {
          return reply.status(400).send({ error: 'policy_id is required' });
        }

        const execution = await scopedDb(request).executeRetentionPolicy(policy_id);

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'retention.executed',
            event_category: 'retention',
            target_type: 'retention_policy',
            target_id: policy_id,
            details: {
              execution_id: execution.id,
              records_processed: execution.records_processed,
              records_deleted: execution.records_deleted,
            },
          });
        }

        return {
          execution_id: execution.id,
          policy_id: execution.policy_id,
          status: execution.status,
          records_processed: execution.records_processed,
          records_deleted: execution.records_deleted,
          records_anonymized: execution.records_anonymized,
          records_archived: execution.records_archived,
          execution_time_ms: execution.execution_time_ms,
        };
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to execute retention policy', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // GET /api/compliance/retention/executions/:policyId - Get execution history for a policy
  app.get<{ Params: { policyId: string }; Querystring: { limit?: string } }>(
    '/api/compliance/retention/executions/:policyId',
    async (request) => {
      const limit = request.query.limit ? parseInt(request.query.limit, 10) : 20;
      const executions = await scopedDb(request).getRetentionExecutions(request.params.policyId, limit);
      return { executions, count: executions.length };
    }
  );

  // =========================================================================
  // Data Processors
  // =========================================================================

  // GET /api/compliance/processors - List data processors
  app.get<{ Querystring: { active_only?: string } }>(
    '/api/compliance/processors',
    async (request) => {
      const activeOnly = request.query.active_only !== 'false';
      const processors = await scopedDb(request).listDataProcessors(activeOnly);
      return { processors, count: processors.length };
    }
  );

  // =========================================================================
  // Data Breaches
  // =========================================================================

  // POST /api/compliance/breaches - Report a new data breach
  app.post<{ Body: CreateBreachRequest }>('/api/compliance/breaches', async (request, reply) => {
    try {
      const body = request.body;

      if (!body.title || !body.description || !body.severity || !body.data_categories) {
        return reply.status(400).send({
          error: 'title, description, severity, and data_categories are required',
        });
      }

      const breach = await scopedDb(request).createBreach(body, fullConfig.breachNotificationHours);

      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: 'breach.created',
          event_category: 'breach',
          target_type: 'data_breach',
          target_id: breach.id,
          details: {
            breach_number: breach.breach_number,
            severity: body.severity,
            notification_deadline: breach.notification_deadline,
          },
        });
      }

      return reply.status(201).send({
        breach_id: breach.id,
        breach_number: breach.breach_number,
        severity: breach.severity,
        status: breach.status,
        notification_required: breach.notification_required,
        notification_deadline: breach.notification_deadline,
        created_at: breach.created_at,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create breach', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // GET /api/compliance/breaches - List data breaches
  app.get<{ Querystring: { status?: string; severity?: string } }>(
    '/api/compliance/breaches',
    async (request) => {
      const breaches = await scopedDb(request).listBreaches({
        status: request.query.status,
        severity: request.query.severity,
      });
      return { breaches, count: breaches.length };
    }
  );

  // GET /api/compliance/breaches/:id - Get breach details
  app.get<{ Params: { id: string } }>('/api/compliance/breaches/:id', async (request, reply) => {
    const breach = await scopedDb(request).getBreach(request.params.id);
    if (!breach) {
      return reply.status(404).send({ error: 'Breach not found' });
    }

    // Get associated notifications
    const notifResult = await scopedDb(request).query<Record<string, unknown>>(
      `SELECT * FROM compliance_breach_notifications
       WHERE source_account_id = $1 AND breach_id = $2
       ORDER BY created_at DESC`,
      [scopedDb(request).getCurrentSourceAccountId(), breach.id]
    );

    return { ...breach, notifications: notifResult.rows };
  });

  // POST /api/compliance/breaches/:id/notify - Send breach notification
  app.post<{
    Params: { id: string };
    Body: { notification_type: string; recipient_type: string; recipient_email?: string; subject?: string; message_body?: string };
  }>(
    '/api/compliance/breaches/:id/notify',
    async (request, reply) => {
      try {
        const { notification_type, recipient_type, recipient_email, subject, message_body } = request.body;

        if (!notification_type || !recipient_type) {
          return reply.status(400).send({ error: 'notification_type and recipient_type are required' });
        }

        const breach = await scopedDb(request).getBreach(request.params.id);
        if (!breach) {
          return reply.status(404).send({ error: 'Breach not found' });
        }

        const notification = await scopedDb(request).addBreachNotification(
          request.params.id,
          notification_type,
          recipient_type,
          recipient_email,
          subject,
          message_body
        );

        if (fullConfig.auditEnabled) {
          await scopedDb(request).createAuditLog({
            event_type: 'breach.notified',
            event_category: 'breach',
            target_type: 'data_breach',
            target_id: request.params.id,
            details: {
              notification_type,
              recipient_type,
              recipient_email,
            },
          });
        }

        return reply.status(201).send(notification);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send breach notification', { error: message });
        return reply.status(400).send({ error: message });
      }
    }
  );

  // =========================================================================
  // Data Export
  // =========================================================================

  // POST /api/compliance/export - Export user data
  app.post<{ Body: ExportDataRequest }>('/api/compliance/export', async (request, reply) => {
    try {
      const { user_id, data_categories } = request.body;

      if (!user_id) {
        return reply.status(400).send({ error: 'user_id is required' });
      }

      const exportData = await scopedDb(request).exportUserData(user_id, data_categories);

      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: 'data.exported',
          event_category: 'dsar',
          data_subject_id: user_id,
          accessed_data_categories: data_categories,
          details: { format: request.body.format ?? fullConfig.exportFormat },
        });
      }

      // In a full implementation, the data would be packaged and stored in object storage
      // Here we return the data directly as JSON
      const expiresAt = new Date(Date.now() + fullConfig.exportExpiryHours * 60 * 60 * 1000);

      return {
        user_id,
        data: exportData,
        format: request.body.format ?? fullConfig.exportFormat,
        exported_at: new Date().toISOString(),
        expires_at: expiresAt.toISOString(),
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to export user data', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Audit Log
  // =========================================================================

  // POST /api/compliance/audit - Create audit log entry
  app.post<{ Body: CreateAuditLogRequest }>('/api/compliance/audit', async (request, reply) => {
    try {
      const body = request.body;

      if (!body.event_type || !body.event_category) {
        return reply.status(400).send({ error: 'event_type and event_category are required' });
      }

      const log = await scopedDb(request).createAuditLog(body);
      return reply.status(201).send(log);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create audit log', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // GET /api/compliance/audit - List audit logs
  app.get<{
    Querystring: {
      event_category?: string;
      actor_id?: string;
      data_subject_id?: string;
      limit?: string;
      offset?: string;
    };
  }>(
    '/api/compliance/audit',
    async (request) => {
      const { event_category, actor_id, data_subject_id, limit, offset } = request.query;

      const result = await scopedDb(request).listAuditLogs({
        event_category,
        actor_id,
        data_subject_id,
        limit: limit ? parseInt(limit, 10) : undefined,
        offset: offset ? parseInt(offset, 10) : undefined,
      });

      return {
        logs: result.logs,
        total: result.total,
        limit: limit ? parseInt(limit, 10) : 50,
        offset: offset ? parseInt(offset, 10) : 0,
      };
    }
  );

  // =========================================================================
  // Processing Records (GDPR Article 30)
  // =========================================================================

  // GET /api/compliance/processing-records - List processing records
  app.get<{ Querystring: { active_only?: string } }>(
    '/api/compliance/processing-records',
    async (request) => {
      const activeOnly = request.query.active_only !== 'false';
      const condition = activeOnly ? 'AND is_active = true' : '';

      const result = await scopedDb(request).query<Record<string, unknown>>(
        `SELECT * FROM compliance_processing_records
         WHERE source_account_id = $1 ${condition}
         ORDER BY activity_name`,
        [scopedDb(request).getCurrentSourceAccountId()]
      );

      return { records: result.rows, count: result.rows.length };
    }
  );

  // POST /api/compliance/processing-records - Create a processing record
  app.post<{
    Body: {
      activity_name: string;
      activity_description?: string;
      processing_purpose: string;
      legal_basis: string;
      data_categories: string[];
      data_subjects?: string[];
      recipient_categories?: string[];
      third_party_transfers?: boolean;
      third_party_countries?: string[];
      safeguards?: string;
      retention_period?: string;
      security_measures?: string;
    };
  }>('/api/compliance/processing-records', async (request, reply) => {
    try {
      const body = request.body;

      if (!body.activity_name || !body.processing_purpose || !body.legal_basis || !body.data_categories) {
        return reply.status(400).send({
          error: 'activity_name, processing_purpose, legal_basis, and data_categories are required',
        });
      }

      const result = await scopedDb(request).query<Record<string, unknown>>(
        `INSERT INTO compliance_processing_records (
          source_account_id, activity_name, activity_description,
          processing_purpose, legal_basis, data_categories,
          data_subjects, recipient_categories,
          third_party_transfers, third_party_countries,
          safeguards, retention_period, security_measures
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        RETURNING *`,
        [
          scopedDb(request).getCurrentSourceAccountId(),
          body.activity_name, body.activity_description ?? null,
          body.processing_purpose, body.legal_basis, body.data_categories,
          body.data_subjects ?? [], body.recipient_categories ?? [],
          body.third_party_transfers ?? false, body.third_party_countries ?? [],
          body.safeguards ?? null, body.retention_period ?? null,
          body.security_measures ?? null,
        ]
      );

      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: 'processing_record.created',
          event_category: 'processing',
          target_type: 'processing_record',
          target_id: (result.rows[0] as Record<string, unknown>).id as string,
          details: { activity_name: body.activity_name },
        });
      }

      return reply.status(201).send(result.rows[0]);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create processing record', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  // =========================================================================
  // Webhook Endpoint
  // =========================================================================

  app.post('/webhook', async (request, reply) => {
    try {
      const payload = request.body as Record<string, unknown>;
      const eventType = payload.type as string;

      if (!eventType) {
        return reply.status(400).send({ error: 'Missing event type' });
      }

      // Log the webhook as an audit event
      if (fullConfig.auditEnabled) {
        await scopedDb(request).createAuditLog({
          event_type: `webhook.${eventType}`,
          event_category: 'webhook',
          actor_type: 'system',
          details: payload,
        });
      }

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
    await app.listen({
      port: fullConfig.port,
      host: fullConfig.host,
    });

    logger.info(`Compliance plugin server running`, {
      port: fullConfig.port,
      host: fullConfig.host,
      gdprEnabled: fullConfig.gdprEnabled,
      ccpaEnabled: fullConfig.ccpaEnabled,
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start server', { error: message });
    process.exit(1);
  }
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
