#!/usr/bin/env node
/**
 * Knowledge Base Plugin CLI
 * Command-line interface for the Knowledge Base plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { KBDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('knowledge-base:cli');

const program = new Command();

program
  .name('nself-knowledge-base')
  .description('Knowledge base plugin for nself - documentation, FAQ, and search')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new KBDatabase();
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
  .description('Start the knowledge base server')
  .option('-p, --port <port>', 'Server port', '3713')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      const { app } = await createServer(config);
      await app.listen({ port: config.port, host: config.host });
      logger.success(`Knowledge Base plugin listening on ${config.host}:${config.port}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show knowledge base status')
  .requiredOption('-w, --workspace <id>', 'Workspace ID')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new KBDatabase();
      await db.connect();

      const stats = await db.getStats(options.workspace);

      console.log('\nKnowledge Base Status');
      console.log('=====================');
      console.log(`Documents:     ${stats.total_documents} (${stats.published_documents} published, ${stats.draft_documents} drafts)`);
      console.log(`FAQs:          ${stats.total_faqs}`);
      console.log(`Collections:   ${stats.total_collections}`);
      console.log(`Comments:      ${stats.total_comments}`);
      console.log(`Views:         ${stats.total_views}`);
      console.log(`Searches:      ${stats.total_searches}`);
      console.log(`Translations:  ${stats.total_translations}`);
      console.log(`Pending Reviews: ${stats.pending_reviews}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Documents command
program
  .command('documents')
  .description('Manage documents')
  .argument('[action]', 'Action: list, get, search', 'list')
  .requiredOption('-w, --workspace <id>', 'Workspace ID')
  .option('--id <id>', 'Document ID (for get)')
  .option('-q, --query <query>', 'Search query (for search)')
  .option('-s, --status <status>', 'Filter by status')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new KBDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const docs = await db.listDocuments(options.workspace, parseInt(options.limit, 10), 0, {
            status: options.status,
          });
          console.log(`\nDocuments (${docs.length}):`);
          console.log('-'.repeat(100));
          for (const doc of docs) {
            console.log(`${doc.id} | ${doc.title} | ${doc.status} | v${doc.version} | ${doc.document_type}`);
          }
          break;
        }
        case 'get': {
          if (!options.id) {
            logger.error('Document ID required (--id)');
            process.exit(1);
          }
          const doc = await db.getDocument(options.id);
          if (!doc) {
            logger.error('Document not found');
            process.exit(1);
          }
          console.log(JSON.stringify(doc, null, 2));
          break;
        }
        case 'search': {
          if (!options.query) {
            logger.error('Search query required (-q)');
            process.exit(1);
          }
          const results = await db.searchDocuments(options.workspace, options.query);
          console.log(`\nSearch results for "${options.query}" (${results.length}):`);
          for (const r of results) {
            console.log(`  ${r.title} (${r.slug}) - rank: ${r.rank}`);
          }
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Collections command
program
  .command('collections')
  .description('Manage collections')
  .argument('[action]', 'Action: list', 'list')
  .requiredOption('-w, --workspace <id>', 'Workspace ID')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new KBDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const colls = await db.listCollections(options.workspace);
          console.log(`\nCollections (${colls.length}):`);
          console.log('-'.repeat(80));
          for (const c of colls) {
            const indent = '  '.repeat(c.depth);
            console.log(`${indent}${c.id} | ${c.name} (${c.slug}) | ${c.visibility}`);
          }
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// FAQs command
program
  .command('faqs')
  .description('Manage FAQs')
  .argument('[action]', 'Action: list', 'list')
  .requiredOption('-w, --workspace <id>', 'Workspace ID')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (action, options) => {
    try {
      loadConfig();
      const db = new KBDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const faqs = await db.listFaqs(options.workspace, parseInt(options.limit, 10));
          console.log(`\nFAQs (${faqs.length}):`);
          console.log('-'.repeat(100));
          for (const f of faqs) {
            console.log(`${f.id} | Q: ${f.question.substring(0, 60)}... | ${f.status}`);
          }
          break;
        }
        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Popular searches command
program
  .command('popular-searches')
  .description('View popular search queries')
  .requiredOption('-w, --workspace <id>', 'Workspace ID')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new KBDatabase();
      await db.connect();

      const searches = await db.getPopularSearches(options.workspace, parseInt(options.limit, 10));
      console.log('\nPopular Searches:');
      console.log('=================');
      for (const s of searches) {
        console.log(`  "${s.search_query}" - ${s.search_count} searches (${s.unique_users} unique users)`);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
