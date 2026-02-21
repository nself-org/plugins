#!/usr/bin/env node
/**
 * Tokens Plugin CLI
 * Command-line interface for token management operations
 */

import { Command } from 'commander';
import { createLogger, createDatabase } from '@nself/plugin-utils';
import { config } from './config.js';
import { TokensDatabase } from './database.js';

const logger = createLogger('tokens:cli');
const program = new Command();

program
  .name('nself-tokens')
  .description('Tokens plugin CLI for nself')
  .version('1.0.0');

// ============================================================================
// Init Command
// ============================================================================

program
  .command('init')
  .description('Initialize tokens database schema')
  .action(async () => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tokensDb = new TokensDatabase(db);
      await tokensDb.initializeSchema();
      logger.success('Tokens plugin initialized successfully');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to initialize tokens plugin', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Server Command
// ============================================================================

program
  .command('server')
  .description('Start tokens HTTP server')
  .action(async () => {
    try {
      logger.info('Starting tokens server...');
      const { start } = await import('./server.js');
      await start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to start tokens server', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Issue Command
// ============================================================================

program
  .command('issue')
  .description('Issue a signed access token')
  .requiredOption('--user <userId>', 'User ID')
  .requiredOption('--content <contentId>', 'Content ID')
  .option('--type <tokenType>', 'Token type (playback|download|preview)', 'playback')
  .option('--ttl <seconds>', 'TTL in seconds', '3600')
  .option('--device <deviceId>', 'Device ID')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/issue`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify({
          userId: options.user,
          contentId: options.content,
          tokenType: options.type,
          ttlSeconds: parseInt(options.ttl, 10),
          deviceId: options.device,
        }),
      });
      const data = await response.json();
      if (!response.ok) {
        logger.error('Failed to issue token', { error: (data as Record<string, string>).message || (data as Record<string, string>).error });
        process.exit(1);
      }
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Issue failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Validate Command
// ============================================================================

program
  .command('validate')
  .description('Validate a token')
  .requiredOption('--token <token>', 'Token to validate')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const response = await fetch(`${baseUrl}/api/validate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify({ token: options.token }),
      });
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Validation failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Revoke Command
// ============================================================================

program
  .command('revoke')
  .description('Revoke tokens')
  .option('--token-id <tokenId>', 'Specific token ID to revoke')
  .option('--user <userId>', 'Revoke all tokens for user')
  .option('--content <contentId>', 'Revoke all tokens for content')
  .option('--reason <reason>', 'Revocation reason')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      let endpoint = '/api/revoke';
      let body: Record<string, unknown> = { reason: options.reason };

      if (options.user) {
        endpoint = '/api/revoke/user';
        body.userId = options.user;
      } else if (options.content) {
        endpoint = '/api/revoke/content';
        body.contentId = options.content;
      } else if (options.tokenId) {
        body.tokenId = options.tokenId;
      } else {
        logger.error('Specify --token-id, --user, or --content');
        process.exit(1);
      }

      const response = await fetch(`${baseUrl}${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify(body),
      });
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Revoke failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Keys Commands
// ============================================================================

const keysCmd = program.command('keys').description('Manage signing keys');

keysCmd
  .command('list')
  .description('List signing keys')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tokensDb = new TokensDatabase(db).forSourceAccount(options.appId);
      const keys = await tokensDb.listSigningKeys();
      console.log(JSON.stringify({ keys: keys.map(k => ({
        id: k.id, name: k.name, algorithm: k.algorithm,
        isActive: k.is_active, createdAt: k.created_at,
      }))}, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list keys', { error: message });
      process.exit(1);
    }
  });

keysCmd
  .command('rotate')
  .description('Rotate a signing key')
  .requiredOption('--name <name>', 'Key name to rotate')
  .option('--expire-hours <hours>', 'Hours until old key expires', '24')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const baseUrl = `http://${config.host === '0.0.0.0' ? 'localhost' : config.host}:${config.port}`;
      const db = createDatabase(config.database);
      await db.connect();
      const tokensDb = new TokensDatabase(db).forSourceAccount(options.appId);
      const key = await tokensDb.getSigningKeyByName(options.name);
      if (!key) {
        logger.error('Key not found', { name: options.name });
        process.exit(1);
      }

      const response = await fetch(`${baseUrl}/api/keys/${key.id}/rotate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-App-Name': options.appId },
        body: JSON.stringify({ expireOldAfterHours: parseInt(options.expireHours, 10) }),
      });
      const data = await response.json();
      console.log(JSON.stringify(data, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Key rotation failed', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Entitlements Command
// ============================================================================

program
  .command('entitlements')
  .description('List user entitlements')
  .requiredOption('--user <userId>', 'User ID')
  .option('--content-type <contentType>', 'Filter by content type')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tokensDb = new TokensDatabase(db).forSourceAccount(options.appId);
      const entitlements = await tokensDb.listUserEntitlements(options.user, options.contentType);
      console.log(JSON.stringify({ entitlements }, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list entitlements', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Stats Command
// ============================================================================

program
  .command('stats')
  .description('Show token statistics')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tokensDb = new TokensDatabase(db).forSourceAccount(options.appId);
      const stats = await tokensDb.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get stats', { error: message });
      process.exit(1);
    }
  });

// ============================================================================
// Status Command
// ============================================================================

program
  .command('status')
  .description('Show tokens plugin status')
  .option('--app-id <appId>', 'Application ID', 'primary')
  .action(async (options) => {
    try {
      const db = createDatabase(config.database);
      await db.connect();
      const tokensDb = new TokensDatabase(db).forSourceAccount(options.appId);
      const stats = await tokensDb.getStats();
      console.log(JSON.stringify(stats, null, 2));
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get status', { error: message });
      process.exit(1);
    }
  });

program.parse();
