#!/usr/bin/env node
/**
 * Moderation Plugin CLI
 * Command-line interface for the Moderation plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ModerationDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('moderation:cli');

const program = new Command();

program
  .name('nself-moderation')
  .description('Content moderation plugin for nself - profanity filtering, toxicity detection, review workflows')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize moderation plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing moderation schema...');
      const db = new ModerationDatabase();
      await db.connect();
      await db.initializeSchema();
      console.log('Done - Moderation schema initialized successfully');
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
  .description('Start moderation plugin server')
  .option('-p, --port <port>', 'Server port', '3704')
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
  .description('Show moderation plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new ModerationDatabase();
      await db.connect();

      const rules = await db.listRules();
      const wordlists = await db.listWordlists();
      const stats = await db.getOverviewStats(30);

      console.log('\nModeration Plugin Status');
      console.log('========================');
      console.log(`Port:       ${config.port}`);
      console.log(`Toxicity:   ${config.toxicityEnabled ? `enabled (${config.toxicityProvider})` : 'disabled'}`);
      console.log(`Appeals:    ${config.appealsEnabled ? 'enabled' : 'disabled'}`);
      console.log(`Rules:      ${rules.length} (${rules.filter(r => r.is_enabled).length} enabled)`);
      console.log(`Wordlists:  ${wordlists.length} (${wordlists.filter(w => w.is_enabled).length} enabled)`);
      console.log(`\nLast 30 Days:`);
      console.log(`  Actions:  ${stats.total_actions}`);
      console.log(`  Flags:    ${stats.total_flags}`);
      console.log(`  Reports:  ${stats.total_reports}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Analyze command
program
  .command('analyze')
  .description('Analyze content for moderation issues')
  .requiredOption('--content <content>', 'Content to analyze')
  .option('--type <type>', 'Content type', 'message')
  .action(async (options) => {
    try {
      const db = new ModerationDatabase();
      await db.connect();

      const profanityResult = await db.checkProfanity(options.content);
      const rules = await db.listRules(true);

      console.log('\nContent Analysis');
      console.log('=================');
      console.log(`Content:    "${options.content}"`);
      console.log(`Profanity:  ${profanityResult.matched_words.length > 0 ? 'YES' : 'No'}`);
      if (profanityResult.matched_words.length > 0) {
        console.log(`  Words:    ${profanityResult.matched_words.join(', ')}`);
        console.log(`  Severity: ${profanityResult.severity}`);
      }
      console.log(`Rules:      ${rules.length} active rules checked`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Analysis failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Queue command
program
  .command('queue')
  .description('View moderation review queue')
  .option('--status <status>', 'Filter by status', 'pending')
  .option('--severity <severity>', 'Filter by severity')
  .option('--limit <limit>', 'Result limit', '20')
  .action(async (options) => {
    try {
      const db = new ModerationDatabase();
      await db.connect();

      const result = await db.listFlags({
        status: options.status,
        severity: options.severity,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nModeration Queue (${result.total} total, showing ${result.flags.length}):`);
      console.log('===========================================================');

      if (result.flags.length === 0) {
        console.log('Queue is empty.');
      } else {
        for (const flag of result.flags) {
          console.log(`\n  [${flag.severity.toUpperCase()}] ${flag.id}`);
          console.log(`    Type:     ${flag.content_type}`);
          console.log(`    Reason:   ${flag.flag_reason}`);
          console.log(`    Category: ${flag.flag_category ?? 'N/A'}`);
          console.log(`    Status:   ${flag.status}`);
          console.log(`    Created:  ${flag.created_at}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Queue failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Action command
program
  .command('action')
  .description('Take a moderation action')
  .requiredOption('--user <user_id>', 'Target user ID')
  .requiredOption('--type <type>', 'Action type (warn, mute, kick, ban)')
  .requiredOption('--reason <reason>', 'Reason for action')
  .option('--duration <minutes>', 'Duration in minutes (for mute/ban)')
  .action(async (options) => {
    try {
      const db = new ModerationDatabase();
      await db.connect();

      const action = await db.createAction({
        user_id: options.user,
        action_type: options.type,
        reason: options.reason,
        duration_minutes: options.duration ? parseInt(options.duration, 10) : undefined,
      });

      console.log(`Done - Action created: ${action.id}`);
      console.log(`  Type:     ${action.action_type}`);
      console.log(`  User:     ${action.target_user_id}`);
      console.log(`  Reason:   ${action.reason}`);
      if (action.expires_at) {
        console.log(`  Expires:  ${action.expires_at.toISOString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Action failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('View moderation statistics')
  .option('--user <user_id>', 'View stats for a specific user')
  .option('--days <days>', 'Number of days for overview', '30')
  .action(async (options) => {
    try {
      const db = new ModerationDatabase();
      await db.connect();

      if (options.user) {
        const stats = await db.getUserStats(options.user);
        if (!stats) {
          console.log('No stats found for this user.');
        } else {
          console.log(`\nUser Moderation Stats: ${options.user}`);
          console.log('======================================');
          console.log(`  Warnings: ${stats.total_warnings}`);
          console.log(`  Mutes:    ${stats.total_mutes}`);
          console.log(`  Bans:     ${stats.total_bans}`);
          console.log(`  Flags:    ${stats.total_flags}`);
          console.log(`  Risk:     ${stats.risk_level} (${stats.risk_score})`);
          console.log(`  Muted:    ${stats.is_muted ? `Yes (until ${stats.muted_until})` : 'No'}`);
          console.log(`  Banned:   ${stats.is_banned ? `Yes (until ${stats.banned_until})` : 'No'}`);
        }
      } else {
        const days = parseInt(options.days, 10);
        const stats = await db.getOverviewStats(days);

        console.log(`\nModeration Overview (last ${days} days):`);
        console.log('========================================');
        console.log(`  Total Actions: ${stats.total_actions}`);
        console.log(`  Total Flags:   ${stats.total_flags}`);
        console.log(`  Total Reports: ${stats.total_reports}`);
        console.log(`  Avg Toxicity:  ${stats.average_toxicity_score.toFixed(4)}`);

        if (Object.keys(stats.actions_by_type).length > 0) {
          console.log('\n  Actions by Type:');
          for (const [type, count] of Object.entries(stats.actions_by_type)) {
            console.log(`    ${type}: ${count}`);
          }
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Cleanup expired actions
program
  .command('cleanup')
  .description('Clean up expired moderation actions')
  .action(async () => {
    try {
      const db = new ModerationDatabase();
      await db.connect();

      const count = await db.expireActions();
      console.log(`Done - Expired ${count} moderation actions`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cleanup failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

program.parse();
