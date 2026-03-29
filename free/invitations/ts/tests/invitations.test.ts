/**
 * Invitations Plugin Tests
 *
 * Uses Node.js built-in test runner (node:test) and native fetch.
 * Tests that require a database are skipped when POSTGRES_HOST is not set.
 *
 * createServer returns { ...fastifyApp, start } — the Fastify instance is
 * spread into the return value, so server.server.address() gives the port.
 *
 * Run: NODE_OPTIONS='--experimental-strip-types' tsx --test tests/*.test.ts
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from '../src/server.js';

const DB_AVAILABLE = Boolean(process.env.POSTGRES_HOST);

async function withServer(fn: (baseUrl: string) => Promise<void>) {
  const server = await createServer({ port: 0, host: '127.0.0.1' });
  // createServer returns { ...fastifyApp, start } — call start() to bind port
  await server.start();
  // The Fastify app is spread, so server.server is the Node.js http.Server
  const addr = server.server.address() as { port: number };
  const baseUrl = `http://127.0.0.1:${addr.port}`;
  try {
    await fn(baseUrl);
  } finally {
    await server.close().catch(() => undefined);
  }
}

test('GET /health returns 200 with { status: "ok" }', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.equal(body.plugin, 'invitations');
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

test('POST /v1/invitations returns 400 when inviter_id is missing', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/invitations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        invitee_email: 'user@example.com',
        type: 'member',
        channel: 'email',
      }),
    });
    assert.equal(res.status, 400);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});

test('GET /v1/invitations returns paginated list', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/invitations`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.data), 'data should be an array');
    assert.ok(typeof body.total === 'number', 'total should be a number');
  });
});

test('GET /v1/validate/:code returns 404 for unknown code', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/validate/totally-invalid-code-xyz`);
    assert.equal(res.status, 404);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.valid, false);
    assert.ok(typeof body.error === 'string');
  });
});
