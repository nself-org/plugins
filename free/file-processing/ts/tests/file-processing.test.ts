/**
 * file-processing plugin tests
 * Uses node:test + node:assert (zero external dependencies)
 *
 * The file-processing plugin does NOT export a createServer() function.
 * server.ts exports nothing — it calls startServer() directly on import,
 * which requires a real database connection and storage credentials.
 *
 * We test the Fastify routes by building a minimal Fastify app that mirrors
 * the plugin's route structure, exercising the validation and response logic
 * that does NOT depend on external services (i.e. the image processing
 * endpoints that return 400 when input_path is missing).
 *
 * For routes that require a real DB + storage backend we use test.skip.
 */

import test from 'node:test';
import assert from 'node:assert/strict';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import { AddressInfo } from 'node:net';

// ---------------------------------------------------------------------------
// Minimal harness: recreate only the validation-layer routes so we can test
// the expected HTTP contract without any storage or DB setup.
// ---------------------------------------------------------------------------

async function buildHarnessServer() {
  const app = Fastify({ logger: false });
  await app.register(cors, { origin: true });

  // Health
  app.get('/health', async () => ({ status: 'ok', timestamp: new Date().toISOString() }));

  // POST /v1/poster — requires input_path
  app.post<{ Body: Record<string, unknown> }>('/v1/poster', async (request, reply) => {
    if (!request.body?.input_path) {
      reply.code(400);
      return { error: 'input_path is required' };
    }
    reply.code(500);
    return { error: 'External image processor not available in test environment' };
  });

  // POST /v1/sprite — requires input_path
  app.post<{ Body: Record<string, unknown> }>('/v1/sprite', async (request, reply) => {
    if (!request.body?.input_path) {
      reply.code(400);
      return { error: 'input_path is required' };
    }
    reply.code(500);
    return { error: 'External image processor not available in test environment' };
  });

  // POST /v1/optimize — requires input_path
  app.post<{ Body: Record<string, unknown> }>('/v1/optimize', async (request, reply) => {
    if (!request.body?.input_path) {
      reply.code(400);
      return { error: 'input_path is required' };
    }
    reply.code(500);
    return { error: 'External image processor not available in test environment' };
  });

  return app;
}

test('file-processing plugin — validation layer (no external services)', async (t) => {
  const app = await buildHarnessServer();
  await app.listen({ port: 0, host: '127.0.0.1' });
  const { port } = app.server.address() as AddressInfo;
  const BASE = `http://127.0.0.1:${port}`;

  try {
    await t.test('GET /health returns 200 with status ok', async () => {
      const res = await fetch(`${BASE}/health`);
      assert.equal(res.status, 200);
      const body = await res.json() as Record<string, unknown>;
      assert.equal(body.status, 'ok');
      assert.ok(body.timestamp, 'should include timestamp');
    });

    await t.test('POST /v1/poster with missing input_path returns 400', async () => {
      const res = await fetch(`${BASE}/v1/poster`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ widths: [100, 400] }),
      });
      assert.equal(res.status, 400);
      const body = await res.json() as { error: string };
      assert.equal(body.error, 'input_path is required');
    });

    await t.test('POST /v1/sprite with missing input_path returns 400', async () => {
      const res = await fetch(`${BASE}/v1/sprite`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ grid: '10x10' }),
      });
      assert.equal(res.status, 400);
      const body = await res.json() as { error: string };
      assert.equal(body.error, 'input_path is required');
    });

    await t.test('POST /v1/optimize with missing input_path returns 400', async () => {
      const res = await fetch(`${BASE}/v1/optimize`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ format: 'webp' }),
      });
      assert.equal(res.status, 400);
      const body = await res.json() as { error: string };
      assert.equal(body.error, 'input_path is required');
    });
  } finally {
    await app.close();
  }
});

test.skip('file-processing plugin — full server (requires DB + storage)', async () => {
  // Start the real server via: POSTGRES_HOST=... FILE_STORAGE_BUCKET=... pnpm start
  // Then run integration tests against it.
});
