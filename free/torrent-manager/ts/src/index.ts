/**
 * Torrent Manager Entry Point
 * Main module exports and server initialization
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { TorrentDatabase } from './database.js';
import { TorrentManagerServer } from './server.js';

const logger = createLogger('torrent-manager');

// Export all modules
export * from './types.js';
export * from './config.js';
export * from './database.js';
export * from './server.js';
export * from './vpn-checker.js';
export * from './clients/base.js';
export * from './clients/transmission.js';
export * from './sources/registry.js';

/**
 * Start Torrent Manager Server
 */
async function startServer() {
  logger.info('Starting Torrent Manager Server', { version: '1.0.0' });

  try {
    // Initialize database
    const database = new TorrentDatabase(config.database_url);
    await database.initialize();
    logger.info('Database initialized');

    // Create and start server
    const server = new TorrentManagerServer(config, database);
    await server.initialize();
    await server.start();

    logger.info('Torrent Manager Server started successfully', { port: config.port });

    // Handle graceful shutdown
    const shutdown = async (signal: string) => {
      logger.info(`Received ${signal}, shutting down gracefully`);
      await server.stop();
      await database.close();
      process.exit(0);
    };

    process.on('SIGTERM', () => shutdown('SIGTERM'));
    process.on('SIGINT', () => shutdown('SIGINT'));
  } catch (error) {
    logger.error('Failed to start Torrent Manager Server', { error: error instanceof Error ? error.message : String(error) });
    process.exit(1);
  }
}

// Start server if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
