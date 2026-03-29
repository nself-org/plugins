/**
 * Tests for stripe plugin
 * Uses node:test + node:assert (zero external dependencies)
 *
 * Skips tests that need a real Stripe API key or PostgreSQL.
 * Set POSTGRES_HOST and STRIPE_API_KEY (sk_test_...) to run the full suite.
 */

import { test, describe, before, after } from 'node:test';
import assert from 'node:assert/strict';

const HAS_DB = Boolean(process.env.POSTGRES_HOST);
const HAS_STRIPE = Boolean(process.env.STRIPE_API_KEY);
const CAN_RUN = HAS_DB && HAS_STRIPE;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function fetchJson(
  url: string,
  options: { method?: string; body?: unknown; headers?: Record<string, string> } = {}
): Promise<{ status: number; body: unknown }> {
  const { method = 'GET', body, headers = {} } = options;
  const res = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json', ...headers },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    parsed = text;
  }
  return { status: res.status, body: parsed };
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

describe('stripe plugin (requires DB + Stripe key)', { skip: !CAN_RUN }, () => {
  let server: {
    app: import('fastify').FastifyInstance;
    db: { disconnect: () => Promise<void> };
    stop: () => Promise<void>;
  };
  let baseUrl: string;

  before(async () => {
    const { createServer } = await import('../src/server.js');
    server = await createServer({
      port: 0,
      host: '127.0.0.1',
      security: { apiKey: '', rateLimitMax: 1000, rateLimitWindowMs: 60000 },
    });
    await server.app.listen({ port: 0, host: '127.0.0.1' });
    const addr = server.app.server.address();
    const port = typeof addr === 'object' && addr ? addr.port : 0;
    baseUrl = `http://127.0.0.1:${port}`;
  });

  after(async () => {
    await server.app.close();
    await server.db.disconnect();
  });

  // -------------------------------------------------------------------------
  // Health
  // -------------------------------------------------------------------------

  test('GET /health returns 200 with plugin name', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/health`);
    assert.equal(status, 200);
    assert.equal((body as Record<string, string>).status, 'ok');
    assert.equal((body as Record<string, string>).plugin, 'stripe');
  });

  test('GET /status returns plugin info', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/status`);
    assert.equal(status, 200);
    assert.equal((body as Record<string, string>).plugin, 'stripe');
    assert.ok(Array.isArray((body as Record<string, unknown>).accounts));
  });

  // -------------------------------------------------------------------------
  // Data endpoints (empty DB is fine — just check shape)
  // -------------------------------------------------------------------------

  test('GET /api/customers returns paginated data', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/api/customers`);
    assert.equal(status, 200);
    assert.ok(Array.isArray((body as Record<string, unknown>).data));
    assert.ok(typeof (body as Record<string, unknown>).total === 'number');
  });

  test('GET /api/products returns paginated data', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/api/products`);
    assert.equal(status, 200);
    assert.ok(Array.isArray((body as Record<string, unknown>).data));
  });

  test('GET /api/subscriptions returns paginated data', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/api/subscriptions`);
    assert.equal(status, 200);
    assert.ok(Array.isArray((body as Record<string, unknown>).data));
  });

  test('GET /api/invoices returns paginated data', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/api/invoices`);
    assert.equal(status, 200);
    assert.ok(Array.isArray((body as Record<string, unknown>).data));
  });

  test('GET /api/customers/:id returns 404 for unknown id', async () => {
    const { status } = await fetchJson(`${baseUrl}/api/customers/cus_nonexistent`);
    assert.equal(status, 404);
  });

  test('GET /api/products/:id returns 404 for unknown id', async () => {
    const { status } = await fetchJson(`${baseUrl}/api/products/prod_nonexistent`);
    assert.equal(status, 404);
  });

  // -------------------------------------------------------------------------
  // Webhook
  // -------------------------------------------------------------------------

  test('POST /webhooks/stripe requires stripe-signature header', async () => {
    const { status } = await fetchJson(`${baseUrl}/webhooks/stripe`, {
      method: 'POST',
      body: { type: 'payment_intent.succeeded' },
    });
    assert.equal(status, 400);
  });

  test('POST /webhooks/stripe rejects invalid signature', async () => {
    const { status } = await fetchJson(`${baseUrl}/webhooks/stripe`, {
      method: 'POST',
      headers: { 'stripe-signature': 'v1=invalidsig,t=1234567890' },
      body: { type: 'payment_intent.succeeded' },
    });
    // 400 (no secret configured) or 401 (bad sig)
    assert.ok(status === 400 || status === 401, `Expected 400 or 401, got ${status}`);
  });

  // -------------------------------------------------------------------------
  // Stats
  // -------------------------------------------------------------------------

  test('GET /api/stats returns stats object', async () => {
    const { status, body } = await fetchJson(`${baseUrl}/api/stats`);
    assert.equal(status, 200);
    assert.ok(body !== null && typeof body === 'object');
  });
});

// ---------------------------------------------------------------------------
// Config validation tests (no external services needed)
// ---------------------------------------------------------------------------

describe('stripe config validation', () => {
  test('loadConfig throws when STRIPE_API_KEY is missing', async () => {
    const { loadConfig } = await import('../src/config.js');
    const original = process.env.STRIPE_API_KEY;
    const originalKeys = process.env.STRIPE_API_KEYS;
    delete process.env.STRIPE_API_KEY;
    delete process.env.STRIPE_API_KEYS;

    try {
      assert.throws(() => loadConfig(), /STRIPE_API_KEY/i);
    } finally {
      if (original !== undefined) process.env.STRIPE_API_KEY = original;
      if (originalKeys !== undefined) process.env.STRIPE_API_KEYS = originalKeys;
    }
  });

  test('isTestMode identifies test keys correctly', async () => {
    const { isTestMode, isLiveMode } = await import('../src/config.js');
    assert.ok(isTestMode('sk_test_abc123'));
    assert.ok(isTestMode('rk_test_abc123'));
    assert.ok(!isTestMode('sk_live_abc123'));
    assert.ok(isLiveMode('sk_live_abc123'));
    assert.ok(!isLiveMode('sk_test_abc123'));
  });
});
