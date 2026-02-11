#!/usr/bin/env node
/**
 * Compliance Plugin CLI
 * Command-line interface for GDPR/CCPA compliance management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ComplianceDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('compliance:cli');

const program = new Command();

program
  .name('nself-compliance')
  .description('Compliance plugin for nself - GDPR/CCPA compliance, DSARs, consent, retention, breach management')
  .version('1.0.0');

// =========================================================================
// Init command
// =========================================================================

program
  .command('init')
  .description('Initialize compliance plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing compliance schema...');

      const db = new ComplianceDatabase();
      await db.connect();
      await db.initializeSchema();

      console.log('Done - Compliance schema initialized successfully');

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Server command
// =========================================================================

program
  .command('server')
  .description('Start compliance plugin server')
  .option('-p, --port <port>', 'Server port', '3706')
  .action(async (options) => {
    try {
      logger.info('Starting compliance server...');
      await startServer({ port: parseInt(options.port, 10) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Status command
// =========================================================================

program
  .command('status')
  .description('Show compliance plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new ComplianceDatabase();
      await db.connect();

      const dsars = await db.listDsars({ limit: 1 });
      const retentionPolicies = await db.listRetentionPolicies();
      const breaches = await db.listBreaches();
      const activePolicy = await db.getActivePrivacyPolicy();

      console.log('\nCompliance Plugin Status');
      console.log('========================');
      console.log(`GDPR Enabled:             ${config.gdprEnabled}`);
      console.log(`CCPA Enabled:             ${config.ccpaEnabled}`);
      console.log(`DSAR Deadline (days):     ${config.dsarDeadlineDays}`);
      console.log(`Breach Notification (hrs): ${config.breachNotificationHours}`);
      console.log(`Retention Enabled:         ${config.retentionEnabled}`);
      console.log(`Audit Enabled:             ${config.auditEnabled}`);

      console.log('\nStatistics:');
      console.log(`Total DSARs:              ${dsars.total}`);
      console.log(`Retention Policies:       ${retentionPolicies.length}`);
      console.log(`Active Breaches:          ${breaches.filter(b => b.status !== 'resolved').length}`);
      console.log(`Active Privacy Policy:    ${activePolicy ? activePolicy.version : 'None'}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// DSAR commands
// =========================================================================

const dsarCmd = program
  .command('dsar')
  .description('Manage Data Subject Access Requests');

dsarCmd
  .command('create')
  .description('Create a new DSAR')
  .requiredOption('-e, --email <email>', 'Requester email')
  .requiredOption('-t, --type <type>', 'Request type (access, erasure, portability, rectification, restriction, objection, ccpa_disclosure, ccpa_deletion, ccpa_opt_out)')
  .option('-n, --name <name>', 'Requester name')
  .option('-d, --description <description>', 'Request description')
  .option('-u, --user-id <userId>', 'Associated user ID')
  .option('-c, --categories <categories>', 'Data categories (comma-separated)')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new ComplianceDatabase();
      await db.connect();

      const deadlineDays = options.type.startsWith('ccpa_')
        ? config.ccpaDeadlineDays
        : config.dsarDeadlineDays;

      const dsar = await db.createDsar({
        request_type: options.type,
        email: options.email,
        name: options.name,
        user_id: options.userId,
        description: options.description,
        data_categories: options.categories?.split(',').map((c: string) => c.trim()),
      }, deadlineDays);

      console.log('\nDSAR Created:');
      console.log(`  ID:              ${dsar.id}`);
      console.log(`  Request Number:  ${dsar.request_number}`);
      console.log(`  Type:            ${dsar.request_type}`);
      console.log(`  Status:          ${dsar.status}`);
      console.log(`  Deadline:        ${dsar.deadline}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('DSAR creation failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

dsarCmd
  .command('list')
  .description('List DSARs')
  .option('-s, --status <status>', 'Filter by status')
  .option('-u, --user-id <userId>', 'Filter by user ID')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const result = await db.listDsars({
        status: options.status,
        user_id: options.userId,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nDSARs (${result.total} total):`);
      console.log('==========================================');

      if (result.dsars.length === 0) {
        console.log('No DSARs found.');
      } else {
        for (const dsar of result.dsars) {
          const deadline = new Date(dsar.deadline);
          const isOverdue = deadline < new Date();
          const statusIndicator = isOverdue ? '[OVERDUE]' : '';
          console.log(`\n  ${dsar.request_number} (${dsar.request_type}) ${statusIndicator}`);
          console.log(`    Status:   ${dsar.status}`);
          console.log(`    Email:    ${dsar.requester_email}`);
          console.log(`    Deadline: ${deadline.toISOString()}`);
          console.log(`    Created:  ${dsar.created_at}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('DSAR list failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

dsarCmd
  .command('process')
  .description('Approve or reject a DSAR')
  .requiredOption('-i, --id <id>', 'DSAR ID')
  .requiredOption('-a, --action <action>', 'Action: approve or reject')
  .option('-n, --notes <notes>', 'Resolution notes')
  .option('-r, --reason <reason>', 'Rejection reason (for reject)')
  .option('--assign-to <assignTo>', 'Assign to user ID')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const dsar = await db.processDsar(options.id, {
        action: options.action,
        notes: options.notes,
        rejection_reason: options.reason,
        assigned_to: options.assignTo,
      });

      if (!dsar) {
        console.error('Error: DSAR not found');
        process.exit(1);
      }

      console.log(`\nDSAR ${options.action === 'approve' ? 'approved' : 'rejected'}:`);
      console.log(`  ID:     ${dsar.id}`);
      console.log(`  Number: ${dsar.request_number}`);
      console.log(`  Status: ${dsar.status}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('DSAR processing failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

dsarCmd
  .command('complete')
  .description('Mark a DSAR as completed')
  .requiredOption('-i, --id <id>', 'DSAR ID')
  .option('-u, --url <url>', 'Data package URL')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const dsar = await db.completeDsar(options.id, options.url);

      if (!dsar) {
        console.error('Error: DSAR not found');
        process.exit(1);
      }

      console.log(`\nDSAR completed:`);
      console.log(`  ID:        ${dsar.id}`);
      console.log(`  Number:    ${dsar.request_number}`);
      console.log(`  Completed: ${dsar.completed_at}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('DSAR completion failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

dsarCmd
  .command('export')
  .description('Export data for a DSAR')
  .requiredOption('-i, --id <id>', 'DSAR ID')
  .option('-f, --format <format>', 'Export format (json, csv)', 'json')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const dsar = await db.getDsar(options.id);
      if (!dsar) {
        console.error('Error: DSAR not found');
        process.exit(1);
      }

      if (dsar.user_id) {
        const data = await db.exportUserData(dsar.user_id);
        console.log(JSON.stringify(data, null, 2));
      } else {
        console.log('No user ID associated with this DSAR. Cannot export data.');
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('DSAR export failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Consent commands
// =========================================================================

const consentCmd = program
  .command('consent')
  .description('Manage user consent records');

consentCmd
  .command('grant')
  .description('Grant consent for a user')
  .requiredOption('-u, --user-id <userId>', 'User ID')
  .requiredOption('-p, --purpose <purpose>', 'Consent purpose')
  .option('-d, --description <description>', 'Purpose description')
  .option('-t, --text <text>', 'Consent text')
  .option('-m, --method <method>', 'Consent method (explicit, implicit, opt_in, opt_out)')
  .option('-v, --policy-version <version>', 'Privacy policy version')
  .option('-e, --expires <expiresAt>', 'Expiry date (ISO format)')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const consent = await db.createConsent({
        user_id: options.userId,
        purpose: options.purpose,
        status: 'granted',
        purpose_description: options.description,
        consent_text: options.text,
        consent_method: options.method,
        privacy_policy_version: options.policyVersion,
        expires_at: options.expires,
      });

      console.log('\nConsent granted:');
      console.log(`  ID:       ${consent.id}`);
      console.log(`  User:     ${consent.user_id}`);
      console.log(`  Purpose:  ${consent.purpose}`);
      console.log(`  Status:   ${consent.status}`);
      console.log(`  Granted:  ${consent.granted_at}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Consent grant failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

consentCmd
  .command('withdraw')
  .description('Withdraw consent')
  .requiredOption('-i, --id <id>', 'Consent ID')
  .option('-r, --reason <reason>', 'Withdrawal reason')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const consent = await db.withdrawConsent(options.id, options.reason);
      if (!consent) {
        console.error('Error: Consent record not found');
        process.exit(1);
      }

      console.log('\nConsent withdrawn:');
      console.log(`  ID:       ${consent.id}`);
      console.log(`  User:     ${consent.user_id}`);
      console.log(`  Purpose:  ${consent.purpose}`);
      console.log(`  Status:   ${consent.status}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Consent withdrawal failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

consentCmd
  .command('list')
  .description('List consent records')
  .option('-u, --user-id <userId>', 'Filter by user ID')
  .option('-p, --purpose <purpose>', 'Filter by purpose')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const consents = await db.listConsents({
        user_id: options.userId,
        purpose: options.purpose,
      });

      console.log(`\nConsent Records (${consents.length}):`);
      console.log('==========================================');

      if (consents.length === 0) {
        console.log('No consent records found.');
      } else {
        for (const consent of consents) {
          const isValid = consent.status === 'granted' &&
            (!consent.expires_at || new Date(consent.expires_at) > new Date());
          const indicator = isValid ? '[VALID]' : '[INVALID]';
          console.log(`\n  ${consent.purpose} ${indicator}`);
          console.log(`    ID:      ${consent.id}`);
          console.log(`    User:    ${consent.user_id}`);
          console.log(`    Status:  ${consent.status}`);
          console.log(`    Granted: ${consent.granted_at ?? 'N/A'}`);
          console.log(`    Expires: ${consent.expires_at ?? 'Never'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Consent list failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

consentCmd
  .command('check')
  .description('Check if a user has valid consent for a purpose')
  .requiredOption('-u, --user-id <userId>', 'User ID')
  .requiredOption('-p, --purpose <purpose>', 'Consent purpose')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const hasConsent = await db.checkUserConsent(options.userId, options.purpose);

      console.log(`\nConsent Check:`);
      console.log(`  User:     ${options.userId}`);
      console.log(`  Purpose:  ${options.purpose}`);
      console.log(`  Valid:    ${hasConsent ? 'Yes' : 'No'}`);

      await db.disconnect();
      process.exit(hasConsent ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Consent check failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Retention commands
// =========================================================================

const retentionCmd = program
  .command('retention')
  .description('Manage data retention policies');

retentionCmd
  .command('create')
  .description('Create a retention policy')
  .requiredOption('-n, --name <name>', 'Policy name')
  .requiredOption('-c, --category <category>', 'Data category')
  .requiredOption('-d, --days <days>', 'Retention period in days')
  .requiredOption('-a, --action <action>', 'Retention action (delete, anonymize, archive, notify)')
  .option('--description <description>', 'Policy description')
  .option('--table <table>', 'Target table name')
  .option('--legal-basis <basis>', 'Legal basis')
  .option('--regulation <regulation>', 'Regulation (GDPR, CCPA)')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const policy = await db.createRetentionPolicy({
        name: options.name,
        description: options.description,
        data_category: options.category,
        table_name: options.table,
        retention_days: parseInt(options.days, 10),
        retention_action: options.action,
        legal_basis: options.legalBasis,
        regulation: options.regulation,
      });

      console.log('\nRetention Policy Created:');
      console.log(`  ID:       ${policy.id}`);
      console.log(`  Name:     ${policy.name}`);
      console.log(`  Category: ${policy.data_category}`);
      console.log(`  Days:     ${policy.retention_days}`);
      console.log(`  Action:   ${policy.retention_action}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Retention policy creation failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

retentionCmd
  .command('list')
  .description('List retention policies')
  .option('-e, --enabled-only', 'Show only enabled policies')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const policies = await db.listRetentionPolicies(options.enabledOnly);

      console.log(`\nRetention Policies (${policies.length}):`);
      console.log('==========================================');

      if (policies.length === 0) {
        console.log('No retention policies found.');
      } else {
        for (const policy of policies) {
          const status = policy.is_enabled ? 'ENABLED' : 'DISABLED';
          console.log(`\n  ${policy.name} [${status}]`);
          console.log(`    ID:       ${policy.id}`);
          console.log(`    Category: ${policy.data_category}`);
          console.log(`    Days:     ${policy.retention_days}`);
          console.log(`    Action:   ${policy.retention_action}`);
          console.log(`    Priority: ${policy.priority}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Retention list failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

retentionCmd
  .command('execute')
  .description('Execute a retention policy')
  .requiredOption('-i, --id <policyId>', 'Policy ID')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      console.log('Executing retention policy...');

      const execution = await db.executeRetentionPolicy(options.id);

      console.log('\nRetention Execution Complete:');
      console.log(`  Execution ID:       ${execution.id}`);
      console.log(`  Status:             ${execution.status}`);
      console.log(`  Records Processed:  ${execution.records_processed}`);
      console.log(`  Records Deleted:    ${execution.records_deleted}`);
      console.log(`  Records Anonymized: ${execution.records_anonymized}`);
      console.log(`  Records Archived:   ${execution.records_archived}`);
      console.log(`  Time:               ${execution.execution_time_ms ?? 0}ms`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Retention execution failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

retentionCmd
  .command('report')
  .description('Show retention execution report')
  .option('-i, --id <policyId>', 'Policy ID (show all if not specified)')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      if (options.id) {
        const executions = await db.getRetentionExecutions(options.id, parseInt(options.limit, 10));

        console.log(`\nRetention Executions for policy ${options.id} (${executions.length}):`);
        console.log('==========================================');

        for (const exec of executions) {
          console.log(`\n  ${exec.id}`);
          console.log(`    Status:     ${exec.status}`);
          console.log(`    Processed:  ${exec.records_processed}`);
          console.log(`    Deleted:    ${exec.records_deleted}`);
          console.log(`    Anonymized: ${exec.records_anonymized}`);
          console.log(`    Archived:   ${exec.records_archived}`);
          console.log(`    Time:       ${exec.execution_time_ms ?? 0}ms`);
          console.log(`    Executed:   ${exec.executed_at}`);
        }
      } else {
        const policies = await db.listRetentionPolicies(true);
        console.log(`\nRetention Report (${policies.length} enabled policies):`);
        console.log('==========================================');

        for (const policy of policies) {
          const executions = await db.getRetentionExecutions(policy.id, 1);
          const lastExec = executions[0];
          console.log(`\n  ${policy.name} (${policy.data_category})`);
          console.log(`    Retention: ${policy.retention_days} days / ${policy.retention_action}`);
          console.log(`    Last Run:  ${lastExec ? lastExec.executed_at : 'Never'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Retention report failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Breach commands
// =========================================================================

const breachCmd = program
  .command('breach')
  .description('Manage data breach incidents');

breachCmd
  .command('create')
  .description('Report a new data breach')
  .requiredOption('-t, --title <title>', 'Breach title')
  .requiredOption('-d, --description <description>', 'Breach description')
  .requiredOption('-s, --severity <severity>', 'Severity (low, medium, high, critical)')
  .requiredOption('-c, --categories <categories>', 'Data categories affected (comma-separated)')
  .option('--discovered-by <discoveredBy>', 'Discovered by user ID')
  .option('--affected-users <count>', 'Number of affected users')
  .option('--data-description <desc>', 'Description of data involved')
  .option('--no-notification', 'Notification not required')
  .action(async (options) => {
    try {
      const config = loadConfig();
      const db = new ComplianceDatabase();
      await db.connect();

      const breach = await db.createBreach({
        title: options.title,
        description: options.description,
        severity: options.severity,
        data_categories: options.categories.split(',').map((c: string) => c.trim()),
        discovered_by: options.discoveredBy,
        affected_users_count: options.affectedUsers ? parseInt(options.affectedUsers, 10) : undefined,
        data_description: options.dataDescription,
        notification_required: options.notification !== false,
      }, config.breachNotificationHours);

      console.log('\nData Breach Reported:');
      console.log(`  ID:                   ${breach.id}`);
      console.log(`  Breach Number:        ${breach.breach_number}`);
      console.log(`  Severity:             ${breach.severity}`);
      console.log(`  Status:               ${breach.status}`);
      console.log(`  Notification Required: ${breach.notification_required}`);
      console.log(`  Notification Deadline: ${breach.notification_deadline}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Breach creation failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

breachCmd
  .command('list')
  .description('List data breaches')
  .option('-s, --status <status>', 'Filter by status')
  .option('--severity <severity>', 'Filter by severity')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const breaches = await db.listBreaches({
        status: options.status,
        severity: options.severity,
      });

      console.log(`\nData Breaches (${breaches.length}):`);
      console.log('==========================================');

      if (breaches.length === 0) {
        console.log('No data breaches found.');
      } else {
        for (const breach of breaches) {
          const needsNotification = breach.notification_required &&
            !breach.authority_notified_at &&
            breach.notification_deadline &&
            new Date(breach.notification_deadline) > new Date();
          const urgent = needsNotification ? '[NEEDS NOTIFICATION]' : '';

          console.log(`\n  ${breach.breach_number} - ${breach.title} ${urgent}`);
          console.log(`    Severity:   ${breach.severity}`);
          console.log(`    Status:     ${breach.status}`);
          console.log(`    Affected:   ${breach.affected_users_count ?? 'Unknown'} users`);
          console.log(`    Discovered: ${breach.discovered_at}`);
          if (breach.notification_deadline) {
            console.log(`    Deadline:   ${breach.notification_deadline}`);
          }
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Breach list failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

breachCmd
  .command('notify')
  .description('Send breach notification')
  .requiredOption('-i, --id <id>', 'Breach ID')
  .requiredOption('-t, --type <type>', 'Notification type (authority, user, media)')
  .requiredOption('--recipient-type <recipientType>', 'Recipient type')
  .option('-e, --email <email>', 'Recipient email')
  .option('-s, --subject <subject>', 'Notification subject')
  .option('-m, --message <message>', 'Notification message')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const notification = await db.addBreachNotification(
        options.id,
        options.type,
        options.recipientType,
        options.email,
        options.subject,
        options.message
      );

      console.log('\nBreach Notification Sent:');
      console.log(`  Notification ID: ${notification.id}`);
      console.log(`  Type:            ${notification.notification_type}`);
      console.log(`  Recipient:       ${notification.recipient_type}`);
      console.log(`  Sent At:         ${notification.sent_at}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Breach notification failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Policy commands
// =========================================================================

const policyCmd = program
  .command('policy')
  .description('Manage privacy policies');

policyCmd
  .command('create')
  .description('Create a new privacy policy version')
  .requiredOption('-v, --version <version>', 'Policy version (e.g., 2.0.0)')
  .requiredOption('-n, --version-number <number>', 'Version number (integer)')
  .requiredOption('-t, --title <title>', 'Policy title')
  .requiredOption('-c, --content <content>', 'Policy content')
  .requiredOption('-e, --effective-from <date>', 'Effective from date (ISO format)')
  .option('-s, --summary <summary>', 'Policy summary')
  .option('--changes <changes>', 'Changes summary from previous version')
  .option('-r, --reacceptance', 'Requires re-acceptance')
  .option('-l, --language <language>', 'Language code', 'en')
  .option('-j, --jurisdiction <jurisdiction>', 'Jurisdiction')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const policy = await db.createPrivacyPolicy({
        version: options.version,
        version_number: parseInt(options.versionNumber, 10),
        title: options.title,
        content: options.content,
        summary: options.summary,
        changes_summary: options.changes,
        requires_reacceptance: options.reacceptance ?? false,
        effective_from: options.effectiveFrom,
        language: options.language,
        jurisdiction: options.jurisdiction,
      });

      console.log('\nPrivacy Policy Created:');
      console.log(`  ID:       ${policy.id}`);
      console.log(`  Version:  ${policy.version}`);
      console.log(`  Title:    ${policy.title}`);
      console.log(`  Active:   ${policy.is_active}`);
      console.log(`  From:     ${policy.effective_from}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Policy creation failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

policyCmd
  .command('publish')
  .description('Publish a privacy policy (makes it active)')
  .requiredOption('-i, --id <id>', 'Policy ID')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const policy = await db.publishPrivacyPolicy(options.id);
      if (!policy) {
        console.error('Error: Privacy policy not found');
        process.exit(1);
      }

      console.log(`\nPrivacy Policy Published:`);
      console.log(`  ID:      ${policy.id}`);
      console.log(`  Version: ${policy.version}`);
      console.log(`  Active:  ${policy.is_active}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Policy publish failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

policyCmd
  .command('current')
  .description('Show current active privacy policy')
  .action(async () => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const policy = await db.getActivePrivacyPolicy();
      if (!policy) {
        console.log('No active privacy policy found.');
        process.exit(0);
      }

      console.log('\nActive Privacy Policy:');
      console.log(`  ID:       ${policy.id}`);
      console.log(`  Version:  ${policy.version}`);
      console.log(`  Title:    ${policy.title}`);
      console.log(`  From:     ${policy.effective_from}`);
      console.log(`  Language: ${policy.language}`);
      if (policy.summary) {
        console.log(`  Summary:  ${policy.summary}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Policy lookup failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Audit commands
// =========================================================================

const auditCmd = program
  .command('audit')
  .description('View compliance audit logs');

auditCmd
  .command('list')
  .description('List audit log entries')
  .option('-c, --category <category>', 'Filter by event category (dsar, consent, retention, breach, policy)')
  .option('-a, --actor <actorId>', 'Filter by actor ID')
  .option('-s, --subject <subjectId>', 'Filter by data subject ID')
  .option('-l, --limit <limit>', 'Limit results', '50')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const result = await db.listAuditLogs({
        event_category: options.category,
        actor_id: options.actor,
        data_subject_id: options.subject,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nCompliance Audit Log (${result.total} total, showing ${result.logs.length}):`);
      console.log('==========================================');

      if (result.logs.length === 0) {
        console.log('No audit log entries found.');
      } else {
        for (const log of result.logs) {
          console.log(`\n  ${log.event_type} [${log.event_category}]`);
          console.log(`    ID:        ${log.id}`);
          console.log(`    Actor:     ${log.actor_id ?? 'system'} (${log.actor_type})`);
          if (log.target_type) {
            console.log(`    Target:    ${log.target_type}/${log.target_id}`);
          }
          if (log.data_subject_id) {
            console.log(`    Subject:   ${log.data_subject_id}`);
          }
          console.log(`    Created:   ${log.created_at}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Audit list failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

auditCmd
  .command('export')
  .description('Export audit logs as JSON')
  .option('-c, --category <category>', 'Filter by event category')
  .option('-a, --actor <actorId>', 'Filter by actor ID')
  .option('-s, --subject <subjectId>', 'Filter by data subject ID')
  .option('-l, --limit <limit>', 'Limit results', '1000')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const result = await db.listAuditLogs({
        event_category: options.category,
        actor_id: options.actor,
        data_subject_id: options.subject,
        limit: parseInt(options.limit, 10),
      });

      console.log(JSON.stringify({
        exported_at: new Date().toISOString(),
        total: result.total,
        count: result.logs.length,
        logs: result.logs,
      }, null, 2));

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Audit export failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Data export command
// =========================================================================

program
  .command('export')
  .description('Export user data for compliance')
  .requiredOption('-u, --user-id <userId>', 'User ID')
  .option('-c, --categories <categories>', 'Data categories (comma-separated)')
  .option('-f, --format <format>', 'Export format (json, csv)', 'json')
  .action(async (options) => {
    try {
      const db = new ComplianceDatabase();
      await db.connect();

      const categories = options.categories?.split(',').map((c: string) => c.trim());
      const data = await db.exportUserData(options.userId, categories);

      console.log(JSON.stringify(data, null, 2));

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Data export failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

program.parse();
