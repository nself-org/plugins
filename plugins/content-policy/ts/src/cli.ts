#!/usr/bin/env node
/**
 * Content Policy Plugin CLI
 * Command-line interface for content policy management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ContentPolicyDatabase } from './database.js';
import { ContentPolicyEvaluator } from './evaluator.js';
import { createServer } from './server.js';

const logger = createLogger('content-policy:cli');

const program = new Command();

program
  .name('nself-content-policy')
  .description('Content policy evaluation and moderation plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new ContentPolicyDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.success('Database schema initialized');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Init failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the content policy server')
  .option('-p, --port <port>', 'Server port', '3504')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info('Starting content policy server...');
      logger.info(`Port: ${config.port}`);
      logger.info(`Host: ${config.host}`);

      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show content policy statistics')
  .option('-d, --days <days>', 'Number of days to look back', '30')
  .action(async (options) => {
    try {
      loadConfig();

      const db = new ContentPolicyDatabase();
      await db.connect();

      const days = parseInt(options.days, 10);
      const since = new Date();
      since.setDate(since.getDate() - days);

      const stats = await db.getStats(since);

      console.log('\nContent Policy Statistics');
      console.log('=========================');
      console.log(`Period: Last ${days} days`);
      console.log(`\nEvaluations: ${stats.total_evaluations}`);
      console.log(`  Allowed:      ${stats.allowed} (${((stats.allowed / stats.total_evaluations) * 100).toFixed(1)}%)`);
      console.log(`  Denied:       ${stats.denied} (${((stats.denied / stats.total_evaluations) * 100).toFixed(1)}%)`);
      console.log(`  Flagged:      ${stats.flagged} (${((stats.flagged / stats.total_evaluations) * 100).toFixed(1)}%)`);
      console.log(`  Quarantined:  ${stats.quarantined} (${((stats.quarantined / stats.total_evaluations) * 100).toFixed(1)}%)`);
      console.log(`\nOverrides: ${stats.override_count}`);
      console.log(`Avg Processing Time: ${stats.avg_processing_time_ms.toFixed(2)}ms`);

      if (stats.top_violations.length > 0) {
        console.log('\nTop Violations:');
        stats.top_violations.forEach((v, i) => {
          console.log(`  ${i + 1}. ${v.rule_name}: ${v.count}`);
        });
      }

      if (stats.evaluations_by_content_type.length > 0) {
        console.log('\nBy Content Type:');
        stats.evaluations_by_content_type.forEach(ct => {
          console.log(`  ${ct.content_type}: ${ct.count}`);
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Evaluate command
program
  .command('evaluate')
  .description('Evaluate content against policies')
  .argument('<content>', 'Content text to evaluate')
  .option('-t, --type <type>', 'Content type', 'text')
  .option('-i, --id <id>', 'Content ID')
  .option('-s, --submitter <submitter>', 'Submitter ID')
  .action(async (content, options) => {
    try {
      loadConfig();

      const db = new ContentPolicyDatabase();
      await db.connect();

      const evaluator = new ContentPolicyEvaluator(db);

      const result = await evaluator.evaluate({
        content_type: options.type,
        content_text: content,
        content_id: options.id,
        submitter_id: options.submitter,
      });

      console.log('\nEvaluation Result');
      console.log('=================');
      console.log(`Result: ${result.result.toUpperCase()}`);
      console.log(`Score: ${(result.score * 100).toFixed(1)}%`);
      console.log(`Processing Time: ${result.processing_time_ms}ms`);
      console.log(`Message: ${result.message}`);

      if (result.matched_rules.length > 0) {
        console.log('\nMatched Rules:');
        result.matched_rules.forEach((rule, i) => {
          console.log(`  ${i + 1}. ${rule.rule_name} (${rule.severity})`);
          console.log(`     Type: ${rule.rule_type}`);
          console.log(`     Action: ${rule.action}`);
          if (rule.message) {
            console.log(`     Message: ${rule.message}`);
          }
          if (rule.matched_text) {
            console.log(`     Matched: "${rule.matched_text}"`);
          }
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evaluation failed', { error: message });
      process.exit(1);
    }
  });

// Policies command
program
  .command('policies')
  .description('Manage content policies')
  .argument('[action]', 'Action: list, show, create, delete', 'list')
  .argument('[id]', 'Policy ID (for show/delete)')
  .option('-n, --name <name>', 'Policy name (for create)')
  .option('-d, --description <description>', 'Policy description')
  .option('-t, --types <types>', 'Comma-separated content types')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      loadConfig();

      const db = new ContentPolicyDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const policies = await db.listPolicies(parseInt(options.limit, 10));
          console.log('\nPolicies:');
          console.log('-'.repeat(80));
          policies.forEach(p => {
            const enabled = p.enabled ? '✓' : '✗';
            const types = p.content_types.length > 0 ? p.content_types.join(', ') : 'all';
            console.log(`[${enabled}] ${p.name} (${p.mode}, priority: ${p.priority})`);
            console.log(`    Types: ${types}`);
            if (p.description) {
              console.log(`    ${p.description}`);
            }
            console.log();
          });
          break;
        }

        case 'show': {
          if (!id) {
            logger.error('Policy ID required');
            process.exit(1);
          }
          const policy = await db.getPolicy(id);
          if (!policy) {
            logger.error('Policy not found');
            process.exit(1);
          }
          const rules = await db.listRules(id);
          console.log('\nPolicy Details:');
          console.log(JSON.stringify({ ...policy, rules }, null, 2));
          break;
        }

        case 'create': {
          if (!options.name) {
            logger.error('Policy name required (--name)');
            process.exit(1);
          }
          const policy = await db.createPolicy({
            name: options.name,
            description: options.description,
            content_types: options.types ? options.types.split(',').map((t: string) => t.trim()) : [],
          });
          logger.success(`Policy created: ${policy.id}`);
          console.log(JSON.stringify(policy, null, 2));
          break;
        }

        case 'delete': {
          if (!id) {
            logger.error('Policy ID required');
            process.exit(1);
          }
          const deleted = await db.deletePolicy(id);
          if (!deleted) {
            logger.error('Policy not found');
            process.exit(1);
          }
          logger.success('Policy deleted');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Policies command failed', { error: message });
      process.exit(1);
    }
  });

// Word lists command
program
  .command('word-lists')
  .description('Manage word lists')
  .argument('[action]', 'Action: list, show, create, add, remove, delete', 'list')
  .argument('[id]', 'Word list ID')
  .option('-n, --name <name>', 'Word list name')
  .option('-t, --type <type>', 'List type: blocklist or allowlist', 'blocklist')
  .option('-w, --words <words>', 'Comma-separated words')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      loadConfig();

      const db = new ContentPolicyDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const wordLists = await db.listWordLists(parseInt(options.limit, 10));
          console.log('\nWord Lists:');
          console.log('-'.repeat(80));
          wordLists.forEach(wl => {
            console.log(`${wl.name} (${wl.list_type})`);
            console.log(`  ID: ${wl.id}`);
            console.log(`  Words: ${wl.words.length}`);
            console.log(`  Case Sensitive: ${wl.case_sensitive}`);
            console.log();
          });
          break;
        }

        case 'show': {
          if (!id) {
            logger.error('Word list ID required');
            process.exit(1);
          }
          const wordList = await db.getWordList(id);
          if (!wordList) {
            logger.error('Word list not found');
            process.exit(1);
          }
          console.log('\nWord List Details:');
          console.log(JSON.stringify(wordList, null, 2));
          break;
        }

        case 'create': {
          if (!options.name || !options.words) {
            logger.error('Name and words required (--name, --words)');
            process.exit(1);
          }
          const words = options.words.split(',').map((w: string) => w.trim());
          const wordList = await db.createWordList({
            name: options.name,
            list_type: options.type as 'blocklist' | 'allowlist',
            words,
          });
          logger.success(`Word list created: ${wordList.id}`);
          console.log(JSON.stringify(wordList, null, 2));
          break;
        }

        case 'add': {
          if (!id || !options.words) {
            logger.error('Word list ID and words required');
            process.exit(1);
          }
          const words = options.words.split(',').map((w: string) => w.trim());
          const wordList = await db.updateWordList(id, { add_words: words });
          if (!wordList) {
            logger.error('Word list not found');
            process.exit(1);
          }
          logger.success(`Added ${words.length} words`);
          break;
        }

        case 'remove': {
          if (!id || !options.words) {
            logger.error('Word list ID and words required');
            process.exit(1);
          }
          const words = options.words.split(',').map((w: string) => w.trim());
          const wordList = await db.updateWordList(id, { remove_words: words });
          if (!wordList) {
            logger.error('Word list not found');
            process.exit(1);
          }
          logger.success(`Removed ${words.length} words`);
          break;
        }

        case 'delete': {
          if (!id) {
            logger.error('Word list ID required');
            process.exit(1);
          }
          const deleted = await db.deleteWordList(id);
          if (!deleted) {
            logger.error('Word list not found');
            process.exit(1);
          }
          logger.success('Word list deleted');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Word lists command failed', { error: message });
      process.exit(1);
    }
  });

// Queue command
program
  .command('queue')
  .description('View flagged/quarantined content awaiting moderation')
  .option('-r, --result <result>', 'Filter by result: flagged or quarantined')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();

      const db = new ContentPolicyDatabase();
      await db.connect();

      const queue = await db.getQueue(
        options.result as 'flagged' | 'quarantined' | undefined,
        parseInt(options.limit, 10)
      );

      console.log('\nModeration Queue:');
      console.log('-'.repeat(80));

      if (queue.length === 0) {
        console.log('No items in queue');
      } else {
        queue.forEach((item, i) => {
          console.log(`${i + 1}. [${item.result.toUpperCase()}] ${item.content_type}`);
          console.log(`   ID: ${item.evaluation_id}`);
          if (item.content_id) {
            console.log(`   Content ID: ${item.content_id}`);
          }
          if (item.submitter_id) {
            console.log(`   Submitter: ${item.submitter_id}`);
          }
          console.log(`   Score: ${(item.score * 100).toFixed(1)}%`);
          console.log(`   Rules: ${item.matched_rules.length} matched`);
          if (item.content_text) {
            const preview = item.content_text.substring(0, 100);
            console.log(`   Preview: ${preview}${item.content_text.length > 100 ? '...' : ''}`);
          }
          console.log(`   Created: ${item.created_at.toISOString()}`);
          console.log();
        });
      }

      console.log(`Total: ${queue.length} items`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Queue command failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show content policy statistics')
  .option('-d, --days <days>', 'Number of days to look back', '7')
  .action(async (options) => {
    try {
      loadConfig();

      const db = new ContentPolicyDatabase();
      await db.connect();

      const days = parseInt(options.days, 10);
      const since = new Date();
      since.setDate(since.getDate() - days);

      const stats = await db.getStats(since);

      console.log('\nContent Policy Statistics');
      console.log('=========================');
      console.log(`Period: Last ${days} days\n`);
      console.log('Evaluations by Result:');
      stats.evaluations_by_result.forEach(r => {
        const pct = stats.total_evaluations > 0 ? ((r.count / stats.total_evaluations) * 100).toFixed(1) : '0.0';
        console.log(`  ${r.result}: ${r.count} (${pct}%)`);
      });

      console.log(`\nTotal: ${stats.total_evaluations}`);
      console.log(`Overrides: ${stats.override_count}`);
      console.log(`Avg Processing Time: ${stats.avg_processing_time_ms.toFixed(2)}ms`);

      if (stats.top_violations.length > 0) {
        console.log('\nTop Violations:');
        stats.top_violations.slice(0, 10).forEach((v, i) => {
          console.log(`  ${i + 1}. ${v.rule_name}: ${v.count}`);
        });
      }

      if (stats.evaluations_by_content_type.length > 0) {
        console.log('\nBy Content Type:');
        stats.evaluations_by_content_type.forEach(ct => {
          console.log(`  ${ct.content_type}: ${ct.count}`);
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
