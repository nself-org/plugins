import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../dist-test/index.mjs';
import { fakeKv, fakeCtx, req, sampleCatalog } from './test-helpers.mjs';

const originalFetch = globalThis.fetch;
const originalCaches = globalThis.caches;
afterEach(() => {
  globalThis.fetch = originalFetch;
  globalThis.caches = originalCaches;
});

// Clone on both sides, as the Workers Cache API consumes response bodies.
function installCache() {
  const store = new Map();
  globalThis.caches = { default: {
    async match(request) { return store.get(request.url)?.clone(); },
    async put(request, response) { store.set(request.url, response.clone()); },
    async delete(request) { return store.delete(request.url); },
  } };
  return store;
}

async function call(path, env, init) {
  const ctx = fakeCtx();
  const response = await worker.fetch(req(path, init), env, ctx);
  await ctx.settle();
  return response;
}

test('D-0902: public reads cache and throttle without KV mutations, even at the daily cap', async () => {
  installCache();
  const mutations = [];
  const kv = fakeKv();
  kv.put = async (key) => { mutations.push(['put', key]); throw new Error('daily operation limit exceeded'); };
  kv.delete = async (key) => { mutations.push(['delete', key]); throw new Error('daily operation limit exceeded'); };
  let fetches = 0;
  globalThis.fetch = async (url) => {
    fetches++;
    return new Response(JSON.stringify(String(url).endsWith('catalog.json')
      ? sampleCatalog() : { plugins: { jobs: { name: 'jobs', version: '1.0.0' } } }));
  };
  const env = { PLUGINS_KV: kv, MARKETPLACE_GET_RATE_LIMIT: '2' };
  for (const path of ['/registry.json', '/manifest.json', '/catalog.json', '/marketplace']) {
    assert.equal((await call(path, env)).status, 200, path);
    const before = fetches;
    assert.equal((await call(path, env)).status, 200, path);
    assert.equal(fetches, before, `${path} should reuse Cache API response`);
  }
  const limited = await call('/marketplace', env);
  assert.equal(limited.status, 429, 'GET limit must work without RATINGS_KV');
  assert.ok(Number(limited.headers.get('Retry-After')) >= 1);
  for (let i = 0; i < 5; i++) {
    assert.equal((await call('/marketplace/ratings/jobs', env, { method: 'POST', body: '{}' })).status, 400);
  }
  assert.equal((await call('/marketplace/ratings/jobs', env, { method: 'POST', body: '{}' })).status, 429);
  assert.deepEqual(mutations, [], 'reads and rejected submissions must never put/delete KV');
});
