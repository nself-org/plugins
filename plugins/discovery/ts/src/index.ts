/**
 * Discovery Plugin Main Entry Point
 * Initializes database, Redis cache, and starts the Fastify server.
 */

import { createLogger } from '@nself/plugin-utils';
import { config, validateConfig } from './config.js';
import { DiscoveryDatabase } from './database.js';
import { DiscoveryCache } from './cache.js';
import { startServer } from './server.js';

const logger = createLogger('discovery:main');

/**
 * Initialize and start the discovery plugin
 */
async function initialize() {
  logger.info('Starting Discovery Plugin...');

  // Validate configuration
  if (!validateConfig(config)) {
    throw new Error('Invalid configuration');
  }

  // Initialize database
  const db = new DiscoveryDatabase(config.database_url);
  logger.info('Initializing database schema...');
  await db.initializeSchema();
  logger.info('Database schema initialized');

  // Initialize Redis cache
  const cache = new DiscoveryCache(db);
  logger.info('Connecting to Redis...');
  await cache.connect();

  if (cache.isConnected()) {
    logger.info('Redis cache connected');
  } else {
    logger.warn('Redis unavailable - operating in degraded mode (no caching)');
  }

  // Start server
  logger.info(`Starting API server on port ${config.port}...`);
  const server = await startServer(db, cache);
  logger.info(`API server running on http://localhost:${config.port}`);

  // Log startup summary
  console.log('');
  console.log('================================================================');
  console.log('          Discovery Plugin for nself - v1.0.0                    ');
  console.log('================================================================');
  console.log('');
  console.log(`  Server:     http://localhost:${config.port}`);
  console.log(`  Health:     http://localhost:${config.port}/health`);
  console.log(`  Database:   Connected`);
  console.log(`  Redis:      ${cache.isConnected() ? 'Connected' : 'Unavailable (degraded)'}`);
  console.log('');
  console.log('  Endpoints:');
  console.log(`    GET  /v1/trending          Trending content`);
  console.log(`    GET  /v1/popular           Popular content`);
  console.log(`    GET  /v1/recent            Recently added`);
  console.log(`    GET  /v1/continue/:userId  Continue watching`);
  console.log(`    GET  /health               Health check`);
  console.log(`    GET  /v1/status            Detailed status`);
  console.log(`    POST /v1/cache/invalidate  Invalidate caches`);
  console.log(`    POST /v1/cache/refresh     Refresh precomputed caches`);
  console.log('');
  console.log('  Configuration:');
  console.log(`    Trending Window:   ${config.trending_window_hours}h`);
  console.log(`    Default Limit:     ${config.default_limit}`);
  console.log(`    Cache Trending:    ${config.cache_ttl_trending}s TTL`);
  console.log(`    Cache Popular:     ${config.cache_ttl_popular}s TTL`);
  console.log(`    Cache Recent:      ${config.cache_ttl_recent}s TTL`);
  console.log(`    Cache Continue:    ${config.cache_ttl_continue}s TTL`);
  console.log('');
  console.log('  CLI: pnpm run cli --help');
  console.log('');

  // Handle graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down...');

    try {
      await server.close();
      logger.info('Server closed');

      await cache.close();
      logger.info('Redis closed');

      await db.close();
      logger.info('Database closed');

      logger.info('Shutdown complete');
      process.exit(0);
    } catch (error) {
      logger.error('Error during shutdown', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  };

  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);

  return { server, db, cache };
}

// Start if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  initialize().catch((error) => {
    logger.error('Failed to start Discovery plugin', { error: error instanceof Error ? error.message : String(error) });
    process.exit(1);
  });
}

export { initialize };
export * from './types.js';
export { DiscoveryDatabase } from './database.js';
export { DiscoveryCache } from './cache.js';
export { config } from './config.js';
