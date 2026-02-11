#!/usr/bin/env node
/**
 * Invitations Plugin CLI
 * Command-line interface for the invitations plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { InvitationsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('invitations:cli');

const program = new Command();

function sourceAccountLabel(record: Record<string, unknown>): string {
  const value = record.source_account_id;
  if (typeof value === 'string' && value.length > 0) {
    return value;
  }
  return 'primary';
}

program
  .name('nself-invitations')
  .description('Invitations plugin for nself - manage invitation lifecycle')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new InvitationsDatabase();
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
  .option('-p, --port <port>', 'Server port', '3402')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting invitations server on ${config.host}:${config.port}`);

      const server = await createServer(config);
      await server.listen({ port: config.port, host: config.host });
      logger.success(`Server listening on ${config.host}:${config.port}`);

      // Graceful shutdown
      process.on('SIGTERM', async () => {
        logger.info('SIGTERM received, shutting down gracefully...');
        await server.close();
        process.exit(0);
      });

      process.on('SIGINT', async () => {
        logger.info('SIGINT received, shutting down gracefully...');
        await server.close();
        process.exit(0);
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show invitation statistics')
  .action(async () => {
    try {
      const db = new InvitationsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nInvitations Plugin Status');
      console.log('=========================');
      console.log(`Total:       ${stats.total}`);
      console.log(`Pending:     ${stats.pending}`);
      console.log(`Sent:        ${stats.sent}`);
      console.log(`Delivered:   ${stats.delivered}`);
      console.log(`Accepted:    ${stats.accepted}`);
      console.log(`Declined:    ${stats.declined}`);
      console.log(`Expired:     ${stats.expired}`);
      console.log(`Revoked:     ${stats.revoked}`);
      console.log(`Conversion:  ${stats.conversionRate.toFixed(2)}%`);

      console.log('\nBy Type:');
      for (const [type, count] of Object.entries(stats.byType)) {
        console.log(`  ${type}: ${count}`);
      }

      console.log('\nBy Channel:');
      for (const [channel, count] of Object.entries(stats.byChannel)) {
        console.log(`  ${channel}: ${count}`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Create command
program
  .command('create')
  .description('Create a new invitation')
  .requiredOption('-i, --inviter-id <id>', 'Inviter user ID')
  .option('-t, --type <type>', 'Invitation type', 'app_signup')
  .option('-e, --email <email>', 'Invitee email address')
  .option('-p, --phone <phone>', 'Invitee phone number')
  .option('-n, --name <name>', 'Invitee name')
  .option('-c, --channel <channel>', 'Invitation channel (email, sms, link)', 'email')
  .option('-m, --message <message>', 'Custom message')
  .option('-r, --role <role>', 'Role to assign')
  .option('--expires-in <hours>', 'Expiry time in hours', '168')
  .action(async (options) => {
    try {
      if (!options.email && !options.phone && options.channel !== 'link') {
        logger.error('Email or phone required unless channel is link');
        process.exit(1);
      }

      const config = loadConfig();
      const db = new InvitationsDatabase();
      await db.connect();

      const code = require('crypto').randomBytes(config.codeLength).toString('base64url').substring(0, config.codeLength);
      const expiresAt = new Date();
      expiresAt.setHours(expiresAt.getHours() + parseInt(options.expiresIn, 10));

      const id = await db.createInvitation({
        type: options.type,
        inviter_id: options.inviterId,
        invitee_email: options.email ?? null,
        invitee_phone: options.phone ?? null,
        invitee_name: options.name ?? null,
        code,
        status: 'pending',
        channel: options.channel,
        message: options.message ?? null,
        role: options.role ?? null,
        resource_type: null,
        resource_id: null,
        expires_at: expiresAt,
        sent_at: null,
        delivered_at: null,
        accepted_at: null,
        accepted_by: null,
        declined_at: null,
        revoked_at: null,
        metadata: {},
      });

      const inviteUrl = config.acceptUrlTemplate.replace('{{code}}', code);

      console.log('\nInvitation Created');
      console.log('==================');
      console.log(`ID:         ${id}`);
      console.log(`Code:       ${code}`);
      console.log(`URL:        ${inviteUrl}`);
      console.log(`Type:       ${options.type}`);
      console.log(`Channel:    ${options.channel}`);
      console.log(`Expires:    ${expiresAt.toISOString()}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Create failed', { error: message });
      process.exit(1);
    }
  });

// List command
program
  .command('list')
  .description('List invitations')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .option('-t, --type <type>', 'Filter by type')
  .option('-s, --status <status>', 'Filter by status')
  .action(async (options) => {
    try {
      const db = new InvitationsDatabase();
      await db.connect();

      const invitations = await db.listInvitations(
        parseInt(options.limit, 10),
        0,
        {
          type: options.type,
          status: options.status,
        }
      );

      console.log('\nInvitations:');
      console.log('-'.repeat(100));
      invitations.forEach(inv => {
        const email = inv.invitee_email ?? inv.invitee_phone ?? 'link-only';
        console.log(
          `[${sourceAccountLabel(inv as unknown as Record<string, unknown>)}] ${inv.id} | ${inv.type} | ${inv.status} | ${email} | ${inv.code.substring(0, 12)}...`
        );
      });

      const total = await db.countInvitations({ type: options.type, status: options.status });
      console.log(`\nTotal: ${total}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('List failed', { error: message });
      process.exit(1);
    }
  });

// Validate command
program
  .command('validate')
  .description('Validate an invitation code')
  .argument('<code>', 'Invitation code')
  .action(async (code) => {
    try {
      const db = new InvitationsDatabase();
      await db.connect();

      const invitation = await db.getInvitationByCode(code);

      if (!invitation) {
        console.log('\n❌ Invalid invitation code');
        await db.disconnect();
        process.exit(1);
      }

      const isExpired = invitation.expires_at && invitation.expires_at < new Date();
      const isValid = !isExpired &&
        invitation.status !== 'accepted' &&
        invitation.status !== 'declined' &&
        invitation.status !== 'revoked';

      console.log('\nInvitation Details');
      console.log('==================');
      console.log(`ID:          ${invitation.id}`);
      console.log(`Type:        ${invitation.type}`);
      console.log(`Status:      ${invitation.status}`);
      console.log(`Inviter:     ${invitation.inviter_id}`);
      console.log(`Invitee:     ${invitation.invitee_email ?? invitation.invitee_phone ?? 'N/A'}`);
      console.log(`Channel:     ${invitation.channel}`);
      console.log(`Expires:     ${invitation.expires_at?.toISOString() ?? 'Never'}`);
      console.log(`Valid:       ${isValid ? '✅ Yes' : '❌ No'}`);

      if (!isValid) {
        if (isExpired) {
          console.log('Reason:      Expired');
        } else {
          console.log(`Reason:      ${invitation.status}`);
        }
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Validate failed', { error: message });
      process.exit(1);
    }
  });

// Templates command
program
  .command('templates')
  .description('List or manage templates')
  .argument('[action]', 'Action: list, create', 'list')
  .option('-n, --name <name>', 'Template name (for create)')
  .option('-t, --type <type>', 'Invitation type (for create)')
  .option('-c, --channel <channel>', 'Channel (for create)')
  .option('-b, --body <body>', 'Template body (for create)')
  .action(async (action, options) => {
    try {
      const db = new InvitationsDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const templates = await db.listTemplates(50);
          console.log('\nTemplates:');
          console.log('-'.repeat(80));
          templates.forEach(t => {
            console.log(`[${sourceAccountLabel(t as unknown as Record<string, unknown>)}] ${t.id} | ${t.name} | ${t.type} | ${t.channel} | ${t.enabled ? 'enabled' : 'disabled'}`);
          });
          console.log(`\nTotal: ${templates.length}`);
          break;
        }
        case 'create': {
          if (!options.name || !options.type || !options.channel || !options.body) {
            logger.error('Name, type, channel, and body are required for create');
            await db.disconnect();
            process.exit(1);
          }

          const id = await db.createTemplate({
            name: options.name,
            type: options.type,
            channel: options.channel,
            subject: null,
            body: options.body,
            variables: [],
            enabled: true,
          });

          logger.success(`Template created: ${id}`);
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Templates command failed', { error: message });
      process.exit(1);
    }
  });

// Stats command (alias for status)
program
  .command('stats')
  .description('Show invitation statistics')
  .action(async () => {
    try {
      const db = new InvitationsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nInvitation Statistics');
      console.log('=====================');
      console.log(`Total Invitations:   ${stats.total}`);
      console.log(`Accepted:            ${stats.accepted} (${stats.conversionRate.toFixed(2)}% conversion)`);
      console.log(`Pending:             ${stats.pending}`);
      console.log(`Sent:                ${stats.sent}`);
      console.log(`Delivered:           ${stats.delivered}`);
      console.log(`Declined:            ${stats.declined}`);
      console.log(`Expired:             ${stats.expired}`);
      console.log(`Revoked:             ${stats.revoked}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
