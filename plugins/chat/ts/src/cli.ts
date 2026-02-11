#!/usr/bin/env node
/**
 * Chat Plugin CLI
 * Command-line interface for the Chat plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ChatDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('chat:cli');

const program = new Command();

program
  .name('nself-chat')
  .description('Chat plugin for nself - manage conversations and messages')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      logger.info('Initializing chat database schema...');

      const db = new ChatDatabase();
      await db.connect();
      await db.initializeSchema();

      logger.success('Chat schema initialized successfully');

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
  .description('Start the HTTP server')
  .option('-p, --port <port>', 'Server port', '3401')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting chat server on ${config.host}:${config.port}...`);

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
      const config = loadConfig();
      logger.info('Fetching chat plugin status...');

      const db = new ChatDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nChat Plugin Status');
      console.log('==================');
      console.log(`Version:              1.0.0`);
      console.log(`Port:                 ${config.port}`);
      console.log(`Max Message Length:   ${config.maxMessageLength}`);
      console.log(`Max Attachments:      ${config.maxAttachments}`);
      console.log(`Edit Window:          ${config.editWindowMinutes} minutes`);
      console.log(`Max Participants:     ${config.maxParticipants}`);
      console.log(`Max Pinned:           ${config.maxPinned}`);

      console.log('\nDatabase Statistics');
      console.log('===================');
      console.log(`Total Conversations:  ${stats.conversations}`);
      console.log(`Active Conversations: ${stats.activeConversations}`);
      console.log(`Total Participants:   ${stats.participants}`);
      console.log(`Total Users:          ${stats.totalUsers}`);
      console.log(`Total Messages:       ${stats.messages}`);
      console.log(`Messages (24h):       ${stats.messagesLast24h}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Conversations command
program
  .command('conversations')
  .description('List conversations')
  .option('-u, --user <userId>', 'Filter by user ID')
  .option('-l, --limit <limit>', 'Number of conversations to show', '20')
  .action(async (options) => {
    try {
      loadConfig();
      logger.info('Fetching conversations...');

      const db = new ChatDatabase();
      await db.connect();

      const limit = parseInt(options.limit, 10);
      const conversations = await db.listConversations(options.user, limit);

      if (conversations.length === 0) {
        console.log('\nNo conversations found.');
      } else {
        console.log(`\nConversations (${conversations.length}):`);
        console.log('='.repeat(80));

        for (const conv of conversations) {
          console.log(`\nID: ${conv.id}`);
          console.log(`Type: ${conv.type}`);
          console.log(`Name: ${conv.name ?? '(unnamed)'}`);
          console.log(`Members: ${conv.member_count}`);
          console.log(`Messages: ${conv.message_count}`);
          console.log(`Created: ${conv.created_at.toISOString()}`);
          if (conv.last_message_at) {
            console.log(`Last Message: ${conv.last_message_at.toISOString()}`);
            console.log(`Preview: ${conv.last_message_preview ?? ''}`);
          }
          console.log(`Archived: ${conv.is_archived ? 'Yes' : 'No'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list conversations', { error: message });
      process.exit(1);
    }
  });

// Messages command
program
  .command('messages')
  .description('List messages in a conversation')
  .requiredOption('-c, --conversation <id>', 'Conversation ID')
  .option('-l, --limit <limit>', 'Number of messages to show', '20')
  .action(async (options) => {
    try {
      loadConfig();
      logger.info(`Fetching messages from conversation ${options.conversation}...`);

      const db = new ChatDatabase();
      await db.connect();

      const limit = parseInt(options.limit, 10);
      const messages = await db.listMessages(options.conversation, limit);

      if (messages.length === 0) {
        console.log('\nNo messages found.');
      } else {
        console.log(`\nMessages (${messages.length}):`);
        console.log('='.repeat(80));

        // Reverse to show oldest first
        for (const msg of messages.reverse()) {
          console.log(`\n[${msg.created_at.toISOString()}] ${msg.sender_id}:`);
          console.log(`  Type: ${msg.content_type}`);
          if (msg.content) {
            const content = msg.deleted_at ? '[deleted]' : msg.content;
            console.log(`  ${content}`);
          }
          if (msg.attachments.length > 0) {
            console.log(`  Attachments: ${msg.attachments.length}`);
          }
          if (msg.mentions.length > 0) {
            console.log(`  Mentions: ${msg.mentions.join(', ')}`);
          }
          if (Object.keys(msg.reactions).length > 0) {
            const reactionStr = Object.entries(msg.reactions)
              .map(([emoji, users]) => `${emoji} (${users.length})`)
              .join(', ');
            console.log(`  Reactions: ${reactionStr}`);
          }
          if (msg.edited_at) {
            console.log(`  Edited: ${msg.edited_at.toISOString()}`);
          }
          if (msg.is_pinned) {
            console.log(`  Pinned by: ${msg.pinned_by}`);
          }
          if (msg.reply_to_id) {
            console.log(`  Reply to: ${msg.reply_to_id}`);
          }
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list messages', { error: message });
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
      logger.info('Calculating statistics...');

      const db = new ChatDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nDetailed Statistics');
      console.log('===================');
      console.log(`\nConversations:`);
      console.log(`  Total:   ${stats.conversations}`);
      console.log(`  Active:  ${stats.activeConversations}`);
      console.log(`  Archived: ${stats.conversations - stats.activeConversations}`);

      console.log(`\nUsers:`);
      console.log(`  Total Users: ${stats.totalUsers}`);
      console.log(`  Total Participants: ${stats.participants}`);

      console.log(`\nMessages:`);
      console.log(`  Total:        ${stats.messages}`);
      console.log(`  Last 24h:     ${stats.messagesLast24h}`);
      console.log(`  Avg per day:  ${(stats.messagesLast24h).toFixed(1)}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to calculate statistics', { error: message });
      process.exit(1);
    }
  });

// Parse arguments
program.parse();
