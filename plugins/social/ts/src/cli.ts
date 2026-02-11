#!/usr/bin/env node
/**
 * Social Plugin CLI
 * Command-line interface for the Social plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { SocialDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('social:cli');

const program = new Command();

program
  .name('nself-social')
  .description('Social plugin for nself - universal social features')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      logger.info('Initializing social plugin database schema...');

      const db = new SocialDatabase();
      await db.connect();
      await db.initializeSchema();

      logger.success('Database schema initialized successfully');

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
  .description('Start the webhook and API server')
  .option('-p, --port <port>', 'Server port', '3502')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info('Starting social plugin server...');
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
  .description('Show plugin status and statistics')
  .action(async () => {
    try {
      logger.info('Fetching social plugin status...');

      const db = new SocialDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nSocial Plugin Status');
      console.log('===================');
      console.log(`Posts:      ${stats.posts}`);
      console.log(`Comments:   ${stats.comments}`);
      console.log(`Reactions:  ${stats.reactions}`);
      console.log(`Follows:    ${stats.follows}`);
      console.log(`Bookmarks:  ${stats.bookmarks}`);
      console.log(`Shares:     ${stats.shares}`);

      if (stats.lastUpdatedAt) {
        console.log(`\nLast Updated: ${stats.lastUpdatedAt.toISOString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Posts command
program
  .command('posts')
  .description('List recent posts')
  .option('-a, --author <author_id>', 'Filter by author ID')
  .option('-t, --hashtag <hashtag>', 'Filter by hashtag')
  .option('-l, --limit <limit>', 'Number of posts to show', '10')
  .action(async (options) => {
    try {
      const db = new SocialDatabase();
      await db.connect();

      const posts = await db.listPosts({
        author_id: options.author,
        hashtag: options.hashtag,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nRecent Posts (${posts.length})`);
      console.log('=============');

      posts.forEach(post => {
        console.log(`\nID: ${post.id}`);
        console.log(`Author: ${post.author_id}`);
        console.log(`Content: ${post.content?.substring(0, 100)}${(post.content?.length ?? 0) > 100 ? '...' : ''}`);
        console.log(`Type: ${post.content_type}`);
        console.log(`Visibility: ${post.visibility}`);
        console.log(`Comments: ${post.comment_count} | Reactions: ${post.reaction_count} | Shares: ${post.share_count}`);
        if (post.hashtags.length > 0) {
          console.log(`Hashtags: ${post.hashtags.join(', ')}`);
        }
        console.log(`Created: ${new Date(post.created_at).toISOString()}`);
      });

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list posts', { error: message });
      process.exit(1);
    }
  });

// Comments command
program
  .command('comments')
  .description('List recent comments')
  .option('-t, --target-type <type>', 'Filter by target type')
  .option('-i, --target-id <id>', 'Filter by target ID')
  .option('-a, --author <author_id>', 'Filter by author ID')
  .option('-l, --limit <limit>', 'Number of comments to show', '10')
  .action(async (options) => {
    try {
      const db = new SocialDatabase();
      await db.connect();

      const comments = await db.listComments({
        target_type: options.targetType,
        target_id: options.targetId,
        author_id: options.author,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nRecent Comments (${comments.length})`);
      console.log('===============');

      comments.forEach(comment => {
        console.log(`\nID: ${comment.id}`);
        console.log(`Author: ${comment.author_id}`);
        console.log(`Target: ${comment.target_type}/${comment.target_id}`);
        console.log(`Content: ${comment.content.substring(0, 100)}${comment.content.length > 100 ? '...' : ''}`);
        console.log(`Depth: ${comment.depth} | Replies: ${comment.reply_count} | Reactions: ${comment.reaction_count}`);
        console.log(`Created: ${new Date(comment.created_at).toISOString()}`);
      });

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list comments', { error: message });
      process.exit(1);
    }
  });

// Reactions command
program
  .command('reactions')
  .description('Show reactions for a target')
  .requiredOption('-t, --target-type <type>', 'Target type')
  .requiredOption('-i, --target-id <id>', 'Target ID')
  .action(async (options) => {
    try {
      const db = new SocialDatabase();
      await db.connect();

      const reactions = await db.getReactions({
        target_type: options.targetType,
        target_id: options.targetId,
      });

      console.log(`\nReactions for ${options.targetType}/${options.targetId}`);
      console.log('===========================================');

      reactions.forEach(reaction => {
        console.log(`\n${reaction.reaction_type}: ${reaction.count} reactions`);
        console.log(`Users: ${reaction.users.slice(0, 10).join(', ')}${reaction.users.length > 10 ? '...' : ''}`);
      });

      const total = reactions.reduce((sum, r) => sum + r.count, 0);
      console.log(`\nTotal: ${total} reactions`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get reactions', { error: message });
      process.exit(1);
    }
  });

// Follows command
program
  .command('follows')
  .description('Show follow relationships')
  .option('--followers <user_id>', 'Show followers of user')
  .option('--following <user_id>', 'Show who user is following')
  .action(async (options) => {
    try {
      if (!options.followers && !options.following) {
        console.error('Error: Either --followers or --following must be specified');
        process.exit(1);
      }

      const db = new SocialDatabase();
      await db.connect();

      if (options.followers) {
        const follows = await db.listFollows({
          following_type: 'user',
          following_id: options.followers,
        });

        console.log(`\nFollowers of ${options.followers} (${follows.length})`);
        console.log('==============================');

        follows.forEach(follow => {
          console.log(`- ${follow.follower_id} (since ${new Date(follow.created_at).toISOString()})`);
        });
      }

      if (options.following) {
        const follows = await db.listFollows({
          follower_id: options.following,
        });

        console.log(`\nFollowing by ${options.following} (${follows.length})`);
        console.log('==============================');

        follows.forEach(follow => {
          console.log(`- ${follow.following_type}/${follow.following_id} (since ${new Date(follow.created_at).toISOString()})`);
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get follows', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show detailed statistics')
  .option('-u, --user <user_id>', 'Show stats for specific user')
  .action(async (options) => {
    try {
      const db = new SocialDatabase();
      await db.connect();

      if (options.user) {
        const profile = await db.getUserProfile(options.user);

        console.log(`\nUser Profile: ${options.user}`);
        console.log('======================');
        console.log(`Posts:      ${profile.post_count}`);
        console.log(`Followers:  ${profile.follower_count}`);
        console.log(`Following:  ${profile.following_count}`);
        console.log(`Bookmarks:  ${profile.bookmark_count}`);
      } else {
        const stats = await db.getStats();
        const trending = await db.getTrendingHashtags(10);

        console.log('\nOverall Statistics');
        console.log('==================');
        console.log(`Posts:      ${stats.posts}`);
        console.log(`Comments:   ${stats.comments}`);
        console.log(`Reactions:  ${stats.reactions}`);
        console.log(`Follows:    ${stats.follows}`);
        console.log(`Bookmarks:  ${stats.bookmarks}`);
        console.log(`Shares:     ${stats.shares}`);

        if (trending.length > 0) {
          console.log('\nTrending Hashtags (Last 7 Days)');
          console.log('================================');
          trending.forEach((tag, index) => {
            console.log(`${index + 1}. #${tag.hashtag} - ${tag.count} uses`);
          });
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get stats', { error: message });
      process.exit(1);
    }
  });

program.parse();
