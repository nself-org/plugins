/**
 * install-event-limit.test.mjs — POST /plugins/:name/install-event rate limit (D-1545)
 *
 * The route is public and each accepted request makes KV writes, so it shares the
 * rating route's per-IP POST limit (5 per minute). Covers the Cache API fallback and
 * the MARKETPLACE_POST_LIMITER binding: the 6th request from one IP is a 429 with a
 * Retry-After header and no KV write; another IP is unaffected; success, 400 and 404
 * responses inside the limit are unchanged. No network, no real token.
 *
 * Run: pnpm test  (from .workers/plugins-registry)
 */

import { test, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import { installCache, fakeCtx, registryKv, req } from './test-helpers.mjs';

const originalFetch = globalThis.fetch;
const originalCaches = globalThis.caches;
beforeEach(() => installCache());
afterEach(() => {
  globalThis.fetch = originalFetch;
  globalThis.caches = originalCaches;
});

/** registryKv that counts every put and delete. */
function countingKv() {
  const kv = registryKv();
  const mutations = [];
  const { put, delete: del } = kv;
  kv.put = async (key, ...rest) => { mutations.push(['put', key]); return put(key, ...rest); };
  kv.delete = async (key) => { mutations.push(['delete', key]); return del(key); };
  kv.mutations = mutations;
  return kv;
}

async function postEvent(env, ip, { plugin = 'jobs', instanceId = 'a'.repeat(64) } = {}) {
  const ctx = fakeCtx();
  const res = await worker.fetch(req(`/plugins/${plugin}/install-event`, {
    method: 'POST',
    headers: { 'CF-Connecting-IP': ip },
    body: JSON.stringify({ instanceId }),
  }), env, ctx);
  await ctx.settle();
  return res;
}

test('Cache API fallback: the 6th request from one IP is a 429 with no KV write; another IP is unaffected', async () => {
  const kv = countingKv();
  const env = { PLUGINS_KV: kv };
  for (let i = 0; i < 5; i++) {
    const res = await postEvent(env, '203.0.113.7', { instanceId: String(i).repeat(64) });
    assert.equal(res.status, 200, `request ${i + 1}`);
  }
  const writes = kv.mutations.length;
  assert.ok(writes > 0, 'accepted requests do write');
  const limited = await postEvent(env, '203.0.113.7', { instanceId: 'f'.repeat(64) });
  assert.equal(limited.status, 429);
  assert.ok(Number(limited.headers.get('Retry-After')) >= 1);
  const body = await limited.json();
  assert.deepEqual(Object.keys(body), ['error']);
  assert.match(body.error, /^Rate limit exceeded\. Retry after \d+s\.$/);
  assert.equal(kv.mutations.length, writes, 'a limited request makes no KV write');
  const other = await postEvent(env, '203.0.113.8', { instanceId: 'e'.repeat(64) });
  assert.equal(other.status, 200);
  assert.ok(kv.mutations.length > writes);
});

test('MARKETPLACE_POST_LIMITER binding decides when bound: denied is a 429 with no KV write, keyed by IP', async () => {
  const kv = countingKv();
  const denied = new Set(['203.0.113.9']);
  const keys = [];
  const binding = { limit: async ({ key }) => { keys.push(key); return { success: !denied.has(key) }; } };
  const env = { PLUGINS_KV: kv, MARKETPLACE_POST_LIMITER: binding };
  const limited = await postEvent(env, '203.0.113.9');
  assert.equal(limited.status, 429);
  assert.ok(Number(limited.headers.get('Retry-After')) >= 1);
  assert.deepEqual(kv.mutations, []);
  assert.equal((await postEvent(env, '203.0.113.10')).status, 200);
  assert.deepEqual(keys, ['203.0.113.9', '203.0.113.10']);
});

test('inside the limit, 200, 400 and 404 responses are unchanged', async () => {
  const kv = countingKv();
  const env = { PLUGINS_KV: kv };
  const ok = await postEvent(env, '203.0.113.20');
  assert.equal(ok.status, 200);
  assert.deepEqual(await ok.json(), { name: 'jobs', downloads: 1, incremented: true });
  const bad = await postEvent(env, '203.0.113.20', { instanceId: 'nothex' });
  assert.equal(bad.status, 400);
  assert.deepEqual(await bad.json(), { error: 'instanceId must be a 64-character lowercase hex string (SHA-256 hash, no PII)' });
  const missing = await postEvent(env, '203.0.113.20', { plugin: 'zz-not-a-plugin' });
  assert.equal(missing.status, 404);
  assert.deepEqual(await missing.json(), { error: 'plugin not found', name: 'zz-not-a-plugin' });
});
