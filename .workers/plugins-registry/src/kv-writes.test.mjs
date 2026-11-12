/**
 * kv-writes.test.mjs — KV write and delete budget (D-1543)
 *
 * Counts every call on a fake KV namespace. Public requests make no KV write
 * or delete, on cold or warm caches and on KV misses. POST /api/sync writes
 * each synced key at most once, writes nothing for identical content, and a
 * burst of concurrent syncs writes once. Revocations read correctly with a
 * cold cache. No network, no real token: fetch and caches are fakes.
 *
 * Run: pnpm test  (from .workers/plugins-registry)
 */

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import { fakeKv, fakeCtx, installCache, readCache, contentsResponse, sampleCatalog, req } from './test-helpers.mjs';

const originalFetch = globalThis.fetch;
const originalCaches = globalThis.caches;
afterEach(() => {
  globalThis.fetch = originalFetch;
  globalThis.caches = originalCaches;
});

const SYNC = { method: 'POST', headers: { Authorization: 'Bearer fake-sync-token' } };
const SYNCED_KEYS = ['catalog:json', 'registry:free', 'registry:pro'];

/** fakeKv that records every get, put and delete (with put options). */
function countingKv(seed = {}) {
  const kv = fakeKv(seed);
  const calls = { get: [], put: [], delete: [] };
  const { get, put, delete: del } = kv;
  kv.get = async (key, type) => { calls.get.push(key); return get(key, type); };
  kv.put = async (key, value, opts) => { calls.put.push([key, opts]); return put(key, value); };
  kv.delete = async (key) => { calls.delete.push(key); return del(key); };
  kv.calls = calls;
  kv.reset = () => { calls.get.length = 0; calls.put.length = 0; calls.delete.length = 0; };
  return kv;
}

const putKeys = (kv) => kv.calls.put.map(([k]) => k).sort();

function docs(version = '1.2.0') {
  return {
    free: { plugins: { jobs: { name: 'jobs', version, tier: 'free', category: 'infrastructure' } } },
    pro: { plugins: [{ name: 'ai', version: '2.0.0', tier: 'max', category: 'ai' }] },
    catalog: sampleCatalog(),
  };
}

/** Fake GitHub: Contents API and raw URLs for the free registry, pro registry and catalog. */
function stubGitHub(d) {
  const calls = [];
  globalThis.fetch = async (url) => {
    const u = String(url);
    calls.push(u);
    if (u.includes('/repos/nself-org/plugins/contents/registry.json')) return contentsResponse(d.free);
    if (u.includes('/repos/nself-org/bundles/contents/registry.json')) return contentsResponse(d.pro);
    if (u.includes('/repos/nself-org/plugins/contents/catalog.json')) return contentsResponse(d.catalog);
    if (u.endsWith('/main/registry.json')) return new Response(JSON.stringify(d.free));
    if (u.endsWith('/main/catalog.json')) return new Response(JSON.stringify(d.catalog));
    return new Response('not found', { status: 404 });
  };
  return calls;
}

const envFor = (kv, extra = {}) => ({
  PLUGINS_KV: kv, GH_ACCESS_TOKEN: 'fake-gh-token', GITHUB_SYNC_TOKEN: 'fake-sync-token', LICENSED_REF: 'v0.0.0-fixture', ...extra,
});

async function call(path, env, init) {
  const ctx = fakeCtx();
  const res = await worker.fetch(req(path, init), env, ctx);
  await ctx.settle();
  return res;
}

const PUBLIC_PATHS = [
  '/registry.json', '/plugins', '/registry/list', '/plugins/jobs', '/plugins/jobs/tarball', '/manifest.json',
  '/categories', '/catalog.json', '/bundles.json', '/marketplace', '/marketplace/ratings/jobs',
  '/plugins/revocations', '/.well-known/revoked-authors.json', '/stats', '/health',
];

test('N public requests on cold and warm caches make 0 KV writes and 0 deletes', async () => {
  for (const seeded of [false, true]) {
    const kv = countingKv();
    stubGitHub(docs());
    installCache();
    if (seeded) await call('/api/sync', envFor(kv), SYNC);
    kv.reset();
    let requests = 0;
    for (let round = 0; round < 20; round++) {
      if (round % 2 === 0) installCache(); // a cold data center every other round
      for (const path of PUBLIC_PATHS) {
        const res = await call(path, envFor(kv), { headers: { 'CF-Connecting-IP': `198.51.100.${round}` } });
        assert.ok(res.status < 500 || path === '/bundles.json', `${path}: ${res.status}`);
        requests++;
      }
    }
    assert.equal(requests, 20 * PUBLIC_PATHS.length);
    assert.deepEqual(kv.calls.put, [], `KV ${seeded ? 'synced' : 'empty'}: no writes`);
    assert.deepEqual(kv.calls.delete, [], `KV ${seeded ? 'synced' : 'empty'}: no deletes`);
  }
});

test('a KV miss fetches GitHub, serves it and fills the Cache API only', async () => {
  const kv = countingKv();
  const calls = stubGitHub(docs());
  installCache();
  const res = await call('/registry.json', envFor(kv));
  assert.equal(res.status, 200);
  assert.ok(calls.length > 0, 'GitHub was fetched');
  assert.ok(kv.calls.get.includes('registry:free'), 'KV was consulted first');
  assert.deepEqual(kv.calls.put, []);
  assert.ok((await readCache('registry:free'))?.data, 'Cache API filled');
});

test('a sync writes each synced key once, with a TTL in hours, and never deletes', async () => {
  const kv = countingKv({ 'stats:global': { registryHits: 3, registryFetches: 4, lastSync: null } });
  stubGitHub(docs());
  installCache();
  const res = await call('/api/sync', envFor(kv), SYNC);
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.deepEqual(Object.keys(body), [
    'success', 'message', 'freeCount', 'proCount', 'totalCount', 'freeError', 'proError', 'catalogSynced', 'timestamp',
  ]);
  assert.equal(body.catalogSynced, true);
  assert.deepEqual(putKeys(kv), SYNCED_KEYS);
  for (const [, opts] of kv.calls.put) assert.ok(opts.expirationTtl >= 3600, 'TTL in hours');
  assert.deepEqual(kv.calls.delete, []);
});

test('an identical second sync writes nothing; a changed registry rewrites only its key', async () => {
  const kv = countingKv();
  stubGitHub(docs());
  installCache();
  await call('/api/sync', envFor(kv), SYNC);
  kv.reset();
  assert.equal((await call('/api/sync', envFor(kv), SYNC)).status, 200);
  assert.deepEqual(kv.calls.put, [], 'identical content: no writes');
  stubGitHub(docs('1.3.0'));
  await call('/api/sync', envFor(kv), SYNC);
  assert.deepEqual(putKeys(kv), ['registry:free']);
  assert.deepEqual(kv.calls.delete, []);
});

test('a burst of concurrent syncs writes each key once', async () => {
  const kv = countingKv();
  stubGitHub(docs());
  installCache();
  const results = await Promise.all(Array.from({ length: 5 }, () => call('/api/sync', envFor(kv), SYNC)));
  for (const res of results) assert.equal(res.status, 200);
  assert.deepEqual(putKeys(kv), SYNCED_KEYS);
});

test('another data center serves the synced copy from KV within CACHE_TTL, without GitHub', async () => {
  const kv = countingKv();
  stubGitHub(docs('1.4.0'));
  installCache();
  await call('/api/sync', envFor(kv), SYNC);
  installCache(); // a different colo: empty Cache API
  globalThis.fetch = async () => { throw new Error('network must not be used'); };
  const res = await call('/plugins/jobs', envFor(kv));
  assert.equal(res.status, 200);
  assert.equal((await res.json()).version, '1.4.0');
  const edge = await readCache('registry:free');
  const freshFor = 300 - (Date.now() - edge.timestamp) / 1000;
  assert.ok(freshFor > 230 && freshFor <= 240, `KV fill fresh for ${freshFor}s, so KV lag + edge <= CACHE_TTL`);
  const catalog = await call('/catalog.json', envFor(kv));
  assert.equal(catalog.status, 200);
  assert.ok(Number(/max-age=(\d+)/.exec(catalog.headers.get('Cache-Control'))[1]) <= 240);
  assert.deepEqual(kv.calls.put.length, 3, 'only the sync wrote');
});

test('a KV registry copy written by the retired index.js (no hash) is ignored, not served', async () => {
  const kv = countingKv({ 'registry:free': { data: docs('0.0.1').free, timestamp: Date.now() } });
  stubGitHub(docs('1.5.0'));
  installCache();
  const res = await call('/plugins/jobs', envFor(kv));
  assert.equal((await res.json()).version, '1.5.0');
  assert.deepEqual(kv.calls.put, []);
});

test('a cold revocation read is correct: tarball 410 and both lists, nothing written', async () => {
  const kv = countingKv({
    'revocations:list': [{ name: 'jobs', version: '1.2.0', revokedAt: '2026-11-01T00:00:00.000Z' }],
    'revocations:authors': [{ authorKey: 'evil', revokedAt: '2026-11-01T00:00:00.000Z' }],
  });
  stubGitHub(docs());
  installCache();
  await call('/api/sync', envFor(kv), SYNC);
  kv.reset();
  installCache(); // cold colo
  globalThis.fetch = async () => { throw new Error('network must not be used'); };
  const tarball = await call('/plugins/jobs/tarball', envFor(kv));
  assert.equal(tarball.status, 410);
  const list = await (await call('/plugins/revocations', envFor(kv))).json();
  assert.deepEqual(list.revoked.map((r) => `${r.name}@${r.version}`), ['jobs@1.2.0']);
  const authors = await (await call('/.well-known/revoked-authors.json', envFor(kv))).json();
  assert.equal(authors.count, 1);
  assert.deepEqual(kv.calls.put, []);
});

test('/stats: lastSync is the newest content-changing sync; counters stay the legacy totals; no write', async () => {
  const kv = countingKv({ 'stats:global': { registryHits: 11, registryFetches: 7, lastSync: '2026-10-01T00:00:00.000Z' } });
  stubGitHub(docs());
  installCache();
  const before = Date.now();
  await call('/api/sync', envFor(kv), SYNC);
  kv.reset();
  const stats = await (await call('/stats', envFor(kv))).json();
  assert.deepEqual(Object.keys(stats).sort(), ['lastSync', 'registryFetches', 'registryHits']);
  assert.equal(stats.registryHits, 11);
  assert.equal(stats.registryFetches, 7);
  assert.ok(Date.parse(stats.lastSync) >= before);
  assert.deepEqual(kv.calls.put, []);
});

test('the rate limit binding decides when bound; no counter is cached and nothing is written', async () => {
  const kv = countingKv();
  stubGitHub(docs());
  const cache = installCache();
  const keys = [];
  let allow = true;
  const binding = { limit: async ({ key }) => { keys.push(key); return { success: allow }; } };
  const env = envFor(kv, { MARKETPLACE_GET_LIMITER: binding, MARKETPLACE_POST_LIMITER: binding, MARKETPLACE_GET_RATE_LIMIT: '1' });
  const ip = { headers: { 'CF-Connecting-IP': '203.0.113.50' } };
  assert.equal((await call('/marketplace', env, ip)).status, 200);
  assert.equal((await call('/marketplace', env, ip)).status, 200, 'binding, not the fallback limit of 1');
  allow = false;
  const limited = await call('/marketplace/ratings/jobs', env, ip);
  assert.equal(limited.status, 429);
  assert.ok(Number(limited.headers.get('Retry-After')) >= 1);
  assert.equal((await call('/marketplace/ratings/jobs', env, { method: 'POST', body: '{}', ...ip })).status, 429);
  assert.ok(keys.every((k) => k === '203.0.113.50'));
  assert.ok(![...cache.store.keys()].some((u) => u.includes('rl%3A')), 'no Cache API counters');
  assert.deepEqual(kv.calls.put, []);
});
