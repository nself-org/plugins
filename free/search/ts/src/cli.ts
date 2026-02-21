#!/usr/bin/env node
/**
 * Search Plugin CLI
 * Command-line interface for the Search plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { SearchDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('search:cli');

const program = new Command();

program
  .name('nself-search')
  .description('Search plugin for nself - Full-text search with PostgreSQL and MeiliSearch')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize search plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing search schema...');

      const db = new SearchDatabase();
      await db.connect();
      await db.initializeSchema();

      console.log('✓ Search schema initialized successfully');

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start search plugin server')
  .option('-p, --port <port>', 'Server port', '3302')
  .action(async (options) => {
    try {
      logger.info('Starting search server...');

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
  .description('Show search plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new SearchDatabase();
      await db.connect();

      const indexes = await db.listIndexes();

      console.log('\nSearch Plugin Status');
      console.log('====================');
      console.log(`Engine:  ${config.engine}`);
      console.log(`Indexes: ${indexes.length}`);

      if (indexes.length > 0) {
        console.log('\nIndexes:');
        for (const index of indexes) {
          const status = index.enabled ? '✓' : '✗';
          console.log(`  ${status} ${index.name} (${index.document_count} documents)`);
          console.log(`    Source: ${index.source_table ?? 'N/A'}`);
          console.log(`    Searchable: ${index.searchable_fields.join(', ')}`);
          console.log(`    Last indexed: ${index.last_indexed_at?.toISOString() ?? 'Never'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// Reindex command
program
  .command('reindex')
  .description('Reindex documents from source table')
  .argument('<index>', 'Index name')
  .option('-f, --full', 'Full reindex (clear existing documents)')
  .option('-b, --batch-size <size>', 'Batch size', '500')
  .action(async (indexName, options) => {
    try {
      const db = new SearchDatabase();
      await db.connect();

      logger.info('Starting reindex...', { index: indexName, full: options.full });

      const result = await db.reindexFromSource(indexName, {
        fullReindex: options.full,
        batchSize: parseInt(options.batchSize, 10),
      });

      console.log('\nReindex Results:');
      console.log('================');
      console.log(`Indexed:  ${result.indexed}`);
      console.log(`Failed:   ${result.failed}`);
      console.log(`Duration: ${(result.duration / 1000).toFixed(1)}s`);

      if (result.errors.length > 0) {
        console.log('\nErrors:');
        result.errors.slice(0, 10).forEach(err => console.log(`  - ${err}`));
        if (result.errors.length > 10) {
          console.log(`  ... and ${result.errors.length - 10} more`);
        }
      }

      await db.disconnect();
      process.exit(result.failed > 0 ? 1 : 0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Reindex failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// Search command
program
  .command('search')
  .description('Search indexes')
  .argument('<query>', 'Search query')
  .option('-i, --indexes <indexes>', 'Comma-separated index names')
  .option('-l, --limit <limit>', 'Result limit', '10')
  .action(async (query, options) => {
    try {
      const db = new SearchDatabase();
      await db.connect();

      const indexes = options.indexes?.split(',').map((s: string) => s.trim());

      const result = await db.search({
        q: query,
        indexes,
        limit: parseInt(options.limit, 10),
        highlight: true,
      });

      console.log(`\nSearch Results (${result.total} found in ${result.processingTimeMs}ms):`);
      console.log('==========================================');

      if (result.hits.length === 0) {
        console.log('No results found.');
      } else {
        result.hits.forEach((hit, i) => {
          console.log(`\n${i + 1}. ${hit.id} (score: ${hit.score.toFixed(4)}, index: ${hit.index})`);
          const preview = JSON.stringify(hit.content).substring(0, 200);
          console.log(`   ${preview}...`);
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Search failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// Indexes command
program
  .command('indexes')
  .description('Manage search indexes')
  .argument('<action>', 'Action: list, create, delete')
  .argument('[name]', 'Index name (for create/delete)')
  .option('-t, --table <table>', 'Source table (for create)')
  .option('-f, --fields <fields>', 'Searchable fields (comma-separated, for create)')
  .action(async (action, name, options) => {
    try {
      const db = new SearchDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const indexes = await db.listIndexes();
          console.log(`\nSearch Indexes (${indexes.length}):`);
          console.log('===================');
          indexes.forEach(idx => {
            console.log(`- ${idx.name} (${idx.document_count} docs, ${idx.enabled ? 'enabled' : 'disabled'})`);
          });
          break;
        }

        case 'create': {
          if (!name) {
            throw new Error('Index name required for create');
          }
          if (!options.table) {
            throw new Error('--table required for create');
          }
          if (!options.fields) {
            throw new Error('--fields required for create');
          }

          const index = await db.createIndex({
            name,
            source_table: options.table,
            searchable_fields: options.fields.split(',').map((s: string) => s.trim()),
          });

          console.log(`✓ Created index: ${index.name}`);
          break;
        }

        case 'delete': {
          if (!name) {
            throw new Error('Index name required for delete');
          }

          const deleted = await db.deleteIndex(name);
          if (deleted) {
            console.log(`✓ Deleted index: ${name}`);
          } else {
            console.log(`✗ Index not found: ${name}`);
          }
          break;
        }

        default:
          throw new Error(`Unknown action: ${action}. Use list, create, or delete`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Indexes command failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// Synonyms command
program
  .command('synonyms')
  .description('Manage search synonyms')
  .argument('<action>', 'Action: list, add, delete')
  .argument('<index>', 'Index name')
  .argument('[word]', 'Word (for add)')
  .option('-s, --synonyms <synonyms>', 'Comma-separated synonyms (for add)')
  .option('-i, --id <id>', 'Synonym ID (for delete)')
  .action(async (action, indexName, word, options) => {
    try {
      const db = new SearchDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const synonyms = await db.getSynonyms(indexName);
          console.log(`\nSynonyms for ${indexName} (${synonyms.length}):`);
          console.log('====================================');
          synonyms.forEach(syn => {
            console.log(`- ${syn.word}: ${syn.synonyms.join(', ')}`);
          });
          break;
        }

        case 'add': {
          if (!word) {
            throw new Error('Word required for add');
          }
          if (!options.synonyms) {
            throw new Error('--synonyms required for add');
          }

          const synonym = await db.addSynonym(
            indexName,
            word,
            options.synonyms.split(',').map((s: string) => s.trim())
          );

          console.log(`✓ Added synonym: ${synonym.word} = ${synonym.synonyms.join(', ')}`);
          break;
        }

        case 'delete': {
          if (!options.id) {
            throw new Error('--id required for delete');
          }

          const deleted = await db.deleteSynonym(indexName, options.id);
          if (deleted) {
            console.log(`✓ Deleted synonym: ${options.id}`);
          } else {
            console.log(`✗ Synonym not found: ${options.id}`);
          }
          break;
        }

        default:
          throw new Error(`Unknown action: ${action}. Use list, add, or delete`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Synonyms command failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// Analytics command
program
  .command('analytics')
  .description('View search analytics')
  .option('-d, --days <days>', 'Number of days to analyze', '30')
  .action(async (options) => {
    try {
      if (!loadConfig().analyticsEnabled) {
        console.log('✗ Analytics disabled');
        process.exit(1);
      }

      const db = new SearchDatabase();
      await db.connect();

      const days = parseInt(options.days, 10);
      const stats = await db.getSearchStats(days);

      console.log(`\nSearch Analytics (last ${days} days):`);
      console.log('====================================');
      console.log(`Total Queries:        ${stats.total_queries}`);
      console.log(`Unique Queries:       ${stats.unique_queries}`);
      console.log(`Avg Results:          ${stats.avg_results.toFixed(1)}`);
      console.log(`Avg Response Time:    ${stats.avg_time_ms.toFixed(1)}ms`);
      console.log(`Zero Results Rate:    ${(stats.zero_results_rate * 100).toFixed(1)}%`);

      console.log('\nTop Queries:');
      stats.top_queries.slice(0, 10).forEach((q, i) => {
        console.log(`  ${i + 1}. "${q.query}" (${q.count} searches, ${q.avg_results.toFixed(1)} avg results)`);
      });

      if (stats.no_result_queries.length > 0) {
        console.log('\nQueries with No Results:');
        stats.no_result_queries.slice(0, 10).forEach((q, i) => {
          console.log(`  ${i + 1}. "${q.query}" (${q.count} times)`);
        });
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Analytics failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program.parse();
