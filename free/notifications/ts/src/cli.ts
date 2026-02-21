#!/usr/bin/env node
/**
 * CLI for notifications plugin
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { db } from './database.js';

const logger = createLogger('notifications:cli');

const command = process.argv[2];
const args = process.argv.slice(3);

async function main() {
  switch (command) {
    case 'init':
      await init();
      break;
    case 'templates':
      await listTemplates();
      break;
    case 'status':
      await showStatus();
      break;
    default:
      showHelp();
      break;
  }
}

async function init() {
  logger.info('Initializing notifications system...');

  try {
    // Test database connection
    const templates = await db.listTemplates();
    logger.info(`Database connected`);
    logger.info(`${templates.length} templates available`);

    // Show configuration
    logger.info('Configuration:');
    logger.info(`  Email: ${config.email.enabled ? 'enabled' : 'disabled'} (${config.email.provider || 'none'})`);
    logger.info(`  Push: ${config.push.enabled ? 'enabled' : 'disabled'} (${config.push.provider || 'none'})`);
    logger.info(`  SMS: ${config.sms.enabled ? 'enabled' : 'disabled'} (${config.sms.provider || 'none'})`);
    logger.info(`  Queue: ${config.queue.backend}`);

    logger.info('Initialization complete!');
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Initialization failed', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function listTemplates() {
  try {
    const templates = await db.listTemplates();

    logger.info(`Templates (${templates.length}):`);

    templates.forEach((template) => {
      logger.info(`  ${template.name}`);
      logger.info(`    Category: ${template.category}`);
      logger.info(`    Channels: ${template.channels.join(', ')}`);
      logger.info(`    Active: ${template.active ? 'yes' : 'no'}`);
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error listing templates', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

async function showStatus() {
  try {
    const stats = await db.getDeliveryStats(1);

    logger.info('Status:');

    if (stats.length > 0) {
      stats.forEach((stat) => {
        logger.info(`  ${stat.channel}: ${stat.total} sent, ${stat.delivered} delivered (${stat.delivery_rate}%)`);
      });
    } else {
      logger.info('  No notifications sent yet');
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error';
    logger.error('Error getting status', { error: message });
    process.exit(1);
  } finally {
    await db.close();
  }
}

function showHelp() {
  logger.info('nself-notifications - Notification system CLI');
  logger.info('');
  logger.info('Usage:');
  logger.info('  nself-notifications <command> [options]');
  logger.info('');
  logger.info('Commands:');
  logger.info('  init        Initialize and verify setup');
  logger.info('  templates   List available templates');
  logger.info('  status      Show system status');
  logger.info('');
  logger.info('For full functionality, use the nself plugin commands:');
  logger.info('  nself plugin notifications <action>');
}

// Run
main().catch((error) => {
  const message = error instanceof Error ? error.message : 'Unknown error';
  logger.error('Fatal error', { error: message });
  process.exit(1);
});
