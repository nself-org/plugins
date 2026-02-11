#!/usr/bin/env node
/**
 * Backup Plugin CLI
 * Command-line interface for the Backup plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { BackupDatabase } from './database.js';
import { BackupService } from './backup.js';
import { BackupScheduler } from './scheduler.js';
import { createServer } from './server.js';

const logger = createLogger('backup:cli');

const program = new Command();

program
  .name('nself-backup')
  .description('Backup plugin for nself - PostgreSQL backup and restore automation')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new BackupDatabase();
      await db.connect();
      await db.initializeSchema();

      console.log('✓ Database schema initialized');

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
  .description('Start the backup automation server')
  .option('-p, --port <port>', 'Server port', '3013')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting backup server on ${config.host}:${config.port}`);

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
  .description('Show backup status and statistics')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new BackupDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nBackup Status');
      console.log('=============');
      console.log(`Schedules:          ${stats.total_schedules} total, ${stats.active_schedules} active`);
      console.log(`Artifacts:          ${stats.total_artifacts} total, ${stats.completed_artifacts} completed, ${stats.failed_artifacts} failed`);
      console.log(`Total Size:         ${(stats.total_size_bytes / 1024 / 1024 / 1024).toFixed(2)} GB`);
      console.log(`Oldest Backup:      ${stats.oldest_backup ? stats.oldest_backup.toISOString() : 'N/A'}`);
      console.log(`Newest Backup:      ${stats.newest_backup ? stats.newest_backup.toISOString() : 'N/A'}`);
      console.log(`Active Restore Jobs: ${stats.active_restore_jobs}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Create schedule command
program
  .command('create-schedule')
  .description('Create a new backup schedule')
  .requiredOption('-n, --name <name>', 'Schedule name')
  .requiredOption('-c, --cron <expression>', 'Cron expression (e.g., "0 2 * * *")')
  .option('-t, --type <type>', 'Backup type (full, incremental, schema_only, data_only)', 'full')
  .option('--include <tables>', 'Comma-separated list of tables to include')
  .option('--exclude <tables>', 'Comma-separated list of tables to exclude')
  .option('--compression <type>', 'Compression type (none, gzip, zstd)', 'gzip')
  .option('--retention <days>', 'Retention period in days', '30')
  .option('--max-backups <count>', 'Maximum number of backups to keep', '10')
  .action(async (options) => {
    try {
      // Validate cron expression
      if (!BackupScheduler.validateCronExpression(options.cron)) {
        console.error('Error: Invalid cron expression');
        process.exit(1);
      }

      const db = new BackupDatabase();
      await db.connect();

      const schedule = await db.createSchedule({
        name: options.name,
        schedule_cron: options.cron,
        backup_type: options.type as 'full' | 'incremental' | 'schema_only' | 'data_only',
        include_tables: options.include ? options.include.split(',').map((s: string) => s.trim()) : [],
        exclude_tables: options.exclude ? options.exclude.split(',').map((s: string) => s.trim()) : [],
        compression: options.compression as 'none' | 'gzip' | 'zstd',
        retention_days: parseInt(options.retention, 10),
        max_backups: parseInt(options.maxBackups, 10),
        enabled: true,
      });

      console.log('\n✓ Schedule created successfully');
      console.log(`  ID:   ${schedule.id}`);
      console.log(`  Name: ${schedule.name}`);
      console.log(`  Cron: ${schedule.schedule_cron}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create schedule', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// List schedules command
program
  .command('list-schedules')
  .description('List all backup schedules')
  .option('-l, --limit <limit>', 'Maximum number of schedules to show', '100')
  .action(async (options) => {
    try {
      const db = new BackupDatabase();
      await db.connect();

      const schedules = await db.listSchedules(parseInt(options.limit, 10), 0);

      console.log('\nBackup Schedules');
      console.log('================\n');

      if (schedules.length === 0) {
        console.log('No schedules found');
      } else {
        schedules.forEach(schedule => {
          const status = schedule.enabled ? 'ENABLED' : 'DISABLED';
          console.log(`${schedule.name} (${schedule.id})`);
          console.log(`  Status:   ${status}`);
          console.log(`  Type:     ${schedule.backup_type}`);
          console.log(`  Cron:     ${schedule.schedule_cron}`);
          console.log(`  Last Run: ${schedule.last_run_at ? schedule.last_run_at.toISOString() : 'Never'}`);
          console.log(`  Next Run: ${schedule.next_run_at ? schedule.next_run_at.toISOString() : 'Not scheduled'}`);
          console.log('');
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list schedules', { error: message });
      process.exit(1);
    }
  });

// Run backup command
program
  .command('run-backup')
  .description('Run a backup immediately')
  .option('-s, --schedule <id>', 'Schedule ID to run')
  .option('-t, --type <type>', 'Backup type (full, incremental, schema_only, data_only)', 'full')
  .option('--include <tables>', 'Comma-separated list of tables to include')
  .option('--exclude <tables>', 'Comma-separated list of tables to exclude')
  .option('--compression <type>', 'Compression type (none, gzip, zstd)', 'gzip')
  .action(async (options) => {
    try {
      const config = loadConfig();

      const db = new BackupDatabase();
      await db.connect();

      const backupService = new BackupService(config, db);

      console.log('Starting backup...');

      const result = await backupService.executeBackup({
        scheduleId: options.schedule,
        backupType: options.type as 'full' | 'incremental' | 'schema_only' | 'data_only',
        includeTables: options.include ? options.include.split(',').map((s: string) => s.trim()) : undefined,
        excludeTables: options.exclude ? options.exclude.split(',').map((s: string) => s.trim()) : undefined,
        compression: options.compression as 'none' | 'gzip' | 'zstd',
        targetProvider: 'local',
        targetConfig: {},
        retentionDays: config.defaultRetentionDays,
      });

      if (result.success) {
        console.log('\n✓ Backup completed successfully');
        console.log(`  Artifact ID: ${result.artifactId}`);
        console.log(`  File Path:   ${result.filePath}`);
        console.log(`  File Size:   ${((result.fileSize ?? 0) / 1024 / 1024).toFixed(2)} MB`);
        console.log(`  Checksum:    ${result.checksum}`);
        console.log(`  Duration:    ${(result.duration / 1000).toFixed(1)}s`);
      } else {
        console.error('\n✗ Backup failed');
        console.error(`  Error: ${result.error}`);
      }

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// List backups command
program
  .command('list-backups')
  .description('List all backup artifacts')
  .option('-l, --limit <limit>', 'Maximum number of backups to show', '100')
  .option('-s, --status <status>', 'Filter by status (completed, failed, running)')
  .action(async (options) => {
    try {
      const db = new BackupDatabase();
      await db.connect();

      const artifacts = await db.listArtifacts(
        parseInt(options.limit, 10),
        0,
        options.status
      );

      console.log('\nBackup Artifacts');
      console.log('================\n');

      if (artifacts.length === 0) {
        console.log('No backups found');
      } else {
        artifacts.forEach(artifact => {
          console.log(`${artifact.id} - ${artifact.status.toUpperCase()}`);
          console.log(`  Type:      ${artifact.backup_type}`);
          console.log(`  Created:   ${artifact.created_at.toISOString()}`);
          if (artifact.completed_at) {
            console.log(`  Completed: ${artifact.completed_at.toISOString()}`);
          }
          if (artifact.file_size_bytes) {
            console.log(`  Size:      ${(artifact.file_size_bytes / 1024 / 1024).toFixed(2)} MB`);
          }
          if (artifact.error_message) {
            console.log(`  Error:     ${artifact.error_message}`);
          }
          console.log('');
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list backups', { error: message });
      process.exit(1);
    }
  });

// Restore command
program
  .command('restore')
  .description('Restore from a backup artifact')
  .requiredOption('-a, --artifact <id>', 'Artifact ID to restore from')
  .option('-d, --database <name>', 'Target database name')
  .option('-t, --tables <tables>', 'Comma-separated list of tables to restore')
  .option('-m, --mode <mode>', 'Restore mode (merge, replace, dry_run)', 'merge')
  .option('-c, --conflict <strategy>', 'Conflict strategy (skip, overwrite, error)', 'skip')
  .action(async (options) => {
    try {
      const config = loadConfig();

      const db = new BackupDatabase();
      await db.connect();

      const backupService = new BackupService(config, db);

      console.log('Starting restore...');

      const result = await backupService.executeRestore({
        artifactId: options.artifact,
        targetDatabase: options.database ?? config.databaseName,
        tablesToRestore: options.tables ? options.tables.split(',').map((s: string) => s.trim()) : undefined,
        restoreMode: options.mode as 'merge' | 'replace' | 'dry_run',
        conflictStrategy: options.conflict as 'skip' | 'overwrite' | 'error',
      });

      if (result.success) {
        console.log('\n✓ Restore completed successfully');
        console.log(`  Job ID:       ${result.jobId}`);
        console.log(`  Rows Restored: ${result.rowsRestored}`);
        console.log(`  Duration:     ${(result.duration / 1000).toFixed(1)}s`);
      } else {
        console.error('\n✗ Restore failed');
        result.errors.forEach(err => {
          console.error(`  ${err.table}: ${err.error}`);
        });
      }

      await db.disconnect();
      process.exit(result.success ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Restore failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Download command
program
  .command('download')
  .description('Download a backup artifact')
  .requiredOption('-a, --artifact <id>', 'Artifact ID to download')
  .requiredOption('-o, --output <path>', 'Output file path')
  .action(async (options) => {
    try {
      const db = new BackupDatabase();
      await db.connect();

      const artifact = await db.getArtifact(options.artifact);

      if (!artifact) {
        console.error('Error: Artifact not found');
        await db.disconnect();
        process.exit(1);
      }

      if (!artifact.file_path) {
        console.error('Error: Artifact has no file');
        await db.disconnect();
        process.exit(1);
      }

      // Copy file
      const fs = await import('fs/promises');
      await fs.copyFile(artifact.file_path, options.output);

      console.log(`✓ Downloaded to ${options.output}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Download failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

program.parse();
