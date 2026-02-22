/**
 * GitHub Plugin Tests
 *
 * Uses Node.js built-in test runner (node:test) and native fetch.
 * Tests that require a database are skipped when POSTGRES_HOST is not set.
 * The GITHUB_TOKEN must be a valid-format PAT (ghp_* or github_pat_*).
 *
 * Run: NODE_OPTIONS='--experimental-strip-types' tsx --test tests/*.test.ts
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from '../src/server.js';

const DB_AVAILABLE = Boolean(process.env.POSTGRES_HOST && process.env.GITHUB_TOKEN);

// GITHUB_TOKEN must be a valid-format PAT — config throws otherwise
const TEST_TOKEN = process.env.GITHUB_TOKEN ?? 'ghp_testplaceholderfortests000000000000';

async function withServer(fn: (baseUrl: string) => Promise<void>) {
  const server = await createServer({
    port: 0,
    host: '127.0.0.1',
    githubToken: TEST_TOKEN,
    githubOrg: 'test-org',
    githubWebhookSecret: 'test-webhook-secret',
  });
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
    assert.equal(body.plugin, 'github');
    assert.ok(typeof body.timestamp === 'string', 'timestamp should be a string');
  });
});

test('GET /health timestamp is a valid ISO date', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(!isNaN(new Date(body.timestamp as string).getTime()));
  });
});

test('GET /api/repos returns paginated data with total', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/api/repos`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.data), 'data should be an array');
    assert.ok(typeof body.total === 'number', 'total should be a number');
    assert.ok(typeof body.limit === 'number', 'limit should be a number');
    assert.ok(typeof body.offset === 'number', 'offset should be a number');
  });
});

test('GET /api/repos/:fullName returns 404 for unknown repo', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/api/repos/nonexistent-org%2Fno-such-repo`);
    assert.equal(res.status, 404);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});

test('POST /webhooks/github returns 400 when event headers are missing', { skip: !DB_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/webhooks/github`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      // intentionally omitting x-github-event and x-github-delivery
      body: JSON.stringify({ action: 'opened' }),
    });
    assert.equal(res.status, 400);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(typeof body.error === 'string');
  });
});
