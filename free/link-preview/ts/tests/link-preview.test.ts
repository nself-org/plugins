/**
 * link-preview plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Mocks the database layer so no real Postgres connection is needed.
 * Starts the Fastify server on port 0 (random) for each suite.
 */

import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';

// ---------------------------------------------------------------------------
// Database mock — must be registered before importing server
// ---------------------------------------------------------------------------

const mockDb = {
  connect: async () => {},
  initializeSchema: async () => {},
  query: async () => ({ rows: [] }),
  forSourceAccount: () => mockDb,
  getCurrentSourceAccountId: () => 'primary',
  getCacheStats: async () => ({ total: 0, fresh: 0, stale: 0, failed: 0 }),
  getPreviewByUrl: async () => null,
  upsertPreview: async (data: Record<string, unknown>) => ({ id: 'preview-1', url: data.url, status: data.status, created_at: new Date().toISOString() }),
  getPreview: async () => null,
  deletePreview: async () => false,
  isUrlBlocked: async () => false,
  listBlocklist: async () => [],
  addToBlocklist: async (data: Record<string, unknown>) => ({ id: 'block-1', ...data }),
  removeFromBlocklist: async () => false,
  listTemplates: async () => [],
  createTemplate: async (data: Record<string, unknown>) => ({ id: 'tmpl-1', ...data }),
  getTemplate: async () => null,
  updateTemplate: async () => null,
  deleteTemplate: async () => false,
  listOEmbedProviders: async () => [],
  addOEmbedProvider: async (data: Record<string, unknown>) => ({ id: 'prov-1', ...data }),
  findOEmbedProvider: async () => null,
  getSettings: async () => null,
  upsertSettings: async (data: Record<string, unknown>) => ({ scope: data.scope, enabled: true }),
  getAnalytics: async () => [],
  getPopularPreviews: async () => [],
  trackUsage: async (data: Record<string, unknown>) => ({ id: 'usage-1', ...data }),
  recordClick: async () => false,
  getPreviewsForMessage: async () => [],
  clearCache: async () => 0,
  cleanupExpiredPreviews: async () => 0,
  disconnect: async () => {},
};

await mock.module('../src/database.js', {
  namedExports: {
    LinkPreviewDatabase: class {
      connect = mockDb.connect;
      initializeSchema = mockDb.initializeSchema;
      query = mockDb.query;
      forSourceAccount = mockDb.forSourceAccount;
      getCurrentSourceAccountId = mockDb.getCurrentSourceAccountId;
      getCacheStats = mockDb.getCacheStats;
      getPreviewByUrl = mockDb.getPreviewByUrl;
      upsertPreview = mockDb.upsertPreview;
      getPreview = mockDb.getPreview;
      deletePreview = mockDb.deletePreview;
      isUrlBlocked = mockDb.isUrlBlocked;
      listBlocklist = mockDb.listBlocklist;
      addToBlocklist = mockDb.addToBlocklist;
      removeFromBlocklist = mockDb.removeFromBlocklist;
      listTemplates = mockDb.listTemplates;
      createTemplate = mockDb.createTemplate;
      getTemplate = mockDb.getTemplate;
      updateTemplate = mockDb.updateTemplate;
      deleteTemplate = mockDb.deleteTemplate;
      listOEmbedProviders = mockDb.listOEmbedProviders;
      addOEmbedProvider = mockDb.addOEmbedProvider;
      findOEmbedProvider = mockDb.findOEmbedProvider;
      getSettings = mockDb.getSettings;
      upsertSettings = mockDb.upsertSettings;
      getAnalytics = mockDb.getAnalytics;
      getPopularPreviews = mockDb.getPopularPreviews;
      trackUsage = mockDb.trackUsage;
      recordClick = mockDb.recordClick;
      getPreviewsForMessage = mockDb.getPreviewsForMessage;
      clearCache = mockDb.clearCache;
      cleanupExpiredPreviews = mockDb.cleanupExpiredPreviews;
      disconnect = mockDb.disconnect;
    },
  },
});

// Also mock metadata-fetcher and oembed-service since they make real HTTP calls
await mock.module('../src/metadata-fetcher.js', {
  namedExports: {
    MetadataFetcher: class {
      constructor(_timeout: number) {}
      async fetchMetadata(_url: string) {
        return { title: 'Test Page', description: 'A test page', image: null, siteName: 'test.com', author: null, publishedTime: null, type: 'website', favicon: null, language: 'en', videoUrl: null, audioUrl: null, estimatedReadTime: 1 };
      }
    },
  },
});

await mock.module('../src/oembed-service.js', {
  namedExports: {
    OEmbedService: class {
      constructor(_timeout: number) {}
      getSupportedProviders() { return []; }
      findProvider(_url: string) { return null; }
      async fetchEmbed(_url: string) { return null; }
    },
  },
});

const { createServer } = await import('../src/server.js');

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('link-preview plugin', () => {
  let baseUrl: string;
  let server: Awaited<ReturnType<typeof createServer>>;

  before(async () => {
    server = await createServer({ port: 0, host: '127.0.0.1' });
    await server.app.listen({ port: 0, host: '127.0.0.1' });
    const address = server.app.server.address();
    const port = typeof address === 'object' && address ? address.port : 0;
    baseUrl = `http://127.0.0.1:${port}`;
  });

  after(async () => {
    await server.app.close();
  });

  it('GET /health returns 200 with status ok', async () => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'link-preview');
    assert.ok(typeof body.timestamp === 'string');
  });

  it('GET /api/link-preview without url returns 400', async () => {
    const res = await fetch(`${baseUrl}/api/link-preview`);
    assert.equal(res.status, 400);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(body.error);
  });

  it('POST /api/link-preview/fetch without body url returns 400', async () => {
    const res = await fetch(`${baseUrl}/api/link-preview/fetch`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({}),
    });
    assert.equal(res.status, 400);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(body.error);
  });

  it('POST /api/link-preview/batch rejects empty urls array', async () => {
    const res = await fetch(`${baseUrl}/api/link-preview/batch`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ urls: [] }),
    });
    assert.equal(res.status, 400);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(body.error);
  });

  it('GET /api/link-preview/settings returns default settings', async () => {
    const res = await fetch(`${baseUrl}/api/link-preview/settings`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok('enabled' in body);
  });
});
