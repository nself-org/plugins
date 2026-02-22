/**
 * Content Acquisition Plugin Tests
 *
 * Uses Node.js built-in test runner (node:test) and native fetch.
 * Tests require DATABASE_URL and related env vars — skipped when not available.
 *
 * ContentAcquisitionServer uses a class-based pattern with initialize() + start().
 * We instantiate it directly with a test port to run against a real server.
 */

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer as createNetServer } from 'node:net';
import { ContentAcquisitionServer } from '../src/server.js';
import { ContentAcquisitionDatabase } from '../src/database.js';
import type { ContentAcquisitionConfig } from '../src/types.js';

// All required env vars must be present for integration tests
const ENV_AVAILABLE = Boolean(
  process.env.DATABASE_URL &&
  process.env.METADATA_ENRICHMENT_URL &&
  process.env.TORRENT_MANAGER_URL
);

/** Find a free port by opening and immediately closing a server */
function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = createNetServer();
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address();
      const port = typeof addr === 'object' && addr !== null ? addr.port : 0;
      srv.close((err) => {
        if (err) reject(err);
        else resolve(port);
      });
    });
  });
}

async function withServer(fn: (baseUrl: string) => Promise<void>) {
  const port = await getFreePort();

  const config: ContentAcquisitionConfig = {
    database_url: process.env.DATABASE_URL!,
    metadata_enrichment_url: process.env.METADATA_ENRICHMENT_URL!,
    torrent_manager_url: process.env.TORRENT_MANAGER_URL!,
    port,
    rss_check_interval: 60,
    max_concurrent_downloads: 2,
    download_path: '/tmp/test-downloads',
    temp_path: '/tmp/test-temp',
    pipeline_timeout_seconds: 30,
    retry_max_attempts: 3,
    retry_delay_seconds: 1,
  } as unknown as ContentAcquisitionConfig;

  const database = new ContentAcquisitionDatabase(config.database_url);
  await database.initialize();

  const server = new ContentAcquisitionServer(config, database);
  await server.initialize();
  await server.start();

  const baseUrl = `http://127.0.0.1:${port}`;
  try {
    await fn(baseUrl);
  } finally {
    await server.stop();
    await database.close().catch(() => undefined);
  }
}

test('GET /health returns 200 with { status: "ok" }', { skip: !ENV_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/health`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.equal(body.status, 'ok');
    assert.ok(typeof body.timestamp === 'string');
  });
});

test('GET /v1/subscriptions returns subscriptions list', { skip: !ENV_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/subscriptions`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.subscriptions), 'subscriptions should be an array');
  });
});

test('GET /v1/queue returns queue list', { skip: !ENV_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/queue`);
    assert.equal(res.status, 200);
    const body = await res.json() as Record<string, unknown>;
    assert.ok(Array.isArray(body.queue), 'queue should be an array');
  });
});

test('POST /v1/subscriptions returns 400 when required fields are missing', { skip: !ENV_AVAILABLE }, async () => {
  await withServer(async (baseUrl) => {
    const res = await fetch(`${baseUrl}/v1/subscriptions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    // Schema validation should reject missing required fields
    assert.ok(res.status === 400 || res.status === 422, `expected 4xx, got ${res.status}`);
  });
});
