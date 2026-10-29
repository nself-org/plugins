import { test, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import { registryKv, fakeCtx, req, sampleCatalog } from './test-helpers.mjs';

const originalCaches = globalThis.caches;
const originalFetch = globalThis.fetch;
let env;
beforeEach(() => {
  env = { PLUGINS_KV: registryKv() };
  globalThis.fetch = async () => new Response(JSON.stringify(sampleCatalog()));
});
afterEach(() => {
  globalThis.caches = originalCaches;
  globalThis.fetch = originalFetch;
});

async function call(path, method, headers) {
  const ctx = fakeCtx();
  const response = await worker.fetch(req(path, { method, headers }), env, ctx);
  await ctx.settle();
  return response;
}

const paths = [
  '/', '/registry', '/registry.json', '/registry/list', '/plugins',
  '/plugins/revocations', '/plugins/jobs', '/plugins/jobs/1.2.0',
  '/plugins/jobs/latest', '/plugins/jobs/tarball', '/plugins/jobs/signature',
  '/categories', '/manifest.json', '/health', '/stats',
  '/.well-known/revoked-authors.json', '/marketplace',
  '/marketplace/ratings/jobs', '/bundles.json', '/bundles-schema.json',
  '/catalog.json', '/v1/catalog.json', '/nope', '/plugins/missing',
  '/api/sync', '/plugins/jobs/install-event',
];

for (const path of paths) {
  test(`HEAD ${path} preserves GET status and headers without a body`, async () => {
    await call(path, 'GET'); // Compare both methods against the same warm cache.
    const get = await call(path, 'GET');
    const head = await call(path, 'HEAD');
    assert.equal(head.status, get.status);
    assert.equal(head.statusText, get.statusText);
    assert.deepEqual([...head.headers], [...get.headers]);
    assert.equal(head.body, null);
    assert.equal(await head.text(), '');
  });
}

test('HEAD catalog preserves ETag and conditional 304', async () => {
  const get = await call('/catalog.json', 'GET');
  const etag = get.headers.get('ETag');
  assert.ok(etag);
  const head = await call('/catalog.json', 'HEAD');
  assert.equal(head.status, 200);
  assert.equal(head.headers.get('ETag'), etag);
  const unchanged = await call('/catalog.json', 'HEAD', { 'If-None-Match': etag });
  assert.equal(unchanged.status, 304);
  assert.equal(unchanged.headers.get('ETag'), etag);
  assert.equal(unchanged.body, null);
});

test('HEAD API version errors have GET metadata without a body', async () => {
  const headers = { 'X-API-Version': 'unsupported' };
  const get = await call('/catalog.json', 'GET', headers);
  const head = await call('/catalog.json', 'HEAD', headers);
  assert.equal(head.status, get.status);
  assert.deepEqual([...head.headers], [...get.headers]);
  assert.equal(head.body, null);
});
