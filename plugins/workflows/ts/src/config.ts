/**
 * Configuration loader for Workflows plugin
 */

import * as dotenv from 'dotenv';
import { WorkflowsConfig } from './types.js';

// Load environment variables
dotenv.config();

export function loadConfig(): WorkflowsConfig {
  return {
    database: {
      host: process.env.POSTGRES_HOST ?? 'postgres',
      port: parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
      database: process.env.POSTGRES_DB ?? 'nself',
      user: process.env.POSTGRES_USER ?? 'postgres',
      password: process.env.POSTGRES_PASSWORD ?? '',
      ssl: process.env.POSTGRES_SSL === 'true',
    },

    server: {
      port: parseInt(process.env.WORKFLOWS_PLUGIN_PORT ?? process.env.PORT ?? '3712', 10),
      host: process.env.HOST ?? '0.0.0.0',
    },

    execution: {
      default_timeout_seconds: parseInt(process.env.WORKFLOWS_DEFAULT_TIMEOUT ?? '300', 10),
      max_concurrent_executions: parseInt(process.env.WORKFLOWS_MAX_CONCURRENT ?? '10', 10),
      history_retention_days: parseInt(process.env.WORKFLOWS_HISTORY_RETENTION ?? '90', 10),
      worker_pool_size: parseInt(process.env.WORKFLOWS_WORKER_POOL_SIZE ?? '20', 10),
    },

    triggers: {
      schedule_check_interval: parseInt(process.env.WORKFLOWS_SCHEDULE_CHECK ?? '60', 10),
      max_webhooks_per_workflow: parseInt(process.env.WORKFLOWS_MAX_WEBHOOKS ?? '5', 10),
    },

    retries: {
      max_retries: parseInt(process.env.WORKFLOWS_MAX_RETRIES ?? '3', 10),
      initial_delay_seconds: parseInt(process.env.WORKFLOWS_RETRY_DELAY ?? '60', 10),
      backoff_multiplier: parseFloat(process.env.WORKFLOWS_BACKOFF_MULTIPLIER ?? '2.0'),
    },
  };
}

export const config = loadConfig();
