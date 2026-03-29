/**
 * Shopify Plugin Tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 *
 * createServer(config) takes a full ShopifyConfig and returns
 * { app (Fastify), start(), stop() }.  We call app.listen({ port: 0 })
 * to bind to a free port.
 *
 * The Shopify config requires:
 *   - SHOPIFY_SHOP_DOMAIN
 *   - SHOPIFY_ACCESS_TOKEN (must start with "shpat_")
 *   - SHOPIFY_WEBHOOK_SECRET (or SHOPIFY_ALLOW_MISSING_WEBHOOK_SECRET=true)
 *   - POSTGRES_HOST
 *
 * All tests are skipped unless these variables are present.
 */

import { describe, it, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from '../src/server.js';
import { loadConfig } from '../src/config.js';

const HAS_CREDENTIALS =
  !!process.env.POSTGRES_HOST &&
  !!process.env.SHOPIFY_SHOP_DOMAIN &&
  !!process.env.SHOPIFY_ACCESS_TOKEN;

describe('Shopify Plugin', () => {
  let server: Awaited<ReturnType<typeof createServer>> | undefined;
  let baseUrl: string;

  before(async () => {
    if (!HAS_CREDENTIALS) return;

    const config = loadConfig({ port: 0, host: '127.0.0.1' });
    server = await createServer(config);

    // app.listen is called inside start(), but we can also listen manually
    // since start() uses config.port which we set to 0
    await server.start();

    const addr = server.app.server.address();
    const port = typeof addr === 'object' && addr ? (addr as { port: number }).port : 0;
    baseUrl = `http://127.0.0.1:${port}`;
  });

  after(async () => {
    if (server) {
      await server.stop();
    }
  });

  // ------------------------------------------------------------------
  // Health check
  // ------------------------------------------------------------------

  it('GET /health returns 200 with plugin shopify', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as { status: string; plugin: string };
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'shopify');
  });

  it('GET /ready returns 200', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/ready`);
    assert.equal(res.status, 200);
    const body = await res.json() as { ready: boolean };
    assert.ok(typeof body.ready === 'boolean');
  });

  // ------------------------------------------------------------------
  // Sync status
  // ------------------------------------------------------------------

  it('GET /api/status returns status object', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/status`);
    assert.equal(res.status, 200);
    const body = await res.json() as unknown;
    assert.ok(body !== null && typeof body === 'object');
  });

  // ------------------------------------------------------------------
  // Products, Customers, Orders
  // ------------------------------------------------------------------

  it('GET /api/products returns data array shape', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/products`);
    assert.equal(res.status, 200);
    const body = await res.json() as { data: unknown[] };
    assert.ok(Array.isArray(body.data));
  });

  it('GET /api/products/:id with nonexistent id returns 404', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/products/nonexistent-id`);
    assert.equal(res.status, 404);
  });

  it('GET /api/orders returns data array shape', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/orders`);
    assert.equal(res.status, 200);
    const body = await res.json() as { data: unknown[] };
    assert.ok(Array.isArray(body.data));
  });

  it('GET /api/customers returns data array shape', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/customers`);
    assert.equal(res.status, 200);
    const body = await res.json() as { data: unknown[] };
    assert.ok(Array.isArray(body.data));
  });

  // ------------------------------------------------------------------
  // Webhook — invalid HMAC should not crash the server
  // ------------------------------------------------------------------

  it('POST /webhook with invalid signature returns 401', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/webhook`, {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        'x-shopify-hmac-sha256': 'invalidsignature',
        'x-shopify-topic': 'orders/create',
        'x-shopify-shop-domain': 'test.myshopify.com',
      },
      body: JSON.stringify({ id: 1 }),
    });
    assert.ok([400, 401, 403].includes(res.status), `Expected 4xx, got ${res.status}`);
  });
});
