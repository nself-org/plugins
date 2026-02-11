#!/usr/bin/env node
/**
 * Documents Plugin CLI
 * Command-line interface for the Documents plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { DocumentsDatabase } from './database.js';
import { createServer } from './server.js';

const logger = createLogger('documents:cli');

const program = new Command();

program
  .name('nself-documents')
  .description('Documents plugin for nself - manage documents, templates, and generation')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new DocumentsDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.info('Database schema initialized successfully');
      console.log('Database schema initialized for documents plugin');
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the documents API server')
  .option('-p, --port <port>', 'Server port', '3029')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info(`Starting documents server on ${config.host}:${config.port}`);
      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status / Stats command
program
  .command('status')
  .alias('stats')
  .description('Show document statistics')
  .action(async () => {
    try {
      const db = new DocumentsDatabase();
      await db.connect();
      const stats = await db.getStats();
      await db.disconnect();

      console.log('\nDocuments Statistics:');
      console.log('====================');
      console.log(`Total Documents: ${stats.total_documents}`);
      console.log(`Total Templates: ${stats.total_templates}`);
      console.log(`Total Shares:    ${stats.total_shares}`);
      console.log(`Total Versions:  ${stats.total_versions}`);
      console.log(`Recent (7d):     ${stats.recent_documents}`);

      if (Object.keys(stats.by_type).length > 0) {
        console.log('\nBy Type:');
        for (const [type, count] of Object.entries(stats.by_type)) {
          console.log(`  ${type}: ${count}`);
        }
      }

      if (Object.keys(stats.by_category).length > 0) {
        console.log('\nBy Category:');
        for (const [cat, count] of Object.entries(stats.by_category)) {
          console.log(`  ${cat}: ${count}`);
        }
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats check failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// List command
program
  .command('list')
  .description('List documents')
  .option('-o, --owner <id>', 'Filter by owner ID')
  .option('-t, --type <type>', 'Filter by document type')
  .option('-c, --category <category>', 'Filter by category')
  .option('-s, --status <status>', 'Filter by status (draft, final, archived)')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (options) => {
    try {
      const db = new DocumentsDatabase();
      await db.connect();

      const documents = await db.listDocuments({
        ownerId: options.owner,
        docType: options.type,
        category: options.category,
        status: options.status,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (documents.length === 0) {
        console.log('No documents found');
        process.exit(0);
      }

      console.log(`\nFound ${documents.length} document(s):\n`);

      for (const doc of documents) {
        console.log(`${doc.title}`);
        console.log(`  ID:       ${doc.id}`);
        console.log(`  Type:     ${doc.doc_type}`);
        console.log(`  Category: ${doc.category ?? 'none'}`);
        console.log(`  Status:   ${doc.status}`);
        console.log(`  Owner:    ${doc.owner_id}`);
        console.log(`  Version:  ${doc.version}`);
        console.log(`  Created:  ${doc.created_at.toISOString()}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list documents', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Templates command
program
  .command('templates')
  .description('List templates')
  .option('-t, --type <type>', 'Filter by document type')
  .action(async (options) => {
    try {
      const db = new DocumentsDatabase();
      await db.connect();

      const templates = await db.listTemplates({
        docType: options.type,
      });

      await db.disconnect();

      if (templates.length === 0) {
        console.log('No templates found');
        process.exit(0);
      }

      console.log(`\nFound ${templates.length} template(s):\n`);

      for (const tmpl of templates) {
        console.log(`${tmpl.name} (v${tmpl.version})`);
        console.log(`  ID:       ${tmpl.id}`);
        console.log(`  Type:     ${tmpl.doc_type}`);
        console.log(`  Format:   ${tmpl.output_format}`);
        console.log(`  Engine:   ${tmpl.template_engine}`);
        console.log(`  Default:  ${tmpl.is_default ? 'yes' : 'no'}`);
        if (tmpl.description) {
          console.log(`  Desc:     ${tmpl.description}`);
        }
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list templates', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Search command
program
  .command('search <query>')
  .description('Search documents')
  .option('-t, --type <type>', 'Filter by document type')
  .option('-c, --category <category>', 'Filter by category')
  .option('-o, --owner <id>', 'Filter by owner ID')
  .option('-l, --limit <limit>', 'Limit results', '20')
  .action(async (query: string, options) => {
    try {
      const db = new DocumentsDatabase();
      await db.connect();

      const documents = await db.searchDocuments({
        query,
        docType: options.type,
        category: options.category,
        ownerId: options.owner,
        limit: parseInt(options.limit, 10),
      });

      await db.disconnect();

      if (documents.length === 0) {
        console.log(`No documents found for "${query}"`);
        process.exit(0);
      }

      console.log(`\nFound ${documents.length} document(s) for "${query}":\n`);

      for (const doc of documents) {
        console.log(`${doc.title}`);
        console.log(`  ID:       ${doc.id}`);
        console.log(`  Type:     ${doc.doc_type}`);
        console.log(`  Category: ${doc.category ?? 'none'}`);
        console.log(`  Status:   ${doc.status}`);
        console.log('');
      }

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

// Generate command
program
  .command('generate')
  .description('Generate document from template')
  .requiredOption('--template <name>', 'Template name or ID')
  .requiredOption('--data <json>', 'Data as JSON string')
  .option('--owner <id>', 'Owner ID', 'system')
  .option('--title <title>', 'Document title')
  .option('--format <format>', 'Output format (pdf, html)', 'pdf')
  .action(async (options) => {
    try {
      const data = JSON.parse(options.data) as Record<string, unknown>;

      const config = loadConfig();
      console.log(`Generating document from template "${options.template}"...`);
      console.log(`Server should be running on port ${config.port}`);
      console.log('Use the /api/generate endpoint with:');
      console.log(JSON.stringify({
        template_name: options.template,
        data,
        owner_id: options.owner,
        title: options.title,
        output_format: options.format,
      }, null, 2));

      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Generate failed', { error: message });
      console.error('Error:', message);
      process.exit(1);
    }
  });

program.parse();
