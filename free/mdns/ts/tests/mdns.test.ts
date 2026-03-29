/**
 * mdns plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Mocks the database layer and mDNS network discovery so no
 * real Postgres connection or network scan is needed.
 */

import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';

// ---------------------------------------------------------------------------
// Database mock
// ---------------------------------------------------------------------------

const mockStats = { total_services: 0, advertised_services: 0, total_discovered: 0 };

const mockDb = {
  connect: async () => {},
  initializeSchema: async () => {},
  query: async () => ({ rows: [], rowCount: 0 }),
  disconnect: async () => {},
  forSourceAccount: () => mockDb,
  getCurrentSourceAccountId: () => 'primary',
  getStats: async () => mockStats,
  createService: async (data: Record<string, unknown>) => ({ id: 'svc-1', ...data }),
  listServices: async () => [],
  getService: async () => null,
  updateService: async () => null,
  deleteService: async () => false,
  setAdvertised: async () => null,
  upsertDiscovery: async () => {},
  listDiscoveries: async () => [],
};

await mock.module('../src/database.js', {
  namedExports: {
    MdnsDatabase: class {
      connect = mockDb.connect;
      initializeSchema = mockDb.initializeSchema;
      query = mockDb.query;
      disconnect = mockDb.disconnect;
      forSourceAccount = mockDb.forSourceAccount;
      getCurrentSourceAccountId = mockDb.getCurrentSourceAccountId;
      getStats = mockDb.getStats;
      createService = mockDb.createService;
      listServices = mockDb.listServices;
      getService = mockDb.getService;
      updateService = mockDb.updateService;
      deleteService = mockDb.deleteService;
      setAdvertised = mockDb.setAdvertised;
      upsertDiscovery = mockDb.upsertDiscovery;
      listDiscoveries = mockDb.listDiscoveries;
    },
  },
});

// Mock real mDNS network discovery — skip actual multicast
await mock.module('../src/discovery.js', {
  namedExports: {
    discoverServices: async () => [],
  },
});

// ---------------------------------------------------------------------------
// Import server after mocks are registered
// ---------------------------------------------------------------------------

const { createServer } = await import('../src/server.js');

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('mdns plugin', () => {
  // We use a fixed high port for this plugin since createServer
  // does not expose the Fastify app instance directly.
  const TEST_PORT = 47299;
  let server: Awaited<ReturnType<typeof createServer>>;
  const baseUrl = `http://127.0.0.1:${TEST_PORT}`;

  before(async () => {
    server = await createServer({ port: TEST_PORT, host: '127.0.0.1' });
    await server.start();
  });

  after(async () => {
    await server.stop();
  });

  it('GET /health returns 200 with status ok', async () => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'mdns');
  });

  it('GET /api/services returns empty list', async () => {
    const res = await fetch(`${baseUrl}/api/services`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.services));
    assert.equal(body.count, 0);
  });

  it('POST /api/services creates a service record', async () => {
    const res = await fetch(`${baseUrl}/api/services`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        service_name: 'test-service',
        service_type: '_http._tcp',
        port: 8080,
      }),
    });
    assert.equal(res.status, 201);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.service_name, 'test-service');
  });

  it('GET /api/discovered returns empty discoveries list', async () => {
    const res = await fetch(`${baseUrl}/api/discovered`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.discoveries));
  });

  it.skip('POST /api/discover performs real mDNS scan (network-dependent)', async () => {
    // Skipped: real mDNS discovery requires multicast-capable network interface.
    // The discovery module is mocked for unit tests; integration tests
    // should run on a host with actual mDNS-discoverable services present.
  });
});
