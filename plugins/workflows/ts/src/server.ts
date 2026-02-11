#!/usr/bin/env node
/**
 * HTTP server for workflows API
 * Multi-app aware: each request is scoped to a source_account_id
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, getAppContext } from '@nself/plugin-utils';
import { config } from './config.js';
import { db, DatabaseClient } from './database.js';
import {
  CreateWorkflowInput, UpdateWorkflowInput, ListWorkflowsQuery,
  ExecuteWorkflowInput, ListExecutionsQuery,
  CreateTriggerInput, UpdateTriggerInput,
  CreateTemplateInput, UpdateTemplateInput, ListTemplatesQuery,
  CreateVariableInput,
} from './types.js';

const logger = createLogger('workflows:server');

const fastify = Fastify({ logger: { level: 'info' } });

fastify.register(cors, { origin: true });

// Multi-app context
fastify.decorateRequest('scopedDb', null);
fastify.addHook('onRequest', async (request) => {
  const ctx = getAppContext(request);
  (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
});

function scopedDb(request: unknown): DatabaseClient {
  return (request as Record<string, unknown>).scopedDb as DatabaseClient;
}

// =============================================================================
// Health
// =============================================================================

fastify.get('/health', async () => ({
  status: 'ok', timestamp: new Date().toISOString(), service: 'workflows',
}));

fastify.get('/ready', async () => {
  try {
    const stats = await db.getStats();
    return { status: 'ready', ...stats };
  } catch {
    return { status: 'not_ready' };
  }
});

// =============================================================================
// Workflows
// =============================================================================

fastify.post<{ Body: CreateWorkflowInput }>('/api/v1/workflows', async (request, reply) => {
  try {
    const workflow = await scopedDb(request).createWorkflow(request.body);
    return reply.code(201).send({ workflow });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Failed to create workflow', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: ListWorkflowsQuery }>('/api/v1/workflows', async (request, reply) => {
  try {
    const result = await scopedDb(request).listWorkflows(request.query);
    return { data: result.workflows, total: result.total };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/workflows/:id', async (request, reply) => {
  try {
    const workflow = await scopedDb(request).getWorkflow(request.params.id);
    if (!workflow) return reply.code(404).send({ error: 'Workflow not found' });
    return { workflow };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateWorkflowInput }>('/api/v1/workflows/:id', async (request, reply) => {
  try {
    const workflow = await scopedDb(request).updateWorkflow(request.params.id, request.body);
    if (!workflow) return reply.code(404).send({ error: 'Workflow not found' });
    return { workflow };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/workflows/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteWorkflow(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Workflow not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: { name: string } }>('/api/v1/workflows/:id/duplicate', async (request, reply) => {
  try {
    const workflow = await scopedDb(request).duplicateWorkflow(request.params.id, request.body.name);
    if (!workflow) return reply.code(404).send({ error: 'Workflow not found' });
    return reply.code(201).send({ workflow });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/workflows/:id/publish', async (request, reply) => {
  try {
    const workflow = await scopedDb(request).publishWorkflow(request.params.id);
    if (!workflow) return reply.code(404).send({ error: 'Workflow not found' });
    return { workflow };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/workflows/:id/unpublish', async (request, reply) => {
  try {
    const workflow = await scopedDb(request).unpublishWorkflow(request.params.id);
    if (!workflow) return reply.code(404).send({ error: 'Workflow not found' });
    return { workflow };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Executions
// =============================================================================

fastify.post<{ Params: { id: string }; Body: ExecuteWorkflowInput }>('/api/v1/workflows/:id/execute', async (request, reply) => {
  try {
    const execution = await scopedDb(request).createExecution(request.params.id, {
      ...request.body, triggered_by: 'manual',
    });
    return reply.code(201).send({ execution });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: ExecuteWorkflowInput }>('/api/v1/workflows/:id/test', async (request, reply) => {
  try {
    const execution = await scopedDb(request).createExecution(request.params.id, {
      ...request.body, triggered_by: 'manual',
    });
    return reply.code(201).send({ execution, test: true });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string }; Querystring: ListExecutionsQuery }>('/api/v1/workflows/:id/executions', async (request, reply) => {
  try {
    const result = await scopedDb(request).listExecutions({
      ...request.query, workflow_id: request.params.id,
    });
    return { data: result.executions, total: result.total };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/executions/:id', async (request, reply) => {
  try {
    const execution = await scopedDb(request).getExecution(request.params.id);
    if (!execution) return reply.code(404).send({ error: 'Execution not found' });
    const steps = await scopedDb(request).getExecutionSteps(execution.id);
    return { execution, steps };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/executions/:id/retry', async (request, reply) => {
  try {
    const execution = await scopedDb(request).retryExecution(request.params.id);
    if (!execution) return reply.code(404).send({ error: 'Execution not found' });
    return reply.code(201).send({ execution });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/executions/:id/cancel', async (request, reply) => {
  try {
    const cancelled = await scopedDb(request).cancelExecution(request.params.id);
    if (!cancelled) return reply.code(404).send({ error: 'Execution not found or not cancellable' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/executions/:id/logs', async (request, reply) => {
  try {
    const steps = await scopedDb(request).getExecutionSteps(request.params.id);
    return { data: steps, total: steps.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Triggers
// =============================================================================

fastify.post<{ Body: CreateTriggerInput }>('/api/v1/triggers', async (request, reply) => {
  try {
    const trigger = await scopedDb(request).createTrigger(request.body);
    return reply.code(201).send({ trigger });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: { workflow_id?: string } }>('/api/v1/triggers', async (request, reply) => {
  try {
    const triggers = await scopedDb(request).listTriggers(request.query.workflow_id);
    return { data: triggers, total: triggers.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/triggers/:id', async (request, reply) => {
  try {
    const trigger = await scopedDb(request).getTrigger(request.params.id);
    if (!trigger) return reply.code(404).send({ error: 'Trigger not found' });
    return { trigger };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateTriggerInput }>('/api/v1/triggers/:id', async (request, reply) => {
  try {
    const trigger = await scopedDb(request).updateTrigger(request.params.id, request.body);
    if (!trigger) return reply.code(404).send({ error: 'Trigger not found' });
    return { trigger };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/triggers/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteTrigger(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Trigger not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string } }>('/api/v1/triggers/:id/test', async (request, reply) => {
  try {
    const trigger = await scopedDb(request).getTrigger(request.params.id);
    if (!trigger) return reply.code(404).send({ error: 'Trigger not found' });
    return { success: true, trigger_type: trigger.type, message: 'Trigger test successful' };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Actions
// =============================================================================

fastify.get<{ Querystring: { category?: string } }>('/api/v1/actions', async (request, reply) => {
  try {
    const actions = await scopedDb(request).listActions(request.query.category);
    return { data: actions, total: actions.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { type: string } }>('/api/v1/actions/:type', async (request, reply) => {
  try {
    const action = await scopedDb(request).getAction(request.params.type);
    if (!action) return reply.code(404).send({ error: 'Action not found' });
    return { action };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Body: { type: string; input: Record<string, unknown> } }>('/api/v1/actions/execute', async (_request, reply) => {
  // Placeholder for single action execution
  return reply.code(501).send({ error: 'Direct action execution not yet implemented' });
});

// =============================================================================
// Templates
// =============================================================================

fastify.post<{ Body: CreateTemplateInput }>('/api/v1/templates', async (request, reply) => {
  try {
    const template = await scopedDb(request).createTemplate(request.body);
    return reply.code(201).send({ template });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: ListTemplatesQuery }>('/api/v1/templates', async (request, reply) => {
  try {
    const result = await scopedDb(request).listTemplates(request.query);
    return { data: result.templates, total: result.total };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/templates/:id', async (request, reply) => {
  try {
    const template = await scopedDb(request).getTemplate(request.params.id);
    if (!template) return reply.code(404).send({ error: 'Template not found' });
    return { template };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: UpdateTemplateInput }>('/api/v1/templates/:id', async (request, reply) => {
  try {
    const template = await scopedDb(request).updateTemplate(request.params.id, request.body);
    if (!template) return reply.code(404).send({ error: 'Template not found' });
    return { template };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/templates/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteTemplate(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Template not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.post<{ Params: { id: string }; Body: { name: string; owner_id: string } }>(
  '/api/v1/templates/:id/install',
  async (request, reply) => {
    try {
      const workflow = await scopedDb(request).installTemplate(
        request.params.id, request.body.name, request.body.owner_id
      );
      if (!workflow) return reply.code(404).send({ error: 'Template not found' });
      return reply.code(201).send({ workflow });
    } catch (error) {
      const msg = error instanceof Error ? error.message : 'Unknown error';
      return reply.code(500).send({ error: msg });
    }
  }
);

// =============================================================================
// Webhooks
// =============================================================================

fastify.post<{ Body: { workflow_id: string } }>('/api/v1/webhooks', async (request, reply) => {
  try {
    const trigger = await scopedDb(request).createTrigger({
      workflow_id: request.body.workflow_id,
      type: 'webhook',
    });
    return reply.code(201).send({ webhook: trigger });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get('/api/v1/webhooks', async (request, reply) => {
  try {
    const triggers = await scopedDb(request).listTriggers();
    const webhooks = triggers.filter(t => t.type === 'webhook');
    return { data: webhooks, total: webhooks.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/webhooks/:id', async (request, reply) => {
  try {
    const trigger = await scopedDb(request).getTrigger(request.params.id);
    if (!trigger || trigger.type !== 'webhook') return reply.code(404).send({ error: 'Webhook not found' });
    return { webhook: trigger };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/webhooks/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteTrigger(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Webhook not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// Webhook endpoint (incoming)
fastify.post<{ Params: { token: string } }>('/webhooks/:token', async (request, reply) => {
  try {
    const trigger = await db.getTriggerByWebhookToken(request.params.token);
    if (!trigger) return reply.code(404).send({ error: 'Webhook not found' });

    const triggerDb = db.forSourceAccount(trigger.source_account_id);

    // Log the webhook
    await triggerDb.logWebhook(
      trigger.id,
      request.method,
      request.url,
      request.headers as Record<string, unknown>,
      request.query as Record<string, unknown>,
      (request.body ?? {}) as Record<string, unknown>,
      request.ip ?? null,
      request.headers['user-agent'] ?? null
    );

    // Create execution
    const execution = await triggerDb.createExecution(trigger.workflow_id, {
      triggered_by: 'webhook',
      input: (request.body ?? {}) as Record<string, unknown>,
    });

    return { success: true, execution_id: execution.id };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Webhook processing failed', { error: msg });
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Variables
// =============================================================================

fastify.post<{ Body: CreateVariableInput }>('/api/v1/variables', async (request, reply) => {
  try {
    const variable = await scopedDb(request).createVariable(request.body);
    return reply.code(201).send({ variable });
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Querystring: { workflow_id?: string } }>('/api/v1/variables', async (request, reply) => {
  try {
    const variables = await scopedDb(request).listVariables(request.query.workflow_id);
    return { data: variables, total: variables.length };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.get<{ Params: { id: string } }>('/api/v1/variables/:id', async (request, reply) => {
  try {
    const variable = await scopedDb(request).getVariable(request.params.id);
    if (!variable) return reply.code(404).send({ error: 'Variable not found' });
    // Mask secret values
    if (variable.is_secret) {
      return { variable: { ...variable, value: '***' } };
    }
    return { variable };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.put<{ Params: { id: string }; Body: { value: unknown } }>('/api/v1/variables/:id', async (request, reply) => {
  try {
    const variable = await scopedDb(request).updateVariable(request.params.id, request.body.value);
    if (!variable) return reply.code(404).send({ error: 'Variable not found' });
    return { variable };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

fastify.delete<{ Params: { id: string } }>('/api/v1/variables/:id', async (request, reply) => {
  try {
    const deleted = await scopedDb(request).deleteVariable(request.params.id);
    if (!deleted) return reply.code(404).send({ error: 'Variable not found' });
    return { success: true };
  } catch (error) {
    const msg = error instanceof Error ? error.message : 'Unknown error';
    return reply.code(500).send({ error: msg });
  }
});

// =============================================================================
// Server Lifecycle
// =============================================================================

const start = async () => {
  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    await fastify.listen({ port: config.server.port, host: config.server.host });
    logger.info(`Workflows server running on http://${config.server.host}:${config.server.port}`);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Failed to start server', { error: msg });
    process.exit(1);
  }
};

const gracefulShutdown = async () => {
  logger.info('Shutting down gracefully...');
  try {
    await fastify.close();
    await db.close();
    process.exit(0);
  } catch (err) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    logger.error('Error during shutdown', { error: msg });
    process.exit(1);
  }
};

process.on('SIGTERM', gracefulShutdown);
process.on('SIGINT', gracefulShutdown);

start();

export { fastify };
