#!/usr/bin/env node
/**
 * Data Export Plugin CLI
 * Command-line interface for the Data Export plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ExportDatabase } from './database.js';
import { ExportService } from './export-service.js';
import { createServer } from './server.js';

const logger = createLogger('data-export:cli');

const program = new Command();

program
  .name('nself-data-export')
  .description('Data Export plugin for nself - GDPR-compliant data export, deletion, and import')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new ExportDatabase();
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
  .description('Start the HTTP server')
  .option('-p, --port <port>', 'Server port', '3306')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting Data Export server on ${config.host}:${config.port}`);
      await createServer(config);
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

      const db = new ExportDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nData Export Plugin Status');
      console.log('========================');
      console.log('\nExports:');
      console.log(`  Total:     ${stats.totalExports}`);
      console.log(`  Pending:   ${stats.pendingExports}`);
      console.log(`  Completed: ${stats.completedExports}`);
      console.log(`  Failed:    ${stats.failedExports}`);
      console.log(`  Last:      ${stats.lastExportAt?.toISOString() ?? 'N/A'}`);

      console.log('\nDeletions:');
      console.log(`  Total:     ${stats.totalDeletions}`);
      console.log(`  Pending:   ${stats.pendingDeletions}`);
      console.log(`  Completed: ${stats.completedDeletions}`);
      console.log(`  Failed:    ${stats.failedDeletions}`);
      console.log(`  Last:      ${stats.lastDeletionAt?.toISOString() ?? 'N/A'}`);

      console.log('\nImports:');
      console.log(`  Total:     ${stats.totalImports}`);
      console.log(`  Last:      ${stats.lastImportAt?.toISOString() ?? 'N/A'}`);

      console.log('\nPlugins:');
      console.log(`  Registered: ${stats.registeredPlugins}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Export command
program
  .command('export')
  .description('Create and manage export requests')
  .argument('[action]', 'Action: create, list, show, process', 'list')
  .argument('[id]', 'Export request ID (for show/process)')
  .option('-t, --type <type>', 'Request type: user_data, plugin_data, full_backup, custom', 'user_data')
  .option('-r, --requester <id>', 'Requester ID')
  .option('-u, --user <id>', 'Target user ID')
  .option('-p, --plugins <plugins>', 'Comma-separated plugin names')
  .option('-f, --format <format>', 'Export format: json, csv, zip', 'json')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      const config = loadConfig();
      const db = new ExportDatabase();
      await db.connect();

      switch (action) {
        case 'create': {
          if (!options.requester) {
            logger.error('Requester ID required (--requester)');
            process.exit(1);
          }

          const exportId = await db.createExportRequest({
            requestType: options.type as 'user_data' | 'plugin_data' | 'full_backup' | 'custom',
            requesterId: options.requester,
            targetUserId: options.user,
            targetPlugins: options.plugins ? options.plugins.split(',').map((s: string) => s.trim()) : undefined,
            format: options.format as 'json' | 'csv' | 'zip',
          });

          logger.success(`Export request created: ${exportId}`);
          break;
        }

        case 'list': {
          const exports = await db.listExportRequests(parseInt(options.limit, 10));
          console.log('\nExport Requests:');
          console.log('-'.repeat(120));
          console.log('ID                                   | Type        | Status     | Created');
          console.log('-'.repeat(120));
          exports.forEach(exp => {
            console.log(`${exp.id} | ${exp.request_type.padEnd(11)} | ${exp.status.padEnd(10)} | ${exp.created_at.toISOString()}`);
          });
          console.log(`\nTotal: ${await db.countExportRequests()}`);
          break;
        }

        case 'show': {
          if (!id) {
            logger.error('Export request ID required');
            process.exit(1);
          }

          const exportReq = await db.getExportRequest(id);
          if (!exportReq) {
            logger.error('Export request not found');
            process.exit(1);
          }

          console.log(JSON.stringify(exportReq, null, 2));
          break;
        }

        case 'process': {
          if (!id) {
            logger.error('Export request ID required');
            process.exit(1);
          }

          const exportService = new ExportService(
            db,
            config.storagePath,
            config.downloadExpiryHours,
            config.deletionCooldownHours,
            config.verificationCodeLength
          );

          logger.info(`Processing export request: ${id}`);
          await exportService.processExportRequest(id);
          logger.success('Export completed');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export command failed', { error: message });
      process.exit(1);
    }
  });

// Delete command
program
  .command('delete')
  .description('Create and manage deletion requests')
  .argument('[action]', 'Action: create, list, show, verify, process, cancel', 'list')
  .argument('[id]', 'Deletion request ID (for show/verify/process/cancel)')
  .option('-r, --requester <id>', 'Requester ID')
  .option('-u, --user <id>', 'Target user ID')
  .option('-c, --code <code>', 'Verification code')
  .option('--reason <reason>', 'Deletion reason')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      const config = loadConfig();
      const db = new ExportDatabase();
      await db.connect();

      const exportService = new ExportService(
        db,
        config.storagePath,
        config.downloadExpiryHours,
        config.deletionCooldownHours,
        config.verificationCodeLength
      );

      switch (action) {
        case 'create': {
          if (!options.requester || !options.user) {
            logger.error('Requester ID and user ID required (--requester, --user)');
            process.exit(1);
          }

          const { id: deletionId, verificationCode } = await exportService.createDeletionWithVerification(
            options.requester,
            options.user,
            options.reason
          );

          logger.success(`Deletion request created: ${deletionId}`);
          logger.warn(`Verification code: ${verificationCode} (send this to the user)`);
          break;
        }

        case 'list': {
          const deletions = await db.listDeletionRequests(parseInt(options.limit, 10));
          console.log('\nDeletion Requests:');
          console.log('-'.repeat(120));
          console.log('ID                                   | User ID          | Status     | Created');
          console.log('-'.repeat(120));
          deletions.forEach(del => {
            console.log(`${del.id} | ${del.target_user_id.padEnd(16)} | ${del.status.padEnd(10)} | ${del.created_at.toISOString()}`);
          });
          console.log(`\nTotal: ${await db.countDeletionRequests()}`);
          break;
        }

        case 'show': {
          if (!id) {
            logger.error('Deletion request ID required');
            process.exit(1);
          }

          const deletion = await db.getDeletionRequest(id);
          if (!deletion) {
            logger.error('Deletion request not found');
            process.exit(1);
          }

          console.log(JSON.stringify(deletion, null, 2));
          break;
        }

        case 'verify': {
          if (!id || !options.code) {
            logger.error('Deletion request ID and verification code required (--code)');
            process.exit(1);
          }

          const verified = await exportService.verifyDeletion(id, options.code);
          if (verified) {
            logger.success('Deletion request verified. Cooldown period started.');
          } else {
            logger.error('Invalid verification code');
            process.exit(1);
          }
          break;
        }

        case 'process': {
          if (!id) {
            logger.error('Deletion request ID required');
            process.exit(1);
          }

          logger.info(`Processing deletion request: ${id}`);
          await exportService.processDeletionRequest(id);
          logger.success('Deletion completed');
          break;
        }

        case 'cancel': {
          if (!id) {
            logger.error('Deletion request ID required');
            process.exit(1);
          }

          await db.cancelDeletionRequest(id);
          logger.success('Deletion request cancelled');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Delete command failed', { error: message });
      process.exit(1);
    }
  });

// Import command
program
  .command('import')
  .description('Create and manage import jobs')
  .argument('[action]', 'Action: create, list, show, process', 'list')
  .argument('[id]', 'Import job ID (for show/process)')
  .option('-r, --requester <id>', 'Requester ID')
  .option('-s, --source <path>', 'Source file path')
  .option('-t, --type <type>', 'Source type: file, url', 'file')
  .option('-f, --format <format>', 'Import format: json, csv, zip', 'json')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, id, options) => {
    try {
      const config = loadConfig();
      const db = new ExportDatabase();
      await db.connect();

      const exportService = new ExportService(
        db,
        config.storagePath,
        config.downloadExpiryHours,
        config.deletionCooldownHours,
        config.verificationCodeLength
      );

      switch (action) {
        case 'create': {
          if (!options.requester || !options.source) {
            logger.error('Requester ID and source path required (--requester, --source)');
            process.exit(1);
          }

          const importId = await db.createImportJob({
            requesterId: options.requester,
            sourceType: options.type as 'file' | 'url',
            sourcePath: options.source,
            format: options.format as 'json' | 'csv' | 'zip',
          });

          logger.success(`Import job created: ${importId}`);
          break;
        }

        case 'list': {
          const imports = await db.listImportJobs(parseInt(options.limit, 10));
          console.log('\nImport Jobs:');
          console.log('-'.repeat(120));
          console.log('ID                                   | Status     | Source                     | Created');
          console.log('-'.repeat(120));
          imports.forEach(imp => {
            const source = (imp.source_path ?? '').substring(0, 24);
            console.log(`${imp.id} | ${imp.status.padEnd(10)} | ${source.padEnd(26)} | ${imp.created_at.toISOString()}`);
          });
          console.log(`\nTotal: ${await db.countImportJobs()}`);
          break;
        }

        case 'show': {
          if (!id) {
            logger.error('Import job ID required');
            process.exit(1);
          }

          const job = await db.getImportJob(id);
          if (!job) {
            logger.error('Import job not found');
            process.exit(1);
          }

          console.log(JSON.stringify(job, null, 2));
          break;
        }

        case 'process': {
          if (!id) {
            logger.error('Import job ID required');
            process.exit(1);
          }

          logger.info(`Processing import job: ${id}`);
          await exportService.processImportJob(id);
          logger.success('Import completed');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import command failed', { error: message });
      process.exit(1);
    }
  });

// Plugins command
program
  .command('plugins')
  .description('Manage plugin registry')
  .argument('[action]', 'Action: list, show, register, update, unregister', 'list')
  .argument('[id]', 'Plugin ID (for show/update/unregister)')
  .option('-n, --name <name>', 'Plugin name')
  .option('-t, --tables <tables>', 'Comma-separated table names')
  .option('-c, --column <column>', 'User ID column name', 'user_id')
  .option('--export-query <query>', 'Custom export query')
  .option('--deletion-query <query>', 'Custom deletion query')
  .option('--enabled <enabled>', 'Enabled status (true/false)', 'true')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (action, id, options) => {
    try {
      loadConfig();
      const db = new ExportDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const plugins = await db.listPluginRegistry(parseInt(options.limit, 10));
          console.log('\nRegistered Plugins:');
          console.log('-'.repeat(100));
          console.log('ID                                   | Plugin Name       | Tables | Enabled');
          console.log('-'.repeat(100));
          plugins.forEach(p => {
            console.log(`${p.id} | ${p.plugin_name.padEnd(17)} | ${String(p.tables.length).padStart(6)} | ${p.enabled ? 'Yes' : 'No'}`);
          });
          console.log(`\nTotal: ${await db.countPluginRegistry()}`);
          break;
        }

        case 'show': {
          if (!id) {
            logger.error('Plugin ID required');
            process.exit(1);
          }

          const plugin = await db.getPluginRegistry(id);
          if (!plugin) {
            logger.error('Plugin not found');
            process.exit(1);
          }

          console.log(JSON.stringify(plugin, null, 2));
          break;
        }

        case 'register': {
          if (!options.name || !options.tables) {
            logger.error('Plugin name and tables required (--name, --tables)');
            process.exit(1);
          }

          const pluginId = await db.registerPlugin({
            pluginName: options.name,
            tables: options.tables.split(',').map((s: string) => s.trim()),
            userIdColumn: options.column,
            exportQuery: options.exportQuery,
            deletionQuery: options.deletionQuery,
            enabled: options.enabled === 'true',
          });

          logger.success(`Plugin registered: ${pluginId}`);
          break;
        }

        case 'update': {
          if (!id) {
            logger.error('Plugin ID required');
            process.exit(1);
          }

          await db.updatePluginRegistry(id, {
            tables: options.tables ? options.tables.split(',').map((s: string) => s.trim()) : undefined,
            userIdColumn: options.column,
            exportQuery: options.exportQuery,
            deletionQuery: options.deletionQuery,
            enabled: options.enabled ? options.enabled === 'true' : undefined,
          });

          logger.success('Plugin updated');
          break;
        }

        case 'unregister': {
          if (!id) {
            logger.error('Plugin ID required');
            process.exit(1);
          }

          await db.deletePluginRegistry(id);
          logger.success('Plugin unregistered');
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Plugins command failed', { error: message });
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
      const db = new ExportDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nData Export Plugin Statistics');
      console.log('============================');
      console.log('\nExports:');
      console.log(`  Total:       ${stats.totalExports}`);
      console.log(`  Pending:     ${stats.pendingExports}`);
      console.log(`  Completed:   ${stats.completedExports}`);
      console.log(`  Failed:      ${stats.failedExports}`);
      console.log(`  Last Export: ${stats.lastExportAt?.toISOString() ?? 'N/A'}`);

      console.log('\nDeletions:');
      console.log(`  Total:         ${stats.totalDeletions}`);
      console.log(`  Pending:       ${stats.pendingDeletions}`);
      console.log(`  Completed:     ${stats.completedDeletions}`);
      console.log(`  Failed:        ${stats.failedDeletions}`);
      console.log(`  Last Deletion: ${stats.lastDeletionAt?.toISOString() ?? 'N/A'}`);

      console.log('\nImports:');
      console.log(`  Total:       ${stats.totalImports}`);
      console.log(`  Last Import: ${stats.lastImportAt?.toISOString() ?? 'N/A'}`);

      console.log('\nPlugins:');
      console.log(`  Registered: ${stats.registeredPlugins}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
