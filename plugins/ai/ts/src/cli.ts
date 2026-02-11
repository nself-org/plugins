#!/usr/bin/env node
/**
 * AI Plugin CLI
 * Command-line interface for the AI gateway plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { AiDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('ai:cli');

const program = new Command();

program
  .name('nself-ai')
  .description('AI gateway plugin for nself - multi-provider LLM, embeddings, and semantic search')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize AI plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing AI schema...');
      const db = new AiDatabase();
      await db.connect();
      await db.initializeSchema();
      console.log('Done - AI schema initialized successfully');
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start AI plugin server')
  .option('-p, --port <port>', 'Server port', '3705')
  .action(async (options) => {
    try {
      await startServer({ port: parseInt(options.port, 10) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show AI plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new AiDatabase();
      await db.connect();

      const models = await db.listModels();
      const enabledModels = models.filter(m => m.is_enabled);
      const features = await db.listFeatures();

      console.log('\nAI Plugin Status');
      console.log('=================');
      console.log(`Port:           ${config.port}`);
      console.log(`Default:        ${config.defaultProvider}`);
      console.log(`Models:         ${models.length} total, ${enabledModels.length} enabled`);
      console.log(`Features:       ${features.length} enabled`);
      console.log(`Embeddings:     ${config.embeddingsEnabled ? 'enabled' : 'disabled'}`);
      console.log(`Streaming:      ${config.enableStreaming ? 'enabled' : 'disabled'}`);

      console.log('\nProviders:');
      console.log(`  OpenAI:       ${config.openaiEnabled ? 'enabled' : 'disabled'}`);
      console.log(`  Anthropic:    ${config.anthropicEnabled ? 'enabled' : 'disabled'}`);
      console.log(`  Google:       ${config.googleEnabled ? 'enabled' : 'disabled'}`);
      console.log(`  Local:        ${config.localEnabled ? 'enabled' : 'disabled'}`);

      if (enabledModels.length > 0) {
        console.log('\nEnabled Models:');
        for (const model of enabledModels) {
          const defaultTag = model.is_default ? ' [default]' : '';
          console.log(`  - ${model.model_name} (${model.provider})${defaultTag}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Models list command
program
  .command('models')
  .description('List AI models')
  .option('--enabled', 'Show only enabled models')
  .action(async (options) => {
    try {
      const db = new AiDatabase();
      await db.connect();

      const models = await db.listModels(options.enabled);

      console.log(`\nAI Models (${models.length}):`);
      console.log('==================');
      for (const model of models) {
        const status = model.is_enabled ? 'enabled' : 'disabled';
        const defaultTag = model.is_default ? ' [default]' : '';
        console.log(`  ${model.model_name} (${model.provider}/${model.model_id}) - ${status}${defaultTag}`);
        console.log(`    Type: ${model.model_type}, Context: ${model.context_window}, Max Tokens: ${model.max_tokens}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Models command failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Chat command
program
  .command('chat')
  .description('Send a chat completion')
  .requiredOption('--prompt <prompt>', 'Message to send')
  .option('--model <model>', 'Model to use')
  .option('--temperature <temp>', 'Temperature', '0.7')
  .action(async (options) => {
    try {
      const db = new AiDatabase();
      await db.connect();

      let model = await db.getDefaultModel();
      if (options.model) {
        const models = await db.listModels(true);
        model = models.find(m => m.model_id === options.model || m.model_name === options.model) ?? model;
      }

      console.log(`\nModel: ${model?.model_name ?? 'none'}`);
      console.log(`Prompt: "${options.prompt}"`);
      console.log(`Temperature: ${options.temperature}`);
      console.log('\nResponse:');
      console.log(`  [AI Response] Configure provider API keys for actual completions.`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Chat failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Usage command
program
  .command('usage')
  .description('View AI usage statistics')
  .option('--start-date <date>', 'Start date (YYYY-MM-DD)')
  .option('--end-date <date>', 'End date (YYYY-MM-DD)')
  .option('--group-by <field>', 'Group by: day, model, provider', 'day')
  .action(async (options) => {
    try {
      const db = new AiDatabase();
      await db.connect();

      const usage = await db.getUsage({
        startDate: options.startDate,
        endDate: options.endDate,
        groupBy: options.groupBy,
      });

      console.log('\nAI Usage Statistics');
      console.log('====================');
      console.log(`Total Requests: ${usage.total_requests}`);
      console.log(`Total Tokens:   ${usage.total_tokens}`);
      console.log(`Total Cost:     $${usage.total_cost.toFixed(4)}`);

      if (usage.breakdown.length > 0) {
        console.log(`\nBreakdown by ${options.groupBy}:`);
        for (const entry of usage.breakdown) {
          const label = entry.date ?? entry.model ?? entry.provider ?? 'unknown';
          console.log(`  ${label}: ${entry.requests} requests, ${entry.tokens} tokens, $${entry.cost.toFixed(4)}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Usage command failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Prompts command
program
  .command('prompts')
  .description('List prompt templates')
  .option('--category <category>', 'Filter by category')
  .action(async (options) => {
    try {
      const db = new AiDatabase();
      await db.connect();

      const templates = await db.listPromptTemplates(options.category);

      console.log(`\nPrompt Templates (${templates.length}):`);
      console.log('========================');
      for (const t of templates) {
        console.log(`  ${t.name} [${t.category ?? 'uncategorized'}] - used ${t.usage_count} times`);
        if (t.description) console.log(`    ${t.description}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Prompts command failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

program.parse();
