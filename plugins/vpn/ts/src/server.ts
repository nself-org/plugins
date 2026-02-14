/**
 * VPN Plugin REST API Server
 * Fastify-based HTTP server for inter-plugin communication
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, loadSecurityConfig } from '@nself/plugin-utils';
import { VPNDatabase } from './database.js';
import { getProvider, getSupportedProviders, providerMetadata, isProviderSupported } from './providers/index.js';
import { config } from './config.js';
import type {
  ConnectVPNRequest,
  ConnectVPNResponse,
  DownloadRequest,
  DownloadResponse,
  VPNStatus,
  ServerListQuery,
  LeakTestResult,
  VPNProvider,
} from './types.js';

const logger = createLogger('vpn:server');

const ENCRYPTION_KEY_RAW = process.env.ENCRYPTION_KEY;
if (!ENCRYPTION_KEY_RAW) {
  throw new Error('ENCRYPTION_KEY environment variable is required. Set a strong random key for credential encryption.');
}
const ENCRYPTION_KEY: string = ENCRYPTION_KEY_RAW;

export async function createServer(db: VPNDatabase) {
  const fastify = Fastify({
    logger: config.log_level === 'debug',
  });

  // Register CORS
  await fastify.register(cors, {
    origin: true,
  });

  // Register authentication and rate limiting
  const securityConfig = loadSecurityConfig('VPN');
  const rateLimiter = new ApiRateLimiter(securityConfig.rateLimitMax, securityConfig.rateLimitWindowMs);

  fastify.addHook('preHandler', createAuthHook(securityConfig.apiKey));
  fastify.addHook('preHandler', createRateLimitHook(rateLimiter));

  // ============================================================================
  // Health Check
  // ============================================================================

  fastify.get('/health', async () => {
    return { status: 'ok', timestamp: new Date().toISOString() };
  });

  // ============================================================================
  // nTV Health Endpoint (leak test info)
  // ============================================================================

  /**
   * GET /api/health
   * Returns VPN connection status and last leak test results.
   * Used by nTV to verify VPN integrity before streaming operations.
   */
  fastify.get('/api/health', async () => {
    const connection = await db.getActiveConnection();
    const vpnConnected = !!connection;

    // Default response when no leak test data is available
    let dnsLeak = false;
    let webrtcLeak = false;
    let ipv6Leak = false;
    let lastTest: string | null = null;

    if (connection) {
      // Fetch the most recent comprehensive leak test for the active connection
      const result = await db.query<{
        passed: boolean;
        details: Record<string, unknown>;
        tested_at: Date;
      }>(
        `SELECT passed, details, tested_at FROM vpn_leak_tests
         WHERE connection_id = $1
         ORDER BY tested_at DESC LIMIT 1`,
        [connection.id]
      );

      if (result.rows.length > 0) {
        const row = result.rows[0];
        lastTest = row.tested_at.toISOString();

        // Parse individual test results from the stored details
        const details = row.details as unknown as LeakTestResult | null;
        const tests = details?.tests;
        dnsLeak = tests?.dns ? !tests.dns.passed : false;
        webrtcLeak = tests?.webrtc ? !tests.webrtc.passed : false;
        ipv6Leak = tests?.ipv6 ? !tests.ipv6.passed : false;
      }
    }

    return {
      vpn_connected: vpnConnected,
      dns_leak: dnsLeak,
      webrtc_leak: webrtcLeak,
      ipv6_leak: ipv6Leak,
      last_test: lastTest,
    };
  });

  // ============================================================================
  // Provider Endpoints
  // ============================================================================

  /**
   * GET /api/providers
   * List all supported providers
   */
  fastify.get('/api/providers', async () => {
    const providers = await db.getAllProviders();

    return {
      providers: providers.map((p) => ({
        id: p.id,
        name: p.display_name,
        cli_available: p.cli_available,
        port_forwarding: p.port_forwarding_supported,
        p2p_servers: p.p2p_server_count,
        total_servers: p.total_servers,
        metadata: providerMetadata[p.name as VPNProvider],
      })),
    };
  });

  /**
   * GET /api/providers/:id
   * Get specific provider details
   */
  fastify.get<{ Params: { id: string } }>('/api/providers/:id', async (request, reply) => {
    const provider = await db.getProvider(request.params.id);

    if (!provider) {
      return reply.code(404).send({ error: 'Provider not found' });
    }

    const hasCredentials = !!(await db.getCredentials(provider.id, ENCRYPTION_KEY));

    return {
      ...provider,
      has_credentials: hasCredentials,
      metadata: providerMetadata[provider.name as VPNProvider],
    };
  });

  /**
   * POST /api/providers/:id/credentials
   * Add provider credentials
   */
  fastify.post<{
    Params: { id: string };
    Body: {
      username?: string;
      password?: string;
      token?: string;
      account_number?: string;
      api_key?: string;
    };
  }>('/api/providers/:id/credentials', {
    schema: {
      body: {
        type: 'object',
        properties: {
          username: { type: 'string' },
          password: { type: 'string' },
          token: { type: 'string' },
          account_number: { type: 'string' },
          api_key: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request, reply) => {
    const { id } = request.params;
    const { username, password, token, account_number, api_key } = request.body;

    const provider = await db.getProvider(id);
    if (!provider) {
      return reply.code(404).send({ error: 'Provider not found' });
    }

    await db.upsertCredentials(
      {
        provider_id: id,
        username,
        password_encrypted: password || '',
        api_token_encrypted: token || '',
        account_number,
        api_key_encrypted: api_key || '',
      },
      ENCRYPTION_KEY
    );

    return { success: true, message: 'Credentials stored' };
  });

  // ============================================================================
  // Server Endpoints
  // ============================================================================

  /**
   * GET /api/servers
   * List available servers
   */
  fastify.get<{ Querystring: ServerListQuery }>('/api/servers', async (request) => {
    const servers = await db.getServers({
      provider: request.query.provider,
      country: request.query.country,
      p2p_only: request.query.p2p_only,
      port_forwarding: request.query.port_forwarding,
      limit: request.query.limit || 100,
    });

    return { servers };
  });

  /**
   * GET /api/servers/p2p
   * List P2P-capable servers
   */
  fastify.get<{ Querystring: Partial<ServerListQuery> }>('/api/servers/p2p', async (request) => {
    const servers = await db.getServers({
      provider: request.query.provider,
      country: request.query.country,
      p2p_only: true,
      limit: request.query.limit || 100,
    });

    return { servers };
  });

  /**
   * POST /api/servers/sync
   * Sync server list from provider
   */
  fastify.post<{
    Body: { provider: string };
  }>('/api/servers/sync', {
    schema: {
      body: {
        type: 'object',
        required: ['provider'],
        properties: {
          provider: { type: 'string', enum: ['nordvpn', 'pia', 'mullvad'] },
        },
        additionalProperties: false,
      },
    },
  }, async (request, reply) => {
    const { provider: providerName } = request.body;

    if (!isProviderSupported(providerName)) {
      return reply.code(400).send({ error: 'Invalid provider' });
    }

    try {
      const provider = getProvider(providerName);
      const credentials = await db.getCredentials(providerName, ENCRYPTION_KEY);

      await provider.initialize();
      if (credentials) {
        await provider.authenticate(credentials);
      }

      const servers = await provider.fetchServers();

      let synced = 0;
      for (const server of servers) {
        await db.upsertServer(server);
        synced++;
      }

      return { success: true, servers_synced: synced };
    } catch (error) {
      logger.error('Server sync failed', { error: error instanceof Error ? error.message : String(error) });
      return reply.code(500).send({
        error: 'Sync failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // ============================================================================
  // Connection Endpoints
  // ============================================================================

  /**
   * POST /api/connect
   * Connect to VPN
   */
  fastify.post<{ Body: ConnectVPNRequest }>('/api/connect', {
    schema: {
      body: {
        type: 'object',
        required: ['provider'],
        properties: {
          provider: { type: 'string', enum: ['nordvpn', 'pia', 'mullvad'] },
          region: { type: 'string' },
          city: { type: 'string' },
          server: { type: 'string' },
          protocol: { type: 'string', enum: ['wireguard', 'openvpn_udp', 'openvpn_tcp', 'ikev2', 'nordlynx', 'lightway'] },
          kill_switch: { type: 'boolean' },
          port_forwarding: { type: 'boolean' },
          requested_by: { type: 'string' },
        },
        additionalProperties: false,
      },
    },
  }, async (request, reply) => {
    const connectRequest = request.body;

    if (!isProviderSupported(connectRequest.provider)) {
      return reply.code(400).send({ error: 'Invalid provider' });
    }

    try {
      // Check if already connected
      const activeConnection = await db.getActiveConnection();
      if (activeConnection) {
        return reply.code(409).send({
          error: 'Already connected',
          connection_id: activeConnection.id,
          provider: activeConnection.provider_id,
        });
      }

      const provider = getProvider(connectRequest.provider);
      const credentials = await db.getCredentials(connectRequest.provider, ENCRYPTION_KEY);

      if (!credentials) {
        return reply.code(401).send({ error: 'No credentials found for provider' });
      }

      await provider.initialize();
      await provider.authenticate(credentials);

      const connection = await provider.connect(connectRequest, credentials);
      await db.createConnection(connection);

      const response: ConnectVPNResponse = {
        connection_id: connection.id,
        provider: connection.provider_id,
        server: connection.server_id || 'unknown',
        vpn_ip: connection.vpn_ip || '',
        interface: connection.interface_name || '',
        dns_servers: connection.dns_servers || [],
        port_forwarded: connection.port_forwarded,
        connected_at: connection.connected_at!,
      };

      return response;
    } catch (error) {
      logger.error('Connection failed', { error: error instanceof Error ? error.message : String(error) });
      return reply.code(500).send({
        error: 'Connection failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  /**
   * POST /api/disconnect
   * Disconnect from VPN
   */
  fastify.post('/api/disconnect', async (request, reply) => {
    try {
      const connection = await db.getActiveConnection();

      if (!connection) {
        return reply.code(404).send({ error: 'No active connection' });
      }

      const provider = getProvider(connection.provider_id as VPNProvider);
      await provider.initialize();
      await provider.disconnect(connection.id);

      await db.updateConnection(connection.id, {
        status: 'disconnected',
        disconnected_at: new Date(),
        duration_seconds: Math.floor((Date.now() - connection.connected_at!.getTime()) / 1000),
      });

      return { success: true, message: 'Disconnected' };
    } catch (error) {
      logger.error('Disconnect failed', { error: error instanceof Error ? error.message : String(error) });
      return reply.code(500).send({
        error: 'Disconnect failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  /**
   * GET /api/status
   * Get connection status
   */
  fastify.get('/api/status', async (request, reply) => {
    try {
      const connection = await db.getActiveConnection();

      if (!connection) {
        return { connected: false };
      }

      const provider = getProvider(connection.provider_id as VPNProvider);
      await provider.initialize();
      const status = await provider.getStatus();

      return {
        ...status,
        connection_id: connection.id,
        provider: connection.provider_id,
      };
    } catch (error) {
      logger.error('Status check failed', { error: error instanceof Error ? error.message : String(error) });
      return reply.code(500).send({
        error: 'Status check failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // ============================================================================
  // Download Endpoints
  // ============================================================================

  /**
   * POST /api/download
   * Start torrent download via VPN
   */
  fastify.post<{ Body: DownloadRequest }>('/api/download', {
    schema: {
      body: {
        type: 'object',
        required: ['magnet_link', 'requested_by'],
        properties: {
          magnet_link: { type: 'string', minLength: 1 },
          destination: { type: 'string' },
          provider: { type: 'string', enum: ['nordvpn', 'pia', 'mullvad'] },
          region: { type: 'string' },
          requested_by: { type: 'string', minLength: 1 },
        },
        additionalProperties: false,
      },
    },
  }, async (request, reply) => {
    const { magnet_link, destination, provider: providerName, region, requested_by } = request.body;

    try {
      // Ensure VPN is connected
      let connection = await db.getActiveConnection();

      if (!connection && providerName) {
        // Auto-connect if provider specified
        if (!isProviderSupported(providerName)) {
          return reply.code(400).send({ error: 'Invalid provider' });
        }

        const provider = getProvider(providerName);
        const credentials = await db.getCredentials(providerName, ENCRYPTION_KEY);

        if (!credentials) {
          return reply.code(401).send({ error: 'No credentials for provider' });
        }

        await provider.initialize();
        await provider.authenticate(credentials);

        connection = await provider.connect(
          {
            provider: providerName,
            region,
            kill_switch: true,
            requested_by: 'download-auto-connect',
          },
          credentials
        );

        await db.createConnection(connection);
      }

      if (!connection) {
        return reply.code(400).send({ error: 'No VPN connection. Connect first or specify provider.' });
      }

      // Extract info hash from magnet link
      const infoHashMatch = magnet_link.match(/urn:btih:([a-fA-F0-9]{40})/);
      const infoHash = infoHashMatch ? infoHashMatch[1] : magnet_link.slice(0, 40);

      // Create download record
      const download = await db.createDownload({
        connection_id: connection.id,
        magnet_link,
        info_hash: infoHash,
        destination_path: destination || config.download_path,
        status: 'queued',
        requested_by,
        provider_id: connection.provider_id,
        server_id: connection.server_id,
      });

      const response: DownloadResponse = {
        download_id: download.id,
        name: download.name,
        status: download.status,
        provider: connection.provider_id,
        server: connection.server_id,
        created_at: download.created_at,
      };

      // NOTE: Torrent download requires integration with torrent-manager plugin
      // Integration requirements:
      // 1. Forward download request to torrent-manager plugin API
      // 2. Include VPN interface binding information for network isolation
      // 3. Monitor download progress via torrent-manager webhooks
      //
      // Example implementation:
      // await axios.post(`${config.torrent_manager_url}/api/downloads`, {
      //   magnet_link,
      //   destination_path: destination,
      //   vpn_interface: connection.interface,
      //   vpn_ip: connection.vpn_ip
      // });

      return response;
    } catch (error) {
      logger.error('Download failed', { error: error instanceof Error ? error.message : String(error) });
      return reply.code(500).send({
        error: 'Download failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  /**
   * GET /api/downloads
   * List downloads
   */
  fastify.get<{
    Querystring: { status?: string; limit?: number };
  }>('/api/downloads', async (request) => {
    const limit = request.query.limit || 100;
    const downloads = await db.getAllDownloads(limit);

    return { downloads };
  });

  /**
   * GET /api/downloads/:id
   * Get download status
   */
  fastify.get<{ Params: { id: string } }>('/api/downloads/:id', async (request, reply) => {
    const download = await db.getDownload(request.params.id);

    if (!download) {
      return reply.code(404).send({ error: 'Download not found' });
    }

    return download;
  });

  /**
   * DELETE /api/downloads/:id
   * Cancel download
   */
  fastify.delete<{ Params: { id: string } }>('/api/downloads/:id', async (request, reply) => {
    const download = await db.getDownload(request.params.id);

    if (!download) {
      return reply.code(404).send({ error: 'Download not found' });
    }

    if (download.status === 'completed') {
      return reply.code(400).send({ error: 'Cannot cancel completed download' });
    }

    await db.updateDownload(request.params.id, {
      status: 'cancelled',
      error_message: 'Cancelled by user',
    });

    // NOTE: Torrent cancellation requires integration with torrent-manager plugin
    // Integration requirements:
    // 1. Send DELETE request to torrent-manager plugin to stop/remove torrent
    // 2. Cleanup partial files if requested
    // 3. Update download status based on torrent-manager response
    //
    // Example implementation:
    // await axios.delete(`${config.torrent_manager_url}/api/downloads/${download.torrent_id}`);

    return { success: true, message: 'Download cancelled' };
  });

  // ============================================================================
  // Security Endpoints
  // ============================================================================

  /**
   * POST /api/test-leak
   * Run leak test
   */
  fastify.post('/api/test-leak', async (request, reply) => {
    try {
      const connection = await db.getActiveConnection();

      if (!connection) {
        return reply.code(400).send({ error: 'No active VPN connection' });
      }

      const provider = getProvider(connection.provider_id as VPNProvider);
      await provider.initialize();
      const result = await provider.testLeaks();

      // Store result
      await db.query(
        `INSERT INTO vpn_leak_tests (connection_id, test_type, passed, expected_value, actual_value, details)
         VALUES ($1, $2, $3, $4, $5, $6)`,
        [
          connection.id,
          'comprehensive',
          result.passed,
          'no leaks',
          JSON.stringify(result.tests),
          JSON.stringify(result),
        ]
      );

      return result;
    } catch (error) {
      logger.error('Leak test failed', { error: error instanceof Error ? error.message : String(error) });
      return reply.code(500).send({
        error: 'Leak test failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      });
    }
  });

  // ============================================================================
  // Statistics Endpoints
  // ============================================================================

  /**
   * GET /api/stats
   * Get usage statistics
   */
  fastify.get('/api/stats', async () => {
    const stats = await db.getStatistics();
    return stats;
  });

  // ============================================================================
  // Error Handler
  // ============================================================================

  fastify.setErrorHandler((error, request, reply) => {
    logger.error('Request error', {
      method: request.method,
      url: request.url,
      error: error.message,
    });

    reply.code(500).send({
      error: 'Internal server error',
      message: error.message,
    });
  });

  return fastify;
}

/**
 * Start server
 */
export async function startServer(db: VPNDatabase) {
  const server = await createServer(db);

  try {
    await server.listen({
      port: config.port,
      host: '0.0.0.0',
    });

    logger.info(`VPN Plugin API server listening on port ${config.port}`);
    logger.info(`Health check: http://localhost:${config.port}/health`);
    logger.info(`API documentation: http://localhost:${config.port}/api/*`);

    return server;
  } catch (error) {
    logger.error('Failed to start server', { error: error instanceof Error ? error.message : String(error) });
    throw error;
  }
}
