import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { SubtitleManagerDatabase } from './database.js';
import { SubtitleManagerServer } from './server.js';

const logger = createLogger('subtitle-manager');

export * from './types.js';
export * from './config.js';
export * from './database.js';
export * from './server.js';
export * from './opensubtitles-client.js';
export * from './sync.js';
export * from './qc.js';
export * from './normalize.js';

async function startServer() {
  logger.info('Starting Subtitle Manager Server', { version: '1.0.0' });

  try {
    const database = new SubtitleManagerDatabase(config.database_url);
    await database.initialize();
    logger.info('Database initialized');

    const server = new SubtitleManagerServer(config, database);
    await server.initialize();
    await server.start();

    logger.info('Subtitle Manager Server started successfully', { port: config.port });

    const shutdown = async (signal: string) => {
      logger.info(`Received ${signal}, shutting down gracefully`);
      await server.stop();
      await database.close();
      process.exit(0);
    };

    process.on('SIGTERM', () => shutdown('SIGTERM'));
    process.on('SIGINT', () => shutdown('SIGINT'));
  } catch (error) {
    logger.error('Failed to start Subtitle Manager Server', { error: error instanceof Error ? error.message : String(error) });
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
