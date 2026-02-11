#!/usr/bin/env node
/**
 * CLI for workflows plugin
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { db } from './database.js';

const logger = createLogger('workflows:cli');

const command = process.argv[2];

async function main() {
  switch (command) {
    case 'init':
      await init();
      break;
    case 'server':
      await startServer();
      break;
    case 'status':
      await showStatus();
      break;
    case 'workflows':
      await listWorkflows();
      break;
    case 'executions':
      await listExecutions();
      break;
    case 'triggers':
      await listTriggers();
      break;
    case 'templates':
      await listTemplatesCmd();
      break;
    case 'variables':
      await listVariables();
      break;
    default:
      showHelp();
      break;
  }
}

async function init() {
  logger.info('Initializing workflows system...');

  try {
    await db.initializeSchema();
    logger.info('Database schema initialized');

    const stats = await db.getStats();
    logger.info('Current state:');
    logger.info(`  Total workflows: ${stats.total_workflows}`);
    logger.info(`  Published: ${stats.published_workflows}`);
    logger.info(`  Executions: ${stats.total_executions}`);
    logger.info(`  Triggers: ${stats.total_triggers}`);
    logger.info(`  Templates: ${stats.total_templates}`);
    logger.info(`  Pending approvals: ${stats.pending_approvals}`);

    logger.info('Configuration:');
    logger.info(`  Port: ${config.server.port}`);
    logger.info(`  Default timeout: ${config.execution.default_timeout_seconds}s`);
    logger.info(`  Max concurrent: ${config.execution.max_concurrent_executions}`);
    logger.info(`  Worker pool: ${config.execution.worker_pool_size}`);
    logger.info(`  Max retries: ${config.retries.max_retries}`);

    logger.info('Initialization complete!');
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Initialization failed', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function startServer() {
  logger.info('Starting workflows server...');
  await import('./server.js');
}

async function showStatus() {
  try {
    const stats = await db.getStats();

    logger.info('Workflows Status:');
    logger.info(`  Total workflows: ${stats.total_workflows}`);
    logger.info(`  Published: ${stats.published_workflows}`);
    logger.info(`  Total executions: ${stats.total_executions}`);
    logger.info(`  Total triggers: ${stats.total_triggers}`);
    logger.info(`  Templates available: ${stats.total_templates}`);
    logger.info(`  Pending approvals: ${stats.pending_approvals}`);
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting status', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listWorkflows() {
  try {
    const result = await db.listWorkflows({ limit: '20' });
    logger.info(`Workflows (${result.total}):`);
    for (const wf of result.workflows) {
      logger.info(`  ${wf.name} [${wf.status}] v${wf.version}`);
      logger.info(`    Enabled: ${wf.is_enabled} | Executions: ${wf.total_executions} (${wf.successful_executions} ok, ${wf.failed_executions} failed)`);
      if (wf.trigger_type) logger.info(`    Trigger: ${wf.trigger_type}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing workflows', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listExecutions() {
  const workflowId = process.argv[3];
  try {
    const result = await db.listExecutions({
      workflow_id: workflowId, limit: '20',
    });
    logger.info(`Executions (${result.total}):`);
    for (const exec of result.executions) {
      logger.info(`  ${exec.id} [${exec.status}] triggered by: ${exec.triggered_by}`);
      if (exec.duration_ms) logger.info(`    Duration: ${exec.duration_ms}ms`);
      if (exec.error_message) logger.info(`    Error: ${exec.error_message}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing executions', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listTriggers() {
  const workflowId = process.argv[3];
  try {
    const triggers = await db.listTriggers(workflowId);
    logger.info(`Triggers (${triggers.length}):`);
    for (const trigger of triggers) {
      logger.info(`  ${trigger.id} [${trigger.type}] active: ${trigger.is_active}`);
      if (trigger.schedule_cron) logger.info(`    Cron: ${trigger.schedule_cron}`);
      if (trigger.webhook_token) logger.info(`    Webhook: /webhooks/${trigger.webhook_token}`);
      if (trigger.event_type) logger.info(`    Event: ${trigger.event_type}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing triggers', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listTemplatesCmd() {
  try {
    const result = await db.listTemplates({});
    logger.info(`Templates (${result.total}):`);
    for (const tmpl of result.templates) {
      logger.info(`  ${tmpl.name} [${tmpl.category ?? 'uncategorized'}]`);
      logger.info(`    Installs: ${tmpl.install_count} | Rating: ${tmpl.rating ?? 'N/A'}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing templates', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listVariables() {
  const workflowId = process.argv[3];
  try {
    const variables = await db.listVariables(workflowId);
    logger.info(`Variables (${variables.length}):`);
    for (const v of variables) {
      const displayValue = v.is_secret ? '***' : JSON.stringify(v.value);
      logger.info(`  ${v.key} (${v.type}) = ${displayValue}`);
      if (v.workflow_id) logger.info(`    Workflow: ${v.workflow_id}`);
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing variables', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

function showHelp() {
  logger.info('nself-workflows - Automation engine CLI');
  logger.info('');
  logger.info('Usage:');
  logger.info('  nself-workflows <command> [options]');
  logger.info('');
  logger.info('Commands:');
  logger.info('  init                     Initialize and verify setup');
  logger.info('  server                   Start the workflows server');
  logger.info('  status                   Show system status');
  logger.info('  workflows                List workflows');
  logger.info('  executions [workflow_id]  List executions');
  logger.info('  triggers [workflow_id]    List triggers');
  logger.info('  templates                List templates');
  logger.info('  variables [workflow_id]   List variables');
  logger.info('');
  logger.info('For full functionality, use the nself plugin commands:');
  logger.info('  nself plugin workflows <action>');
}

main().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
