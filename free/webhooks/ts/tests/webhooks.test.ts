/**
 * Webhooks Plugin Tests
 *
 * Uses Node.js built-in test runner (node:test) and native fetch.
 * Tests that require a database are skipped when POSTGRES_HOST is not set.
 *
 * createServer returns the Fastify app directly (no wrapper object).
 * Call app.listen({ port: 0 }) to bind to a random port.
 *
 * Run: NODE_OPTIONS='--experimental-strip-types' tsx --test tests/*.test.ts
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from '../src/server.js';

const DB_AVAILABLE = Boolean(process.env.POSTGRES_HOST);

async function withServer(fn: (baseUrl: string) => Promise<void>) {
  // createServer returns the Fastify app directly
  const app = await createServer({ port: 0, host: '127.0.0.1' });
  await app.listen({ port: 0, host: '127.0.0.1' });
  const port = (app.server.address() as { port: number }).port;
  const baseUrl = `http://127.0.0.1:${port}`;
  try {
    await fn(baseUrl);
  } finally {
    await app.close().catch(() => undefined);
  }
}

test('GET /health returns 200 with { status: "ok" }', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'webhooks');
    assert.ok(typeof body.timestamp === 'string');
  });
});

test('GET /health timestamp is a valid ISO date', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(!isNaN(new Date(body.timestamp as string).getTime()));
  });
});

test('GET /v1/endpoints returns endpoints array', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/endpoints`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.endpoints), 'endpoints should be an array');
  });
});

test('GET /v1/endpoints/:id returns 404 for unknown endpoint', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/endpoints/00000000-0000-0000-0000-000000000000`);
    assert.equal(res.status, 404);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});

test('GET /v1/event-types returns event types array', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/event-types`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.event_types), 'event_types should be an array');
  });
});
