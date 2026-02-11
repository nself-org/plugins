#!/usr/bin/env node
/**
 * Bots Plugin CLI
 * Command-line interface for the Bots plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { BotsDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('bots:cli');

const program = new Command();

program
  .name('nself-bots')
  .description('Bots plugin for nself - Bot framework, commands, marketplace')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize bots plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing bots schema...');
      const db = new BotsDatabase();
      await db.connect();
      await db.initializeSchema();
      console.log('Schema initialized successfully');
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
  .description('Start bots plugin server')
  .option('-p, --port <port>', 'Server port', '3708')
  .action(async (options) => {
    try {
      logger.info('Starting bots server...');
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
  .description('Show bots plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new BotsDatabase();
      await db.connect();
      const stats = await db.getStats();

      console.log('\nBots Plugin Status');
      console.log('===================');
      console.log(`Marketplace:         ${config.marketplaceEnabled ? 'enabled' : 'disabled'}`);
      console.log(`OAuth:               ${config.oauthEnabled ? 'enabled' : 'disabled'}`);
      console.log(`Total Bots:          ${stats.totalBots}`);
      console.log(`Enabled Bots:        ${stats.enabledBots}`);
      console.log(`Public Bots:         ${stats.publicBots}`);
      console.log(`Verified Bots:       ${stats.verifiedBots}`);
      console.log(`Total Commands:      ${stats.totalCommands}`);
      console.log(`Subscriptions:       ${stats.totalSubscriptions}`);
      console.log(`Installations:       ${stats.totalInstallations} (${stats.activeInstallations} active)`);
      console.log(`API Keys:            ${stats.totalApiKeys}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Bots: create
program
  .command('bots:create')
  .description('Create a new bot')
  .argument('<name>', 'Bot name')
  .argument('<username>', 'Bot username')
  .option('-d, --description <description>', 'Bot description')
  .option('-o, --owner <ownerId>', 'Owner user ID', 'system')
  .action(async (name, username, options) => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      const { bot, token } = await db.createBot({
        name,
        username,
        description: options.description,
        ownerId: options.owner,
      });

      console.log(`\nBot created successfully!`);
      console.log(`ID:       ${bot.id}`);
      console.log(`Name:     ${bot.name}`);
      console.log(`Username: ${bot.username}`);
      console.log(`Token:    ${token}`);
      console.log('\nSave the token - it will not be shown again.');

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create bot', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Bots: list
program
  .command('bots:list')
  .description('List bots')
  .option('-o, --owner <ownerId>', 'Filter by owner')
  .option('--public', 'Show only public bots')
  .option('-l, --limit <limit>', 'Result limit', '20')
  .action(async (options) => {
    try {
      const db = new BotsDatabase();
      await db.connect();

      const bots = await db.listBots({
        ownerId: options.owner,
        isPublic: options.public ? true : undefined,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nBots (${bots.length}):`);
      console.log('===========');
      for (const bot of bots) {
        const flags = [
          bot.is_enabled ? 'enabled' : 'disabled',
          bot.is_public ? 'public' : 'private',
          bot.is_verified ? 'verified' : '',
        ].filter(Boolean).join(', ');
        console.log(`- ${bot.name} (@${bot.username}) [${flags}]`);
        console.log(`  ID: ${bot.id} | Installs: ${bot.install_count} | Rating: ${bot.rating_avg}/5`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list bots', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Bots: info
program
  .command('bots:info')
  .description('Get bot details')
  .argument('<botId>', 'Bot ID')
  .action(async (botId) => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      const bot = await db.getBot(botId);
      if (!bot) {
        console.log(`Bot not found: ${botId}`);
        await db.disconnect();
        process.exit(1);
        return;
      }

      console.log(`\nBot: ${bot.name} (@${bot.username})`);
      console.log('================================');
      console.log(`ID:          ${bot.id}`);
      console.log(`Type:        ${bot.bot_type}`);
      console.log(`Description: ${bot.description ?? 'N/A'}`);
      console.log(`Enabled:     ${bot.is_enabled}`);
      console.log(`Public:      ${bot.is_public}`);
      console.log(`Verified:    ${bot.is_verified}`);
      console.log(`Category:    ${bot.category ?? 'N/A'}`);
      console.log(`Tags:        ${bot.tags.join(', ') || 'N/A'}`);
      console.log(`Installs:    ${bot.install_count}`);
      console.log(`Messages:    ${bot.message_count}`);
      console.log(`Commands:    ${bot.command_count}`);
      console.log(`Rating:      ${bot.rating_avg}/5 (${bot.rating_count} reviews)`);

      const commands = await db.listCommands(bot.id);
      if (commands.length > 0) {
        console.log(`\nCommands (${commands.length}):`);
        for (const cmd of commands) {
          console.log(`  /${cmd.command} - ${cmd.description} (${cmd.usage_count} uses)`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get bot info', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Bots: delete
program
  .command('bots:delete')
  .description('Delete a bot')
  .argument('<botId>', 'Bot ID')
  .action(async (botId) => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      const deleted = await db.deleteBot(botId);
      if (deleted) {
        console.log(`Bot deleted: ${botId}`);
      } else {
        console.log(`Bot not found: ${botId}`);
      }
      await db.disconnect();
      process.exit(deleted ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to delete bot', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Commands: register
program
  .command('bots:commands:register')
  .description('Register a command')
  .argument('<botId>', 'Bot ID')
  .argument('<command>', 'Command name')
  .argument('<description>', 'Command description')
  .option('-u, --usage <usage>', 'Usage hint')
  .action(async (botId, command, description, options) => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      const cmd = await db.createCommand({
        botId, command, description, usageHint: options.usage,
      });
      console.log(`Command registered: /${cmd.command} for bot ${botId}`);
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to register command', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Commands: list
program
  .command('bots:commands:list')
  .description('List bot commands')
  .argument('<botId>', 'Bot ID')
  .action(async (botId) => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      const commands = await db.listCommands(botId);
      console.log(`\nCommands for bot ${botId} (${commands.length}):`);
      console.log('================================');
      for (const cmd of commands) {
        const status = cmd.is_enabled ? 'enabled' : 'disabled';
        console.log(`  /${cmd.command} - ${cmd.description} [${status}] (${cmd.usage_count} uses)`);
        if (cmd.usage_hint) console.log(`    Usage: ${cmd.usage_hint}`);
      }
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list commands', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Marketplace: search
program
  .command('marketplace:search')
  .description('Search bot marketplace')
  .option('-c, --category <category>', 'Filter by category')
  .option('--verified', 'Show only verified bots')
  .option('-s, --search <query>', 'Search query')
  .option('-l, --limit <limit>', 'Result limit', '20')
  .action(async (options) => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      const bots = await db.searchMarketplace({
        category: options.category,
        verified: options.verified,
        search: options.search,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nMarketplace Bots (${bots.length}):`);
      console.log('=======================');
      for (const bot of bots) {
        const verified = bot.is_verified ? ' [verified]' : '';
        console.log(`- ${bot.name} (@${bot.username})${verified}`);
        console.log(`  ${bot.description ?? 'No description'}`);
        console.log(`  Installs: ${bot.install_count} | Rating: ${bot.rating_avg}/5 | Category: ${bot.category ?? 'N/A'}`);
      }
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Marketplace search failed', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Token: generate
program
  .command('bots:token:generate')
  .description('Generate new API token for a bot')
  .argument('<botId>', 'Bot ID')
  .option('-n, --name <name>', 'Key name', 'default')
  .option('-s, --scopes <scopes>', 'Comma-separated scopes')
  .action(async (botId, options) => {
    try {
      const db = new BotsDatabase();
      await db.connect();

      const { apiKey, rawKey } = await db.createApiKey({
        botId,
        keyName: options.name,
        permissions: 3, // READ_MESSAGES | SEND_MESSAGES default
        scopes: options.scopes?.split(',').map((s: string) => s.trim()),
      });

      console.log(`\nAPI Key generated for bot ${botId}`);
      console.log(`Key Name:   ${apiKey.key_name}`);
      console.log(`Key Prefix: ${apiKey.key_prefix}`);
      console.log(`Raw Key:    ${rawKey}`);
      console.log('\nSave the raw key - it will not be shown again.');

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to generate token', { error: message });
      console.error(`Error: ${message}`);
      process.exit(1);
    }
  });

// Health command
program
  .command('health')
  .description('Health check')
  .action(async () => {
    try {
      const db = new BotsDatabase();
      await db.connect();
      await db.query('SELECT 1');
      console.log('Database: connected');
      console.log('Status: healthy');
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      console.error(`Health check failed: ${message}`);
      process.exit(1);
    }
  });

program.parse();
