/**
 * media-processing plugin tests
 * Uses node:test + node:assert (zero external dependencies)
 *
 * createServer() returns a Fastify instance directly.
 * We listen on port 0 and read the assigned port from app.server.address().
 */

import test from 'node:test';
import assert from 'node:assert/strict';
import { AddressInfo } from 'node:net';

const SKIP_DB = !process.env.POSTGRES_HOST;

test('media-processing plugin', { skip: SKIP_DB ? 'POSTGRES_HOST not set' : false }, async (t) => {
  const { createServer } = await import('../src/server.js');

  const { app } = await createServer({ port: 0, host: '127.0.0.1' });

  await app.listen({ port: 0, host: '127.0.0.1' });
  const { port } = app.server.address() as AddressInfo;
  const BASE = `http://127.0.0.1:${port}`;

  try {
    await t.test('GET /health returns 200 with plugin name', async () => {
      const res = await fetch(`${BASE}/health`);
      assert.equal(res.status, 200);
      const body = await res.json() as Record<string, unknown>;
      assert.equal(body.status, 'ok');
      assert.equal(body.plugin, 'media-processing');
    });

    await t.test('GET /v1/status returns running status', async () => {
      const res = await fetch(`${BASE}/v1/status`);
      assert.equal(res.status, 200);
      const body = await res.json() as Record<string, unknown>;
      assert.equal(body.status, 'running');
      assert.equal(body.plugin, 'media-processing');
    });

    await t.test('GET /v1/profiles returns encoding profiles', async () => {
      const res = await fetch(`${BASE}/v1/profiles`);
      assert.equal(res.status, 200);
      const body = await res.json() as { success: boolean; data: unknown[] };
      assert.equal(body.success, true);
      assert.ok(Array.isArray(body.data));
    });

    await t.test('GET /v1/jobs returns job list', async () => {
      const res = await fetch(`${BASE}/v1/jobs`);
      assert.equal(res.status, 200);
      const body = await res.json() as { success: boolean; data: unknown[] };
      assert.equal(body.success, true);
      assert.ok(Array.isArray(body.data));
    });

    await t.test('GET /v1/jobs/:id returns 404 for unknown job', async () => {
      const res = await fetch(`${BASE}/v1/jobs/00000000-0000-0000-0000-000000000000`);
      assert.equal(res.status, 404);
      const body = await res.json() as { success: boolean };
      assert.equal(body.success, false);
    });

    await t.test('POST /v1/analyze with missing url returns 400', async () => {
      const res = await fetch(`${BASE}/v1/analyze`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      });
      assert.equal(res.status, 400);
      const body = await res.json() as { success: boolean };
      assert.equal(body.success, false);
    });

    await t.test('GET /v1/watcher/status returns watcher state', async () => {
      const res = await fetch(`${BASE}/v1/watcher/status`);
      assert.equal(res.status, 200);
      const body = await res.json() as { success: boolean };
      assert.equal(body.success, true);
    });

    await t.test('GET /v1/stats returns stats', async () => {
      const res = await fetch(`${BASE}/v1/stats`);
      assert.equal(res.status, 200);
      const body = await res.json() as { success: boolean };
      assert.equal(body.success, true);
    });
  } finally {
    await app.close();
  }
});
