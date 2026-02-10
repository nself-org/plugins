#!/usr/bin/env node
/**
 * Audit Plugin CLI
 * Command-line interface for audit operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { AuditDatabase } from './database.js';
import { AuditService } from './services.js';

const logger = createLogger('audit:cli');
const program = new Command();

program
  .name('nself-audit')
  .description('Audit plugin CLI for nself')
  .version('1.0.0');

/**
 * Initialize database
 */
program
  .command('init')
  .description('Initialize audit database schema with immutability triggers')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db);

      await auditDb.initSchema();

      // Verify triggers
      const triggersValid = await auditDb.verifyImmutabilityTriggers();
      if (triggersValid) {
        logger.success('Immutability triggers verified');
      } else {
        logger.warn('Immutability triggers not found - events may not be protected');
      }

      logger.success('Audit plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize audit plugin', { error: message });
      process.exit(1);
    }
  });

/**
 * Start server
 */
program
  .command('server')
  .description('Start audit HTTP server')
  .action(async () => {
    try {
      logger.info('Starting audit server...');
      await import('./server.js');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start audit server', { error: message });
      process.exit(1);
    }
  });

/**
 * Log an event
 */
program
  .command('log')
  .description('Append an audit event')
  .requiredOption('--plugin <plugin>', 'Source plugin name')
  .requiredOption('--event-type <type>', 'Event type')
  .requiredOption('--action <action>', 'Action performed')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .option('--actor-id <actorId>', 'Actor ID')
  .option('--actor-type <actorType>', 'Actor type')
  .option('--resource-type <resourceType>', 'Resource type')
  .option('--resource-id <resourceId>', 'Resource ID')
  .option('--outcome <outcome>', 'Outcome (success|failure|unknown)', 'success')
  .option('--severity <severity>', 'Severity (low|medium|high|critical)', 'low')
  .option('--ip <ip>', 'IP address')
  .option('--user-agent <userAgent>', 'User agent')
  .option('--location <location>', 'Location')
  .option('--details <details>', 'Details JSON string')
  .option('--metadata <metadata>', 'Metadata JSON string')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db).forApp(options.appId);

      const event = await auditDb.insertEvent({
        sourcePlugin: options.plugin,
        eventType: options.eventType,
        action: options.action,
        actorId: options.actorId,
        actorType: options.actorType,
        resourceType: options.resourceType,
        resourceId: options.resourceId,
        outcome: options.outcome,
        severity: options.severity,
        ipAddress: options.ip,
        userAgent: options.userAgent,
        location: options.location,
        details: options.details ? JSON.parse(options.details) : {},
        metadata: options.metadata ? JSON.parse(options.metadata) : {},
      });

      logger.success('Event logged successfully', {
        eventId: event.id,
        checksum: event.checksum,
      });

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to log event', { error: message });
      process.exit(1);
    }
  });

/**
 * Query events
 */
program
  .command('query')
  .description('Query audit events')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .option('--plugin <plugin>', 'Filter by source plugin')
  .option('--event-type <type>', 'Filter by event type')
  .option('--actor-id <actorId>', 'Filter by actor ID')
  .option('--resource-type <resourceType>', 'Filter by resource type')
  .option('--resource-id <resourceId>', 'Filter by resource ID')
  .option('--action <action>', 'Filter by action')
  .option('--outcome <outcome>', 'Filter by outcome')
  .option('--severity <severity>', 'Filter by severity')
  .option('--start-date <startDate>', 'Start date (ISO 8601)')
  .option('--end-date <endDate>', 'End date (ISO 8601)')
  .option('--limit <limit>', 'Limit results', '100')
  .option('--offset <offset>', 'Offset results', '0')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db).forApp(options.appId);

      const result = await auditDb.queryEvents({
        sourcePlugin: options.plugin,
        eventType: options.eventType,
        actorId: options.actorId,
        resourceType: options.resourceType,
        resourceId: options.resourceId,
        action: options.action,
        outcome: options.outcome,
        severity: options.severity,
        startDate: options.startDate,
        endDate: options.endDate,
        limit: parseInt(options.limit),
        offset: parseInt(options.offset),
      });

      console.log(JSON.stringify(result, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to query events', { error: message });
      process.exit(1);
    }
  });

/**
 * Export events
 */
program
  .command('export')
  .description('Export audit events')
  .requiredOption('--format <format>', 'Export format (csv|json|jsonl|cef|leef|syslog)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .option('--plugin <plugin>', 'Filter by source plugin')
  .option('--event-type <type>', 'Filter by event type')
  .option('--start-date <startDate>', 'Start date (ISO 8601)')
  .option('--end-date <endDate>', 'End date (ISO 8601)')
  .option('--limit <limit>', 'Limit results', '10000')
  .option('--output <output>', 'Output file (default: stdout)')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db).forApp(options.appId);
      const service = new AuditService(auditDb);

      const result = await auditDb.queryEvents({
        sourcePlugin: options.plugin,
        eventType: options.eventType,
        startDate: options.startDate,
        endDate: options.endDate,
        limit: Math.min(parseInt(options.limit), config.export.maxRows),
      });

      const exportData = await service.exportEvents(result.events, options.format);

      if (options.output) {
        const fs = await import('fs/promises');
        await fs.writeFile(options.output, exportData);
        logger.success(`Exported ${result.events.length} events to ${options.output}`);
      } else {
        console.log(exportData);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to export events', { error: message });
      process.exit(1);
    }
  });

/**
 * Verify event
 */
program
  .command('verify')
  .description('Verify event integrity using checksum')
  .requiredOption('--event-id <eventId>', 'Event ID to verify')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db).forApp(options.appId);

      const result = await auditDb.verifyEventChecksum(options.eventId);

      if (result.valid) {
        logger.success('Event integrity verified', {
          eventId: options.eventId,
          checksum: result.expectedChecksum,
        });
      } else {
        logger.error('Event integrity check FAILED - possible tampering detected', {
          eventId: options.eventId,
          expectedChecksum: result.expectedChecksum,
          actualChecksum: result.actualChecksum,
        });
        process.exit(1);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to verify event', { error: message });
      process.exit(1);
    }
  });

/**
 * Compliance report
 */
program
  .command('compliance')
  .description('Generate compliance report')
  .requiredOption('--framework <framework>', 'Compliance framework (SOC2|HIPAA|GDPR|PCI)')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .option('--start-date <startDate>', 'Start date (ISO 8601)')
  .option('--end-date <endDate>', 'End date (ISO 8601)')
  .option('--output <output>', 'Output file (default: stdout)')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db).forApp(options.appId);
      const service = new AuditService(auditDb);

      const startDate = options.startDate ? new Date(options.startDate) : new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);
      const endDate = options.endDate ? new Date(options.endDate) : new Date();

      const report = await service.generateComplianceReport(options.framework, startDate, endDate);

      const reportJson = JSON.stringify(report, null, 2);

      if (options.output) {
        const fs = await import('fs/promises');
        await fs.writeFile(options.output, reportJson);
        logger.success(`Compliance report generated: ${options.output}`);
      } else {
        console.log(reportJson);
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate compliance report', { error: message });
      process.exit(1);
    }
  });

/**
 * Statistics
 */
program
  .command('stats')
  .description('Show audit plugin statistics')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const auditDb = new AuditDatabase(db).forApp(options.appId);

      const stats = await auditDb.getStats();

      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get statistics', { error: message });
      process.exit(1);
    }
  });

program.parse();
