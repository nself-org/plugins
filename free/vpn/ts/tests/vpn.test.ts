/**
 * vpn plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Mocks the config, database, and provider layers so no real VPN
 * software, Postgres connection, or network access is needed.
 *
 * Two TODOs from the implementation (torrent-manager integration):
 * - POST /api/download actual forwarding to torrent-manager
 * - DELETE /api/downloads/:id cancellation via torrent-manager
 * These are represented as test.todo() below.
 */

import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';

// ---------------------------------------------------------------------------
// Required environment variables — must be set before any module imports
// ---------------------------------------------------------------------------

process.env.ENCRYPTION_KEY = 'vpn-test-encryption-key-32chars!';
process.env.DATABASE_URL = 'postgresql://unused:unused@127.0.0.1:5432/unused';
process.env.LOG_LEVEL = 'error';
process.env.PORT = '47304';

// ---------------------------------------------------------------------------
// Mock config to avoid real DB URL validation side effects
// ---------------------------------------------------------------------------

await mock.module('../src/config.js', {
  namedExports: {
    config: {
      database_url: 'postgresql://unused:unused@127.0.0.1:5432/unused',
      default_provider: undefined,
      default_region: undefined,
      download_path: '/tmp/vpn-test-downloads',
      enable_kill_switch: true,
      enable_auto_reconnect: true,
      server_carousel_enabled: false,
      carousel_interval_minutes: 60,
      port: 47304,
      log_level: 'error',
      torrent_manager_url: 'http://127.0.0.1:47305',
    },
    loadConfig: () => ({
      database_url: 'postgresql://unused:unused@127.0.0.1:5432/unused',
      port: 47304,
      log_level: 'error',
      torrent_manager_url: 'http://127.0.0.1:47305',
    }),
    validateConfig: () => true,
  },
});

// ---------------------------------------------------------------------------
// Mock VPN providers — avoid importing real VPN CLI wrappers
// ---------------------------------------------------------------------------

const mockProvider = {
  initialize: async () => {},
  authenticate: async () => {},
  connect: async (_req: unknown, _creds: unknown) => ({
    id: 'conn-1',
    provider_id: 'nordvpn',
    status: 'connected',
    server_id: 'server-1',
    vpn_ip: '10.0.0.1',
    interface_name: 'tun0',
    dns_servers: ['1.1.1.1'],
    port_forwarded: false,
    connected_at: new Date(),
  }),
  disconnect: async (_id: string) => {},
  getStatus: async () => ({ connected: true, vpn_ip: '10.0.0.1' }),
  fetchServers: async () => [],
  testLeaks: async () => ({ passed: true, tests: {} }),
};

await mock.module('../src/providers/index.js', {
  namedExports: {
    getProvider: (_name: string) => mockProvider,
    getSupportedProviders: () => ['nordvpn', 'pia', 'mullvad'],
    isProviderSupported: (name: string) => ['nordvpn', 'pia', 'mullvad'].includes(name),
    providerMetadata: {
      nordvpn: { name: 'NordVPN', features: [] },
      pia: { name: 'PIA', features: [] },
      mullvad: { name: 'Mullvad', features: [] },
    },
  },
});

// ---------------------------------------------------------------------------
// Stub VPNDatabase — injected into createServer
// ---------------------------------------------------------------------------

const mockDb = {
  query: async (_sql: string, _params?: unknown[]) => ({ rows: [], rowCount: 0 }),
  getAllProviders: async () => [],
  getProvider: async (_id: string) => null,
  getCredentials: async (_providerId: string, _encKey: string) => null,
  upsertCredentials: async () => {},
  getServers: async () => [],
  upsertServer: async () => {},
  getActiveConnection: async () => null,
  createConnection: async (data: Record<string, unknown>) => ({ id: 'conn-1', ...data }),
  updateConnection: async () => {},
  createDownload: async (data: Record<string, unknown>) => ({
    id: 'dl-1',
    name: 'test',
    status: 'queued',
    created_at: new Date().toISOString(),
    ...data,
  }),
  getAllDownloads: async () => [],
  getDownload: async (_id: string) => null,
  updateDownload: async () => {},
  getStatistics: async () => ({ total_connections: 0, total_downloads: 0 }),
};

// ---------------------------------------------------------------------------
// Import server after mocks are registered
// ---------------------------------------------------------------------------

const { createServer } = await import('../src/server.js');

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('vpn plugin', () => {
  const TEST_PORT = 47304;
  let fastify: Awaited<ReturnType<typeof createServer>>;
  const baseUrl = `http://127.0.0.1:${TEST_PORT}`;

  // --- MOCK TORRENT MANAGER SERVER ---
  let tmServer: http.Server;
  let tmMockRequests: any[] = [];
  const TM_PORT = 47305;

  before(async () => {
    // Start the mock torrent manager
    tmServer = http.createServer((req, res) => {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        tmMockRequests.push({ method: req.method, url: req.url, body });
        res.writeHead(200, { 'Content-Type': 'application/json' });
        if (req.method === 'POST') {
          res.end(JSON.stringify({ id: 'tm-dl-123', status: 'downloading' }));
        } else {
          res.end(JSON.stringify({ ok: true }));
        }
      });
    });
    
    await new Promise<void>((resolve) => tmServer.listen(TM_PORT, '127.0.0.1', resolve));

    fastify = await createServer(mockDb as never);
    await fastify.listen({ port: TEST_PORT, host: '127.0.0.1' });
  });

  after(async () => {
    await fastify.close();
    tmServer.close();
  });

  it('GET /health returns 200 with status ok', async () => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
  });

  it('GET /api/health returns VPN connection status', async () => {
    const res = await fetch(`${baseUrl}/api/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok('vpn_connected' in body);
    assert.ok('dns_leak' in body);
  });

  it('GET /api/providers returns provider list', async () => {
    const res = await fetch(`${baseUrl}/api/providers`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.providers));
  });

  it('GET /api/servers returns server list', async () => {
    const res = await fetch(`${baseUrl}/api/servers`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.servers));
  });

  it('POST /api/connect with invalid provider returns 4xx or 5xx error', async () => {
    // The VPN server uses a custom error handler that returns 500 for all errors,
    // including Fastify schema validation errors. An invalid provider enum value
    // fails AJV validation and is caught by setErrorHandler → 500.
    const res = await fetch(`${baseUrl}/api/connect`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ provider: 'invalid-vpn' }),
    });
    assert.ok(res.status >= 400, `Expected error status, got ${res.status}`);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(body.error);
  });

  it('POST /api/download forwards torrent to torrent-manager', async () => {
    tmMockRequests = [];
    
    // Simulate active connection to bypass vpn check
    mockDb.getActiveConnection = async () => ({ id: 'conn-1', status: 'connected', vpn_ip: '10.0.0.1' }) as any;
    
    const res = await fetch(`${baseUrl}/api/download`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        url: 'magnet:?xt=urn:btih:mock',
        type: 'torrent',
        name: 'test torrent'
      }),
    });
    
    assert.equal(res.status, 202);
    
    // Check that we hit the mock torrent manager
    assert.equal(tmMockRequests.length, 1);
    assert.equal(tmMockRequests[0].method, 'POST');
    assert.equal(tmMockRequests[0].url, '/api/downloads');
    const forwardedBody = JSON.parse(tmMockRequests[0].body);
    assert.equal(forwardedBody.url, 'magnet:?xt=urn:btih:mock');
  });

  it('DELETE /api/downloads/:id cancels via torrent-manager if tm tracking id exists', async () => {
    tmMockRequests = [];
    
    mockDb.getDownload = async () => ({
      id: 'dl-1',
      status: 'downloading',
      metadata: { torrent_manager_id: 'tm-dl-123' }
    }) as any;
    
    const res = await fetch(`${baseUrl}/api/downloads/dl-1`, { method: 'DELETE' });
    assert.equal(res.status, 200);
    
    // Should have sent a DELETE to the mock TM
    assert.equal(tmMockRequests.length, 1);
    assert.equal(tmMockRequests[0].method, 'DELETE');
    assert.equal(tmMockRequests[0].url, '/api/downloads/tm-dl-123');
  });
});
