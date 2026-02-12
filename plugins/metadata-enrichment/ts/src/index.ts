import { createLogger } from '@nself/plugin-utils';
import { config } from './config.js';
import { MetadataEnrichmentDatabase } from './database.js';
import { MetadataEnrichmentServer } from './server.js';

const logger = createLogger('metadata-enrichment');

export * from './types.js';
export * from './config.js';
export * from './database.js';
export * from './server.js';
export * from './tmdb-client.js';

async function startServer() {
  logger.info('Starting Metadata Enrichment Server', { version: '1.0.0' });

  try {
    const database = new MetadataEnrichmentDatabase(config.database_url);
    await database.initialize();
    logger.info('Database initialized');

    const server = new MetadataEnrichmentServer(config, database);
    await server.initialize();
    await server.start();

    logger.info('Metadata Enrichment Server started successfully', { port: config.port });

    const shutdown = async (signal: string) => {
      logger.info(`Received ${signal}, shutting down gracefully`);
      await server.stop();
      await database.close();
      process.exit(0);
    };

    process.on('SIGTERM', () => shutdown('SIGTERM'));
    process.on('SIGINT', () => shutdown('SIGINT'));
  } catch (error) {
    logger.error('Failed to start Metadata Enrichment Server', { error: error instanceof Error ? error.message : String(error) });
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  startServer();
}
