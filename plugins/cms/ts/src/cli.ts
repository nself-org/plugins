#!/usr/bin/env node
/**
 * CMS Plugin CLI
 * Command-line interface for the CMS plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { CmsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('cms:cli');

const program = new Command();

program
  .name('nself-cms')
  .description('CMS plugin for nself - headless content management')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config
      logger.info('Initializing CMS database schema...');

      const db = new CmsDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.success('Schema initialized successfully');
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
  .option('-p, --port <port>', 'Server port', '3501')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting CMS server on ${config.host}:${config.port}...`);

      const server = await createServer(config);
      if ('start' in server && typeof server.start === 'function') {
        await server.start();
      } else {
        throw new Error('Server start method not available');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show plugin status and content statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nCMS Plugin Status');
      console.log('=================');
      console.log(`Content Types:    ${stats.contentTypes}`);
      console.log(`Posts:            ${stats.posts}`);
      console.log(`  - Published:    ${stats.publishedPosts}`);
      console.log(`  - Draft:        ${stats.draftPosts}`);
      console.log(`  - Scheduled:    ${stats.scheduledPosts}`);
      console.log(`Categories:       ${stats.categories}`);
      console.log(`Tags:             ${stats.tags}`);
      console.log(`Total Words:      ${stats.totalWordCount.toLocaleString()}`);
      console.log(`Total Views:      ${stats.totalViews.toLocaleString()}`);
      if (stats.lastPublishedAt) {
        console.log(`Last Published:   ${stats.lastPublishedAt.toLocaleString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Stats command
program
  .command('stats')
  .description('Show content statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nContent Statistics');
      console.log('==================');
      console.log(JSON.stringify(stats, null, 2));

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

// Posts command
program
  .command('posts')
  .description('Manage posts')
  .option('-l, --list', 'List all posts')
  .option('-s, --status <status>', 'Filter by status')
  .option('--type <type>', 'Filter by content type')
  .option('--featured', 'Show only featured posts')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      if (options.list) {
        const posts = await db.listPosts({
          status: options.status,
          content_type: options.type,
          is_featured: options.featured,
        });

        console.log(`\nFound ${posts.length} posts:\n`);
        posts.forEach(post => {
          console.log(`${post.status.toUpperCase().padEnd(10)} | ${post.title}`);
          console.log(`  ID: ${post.id}`);
          console.log(`  Slug: ${post.slug}`);
          console.log(`  Type: ${post.content_type}`);
          if (post.published_at) {
            console.log(`  Published: ${post.published_at.toLocaleString()}`);
          }
          console.log('');
        });
      } else {
        console.log('Use --list to list posts');
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Posts command failed', { error: message });
      process.exit(1);
    }
  });

// Categories command
program
  .command('categories')
  .description('Manage categories')
  .option('-l, --list', 'List all categories')
  .option('-t, --tree', 'Show category tree')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      if (options.tree) {
        const tree = await db.getCategoryTree();

        console.log('\nCategory Tree:\n');
        function printTree(categories: typeof tree, indent = 0) {
          for (const category of categories) {
            const prefix = '  '.repeat(indent);
            console.log(`${prefix}- ${category.name} (${category.post_count} posts)`);
            if (category.children && category.children.length > 0) {
              printTree(category.children, indent + 1);
            }
          }
        }
        printTree(tree);
      } else if (options.list) {
        const categories = await db.listCategories();

        console.log(`\nFound ${categories.length} categories:\n`);
        categories.forEach(category => {
          console.log(`- ${category.name} (${category.post_count} posts)`);
          console.log(`  ID: ${category.id}`);
          console.log(`  Slug: ${category.slug}`);
          console.log('');
        });
      } else {
        console.log('Use --list or --tree to view categories');
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Categories command failed', { error: message });
      process.exit(1);
    }
  });

// Tags command
program
  .command('tags')
  .description('Manage tags')
  .option('-l, --list', 'List all tags')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      if (options.list) {
        const tags = await db.listTags();

        console.log(`\nFound ${tags.length} tags:\n`);
        tags.forEach(tag => {
          console.log(`- ${tag.name} (${tag.post_count} posts)`);
          console.log(`  ID: ${tag.id}`);
          console.log(`  Slug: ${tag.slug}`);
          console.log('');
        });
      } else {
        console.log('Use --list to list tags');
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Tags command failed', { error: message });
      process.exit(1);
    }
  });

// Content Types command
program
  .command('content-types')
  .description('Manage content types')
  .option('-l, --list', 'List all content types')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      if (options.list) {
        const contentTypes = await db.listContentTypes();

        console.log(`\nFound ${contentTypes.length} content types:\n`);
        contentTypes.forEach(type => {
          console.log(`- ${type.display_name ?? type.name} (${type.name})`);
          console.log(`  ID: ${type.id}`);
          console.log(`  Enabled: ${type.enabled}`);
          if (type.description) {
            console.log(`  Description: ${type.description}`);
          }
          console.log('');
        });
      } else {
        console.log('Use --list to list content types');
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Content types command failed', { error: message });
      process.exit(1);
    }
  });

// Publish command
program
  .command('publish <post-id>')
  .description('Publish a post by ID')
  .action(async (postId: string) => {
    try {
      loadConfig();
      const db = new CmsDatabase();
      await db.connect();

      const post = await db.publishPost(postId);
      if (!post) {
        logger.error('Post not found');
        process.exit(1);
      }

      logger.success(`Published post: ${post.title}`);
      console.log(`  ID: ${post.id}`);
      console.log(`  Slug: ${post.slug}`);
      console.log(`  Published at: ${post.published_at?.toLocaleString()}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Publish failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
