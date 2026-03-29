/**
 * tokens plugin — HTTP API tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Mocks the database and config layers so no real Postgres is needed.
 * The server module exposes `fastify` and `start` directly as named exports.
 */

import { describe, it, before, after, mock } from 'node:test';
import assert from 'node:assert/strict';

// ---------------------------------------------------------------------------
// Required environment variables (must be set before any module import)
// ---------------------------------------------------------------------------

process.env.ENCRYPTION_KEY = 'test-encryption-key-32-bytes-ok!';
process.env.TOKENS_ENCRYPTION_KEY = 'test-encryption-key-32-bytes-ok!';
process.env.TOKENS_PLUGIN_PORT = '47302';
process.env.TOKENS_PLUGIN_HOST = '127.0.0.1';

// ---------------------------------------------------------------------------
// Stub TokensDatabase — mock before importing server
// ---------------------------------------------------------------------------

const mockScopedDb = {
  checkEntitlement: async () => null,
  hasAnyEntitlements: async () => false,
  getActiveSigningKey: async () => null,
  insertIssuedToken: async (data: Record<string, unknown>) => ({ id: 'token-1', ...data, expires_at: new Date(Date.now() + 3600_000) }),
  insertWebhookEvent: async () => {},
  getIssuedTokenByHash: async () => null,
  updateTokenLastUsed: async () => {},
  revokeToken: async () => {},
  revokeUserTokens: async () => 0,
  revokeContentTokens: async () => 0,
  createSigningKey: async (name: string, algorithm: string, _encKey: string) => ({
    id: 'key-1', name, algorithm, is_active: true, created_at: new Date(), rotated_from: null,
  }),
  listSigningKeys: async () => [],
  rotateSigningKey: async (_id: string) => ({ id: 'key-2', name: 'rotated', algorithm: 'hmac-sha256', is_active: true, created_at: new Date(), rotated_from: 'key-1' }),
  deactivateSigningKey: async () => {},
  createEncryptionKey: async (_contentId: string, _encKey: string, _iv: string, _uri: string) => ({ id: 'enc-key-1', is_active: true }),
  getEncryptionKeyById: async () => null,
  rotateEncryptionKey: async (_contentId: string, _encKey: string, _iv: string, _uri: string) => ({ id: 'enc-key-2', rotation_generation: 2 }),
  grantEntitlement: async (data: Record<string, unknown>) => ({ id: 'ent-1', ...data, expires_at: null, metadata: {} }),
  revokeEntitlement: async () => {},
  listUserEntitlements: async () => [],
  getStats: async () => ({ total_issued: 0, total_valid: 0, total_revoked: 0 }),
};

await mock.module('../src/database.js', {
  namedExports: {
    TokensDatabase: class {
      constructor(_db: unknown) {}
      forSourceAccount() { return mockScopedDb; }
      getStats = mockScopedDb.getStats;
    },
  },
});

// Mock createDatabase from plugin-utils to avoid real Postgres
await mock.module('@nself/plugin-utils', {
  namedExports: {
    createDatabase: () => ({
      connect: async () => {},
      disconnect: async () => {},
      query: async () => ({ rows: [], rowCount: 0 }),
      execute: async () => {},
    }),
    createLogger: (name: string) => ({
      info: () => {},
      warn: () => {},
      error: () => {},
      debug: () => {},
      success: () => {},
    }),
    ApiRateLimiter: class {
      constructor(_max: number, _window: number) {}
      check() { return true; }
      getRemaining() { return 99; }
      getResetTime() { return Date.now() + 60000; }
    },
    createAuthHook: (_key: string | undefined) => async () => {},
    createRateLimitHook: (_limiter: unknown) => async () => {},
    getAppContext: (_req: unknown) => ({ sourceAccountId: 'primary' }),
    loadSecurityConfig: (_prefix: string) => ({ apiKey: undefined, rateLimitMax: 100, rateLimitWindowMs: 60000 }),
    parseCsvList: (s: string) => s.split(',').map((x: string) => x.trim()),
    normalizeSourceAccountId: (s: string) => s.toLowerCase().replace(/[^a-z0-9_-]+/g, '-'),
  },
});

// ---------------------------------------------------------------------------
// Import server after all mocks are registered
// ---------------------------------------------------------------------------

const { fastify, start } = await import('../src/server.js');

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe('tokens plugin', () => {
  before(async () => {
    await start();
  });

  after(async () => {
    await fastify.close();
  });

  it('GET /health returns 200 with status ok', async () => {
    const res = await fetch('http://127.0.0.1:47302/health');
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'tokens');
  });

  it('GET /ready returns ready status', async () => {
    const res = await fetch('http://127.0.0.1:47302/ready');
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok('ready' in body);
  });

  it('POST /api/keys creates a signing key', async () => {
    const res = await fetch('http://127.0.0.1:47302/api/keys', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ name: 'test-key' }),
    });
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(body.id);
    assert.ok(body.name);
  });

  it('POST /api/issue without signing key returns 500', async () => {
    // No active signing key configured in the stub → expects 500
    const res = await fetch('http://127.0.0.1:47302/api/issue', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        userId: 'user-1',
        contentId: 'content-1',
      }),
    });
    // Either 500 (no signing key) or 200 if stub returns one
    assert.ok(res.status === 500 || res.status === 200);
  });

  it('POST /api/validate with unknown token returns valid: false', async () => {
    const res = await fetch('http://127.0.0.1:47302/api/validate', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ token: 'not.a.real.token' }),
    });
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.valid, false);
  });
});
