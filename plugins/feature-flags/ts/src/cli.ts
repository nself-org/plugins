#!/usr/bin/env node
/**
 * Feature Flags Plugin CLI
 * Command-line interface for the feature flags plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { FeatureFlagsDatabase } from './database.js';
import { createServer } from './server.js';
import { evaluateFlag } from './evaluator.js';

const logger = createLogger('feature-flags:cli');

const program = new Command();

program
  .name('nself-feature-flags')
  .description('Feature flags plugin for nself - feature flag management and evaluation')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      logger.info('Initializing feature flags schema...');

      const db = new FeatureFlagsDatabase();
      await db.connect();
      await db.initializeSchema();

      logger.success('Schema initialized successfully');
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the API server')
  .option('-p, --port <port>', 'Server port', '3305')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info('Starting feature flags server...');
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
  .description('Show plugin status and statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new FeatureFlagsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nFeature Flags Status');
      console.log('====================');
      console.log(`Flags:       ${stats.flags}`);
      console.log(`Rules:       ${stats.rules}`);
      console.log(`Segments:    ${stats.segments}`);
      console.log(`Evaluations: ${stats.evaluations}`);
      if (stats.lastEvaluatedAt) {
        console.log(`Last Eval:   ${stats.lastEvaluatedAt.toISOString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Flags command
program
  .command('flags')
  .description('List all feature flags')
  .option('-t, --type <type>', 'Filter by flag type')
  .option('--tag <tag>', 'Filter by tag')
  .option('-e, --enabled', 'Show only enabled flags')
  .option('-d, --disabled', 'Show only disabled flags')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new FeatureFlagsDatabase();
      await db.connect();

      let enabled: boolean | undefined;
      if (options.enabled) {
        enabled = true;
      } else if (options.disabled) {
        enabled = false;
      }

      const flags = await db.listFlags({
        flag_type: options.type,
        tag: options.tag,
        enabled,
      });

      if (flags.length === 0) {
        console.log('No flags found');
      } else {
        console.log(`\nFound ${flags.length} flag(s):\n`);
        for (const flag of flags) {
          const status = flag.enabled ? 'ENABLED' : 'DISABLED';
          const tags = flag.tags.length > 0 ? ` [${flag.tags.join(', ')}]` : '';
          console.log(`${flag.key} (${flag.flag_type}) - ${status}${tags}`);
          if (flag.description) {
            console.log(`  ${flag.description}`);
          }
          console.log(`  Default: ${JSON.stringify(flag.default_value)}`);
          console.log(`  Evaluations: ${flag.evaluation_count}`);
          console.log();
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list flags', { error: message });
      process.exit(1);
    }
  });

// Evaluate command
program
  .command('evaluate <flag>')
  .description('Evaluate a feature flag')
  .option('-u, --user <userId>', 'User ID for evaluation')
  .option('-c, --context <json>', 'Context as JSON string')
  .action(async (flag, options) => {
    try {
      loadConfig();
      const db = new FeatureFlagsDatabase();
      await db.connect();

      let context = {};
      if (options.context) {
        try {
          context = JSON.parse(options.context);
        } catch (error) {
          logger.error('Invalid JSON context', { error });
          process.exit(1);
        }
      }

      const result = await evaluateFlag(flag, options.user, context, db);

      console.log('\nEvaluation Result:');
      console.log('==================');
      console.log(`Flag:   ${result.flag_key}`);
      console.log(`Value:  ${JSON.stringify(result.value)}`);
      console.log(`Reason: ${result.reason}`);
      if (result.rule_id) {
        console.log(`Rule:   ${result.rule_id}`);
      }
      if (result.error) {
        console.log(`Error:  ${result.error}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Evaluation failed', { error: message });
      process.exit(1);
    }
  });

// Segments command
program
  .command('segments')
  .description('List all segments')
  .action(async () => {
    try {
      loadConfig();
      const db = new FeatureFlagsDatabase();
      await db.connect();

      const segments = await db.listSegments();

      if (segments.length === 0) {
        console.log('No segments found');
      } else {
        console.log(`\nFound ${segments.length} segment(s):\n`);
        for (const segment of segments) {
          console.log(`${segment.name} (${segment.match_type})`);
          if (segment.description) {
            console.log(`  ${segment.description}`);
          }
          console.log(`  Rules: ${segment.rules.length}`);
          for (const rule of segment.rules) {
            console.log(`    - ${rule.attribute} ${rule.operator} ${JSON.stringify(rule.value)}`);
          }
          console.log();
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list segments', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show detailed statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new FeatureFlagsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nDetailed Statistics');
      console.log('===================');
      console.log(`Total Flags:       ${stats.flags}`);
      console.log(`Total Rules:       ${stats.rules}`);
      console.log(`Total Segments:    ${stats.segments}`);
      console.log(`Total Evaluations: ${stats.evaluations}`);
      if (stats.lastEvaluatedAt) {
        console.log(`Last Evaluation:   ${stats.lastEvaluatedAt.toISOString()}`);
      }

      // Get flags by type
      const releaseFlags = await db.listFlags({ flag_type: 'release' });
      const opsFlags = await db.listFlags({ flag_type: 'ops' });
      const experimentFlags = await db.listFlags({ flag_type: 'experiment' });
      const killSwitchFlags = await db.listFlags({ flag_type: 'kill_switch' });

      console.log('\nFlags by Type:');
      console.log(`  Release:     ${releaseFlags.length}`);
      console.log(`  Ops:         ${opsFlags.length}`);
      console.log(`  Experiment:  ${experimentFlags.length}`);
      console.log(`  Kill Switch: ${killSwitchFlags.length}`);

      // Get enabled vs disabled
      const enabledFlags = await db.listFlags({ enabled: true });
      const disabledFlags = await db.listFlags({ enabled: false });

      console.log('\nFlags by Status:');
      console.log(`  Enabled:  ${enabledFlags.length}`);
      console.log(`  Disabled: ${disabledFlags.length}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
