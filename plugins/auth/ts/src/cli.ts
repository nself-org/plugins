#!/usr/bin/env node
/**
 * Auth Plugin CLI
 * Commander.js CLI with all user commands
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { createAuthDatabase } from './database.js';
import { createAuthServer } from './server.js';

const logger = createLogger('auth:cli');
const program = new Command();

program
  .name('nself-auth')
  .description('Auth plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize auth database schema')
  .action(async () => {
    try {
      logger.info('Initializing auth plugin...');
      const db = await createAuthDatabase(config);
      logger.success('Auth plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to initialize auth plugin', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start auth HTTP server')
  .action(async () => {
    try {
      logger.info('Starting auth server...');
      const db = await createAuthDatabase(config);
      const server = await createAuthServer(db, config);
      await server.start();

      // Handle graceful shutdown
      process.on('SIGINT', async () => {
        logger.info('Shutting down...');
        await server.stop();
        process.exit(0);
      });

      process.on('SIGTERM', async () => {
        logger.info('Shutting down...');
        await server.stop();
        process.exit(0);
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to start server', { error: message });
      process.exit(1);
    }
  });

// Sessions command
program
  .command('sessions')
  .description('List active sessions for a user')
  .requiredOption('--user <userId>', 'User ID')
  .action(async (options: { user: string }) => {
    try {
      const db = await createAuthDatabase(config);
      const sessions = await db.getActiveSessions(options.user);
      console.log(JSON.stringify(sessions, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to fetch sessions', { error: message });
      process.exit(1);
    }
  });

// Revoke session command
program
  .command('revoke-session')
  .description('Revoke a specific session')
  .requiredOption('--session-id <sessionId>', 'Session ID')
  .option('--reason <reason>', 'Revocation reason')
  .action(async (options: { sessionId: string; reason?: string }) => {
    try {
      const db = await createAuthDatabase(config);
      await db.revokeSession(options.sessionId, options.reason);
      logger.success('Session revoked successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to revoke session', { error: message });
      process.exit(1);
    }
  });

// Revoke all sessions command
program
  .command('revoke-all')
  .description('Revoke all sessions for a user')
  .requiredOption('--user <userId>', 'User ID')
  .option('--except <sessionId>', 'Session ID to exclude')
  .option('--reason <reason>', 'Revocation reason')
  .action(async (options: { user: string; except?: string; reason?: string }) => {
    try {
      const db = await createAuthDatabase(config);
      const count = await db.revokeAllUserSessions(options.user, options.except, options.reason);
      logger.success(`Revoked ${count} sessions`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to revoke sessions', { error: message });
      process.exit(1);
    }
  });

// MFA status command
program
  .command('mfa-status')
  .description('Check MFA enrollment status for a user')
  .requiredOption('--user <userId>', 'User ID')
  .action(async (options: { user: string }) => {
    try {
      const db = await createAuthDatabase(config);
      const enrollment = await db.getMfaEnrollment(options.user, 'totp');
      if (!enrollment) {
        console.log('MFA not enrolled');
      } else {
        console.log(JSON.stringify({
          enrolled: true,
          method: enrollment.method,
          verified: enrollment.verified,
          backupCodesRemaining: enrollment.backup_codes_remaining,
        }, null, 2));
      }
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to check MFA status', { error: message });
      process.exit(1);
    }
  });

// Login attempts command
program
  .command('login-attempts')
  .description('View recent login attempts for a user')
  .requiredOption('--user <userId>', 'User ID')
  .option('--limit <limit>', 'Number of attempts to show', '20')
  .action(async (options: { user: string; limit: string }) => {
    try {
      const db = await createAuthDatabase(config);
      const attempts = await db.getLoginAttempts(options.user, parseInt(options.limit, 10));
      console.log(JSON.stringify(attempts, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to fetch login attempts', { error: message });
      process.exit(1);
    }
  });

// OAuth connections command
program
  .command('oauth-connections')
  .description('List OAuth connections for a user')
  .requiredOption('--user <userId>', 'User ID')
  .action(async (options: { user: string }) => {
    try {
      const db = await createAuthDatabase(config);
      const providers = await db.getOAuthProvidersByUser(options.user);
      console.log(JSON.stringify(providers, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to fetch OAuth connections', { error: message });
      process.exit(1);
    }
  });

// Cleanup expired command
program
  .command('cleanup-expired')
  .description('Clean up expired tokens, codes, and sessions')
  .action(async () => {
    try {
      logger.info('Cleaning up expired data...');
      const db = await createAuthDatabase(config);

      const [expiredDeviceCodes, expiredMagicLinks, expiredSessions, oldAttempts] = await Promise.all([
        db.expireOldDeviceCodes(),
        db.expireOldMagicLinks(),
        db.expireOldSessions(config.session.idleTimeoutHours, config.session.absoluteTimeoutHours),
        db.cleanupOldLoginAttempts(90),
      ]);

      logger.success(`Cleaned up:
  - ${expiredDeviceCodes} expired device codes
  - ${expiredMagicLinks} expired magic links
  - ${expiredSessions} expired sessions
  - ${oldAttempts} old login attempts`);
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to cleanup', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show auth plugin statistics')
  .action(async () => {
    try {
      const db = await createAuthDatabase(config);
      const stats = await db.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error('Failed to fetch stats', { error: message });
      process.exit(1);
    }
  });

program.parse();
