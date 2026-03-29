/**
 * Content Progress Plugin Tests
 *
 * Uses Node.js built-in test runner (node:test) and native fetch.
 * Tests that require a database are skipped when POSTGRES_HOST is not set.
 *
 * createServer returns { app, db, start, stop }.
 * Call start() to listen, then use app.server.address() to get the port.
 *
 * Run: NODE_OPTIONS='--experimental-strip-types' tsx --test tests/*.test.ts
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from '../src/server.js';

const DB_AVAILABLE = Boolean(process.env.POSTGRES_HOST);

async function withServer(fn: (baseUrl: string) => Promise<void>) {
  const server = await createServer({ port: 0, host: '127.0.0.1' });
  await server.start();
  const port = (server.app.server.address() as { port: number }).port;
  const baseUrl = `http://127.0.0.1:${port}`;
  try {
    await fn(baseUrl);
  } finally {
    await server.stop().catch(() => undefined);
  }
}

test('GET /health returns 200 with { status: "ok" }', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'content-progress');
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

test('GET /v1/progress/:userId returns user progress array', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/progress/test-user-001`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.data), 'data should be an array');
    assert.ok(typeof body.limit === 'number', 'limit should be a number');
    assert.ok(typeof body.offset === 'number', 'offset should be a number');
  });
});

test('GET /v1/progress/:userId/:contentType/:contentId returns 404 for unknown entry', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/progress/test-user-001/movie/no-such-content-id`);
    assert.equal(res.status, 404);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});

test('POST /v1/progress returns 400 when body fails schema validation', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/progress`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    assert.equal(res.status, 400);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});
