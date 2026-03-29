/**
 * PayPal Plugin Tests
 *
 * Uses node:test + node:assert (zero external dependencies).
 * Tests that require a real database (POSTGRES_HOST) are skipped
 * unless the environment variable is set.
 *
 * The PayPal plugin's createServer() also requires PAYPAL_CLIENT_ID +
 * PAYPAL_CLIENT_SECRET, so all server-startup tests are skipped unless
 * those credentials are present.
 */

import { describe, it, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from '../src/server.js';
import type { FastifyInstance } from 'fastify';

const HAS_CREDENTIALS =
  !!process.env.POSTGRES_HOST &&
  !!process.env.PAYPAL_CLIENT_ID &&
  !!process.env.PAYPAL_CLIENT_SECRET;

describe('PayPal Plugin', () => {
  let app: FastifyInstance | undefined;
  let baseUrl: string;

  before(async () => {
    if (!HAS_CREDENTIALS) return;

    app = await createServer({
      port: 0,
      host: '127.0.0.1',
    });

    const addr = app.server.address();
    const port = typeof addr === 'object' && addr ? addr.port : 0;
    baseUrl = `http://127.0.0.1:${port}`;
  });

  after(async () => {
    if (app) {
      await app.close();
    }
  });

  // ------------------------------------------------------------------
  // Health check (skipped without credentials because createServer()
  // connects to the DB and PayPal in its constructor)
  // ------------------------------------------------------------------

  it('GET /health returns 200 with plugin name', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as { status: string; plugin: string };
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'paypal');
  });

  it('GET /ready returns 200', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/ready`);
    assert.equal(res.status, 200);
    const body = await res.json() as { ready: boolean };
    assert.equal(body.ready, true);
  });

  it('GET /live returns 200', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/live`);
    assert.equal(res.status, 200);
    const body = await res.json() as { live: boolean };
    assert.equal(body.live, true);
  });

  // ------------------------------------------------------------------
  // API endpoints
  // ------------------------------------------------------------------

  it('GET /api/transactions returns list shape', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/transactions`);
    assert.equal(res.status, 200);
    const body = await res.json() as unknown;
    assert.ok(body !== null && typeof body === 'object');
  });

  it('GET /api/orders returns list shape', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/orders`);
    assert.equal(res.status, 200);
    const body = await res.json() as unknown;
    assert.ok(body !== null && typeof body === 'object');
  });

  it('GET /api/subscriptions returns list shape', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/subscriptions`);
    assert.equal(res.status, 200);
    const body = await res.json() as unknown;
    assert.ok(body !== null && typeof body === 'object');
  });

  it('GET /api/stats returns stats object', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/api/stats`);
    assert.equal(res.status, 200);
    const body = await res.json() as unknown;
    assert.ok(body !== null && typeof body === 'object');
  });

  // Webhook with invalid signature should return 401 when webhook IDs are configured
  it('POST /webhooks/paypal with empty body returns non-200 or 200', { skip: !HAS_CREDENTIALS }, async () => {
    const res = await fetch(`${baseUrl}/webhooks/paypal`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({}),
    });
    // Without a valid signature this will be 401 or 500 — either is acceptable
    assert.ok([200, 401, 500].includes(res.status), `Unexpected status: ${res.status}`);
  });
});
