/**
 * torrent-manager plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Passes stub database and mocks external clients (Transmission, search)
 * directly so no real Postgres, Transmission, or internet access is needed.
 */

import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';

// ---------------------------------------------------------------------------
// Mock external clients before importing server
// ---------------------------------------------------------------------------

// Mock Transmission client — avoid real connection to torrent daemon
await mock.module('../src/clients/transmission.js', {
  namedExports: {
    TransmissionClient: class {
      constructor(_host: string, _port: number, _user?: string, _pass?: string) {}
      async connect() { return false; }
      async isConnected() { return false; }
      async addTorrent(_magnet: string, _opts: unknown) { return { id: 'dl-1', name: 'Test Torrent', status: 'downloading', progress: 0, size: 0, downloaded: 0, uploaded: 0, speed_down: 0, speed_up: 0, ratio: 0, eta: 0, hash: 'abc123', magnet_uri: _magnet, added_at: new Date().toISOString(), client_id: '', requested_by: '' }; }
      async pauseTorrent(_id: string) {}
      async resumeTorrent(_id: string) {}
      async removeTorrent(_id: string, _deleteFiles: boolean) {}
      async getStats() { return { active_downloads: 0, total_torrents: 0 }; }
    },
  },
});

// Mock search aggregator — avoid real torrent site scraping
await mock.module('../src/search/aggregator.js', {
  namedExports: {
    TorrentSearchAggregator: class {
      constructor(_sources?: string[]) {}
      async search(_params: unknown) { return []; }
      async getMagnetLink(_torrent: unknown) { return ''; }
    },
  },
});

// Mock smart matcher
await mock.module('../src/matching/smart-matcher.js', {
  namedExports: {
    SmartMatcher: class {
      findBestMatch(_results: unknown[], _criteria: unknown) { return null; }
    },
  },
});

// Mock VPN checker — avoid real HTTP to vpn plugin
await mock.module('../src/vpn-checker.js', {
  namedExports: {
    VPNChecker: class {
      constructor(_url?: string) {}
      async isVPNActive() { return true; }
      startMonitoring(_cb: () => void) {}
      stopMonitoring() {}
    },
  },
});

// Mock source registry
await mock.module('../src/sources/registry.js', {
  namedExports: {
    getAllSources: () => [],
  },
});

const { TorrentManagerServer } = await import('../src/server.js');

// ---------------------------------------------------------------------------
// Stub database
// ---------------------------------------------------------------------------

const mockDatabase = {
  connect: async () => {},
  disconnect: async () => {},
  listClients: async () => [],
  getDefaultClient: async () => null,
  createDownload: async (data: Record<string, unknown>) => ({ id: 'dl-1', name: 'Test', status: 'downloading', ...data }),
  listDownloads: async () => [],
  getDownload: async () => null,
  updateDownload: async () => {},
  deleteDownload: async () => {},
  getSearchCache: async () => null,
  upsertDownloadSeedingPolicy: async () => {},
  getDownloadSeedingPolicy: async () => null,
  getStats: async () => ({ total_downloads: 0, completed: 0, active: 0, total_size_bytes: 0 }),
};

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('torrent-manager plugin', () => {
  const TEST_PORT = 47303;
  let server: InstanceType<typeof TorrentManagerServer>;
  const baseUrl = `http://127.0.0.1:${TEST_PORT}`;

  before(async () => {
    const config = {
      port: TEST_PORT,
      host: '127.0.0.1',
      default_client: 'none',
      transmission_host: '127.0.0.1',
      transmission_port: 9091,
      transmission_username: '',
      transmission_password: '',
      vpn_required: false,
      vpn_manager_url: 'http://127.0.0.1:3200',
      enabled_sources: '',
      download_path: '/tmp/torrent-test',
      database_url: 'postgresql://unused:unused@127.0.0.1:5432/unused',
    };
    server = new TorrentManagerServer(config as never, mockDatabase as never);
    await server.initialize();
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
  });

  it('GET /v1/clients returns empty clients list', async () => {
    const res = await fetch(`${baseUrl}/v1/clients`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.clients));
  });

  it('GET /v1/downloads returns empty downloads list', async () => {
    const res = await fetch(`${baseUrl}/v1/downloads`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.downloads));
    assert.equal(body.total, 0);
  });

  it('POST /v1/search missing query returns 400', async () => {
    const res = await fetch(`${baseUrl}/v1/search`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({}),
    });
    assert.equal(res.status, 400);
  });

  it('GET /v1/stats returns stats object', async () => {
    const res = await fetch(`${baseUrl}/v1/stats`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok('database' in body);
    assert.ok('timestamp' in body);
  });
});
