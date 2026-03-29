/**
 * Jobs Plugin Tests
 *
 * Uses Node.js built-in test runner (node:test) and native fetch.
 *
 * server.ts is a self-starting module that requires Redis + database at import time.
 * Tests here build a minimal Fastify server directly using JobsDatabase, bypassing
 * the BullMQ/Redis dependency so the database layer can be tested in isolation.
 *
 * Tests are skipped when POSTGRES_HOST is not set.
 *
 * Run: NODE_OPTIONS='--experimental-strip-types' tsx --test tests/*.test.ts
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import Fastify from 'fastify';
import cors from '@fastify/cors';
import { JobsDatabase } from '../src/database.js';
import { loadConfig } from '../src/config.js';
import { getAppContext } from '@nself/plugin-utils';

const DB_AVAILABLE = Boolean(process.env.POSTGRES_HOST);

/** Build a minimal Fastify app that wires up the Jobs database without Redis/BullMQ */
async function buildTestServer() {
  const config = loadConfig();
  const db = new JobsDatabase(config);
  await db.connect();
  await db.initializeSchema();

  const app = Fastify({ logger: false });
  await app.register(cors, { origin: true, credentials: true });

  app.decorateRequest('scopedDb', null);
  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    (request as unknown as Record<string, unknown>).scopedDb = db.forSourceAccount(ctx.sourceAccountId);
  });

  function scopedDb(request: unknown): JobsDatabase {
    return (request as Record<string, unknown>).scopedDb as JobsDatabase;
  }

  app.get('/health', async () => ({
    status: 'ok',
    plugin: 'jobs',
    timestamp: new Date().toISOString(),
  }));

  app.get('/api/stats', async (request) => {
    return await scopedDb(request).getStats();
  });

  app.get<{ Params: { id: string } }>('/api/jobs/:id', async (request, reply) => {
    const job = await scopedDb(request).getJobByBullMQId(request.params.id);
    if (!job) {
      return reply.status(404).send({ error: 'Job not found' });
    }
    return job;
  });

  return { app, db };
}

async function withServer(fn: (baseUrl: string) => Promise<void>) {
  const { app, db } = await buildTestServer();
  await app.listen({ port: 0, host: '127.0.0.1' });
  const port = (app.server.address() as { port: number }).port;
  const baseUrl = `http://127.0.0.1:${port}`;
  try {
    await fn(baseUrl);
  } finally {
    await app.close().catch(() => undefined);
    await db.disconnect().catch(() => undefined);
  }
}

test('GET /health returns 200 with { status: "ok" }', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'jobs');
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

test('GET /api/stats returns job statistics object', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/api/stats`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body === 'object' && body !== null, 'stats should be an object');
  });
});

test('GET /api/jobs/:id returns 404 for unknown job id', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/api/jobs/nonexistent-bullmq-id-xyz`);
    assert.equal(res.status, 404);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});
