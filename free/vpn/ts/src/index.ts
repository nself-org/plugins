/**
 * VPN Plugin Main Entry Point
 * Initializes database, providers, and starts server
 */

import { createLogger } from '@nself/plugin-utils';
import { config, validateConfig } from './config.js';
import { VPNDatabase } from './database.js';
import { startServer } from './server.js';
import { getSupportedProviders, providerMetadata } from './providers/index.js';
import type { VPNProvider } from './types.js';

const logger = createLogger('vpn:main');

/**
 * Initialize plugin
 */
async function initialize() {
  logger.info('🚀 Starting VPN Plugin...');

  // Validate configuration
  if (!validateConfig(config)) {
    throw new Error('Invalid configuration');
  }

  // Initialize database
  const db = new VPNDatabase(config.database_url);
  logger.info('Initializing database schema...');
  await db.initializeSchema();
  logger.info('✓ Database schema initialized');

  // Initialize providers in database
  logger.info('Initializing providers...');
  const providers = getSupportedProviders();

  for (const providerName of providers) {
    const metadata = providerMetadata[providerName];

    await db.upsertProvider({
      id: providerName,
      name: providerName,
      display_name: metadata.name,
      cli_available: metadata.cliRequired,
      api_available: false,
      port_forwarding_supported: metadata.portForwarding,
      p2p_all_servers: metadata.p2pServers.includes('All'),
      p2p_server_count: 0,
      total_servers: 0,
      total_countries: 0,
      wireguard_supported: true,
      openvpn_supported: true,
      kill_switch_available: true,
      split_tunneling_available: false,
      config: {},
    });
  }

  logger.info(`✓ Initialized ${providers.length} providers`);

  // Start server
  logger.info(`Starting API server on port ${config.port}...`);
  const server = await startServer(db);
  logger.info(`✓ API server running on http://localhost:${config.port}`);

  // Log startup info
  console.log('');
  console.log('╔═══════════════════════════════════════════════════════════════╗');
  console.log('║          VPN Plugin for nself - v1.0.0                        ║');
  console.log('╚═══════════════════════════════════════════════════════════════╝');
  console.log('');
  console.log(`✓ Server:     http://localhost:${config.port}`);
  console.log(`✓ Health:     http://localhost:${config.port}/health`);
  console.log(`✓ Providers:  ${providers.length} supported`);
  console.log(`✓ Database:   Connected`);
  console.log('');
  console.log('Supported Providers:');
  providers.forEach((p) => {
    const meta = providerMetadata[p];
    const pfIcon = meta.portForwarding ? '🔓' : '  ';
    console.log(`  ${pfIcon} ${meta.name}`);
  });
  console.log('');
  console.log('Quick Start:');
  console.log('  1. Add credentials:  POST /api/providers/:id/credentials');
  console.log('  2. Connect to VPN:   POST /api/connect');
  console.log('  3. Check status:     GET  /api/status');
  console.log('  4. Download torrent: POST /api/download');
  console.log('');
  console.log('CLI Available:');
  console.log('  npx tsx src/cli.ts --help');
  console.log('');

  // Handle graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down...');

    try {
      // Close server
      await server.close();
      logger.info('✓ Server closed');

      // Close database
      await db.close();
      logger.info('✓ Database closed');

      logger.info('✓ Shutdown complete');
      process.exit(0);
    } catch (error) {
      logger.error('Error during shutdown', { error: error instanceof Error ? error.message : String(error) });
      process.exit(1);
    }
  };

  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);

  return { server, db };
}

// Start if run directly
if (import.meta.url === `file://${process.argv[1]}`) {
  initialize().catch((error) => {
    logger.error('Failed to start VPN plugin', error);
    process.exit(1);
  });
}

export { initialize };
export * from './types.js';
export * from './database.js';
export * from './providers/index.js';
