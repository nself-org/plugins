#!/usr/bin/env node
/**
 * Audit Plugin HTTP Server
 * REST API endpoints for audit logging
 */

import Fastify from 'fastify';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { AuditDatabase } from './database.js';
import { AuditService } from './services.js';
import {
  LogEventRequest,
  LogEventResponse,
  QueryEventsRequest,
  QueryEventsResponse,
  ExportEventsRequest,
  ExportEventsResponse,
  CreateRetentionPolicyRequest,
  UpdateRetentionPolicyRequest,
  ExecuteRetentionResponse,
  CreateAlertRuleRequest,
  UpdateAlertRuleRequest,
  GenerateComplianceReportRequest,
  ComplianceReportResponse,
  VerifyEventRequest,
  VerifyEventResponse,
  HealthCheckResponse,
  ReadyCheckResponse,
  LiveCheckResponse,
  AuditEventInfo,
  RetentionPolicyInfo,
  AlertRuleInfo,
} from './types.js';
import fs from 'fs';
import path from 'path';

const logger = createLogger('audit:server');

const fastify = Fastify({
  logger: false,
  bodyLimit: 10485760, // 10MB
});

let auditDb: AuditDatabase;
let auditService: AuditService;

/**
 * Fallback logging to file if database fails
 */
async function fallbackLog(event: LogEventRequest): Promise<void> {
  try {
    const logDir = path.dirname(config.fallback.logPath);
    await fs.promises.mkdir(logDir, { recursive: true });

    const logEntry = JSON.stringify({
      ...event,
      timestamp: new Date().toISOString(),
      fallback: true,
    });

    await fs.promises.appendFile(config.fallback.logPath, logEntry + '\n');
    logger.warn('Event logged to fallback file', { path: config.fallback.logPath });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Fallback logging also failed', { error: message });
  }
}

/**
 * Map database record to API response
 */
function mapEventToInfo(event: any): AuditEventInfo {
  return {
    id: event.id,
    sourcePlugin: event.source_plugin,
    eventType: event.event_type,
    actorId: event.actor_id,
    actorType: event.actor_type,
    resourceType: event.resource_type,
    resourceId: event.resource_id,
    action: event.action,
    outcome: event.outcome,
    severity: event.severity,
    ipAddress: event.ip_address,
    userAgent: event.user_agent,
    location: event.location,
    details: event.details || {},
    metadata: event.metadata || {},
    checksum: event.checksum,
    createdAt: event.created_at.toISOString(),
  };
}

function mapRetentionPolicyToInfo(policy: any): RetentionPolicyInfo {
  return {
    id: policy.id,
    name: policy.name,
    description: policy.description,
    eventTypePattern: policy.event_type_pattern,
    retentionDays: policy.retention_days,
    enabled: policy.enabled,
    lastExecutedAt: policy.last_executed_at?.toISOString() || null,
    createdAt: policy.created_at.toISOString(),
    updatedAt: policy.updated_at.toISOString(),
  };
}

function mapAlertRuleToInfo(rule: any): AlertRuleInfo {
  return {
    id: rule.id,
    name: rule.name,
    description: rule.description,
    eventTypePattern: rule.event_type_pattern,
    severityThreshold: rule.severity_threshold,
    conditions: rule.conditions || {},
    webhookUrl: rule.webhook_url,
    enabled: rule.enabled,
    lastTriggeredAt: rule.last_triggered_at?.toISOString() || null,
    triggerCount: rule.trigger_count,
    createdAt: rule.created_at.toISOString(),
    updatedAt: rule.updated_at.toISOString(),
  };
}

// ============================================================================
// Health Check Endpoints
// ============================================================================

fastify.get('/health', async (): Promise<HealthCheckResponse> => {
  return {
    status: 'ok',
    plugin: 'audit',
    timestamp: new Date().toISOString(),
    version: '1.0.0',
  };
});

fastify.get('/ready', async (): Promise<ReadyCheckResponse> => {
  let databaseStatus: 'ok' | 'error' = 'ok';
  let triggersStatus: 'ok' | 'missing' = 'ok';

  try {
    // Check database connection
    await auditDb.getStats();

    // Check immutability triggers
    const triggersValid = await auditDb.verifyImmutabilityTriggers();
    if (!triggersValid) {
      triggersStatus = 'missing';
    }
  } catch (error) {
    databaseStatus = 'error';
  }

  return {
    ready: databaseStatus === 'ok' && triggersStatus === 'ok',
    database: databaseStatus,
    immutabilityTriggers: triggersStatus,
    timestamp: new Date().toISOString(),
  };
});

fastify.get('/live', async (): Promise<LiveCheckResponse> => {
  const stats = await auditDb.getStats();

  return {
    alive: true,
    uptime: process.uptime(),
    memory: {
      used: process.memoryUsage().heapUsed,
      total: process.memoryUsage().heapTotal,
    },
    stats,
  };
});

// ============================================================================
// Event Logging Endpoints
// ============================================================================

fastify.post<{ Body: LogEventRequest }>('/v1/events', async (request, reply): Promise<LogEventResponse> => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  try {
    const event = await scopedDb.insertEvent(request.body);

    // Check alert rules
    await checkAlertRules(scopedDb, event);

    // Send to SIEM if configured
    if (Object.keys(config.siem).length > 0) {
      auditService.sendToSiem([event]).catch((error) => {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send event to SIEM', { error: message });
      });
    }

    return {
      eventId: event.id,
      checksum: event.checksum,
      createdAt: event.created_at.toISOString(),
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to insert audit event', { error: message });

    // Fallback to file logging
    await fallbackLog(request.body);

    reply.code(500);
    throw new Error('Failed to log audit event. Event saved to fallback log.');
  }
});

async function checkAlertRules(db: AuditDatabase, event: any): Promise<void> {
  const rules = await db.getAlertRules();

  for (const rule of rules) {
    if (!rule.enabled) continue;

    // Check if event matches rule pattern
    const pattern = rule.event_type_pattern.replace(/\*/g, '.*');
    const regex = new RegExp(`^${pattern}$`);

    if (!regex.test(event.event_type)) continue;

    // Check severity threshold
    const severityLevels = ['low', 'medium', 'high', 'critical'];
    const eventSeverityIndex = severityLevels.indexOf(event.severity);
    const thresholdIndex = severityLevels.indexOf(rule.severity_threshold);

    if (eventSeverityIndex < thresholdIndex) continue;

    // Alert triggered
    await db.incrementAlertTriggerCount(rule.id);

    // Send webhook if configured
    if (rule.webhook_url || config.alerts.webhookUrl) {
      const webhookUrl = rule.webhook_url || config.alerts.webhookUrl!;
      const payload = {
        rule: {
          id: rule.id,
          name: rule.name,
          description: rule.description,
        },
        event: {
          id: event.id,
          eventType: event.event_type,
          severity: event.severity,
          outcome: event.outcome,
          actorId: event.actor_id,
          resourceType: event.resource_type,
          resourceId: event.resource_id,
          action: event.action,
          createdAt: event.created_at.toISOString(),
        },
        triggeredAt: new Date().toISOString(),
      };

      fetch(webhookUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      }).catch((error) => {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to send alert webhook', { error: message });
      });

      await db.insertWebhookEvent('audit.alert.triggered', payload);
    }
  }
}

// ============================================================================
// Query Endpoints
// ============================================================================

fastify.get<{ Querystring: QueryEventsRequest }>('/v1/events', async (request): Promise<QueryEventsResponse> => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const { events, total } = await scopedDb.queryEvents(request.query);

  return {
    events: events.map(mapEventToInfo),
    total,
    limit: request.query.limit || 100,
    offset: request.query.offset || 0,
  };
});

fastify.get<{ Params: { id: string } }>('/v1/events/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const event = await scopedDb.getEventById(request.params.id);

  if (!event) {
    reply.code(404);
    throw new Error('Event not found');
  }

  return mapEventToInfo(event);
});

// ============================================================================
// Export Endpoints
// ============================================================================

fastify.post<{ Body: ExportEventsRequest }>('/v1/export', async (request, reply): Promise<ExportEventsResponse> => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);
  const service = new AuditService(scopedDb);

  const { events } = await scopedDb.queryEvents({
    sourcePlugin: request.body.sourcePlugin,
    eventType: request.body.eventType,
    startDate: request.body.startDate,
    endDate: request.body.endDate,
    limit: Math.min(request.body.limit || 10000, config.export.maxRows),
  });

  const data = await service.exportEvents(events, request.body.format);

  // Set appropriate content type
  const contentTypes: Record<string, string> = {
    csv: 'text/csv',
    json: 'application/json',
    jsonl: 'application/x-ndjson',
    cef: 'text/plain',
    leef: 'text/plain',
    syslog: 'text/plain',
  };

  reply.header('Content-Type', contentTypes[request.body.format] || 'text/plain');

  return {
    format: request.body.format,
    data,
    rowCount: events.length,
    exportedAt: new Date().toISOString(),
  };
});

// ============================================================================
// Retention Policy Endpoints
// ============================================================================

fastify.post<{ Body: CreateRetentionPolicyRequest }>('/v1/retention', async (request) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const policy = await scopedDb.createRetentionPolicy({
    name: request.body.name,
    description: request.body.description || null,
    event_type_pattern: request.body.eventTypePattern,
    retention_days: request.body.retentionDays,
    enabled: request.body.enabled ?? true,
  });

  return mapRetentionPolicyToInfo(policy);
});

fastify.get('/v1/retention', async (request) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const policies = await scopedDb.getRetentionPolicies();
  return { policies: policies.map(mapRetentionPolicyToInfo) };
});

fastify.get<{ Params: { id: string } }>('/v1/retention/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const policy = await scopedDb.getRetentionPolicyById(request.params.id);

  if (!policy) {
    reply.code(404);
    throw new Error('Retention policy not found');
  }

  return mapRetentionPolicyToInfo(policy);
});

fastify.patch<{ Params: { id: string }; Body: UpdateRetentionPolicyRequest }>('/v1/retention/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  try {
    const policy = await scopedDb.updateRetentionPolicy(request.params.id, {
      name: request.body.name,
      description: request.body.description,
      event_type_pattern: request.body.eventTypePattern,
      retention_days: request.body.retentionDays,
      enabled: request.body.enabled,
    });

    return mapRetentionPolicyToInfo(policy);
  } catch (error) {
    reply.code(404);
    throw new Error('Retention policy not found');
  }
});

fastify.delete<{ Params: { id: string } }>('/v1/retention/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  await scopedDb.deleteRetentionPolicy(request.params.id);
  reply.code(204);
});

fastify.post('/v1/retention/execute', async (request): Promise<ExecuteRetentionResponse> => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const result = await scopedDb.executeRetentionPolicies();

  return {
    policiesExecuted: result.policiesExecuted,
    eventsDeleted: result.eventsDeleted,
    executedAt: new Date().toISOString(),
  };
});

// ============================================================================
// Alert Rule Endpoints
// ============================================================================

fastify.post<{ Body: CreateAlertRuleRequest }>('/v1/alerts', async (request) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const rule = await scopedDb.createAlertRule({
    name: request.body.name,
    description: request.body.description || null,
    event_type_pattern: request.body.eventTypePattern,
    severity_threshold: request.body.severityThreshold,
    conditions: request.body.conditions || {},
    webhook_url: request.body.webhookUrl || null,
    enabled: request.body.enabled ?? true,
  });

  return mapAlertRuleToInfo(rule);
});

fastify.get('/v1/alerts', async (request) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const rules = await scopedDb.getAlertRules();
  return { rules: rules.map(mapAlertRuleToInfo) };
});

fastify.get<{ Params: { id: string } }>('/v1/alerts/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  const rule = await scopedDb.getAlertRuleById(request.params.id);

  if (!rule) {
    reply.code(404);
    throw new Error('Alert rule not found');
  }

  return mapAlertRuleToInfo(rule);
});

fastify.patch<{ Params: { id: string }; Body: UpdateAlertRuleRequest }>('/v1/alerts/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  try {
    const rule = await scopedDb.updateAlertRule(request.params.id, {
      name: request.body.name,
      description: request.body.description,
      event_type_pattern: request.body.eventTypePattern,
      severity_threshold: request.body.severityThreshold,
      conditions: request.body.conditions,
      webhook_url: request.body.webhookUrl,
      enabled: request.body.enabled,
    });

    return mapAlertRuleToInfo(rule);
  } catch (error) {
    reply.code(404);
    throw new Error('Alert rule not found');
  }
});

fastify.delete<{ Params: { id: string } }>('/v1/alerts/:id', async (request, reply) => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  await scopedDb.deleteAlertRule(request.params.id);
  reply.code(204);
});

// ============================================================================
// Compliance Report Endpoints
// ============================================================================

fastify.post<{ Body: GenerateComplianceReportRequest }>('/v1/compliance/reports', async (request): Promise<ComplianceReportResponse> => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);
  const service = new AuditService(scopedDb);

  const startDate = request.body.startDate ? new Date(request.body.startDate) : new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);
  const endDate = request.body.endDate ? new Date(request.body.endDate) : new Date();

  return await service.generateComplianceReport(request.body.framework, startDate, endDate);
});

// ============================================================================
// Verification Endpoints
// ============================================================================

fastify.post<{ Body: VerifyEventRequest }>('/v1/verify', async (request, reply): Promise<VerifyEventResponse> => {
  const appId = (request.headers['x-app-id'] as string) || 'primary';
  const scopedDb = auditDb.forApp(appId);

  try {
    const result = await scopedDb.verifyEventChecksum(request.body.eventId);

    return {
      eventId: request.body.eventId,
      valid: result.valid,
      expectedChecksum: result.expectedChecksum,
      actualChecksum: result.actualChecksum,
      message: result.valid ? 'Event integrity verified' : 'Event integrity check failed - possible tampering',
    };
  } catch (error) {
    reply.code(404);
    throw new Error('Event not found');
  }
});

// ============================================================================
// Server Startup
// ============================================================================

async function start() {
  try {
    // Initialize database
    const db = createDatabase(config.database);
    await db.connect();
    auditDb = new AuditDatabase(db);
    auditService = new AuditService(auditDb);

    logger.info('Audit database connection established');

    // Start server
    await fastify.listen({ port: config.port, host: config.host });
    logger.success(`Audit plugin server listening on ${config.host}:${config.port}`);
    logger.info(`Health check: http://${config.host}:${config.port}/health`);
    logger.info(`Ready check: http://${config.host}:${config.port}/ready`);
    logger.info(`Live check: http://${config.host}:${config.port}/live`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to start audit server', { error: message });
    process.exit(1);
  }
}

// Handle graceful shutdown
process.on('SIGINT', async () => {
  logger.info('Received SIGINT, shutting down gracefully...');
  await fastify.close();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  logger.info('Received SIGTERM, shutting down gracefully...');
  await fastify.close();
  process.exit(0);
});

// Start server if run directly (check if this is the main module)
// For ES modules, check if argv[1] ends with our filename
const isMainModule = process.argv[1] && (
  process.argv[1].endsWith('server.ts') ||
  process.argv[1].endsWith('server.js')
);

if (isMainModule) {
  start();
}

export { fastify };
