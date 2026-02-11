#!/usr/bin/env node
/**
 * Export/Import Plugin CLI
 * Command-line interface for data export, import, migration, backup, and restore
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ExportImportDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('export-import:cli');

const program = new Command();

program
  .name('nself-export-import')
  .description('Data export/import, migration, backup, and restore plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
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
  .description('Start the export/import server')
  .option('-p, --port <port>', 'Server port', '3717')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info('Starting export/import server...');
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
  .description('Show export/import statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nExport/Import Statistics');
      console.log('========================');
      console.log(`\nExport Jobs:     ${stats.export_jobs.total} total`);
      console.log(`  Pending:       ${stats.export_jobs.pending}`);
      console.log(`  Running:       ${stats.export_jobs.running}`);
      console.log(`  Completed:     ${stats.export_jobs.completed}`);
      console.log(`  Failed:        ${stats.export_jobs.failed}`);

      console.log(`\nImport Jobs:     ${stats.import_jobs.total} total`);
      console.log(`  Pending:       ${stats.import_jobs.pending}`);
      console.log(`  Running:       ${stats.import_jobs.running}`);
      console.log(`  Completed:     ${stats.import_jobs.completed}`);
      console.log(`  Failed:        ${stats.import_jobs.failed}`);

      console.log(`\nMigration Jobs:  ${stats.migration_jobs.total} total`);
      console.log(`  Pending:       ${stats.migration_jobs.pending}`);
      console.log(`  Running:       ${stats.migration_jobs.running}`);
      console.log(`  Completed:     ${stats.migration_jobs.completed}`);
      console.log(`  Failed:        ${stats.migration_jobs.failed}`);

      console.log(`\nBackup Snapshots: ${stats.backup_snapshots.total} total`);
      console.log(`  Verified:      ${stats.backup_snapshots.verified}`);
      console.log(`  Expired:       ${stats.backup_snapshots.expired}`);

      console.log(`\nRestore Jobs:    ${stats.restore_jobs.total} total`);
      console.log(`  Pending:       ${stats.restore_jobs.pending}`);
      console.log(`  Running:       ${stats.restore_jobs.running}`);
      console.log(`  Completed:     ${stats.restore_jobs.completed}`);
      console.log(`  Failed:        ${stats.restore_jobs.failed}`);

      console.log(`\nTransform Templates: ${stats.transform_templates.total} (${stats.transform_templates.public_count} public)`);
      console.log(`Audit Entries:   ${stats.audit_entries}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Export commands
const exportCmd = program
  .command('export')
  .description('Manage export jobs');

exportCmd
  .command('list')
  .description('List export jobs')
  .option('-s, --status <status>', 'Filter by status')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const jobs = await db.listExportJobs(parseInt(options.limit, 10), 0, options.status);

      console.log('\nExport Jobs:');
      console.log('-'.repeat(80));
      if (jobs.length === 0) {
        console.log('No export jobs found');
      } else {
        jobs.forEach((j) => {
          console.log(`[${j.status.toUpperCase()}] ${j.name} (${j.export_type}, ${j.format})`);
          console.log(`  ID: ${j.id}`);
          console.log(`  Progress: ${j.progress_percentage}%`);
          console.log(`  Records: ${j.exported_records}/${j.total_records}`);
          console.log(`  Created: ${j.created_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export list failed', { error: message });
      process.exit(1);
    }
  });

exportCmd
  .command('info <id>')
  .description('Show export job details')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const job = await db.getExportJob(id);
      if (!job) {
        logger.error('Export job not found');
        process.exit(1);
      }
      console.log('\nExport Job Details:');
      console.log(JSON.stringify(job, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export info failed', { error: message });
      process.exit(1);
    }
  });

exportCmd
  .command('cancel <id>')
  .description('Cancel an export job')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const updated = await db.updateExportJobStatus(id, 'cancelled');
      if (!updated) {
        logger.error('Export job not found');
        process.exit(1);
      }
      logger.success('Export job cancelled');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export cancel failed', { error: message });
      process.exit(1);
    }
  });

exportCmd
  .command('delete <id>')
  .description('Delete an export job')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const deleted = await db.deleteExportJob(id);
      if (!deleted) {
        logger.error('Export job not found');
        process.exit(1);
      }
      logger.success('Export job deleted');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Export delete failed', { error: message });
      process.exit(1);
    }
  });

// Import commands
const importCmd = program
  .command('import')
  .description('Manage import jobs');

importCmd
  .command('list')
  .description('List import jobs')
  .option('-s, --status <status>', 'Filter by status')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const jobs = await db.listImportJobs(parseInt(options.limit, 10), 0, options.status);

      console.log('\nImport Jobs:');
      console.log('-'.repeat(80));
      if (jobs.length === 0) {
        console.log('No import jobs found');
      } else {
        jobs.forEach((j) => {
          console.log(`[${j.status.toUpperCase()}] ${j.name} (${j.import_type}, ${j.source_format})`);
          console.log(`  ID: ${j.id}`);
          console.log(`  Progress: ${j.progress_percentage}%`);
          console.log(`  Imported: ${j.imported_records}/${j.total_records} (${j.skipped_records} skipped, ${j.failed_records} failed)`);
          console.log(`  Created: ${j.created_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import list failed', { error: message });
      process.exit(1);
    }
  });

importCmd
  .command('info <id>')
  .description('Show import job details')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const job = await db.getImportJob(id);
      if (!job) {
        logger.error('Import job not found');
        process.exit(1);
      }
      console.log('\nImport Job Details:');
      console.log(JSON.stringify(job, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import info failed', { error: message });
      process.exit(1);
    }
  });

importCmd
  .command('cancel <id>')
  .description('Cancel an import job')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const updated = await db.updateImportJobStatus(id, 'cancelled');
      if (!updated) {
        logger.error('Import job not found');
        process.exit(1);
      }
      logger.success('Import job cancelled');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Import cancel failed', { error: message });
      process.exit(1);
    }
  });

// Migration commands
const migrateCmd = program
  .command('migrate')
  .description('Manage migration jobs');

migrateCmd
  .command('list')
  .description('List migration jobs')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const jobs = await db.listMigrationJobs(parseInt(options.limit, 10));

      console.log('\nMigration Jobs:');
      console.log('-'.repeat(80));
      if (jobs.length === 0) {
        console.log('No migration jobs found');
      } else {
        jobs.forEach((j) => {
          console.log(`[${j.status.toUpperCase()}] ${j.name} (${j.source_platform})`);
          console.log(`  ID: ${j.id}`);
          console.log(`  Phase: ${j.phase ?? 'N/A'}`);
          console.log(`  Progress: ${j.progress_percentage}%`);
          console.log(`  Items: ${j.migrated_items}/${j.total_items} (${j.failed_items} failed)`);
          console.log(`  Created: ${j.created_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Migration list failed', { error: message });
      process.exit(1);
    }
  });

migrateCmd
  .command('platforms')
  .description('List supported migration platforms')
  .action(() => {
    console.log('\nSupported Migration Platforms:');
    console.log('-'.repeat(40));
    console.log('  slack         - Slack');
    console.log('  discord       - Discord');
    console.log('  teams         - Microsoft Teams');
    console.log('  mattermost    - Mattermost');
    console.log('  rocket_chat   - Rocket.Chat');
    console.log('  telegram      - Telegram');
  });

migrateCmd
  .command('info <id>')
  .description('Show migration job details')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const job = await db.getMigrationJob(id);
      if (!job) {
        logger.error('Migration job not found');
        process.exit(1);
      }
      console.log('\nMigration Job Details:');
      console.log(JSON.stringify(job, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Migration info failed', { error: message });
      process.exit(1);
    }
  });

// Backup commands
const backupCmd = program
  .command('backup')
  .description('Manage backup snapshots');

backupCmd
  .command('list')
  .description('List backup snapshots')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const snapshots = await db.listBackupSnapshots(parseInt(options.limit, 10));

      console.log('\nBackup Snapshots:');
      console.log('-'.repeat(80));
      if (snapshots.length === 0) {
        console.log('No backup snapshots found');
      } else {
        snapshots.forEach((s) => {
          const verified = s.verification_status === 'verified' ? 'VERIFIED' : (s.verification_status ?? 'UNVERIFIED');
          console.log(`[${verified}] ${s.name} (${s.backup_type})`);
          console.log(`  ID: ${s.id}`);
          console.log(`  Backend: ${s.storage_backend}`);
          console.log(`  Size: ${s.total_size_bytes ?? 'N/A'} bytes`);
          console.log(`  Retention: ${s.retention_days} days`);
          console.log(`  Created: ${s.created_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup list failed', { error: message });
      process.exit(1);
    }
  });

backupCmd
  .command('info <id>')
  .description('Show backup snapshot details')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const snapshot = await db.getBackupSnapshot(id);
      if (!snapshot) {
        logger.error('Backup snapshot not found');
        process.exit(1);
      }
      console.log('\nBackup Snapshot Details:');
      console.log(JSON.stringify(snapshot, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup info failed', { error: message });
      process.exit(1);
    }
  });

backupCmd
  .command('verify <id>')
  .description('Verify backup snapshot integrity')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const updated = await db.verifyBackupSnapshot(id);
      if (!updated) {
        logger.error('Backup snapshot not found');
        process.exit(1);
      }
      logger.success('Backup snapshot verified');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup verify failed', { error: message });
      process.exit(1);
    }
  });

backupCmd
  .command('delete <id>')
  .description('Delete a backup snapshot')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const deleted = await db.deleteBackupSnapshot(id);
      if (!deleted) {
        logger.error('Backup snapshot not found');
        process.exit(1);
      }
      logger.success('Backup snapshot deleted');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup delete failed', { error: message });
      process.exit(1);
    }
  });

backupCmd
  .command('cleanup')
  .description('Remove expired backup snapshots')
  .action(async () => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const count = await db.cleanupExpiredSnapshots();
      logger.success(`Cleaned up ${count} expired snapshots`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Backup cleanup failed', { error: message });
      process.exit(1);
    }
  });

// Restore commands
const restoreCmd = program
  .command('restore')
  .description('Manage restore jobs');

restoreCmd
  .command('list')
  .description('List restore jobs')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const jobs = await db.listRestoreJobs(parseInt(options.limit, 10));

      console.log('\nRestore Jobs:');
      console.log('-'.repeat(80));
      if (jobs.length === 0) {
        console.log('No restore jobs found');
      } else {
        jobs.forEach((j) => {
          console.log(`[${j.status.toUpperCase()}] ${j.restore_type} restore from ${j.snapshot_id}`);
          console.log(`  ID: ${j.id}`);
          console.log(`  Progress: ${j.progress_percentage}%`);
          console.log(`  Items: ${j.restored_items}/${j.total_items} (${j.failed_items} failed)`);
          console.log(`  Created: ${j.created_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Restore list failed', { error: message });
      process.exit(1);
    }
  });

restoreCmd
  .command('info <id>')
  .description('Show restore job details')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const job = await db.getRestoreJob(id);
      if (!job) {
        logger.error('Restore job not found');
        process.exit(1);
      }
      console.log('\nRestore Job Details:');
      console.log(JSON.stringify(job, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Restore info failed', { error: message });
      process.exit(1);
    }
  });

// Audit commands
const auditCmd = program
  .command('audit')
  .description('View data transfer audit log');

auditCmd
  .command('list')
  .description('List audit entries')
  .option('-t, --type <type>', 'Filter by job type (export, import, migration, backup, restore)')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const entries = await db.listAuditEntries(parseInt(options.limit, 10), 0, {
        job_type: options.type,
      });

      console.log('\nAudit Log:');
      console.log('-'.repeat(80));
      if (entries.length === 0) {
        console.log('No audit entries found');
      } else {
        entries.forEach((e) => {
          console.log(`[${e.job_type.toUpperCase()}] ${e.action} by ${e.user_id}`);
          console.log(`  Job ID: ${e.job_id}`);
          if (e.records_affected) console.log(`  Records Affected: ${e.records_affected}`);
          if (e.data_size_bytes) console.log(`  Data Size: ${e.data_size_bytes} bytes`);
          console.log(`  Created: ${e.created_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Audit list failed', { error: message });
      process.exit(1);
    }
  });

// Transform commands
const transformCmd = program
  .command('transform')
  .description('Manage data transformation templates');

transformCmd
  .command('list')
  .description('List transform templates')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const templates = await db.listTransformTemplates(parseInt(options.limit, 10));

      console.log('\nTransform Templates:');
      console.log('-'.repeat(80));
      if (templates.length === 0) {
        console.log('No transform templates found');
      } else {
        templates.forEach((t) => {
          const pub = t.is_public ? 'PUBLIC' : 'PRIVATE';
          console.log(`[${pub}] ${t.name} (${t.source_format} -> ${t.target_format})`);
          console.log(`  ID: ${t.id}`);
          console.log(`  Usage: ${t.usage_count} times`);
          if (t.description) console.log(`  ${t.description}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Transform list failed', { error: message });
      process.exit(1);
    }
  });

transformCmd
  .command('delete <id>')
  .description('Delete a transform template')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new ExportImportDatabase();
      await db.connect();

      const deleted = await db.deleteTransformTemplate(id);
      if (!deleted) {
        logger.error('Transform template not found');
        process.exit(1);
      }
      logger.success('Transform template deleted');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Transform delete failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
