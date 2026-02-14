/**
 * Content Acquisition Entry Point
 */

import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { ContentAcquisitionDatabase } from './database.js';
import { ContentAcquisitionServer } from './server.js';

const logger = createLogger('content-acquisition');

export * from './types.js';
export * from './config.js';
export * from './database.js';
export * from './server.js';
export * from './rss-monitor.js';
export * from './pipeline.js';
export * from './state-machine.js';
export * from './quality-profiles.js';

async function startServer() {
  logger.info('Starting Content Acquisition Server', { version: '1.0.0' });

  try {
    const database = new ContentAcquisitionDatabase(config.database_url);
    await database.initialize();
    logger.info('Database initialized');

    const server = new ContentAcquisitionServer(config, database);
    await server.initialize();
    await server.start();

    logger.info('Content Acquisition Server started successfully', { port: config.port });

    const shutdown = async (signal: string) => {
      logger.info(`Received ${signal}, shutting down gracefully`);
      await server.stop();
      await database.close();
      process.exit(0);
    };

    process.on('SIGTERM', () => shutdown('SIGTERM'));
    process.on('SIGINT', () => shutdown('SIGINT'));
  } catch (error) {
    logger.error('Failed to start Content Acquisition Server', { error: error instanceof Error ? error.message : String(error) });
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
