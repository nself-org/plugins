#!/usr/bin/env node
/**
 * Link Preview Plugin CLI
 * Command-line interface for link preview management, templates, oEmbed, blocklist, and analytics
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { LinkPreviewDatabase } from './database.js';
import { createServer } from './server.js';
import { createHash } from 'crypto';

const logger = createLogger('link-preview:cli');

const program = new Command();

program
  .name('nself-link-preview')
  .description('URL metadata extraction, caching, and link preview plugin for nself')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
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
  .description('Start the link preview server')
  .option('-p, --port <port>', 'Server port', '3718')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      logger.info('Starting link preview server...');
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
  .description('Show link preview cache statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const stats = await db.getCacheStats();

      console.log('\nLink Preview Cache Statistics');
      console.log('=============================');
      console.log(`Total Previews:    ${stats.total_previews}`);
      console.log(`  Successful:      ${stats.successful}`);
      console.log(`  Failed:          ${stats.failed}`);
      console.log(`  Expired:         ${stats.expired}`);
      console.log(`Avg Fetch Time:    ${stats.avg_fetch_duration_ms.toFixed(2)}ms`);
      console.log(`oEmbed Count:      ${stats.oembed_count}`);
      console.log(`Unique Sites:      ${stats.unique_sites}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Fetch command
program
  .command('fetch <url>')
  .description('Fetch preview for a URL')
  .option('-f, --force', 'Force refresh even if cached')
  .action(async (url, options) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      // Check blocklist
      const blocked = await db.isUrlBlocked(url);
      if (blocked) {
        logger.error('URL is blocked');
        process.exit(1);
      }

      if (!options.force) {
        const cached = await db.getPreviewByUrl(url);
        if (cached) {
          console.log('\nCached Preview:');
          console.log(JSON.stringify(cached, null, 2));
          await db.disconnect();
          return;
        }
      }

      const urlHash = createHash('sha256').update(url.toLowerCase().trim()).digest('hex');
      const preview = await db.upsertPreview({
        url,
        url_hash: urlHash,
        status: 'partial',
      });

      console.log('\nPreview:');
      console.log(JSON.stringify(preview, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Fetch failed', { error: message });
      process.exit(1);
    }
  });

// Refresh command
program
  .command('refresh <id>')
  .description('Refresh a cached preview')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const preview = await db.getPreview(id);
      if (!preview) {
        logger.error('Preview not found');
        process.exit(1);
      }

      const updated = await db.upsertPreview({
        url: preview.url,
        url_hash: preview.url_hash,
        status: 'success',
      });

      console.log('\nRefreshed Preview:');
      console.log(JSON.stringify(updated, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Refresh failed', { error: message });
      process.exit(1);
    }
  });

// Delete command
program
  .command('delete <id>')
  .description('Delete a cached preview')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const deleted = await db.deletePreview(id);
      if (!deleted) {
        logger.error('Preview not found');
        process.exit(1);
      }
      logger.success('Preview deleted');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Delete failed', { error: message });
      process.exit(1);
    }
  });

// Template commands
const templateCmd = program
  .command('template')
  .description('Manage custom preview templates');

templateCmd
  .command('list')
  .description('List custom preview templates')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const templates = await db.listTemplates(parseInt(options.limit, 10));

      console.log('\nPreview Templates:');
      console.log('-'.repeat(80));
      if (templates.length === 0) {
        console.log('No templates found');
      } else {
        templates.forEach((t) => {
          const active = t.is_active ? 'ACTIVE' : 'INACTIVE';
          console.log(`[${active}] ${t.name} (priority: ${t.priority})`);
          console.log(`  ID: ${t.id}`);
          console.log(`  Pattern: ${t.url_pattern}`);
          if (t.description) console.log(`  ${t.description}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Template list failed', { error: message });
      process.exit(1);
    }
  });

templateCmd
  .command('test <id> <url>')
  .description('Test a template against a URL')
  .action(async (id, url) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const template = await db.getTemplate(id);
      if (!template) {
        logger.error('Template not found');
        process.exit(1);
      }

      try {
        const regex = new RegExp(template.url_pattern);
        const matches = regex.test(url);
        console.log(`\nTemplate: ${template.name}`);
        console.log(`Pattern:  ${template.url_pattern}`);
        console.log(`URL:      ${url}`);
        console.log(`Matches:  ${matches ? 'YES' : 'NO'}`);
      } catch {
        logger.error('Invalid regex pattern in template');
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Template test failed', { error: message });
      process.exit(1);
    }
  });

templateCmd
  .command('delete <id>')
  .description('Delete a template')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const deleted = await db.deleteTemplate(id);
      if (!deleted) {
        logger.error('Template not found');
        process.exit(1);
      }
      logger.success('Template deleted');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Template delete failed', { error: message });
      process.exit(1);
    }
  });

// oEmbed commands
const oembedCmd = program
  .command('oembed')
  .description('Manage oEmbed providers');

oembedCmd
  .command('providers')
  .description('List oEmbed providers')
  .action(async () => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const providers = await db.listOEmbedProviders();

      console.log('\noEmbed Providers:');
      console.log('-'.repeat(80));
      if (providers.length === 0) {
        console.log('No oEmbed providers registered');
      } else {
        providers.forEach((p) => {
          const active = p.is_active ? 'ACTIVE' : 'INACTIVE';
          console.log(`[${active}] ${p.provider_name}`);
          console.log(`  ID: ${p.id}`);
          console.log(`  URL: ${p.provider_url}`);
          console.log(`  Endpoint: ${p.endpoint_url}`);
          console.log(`  Schemes: ${p.url_schemes.join(', ')}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('oEmbed providers list failed', { error: message });
      process.exit(1);
    }
  });

oembedCmd
  .command('discover <url>')
  .description('Discover oEmbed provider for a URL')
  .action(async (url) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const provider = await db.findOEmbedProvider(url);
      if (!provider) {
        console.log('No oEmbed provider found for this URL');
      } else {
        console.log('\nMatching Provider:');
        console.log(JSON.stringify(provider, null, 2));
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('oEmbed discover failed', { error: message });
      process.exit(1);
    }
  });

// Blocklist commands
program
  .command('block <url>')
  .description('Add a URL to the blocklist')
  .option('-t, --type <type>', 'Pattern type: exact, domain, regex', 'exact')
  .option('-r, --reason <reason>', 'Reason: spam, phishing, malware, offensive, other', 'other')
  .option('-d, --description <description>', 'Description')
  .action(async (url, options) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const entry = await db.addToBlocklist({
        url_pattern: url,
        pattern_type: options.type,
        reason: options.reason,
        description: options.description,
      });

      logger.success(`Added to blocklist: ${entry.id}`);
      console.log(JSON.stringify(entry, null, 2));

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Block failed', { error: message });
      process.exit(1);
    }
  });

program
  .command('unblock <id>')
  .description('Remove a URL from the blocklist')
  .action(async (id) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const removed = await db.removeFromBlocklist(id);
      if (!removed) {
        logger.error('Blocklist entry not found');
        process.exit(1);
      }
      logger.success('Removed from blocklist');

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Unblock failed', { error: message });
      process.exit(1);
    }
  });

program
  .command('blocklist')
  .description('List blocked URLs')
  .option('-l, --limit <limit>', 'Number of records', '20')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const entries = await db.listBlocklist(parseInt(options.limit, 10));

      console.log('\nBlocked URLs:');
      console.log('-'.repeat(80));
      if (entries.length === 0) {
        console.log('No blocked URLs');
      } else {
        entries.forEach((e) => {
          console.log(`[${e.reason.toUpperCase()}] ${e.url_pattern} (${e.pattern_type})`);
          console.log(`  ID: ${e.id}`);
          if (e.description) console.log(`  ${e.description}`);
          if (e.expires_at) console.log(`  Expires: ${e.expires_at}`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Blocklist failed', { error: message });
      process.exit(1);
    }
  });

program
  .command('check <url>')
  .description('Check if a URL is blocked')
  .action(async (url) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const blocked = await db.isUrlBlocked(url);
      console.log(`\nURL: ${url}`);
      console.log(`Blocked: ${blocked ? 'YES' : 'NO'}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Check failed', { error: message });
      process.exit(1);
    }
  });

// Analytics commands
program
  .command('popular')
  .description('Show most popular link previews')
  .option('-l, --limit <limit>', 'Number of records', '10')
  .action(async (options) => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const popular = await db.getPopularPreviews(parseInt(options.limit, 10));

      console.log('\nPopular Link Previews:');
      console.log('-'.repeat(80));
      if (popular.length === 0) {
        console.log('No popular links found');
      } else {
        popular.forEach((p, i) => {
          console.log(`${i + 1}. ${p.title ?? p.url}`);
          console.log(`   URL: ${p.url}`);
          console.log(`   Usage: ${p.usage_count} | Clicks: ${p.click_count} | CTR: ${(Number(p.click_through_rate) * 100).toFixed(1)}%`);
          console.log();
        });
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Popular links failed', { error: message });
      process.exit(1);
    }
  });

program
  .command('stats')
  .description('Show detailed cache statistics')
  .action(async () => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const stats = await db.getCacheStats();

      console.log('\nCache Statistics:');
      console.log('=================');
      console.log(`Total Previews:    ${stats.total_previews}`);
      console.log(`  Successful:      ${stats.successful}`);
      console.log(`  Failed:          ${stats.failed}`);
      console.log(`  Expired:         ${stats.expired}`);
      console.log(`Avg Fetch Time:    ${stats.avg_fetch_duration_ms.toFixed(2)}ms`);
      console.log(`oEmbed Count:      ${stats.oembed_count}`);
      console.log(`Unique Sites:      ${stats.unique_sites}`);

      const hitRate = stats.total_previews > 0
        ? ((stats.successful / stats.total_previews) * 100).toFixed(1)
        : '0.0';
      console.log(`\nCache Hit Rate:    ${hitRate}%`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Stats failed', { error: message });
      process.exit(1);
    }
  });

// Cache maintenance commands
const cacheCmd = program
  .command('cache')
  .description('Cache management');

cacheCmd
  .command('clear')
  .description('Clear all cached previews')
  .action(async () => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const count = await db.clearCache();
      logger.success(`Cleared ${count} cached previews`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cache clear failed', { error: message });
      process.exit(1);
    }
  });

program
  .command('cleanup')
  .description('Remove expired previews')
  .action(async () => {
    try {
      loadConfig();
      const db = new LinkPreviewDatabase();
      await db.connect();

      const count = await db.cleanupExpiredPreviews();
      logger.success(`Cleaned up ${count} expired previews`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Cleanup failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
